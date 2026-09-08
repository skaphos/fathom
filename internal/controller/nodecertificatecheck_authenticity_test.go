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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
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
	// Claim the node the annotation names: admission binds the annotation to the
	// writer's node claim, and this models the #155 case where a node-agent that
	// legitimately holds node-a's claim lies in the *payload*. Admission cannot
	// see that; the collect-time binding is what catches it.
	Expect(nodeAgentClient(check, annotationNode).Create(ctx, cm)).To(Succeed())
}

// writeReportAtName writes a fully self-consistent report — payload node and
// annotation agree — at a ConfigMap name of the caller's choosing. This is what
// a namespace principal who is not a node-agent can produce: admission's
// node-claim binding never ran on their write, so the only thing left to catch
// them is the canonical-name binding.
func writeReportAtName(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck, cmName, reportNode string, certs []nodecert.CertResult) {
	report := nodecert.NodeReport{
		Node:       reportNode,
		CheckName:  check.Name,
		ObservedAt: time.Now(),
		Aggregate:  nodecert.WorstOutcome(certs),
		Certs:      certs,
	}
	encoded, err := nodecert.EncodeReport(report)
	Expect(err).NotTo(HaveOccurred())
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: check.Namespace,
			Labels: map[string]string{
				nodecert.LabelManagedBy:  nodecert.ManagedByValue,
				nodecert.LabelSourceKind: nodecert.KindNodeCertificateCheck,
				nodecert.LabelSourceName: check.Name,
				nodecert.LabelNode:       reportNode,
			},
			Annotations: map[string]string{nodecert.AnnotationNodeName: reportNode},
		},
		Data: map[string]string{nodecert.ConfigMapReportKey: encoded},
	}
	// Self-consistent and admission-clean: the writer claims the node it
	// annotates. Only the canonical-name binding catches this one.
	Expect(nodeAgentClient(check, reportNode).Create(ctx, cm)).To(Succeed())
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

		// SEC-1: the rejection is surfaced, not silently skipped.
		authentic := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeCertConditionAuthentic)
		Expect(authentic).NotTo(BeNil())
		Expect(authentic.Status).To(Equal(metav1.ConditionFalse))
		Expect(authentic.Reason).To(Equal(eventReasonForgedReport))
		Expect(authentic.Message).To(ContainSubstring(string(nodecert.RejectNodeMismatch)))
	})

	It("rejects a self-consistent report written at a non-canonical name (SEC-1)", func() {
		name := types.NamespacedName{Name: "nc-offname", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeAgentDaemonSetStatus(ctx, check, 1, 1)

		// The legitimate agent on node-a reports Pass at the canonical name.
		writeNodeReport(ctx, check, "node-a", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
		})
		// A namespace principal whose ServiceAccount does not end in -node-agent
		// was never matched by the old policy at all, so it could write a wholly
		// self-consistent report — payload node and annotation agreeing — for a
		// node it does not run on. Only the canonical-name binding catches it.
		writeReportAtName(ctx, check, "attacker-supplied-report", "node-b", []nodecert.CertResult{
			{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomeFail, DaysRemaining: 1, NotAfter: time.Now().Add(24 * time.Hour)},
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.ReportingNodes).To(Equal(int32(1)), "the off-name report must not be consumed")
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)), "a forged Fail must not poison the aggregate")

		authentic := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeCertConditionAuthentic)
		Expect(authentic).NotTo(BeNil())
		Expect(authentic.Status).To(Equal(metav1.ConditionFalse))
		Expect(authentic.Reason).To(Equal(eventReasonForgedReport))
		Expect(authentic.Message).To(ContainSubstring(string(nodecert.RejectNonCanonicalName)))
		Expect(authentic.Message).To(ContainSubstring("attacker-supplied-report"))
	})

	It("reports every collected node report as bound when none was forged", func() {
		name := types.NamespacedName{Name: "nc-authentic", Namespace: "default"}
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

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		authentic := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeCertConditionAuthentic)
		Expect(authentic).NotTo(BeNil())
		Expect(authentic.Status).To(Equal(metav1.ConditionTrue))
		Expect(authentic.Reason).To(Equal("AllReportsBound"))
	})

	It("denies a node-report write from a principal with no node claim (SEC-1)", func() {
		name := types.NamespacedName{Name: "nc-noclaim", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		// This is the SEC-1 bypass itself. The old policy only *applied* to writers
		// whose ServiceAccount name ended in -node-agent, so any other principal
		// with namespace ConfigMap write — including this test's own admin client —
		// sailed past it and could fabricate a node's verdict. The policy now
		// applies to everyone, so a writer with no node claim is refused at
		// admission, before the controller ever sees the object.
		report := nodecert.NodeReport{
			Node:       "node-a",
			CheckName:  check.Name,
			ObservedAt: time.Now(),
			Aggregate:  nodecert.OutcomePass,
		}
		encoded, err := nodecert.EncodeReport(report)
		Expect(err).NotTo(HaveOccurred())
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nodecert.NodeReportConfigMapName(check.Name, "node-a"),
				Namespace: check.Namespace,
				Labels: map[string]string{
					nodecert.LabelManagedBy:  nodecert.ManagedByValue,
					nodecert.LabelSourceKind: nodecert.KindNodeCertificateCheck,
					nodecert.LabelSourceName: check.Name,
				},
				Annotations: map[string]string{nodecert.AnnotationNodeName: "node-a"},
			},
			Data: map[string]string{nodecert.ConfigMapReportKey: encoded},
		}
		err = k8sClient.Create(ctx, cm)
		Expect(err).To(HaveOccurred(), "a writer with no node claim must not be able to write a node report")
		Expect(apierrors.IsForbidden(err)).To(BeTrue(), "expected an admission denial, got: %v", err)

		// An unannotated report is refused for the same reason: there is nothing
		// for admission to bind to the writer.
		unannotated := cm.DeepCopy()
		unannotated.Annotations = nil
		err = nodeAgentClient(check, "node-a").Create(ctx, unannotated)
		Expect(err).To(HaveOccurred(), "a report with no node-name annotation binds to nothing")
		Expect(apierrors.IsForbidden(err)).To(BeTrue(), "expected an admission denial, got: %v", err)
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
		// SEC-1. A MatchCondition that evaluates false makes the API server skip
		// the policy entirely, so the old writer-is-node-agent name filter exempted
		// precisely the principals it needed to catch. There must be none.
		Expect(policy.Spec.MatchConditions).To(BeEmpty(), "a name-pattern MatchCondition exempts every writer it does not match (SEC-1)")

		// Selected by managed-by alone so NodeHealthCheck (#206) inherits the same
		// boundary instead of provisioning a second copy.
		Expect(policy.Spec.MatchConstraints.ObjectSelector.MatchLabels).To(HaveKeyWithValue(nodecert.LabelManagedBy, nodecert.ManagedByValue))
		Expect(policy.Spec.MatchConstraints.ObjectSelector.MatchLabels).NotTo(HaveKey(nodecert.LabelSourceKind))

		Expect(policy.Spec.Validations).To(HaveLen(1))
		Expect(policy.Spec.Validations[0].Expression).To(ContainSubstring("variables.annotatedNode == variables.claimNode"))
		// The operator's own adoption Update carries no node claim, so it passes
		// on the content-unchanged branch rather than an identity carve-out.
		Expect(policy.Spec.Validations[0].Expression).To(ContainSubstring("variables.contentUnchanged"))

		binding := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: reportAuthenticityPolicyName}, binding)).To(Succeed())
		Expect(binding.Spec.PolicyName).To(Equal(reportAuthenticityPolicyName))
		Expect(binding.Spec.ValidationActions).To(ContainElement(admissionregistrationv1.Deny))
	})
})
