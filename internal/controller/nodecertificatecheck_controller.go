/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"sort"
	"strconv"
	"time"

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

	// nodeAgentComponentLabel/Value tag the DaemonSet and its pods so the
	// DaemonSet selector is stable and pods are discoverable.
	nodeAgentComponentLabel = "fathom.skaphos.io/component"
	nodeAgentComponentValue = "node-agent"

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

	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: check.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, ds, func() error {
		ds.Labels = mergeLabels(ds.Labels, desired.Labels)
		if ds.CreationTimestamp.IsZero() {
			// Selector is immutable: set it only on create.
			ds.Spec.Selector = desired.Spec.Selector
		}
		ds.Spec.Template = desired.Spec.Template
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
// adopts any not yet owner-referenced (so they are garbage-collected with the
// check), and decodes them into NodeReports. Reports are keyed by unique node
// name so duplicate ConfigMaps cannot inflate coverage.
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
		r.adoptReportConfigMap(ctx, log, check, cm)
		if report.CheckName != check.Name {
			log.V(1).Info("skipping node report for different check", "configmap", cm.Name, "reportCheckName", report.CheckName)
			continue
		}
		if report.Node == "" {
			log.V(1).Info("skipping node report without node name", "configmap", cm.Name)
			continue
		}
		if !nodeCertReportFresh(report, now, maxAge) {
			log.V(1).Info("skipping stale node report", "configmap", cm.Name, "node", report.Node, "observedAt", report.ObservedAt, "maxAge", maxAge.String())
			continue
		}
		if existing, ok := reportsByNode[report.Node]; ok && !report.ObservedAt.After(existing.ObservedAt) {
			continue
		}
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
	if r.Scheme != nil {
		if err := controllerutil.SetControllerReference(check, report, r.Scheme); err != nil {
			return err
		}
	}
	if err := r.Create(ctx, report); err != nil {
		return err
	}
	pruneNodeCertHealthReports(ctx, r.Client, log, check)

	check.Status.LastRunTime = &observedAt
	check.Status.LastReportName = report.Name
	check.Status.LastResult = string(aggregate)
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

func nodeCertReportsComplete(ds *appsv1.DaemonSet, reportCount int) bool {
	return ds.Status.DesiredNumberScheduled > 0 && int32(reportCount) == ds.Status.DesiredNumberScheduled
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
	case ds.Status.NumberReady >= ds.Status.DesiredNumberScheduled:
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
	case int32(reportCount) > ds.Status.DesiredNumberScheduled:
		r.setReady(check, metav1.ConditionFalse, "ReportMismatch", "Fresh node-agent reports do not match the selected node count.")
	case ds.Status.NumberReady < ds.Status.DesiredNumberScheduled:
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
