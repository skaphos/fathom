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
	"sort"
	"strconv"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

	defaultNodeCertInterval     = time.Hour
	defaultNodeCertTimeout      = 30 * time.Second
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
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=nodecertificatechecks,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=nodecertificatechecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=nodecertificatechecks/finalizers,verbs=update
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthreports,verbs=create;get;list;watch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// The node-agent ServiceAccount needs create/get/update on its own report
// ConfigMap; the operator grants that via the runtime fathom-node-agent-role
// ClusterRole. RBAC escalation prevention requires the operator to already hold
// every verb it confers, so the manager must also hold create (not just
// get;list;watch;update) on configmaps.
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch
// The controller ensures a cluster-scoped ValidatingAdmissionPolicy + binding
// that authenticate per-node report ConfigMaps (#155). Creating a VAP confers no
// privilege of its own, so this grant does not trip the RBAC escalation check.
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingadmissionpolicies;validatingadmissionpolicybindings,verbs=get;list;watch;create;update;patch

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
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	before := check.Status.DeepCopy()
	previousObservedGeneration := check.Status.ObservedGeneration
	check.Status.ObservedGeneration = check.Generation
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               nodeCertConditionAccepted,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "SpecAccepted",
		Message:            "NodeCertificateCheck specification has been accepted for reconciliation.",
	})

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
		r.setReady(&check, metav1.ConditionFalse, "RBACProvisioningFailed", err.Error())
		return ctrl.Result{}, err
	}

	if err := r.ensureReportAuthenticityPolicy(ctx, log); err != nil {
		r.setReady(&check, metav1.ConditionFalse, "AdmissionPolicyProvisioningFailed", err.Error())
		return ctrl.Result{}, err
	}

	saName, err := r.ensureAgentRBAC(ctx, &check)
	if err != nil {
		r.setReady(&check, metav1.ConditionFalse, "RBACProvisioningFailed", err.Error())
		return ctrl.Result{}, err
	}

	ds, err := r.ensureDaemonSet(ctx, &check, saName)
	if err != nil {
		r.setReady(&check, metav1.ConditionFalse, "DaemonSetProvisioningFailed", err.Error())
		return ctrl.Result{}, err
	}
	check.Status.DesiredNodes = ds.Status.DesiredNumberScheduled
	r.setAgentReady(&check, ds)

	reports, err := r.collectNodeReports(ctx, log, &check, time.Now(), nodeCertReportMaxAge(&check))
	if err != nil {
		return ctrl.Result{}, err
	}
	check.Status.ReportingNodes = int32(len(reports))

	if nodeCertReportsComplete(ds, len(reports)) {
		aggregate := aggregateNodeReports(reports)
		genChanged := previousObservedGeneration != check.Generation
		if r.rollupDue(&check, interval, string(aggregate), genChanged) {
			if err := r.rollup(ctx, log, &check, reports, aggregate); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		clearNodeCertRollupStatus(&check)
	}

	r.setReadyFromState(&check, ds, len(reports))
	return r.finish(ctx, log, before, &check, interval)
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
// its writing node-agent. The ObjectSelector narrows evaluation to Fathom
// node-report ConfigMaps so the policy never fires on unrelated ConfigMap writes;
// the writer-is-node-agent match condition exempts the operator's own owner-
// reference updates (it does not write as a "<check>-node-agent" ServiceAccount).
func reportAuthenticityPolicySpec() admissionregistrationv1.ValidatingAdmissionPolicySpec {
	fail := admissionregistrationv1.Fail
	forbidden := metav1.StatusReasonForbidden
	return admissionregistrationv1.ValidatingAdmissionPolicySpec{
		FailurePolicy: &fail,
		MatchConstraints: &admissionregistrationv1.MatchResources{
			ObjectSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				nodecert.LabelManagedBy:  nodecert.ManagedByValue,
				nodecert.LabelSourceKind: nodecert.KindNodeCertificateCheck,
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
		MatchConditions: []admissionregistrationv1.MatchCondition{{
			Name:       "writer-is-node-agent",
			Expression: `request.userInfo.username.matches('^system:serviceaccount:[^:]+:[^:]+-node-agent$')`,
		}},
		Variables: []admissionregistrationv1.Variable{
			{
				Name:       "claimNode",
				Expression: `request.userInfo.extra[?'authentication.kubernetes.io/node-name'].orValue([''])[0]`,
			},
			{
				Name:       "annotatedNode",
				Expression: `has(object.metadata.annotations) ? object.metadata.annotations[?'` + nodecert.AnnotationNodeName + `'].orValue('') : ''`,
			},
		},
		Validations: []admissionregistrationv1.Validation{{
			Expression: `variables.claimNode != '' && variables.annotatedNode == variables.claimNode`,
			Message:    "node-report ConfigMap fathom.skaphos.io/node-name annotation must match the writing node-agent's ServiceAccount-token node claim (authentication.kubernetes.io/node-name)",
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
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
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
						Name:            "node-agent",
						Image:           r.NodeAgentImage,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"/node-agent"},
						Args:            args,
						Env: []corev1.EnvVar{{
							Name: "NODE_NAME",
							// APIVersion is set explicitly to the value the API server
							// defaults it to, so the desired template round-trips and
							// CreateOrUpdate converges to a no-op (no churn).
							ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "spec.nodeName"}},
						}},
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
func (r *NodeCertificateCheckReconciler) collectNodeReports(ctx context.Context, log logr.Logger, check *fathomv1alpha1.NodeCertificateCheck, now time.Time, maxAge time.Duration) ([]nodecert.NodeReport, error) {
	var cms corev1.ConfigMapList
	if err := r.List(ctx, &cms,
		client.InNamespace(check.Namespace),
		client.MatchingLabels{
			nodecert.LabelManagedBy:  nodecert.ManagedByValue,
			nodecert.LabelSourceKind: nodecert.KindNodeCertificateCheck,
			nodecert.LabelSourceName: check.Name,
		},
	); err != nil {
		return nil, err
	}

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
		if report.CheckName != check.Name {
			log.V(1).Info("skipping node report for different check", "configmap", cm.Name, "reportCheckName", report.CheckName)
			continue
		}
		if report.Node == "" {
			log.V(1).Info("skipping node report without node name", "configmap", cm.Name)
			continue
		}
		// Consistency cross-check. The node-name annotation is an authenticity signal
		// only because the report-authenticity ValidatingAdmissionPolicy binds it to
		// the writing agent's ServiceAccount-token node claim at admission; this check
		// then rejects a payload whose Node disagrees with that bound annotation, and
		// skips a report missing it (one predating the contract, or written where the
		// policy is unavailable). On a cluster without the policy an attacker who can
		// write the ConfigMap can set both the payload Node and the annotation to a
		// victim node, so this check alone enforces internal consistency, not
		// authenticity — the admission policy is what provides authenticity.
		if annotated := cm.Annotations[nodecert.AnnotationNodeName]; annotated == "" || annotated != report.Node {
			log.V(1).Info("skipping node report whose payload node does not match its authenticated node-name annotation", "configmap", cm.Name, "annotatedNode", annotated, "reportNode", report.Node)
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
	return reports, nil
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

// rollup creates a HealthReport from the node reports, prunes history, and
// mirrors the aggregate into Status.
func (r *NodeCertificateCheckReconciler) rollup(ctx context.Context, log logr.Logger, check *fathomv1alpha1.NodeCertificateCheck, reports []nodecert.NodeReport, aggregate fathomv1alpha1.HealthReportResult) error {
	observedAt := metav1.NewTime(time.Now())
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

	check.Status.LastRunTime = &persistedReport.Spec.ObservedAt
	check.Status.LastReportName = persistedReport.Name
	check.Status.LastResult = string(persistedReport.Spec.Result)
	return nil
}

func (r *NodeCertificateCheckReconciler) rollupDue(check *fathomv1alpha1.NodeCertificateCheck, interval time.Duration, aggregate string, genChanged bool) bool {
	if check.Status.LastRunTime == nil || genChanged {
		return true
	}
	if check.Status.LastResult != aggregate {
		return true
	}
	return time.Since(check.Status.LastRunTime.Time) >= interval
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

func nodeCertReportsComplete(ds *appsv1.DaemonSet, reportCount int) bool {
	// Require a fully converged rollout so a rollup is never computed from
	// stale-template pods, and tolerate transient node-count churn: a report from
	// a node that was removed or deselected survives (owner-referenced by the
	// check) until it ages out, so accept reportCount >= desired rather than exact
	// equality, which would otherwise blank the rollup for up to interval+timeout
	// (SKA-589).
	return nodeAgentRolledOut(ds) && int32(reportCount) >= ds.Status.DesiredNumberScheduled
}

func clearNodeCertRollupStatus(check *fathomv1alpha1.NodeCertificateCheck) {
	check.Status.LastRunTime = nil
	check.Status.LastReportName = ""
	check.Status.LastResult = ""
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

func (r *NodeCertificateCheckReconciler) setReadyFromState(check *fathomv1alpha1.NodeCertificateCheck, ds *appsv1.DaemonSet, reportCount int) {
	switch {
	case ds.Status.DesiredNumberScheduled == 0:
		r.setReady(check, metav1.ConditionFalse, "NoMatchingNodes", "No nodes match the node-agent DaemonSet.")
	case reportCount == 0:
		r.setReady(check, metav1.ConditionFalse, "AwaitingReports", "Waiting for node-agents to publish fresh scan results.")
	case int32(reportCount) < ds.Status.DesiredNumberScheduled:
		r.setReady(check, metav1.ConditionFalse, "PartialReports", "Waiting for every selected node-agent to publish a fresh scan result.")
	case !nodeAgentRolledOut(ds):
		// reportCount >= desired but the DaemonSet has not fully converged (a pod
		// is not ready or an update is still rolling). A surplus of reports here is
		// tolerated as transient node-count churn rather than flagged as a mismatch.
		r.setReady(check, metav1.ConditionFalse, "AgentRollingOut", "Node-agent DaemonSet is still rolling out.")
	default:
		r.setReady(check, metav1.ConditionTrue, "Reporting", "Node-agents are reporting and a HealthReport was rolled up.")
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
