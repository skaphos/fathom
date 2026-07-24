/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodelocaldns

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
// pre-staged Result keyed by Request.Target. Unmapped targets fall back to
// nextResult.
type fakeLauncher struct {
	mu         sync.Mutex
	calls      []probe.Request
	byTarget   map[string]probe.Result
	nextResult probe.Result
	nextErr    error
}

func (f *fakeLauncher) Run(_ context.Context, req probe.Request) (probe.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	if r, ok := f.byTarget[req.Target]; ok {
		return r, nil
	}
	return f.nextResult, f.nextErr
}

func (f *fakeLauncher) lastCall(t *testing.T) probe.Request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("launcher was never invoked")
	}
	return f.calls[len(f.calls)-1]
}

// adapterWithLauncher wires a fake launcher into the unexported launcher
// field. Production paths construct a real probe.Launcher{Client: req.Client}.
func adapterWithLauncher(l dnsProbeLauncher) Adapter { return Adapter{launcher: l} }

func passingDNSLauncher() *fakeLauncher {
	return &fakeLauncher{nextResult: probe.Result{Outcome: probe.OutcomePass, Summary: "DNS resolution succeeded"}}
}

func TestAdapterMetadata(t *testing.T) {
	a := New()
	if a.Name() != "node-local-dns" {
		t.Fatalf("Name: got %q, want node-local-dns", a.Name())
	}
	if a.ContractVersion() != adapter.ContractVersion {
		t.Fatalf("ContractVersion: got %q, want %q", a.ContractVersion(), adapter.ContractVersion)
	}
	caps := a.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "node-local-dns" {
		t.Fatalf("AddonTypes: got %#v, want [node-local-dns]", caps.AddonTypes)
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
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultDaemonSetName, adapter.OutcomePass, "fully ready")
	assertHasOutcome(t, result.Checks, "Pod", "node-local-dns-abcde", adapter.OutcomePass, "ready")
	assertHasOutcome(t, result.Checks, "Node", "nodes", adapter.OutcomePass, "every schedulable node")
	assertHasOutcome(t, result.Checks, "DNSName", defaultDNSTargets, adapter.OutcomePass, "")

	req := launcher.lastCall(t)
	if len(req.DNSNameservers) != 1 || req.DNSNameservers[0] != defaultListenAddress {
		t.Fatalf("probe DNSNameservers: got %#v, want [%s]", req.DNSNameservers, defaultListenAddress)
	}
	if !strings.HasPrefix(req.Name, "fathom-nldns-") {
		t.Fatalf("probe pod name %q lacks fathom-nldns- prefix", req.Name)
	}
	if req.Mode != probe.ModeDNS {
		t.Fatalf("probe mode: got %q, want %q", req.Mode, probe.ModeDNS)
	}
	if req.Namespace != "default" {
		t.Fatalf("probe namespace: got %q, want the AddonCheck namespace", req.Namespace)
	}
}

func TestRun_MissingDaemonSetFailsWithAbsentMarker(t *testing.T) {
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, readyNode("node-a")),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultDaemonSetName, adapter.OutcomeFail, "missing")
	assertHasDetail(t, result.Checks, "DaemonSet", defaultDaemonSetName, adapter.DetailAbsent, "true")
	// No DaemonSet means no pod or coverage checks — the miss is the verdict.
	assertNoTarget(t, result.Checks, "Node", "nodes")
}

func TestRun_NodeCoverageGapNamesMissingNodes(t *testing.T) {
	daemonSet := daemonSetWithStatus(2, 1)
	objects := []clientObject{
		daemonSet,
		readyCachePod("node-local-dns-abcde", "node-a"),
		readyNode("node-a"),
		readyNode("node-b"),
	}
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultDaemonSetName, adapter.OutcomeFail, "not ready on every scheduled node")
	assertHasOutcome(t, result.Checks, "Node", "nodes", adapter.OutcomeFail, "1 of 2 schedulable node(s)")
	assertHasDetail(t, result.Checks, "Node", "nodes", "missingNodes", "node-b")
	assertHasDetail(t, result.Checks, "Node", "nodes", "missingNodeCount", "1")
	assertHasDetail(t, result.Checks, "Node", "nodes", "schedulableNodes", "2")
}

func TestRun_CordonedNodeIsNotACoverageGap(t *testing.T) {
	cordoned := readyNode("node-b")
	cordoned.Spec.Unschedulable = true
	objects := []clientObject{
		daemonSetWithStatus(1, 1),
		readyCachePod("node-local-dns-abcde", "node-a"),
		readyNode("node-a"),
		cordoned,
	}
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Node", "nodes", adapter.OutcomePass, "every schedulable node")
	assertHasDetail(t, result.Checks, "Node", "nodes", "schedulableNodes", "1")
}

func TestRun_UnreadyCachePodIsACoverageGapEvenWhenCountsMatch(t *testing.T) {
	// Regression for the "count mismatch" blind spot the per-node check exists
	// to close: status counters can lag reality, so coverage is computed from
	// observed pods, not from DaemonSet status arithmetic.
	unready := readyCachePod("node-local-dns-abcde", "node-a")
	unready.Status.Conditions[0].Status = corev1.ConditionFalse
	objects := []clientObject{
		daemonSetWithStatus(1, 1),
		unready,
		readyNode("node-a"),
	}
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Node", "nodes", adapter.OutcomeFail, "1 of 1 schedulable node(s)")
	assertHasDetail(t, result.Checks, "Node", "nodes", "missingNodes", "node-a")
	assertHasOutcome(t, result.Checks, "Pod", "node-local-dns-abcde", adapter.OutcomeWarn, "not ready")
}

func TestRun_DaemonSetRolloutInProgressWarns(t *testing.T) {
	daemonSet := daemonSetWithStatus(1, 1)
	daemonSet.Status.UpdatedNumberScheduled = 0
	objects := []clientObject{daemonSet, readyCachePod("node-local-dns-abcde", "node-a"), readyNode("node-a")}
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultDaemonSetName, adapter.OutcomeWarn, "rollout is in progress")
}

func TestRun_SystemHealthSupportsRenamedDaemonSet(t *testing.T) {
	daemonSet := daemonSetWithStatus(1, 1)
	daemonSet.Name = "node-local-dns-renamed"
	pod := readyCachePod("node-local-dns-renamed-abcde", "node-a")
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, daemonSet, pod, readyNode("node-a")),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilySystemHealth: {Enabled: true, Thresholds: map[string]string{thresholdDaemonSetName: "node-local-dns-renamed"}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", "node-local-dns-renamed", adapter.OutcomePass, "fully ready")
}

func TestRun_DNSResolutionFailureSurfacesAsFail(t *testing.T) {
	launcher := &fakeLauncher{nextResult: probe.Result{Outcome: probe.OutcomeFail, Summary: "DNS resolution failed"}}
	a := adapterWithLauncher(launcher)
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyObjects()...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DNSName", defaultDNSTargets, adapter.OutcomeFail, "DNS resolution failed")
}

func TestRun_DNSLauncherErrorSurfacesAsError(t *testing.T) {
	launcher := &fakeLauncher{nextErr: errors.New("pods is forbidden")}
	a := adapterWithLauncher(launcher)
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyObjects()...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DNSName", defaultDNSTargets, adapter.OutcomeError, "probe pod execution failed")
}

func TestRun_DNSResolutionHonorsThresholdsAndTimeout(t *testing.T) {
	launcher := passingDNSLauncher()
	a := adapterWithLauncher(launcher)
	_, err := a.Run(context.Background(), adapter.Request{
		Client:  newFakeClient(t, healthyObjects()...),
		Logger:  logr.Discard(),
		Target:  adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
		Timeout: 17 * time.Second,
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilyDNSResolution: {Enabled: true, Thresholds: map[string]string{
				thresholdListenAddress: "169.254.25.10",
				thresholdTargets:       "example.org.",
				thresholdProbeImage:    "ghcr.io/example/probe:override",
			}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	req := launcher.lastCall(t)
	if req.DNSNameservers[0] != "169.254.25.10" {
		t.Fatalf("listenAddress threshold not honored: got %#v", req.DNSNameservers)
	}
	if req.Target != "example.org." {
		t.Fatalf("targets threshold not honored: got %q", req.Target)
	}
	if req.Image != "ghcr.io/example/probe:override" {
		t.Fatalf("probeImage threshold not honored: got %q", req.Image)
	}
	if req.Timeout != 17*time.Second {
		t.Fatalf("req.Timeout not propagated: got %v", req.Timeout)
	}
}

func TestRun_DNSResolutionProbeImagePrecedence(t *testing.T) {
	tests := []struct {
		name            string
		threshold       string
		operatorDefault string
		want            string
	}{
		{name: "threshold wins", threshold: "ghcr.io/x/probe:t", operatorDefault: "ghcr.io/x/probe:o", want: "ghcr.io/x/probe:t"},
		{name: "operator default when no threshold", operatorDefault: "ghcr.io/x/probe:o", want: "ghcr.io/x/probe:o"},
		{name: "fallback when nothing is set", want: fallbackProbeImage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			launcher := passingDNSLauncher()
			a := adapterWithLauncher(launcher)
			policy := map[adapter.Family]adapter.FamilyPolicy{FamilyDNSResolution: {Enabled: true}}
			if tt.threshold != "" {
				policy[FamilyDNSResolution] = adapter.FamilyPolicy{Enabled: true, Thresholds: map[string]string{thresholdProbeImage: tt.threshold}}
			}
			_, err := a.Run(context.Background(), adapter.Request{
				Client:     newFakeClient(t),
				Logger:     logr.Discard(),
				Target:     adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
				ProbeImage: tt.operatorDefault,
				Policy:     policy,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := launcher.lastCall(t).Image; got != tt.want {
				t.Fatalf("probe image: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRun_DNSResolutionSkipsWhenNamespaceCannotBeResolved(t *testing.T) {
	launcher := passingDNSLauncher()
	a := adapterWithLauncher(launcher)
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		// Cluster-scoped target: no namespace to inherit and no threshold set.
		Target: adapter.TargetRef{Kind: "AddonCheck", Name: "node-local-dns"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilyDNSResolution: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DNSName", defaultDNSTargets, adapter.OutcomeSkipped, "probe namespace is required")
	if len(launcher.calls) != 0 {
		t.Fatalf("launcher must not run without a namespace; got %d calls", len(launcher.calls))
	}
}

func TestRun_AllFamiliesDisabledEmitsSkippedSentinel(t *testing.T) {
	a := adapterWithLauncher(passingDNSLauncher())
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "node-local-dns"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Checks) != 1 || result.Checks[0].Outcome != adapter.OutcomeSkipped {
		t.Fatalf("expected a single Skipped sentinel, got %#v", result.Checks)
	}
}

func TestDNSProbePodNameIsDNS1123Compliant(t *testing.T) {
	for _, target := range []string{
		"kubernetes.default.svc.cluster.local",
		"UPPER.case.Example",
		"...",
		strings.Repeat("very-long-label.", 20) + "example.com",
	} {
		name := dnsProbePodName(target)
		if len(name) > 63 {
			t.Fatalf("pod name %q exceeds 63 chars (%d)", name, len(name))
		}
		for _, r := range name {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				t.Fatalf("pod name %q contains invalid rune %q", name, r)
			}
		}
		if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
			t.Fatalf("pod name %q starts or ends with a dash", name)
		}
	}
}

func TestBoundedNodeListCapsAtMax(t *testing.T) {
	names := make([]string, maxListedNodes+5)
	for i := range names {
		names[i] = "node"
	}
	got := boundedNodeList(names)
	if !strings.HasSuffix(got, "+5 more") {
		t.Fatalf("boundedNodeList: got %q, want a +5 more suffix", got)
	}
	if n := strings.Count(got, "node"); n != maxListedNodes {
		t.Fatalf("boundedNodeList listed %d names, want %d", n, maxListedNodes)
	}
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

func assertNoTarget(t *testing.T, checks []adapter.CheckResult, kind, name string) {
	t.Helper()
	for _, check := range checks {
		if check.TargetRef.Kind == kind && check.TargetRef.Name == name {
			t.Fatalf("unexpected check for %s/%s: %#v", kind, name, check)
		}
	}
}

func newFakeClient(t *testing.T, objects ...clientObject) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
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
	return []clientObject{
		daemonSetWithStatus(1, 1),
		readyCachePod("node-local-dns-abcde", "node-a"),
		readyNode("node-a"),
	}
}

// daemonSetWithStatus builds the node-local-dns DaemonSet with desired
// scheduled/ready counters; UpdatedNumberScheduled tracks desired so the
// default is "no rollout in progress".
func daemonSetWithStatus(desired, ready int32) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: defaultDaemonSetName, Namespace: defaultNamespace},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "node-local-dns"}},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: desired,
			NumberReady:            ready,
			UpdatedNumberScheduled: desired,
		},
	}
}

func readyCachePod(name, nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNamespace, Labels: map[string]string{"k8s-app": "node-local-dns"}},
		Spec:       corev1.PodSpec{NodeName: nodeName},
		Status: corev1.PodStatus{
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: "node-cache", RestartCount: 0}},
		},
	}
}

func readyNode(name string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
}
