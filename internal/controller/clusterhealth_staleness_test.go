/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// ClusterHealth staleness semantics (skaphos/fathom#277).
//
// The aggregate folds the WORST verdict across children but used to publish the
// NEWEST observation, so a frozen child's stale Fail kept propagating while a
// live sibling made the roll-up look perfectly current. These specs pin the
// corrected contract: the published observation is the STALEST contributing
// evidence, so a frozen child can never hide behind a healthy one.
var _ = Describe("ClusterHealth staleness", func() {
	ctx := context.Background()

	newReconciler := func() *ClusterHealthReconciler {
		return &ClusterHealthReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	}

	// createHealthCheckObservedAt creates a HealthCheck whose SourceObservedAt is
	// pinned to observedAt. A nil observedAt leaves it unset, which is how a
	// selected-but-never-evaluated child presents.
	createHealthCheckObservedAt := func(name string, lbls map[string]string,
		result fathomv1alpha1.HealthReportResult, observedAt *metav1.Time,
	) *fathomv1alpha1.HealthCheck {
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
		hc.Status = fathomv1alpha1.HealthCheckStatus{
			Result:           result,
			Summary:          string(result) + " for " + name,
			SourceObservedAt: observedAt,
		}
		Expect(k8sClient.Status().Update(ctx, hc)).To(Succeed())
		return hc
	}

	createAggregate := func(name string, selector map[string]string) {
		ch := &fathomv1alpha1.ClusterHealth{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: fathomv1alpha1.ClusterHealthSpec{
				Selector: &metav1.LabelSelector{MatchLabels: selector},
			},
		}
		Expect(k8sClient.Create(ctx, ch)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ch))).To(Succeed())
		})
	}

	reconcileAggregate := func(name string) fathomv1alpha1.ClusterHealth {
		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name},
		})
		Expect(err).NotTo(HaveOccurred())
		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, &got)).To(Succeed())
		return got
	}

	// T008 — the regression this feature exists to fix (SC-005).
	It("reports the frozen child's observation, not the live sibling's", func() {
		lbls := map[string]string{"stale-case": "masked"}
		frozen := metav1.NewTime(time.Now().Add(-6 * time.Hour).Truncate(time.Second))
		live := metav1.NewTime(time.Now().Truncate(time.Second))

		createHealthCheckObservedAt("stale-frozen-fail", lbls, fathomv1alpha1.HealthReportResultFail, &frozen)
		createHealthCheckObservedAt("stale-live-pass", lbls, fathomv1alpha1.HealthReportResultPass, &live)
		createAggregate("ch-staleness-masked", lbls)

		got := reconcileAggregate("ch-staleness-masked")

		By("still folding the frozen child's Fail into the verdict")
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultFail))

		By("publishing the STALEST observation so the Fail cannot look fresh")
		Expect(got.Status.ObservedAt).NotTo(BeNil())
		Expect(got.Status.ObservedAt.Unix()).To(Equal(frozen.Unix()),
			"the aggregate must not report the live sibling's timestamp; that is what masked the frozen child")
	})

	// T009 — staleness must never alter the verdict (D1, FR-007, SC-006).
	DescribeTable("leaves the worst-of verdict untouched regardless of staleness",
		func(childResults []fathomv1alpha1.HealthReportResult, want fathomv1alpha1.HealthReportResult) {
			// Kubernetes names are RFC1123: lowercase only.
			slug := strings.ToLower(string(want))
			lbls := map[string]string{"verdict-case": slug + "-fold"}
			old := metav1.NewTime(time.Now().Add(-9 * time.Hour))
			for i, r := range childResults {
				// Every child is deliberately stale; the verdict must be unmoved.
				name := "verdict-" + slug + "-" + string(rune('a'+i))
				createHealthCheckObservedAt(name, lbls, r, &old)
			}
			createAggregate("ch-verdict-"+slug, lbls)

			got := reconcileAggregate("ch-verdict-" + slug)
			Expect(got.Status.Result).To(Equal(want))
		},
		Entry("Error outranks Fail",
			[]fathomv1alpha1.HealthReportResult{fathomv1alpha1.HealthReportResultFail, fathomv1alpha1.HealthReportResultError},
			fathomv1alpha1.HealthReportResultError),
		Entry("Fail outranks Unknown",
			[]fathomv1alpha1.HealthReportResult{fathomv1alpha1.HealthReportResultUnknown, fathomv1alpha1.HealthReportResultFail},
			fathomv1alpha1.HealthReportResultFail),
		Entry("Pass stays Pass",
			[]fathomv1alpha1.HealthReportResult{fathomv1alpha1.HealthReportResultPass, fathomv1alpha1.HealthReportResultPass},
			fathomv1alpha1.HealthReportResultPass),
	)

	// T010 — the gauge is fed from the same field, so it must agree (FR-009).
	It("exports a staleness gauge that agrees with the published status", func() {
		lbls := map[string]string{"stale-case": "gauge"}
		frozen := metav1.NewTime(time.Now().Add(-4 * time.Hour).Truncate(time.Second))
		live := metav1.NewTime(time.Now().Truncate(time.Second))

		createHealthCheckObservedAt("gauge-frozen", lbls, fathomv1alpha1.HealthReportResultFail, &frozen)
		createHealthCheckObservedAt("gauge-live", lbls, fathomv1alpha1.HealthReportResultPass, &live)
		createAggregate("ch-staleness-gauge", lbls)

		got := reconcileAggregate("ch-staleness-gauge")
		Expect(got.Status.ObservedAt).NotTo(BeNil())

		value, ok := lastRunGaugeValue(GinkgoTB(), "ClusterHealth", "ch-staleness-gauge", "")
		Expect(ok).To(BeTrue(), "expected a last-run series for the aggregate")
		Expect(int64(value)).To(Equal(got.Status.ObservedAt.Unix()),
			"the gauge and the status must not be able to disagree")
		Expect(int64(value)).To(Equal(frozen.Unix()))
	})

	// T011 — edge cases from the spec.
	It("treats a never-observed child as the strongest staleness signal", func() {
		lbls := map[string]string{"stale-case": "never-ran"}
		live := metav1.NewTime(time.Now())
		createHealthCheckObservedAt("never-observed", lbls, fathomv1alpha1.HealthReportResultPass, nil)
		createHealthCheckObservedAt("never-sibling", lbls, fathomv1alpha1.HealthReportResultPass, &live)
		createAggregate("ch-never-ran", lbls)

		got := reconcileAggregate("ch-never-ran")

		Expect(got.Status.ObservedAt).To(BeNil(),
			"a selected child that has never been evaluated must dominate; nil renders as the gauge's 0 never-ran sentinel")

		value, ok := lastRunGaugeValue(GinkgoTB(), "ClusterHealth", "ch-never-ran", "")
		Expect(ok).To(BeTrue())
		Expect(value).To(Equal(0.0))
	})

	It("never reports an aggregate as more current than now", func() {
		lbls := map[string]string{"stale-case": "skew"}
		future := metav1.NewTime(time.Now().Add(3 * time.Hour))
		createHealthCheckObservedAt("skewed-child", lbls, fathomv1alpha1.HealthReportResultPass, &future)
		createAggregate("ch-clock-skew", lbls)

		before := time.Now()
		got := reconcileAggregate("ch-clock-skew")

		Expect(got.Status.ObservedAt).NotTo(BeNil())
		Expect(got.Status.ObservedAt.Time).NotTo(BeTemporally(">", before.Add(time.Minute)),
			"a child clock running fast must not let the aggregate look indefinitely current")
	})

	It("reports the stalest evidence when every child is frozen", func() {
		lbls := map[string]string{"stale-case": "all-frozen"}
		older := metav1.NewTime(time.Now().Add(-12 * time.Hour).Truncate(time.Second))
		old := metav1.NewTime(time.Now().Add(-2 * time.Hour).Truncate(time.Second))

		createHealthCheckObservedAt("frozen-older", lbls, fathomv1alpha1.HealthReportResultFail, &older)
		createHealthCheckObservedAt("frozen-old", lbls, fathomv1alpha1.HealthReportResultFail, &old)
		createAggregate("ch-all-frozen", lbls)

		got := reconcileAggregate("ch-all-frozen")

		Expect(got.Status.ObservedAt).NotTo(BeNil(),
			"an all-frozen aggregate must not fall back to fresh through an empty-set default")
		Expect(got.Status.ObservedAt.Unix()).To(Equal(older.Unix()))
	})

	It("leaves the NoMatches path unchanged", func() {
		createAggregate("ch-no-matches-stale", map[string]string{"stale-case": "matches-nothing"})

		got := reconcileAggregate("ch-no-matches-stale")

		Expect(got.Status.MatchedCount).To(Equal(int32(0)))
		Expect(got.Status.Children).To(BeEmpty())
		Expect(got.Status.ObservedAt).To(BeNil(),
			"an aggregate matching nothing must not additionally claim a staleness timestamp")
	})
})
