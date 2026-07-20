/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package coredns

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
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/skaphos/fathom/internal/probe"
	"github.com/skaphos/fathom/pkg/adapter"
)

// fakeLauncher records every probe.Request it receives and returns a
// pre-staged Result keyed by Request.Target. Unmapped targets fall back to
// nextResult, which lets simple tests stage one result and richer tests
// stage per-target outcomes.
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

func (f *fakeLauncher) callsForTarget(target string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.Target == target {
			n++
		}
	}
	return n
}

// adapterWithLauncher wires a fake launcher into the unexported launcher
// field. Production paths construct a real probe.Launcher{Client: req.Client}.
func adapterWithLauncher(l dnsProbeLauncher) Adapter { return Adapter{launcher: l} }

// passingDNSLauncher returns a fake that returns Pass for every probe.
func passingDNSLauncher() *fakeLauncher {
	return &fakeLauncher{nextResult: probe.Result{Outcome: probe.OutcomePass, Summary: "DNS resolution succeeded"}}
}

func TestAdapterMetadata(t *testing.T) {
	a := New()
	if a.Name() != "coredns" {
		t.Fatalf("Name: got %q, want coredns", a.Name())
	}
	if a.ContractVersion() != adapter.ContractVersion {
		t.Fatalf("ContractVersion: got %q, want %q", a.ContractVersion(), adapter.ContractVersion)
	}
	caps := a.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "coredns" {
		t.Fatalf("AddonTypes: got %#v, want [coredns]", caps.AddonTypes)
	}
	if len(caps.Families) != 2 {
		t.Fatalf("Families: got %#v, want 2 families", caps.Families)
	}
}

func TestRun_SystemHealthAndDNSResolutionPass(t *testing.T) {
	launcher := passingDNSLauncher()
	a := adapterWithLauncher(launcher)
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyObjects()...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "coredns"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "coredns", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "Pod", "coredns", adapter.OutcomePass, "ready")
	assertHasOutcome(t, result.Checks, "Service", "kube-dns", adapter.OutcomePass, "routable")
	assertHasOutcome(t, result.Checks, "EndpointSlice", "kube-dns", adapter.OutcomePass, "ready endpoints")
	assertHasOutcome(t, result.Checks, "DNSName", defaultDNSTargets, adapter.OutcomePass, "succeeded")
	if launcher.callsForTarget(defaultDNSTargets) != 1 {
		t.Fatalf("default target probe count: got %d, want 1", launcher.callsForTarget(defaultDNSTargets))
	}
}

func TestRun_DNSResolutionFailureIncludesError(t *testing.T) {
	launcher := &fakeLauncher{
		byTarget: map[string]probe.Result{
			"svc-a": {Outcome: probe.OutcomeError, Summary: "DNS resolution failed", Details: map[string]string{"error": "no such host"}},
			"svc-b": {Outcome: probe.OutcomeError, Summary: "DNS resolution failed", Details: map[string]string{"error": "no such host"}},
		},
	}
	a := adapterWithLauncher(launcher)
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "coredns"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilyDNSResolution: {Enabled: true, Thresholds: map[string]string{thresholdTargets: "svc-a,svc-b"}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DNSName", "svc-a", adapter.OutcomeError, "failed")
	assertHasOutcome(t, result.Checks, "DNSName", "svc-b", adapter.OutcomeError, "failed")
	assertHasDetail(t, result.Checks, "DNSName", "svc-a", "error", "no such host")
}

func TestRun_DNSLauncherErrorSurfacesAsError(t *testing.T) {
	// The launcher itself failing (e.g. could not create probe pod) — distinct
	// from the probe binary returning Outcome=Error. The adapter must surface
	// this as adapter.OutcomeError with a clear summary.
	launcher := &fakeLauncher{nextErr: errors.New("create probe pod: forbidden")}
	a := adapterWithLauncher(launcher)
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "coredns"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilyDNSResolution: {Enabled: true, Thresholds: map[string]string{thresholdTargets: "svc-a"}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DNSName", "svc-a", adapter.OutcomeError, "probe pod execution failed")
	assertHasDetail(t, result.Checks, "DNSName", "svc-a", "error", "create probe pod: forbidden")
}

func TestRun_DNSResolutionHonorsRequestTimeoutAndProbeImage(t *testing.T) {
	launcher := passingDNSLauncher()
	a := adapterWithLauncher(launcher)
	customImage := "registry.example.com/fathom-probe:custom"
	customTimeout := 7 * time.Second
	_, err := a.Run(context.Background(), adapter.Request{
		Client:  newFakeClient(t),
		Logger:  logr.Discard(),
		Target:  adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "coredns"},
		Timeout: customTimeout,
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilyDNSResolution: {Enabled: true, Thresholds: map[string]string{
			thresholdTargets:    "kubernetes.default",
			thresholdProbeImage: customImage,
		}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("launcher.calls: got %d, want 1", len(launcher.calls))
	}
	got := launcher.calls[0]
	if got.Image != customImage {
		t.Errorf("Request.Image: got %q, want %q", got.Image, customImage)
	}
	if got.Timeout != customTimeout {
		t.Errorf("Request.Timeout: got %v, want %v", got.Timeout, customTimeout)
	}
	if got.Mode != probe.ModeDNS {
		t.Errorf("Request.Mode: got %q, want %q", got.Mode, probe.ModeDNS)
	}
	if got.Target != "kubernetes.default" {
		t.Errorf("Request.Target: got %q", got.Target)
	}
	if !strings.HasPrefix(got.Name, "fathom-dns-kubernetes-default-") {
		t.Errorf("Request.Name: got %q, want fathom-dns-kubernetes-default-* prefix", got.Name)
	}
}

func TestRun_DNSResolutionProbeImagePrecedence(t *testing.T) {
	cases := []struct {
		name      string
		threshold string
		operator  string
		want      string
	}{
		{name: "threshold wins over operator", threshold: "registry.example.com/probe:thr", operator: "registry.example.com/probe:op", want: "registry.example.com/probe:thr"},
		{name: "operator default fills empty threshold", threshold: "", operator: "registry.example.com/probe:op", want: "registry.example.com/probe:op"},
		{name: "fallback when nothing is set", threshold: "", operator: "", want: fallbackProbeImage},
		{name: "threshold whitespace falls through to operator", threshold: "   ", operator: "registry.example.com/probe:op", want: "registry.example.com/probe:op"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			launcher := passingDNSLauncher()
			a := adapterWithLauncher(launcher)
			policy := adapter.FamilyPolicy{Enabled: true, Thresholds: map[string]string{thresholdTargets: "kubernetes.default"}}
			if tc.threshold != "" {
				policy.Thresholds[thresholdProbeImage] = tc.threshold
			}
			_, err := a.Run(context.Background(), adapter.Request{
				Client:     newFakeClient(t),
				Logger:     logr.Discard(),
				Target:     adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "coredns"},
				ProbeImage: tc.operator,
				Policy:     map[adapter.Family]adapter.FamilyPolicy{FamilyDNSResolution: policy},
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(launcher.calls) != 1 {
				t.Fatalf("launcher.calls: got %d, want 1", len(launcher.calls))
			}
			if got := launcher.calls[0].Image; got != tc.want {
				t.Errorf("Request.Image: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRun_DNSResolutionUsesAddonCheckNamespaceForProbePods(t *testing.T) {
	launcher := passingDNSLauncher()
	a := adapterWithLauncher(launcher)
	_, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "tenant-platform", Name: "coredns"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilyDNSResolution: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(launcher.calls) != 1 || launcher.calls[0].Namespace != "tenant-platform" {
		t.Errorf("probe pod namespace: got %#v", launcher.calls)
	}
}

func TestRun_DNSResolutionProbeNamespaceThresholdOverridesAddonCheckNamespace(t *testing.T) {
	launcher := passingDNSLauncher()
	a := adapterWithLauncher(launcher)
	_, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "tenant-platform", Name: "coredns"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilyDNSResolution: {Enabled: true, Thresholds: map[string]string{
			thresholdTargets:        "kubernetes.default",
			thresholdProbeNamespace: "fathom-probes",
		}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("launcher.calls: got %d, want 1", len(launcher.calls))
	}
	if got := launcher.calls[0].Namespace; got != "fathom-probes" {
		t.Errorf("probe pod namespace: got %q, want fathom-probes", got)
	}
}

func TestRun_DNSResolutionSkipsWhenNamespaceCannotBeResolved(t *testing.T) {
	// Target.Namespace is empty (cluster-scoped feeder, or synthetic request)
	// and no probeNamespace threshold is set. The adapter must not let
	// probe.Pod() fail per target; it must surface one Skipped per target
	// with an actionable summary, and must not call the launcher at all.
	launcher := passingDNSLauncher()
	a := adapterWithLauncher(launcher)
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Name: "coredns"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilyDNSResolution: {Enabled: true, Thresholds: map[string]string{
			thresholdTargets: "svc-a,svc-b",
		}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(launcher.calls) != 0 {
		t.Errorf("launcher must not be called when namespace is unresolved: got %d calls", len(launcher.calls))
	}
	assertHasOutcome(t, result.Checks, "DNSName", "svc-a", adapter.OutcomeSkipped, "probe namespace is required")
	assertHasOutcome(t, result.Checks, "DNSName", "svc-b", adapter.OutcomeSkipped, "probe namespace is required")
}

func TestRun_DNSResolutionProbeNamespaceFillsEmptyAddonCheckNamespace(t *testing.T) {
	launcher := passingDNSLauncher()
	a := adapterWithLauncher(launcher)
	_, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Name: "coredns"}, // no Namespace
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilyDNSResolution: {Enabled: true, Thresholds: map[string]string{
			thresholdProbeNamespace: "fathom-probes",
		}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("launcher.calls: got %d, want 1", len(launcher.calls))
	}
	if got := launcher.calls[0].Namespace; got != "fathom-probes" {
		t.Errorf("probe pod namespace: got %q, want fathom-probes", got)
	}
}

func TestDNSProbePodNameIsDNS1123Compliant(t *testing.T) {
	cases := []struct {
		in       string
		mustHave string
	}{
		{"kubernetes.default.svc.cluster.local", "kubernetes-default-svc-cluster"},
		{"UPPER.example.COM", "upper-example-com"},
		{"!!!", "target"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := dnsProbePodName(tc.in)
			if !strings.HasPrefix(got, "fathom-dns-") {
				t.Errorf("missing fathom-dns prefix: %q", got)
			}
			if !strings.Contains(got, tc.mustHave) {
				t.Errorf("name %q should contain %q", got, tc.mustHave)
			}
			if len(got) > 253 {
				t.Errorf("name length %d exceeds DNS-1123 max", len(got))
			}
			for _, r := range got {
				if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
					t.Errorf("non-DNS-1123 char %q in %q", r, got)
					break
				}
			}
		})
	}
}

func TestRun_MissingDeploymentFails(t *testing.T) {
	objects := healthyObjects()
	objects = objects[1:]
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "coredns", adapter.OutcomeFail, "missing")
	// The missing Deployment is a Fail that carries the absent marker so
	// status.absent counts it (SKA-526).
	assertHasDetail(t, result.Checks, "Deployment", "coredns", adapter.DetailAbsent, "true")
}

// TestRun_MissingServiceFails covers the second CoreDNS absence site: the kube-dns
// Service NotFound is a Fail carrying the absent marker (SKA-526).
func TestRun_MissingServiceFails(t *testing.T) {
	// Deployment and pod present, Service absent.
	objects := []clientObject{healthyDeployment(), readyPod(), dnsEndpointSlice()}
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Service", "kube-dns", adapter.OutcomeFail, "missing")
	assertHasDetail(t, result.Checks, "Service", "kube-dns", adapter.DetailAbsent, "true")
}

func TestRun_UnreadyPodWarns(t *testing.T) {
	// A live not-ready pod is Warn, not Fail: checkDeployment is the
	// authoritative outage signal and the deployment here is available (#160).
	objects := healthyObjects()
	pod := objects[1].(*corev1.Pod)
	pod.Status.Conditions[0].Status = corev1.ConditionFalse
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", "coredns", adapter.OutcomeWarn, "not ready")
	assertNoOutcome(t, result.Checks, "Pod", "coredns", adapter.OutcomeFail)
	// A present-but-unhealthy target is not absent (SKA-526).
	assertHasDetail(t, result.Checks, "Pod", "coredns", adapter.DetailAbsent, "")
}

func TestRun_TerminatingAndEvictedPodsNotGraded(t *testing.T) {
	dnsLabels := map[string]string{"k8s-app": "kube-dns"}
	objects := append(healthyObjects(),
		terminatingPodNamed("coredns-old", dnsLabels),
		evictedPodNamed("coredns-evicted", dnsLabels),
	)
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", "coredns", adapter.OutcomePass, "ready")
	// Filtered pods must produce no verdict at all — not merely no Fail.
	for _, filtered := range []string{"coredns-old", "coredns-evicted"} {
		for _, o := range []adapter.Outcome{adapter.OutcomeFail, adapter.OutcomeWarn, adapter.OutcomePass} {
			assertNoOutcome(t, result.Checks, "Pod", filtered, o)
		}
	}
}

func TestRun_AllPodsTerminatingButUnavailableStillFails(t *testing.T) {
	// Safety invariant: filtering all pods can never mask a real outage — the
	// deployment check still Fails; checkPods only Skips.
	dnsLabels := map[string]string{"k8s-app": "kube-dns"}
	objects := []clientObject{
		unavailableDeployment(),
		terminatingPodNamed("coredns-old", dnsLabels),
		dnsService(), dnsEndpointSlice(),
	}
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "coredns", adapter.OutcomeFail, "not fully available")
	assertHasOutcome(t, result.Checks, "Pod", "coredns", adapter.OutcomeSkipped, "only terminating")
}

func TestRun_NoReadyEndpointSlicesFails(t *testing.T) {
	objects := healthyObjects()
	objects = objects[:len(objects)-1]
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "EndpointSlice", "kube-dns", adapter.OutcomeFail, "no ready endpoints")
}

func TestRun_SystemHealthSupportsDistributionNamesAndAutoscaler(t *testing.T) {
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t,
			healthyDeploymentNamed("rke2-coredns", map[string]string{"k8s-app": "kube-dns"}),
			readyPodNamed("rke2-coredns", map[string]string{"k8s-app": "kube-dns"}),
			healthyDeploymentNamed("rke2-coredns-autoscaler", map[string]string{"k8s-app": "kube-dns-autoscaler"}),
			readyPodNamed("rke2-coredns-autoscaler", map[string]string{"k8s-app": "kube-dns-autoscaler"}),
			dnsServiceNamed("rke2-coredns-rke2-coredns"),
			dnsEndpointSliceNamed("rke2-coredns-rke2-coredns"),
		),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true, Thresholds: map[string]string{
			thresholdDeploymentName: "rke2-coredns",
			thresholdAutoscalerName: "rke2-coredns-autoscaler",
			thresholdServiceName:    "rke2-coredns-rke2-coredns",
		}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "rke2-coredns", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "Deployment", "rke2-coredns-autoscaler", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "Service", "rke2-coredns-rke2-coredns", adapter.OutcomePass, "routable")
}

func assertHasOutcome(t *testing.T, checks []adapter.CheckResult, kind, name string, outcome adapter.Outcome, summaryContains string) {
	t.Helper()
	for _, check := range checks {
		if check.TargetRef.Kind == kind && check.TargetRef.Name == name && check.Outcome == outcome {
			if summaryContains == "" || strings.Contains(check.Summary, summaryContains) {
				return
			}
		}
	}
	t.Fatalf("missing %s/%s outcome %s containing %q in %#v", kind, name, outcome, summaryContains, checks)
}

func assertHasDetail(t *testing.T, checks []adapter.CheckResult, kind, name, key, want string) {
	t.Helper()
	for _, check := range checks {
		if check.TargetRef.Kind == kind && check.TargetRef.Name == name {
			if got := check.Details[key]; got != want {
				t.Fatalf("detail %s for %s/%s: got %q, want %q", key, kind, name, got, want)
			}
			return
		}
	}
	t.Fatalf("missing %s/%s in %#v", kind, name, checks)
}

func assertNoOutcome(t *testing.T, checks []adapter.CheckResult, kind, name string, outcome adapter.Outcome) {
	t.Helper()
	for _, check := range checks {
		if check.TargetRef.Kind == kind && check.TargetRef.Name == name && check.Outcome == outcome {
			t.Fatalf("unexpected %s/%s outcome %s: %#v", kind, name, outcome, check)
		}
	}
}

func newFakeClient(t *testing.T, objects ...clientObject) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apps scheme: %v", err)
	}
	if err := discoveryv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add discovery scheme: %v", err)
	}
	clientObjects := make([]runtime.Object, 0, len(objects))
	for _, obj := range objects {
		clientObjects = append(clientObjects, obj)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(clientObjects...).Build()
}

type clientObject interface {
	runtime.Object
	client.Object
}

func healthyObjects() []clientObject {
	return []clientObject{healthyDeployment(), readyPod(), dnsService(), dnsEndpointSlice()}
}

func healthyDeployment() *appsv1.Deployment {
	return healthyDeploymentNamed("coredns", map[string]string{"k8s-app": "kube-dns"})
}

func healthyDeploymentNamed(name string, labels map[string]string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNamespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 1, Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}},
	}
}

func readyPod() *corev1.Pod {
	return readyPodNamed("coredns", map[string]string{"k8s-app": "kube-dns"})
}

func readyPodNamed(name string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNamespace, Labels: labels},
		Status: corev1.PodStatus{
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: name, RestartCount: 0}},
		},
	}
}

// terminatingPodNamed is a not-ready pod with a DeletionTimestamp — the old
// ReplicaSet's pod during a rolling update.
func terminatingPodNamed(name string, labels map[string]string) *corev1.Pod {
	p := readyPodNamed(name, labels)
	p.Status.Conditions[0].Status = corev1.ConditionFalse
	now := metav1.Now()
	p.DeletionTimestamp = &now
	p.Finalizers = []string{"kubernetes.io/test"}
	return p
}

// evictedPodNamed is a terminal-phase pod still matching the deployment selector.
func evictedPodNamed(name string, labels map[string]string) *corev1.Pod {
	p := readyPodNamed(name, labels)
	p.Status.Conditions[0].Status = corev1.ConditionFalse
	p.Status.Phase = corev1.PodFailed
	return p
}

// unavailableDeployment reports desired>available with no Available condition,
// so checkDeployment Fails while still returning the deployment for pod checks.
func unavailableDeployment() *appsv1.Deployment {
	d := healthyDeployment()
	d.Status.AvailableReplicas = 0
	d.Status.Conditions = nil
	return d
}

func dnsService() *corev1.Service {
	return dnsServiceNamed(defaultDNSServiceName)
}

func dnsServiceNamed(name string) *corev1.Service {
	return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNamespace}, Spec: corev1.ServiceSpec{ClusterIP: "10.96.0.10"}}
}

func dnsEndpointSlice() *discoveryv1.EndpointSlice {
	return dnsEndpointSliceNamed(defaultDNSServiceName)
}

func dnsEndpointSliceNamed(serviceName string) *discoveryv1.EndpointSlice {
	ready := true
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Name: serviceName + "-abc", Namespace: defaultNamespace, Labels: map[string]string{"kubernetes.io/service-name": serviceName}},
		Endpoints:  []discoveryv1.Endpoint{{Conditions: discoveryv1.EndpointConditions{Ready: &ready}}},
	}
}
