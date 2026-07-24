/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package kubestatemetrics provides the built-in kube-state-metrics addon
// adapter (skaphos/fathom#189). It validates the two things a blind KSM
// silently breaks: the exporter workload itself (system_health) and the
// metrics it is supposed to produce (metrics_endpoint) — a KSM whose pods are
// Ready but whose /metrics endpoint serves nothing takes every alert built on
// kube_* series down with it.
package kubestatemetrics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

// tracer is the OpenTelemetry instrumentation scope for the kube-state-metrics
// adapter. It draws from the global provider, so it is a no-op unless the
// operator enabled tracing (SKA-293).
var tracer = otel.Tracer("github.com/skaphos/fathom/internal/adapter/kubestatemetrics")

const (
	Name                  = "kube-state-metrics"
	Version               = "0.1.0"
	FamilySystemHealth    = adapter.Family("system_health")
	FamilyMetricsEndpoint = adapter.Family("metrics_endpoint")

	defaultNamespace        = "kube-system"
	defaultWorkloadName     = "kube-state-metrics"
	defaultServiceName      = "kube-state-metrics"
	defaultRestartWarnCount = int32(3)
	// defaultMetricsPort is the main /metrics port both the canonical
	// kubernetes/kube-state-metrics manifests and the prometheus-community Helm
	// chart expose on the Service.
	defaultMetricsPort = int32(8080)
	// defaultTelemetryPort is KSM's self-telemetry port. The canonical
	// manifests expose it on the Service; the Helm chart only does so with
	// selfMonitor enabled — when the Service does not declare it, the
	// self-telemetry check is Skipped, not Failed (see checkMetricsEndpoint).
	defaultTelemetryPort = int32(8081)
	// defaultExpectedFamilies are metric families present on any real cluster.
	// Sharded installs may want to override this: the Service round-robins to a
	// single shard, and each shard serves only its object slice.
	defaultExpectedFamilies = "kube_node_info,kube_pod_info"
	// defaultTelemetryFamilies is the self-telemetry sanity assertion.
	// kube_state_metrics_build_info is registered unconditionally on the
	// telemetry registry at startup, so its absence means the endpoint is not
	// actually KSM's self-telemetry.
	defaultTelemetryFamilies = "kube_state_metrics_build_info"

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

	thresholdRestartWarnCount  = "restartWarnCount"
	thresholdWorkloadName      = "workloadName"
	thresholdServiceName       = "serviceName"
	thresholdMetricsPort       = "metricsPort"
	thresholdTelemetryPort     = "telemetryPort"
	thresholdExpectedFamilies  = "expectedFamilies"
	thresholdTelemetryFamilies = "telemetryFamilies"
	thresholdProbeImage        = "probeImage"
	thresholdProbeNamespace    = "probeNamespace"
)

// scrapeProbeLauncher is the surface the metrics_endpoint family needs from
// probe.Launcher. The interface exists so unit tests can supply a fake without
// a real client and without standing up a probe pod.
type scrapeProbeLauncher interface {
	Run(ctx context.Context, req probe.Request) (probe.Result, error)
}

// Adapter implements kube-state-metrics system and metrics-endpoint checks.
type Adapter struct {
	// launcher is injected for testing. Production runs construct a real
	// probe.Launcher per Run with the request's controller-runtime client.
	launcher scrapeProbeLauncher
}

// New returns the built-in kube-state-metrics adapter.
func New() Adapter { return Adapter{} }

func (Adapter) Name() string            { return Name }
func (Adapter) Version() string         { return Version }
func (Adapter) ContractVersion() string { return adapter.ContractVersion }

func (Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{Name}, Families: []adapter.Family{FamilySystemHealth, FamilyMetricsEndpoint}}
}

// RBACRules declares kube-state-metrics's least-privilege grants
// (adapter.RBACDeclarer): the passive workload/service reads, plus the
// probe-pod lifecycle write the metrics-endpoint scrape performs — all under
// this adapter's impersonated ServiceAccount (SKA-58). Each rule carries a
// defensive Justification; the generator emits a scoped ClusterRole and
// renders the justifications into docs/reference/rbac.md, and the guard
// permits the pods create;delete only because it is justified.
func (Adapter) RBACRules() []adapter.PolicyRule {
	return []adapter.PolicyRule{
		{APIGroups: []string{"apps"}, Resources: []string{"deployments", "statefulsets"}, Verbs: []string{"get"},
			Justification: "Get the kube-state-metrics workload by name to score readiness: the Deployment on standard installs, falling back to the same-named StatefulSet on autosharded installs (every shard must be ready). Both reads are single named Gets, so get without list/watch is sufficient; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"},
			Justification: "List the kube-state-metrics Pods by label selector for restart counts and readiness behind the workload (list, because Pod names are dynamic), and Get the short-lived scrape probe Pod while polling it to completion. Read-only."},
		{APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"get"},
			Justification: "Get the kube-state-metrics Service by name to confirm which scrape ports (metrics, self-telemetry) are actually exposed before probing them — this is what lets a not-exposed telemetry port be Skipped instead of falsely Failed. A single named Get; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"create", "delete"},
			Justification: "WRITE EXCEPTION: launch and immediately tear down a single-shot HTTP scrape probe Pod per endpoint to verify /metrics is scrapeable from a workload's perspective (ADR-0003). create+delete are the minimal verbs for an ephemeral Pod; the operator pod's own network namespace cannot honestly stand in for in-cluster scrape reachability. The Pod is deleted as soon as it completes — no update, no long-lived workload."},
	}
}

// kube-state-metrics's cluster permissions (including the probe-pod
// create;delete) are NOT granted to the operator ServiceAccount. They live on
// this adapter's per-addon ServiceAccount (RBACRules above), generated into
// config/rbac/addons/addon-kube-state-metrics.yaml; the operator only
// impersonates that ServiceAccount at run time (SKA-58). No +kubebuilder:rbac
// markers here.

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
	if endpointPolicy, enabled := familyPolicy(req.Policy, FamilyMetricsEndpoint, true); enabled {
		familyStart := time.Now()
		familyChecks := a.checkMetricsEndpoint(ctx, req, endpointPolicy)
		checks = append(checks, familyChecks...)
		metrics.RecordAdapterRun(Name, string(FamilyMetricsEndpoint), string(adapter.FamilyOutcome(familyChecks, FamilyMetricsEndpoint)), time.Since(familyStart))
	}
	if len(checks) == 0 {
		checks = append(checks, skipped(FamilySystemHealth, req.Target, "all kube-state-metrics check families are disabled by policy"))
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

// checkSystemHealth scores the exporter workload. Standard installs run a
// Deployment; the Helm chart's autosharding mode runs a same-named StatefulSet
// whose replicas are the shards — so a NotFound Deployment falls back to the
// StatefulSet, and "every shard ready" is readyReplicas == desired.
func (a Adapter) checkSystemHealth(ctx context.Context, c client.Client, policy adapter.FamilyPolicy) []adapter.CheckResult {
	namespace := firstNamespace(policy)
	workloadName := stringThreshold(policy, thresholdWorkloadName, defaultWorkloadName)
	restartWarnCount := int32Threshold(policy, thresholdRestartWarnCount, defaultRestartWarnCount)

	checks := []adapter.CheckResult{}
	selector, podNamespace, check := a.checkWorkload(ctx, c, namespace, workloadName)
	checks = append(checks, check)
	if selector != nil {
		checks = append(checks, a.checkPods(ctx, c, podNamespace, selector, workloadName, restartWarnCount)...)
	}
	return checks
}

// checkWorkload reads the Deployment (standard install) or, when it does not
// exist, the same-named StatefulSet (sharded install). It returns the pod
// selector and namespace for the pod sub-check when the workload was found and
// schedules pods, or nil to skip it.
func (Adapter) checkWorkload(ctx context.Context, c client.Client, namespace, name string) (*metav1.LabelSelector, string, adapter.CheckResult) {
	started := time.Now()
	key := types.NamespacedName{Namespace: namespace, Name: name}
	details := map[string]string{"component": name}

	var deployment appsv1.Deployment
	err := c.Get(ctx, key, &deployment)
	if err == nil {
		target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: namespace, Name: name}
		desired := int32(1)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}
		if desired == 0 {
			return nil, "", result(FamilySystemHealth, target, adapter.OutcomeWarn, "kube-state-metrics deployment is scaled to zero", details, started)
		}
		details["desiredReplicas"] = strconv.FormatInt(int64(desired), 10)
		details["availableReplicas"] = strconv.FormatInt(int64(deployment.Status.AvailableReplicas), 10)
		if !deploymentAvailable(&deployment) || deployment.Status.AvailableReplicas < desired {
			return deployment.Spec.Selector, deployment.Namespace, result(FamilySystemHealth, target, adapter.OutcomeFail, "kube-state-metrics deployment is not fully available", details, started)
		}
		return deployment.Spec.Selector, deployment.Namespace, result(FamilySystemHealth, target, adapter.OutcomePass, "kube-state-metrics deployment is available", details, started)
	}
	if !apierrors.IsNotFound(err) {
		target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: namespace, Name: name}
		return nil, "", result(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("failed to read kube-state-metrics deployment: %v", err), details, started)
	}

	// No Deployment: sharded installs (Helm autosharding) run a same-named
	// StatefulSet where each replica is one shard of the object space. A shard
	// that is not ready silently drops its slice of every kube_* series, so
	// anything short of all shards ready is a Fail, not a Warn.
	var sts appsv1.StatefulSet
	stsErr := c.Get(ctx, key, &sts)
	if stsErr == nil {
		target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "StatefulSet", Namespace: namespace, Name: name}
		details["sharded"] = "true"
		desired := int32(1)
		if sts.Spec.Replicas != nil {
			desired = *sts.Spec.Replicas
		}
		if desired == 0 {
			return nil, "", result(FamilySystemHealth, target, adapter.OutcomeWarn, "kube-state-metrics statefulset is scaled to zero", details, started)
		}
		details["desiredShards"] = strconv.FormatInt(int64(desired), 10)
		details["readyShards"] = strconv.FormatInt(int64(sts.Status.ReadyReplicas), 10)
		if sts.Status.ReadyReplicas < desired {
			return sts.Spec.Selector, sts.Namespace, result(FamilySystemHealth, target, adapter.OutcomeFail, "kube-state-metrics statefulset does not have every shard ready", details, started)
		}
		return sts.Spec.Selector, sts.Namespace, result(FamilySystemHealth, target, adapter.OutcomePass, "kube-state-metrics statefulset has every shard ready", details, started)
	}

	target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: namespace, Name: name}
	if apierrors.IsNotFound(stsErr) {
		return nil, "", result(FamilySystemHealth, target, adapter.OutcomeFail, "kube-state-metrics workload is missing (no Deployment or StatefulSet)", adapter.MarkAbsent(details), started)
	}
	return nil, "", result(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("failed to read kube-state-metrics statefulset: %v", stsErr), details, started)
}

func deploymentAvailable(deployment *appsv1.Deployment) bool {
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// checkPods is the selector -> ready -> restart-warn sub-check shared with the
// other workload adapters: terminating and Failed/Evicted pods are filtered
// before grading (#160), a live not-ready pod is Warn (the workload check is
// the authoritative outage signal), and restarts over the threshold Warn.
func (Adapter) checkPods(ctx context.Context, c client.Client, namespace string, selector *metav1.LabelSelector, component string, restartWarnCount int32) []adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "v1", Kind: "Pod", Namespace: namespace, Name: component}
	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return []adapter.CheckResult{result(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("workload has invalid pod selector: %v", err), nil, started)}
	}
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: sel}); err != nil {
		return []adapter.CheckResult{result(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("failed to list kube-state-metrics pods: %v", err), nil, started)}
	}
	if len(pods.Items) == 0 {
		return []adapter.CheckResult{result(FamilySystemHealth, target, adapter.OutcomeFail, "kube-state-metrics workload has no matching pods", nil, started)}
	}

	live := make([]*corev1.Pod, 0, len(pods.Items))
	for i := range pods.Items {
		if podutil.Active(&pods.Items[i]) {
			live = append(live, &pods.Items[i])
		}
	}
	if len(live) == 0 {
		return []adapter.CheckResult{result(FamilySystemHealth, target, adapter.OutcomeSkipped, "kube-state-metrics workload has only terminating, failed, or completed pods", nil, started)}
	}

	checks := make([]adapter.CheckResult, 0, len(live))
	for _, pod := range live {
		if !podReady(pod) {
			checks = append(checks, result(FamilySystemHealth, podTarget(pod), adapter.OutcomeWarn, "kube-state-metrics pod is not ready", map[string]string{"phase": string(pod.Status.Phase)}, started))
			continue
		}
		if restarts := maxRestartCount(pod); restarts > restartWarnCount {
			checks = append(checks, result(FamilySystemHealth, podTarget(pod), adapter.OutcomeWarn, "kube-state-metrics pod restart count exceeds warning threshold", map[string]string{"restartCount": strconv.FormatInt(int64(restarts), 10), "restartWarnCount": strconv.FormatInt(int64(restartWarnCount), 10)}, started))
			continue
		}
		checks = append(checks, result(FamilySystemHealth, podTarget(pod), adapter.OutcomePass, "kube-state-metrics pod is ready", nil, started))
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

// checkMetricsEndpoint verifies the exporter's output: the main /metrics
// endpoint is scrapeable through the Service and carries the expected metric
// families, and — when the Service exposes the telemetry port — KSM's
// self-telemetry endpoint serves its own kube_state_metrics_* metrics. Both
// scrapes run as short-lived probe pods (ADR-0003), so reachability is
// asserted from a workload's network perspective, bounded by the request
// timeout.
func (a Adapter) checkMetricsEndpoint(ctx context.Context, req adapter.Request, policy adapter.FamilyPolicy) []adapter.CheckResult {
	started := time.Now()
	namespace := firstNamespace(policy)
	serviceName := stringThreshold(policy, thresholdServiceName, defaultServiceName)
	metricsPort := int32Threshold(policy, thresholdMetricsPort, defaultMetricsPort)
	telemetryPort := int32Threshold(policy, thresholdTelemetryPort, defaultTelemetryPort)
	expected := csvThreshold(policy, thresholdExpectedFamilies, defaultExpectedFamilies)
	telemetryExpected := csvThreshold(policy, thresholdTelemetryFamilies, defaultTelemetryFamilies)

	// The Service is the scrape contract: read it first so a missing endpoint
	// is reported precisely (absent Service vs. not-exposed port) instead of as
	// an opaque connection failure from inside a probe pod.
	serviceTarget := adapter.TargetRef{APIVersion: "v1", Kind: "Service", Namespace: namespace, Name: serviceName}
	var service corev1.Service
	if err := req.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: serviceName}, &service); err != nil {
		if apierrors.IsNotFound(err) {
			return []adapter.CheckResult{result(FamilyMetricsEndpoint, serviceTarget, adapter.OutcomeFail, "kube-state-metrics service is missing", adapter.MarkAbsent(nil), started)}
		}
		return []adapter.CheckResult{result(FamilyMetricsEndpoint, serviceTarget, adapter.OutcomeError, fmt.Sprintf("failed to read kube-state-metrics service: %v", err), nil, started)}
	}

	checks := []adapter.CheckResult{}
	if !servicePortDeclared(&service, metricsPort) {
		checks = append(checks, result(FamilyMetricsEndpoint, endpointTarget(namespace, serviceName, metricsPort), adapter.OutcomeFail,
			fmt.Sprintf("kube-state-metrics service does not expose the metrics port %d", metricsPort),
			map[string]string{"component": "metrics", "servicePorts": servicePortList(&service)}, started))
	} else {
		checks = append(checks, a.scrapeCheck(ctx, req, policy, namespace, serviceName, metricsPort, "metrics", expected))
	}

	switch {
	case telemetryPort == 0:
		checks = append(checks, skipped(FamilyMetricsEndpoint, endpointTarget(namespace, serviceName, telemetryPort),
			"self-telemetry check is disabled (telemetryPort threshold is 0)"))
	case !servicePortDeclared(&service, telemetryPort):
		// The Helm chart only exposes the telemetry port with selfMonitor
		// enabled, so a not-exposed port is a legitimate install shape —
		// Skipped, not Fail. Set telemetryPort to 0 to silence the skip.
		checks = append(checks, skipped(FamilyMetricsEndpoint, endpointTarget(namespace, serviceName, telemetryPort),
			fmt.Sprintf("kube-state-metrics service does not expose the self-telemetry port %d; expose it (e.g. Helm selfMonitor.enabled) or set the telemetryPort threshold to 0", telemetryPort)))
	default:
		checks = append(checks, a.scrapeCheck(ctx, req, policy, namespace, serviceName, telemetryPort, "telemetry", telemetryExpected))
	}
	return checks
}

// scrapeCheck launches one http-get probe pod against the service endpoint and
// maps its result back to a CheckResult. Probe-pod-create errors surface as
// Outcome=Error; probe-binary outcomes (Pass/Fail/Error) flow through
// unchanged.
func (a Adapter) scrapeCheck(ctx context.Context, req adapter.Request, policy adapter.FamilyPolicy, namespace, serviceName string, port int32, component string, expect []string) adapter.CheckResult {
	started := time.Now()
	target := endpointTarget(namespace, serviceName, port)
	probeNamespace := resolveProbeNamespace(policy, req.Target.Namespace)
	if probeNamespace == "" {
		return skipped(FamilyMetricsEndpoint, target,
			"probe namespace is required; set the probeNamespace threshold or run the AddonCheck in a namespace")
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	launcher := a.launcher
	if launcher == nil {
		launcher = &probe.Launcher{Client: req.Client}
	}

	// <service>.<namespace>.svc resolves through the probe pod's search path on
	// any cluster domain, so the URL does not hardcode cluster.local.
	url := fmt.Sprintf("http://%s.%s.svc:%d/metrics", serviceName, namespace, port)
	probeReq := probe.Request{
		Name:      scrapeProbePodName(component),
		Namespace: probeNamespace,
		Image:     resolveProbeImage(policy, req.ProbeImage),
		Mode:      probe.ModeHTTPGet,
		Target:    url,
		Expect:    expect,
		Timeout:   timeout,
	}
	probeResult, err := launcher.Run(ctx, probeReq)
	duration := time.Since(started)
	details := map[string]string{
		"component":     component,
		"endpoint":      url,
		"latencyMillis": strconv.FormatInt(duration.Milliseconds(), 10),
	}
	if len(expect) > 0 {
		details["expectedFamilies"] = strings.Join(expect, ",")
	}
	if err != nil {
		details["error"] = err.Error()
		return result(FamilyMetricsEndpoint, target, adapter.OutcomeError, "metrics scrape probe pod execution failed", details, started)
	}
	for k, v := range probeResult.Details {
		// Don't let probe-side details overwrite our latency stamp; the probe
		// binary measured its own latency separately, surface both.
		if k == "latencyMillis" {
			details["probeLatencyMillis"] = v
			continue
		}
		details[k] = v
	}
	summary := probeResult.Summary
	if summary == "" {
		summary = "metrics scrape completed"
	}
	return result(FamilyMetricsEndpoint, target, adapterOutcome(probeResult.Outcome), summary, details, started)
}

// endpointTarget names one scrape endpoint deterministically
// ("<service>:<port>") so the two metrics_endpoint scrapes never collide on a
// TargetRef.
func endpointTarget(namespace, serviceName string, port int32) adapter.TargetRef {
	return adapter.TargetRef{Kind: "MetricsEndpoint", Namespace: namespace, Name: fmt.Sprintf("%s:%d", serviceName, port)}
}

func servicePortDeclared(service *corev1.Service, port int32) bool {
	for _, p := range service.Spec.Ports {
		if p.Port == port {
			return true
		}
	}
	return false
}

func servicePortList(service *corev1.Service) string {
	ports := make([]string, 0, len(service.Spec.Ports))
	for _, p := range service.Spec.Ports {
		ports = append(ports, strconv.FormatInt(int64(p.Port), 10))
	}
	return strings.Join(ports, ",")
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

// scrapeProbePodName builds a DNS-1123-compliant Pod name for one scrape
// probe. The nanosecond suffix avoids collisions when the same endpoint is
// probed from concurrent reconciles.
func scrapeProbePodName(component string) string {
	sanitized := strings.ToLower(component)
	if len(sanitized) > probePodNameMaxLabel {
		sanitized = sanitized[:probePodNameMaxLabel]
	}
	return fmt.Sprintf("fathom-ksm-%s-%s", sanitized, strconv.FormatInt(time.Now().UnixNano(), 36))
}

func firstNamespace(policy adapter.FamilyPolicy) string {
	if len(policy.Namespaces) == 0 {
		return defaultNamespace
	}
	return policy.Namespaces[0]
}

// resolveProbeNamespace picks the namespace in which to launch probe pods.
// Precedence: per-AddonCheck probeNamespace threshold > AddonCheck namespace
// > "". Returning "" tells the caller no namespace could be resolved; the
// caller surfaces that as Skipped with a clear summary.
func resolveProbeNamespace(policy adapter.FamilyPolicy, addonCheckNamespace string) string {
	if v := strings.TrimSpace(policy.Thresholds[thresholdProbeNamespace]); v != "" {
		return v
	}
	return strings.TrimSpace(addonCheckNamespace)
}

// resolveProbeImage implements the probe-image precedence chain:
// per-AddonCheck threshold -> operator-level Request.ProbeImage -> hardcoded
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

func int32Threshold(policy adapter.FamilyPolicy, key string, defaultValue int32) int32 {
	if policy.Thresholds == nil {
		return defaultValue
	}
	value, ok := policy.Thresholds[key]
	if !ok {
		return defaultValue
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil || parsed < 0 {
		return defaultValue
	}
	return int32(parsed)
}

// csvThreshold parses a comma-separated threshold into a trimmed slice,
// falling back to defaultValue when the threshold is unset or empty.
func csvThreshold(policy adapter.FamilyPolicy, key, defaultValue string) []string {
	raw := stringThreshold(policy, key, defaultValue)
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func skipped(family adapter.Family, target adapter.TargetRef, summary string) adapter.CheckResult {
	return adapter.CheckResult{Family: family, Outcome: adapter.OutcomeSkipped, TargetRef: target, Summary: summary, ObservedAt: time.Now()}
}

func result(family adapter.Family, target adapter.TargetRef, outcome adapter.Outcome, summary string, details map[string]string, started time.Time) adapter.CheckResult {
	return adapter.CheckResult{Family: family, Outcome: outcome, TargetRef: target, Summary: summary, Details: details, ObservedAt: time.Now(), Duration: time.Since(started)}
}
