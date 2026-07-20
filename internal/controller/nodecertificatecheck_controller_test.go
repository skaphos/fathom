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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodecert"
)

func newNodeCertReconciler() *NodeCertificateCheckReconciler {
	return &NodeCertificateCheckReconciler{
		Client:            k8sClient,
		Scheme:            k8sClient.Scheme(),
		NodeAgentImage:    "ghcr.io/skaphos/fathom-node-agent:test",
		NodeAgentRoleName: defaultNodeAgentRoleName,
	}
}

func writeNodeReport(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck, node string, certs []nodecert.CertResult) {
	writeNodeReportForCheck(ctx, check, node, check.Name, time.Now(), certs)
}

func writeNodeReportAt(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck, node string, observedAt time.Time, certs []nodecert.CertResult) {
	writeNodeReportForCheck(ctx, check, node, check.Name, observedAt, certs)
}

func writeNodeReportForCheck(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck, node, reportCheckName string, observedAt time.Time, certs []nodecert.CertResult) {
	report := nodecert.NodeReport{
		Node:       node,
		CheckName:  reportCheckName,
		ObservedAt: observedAt,
		Aggregate:  nodecert.WorstOutcome(certs),
		Certs:      certs,
	}
	encoded, err := nodecert.EncodeReport(report)
	Expect(err).NotTo(HaveOccurred())
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodecert.NodeReportConfigMapName(check.Name, node),
			Namespace: check.Namespace,
			Labels: map[string]string{
				nodecert.LabelManagedBy:  nodecert.ManagedByValue,
				nodecert.LabelSourceKind: nodecert.KindNodeCertificateCheck,
				nodecert.LabelSourceName: check.Name,
				nodecert.LabelNode:       node,
			},
			// Mirror the node-agent: stamp the authenticity annotation the
			// controller cross-checks against report.Node (#155).
			Annotations: map[string]string{nodecert.AnnotationNodeName: node},
		},
		Data: map[string]string{nodecert.ConfigMapReportKey: encoded},
	}
	existing := &corev1.ConfigMap{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, existing)
	if err == nil {
		existing.Data = cm.Data
		existing.Labels = cm.Labels
		existing.Annotations = cm.Annotations
		Expect(k8sClient.Update(ctx, existing)).To(Succeed())
		return
	}
	Expect(client.IgnoreNotFound(err)).To(Succeed())
	Expect(k8sClient.Create(ctx, cm)).To(Succeed())
}

// setNodeAgentDaemonSetStatus marks the DaemonSet fully rolled out: every desired
// pod is updated and `ready` of them are ready, and the controller has observed
// the current generation. Use setNodeAgentDaemonSetStatusFull to simulate a
// partially-rolled-out DaemonSet.
func setNodeAgentDaemonSetStatus(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck, desired, ready int32) {
	setNodeAgentDaemonSetStatusFull(ctx, check, desired, desired, ready)
}

func setNodeAgentDaemonSetStatusFull(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck, desired, updated, ready int32) {
	ds := &appsv1.DaemonSet{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: agentResourceName(check), Namespace: check.Namespace}, ds)).To(Succeed())
	ds.Status.DesiredNumberScheduled = desired
	ds.Status.CurrentNumberScheduled = desired
	ds.Status.UpdatedNumberScheduled = updated
	ds.Status.NumberAvailable = ready
	ds.Status.NumberReady = ready
	// Mirror a healthy controller: the DaemonSet controller has observed the
	// current generation. nodeAgentRolledOut gates on this.
	ds.Status.ObservedGeneration = ds.Generation
	Expect(k8sClient.Status().Update(ctx, ds)).To(Succeed())
}

func nodeCertHealthReportCount(ctx context.Context, source types.NamespacedName) int {
	reports := &fathomv1alpha1.HealthReportList{}
	Expect(k8sClient.List(ctx, reports, client.InNamespace(source.Namespace), client.MatchingLabels{
		labelHealthReportSourceKind: "NodeCertificateCheck",
		labelHealthReportSourceName: source.Name,
	})).To(Succeed())
	return len(reports.Items)
}

var _ = Describe("NodeCertificateCheck Controller", func() {
	ctx := context.Background()

	It("provisions the node-agent DaemonSet and RBAC from spec", func() {
		name := types.NamespacedName{Name: "nc-provision", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec:       fathomv1alpha1.NodeCertificateCheckSpec{Paths: []string{"/etc/kubernetes/pki", "/etc/kubernetes/admin.conf"}},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		ds := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nc-provision-node-agent", Namespace: "default"}, ds)).To(Succeed())
		container := ds.Spec.Template.Spec.Containers[0]
		Expect(container.Image).To(Equal("ghcr.io/skaphos/fathom-node-agent:test"))
		Expect(container.Args).To(ContainElements("--check-name", "nc-provision", "--check-namespace", "default"))
		Expect(container.SecurityContext.ReadOnlyRootFilesystem).To(HaveValue(BeTrue()))
		Expect(container.SecurityContext.Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))
		Expect(ds.Spec.Template.Spec.ServiceAccountName).To(Equal("nc-provision-node-agent"))

		// Configured paths /etc/kubernetes/pki + /etc/kubernetes/admin.conf collapse
		// to a single read-only /etc/kubernetes hostPath mount.
		Expect(ds.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(ds.Spec.Template.Spec.Volumes[0].HostPath.Path).To(Equal("/etc/kubernetes"))
		Expect(container.VolumeMounts[0].ReadOnly).To(BeTrue())

		sa := &corev1.ServiceAccount{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nc-provision-node-agent", Namespace: "default"}, sa)).To(Succeed())
		rb := &rbacv1.RoleBinding{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nc-provision-node-agent", Namespace: "default"}, rb)).To(Succeed())
		Expect(rb.RoleRef.Name).To(Equal(defaultNodeAgentRoleName))
		Expect(rb.RoleRef.Kind).To(Equal("ClusterRole"))

		// The operator owns the referenced ClusterRole at runtime so its name
		// survives kustomize/OLM name prefixing.
		cr := &rbacv1.ClusterRole{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: defaultNodeAgentRoleName}, cr)).To(Succeed())
		Expect(cr.Rules).To(HaveLen(1))
		Expect(cr.Rules[0].Resources).To(ContainElement("configmaps"))

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(apiMeta.FindStatusCondition(updated.Status.Conditions, nodeCertConditionAccepted).Status).To(Equal(metav1.ConditionTrue))
	})

	It("is idempotent: a second reconcile does not churn the DaemonSet", func() {
		name := types.NamespacedName{Name: "nc-idempotent", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		ds1 := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nc-idempotent-node-agent", Namespace: "default"}, ds1)).To(Succeed())

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		ds2 := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nc-idempotent-node-agent", Namespace: "default"}, ds2)).To(Succeed())
		Expect(ds2.ResourceVersion).To(Equal(ds1.ResourceVersion), "DaemonSet must not be rewritten on a no-op reconcile")
		Expect(ds2.Annotations).To(HaveKey(nodeAgentSpecHashAnnotation), "spec-hash annotation must be stamped on the DaemonSet")
	})

	It("does not churn the DaemonSet template as agents rewrite report ConfigMaps (SKA-589)", func() {
		name := types.NamespacedName{Name: "nc-nochurn", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		created := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nc-nochurn-node-agent", Namespace: "default"}, created)).To(Succeed())
		// metadata.generation only advances when the spec changes; a DaemonSet
		// rewrite (rolling restart) is exactly what SKA-589 must prevent.
		firstGen := created.Generation
		setNodeAgentDaemonSetStatus(ctx, check, 1, 1)

		// Simulate the agent publishing successive fresh scans (each rewrites the
		// report ConfigMap and, via the watch, wakes the reconciler).
		for i := 0; i < 3; i++ {
			writeNodeReportAt(ctx, check, "node-a", time.Now().Add(time.Duration(i)*time.Second), []nodecert.CertResult{
				{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
			})
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())
		}

		after := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nc-nochurn-node-agent", Namespace: "default"}, after)).To(Succeed())
		Expect(after.Generation).To(Equal(firstGen), "DaemonSet spec must not be rewritten across reconciles driven by report ConfigMap churn")
	})

	It("does not roll up while the DaemonSet is still rolling out (SKA-589)", func() {
		name := types.NamespacedName{Name: "nc-midrollout", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		// Two nodes desired, both ready, but only one pod updated to the current
		// template: the rollout has not converged, so no rollup may be stamped.
		setNodeAgentDaemonSetStatusFull(ctx, check, 2, 1, 2)
		writeNodeReport(ctx, check, "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})
		writeNodeReport(ctx, check, "node-b", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/kubelet.crt", Subject: "CN=kubelet", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastResult).To(BeEmpty(), "must not roll up from stale-template pods mid-rollout")
		Expect(updated.Status.LastReportName).To(BeEmpty())
		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeCertConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("AgentRollingOut"))
	})

	It("tolerates a removed node's surviving report without blanking status (SKA-589)", func() {
		name := types.NamespacedName{Name: "nc-node-removed", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeAgentDaemonSetStatus(ctx, check, 2, 2)
		writeNodeReport(ctx, check, "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})
		writeNodeReport(ctx, check, "node-b", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/kubelet.crt", Subject: "CN=kubelet", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		rolledUp := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, rolledUp)).To(Succeed())
		Expect(rolledUp.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)))

		// node-b is removed: desired drops to 1 while its report ConfigMap survives
		// (still fresh). reportCount (2) now exceeds desired (1); status must not be
		// blanked, and Ready must stay True.
		setNodeAgentDaemonSetStatus(ctx, check, 1, 1)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.ReportingNodes).To(Equal(int32(2)))
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)), "surplus report must not blank the rollup")
		Expect(updated.Status.LastReportName).NotTo(BeEmpty())
		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeCertConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal("Reporting"))
	})

	It("rolls up per-node report ConfigMaps into a HealthReport and mirrors status", func() {
		name := types.NamespacedName{Name: "nc-rollup", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeAgentDaemonSetStatus(ctx, check, 2, 2)

		// Simulate two node-agents reporting: one healthy node, one with a cert
		// expiring inside the critical window.
		writeNodeReport(ctx, check, "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})
		writeNodeReport(ctx, check, "node-b", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomeFail, DaysRemaining: 2, NotAfter: time.Now().Add(2 * 24 * time.Hour)},
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.ReportingNodes).To(Equal(int32(2)))
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultFail)))
		Expect(updated.Status.LastReportName).NotTo(BeEmpty())

		reports := &fathomv1alpha1.HealthReportList{}
		Expect(k8sClient.List(ctx, reports, client.InNamespace("default"), client.MatchingLabels{
			labelHealthReportSourceKind: "NodeCertificateCheck",
			labelHealthReportSourceName: "nc-rollup",
		})).To(Succeed())
		Expect(reports.Items).To(HaveLen(1))
		Expect(reports.Items[0].Spec.Result).To(Equal(fathomv1alpha1.HealthReportResultFail))
		Expect(reports.Items[0].Spec.Checks).To(HaveLen(2))

		// The report ConfigMaps are adopted (owner-referenced) for GC.
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodecert.NodeReportConfigMapName("nc-rollup", "node-a"), Namespace: "default"}, cm)).To(Succeed())
		Expect(metav1.IsControlledBy(cm, updated)).To(BeTrue())
	})

	It("refreshes liveness without a new HealthReport when the aggregate is unchanged across an interval", func() {
		// Regression for #157: a healthy cluster re-scanning every interval used to
		// mint a fresh identical HealthReport each time — the deterministic name
		// folds in LastReportName, so the name changed every roll-up — pruning a
		// real Fail incident out of the bounded history after historyLimit
		// intervals. The roll-up is now transition-only: an unchanged aggregate
		// refreshes LastRunTime for liveness but writes no new report.
		name := types.NamespacedName{Name: "nc-unchanged", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeAgentDaemonSetStatus(ctx, check, 1, 1)
		writeNodeReport(ctx, check, "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		firstRollup := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, firstRollup)).To(Succeed())
		Expect(firstRollup.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)))
		firstReportName := firstRollup.Status.LastReportName
		Expect(firstReportName).NotTo(BeEmpty())
		Expect(nodeCertHealthReportCount(ctx, name)).To(Equal(1))

		// Simulate the interval elapsing since the last roll-up without changing the
		// reported result, then reconcile again (as the periodic requeue would).
		backdated := metav1.NewTime(time.Now().Add(-2 * nodeCertInterval(check)))
		firstRollup.Status.LastRunTime = &backdated
		Expect(k8sClient.Status().Update(ctx, firstRollup)).To(Succeed())

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		afterInterval := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, afterInterval)).To(Succeed())
		// No new report: an identical Pass must not churn history.
		Expect(nodeCertHealthReportCount(ctx, name)).To(Equal(1))
		Expect(afterInterval.Status.LastReportName).To(Equal(firstReportName))
		Expect(afterInterval.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)))
		// Liveness was refreshed: LastRunTime advanced past the back-dated value.
		Expect(afterInterval.Status.LastRunTime.Time).To(BeTemporally(">", backdated.Time))
	})

	It("writes a new HealthReport when the aggregate result transitions", func() {
		// The complement of the transition-only contract: a genuine change in the
		// aggregate result is persisted immediately (never throttled by the
		// interval), and prior history is retained so the incident is recorded.
		name := types.NamespacedName{Name: "nc-transition", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeAgentDaemonSetStatus(ctx, check, 1, 1)
		writeNodeReport(ctx, check, "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		passRollup := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, passRollup)).To(Succeed())
		Expect(passRollup.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)))
		passReportName := passRollup.Status.LastReportName
		Expect(nodeCertHealthReportCount(ctx, name)).To(Equal(1))

		// The cert now expires inside the critical window: aggregate goes Pass -> Fail.
		writeNodeReport(ctx, check, "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomeFail, DaysRemaining: 2, NotAfter: time.Now().Add(2 * 24 * time.Hour)},
		})
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		failRollup := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, failRollup)).To(Succeed())
		// The transition writes a second report; both are retained (history limit 10).
		Expect(nodeCertHealthReportCount(ctx, name)).To(Equal(2))
		Expect(failRollup.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultFail)))
		Expect(failRollup.Status.LastReportName).NotTo(Equal(passReportName))
	})

	It("does not roll up until every desired node has a fresh report", func() {
		name := types.NamespacedName{Name: "nc-partial", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeAgentDaemonSetStatus(ctx, check, 2, 2)

		writeNodeReport(ctx, check, "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.DesiredNodes).To(Equal(int32(2)))
		Expect(updated.Status.ReportingNodes).To(Equal(int32(1)))
		Expect(updated.Status.LastResult).To(BeEmpty())
		Expect(updated.Status.LastReportName).To(BeEmpty())
		Expect(updated.Status.LastRunTime).To(BeNil())
		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeCertConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("PartialReports"))

		reports := &fathomv1alpha1.HealthReportList{}
		Expect(k8sClient.List(ctx, reports, client.InNamespace("default"), client.MatchingLabels{
			labelHealthReportSourceKind: "NodeCertificateCheck",
			labelHealthReportSourceName: "nc-partial",
		})).To(Succeed())
		Expect(reports.Items).To(BeEmpty())
	})

	It("clears a previous roll-up when a node report becomes stale", func() {
		name := types.NamespacedName{Name: "nc-stale", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeAgentDaemonSetStatus(ctx, check, 2, 2)
		writeNodeReport(ctx, check, "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})
		writeNodeReport(ctx, check, "node-b", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/kubelet.crt", Subject: "CN=kubelet", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		current := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, current)).To(Succeed())
		Expect(current.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)))
		Expect(current.Status.LastReportName).NotTo(BeEmpty())

		writeNodeReportAt(ctx, check, "node-b", time.Now().Add(-2*nodeCertReportMaxAge(check)), []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/kubelet.crt", Subject: "CN=kubelet", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.DesiredNodes).To(Equal(int32(2)))
		Expect(updated.Status.ReportingNodes).To(Equal(int32(1)))
		Expect(updated.Status.LastResult).To(BeEmpty())
		Expect(updated.Status.LastReportName).To(BeEmpty())
		Expect(updated.Status.LastRunTime).To(BeNil())
		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeCertConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("PartialReports"))
	})

	It("reuses a HealthReport after a status update conflict", func() {
		name := types.NamespacedName{Name: "nc-status-conflict", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeAgentDaemonSetStatus(ctx, check, 2, 2)
		writeNodeReport(ctx, check, "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})
		writeNodeReport(ctx, check, "node-b", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/kubelet.crt", Subject: "CN=kubelet", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})

		conflict := true
		conflictReconciler := newNodeCertReconciler()
		conflictReconciler.Client = conflictOnceStatusClient{Client: k8sClient, conflict: &conflict}
		_, err = conflictReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(apierrors.IsConflict(err)).To(BeTrue())
		Expect(nodeCertHealthReportCount(ctx, name)).To(Equal(1))

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(nodeCertHealthReportCount(ctx, name)).To(Equal(1))

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)))
		Expect(updated.Status.LastReportName).NotTo(BeEmpty())
	})

	It("ignores report ConfigMaps whose payload belongs to a different check", func() {
		name := types.NamespacedName{Name: "nc-mismatched-report", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeAgentDaemonSetStatus(ctx, check, 1, 1)
		writeNodeReportForCheck(ctx, check, "node-a", "other-check", time.Now(), []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.ReportingNodes).To(Equal(int32(0)))
		Expect(updated.Status.LastResult).To(BeEmpty())
		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeCertConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("AwaitingReports"))

		reportCM := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      nodecert.NodeReportConfigMapName(check.Name, "node-a"),
			Namespace: name.Namespace,
		}, reportCM)).To(Succeed())
		Expect(metav1.IsControlledBy(reportCM, check)).To(BeFalse())
	})

	It("removes the node-agent DaemonSet while paused", func() {
		name := types.NamespacedName{Name: "nc-paused", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nc-paused-node-agent", Namespace: "default"}, &appsv1.DaemonSet{})).To(Succeed())

		Expect(k8sClient.Get(ctx, name, check)).To(Succeed())
		check.Spec.Paused = true
		Expect(k8sClient.Update(ctx, check)).To(Succeed())

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.Get(ctx, types.NamespacedName{Name: "nc-paused-node-agent", Namespace: "default"}, &appsv1.DaemonSet{})
		Expect(client.IgnoreNotFound(err)).To(Succeed())
		Expect(err).To(HaveOccurred(), "DaemonSet should be deleted while paused")

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(apiMeta.FindStatusCondition(updated.Status.Conditions, nodeCertConditionPaused).Status).To(Equal(metav1.ConditionTrue))
	})
})
