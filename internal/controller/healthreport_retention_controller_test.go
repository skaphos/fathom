/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1 "github.com/skaphos/fathom/api/v1"
	"github.com/skaphos/fathom/internal/metrics"
)

// HealthReport retention (spec 014, contracts/retention.md): the window, the
// stamp, the per-kind cap, the recreated-check boundary, the budget and the
// observability signals, against a real API server.
var _ = Describe("HealthReport retention", func() {
	var (
		ns       string
		now      time.Time
		recorder *events.FakeRecorder
		r        *HealthReportRetentionReconciler
	)

	newReport := func(name, kind, checkName, uid string, observed time.Time) *fathomv1.HealthReport {
		return &fathomv1.HealthReport{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: ns,
				Labels: map[string]string{
					fathomv1.LabelHealthReportSourceKind: kind,
					fathomv1.LabelHealthReportSourceName: checkName,
					fathomv1.LabelHealthReportSourceUID:  uid,
				},
			},
			Spec: fathomv1.HealthReportSpec{
				SourceRef:  fathomv1.HealthReportTargetRef{Kind: kind, Name: checkName, Namespace: ns},
				Result:     fathomv1.HealthReportResultPass,
				ObservedAt: metav1.NewTime(observed),
			},
		}
	}
	create := func(objs ...client.Object) {
		for _, o := range objs {
			Expect(k8sClient.Create(ctx, o)).To(Succeed())
		}
	}
	reconcile := func() ctrl.Result {
		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: ns}})
		Expect(err).NotTo(HaveOccurred())
		return res
	}
	names := func() []string {
		var list fathomv1.HealthReportList
		Expect(k8sClient.List(ctx, &list, client.InNamespace(ns))).To(Succeed())
		var out []string
		for _, item := range list.Items {
			out = append(out, item.Name)
		}
		return out
	}
	deletedAt := func(name string) string {
		var report fathomv1.HealthReport
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &report)).To(Succeed())
		return report.Annotations[fathomv1.AnnotationHealthReportSourceDeletedAt]
	}

	BeforeEach(func() {
		ns = fmt.Sprintf("retention-%d", GinkgoRandomSeed()+int64(GinkgoParallelProcess())*1000+time.Now().UnixNano()%100000)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		now = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
		recorder = events.NewFakeRecorder(16)
		metrics.HealthReportsRetained.Reset()
		metrics.HealthReportsPrunedTotal.Reset()
		r = &HealthReportRetentionReconciler{Client: k8sClient, Recorder: recorder, Window: 24 * time.Hour, CapPerKind: 500, Clock: func() time.Time { return now }}
	})

	It("stamps a deleted check's reports once and prunes them after the window", func() {
		check := &fathomv1.AddonCheck{ObjectMeta: metav1.ObjectMeta{Name: "gone", Namespace: ns}, Spec: fathomv1.AddonCheckSpec{AddonType: "cert-manager"}}
		create(check)
		create(newReport("gone-1", "AddonCheck", "gone", string(check.UID), now.Add(-2*time.Hour)))
		Expect(k8sClient.Delete(ctx, check)).To(Succeed())

		reconcile()
		Expect(deletedAt("gone-1")).To(Equal(now.Format(time.RFC3339)), "the first sweep stamps the deletion time")
		Expect(testutil.ToFloat64(metrics.HealthReportsRetained.WithLabelValues(ns, "AddonCheck"))).To(Equal(1.0))

		now = now.Add(time.Hour)
		reconcile()
		Expect(deletedAt("gone-1")).To(Equal(now.Add(-time.Hour).Format(time.RFC3339)), "the stamp is never rewritten")
		Expect(names()).To(ContainElement("gone-1"), "inside the window the report stays")

		now = now.Add(24 * time.Hour)
		reconcile()
		Expect(names()).NotTo(ContainElement("gone-1"))
		Expect(testutil.ToFloat64(metrics.HealthReportsPrunedTotal.WithLabelValues(ns, "AddonCheck", "window"))).To(Equal(1.0))
		Expect(testutil.ToFloat64(metrics.HealthReportsRetained.WithLabelValues(ns, "AddonCheck"))).To(Equal(0.0))
		Eventually(recorder.Events).Should(Receive(ContainSubstring("ReportsPruned")), "one Event per pass that deleted something")
		Expect(recorder.Events).NotTo(Receive(), "and no second Event for the same pass")
	})

	It("prunes orphans on the first pass when the window is zero", func() {
		r.Window = 0
		create(newReport("orphan-1", "DNSCheck", "never-existed", "uid-x", now))
		reconcile()
		Expect(names()).NotTo(ContainElement("orphan-1"))
	})

	It("leaves a recreated check's predecessor history alone and distinct", func() {
		old := newReport("old-1", "AddonCheck", "same-name", "old-uid", now.Add(-time.Hour))
		create(old)
		recreated := &fathomv1.AddonCheck{ObjectMeta: metav1.ObjectMeta{Name: "same-name", Namespace: ns}, Spec: fathomv1.AddonCheckSpec{AddonType: "cert-manager"}}
		create(recreated)
		create(newReport("new-1", "AddonCheck", "same-name", string(recreated.UID), now))

		reconcile()
		Expect(deletedAt("old-1")).NotTo(BeEmpty(), "the old UID's report is an orphan although a same-name check exists")
		Expect(deletedAt("new-1")).To(BeEmpty(), "the live check's report is not")
		Expect(names()).To(ConsistOf("old-1", "new-1"))
	})

	It("keeps a namespace under the per-kind cap, oldest first, live and orphaned alike", func() {
		r.CapPerKind = 2
		check := &fathomv1.DNSCheck{ObjectMeta: metav1.ObjectMeta{Name: "dns", Namespace: ns}, Spec: fathomv1.DNSCheckSpec{Targets: []fathomv1.DNSTarget{{Name: "kubernetes.default.svc.cluster.local"}}}}
		create(check)
		create(
			newReport("dns-oldest", "DNSCheck", "dns", string(check.UID), now.Add(-3*time.Hour)),
			newReport("dns-mid", "DNSCheck", "dns", string(check.UID), now.Add(-2*time.Hour)),
			newReport("orphan-newest", "DNSCheck", "other", "other-uid", now.Add(-time.Hour)),
			newReport("ncc-1", "NodeCertificateCheck", "ncc", "ncc-uid", now.Add(-5*time.Hour)),
		)
		reconcile()
		Expect(names()).To(ConsistOf("dns-mid", "orphan-newest", "ncc-1"), "only the DNSCheck kind was over its cap; its oldest went")
		Expect(testutil.ToFloat64(metrics.HealthReportsPrunedTotal.WithLabelValues(ns, "DNSCheck", "cap"))).To(Equal(1.0))
		Eventually(recorder.Events).Should(Receive(ContainSubstring("cap: 1")))
	})

	It("never touches a report it did not write", func() {
		foreign := &fathomv1.HealthReport{
			ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: ns},
			Spec:       fathomv1.HealthReportSpec{SourceRef: fathomv1.HealthReportTargetRef{Kind: "AddonCheck", Name: "x", Namespace: ns}, Result: fathomv1.HealthReportResultPass, ObservedAt: metav1.NewTime(now)},
		}
		create(foreign)
		r.Window = 0
		r.CapPerKind = 1
		reconcile()
		Expect(names()).To(ConsistOf("foreign"))
	})

	It("deletes at most the budget per pass and continues shortly after", func() {
		r.Window = 0
		var objs []client.Object
		for i := 0; i < retentionWriteBudget+5; i++ {
			objs = append(objs, newReport(fmt.Sprintf("o-%03d", i), "AddonCheck", "gone", "gone-uid", now.Add(-time.Duration(i)*time.Minute)))
		}
		create(objs...)
		res := reconcile()
		Expect(res.RequeueAfter).To(Equal(retentionContinueAfter), "the budget was spent, so another pass follows shortly")
		Expect(names()).To(HaveLen(5))
		res = reconcile()
		Expect(res.RequeueAfter).To(Equal(retentionResync), "nothing left: back to the hourly resync")
		Expect(names()).To(BeEmpty())
	})

	It("sweeps again when the earliest retained report's window elapses", func() {
		r.Window = 10 * time.Minute
		create(newReport("soon", "AddonCheck", "gone", "gone-uid", now.Add(-time.Hour)))
		res := reconcile()
		Expect(res.RequeueAfter).To(Equal(10*time.Minute), "a fresh stamp expires one window from now")

		now = now.Add(4 * time.Minute)
		res = reconcile()
		Expect(res.RequeueAfter).To(Equal(6*time.Minute), "the pause shrinks to the time left on the earliest stamp")
		Expect(names()).To(ContainElement("soon"))

		now = now.Add(6 * time.Minute)
		res = reconcile()
		Expect(names()).NotTo(ContainElement("soon"))
		Expect(res.RequeueAfter).To(Equal(retentionResync), "nothing retained: back to the hourly resync")
	})

	It("stamps at most the budget per pass and continues shortly after", func() {
		var objs []client.Object
		for i := 0; i < retentionWriteBudget+5; i++ {
			objs = append(objs, newReport(fmt.Sprintf("s-%03d", i), "AddonCheck", "gone", "gone-uid", now.Add(-time.Duration(i)*time.Minute)))
		}
		create(objs...)
		stamped := func() int {
			var list fathomv1.HealthReportList
			Expect(k8sClient.List(ctx, &list, client.InNamespace(ns))).To(Succeed())
			n := 0
			for _, item := range list.Items {
				if _, ok := item.Annotations[fathomv1.AnnotationHealthReportSourceDeletedAt]; ok {
					n++
				}
			}
			return n
		}
		res := reconcile()
		Expect(res.RequeueAfter).To(Equal(retentionContinueAfter), "the budget was spent on stamps")
		Expect(stamped()).To(Equal(retentionWriteBudget))
		res = reconcile()
		Expect(stamped()).To(Equal(retentionWriteBudget + 5))
		Expect(res.RequeueAfter).To(Equal(retentionResync), "a 24h window: the hourly resync comes first")
	})

	It("keeps sweeping past a report it cannot stamp or delete", func() {
		r.Client = refusingClient{Client: k8sClient, name: "stuck"}
		r.CapPerKind = 2
		create(
			newReport("stuck", "AddonCheck", "gone", "gone-uid", now.Add(-3*time.Hour)),
			newReport("gone-2", "AddonCheck", "gone", "gone-uid", now.Add(-2*time.Hour)),
			newReport("dns-old", "DNSCheck", "dns", "dns-uid", now.Add(-3*time.Hour)),
			newReport("dns-mid", "DNSCheck", "dns", "dns-uid", now.Add(-2*time.Hour)),
			newReport("dns-new", "DNSCheck", "dns", "dns-uid", now.Add(-time.Hour)),
		)
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: ns}})
		Expect(err).To(MatchError(ContainSubstring("stuck")), "the refused write is still reported")
		Expect(deletedAt("stuck")).To(BeEmpty())
		Expect(deletedAt("gone-2")).NotTo(BeEmpty(), "the other orphans were stamped regardless")
		Expect(names()).To(ConsistOf("stuck", "gone-2", "dns-mid", "dns-new"), "and the cap still took the oldest DNSCheck report")

		r.Window = 0
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: ns}})
		Expect(err).To(HaveOccurred())
		Expect(names()).To(ConsistOf("stuck"), "a refused delete does not stop the others")
	})

	It("spends the budget on refused writes too", func() {
		calls := 0
		r.Client = refusingClient{Client: k8sClient, all: true, calls: &calls}
		r.Window = 0
		var objs []client.Object
		for i := 0; i < retentionWriteBudget+10; i++ {
			objs = append(objs, newReport(fmt.Sprintf("r-%03d", i), "AddonCheck", "gone", "gone-uid", now))
		}
		create(objs...)
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: ns}})
		Expect(err).To(HaveOccurred())
		Expect(calls).To(Equal(retentionWriteBudget), "a locked-down namespace cannot drive unbounded API churn per pass")
	})

	It("drops the namespace's gauge series once it holds no reports", func() {
		r.Window = 0
		create(newReport("only", "AddonCheck", "gone", "gone-uid", now))
		reconcile()
		Expect(testutil.CollectAndCount(metrics.HealthReportsRetained)).To(Equal(4), "one series per report-writing kind while the namespace holds reports")
		reconcile()
		Expect(testutil.CollectAndCount(metrics.HealthReportsRetained)).To(BeZero(), "an empty namespace keeps no series")
	})

	It("never caps away the report a live check's status still names", func() {
		quiet := &fathomv1.AddonCheck{ObjectMeta: metav1.ObjectMeta{Name: "quiet", Namespace: ns}, Spec: fathomv1.AddonCheckSpec{AddonType: "cert-manager"}}
		busy := &fathomv1.AddonCheck{ObjectMeta: metav1.ObjectMeta{Name: "busy", Namespace: ns}, Spec: fathomv1.AddonCheckSpec{AddonType: "cert-manager"}}
		create(quiet, busy)
		quiet.Status.LastReportName = "quiet-only"
		Expect(k8sClient.Status().Update(ctx, quiet)).To(Succeed())
		create(
			newReport("quiet-only", "AddonCheck", "quiet", string(quiet.UID), now.Add(-3*time.Hour)),
			newReport("busy-old", "AddonCheck", "busy", string(busy.UID), now.Add(-2*time.Hour)),
			newReport("busy-new", "AddonCheck", "busy", string(busy.UID), now.Add(-time.Hour)),
		)
		r.CapPerKind = 2
		reconcile()
		Expect(names()).To(ConsistOf("quiet-only", "busy-new"), "the oldest is the quiet check's only report, so the next oldest went instead")
	})

	It("tolerates a report that vanished between the list and the delete", func() {
		r.Window = 0
		rep := newReport("racy", "AddonCheck", "gone", "gone-uid", now)
		create(rep)
		Expect(k8sClient.Delete(ctx, rep)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(rep), &fathomv1.HealthReport{}))
		}).Should(BeTrue())
		reconcile()
	})
})

// refusingClient fails every Patch and Delete of one report, standing in for
// an admission policy or RBAC gap that affects a single object.
type refusingClient struct {
	client.Client
	name  string
	all   bool // refuse every report, not just name
	calls *int // writes attempted, refused or not
}

func (c refusingClient) refuses(obj client.Object) bool {
	if c.calls != nil {
		*c.calls++
	}
	return c.all || obj.GetName() == c.name
}

func (c refusingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if c.refuses(obj) {
		return apierrors.NewForbidden(schema.GroupResource{Group: "fathom.skaphos.io", Resource: "healthreports"}, c.name, errors.New("denied by policy"))
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c refusingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.refuses(obj) {
		return apierrors.NewForbidden(schema.GroupResource{Group: "fathom.skaphos.io", Resource: "healthreports"}, c.name, errors.New("denied by policy"))
	}
	return c.Client.Delete(ctx, obj, opts...)
}
