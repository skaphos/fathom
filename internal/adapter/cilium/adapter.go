/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package cilium provides the built-in Cilium (CNI baseline) addon adapter.
//
// The adapter checks three independent families: control_plane_health
// (the cilium-operator Deployment and its pods), agent_health (the cilium
// agent DaemonSet and its pods), and crd_health (the core Cilium
// CustomResourceDefinitions). Each family is the per-CNI capability axis —
// a future CNI adapter (Calico, etc.) lives in its own package and declares
// its own families, so "generic CNI extension points" are served by the
// existing per-adapter/per-family model rather than a plugin system.
//
// When Cilium is not installed the workloads and CRDs are absent: the adapter
// reports OutcomeSkipped (which rolls up green) rather than OutcomeFail, so a
// cilium AddonCheck on a cluster that may or may not run Cilium stays quiet.
// A workload that exists but is unhealthy is still OutcomeFail.
package cilium

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/skaphos/fathom/internal/adapter/crdutil"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/pkg/adapter"
)

// tracer is the OpenTelemetry instrumentation scope for the cilium adapter.
// It draws from the global provider, so it is a no-op unless the operator
// enabled tracing (SKA-293).
var tracer = otel.Tracer("github.com/skaphos/fathom/internal/adapter/cilium")

const (
	Name    = "cilium"
	Version = "0.1.0"

	// FamilyControlPlaneHealth covers the cilium-operator Deployment and pods.
	FamilyControlPlaneHealth = adapter.Family("control_plane_health")
	// FamilyAgentHealth covers the cilium agent DaemonSet and pods.
	FamilyAgentHealth = adapter.Family("agent_health")
	// FamilyCRDHealth covers the core Cilium CustomResourceDefinitions.
	FamilyCRDHealth = adapter.Family("crd_health")

	defaultNamespace        = "kube-system"
	defaultOperatorName     = "cilium-operator"
	defaultAgentName        = "cilium"
	defaultRestartWarnCount = int32(3)

	thresholdRestartWarnCount = "restartWarnCount"
	thresholdOperatorName     = "operatorDeploymentName"
	thresholdAgentName        = "agentDaemonSetName"

	componentOperator = "cilium-operator"
	componentAgent    = "cilium-agent"
)

var (
	// ciliumCRDs are the core Cilium CustomResourceDefinitions whose
	// establishment the crd_health family verifies. These five have been
	// present across Cilium's stable v2 API; distro- or version-specific
	// CRDs (e.g. ciliumloadbalancerippools) are intentionally excluded from
	// the baseline so the check does not flap across Cilium versions.
	ciliumCRDs = []string{
		"ciliumnetworkpolicies.cilium.io",
		"ciliumclusterwidenetworkpolicies.cilium.io",
		"ciliumendpoints.cilium.io",
		"ciliumidentities.cilium.io",
		"ciliumnodes.cilium.io",
	}
	// supportedAPIVersions lists the cilium.io API versions Fathom
	// understands, in descending preference order. Cilium has served its
	// CRDs at v2 for years; v2alpha1 is accepted as a tolerated fallback.
	supportedAPIVersions = []string{"v2", "v2alpha1"}
)

// Adapter implements Cilium (CNI baseline) health checks.
type Adapter struct{}

// New returns the built-in Cilium adapter.
func New() Adapter {
	return Adapter{}
}

func (Adapter) Name() string            { return Name }
func (Adapter) Version() string         { return Version }
func (Adapter) ContractVersion() string { return adapter.ContractVersion }

func (Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{
		AddonTypes: []string{Name},
		Families:   []adapter.Family{FamilyControlPlaneHealth, FamilyAgentHealth, FamilyCRDHealth},
	}
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

func (a Adapter) Run(ctx context.Context, req adapter.Request) (result adapter.Result, err error) {
	ctx, span := tracer.Start(ctx, Name+".run")
	span.SetAttributes(attribute.String("fathom.adapter", Name))
	defer func() { endAdapterRunSpan(span, result, err) }()

	started := time.Now()
	controlPolicy, controlEnabled := familyPolicy(req.Policy, FamilyControlPlaneHealth, true)
	agentPolicy, agentEnabled := familyPolicy(req.Policy, FamilyAgentHealth, true)
	_, crdEnabled := familyPolicy(req.Policy, FamilyCRDHealth, true)

	if !controlEnabled && !agentEnabled && !crdEnabled {
		return adapter.Result{
			Checks:   []adapter.CheckResult{skipped(FamilyControlPlaneHealth, req.Target, "all cilium check families are disabled by policy")},
			Duration: time.Since(started),
		}, nil
	}

	checks := []adapter.CheckResult{}

	// Per-family timing for fathom_adapter_run_duration_seconds: time each
	// family independently so a multi-family run does not observe the same
	// wall-clock duration more than once (SKA-290).
	var controlDur, agentDur, crdDur time.Duration
	if controlEnabled {
		controlStart := time.Now()
		checks = append(checks, a.checkControlPlane(ctx, req.Client, controlPolicy)...)
		controlDur = time.Since(controlStart)
	}
	if agentEnabled {
		agentStart := time.Now()
		checks = append(checks, a.checkAgent(ctx, req.Client, agentPolicy)...)
		agentDur = time.Since(agentStart)
	}
	if crdEnabled {
		crdStart := time.Now()
		checks = append(checks, a.checkCRDs(ctx, req.Client)...)
		crdDur = time.Since(crdStart)
	}

	// Record per family that ran, with that family's own duration and roll-up.
	// FamilyOutcome considers only that family's checks, so one family's
	// failure does not taint another's metric (SKA-290).
	if controlEnabled {
		metrics.RecordAdapterRun(Name, string(FamilyControlPlaneHealth), string(adapter.FamilyOutcome(checks, FamilyControlPlaneHealth)), controlDur)
	}
	if agentEnabled {
		metrics.RecordAdapterRun(Name, string(FamilyAgentHealth), string(adapter.FamilyOutcome(checks, FamilyAgentHealth)), agentDur)
	}
	if crdEnabled {
		metrics.RecordAdapterRun(Name, string(FamilyCRDHealth), string(adapter.FamilyOutcome(checks, FamilyCRDHealth)), crdDur)
	}

	return adapter.Result{Checks: checks, Duration: time.Since(started)}, nil
}

// endAdapterRunSpan annotates span with the check count and a per-family
// outcome (the same adapter.FamilyOutcome roll-up the per-family metrics use),
// records err, and ends the span. Only families that actually produced checks
// are tagged, so a disabled family is not mislabeled as a passing one.
func endAdapterRunSpan(span trace.Span, result adapter.Result, err error) {
	span.SetAttributes(attribute.Int("fathom.adapter.check_count", len(result.Checks)))
	seen := map[adapter.Family]struct{}{}
	for _, c := range result.Checks {
		if _, ok := seen[c.Family]; ok {
			continue
		}
		seen[c.Family] = struct{}{}
		span.SetAttributes(attribute.String(
			"fathom.outcome."+string(c.Family),
			string(adapter.FamilyOutcome(result.Checks, c.Family)),
		))
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// checkControlPlane evaluates the cilium-operator Deployment and its pods.
func (a Adapter) checkControlPlane(ctx context.Context, c client.Client, policy adapter.FamilyPolicy) []adapter.CheckResult {
	namespace := policyNamespace(policy)
	name := stringThreshold(policy, thresholdOperatorName, defaultOperatorName)
	restartWarnCount := int32Threshold(policy, thresholdRestartWarnCount, defaultRestartWarnCount)

	deployment, deploymentCheck := a.checkDeployment(ctx, c, FamilyControlPlaneHealth, namespace, name, componentOperator)
	checks := []adapter.CheckResult{deploymentCheck}
	if deployment != nil && desiredReplicas(deployment) > 0 {
		checks = append(checks, a.checkPods(ctx, c, FamilyControlPlaneHealth, deployment.Namespace, deployment.Spec.Selector, componentOperator, restartWarnCount)...)
	}
	return checks
}

// checkAgent evaluates the cilium agent DaemonSet and its pods.
func (a Adapter) checkAgent(ctx context.Context, c client.Client, policy adapter.FamilyPolicy) []adapter.CheckResult {
	namespace := policyNamespace(policy)
	name := stringThreshold(policy, thresholdAgentName, defaultAgentName)
	restartWarnCount := int32Threshold(policy, thresholdRestartWarnCount, defaultRestartWarnCount)

	daemonset, daemonsetCheck := a.checkDaemonSet(ctx, c, FamilyAgentHealth, namespace, name, componentAgent)
	checks := []adapter.CheckResult{daemonsetCheck}
	if daemonset != nil && daemonset.Status.DesiredNumberScheduled > 0 {
		checks = append(checks, a.checkPods(ctx, c, FamilyAgentHealth, daemonset.Namespace, daemonset.Spec.Selector, componentAgent, restartWarnCount)...)
	}
	return checks
}

// checkCRDs verifies that the core Cilium CRDs are established and serve a
// supported version. Absent CRDs are reported Skipped (Cilium not installed).
func (a Adapter) checkCRDs(ctx context.Context, c client.Client) []adapter.CheckResult {
	checks := make([]adapter.CheckResult, 0, len(ciliumCRDs))
	for _, name := range ciliumCRDs {
		checks = append(checks, a.checkCRD(ctx, c, FamilyCRDHealth, name))
	}
	return checks
}

func (Adapter) checkDeployment(ctx context.Context, c client.Client, family adapter.Family, namespace, name, component string) (*appsv1.Deployment, adapter.CheckResult) {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: namespace, Name: name}
	details := map[string]string{"component": component}
	var deployment appsv1.Deployment
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, check(family, target, adapter.OutcomeSkipped, "cilium operator deployment not found; Cilium may not be installed", details, started)
		}
		return nil, check(family, target, adapter.OutcomeError, fmt.Sprintf("failed to read cilium operator deployment: %v", err), details, started)
	}

	desired := desiredReplicas(&deployment)
	if desired == 0 {
		return &deployment, check(family, target, adapter.OutcomeWarn, "cilium operator deployment is scaled to zero", details, started)
	}
	if !deploymentAvailable(&deployment) || deployment.Status.AvailableReplicas < desired {
		details["desiredReplicas"] = strconv.FormatInt(int64(desired), 10)
		details["availableReplicas"] = strconv.FormatInt(int64(deployment.Status.AvailableReplicas), 10)
		return &deployment, check(family, target, adapter.OutcomeFail, "cilium operator deployment is not fully available", details, started)
	}
	return &deployment, check(family, target, adapter.OutcomePass, "cilium operator deployment is available", details, started)
}

func (Adapter) checkDaemonSet(ctx context.Context, c client.Client, family adapter.Family, namespace, name, component string) (*appsv1.DaemonSet, adapter.CheckResult) {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "DaemonSet", Namespace: namespace, Name: name}
	details := map[string]string{"component": component}
	var daemonset appsv1.DaemonSet
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &daemonset); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, check(family, target, adapter.OutcomeSkipped, "cilium agent daemonset not found; Cilium may not be installed", details, started)
		}
		return nil, check(family, target, adapter.OutcomeError, fmt.Sprintf("failed to read cilium agent daemonset: %v", err), details, started)
	}

	status := daemonset.Status
	details["desiredNumberScheduled"] = strconv.FormatInt(int64(status.DesiredNumberScheduled), 10)
	details["numberReady"] = strconv.FormatInt(int64(status.NumberReady), 10)
	details["numberAvailable"] = strconv.FormatInt(int64(status.NumberAvailable), 10)
	details["numberUnavailable"] = strconv.FormatInt(int64(status.NumberUnavailable), 10)
	details["updatedNumberScheduled"] = strconv.FormatInt(int64(status.UpdatedNumberScheduled), 10)

	// A DaemonSet legitimately schedules zero pods when no node matches its
	// nodeSelector. That is unusual for a CNI agent, so surface it as a Warn
	// rather than a silent Pass — but it is not a Fail.
	if status.DesiredNumberScheduled == 0 {
		return &daemonset, check(family, target, adapter.OutcomeWarn, "cilium agent daemonset schedules zero pods (no matching nodes)", details, started)
	}
	if status.NumberUnavailable > 0 || status.NumberReady < status.DesiredNumberScheduled {
		return &daemonset, check(family, target, adapter.OutcomeFail, "cilium agent daemonset is not fully ready", details, started)
	}
	if status.UpdatedNumberScheduled < status.DesiredNumberScheduled {
		return &daemonset, check(family, target, adapter.OutcomeWarn, "cilium agent daemonset rollout is in progress", details, started)
	}
	return &daemonset, check(family, target, adapter.OutcomePass, "cilium agent daemonset is fully ready", details, started)
}

func (Adapter) checkPods(ctx context.Context, c client.Client, family adapter.Family, namespace string, selector *metav1.LabelSelector, component string, restartWarnCount int32) []adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "v1", Kind: "Pod", Namespace: namespace, Name: component}
	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return []adapter.CheckResult{check(family, target, adapter.OutcomeError, fmt.Sprintf("%s has an invalid pod selector: %v", component, err), map[string]string{"component": component}, started)}
	}
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: sel}); err != nil {
		return []adapter.CheckResult{check(family, target, adapter.OutcomeError, fmt.Sprintf("failed to list %s pods: %v", component, err), map[string]string{"component": component}, started)}
	}
	if len(pods.Items) == 0 {
		return []adapter.CheckResult{check(family, target, adapter.OutcomeFail, fmt.Sprintf("%s has no matching pods", component), map[string]string{"component": component}, started)}
	}

	checks := make([]adapter.CheckResult, 0, len(pods.Items))
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !podReady(pod) {
			checks = append(checks, check(family, podTarget(pod), adapter.OutcomeFail, fmt.Sprintf("%s pod is not ready", component), map[string]string{"component": component, "phase": string(pod.Status.Phase)}, started))
			continue
		}
		if restarts := maxRestartCount(pod); restarts > restartWarnCount {
			checks = append(checks, check(family, podTarget(pod), adapter.OutcomeWarn, fmt.Sprintf("%s pod restart count exceeds warning threshold", component), map[string]string{
				"component":        component,
				"restartCount":     strconv.FormatInt(int64(restarts), 10),
				"restartWarnCount": strconv.FormatInt(int64(restartWarnCount), 10),
			}, started))
			continue
		}
		checks = append(checks, check(family, podTarget(pod), adapter.OutcomePass, fmt.Sprintf("%s pod is ready", component), map[string]string{"component": component}, started))
	}
	return checks
}

func (Adapter) checkCRD(ctx context.Context, c client.Client, family adapter.Family, name string) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apiextensions.k8s.io/v1", Kind: "CustomResourceDefinition", Name: name}
	details := map[string]string{"crd": name}
	var crd apixv1.CustomResourceDefinition
	if err := c.Get(ctx, types.NamespacedName{Name: name}, &crd); err != nil {
		if apierrors.IsNotFound(err) {
			return check(family, target, adapter.OutcomeSkipped, "cilium CRD not found; Cilium may not be installed", details, started)
		}
		return check(family, target, adapter.OutcomeError, fmt.Sprintf("failed to read cilium CRD: %v", err), details, started)
	}
	if !crdutil.Established(&crd) {
		return check(family, target, adapter.OutcomeFail, "cilium CRD is not established", details, started)
	}
	servedVersion, ok := crdutil.PreferredServedVersion(&crd, supportedAPIVersions)
	if !ok {
		// Established but serving an unrecognized version: tolerate as a Warn
		// (a newer Cilium API Fathom has not learned about yet) rather than a
		// hard Fail.
		details["expectedVersions"] = strings.Join(supportedAPIVersions, ",")
		return check(family, target, adapter.OutcomeWarn, "cilium CRD serves no version Fathom recognizes", details, started)
	}
	details["version"] = servedVersion
	return check(family, target, adapter.OutcomePass, "cilium CRD is established", details, started)
}

func desiredReplicas(deployment *appsv1.Deployment) int32 {
	if deployment.Spec.Replicas != nil {
		return *deployment.Spec.Replicas
	}
	return 1
}

func deploymentAvailable(deployment *appsv1.Deployment) bool {
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func maxRestartCount(pod *corev1.Pod) int32 {
	var max int32
	for _, status := range pod.Status.ContainerStatuses {
		if status.RestartCount > max {
			max = status.RestartCount
		}
	}
	return max
}

func podTarget(pod *corev1.Pod) adapter.TargetRef {
	return adapter.TargetRef{APIVersion: "v1", Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name}
}

// policyNamespace resolves the namespace the Cilium workloads live in. The
// operator and agent are single workloads in one namespace, so only the first
// entry of policy.Namespaces is honored; an empty list falls back to the
// canonical kube-system.
func policyNamespace(policy adapter.FamilyPolicy) string {
	if len(policy.Namespaces) > 0 {
		return policy.Namespaces[0]
	}
	return defaultNamespace
}

func familyPolicy(policy map[adapter.Family]adapter.FamilyPolicy, family adapter.Family, defaultEnabled bool) (adapter.FamilyPolicy, bool) {
	if policy == nil {
		return adapter.FamilyPolicy{Enabled: defaultEnabled}, defaultEnabled
	}
	familyPolicy, ok := policy[family]
	if !ok {
		return adapter.FamilyPolicy{}, false
	}
	return familyPolicy, familyPolicy.Enabled
}

func stringThreshold(policy adapter.FamilyPolicy, key, defaultValue string) string {
	if policy.Thresholds == nil {
		return defaultValue
	}
	value := strings.TrimSpace(policy.Thresholds[key])
	if value == "" {
		return defaultValue
	}
	return value
}

func int32Threshold(policy adapter.FamilyPolicy, key string, defaultValue int32) int32 {
	if policy.Thresholds == nil {
		return defaultValue
	}
	value, ok := policy.Thresholds[key]
	if !ok {
		return defaultValue
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 {
		return defaultValue
	}
	return int32(parsed)
}

// skipped emits a CheckResult marking the named target as intentionally not
// executed. Family is required so the all-disabled contract surfaces under a
// real policy family rather than collapsing into a single bucket.
func skipped(family adapter.Family, target adapter.TargetRef, summary string) adapter.CheckResult {
	return adapter.CheckResult{Family: family, Outcome: adapter.OutcomeSkipped, TargetRef: target, Summary: summary, ObservedAt: time.Now()}
}

// check emits a CheckResult tagged with the caller's policy family. Callers
// must pass the family that gates the surrounding work so the HealthReport's
// family attribution stays aligned with AddonCheck.spec.policy.
func check(family adapter.Family, target adapter.TargetRef, outcome adapter.Outcome, summary string, details map[string]string, started time.Time) adapter.CheckResult {
	return adapter.CheckResult{
		Family:     family,
		Outcome:    outcome,
		TargetRef:  target,
		Summary:    summary,
		Details:    details,
		ObservedAt: time.Now(),
		Duration:   time.Since(started),
	}
}
