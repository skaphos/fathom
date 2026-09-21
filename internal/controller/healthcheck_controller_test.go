/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

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

// transientAddonCheckGetClient fails every AddonCheck Get with a non-NotFound
// error, simulating an API-server blip during the mirror's target lookup.
type transientAddonCheckGetClient struct {
	client.Client
}

func (c transientAddonCheckGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*fathomv1alpha1.AddonCheck); ok {
		return apierrors.NewInternalError(errors.New("injected transient target lookup failure"))
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

var _ = Describe("HealthCheck Controller", func() {
	ctx := context.Background()

	newReconciler := func() *HealthCheckReconciler {
		return &HealthCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	}

	createAddonCheckWithStatusInNamespace := func(namespace, name string, status fathomv1alpha1.AddonCheckStatus) *fathomv1alpha1.AddonCheck {
		ac := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
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

	// createAddonCheckWithStatus creates an AddonCheck and writes the supplied
	// status fields via the status subresource. envtest preserves status writes,
	// so the HealthCheck reconciler can read them back.
	createAddonCheckWithStatus := func(name string, status fathomv1alpha1.AddonCheckStatus) *fathomv1alpha1.AddonCheck {
		return createAddonCheckWithStatusInNamespace("default", name, status)
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

	It("mirrors an explicit cross-namespace target while ClusterHealth aggregates the local wrapper", func() {
		targetNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "hc-cross-target-"}}
		Expect(k8sClient.Create(ctx, targetNamespace)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, targetNamespace))).To(Succeed())
		})

		runTime := metav1.NewTime(time.Now().Add(-time.Minute))
		createAddonCheckWithStatusInNamespace(targetNamespace.Name, "ac-cross-target", fathomv1alpha1.AddonCheckStatus{
			LastResult:     "Pass",
			LastRunTime:    &runTime,
			LastReportName: "ac-cross-target-1",
			Conditions: []metav1.Condition{{
				Type:               healthCheckConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             "RunCompleted",
				Message:            "cross-namespace source is healthy",
				LastTransitionTime: metav1.Now(),
			}},
		})
		hc := &fathomv1alpha1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hc-cross-namespace",
				Namespace: "default",
				Labels:    map[string]string{"scope": "cross-namespace"},
			},
			Spec: fathomv1alpha1.HealthCheckSpec{
				CheckRef: fathomv1alpha1.CheckTargetRef{
					Kind:      "AddonCheck",
					Namespace: targetNamespace.Name,
					Name:      "ac-cross-target",
				},
			},
		}
		Expect(k8sClient.Create(ctx, hc)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, hc))).To(Succeed()) })

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace},
		})
		Expect(err).NotTo(HaveOccurred())

		ch := &fathomv1alpha1.ClusterHealth{
			ObjectMeta: metav1.ObjectMeta{Name: "ch-cross-namespace-wrapper"},
			Spec: fathomv1alpha1.ClusterHealthSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"scope": "cross-namespace"}},
			},
		}
		Expect(k8sClient.Create(ctx, ch)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ch))).To(Succeed()) })

		_, err = (&ClusterHealthReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}).
			Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: ch.Name}})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ch.Name}, &got)).To(Succeed())
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		Expect(got.Status.MatchedCount).To(Equal(int32(1)))
		Expect(got.Status.Children).To(HaveLen(1))
		Expect(got.Status.Children[0].Name).To(Equal(hc.Name))
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

	It("clears mirrored fields when a previously mirrored target disappears", func() {
		runTime := metav1.NewTime(time.Now().Add(-time.Minute))
		ac := createAddonCheckWithStatus("ac-disappears", fathomv1alpha1.AddonCheckStatus{
			LastResult:     "Pass",
			LastRunTime:    &runTime,
			LastReportName: "ac-disappears-1",
			Conditions: []metav1.Condition{{
				Type:               healthCheckConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             "RunCompleted",
				Message:            "source was healthy",
				LastTransitionTime: metav1.Now(),
			}},
		})
		createHealthCheck("hc-target-disappears", fathomv1alpha1.HealthCheckSpec{
			CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: ac.Name},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "hc-target-disappears", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Delete(ctx, ac)).To(Succeed())

		_, err = newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "hc-target-disappears", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var got fathomv1alpha1.HealthCheck
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "hc-target-disappears", Namespace: "default"}, &got)).To(Succeed())
		ready := apiMeta.FindStatusCondition(got.Status.Conditions, healthCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("TargetNotFound"))
		Expect(got.Status.Result).To(BeEmpty())
		Expect(got.Status.SourceObservedAt).To(BeNil())
		Expect(got.Status.LastReportName).To(BeEmpty())
		Expect(got.Status.Summary).To(BeEmpty())
	})

	It("preserves the last mirrored result and requeues on a transient target lookup failure", func() {
		runTime := metav1.NewTime(time.Now().Add(-time.Minute))
		createAddonCheckWithStatus("ac-transient-blip", fathomv1alpha1.AddonCheckStatus{
			LastResult:     "Pass",
			LastRunTime:    &runTime,
			LastReportName: "ac-transient-blip-1",
			Conditions: []metav1.Condition{{
				Type:               healthCheckConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             "RunCompleted",
				Message:            "source was healthy",
				LastTransitionTime: metav1.Now(),
			}},
		})
		createHealthCheck("hc-transient-blip", fathomv1alpha1.HealthCheckSpec{
			CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "ac-transient-blip"},
		})

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "hc-transient-blip", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		failing := &HealthCheckReconciler{
			Client: transientAddonCheckGetClient{Client: k8sClient},
			Scheme: k8sClient.Scheme(),
		}
		_, err = failing.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "hc-transient-blip", Namespace: "default"},
		})
		Expect(err).To(HaveOccurred(), "a transient target lookup failure must be returned so the reconcile requeues")
		Expect(apierrors.IsNotFound(err)).To(BeFalse())

		var got fathomv1alpha1.HealthCheck
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "hc-transient-blip", Namespace: "default"}, &got)).To(Succeed())
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass),
			"a transient lookup failure must not clear the last-good mirrored result")
		Expect(got.Status.LastReportName).To(Equal("ac-transient-blip-1"))
		Expect(got.Status.SourceObservedAt).NotTo(BeNil())
		Expect(got.Status.Summary).To(Equal("source was healthy"))
		ready := apiMeta.FindStatusCondition(got.Status.Conditions, healthCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("TargetLookupFailed"))
	})

	It("rejects unsupported CheckRef.Kind values", func() {
		createHealthCheck("hc-unsupported", fathomv1alpha1.HealthCheckSpec{
			CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "ReachabilityCheck", Name: "future-kind"},
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
		Expect(hc.Status.SourceInterval).NotTo(BeNil())
		interval, ok := gatherGaugeValue("fathom_check_interval_seconds", map[string]string{
			"kind": "HealthCheck", "name": hc.Name, "namespace": hc.Namespace,
		})
		Expect(ok).To(BeTrue(), "a paused wrapper must retain cadence-relative stale-alert coverage")
		Expect(interval).To(Equal(fathomv1alpha1.DefaultAddonCheckInterval.Seconds()))
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

// ---------------------------------------------------------------------------
// T046 — mirrored readiness and freshness, and the ClusterHealth contract
// ---------------------------------------------------------------------------
//
// data-model.md: "HealthCheck mirrors current readiness/freshness;
// ClusterHealth continues reading only HealthCheck.status."
//
// The bug these specs exist to prevent is named in spec.md: "Old success must
// never be presented as new coverage." A retained Pass is legitimate — evidence
// is preserved across a failed attempt precisely so an operator can still see
// what was last observed — but it must never read as a FRESH success at the
// layer operators actually watch.

// healthReportBlindClient fails any read of a HealthReport and counts the
// attempt. ClusterHealth's input is HealthCheck.status and nothing else
// (AGENTS.md: "It is derived only from HealthCheck.status — never from
// HealthReport history"), so under this client a correct aggregate is
// unaffected and an incorrect one fails loudly instead of quietly consulting
// history.
type healthReportBlindClient struct {
	client.Client
	reads *int
}

func (c healthReportBlindClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*fathomv1alpha1.HealthReport); ok {
		*c.reads++
		return errors.New("ClusterHealth read a HealthReport; its only input is HealthCheck.status")
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c healthReportBlindClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*fathomv1alpha1.HealthReportList); ok {
		*c.reads++
		return errors.New("ClusterHealth listed HealthReports; its only input is HealthCheck.status")
	}
	return c.Client.List(ctx, list, opts...)
}

var _ = Describe("HealthCheck readiness and freshness mirror", func() {
	ctx := context.Background()

	newReconciler := func() *HealthCheckReconciler {
		return &HealthCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	}

	createAddonCheck := func(name string, status fathomv1alpha1.AddonCheckStatus) *fathomv1alpha1.AddonCheck {
		ac := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "runtime-addon"},
		}
		Expect(k8sClient.Create(ctx, ac)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ac))).To(Succeed())
		})
		ac.Status = status
		Expect(k8sClient.Status().Update(ctx, ac)).To(Succeed())
		return ac
	}

	mirror := func(name, target string) fathomv1alpha1.HealthCheck {
		hc := &fathomv1alpha1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: fathomv1alpha1.HealthCheckSpec{
				CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: target},
			},
		}
		Expect(k8sClient.Create(ctx, hc)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, hc))).To(Succeed())
		})
		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())
		var got fathomv1alpha1.HealthCheck
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, &got)).To(Succeed())
		return got
	}

	// passEvidence is completed Pass evidence observed at observedAt, carrying
	// the revision and authority a real run would have recorded.
	passEvidence := func(observedAt metav1.Time) *fathomv1alpha1.AddonCheckEvidence {
		return &fathomv1alpha1.AddonCheckEvidence{
			Verdict:    fathomv1alpha1.AddonCheckEvidenceVerdictPass,
			Coverage:   fathomv1alpha1.AddonCheckCoverageChecksEvaluated,
			Message:    "4 checks evaluated",
			ObservedAt: observedAt,
			Revision: fathomv1alpha1.AddonCheckEvidenceRevision{
				DefinitionUID: "definition-uid-1", DefinitionGeneration: 2,
				SchemaVersion: "v1alpha1", SemanticsVersion: 1,
			},
		}
	}

	// THE regression this task exists for. Evidence is retained across a failed
	// attempt with its ORIGINAL observation — "Attempt Error still preserves
	// previous completed evidence" — so the wrapper keeps showing the stored
	// Pass. What it must never do is present that Pass as current coverage:
	// freshness says Unavailable, readiness says the run could not execute, and
	// the mirrored observation is the evidence's own, not the failed attempt's.
	It("never lets a retained Pass present as a fresh success", func() {
		// Deliberately BOTH ineligible and aged: eligibility is the stronger
		// statement, so Unavailable must not decay into a mere Stale. "Old" is
		// something a later run can fix; "the inputs are gone" is not.
		observed := metav1.NewTime(time.Now().Add(-time.Hour)).Rfc3339Copy()
		attempted := metav1.NewTime(time.Now()).Rfc3339Copy()
		createAddonCheck("ac-retained-pass", fathomv1alpha1.AddonCheckStatus{
			LastResult:               "Pass",
			LastRunTime:              &observed,
			LastSuccessfulEvaluation: passEvidence(observed),
			LatestAttemptAt:          &attempted,
			LatestAttemptOutcome:     fathomv1alpha1.AddonCheckAttemptError,
			LatestAttemptReason:      reasonAccessDenied,
			LatestAttemptMessage:     "the dedicated reader was denied a required read",
			EvidenceFreshness:        fathomv1alpha1.AddonCheckEvidenceUnavailable,
			EvidenceFreshnessReason:  "the inputs this evidence was produced from are no longer eligible",
			Conditions: []metav1.Condition{{
				Type:               addonCheckConditionReady,
				Status:             metav1.ConditionFalse,
				Reason:             reasonAccessDenied,
				Message:            "the dedicated reader was denied a required read",
				LastTransitionTime: attempted,
			}},
		})

		got := mirror("hc-retained-pass", "ac-retained-pass")

		// The stored verdict is still shown: preserved evidence is the point.
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		// ... but every signal that would make it read as a fresh success is
		// explicitly negative.
		Expect(got.Status.EvidenceFreshness).To(Equal(fathomv1alpha1.AddonCheckEvidenceUnavailable))
		Expect(got.Status.EvidenceFreshnessReason).NotTo(BeEmpty())
		Expect(got.Status.SourceReady).NotTo(BeNil())
		Expect(*got.Status.SourceReady).To(BeFalse())
		Expect(got.Status.SourceReadyReason).To(Equal(reasonAccessDenied))
		// The mirrored observation is the evidence's own. Publishing the failed
		// attempt's timestamp here would re-date old evidence at the mirror
		// boundary and make every downstream staleness rule read it as current.
		Expect(got.Status.SourceObservedAt).NotTo(BeNil())
		Expect(got.Status.SourceObservedAt.Time).To(BeTemporally("==", observed.Time))
		Expect(got.Status.SourceObservedAt.Time).NotTo(BeTemporally("==", attempted.Time))
	})

	// An undeterminable run is the one case where the source's own lastRunTime
	// advances past its evidence: the run completed, so it was a real run, but
	// it could not determine health, so it produced no evidence. The mirrored
	// observation answers "when was this addon's health last actually
	// observed", and the answer is the retained evidence's time — pairing an
	// Error verdict with "now" would let a check that has been undeterminable
	// for a week read as freshly observed by every cadence-relative rule
	// downstream.
	It("mirrors the evidence's observation, not a run that determined nothing", func() {
		observed := metav1.NewTime(time.Now().Add(-time.Hour)).Rfc3339Copy()
		ran := metav1.NewTime(time.Now()).Rfc3339Copy()
		createAddonCheck("ac-undeterminable", fathomv1alpha1.AddonCheckStatus{
			// The most recent run aggregated to Error, so lastResult and
			// lastRunTime both describe it, while the evidence it could not
			// replace is preserved with its original observation.
			LastResult:               "Error",
			LastRunTime:              &ran,
			LastSuccessfulEvaluation: passEvidence(observed),
			LatestAttemptAt:          &ran,
			LatestAttemptOutcome:     fathomv1alpha1.AddonCheckAttemptError,
			LatestAttemptReason:      reasonIncompleteEvaluation,
			EvidenceFreshness:        fathomv1alpha1.AddonCheckEvidenceCurrent,
			Conditions: []metav1.Condition{{
				Type:               addonCheckConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             reasonRunCompleted,
				Message:            "runtime evaluation completed with eligible inputs",
				LastTransitionTime: ran,
			}},
		})

		got := mirror("hc-undeterminable", "ac-undeterminable")

		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultError))
		Expect(got.Status.SourceObservedAt).NotTo(BeNil())
		Expect(got.Status.SourceObservedAt.Time).To(BeTemporally("==", observed.Time))
		Expect(got.Status.SourceObservedAt.Time).NotTo(BeTemporally("==", ran.Time))
		// And the retained evidence is an hour old, so it is not current
		// coverage either, whatever the source last stored.
		Expect(got.Status.EvidenceFreshness).To(Equal(fathomv1alpha1.AddonCheckEvidenceStale))
	})

	// contracts/runtime.md: "Evidence ages out | Freshness=Stale even if stored
	// verdict was Pass". Freshness is re-derived from the evidence's age at
	// MIRROR time, not copied: a stored freshness only advances when the source
	// reconciles, and a check that has gone quiet is exactly the case where
	// nothing reconciles it.
	It("re-derives Stale from the evidence's age even when the source still says Current", func() {
		// Two effective intervals plus one timeout is the window; the default
		// cadence puts it at 10m30s, so an hour-old observation is far past it.
		observed := metav1.NewTime(time.Now().Add(-time.Hour))
		createAddonCheck("ac-aged-pass", fathomv1alpha1.AddonCheckStatus{
			LastResult:               "Pass",
			LastRunTime:              &observed,
			LastSuccessfulEvaluation: passEvidence(observed),
			LatestAttemptAt:          &observed,
			LatestAttemptOutcome:     fathomv1alpha1.AddonCheckAttemptCompleted,
			// Current is what the source correctly wrote at publication time.
			EvidenceFreshness: fathomv1alpha1.AddonCheckEvidenceCurrent,
			Conditions: []metav1.Condition{{
				Type:               addonCheckConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             reasonRunCompleted,
				Message:            "runtime evaluation completed with eligible inputs",
				LastTransitionTime: observed,
			}},
		})

		got := mirror("hc-aged-pass", "ac-aged-pass")

		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		Expect(got.Status.EvidenceFreshness).To(Equal(fathomv1alpha1.AddonCheckEvidenceStale))
		Expect(got.Status.EvidenceFreshnessReason).NotTo(BeEmpty())
		// Ready still mirrors what it says — the last run DID complete. Ready
		// is eligibility, not recency, and conflating them is how an aged Pass
		// would look healthy again.
		Expect(got.Status.SourceReady).NotTo(BeNil())
		Expect(*got.Status.SourceReady).To(BeTrue())
	})

	It("reports Current only while eligible evidence is inside its window", func() {
		observed := metav1.NewTime(time.Now().Add(-time.Minute))
		createAddonCheck("ac-current-pass", fathomv1alpha1.AddonCheckStatus{
			LastResult:               "Pass",
			LastRunTime:              &observed,
			LastSuccessfulEvaluation: passEvidence(observed),
			LatestAttemptAt:          &observed,
			LatestAttemptOutcome:     fathomv1alpha1.AddonCheckAttemptCompleted,
			EvidenceFreshness:        fathomv1alpha1.AddonCheckEvidenceCurrent,
			Conditions: []metav1.Condition{{
				Type:               addonCheckConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             reasonRunCompleted,
				Message:            "runtime evaluation completed with eligible inputs",
				LastTransitionTime: observed,
			}},
		})

		got := mirror("hc-current-pass", "ac-current-pass")
		Expect(got.Status.EvidenceFreshness).To(Equal(fathomv1alpha1.AddonCheckEvidenceCurrent))
		Expect(got.Status.EvidenceFreshnessReason).To(BeEmpty())
	})

	// A built-in AddonCheck has no evidence model at all, and this feature is
	// default-off: its mirrored status must be exactly what it was before this
	// task. Freshness stays unset — absent, not "Unavailable" — because an
	// unknown freshness is a different statement from an unusable one.
	It("leaves a built-in check's mirror unchanged", func() {
		ran := metav1.NewTime(time.Now().Add(-time.Minute)).Rfc3339Copy()
		createAddonCheck("ac-builtin-mirror", fathomv1alpha1.AddonCheckStatus{
			LastResult:     "Warn",
			LastRunTime:    &ran,
			LastReportName: "ac-builtin-mirror-1",
			Conditions: []metav1.Condition{{
				Type:               addonCheckConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             "RunCompleted",
				Message:            "AddonCheck adapter run completed.",
				LastTransitionTime: ran,
			}},
		})

		got := mirror("hc-builtin-mirror", "ac-builtin-mirror")
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultWarn))
		Expect(got.Status.LastReportName).To(Equal("ac-builtin-mirror-1"))
		Expect(got.Status.SourceObservedAt.Time).To(BeTemporally("==", ran.Time))
		Expect(got.Status.EvidenceFreshness).To(BeEmpty())
		Expect(got.Status.EvidenceFreshnessReason).To(BeEmpty())
		Expect(got.Status.SourceReady).NotTo(BeNil())
		Expect(*got.Status.SourceReady).To(BeTrue())
	})

	// A terminal mirror failure clears the mirrored snapshot, and the new
	// fields are part of that snapshot: a freshness left behind from a target
	// that no longer exists would describe evidence nothing can produce.
	It("clears mirrored readiness and freshness when the target is gone", func() {
		observed := metav1.NewTime(time.Now().Add(-time.Minute))
		target := createAddonCheck("ac-deleted-target", fathomv1alpha1.AddonCheckStatus{
			LastResult:               "Pass",
			LastRunTime:              &observed,
			LastSuccessfulEvaluation: passEvidence(observed),
			EvidenceFreshness:        fathomv1alpha1.AddonCheckEvidenceCurrent,
			Conditions: []metav1.Condition{{
				Type:               addonCheckConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             reasonRunCompleted,
				Message:            "runtime evaluation completed with eligible inputs",
				LastTransitionTime: observed,
			}},
		})
		got := mirror("hc-deleted-target", "ac-deleted-target")
		Expect(got.Status.EvidenceFreshness).To(Equal(fathomv1alpha1.AddonCheckEvidenceCurrent))

		Expect(k8sClient.Delete(ctx, target)).To(Succeed())
		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "hc-deleted-target", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "hc-deleted-target", Namespace: "default"}, &got)).To(Succeed())

		Expect(got.Status.Result).To(BeEmpty())
		Expect(got.Status.EvidenceFreshness).To(BeEmpty())
		Expect(got.Status.EvidenceFreshnessReason).To(BeEmpty())
		Expect(got.Status.SourceReady).To(BeNil())
		Expect(got.Status.SourceReadyReason).To(BeEmpty())
	})

	// AGENTS.md: "Keep the ClusterHealth external contract stable. It is
	// derived only from HealthCheck.status — never from HealthReport history."
	// The aggregate here is offered a HealthReport that flatly contradicts its
	// child, under a client that fails any attempt to read one.
	It("aggregates ClusterHealth from HealthCheck.status alone", func() {
		observed := metav1.NewTime(time.Now().Add(-90 * time.Second)).Rfc3339Copy()
		createAddonCheck("ac-contract", fathomv1alpha1.AddonCheckStatus{
			LastResult:               "Fail",
			LastRunTime:              &observed,
			LastSuccessfulEvaluation: passEvidence(observed),
			EvidenceFreshness:        fathomv1alpha1.AddonCheckEvidenceUnavailable,
			Conditions: []metav1.Condition{{
				Type:               addonCheckConditionReady,
				Status:             metav1.ConditionFalse,
				Reason:             reasonAccessDenied,
				Message:            "the dedicated reader was denied a required read",
				LastTransitionTime: observed,
			}},
		})
		// History that says the opposite of the child's status, labelled so any
		// history-consulting implementation would find it.
		contradiction := &fathomv1alpha1.HealthReport{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hr-contract-pass", Namespace: "default",
				Labels: map[string]string{
					fathomv1alpha1.LabelHealthReportSourceKind: "AddonCheck",
					fathomv1alpha1.LabelHealthReportSourceName: "ac-contract",
				},
			},
			Spec: fathomv1alpha1.HealthReportSpec{
				SourceRef: fathomv1alpha1.HealthReportTargetRef{
					APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: "AddonCheck",
					Namespace: "default", Name: "ac-contract",
				},
				Result:     fathomv1alpha1.HealthReportResultPass,
				ObservedAt: metav1.NewTime(time.Now()),
			},
		}
		Expect(k8sClient.Create(ctx, contradiction)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, contradiction))).To(Succeed())
		})

		hc := &fathomv1alpha1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hc-contract", Namespace: "default",
				Labels: map[string]string{"suite": "clusterhealth-contract"},
			},
			Spec: fathomv1alpha1.HealthCheckSpec{
				CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "ac-contract"},
			},
		}
		Expect(k8sClient.Create(ctx, hc)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, hc))).To(Succeed())
		})
		_, err := newReconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "hc-contract", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		ch := &fathomv1alpha1.ClusterHealth{
			ObjectMeta: metav1.ObjectMeta{Name: "ch-contract"},
			Spec: fathomv1alpha1.ClusterHealthSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"suite": "clusterhealth-contract"}},
			},
		}
		Expect(k8sClient.Create(ctx, ch)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ch))).To(Succeed())
		})

		reportReads := 0
		blind := healthReportBlindClient{Client: k8sClient, reads: &reportReads}
		_, err = (&ClusterHealthReconciler{Client: blind, Scheme: k8sClient.Scheme()}).
			Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "ch-contract"}})
		Expect(err).NotTo(HaveOccurred())

		var aggregate fathomv1alpha1.ClusterHealth
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ch-contract"}, &aggregate)).To(Succeed())
		Expect(reportReads).To(Equal(0))
		// The child's mirrored status, not the Pass sitting in history.
		Expect(aggregate.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultFail))
		Expect(aggregate.Status.Children).To(HaveLen(1))
		Expect(aggregate.Status.Children[0].Result).To(Equal(fathomv1alpha1.HealthReportResultFail))
		// The aggregate's observation is the child's mirrored evidence time,
		// which is the retained evidence's own — an aged child cannot make the
		// roll-up look current.
		Expect(aggregate.Status.ObservedAt).NotTo(BeNil())
		Expect(aggregate.Status.ObservedAt.Time).To(BeTemporally("==", observed.Time))
	})

	// The mirror is itself a status write with its own API bounds. A source
	// reason may be up to the 1024 characters metav1.Condition permits;
	// SourceReadyReason admits 128. Mirroring one verbatim does not produce an
	// over-long string in the status — it produces NO status at all: the API
	// server rejects the whole update, so result, readiness, freshness and
	// lastReportName stop being mirrored together, and the wrapper freezes on
	// whatever it last showed. The bound is enforced at the mirror boundary for
	// exactly the reason Summary's is.
	It("truncates an over-long source reason instead of losing the whole status write", func() {
		observed := metav1.NewTime(time.Now().Add(-time.Minute)).Rfc3339Copy()
		// A legal condition reason (letters only, inside the source's own
		// 1024-character bound) that is comfortably past the mirror's 128.
		longReason := strings.Repeat("A", 200)
		createAddonCheck("ac-long-reason", fathomv1alpha1.AddonCheckStatus{
			LastResult:  "Pass",
			LastRunTime: &observed,
			Conditions: []metav1.Condition{{
				Type:               addonCheckConditionReady,
				Status:             metav1.ConditionFalse,
				Reason:             longReason,
				Message:            "the dedicated reader was denied a required read",
				LastTransitionTime: observed,
			}},
		})

		got := mirror("hc-long-reason", "ac-long-reason")

		Expect(utf8.RuneCountInString(got.Status.SourceReadyReason)).
			To(Equal(healthCheckReadyReasonMaxLen))
		// Truncated, not replaced: the surviving prefix still identifies the
		// source's reason.
		Expect(got.Status.SourceReadyReason).To(HavePrefix("AAAA"))
		// And the rest of the snapshot landed, which is the whole point.
		Expect(got.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		Expect(got.Status.SourceReady).NotTo(BeNil())
		Expect(*got.Status.SourceReady).To(BeFalse())
		Expect(got.Status.SourceObservedAt).NotTo(BeNil())
		Expect(got.Status.SourceObservedAt.Time).To(BeTemporally("==", observed.Time))
	})
})
