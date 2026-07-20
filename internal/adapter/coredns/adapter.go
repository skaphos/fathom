/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package coredns provides the built-in CoreDNS addon adapter.
package coredns

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/skaphos/fathom/internal/adapter/podutil"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/internal/probe"
	"github.com/skaphos/fathom/pkg/adapter"
)

// tracer is the OpenTelemetry instrumentation scope for the CoreDNS adapter.
// It draws from the global provider, so it is a no-op unless the operator
// enabled tracing (SKA-293).
var tracer = otel.Tracer("github.com/skaphos/fathom/internal/adapter/coredns")

const (
	Name                = "coredns"
	Version             = "0.1.1"
	FamilySystemHealth  = adapter.Family("system_health")
	FamilyDNSResolution = adapter.Family("dns_resolution")

	defaultNamespace        = "kube-system"
	defaultDeploymentName   = "coredns"
	defaultRestartWarnCount = int32(3)
	defaultDNSServiceName   = "kube-dns"
	defaultDNSTargets       = "kubernetes.default.svc.cluster.local"
	// fallbackProbeImage is the last-resort probe image when neither the
	// per-AddonCheck probeImage threshold nor the operator-level
	// adapter.Request.ProbeImage is set. Keeping a non-empty fallback means a
	// misconfigured operator produces a clear ImagePullBackOff against this
	// reference instead of an empty Pod spec. The version tag is bumped
	// automatically in lockstep with the operator release by release-please
	// (x-release-please-version; see release-please-config.json) and enforced by
	// scripts/check-version-lockstep.sh — do not hand-edit it.
	fallbackProbeImage   = "ghcr.io/skaphos/fathom-probe:v0.4.1" // x-release-please-version
	defaultProbeTimeout  = 10 * time.Second
	probePodNameMaxLabel = 30

	thresholdRestartWarnCount = "restartWarnCount"
	thresholdDeploymentName   = "deploymentName"
	thresholdAutoscalerName   = "autoscalerName"
	thresholdServiceName      = "serviceName"
	thresholdTargets          = "targets"
	thresholdProbeImage       = "probeImage"
	thresholdProbeNamespace   = "probeNamespace"
)

// dnsProbeLauncher is the surface the dns_resolution family needs from
// probe.Launcher. The interface exists so unit tests can supply a fake
// without a real client and without standing up a probe pod.
type dnsProbeLauncher interface {
	Run(ctx context.Context, req probe.Request) (probe.Result, error)
}

// Adapter implements CoreDNS system and DNS behavior checks.
type Adapter struct {
	// launcher is injected for testing. Production runs construct a real
	// probe.Launcher per Run with the request's controller-runtime client.
	launcher dnsProbeLauncher
}

// New returns the built-in CoreDNS adapter.
func New() Adapter { return Adapter{} }

func (Adapter) Name() string            { return Name }
func (Adapter) Version() string         { return Version }
func (Adapter) ContractVersion() string { return adapter.ContractVersion }

func (Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{Name}, Families: []adapter.Family{FamilySystemHealth, FamilyDNSResolution}}
}

// RBACRules declares CoreDNS's least-privilege grants (adapter.RBACDeclarer): the
// passive workload/endpoint reads, plus the probe-pod lifecycle write the
// DNS-resolution check performs — all under CoreDNS's impersonated ServiceAccount
// (SKA-58). Each rule carries a defensive Justification (why it is needed and why
// less would not suffice); the generator emits a scoped ClusterRole and renders
// the justifications into docs/reference/rbac.md, and the guard permits the pods
// create;delete only because it is justified.
func (Adapter) RBACRules() []adapter.PolicyRule {
	return []adapter.PolicyRule{
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read the CoreDNS Deployment to score replica/rollout readiness. list+watch because the deployment name is policy-overridable; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods", "services"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read CoreDNS Pods (restart counts, readiness) and the kube-dns Service (the probe's resolution target). list is required because Pod names are dynamic; read-only and limited to these two core kinds."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"create", "delete"},
			Justification: "WRITE EXCEPTION: launch and immediately tear down a single-shot DNS probe Pod per check to measure resolution from a workload's perspective (ADR-0003). create+delete are the minimal verbs for an ephemeral Pod; no in-process read can observe real in-cluster DNS. The Pod is deleted as soon as it completes — no update, no long-lived workload."},
		{APIGroups: []string{"discovery.k8s.io"}, Resources: []string{"endpointslices"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read the EndpointSlices behind the DNS Service to confirm it has ready backends. list is required to enumerate slices for the Service; read-only."},
	}
}

// CoreDNS's cluster permissions (including the probe-pod create;delete) are NOT
// granted to the operator ServiceAccount. They live on this adapter's per-addon
// ServiceAccount (RBACRules above), generated into
// config/rbac/addons/addon-coredns.yaml; the operator only impersonates that
// ServiceAccount at run time (SKA-58). No +kubebuilder:rbac markers here.

func (a Adapter) Run(ctx context.Context, req adapter.Request) (result adapter.Result, err error) {
	ctx, span := tracer.Start(ctx, Name+".run")
	span.SetAttributes(attribute.String("fathom.adapter", Name))
	defer func() { endAdapterRunSpan(span, result, err) }()

	started := time.Now()
	checks := []adapter.CheckResult{}
	// Record fathom_adapter_run_duration_seconds per family inside its own
	// branch, timing and labelling each family independently (SKA-290).
	if systemPolicy, enabled := familyPolicy(req.Policy, FamilySystemHealth, true); enabled {
		familyStart := time.Now()
		familyChecks := a.checkSystemHealth(ctx, req.Client, systemPolicy)
		checks = append(checks, familyChecks...)
		metrics.RecordAdapterRun(Name, string(FamilySystemHealth), string(adapter.FamilyOutcome(familyChecks, FamilySystemHealth)), time.Since(familyStart))
	}
	if dnsPolicy, enabled := familyPolicy(req.Policy, FamilyDNSResolution, true); enabled {
		familyStart := time.Now()
		familyChecks := a.checkDNSResolution(ctx, req, dnsPolicy)
		checks = append(checks, familyChecks...)
		metrics.RecordAdapterRun(Name, string(FamilyDNSResolution), string(adapter.FamilyOutcome(familyChecks, FamilyDNSResolution)), time.Since(familyStart))
	}
	if len(checks) == 0 {
		checks = append(checks, skipped(req.Target, "all CoreDNS check families are disabled by policy"))
	}

	return adapter.Result{Checks: checks, Duration: time.Since(started)}, nil
}

// endAdapterRunSpan annotates span with the check count and a per-family
// outcome (the same adapter.FamilyOutcome roll-up the per-family metrics use),
// records err, and ends the span. Only families that actually produced checks
// are tagged, so a disabled family is not mislabeled as a passing one. Mirrors
// the per-adapter helper style already used in this package.
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

func (a Adapter) checkSystemHealth(ctx context.Context, c client.Client, policy adapter.FamilyPolicy) []adapter.CheckResult {
	namespace := firstNamespace(policy)
	deploymentName := stringThreshold(policy, thresholdDeploymentName, defaultDeploymentName)
	serviceName := stringThreshold(policy, thresholdServiceName, defaultDNSServiceName)
	restartWarnCount := int32Threshold(policy, thresholdRestartWarnCount, defaultRestartWarnCount)
	checks := []adapter.CheckResult{}
	deployment, check := a.checkDeployment(ctx, c, namespace, deploymentName)
	checks = append(checks, check)
	if deployment != nil {
		checks = append(checks, a.checkPods(ctx, c, deployment, restartWarnCount)...)
	}
	if autoscalerName := stringThreshold(policy, thresholdAutoscalerName, ""); autoscalerName != "" {
		autoscaler, check := a.checkDeployment(ctx, c, namespace, autoscalerName)
		checks = append(checks, check)
		if autoscaler != nil {
			checks = append(checks, a.checkPods(ctx, c, autoscaler, restartWarnCount)...)
		}
	}
	checks = append(checks, a.checkService(ctx, c, namespace, serviceName))
	checks = append(checks, a.checkEndpointSlices(ctx, c, namespace, serviceName))
	return checks
}

func (Adapter) checkDeployment(ctx context.Context, c client.Client, namespace, name string) (*appsv1.Deployment, adapter.CheckResult) {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: namespace, Name: name}
	var deployment appsv1.Deployment
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, check(target, adapter.OutcomeFail, "CoreDNS deployment is missing", adapter.MarkAbsent(map[string]string{"component": name}), started)
		}
		return nil, check(target, adapter.OutcomeError, fmt.Sprintf("failed to read CoreDNS deployment: %v", err), map[string]string{"component": name}, started)
	}
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	if desired == 0 {
		return &deployment, check(target, adapter.OutcomeWarn, "CoreDNS deployment is scaled to zero", map[string]string{"component": name}, started)
	}
	if !deploymentAvailable(&deployment) || deployment.Status.AvailableReplicas < desired {
		return &deployment, check(target, adapter.OutcomeFail, "CoreDNS deployment is not fully available", map[string]string{"component": name, "desiredReplicas": strconv.FormatInt(int64(desired), 10), "availableReplicas": strconv.FormatInt(int64(deployment.Status.AvailableReplicas), 10)}, started)
	}
	return &deployment, check(target, adapter.OutcomePass, "CoreDNS deployment is available", map[string]string{"component": name}, started)
}

func deploymentAvailable(deployment *appsv1.Deployment) bool {
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (Adapter) checkPods(ctx context.Context, c client.Client, deployment *appsv1.Deployment, restartWarnCount int32) []adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "v1", Kind: "Pod", Namespace: deployment.Namespace, Name: deployment.Name}
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return []adapter.CheckResult{check(target, adapter.OutcomeError, fmt.Sprintf("deployment has invalid pod selector: %v", err), nil, started)}
	}
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(deployment.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return []adapter.CheckResult{check(target, adapter.OutcomeError, fmt.Sprintf("failed to list CoreDNS pods: %v", err), nil, started)}
	}
	if len(pods.Items) == 0 {
		return []adapter.CheckResult{check(target, adapter.OutcomeFail, "CoreDNS deployment has no matching pods", nil, started)}
	}

	// Filter terminating (rolling-update) and Failed/Evicted pods before
	// grading: they still match the selector but are not serving candidates and
	// must not force a false Fail (#160). A live not-ready pod is Warn, not
	// Fail — checkDeployment already Fails on unmet capacity.
	live := make([]*corev1.Pod, 0, len(pods.Items))
	for i := range pods.Items {
		if podutil.Active(&pods.Items[i]) {
			live = append(live, &pods.Items[i])
		}
	}
	if len(live) == 0 {
		return []adapter.CheckResult{check(target, adapter.OutcomeSkipped, "CoreDNS deployment has only terminating, failed, or completed pods", nil, started)}
	}

	checks := make([]adapter.CheckResult, 0, len(live))
	for _, pod := range live {
		if !podReady(pod) {
			checks = append(checks, check(podTarget(pod), adapter.OutcomeWarn, "CoreDNS pod is not ready", map[string]string{"phase": string(pod.Status.Phase)}, started))
			continue
		}
		if restarts := maxRestartCount(pod); restarts > restartWarnCount {
			checks = append(checks, check(podTarget(pod), adapter.OutcomeWarn, "CoreDNS pod restart count exceeds warning threshold", map[string]string{"restartCount": strconv.FormatInt(int64(restarts), 10), "restartWarnCount": strconv.FormatInt(int64(restartWarnCount), 10)}, started))
			continue
		}
		checks = append(checks, check(podTarget(pod), adapter.OutcomePass, "CoreDNS pod is ready", nil, started))
	}
	return checks
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

func (Adapter) checkService(ctx context.Context, c client.Client, namespace, serviceName string) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "v1", Kind: "Service", Namespace: namespace, Name: serviceName}
	var service corev1.Service
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: serviceName}, &service); err != nil {
		if apierrors.IsNotFound(err) {
			return check(target, adapter.OutcomeFail, "CoreDNS service is missing", adapter.MarkAbsent(nil), started)
		}
		return check(target, adapter.OutcomeError, fmt.Sprintf("failed to read CoreDNS service: %v", err), nil, started)
	}
	if service.Spec.ClusterIP == "" || service.Spec.ClusterIP == corev1.ClusterIPNone {
		return check(target, adapter.OutcomeFail, "CoreDNS service has no cluster IP", map[string]string{"clusterIP": service.Spec.ClusterIP}, started)
	}
	return check(target, adapter.OutcomePass, "CoreDNS service is routable", map[string]string{"clusterIP": service.Spec.ClusterIP}, started)
}

func (Adapter) checkEndpointSlices(ctx context.Context, c client.Client, namespace, serviceName string) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "discovery.k8s.io/v1", Kind: "EndpointSlice", Namespace: namespace, Name: serviceName}
	var slices discoveryv1.EndpointSliceList
	if err := c.List(ctx, &slices, client.InNamespace(namespace), client.MatchingLabels{"kubernetes.io/service-name": serviceName}); err != nil {
		return check(target, adapter.OutcomeError, fmt.Sprintf("failed to list CoreDNS EndpointSlices: %v", err), nil, started)
	}
	ready := 0
	for _, slice := range slices.Items {
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready {
				ready++
			}
		}
	}
	if ready == 0 {
		return check(target, adapter.OutcomeFail, "CoreDNS service has no ready endpoints", map[string]string{"endpointSlices": strconv.Itoa(len(slices.Items))}, started)
	}
	return check(target, adapter.OutcomePass, "CoreDNS service has ready endpoints", map[string]string{"readyEndpoints": strconv.Itoa(ready), "endpointSlices": strconv.Itoa(len(slices.Items))}, started)
}

// checkDNSResolution runs one probe pod per configured target and maps each
// result back to a CheckResult. Per ADR-0003, resolving from inside a probe
// pod (rather than from the operator pod's network namespace) is the only
// way to assert workload-perspective DNS behavior.
func (a Adapter) checkDNSResolution(ctx context.Context, req adapter.Request, policy adapter.FamilyPolicy) []adapter.CheckResult {
	targets := dnsTargets(policy)
	image := resolveProbeImage(policy, req.ProbeImage)
	namespace := resolveProbeNamespace(policy, req.Target.Namespace)
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	if namespace == "" {
		// Bail out before any pod-build attempt so callers see one Skipped per
		// target with an actionable summary, not an opaque "probe namespace is
		// required" error per target from probe.Pod.
		checks := make([]adapter.CheckResult, 0, len(targets))
		for _, name := range targets {
			checks = append(checks, skipped(
				adapter.TargetRef{Kind: "DNSName", Name: name},
				"probe namespace is required; set the probeNamespace threshold or run the AddonCheck in a namespace",
			))
		}
		return checks
	}
	launcher := a.launcher
	if launcher == nil {
		launcher = &probe.Launcher{Client: req.Client}
	}
	checks := make([]adapter.CheckResult, 0, len(targets))
	for _, name := range targets {
		checks = append(checks, runDNSProbe(ctx, launcher, image, namespace, name, timeout))
	}
	return checks
}

// runDNSProbe launches a single probe pod for one target and returns its
// CheckResult. Probe-pod-create errors are surfaced as Outcome=Error;
// probe-binary outcomes (Pass/Fail/Error) flow through unchanged.
func runDNSProbe(ctx context.Context, launcher dnsProbeLauncher, image, namespace, target string, timeout time.Duration) adapter.CheckResult {
	started := time.Now()
	targetRef := adapter.TargetRef{Kind: "DNSName", Name: target}
	probeReq := probe.Request{
		Name:      dnsProbePodName(target),
		Namespace: namespace,
		Image:     image,
		Mode:      probe.ModeDNS,
		Target:    target,
		Timeout:   timeout,
	}
	result, err := launcher.Run(ctx, probeReq)
	duration := time.Since(started)
	details := map[string]string{
		"latencyMillis": strconv.FormatInt(duration.Milliseconds(), 10),
		"target":        target,
	}
	if err != nil {
		details["error"] = err.Error()
		return check(targetRef, adapter.OutcomeError, "DNS probe pod execution failed", details, started)
	}
	for k, v := range result.Details {
		// Don't let probe-side details overwrite our latency stamp; the
		// probe binary measured its own latency separately, surface both.
		if k == "latencyMillis" {
			details["probeLatencyMillis"] = v
			continue
		}
		details[k] = v
	}
	summary := result.Summary
	if summary == "" {
		summary = "DNS probe completed"
	}
	return check(targetRef, adapterOutcome(result.Outcome), summary, details, started)
}

// adapterOutcome maps a probe.Outcome (Pass/Fail/Error) to its adapter.Outcome
// equivalent. Probe outcomes outside the documented set surface as
// adapter.OutcomeError — we cannot trust an unrecognized result.
func adapterOutcome(o probe.Outcome) adapter.Outcome {
	switch o {
	case probe.OutcomePass:
		return adapter.OutcomePass
	case probe.OutcomeFail:
		return adapter.OutcomeFail
	case probe.OutcomeError:
		return adapter.OutcomeError
	default:
		return adapter.OutcomeError
	}
}

// dnsProbePodName builds a DNS-1123-compliant Pod name from a probe target.
// Length is bounded so the name + namespace stays within Kubernetes limits.
// The nanosecond suffix avoids collisions when the same target is probed
// from concurrent reconciles.
func dnsProbePodName(target string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(target) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	sanitized := strings.Trim(b.String(), "-")
	if len(sanitized) > probePodNameMaxLabel {
		sanitized = sanitized[:probePodNameMaxLabel]
		sanitized = strings.TrimRight(sanitized, "-")
	}
	if sanitized == "" {
		sanitized = "target"
	}
	return fmt.Sprintf("fathom-dns-%s-%s", sanitized, strconv.FormatInt(time.Now().UnixNano(), 36))
}

func firstNamespace(policy adapter.FamilyPolicy) string {
	if len(policy.Namespaces) == 0 {
		return defaultNamespace
	}
	return policy.Namespaces[0]
}

func dnsTargets(policy adapter.FamilyPolicy) []string {
	if policy.Thresholds == nil || policy.Thresholds[thresholdTargets] == "" {
		return []string{defaultDNSTargets}
	}
	parts := strings.Split(policy.Thresholds[thresholdTargets], ",")
	targets := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			targets = append(targets, part)
		}
	}
	if len(targets) == 0 {
		return []string{defaultDNSTargets}
	}
	return targets
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

// resolveProbeNamespace picks the namespace in which to launch probe pods.
// Precedence: per-AddonCheck probeNamespace threshold > AddonCheck namespace
// > "". Returning "" tells the caller no namespace could be resolved; the
// caller is expected to surface that as Skipped with a clear summary instead
// of letting downstream pod-build code fail per target.
//
// An operator-level default could slot between the threshold and the
// AddonCheck namespace in a future change without altering this signature.
func resolveProbeNamespace(policy adapter.FamilyPolicy, addonCheckNamespace string) string {
	if v := strings.TrimSpace(policy.Thresholds[thresholdProbeNamespace]); v != "" {
		return v
	}
	return strings.TrimSpace(addonCheckNamespace)
}

// resolveProbeImage implements the probe-image precedence chain:
// per-AddonCheck threshold → operator-level Request.ProbeImage → hardcoded
// fallback. The fallback is intentionally non-empty so a misconfigured
// operator yields a recognizable ImagePullBackOff rather than a Pod with no
// container image.
func resolveProbeImage(policy adapter.FamilyPolicy, operatorDefault string) string {
	if v := strings.TrimSpace(policy.Thresholds[thresholdProbeImage]); v != "" {
		return v
	}
	if v := strings.TrimSpace(operatorDefault); v != "" {
		return v
	}
	return fallbackProbeImage
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

func skipped(target adapter.TargetRef, summary string) adapter.CheckResult {
	return adapter.CheckResult{Family: FamilyDNSResolution, Outcome: adapter.OutcomeSkipped, TargetRef: target, Summary: summary, ObservedAt: time.Now()}
}

func check(target adapter.TargetRef, outcome adapter.Outcome, summary string, details map[string]string, started time.Time) adapter.CheckResult {
	return adapter.CheckResult{Family: familyForTarget(target), Outcome: outcome, TargetRef: target, Summary: summary, Details: details, ObservedAt: time.Now(), Duration: time.Since(started)}
}

func familyForTarget(target adapter.TargetRef) adapter.Family {
	if target.Kind == "DNSName" {
		return FamilyDNSResolution
	}
	return FamilySystemHealth
}
