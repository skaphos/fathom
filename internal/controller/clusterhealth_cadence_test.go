/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// Cadence publication (skaphos/fathom#277).
//
// Staleness is only meaningful relative to how often a check is meant to run:
// a result 20 minutes old is healthy at an hourly cadence and badly overdue at
// a five-minute one. Publishing each check's effective cadence is what lets one
// alerting rule be correct for every kind, instead of a constant that suits the
// default AddonCheck interval and false-positives on everything slower.
var _ = Describe("check cadence publication", func() {
	ctx := context.Background()

	intervalGauge := func(kind, name, namespace string) (float64, bool) {
		return gatherGaugeValue("fathom_check_interval_seconds", map[string]string{
			"kind": kind, "name": name, "namespace": namespace,
		})
	}

	// T017 — each self-scheduling kind publishes the cadence actually in force.
	Context("a self-scheduling check", func() {
		It("publishes its default cadence when spec.interval is unset", func() {
			check := &fathomv1alpha1.AddonCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "cadence-default", Namespace: "default"},
				Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "coredns"},
			}
			Expect(k8sClient.Create(ctx, check)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed())
			})

			_, err := (&AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}).Reconcile(ctx,
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "cadence-default"}})
			Expect(err).NotTo(HaveOccurred())

			value, ok := intervalGauge("AddonCheck", "cadence-default", "default")
			Expect(ok).To(BeTrue(), "every self-scheduling check must publish a cadence")
			Expect(value).To(Equal(defaultAddonCheckInterval.Seconds()))
		})

		It("publishes the per-resource override rather than the default", func() {
			check := &fathomv1alpha1.AddonCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "cadence-override", Namespace: "default"},
				Spec: fathomv1alpha1.AddonCheckSpec{
					AddonType: "coredns",
					Interval:  &metav1.Duration{Duration: 30 * time.Minute},
				},
			}
			Expect(k8sClient.Create(ctx, check)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed())
			})

			_, err := (&AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}).Reconcile(ctx,
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "cadence-override"}})
			Expect(err).NotTo(HaveOccurred())

			value, ok := intervalGauge("AddonCheck", "cadence-override", "default")
			Expect(ok).To(BeTrue())
			Expect(value).To(Equal((30 * time.Minute).Seconds()),
				"the published cadence must be the one in force, not the kind default")
		})
	})

	// T018 — a HealthCheck has no cadence of its own, but its checkRef always
	// names a Fathom-native check whose cadence the operator owns.
	Context("a HealthCheck wrapper", func() {
		It("publishes the wrapped check's cadence", func() {
			target := &fathomv1alpha1.AddonCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "wrapped-target", Namespace: "default"},
				Spec: fathomv1alpha1.AddonCheckSpec{
					AddonType: "coredns",
					Interval:  &metav1.Duration{Duration: 45 * time.Minute},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, target))).To(Succeed())
			})

			hc := &fathomv1alpha1.HealthCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "wrapper-cadence", Namespace: "default"},
				Spec: fathomv1alpha1.HealthCheckSpec{
					CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "wrapped-target"},
				},
			}
			Expect(k8sClient.Create(ctx, hc)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, hc))).To(Succeed())
			})

			_, err := (&HealthCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}).Reconcile(ctx,
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "wrapper-cadence"}})
			Expect(err).NotTo(HaveOccurred())

			var got fathomv1alpha1.HealthCheck
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "wrapper-cadence"}, &got)).To(Succeed())
			Expect(got.Status.SourceInterval).NotTo(BeNil(),
				"the wrapper must surface the wrapped cadence; an aggregate can reach it no other way")
			Expect(got.Status.SourceInterval.Duration).To(Equal(45 * time.Minute))

			value, ok := intervalGauge("HealthCheck", "wrapper-cadence", "default")
			Expect(ok).To(BeTrue())
			Expect(value).To(Equal((45 * time.Minute).Seconds()))
		})

		It("publishes no cadence for an unresolvable kind, without failing the reconcile", func() {
			hc := &fathomv1alpha1.HealthCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "wrapper-unknown-kind", Namespace: "default"},
				Spec: fathomv1alpha1.HealthCheckSpec{
					CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "ReachabilityCheck", Name: "not-built-yet"},
				},
			}
			Expect(k8sClient.Create(ctx, hc)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, hc))).To(Succeed())
			})

			_, err := (&HealthCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}).Reconcile(ctx,
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "wrapper-unknown-kind"}})
			Expect(err).NotTo(HaveOccurred(),
				"an unsupported checkRef kind is a status condition, never a reconcile failure")

			var got fathomv1alpha1.HealthCheck
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "wrapper-unknown-kind"}, &got)).To(Succeed())
			Expect(got.Status.SourceInterval).To(BeNil())

			_, ok := intervalGauge("HealthCheck", "wrapper-unknown-kind", "default")
			Expect(ok).To(BeFalse(),
				"an unresolvable cadence must leave the series absent, not set it to zero — zero would read as permanently overdue")
		})
	})

	// T019 — the aggregate can only be as current as its slowest contributor.
	Context("a ClusterHealth aggregate", func() {
		It("publishes the slowest child's cadence, so a healthy slow child cannot poison it", func() {
			lbls := map[string]string{"cadence-case": "mixed"}

			mk := func(name string, interval time.Duration) {
				target := &fathomv1alpha1.AddonCheck{
					ObjectMeta: metav1.ObjectMeta{Name: name + "-target", Namespace: "default"},
					Spec: fathomv1alpha1.AddonCheckSpec{
						AddonType: "coredns",
						Interval:  &metav1.Duration{Duration: interval},
					},
				}
				Expect(k8sClient.Create(ctx, target)).To(Succeed())
				DeferCleanup(func() {
					Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, target))).To(Succeed())
				})

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
					Result:           fathomv1alpha1.HealthReportResultPass,
					SourceInterval:   &metav1.Duration{Duration: interval},
					SourceObservedAt: &now,
				}
				Expect(k8sClient.Status().Update(ctx, hc)).To(Succeed())
			}

			mk("cadence-fast-child", 5*time.Minute)
			mk("cadence-slow-child", time.Hour)

			ch := &fathomv1alpha1.ClusterHealth{
				ObjectMeta: metav1.ObjectMeta{Name: "ch-mixed-cadence"},
				Spec: fathomv1alpha1.ClusterHealthSpec{
					Selector: &metav1.LabelSelector{MatchLabels: lbls},
				},
			}
			Expect(k8sClient.Create(ctx, ch)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ch))).To(Succeed())
			})

			_, err := (&ClusterHealthReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}).Reconcile(ctx,
				reconcile.Request{NamespacedName: types.NamespacedName{Name: "ch-mixed-cadence"}})
			Expect(err).NotTo(HaveOccurred())

			value, ok := intervalGauge("ClusterHealth", "ch-mixed-cadence", "")
			Expect(ok).To(BeTrue())
			Expect(value).To(Equal(time.Hour.Seconds()),
				"taking the fastest child would mark every mixed-cadence aggregate permanently overdue")

			By("and both children remain within their own cadence, so nothing is overdue")
			var got fathomv1alpha1.ClusterHealth
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-mixed-cadence"}, &got)).To(Succeed())
			Expect(got.Status.ObservedAt).NotTo(BeNil())
			age := time.Since(got.Status.ObservedAt.Time)
			Expect(age).To(BeNumerically("<", 3*time.Hour),
				"3x the published hourly cadence must not be exceeded by healthy children")
		})
	})
})
