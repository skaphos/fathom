/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/go-logr/logr"
	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/internal/nodehealth"
)

const (
	nodeHealthKind = nodehealth.KindNodeHealthCheck

	nodeHealthConditionAccepted   = checkConditionAccepted
	nodeHealthConditionAgentReady = nodeCertConditionAgentReady
	nodeHealthConditionReady      = checkConditionReady
	nodeHealthConditionCoverage   = nodeCertConditionCoverage
	nodeHealthConditionAuthentic  = nodeCertConditionAuthentic
	// nodeHealthConditionPrivileged makes the elevated posture a spec buys
	// visible on the object: KubeletHealthz puts the agent on the host network
	// and ContainerRuntime runs it as root. A reader should never have to
	// derive that from the DaemonSet (#206).
	nodeHealthConditionPrivileged = "AgentPrivileged"
)

// NodeHealthCheckReconciler reconciles a NodeHealthCheck object. It manages a
// node-agent DaemonSet (one pod per selected node) running in health mode,
// merges the per-node report ConfigMaps those agents publish with the node
// conditions it reads itself, rolls the result into a single HealthReport,
// and mirrors the aggregate into Status (#206).
//
// It is written against the corrected node-scoped shape from the v0.5.0
// adversarial review rather than a copy of the certificate controller: every
// ensure-failure path persists status (COR-2), an incomplete window freezes
// the verdict (COR-3), completeness is per node identity (COR-4), and report
// authenticity is bound to the writing identity (SEC-1). Report freshness
// follows the agent's capped cadence, not the roll-up interval (#270).
type NodeHealthCheckReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// NodeAgentImage is the dedicated node-agent container image
	// (cmd/node-agent), run here in --mode health.
	NodeAgentImage string

	// NodeAgentRoleName is the ClusterRole the per-check RoleBinding grants to
	// the node-agent ServiceAccount. Defaults to defaultNodeAgentRoleName; the
	// same runtime singleton NodeCertificateCheck converges.
	NodeAgentRoleName string

	// NodeReader reads Node objects for NodeCondition items. It MUST be an
	// uncached reader: nodes are read one at a time, only for the nodes agent
	// pods actually landed on, so the operator never starts a cluster-wide Node
	// informer and never needs list or watch on nodes — a `get` grant is the
	// whole surface. Nil falls back to Client, which is only appropriate in
	// tests with an uncached client.
	NodeReader client.Reader

	// Tracer creates the per-Reconcile span. Optional; a nil Tracer falls back
	// to the global provider (a no-op unless tracing is enabled).
	Tracer trace.Tracer

	// Recorder emits the Kubernetes Events contract on NodeHealthCheck
	// resources. Optional: nil disables event recording.
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=nodehealthchecks,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=nodehealthchecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=nodehealthchecks/finalizers,verbs=update
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthreports,verbs=create;get;list;watch;delete
// The node-agent infrastructure grants below are identical to the
// NodeCertificateCheck controller's: both kinds provision the same object
// shapes, and controller-gen merges identical rules. They are repeated here
// so this kind's needs are declared where its code is, not inherited from a
// sibling that could be removed.
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingadmissionpolicies;validatingadmissionpolicybindings,verbs=get;list;watch;create;update
// NodeCondition items are graded from the Node object. The operator reads
// exactly the nodes its agent pods landed on, one Get each through an
// uncached reader — never a list, never a watch — so a compromised operator
// token cannot enumerate the fleet through this grant (compare #255). This is
// the only cluster-scoped read on nodes the operator holds.
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get

// Reconcile ensures the node-agent DaemonSet and its RBAC exist, merges the
// per-node report ConfigMaps with the node conditions, rolls them into a
// HealthReport, and mirrors the aggregate into Status.
func (r *NodeHealthCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	ctx, span := reconcilerTracer(r.Tracer).Start(ctx, "nodehealthcheck.reconcile", trace.WithAttributes(
		attribute.String("fathom.kind", nodeHealthKind),
		attribute.String("fathom.namespace", req.Namespace),
		attribute.String("fathom.name", req.Name),
	))
	defer func() { endReconcileSpan(span, err) }()

	start := time.Now()
	defer func() {
		outcome := "success"
		if err != nil {
			outcome = "error"
		}
		metrics.RecordReconcile(nodeHealthKind, outcome, time.Since(start))
	}()

	log := logf.FromContext(ctx).WithValues("namespacedName", req.NamespacedName)

	var check fathomv1alpha1.NodeHealthCheck
	if err := r.Get(ctx, req.NamespacedName, &check); err != nil {
		if apierrors.IsNotFound(err) {
			metrics.DeleteCheckSeries(nodeHealthKind, req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	before := check.Status.DeepCopy()
	defer func() {
		observeCheck(r.Recorder, &check, nodeHealthKind,
			fathomv1alpha1.HealthReportResult(before.LastResult), fathomv1alpha1.HealthReportResult(check.Status.LastResult),
			before.Conditions, check.Status.Conditions,
			check.Status.LastRunTime, nodeHealthInterval(&check), err)
	}()
	check.Status.ObservedGeneration = check.Generation
	accepted := metav1.Condition{
		Type:               nodeHealthConditionAccepted,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "SpecAccepted",
		Message:            "NodeHealthCheck specification has been accepted for reconciliation.",
	}
	if msgs := cadenceClampMessages(check.Spec.Interval, check.Spec.Timeout); len(msgs) > 0 {
		accepted.Reason = conditionReasonSpecClamped
		accepted.Message = strings.Join(msgs, "; ") + "."
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, accepted)

	interval := nodeHealthInterval(&check)
	items := resolveNodeHealthItems(&check)

	if err := ensureNodeAgentClusterRole(ctx, r.Client, r.roleName()); err != nil {
		return r.failProvisioning(ctx, log, before, &check, "RBACProvisioningFailed", err)
	}
	if err := ensureReportAuthenticityPolicy(ctx, r.Client, log); err != nil {
		return r.failProvisioning(ctx, log, before, &check, "AdmissionPolicyProvisioningFailed", err)
	}
	saName, err := r.ensureAgentRBAC(ctx, &check)
	if err != nil {
		return r.failProvisioning(ctx, log, before, &check, "RBACProvisioningFailed", err)
	}
	// Converge the NetworkPolicy before the DaemonSet so agent pods never start
	// in a window where their metrics port is open cluster-wide (#153). For a
	// host-network agent the policy is inert — see setAgentPrivileged.
	if err := r.ensureAgentNetworkPolicy(ctx, &check, items); err != nil {
		return r.failProvisioning(ctx, log, before, &check, "NetworkPolicyProvisioningFailed", err)
	}
	ds, err := r.ensureDaemonSet(ctx, &check, saName, items)
	if err != nil {
		return r.failProvisioning(ctx, log, before, &check, "DaemonSetProvisioningFailed", err)
	}
	check.Status.DesiredNodes = ds.Status.DesiredNumberScheduled
	r.setAgentReady(&check, ds)
	r.setAgentPrivileged(&check, items)

	now := time.Now()
	reports, rejections, err := r.collectNodeReports(ctx, log, &check, nodeHealthAgentItems(items), now, nodeHealthReportMaxAge(&check))
	if err != nil {
		return ctrl.Result{}, err
	}
	check.Status.ReportingNodes = int32(len(reports))
	r.setReportsAuthentic(&check, rejections)

	expected, err := r.expectedAgentNodes(ctx, &check, ds)
	if err != nil {
		return ctrl.Result{}, err
	}

	evals, err := r.evaluateNodes(ctx, log, &check, reports)
	if err != nil {
		return ctrl.Result{}, err
	}
	reported := nodeHealthNodeNameSet(evals)

	token, triggerPending := runTriggerDue(check.Annotations, check.Status.LastRunTrigger)

	// An incomplete window — a rollout, a node joining, an agent restart —
	// freezes the last known verdict instead of clearing it (COR-3). The
	// CoverageComplete condition carries the "this verdict is frozen" signal.
	complete := nodeCertReportsComplete(ds, expected, reported)
	if complete {
		// Only nodes currently in scope contribute to the verdict. A departed
		// node's still-fresh report is tolerated for coverage (it neither
		// completes nor blocks it) but must not shape the verdict, summary,
		// nodeResults, or HealthReport of a fleet it has left.
		inScope := nodeHealthEvaluationsInScope(evals, expected)
		aggregate := aggregateNodeHealth(inScope)
		if err := r.rollup(ctx, log, &check, inScope, aggregate, interval); err != nil {
			return ctrl.Result{}, err
		}
	}
	r.setCoverage(&check, ds, expected, reported, complete)

	if triggerPending {
		consumeNodeHealthRunTrigger(&check, before, ds, expected, reports, token, time.Now())
	}

	r.setReadyFromState(&check, ds, expected, reported)
	return r.finish(ctx, log, before, &check, interval)
}

func (r *NodeHealthCheckReconciler) roleName() string {
	if r.NodeAgentRoleName == "" {
		return defaultNodeAgentRoleName
	}
	return r.NodeAgentRoleName
}

func (r *NodeHealthCheckReconciler) nodeReader() client.Reader {
	if r.NodeReader != nil {
		return r.NodeReader
	}
	return r.Client
}

// nodeHealthTemplateToken is the run-now token the agent template carries: the
// annotation when set (a new one rolls the agents), otherwise the last
// consumed token so removing the annotation is not itself a template change.
func nodeHealthTemplateToken(check *fathomv1alpha1.NodeHealthCheck) string {
	if token, _ := runTriggerDue(check.Annotations, check.Status.LastRunTrigger); token != "" {
		return token
	}
	return check.Status.LastRunTrigger
}

// consumeNodeHealthRunTrigger completes a pending on-demand run once every
// node in scope has a fresh report carrying the token — the same per-node-
// identity rule as the routine roll-up, so a departed node cannot complete a
// trigger on a live node's behalf. LastRunTime is refreshed when the rollup did
// not already move it, so a waiter sees a run time at or after its request
// even when the aggregate was unchanged.
func consumeNodeHealthRunTrigger(check *fathomv1alpha1.NodeHealthCheck, before *fathomv1alpha1.NodeHealthCheckStatus, ds *appsv1.DaemonSet, expected map[string]struct{}, reports []nodehealth.NodeReport, token string, now time.Time) {
	if !nodeCertReportsComplete(ds, expected, nodeHealthTriggeredNodeSet(reports, token)) {
		return
	}
	check.Status.LastRunTrigger = token
	moved := check.Status.LastRunTime != nil &&
		(before.LastRunTime == nil || check.Status.LastRunTime.After(before.LastRunTime.Time))
	if !moved {
		refreshed := metav1.NewTime(now)
		check.Status.LastRunTime = &refreshed
	}
}

// finish persists Status if it changed and requeues after interval.
func (r *NodeHealthCheckReconciler) finish(ctx context.Context, log logr.Logger, before *fathomv1alpha1.NodeHealthCheckStatus, check *fathomv1alpha1.NodeHealthCheck, interval time.Duration) (ctrl.Result, error) {
	if !equality.Semantic.DeepEqual(before, &check.Status) {
		if err := r.Status().Update(ctx, check); err != nil {
			return ctrl.Result{}, err
		}
		log.V(1).Info("updated NodeHealthCheck status")
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

// failProvisioning records a provisioning failure on Ready and persists it
// before returning the error (COR-2): a check whose provisioning has been
// failing for days must not keep advertising the Ready=True/Pass it last
// succeeded with. The verdict itself is not cleared — provisioning failing says
// nothing about what the last complete evaluation found.
func (r *NodeHealthCheckReconciler) failProvisioning(ctx context.Context, log logr.Logger, before *fathomv1alpha1.NodeHealthCheckStatus, check *fathomv1alpha1.NodeHealthCheck, reason string, cause error) (ctrl.Result, error) {
	r.setReady(check, metav1.ConditionFalse, reason, cause.Error())
	if !equality.Semantic.DeepEqual(before, &check.Status) {
		if err := r.Status().Update(ctx, check); err != nil {
			log.Error(err, "failed to persist NodeHealthCheck status after provisioning failure", "reason", reason)
		}
	}
	return ctrl.Result{}, cause
}

// ensureAgentRBAC provisions the per-check ServiceAccount and RoleBinding
// (both owner-referenced, in the check namespace) that grant the node-agent
// its least-privilege, namespaced ConfigMap access.
func (r *NodeHealthCheckReconciler) ensureAgentRBAC(ctx context.Context, check *fathomv1alpha1.NodeHealthCheck) (string, error) {
	name := nodeHealthAgentResourceName(check)
	labels := nodeHealthAgentLabels(check)

	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: check.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		sa.Labels = mergeLabels(sa.Labels, labels)
		return controllerutil.SetControllerReference(check, sa, r.Scheme)
	}); err != nil {
		return "", err
	}

	rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: check.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
		rb.Labels = mergeLabels(rb.Labels, labels)
		if rb.CreationTimestamp.IsZero() {
			rb.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: r.roleName()}
		}
		rb.Subjects = []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: name, Namespace: check.Namespace}}
		return controllerutil.SetControllerReference(check, rb, r.Scheme)
	}); err != nil {
		return "", err
	}
	return name, nil
}

// ensureAgentNetworkPolicy converges the per-check NetworkPolicy that isolates
// the agent pods (#153): metrics ingress only from namespaces labeled
// metrics=enabled, egress only to the API server. It is created
// unconditionally so the surface is uniform; on a host-network agent it is
// inert, which the AgentPrivileged condition says out loud.
func (r *NodeHealthCheckReconciler) ensureAgentNetworkPolicy(ctx context.Context, check *fathomv1alpha1.NodeHealthCheck, items []nodehealth.Item) error {
	tcp := corev1.ProtocolTCP
	metricsPort := intstr.FromInt32(nodeHealthMetricsPort(check, items))
	apiServerPort := intstr.FromInt32(443)
	apiServerEndpointPort := intstr.FromInt32(6443)

	np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: nodeHealthAgentResourceName(check), Namespace: check.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
		np.Labels = mergeLabels(np.Labels, nodeHealthAgentLabels(check))
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: nodeHealthAgentSelectorLabels(check)},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
						metricsNamespaceLabelKey: metricsNamespaceLabelValue,
					}},
				}},
				Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &metricsPort}},
			}},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &tcp, Port: &apiServerPort},
					{Protocol: &tcp, Port: &apiServerEndpointPort},
				},
			}},
		}
		return controllerutil.SetControllerReference(check, np, r.Scheme)
	})
	return err
}

// nodeHealthMetricsPort is the port the agent serves metrics on: the shared
// container port normally, a per-check host port when the agent shares the
// node's network namespace.
func nodeHealthMetricsPort(check *fathomv1alpha1.NodeHealthCheck, items []nodehealth.Item) int32 {
	if nodeHealthNeedsHostNetwork(items) {
		return nodeHealthHostMetricsPort(check)
	}
	return metricsContainerPort
}

// ensureDaemonSet converges the node-agent DaemonSet to the desired spec and
// returns the live object (with Status) for rollout bookkeeping. The template
// is rewritten only when the controller's own computed intent changes (the
// stored spec hash), so server-side defaulting never reads as drift and
// churns the agents into a perpetual rolling restart (#143 / SKA-589).
func (r *NodeHealthCheckReconciler) ensureDaemonSet(ctx context.Context, check *fathomv1alpha1.NodeHealthCheck, saName string, items []nodehealth.Item) (*appsv1.DaemonSet, error) {
	name := nodeHealthAgentResourceName(check)
	desired := r.desiredDaemonSet(check, saName, items)
	desiredHash := nodeAgentSpecHash(desired)

	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: check.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, ds, func() error {
		ds.Labels = mergeLabels(ds.Labels, desired.Labels)
		if ds.CreationTimestamp.IsZero() {
			ds.Spec.Selector = desired.Spec.Selector
		}
		if nodeAgentTemplateNeedsWrite(ds.Annotations[nodeAgentSpecHashAnnotation], desiredHash) {
			ds.Spec.Template = desired.Spec.Template
			if desiredHash != "" {
				if ds.Annotations == nil {
					ds.Annotations = make(map[string]string, 1)
				}
				ds.Annotations[nodeAgentSpecHashAnnotation] = desiredHash
			}
		}
		return controllerutil.SetControllerReference(check, ds, r.Scheme)
	}); err != nil {
		return nil, err
	}
	if err := r.Get(ctx, types.NamespacedName{Namespace: check.Namespace, Name: name}, ds); err != nil {
		return nil, err
	}
	return ds, nil
}

// desiredDaemonSet renders the agent pod template. The privilege each check
// type costs is granted only when an item of that type is present: hostPath
// mounts for the headroom paths, hostNetwork for KubeletHealthz, and the CRI
// socket plus root for ContainerRuntime. A spec with only headroom checks
// keeps exactly the hardened profile the certificate agent runs with.
func (r *NodeHealthCheckReconciler) desiredDaemonSet(check *fathomv1alpha1.NodeHealthCheck, saName string, items []nodehealth.Item) *appsv1.DaemonSet {
	labels := nodeHealthAgentLabels(check)
	agentItems := nodeHealthAgentItems(items)
	hostNetwork := nodeHealthNeedsHostNetwork(items)
	root := nodeHealthNeedsRoot(items)
	metricsPort := nodeHealthMetricsPort(check, items)

	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount
	dirType := corev1.HostPathDirectoryOrCreate
	for i, dir := range nodehealth.MountDirs(agentItems) {
		volName := "host-" + strconv.Itoa(i)
		volumes = append(volumes, corev1.Volume{
			Name:         volName,
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: dir, Type: &dirType}},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: volName, MountPath: dir, ReadOnly: true})
	}
	// hostPath type Socket: the kubelet refuses to start the pod when the
	// socket is absent, so a wrong socketPath surfaces as AgentReady=False and
	// incomplete coverage rather than as a verdict about a runtime that was
	// never reached.
	socketType := corev1.HostPathSocket
	for i, sock := range nodeHealthSocketPaths(agentItems) {
		volName := "cri-" + strconv.Itoa(i)
		volumes = append(volumes, corev1.Volume{
			Name:         volName,
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: sock, Type: &socketType}},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: volName, MountPath: sock, ReadOnly: true})
	}

	args := []string{
		"--mode", "health",
		"--check-name", check.Name,
		"--check-namespace", check.Namespace,
		"--checks", joinNodeHealthArgs(agentItems),
		"--interval", nodeHealthAgentInterval(check).String(),
		"--timeout", nodeHealthTimeout(check).String(),
		"--metrics-bind-address", ":" + strconv.Itoa(int(metricsPort)),
	}

	env := []corev1.EnvVar{{
		Name:      "NODE_NAME",
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "spec.nodeName"}},
	}}
	var templateAnnotations map[string]string
	if token := nodeHealthTemplateToken(check); token != "" {
		templateAnnotations = map[string]string{fathomv1alpha1.AnnotationRunNow: token}
		env = append(env, corev1.EnvVar{
			Name: nodecert.EnvRunTrigger,
			ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.annotations['" + fathomv1alpha1.AnnotationRunNow + "']",
			}},
		})
	}

	runAsNonRoot := !root
	runAsUser := int64(65532)
	if root {
		runAsUser = 0
	}
	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true
	automount := true
	graceperiod := int64(30)
	seccomp := corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
	dnsPolicy := corev1.DNSClusterFirst
	if hostNetwork {
		dnsPolicy = corev1.DNSClusterFirstWithHostNet
	}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHealthAgentResourceName(check), Namespace: check.Namespace, Labels: labels},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: nodeHealthAgentSelectorLabels(check)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: templateAnnotations},
				Spec: corev1.PodSpec{
					ServiceAccountName:            saName,
					AutomountServiceAccountToken:  &automount,
					NodeSelector:                  copyStringMap(check.Spec.NodeSelector),
					Tolerations:                   resolveNodeHealthTolerations(check),
					HostNetwork:                   hostNetwork,
					RestartPolicy:                 corev1.RestartPolicyAlways,
					DNSPolicy:                     dnsPolicy,
					SchedulerName:                 corev1.DefaultSchedulerName,
					TerminationGracePeriodSeconds: &graceperiod,
					SecurityContext:               &corev1.PodSecurityContext{RunAsNonRoot: &runAsNonRoot, RunAsUser: &runAsUser, SeccompProfile: &seccomp},
					Volumes:                       volumes,
					Containers: []corev1.Container{{
						Name:                     "node-agent",
						Image:                    r.NodeAgentImage,
						ImagePullPolicy:          corev1.PullIfNotPresent,
						Command:                  []string{"/node-agent"},
						Args:                     args,
						Env:                      env,
						Ports:                    []corev1.ContainerPort{{Name: "metrics", ContainerPort: metricsPort, Protocol: corev1.ProtocolTCP}},
						TerminationMessagePath:   "/dev/termination-log",
						TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &allowPrivilegeEscalation,
							ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
							RunAsNonRoot:             &runAsNonRoot,
							RunAsUser:                &runAsUser,
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
						},
						VolumeMounts: mounts,
					}},
				},
			},
		},
	}
}

// collectNodeReports lists fresh per-node report ConfigMaps for this check,
// decodes them, and keeps only reports that satisfy the shared authenticity
// bindings (SEC-1), the freshness bound, and the current spec (a report that
// does not carry a result for every agent-side item predates the template and
// is not consumed). Reports are keyed by node so duplicates cannot inflate
// coverage.
func (r *NodeHealthCheckReconciler) collectNodeReports(ctx context.Context, log logr.Logger, check *fathomv1alpha1.NodeHealthCheck, agentItems []nodehealth.Item, now time.Time, maxAge time.Duration) ([]nodehealth.NodeReport, []reportRejection, error) {
	var cms corev1.ConfigMapList
	if err := r.List(ctx, &cms,
		client.InNamespace(check.Namespace),
		client.MatchingLabels{
			nodecert.LabelManagedBy:  nodecert.ManagedByValue,
			nodecert.LabelSourceKind: nodeHealthKind,
			nodecert.LabelSourceName: check.Name,
		},
	); err != nil {
		return nil, nil, err
	}

	var rejections []reportRejection
	reportsByNode := make(map[string]nodehealth.NodeReport, len(cms.Items))
	for i := range cms.Items {
		cm := &cms.Items[i]
		raw, ok := cm.Data[nodecert.ConfigMapReportKey]
		if !ok {
			continue
		}
		report, err := nodehealth.DecodeReport(raw)
		if err != nil {
			log.Error(err, "skipping unparsable node health report ConfigMap", "configmap", cm.Name)
			continue
		}
		annotated := cm.Annotations[nodecert.AnnotationNodeName]
		if reason := nodehealth.VerifyReportBinding(cm.Name, annotated, check.Name, report); reason != nodecert.ReportAccepted {
			rejections = append(rejections, reportRejection{ConfigMap: cm.Name, Node: report.Node, Reason: reason})
			if reason.IndicatesForgery() {
				log.Info("rejected node health report that failed its authenticity bindings",
					"configmap", cm.Name, "reason", string(reason), "annotatedNode", annotated, "reportNode", report.Node)
			} else {
				log.V(1).Info("skipping node health report", "configmap", cm.Name, "reason", string(reason))
			}
			continue
		}
		if !nodeHealthReportFresh(report.ObservedAt, now, maxAge) {
			log.V(1).Info("skipping stale node health report", "configmap", cm.Name, "node", report.Node, "observedAt", report.ObservedAt, "maxAge", maxAge.String())
			continue
		}
		if !nodeHealthReportCoversSpec(report, agentItems) {
			log.V(1).Info("skipping node health report that predates the current spec", "configmap", cm.Name, "node", report.Node)
			continue
		}
		if existing, ok := reportsByNode[report.Node]; ok && !report.ObservedAt.After(existing.ObservedAt) {
			continue
		}
		r.adoptReportConfigMap(ctx, log, check, cm)
		reportsByNode[report.Node] = report
	}
	reports := make([]nodehealth.NodeReport, 0, len(reportsByNode))
	for _, report := range reportsByNode {
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Node < reports[j].Node })
	return reports, rejections, nil
}

// adoptReportConfigMap sets a controller owner reference on a report ConfigMap
// so it is garbage-collected with the check. Failures are non-fatal.
func (r *NodeHealthCheckReconciler) adoptReportConfigMap(ctx context.Context, log logr.Logger, check *fathomv1alpha1.NodeHealthCheck, cm *corev1.ConfigMap) {
	if metav1.IsControlledBy(cm, check) {
		return
	}
	patched := cm.DeepCopy()
	if err := controllerutil.SetControllerReference(check, patched, r.Scheme); err != nil {
		log.V(1).Info("cannot set owner reference on report ConfigMap", "configmap", cm.Name, "error", err.Error())
		return
	}
	if err := r.Update(ctx, patched); err != nil {
		log.V(1).Info("adopt report ConfigMap failed; will retry", "configmap", cm.Name, "error", err.Error())
	}
}

// expectedAgentNodes returns the identities of the nodes the agent DaemonSet
// is currently scheduled on, read from the agent pods themselves (the source of
// truth for scope without a cluster-wide Node read). Every agent pod carrying a
// node name counts, including one that is terminating, so coverage fails
// closed mid-rollout.
//
// Only pods the managed DaemonSet controls count. Labels are public: any
// principal with pod create in the namespace could otherwise plant a labelled
// pod naming a node that will never report and pin coverage incomplete — a
// frozen verdict — for as long as it likes.
func (r *NodeHealthCheckReconciler) expectedAgentNodes(ctx context.Context, check *fathomv1alpha1.NodeHealthCheck, ds *appsv1.DaemonSet) (map[string]struct{}, error) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods,
		client.InNamespace(check.Namespace),
		client.MatchingLabels(nodeHealthAgentSelectorLabels(check)),
	); err != nil {
		return nil, err
	}
	nodes := make(map[string]struct{}, len(pods.Items))
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !metav1.IsControlledBy(pod, ds) {
			continue
		}
		if node := pod.Spec.NodeName; node != "" {
			nodes[node] = struct{}{}
		}
	}
	return nodes, nil
}

// evaluateNodes merges each fresh agent report with the operator-graded node
// conditions. Nodes are read one at a time through the uncached reader, only
// for nodes that reported, so the read surface is exactly the fleet in scope.
// A node that cannot be read is an Error on that node's NodeCondition checks —
// the report still counts toward coverage, because the agent did its part.
func (r *NodeHealthCheckReconciler) evaluateNodes(ctx context.Context, log logr.Logger, check *fathomv1alpha1.NodeHealthCheck, reports []nodehealth.NodeReport) ([]nodeHealthEvaluation, error) {
	conditionTypes := nodeHealthConditionTypes(check)
	evals := make([]nodeHealthEvaluation, 0, len(reports))
	for _, report := range reports {
		var conditions []nodehealth.CheckResult
		if len(conditionTypes) > 0 {
			var node corev1.Node
			if err := r.nodeReader().Get(ctx, types.NamespacedName{Name: report.Node}, &node); err != nil {
				if !apierrors.IsNotFound(err) && !apierrors.IsForbidden(err) {
					return nil, err
				}
				// NotFound: the node left between the agent's scan and now.
				// Forbidden: the operator lacks the nodes/get grant on this
				// cluster (a restricted install). Both are graded, not hidden.
				log.V(1).Info("cannot read node for condition checks", "node", report.Node, "error", err.Error())
				for _, typ := range conditionTypes {
					conditions = append(conditions, nodehealth.CheckResult{
						Type: nodehealth.TypeNodeCondition, Path: typ, Outcome: nodehealth.OutcomeError,
						Summary: fmt.Sprintf("cannot read node: %v", err),
					})
				}
			} else {
				conditions = evaluateNodeConditions(&node, conditionTypes)
			}
		}
		evals = append(evals, mergeNodeHealthEvaluation(report, conditions))
	}
	return evals, nil
}

// rollup reconciles the aggregate into HealthReport history under the
// transition-only contract (#157): a new HealthReport only when the aggregate
// changes; otherwise LastRunTime is refreshed on the interval cadence. The
// per-node results and summary are refreshed on every complete evaluation so
// status reflects the latest measurements even when the fold is unchanged.
func (r *NodeHealthCheckReconciler) rollup(ctx context.Context, log logr.Logger, check *fathomv1alpha1.NodeHealthCheck, evals []nodeHealthEvaluation, aggregate fathomv1alpha1.HealthReportResult, interval time.Duration) error {
	now := time.Now()
	check.Status.NodeResults = nodeHealthNodeResults(evals)
	check.Status.Summary = nodeHealthSummary(evals, aggregate)

	switch decideNodeHealthRollup(&check.Status, string(aggregate), interval, now) {
	case rollupNoop:
		return nil
	case rollupRefreshLiveness:
		refreshed := metav1.NewTime(now)
		check.Status.LastRunTime = &refreshed
		return nil
	}

	observedAt := metav1.NewTime(now)
	report := healthReportForNodeHealth(check, evals, aggregate, observedAt)
	useDeterministicHealthReportName(report, check.Name,
		nodeHealthKind,
		string(check.UID),
		strconv.FormatInt(check.Generation, 10),
		check.Status.LastReportName,
		check.Status.LastResult,
		string(aggregate),
	)
	if r.Scheme != nil {
		if err := controllerutil.SetControllerReference(check, report, r.Scheme); err != nil {
			return err
		}
	}
	persistedReport, created, err := createOrReuseHealthReport(ctx, r.Client, report)
	if err != nil {
		return err
	}
	if created {
		pruneNodeHealthHealthReports(ctx, r.Client, log, check)
	}

	// The observable run time must never move backward (COR-3), even when a
	// deterministic HealthReport is reused with its original ObservedAt.
	if check.Status.LastRunTime == nil || persistedReport.Spec.ObservedAt.After(check.Status.LastRunTime.Time) {
		check.Status.LastRunTime = &persistedReport.Spec.ObservedAt
	}
	check.Status.LastReportName = persistedReport.Name
	check.Status.LastResult = string(persistedReport.Spec.Result)
	return nil
}

// setCoverage records whether the current verdict was computed from a
// complete evaluation of the fleet, so a frozen verdict is distinguishable
// from a fresh one.
func (r *NodeHealthCheckReconciler) setCoverage(check *fathomv1alpha1.NodeHealthCheck, ds *appsv1.DaemonSet, expected, reported map[string]struct{}, complete bool) {
	condition := metav1.Condition{
		Type:               nodeHealthConditionCoverage,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "AllNodesReporting",
		Message:            fmt.Sprintf("All %d node(s) in scope published a fresh evaluation.", len(expected)),
	}
	if !complete {
		condition.Status = metav1.ConditionFalse
		switch {
		case ds.Status.DesiredNumberScheduled == 0:
			condition.Reason = "NoMatchingNodes"
			condition.Message = "No nodes match the node-agent DaemonSet; nothing to evaluate."
		case !nodeAgentRolledOut(ds):
			condition.Reason = "AgentRollingOut"
			condition.Message = "Node-agent DaemonSet is still rolling out; the last known verdict is retained."
		default:
			condition.Reason = "PartialReports"
			condition.Message = fmt.Sprintf("%s have not published a fresh evaluation; the last known verdict is retained.",
				coverageGapSummary(expected, reported, ds.Status.DesiredNumberScheduled))
		}
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, condition)
}

func (r *NodeHealthCheckReconciler) setAgentReady(check *fathomv1alpha1.NodeHealthCheck, ds *appsv1.DaemonSet) {
	status := metav1.ConditionFalse
	reason := "RollingOut"
	message := "Node-agent DaemonSet is rolling out."
	switch {
	case ds.Status.DesiredNumberScheduled == 0:
		reason = "NoMatchingNodes"
		message = "No nodes match the node-agent DaemonSet; nothing to evaluate."
	case nodeAgentRolledOut(ds):
		status = metav1.ConditionTrue
		reason = "RolledOut"
		message = "Node-agent DaemonSet is ready on all selected nodes."
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               nodeHealthConditionAgentReady,
		Status:             status,
		ObservedGeneration: check.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// setAgentPrivileged records the elevated posture the resolved items require.
// True means the agent runs beyond the hardened default profile; the message
// says exactly how and why, so the cost of a check type is auditable on the
// object that asked for it.
func (r *NodeHealthCheckReconciler) setAgentPrivileged(check *fathomv1alpha1.NodeHealthCheck, items []nodehealth.Item) {
	hostNetwork := nodeHealthNeedsHostNetwork(items)
	root := nodeHealthNeedsRoot(items)
	condition := metav1.Condition{
		Type:               nodeHealthConditionPrivileged,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: check.Generation,
		Reason:             "Hardened",
		Message:            "Node-agent runs non-root with no host network; only read-only hostPath mounts for the headroom paths.",
	}
	switch {
	case hostNetwork && root:
		condition.Status = metav1.ConditionTrue
		condition.Reason = "HostNetworkAndRoot"
	case hostNetwork:
		condition.Status = metav1.ConditionTrue
		condition.Reason = "HostNetwork"
	case root:
		condition.Status = metav1.ConditionTrue
		condition.Reason = "RunAsRoot"
	}
	if condition.Status == metav1.ConditionTrue {
		condition.Message = "Node-agent runs with elevated privilege: " +
			nodeHealthPrivilegeSummary(hostNetwork, root, nodeHealthHostMetricsPort(check), nodeHealthSocketPaths(items)) + "."
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, condition)
}

func (r *NodeHealthCheckReconciler) setReadyFromState(check *fathomv1alpha1.NodeHealthCheck, ds *appsv1.DaemonSet, expected, reported map[string]struct{}) {
	switch {
	case ds.Status.DesiredNumberScheduled == 0:
		r.setReady(check, metav1.ConditionFalse, "NoMatchingNodes", "No nodes match the node-agent DaemonSet.")
	case len(reported) == 0:
		r.setReady(check, metav1.ConditionFalse, "AwaitingReports", "Waiting for node-agents to publish fresh evaluations.")
	case len(missingNodes(expected, reported)) > 0 || int32(len(expected)) < ds.Status.DesiredNumberScheduled:
		r.setReady(check, metav1.ConditionFalse, "PartialReports", "Waiting for every selected node-agent to publish a fresh evaluation.")
	case !nodeAgentRolledOut(ds):
		r.setReady(check, metav1.ConditionFalse, "AgentRollingOut", "Node-agent DaemonSet is still rolling out.")
	default:
		r.setReady(check, metav1.ConditionTrue, "Reporting", "Node-agents are reporting and a HealthReport was rolled up.")
	}
}

// setReportsAuthentic records whether any collected report failed its
// authenticity bindings, and raises a Warning event when one did (SEC-1).
func (r *NodeHealthCheckReconciler) setReportsAuthentic(check *fathomv1alpha1.NodeHealthCheck, rejections []reportRejection) {
	forged := forgeryRejections(rejections)
	if len(forged) == 0 {
		apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
			Type:               nodeHealthConditionAuthentic,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: check.Generation,
			Reason:             "AllReportsBound",
			Message:            "Every collected node report is bound to the node it claims.",
		})
		return
	}

	named := forged
	suffix := ""
	if len(named) > maxRejectionMessageReports {
		named = named[:maxRejectionMessageReports]
		suffix = fmt.Sprintf(" and %d more", len(forged)-maxRejectionMessageReports)
	}
	details := make([]string, 0, len(named))
	for _, rej := range named {
		details = append(details, fmt.Sprintf("%s (%s)", rej.ConfigMap, rej.Reason))
	}
	message := fmt.Sprintf("Rejected %d node report(s) that failed authenticity bindings: %s%s.",
		len(forged), strings.Join(details, ", "), suffix)

	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               nodeHealthConditionAuthentic,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: check.Generation,
		Reason:             eventReasonForgedReport,
		Message:            message,
	})
	if r.Recorder != nil {
		r.Recorder.Eventf(check, nil, corev1.EventTypeWarning, eventReasonForgedReport, eventActionEvaluate, "%s", message)
	}
}

func (r *NodeHealthCheckReconciler) setReady(check *fathomv1alpha1.NodeHealthCheck, status metav1.ConditionStatus, reason, message string) {
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               nodeHealthConditionReady,
		Status:             status,
		ObservedGeneration: check.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager wires the reconciler. It owns the DaemonSet, ServiceAccount,
// RoleBinding, and NetworkPolicy it creates, and watches report ConfigMaps by
// label so a fresh node report triggers a roll-up.
func (r *NodeHealthCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.NodeAgentRoleName == "" {
		r.NodeAgentRoleName = defaultNodeAgentRoleName
	}
	if r.NodeReader == nil {
		r.NodeReader = mgr.GetAPIReader()
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.NodeHealthCheck{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(checkForNodeHealthReportConfigMap),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Named("nodehealthcheck").
		Complete(r)
}

// checkForNodeHealthReportConfigMap maps a per-node report ConfigMap back to
// the NodeHealthCheck that owns it, using the source labels the agent writes.
func checkForNodeHealthReportConfigMap(_ context.Context, obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()
	if labels[nodecert.LabelManagedBy] != nodecert.ManagedByValue ||
		labels[nodecert.LabelSourceKind] != nodeHealthKind {
		return nil
	}
	name := labels[nodecert.LabelSourceName]
	if name == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: obj.GetNamespace(), Name: name}}}
}
