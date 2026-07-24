/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/metrics"
)

// checkGaugeValue returns (value, found) for one fathom_check_result series.
func checkGaugeValue(t interface{ Helper() }, kind, name, namespace, result string) (float64, bool) {
	t.Helper()
	return gatherGaugeValue("fathom_check_result", map[string]string{
		"kind": kind, "name": name, "namespace": namespace, "result": result,
	})
}

func lastRunGaugeValue(t interface{ Helper() }, kind, name, namespace string) (float64, bool) {
	t.Helper()
	return gatherGaugeValue("fathom_check_last_run_timestamp_seconds", map[string]string{
		"kind": kind, "name": name, "namespace": namespace,
	})
}

func gatherGaugeValue(family string, want map[string]string) (float64, bool) {
	mfs, err := ctrlmetrics.Registry.Gather()
	if err != nil {
		return 0, false
	}
	for _, mf := range mfs {
		if mf.GetName() != family {
			continue
		}
	metric:
		for _, m := range mf.GetMetric() {
			got := map[string]string{}
			for _, lp := range m.GetLabel() {
				got[lp.GetName()] = lp.GetValue()
			}
			for k, v := range want {
				if got[k] != v {
					continue metric
				}
			}
			return m.GetGauge().GetValue(), true
		}
	}
	return 0, false
}

// drainEvents empties a FakeRecorder channel into a slice.
func drainEvents(rec *events.FakeRecorder) []string {
	var out []string
	for {
		select {
		case e := <-rec.Events:
			out = append(out, e)
		default:
			return out
		}
	}
}

func readyCondition(status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               checkConditionReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
}

func testCheckObject(name string) client.Object {
	return &fathomv1alpha1.HealthCheck{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}
}

func TestObserveCheckFirstResultTransitionsFromUnknown(t *testing.T) {
	metrics.CheckResult.Reset()
	rec := events.NewFakeRecorder(10)
	now := metav1.Now()

	observeCheck(rec, testCheckObject("first"), "HealthCheck",
		"", fathomv1alpha1.HealthReportResultPass, nil, nil, &now, nil)

	events := drainEvents(rec)
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %v", events)
	}
	if events[0] != "Normal ResultChanged check result changed from Unknown to Pass" {
		t.Errorf("unexpected event: %q", events[0])
	}
	if v, ok := checkGaugeValue(t, "HealthCheck", "first", "default", "Pass"); !ok || v != 1 {
		t.Errorf("gauge Pass = %v (found=%v), want 1", v, ok)
	}
}

func TestObserveCheckDegradationIsWarning(t *testing.T) {
	rec := events.NewFakeRecorder(10)

	observeCheck(rec, testCheckObject("degrade"), "HealthCheck",
		fathomv1alpha1.HealthReportResultPass, fathomv1alpha1.HealthReportResultFail, nil, nil, nil, nil)

	events := drainEvents(rec)
	if len(events) != 1 || events[0] != "Warning ResultChanged check result changed from Pass to Fail" {
		t.Errorf("unexpected events: %v", events)
	}
}

func TestObserveCheckNoChangeIsSilent(t *testing.T) {
	rec := events.NewFakeRecorder(10)

	// A restart re-observation: previous result comes from status, so an
	// unchanged result — even with a fresh reconciler process — emits nothing.
	observeCheck(rec, testCheckObject("steady"), "HealthCheck",
		fathomv1alpha1.HealthReportResultPass, fathomv1alpha1.HealthReportResultPass, nil, nil, nil, nil)

	if events := drainEvents(rec); len(events) != 0 {
		t.Errorf("no-change observation emitted events: %v", events)
	}
}

func TestObserveCheckReadyFailureEmitsOncePerEpisode(t *testing.T) {
	rec := events.NewFakeRecorder(10)
	failing := []metav1.Condition{readyCondition(metav1.ConditionFalse, "AdapterRunFailed", "boom")}

	// Newly failing: one Warning with the condition's reason.
	observeCheck(rec, testCheckObject("fail-once"), "AddonCheck",
		fathomv1alpha1.HealthReportResultError, fathomv1alpha1.HealthReportResultError, nil, failing, nil, nil)
	events := drainEvents(rec)
	if len(events) != 1 || events[0] != "Warning AdapterRunFailed boom" {
		t.Fatalf("unexpected events: %v", events)
	}

	// Same failure next reconcile: silent (single event per failure episode).
	observeCheck(rec, testCheckObject("fail-once"), "AddonCheck",
		fathomv1alpha1.HealthReportResultError, fathomv1alpha1.HealthReportResultError, failing, failing, nil, nil)
	if events := drainEvents(rec); len(events) != 0 {
		t.Errorf("persistent failure re-emitted: %v", events)
	}

	// Reason change: a new episode, one new event.
	probeFailing := []metav1.Condition{readyCondition(metav1.ConditionFalse, "ProbeLaunchFailed", "no image")}
	observeCheck(rec, testCheckObject("fail-once"), "AddonCheck",
		fathomv1alpha1.HealthReportResultError, fathomv1alpha1.HealthReportResultError, failing, probeFailing, nil, nil)
	events = drainEvents(rec)
	if len(events) != 1 || events[0] != "Warning ProbeLaunchFailed no image" {
		t.Errorf("unexpected events: %v", events)
	}
}

func TestObserveCheckReconcileError(t *testing.T) {
	rec := events.NewFakeRecorder(10)

	// A reconcile error with no Ready-failure transition gets the generic event.
	observeCheck(rec, testCheckObject("rec-err"), "NodeCertificateCheck",
		"", "", nil, nil, nil, errors.New("update conflict"))
	events := drainEvents(rec)
	if len(events) != 1 || events[0] != "Warning ReconcileError reconcile failed: update conflict" {
		t.Fatalf("unexpected events: %v", events)
	}

	// When the same failure already surfaced through a Ready condition this
	// reconcile, the generic event is suppressed — one event per cause.
	failing := []metav1.Condition{readyCondition(metav1.ConditionFalse, "RBACProvisioningFailed", "denied")}
	observeCheck(rec, testCheckObject("rec-err"), "NodeCertificateCheck",
		"", "", nil, failing, nil, errors.New("denied"))
	events = drainEvents(rec)
	if len(events) != 1 || events[0] != "Warning RBACProvisioningFailed denied" {
		t.Errorf("unexpected events: %v", events)
	}
}

func TestObserveCheckNilRecorderStillMirrorsGauges(t *testing.T) {
	metrics.CheckResult.Reset()
	metrics.CheckLastRunTimestamp.Reset()
	now := metav1.NewTime(time.Now())

	observeCheck(nil, testCheckObject("no-rec"), "HealthCheck",
		"", fathomv1alpha1.HealthReportResultWarn, nil, nil, &now, nil)

	if v, ok := checkGaugeValue(t, "HealthCheck", "no-rec", "default", "Warn"); !ok || v != 1 {
		t.Errorf("gauge Warn = %v (found=%v), want 1", v, ok)
	}
	if v, ok := lastRunGaugeValue(t, "HealthCheck", "no-rec", "default"); !ok || v != float64(now.Unix()) {
		t.Errorf("last-run = %v (found=%v), want %v", v, ok, now.Unix())
	}
}

var _ = Describe("Check observability wiring", func() {
	ctx := context.Background()

	BeforeEach(func() {
		metrics.CheckResult.Reset()
		metrics.CheckLastRunTimestamp.Reset()
	})

	reconcileOnce := func(r reconcile.Reconciler, namespace, name string) {
		GinkgoHelper()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}})
		Expect(err).NotTo(HaveOccurred())
	}

	It("HealthCheck: mirrors gauges, stamps freshness, emits the first-result event, and clears series on deletion", func() {
		runTime := metav1.NewTime(time.Now().Add(-time.Minute).Truncate(time.Second))
		createAddonCheckWithStatusForObservability(ctx, "obs-target", fathomv1alpha1.AddonCheckStatus{
			LastResult:     "Pass",
			LastRunTime:    &runTime,
			LastReportName: "obs-target-report",
		})
		hc := &fathomv1alpha1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{Name: "obs-hc", Namespace: "default"},
			Spec: fathomv1alpha1.HealthCheckSpec{
				CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "obs-target"},
			},
		}
		Expect(k8sClient.Create(ctx, hc)).To(Succeed())

		rec := events.NewFakeRecorder(10)
		r := &HealthCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Recorder: rec}
		reconcileOnce(r, "default", "obs-hc")

		v, ok := checkGaugeValue(GinkgoT(), "HealthCheck", "obs-hc", "default", "Pass")
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal(1.0))
		ts, ok := lastRunGaugeValue(GinkgoT(), "HealthCheck", "obs-hc", "default")
		Expect(ok).To(BeTrue())
		Expect(ts).To(Equal(float64(runTime.Unix())))
		Expect(drainEvents(rec)).To(ContainElement("Normal ResultChanged check result changed from Unknown to Pass"))

		Expect(k8sClient.Delete(ctx, hc)).To(Succeed())
		reconcileOnce(r, "default", "obs-hc")
		_, ok = checkGaugeValue(GinkgoT(), "HealthCheck", "obs-hc", "default", "Pass")
		Expect(ok).To(BeFalse(), "deleted check must not keep series")
		_, ok = lastRunGaugeValue(GinkgoT(), "HealthCheck", "obs-hc", "default")
		Expect(ok).To(BeFalse())
	})

	It("AddonCheck: discovery emits the Unknown/0 sentinels and a Ready failure surfaces as a Warning event", func() {
		check := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Name: "obs-ac", Namespace: "default"},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager"},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed())
		})

		rec := events.NewFakeRecorder(10)
		// No adapter registry: Ready lands False/MissingAdapter, nothing runs.
		r := &AddonCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Recorder: rec}
		reconcileOnce(r, "default", "obs-ac")

		v, ok := checkGaugeValue(GinkgoT(), "AddonCheck", "obs-ac", "default", "Unknown")
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal(1.0), "never-evaluated check reads Unknown")
		ts, ok := lastRunGaugeValue(GinkgoT(), "AddonCheck", "obs-ac", "default")
		Expect(ok).To(BeTrue())
		Expect(ts).To(Equal(0.0), "never-ran sentinel")
		Expect(drainEvents(rec)).To(ContainElement(HavePrefix("Warning MissingAdapter")))

		// The same failing reconcile again must not repeat the event.
		reconcileOnce(r, "default", "obs-ac")
		Expect(drainEvents(rec)).To(BeEmpty())
	})

	It("ClusterHealth: cluster-scoped series carry an empty namespace label and vanish on deletion", func() {
		ch := &fathomv1alpha1.ClusterHealth{ObjectMeta: metav1.ObjectMeta{Name: "obs-ch"}}
		Expect(k8sClient.Create(ctx, ch)).To(Succeed())

		r := &ClusterHealthReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		reconcileOnce(r, "", "obs-ch")

		// No HealthChecks match: the aggregate evaluates to Unknown.
		v, ok := checkGaugeValue(GinkgoT(), "ClusterHealth", "obs-ch", "", "Unknown")
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal(1.0))

		Expect(k8sClient.Delete(ctx, ch)).To(Succeed())
		reconcileOnce(r, "", "obs-ch")
		_, ok = checkGaugeValue(GinkgoT(), "ClusterHealth", "obs-ch", "", "Unknown")
		Expect(ok).To(BeFalse())
	})

	It("NodeCertificateCheck: discovery emits the sentinels and deletion clears the series", func() {
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: "obs-ncc", Namespace: "default"},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())

		r := &NodeCertificateCheckReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), NodeAgentImage: "example.com/node-agent:test"}
		reconcileOnce(r, "default", "obs-ncc")

		v, ok := checkGaugeValue(GinkgoT(), "NodeCertificateCheck", "obs-ncc", "default", "Unknown")
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal(1.0))
		ts, ok := lastRunGaugeValue(GinkgoT(), "NodeCertificateCheck", "obs-ncc", "default")
		Expect(ok).To(BeTrue())
		Expect(ts).To(Equal(0.0))

		Expect(k8sClient.Delete(ctx, check)).To(Succeed())
		reconcileOnce(r, "default", "obs-ncc")
		_, ok = checkGaugeValue(GinkgoT(), "NodeCertificateCheck", "obs-ncc", "default", "Unknown")
		Expect(ok).To(BeFalse())
	})
})

// createAddonCheckWithStatusForObservability mirrors the helper in
// healthcheck_controller_test.go without capturing its closure scope.
func createAddonCheckWithStatusForObservability(ctx context.Context, name string, status fathomv1alpha1.AddonCheckStatus) {
	GinkgoHelper()
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
}
