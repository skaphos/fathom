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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/metrics"
)

var _ = Describe("ClusterHealth Controller", func() {
	ctx := context.Background()

	newReconciler := func() *ClusterHealthReconciler {
		return &ClusterHealthReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	}

	// createHealthCheckIn creates a HealthCheck in the given namespace with the
	// supplied labels and writes Result+Summary to its status subresource. The
	// CheckRef is filled with a placeholder; this controller never reads it.
	createHealthCheckIn := func(namespace, name string, lbls map[string]string, result fathomv1alpha1.HealthReportResult, summary string) *fathomv1alpha1.HealthCheck {
		hc := &fathomv1alpha1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: lbls},
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

	createHealthCheckWithStatus := func(name string, lbls map[string]string, result fathomv1alpha1.HealthReportResult, summary string) *fathomv1alpha1.HealthCheck {
		return createHealthCheckIn("default", name, lbls, result, summary)
	}

	// ensureNamespace creates the namespace if it does not already exist.
	// envtest has no namespace controller, so namespaces are never cleaned up;
	// specs use dedicated names and unique labels to stay isolated.
	ensureNamespace := func(name string) {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		err := k8sClient.Create(ctx, ns)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	}

	createClusterHealth := func(name string, spec fathomv1alpha1.ClusterHealthSpec) *fathomv1alpha1.ClusterHealth {
		ch := &fathomv1alpha1.ClusterHealth{
			ObjectMeta: metav1.ObjectMeta{Name: name},
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
			NamespacedName: types.NamespacedName{Name: "ch-all-pass"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-all-pass"}, &got)).To(Succeed())
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		Expect(got.Status.MatchedCount).To(Equal(int32(2)))
		Expect(got.Status.Children).To(HaveLen(2))
		ready := apiMeta.FindStatusCondition(got.Status.Conditions, clusterHealthConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))

		// Smoke test for ClusterHealth reconciler instrumentation
		metrics.ReconcileTotal.Reset()
		_, err = newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-all-pass"},
		})
		Expect(err).NotTo(HaveOccurred())

		mfs, err := ctrlmetrics.Registry.Gather()
		Expect(err).NotTo(HaveOccurred())

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "fathom_reconcile_total" {
				for _, m := range mf.GetMetric() {
					for _, lp := range m.GetLabel() {
						if lp.GetName() == "kind" && lp.GetValue() == "ClusterHealth" {
							found = true
						}
					}
				}
			}
		}
		Expect(found).To(BeTrue(), "expected fathom_reconcile_total series for kind=ClusterHealth")
	})

	It("rolls up the worst-severity Result when HealthChecks disagree", func() {
		createHealthCheckWithStatus("hc-mix-pass", map[string]string{"team": "beta"}, fathomv1alpha1.HealthReportResultPass, "ok")
		createHealthCheckWithStatus("hc-mix-warn", map[string]string{"team": "beta"}, fathomv1alpha1.HealthReportResultWarn, "warning state")
		createHealthCheckWithStatus("hc-mix-fail", map[string]string{"team": "beta"}, fathomv1alpha1.HealthReportResultFail, "failing")
		createClusterHealth("ch-mixed", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "beta"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-mixed"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-mixed"}, &got)).To(Succeed())
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
			NamespacedName: types.NamespacedName{Name: "ch-error"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-error"}, &got)).To(Succeed())
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
			NamespacedName: types.NamespacedName{Name: "ch-pending"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-pending"}, &got)).To(Succeed())
		Expect(got.Status.MatchedCount).To(Equal(int32(2)), "pending HealthCheck contributes to MatchedCount")
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass), "pending HealthCheck does not influence the worst-case roll-up")
		Expect(got.Status.Children).To(HaveLen(2))
	})

	It("returns an empty Result when no HealthChecks match", func() {
		createClusterHealth("ch-no-match", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "epsilon-no-such-team"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-no-match"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-no-match"}, &got)).To(Succeed())
		Expect(got.Status.MatchedCount).To(BeNumerically("==", 0))
		Expect(got.Status.Result).To(BeEmpty())
		Expect(got.Status.Children).To(BeEmpty())
		ready := apiMeta.FindStatusCondition(got.Status.Conditions, clusterHealthConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
	})

	It("clears aggregate fields when a previously valid selector becomes invalid", func() {
		createHealthCheckWithStatus("hc-selector-goes-invalid", map[string]string{"team": "invalid-selector"}, fathomv1alpha1.HealthReportResultPass, "ok")
		ch := createClusterHealth("ch-selector-goes-invalid", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "invalid-selector"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: ch.Name},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ch.Name}, ch)).To(Succeed())
		Expect(ch.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		Expect(ch.Status.Children).NotTo(BeEmpty())

		ch.Spec.Selector = &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key:      "team",
			Operator: metav1.LabelSelectorOpIn,
		}}}
		Expect(k8sClient.Update(ctx, ch)).To(Succeed())

		_, err = newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: ch.Name},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ch.Name}, &got)).To(Succeed())
		ready := apiMeta.FindStatusCondition(got.Status.Conditions, clusterHealthConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("InvalidSelector"))
		Expect(got.Status.Result).To(BeEmpty())
		Expect(got.Status.MatchedCount).To(BeNumerically("==", 0))
		Expect(got.Status.Children).To(BeEmpty())
		Expect(got.Status.ObservedAt).To(BeNil())
	})

	It("treats a nil/empty Selector as 'every HealthCheck in scope'", func() {
		createHealthCheckWithStatus("hc-empty-selector", map[string]string{"unique": "empty-selector-test"}, fathomv1alpha1.HealthReportResultPass, "ok")
		createClusterHealth("ch-empty-selector", fathomv1alpha1.ClusterHealthSpec{Selector: nil})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-empty-selector"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-empty-selector"}, &got)).To(Succeed())
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
			NamespacedName: types.NamespacedName{Name: "ch-ordered"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-ordered"}, &got)).To(Succeed())
		names := []string{got.Status.Children[0].Name, got.Status.Children[1].Name, got.Status.Children[2].Name}
		Expect(names).To(Equal([]string{"hc-order-a", "hc-order-b", "hc-order-c"}))
	})

	It("does not write status on a no-op reconcile", func() {
		createHealthCheckWithStatus("hc-noop", map[string]string{"team": "eta"}, fathomv1alpha1.HealthReportResultPass, "")
		ch := createClusterHealth("ch-noop", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "eta"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: ch.Name},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ch.Name}, ch)).To(Succeed())
		rvAfterFirst := ch.ResourceVersion
		time.Sleep(50 * time.Millisecond)

		_, err = newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: ch.Name},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ch.Name}, ch)).To(Succeed())
		Expect(ch.ResourceVersion).To(Equal(rvAfterFirst), "second reconcile against unchanged inputs must not write status")
	})

	It("aggregates HealthChecks across namespaces", func() {
		ensureNamespace("ch-scope-a")
		ensureNamespace("ch-scope-b")
		createHealthCheckIn("ch-scope-a", "hc-xns-pass", map[string]string{"xns": "cross"}, fathomv1alpha1.HealthReportResultPass, "ok")
		createHealthCheckIn("ch-scope-b", "hc-xns-fail", map[string]string{"xns": "cross"}, fathomv1alpha1.HealthReportResultFail, "broken")
		createClusterHealth("ch-cross-ns", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"xns": "cross"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-cross-ns"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-cross-ns"}, &got)).To(Succeed())
		Expect(got.Status.MatchedCount).To(Equal(int32(2)), "HealthChecks in different namespaces both contribute")
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultFail))
		Expect(got.Status.Children).To(HaveLen(2))
		Expect(got.Status.Children[0].Namespace).To(Equal("ch-scope-a"))
		Expect(got.Status.Children[1].Namespace).To(Equal("ch-scope-b"))
	})

	It("narrows the aggregate to spec.namespaces when set", func() {
		ensureNamespace("ch-scope-a")
		ensureNamespace("ch-scope-b")
		createHealthCheckIn("ch-scope-a", "hc-nsfilter-in", map[string]string{"nsfilter": "yes"}, fathomv1alpha1.HealthReportResultPass, "ok")
		createHealthCheckIn("ch-scope-b", "hc-nsfilter-out", map[string]string{"nsfilter": "yes"}, fathomv1alpha1.HealthReportResultFail, "excluded")
		createClusterHealth("ch-ns-filter", fathomv1alpha1.ClusterHealthSpec{
			Selector:   &metav1.LabelSelector{MatchLabels: map[string]string{"nsfilter": "yes"}},
			Namespaces: []string{"ch-scope-a"},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-ns-filter"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-ns-filter"}, &got)).To(Succeed())
		Expect(got.Status.MatchedCount).To(Equal(int32(1)), "the HealthCheck outside spec.namespaces is excluded")
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		Expect(got.Status.Children[0].Namespace).To(Equal("ch-scope-a"))
		Expect(got.Status.Children[0].Name).To(Equal("hc-nsfilter-in"))
	})

	It("orders Children by namespace before name", func() {
		ensureNamespace("ch-scope-a")
		ensureNamespace("ch-scope-b")
		createHealthCheckIn("ch-scope-b", "hc-nsorder-a", map[string]string{"nsorder": "t"}, fathomv1alpha1.HealthReportResultPass, "")
		createHealthCheckIn("ch-scope-a", "hc-nsorder-z", map[string]string{"nsorder": "t"}, fathomv1alpha1.HealthReportResultPass, "")
		createClusterHealth("ch-ns-ordered", fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"nsorder": "t"}},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "ch-ns-ordered"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-ns-ordered"}, &got)).To(Succeed())
		Expect(got.Status.Children).To(HaveLen(2))
		Expect(got.Status.Children[0].Namespace).To(Equal("ch-scope-a"), "namespace sorts before name")
		Expect(got.Status.Children[0].Name).To(Equal("hc-nsorder-z"))
		Expect(got.Status.Children[1].Namespace).To(Equal("ch-scope-b"))
		Expect(got.Status.Children[1].Name).To(Equal("hc-nsorder-a"))
	})

	It("does not enqueue a ClusterHealth whose spec.namespaces excludes the changed HealthCheck", func() {
		ensureNamespace("ch-scope-a")
		hc := createHealthCheckIn("ch-scope-a", "hc-watch-nsfilter", map[string]string{"watchns": "t"}, fathomv1alpha1.HealthReportResultPass, "")
		createClusterHealth("ch-watch-ns-covered", fathomv1alpha1.ClusterHealthSpec{
			Selector:   &metav1.LabelSelector{MatchLabels: map[string]string{"watchns": "t"}},
			Namespaces: []string{"ch-scope-a"},
		})
		createClusterHealth("ch-watch-ns-excluded", fathomv1alpha1.ClusterHealthSpec{
			Selector:   &metav1.LabelSelector{MatchLabels: map[string]string{"watchns": "t"}},
			Namespaces: []string{"some-other-namespace"},
		})

		got := newReconciler().clusterHealthsForHealthCheck(ctx, hc)
		names := []string{}
		for _, r := range got {
			names = append(names, r.Name)
		}
		Expect(names).To(ContainElement("ch-watch-ns-covered"))
		Expect(names).NotTo(ContainElement("ch-watch-ns-excluded"))
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
