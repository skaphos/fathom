/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/registry"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
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

// legacyRatioAdapter models a 1.0 adapter that used failRatio while threshold
// names were still adapter-owned. It deliberately advertises and reads the key
// so the compatibility regression cannot mistake ThresholdAdvertiser support
// for evidence that engine reinterpretation is safe.
type legacyRatioAdapter struct {
	mu                    sync.Mutex
	runs                  int
	consumedLegacyPrivate bool
}

func (a *legacyRatioAdapter) Name() string            { return "legacy-ratio-adapter" }
func (a *legacyRatioAdapter) Version() string         { return "0.9.0" }
func (a *legacyRatioAdapter) ContractVersion() string { return "1.0.0" }
func (a *legacyRatioAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{"legacy-addon"}, Families: []adapter.Family{"system_health"}}
}
func (a *legacyRatioAdapter) ThresholdKeys() map[adapter.Family][]string {
	return map[adapter.Family][]string{"system_health": {adapter.ThresholdKeyFailRatio, "legacyLimit"}}
}
func (a *legacyRatioAdapter) Run(_ context.Context, req adapter.Request) (adapter.Result, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.runs++
	thresholds := req.Policy["system_health"].Thresholds
	if _, ok := thresholds["legacyLimit"]; ok {
		a.consumedLegacyPrivate = true
	}
	// This adapter's private 1.0 meaning is deliberately incompatible with the
	// engine meaning: "100" fails immediately, while an engine failRatio of
	// 100 permits every evaluated check. Before the gate, both interpretations
	// were applied to one policy.
	outcome := adapter.OutcomePass
	if thresholds[adapter.ThresholdKeyFailRatio] == "100" {
		outcome = adapter.OutcomeFail
	}
	return adapter.Result{Checks: []adapter.CheckResult{{
		Family:    "system_health",
		Outcome:   outcome,
		TargetRef: adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "legacy"},
		Summary:   "legacy adapter ran",
	}}}, nil
}
func (a *legacyRatioAdapter) state() (runs int, consumedLegacyPrivate bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.runs, a.consumedLegacyPrivate
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

// absentReportingAdapter returns a fixed mix of checks — one required-absent Fail,
// one optional-absent Skip, one present-but-unhealthy Fail, and one healthy Pass —
// so the reconciler's status.absent wiring (countAbsent over the absent marker)
// can be asserted end-to-end (SKA-526).
type absentReportingAdapter struct{}

func (absentReportingAdapter) Name() string            { return "absent-addon" }
func (absentReportingAdapter) Version() string         { return "0.0.1" }
func (absentReportingAdapter) ContractVersion() string { return adapter.ContractVersion }
func (absentReportingAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{"absent-addon"}, Families: []adapter.Family{"system_health"}}
}

func (absentReportingAdapter) Run(_ context.Context, _ adapter.Request) (adapter.Result, error) {
	ref := func(name string) adapter.TargetRef {
		return adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "x", Name: name}
	}
	return adapter.Result{
		Duration: time.Millisecond,
		Checks: []adapter.CheckResult{
			{Family: adapter.Family("system_health"), Outcome: adapter.OutcomeFail, TargetRef: ref("required-absent"), Summary: "not installed", Details: adapter.MarkAbsent(map[string]string{"component": "a"}), ObservedAt: time.Now()},
			{Family: adapter.Family("system_health"), Outcome: adapter.OutcomeSkipped, TargetRef: ref("optional-absent"), Summary: "not installed", Details: adapter.MarkAbsent(map[string]string{"component": "b"}), ObservedAt: time.Now()},
			{Family: adapter.Family("system_health"), Outcome: adapter.OutcomeFail, TargetRef: ref("present-unhealthy"), Summary: "broken", Details: map[string]string{"component": "c"}, ObservedAt: time.Now()},
			{Family: adapter.Family("system_health"), Outcome: adapter.OutcomePass, TargetRef: ref("healthy"), Summary: "ok", ObservedAt: time.Now()},
		},
	}, nil
}

// versionReportingAdapter returns a healthy check plus a DetectedVersion, so the
// reconciler's status.detectedVersion + HealthReport wiring can be asserted
// end-to-end (SKA-527).
type versionReportingAdapter struct{}

func (versionReportingAdapter) Name() string            { return "version-addon" }
func (versionReportingAdapter) Version() string         { return "0.0.1" }
func (versionReportingAdapter) ContractVersion() string { return adapter.ContractVersion }
func (versionReportingAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{"version-addon"}, Families: []adapter.Family{"system_health"}}
}

func (versionReportingAdapter) Run(_ context.Context, _ adapter.Request) (adapter.Result, error) {
	return adapter.Result{
		Duration:        time.Millisecond,
		DetectedVersion: "1.15.6",
		Checks: []adapter.CheckResult{{
			Family:     adapter.Family("system_health"),
			Outcome:    adapter.OutcomePass,
			TargetRef:  adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "x", Name: "addon"},
			Summary:    "ok",
			ObservedAt: time.Now(),
		}},
	}, nil
}

// healthReportCount returns how many HealthReports the given AddonCheck has
// produced, matched via the source-kind/name labels.
func healthReportCount(ctx context.Context, source types.NamespacedName) int {
	var reports fathomv1alpha1.HealthReportList
	ExpectWithOffset(1, k8sClient.List(ctx, &reports,
		client.InNamespace(source.Namespace),
		client.MatchingLabels{
			fathomv1alpha1.LabelHealthReportSourceKind: "AddonCheck",
			fathomv1alpha1.LabelHealthReportSourceName: source.Name,
		},
	)).To(Succeed())
	return len(reports.Items)
}

// countingStatusClient wraps a client.Client to count Status().Update calls, so
// a test can assert a reconcile attempted no status write (converged). It is
// robust against the API server's second-granularity dedup of metav1.Time,
// which otherwise hides a condition churning within a single wall-clock second.
type countingStatusClient struct {
	client.Client
	statusUpdates *int
}

func (c countingStatusClient) Status() client.SubResourceWriter {
	return countingStatusWriter{SubResourceWriter: c.Client.Status(), n: c.statusUpdates}
}

type countingStatusWriter struct {
	client.SubResourceWriter
	n *int
}

func (w countingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	*w.n++
	return w.SubResourceWriter.Update(ctx, obj, opts...)
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
					"system_health": {Enabled: ptr.To(true)},
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
					"system_health": {Enabled: ptr.To(true), Thresholds: map[string]fathomv1alpha1.ThresholdValue{"warnDays": "14"}},
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

	It("reuses a HealthReport after a status update conflict", func() {
		name := types.NamespacedName{Name: "addoncheck-status-conflict", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager"},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed()) })

		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(fakeAddonAdapter{})).To(Succeed())

		conflict := true
		r := &AddonCheckReconciler{
			Client:   conflictOnceStatusClient{Client: k8sClient, conflict: &conflict},
			Scheme:   k8sClient.Scheme(),
			Adapters: adapters,
		}
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(apierrors.IsConflict(err)).To(BeTrue())
		Expect(healthReportCount(ctx, name)).To(Equal(1))

		_, err = (&AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}).
			Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(healthReportCount(ctx, name)).To(Equal(1))

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastReportName).NotTo(BeEmpty())
	})

	It("counts absent components into status.absent (SKA-526)", func() {
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-absent",
			Namespace: "default",
		}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      typeNamespacedName.Name,
				Namespace: typeNamespacedName.Namespace,
			},
			Spec: fathomv1alpha1.AddonCheckSpec{
				AddonType: "absent-addon",
				Policy: map[string]fathomv1alpha1.AddonCheckFamilyPolicy{
					"system_health": {Enabled: ptr.To(true)},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(absentReportingAdapter{})).To(Succeed())
		controllerReconciler := &AddonCheckReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Adapters: adapters,
		}

		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		// Two of the four checks carry the absent marker (required-absent Fail +
		// optional-absent Skip); the present-but-unhealthy Fail and the Pass do not.
		Expect(updated.Status.Absent).To(Equal(int32(2)))
	})

	It("surfaces the adapter's detected version into status and the HealthReport (SKA-527)", func() {
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-version",
			Namespace: "default",
		}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      typeNamespacedName.Name,
				Namespace: typeNamespacedName.Namespace,
			},
			Spec: fathomv1alpha1.AddonCheckSpec{AddonType: "version-addon"},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(versionReportingAdapter{})).To(Succeed())
		r := &AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}

		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.DetectedVersion).To(Equal("1.15.6"))

		report := &fathomv1alpha1.HealthReport{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: updated.Status.LastReportName, Namespace: typeNamespacedName.Namespace}, report)).To(Succeed())
		Expect(report.Spec.DetectedVersion).To(Equal("1.15.6"))
	})

	It("rejects a policy with an unknown family: Accepted=False and the adapter is not run (SKA-54)", func() {
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-invalid-policy",
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
					// fakeAddonAdapter advertises only "system_health".
					"bogus_family": {Enabled: ptr.To(true)},
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

		result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())
		// An invalid policy waits for a spec change to clear, so no interval requeue.
		Expect(result.RequeueAfter).To(BeZero())

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())

		accepted := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionAccepted)
		Expect(accepted).NotTo(BeNil())
		Expect(accepted.Status).To(Equal(metav1.ConditionFalse))
		Expect(accepted.Reason).To(Equal("InvalidPolicy"))
		Expect(accepted.Message).To(ContainSubstring(`unknown family "bogus_family"`))

		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("InvalidPolicy"))

		// The adapter never ran: no run time, no HealthReport.
		Expect(updated.Status.LastRunTime).To(BeNil())
		Expect(updated.Status.LastReportName).To(BeEmpty())
	})

	It("converges: repeated reconciles of an invalid policy attempt no further status write", func() {
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-invalid-policy-converge",
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
					"bogus_family": {Enabled: ptr.To(true)},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(fakeAddonAdapter{})).To(Succeed())
		// Count status-write attempts directly, rather than comparing
		// resourceVersion: the API server serializes metav1.Time to second
		// precision and dedupes byte-identical updates, so a churning condition is
		// invisible via resourceVersion when two reconciles share a wall-clock
		// second. The reconciler's own before/after DeepEqual (nanosecond LTT) is
		// the true signal, observed here as the Status().Update call count.
		statusUpdates := 0
		r := &AddonCheckReconciler{
			Client:   countingStatusClient{Client: k8sClient, statusUpdates: &statusUpdates},
			Scheme:   k8sClient.Scheme(),
			Adapters: adapters,
		}

		// First reconcile persists Accepted=False / Ready=False: one write.
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())
		Expect(statusUpdates).To(Equal(1))

		// A second reconcile of the unchanged, still-invalid check must attempt no
		// write. Regression guard: setting Ready True in resolveAddonAdapter and
		// then flipping it False for the invalid policy bumped LastTransitionTime
		// every pass, so before != after and the reconciler rewrote status on
		// every reconcile (churn), self-re-triggering via the predicate-less watch.
		statusUpdates = 0
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())
		Expect(statusUpdates).To(Equal(0), "second reconcile rewrote status (churn) instead of converging")
	})

	It("rejects engine ratio keys for a legacy 1.0 adapter before Run", func() {
		name := types.NamespacedName{Name: "addoncheck-legacy-ratio", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec: fathomv1alpha1.AddonCheckSpec{
				AddonType: "legacy-addon",
				Policy: map[string]fathomv1alpha1.AddonCheckFamilyPolicy{
					"system_health": {
						Enabled:    ptr.To(false),
						Thresholds: map[string]fathomv1alpha1.ThresholdValue{adapter.ThresholdKeyFailRatio: "100"},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed()) })

		legacy := &legacyRatioAdapter{}
		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(legacy)).To(Succeed(), "ordinary 1.0 adapters remain loadable by a 1.1 host")
		result, err := (&AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}).
			Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		runs, _ := legacy.state()
		Expect(runs).To(BeZero(), "a rejected policy must never reach the legacy adapter")
		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		accepted := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionAccepted)
		Expect(accepted).NotTo(BeNil())
		Expect(accepted.Status).To(Equal(metav1.ConditionFalse))
		Expect(accepted.Reason).To(Equal("InvalidPolicy"))
		Expect(accepted.Message).To(ContainSubstring("adapter \"legacy-ratio-adapter\" uses contract version 1.0.0"))
		Expect(accepted.Message).To(ContainSubstring("require contract version 1.1.0 or newer"))
		Expect(updated.Status.LastRunTime).To(BeNil())
		Expect(updated.Status.LastReportName).To(BeEmpty())
	})

	It("continues to run a legacy 1.0 adapter with an ordinary private threshold", func() {
		name := types.NamespacedName{Name: "addoncheck-legacy-private-threshold", Namespace: "default"}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec: fathomv1alpha1.AddonCheckSpec{
				AddonType: "legacy-addon",
				Policy: map[string]fathomv1alpha1.AddonCheckFamilyPolicy{
					"system_health": {
						Thresholds: map[string]fathomv1alpha1.ThresholdValue{"legacyLimit": "7"},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed()) })

		legacy := &legacyRatioAdapter{}
		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(legacy)).To(Succeed())
		_, err := (&AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Adapters: adapters}).
			Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		runs, consumedLegacyPrivate := legacy.state()
		Expect(runs).To(Equal(1))
		Expect(consumedLegacyPrivate).To(BeTrue())
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
		Expect(result.RequeueAfter).To(Equal(fathomv1alpha1.DefaultAddonCheckInterval))
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
		Expect(result.RequeueAfter).To(Equal(fathomv1alpha1.DefaultAddonCheckInterval))
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
		updated.Annotations = map[string]string{fathomv1alpha1.AnnotationRunNow: "token-1"}
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
				Annotations: map[string]string{fathomv1alpha1.AnnotationRunNow: "tok"},
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
		updated.Annotations = map[string]string{fathomv1alpha1.AnnotationRunNow: "tok"}
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
		func(setup func(*fathomv1alpha1.AddonCheck), prevGen int64, triggerDue bool, interval time.Duration, want bool) {
			check := &fathomv1alpha1.AddonCheck{}
			check.Generation = 1
			setup(check)
			Expect(addonCheckDueForRun(check, prevGen, triggerDue, interval)).To(Equal(want))
		},
		Entry("first sight (no LastRunTime) is due",
			func(c *fathomv1alpha1.AddonCheck) {}, int64(1), false, time.Minute, true),
		Entry("generation change is due",
			func(c *fathomv1alpha1.AddonCheck) { n := metav1.Now(); c.Status.LastRunTime = &n }, int64(0), false, time.Minute, true),
		Entry("a due run-now trigger is due",
			func(c *fathomv1alpha1.AddonCheck) { n := metav1.Now(); c.Status.LastRunTime = &n }, int64(1), true, time.Minute, true),
		Entry("a consumed trigger within the interval is not due",
			func(c *fathomv1alpha1.AddonCheck) {
				n := metav1.Now()
				c.Status.LastRunTime = &n
				c.Status.LastRunTrigger = "t1"
			}, int64(1), false, time.Minute, false),
		Entry("an elapsed interval is due",
			func(c *fathomv1alpha1.AddonCheck) {
				p := metav1.NewTime(time.Now().Add(-time.Hour))
				c.Status.LastRunTime = &p
			}, int64(1), false, time.Minute, true),
		Entry("within the interval with no triggers is not due",
			func(c *fathomv1alpha1.AddonCheck) { n := metav1.Now(); c.Status.LastRunTime = &n }, int64(1), false, time.Minute, false),
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
		Entry("Pass+Skipped aggregates to Pass (Skipped is informational, #160)",
			[]adapter.Outcome{adapter.OutcomePass, adapter.OutcomeSkipped}, fathomv1alpha1.HealthReportResultPass),
		Entry("Warn+Skipped aggregates to Warn (Skipped never wins over a participating result)",
			[]adapter.Outcome{adapter.OutcomeWarn, adapter.OutcomeSkipped}, fathomv1alpha1.HealthReportResultWarn),
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

// ---------------------------------------------------------------------------
// T047 — the runtime path's production caller
// ---------------------------------------------------------------------------
//
// These are plain stdlib tests rather than specs because they reuse
// runtimeCheckFixture, which drives a complete runtime run over a fake API with
// an injected clock. What they pin is the WIRING: which path a check takes,
// what reaches the shared worker pool, and what the pool's handler does with a
// completed run. The run itself is covered next door.
//
// The default-off property is asserted in BOTH directions on every row that has
// two: a reconciler with no runner and no queue must behave exactly as it did
// before this feature existed, and it must keep doing so while a runtime
// snapshot is published and admitted underneath it.

// fakeRuntimeQueue records what the reconciler asks of the shared pool. It is
// deliberately inert: nothing it receives is executed, so a test that expects
// work to run must go through RunRuntimeWork explicitly.
type fakeRuntimeQueue struct {
	mu        sync.Mutex
	enqueued  []execution.Work
	forgotten []types.NamespacedName
	err       error
}

func (q *fakeRuntimeQueue) Enqueue(work execution.Work, _ time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.enqueued = append(q.enqueued, work)
	return nil
}

func (q *fakeRuntimeQueue) Forget(check types.NamespacedName, _ time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.forgotten = append(q.forgotten, check)
}

func (q *fakeRuntimeQueue) works() []execution.Work {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]execution.Work(nil), q.enqueued...)
}

func (q *fakeRuntimeQueue) forgets() []types.NamespacedName {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]types.NamespacedName(nil), q.forgotten...)
}

// runtimeWiredReconciler returns an AddonCheck reconciler wired exactly as
// internal/app wires it once runtime loading is enabled: the SAME registry the
// built-in adapters are registered in, the runtime runner, and the shared pool
// queue.
func runtimeWiredReconciler(f *runtimeCheckFixture, queue RuntimeWorkQueue) *AddonCheckReconciler {
	return &AddonCheckReconciler{
		Client:       f.cached,
		Scheme:       f.scheme,
		Adapters:     f.registry,
		Runtime:      f.runner,
		RuntimeQueue: queue,
	}
}

// builtinReconciler is the same reconciler with runtime loading OFF — the
// default, and every deployment that has not opted in.
func builtinReconciler(f *runtimeCheckFixture) *AddonCheckReconciler {
	return &AddonCheckReconciler{Client: f.cached, Scheme: f.scheme, Adapters: f.registry}
}

func runtimeCheckKey() types.NamespacedName {
	return types.NamespacedName{Namespace: runtimeCheckNamespace, Name: runtimeCheckName}
}

// countRuntimeEvaluations makes evaluator runs observable. The fixture counts a
// run only while its adapter is SCRIPTED, so a test that asserts "no evaluator
// ran" against the unscripted default asserts nothing at all.
func countRuntimeEvaluations(f *runtimeCheckFixture) {
	f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
		return adapter.Result{Checks: []adapter.CheckResult{{
			Family: "health", Outcome: adapter.OutcomePass, Summary: "controller is available",
		}}}, nil
	})
}

// The routing decision. Each row names the reason a check does or does not
// belong to the runtime pool; the two "false" rows at the top are the
// default-off guarantee, and the built-in row is "preserve unrelated built-ins".
func TestRuntimeBackedRouting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		arrange func(*testing.T, *runtimeCheckFixture, *AddonCheckReconciler, *fathomv1alpha1.AddonCheck)
		want    bool
	}{
		{
			name: "runtime loading disabled leaves every check on the built-in path",
			arrange: func(_ *testing.T, f *runtimeCheckFixture, r *AddonCheckReconciler, _ *fathomv1alpha1.AddonCheck) {
				f.ready()
				r.Runtime, r.RuntimeQueue = nil, nil
			},
		},
		{
			name: "a queue without a runner is not a wired runtime",
			arrange: func(_ *testing.T, f *runtimeCheckFixture, r *AddonCheckReconciler, _ *fathomv1alpha1.AddonCheck) {
				f.ready()
				r.Runtime = nil
			},
		},
		{
			// A runner with nowhere to queue would have to execute inline on a
			// reconcile worker, which is precisely what the separate runtime
			// pool exists to prevent.
			name: "a runner without a queue is not a wired runtime",
			arrange: func(_ *testing.T, f *runtimeCheckFixture, r *AddonCheckReconciler, _ *fathomv1alpha1.AddonCheck) {
				f.ready()
				r.RuntimeQueue = nil
			},
		},
		{
			name: "a built-in adapter keeps its own path",
			arrange: func(t *testing.T, f *runtimeCheckFixture, _ *AddonCheckReconciler, check *fathomv1alpha1.AddonCheck) {
				f.ready()
				if err := f.registry.Register(fakeAddonAdapter{}); err != nil {
					t.Fatalf("register built-in: %v", err)
				}
				check.Spec.AddonType = "cert-manager"
			},
		},
		{
			name: "a paused check runs nowhere",
			arrange: func(_ *testing.T, f *runtimeCheckFixture, _ *AddonCheckReconciler, check *fathomv1alpha1.AddonCheck) {
				f.ready()
				check.Spec.Paused = true
			},
		},
		{
			name: "an addon type that cannot name a definition keeps the built-in answer",
			arrange: func(_ *testing.T, f *runtimeCheckFixture, _ *AddonCheckReconciler, check *fathomv1alpha1.AddonCheck) {
				f.ready()
				check.Spec.AddonType = "Not_A_Label"
			},
		},
		{
			name: "an admitted runtime snapshot",
			arrange: func(_ *testing.T, f *runtimeCheckFixture, _ *AddonCheckReconciler, _ *fathomv1alpha1.AddonCheck) {
				f.ready()
			},
			want: true,
		},
		{
			name: "a published snapshot whose dispatch is not admitted",
			arrange: func(_ *testing.T, f *runtimeCheckFixture, _ *AddonCheckReconciler, _ *fathomv1alpha1.AddonCheck) {
				f.publish(1)
			},
			want: true,
		},
		{
			name: "an identity nobody claims",
			arrange: func(_ *testing.T, f *runtimeCheckFixture, _ *AddonCheckReconciler, _ *fathomv1alpha1.AddonCheck) {
				f.admit()
			},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			queue := &fakeRuntimeQueue{}
			r := runtimeWiredReconciler(f, queue)
			check := runtimeCheckObject()
			tc.arrange(t, f, r, check)
			if got := r.runtimeBacked(check); got != tc.want {
				t.Fatalf("runtimeBacked = %v, want %v", got, tc.want)
			}
		})
	}
}

// A runtime-backed check is ENQUEUED, not executed inline: the contract's
// scheduling row demands a separate runtime worker pool, so a reconcile worker
// must never be the thing that runs a definition.
func TestReconcileEnqueuesRuntimeChecksInsteadOfRunningThemInline(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	countRuntimeEvaluations(f)
	queue := &fakeRuntimeQueue{}
	r := runtimeWiredReconciler(f, queue)

	result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: runtimeCheckKey()})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	works := queue.works()
	if len(works) != 1 {
		t.Fatalf("enqueued %d runtime runs, want exactly 1: %+v", len(works), works)
	}
	if works[0].Definition != lifecycleAddon || works[0].Check != runtimeCheckKey() {
		t.Errorf("enqueued %+v, want the check keyed by its definition", works[0])
	}
	if _, _, _, runs := f.counters(); runs != 0 {
		t.Errorf("the reconcile worker executed %d evaluator runs; runtime work belongs to the pool", runs)
	}
	if result.RequeueAfter != addonCheckInterval(runtimeCheckObject()) {
		t.Errorf("RequeueAfter = %v, want the check interval; a runtime check must keep its cadence", result.RequeueAfter)
	}
	// MissingAdapter is the built-in answer for an identity no built-in claims.
	// Reporting it for a runtime identity would contradict the runtime path,
	// which owns UnknownAddonType and every barrier reason.
	if cond := runtimeReadyCondition(t, f.check()); cond != nil && cond.Reason == "MissingAdapter" {
		t.Errorf("Ready reason = MissingAdapter for a runtime-backed check")
	}
}

// The same reconcile, with runtime loading off. This is the default-off
// property stated as behaviour rather than as configuration: nothing is
// enqueued, and the answer is the one the operator has always given.
func TestReconcileWithoutRuntimeWiringKeepsTheBuiltInAnswer(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready() // a snapshot IS published and admitted; it must still change nothing
	countRuntimeEvaluations(f)
	r := builtinReconciler(f)

	result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: runtimeCheckKey()})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("RequeueAfter = %v, want 0 for a check with no built-in adapter", result.RequeueAfter)
	}
	cond := runtimeReadyCondition(t, f.check())
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "MissingAdapter" {
		t.Fatalf("Ready condition = %+v, want False/MissingAdapter", cond)
	}
	if _, _, _, runs := f.counters(); runs != 0 {
		t.Errorf("an unwired reconciler ran %d evaluator runs", runs)
	}
}

// Built-ins are preserved: with the runtime fully wired, a built-in identity
// still runs inline on its own workers and never reaches the runtime queue.
func TestBuiltInChecksStillRunInlineWhileRuntimeIsWired(t *testing.T) {
	builtinCheck := &fathomv1alpha1.AddonCheck{
		ObjectMeta: metav1.ObjectMeta{Namespace: runtimeCheckNamespace, Name: "builtin-check", UID: "builtin-uid", Generation: 1},
		Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager"},
	}
	f := newRuntimeCheckFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount(), runtimeCheckObject(), builtinCheck)
	f.ready()
	if err := f.registry.Register(fakeAddonAdapter{}); err != nil {
		t.Fatalf("register built-in: %v", err)
	}
	queue := &fakeRuntimeQueue{}
	r := runtimeWiredReconciler(f, queue)

	key := types.NamespacedName{Namespace: runtimeCheckNamespace, Name: "builtin-check"}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if works := queue.works(); len(works) != 0 {
		t.Fatalf("a built-in check reached the runtime pool: %+v", works)
	}
	var ran fathomv1alpha1.AddonCheck
	if err := f.store.Get(context.Background(), key, &ran); err != nil {
		t.Fatalf("get built-in check: %v", err)
	}
	if ran.Status.LastRunTime == nil {
		t.Error("the built-in adapter did not run; runtime wiring must not gate built-in dispatch")
	}
}

// A paused check must not keep a queued wake, and a deleted one must not keep
// one either: the pool would otherwise admit a run for an object that no longer
// wants one.
func TestPausedAndDeletedRuntimeChecksAreForgotten(t *testing.T) {
	t.Run("paused", func(t *testing.T) {
		f := newRuntimeCheckFixture(t)
		f.ready()
		paused := f.check()
		paused.Spec.Paused = true
		f.update(paused)
		queue := &fakeRuntimeQueue{}
		r := runtimeWiredReconciler(f, queue)

		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: runtimeCheckKey()}); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if works := queue.works(); len(works) != 0 {
			t.Errorf("a paused check was enqueued: %+v", works)
		}
		if forgets := queue.forgets(); len(forgets) != 1 || forgets[0] != runtimeCheckKey() {
			t.Errorf("forgotten = %+v, want the paused check withdrawn once", forgets)
		}
	})

	t.Run("deleted", func(t *testing.T) {
		f := newRuntimeCheckFixture(t)
		f.ready()
		if err := f.store.Delete(context.Background(), f.check()); err != nil {
			t.Fatalf("delete check: %v", err)
		}
		queue := &fakeRuntimeQueue{}
		r := runtimeWiredReconciler(f, queue)

		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: runtimeCheckKey()}); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if forgets := queue.forgets(); len(forgets) != 1 || forgets[0] != runtimeCheckKey() {
			t.Errorf("forgotten = %+v, want the deleted check withdrawn once", forgets)
		}
	})
}

// RunRuntimeWork is the pool handler: it must execute the run AND record the
// transition, because the runner publishes evidence and knows nothing about
// history. A completed first run therefore produces both new evidence and the
// one HealthReport that says the verdict moved.
func TestRunRuntimeWorkPublishesEvidenceAndRecordsTheTransition(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	r := runtimeWiredReconciler(f, &fakeRuntimeQueue{})
	work := execution.Work{Definition: lifecycleAddon, Check: runtimeCheckKey()}

	if got := r.RunRuntimeWork(context.Background(), work); got != execution.Completed {
		t.Fatalf("disposition = %v, want Completed", got)
	}
	published := f.check()
	if published.Status.LastSuccessfulEvaluation == nil {
		t.Fatal("no completed evidence was published; the pool handler did not run the check")
	}
	reports := f.reports()
	if len(reports) != 1 {
		t.Fatalf("stored %d HealthReports, want exactly 1 for the first completed run", len(reports))
	}
	if published.Status.LastReportName != reports[0].Name {
		t.Errorf("lastReportName = %q, want the report just created (%q); the handler persists what the transition path names",
			published.Status.LastReportName, reports[0].Name)
	}

	// The same verdict again: "No-change verdicts do not create reports".
	f.advance(time.Hour)
	if got := r.RunRuntimeWork(context.Background(), work); got != execution.Completed {
		t.Fatalf("second disposition = %v, want Completed", got)
	}
	if reports := f.reports(); len(reports) != 1 {
		t.Fatalf("stored %d HealthReports after an unchanged verdict, want 1", len(reports))
	}
}

func TestRunRuntimeWorkDoesNotAttributeReportsToStaleCachedStatus(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	stale := f.check().DeepCopy()
	r := runtimeWiredReconciler(f, &fakeRuntimeQueue{})
	r.Client = interceptor.NewClient(f.cached, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if check, ok := obj.(*fathomv1alpha1.AddonCheck); ok && key == runtimeCheckKey() {
				stale.DeepCopyInto(check)
				return nil
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	work := execution.Work{Definition: lifecycleAddon, Check: runtimeCheckKey()}
	if got := r.RunRuntimeWork(context.Background(), work); got != execution.Completed {
		t.Fatalf("first disposition = %v, want Completed", got)
	}
	reports := f.reports()
	if len(reports) != 1 {
		t.Fatalf("first run stored %d reports, want 1", len(reports))
	}
	first := f.check().Status.LastSuccessfulEvaluation
	if first == nil || !reports[0].Spec.ObservedAt.Equal(&first.ObservedAt) {
		t.Fatal("first report was attributed to stale evidence instead of the published observation")
	}
	f.advance(time.Hour)
	if got := r.RunRuntimeWork(context.Background(), work); got != execution.Completed {
		t.Fatalf("second disposition = %v, want Completed", got)
	}
	if reports := f.reports(); len(reports) != 1 {
		t.Fatalf("unchanged verdict under a stale cache stored %d reports, want 1", len(reports))
	}
}

func TestRunRuntimeWorkReusesReportAfterReportPointerConflict(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	r := runtimeWiredReconciler(f, &fakeRuntimeQueue{})
	var conflict atomic.Bool
	conflict.Store(true)
	r.Client = interceptor.NewClient(f.cached, interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			check, ok := obj.(*fathomv1alpha1.AddonCheck)
			if sub == "status" && ok && check.Status.LastReportName != "" && conflict.Swap(false) {
				return apierrors.NewConflict(fathomv1alpha1.GroupVersion.WithResource("addonchecks").GroupResource(), check.Name, errors.New("synthetic concurrent status update"))
			}
			return c.SubResource(sub).Update(ctx, obj, opts...)
		},
	})
	work := execution.Work{Definition: lifecycleAddon, Check: runtimeCheckKey()}
	if got := r.RunRuntimeWork(context.Background(), work); got != execution.Retry {
		t.Fatalf("first disposition = %v, want Retry after report pointer conflict", got)
	}
	reports := f.reports()
	if len(reports) != 1 || f.check().Status.LastReportName != "" {
		t.Fatalf("after pointer conflict reports=%d lastReportName=%q, want 1 and empty pointer", len(reports), f.check().Status.LastReportName)
	}
	first := reports[0].DeepCopy()
	// A check spec edit can advance generation before the pointer recovers;
	// the historical transition is still the one already created.
	edited := f.check()
	edited.Generation++
	if err := f.store.Update(context.Background(), edited); err != nil {
		t.Fatalf("advance check generation: %v", err)
	}
	f.advance(time.Hour)
	if got := r.RunRuntimeWork(context.Background(), work); got != execution.Completed {
		t.Fatalf("retry disposition = %v, want Completed", got)
	}
	reports = f.reports()
	if len(reports) != 1 {
		t.Fatalf("same-verdict retry stored %d reports, want one", len(reports))
	}
	if reports[0].Name != first.Name || !reports[0].Spec.ObservedAt.Equal(&first.Spec.ObservedAt) {
		t.Fatal("retry replaced the original report's identity or attribution")
	}
	if got := f.check().Status.LastReportName; got != first.Name {
		t.Fatalf("lastReportName=%q, want recovered pointer %q", got, first.Name)
	}
}

// The lost-transition repair, driven through its production caller. A report
// create that fails leaves status showing a verdict history does not hold; the
// next run must backfill it rather than compare the verdict against itself
// forever.
func TestRunRuntimeWorkBackfillsATransitionLostByAFailedReportWrite(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	var blocked atomic.Bool
	blocked.Store(true)
	blocking := interceptor.NewClient(f.cached, interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*fathomv1alpha1.HealthReport); ok && blocked.Load() {
				return errors.New("synthetic history write failure")
			}
			return c.Create(ctx, obj, opts...)
		},
	})
	r := runtimeWiredReconciler(f, &fakeRuntimeQueue{})
	r.Client = blocking
	work := execution.Work{Definition: lifecycleAddon, Check: runtimeCheckKey()}

	if got := r.RunRuntimeWork(context.Background(), work); got != execution.Retry {
		t.Fatalf("disposition = %v, want Retry when history could not be written", got)
	}
	if reports := f.reports(); len(reports) != 0 {
		t.Fatalf("stored %d HealthReports while history writes were failing", len(reports))
	}
	if name := f.check().Status.LastReportName; name != "" {
		t.Fatalf("lastReportName = %q, want empty: no report exists to name", name)
	}

	// Same verdict, but history holds nothing. The backfill must write it.
	blocked.Store(false)
	f.advance(time.Hour)
	if got := r.RunRuntimeWork(context.Background(), work); got != execution.Completed {
		t.Fatalf("disposition = %v, want Completed", got)
	}
	reports := f.reports()
	if len(reports) != 1 {
		t.Fatalf("stored %d HealthReports, want the backfilled transition", len(reports))
	}
	if got := f.check().Status.LastReportName; got != reports[0].Name {
		t.Errorf("lastReportName = %q, want %q", got, reports[0].Name)
	}
}

// What the handler does with a run that published nothing. MissingInput is the
// contract's 60s poll and belongs to the rows whose recovery is an
// administrator action or another controller's publication; everything else
// rides the bounded retry ramp.
func TestRunRuntimeWorkPacesUnpublishedOutcomes(t *testing.T) {
	for _, tc := range []struct {
		reason string
		want   execution.Disposition
	}{
		{reasonUnknownAddonType, execution.MissingInput},
		{reasonAuthorizationUnavailable, execution.MissingInput},
		{reasonAuthorizationRevoked, execution.MissingInput},
		{reasonDefinitionUnavailable, execution.MissingInput},
		{reasonBindingMismatch, execution.MissingInput},
		{registry.ReasonAdmissionClosed, execution.MissingInput},
		{registry.ReasonBuiltinCollision, execution.MissingInput},
		{registry.ReasonRuntimeCollision, execution.MissingInput},
		{reasonRuntimeTimeout, execution.Retry},
		{reasonSuperseded, execution.Retry},
		{reasonPublicationConflict, execution.Retry},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			if got := runtimeDisposition(RuntimeAttempt{Reason: tc.reason}); got != tc.want {
				t.Fatalf("disposition for %s = %v, want %v", tc.reason, got, tc.want)
			}
		})
	}
}

// An unadmitted gate is the steady state of a process that is not the leader.
// The handler must report it as a poll rather than run anything.
func TestRunRuntimeWorkRefusesWhileDispatchIsNotAdmitted(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.publish(1) // published, never admitted
	countRuntimeEvaluations(f)
	r := runtimeWiredReconciler(f, &fakeRuntimeQueue{})

	got := r.RunRuntimeWork(context.Background(), execution.Work{Definition: lifecycleAddon, Check: runtimeCheckKey()})
	if got != execution.MissingInput {
		t.Fatalf("disposition = %v, want MissingInput while runtime dispatch is closed", got)
	}
	if _, _, _, runs := f.counters(); runs != 0 {
		t.Errorf("%d evaluator runs happened while dispatch was closed", runs)
	}
	if f.check().Status.LastSuccessfulEvaluation != nil {
		t.Error("evidence was published while runtime dispatch was closed")
	}
}

// The queue entry is keyed by the check, so its spec can change between the
// enqueue and the admission. Neither a retargeted, a paused nor a deleted check
// is this run's to execute, and none of them is a failure to retry.
func TestRunRuntimeWorkDeclinesWorkItsCheckNoLongerWants(t *testing.T) {
	for _, tc := range []struct {
		name    string
		arrange func(*runtimeCheckFixture) execution.Work
	}{
		{
			name: "deleted check",
			arrange: func(f *runtimeCheckFixture) execution.Work {
				if err := f.store.Delete(context.Background(), f.check()); err != nil {
					f.t.Fatalf("delete: %v", err)
				}
				return execution.Work{Definition: lifecycleAddon, Check: runtimeCheckKey()}
			},
		},
		{
			name: "retargeted check",
			arrange: func(f *runtimeCheckFixture) execution.Work {
				return execution.Work{Definition: "another-addon", Check: runtimeCheckKey()}
			},
		},
		{
			name: "paused check",
			arrange: func(f *runtimeCheckFixture) execution.Work {
				paused := f.check()
				paused.Spec.Paused = true
				f.update(paused)
				return execution.Work{Definition: lifecycleAddon, Check: runtimeCheckKey()}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			countRuntimeEvaluations(f)
			r := runtimeWiredReconciler(f, &fakeRuntimeQueue{})
			work := tc.arrange(f)
			if got := r.RunRuntimeWork(context.Background(), work); got != execution.Completed {
				t.Fatalf("disposition = %v, want Completed", got)
			}
			if _, _, _, runs := f.counters(); runs != 0 {
				t.Errorf("the handler executed %d evaluator runs for work its check no longer wants", runs)
			}
		})
	}
}
