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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

var _ = Describe("ClusterHealth Controller", func() {
	ctx := context.Background()

	newReconciler := func() *ClusterHealthReconciler {
		return &ClusterHealthReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	}

	// createHealthCheckWithStatus creates a HealthCheck with the supplied
	// labels and writes Result+Summary to its status subresource. The CheckRef
	// is filled with a placeholder; this controller never reads it.
	createHealthCheckWithStatus := func(name string, lbls map[string]string, result fathomv1alpha1.HealthReportResult, summary string) *fathomv1alpha1.HealthCheck {
		hc := &fathomv1alpha1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: lbls},
			Spec: fathomv1alpha1.HealthCheckSpec{
				CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: name + "-target"},
			},
		}
		Expect(k8sClient.Create(ctx, hc)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, hc))).To(Succeed())
		})
		now := metav1.Now()
		hc.Status = fathomv1alpha1.HealthCheckStatus{
			Result:           result,
			Summary:          summary,
			SourceObservedAt: &now,
		}
		Expect(k8sClient.Status().Update(ctx, hc)).To(Succeed())
		return hc
	}

	createClusterHealth := func(name string, spec fathomv1alpha1.ClusterHealthSpec) *fathomv1alpha1.ClusterHealth {
		ch := &fathomv1alpha1.ClusterHealth{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       spec,
		}
		Expect(k8sClient.Create(ctx, ch)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ch))).To(Succeed())
		})
		return ch
	}

	It("rolls up Pass when every selected HealthCheck is passing", func() {
		createHealthCheckWithStatus("hc-pass-1", map[string]string{"team": "alpha"}, fathomv1alpha1.HealthReportResultPass, "alpha 1 ok")
		createHealthCheckWithStatus("hc-pass-2", map[string]string{"team": "alpha"}, fathomv1alpha1.HealthReportResultPass, "alpha 2 ok")
		createClusterHealth("ch-all-pass", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "alpha"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-all-pass", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-all-pass", Namespace: "default"}, &got)).To(Succeed())
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		Expect(got.Status.MatchedCount).To(Equal(int32(2)))
		Expect(got.Status.Children).To(HaveLen(2))
		ready := apiMeta.FindStatusCondition(got.Status.Conditions, clusterHealthConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
	})

	It("rolls up the worst-severity Result when HealthChecks disagree", func() {
		createHealthCheckWithStatus("hc-mix-pass", map[string]string{"team": "beta"}, fathomv1alpha1.HealthReportResultPass, "ok")
		createHealthCheckWithStatus("hc-mix-warn", map[string]string{"team": "beta"}, fathomv1alpha1.HealthReportResultWarn, "warning state")
		createHealthCheckWithStatus("hc-mix-fail", map[string]string{"team": "beta"}, fathomv1alpha1.HealthReportResultFail, "failing")
		createClusterHealth("ch-mixed", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "beta"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-mixed", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-mixed", Namespace: "default"}, &got)).To(Succeed())
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultFail), "Fail outranks Warn outranks Pass")
		Expect(got.Status.MatchedCount).To(Equal(int32(3)))
	})

	It("rolls up Error when any selected HealthCheck has Result=Error", func() {
		createHealthCheckWithStatus("hc-err-pass", map[string]string{"team": "gamma"}, fathomv1alpha1.HealthReportResultPass, "")
		createHealthCheckWithStatus("hc-err-fail", map[string]string{"team": "gamma"}, fathomv1alpha1.HealthReportResultFail, "")
		createHealthCheckWithStatus("hc-err-error", map[string]string{"team": "gamma"}, fathomv1alpha1.HealthReportResultError, "")
		createClusterHealth("ch-error", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "gamma"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-error", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-error", Namespace: "default"}, &got)).To(Succeed())
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultError))
	})

	It("counts pending HealthChecks but excludes them from the worst-case Result", func() {
		// Pending: HealthCheck created but Status.Result never set.
		pending := &fathomv1alpha1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{Name: "hc-pending", Namespace: "default", Labels: map[string]string{"team": "delta"}},
			Spec: fathomv1alpha1.HealthCheckSpec{
				CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "pending-target"},
			},
		}
		Expect(k8sClient.Create(ctx, pending)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, pending))).To(Succeed())
		})
		createHealthCheckWithStatus("hc-pending-pass", map[string]string{"team": "delta"}, fathomv1alpha1.HealthReportResultPass, "ok")
		createClusterHealth("ch-pending", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "delta"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-pending", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-pending", Namespace: "default"}, &got)).To(Succeed())
		Expect(got.Status.MatchedCount).To(Equal(int32(2)), "pending HealthCheck contributes to MatchedCount")
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass), "pending HealthCheck does not influence the worst-case roll-up")
		Expect(got.Status.Children).To(HaveLen(2))
	})

	It("returns an empty Result when no HealthChecks match", func() {
		createClusterHealth("ch-no-match", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "epsilon-no-such-team"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-no-match", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-no-match", Namespace: "default"}, &got)).To(Succeed())
		Expect(got.Status.MatchedCount).To(BeNumerically("==", 0))
		Expect(got.Status.Result).To(BeEmpty())
		Expect(got.Status.Children).To(BeEmpty())
		ready := apiMeta.FindStatusCondition(got.Status.Conditions, clusterHealthConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
	})

	It("treats a nil/empty Selector as 'every HealthCheck in the namespace'", func() {
		createHealthCheckWithStatus("hc-empty-selector", map[string]string{"unique": "empty-selector-test"}, fathomv1alpha1.HealthReportResultPass, "ok")
		createClusterHealth("ch-empty-selector", fathomv1alpha1.ClusterHealthSpec{Selector: nil})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-empty-selector", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-empty-selector", Namespace: "default"}, &got)).To(Succeed())
		Expect(got.Status.MatchedCount).To(BeNumerically(">=", 1), "nil selector should match at least the just-created HealthCheck")
	})

	It("orders Children deterministically by name", func() {
		createHealthCheckWithStatus("hc-order-c", map[string]string{"team": "zeta"}, fathomv1alpha1.HealthReportResultPass, "")
		createHealthCheckWithStatus("hc-order-a", map[string]string{"team": "zeta"}, fathomv1alpha1.HealthReportResultPass, "")
		createHealthCheckWithStatus("hc-order-b", map[string]string{"team": "zeta"}, fathomv1alpha1.HealthReportResultPass, "")
		createClusterHealth("ch-ordered", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "zeta"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-ordered", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-ordered", Namespace: "default"}, &got)).To(Succeed())
		names := []string{got.Status.Children[0].Name, got.Status.Children[1].Name, got.Status.Children[2].Name}
		Expect(names).To(Equal([]string{"hc-order-a", "hc-order-b", "hc-order-c"}))
	})

	It("does not write status on a no-op reconcile", func() {
		createHealthCheckWithStatus("hc-noop", map[string]string{"team": "eta"}, fathomv1alpha1.HealthReportResultPass, "")
		ch := createClusterHealth("ch-noop", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "eta"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: ch.Name, Namespace: ch.Namespace},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ch.Name, Namespace: ch.Namespace}, ch)).To(Succeed())
		rvAfterFirst := ch.ResourceVersion
		time.Sleep(50 * time.Millisecond)

		_, err = newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: ch.Name, Namespace: ch.Namespace},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ch.Name, Namespace: ch.Namespace}, ch)).To(Succeed())
		Expect(ch.ResourceVersion).To(Equal(rvAfterFirst), "second reconcile against unchanged inputs must not write status")
	})

	It("enqueues only ClusterHealths whose selector matches the changed HealthCheck", func() {
		hc := createHealthCheckWithStatus("hc-watch-match", map[string]string{"team": "theta"}, fathomv1alpha1.HealthReportResultPass, "")
		createClusterHealth("ch-watch-match", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "theta"}},
		})
		createClusterHealth("ch-watch-nomatch", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "iota"}},
		})

		got := newReconciler().clusterHealthsForHealthCheck(ctx, hc)
		names := []string{}
		for _, r := range got {
			names = append(names, r.Name)
		}
		Expect(names).To(ContainElement("ch-watch-match"))
		Expect(names).NotTo(ContainElement("ch-watch-nomatch"))
	})
})
