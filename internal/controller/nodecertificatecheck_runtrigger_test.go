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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodecert"
)

var _ = Describe("NodeCertificateCheck run-now trigger", func() {
	ctx := context.Background()
	passing := []nodecert.CertResult{
		{Path: "/etc/kubernetes/pki/apiserver.crt", Subject: "CN=apiserver", Outcome: nodecert.OutcomePass, DaysRemaining: 300, NotAfter: time.Now().Add(300 * 24 * time.Hour)},
	}

	It("stamps the token on the agent template and consumes it only once every node reports it", func() {
		name := types.NamespacedName{Name: "nc-runnow", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name: name.Name, Namespace: name.Namespace,
				Annotations: map[string]string{fathomv1alpha1.AnnotationRunNow: "tok-1"},
			},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		// The token rides on the pod template: annotation for the hash, env var
		// (downward API) for the agent.
		ds := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nc-runnow-node-agent", Namespace: "default"}, ds)).To(Succeed())
		Expect(ds.Spec.Template.Annotations).To(HaveKeyWithValue(fathomv1alpha1.AnnotationRunNow, "tok-1"))
		var triggerEnv *corev1.EnvVar
		for i := range ds.Spec.Template.Spec.Containers[0].Env {
			if ds.Spec.Template.Spec.Containers[0].Env[i].Name == nodecert.EnvRunTrigger {
				triggerEnv = &ds.Spec.Template.Spec.Containers[0].Env[i]
			}
		}
		Expect(triggerEnv).NotTo(BeNil(), "agent must receive the token through "+nodecert.EnvRunTrigger)
		Expect(triggerEnv.ValueFrom.FieldRef.FieldPath).To(Equal("metadata.annotations['" + fathomv1alpha1.AnnotationRunNow + "']"))
		generationWithToken := ds.Generation

		setNodeAgentDaemonSetStatus(ctx, check, 2, 2)

		// Reports that do not carry the token roll up normally but do not
		// consume it: an agent predating the field can never complete a wait.
		writeNodeReport(ctx, check, "node-a", passing)
		writeNodeReport(ctx, check, "node-b", passing)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)))
		Expect(updated.Status.LastRunTrigger).To(BeEmpty(), "untriggered reports must not consume the token")
		firstRun := updated.Status.LastRunTime
		Expect(firstRun).NotTo(BeNil())

		// While the DaemonSet is mid-rollout for this trigger the previous
		// verdict must be retained, not blanked: a forced run must never flap
		// the mirroring HealthCheck/ClusterHealth to no-result.
		setNodeAgentDaemonSetStatusFull(ctx, check, 2, 1, 1)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)), "mid-rollout with a pending trigger must keep the prior verdict")
		Expect(updated.Status.LastReportName).NotTo(BeEmpty(), "mid-rollout with a pending trigger must keep the prior report")
		Expect(updated.Status.LastRunTime).NotTo(BeNil())
		setNodeAgentDaemonSetStatus(ctx, check, 2, 2)

		// One of two nodes carrying the token is not completion.
		writeTriggeredNodeReport(ctx, check, "node-a", "tok-1", passing)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastRunTrigger).To(BeEmpty(), "a partial triggered set must keep the token pending")
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)), "the prior verdict is retained while pending")

		// Every desired node carrying the token completes the trigger, and a
		// forced run whose aggregate did not change still moves LastRunTime.
		writeTriggeredNodeReport(ctx, check, "node-b", "tok-1", passing)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastRunTrigger).To(Equal("tok-1"))
		Expect(updated.Status.LastRunTime).NotTo(BeNil())
		Expect(updated.Status.LastRunTime.Time).To(BeTemporally(">=", firstRun.Time))

		// Consumption must not roll the agents a second time.
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nc-runnow-node-agent", Namespace: "default"}, ds)).To(Succeed())
		Expect(ds.Generation).To(Equal(generationWithToken), "consuming the token must not rewrite the DaemonSet template")

		// A reconcile with the annotation gone preserves the consumed token and,
		// because the template falls back to it, does not roll the agents.
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		updated.Annotations = nil
		Expect(k8sClient.Update(ctx, updated)).To(Succeed())
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastRunTrigger).To(Equal("tok-1"), "a run with no annotation must not clear the consumed token")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nc-runnow-node-agent", Namespace: "default"}, ds)).To(Succeed())
		Expect(ds.Generation).To(Equal(generationWithToken), "removing a consumed annotation must not rewrite the DaemonSet template")
		Expect(ds.Spec.Template.Annotations).To(HaveKeyWithValue(fathomv1alpha1.AnnotationRunNow, "tok-1"))
	})

	It("leaves the token unconsumed while paused", func() {
		name := types.NamespacedName{Name: "nc-runnow-paused", Namespace: "default"}
		check := &fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name: name.Name, Namespace: name.Namespace,
				Annotations: map[string]string{fathomv1alpha1.AnnotationRunNow: "tok-p"},
			},
			Spec: fathomv1alpha1.NodeCertificateCheckSpec{Paused: true},
		}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeCertReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.NodeCertificateCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.LastRunTrigger).To(BeEmpty())
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "nc-runnow-paused-node-agent", Namespace: "default"}, &appsv1.DaemonSet{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "paused checks run no agents, so nothing can consume the token")
	})

	It("changes the template hash with the token and keeps an untriggered template unchanged", func() {
		r := newNodeCertReconciler()
		base := &fathomv1alpha1.NodeCertificateCheck{ObjectMeta: metav1.ObjectMeta{Name: "nc-hash", Namespace: "default"}}
		withToken := base.DeepCopy()
		withToken.Annotations = map[string]string{fathomv1alpha1.AnnotationRunNow: "tok-h"}

		plain := r.desiredDaemonSet(base, "sa")
		triggered := r.desiredDaemonSet(withToken, "sa")

		Expect(nodeAgentSpecHash(plain)).To(Equal(nodeAgentSpecHash(r.desiredDaemonSet(base, "sa"))), "hash must be stable without a token")
		Expect(nodeAgentSpecHash(plain)).NotTo(Equal(nodeAgentSpecHash(triggered)), "a token must change the hash so the agents roll")
		// A check that has never been triggered keeps exactly today's template:
		// no annotation, only the NODE_NAME env var. That is what keeps an
		// operator upgrade from restarting every agent fleet-wide.
		Expect(plain.Spec.Template.Annotations).To(BeEmpty())
		Expect(plain.Spec.Template.Spec.Containers[0].Env).To(HaveLen(1))
		Expect(triggered.Spec.Template.Spec.Containers[0].Env).To(HaveLen(2))
	})
})
