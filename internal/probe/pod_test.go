/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package probe

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestPodBuildsHardenedDNSProbe(t *testing.T) {
	pod, err := Pod(Request{Name: "dns-probe", Namespace: "tenant-a", Image: "example.com/fathom-probe:v1", Mode: ModeDNS, Target: "kubernetes.default.svc", Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("Pod: %v", err)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatalf("AutomountServiceAccountToken: got %#v, want false", pod.Spec.AutomountServiceAccountToken)
	}
	if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Fatalf("RestartPolicy: got %q, want Never", pod.Spec.RestartPolicy)
	}
	container := pod.Spec.Containers[0]
	if container.Image != "example.com/fathom-probe:v1" {
		t.Fatalf("Image: got %q", container.Image)
	}
	if got, want := container.Command[0], "/probe"; got != want {
		t.Fatalf("Command: got %q, want %q", got, want)
	}
	assertArgs(t, container.Args, "-mode", "dns", "-target", "kubernetes.default.svc", "-timeout", "3s")
	if container.SecurityContext == nil || container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
		t.Fatalf("AllowPrivilegeEscalation: got %#v, want false", container.SecurityContext)
	}
	if container.SecurityContext.ReadOnlyRootFilesystem == nil || !*container.SecurityContext.ReadOnlyRootFilesystem {
		t.Fatalf("ReadOnlyRootFilesystem: got %#v, want true", container.SecurityContext)
	}
	if len(container.SecurityContext.Capabilities.Drop) != 1 || container.SecurityContext.Capabilities.Drop[0] != "ALL" {
		t.Fatalf("Capabilities.Drop: got %#v, want [ALL]", container.SecurityContext.Capabilities.Drop)
	}
}

// TestPodStampsOwnerReferences covers the field DNSCheck needs so deleting a
// check garbage-collects its in-flight probe pods. The nil case is the
// load-bearing half: every adapter caller leaves OwnerReferences unset, and an
// empty-but-non-nil slice would be a gratuitous manifest diff for them.
func TestPodStampsOwnerReferences(t *testing.T) {
	controller := true
	owner := metav1.OwnerReference{
		APIVersion: "fathom.skaphos.io/v1alpha1",
		Kind:       "DNSCheck",
		Name:       "internal-names",
		UID:        types.UID("6f3a1c2e-0000-4000-8000-000000000001"),
		Controller: &controller,
	}

	t.Run("stamped when set", func(t *testing.T) {
		pod, err := Pod(Request{
			Name: "dns-probe", Namespace: "tenant-a", Image: "example.com/probe:v1",
			Mode: ModeDNS, Target: "kubernetes.default.svc", Timeout: 3 * time.Second,
			OwnerReferences: []metav1.OwnerReference{owner},
		})
		if err != nil {
			t.Fatalf("Pod: %v", err)
		}
		if len(pod.OwnerReferences) != 1 {
			t.Fatalf("OwnerReferences: got %d, want 1", len(pod.OwnerReferences))
		}
		if pod.OwnerReferences[0] != owner {
			t.Errorf("OwnerReferences[0]: got %+v, want %+v", pod.OwnerReferences[0], owner)
		}
	})

	t.Run("nil stays nil for existing callers", func(t *testing.T) {
		pod, err := Pod(Request{
			Name: "dns-probe", Namespace: "tenant-a", Image: "example.com/probe:v1",
			Mode: ModeDNS, Target: "kubernetes.default.svc", Timeout: 3 * time.Second,
		})
		if err != nil {
			t.Fatalf("Pod: %v", err)
		}
		if pod.OwnerReferences != nil {
			t.Errorf("OwnerReferences: got %#v, want nil", pod.OwnerReferences)
		}
	})

	t.Run("caller slice is copied, not aliased", func(t *testing.T) {
		refs := []metav1.OwnerReference{owner}
		pod, err := Pod(Request{
			Name: "dns-probe", Namespace: "tenant-a", Image: "example.com/probe:v1",
			Mode: ModeDNS, Target: "kubernetes.default.svc", Timeout: 3 * time.Second,
			OwnerReferences: refs,
		})
		if err != nil {
			t.Fatalf("Pod: %v", err)
		}
		refs[0].Name = "mutated-after-build"
		if pod.OwnerReferences[0].Name != "internal-names" {
			t.Errorf("pod aliased the caller's slice: got %q", pod.OwnerReferences[0].Name)
		}
	})
}

func TestPodSupportsCrossNodeAntiAffinity(t *testing.T) {
	pod, err := Pod(Request{
		Name:           "client",
		Namespace:      "tenant-a",
		Image:          "example.com/fathom-probe:v1",
		Mode:           ModeTCPConnect,
		Target:         "server.tenant-a.svc",
		Port:           8080,
		AvoidPodLabels: map[string]string{"fathom.skaphos.io/probe-role": "server"},
	})
	if err != nil {
		t.Fatalf("Pod: %v", err)
	}
	terms := pod.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if len(terms) != 1 {
		t.Fatalf("anti-affinity terms: got %d, want 1", len(terms))
	}
	if got, want := terms[0].TopologyKey, corev1.LabelHostname; got != want {
		t.Fatalf("TopologyKey: got %q, want %q", got, want)
	}
	if got := terms[0].LabelSelector.MatchLabels["fathom.skaphos.io/probe-role"]; got != "server" {
		t.Fatalf("anti-affinity label: got %q, want server", got)
	}
}

// The reserved labels are how Sweeper tells a reapable orphan from a pod it
// must not touch, so a caller-supplied label must never displace them.
func TestPodReservedLabelsSurviveCallerOverride(t *testing.T) {
	pod, err := Pod(Request{
		Name:      "dns-probe",
		Namespace: "default",
		Image:     "probe:latest",
		Mode:      ModeDNS,
		Target:    "kubernetes.default",
		Labels: map[string]string{
			labelManagedBy: "someone-else",
			labelProbeName: "hijacked",
			"team":         "platform",
		},
	})
	if err != nil {
		t.Fatalf("Pod: %v", err)
	}
	if got := pod.Labels[labelManagedBy]; got != managedByValue {
		t.Fatalf("%s: got %q, want %q", labelManagedBy, got, managedByValue)
	}
	if got := pod.Labels[labelProbeName]; got != "dns-probe" {
		t.Fatalf("%s: got %q, want dns-probe", labelProbeName, got)
	}
	if got := pod.Labels["team"]; got != "platform" {
		t.Fatalf("non-reserved caller label was dropped: got %q, want platform", got)
	}
}

func TestPodRejectsInvalidRequests(t *testing.T) {
	for _, tt := range []struct {
		name string
		req  Request
	}{
		{name: "missing image", req: Request{Name: "probe", Namespace: "ns", Mode: ModeDNS, Target: "svc"}},
		{name: "missing dns target", req: Request{Name: "probe", Namespace: "ns", Image: "image", Mode: ModeDNS}},
		{name: "missing tcp port", req: Request{Name: "probe", Namespace: "ns", Image: "image", Mode: ModeTCPConnect, Target: "svc"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Pod(tt.req); err == nil {
				t.Fatal("Pod returned nil error")
			}
		})
	}
}

// TestPodPinsResolverWhenDNSNameserversSet covers the node-local DNS path
// (SKA-511): a non-empty DNSNameservers must yield dnsPolicy None with exactly
// the requested nameservers, and an empty one must leave the pod's DNS policy
// untouched (cluster default).
func TestPodPinsResolverWhenDNSNameserversSet(t *testing.T) {
	pinned, err := Pod(Request{
		Name:           "probe-nldns",
		Namespace:      "default",
		Image:          "ghcr.io/skaphos/fathom-probe:test",
		Mode:           ModeDNS,
		Target:         "kubernetes.default.svc.cluster.local",
		DNSNameservers: []string{"169.254.20.10"},
	})
	if err != nil {
		t.Fatalf("Pod: %v", err)
	}
	if pinned.Spec.DNSPolicy != corev1.DNSNone {
		t.Fatalf("DNSPolicy: got %q, want %q", pinned.Spec.DNSPolicy, corev1.DNSNone)
	}
	if pinned.Spec.DNSConfig == nil || len(pinned.Spec.DNSConfig.Nameservers) != 1 || pinned.Spec.DNSConfig.Nameservers[0] != "169.254.20.10" {
		t.Fatalf("DNSConfig: got %#v, want nameservers [169.254.20.10]", pinned.Spec.DNSConfig)
	}

	unpinned, err := Pod(Request{
		Name:      "probe-dns",
		Namespace: "default",
		Image:     "ghcr.io/skaphos/fathom-probe:test",
		Mode:      ModeDNS,
		Target:    "kubernetes.default.svc.cluster.local",
	})
	if err != nil {
		t.Fatalf("Pod: %v", err)
	}
	if unpinned.Spec.DNSPolicy != "" || unpinned.Spec.DNSConfig != nil {
		t.Fatalf("unpinned pod must keep default DNS policy: got policy %q, config %#v", unpinned.Spec.DNSPolicy, unpinned.Spec.DNSConfig)
	}
}

// TestPodBuildsHTTPGetArgs pins the http-get arg contract the adapter relies
// on: the URL rides -target and the expected metric families are joined into
// a single -expect value (omitted entirely when none are declared).
// TestPodLeavesDNSPolicyAloneByDefault pins the FR-030 guarantee on the pod
// side: a Request that names no vantage point must produce a pod with no
// dnsPolicy override and no dnsConfig, so the pod inherits cluster DNS exactly
// as it did before vantage points existed. Every existing caller other than
// nodelocaldns is in this shape, so a default that set dnsPolicy explicitly
// would change where every current probe resolves.
func TestPodLeavesDNSPolicyAloneByDefault(t *testing.T) {
	pod, err := Pod(Request{Name: "dns-probe", Namespace: "tenant-a", Image: "img", Mode: ModeDNS, Target: "example.com"})
	if err != nil {
		t.Fatalf("Pod: %v", err)
	}
	if pod.Spec.DNSPolicy != "" {
		t.Fatalf("DNSPolicy = %q, want empty (inherit cluster DNS)", pod.Spec.DNSPolicy)
	}
	if pod.Spec.DNSConfig != nil {
		t.Fatalf("DNSConfig = %#v, want nil", pod.Spec.DNSConfig)
	}
	// The default dns argv must stay exactly -mode/-target/-timeout: an
	// unconditional -record-type would reach the probe binary for every
	// existing caller.
	assertArgs(t, pod.Spec.Containers[0].Args, "-mode", "dns", "-target", "example.com", "-timeout", "10s")
}

// TestPodResolvesFromTheDeclaredVantagePoint covers all three vantage points.
// The mapping is easy to get backwards: dnsPolicy "Default" is the *node's*
// resolver, not the pod default, and the pod default (cluster DNS) is
// expressed by setting no policy at all.
func TestPodResolvesFromTheDeclaredVantagePoint(t *testing.T) {
	tests := []struct {
		name        string
		req         Request
		wantPolicy  corev1.DNSPolicy
		wantServers []string
	}{
		{
			name:       "cluster is the zero value and sets no policy",
			req:        Request{DNSFrom: DNSFromCluster},
			wantPolicy: "",
		},
		{
			name:       "node uses dnsPolicy Default",
			req:        Request{DNSFrom: DNSFromNode},
			wantPolicy: corev1.DNSDefault,
		},
		{
			name:        "explicit pins the nameservers",
			req:         Request{DNSFrom: DNSFromExplicit, DNSNameservers: []string{"10.0.0.10"}},
			wantPolicy:  corev1.DNSNone,
			wantServers: []string{"10.0.0.10"},
		},
		{
			// Backward compatibility: nameservers alone meant Explicit before
			// DNSFrom existed, and must keep doing so.
			name:        "nameservers alone still imply explicit",
			req:         Request{DNSNameservers: []string{"10.0.0.10"}},
			wantPolicy:  corev1.DNSNone,
			wantServers: []string{"10.0.0.10"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			req.Name, req.Namespace, req.Image, req.Mode, req.Target = "p", "ns", "img", ModeDNS, "example.com."
			pod, err := Pod(req)
			if err != nil {
				t.Fatalf("Pod: %v", err)
			}
			if pod.Spec.DNSPolicy != tc.wantPolicy {
				t.Fatalf("DNSPolicy = %q, want %q", pod.Spec.DNSPolicy, tc.wantPolicy)
			}
			if len(tc.wantServers) == 0 {
				if pod.Spec.DNSConfig != nil {
					t.Fatalf("DNSConfig = %#v, want nil", pod.Spec.DNSConfig)
				}
				return
			}
			if pod.Spec.DNSConfig == nil || strings.Join(pod.Spec.DNSConfig.Nameservers, ",") != strings.Join(tc.wantServers, ",") {
				t.Fatalf("DNSConfig = %#v, want nameservers %v", pod.Spec.DNSConfig, tc.wantServers)
			}
		})
	}
}

// A pod with dnsPolicy None and no nameservers cannot resolve anything, and
// the failures it produces read as a DNS outage rather than the
// misconfiguration they are. Catch it while building the pod.
func TestPodRejectsExplicitVantagePointWithoutNameservers(t *testing.T) {
	_, err := Pod(Request{Name: "p", Namespace: "ns", Image: "img", Mode: ModeDNS, Target: "example.com.", DNSFrom: DNSFromExplicit})
	if err == nil {
		t.Fatal("expected an error for an explicit vantage point with no nameservers")
	}
}

// Nameservers mean nothing under the node vantage point. Accepting them and
// quietly discarding them would build a probe that queries the node's resolver
// while its caller believes it is querying the nameservers it supplied.
func TestPodRejectsNodeVantagePointWithNameservers(t *testing.T) {
	_, err := Pod(Request{
		Name: "p", Namespace: "ns", Image: "img", Mode: ModeDNS, Target: "example.com.",
		DNSFrom: DNSFromNode, DNSNameservers: []string{"10.0.0.10"},
	})
	if err == nil {
		t.Fatal("expected an error for a node vantage point carrying nameservers")
	}
}

func TestPodBuildsDNSAssertionArgs(t *testing.T) {
	pod, err := Pod(Request{
		Name: "p", Namespace: "ns", Image: "img", Mode: ModeDNS, Target: "_https._tcp.example.com.",
		RecordType: "SRV", ExpectedAnswers: []string{"a.example.com:443", "b.example.com:443"},
	})
	if err != nil {
		t.Fatalf("Pod: %v", err)
	}
	assertArgs(t, pod.Spec.Containers[0].Args,
		"-mode", "dns", "-target", "_https._tcp.example.com.",
		"-record-type", "SRV",
		"-expect-answers", "a.example.com:443,b.example.com:443",
		"-timeout", "10s")

	absent, err := Pod(Request{Name: "p", Namespace: "ns", Image: "img", Mode: ModeDNS, Target: "gone.example.com.", Absent: true})
	if err != nil {
		t.Fatalf("Pod: %v", err)
	}
	assertArgs(t, absent.Spec.Containers[0].Args, "-mode", "dns", "-target", "gone.example.com.", "-absent", "-timeout", "10s")
}

func TestPodBuildsHTTPGetArgs(t *testing.T) {
	pod, err := Pod(Request{
		Name:      "scrape",
		Namespace: "ns",
		Image:     "image",
		Mode:      ModeHTTPGet,
		Target:    "http://svc.ns.svc:8080/metrics",
		Expect:    []string{"kube_node_info", "kube_pod_info"},
	})
	if err != nil {
		t.Fatalf("Pod: %v", err)
	}
	args := strings.Join(pod.Spec.Containers[0].Args, " ")
	for _, want := range []string{
		"-mode http-get",
		"-target http://svc.ns.svc:8080/metrics",
		"-expect kube_node_info,kube_pod_info",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("args %q missing %q", args, want)
		}
	}

	noExpect, err := Pod(Request{Name: "scrape", Namespace: "ns", Image: "image", Mode: ModeHTTPGet, Target: "http://svc.ns.svc:8080/metrics"})
	if err != nil {
		t.Fatalf("Pod (no expect): %v", err)
	}
	if strings.Contains(strings.Join(noExpect.Spec.Containers[0].Args, " "), "-expect") {
		t.Error("-expect must be omitted when no families are declared")
	}
}

func TestParseResult(t *testing.T) {
	result, err := ParseResult(`{"outcome":"Pass","summary":"ok","details":{"latencyMillis":"1"}}`)
	if err != nil {
		t.Fatalf("ParseResult: %v", err)
	}
	if result.Outcome != OutcomePass || result.Summary != "ok" || result.Details["latencyMillis"] != "1" {
		t.Fatalf("result: got %#v", result)
	}
}

func assertArgs(t *testing.T, args []string, want ...string) {
	t.Helper()
	if len(args) != len(want) {
		t.Fatalf("args length: got %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d]: got %q, want %q in %#v", i, args[i], want[i], args)
		}
	}
}
