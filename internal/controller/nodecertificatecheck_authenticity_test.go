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

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodecert"
)

// writeReportWithAnnotation writes a per-node report ConfigMap where the payload's
// Node and the authenticity node-name annotation can be set independently, so a
// test can forge the mismatch a compromised node would produce. An empty
// annotationNode omits the annotation entirely (a pre-authenticity report).
func writeReportWithAnnotation(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck, cmSuffix, reportNode, annotationNode string, certs []nodecert.CertResult) {
	report := nodecert.NodeReport{
		Node:       reportNode,
		CheckName:  check.Name,
		ObservedAt: time.Now(),
		Aggregate:  nodecert.WorstOutcome(certs),
		Certs:      certs,
	}
	encoded, err := nodecert.EncodeReport(report)
	Expect(err).NotTo(HaveOccurred())

	annotations := map[string]string{}
	if annotationNode != "" {
		annotations[nodecert.AnnotationNodeName] = annotationNode
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodecert.NodeReportConfigMapName(check.Name, cmSuffix),
			Namespace: check.Namespace,
			Labels: map[string]string{
				nodecert.LabelManagedBy:  nodecert.ManagedByValue,
				nodecert.LabelSourceKind: nodecert.KindNodeCertificateCheck,
				nodecert.LabelSourceName: check.Name,
				nodecert.LabelNode:       reportNode,
			},
			Annotations: annotations,
		},
		Data: map[string]string{nodecert.ConfigMapReportKey: encoded},
	}
	Expect(k8sClient.Create(ctx, cm)).To(Succeed())
}

var _ = Describe("NodeCertificateCheck report authenticity (#155)", func() {
	ctx := context.Background()

	It("excludes a report whose payload node does not match its authenticated annotation", func() {
		name := types.NamespacedName{Name: "nc-forged", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeAgentDaemonSetStatus(ctx, check, 1, 1)

		// Legitimate node-a report: annotation matches payload, Pass.
		writeNodeReport(ctx, check, "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})
		// Forged report: a compromised node-a (annotation node-a — what the
		// admission policy would bind) writes a report claiming to be node-b with a
		// Fail verdict, to poison the aggregate. The controller must drop it.
		writeReportWithAnnotation(ctx, check, "node-b", "node-b", "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomeFail, DaysRemaining: 1, NotAfter: time.Now().Add(24 * time.Hour)},
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.ReportingNodes).To(Equal(int32(1)), "only the authentic node-a report may count")
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)), "the forged Fail must not poison the aggregate")
	})

	It("excludes a report missing the node-name annotation", func() {
		name := types.NamespacedName{Name: "nc-unannotated", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeAgentDaemonSetStatus(ctx, check, 1, 1)

		writeReportWithAnnotation(ctx, check, "node-a", "node-a", "", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.ReportingNodes).To(Equal(int32(0)), "an unauthenticated report must not be consumed")
	})

	It("provisions the report-authenticity ValidatingAdmissionPolicy and binding", func() {
		name := types.NamespacedName{Name: "nc-vap", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		policy := &admissionregistrationv1.ValidatingAdmissionPolicy{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: reportAuthenticityPolicyName}, policy)).To(Succeed())
		Expect(policy.Spec.FailurePolicy).To(HaveValue(Equal(admissionregistrationv1.Fail)))
		Expect(policy.Spec.MatchConstraints.ObjectSelector.MatchLabels).To(HaveKeyWithValue(nodecert.LabelSourceKind, nodecert.KindNodeCertificateCheck))
		Expect(policy.Spec.MatchConditions).To(HaveLen(1))
		Expect(policy.Spec.MatchConditions[0].Expression).To(ContainSubstring("-node-agent$"))
		Expect(policy.Spec.Validations).To(HaveLen(1))
		Expect(policy.Spec.Validations[0].Expression).To(ContainSubstring("variables.annotatedNode == variables.claimNode"))

		binding := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: reportAuthenticityPolicyName}, binding)).To(Succeed())
		Expect(binding.Spec.PolicyName).To(Equal(reportAuthenticityPolicyName))
		Expect(binding.Spec.ValidationActions).To(ContainElement(admissionregistrationv1.Deny))
	})
})
