/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package probe

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
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
