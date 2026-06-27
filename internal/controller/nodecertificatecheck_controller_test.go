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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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
	report := nodecert.NodeReport{
		Node:       node,
		CheckName:  check.Name,
		ObservedAt: time.Now(),
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
		},
		Data: map[string]string{nodecert.ConfigMapReportKey: encoded},
	}
	Expect(k8sClient.Create(ctx, cm)).To(Succeed())
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
