/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package nodelocaldns provides the built-in NodeLocal DNSCache addon adapter.
//
// NodeLocal DNSCache runs a per-node DNS cache DaemonSet that sits on the DNS
// data path of every pod on the node (kubelet points pod resolv.conf at the
// cache's link-local listen address). A node whose cache pod is down loses DNS
// for every workload on that node even while the cluster-level CoreDNS
// Deployment is fully healthy — which is exactly the blind spot the CoreDNS
// adapter cannot see. Two families cover it:
//
//   - system_health — the node-local-dns DaemonSet is Ready on every
//     schedulable node, with per-node gap detection: the check names the
//     schedulable nodes that lack a ready cache pod instead of reporting a
//     bare count mismatch.
//   - dns_resolution — resolution through the node-local cache itself, via
//     the shared probe-pod foundation (internal/probe, ADR-0003). The probe
//     pod's resolver is pinned to the cache's listen address (dnsPolicy None),
//     so the query is answered by the node-local cache, not by whatever the
//     cluster's default resolver path happens to be.
package nodelocaldns

import (
	"context"
	"fmt"
	"sort"
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

// tracer is the OpenTelemetry instrumentation scope for the NodeLocal DNSCache
// adapter. It draws from the global provider, so it is a no-op unless the
// operator enabled tracing.
var tracer = otel.Tracer("github.com/skaphos/fathom/internal/adapter/nodelocaldns")

const (
	Name                = "node-local-dns"
	Version             = "0.1.0"
	FamilySystemHealth  = adapter.Family("system_health")
	FamilyDNSResolution = adapter.Family("dns_resolution")

	defaultNamespace        = "kube-system"
	defaultDaemonSetName    = "node-local-dns"
	defaultRestartWarnCount = int32(3)
	defaultDNSTargets       = "kubernetes.default.svc.cluster.local"
	// defaultListenAddress is the upstream NodeLocal DNSCache convention: the
	// link-local IP the cache binds on a per-node dummy interface
	// (kubernetes.io/docs/tasks/administer-cluster/nodelocaldns). Distributions
	// that pick another address override it via the listenAddress threshold.
	defaultListenAddress = "169.254.20.10"
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
	// maxListedNodes bounds how many gap-node names the coverage check embeds
	// in its details, so a large-cluster outage cannot balloon a CheckResult.
	maxListedNodes = 10

	thresholdRestartWarnCount = "restartWarnCount"
	thresholdDaemonSetName    = "daemonSetName"
	thresholdListenAddress    = "listenAddress"
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

// Adapter implements NodeLocal DNSCache system and DNS behavior checks.
type Adapter struct {
	// launcher is injected for testing. Production runs construct a real
	// probe.Launcher per Run with the request's controller-runtime client.
	launcher dnsProbeLauncher
}

// New returns the built-in NodeLocal DNSCache adapter.
func New() Adapter { return Adapter{} }

func (Adapter) Name() string            { return Name }
func (Adapter) Version() string         { return Version }
func (Adapter) ContractVersion() string { return adapter.ContractVersion }

func (Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{Name}, Families: []adapter.Family{FamilySystemHealth, FamilyDNSResolution}}
}

// RBACRules declares NodeLocal DNSCache's least-privilege grants
// (adapter.RBACDeclarer): the passive DaemonSet/pod/node reads, plus the
// probe-pod lifecycle write the DNS-resolution check performs — all under this
// adapter's impersonated ServiceAccount (SKA-58). Each rule carries a defensive
// Justification; the generator emits a scoped ClusterRole and renders the
// justifications into docs/reference/rbac.md, and the guard permits the pods
// create;delete only because it is justified.
func (Adapter) RBACRules() []adapter.PolicyRule {
	return []adapter.PolicyRule{
		{APIGroups: []string{"apps"}, Resources: []string{"daemonsets"}, Verbs: []string{"get"},
			Justification: "Get the node-local-dns DaemonSet by name to score rollout readiness and to obtain its pod selector. get only — the name is known (policy-overridable threshold) and the impersonating client is cache-free, so no list/watch; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"},
			Justification: "List the cache pods behind the DaemonSet (readiness, restart counts, per-node placement) and get the short-lived DNS probe pod while polling for its terminal state. list is required because pod names are dynamic; read-only."},
		{APIGroups: []string{""}, Resources: []string{"nodes"}, Verbs: []string{"list"},
			Justification: "List Nodes for per-node gap detection: naming the schedulable nodes that lack a ready cache pod, not just a count mismatch. list only — node names are not known in advance; only node metadata and spec.unschedulable are read."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"create", "delete"},
			Justification: "WRITE EXCEPTION: launch and immediately tear down a single-shot DNS probe Pod per check, its resolver pinned to the node-local cache's listen address, to measure resolution through the cache from a workload's perspective (ADR-0003). create+delete are the minimal verbs for an ephemeral Pod; no in-process read can observe the node-local DNS data path. The Pod is deleted as soon as it completes — no update, no long-lived workload."},
	}
}

// NodeLocal DNSCache's cluster permissions (including the probe-pod
// create;delete) are NOT granted to the operator ServiceAccount. They live on
// this adapter's per-addon ServiceAccount (RBACRules above), generated into
// config/rbac/addons/addon-node-local-dns.yaml; the operator only impersonates
// that ServiceAccount at run time (SKA-58). No +kubebuilder:rbac markers here.

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
		checks = append(checks, skipped(FamilySystemHealth, req.Target, "all NodeLocal DNSCache check families are disabled by policy"))
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

func (a Adapter) checkSystemHealth(ctx context.Context, c client.Client, policy adapter.FamilyPolicy) []adapter.CheckResult {
	namespace := firstNamespace(policy)
	daemonSetName := stringThreshold(policy, thresholdDaemonSetName, defaultDaemonSetName)
	restartWarnCount := int32Threshold(policy, thresholdRestartWarnCount, defaultRestartWarnCount)
	checks := []adapter.CheckResult{}
	daemonSet, check := a.checkDaemonSet(ctx, c, namespace, daemonSetName)
	checks = append(checks, check)
	if daemonSet == nil {
		return checks
	}
	if daemonSet.Status.DesiredNumberScheduled == 0 {
		// Zero-scheduled is graded Warn above (no matching nodes). Pod and
		// coverage checks would pile Fail results on top and flip the family
		// to Fail, contradicting that grading — stop at the Warn.
		return checks
	}
	pods, podChecks := a.checkPods(ctx, c, daemonSet, restartWarnCount)
	checks = append(checks, podChecks...)
	checks = append(checks, a.checkNodeCoverage(ctx, c, daemonSet, pods))
	return checks
}

// checkDaemonSet grades the node-local-dns DaemonSet. A missing DaemonSet is a
// Fail (with the absent marker), not a Skipped: once kubelet points pod
// resolv.conf at the node-local listen address, an absent cache is a DNS
// outage for every pod on every node — the exact failure this adapter exists
// to catch. Grading mirrors the declarative DaemonSet semantics: zero
// scheduled → Warn, unready → Fail, mid-rollout → Warn.
func (Adapter) checkDaemonSet(ctx context.Context, c client.Client, namespace, name string) (*appsv1.DaemonSet, adapter.CheckResult) {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "DaemonSet", Namespace: namespace, Name: name}
	var daemonSet appsv1.DaemonSet
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &daemonSet); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, check(target, adapter.OutcomeFail, "NodeLocal DNSCache DaemonSet is missing", adapter.MarkAbsent(map[string]string{"component": name}), started)
		}
		return nil, check(target, adapter.OutcomeError, fmt.Sprintf("failed to read NodeLocal DNSCache DaemonSet: %v", err), map[string]string{"component": name}, started)
	}
	status := daemonSet.Status
	details := map[string]string{
		"component":              name,
		"desiredNumberScheduled": strconv.FormatInt(int64(status.DesiredNumberScheduled), 10),
		"numberReady":            strconv.FormatInt(int64(status.NumberReady), 10),
		"numberUnavailable":      strconv.FormatInt(int64(status.NumberUnavailable), 10),
		"updatedNumberScheduled": strconv.FormatInt(int64(status.UpdatedNumberScheduled), 10),
	}
	// Order is load-bearing: zero-scheduled (Warn) precedes unready (Fail)
	// precedes mid-rollout (Warn).
	if status.DesiredNumberScheduled == 0 {
		return &daemonSet, check(target, adapter.OutcomeWarn, "NodeLocal DNSCache DaemonSet schedules zero pods (no matching nodes)", details, started)
	}
	if status.NumberUnavailable > 0 || status.NumberReady < status.DesiredNumberScheduled {
		return &daemonSet, check(target, adapter.OutcomeFail, "NodeLocal DNSCache DaemonSet is not ready on every scheduled node", details, started)
	}
	if status.UpdatedNumberScheduled < status.DesiredNumberScheduled {
		return &daemonSet, check(target, adapter.OutcomeWarn, "NodeLocal DNSCache DaemonSet rollout is in progress", details, started)
	}
	return &daemonSet, check(target, adapter.OutcomePass, "NodeLocal DNSCache DaemonSet is fully ready", details, started)
}

// checkPods grades the individual cache pods (readiness, restart counts) and
// returns the listed pods so checkNodeCoverage can reuse the same observation
// instead of listing twice.
func (Adapter) checkPods(ctx context.Context, c client.Client, daemonSet *appsv1.DaemonSet, restartWarnCount int32) ([]corev1.Pod, []adapter.CheckResult) {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "v1", Kind: "Pod", Namespace: daemonSet.Namespace, Name: daemonSet.Name}
	selector, err := metav1.LabelSelectorAsSelector(daemonSet.Spec.Selector)
	if err != nil {
		return nil, []adapter.CheckResult{check(target, adapter.OutcomeError, fmt.Sprintf("DaemonSet has invalid pod selector: %v", err), nil, started)}
	}
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(daemonSet.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, []adapter.CheckResult{check(target, adapter.OutcomeError, fmt.Sprintf("failed to list NodeLocal DNSCache pods: %v", err), nil, started)}
	}
	if len(pods.Items) == 0 {
		return pods.Items, []adapter.CheckResult{check(target, adapter.OutcomeFail, "NodeLocal DNSCache DaemonSet has no matching pods", nil, started)}
	}

	// Filter terminating and Failed/Evicted pods before grading: they still
	// match the selector but are not serving candidates and must not force a
	// false Fail. A live not-ready pod is Warn, not Fail — checkDaemonSet
	// already Fails on unmet capacity, and checkNodeCoverage Fails on the
	// nodes left uncovered.
	live := make([]*corev1.Pod, 0, len(pods.Items))
	for i := range pods.Items {
		if podutil.Active(&pods.Items[i]) {
			live = append(live, &pods.Items[i])
		}
	}
	if len(live) == 0 {
		return pods.Items, []adapter.CheckResult{check(target, adapter.OutcomeSkipped, "NodeLocal DNSCache DaemonSet has only terminating, failed, or completed pods", nil, started)}
	}

	checks := make([]adapter.CheckResult, 0, len(live))
	for _, pod := range live {
		if !podReady(pod) {
			checks = append(checks, check(podTarget(pod), adapter.OutcomeWarn, "NodeLocal DNSCache pod is not ready", map[string]string{"phase": string(pod.Status.Phase), "node": pod.Spec.NodeName}, started))
			continue
		}
		if restarts := maxRestartCount(pod); restarts > restartWarnCount {
			checks = append(checks, check(podTarget(pod), adapter.OutcomeWarn, "NodeLocal DNSCache pod restart count exceeds warning threshold", map[string]string{"restartCount": strconv.FormatInt(int64(restarts), 10), "restartWarnCount": strconv.FormatInt(int64(restartWarnCount), 10), "node": pod.Spec.NodeName}, started))
			continue
		}
		checks = append(checks, check(podTarget(pod), adapter.OutcomePass, "NodeLocal DNSCache pod is ready", map[string]string{"node": pod.Spec.NodeName}, started))
	}
	return pods.Items, checks
}

// checkNodeCoverage is the per-node gap detection: every schedulable node must
// host a ready cache pod. It names the uncovered nodes (bounded by
// maxListedNodes) instead of reporting a bare count mismatch, so an operator
// can go straight to the broken nodes. Cordoned nodes are excluded — their
// workloads are being drained away, so an unready cache there is not a
// data-path gap worth failing on.
func (Adapter) checkNodeCoverage(ctx context.Context, c client.Client, daemonSet *appsv1.DaemonSet, pods []corev1.Pod) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "v1", Kind: "Node", Name: "nodes"}
	var nodes corev1.NodeList
	if err := c.List(ctx, &nodes); err != nil {
		return check(target, adapter.OutcomeError, fmt.Sprintf("failed to list nodes for coverage detection: %v", err), nil, started)
	}
	covered := map[string]bool{}
	for i := range pods {
		pod := &pods[i]
		if podutil.Active(pod) && podReady(pod) && pod.Spec.NodeName != "" {
			covered[pod.Spec.NodeName] = true
		}
	}
	schedulable := 0
	missing := []string{}
	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue
		}
		schedulable++
		if !covered[node.Name] {
			missing = append(missing, node.Name)
		}
	}
	details := map[string]string{
		"component":        daemonSet.Name,
		"schedulableNodes": strconv.Itoa(schedulable),
		"coveredNodes":     strconv.Itoa(schedulable - len(missing)),
	}
	if schedulable == 0 {
		return check(target, adapter.OutcomeSkipped, "cluster has no schedulable nodes to cover", details, started)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		details["missingNodeCount"] = strconv.Itoa(len(missing))
		details["missingNodes"] = boundedNodeList(missing)
		return check(target, adapter.OutcomeFail,
			fmt.Sprintf("%d of %d schedulable node(s) lack a ready NodeLocal DNSCache pod", len(missing), schedulable), details, started)
	}
	return check(target, adapter.OutcomePass, "every schedulable node hosts a ready NodeLocal DNSCache pod", details, started)
}

// boundedNodeList renders up to maxListedNodes names, appending a "+N more"
// marker for the remainder so the detail stays bounded on large clusters.
func boundedNodeList(names []string) string {
	if len(names) <= maxListedNodes {
		return strings.Join(names, ",")
	}
	return strings.Join(names[:maxListedNodes], ",") + fmt.Sprintf(",+%d more", len(names)-maxListedNodes)
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
	var maxRestarts int32
	for _, status := range pod.Status.ContainerStatuses {
		if status.RestartCount > maxRestarts {
			maxRestarts = status.RestartCount
		}
	}
	return maxRestarts
}

func podTarget(pod *corev1.Pod) adapter.TargetRef {
	return adapter.TargetRef{APIVersion: "v1", Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name}
}

// checkDNSResolution runs one probe pod per configured target with the pod's
// resolver pinned to the node-local listen address, and maps each result back
// to a CheckResult. Per ADR-0003, resolving from inside a probe pod is the
// only way to assert the node-local data path a workload actually uses.
//
// Because the probe pod runs with dnsPolicy None (no cluster search domains),
// targets must be fully qualified names — the default target is.
func (a Adapter) checkDNSResolution(ctx context.Context, req adapter.Request, policy adapter.FamilyPolicy) []adapter.CheckResult {
	targets := dnsTargets(policy)
	image := resolveProbeImage(policy, req.ProbeImage)
	namespace := resolveProbeNamespace(policy, req.Target.Namespace)
	listenAddress := stringThreshold(policy, thresholdListenAddress, defaultListenAddress)
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
				FamilyDNSResolution,
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
		checks = append(checks, runDNSProbe(ctx, launcher, image, namespace, name, listenAddress, timeout))
	}
	return checks
}

// runDNSProbe launches a single probe pod for one target and returns its
// CheckResult. Probe-pod-create errors are surfaced as Outcome=Error;
// probe-binary outcomes (Pass/Fail/Error) flow through unchanged.
func runDNSProbe(ctx context.Context, launcher dnsProbeLauncher, image, namespace, target, listenAddress string, timeout time.Duration) adapter.CheckResult {
	started := time.Now()
	targetRef := adapter.TargetRef{Kind: "DNSName", Name: target}
	probeReq := probe.Request{
		Name:      dnsProbePodName(target),
		Namespace: namespace,
		Image:     image,
		Mode:      probe.ModeDNS,
		Target:    target,
		Timeout:   timeout,
		// Pinning the resolver is what makes this the node-local check: the
		// query is answered by the cache at listenAddress, not by whatever
		// resolver the pod would otherwise inherit.
		DNSNameservers: []string{listenAddress},
	}
	result, err := launcher.Run(ctx, probeReq)
	duration := time.Since(started)
	details := map[string]string{
		"latencyMillis": strconv.FormatInt(duration.Milliseconds(), 10),
		"target":        target,
		"nameserver":    listenAddress,
	}
	if err != nil {
		details["error"] = err.Error()
		return check(targetRef, adapter.OutcomeError, "node-local DNS probe pod execution failed", details, started)
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
		summary = "node-local DNS probe completed"
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
	return fmt.Sprintf("fathom-nldns-%s-%s", sanitized, strconv.FormatInt(time.Now().UnixNano(), 36))
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

func skipped(family adapter.Family, target adapter.TargetRef, summary string) adapter.CheckResult {
	return adapter.CheckResult{Family: family, Outcome: adapter.OutcomeSkipped, TargetRef: target, Summary: summary, ObservedAt: time.Now()}
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
