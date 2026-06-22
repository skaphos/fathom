/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/metrics"
)

var _ = Describe("HealthCheck Controller", func() {
	ctx := context.Background()

	newReconciler := func() *HealthCheckReconciler {
		return &HealthCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	}

	// createAddonCheckWithStatus creates an AddonCheck and writes the supplied
	// status fields via the status subresource. envtest preserves status writes,
	// so the HealthCheck reconciler can read them back.
	createAddonCheckWithStatus := func(name string, status fathomv1alpha1.AddonCheckStatus) *fathomv1alpha1.AddonCheck {
		ac := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager"},
		}
		Expect(k8sClient.Create(ctx, ac)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ac))).To(Succeed())
		})
		ac.Status = status
		Expect(k8sClient.Status().Update(ctx, ac)).To(Succeed())
		return ac
	}

	createHealthCheck := func(name string, spec fathomv1alpha1.HealthCheckSpec) *fathomv1alpha1.HealthCheck {
		hc := &fathomv1alpha1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       spec,
		}
		Expect(k8sClient.Create(ctx, hc)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, hc))).To(Succeed())
		})
		return hc
	}

	It("mirrors the target AddonCheck status into HealthCheck.Status", func() {
		runTime := metav1.NewTime(time.Now().Add(-time.Minute))
		createAddonCheckWithStatus("ac-mirror-pass", fathomv1alpha1.AddonCheckStatus{
			LastResult:     "Pass",
			LastRunTime:    &runTime,
			LastReportName: "ac-mirror-pass-abcd",
			Conditions: []metav1.Condition{{
				Type:               healthCheckConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             "RunCompleted",
				Message:            "AddonCheck adapter run completed and a HealthReport was created.",
				LastTransitionTime: metav1.NewTime(time.Now()),
			}},
		})
		createHealthCheck("hc-mirror-pass", fathomv1alpha1.HealthCheckSpec{
			CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "ac-mirror-pass"},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "hc-mirror-pass", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.HealthCheck
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "hc-mirror-pass", Namespace: "default"}, &got)).To(Succeed())
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		Expect(got.Status.LastReportName).To(Equal("ac-mirror-pass-abcd"))
		Expect(got.Status.SourceObservedAt).NotTo(BeNil())
		Expect(got.Status.Summary).To(ContainSubstring("HealthReport was created"))
		ready := apiMeta.FindStatusCondition(got.Status.Conditions, healthCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal("TargetMirrored"))

		// Smoke test: the reconciler now records metrics via RecordReconcile.
		// We mainly verify it doesn't panic and that we can interact with the metric.
		metrics.ReconcileTotal.Reset()
		_, err = newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "hc-mirror-pass", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		// After the reconcile we should be able to see at least the series we just created.
		// Using Gather from the controller-runtime registry (where our metrics live).
		mfs, err := ctrlmetrics.Registry.Gather()
		Expect(err).NotTo(HaveOccurred())

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "fathom_reconcile_total" {
				for _, m := range mf.GetMetric() {
					for _, lp := range m.GetLabel() {
						if lp.GetName() == "kind" && lp.GetValue() == "HealthCheck" {
							found = true
						}
					}
				}
			}
		}
		Expect(found).To(BeTrue(), "expected to find a fathom_reconcile_total series for kind=HealthCheck")
	})

	It("records TargetNotFound when the referenced AddonCheck does not exist", func() {
		createHealthCheck("hc-missing-target", fathomv1alpha1.HealthCheckSpec{
			CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "does-not-exist"},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "hc-missing-target", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.HealthCheck
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "hc-missing-target", Namespace: "default"}, &got)).To(Succeed())
		ready := apiMeta.FindStatusCondition(got.Status.Conditions, healthCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("TargetNotFound"))
		Expect(got.Status.Result).To(BeEmpty())
	})

	It("rejects unsupported CheckRef.Kind values", func() {
		createHealthCheck("hc-unsupported", fathomv1alpha1.HealthCheckSpec{
			CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "DNSCheck", Name: "future-kind"},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "hc-unsupported", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.HealthCheck
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "hc-unsupported", Namespace: "default"}, &got)).To(Succeed())
		ready := apiMeta.FindStatusCondition(got.Status.Conditions, healthCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("UnsupportedKind"))
	})

	It("preserves the last mirrored snapshot when paused", func() {
		runTime := metav1.NewTime(time.Now().Add(-time.Hour))
		createAddonCheckWithStatus("ac-paused-source", fathomv1alpha1.AddonCheckStatus{
			LastResult:     "Pass",
			LastRunTime:    &runTime,
			LastReportName: "ac-paused-source-xyz",
		})
		hc := createHealthCheck("hc-paused", fathomv1alpha1.HealthCheckSpec{
			CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "ac-paused-source"},
		})

		// First reconcile mirrors successfully.
		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace}, hc)).To(Succeed())
		Expect(hc.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		Expect(hc.Status.LastReportName).To(Equal("ac-paused-source-xyz"))

		// Flip Paused=true and update the source so the reconciler would mirror something different if it ran.
		hc.Spec.Paused = true
		Expect(k8sClient.Update(ctx, hc)).To(Succeed())
		var src fathomv1alpha1.AddonCheck
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ac-paused-source", Namespace: "default"}, &src)).To(Succeed())
		src.Status.LastResult = "Fail"
		src.Status.LastReportName = "would-be-mirrored-if-not-paused"
		Expect(k8sClient.Status().Update(ctx, &src)).To(Succeed())

		_, err = newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace}, hc)).To(Succeed())
		Expect(hc.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass), "Result should be preserved while paused")
		Expect(hc.Status.LastReportName).To(Equal("ac-paused-source-xyz"), "LastReportName should be preserved while paused")
		paused := apiMeta.FindStatusCondition(hc.Status.Conditions, healthCheckConditionPaused)
		Expect(paused).NotTo(BeNil())
		Expect(paused.Status).To(Equal(metav1.ConditionTrue))
		ready := apiMeta.FindStatusCondition(hc.Status.Conditions, healthCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("Paused"))
	})

	It("does not write status on a no-op reconcile", func() {
		runTime := metav1.NewTime(time.Now())
		createAddonCheckWithStatus("ac-noop", fathomv1alpha1.AddonCheckStatus{
			LastResult: "Pass", LastRunTime: &runTime, LastReportName: "ac-noop-1",
		})
		hc := createHealthCheck("hc-noop", fathomv1alpha1.HealthCheckSpec{
			CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "ac-noop"},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace}, hc)).To(Succeed())
		rvAfterFirst := hc.ResourceVersion

		_, err = newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace}, hc)).To(Succeed())
		Expect(hc.ResourceVersion).To(Equal(rvAfterFirst), "second reconcile should not write status")
	})

	It("returns no requests for unrelated AddonCheck status changes", func() {
		createHealthCheck("hc-watch-points-elsewhere", fathomv1alpha1.HealthCheckSpec{
			CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "some-other-target"},
		})
		// Create the unrelated AddonCheck the watch will fire on.
		other := createAddonCheckWithStatus("ac-unrelated", fathomv1alpha1.AddonCheckStatus{LastResult: "Pass"})

		got := newReconciler().healthChecksForAddonCheck(ctx, other)
		Expect(got).To(BeEmpty(), "AddonCheck with no HealthCheck pointing at it must not enqueue anything")
	})

	It("enqueues every HealthCheck that points at a changed AddonCheck", func() {
		createAddonCheckWithStatus("ac-watch-target", fathomv1alpha1.AddonCheckStatus{LastResult: "Pass"})
		createHealthCheck("hc-watch-a", fathomv1alpha1.HealthCheckSpec{
			CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "ac-watch-target"},
		})
		createHealthCheck("hc-watch-b", fathomv1alpha1.HealthCheckSpec{
			CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "ac-watch-target"},
		})

		var src fathomv1alpha1.AddonCheck
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ac-watch-target", Namespace: "default"}, &src)).To(Succeed())
		got := newReconciler().healthChecksForAddonCheck(ctx, &src)
		names := []string{}
		for _, r := range got {
			names = append(names, r.Name)
		}
		Expect(names).To(ConsistOf("hc-watch-a", "hc-watch-b"))
	})
})
