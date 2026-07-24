/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// In-package white-box tests: the fake probe launcher is injected through the
// unexported launcher field, mirroring the CoreDNS adapter's test seam.
package kubestatemetrics

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/skaphos/fathom/internal/probe"
	"github.com/skaphos/fathom/pkg/adapter"
)

// fakeLauncher records every probe.Request it receives and returns a
// pre-staged Result keyed by Request.Target (the scrape URL). Unmapped targets
// fall back to nextResult.
type fakeLauncher struct {
	mu          sync.Mutex
	calls       []probe.Request
	byTarget    map[string]probe.Result
	byTargetErr map[string]error
	nextResult  probe.Result
	nextErr     error
}

func (f *fakeLauncher) Run(_ context.Context, req probe.Request) (probe.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	if r, ok := f.byTarget[req.Target]; ok {
		return r, f.byTargetErr[req.Target]
	}
	return f.nextResult, f.nextErr
}

func (f *fakeLauncher) requests() []probe.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]probe.Request(nil), f.calls...)
}

func passingLauncher() *fakeLauncher {
	return &fakeLauncher{nextResult: probe.Result{Outcome: probe.OutcomePass, Summary: "metrics scrape succeeded"}}
}

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("build scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func healthyDeployment() *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: defaultWorkloadName, Namespace: defaultNamespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "kube-state-metrics"}},
		},
		Status: appsv1.DeploymentStatus{
			AvailableReplicas: 1,
			Conditions:        []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}},
		},
	}
}

func readyPod(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNamespace, Labels: map[string]string{"app.kubernetes.io/name": "kube-state-metrics"}},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
}

// ksmService returns the exporter Service exposing the given ports.
func ksmService(ports ...int32) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: defaultServiceName, Namespace: defaultNamespace},
		Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.10"},
	}
	for _, p := range ports {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Port: p})
	}
	return svc
}

func healthyObjects() []client.Object {
	return []client.Object{healthyDeployment(), readyPod("kube-state-metrics-abc"), ksmService(8080, 8081)}
}

func runRequest(t *testing.T, objs ...client.Object) adapter.Request {
	t.Helper()
	return adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "ksm"},
	}
}

func findCheck(t *testing.T, checks []adapter.CheckResult, kind, name string) *adapter.CheckResult {
	t.Helper()
	for i := range checks {
		if checks[i].TargetRef.Kind == kind && checks[i].TargetRef.Name == name {
			return &checks[i]
		}
	}
	return nil
}

func assertCheck(t *testing.T, checks []adapter.CheckResult, kind, name string, outcome adapter.Outcome, summaryFragment string) *adapter.CheckResult {
	t.Helper()
	c := findCheck(t, checks, kind, name)
	if c == nil {
		t.Fatalf("no check for %s/%s in %+v", kind, name, checks)
	}
	if c.Outcome != outcome {
		t.Fatalf("%s/%s outcome: got %s (summary %q), want %s", kind, name, c.Outcome, c.Summary, outcome)
	}
	if summaryFragment != "" && !strings.Contains(c.Summary, summaryFragment) {
		t.Fatalf("%s/%s summary %q does not contain %q", kind, name, c.Summary, summaryFragment)
	}
	return c
}

func TestAdapterMetadata(t *testing.T) {
	a := New()
	if a.Name() != "kube-state-metrics" {
		t.Fatalf("Name: got %q, want kube-state-metrics", a.Name())
	}
	if a.ContractVersion() != adapter.ContractVersion {
		t.Fatalf("ContractVersion: got %q, want %q", a.ContractVersion(), adapter.ContractVersion)
	}
	caps := a.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "kube-state-metrics" {
		t.Fatalf("AddonTypes: got %#v, want [kube-state-metrics]", caps.AddonTypes)
	}
	if len(caps.Families) != 2 {
		t.Fatalf("Families: got %#v, want 2 families", caps.Families)
	}
}

func TestRun_HealthyDeploymentAndEndpointsPass(t *testing.T) {
	launcher := passingLauncher()
	a := Adapter{launcher: launcher}
	result, err := a.Run(context.Background(), runRequest(t, healthyObjects()...))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, result.Checks, "Deployment", defaultWorkloadName, adapter.OutcomePass, "available")
	assertCheck(t, result.Checks, "Pod", "kube-state-metrics-abc", adapter.OutcomePass, "ready")
	assertCheck(t, result.Checks, "MetricsEndpoint", "kube-state-metrics:8080", adapter.OutcomePass, "succeeded")
	assertCheck(t, result.Checks, "MetricsEndpoint", "kube-state-metrics:8081", adapter.OutcomePass, "succeeded")

	reqs := launcher.requests()
	if len(reqs) != 2 {
		t.Fatalf("probe launches: got %d, want 2 (%+v)", len(reqs), reqs)
	}
	wantMain := "http://kube-state-metrics.kube-system.svc:8080/metrics"
	if reqs[0].Target != wantMain {
		t.Fatalf("main scrape URL: got %q, want %q", reqs[0].Target, wantMain)
	}
	if strings.Join(reqs[0].Expect, ",") != defaultExpectedFamilies {
		t.Fatalf("main scrape expect: got %v, want %q", reqs[0].Expect, defaultExpectedFamilies)
	}
	if reqs[0].Mode != probe.ModeHTTPGet {
		t.Fatalf("probe mode: got %q, want %q", reqs[0].Mode, probe.ModeHTTPGet)
	}
	// Probe pods run in the AddonCheck's namespace when no probeNamespace
	// threshold overrides it.
	if reqs[0].Namespace != "default" {
		t.Fatalf("probe namespace: got %q, want default", reqs[0].Namespace)
	}
	wantTelemetry := "http://kube-state-metrics.kube-system.svc:8081/metrics"
	if reqs[1].Target != wantTelemetry {
		t.Fatalf("telemetry scrape URL: got %q, want %q", reqs[1].Target, wantTelemetry)
	}
	if strings.Join(reqs[1].Expect, ",") != defaultTelemetryFamilies {
		t.Fatalf("telemetry scrape expect: got %v, want %q", reqs[1].Expect, defaultTelemetryFamilies)
	}
}

func TestRun_ShardedStatefulSetAllShardsReadyPass(t *testing.T) {
	replicas := int32(2)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: defaultWorkloadName, Namespace: defaultNamespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "kube-state-metrics"}},
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 2},
	}
	a := Adapter{launcher: passingLauncher()}
	result, err := a.Run(context.Background(), runRequest(t, sts, readyPod("kube-state-metrics-0"), readyPod("kube-state-metrics-1"), ksmService(8080, 8081)))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := assertCheck(t, result.Checks, "StatefulSet", defaultWorkloadName, adapter.OutcomePass, "every shard ready")
	if c.Details["sharded"] != "true" || c.Details["desiredShards"] != "2" || c.Details["readyShards"] != "2" {
		t.Fatalf("sharded details: got %+v", c.Details)
	}
	assertCheck(t, result.Checks, "Pod", "kube-state-metrics-0", adapter.OutcomePass, "ready")
	assertCheck(t, result.Checks, "Pod", "kube-state-metrics-1", adapter.OutcomePass, "ready")
}

func TestRun_ShardedStatefulSetMissingShardFails(t *testing.T) {
	replicas := int32(3)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: defaultWorkloadName, Namespace: defaultNamespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "kube-state-metrics"}},
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 2},
	}
	a := Adapter{launcher: passingLauncher()}
	result, err := a.Run(context.Background(), runRequest(t, sts, ksmService(8080, 8081)))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := assertCheck(t, result.Checks, "StatefulSet", defaultWorkloadName, adapter.OutcomeFail, "every shard")
	if c.Details["desiredShards"] != "3" || c.Details["readyShards"] != "2" {
		t.Fatalf("shard details: got %+v", c.Details)
	}
}

func TestRun_WorkloadAbsentFailsWithAbsentMarker(t *testing.T) {
	a := Adapter{launcher: passingLauncher()}
	result, err := a.Run(context.Background(), runRequest(t, ksmService(8080)))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := assertCheck(t, result.Checks, "Deployment", defaultWorkloadName, adapter.OutcomeFail, "missing")
	if !adapter.IsAbsent(c.Details) {
		t.Fatalf("workload absence must carry the absent marker, got %+v", c.Details)
	}
}

func TestRun_ServiceAbsentFailsMetricsEndpoint(t *testing.T) {
	a := Adapter{launcher: passingLauncher()}
	result, err := a.Run(context.Background(), runRequest(t, healthyDeployment(), readyPod("kube-state-metrics-abc")))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := assertCheck(t, result.Checks, "Service", defaultServiceName, adapter.OutcomeFail, "missing")
	if !adapter.IsAbsent(c.Details) {
		t.Fatalf("service absence must carry the absent marker, got %+v", c.Details)
	}
}

func TestRun_TelemetryPortNotExposedIsSkipped(t *testing.T) {
	// The Helm chart's default install exposes only the main metrics port on
	// the Service (selfMonitor disabled); that legitimate shape must not Fail.
	launcher := passingLauncher()
	a := Adapter{launcher: launcher}
	result, err := a.Run(context.Background(), runRequest(t, healthyDeployment(), readyPod("kube-state-metrics-abc"), ksmService(8080)))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, result.Checks, "MetricsEndpoint", "kube-state-metrics:8080", adapter.OutcomePass, "")
	assertCheck(t, result.Checks, "MetricsEndpoint", "kube-state-metrics:8081", adapter.OutcomeSkipped, "does not expose the self-telemetry port")
	if n := len(launcher.requests()); n != 1 {
		t.Fatalf("probe launches: got %d, want only the main scrape", n)
	}
}

func TestRun_TelemetryDisabledByThresholdIsSkipped(t *testing.T) {
	launcher := passingLauncher()
	a := Adapter{launcher: launcher}
	req := runRequest(t, healthyObjects()...)
	req.Policy = map[adapter.Family]adapter.FamilyPolicy{
		FamilySystemHealth:    {Enabled: true},
		FamilyMetricsEndpoint: {Enabled: true, Thresholds: map[string]string{thresholdTelemetryPort: "0"}},
	}
	result, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, result.Checks, "MetricsEndpoint", "kube-state-metrics:0", adapter.OutcomeSkipped, "disabled")
	if n := len(launcher.requests()); n != 1 {
		t.Fatalf("probe launches: got %d, want only the main scrape", n)
	}
}

func TestRun_MetricsPortNotExposedFails(t *testing.T) {
	a := Adapter{launcher: passingLauncher()}
	result, err := a.Run(context.Background(), runRequest(t, healthyDeployment(), readyPod("kube-state-metrics-abc"), ksmService(9999)))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := assertCheck(t, result.Checks, "MetricsEndpoint", "kube-state-metrics:8080", adapter.OutcomeFail, "does not expose the metrics port")
	if c.Details["servicePorts"] != "9999" {
		t.Fatalf("servicePorts detail: got %+v", c.Details)
	}
}

func TestRun_ScrapeFailureSurfacesAsFail(t *testing.T) {
	// A probe-binary Fail (unreachable endpoint, missing families) is the
	// condition the check exists to detect — it must pass through as Fail, not
	// be reclassified as Error.
	launcher := &fakeLauncher{
		nextResult: probe.Result{
			Outcome: probe.OutcomeFail,
			Summary: "expected metric families are missing",
			Details: map[string]string{"missingFamilies": "kube_node_info", "latencyMillis": "12"},
		},
	}
	a := Adapter{launcher: launcher}
	result, err := a.Run(context.Background(), runRequest(t, healthyObjects()...))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := assertCheck(t, result.Checks, "MetricsEndpoint", "kube-state-metrics:8080", adapter.OutcomeFail, "missing")
	if c.Details["missingFamilies"] != "kube_node_info" {
		t.Fatalf("probe details must flow through, got %+v", c.Details)
	}
	if c.Details["probeLatencyMillis"] != "12" {
		t.Fatalf("probe latency must be preserved separately, got %+v", c.Details)
	}
}

func TestRun_LauncherErrorSurfacesAsError(t *testing.T) {
	launcher := &fakeLauncher{nextErr: errors.New("create probe pod: forbidden")}
	a := Adapter{launcher: launcher}
	result, err := a.Run(context.Background(), runRequest(t, healthyObjects()...))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := assertCheck(t, result.Checks, "MetricsEndpoint", "kube-state-metrics:8080", adapter.OutcomeError, "probe pod execution failed")
	if !strings.Contains(c.Details["error"], "forbidden") {
		t.Fatalf("error detail: got %+v", c.Details)
	}
}

func TestRun_NoProbeNamespaceSkipsScrapes(t *testing.T) {
	launcher := passingLauncher()
	a := Adapter{launcher: launcher}
	req := runRequest(t, healthyObjects()...)
	req.Target.Namespace = ""
	result, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, result.Checks, "MetricsEndpoint", "kube-state-metrics:8080", adapter.OutcomeSkipped, "probe namespace is required")
	if n := len(launcher.requests()); n != 0 {
		t.Fatalf("probe launches: got %d, want 0", n)
	}
}

func TestRun_ThresholdOverrides(t *testing.T) {
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "ksm-custom", Namespace: "monitoring"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "ksm"}},
		},
		Status: appsv1.DeploymentStatus{
			AvailableReplicas: 1,
			Conditions:        []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}},
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "ksm-svc", Namespace: "monitoring"},
		Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.11", Ports: []corev1.ServicePort{{Port: 9443}}},
	}
	launcher := passingLauncher()
	a := Adapter{launcher: launcher}
	req := runRequest(t, deployment, svc)
	req.Policy = map[adapter.Family]adapter.FamilyPolicy{
		FamilySystemHealth: {Enabled: true, Namespaces: []string{"monitoring"}, Thresholds: map[string]string{thresholdWorkloadName: "ksm-custom"}},
		FamilyMetricsEndpoint: {Enabled: true, Namespaces: []string{"monitoring"}, Thresholds: map[string]string{
			thresholdServiceName:      "ksm-svc",
			thresholdMetricsPort:      "9443",
			thresholdTelemetryPort:    "0",
			thresholdExpectedFamilies: "kube_deployment_status_replicas",
			thresholdProbeImage:       "example.com/probe:v1",
			thresholdProbeNamespace:   "probe-ns",
		}},
	}
	result, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, result.Checks, "Deployment", "ksm-custom", adapter.OutcomePass, "available")
	assertCheck(t, result.Checks, "MetricsEndpoint", "ksm-svc:9443", adapter.OutcomePass, "")
	reqs := launcher.requests()
	if len(reqs) != 1 {
		t.Fatalf("probe launches: got %d, want 1", len(reqs))
	}
	if reqs[0].Target != "http://ksm-svc.monitoring.svc:9443/metrics" {
		t.Fatalf("scrape URL: got %q", reqs[0].Target)
	}
	if reqs[0].Image != "example.com/probe:v1" || reqs[0].Namespace != "probe-ns" {
		t.Fatalf("probe image/namespace overrides not honored: %+v", reqs[0])
	}
	if strings.Join(reqs[0].Expect, ",") != "kube_deployment_status_replicas" {
		t.Fatalf("expected families override not honored: %v", reqs[0].Expect)
	}
}

func TestRun_AllFamiliesDisabledEmitsSentinelSkip(t *testing.T) {
	a := Adapter{launcher: passingLauncher()}
	req := runRequest(t)
	req.Policy = map[adapter.Family]adapter.FamilyPolicy{}
	result, err := a.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Checks) != 1 || result.Checks[0].Outcome != adapter.OutcomeSkipped {
		t.Fatalf("want a single sentinel Skipped, got %+v", result.Checks)
	}
}

func TestRun_HonorsRequestTimeoutForProbes(t *testing.T) {
	launcher := passingLauncher()
	a := Adapter{launcher: launcher}
	req := runRequest(t, healthyObjects()...)
	req.Timeout = 7 * time.Second
	if _, err := a.Run(context.Background(), req); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, pr := range launcher.requests() {
		if pr.Timeout != 7*time.Second {
			t.Fatalf("probe timeout: got %s, want 7s", pr.Timeout)
		}
	}
}
