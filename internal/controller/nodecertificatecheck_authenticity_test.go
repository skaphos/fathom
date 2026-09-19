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

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/internal/nodehealth"
)

func reportWriterClient(namespace, serviceAccount, claimNode string) client.Client {
	impersonated := rest.CopyConfig(cfg)
	impersonated.Impersonate = rest.ImpersonationConfig{
		UserName: "system:serviceaccount:" + namespace + ":" + serviceAccount,
		Extra:    map[string][]string{"authentication.kubernetes.io/node-name": {claimNode}},
	}
	c, err := client.New(impersonated, client.Options{Scheme: k8sClient.Scheme()})
	Expect(err).NotTo(HaveOccurred())
	return c
}

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
// annotation agree — at a ConfigMap name of the caller's choosing. Admission
// authenticates the writer→node binding via the node-name claim, but it cannot
// enforce that the report is written at the canonical ConfigMap name; the
// canonical-name binding is what catches this one.
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

		Expect(policy.Spec.Validations).To(HaveLen(2))
		Expect(policy.Spec.Validations[0].Expression).To(Equal("variables.identityUnchanged"))
		Expect(policy.Spec.Validations[1].Expression).To(ContainSubstring("variables.annotatedNode == variables.claimNode"))
		Expect(policy.Spec.Validations[1].Expression).To(ContainSubstring("request.userInfo.username == variables.expectedWriter"))
		// The operator's own adoption Update carries no node claim, so it passes
		// on the content-unchanged branch rather than an identity carve-out.
		Expect(policy.Spec.Validations[1].Expression).To(ContainSubstring("variables.contentUnchanged"))

		binding := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: reportAuthenticityPolicyName}, binding)).To(Succeed())
		Expect(binding.Spec.PolicyName).To(Equal(reportAuthenticityPolicyName))
		Expect(binding.Spec.ValidationActions).To(ContainElement(admissionregistrationv1.Deny))
	})

	It("keeps report identity immutable for both node-scoped kinds while allowing adoption and same-node refresh (#338)", func() {
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: "nc-identity", Namespace: "default"},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		// Reconcile once so this test always exercises the current singleton spec,
		// independent of which Ginkgo example happened to create it first.
		_, err := newNodeCertReconciler().Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(check)})
		Expect(err).NotTo(HaveOccurred())
		healthServiceAccount := check.Name + nodeHealthAgentSuffix
		for _, sourceKind := range []string{nodecert.KindNodeCertificateCheck, nodehealth.KindNodeHealthCheck} {
			sourceKind := sourceKind
			serviceAccount := agentResourceName(check)
			if sourceKind == nodehealth.KindNodeHealthCheck {
				serviceAccount = healthServiceAccount
			}
			writer := func(node string) client.Client {
				return reportWriterClient(check.Namespace, serviceAccount, node)
			}
			By("protecting " + sourceKind + " report identity")
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "identity-" + strings.ToLower(sourceKind),
					Namespace: check.Namespace,
					Labels: map[string]string{
						nodecert.LabelManagedBy:  nodecert.ManagedByValue,
						nodecert.LabelSourceKind: sourceKind,
						nodecert.LabelSourceName: check.Name,
					},
					Annotations: map[string]string{nodecert.AnnotationNodeName: "node-a"},
				},
				Data: map[string]string{nodecert.ConfigMapReportKey: `{"node":"node-a","generation":1}`},
			}
			// Grant this deliberately non-canonical fixture's name so every update
			// reaches admission; these assertions exercise the policy, not RBAC.
			Expect(ensureScopedReportRBAC(ctx, k8sClient, k8sClient.Scheme(), check, agentLabels(check), serviceAccount, []string{cm.Name})).To(Succeed())
			Expect(writer("node-a").Create(ctx, cm)).To(Succeed())
			DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cm))).To(Succeed()) })

			mutations := []struct {
				name   string
				mutate func(*corev1.ConfigMap)
			}{
				{"remove managed-by", func(cm *corev1.ConfigMap) { delete(cm.Labels, nodecert.LabelManagedBy) }},
				{"change managed-by", func(cm *corev1.ConfigMap) { cm.Labels[nodecert.LabelManagedBy] = "attacker" }},
				{"remove source-kind", func(cm *corev1.ConfigMap) { delete(cm.Labels, nodecert.LabelSourceKind) }},
				{"change source-kind", func(cm *corev1.ConfigMap) { cm.Labels[nodecert.LabelSourceKind] = "Other" }},
				{"remove source-name", func(cm *corev1.ConfigMap) { delete(cm.Labels, nodecert.LabelSourceName) }},
				{"change source-name", func(cm *corev1.ConfigMap) { cm.Labels[nodecert.LabelSourceName] = "other" }},
				{"rebind node annotation", func(cm *corev1.ConfigMap) { cm.Annotations[nodecert.AnnotationNodeName] = "node-b" }},
			}
			for _, mutation := range mutations {
				current := &corev1.ConfigMap{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), current)).To(Succeed())
				mutation.mutate(current)
				err := writer("node-b").Update(ctx, current)
				Expect(err).To(HaveOccurred(), mutation.name+" must be denied for "+sourceKind)
				Expect(apierrors.IsForbidden(err)).To(BeTrue(), "expected admission denial for %s/%s, got: %v", sourceKind, mutation.name, err)
			}

			// Owner-reference adoption changes metadata outside the protected
			// identity and carries no node claim; unchanged report content permits it.
			current := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), current)).To(Succeed())
			Expect(controllerutil.SetControllerReference(check, current, k8sClient.Scheme())).To(Succeed())
			Expect(k8sClient.Update(ctx, current)).To(Succeed())

			// The genuine node may refresh report content while retaining identity.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), current)).To(Succeed())
			current.Data[nodecert.ConfigMapReportKey] = `{"node":"node-a","generation":2}`
			Expect(writer("node-a").Update(ctx, current)).To(Succeed())

			// A different check's agent on the same node has an equally valid node
			// claim and namespace-wide ConfigMap permission, but it is not the
			// ServiceAccount named by this report's source identity.
			crossCheck := cm.DeepCopy()
			crossCheck.ResourceVersion = ""
			crossCheck.UID = ""
			crossCheck.OwnerReferences = nil
			crossCheck.Name += "-cross-check"
			crossCheck.Labels[nodecert.LabelSourceName] = check.Name + "-victim"
			err = writer("node-a").Create(ctx, crossCheck)
			Expect(err).To(HaveOccurred(), "one check's %s agent must not write another check's report", sourceKind)
			Expect(apierrors.IsForbidden(err)).To(BeTrue(), "expected cross-check admission denial for %s, got: %v", sourceKind, err)
		}
	})

	It("limits report reads and updates to the check's active canonical report names", func() {
		check := &fathomv1alpha1.NodeCertificateCheck{ObjectMeta: metav1.ObjectMeta{Name: "nc-scoped-rbac", Namespace: "default"}}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(check)})
		Expect(err).NotTo(HaveOccurred())
		scheduleAgentPods(ctx, check, "node-a")
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(check)})
		Expect(err).NotTo(HaveOccurred())

		role := &rbacv1.Role{}
		roleKey := types.NamespacedName{Name: scopedReportAccessName(agentResourceName(check)), Namespace: check.Namespace}
		Expect(k8sClient.Get(ctx, roleKey, role)).To(Succeed())
		Expect(role.Rules).To(HaveLen(2))
		ownName := nodecert.NodeReportConfigMapName(check.Name, "node-a")
		Expect(role.Rules).To(ContainElement(rbacv1.PolicyRule{
			APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"create"},
		}))
		Expect(role.Rules).To(ContainElement(rbacv1.PolicyRule{
			APIGroups: []string{""}, Resources: []string{"configmaps"},
			ResourceNames: []string{ownName}, Verbs: []string{"get", "update"},
		}))

		writer := reportWriterClient(check.Namespace, agentResourceName(check), "node-a")
		own := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: ownName, Namespace: check.Namespace,
			Labels: map[string]string{
				nodecert.LabelManagedBy: nodecert.ManagedByValue, nodecert.LabelSourceKind: nodecert.KindNodeCertificateCheck,
				nodecert.LabelSourceName: check.Name, nodecert.LabelNode: "node-a",
			},
			Annotations: map[string]string{nodecert.AnnotationNodeName: "node-a"},
		}, Data: map[string]string{nodecert.ConfigMapReportKey: `{"node":"node-a"}`}}
		Expect(writer.Create(ctx, own)).To(Succeed(), "the per-check Role grants the minimum create capability")
		Eventually(func() error { return writer.Get(ctx, client.ObjectKeyFromObject(own), &corev1.ConfigMap{}) }).Should(Succeed())
		Expect(writer.Get(ctx, client.ObjectKeyFromObject(own), own)).To(Succeed())
		own.Data[nodecert.ConfigMapReportKey] = `{"node":"node-a","generation":2}`
		Expect(writer.Update(ctx, own)).To(Succeed())

		ordinary := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "ordinary-config", Namespace: check.Namespace}}
		Expect(k8sClient.Create(ctx, ordinary)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ordinary))).To(Succeed()) })
		crossCheck := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: nodecert.NodeReportConfigMapName("another-check", "node-a"), Namespace: check.Namespace}}
		Expect(k8sClient.Create(ctx, crossCheck)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, crossCheck))).To(Succeed()) })
		for _, forbidden := range []*corev1.ConfigMap{ordinary, crossCheck} {
			err = writer.Get(ctx, client.ObjectKeyFromObject(forbidden), &corev1.ConfigMap{})
			Expect(apierrors.IsForbidden(err)).To(BeTrue(), "GET %s must be denied, got %v", forbidden.Name, err)
			forbidden.Data = map[string]string{"changed": "true"}
			err = writer.Update(ctx, forbidden)
			Expect(apierrors.IsForbidden(err)).To(BeTrue(), "UPDATE %s must be denied, got %v", forbidden.Name, err)
		}

		scheduleAgentPods(ctx, check)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(check)})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, roleKey, role)).To(Succeed())
		Expect(role.Rules).To(ConsistOf(rbacv1.PolicyRule{
			APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"create"},
		}), "an empty fleet must retain create without wildcard read/update access")
		Eventually(func() bool {
			err := writer.Get(ctx, client.ObjectKeyFromObject(own), &corev1.ConfigMap{})
			return apierrors.IsForbidden(err)
		}).Should(BeTrue(), "a departed node's report permission must be revoked")
	})

	It("revokes the per-check ConfigMap grant while an agent DaemonSet still exists", func() {
		check := &fathomv1alpha1.NodeCertificateCheck{ObjectMeta: metav1.ObjectMeta{Name: "nc-create-revoke", Namespace: "default"}}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })
		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(check)})
		Expect(err).NotTo(HaveOccurred())
		writer := reportWriterClient(check.Namespace, agentResourceName(check), "node-a")
		before := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "ordinary-before-revoke", Namespace: check.Namespace}}
		Expect(writer.Create(ctx, before)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, before))).To(Succeed()) })

		Expect(clearNodeAgentAccess(ctx, k8sClient, k8sClient, check, agentResourceName(check), r.roleName())).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: agentResourceName(check), Namespace: check.Namespace}, &appsv1.DaemonSet{})).To(Succeed(), "access revocation must work even if DaemonSet deletion later fails")
		Eventually(func() bool {
			after := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{GenerateName: "ordinary-after-revoke-", Namespace: check.Namespace}}
			err := writer.Create(ctx, after)
			if err == nil {
				Expect(k8sClient.Delete(ctx, after)).To(Succeed())
				return false
			}
			return apierrors.IsForbidden(err)
		}).Should(BeTrue(), "clearing the per-check Role must remove ConfigMap create access")
	})

	It("revokes scoped report updates while paused and persists revocation failures", func() {
		check := &fathomv1alpha1.NodeCertificateCheck{ObjectMeta: metav1.ObjectMeta{Name: "nc-pause-rbac", Namespace: "default"}}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })
		r := newNodeCertReconciler()
		request := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(check)}
		_, err := r.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		scheduleAgentPods(ctx, check, "node-a")
		_, err = r.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		roleKey := types.NamespacedName{Name: scopedReportAccessName(agentResourceName(check)), Namespace: check.Namespace}
		role := &rbacv1.Role{}
		Expect(k8sClient.Get(ctx, roleKey, role)).To(Succeed())
		Expect(role.Rules).NotTo(BeEmpty())
		Expect(k8sClient.Get(ctx, request.NamespacedName, check)).To(Succeed())
		check.Spec.Paused = true
		Expect(k8sClient.Update(ctx, check)).To(Succeed())
		_, err = r.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, roleKey, role)).To(Succeed())
		Expect(role.Rules).To(BeEmpty())

		// A role that lost the check's controller reference is not ours to edit.
		Expect(k8sClient.Get(ctx, request.NamespacedName, check)).To(Succeed())
		check.Spec.Paused = false
		Expect(k8sClient.Update(ctx, check)).To(Succeed())
		_, err = r.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		legacyBinding := &rbacv1.RoleBinding{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: agentResourceName(check), Namespace: check.Namespace}, legacyBinding)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "resume must not restore the legacy ClusterRole binding")
		scheduleAgentPods(ctx, check, "node-a")
		_, err = r.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, roleKey, role)).To(Succeed())
		role.OwnerReferences = nil
		Expect(k8sClient.Update(ctx, role)).To(Succeed())
		Expect(k8sClient.Get(ctx, request.NamespacedName, check)).To(Succeed())
		check.Spec.Paused = true
		Expect(k8sClient.Update(ctx, check)).To(Succeed())
		_, err = r.Reconcile(ctx, request)
		Expect(err).To(HaveOccurred())
		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, request.NamespacedName, updated)).To(Succeed())
		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeCertConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("RBACRevocationFailed"))
		err = k8sClient.Get(ctx, types.NamespacedName{Name: agentResourceName(check), Namespace: check.Namespace}, &appsv1.DaemonSet{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "DaemonSet deletion must still be attempted when Role revocation fails")
	})
})
