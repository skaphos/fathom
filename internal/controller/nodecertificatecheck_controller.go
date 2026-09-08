/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
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
)

const (
	nodeCertConditionAccepted   = "Accepted"
	nodeCertConditionPaused     = "Paused"
	nodeCertConditionAgentReady = "AgentReady"
	nodeCertConditionReady      = "Ready"
	// nodeCertConditionCoverage distinguishes "this verdict is current" from
	// "this verdict is the last one we could compute". An incomplete scan window
	// freezes LastResult, so without an explicit coverage signal a frozen verdict
	// is indistinguishable from a fresh one (COR-3).
	nodeCertConditionCoverage = "CoverageComplete"
	// nodeCertConditionAuthentic reports whether any collected report failed the
	// structural bindings that tie it to the node it claims. It is separate from
	// Ready on purpose: rejecting a forged report does not stop the legitimate
	// ones from covering the fleet, so the check can be Ready and still be under
	// attack (SEC-1).
	nodeCertConditionAuthentic = "ReportsAuthentic"

	// eventReasonForgedReport marks the Warning event raised when a report is
	// rejected for a reason only a writer passing off another node's report can
	// produce.
	eventReasonForgedReport = "ForgedReportRejected"

	// maxRejectionMessageReports bounds how many rejected ConfigMaps the
	// ReportsAuthentic condition names before it summarises.
	maxRejectionMessageReports = 5

	// maxCoverageMessageNodes bounds how many missing node names the coverage
	// condition names before it summarises, so a large fleet cannot push the
	// message toward the API server's condition-message limit.
	maxCoverageMessageNodes = 5

	defaultNodeCertWarnDays     = 30
	defaultNodeCertCriticalDays = 7

	// defaultNodeAgentRoleName is the static ClusterRole the per-check
	// RoleBinding grants to the node-agent ServiceAccount (namespaced ConfigMap
	// access only). It is shipped under config/rbac and the Helm chart.
	defaultNodeAgentRoleName = "fathom-node-agent-role"

	// reportAuthenticityPolicyName names the cluster-scoped
	// ValidatingAdmissionPolicy (and its binding) the controller ensures at
	// runtime to bind each per-node report ConfigMap to the writing node-agent's
	// identity, so one node cannot forge or suppress another node's certificate
	// verdict (#155). Like the node-agent ClusterRole it is created at runtime,
	// not shipped statically, so kustomize's namePrefix and the OLM bundle
	// transforms cannot rename it and break the policy↔binding pairing.
	reportAuthenticityPolicyName = "fathom-node-report-authenticity"

	// nodeAgentComponentLabel/Value tag the DaemonSet and its pods so the
	// DaemonSet selector is stable and pods are discoverable.
	nodeAgentComponentLabel = "fathom.skaphos.io/component"
	nodeAgentComponentValue = "node-agent"

	// nodeAgentSpecHashAnnotation records a hash of the pod template and selector
	// the controller last applied to the node-agent DaemonSet. ensureDaemonSet
	// rewrites Spec.Template only when this hash changes, so server-defaulted
	// fields on the live object never masquerade as drift and trigger an endless
	// rolling restart (SKA-589). nodeAgentSpecHashLength truncates the SHA-256 hex
	// digest to 32 characters (128 bits) — short enough for an annotation value
	// while staying collision-safe for this single-object use.
	nodeAgentSpecHashAnnotation = "fathom.skaphos.io/spec-hash"
	nodeAgentSpecHashLength     = 32

	// metricsNamespaceLabelKey/Value gate ingress to the node-agent metrics port
	// in the managed NetworkPolicy. The same namespace-label contract already
	// guards the operator's own metrics endpoint
	// (config/network-policy/allow-metrics-traffic.yaml): label the scraping
	// namespace `metrics: enabled` or, on a CNI that enforces NetworkPolicy, the
	// scrape is dropped.
	metricsNamespaceLabelKey   = "metrics"
	metricsNamespaceLabelValue = "enabled"

	// nodeCertReportFamily is the HealthReportCheck family for on-disk
	// certificate observations.
	nodeCertReportFamily = "node_certificate"

	nodeCertReportAdapterName = "node-certificate-check"
	nodeCertReportAdapterVer  = "0.1.0"

	metricsContainerPort = 8080
)

// NodeCertificateCheckReconciler reconciles a NodeCertificateCheck object. It
// manages a hardened, read-only node-agent DaemonSet (one pod per selected
// node), aggregates the per-node report ConfigMaps those agents publish into a
// single HealthReport, and mirrors the aggregate into Status (SKA-49 / SKA-519).
type NodeCertificateCheckReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// NodeAgentImage is the dedicated node-agent container image (cmd/node-agent),
	// distinct from the operator and probe images. Forwarded into the managed
	// DaemonSet's pod spec.
	NodeAgentImage string

	// NodeAgentRoleName is the ClusterRole the per-check RoleBinding grants to
	// the node-agent ServiceAccount. Defaults to defaultNodeAgentRoleName.
	NodeAgentRoleName string

	// Tracer creates the per-Reconcile span. Optional; a nil Tracer falls back
	// to the global provider (a no-op unless tracing is enabled).
	Tracer trace.Tracer

	// Recorder emits the Kubernetes Events contract (result transitions and
	// operational failures) on NodeCertificateCheck resources. Optional: nil
	// disables event recording; the check gauges are unaffected.
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=nodecertificatechecks,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=nodecertificatechecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=nodecertificatechecks/finalizers,verbs=update
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthreports,verbs=create;get;list;watch;delete
// All managed-resource writes below go through CreateOrUpdate (Update, never
// Patch), and only the DaemonSet is ever deleted directly (reconcilePaused);
// the ServiceAccount, RoleBinding, and NetworkPolicy are owner-referenced and
// removed by garbage collection, so those grants carry no patch/delete (#153).
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;delete
// The node-agent ServiceAccount needs create/get/update on its own report
// ConfigMap; the operator grants that via the runtime fathom-node-agent-role
// ClusterRole. RBAC escalation prevention requires the operator to already hold
// every verb it confers, so the manager must also hold create (not just
// get;list;watch;update) on configmaps.
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update
// The per-check node-agent NetworkPolicy (#153) is owner-referenced, so
// deletion rides garbage collection — no delete verb.
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update
// Coverage is computed against the nodes the agent pods actually landed on
// (expectedAgentNodes). Reading the agent's own pods is a namespaced list the
// operator already holds for the probe; resolving the DaemonSet's node selector
// instead would require a cluster-wide read on nodes, which this operator
// deliberately does not take (compare #255).
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list
// The controller ensures a cluster-scoped ValidatingAdmissionPolicy + binding
// that authenticate per-node report ConfigMaps (#155). Creating a VAP confers no
// privilege of its own, so this grant does not trip the RBAC escalation check.
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingadmissionpolicies;validatingadmissionpolicybindings,verbs=get;list;watch;create;update

// Reconcile ensures the node-agent DaemonSet and its RBAC exist (or are removed
// while paused), rolls up the per-node report ConfigMaps into a HealthReport,
// and mirrors the aggregate into Status.
func (r *NodeCertificateCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	ctx, span := reconcilerTracer(r.Tracer).Start(ctx, "nodecertificatecheck.reconcile", trace.WithAttributes(
		attribute.String("fathom.kind", "NodeCertificateCheck"),
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
		metrics.RecordReconcile("NodeCertificateCheck", outcome, time.Since(start))
	}()

	log := logf.FromContext(ctx).WithValues("namespacedName", req.NamespacedName)

	var check fathomv1alpha1.NodeCertificateCheck
	if err := r.Get(ctx, req.NamespacedName, &check); err != nil {
		if apierrors.IsNotFound(err) {
			metrics.DeleteCheckSeries("NodeCertificateCheck", req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	before := check.Status.DeepCopy()
	defer func() {
		observeCheck(r.Recorder, &check, "NodeCertificateCheck",
			fathomv1alpha1.HealthReportResult(before.LastResult), fathomv1alpha1.HealthReportResult(check.Status.LastResult),
			before.Conditions, check.Status.Conditions,
			check.Status.LastRunTime, nodeCertInterval(&check), err)
	}()
	check.Status.ObservedGeneration = check.Generation
	accepted := metav1.Condition{
		Type:               nodeCertConditionAccepted,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "SpecAccepted",
		Message:            "NodeCertificateCheck specification has been accepted for reconciliation.",
	}
	// A stored sub-floor cadence (pre-floor object) runs clamped, not rejected:
	// the cadence helpers raise it to the api/v1alpha1 floors, and the Accepted
	// condition says so (observeCheck turns the transition into a Warning event).
	if msgs := cadenceClampMessages(check.Spec.Interval, check.Spec.Timeout); len(msgs) > 0 {
		accepted.Reason = conditionReasonSpecClamped
		accepted.Message = strings.Join(msgs, "; ") + "."
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, accepted)

	interval := nodeCertInterval(&check)

	if check.Spec.Paused {
		if err := r.reconcilePaused(ctx, &check); err != nil {
			return ctrl.Result{}, err
		}
		return r.finish(ctx, log, before, &check, 0)
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               nodeCertConditionPaused,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: check.Generation,
		Reason:             "RunEnabled",
		Message:            "NodeCertificateCheck is eligible for node-agent execution.",
	})

	if err := r.ensureNodeAgentClusterRole(ctx); err != nil {
		return r.failProvisioning(ctx, log, before, &check, "RBACProvisioningFailed", err)
	}

	if err := r.ensureReportAuthenticityPolicy(ctx, log); err != nil {
		return r.failProvisioning(ctx, log, before, &check, "AdmissionPolicyProvisioningFailed", err)
	}

	saName, err := r.ensureAgentRBAC(ctx, &check)
	if err != nil {
		return r.failProvisioning(ctx, log, before, &check, "RBACProvisioningFailed", err)
	}

	// Converge the NetworkPolicy before the DaemonSet so agent pods never start
	// in a window where their metrics port is open cluster-wide (#153).
	if err := r.ensureAgentNetworkPolicy(ctx, &check); err != nil {
		return r.failProvisioning(ctx, log, before, &check, "NetworkPolicyProvisioningFailed", err)
	}

	ds, err := r.ensureDaemonSet(ctx, &check, saName)
	if err != nil {
		return r.failProvisioning(ctx, log, before, &check, "DaemonSetProvisioningFailed", err)
	}
	check.Status.DesiredNodes = ds.Status.DesiredNumberScheduled
	r.setAgentReady(&check, ds)

	reports, rejections, err := r.collectNodeReports(ctx, log, &check, time.Now(), nodeCertReportMaxAge(&check))
	if err != nil {
		return ctrl.Result{}, err
	}
	check.Status.ReportingNodes = int32(len(reports))
	r.setReportsAuthentic(&check, rejections)

	expected, err := r.expectedAgentNodes(ctx, &check)
	if err != nil {
		return ctrl.Result{}, err
	}
	reported := nodeNameSet(reports)

	token, triggerPending := runTriggerDue(check.Annotations, check.Status.LastRunTrigger)

	// An incomplete window — a DaemonSet rollout, a node joining, an agent
	// restart — freezes the last known verdict instead of clearing it. A gap in
	// reporting is not evidence that the fleet changed, and clearing it churned
	// every mirroring HealthCheck and ClusterHealth through Unknown and moved
	// lastRunTime backward (COR-3). The freeze is what status.lastRunTrigger
	// already promised for the pending-trigger case; it now holds unconditionally,
	// with CoverageComplete carrying the "this verdict is frozen" signal.
	complete := nodeCertReportsComplete(ds, expected, reported)
	if complete {
		aggregate := aggregateNodeReports(reports)
		if err := r.rollup(ctx, log, &check, reports, aggregate, interval); err != nil {
			return ctrl.Result{}, err
		}
	}
	r.setCoverage(&check, ds, expected, reported, complete)

	if triggerPending {
		consumeNodeCertRunTrigger(&check, before, ds, expected, reports, token, time.Now())
	}

	r.setReadyFromState(&check, ds, expected, reported)
	return r.finish(ctx, log, before, &check, interval)
}

// nodeCertTemplateToken is the run-now token the agent template carries: the
// annotation when set (a new one rolls the agents), otherwise the last
// consumed token so removing the annotation is not itself a template change.
func nodeCertTemplateToken(check *fathomv1alpha1.NodeCertificateCheck) string {
	if token, _ := runTriggerDue(check.Annotations, check.Status.LastRunTrigger); token != "" {
		return token
	}
	return check.Status.LastRunTrigger
}

// consumeNodeCertRunTrigger completes a pending on-demand run: the token is
// recorded as consumed only when every desired node has a fresh report that
// carries it, which is the proof that every agent restarted with the token and
// scanned. Reports with an empty or different trigger never count, so an agent
// predating the field cannot complete a wait falsely. While the set is
// incomplete nothing changes: the ordinary rollup above keeps the verdict live
// and the pending token stays pending.
//
// A forced run that leaves the aggregate unchanged would otherwise be a rollup
// no-op, so LastRunTime is refreshed when the rollup did not move it; a waiter
// then sees both the consumed token and a run time at or after its request.
func consumeNodeCertRunTrigger(check *fathomv1alpha1.NodeCertificateCheck, before *fathomv1alpha1.NodeCertificateCheckStatus, ds *appsv1.DaemonSet, expected map[string]struct{}, reports []nodecert.NodeReport, token string, now time.Time) {
	// Per-node-identity, exactly like the routine roll-up: a trigger completes
	// only once every node in scope has answered with this token, so a departed
	// node's report can never stand in for a live node that has not run yet.
	if !nodeCertReportsComplete(ds, expected, triggeredNodeSet(reports, token)) {
		return
	}
	check.Status.LastRunTrigger = token
	// Refresh unless the rollup above already moved LastRunTime forward, so a
	// waiter always sees a run time at or after its request even when the
	// aggregate was unchanged or a deterministic report was reused.
	moved := check.Status.LastRunTime != nil &&
		(before.LastRunTime == nil || check.Status.LastRunTime.After(before.LastRunTime.Time))
	if !moved {
		refreshed := metav1.NewTime(now)
		check.Status.LastRunTime = &refreshed
	}
}

// finish persists Status if it changed and requeues after interval (0 disables
// the periodic requeue, used while paused).
func (r *NodeCertificateCheckReconciler) finish(ctx context.Context, log logr.Logger, before *fathomv1alpha1.NodeCertificateCheckStatus, check *fathomv1alpha1.NodeCertificateCheck, interval time.Duration) (ctrl.Result, error) {
	if !equality.Semantic.DeepEqual(before, &check.Status) {
		if err := r.Status().Update(ctx, check); err != nil {
			return ctrl.Result{}, err
		}
		log.V(1).Info("updated NodeCertificateCheck status")
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

func (r *NodeCertificateCheckReconciler) reconcilePaused(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck) error {
	// Stop scanning: delete the agent DaemonSet. RBAC and report ConfigMaps are
	// owner-referenced and harmless while idle, so they are left in place; the
	// most recent Status snapshot is preserved.
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: agentResourceName(check), Namespace: check.Namespace}}
	if err := r.Delete(ctx, ds); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               nodeCertConditionPaused,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "Paused",
		Message:            "NodeCertificateCheck is paused; the node-agent DaemonSet has been removed.",
	})
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               nodeCertConditionAgentReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: check.Generation,
		Reason:             "Paused",
		Message:            "Node-agent DaemonSet is not running while paused.",
	})
	r.setReady(check, metav1.ConditionFalse, "Paused", "NodeCertificateCheck is paused.")
	check.Status.DesiredNodes = 0
	return nil
}

func (r *NodeCertificateCheckReconciler) roleName() string {
	if r.NodeAgentRoleName == "" {
		return defaultNodeAgentRoleName
	}
	return r.NodeAgentRoleName
}

// ensureNodeAgentClusterRole guarantees the ClusterRole the per-check
// RoleBinding references exists with the exact name the controller uses. It is
// created at runtime (rather than shipped statically) so the name stays stable
// across deploy tooling — kustomize's namePrefix and OLM bundle transforms would
// otherwise rename a static ClusterRole and break the binding. The role grants
// only namespaced ConfigMap access (the verbs never apply cluster-wide because
// they are only ever bound via the per-check RoleBinding). The operator already
// holds these ConfigMap verbs, so creating the role does not escalate privilege.
func (r *NodeCertificateCheckReconciler) ensureNodeAgentClusterRole(ctx context.Context) error {
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: r.roleName()}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		role.Labels = mergeLabels(role.Labels, map[string]string{nodecert.LabelManagedBy: nodecert.ManagedByValue})
		// Exactly the verbs the node-agent uses on its own report ConfigMap
		// (Get, Create, Update — see cmd/node-agent upsertReportConfigMap). No
		// list/watch/patch: the agent runs on every node and must not be able to
		// enumerate or tamper with other ConfigMaps in the namespace.
		role.Rules = []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"create", "get", "update"},
		}}
		return nil
	})
	return err
}

// ensureReportAuthenticityPolicy converges the cluster-scoped
// ValidatingAdmissionPolicy and binding that authenticate per-node report
// ConfigMaps. The policy only inspects Fathom node-report ConfigMaps written by a
// node-agent ServiceAccount ("<check>-node-agent"), and requires the report's
// node-name annotation to equal that writer's ServiceAccount-token node claim
// (authentication.kubernetes.io/node-name). A node-agent token therefore can
// only publish a report attributed to its own node, closing the report-spoofing
// gap where the shared, namespace-wide ConfigMap write let one node forge or
// suppress another node's verdict (#155). It fails closed. On a cluster that does
// not serve ValidatingAdmissionPolicy the ensure is skipped — the operator's
// collect-time cross-check in collectNodeReports still rejects mismatched reports.
func (r *NodeCertificateCheckReconciler) ensureReportAuthenticityPolicy(ctx context.Context, log logr.Logger) error {
	policy := &admissionregistrationv1.ValidatingAdmissionPolicy{ObjectMeta: metav1.ObjectMeta{Name: reportAuthenticityPolicyName}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, policy, func() error {
		policy.Labels = mergeLabels(policy.Labels, map[string]string{nodecert.LabelManagedBy: nodecert.ManagedByValue})
		policy.Spec = reportAuthenticityPolicySpec()
		return nil
	}); err != nil {
		if admissionPolicyUnsupported(err) {
			// Security-significant degradation: without the policy, a compromised
			// node-agent token can forge or suppress another node's report. Log at
			// the default level (not V(1)) so it is visible in normal operator logs.
			log.Info("ValidatingAdmissionPolicy is not served by this cluster: node-report authenticity enforcement is DISABLED; only the controller's collect-time consistency check applies", "error", err.Error())
			return nil
		}
		return err
	}

	binding := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{ObjectMeta: metav1.ObjectMeta{Name: reportAuthenticityPolicyName}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, binding, func() error {
		binding.Labels = mergeLabels(binding.Labels, map[string]string{nodecert.LabelManagedBy: nodecert.ManagedByValue})
		binding.Spec = admissionregistrationv1.ValidatingAdmissionPolicyBindingSpec{
			PolicyName:        reportAuthenticityPolicyName,
			ValidationActions: []admissionregistrationv1.ValidationAction{admissionregistrationv1.Deny},
		}
		return nil
	}); err != nil {
		if admissionPolicyUnsupported(err) {
			log.Info("ValidatingAdmissionPolicyBinding is not served by this cluster: node-report authenticity enforcement is DISABLED; only the controller's collect-time consistency check applies", "error", err.Error())
			return nil
		}
		return err
	}
	return nil
}

// admissionPolicyUnsupported reports whether err means the API server does not
// serve the ValidatingAdmissionPolicy types (feature-gate disabled or a very old
// cluster), in which case the controller degrades gracefully rather than wedging
// every reconcile. Only a NoMatch error — the REST mapper cannot resolve the kind
// — qualifies. A missing scheme registration or a bare NotFound is a
// programming/configuration error, not an unsupported cluster, so it is
// deliberately NOT swallowed here and instead fails the reconcile loudly.
func admissionPolicyUnsupported(err error) bool {
	return apiMeta.IsNoMatchError(err)
}

// reportAuthenticityPolicySpec is the CEL policy that binds a report ConfigMap to
// the node its writer actually runs on. The ObjectSelector narrows evaluation to
// Fathom node-report ConfigMaps so the policy never fires on unrelated ConfigMap
// writes.
//
// This is the authentication boundary for node reports: Kubernetes does not
// record the writer on the stored object, so nothing downstream can re-derive
// who wrote a report. Everything the controller checks at collect time
// (nodecert.VerifyReportBinding) is corroboration layered on top.
func reportAuthenticityPolicySpec() admissionregistrationv1.ValidatingAdmissionPolicySpec {
	fail := admissionregistrationv1.Fail
	forbidden := metav1.StatusReasonForbidden
	return admissionregistrationv1.ValidatingAdmissionPolicySpec{
		FailurePolicy: &fail,
		MatchConstraints: &admissionregistrationv1.MatchResources{
			// Selected by managed-by alone, not by source-kind: every node-scoped
			// kind writes reports through this same wire contract, so one policy
			// protects them all and NodeHealthCheck (#206) inherits the boundary
			// instead of provisioning a second, drifting copy.
			ObjectSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				nodecert.LabelManagedBy: nodecert.ManagedByValue,
			}},
			ResourceRules: []admissionregistrationv1.NamedRuleWithOperations{{
				RuleWithOperations: admissionregistrationv1.RuleWithOperations{
					Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
					Rule: admissionregistrationv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"configmaps"},
					},
				},
			}},
		},
		// No MatchConditions. A MatchCondition that evaluates false makes the API
		// server skip the policy entirely, so the previous `writer-is-node-agent`
		// name filter exempted every principal whose ServiceAccount did not end in
		// `-node-agent` — i.e. exactly the attacker (SEC-1). Name-matching in a VAP
		// is a hint, never an authentication boundary. The policy now applies to
		// every writer of a node-report ConfigMap.
		//
		// The filter existed so the operator's own adoptReportConfigMap Update —
		// made as the operator ServiceAccount, which has no node claim — could set
		// an owner reference. That is preserved without an identity carve-out:
		// contentUnchanged permits any update that leaves the report payload and
		// its node-name annotation byte-identical, which is all adoption does.
		// Nothing that *writes* a report can take that path.
		Variables: []admissionregistrationv1.Variable{
			{
				Name:       "claimNode",
				Expression: `request.userInfo.extra[?'authentication.kubernetes.io/node-name'].orValue([''])[0]`,
			},
			{
				Name:       "annotatedNode",
				Expression: `has(object.metadata.annotations) ? object.metadata.annotations[?'` + nodecert.AnnotationNodeName + `'].orValue('') : ''`,
			},
			{
				Name:       "oldAnnotatedNode",
				Expression: `(request.operation == 'UPDATE' && oldObject != null && has(oldObject.metadata.annotations)) ? oldObject.metadata.annotations[?'` + nodecert.AnnotationNodeName + `'].orValue('') : ''`,
			},
			{
				Name:       "reportData",
				Expression: `has(object.data) ? object.data[?'` + nodecert.ConfigMapReportKey + `'].orValue('') : ''`,
			},
			{
				Name:       "oldReportData",
				Expression: `(request.operation == 'UPDATE' && oldObject != null && has(oldObject.data)) ? oldObject.data[?'` + nodecert.ConfigMapReportKey + `'].orValue('') : ''`,
			},
			{
				Name:       "contentUnchanged",
				Expression: `request.operation == 'UPDATE' && oldObject != null && variables.annotatedNode == variables.oldAnnotatedNode && variables.reportData == variables.oldReportData`,
			},
		},
		Validations: []admissionregistrationv1.Validation{{
			Expression: `variables.contentUnchanged || (variables.claimNode != '' && variables.annotatedNode == variables.claimNode)`,
			Message:    "a node-report ConfigMap's fathom.skaphos.io/node-name annotation must match the writing identity's ServiceAccount-token node claim (authentication.kubernetes.io/node-name); only metadata-only updates that leave the report payload and annotation unchanged are exempt",
			Reason:     &forbidden,
		}},
	}
}

// ensureAgentRBAC provisions the per-check ServiceAccount and RoleBinding (both
// owner-referenced, in the check namespace) that grant the node-agent its
// least-privilege, namespaced ConfigMap access. It returns the ServiceAccount name.
func (r *NodeCertificateCheckReconciler) ensureAgentRBAC(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck) (string, error) {
	name := agentResourceName(check)
	labels := agentLabels(check)

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
			// RoleRef is immutable: set it only on create.
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
// the node-agent pods (#153). Ingress: only the metrics port, and only from
// namespaces labeled metrics=enabled — the same label contract that guards the
// operator's own metrics endpoint — so the unauthenticated plaintext
// cert-inventory gauges are not scrapeable from every pod on every node.
// Egress: only the API server ports; the agent talks to nothing else (it
// reaches the API server via KUBERNETES_SERVICE_HOST, an IP, so it needs no
// DNS egress either). Owner-referenced, so it is garbage-collected with the
// check; like the agent RBAC it is deliberately left in place while paused.
// Enforcement requires a NetworkPolicy-capable CNI — on clusters without one
// this object is inert, which is also why creating it is safe unconditionally.
func (r *NodeCertificateCheckReconciler) ensureAgentNetworkPolicy(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck) error {
	tcp := corev1.ProtocolTCP
	metricsPort := intstr.FromInt32(metricsContainerPort)
	// The API server is reached through the kubernetes Service ClusterIP
	// (usually port 443) which most CNIs police post-DNAT against the endpoint
	// port (usually 6443), so both must be allowed. Clusters serving the API on
	// a nonstandard port need an additional operator-authored allowance; see
	// docs/reference/network-policies.md.
	apiServerPort := intstr.FromInt32(443)
	apiServerEndpointPort := intstr.FromInt32(6443)

	np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: agentResourceName(check), Namespace: check.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
		np.Labels = mergeLabels(np.Labels, agentLabels(check))
		np.Spec = networkingv1.NetworkPolicySpec{
			// Same selector as the DaemonSet: only agent pods are isolated,
			// never anything else running in the check's namespace.
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{
				nodecert.LabelSourceName: check.Name,
				nodeAgentComponentLabel:  nodeAgentComponentValue,
			}},
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

// ensureDaemonSet converges the node-agent DaemonSet to the desired spec and
// returns the live object (with Status) for rollout bookkeeping.
func (r *NodeCertificateCheckReconciler) ensureDaemonSet(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck, saName string) (*appsv1.DaemonSet, error) {
	name := agentResourceName(check)
	desired := r.desiredDaemonSet(check, saName)
	desiredHash := nodeAgentSpecHash(desired)

	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: check.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, ds, func() error {
		ds.Labels = mergeLabels(ds.Labels, desired.Labels)
		if ds.CreationTimestamp.IsZero() {
			// Selector is immutable: set it only on create.
			ds.Spec.Selector = desired.Spec.Selector
		}
		// Rewrite the pod template only when our own computed intent changes.
		// Comparing a stored hash of the desired template (rather than the live
		// object) keeps API-server defaulting from looking like drift, which
		// would otherwise churn the DaemonSet into a perpetual rolling restart
		// every time an agent rewrites its report ConfigMap (SKA-589). An empty
		// desiredHash (the impossible marshal failure) forces the write so a
		// create still produces a valid template, and we never stamp the empty
		// hash — a later reconcile then recomputes and converges.
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
	// Refresh to read DaemonSet Status (CreateOrUpdate leaves it stale on create).
	if err := r.Get(ctx, types.NamespacedName{Namespace: check.Namespace, Name: name}, ds); err != nil {
		return nil, err
	}
	return ds, nil
}

// nodeAgentTemplateNeedsWrite reports whether ensureDaemonSet must (re)write the
// pod template. It writes when the stored hash differs from the desired hash, and
// always when desiredHash is empty: an empty hash means the digest could not be
// computed, so skipping the write would (on create) leave an invalid, empty pod
// template. The caller must not stamp an empty hash, so a later reconcile
// recomputes and converges.
func nodeAgentTemplateNeedsWrite(storedHash, desiredHash string) bool {
	return desiredHash == "" || storedHash != desiredHash
}

// nodeAgentSpecHash returns a stable digest of the parts of the DaemonSet the
// controller authors: the pod template and the selector. ensureDaemonSet stores
// it as an annotation and only rewrites the template when the digest changes, so
// the update decision is driven by our intent rather than server-side defaulting.
func nodeAgentSpecHash(desired *appsv1.DaemonSet) string {
	payload := struct {
		Template corev1.PodTemplateSpec `json:"template"`
		Selector *metav1.LabelSelector  `json:"selector"`
	}{
		Template: desired.Spec.Template,
		Selector: desired.Spec.Selector,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		// A PodTemplateSpec always marshals; on the impossible error, return an
		// empty hash so the template is rewritten rather than silently pinned.
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:nodeAgentSpecHashLength]
}

func (r *NodeCertificateCheckReconciler) desiredDaemonSet(check *fathomv1alpha1.NodeCertificateCheck, saName string) *appsv1.DaemonSet {
	labels := agentLabels(check)
	selectorLabels := map[string]string{
		nodecert.LabelSourceName: check.Name,
		nodeAgentComponentLabel:  nodeAgentComponentValue,
	}

	paths := resolveCertPaths(check)
	mountDirs := nodecert.MinimalMountDirs(paths)

	volumes := make([]corev1.Volume, 0, len(mountDirs))
	mounts := make([]corev1.VolumeMount, 0, len(mountDirs))
	hostPathType := corev1.HostPathDirectoryOrCreate
	for i, dir := range mountDirs {
		volName := "host-" + strconv.Itoa(i)
		volumes = append(volumes, corev1.Volume{
			Name:         volName,
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: dir, Type: &hostPathType}},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: volName, MountPath: dir, ReadOnly: true})
	}

	warnDays, criticalDays := resolveThresholds(check)
	args := []string{
		"--check-name", check.Name,
		"--check-namespace", check.Namespace,
		"--paths", joinPaths(paths),
		"--warn-days", strconv.Itoa(warnDays),
		"--critical-days", strconv.Itoa(criticalDays),
		"--interval", nodeCertInterval(check).String(),
		"--timeout", nodeCertTimeout(check).String(),
		"--metrics-bind-address", ":" + strconv.Itoa(metricsContainerPort),
	}

	env := []corev1.EnvVar{{
		Name: "NODE_NAME",
		// APIVersion is set explicitly to the value the API server
		// defaults it to, so the desired template round-trips and
		// CreateOrUpdate converges to a no-op (no churn).
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "spec.nodeName"}},
	}}
	// The on-demand run token rides on the pod template: the annotation is part
	// of the spec hash, so a new token rolls the agents (each scans on start),
	// and the downward-API env var hands each agent the token to stamp into its
	// report. Both are added only when a token exists so a check that has never
	// been triggered keeps exactly the template it has today (no restart on
	// operator upgrade). The consumed token is used when the annotation is
	// absent, so neither consumption nor removing the annotation afterwards
	// changes the template: exactly one rollout per new token.
	var templateAnnotations map[string]string
	if token := nodeCertTemplateToken(check); token != "" {
		templateAnnotations = map[string]string{fathomv1alpha1.AnnotationRunNow: token}
		env = append(env, corev1.EnvVar{
			Name: nodecert.EnvRunTrigger,
			ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.annotations['" + fathomv1alpha1.AnnotationRunNow + "']",
			}},
		})
	}

	runAsNonRoot := true
	runAsUser := int64(65532)
	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true
	automount := true
	graceperiod := int64(30)
	seccomp := corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: agentResourceName(check), Namespace: check.Namespace, Labels: labels},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: selectorLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: templateAnnotations},
				Spec: corev1.PodSpec{
					ServiceAccountName:            saName,
					AutomountServiceAccountToken:  &automount,
					NodeSelector:                  copyStringMap(check.Spec.NodeSelector),
					Tolerations:                   resolveTolerations(check),
					RestartPolicy:                 corev1.RestartPolicyAlways,
					DNSPolicy:                     corev1.DNSClusterFirst,
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
						Ports:                    []corev1.ContainerPort{{Name: "metrics", ContainerPort: metricsContainerPort, Protocol: corev1.ProtocolTCP}},
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
// decodes them into NodeReports, and adopts only reports whose payload belongs
// to this check. Reports are keyed by unique node name so duplicate ConfigMaps
// cannot inflate coverage.
func (r *NodeCertificateCheckReconciler) collectNodeReports(ctx context.Context, log logr.Logger, check *fathomv1alpha1.NodeCertificateCheck, now time.Time, maxAge time.Duration) ([]nodecert.NodeReport, []reportRejection, error) {
	var cms corev1.ConfigMapList
	if err := r.List(ctx, &cms,
		client.InNamespace(check.Namespace),
		client.MatchingLabels{
			nodecert.LabelManagedBy:  nodecert.ManagedByValue,
			nodecert.LabelSourceKind: nodecert.KindNodeCertificateCheck,
			nodecert.LabelSourceName: check.Name,
		},
	); err != nil {
		return nil, nil, err
	}

	var rejections []reportRejection
	reportsByNode := make(map[string]nodecert.NodeReport, len(cms.Items))
	for i := range cms.Items {
		cm := &cms.Items[i]
		raw, ok := cm.Data[nodecert.ConfigMapReportKey]
		if !ok {
			continue
		}
		report, err := nodecert.DecodeReport(raw)
		if err != nil {
			log.Error(err, "skipping unparsable node report ConfigMap", "configmap", cm.Name)
			continue
		}
		// Structural bindings, shared with NodeHealthCheck (#206) rather than
		// reimplemented. These corroborate; the ValidatingAdmissionPolicy is what
		// authenticates, by binding the node-name annotation to the writer's
		// ServiceAccount-token node claim. Re-checking here covers the cluster
		// where that policy is unavailable, and catches the one vector admission
		// alone does not: an off-name ConfigMap competing with a node's real one.
		annotated := cm.Annotations[nodecert.AnnotationNodeName]
		if reason := nodecert.VerifyReportBinding(cm.Name, annotated, check.Name, report); reason != nodecert.ReportAccepted {
			rejections = append(rejections, reportRejection{ConfigMap: cm.Name, Node: report.Node, Reason: reason})
			if reason.IndicatesForgery() {
				// Default level, not V(1): some principal with ConfigMap write in
				// this namespace is trying to steer a node's verdict.
				log.Info("rejected node report that failed its authenticity bindings",
					"configmap", cm.Name, "reason", string(reason), "annotatedNode", annotated, "reportNode", report.Node)
			} else {
				log.V(1).Info("skipping node report", "configmap", cm.Name, "reason", string(reason))
			}
			continue
		}
		if !nodeCertReportFresh(report, now, maxAge) {
			log.V(1).Info("skipping stale node report", "configmap", cm.Name, "node", report.Node, "observedAt", report.ObservedAt, "maxAge", maxAge.String())
			continue
		}
		if existing, ok := reportsByNode[report.Node]; ok && !report.ObservedAt.After(existing.ObservedAt) {
			continue
		}
		r.adoptReportConfigMap(ctx, log, check, cm)
		reportsByNode[report.Node] = report
	}
	reports := make([]nodecert.NodeReport, 0, len(reportsByNode))
	for _, report := range reportsByNode {
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Node < reports[j].Node })
	return reports, rejections, nil
}

// adoptReportConfigMap sets a controller owner reference on a report ConfigMap
// so it is garbage-collected with the check. Failures are non-fatal: the report
// was still consumed and the next reconcile retries.
func (r *NodeCertificateCheckReconciler) adoptReportConfigMap(ctx context.Context, log logr.Logger, check *fathomv1alpha1.NodeCertificateCheck, cm *corev1.ConfigMap) {
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

// rollup reconciles the aggregate scan result into HealthReport history under a
// transition-only contract that mirrors AddonCheck (ADR-0002): a new
// HealthReport is persisted only when the aggregate result changes from the last
// persisted one (or on the first roll-up). When the result is unchanged,
// LastRunTime is refreshed on the interval cadence — keeping the check's
// liveness fresh, since it feeds HealthCheck.Status.SourceObservedAt — without
// minting an identical report every interval. Writing an identical report each
// interval churned the deterministic name (it folds in LastReportName) and,
// with a bounded history, pruned a real Fail incident out of existence after
// historyLimit intervals (#157). Per-node daysRemaining drift is intentionally
// not a transition: unchanged-aggregate reports differ only in details.
func (r *NodeCertificateCheckReconciler) rollup(ctx context.Context, log logr.Logger, check *fathomv1alpha1.NodeCertificateCheck, reports []nodecert.NodeReport, aggregate fathomv1alpha1.HealthReportResult, interval time.Duration) error {
	now := time.Now()
	switch decideNodeCertRollup(&check.Status, string(aggregate), interval, now) {
	case rollupNoop:
		return nil
	case rollupRefreshLiveness:
		refreshed := metav1.NewTime(now)
		check.Status.LastRunTime = &refreshed
		return nil
	}

	// rollupPersist: the aggregate transitioned (or this is the first roll-up).
	observedAt := metav1.NewTime(now)
	report := healthReportForNodeCert(check, reports, aggregate, observedAt)
	useDeterministicHealthReportName(report, check.Name,
		"NodeCertificateCheck",
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
		pruneNodeCertHealthReports(ctx, r.Client, log, check)
	}

	// Reusing a deterministic HealthReport hands back that report's original
	// ObservedAt, which can predate the current LastRunTime. The observable run
	// time must never move backward (COR-3).
	if check.Status.LastRunTime == nil || persistedReport.Spec.ObservedAt.After(check.Status.LastRunTime.Time) {
		check.Status.LastRunTime = &persistedReport.Spec.ObservedAt
	}
	check.Status.LastReportName = persistedReport.Name
	check.Status.LastResult = string(persistedReport.Spec.Result)
	return nil
}

// reportRejection records one collected ConfigMap that failed its structural
// bindings, so the controller can surface the rejection instead of dropping it
// on the floor (SEC-1: an explicit error, not a silent skip).
type reportRejection struct {
	ConfigMap string
	Node      string
	Reason    nodecert.ReportRejection
}

// forgeryRejections returns only the rejections that indicate a writer trying to
// pass off another node's report, sorted by ConfigMap for stable messages.
func forgeryRejections(rejections []reportRejection) []reportRejection {
	var forged []reportRejection
	for _, rej := range rejections {
		if rej.Reason.IndicatesForgery() {
			forged = append(forged, rej)
		}
	}
	sort.Slice(forged, func(i, j int) bool { return forged[i].ConfigMap < forged[j].ConfigMap })
	return forged
}

// nodeCertRollupDecision is what a completed scan cycle does with its aggregate.
type nodeCertRollupDecision int

const (
	// rollupPersist writes a new HealthReport: the aggregate result transitioned
	// from the last persisted one, or this is the first roll-up.
	rollupPersist nodeCertRollupDecision = iota
	// rollupRefreshLiveness advances LastRunTime only: the aggregate is unchanged
	// but the interval has elapsed, so liveness is renewed without a new report.
	rollupRefreshLiveness
	// rollupNoop leaves status untouched: the aggregate is unchanged and the
	// interval has not yet elapsed.
	rollupNoop
)

// decideNodeCertRollup implements the transition-only contract documented on
// rollup. It is pure so the branching is unit-tested without envtest. The
// liveness refresh is throttled to the interval because the controller watches
// node-report ConfigMaps with a ResourceVersionChangedPredicate and every
// node-agent rewrites its ConfigMap each scan, so an unthrottled refresh would
// rewrite Status on each of the ~N-per-interval watch events instead of once.
func decideNodeCertRollup(status *fathomv1alpha1.NodeCertificateCheckStatus, aggregate string, interval time.Duration, now time.Time) nodeCertRollupDecision {
	if status.LastReportName == "" || status.LastRunTime == nil || status.LastResult != aggregate {
		return rollupPersist
	}
	if now.Sub(status.LastRunTime.Time) >= interval {
		return rollupRefreshLiveness
	}
	return rollupNoop
}

func nodeCertReportMaxAge(check *fathomv1alpha1.NodeCertificateCheck) time.Duration {
	return nodeCertInterval(check) + nodeCertTimeout(check)
}

func nodeCertReportFresh(report nodecert.NodeReport, now time.Time, maxAge time.Duration) bool {
	if report.ObservedAt.IsZero() {
		return false
	}
	if report.ObservedAt.After(now.Add(maxAge)) {
		return false
	}
	return now.Sub(report.ObservedAt) <= maxAge
}

// nodeAgentRolledOut reports whether the DaemonSet has fully converged to its
// current spec: the controller has observed the latest generation and every
// desired pod is updated and ready. Rollups and the RolledOut/Ready conditions
// gate on this so status is never stamped from stale-template pods that are
// mid-rollout (SKA-589).
func nodeAgentRolledOut(ds *appsv1.DaemonSet) bool {
	// ObservedGeneration == Generation: the DaemonSet controller never observes a
	// generation ahead of the spec, so equality matches the documented contract
	// and the e2e assertion exactly (no more permissive than either).
	return ds.Status.DesiredNumberScheduled > 0 &&
		ds.Status.ObservedGeneration == ds.Generation &&
		ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled &&
		ds.Status.NumberReady >= ds.Status.DesiredNumberScheduled
}

// expectedAgentNodes returns the identities of the nodes the agent DaemonSet is
// currently scheduled on, read from the agent pods themselves.
//
// The pods are the source of truth rather than the DaemonSet's node selector:
// resolving the selector would mean re-implementing scheduling, and listing
// Nodes would take a cluster-wide read this operator deliberately does not hold
// (compare #255). Every agent pod carrying a node name counts, including one
// that is terminating — a node mid-rollout is still in scope, and counting it
// keeps coverage failing closed until its replacement reports.
func (r *NodeCertificateCheckReconciler) expectedAgentNodes(ctx context.Context, check *fathomv1alpha1.NodeCertificateCheck) (map[string]struct{}, error) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods,
		client.InNamespace(check.Namespace),
		client.MatchingLabels{
			nodecert.LabelSourceName: check.Name,
			nodeAgentComponentLabel:  nodeAgentComponentValue,
		},
	); err != nil {
		return nil, err
	}
	nodes := make(map[string]struct{}, len(pods.Items))
	for i := range pods.Items {
		if node := pods.Items[i].Spec.NodeName; node != "" {
			nodes[node] = struct{}{}
		}
	}
	return nodes, nil
}

// nodeNameSet indexes reports by the node that produced them.
func nodeNameSet(reports []nodecert.NodeReport) map[string]struct{} {
	nodes := make(map[string]struct{}, len(reports))
	for _, report := range reports {
		nodes[report.Node] = struct{}{}
	}
	return nodes
}

// triggeredNodeSet indexes the nodes whose fresh report carries token. The empty
// token never matches, so an agent predating the field cannot complete a wait.
func triggeredNodeSet(reports []nodecert.NodeReport, token string) map[string]struct{} {
	nodes := make(map[string]struct{}, len(reports))
	for _, report := range reports {
		if token != "" && report.Trigger == token {
			nodes[report.Node] = struct{}{}
		}
	}
	return nodes
}

// missingNodes lists expected nodes absent from reported, sorted so status
// messages and log lines are stable across reconciles.
func missingNodes(expected, reported map[string]struct{}) []string {
	var missing []string
	for node := range expected {
		if _, ok := reported[node]; !ok {
			missing = append(missing, node)
		}
	}
	sort.Strings(missing)
	return missing
}

// nodeCertReportsComplete reports whether every node in scope published a fresh
// scan result.
//
// Completeness is per-node-identity, not a count. Counting let a departed node's
// still-fresh report substitute for a newly joined node's missing one, so the
// roll-up claimed full coverage while a live node had never been scanned
// (COR-4). Surplus reports from nodes that have left are simply not consulted,
// which preserves the churn tolerance the count comparison was reaching for
// (SKA-589) without the substitution it allowed.
//
// The count floor is kept on top of the identity check: before the agent pods
// exist, expected is a subset of the fleet and identity alone would pass
// vacuously. A fully converged rollout is still required so a roll-up is never
// computed from stale-template pods.
func nodeCertReportsComplete(ds *appsv1.DaemonSet, expected, reported map[string]struct{}) bool {
	return nodeAgentRolledOut(ds) &&
		int32(len(expected)) >= ds.Status.DesiredNumberScheduled &&
		len(missingNodes(expected, reported)) == 0
}

// setCoverage records whether the current verdict was computed from a complete
// scan of the fleet, so a frozen verdict is distinguishable from a fresh one.
func (r *NodeCertificateCheckReconciler) setCoverage(check *fathomv1alpha1.NodeCertificateCheck, ds *appsv1.DaemonSet, expected, reported map[string]struct{}, complete bool) {
	condition := metav1.Condition{
		Type:               nodeCertConditionCoverage,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "AllNodesReporting",
		Message:            fmt.Sprintf("All %d node(s) in scope published a fresh scan result.", len(expected)),
	}
	if !complete {
		condition.Status = metav1.ConditionFalse
		switch {
		case ds.Status.DesiredNumberScheduled == 0:
			condition.Reason = "NoMatchingNodes"
			condition.Message = "No nodes match the node-agent DaemonSet; nothing to scan."
		case !nodeAgentRolledOut(ds):
			condition.Reason = "AgentRollingOut"
			condition.Message = "Node-agent DaemonSet is still rolling out; the last known verdict is retained."
		default:
			condition.Reason = "PartialReports"
			condition.Message = fmt.Sprintf("%s have not published a fresh scan result; the last known verdict is retained.",
				coverageGapSummary(expected, reported, ds.Status.DesiredNumberScheduled))
		}
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, condition)
}

// coverageGapSummary describes the coverage gap, naming at most
// maxCoverageMessageNodes missing nodes so the condition message stays bounded.
func coverageGapSummary(expected, reported map[string]struct{}, desired int32) string {
	if int32(len(expected)) < desired {
		return fmt.Sprintf("Only %d of %d node-agent pod(s) are scheduled, so some nodes", len(expected), desired)
	}
	missing := missingNodes(expected, reported)
	if len(missing) > maxCoverageMessageNodes {
		return fmt.Sprintf("%d of %d node(s) in scope (%s and %d more)",
			len(missing), len(expected), strings.Join(missing[:maxCoverageMessageNodes], ", "), len(missing)-maxCoverageMessageNodes)
	}
	return fmt.Sprintf("%d of %d node(s) in scope (%s)", len(missing), len(expected), strings.Join(missing, ", "))
}

func (r *NodeCertificateCheckReconciler) setAgentReady(check *fathomv1alpha1.NodeCertificateCheck, ds *appsv1.DaemonSet) {
	status := metav1.ConditionFalse
	reason := "RollingOut"
	message := "Node-agent DaemonSet is rolling out."
	switch {
	case ds.Status.DesiredNumberScheduled == 0:
		reason = "NoMatchingNodes"
		message = "No nodes match the node-agent DaemonSet; nothing to scan."
	case nodeAgentRolledOut(ds):
		status = metav1.ConditionTrue
		reason = "RolledOut"
		message = "Node-agent DaemonSet is ready on all selected nodes."
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               nodeCertConditionAgentReady,
		Status:             status,
		ObservedGeneration: check.Generation,
		Reason:             reason,
		Message:            message,
	})
}

func (r *NodeCertificateCheckReconciler) setReadyFromState(check *fathomv1alpha1.NodeCertificateCheck, ds *appsv1.DaemonSet, expected, reported map[string]struct{}) {
	switch {
	case ds.Status.DesiredNumberScheduled == 0:
		r.setReady(check, metav1.ConditionFalse, "NoMatchingNodes", "No nodes match the node-agent DaemonSet.")
	case len(reported) == 0:
		r.setReady(check, metav1.ConditionFalse, "AwaitingReports", "Waiting for node-agents to publish fresh scan results.")
	case len(missingNodes(expected, reported)) > 0 || int32(len(expected)) < ds.Status.DesiredNumberScheduled:
		r.setReady(check, metav1.ConditionFalse, "PartialReports", "Waiting for every selected node-agent to publish a fresh scan result.")
	case !nodeAgentRolledOut(ds):
		// Every node in scope has reported but the DaemonSet has not fully
		// converged (a pod is not ready, or an update is still rolling). Surplus
		// reports from departed nodes are tolerated as transient churn rather than
		// flagged as a mismatch.
		r.setReady(check, metav1.ConditionFalse, "AgentRollingOut", "Node-agent DaemonSet is still rolling out.")
	default:
		r.setReady(check, metav1.ConditionTrue, "Reporting", "Node-agents are reporting and a HealthReport was rolled up.")
	}
}

// failProvisioning records a provisioning failure on Ready and persists it
// before returning the error.
//
// Setting the condition in memory and returning was the whole of COR-2: nothing
// reached the API server, so a check whose provisioning had been failing for
// days kept advertising the Ready=True/Pass it last succeeded with — the worst
// failure mode for a health product, whose entire job is to be believed when it
// says something is wrong. The provisioning error is returned unchanged so the
// controller keeps its rate-limited retry.
func (r *NodeCertificateCheckReconciler) failProvisioning(ctx context.Context, log logr.Logger, before *fathomv1alpha1.NodeCertificateCheckStatus, check *fathomv1alpha1.NodeCertificateCheck, reason string, cause error) (ctrl.Result, error) {
	r.setReady(check, metav1.ConditionFalse, reason, cause.Error())
	if !equality.Semantic.DeepEqual(before, &check.Status) {
		if err := r.Status().Update(ctx, check); err != nil {
			// A failed persist must not mask the provisioning error that caused it;
			// the next reconcile retries both.
			log.Error(err, "failed to persist NodeCertificateCheck status after provisioning failure", "reason", reason)
		}
	}
	return ctrl.Result{}, cause
}

// setReportsAuthentic records whether any collected report failed its
// authenticity bindings, and raises a Warning event when one did. A forgery
// signal means some principal with ConfigMap write in the namespace is actively
// trying to steer a node's verdict, which must not be a V(1) log line nobody
// reads (SEC-1).
func (r *NodeCertificateCheckReconciler) setReportsAuthentic(check *fathomv1alpha1.NodeCertificateCheck, rejections []reportRejection) {
	forged := forgeryRejections(rejections)
	if len(forged) == 0 {
		apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
			Type:               nodeCertConditionAuthentic,
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
		Type:               nodeCertConditionAuthentic,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: check.Generation,
		Reason:             eventReasonForgedReport,
		Message:            message,
	})
	if r.Recorder != nil {
		r.Recorder.Eventf(check, nil, corev1.EventTypeWarning, eventReasonForgedReport, eventActionEvaluate, "%s", message)
	}
}

func (r *NodeCertificateCheckReconciler) setReady(check *fathomv1alpha1.NodeCertificateCheck, status metav1.ConditionStatus, reason, message string) {
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               nodeCertConditionReady,
		Status:             status,
		ObservedGeneration: check.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager wires the reconciler. It owns the DaemonSet, ServiceAccount,
// and RoleBinding it creates, and watches report ConfigMaps by label so a fresh
// node report (which may not yet carry the owner reference) triggers a roll-up.
func (r *NodeCertificateCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.NodeAgentRoleName == "" {
		r.NodeAgentRoleName = defaultNodeAgentRoleName
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.NodeCertificateCheck{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(checkForReportConfigMap),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Named("nodecertificatecheck").
		Complete(r)
}

// checkForReportConfigMap maps a per-node report ConfigMap back to the
// NodeCertificateCheck that owns it, using the source labels the agent writes.
func checkForReportConfigMap(_ context.Context, obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()
	if labels[nodecert.LabelManagedBy] != nodecert.ManagedByValue ||
		labels[nodecert.LabelSourceKind] != nodecert.KindNodeCertificateCheck {
		return nil
	}
	name := labels[nodecert.LabelSourceName]
	if name == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: obj.GetNamespace(), Name: name}}}
}
