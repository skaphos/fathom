/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

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
func (fakeAddonAdapter) Run(_ context.Context, req adapter.Request) (adapter.Result, error) {
	return adapter.Result{
		Duration: 25 * time.Millisecond,
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

		// Smoke test for AddonCheckReconciler instrumentation (SKA-290)
		metrics.ReconcileTotal.Reset()
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		mfs, err := ctrlmetrics.Registry.Gather()
		Expect(err).NotTo(HaveOccurred())

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "fathom_reconcile_total" {
				for _, m := range mf.GetMetric() {
					for _, lp := range m.GetLabel() {
						if lp.GetName() == "kind" && lp.GetValue() == "AddonCheck" {
							found = true
						}
					}
				}
			}
		}
		Expect(found).To(BeTrue(), "expected fathom_reconcile_total series for kind=AddonCheck")

		// Smoke test for adapter execution metrics (SKA-290)
		metrics.AdapterRunDuration.Reset()
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		adapterMfs, err := ctrlmetrics.Registry.Gather()
		Expect(err).NotTo(HaveOccurred())

		adapterFound := false
		familyLabelImproved := false
		for _, mf := range adapterMfs {
			if mf.GetName() == "fathom_adapter_run_duration_seconds" {
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
