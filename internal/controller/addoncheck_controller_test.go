/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/registry"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/pkg/adapter"
)

type fakeAddonAdapter struct{}

func (fakeAddonAdapter) Name() string            { return "fake-cert-manager" }
func (fakeAddonAdapter) Version() string         { return "1.2.3" }
func (fakeAddonAdapter) ContractVersion() string { return adapter.ContractVersion }
func (fakeAddonAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{"cert-manager"}, Families: []adapter.Family{"system_health"}}
}
func (f fakeAddonAdapter) Run(_ context.Context, req adapter.Request) (adapter.Result, error) {
	duration := 25 * time.Millisecond
	// Mirror real adapters: self-instrument fathom_adapter_run_duration_seconds
	// per executed family (SKA-290 / SKA-504). The controller no longer records
	// this metric, so the fake must, for the controller metrics test to observe it.
	metrics.RecordAdapterRun(f.Name(), "system_health", string(adapter.OutcomePass), duration)
	return adapter.Result{
		Duration: duration,
		Checks: []adapter.CheckResult{{
			Family:  adapter.Family("system_health"),
			Outcome: adapter.OutcomePass,
			TargetRef: adapter.TargetRef{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Namespace:  "cert-manager",
				Name:       "cert-manager",
			},
			Summary:    "cert-manager deployment is available",
			Details:    map[string]string{"available": "true"},
			ObservedAt: time.Now(),
			Duration:   10 * time.Millisecond,
		}},
	}, nil
}

// programmableAdapter is a stateful test adapter: it counts Run invocations and
// returns a configurable outcome, so tests can prove the reconciler re-ran the
// adapter (runCount) independently of whether a HealthReport was written — the
// controller only persists a report when the result changes.
type programmableAdapter struct {
	mu      sync.Mutex
	runs    int
	outcome adapter.Outcome
}

func (a *programmableAdapter) Name() string            { return "prog-cert-manager" }
func (a *programmableAdapter) Version() string         { return "0.0.1" }
func (a *programmableAdapter) ContractVersion() string { return adapter.ContractVersion }
func (a *programmableAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{"cert-manager"}, Families: []adapter.Family{"system_health"}}
}

func (a *programmableAdapter) Run(_ context.Context, _ adapter.Request) (adapter.Result, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.runs++
	outcome := a.outcome
	if outcome == "" {
		outcome = adapter.OutcomePass
	}
	return adapter.Result{
		Duration: time.Millisecond,
		Checks: []adapter.CheckResult{{
			Family:    adapter.Family("system_health"),
			Outcome:   outcome,
			TargetRef: adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "cert-manager", Name: "cert-manager"},
			Summary:   "programmed outcome",
		}},
	}, nil
}

func (a *programmableAdapter) runCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.runs
}

func (a *programmableAdapter) setOutcome(o adapter.Outcome) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.outcome = o
}

// healthReportCount returns how many HealthReports the given AddonCheck has
// produced, matched via the source-kind/name labels.
func healthReportCount(ctx context.Context, source types.NamespacedName) int {
	var reports fathomv1alpha1.HealthReportList
	ExpectWithOffset(1, k8sClient.List(ctx, &reports,
		client.InNamespace(source.Namespace),
		client.MatchingLabels{
			labelHealthReportSourceKind: "AddonCheck",
			labelHealthReportSourceName: source.Name,
		},
	)).To(Succeed())
	return len(reports.Items)
}

var _ = Describe("AddonCheck Controller", func() {
	ctx := context.Background()

	It("records accepted and paused status conditions", func() {
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-paused",
			Namespace: "default",
		}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      typeNamespacedName.Name,
				Namespace: typeNamespacedName.Namespace,
			},
			Spec: fathomv1alpha1.AddonCheckSpec{
				AddonType: "cert-manager",
				Paused:    true,
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		controllerReconciler := &AddonCheckReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))

		accepted := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionAccepted)
		Expect(accepted).NotTo(BeNil())
		Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
		Expect(accepted.Reason).To(Equal("SpecAccepted"))

		paused := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionPaused)
		Expect(paused).NotTo(BeNil())
		Expect(paused.Status).To(Equal(metav1.ConditionTrue))
		Expect(paused.Reason).To(Equal("Paused"))
	})

	It("records reconcile and adapter execution metrics (SKA-290)", func() {
		// A paused check or one with no adapter never reaches runAddonCheck,
		// so adapter metrics are exercised here with a non-paused check, a
		// registered adapter, and a policy with an enabled family — the only
		// path on which RecordAdapterRun fires (once per enabled family).
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-metrics",
			Namespace: "default",
		}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      typeNamespacedName.Name,
				Namespace: typeNamespacedName.Namespace,
			},
			Spec: fathomv1alpha1.AddonCheckSpec{
				AddonType: "cert-manager",
				Policy: map[string]fathomv1alpha1.AddonCheckFamilyPolicy{
					"system_health": {Enabled: true},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(fakeAddonAdapter{})).To(Succeed())
		controllerReconciler := &AddonCheckReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Adapters: adapters,
		}

		metrics.ReconcileTotal.Reset()
		metrics.AdapterRunDuration.Reset()
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		mfs, err := ctrlmetrics.Registry.Gather()
		Expect(err).NotTo(HaveOccurred())

		reconcileFound := false
		adapterFound := false
		familyLabelImproved := false
		for _, mf := range mfs {
			switch mf.GetName() {
			case "fathom_reconcile_total":
				for _, m := range mf.GetMetric() {
					for _, lp := range m.GetLabel() {
						if lp.GetName() == "kind" && lp.GetValue() == "AddonCheck" {
							reconcileFound = true
						}
					}
				}
			case "fathom_adapter_run_duration_seconds":
				adapterFound = true
				for _, m := range mf.GetMetric() {
					for _, lp := range m.GetLabel() {
						if lp.GetName() == "family" && lp.GetValue() != "overall" {
							familyLabelImproved = true
						}
					}
				}
			}
		}
		Expect(reconcileFound).To(BeTrue(), "expected fathom_reconcile_total series for kind=AddonCheck")
		Expect(adapterFound).To(BeTrue(), "expected fathom_adapter_run_duration_seconds to be recorded")
		Expect(familyLabelImproved).To(BeTrue(), "expected family label to be something other than the old 'overall' placeholder")
	})

	It("sets Ready false when paused", func() {
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-paused",
			Namespace: "default",
		}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      typeNamespacedName.Name,
				Namespace: typeNamespacedName.Namespace,
			},
			Spec: fathomv1alpha1.AddonCheckSpec{
				AddonType: "cert-manager",
				Paused:    true,
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		controllerReconciler := &AddonCheckReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("Paused"))
	})

	It("sets Ready false when no adapter is registered", func() {
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-missing-adapter",
			Namespace: "default",
		}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      typeNamespacedName.Name,
				Namespace: typeNamespacedName.Namespace,
			},
			Spec: fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager"},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		controllerReconciler := &AddonCheckReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Adapters: registry.New(logr.Discard()),
		}

		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("MissingAdapter"))
		Expect(ready.Message).To(ContainSubstring("cert-manager"))
	})

	It("runs a registered adapter and creates a HealthReport", func() {
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-report",
			Namespace: "default",
		}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      typeNamespacedName.Name,
				Namespace: typeNamespacedName.Namespace,
			},
			Spec: fathomv1alpha1.AddonCheckSpec{
				AddonType: "cert-manager",
				Policy: map[string]fathomv1alpha1.AddonCheckFamilyPolicy{
					"system_health": {Enabled: true, Thresholds: map[string]string{"warnDays": "14"}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(fakeAddonAdapter{})).To(Succeed())
		controllerReconciler := &AddonCheckReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Adapters: adapters,
		}

		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)))
		Expect(updated.Status.LastRunTime).NotTo(BeNil())
		Expect(updated.Status.LastReportName).NotTo(BeEmpty())

		report := &fathomv1alpha1.HealthReport{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: updated.Status.LastReportName, Namespace: typeNamespacedName.Namespace}, report)).To(Succeed())
		Expect(report.Spec.SourceRef.Name).To(Equal(typeNamespacedName.Name))
		Expect(report.Spec.AddonType).To(Equal("cert-manager"))
		Expect(report.Spec.AdapterName).To(Equal("fake-cert-manager"))
		Expect(report.Spec.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		Expect(report.Spec.Checks).To(HaveLen(1))
		Expect(report.Spec.Checks[0].Family).To(Equal("system_health"))
		Expect(report.Spec.Checks[0].Result).To(Equal(fathomv1alpha1.HealthReportResultPass))

		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal("RunCompleted"))
	})

	It("requeues a ready AddonCheck after Spec.Interval so it re-runs", func() {
		name := types.NamespacedName{Name: "addoncheck-requeue", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager", Interval: &metav1.Duration{Duration: 2 * time.Minute}},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed()) })

		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(fakeAddonAdapter{})).To(Succeed())
		r := &AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}

		result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(2 * time.Minute))

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastRunTime).NotTo(BeNil())
	})

	It("requeues after the default interval when Spec.Interval is unset", func() {
		name := types.NamespacedName{Name: "addoncheck-default-interval", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager"},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed()) })

		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(fakeAddonAdapter{})).To(Succeed())
		result, err := (&AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}).
			Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(defaultAddonCheckInterval))
	})

	It("does not requeue a paused AddonCheck", func() {
		name := types.NamespacedName{Name: "addoncheck-paused-norequeue", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager", Paused: true},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed()) })

		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(fakeAddonAdapter{})).To(Succeed())
		result, err := (&AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}).
			Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
	})

	It("re-runs once the interval has elapsed, refreshing liveness without a duplicate report", func() {
		name := types.NamespacedName{Name: "addoncheck-interval-elapsed", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager", Interval: &metav1.Duration{Duration: time.Minute}},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed()) })

		prog := &programmableAdapter{}
		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(prog)).To(Succeed())
		r := &AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}

		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(prog.runCount()).To(Equal(1))
		Expect(healthReportCount(ctx, name)).To(Equal(1))

		// Backdate the last run beyond the interval to simulate elapsed time.
		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		past := metav1.NewTime(time.Now().Add(-time.Hour))
		updated.Status.LastRunTime = &past
		Expect(k8sClient.Status().Update(ctx, updated)).To(Succeed())

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(prog.runCount()).To(Equal(2))              // the adapter re-ran
		Expect(healthReportCount(ctx, name)).To(Equal(1)) // same result -> no duplicate report
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastRunTime.Time).To(BeTemporally(">", past.Time)) // liveness refreshed
	})

	It("does not re-run within the interval but keeps requeuing", func() {
		name := types.NamespacedName{Name: "addoncheck-within-interval", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager"},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed()) })

		prog := &programmableAdapter{}
		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(prog)).To(Succeed())
		r := &AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}

		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(prog.runCount()).To(Equal(1))

		// A reconcile within the interval must not re-run, but MUST still requeue
		// one interval out — the requeue has to survive the no-status-change fast
		// path, or periodic execution stalls after the first run.
		result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(prog.runCount()).To(Equal(1))
		Expect(result.RequeueAfter).To(Equal(defaultAddonCheckInterval))
	})

	It("runs immediately when the run-now annotation changes, once per value", func() {
		name := types.NamespacedName{Name: "addoncheck-runnow", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager"},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed()) })

		prog := &programmableAdapter{}
		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(prog)).To(Succeed())
		r := &AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}

		// First reconcile: initial run.
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(prog.runCount()).To(Equal(1))

		// A new run-now value forces an out-of-band run.
		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		updated.Annotations = map[string]string{annotationRunNow: "token-1"}
		Expect(k8sClient.Update(ctx, updated)).To(Succeed())

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(prog.runCount()).To(Equal(2))

		// Same value, still within the interval: no further run.
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(prog.runCount()).To(Equal(2))

		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastRunTrigger).To(Equal("token-1"))
	})

	It("preserves a consumed run-now token across a periodic re-run", func() {
		name := types.NamespacedName{Name: "addoncheck-runnow-preserve", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name.Name,
				Namespace:   name.Namespace,
				Annotations: map[string]string{annotationRunNow: "tok"},
			},
			Spec: fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager", Interval: &metav1.Duration{Duration: time.Minute}},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed()) })

		prog := &programmableAdapter{}
		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(prog)).To(Succeed())
		r := &AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}

		// First reconcile consumes token "tok".
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(prog.runCount()).To(Equal(1))
		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastRunTrigger).To(Equal("tok"))

		// Remove the annotation and backdate the last run so the next reconcile
		// re-runs on the interval with no run-now present.
		updated.Annotations = map[string]string{}
		Expect(k8sClient.Update(ctx, updated)).To(Succeed())
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		past := metav1.NewTime(time.Now().Add(-time.Hour))
		updated.Status.LastRunTime = &past
		Expect(k8sClient.Status().Update(ctx, updated)).To(Succeed())

		// Periodic re-run must run again but NOT clear the consumed token.
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(prog.runCount()).To(Equal(2))
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastRunTrigger).To(Equal("tok"))

		// Re-applying the same, already-consumed token must NOT re-trigger, and
		// we are within the interval, so no new run happens.
		updated.Annotations = map[string]string{annotationRunNow: "tok"}
		Expect(k8sClient.Update(ctx, updated)).To(Succeed())
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(prog.runCount()).To(Equal(2))
	})

	It("refreshes the result and records a transition report when addon state changes", func() {
		name := types.NamespacedName{Name: "addoncheck-refresh", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager", Interval: &metav1.Duration{Duration: time.Minute}},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed()) })

		prog := &programmableAdapter{outcome: adapter.OutcomePass}
		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(prog)).To(Succeed())
		r := &AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}

		// Healthy: first run -> Pass, one report.
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)))
		Expect(healthReportCount(ctx, name)).To(Equal(1))

		// Addon degrades; a periodic re-run (no spec edit) must flip the result
		// and record the transition as a new HealthReport.
		prog.setOutcome(adapter.OutcomeFail)
		past := metav1.NewTime(time.Now().Add(-time.Hour))
		updated.Status.LastRunTime = &past
		Expect(k8sClient.Status().Update(ctx, updated)).To(Succeed())

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultFail)))
		Expect(healthReportCount(ctx, name)).To(Equal(2)) // transition recorded
	})

	DescribeTable("addonCheckDueForRun",
		func(setup func(*fathomv1alpha1.AddonCheck), prevGen int64, runNow string, interval time.Duration, want bool) {
			check := &fathomv1alpha1.AddonCheck{}
			check.Generation = 1
			setup(check)
			Expect(addonCheckDueForRun(check, prevGen, runNow, interval)).To(Equal(want))
		},
		Entry("first sight (no LastRunTime) is due",
			func(c *fathomv1alpha1.AddonCheck) {}, int64(1), "", time.Minute, true),
		Entry("generation change is due",
			func(c *fathomv1alpha1.AddonCheck) { n := metav1.Now(); c.Status.LastRunTime = &n }, int64(0), "", time.Minute, true),
		Entry("a new run-now trigger is due",
			func(c *fathomv1alpha1.AddonCheck) { n := metav1.Now(); c.Status.LastRunTime = &n }, int64(1), "t1", time.Minute, true),
		Entry("the same trigger within the interval is not due",
			func(c *fathomv1alpha1.AddonCheck) {
				n := metav1.Now()
				c.Status.LastRunTime = &n
				c.Status.LastRunTrigger = "t1"
			}, int64(1), "t1", time.Minute, false),
		Entry("an elapsed interval is due",
			func(c *fathomv1alpha1.AddonCheck) {
				p := metav1.NewTime(time.Now().Add(-time.Hour))
				c.Status.LastRunTime = &p
			}, int64(1), "", time.Minute, true),
		Entry("within the interval with no triggers is not due",
			func(c *fathomv1alpha1.AddonCheck) { n := metav1.Now(); c.Status.LastRunTime = &n }, int64(1), "", time.Minute, false),
	)

	DescribeTable("aggregateHealthReportResult worst-case ranking",
		func(outcomes []adapter.Outcome, want fathomv1alpha1.HealthReportResult) {
			checks := make([]adapter.CheckResult, 0, len(outcomes))
			for _, o := range outcomes {
				checks = append(checks, adapter.CheckResult{Outcome: o})
			}
			Expect(aggregateHealthReportResult(checks)).To(Equal(want))
		},
		Entry("empty input returns Skipped (adapter ran, produced no outcomes)",
			[]adapter.Outcome{}, fathomv1alpha1.HealthReportResultSkipped),
		Entry("all Pass aggregates to Pass",
			[]adapter.Outcome{adapter.OutcomePass, adapter.OutcomePass}, fathomv1alpha1.HealthReportResultPass),
		Entry("Pass+Skipped aggregates to Skipped",
			[]adapter.Outcome{adapter.OutcomePass, adapter.OutcomeSkipped}, fathomv1alpha1.HealthReportResultSkipped),
		Entry("Pass+Warn aggregates to Warn",
			[]adapter.Outcome{adapter.OutcomePass, adapter.OutcomeWarn}, fathomv1alpha1.HealthReportResultWarn),
		Entry("Pass+Fail aggregates to Fail",
			[]adapter.Outcome{adapter.OutcomePass, adapter.OutcomeFail}, fathomv1alpha1.HealthReportResultFail),
		Entry("Pass+Error aggregates to Error",
			[]adapter.Outcome{adapter.OutcomePass, adapter.OutcomeError}, fathomv1alpha1.HealthReportResultError),
		Entry("Fail+Unknown aggregates to Fail (Fail outranks Unknown)",
			[]adapter.Outcome{adapter.OutcomeFail, adapter.Outcome("synthetic-unknown")}, fathomv1alpha1.HealthReportResultFail),
		Entry("Error wins everything",
			[]adapter.Outcome{adapter.OutcomeFail, adapter.OutcomeError, adapter.OutcomeWarn}, fathomv1alpha1.HealthReportResultError),
		Entry("All Skipped aggregates to Skipped",
			[]adapter.Outcome{adapter.OutcomeSkipped, adapter.OutcomeSkipped}, fathomv1alpha1.HealthReportResultSkipped),
	)

	It("ignores deleted AddonChecks", func() {
		controllerReconciler := &AddonCheckReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("labels created HealthReports with their source kind and name", func() {
		typeNamespacedName := types.NamespacedName{Name: "addoncheck-labels", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: typeNamespacedName.Name, Namespace: typeNamespacedName.Namespace},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager"},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(fakeAddonAdapter{})).To(Succeed())
		_, err := (&AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}).
			Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		var reports fathomv1alpha1.HealthReportList
		Expect(k8sClient.List(ctx, &reports,
			client.InNamespace(typeNamespacedName.Namespace),
			client.MatchingLabels{
				"fathom.skaphos.io/source-kind": "AddonCheck",
				"fathom.skaphos.io/source-name": typeNamespacedName.Name,
			},
		)).To(Succeed())
		Expect(reports.Items).To(HaveLen(1))
		Expect(reports.Items[0].Labels["fathom.skaphos.io/source-kind"]).To(Equal("AddonCheck"))
		Expect(reports.Items[0].Labels["fathom.skaphos.io/source-name"]).To(Equal(typeNamespacedName.Name))
	})

	It("prunes HealthReports beyond Spec.HistoryLimit, oldest first", func() {
		name := "addoncheck-prune"
		ns := "default"
		limit := int32(2)
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager", HistoryLimit: &limit},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		// Seed three HealthReports above the eventual cap. metav1.Time
		// serializes at second precision (RFC3339, not Nano), so the seeds
		// may share a second among themselves. We don't care which seed
		// survives — only that the just-reconciled report does. The 2s
		// sleep between the seed batch and Reconcile guarantees the new
		// report's CreationTimestamp is strictly later (in seconds) than
		// every seed, making the oldest-first prune deterministic at the
		// new-vs-seed boundary.
		var seeded []string
		for i := 0; i < 3; i++ {
			seed := &fathomv1alpha1.HealthReport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    ns,
					GenerateName: name + "-seed-",
					Labels: map[string]string{
						"fathom.skaphos.io/source-kind": "AddonCheck",
						"fathom.skaphos.io/source-name": name,
					},
				},
				Spec: fathomv1alpha1.HealthReportSpec{
					SourceRef:  fathomv1alpha1.HealthReportTargetRef{Kind: "AddonCheck", Name: name},
					Result:     fathomv1alpha1.HealthReportResultPass,
					ObservedAt: metav1.NewTime(time.Now()),
				},
			}
			Expect(k8sClient.Create(ctx, seed)).To(Succeed())
			seeded = append(seeded, seed.Name)
		}
		time.Sleep(2 * time.Second)

		// Reconcile creates a fourth HealthReport, then prunes to limit=2.
		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(fakeAddonAdapter{})).To(Succeed())
		_, err := (&AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}).
			Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: ns}})
		Expect(err).NotTo(HaveOccurred())

		var reports fathomv1alpha1.HealthReportList
		Expect(k8sClient.List(ctx, &reports,
			client.InNamespace(ns),
			client.MatchingLabels{"fathom.skaphos.io/source-name": name},
		)).To(Succeed())
		Expect(reports.Items).To(HaveLen(int(limit)))

		// Newest survivor = the report the reconcile just created.
		var updated fathomv1alpha1.AddonCheck
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &updated)).To(Succeed())
		survivors := map[string]bool{}
		for _, r := range reports.Items {
			survivors[r.Name] = true
		}
		Expect(survivors[updated.Status.LastReportName]).To(BeTrue(), "newly created HealthReport must survive pruning")
		// Two of the three seeds must be deleted — but since seeds may share
		// a CreationTimestamp second, we cannot claim which two. The new-vs-
		// seed boundary is the only reliably ordered cut.
		seedSurvivors := 0
		for _, s := range seeded {
			if survivors[s] {
				seedSurvivors++
			}
		}
		Expect(seedSurvivors).To(Equal(1), "exactly one seed should survive when limit=2 and one slot is taken by the new report")
	})

	It("prunes HealthReports without going through a reconcile", func() {
		name := "addoncheck-prune-direct"
		ns := "default"
		limit := int32(1)
		check := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager", HistoryLimit: &limit},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed())
		})

		for i := 0; i < 3; i++ {
			seed := &fathomv1alpha1.HealthReport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    ns,
					GenerateName: name + "-seed-",
					Labels: map[string]string{
						"fathom.skaphos.io/source-kind": "AddonCheck",
						"fathom.skaphos.io/source-name": name,
					},
				},
				Spec: fathomv1alpha1.HealthReportSpec{
					SourceRef:  fathomv1alpha1.HealthReportTargetRef{Kind: "AddonCheck", Name: name},
					Result:     fathomv1alpha1.HealthReportResultPass,
					ObservedAt: metav1.NewTime(time.Now()),
				},
			}
			Expect(k8sClient.Create(ctx, seed)).To(Succeed())
			time.Sleep(100 * time.Millisecond)
		}

		(&AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}).
			pruneHealthReportHistory(ctx, logr.Discard(), check)

		var reports fathomv1alpha1.HealthReportList
		Expect(k8sClient.List(ctx, &reports,
			client.InNamespace(ns),
			client.MatchingLabels{"fathom.skaphos.io/source-name": name},
		)).To(Succeed())
		Expect(reports.Items).To(HaveLen(int(limit)))
	})
})
