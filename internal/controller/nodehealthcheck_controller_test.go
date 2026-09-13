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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/internal/nodehealth"
)

func newNodeHealthReconciler() *NodeHealthCheckReconciler {
	return &NodeHealthCheckReconciler{
		Client:            k8sClient,
		Scheme:            k8sClient.Scheme(),
		NodeAgentImage:    "ghcr.io/skaphos/fathom-node-agent:test",
		NodeAgentRoleName: defaultNodeAgentRoleName,
		// k8sClient is uncached, so it is a legitimate NodeReader here.
		NodeReader: k8sClient,
	}
}

var nodeHealthPassing = []nodehealth.CheckResult{
	{Type: nodehealth.TypeDiskHeadroom, Path: "/var/lib/kubelet", Outcome: nodehealth.OutcomePass, Summary: "55.0% of bytes free", PercentFree: ptr.To(55.0)},
}

// nodeHealthAgentClient writes as the per-check node-health agent
// ServiceAccount carrying the node claim for claimNode, the only identity the
// report-authenticity policy admits (SEC-1).
func nodeHealthAgentClient(check *fathomv1alpha1.NodeHealthCheck, claimNode string) client.Client {
	impersonated := rest.CopyConfig(cfg)
	impersonated.Impersonate = rest.ImpersonationConfig{
		UserName: "system:serviceaccount:" + check.Namespace + ":" + nodeHealthAgentResourceName(check),
		Extra:    map[string][]string{"authentication.kubernetes.io/node-name": {claimNode}},
	}
	c, err := client.New(impersonated, client.Options{Scheme: k8sClient.Scheme()})
	Expect(err).NotTo(HaveOccurred())
	return c
}

func writeNodeHealthReport(ctx context.Context, check *fathomv1alpha1.NodeHealthCheck, node string, checks []nodehealth.CheckResult) {
	writeNodeHealthReportObject(ctx, check, node, node, nodehealth.NodeReport{
		Node: node, CheckName: check.Name, ObservedAt: time.Now(), Aggregate: nodehealth.WorstOutcome(checks), Checks: checks,
	})
}

func writeNodeHealthReportAt(ctx context.Context, check *fathomv1alpha1.NodeHealthCheck, node string, observedAt time.Time, checks []nodehealth.CheckResult) {
	writeNodeHealthReportObject(ctx, check, node, node, nodehealth.NodeReport{
		Node: node, CheckName: check.Name, ObservedAt: observedAt, Aggregate: nodehealth.WorstOutcome(checks), Checks: checks,
	})
}

func writeTriggeredNodeHealthReport(ctx context.Context, check *fathomv1alpha1.NodeHealthCheck, node, trigger string, checks []nodehealth.CheckResult) {
	writeNodeHealthReportObject(ctx, check, node, node, nodehealth.NodeReport{
		Node: node, CheckName: check.Name, ObservedAt: time.Now(), Aggregate: nodehealth.WorstOutcome(checks), Checks: checks, Trigger: trigger,
	})
}

// writeNodeHealthReportObject upserts a report at the canonical name for
// cmNode, annotated (and written by an identity claiming) annotationNode. A
// genuine agent passes the same node for both; a forgery test separates them.
func writeNodeHealthReportObject(ctx context.Context, check *fathomv1alpha1.NodeHealthCheck, cmNode, annotationNode string, report nodehealth.NodeReport) {
	encoded, err := nodehealth.EncodeReport(report)
	Expect(err).NotTo(HaveOccurred())
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodehealth.ReportConfigMapName(check.Name, cmNode),
			Namespace: check.Namespace,
			Labels: map[string]string{
				nodecert.LabelManagedBy:  nodecert.ManagedByValue,
				nodecert.LabelSourceKind: nodehealth.KindNodeHealthCheck,
				nodecert.LabelSourceName: check.Name,
				nodecert.LabelNode:       cmNode,
			},
			Annotations: map[string]string{nodecert.AnnotationNodeName: annotationNode},
		},
		Data: map[string]string{nodecert.ConfigMapReportKey: encoded},
	}
	writer := nodeHealthAgentClient(check, annotationNode)
	existing := &corev1.ConfigMap{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, existing)
	if err == nil {
		existing.Data = cm.Data
		existing.Labels = cm.Labels
		existing.Annotations = cm.Annotations
		Expect(writer.Update(ctx, existing)).To(Succeed())
		return
	}
	Expect(client.IgnoreNotFound(err)).To(Succeed())
	Expect(writer.Create(ctx, cm)).To(Succeed())
}

// setNodeHealthDaemonSetStatus marks the DaemonSet fully rolled out and fakes
// the agent pods the real DaemonSet controller would create (envtest runs
// neither a DaemonSet controller nor a scheduler).
func setNodeHealthDaemonSetStatus(ctx context.Context, check *fathomv1alpha1.NodeHealthCheck, desired, ready int32) {
	ds := &appsv1.DaemonSet{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeHealthAgentResourceName(check), Namespace: check.Namespace}, ds)).To(Succeed())
	ds.Status.DesiredNumberScheduled = desired
	ds.Status.CurrentNumberScheduled = desired
	ds.Status.UpdatedNumberScheduled = desired
	ds.Status.NumberAvailable = ready
	ds.Status.NumberReady = ready
	ds.Status.ObservedGeneration = ds.Generation
	Expect(k8sClient.Status().Update(ctx, ds)).To(Succeed())
	scheduleNodeHealthAgentPods(ctx, check, conventionalAgentNodes(desired)...)
}

// scheduleNodeHealthAgentPods makes the agent pod set for check exactly nodes,
// under this kind's three-label selector.
func scheduleNodeHealthAgentPods(ctx context.Context, check *fathomv1alpha1.NodeHealthCheck, nodes ...string) {
	labels := nodeHealthAgentSelectorLabels(check)
	keep := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		keep[node] = struct{}{}
	}
	var existing corev1.PodList
	Expect(k8sClient.List(ctx, &existing, client.InNamespace(check.Namespace), client.MatchingLabels(labels))).To(Succeed())
	for i := range existing.Items {
		pod := &existing.Items[i]
		if _, ok := keep[pod.Spec.NodeName]; ok {
			continue
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, pod, client.GracePeriodSeconds(0)))).To(Succeed())
	}
	ds := &appsv1.DaemonSet{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeHealthAgentResourceName(check), Namespace: check.Namespace}, ds)).To(Succeed())
	for _, node := range nodes {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: nodeHealthAgentResourceName(check) + "-" + node, Namespace: check.Namespace, Labels: labels},
			Spec: corev1.PodSpec{
				NodeName:   node,
				Containers: []corev1.Container{{Name: "node-agent", Image: "ghcr.io/skaphos/fathom-node-agent:test"}},
			},
		}
		Expect(controllerutil.SetControllerReference(ds, pod, k8sClient.Scheme())).To(Succeed())
		err := k8sClient.Create(ctx, pod)
		if apierrors.IsAlreadyExists(err) {
			continue
		}
		Expect(err).NotTo(HaveOccurred())
	}
}

// ensureNode creates (or updates) a cluster-scoped Node with the given
// conditions so NodeCondition items have something real to grade.
func ensureNode(ctx context.Context, name string, conditions ...corev1.NodeCondition) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, node)
	if apierrors.IsNotFound(err) {
		Expect(k8sClient.Create(ctx, node)).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, node)).To(Succeed())
	} else {
		Expect(err).NotTo(HaveOccurred())
	}
	node.Status.Conditions = conditions
	Expect(k8sClient.Status().Update(ctx, node)).To(Succeed())
}

func healthyNodeConditions() []corev1.NodeCondition {
	return []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
		{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
		{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
		{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
	}
}

func nodeHealthHealthReportCount(ctx context.Context, source types.NamespacedName) int {
	reports := &fathomv1alpha1.HealthReportList{}
	Expect(k8sClient.List(ctx, reports, client.InNamespace(source.Namespace), client.MatchingLabels{
		fathomv1alpha1.LabelHealthReportSourceKind: nodehealth.KindNodeHealthCheck,
		fathomv1alpha1.LabelHealthReportSourceName: source.Name,
	})).To(Succeed())
	return len(reports.Items)
}

func newNHC(name types.NamespacedName, items ...fathomv1alpha1.NodeHealthCheckItem) *fathomv1alpha1.NodeHealthCheck {
	return &fathomv1alpha1.NodeHealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		Spec:       fathomv1alpha1.NodeHealthCheckSpec{Checks: items},
	}
}

var _ = Describe("NodeHealthCheck Controller", func() {
	ctx := context.Background()
	headroom := fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet"}

	It("provisions a hardened agent for a headroom-only spec: no host network, non-root, read-only mounts", func() {
		name := types.NamespacedName{Name: "nh-provision", Namespace: "default"}
		check := newNHC(name, headroom,
			fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckInodeHeadroom, Path: "/var/log"},
			fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckNodeCondition})
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		ds := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nh-provision-node-health-agent", Namespace: "default"}, ds)).To(Succeed())
		pod := ds.Spec.Template.Spec
		container := pod.Containers[0]
		Expect(container.Image).To(Equal("ghcr.io/skaphos/fathom-node-agent:test"))
		Expect(container.Args).To(ContainElements("--mode", "health", "--check-name", "nh-provision", "--check-namespace", "default"))
		// NodeCondition is operator-side: it must not be handed to the agent.
		checksArg := container.Args[indexOf(container.Args, "--checks")+1]
		Expect(checksArg).To(ContainSubstring(`"DiskHeadroom"`))
		Expect(checksArg).To(ContainSubstring(`"InodeHeadroom"`))
		Expect(checksArg).NotTo(ContainSubstring("NodeCondition"))
		// Resolved defaults ride in the args, so the agent never infers them.
		Expect(checksArg).To(ContainSubstring(`"warnPercentFree":20`))
		Expect(checksArg).To(ContainSubstring(`"criticalPercentFree":10`))

		Expect(pod.HostNetwork).To(BeFalse())
		Expect(pod.DNSPolicy).To(Equal(corev1.DNSClusterFirst))
		Expect(pod.SecurityContext.RunAsNonRoot).To(HaveValue(BeTrue()))
		Expect(pod.SecurityContext.RunAsUser).To(HaveValue(BeEquivalentTo(65532)))
		Expect(container.SecurityContext.ReadOnlyRootFilesystem).To(HaveValue(BeTrue()))
		Expect(container.SecurityContext.Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))
		Expect(container.Ports[0].ContainerPort).To(BeEquivalentTo(metricsContainerPort))

		// /var/lib/kubelet and /var/log: two read-only directory mounts, no socket.
		Expect(pod.Volumes).To(HaveLen(2))
		for _, v := range pod.Volumes {
			Expect(*v.HostPath.Type).To(Equal(corev1.HostPathDirectoryOrCreate))
		}
		for _, m := range container.VolumeMounts {
			Expect(m.ReadOnly).To(BeTrue())
		}

		// Selector carries the kind, so a NodeCertificateCheck of the same name
		// can never match these pods.
		Expect(ds.Spec.Selector.MatchLabels).To(HaveKeyWithValue(nodecert.LabelSourceKind, nodehealth.KindNodeHealthCheck))

		sa := &corev1.ServiceAccount{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nh-provision-node-health-agent", Namespace: "default"}, sa)).To(Succeed())
		rb := &rbacv1.RoleBinding{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nh-provision-node-health-agent", Namespace: "default"}, rb)).To(Succeed())
		Expect(rb.RoleRef.Name).To(Equal(defaultNodeAgentRoleName), "reuses the shared node-agent ClusterRole")
		np := &networkingv1.NetworkPolicy{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nh-provision-node-health-agent", Namespace: "default"}, np)).To(Succeed())
		Expect(np.Spec.PodSelector.MatchLabels).To(Equal(ds.Spec.Selector.MatchLabels))
		Expect(np.Spec.Ingress[0].Ports[0].Port.IntValue()).To(Equal(metricsContainerPort))

		updated := &fathomv1alpha1.NodeHealthCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(apiMeta.FindStatusCondition(updated.Status.Conditions, nodeHealthConditionAccepted).Status).To(Equal(metav1.ConditionTrue))
		privileged := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeHealthConditionPrivileged)
		Expect(privileged).NotTo(BeNil())
		Expect(privileged.Status).To(Equal(metav1.ConditionFalse))
		Expect(privileged.Reason).To(Equal("Hardened"))
	})

	It("grants host network only for KubeletHealthz and root only for ContainerRuntime, and says so on the object", func() {
		name := types.NamespacedName{Name: "nh-priv", Namespace: "default"}
		check := newNHC(name, fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckKubeletHealthz})
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		ds := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nh-priv-node-health-agent", Namespace: "default"}, ds)).To(Succeed())
		pod := ds.Spec.Template.Spec
		Expect(pod.HostNetwork).To(BeTrue())
		Expect(pod.DNSPolicy).To(Equal(corev1.DNSClusterFirstWithHostNet))
		Expect(pod.SecurityContext.RunAsNonRoot).To(HaveValue(BeTrue()), "host network alone does not need root")
		Expect(pod.Volumes).To(BeEmpty(), "no headroom paths, nothing to mount")
		hostPort := pod.Containers[0].Ports[0].ContainerPort
		Expect(hostPort).To(BeNumerically(">=", nodeHealthHostMetricsPortMin))
		Expect(hostPort).To(BeNumerically("<=", nodeHealthHostMetricsPortMax))
		Expect(pod.Containers[0].Args).To(ContainElement("--metrics-bind-address"))
		np := &networkingv1.NetworkPolicy{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nh-priv-node-health-agent", Namespace: "default"}, np)).To(Succeed())
		Expect(np.Spec.Ingress[0].Ports[0].Port.IntValue()).To(BeEquivalentTo(hostPort), "policy follows the host port")

		updated := &fathomv1alpha1.NodeHealthCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		privileged := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeHealthConditionPrivileged)
		Expect(privileged.Status).To(Equal(metav1.ConditionTrue))
		Expect(privileged.Reason).To(Equal("HostNetwork"))
		Expect(privileged.Message).To(ContainSubstring("NetworkPolicy does not isolate"))

		// Add ContainerRuntime: root + the socket mounted as type Socket.
		Expect(k8sClient.Get(ctx, name, check)).To(Succeed())
		check.Spec.Checks = append(check.Spec.Checks, fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckContainerRuntime, SocketPath: "/run/crio/crio.sock"})
		Expect(k8sClient.Update(ctx, check)).To(Succeed())
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nh-priv-node-health-agent", Namespace: "default"}, ds)).To(Succeed())
		pod = ds.Spec.Template.Spec
		Expect(pod.SecurityContext.RunAsNonRoot).To(HaveValue(BeFalse()))
		Expect(pod.SecurityContext.RunAsUser).To(HaveValue(BeEquivalentTo(0)))
		Expect(pod.Containers[0].SecurityContext.Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")), "root still drops every capability")
		Expect(pod.Containers[0].SecurityContext.ReadOnlyRootFilesystem).To(HaveValue(BeTrue()))
		Expect(pod.Volumes).To(HaveLen(1))
		Expect(pod.Volumes[0].HostPath.Path).To(Equal("/run/crio/crio.sock"))
		Expect(*pod.Volumes[0].HostPath.Type).To(Equal(corev1.HostPathSocket))

		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		privileged = apiMeta.FindStatusCondition(updated.Status.Conditions, nodeHealthConditionPrivileged)
		Expect(privileged.Reason).To(Equal("HostNetworkAndRoot"))
		Expect(privileged.Message).To(ContainSubstring("/run/crio/crio.sock"))
	})

	It("is idempotent: a second reconcile and report rewrites do not churn the DaemonSet template", func() {
		name := types.NamespacedName{Name: "nh-idem", Namespace: "default"}
		check := newNHC(name, headroom, fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckKubeletHealthz})
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		ds := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nh-idem-node-health-agent", Namespace: "default"}, ds)).To(Succeed())
		gen, hash := ds.Generation, ds.Annotations[nodeAgentSpecHashAnnotation]
		Expect(hash).NotTo(BeEmpty())

		setNodeHealthDaemonSetStatus(ctx, check, 1, 1)
		for i := 0; i < 3; i++ {
			writeNodeHealthReport(ctx, check, "node-a", nodeHealthPassing)
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nh-idem-node-health-agent", Namespace: "default"}, ds)).To(Succeed())
		Expect(ds.Generation).To(Equal(gen), "template rewritten without an intent change (#143 churn class)")
		Expect(ds.Annotations[nodeAgentSpecHashAnnotation]).To(Equal(hash))
	})

	It("rolls up agent reports merged with operator-graded node conditions, and writes a new HealthReport only on transition", func() {
		name := types.NamespacedName{Name: "nh-rollup", Namespace: "default"}
		check := newNHC(name, headroom, fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckNodeCondition})
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })
		ensureNode(ctx, "node-a", healthyNodeConditions()...)
		ensureNode(ctx, "node-b", healthyNodeConditions()...)

		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeHealthDaemonSetStatus(ctx, check, 2, 2)
		writeNodeHealthReport(ctx, check, "node-a", nodeHealthPassing)
		writeNodeHealthReport(ctx, check, "node-b", nodeHealthPassing)

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		current := &fathomv1alpha1.NodeHealthCheck{}
		Expect(k8sClient.Get(ctx, name, current)).To(Succeed())
		Expect(current.Status.LastResult).To(Equal("Pass"))
		Expect(current.Status.Summary).To(Equal("2 of 2 node(s) passed"))
		Expect(current.Status.ReportingNodes).To(BeEquivalentTo(2))
		Expect(current.Status.NodeResults).To(HaveLen(2))
		Expect(current.Status.NodeResults[0].Node).To(Equal("node-a"))
		Expect(current.Status.NodeResults[1].Result).To(Equal("Pass"))
		Expect(apiMeta.FindStatusCondition(current.Status.Conditions, nodeHealthConditionReady).Status).To(Equal(metav1.ConditionTrue))
		Expect(apiMeta.FindStatusCondition(current.Status.Conditions, nodeHealthConditionCoverage).Status).To(Equal(metav1.ConditionTrue))
		Expect(nodeHealthHealthReportCount(ctx, name)).To(Equal(1))
		firstReport := current.Status.LastReportName

		report := &fathomv1alpha1.HealthReport{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: firstReport, Namespace: name.Namespace}, report)).To(Succeed())
		// 2 nodes × (1 headroom + 4 conditions) = 10 checks, all node_health on a Node target.
		Expect(report.Spec.Checks).To(HaveLen(10))
		var conditionChecks int
		for _, c := range report.Spec.Checks {
			Expect(c.Family).To(Equal(nodeHealthReportFamily))
			Expect(c.TargetRef.Kind).To(Equal("Node"))
			if c.Details["type"] == nodehealth.TypeNodeCondition {
				conditionChecks++
				Expect(c.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
			}
		}
		Expect(conditionChecks).To(Equal(8), "operator-graded node conditions reach the report")

		// Unchanged result: no new report, even after fresh rewrites.
		writeNodeHealthReport(ctx, check, "node-a", nodeHealthPassing)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(nodeHealthHealthReportCount(ctx, name)).To(Equal(1))

		// node-b develops memory pressure on the Node object itself: the
		// operator sees it without any new agent report, and the fold transitions.
		ensureNode(ctx, "node-b",
			corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			corev1.NodeCondition{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue, Reason: "KubeletHasInsufficientMemory"},
			corev1.NodeCondition{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
			corev1.NodeCondition{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse})
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, current)).To(Succeed())
		Expect(current.Status.LastResult).To(Equal("Fail"))
		Expect(current.Status.Summary).To(ContainSubstring("1 of 2 node(s) passed; worst: node-b NodeCondition MemoryPressure"))
		Expect(current.Status.Summary).To(ContainSubstring("KubeletHasInsufficientMemory"))
		Expect(current.Status.LastReportName).NotTo(Equal(firstReport))
		Expect(nodeHealthHealthReportCount(ctx, name)).To(Equal(2))
		Expect(current.Status.NodeResults[1].Result).To(Equal("Fail"))
		Expect(current.Status.NodeResults[1].Message).To(ContainSubstring("MemoryPressure"))
	})

	It("freezes the previous roll-up when a node report ages out (COR-3)", func() {
		name := types.NamespacedName{Name: "nh-stale", Namespace: "default"}
		check := newNHC(name, headroom)
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeHealthDaemonSetStatus(ctx, check, 2, 2)
		writeNodeHealthReport(ctx, check, "node-a", nodeHealthPassing)
		writeNodeHealthReport(ctx, check, "node-b", nodeHealthPassing)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		current := &fathomv1alpha1.NodeHealthCheck{}
		Expect(k8sClient.Get(ctx, name, current)).To(Succeed())
		Expect(current.Status.LastResult).To(Equal("Pass"))
		frozenRun, frozenReport, frozenResults := current.Status.LastRunTime, current.Status.LastReportName, current.Status.NodeResults

		// Freshness follows the (capped) agent cadence: a report older than
		// agentInterval+timeout is stale even though the roll-up interval is the
		// default 5m — and would still be stale under a 24h interval (#270).
		writeNodeHealthReportAt(ctx, check, "node-b", time.Now().Add(-2*nodeHealthReportMaxAge(check)), nodeHealthPassing)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.NodeHealthCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.ReportingNodes).To(BeEquivalentTo(1))
		Expect(updated.Status.LastResult).To(Equal("Pass"), "an incomplete window must freeze the verdict, not wipe it")
		Expect(updated.Status.LastReportName).To(Equal(frozenReport))
		Expect(updated.Status.LastRunTime.Time).To(Equal(frozenRun.Time), "lastRunTime must not move backward")
		Expect(updated.Status.NodeResults).To(Equal(frozenResults), "per-node results are frozen with the verdict")

		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeHealthConditionReady)
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("PartialReports"))
		coverage := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeHealthConditionCoverage)
		Expect(coverage.Status).To(Equal(metav1.ConditionFalse))
		Expect(coverage.Reason).To(Equal("PartialReports"))
		Expect(coverage.Message).To(ContainSubstring("node-b"))
	})

	It("does not let a departed node's report cover a newly joined node (COR-4)", func() {
		name := types.NamespacedName{Name: "nh-identity", Namespace: "default"}
		check := newNHC(name, headroom)
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeHealthDaemonSetStatus(ctx, check, 2, 2)
		writeNodeHealthReport(ctx, check, "node-a", nodeHealthPassing)
		writeNodeHealthReport(ctx, check, "node-b", nodeHealthPassing)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		// node-a departs, node-c joins: still 2 fresh reports for 2 desired nodes,
		// but node-c has never been evaluated.
		scheduleNodeHealthAgentPods(ctx, check, "node-b", "node-c")
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		updated := &fathomv1alpha1.NodeHealthCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(updated.Status.ReportingNodes).To(BeEquivalentTo(2))
		coverage := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeHealthConditionCoverage)
		Expect(coverage.Status).To(Equal(metav1.ConditionFalse), "node-c has never reported")
		Expect(coverage.Message).To(ContainSubstring("node-c"))
		Expect(updated.Status.LastResult).To(Equal("Pass"), "frozen, per COR-3")

		// node-c reports with a failing check while node-a's departed report is
		// still fresh and passing. Coverage closes, and the roll-up must reflect
		// exactly the fleet in scope: node-a shapes nothing.
		failing := []nodehealth.CheckResult{{Type: nodehealth.TypeDiskHeadroom, Path: "/var/lib/kubelet", Outcome: nodehealth.OutcomeFail, Summary: "3.0% of bytes free (at or below criticalPercentFree 10)"}}
		writeNodeHealthReport(ctx, check, "node-c", failing)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		Expect(apiMeta.FindStatusCondition(updated.Status.Conditions, nodeHealthConditionCoverage).Status).To(Equal(metav1.ConditionTrue))
		Expect(updated.Status.ReportingNodes).To(BeEquivalentTo(3), "the surplus report is still counted as reporting")
		Expect(updated.Status.LastResult).To(Equal("Fail"))
		Expect(updated.Status.Summary).To(Equal("1 of 2 node(s) passed; worst: node-c DiskHeadroom /var/lib/kubelet: 3.0% of bytes free (at or below criticalPercentFree 10)"))
		Expect(updated.Status.NodeResults).To(HaveLen(2), "a departed node must not appear in the roll-up")
		Expect(updated.Status.NodeResults[0].Node).To(Equal("node-b"))
		Expect(updated.Status.NodeResults[1].Node).To(Equal("node-c"))
		report := &fathomv1alpha1.HealthReport{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: updated.Status.LastReportName, Namespace: name.Namespace}, report)).To(Succeed())
		for _, c := range report.Spec.Checks {
			Expect(c.TargetRef.Name).NotTo(Equal("node-a"), "a departed node's evidence must not reach the HealthReport")
		}
	})

	It("rolls up a NodeCondition-only spec from empty agent reports", func() {
		name := types.NamespacedName{Name: "nh-condonly", Namespace: "default"}
		check := newNHC(name, fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckNodeCondition})
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })
		ensureNode(ctx, "node-a", healthyNodeConditions()...)

		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		// The agent has nothing to evaluate but must still be told so, with an
		// empty (never absent) item list.
		ds := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nh-condonly-node-health-agent", Namespace: "default"}, ds)).To(Succeed())
		args := ds.Spec.Template.Spec.Containers[0].Args
		Expect(args[indexOf(args, "--checks")+1]).To(Equal("[]"))
		Expect(ds.Spec.Template.Spec.Volumes).To(BeEmpty())

		setNodeHealthDaemonSetStatus(ctx, check, 1, 1)
		writeNodeHealthReport(ctx, check, "node-a", nil) // what the agent publishes: no checks, Skipped
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		current := &fathomv1alpha1.NodeHealthCheck{}
		Expect(k8sClient.Get(ctx, name, current)).To(Succeed())
		Expect(current.Status.LastResult).To(Equal("Pass"), "the operator-graded conditions alone decide the verdict")
		Expect(current.Status.Summary).To(Equal("1 of 1 node(s) passed"))
		Expect(apiMeta.FindStatusCondition(current.Status.Conditions, nodeHealthConditionReady).Status).To(Equal(metav1.ConditionTrue))
		report := &fathomv1alpha1.HealthReport{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: current.Status.LastReportName, Namespace: name.Namespace}, report)).To(Succeed())
		Expect(report.Spec.Checks).To(HaveLen(4), "one check per graded condition")
	})

	It("rejects a report whose payload node does not match its authenticated annotation, and says so (SEC-1)", func() {
		name := types.NamespacedName{Name: "nh-forged", Namespace: "default"}
		check := newNHC(name, headroom)
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeHealthDaemonSetStatus(ctx, check, 2, 2)
		writeNodeHealthReport(ctx, check, "node-a", nodeHealthPassing)
		// A writer that legitimately holds node-b's claim tries to publish a
		// failing verdict attributed to node-a, at node-a's canonical name.
		failing := []nodehealth.CheckResult{{Type: nodehealth.TypeDiskHeadroom, Path: "/var/lib/kubelet", Outcome: nodehealth.OutcomeFail, Summary: "0.1% of bytes free"}}
		writeNodeHealthReportObject(ctx, check, "node-a", "node-b", nodehealth.NodeReport{
			Node: "node-a", CheckName: check.Name, ObservedAt: time.Now(), Aggregate: nodehealth.OutcomeFail, Checks: failing,
		})

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		updated := &fathomv1alpha1.NodeHealthCheck{}
		Expect(k8sClient.Get(ctx, name, updated)).To(Succeed())
		authentic := apiMeta.FindStatusCondition(updated.Status.Conditions, nodeHealthConditionAuthentic)
		Expect(authentic).NotTo(BeNil())
		Expect(authentic.Status).To(Equal(metav1.ConditionFalse))
		Expect(authentic.Reason).To(Equal(eventReasonForgedReport))
		Expect(authentic.Message).To(ContainSubstring(string(nodecert.RejectNodeMismatch)))
		// The forged report was excluded: node-a's report is now the forged
		// one (same name), so node-a has no accepted report and nothing rolled
		// up to Fail.
		Expect(updated.Status.LastResult).NotTo(Equal("Fail"))
	})

	It("denies a node-report write from a principal with no node claim (SEC-1)", func() {
		name := types.NamespacedName{Name: "nh-noclaim", Namespace: "default"}
		check := newNHC(name, headroom)
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })
		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		encoded, err := nodehealth.EncodeReport(nodehealth.NodeReport{Node: "node-a", CheckName: check.Name, ObservedAt: time.Now(), Checks: nodeHealthPassing})
		Expect(err).NotTo(HaveOccurred())
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodehealth.ReportConfigMapName(check.Name, "node-a"), Namespace: check.Namespace,
				Labels:      map[string]string{nodecert.LabelManagedBy: nodecert.ManagedByValue, nodecert.LabelSourceKind: nodehealth.KindNodeHealthCheck, nodecert.LabelSourceName: check.Name},
				Annotations: map[string]string{nodecert.AnnotationNodeName: "node-a"},
			},
			Data: map[string]string{nodecert.ConfigMapReportKey: encoded},
		}
		// The envtest admin has no node claim: the shared authenticity policy
		// (one singleton for both node-scoped kinds) must deny it.
		err = k8sClient.Create(ctx, cm)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsForbidden(err)).To(BeTrue(), "expected the report-authenticity policy to deny: %v", err)
		Expect(err.Error()).To(ContainSubstring("node-name annotation must match"))
	})

	It("stamps the run-now token on the agent template and consumes it only once every node reports it", func() {
		name := types.NamespacedName{Name: "nh-runnow", Namespace: "default"}
		check := newNHC(name, headroom)
		check.Annotations = map[string]string{fathomv1alpha1.AnnotationRunNow: "tok-1"}
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })

		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		ds := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "nh-runnow-node-health-agent", Namespace: "default"}, ds)).To(Succeed())
		Expect(ds.Spec.Template.Annotations).To(HaveKeyWithValue(fathomv1alpha1.AnnotationRunNow, "tok-1"))
		var sawEnv bool
		for _, e := range ds.Spec.Template.Spec.Containers[0].Env {
			if e.Name == nodecert.EnvRunTrigger {
				sawEnv = true
			}
		}
		Expect(sawEnv).To(BeTrue())

		setNodeHealthDaemonSetStatus(ctx, check, 2, 2)
		writeTriggeredNodeHealthReport(ctx, check, "node-a", "tok-1", nodeHealthPassing)
		writeNodeHealthReport(ctx, check, "node-b", nodeHealthPassing) // no token yet
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		current := &fathomv1alpha1.NodeHealthCheck{}
		Expect(k8sClient.Get(ctx, name, current)).To(Succeed())
		Expect(current.Status.LastRunTrigger).To(BeEmpty(), "node-b has not answered with the token")
		Expect(current.Status.LastResult).To(Equal("Pass"), "the ordinary roll-up still runs")

		writeTriggeredNodeHealthReport(ctx, check, "node-b", "tok-1", nodeHealthPassing)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, current)).To(Succeed())
		Expect(current.Status.LastRunTrigger).To(Equal("tok-1"))
		Expect(current.Status.LastRunTime).NotTo(BeNil())
	})

	It("keeps its agent and reports apart from a NodeCertificateCheck of the same name", func() {
		name := types.NamespacedName{Name: "shared", Namespace: "default"}
		nhc := newNHC(name, headroom)
		Expect(k8sClient.Create(ctx, nhc)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, nhc))).To(Succeed()) })
		ncc := &fathomv1alpha1.NodeCertificateCheck{ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace}}
		Expect(k8sClient.Create(ctx, ncc)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ncc))).To(Succeed()) })

		_, err := newNodeHealthReconciler().Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		_, err = newNodeCertReconciler().Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())

		Expect(nodeHealthAgentResourceName(nhc)).NotTo(Equal(agentResourceName(ncc)))
		for _, dsName := range []string{nodeHealthAgentResourceName(nhc), agentResourceName(ncc)} {
			ds := &appsv1.DaemonSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: "default"}, ds)).To(Succeed())
		}
		Expect(nodehealth.ReportConfigMapName("shared", "node-a")).NotTo(Equal(nodecert.NodeReportConfigMapName("shared", "node-a")))

		// The two kinds' selectors are disjoint: neither DaemonSet nor
		// NetworkPolicy can ever select the other's pods, even for a shared name.
		nhDS, ncDS := &appsv1.DaemonSet{}, &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeHealthAgentResourceName(nhc), Namespace: "default"}, nhDS)).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: agentResourceName(ncc), Namespace: "default"}, ncDS)).To(Succeed())
		ncSelector := labels.SelectorFromSet(ncDS.Spec.Selector.MatchLabels)
		nhSelector := labels.SelectorFromSet(nhDS.Spec.Selector.MatchLabels)
		Expect(ncSelector.Matches(labels.Set(nhDS.Spec.Template.Labels))).To(BeFalse(), "certificate selector matches health-agent pods")
		Expect(nhSelector.Matches(labels.Set(ncDS.Spec.Template.Labels))).To(BeFalse(), "health selector matches certificate-agent pods")
		ncNP, nhNP := &networkingv1.NetworkPolicy{}, &networkingv1.NetworkPolicy{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: agentResourceName(ncc), Namespace: "default"}, ncNP)).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeHealthAgentResourceName(nhc), Namespace: "default"}, nhNP)).To(Succeed())
		Expect(labels.SelectorFromSet(ncNP.Spec.PodSelector.MatchLabels).Matches(labels.Set(nhDS.Spec.Template.Labels))).To(BeFalse(), "certificate NetworkPolicy isolates health-agent pods")
		Expect(labels.SelectorFromSet(nhNP.Spec.PodSelector.MatchLabels).Matches(labels.Set(ncDS.Spec.Template.Labels))).To(BeFalse(), "health NetworkPolicy isolates certificate-agent pods")

		// Health agent pods must not count toward the certificate check's coverage.
		scheduleNodeHealthAgentPods(ctx, nhc, "node-a")
		r := newNodeCertReconciler()
		expected, err := r.expectedAgentNodes(ctx, ncc, ncDS)
		Expect(err).NotTo(HaveOccurred())
		Expect(expected).To(BeEmpty(), "NodeCertificateCheck counted a NodeHealthCheck agent pod as its own")
	})

	It("ignores a planted pod that carries the agent labels but is not controlled by the DaemonSet", func() {
		name := types.NamespacedName{Name: "nh-planted", Namespace: "default"}
		check := newNHC(name, headroom)
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })
		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeHealthDaemonSetStatus(ctx, check, 1, 1)
		writeNodeHealthReport(ctx, check, "node-a", nodeHealthPassing)

		// A principal with pod create plants a labelled pod on a node that will
		// never report. Without the owner check this pins coverage incomplete.
		planted := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "planted", Namespace: name.Namespace, Labels: nodeHealthAgentSelectorLabels(check)},
			Spec:       corev1.PodSpec{NodeName: "node-z", Containers: []corev1.Container{{Name: "x", Image: "x"}}},
		}
		Expect(k8sClient.Create(ctx, planted)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, planted, client.GracePeriodSeconds(0)) })

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		current := &fathomv1alpha1.NodeHealthCheck{}
		Expect(k8sClient.Get(ctx, name, current)).To(Succeed())
		Expect(apiMeta.FindStatusCondition(current.Status.Conditions, nodeHealthConditionCoverage).Status).To(Equal(metav1.ConditionTrue), "a pod the DaemonSet does not control must not count as a node in scope")
		Expect(current.Status.LastResult).To(Equal("Pass"))
	})

	It("does not consume a fresh report that predates the current spec's agent items", func() {
		name := types.NamespacedName{Name: "nh-specchange", Namespace: "default"}
		check := newNHC(name, headroom)
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed()) })
		r := newNodeHealthReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeHealthDaemonSetStatus(ctx, check, 1, 1)
		writeNodeHealthReport(ctx, check, "node-a", nodeHealthPassing)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		current := &fathomv1alpha1.NodeHealthCheck{}
		Expect(k8sClient.Get(ctx, name, current)).To(Succeed())
		Expect(current.Status.LastResult).To(Equal("Pass"))
		frozenReport := current.Status.LastReportName

		// Add a check. The rollout completes, but node-a's still-fresh report
		// carries no result for the new item: it is a spec-change window, not
		// evidence, so the verdict stays frozen with an honest coverage signal.
		Expect(k8sClient.Get(ctx, name, check)).To(Succeed())
		check.Spec.Checks = append(check.Spec.Checks, fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckInodeHeadroom, Path: "/var/lib/kubelet"})
		Expect(k8sClient.Update(ctx, check)).To(Succeed())
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		setNodeHealthDaemonSetStatus(ctx, check, 1, 1)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, current)).To(Succeed())
		Expect(current.Status.ReportingNodes).To(BeEquivalentTo(0), "the pre-change report must not be consumed")
		coverage := apiMeta.FindStatusCondition(current.Status.Conditions, nodeHealthConditionCoverage)
		Expect(coverage.Status).To(Equal(metav1.ConditionFalse))
		Expect(coverage.Reason).To(Equal("PartialReports"))
		Expect(current.Status.LastReportName).To(Equal(frozenReport), "frozen, per COR-3")

		// The updated agent reports every item: coverage closes again.
		writeNodeHealthReport(ctx, check, "node-a", append(nodeHealthPassing,
			nodehealth.CheckResult{Type: nodehealth.TypeInodeHeadroom, Path: "/var/lib/kubelet", Outcome: nodehealth.OutcomePass, Summary: "90.0% of inodes free"}))
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, current)).To(Succeed())
		Expect(apiMeta.FindStatusCondition(current.Status.Conditions, nodeHealthConditionCoverage).Status).To(Equal(metav1.ConditionTrue))
	})
})

// indexOf returns the position of needle in args, or -1.
func indexOf(args []string, needle string) int {
	for i, a := range args {
		if strings.EqualFold(a, needle) {
			return i
		}
	}
	return -1
}
