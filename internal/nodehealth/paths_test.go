/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodehealth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func TestPathAllowed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want bool
	}{
		{"/var/lib/kubelet", true},
		{"/var/lib/kubelet/pods", true},
		{"/var/log", true},
		{"/etc/kubernetes/pki", true},
		{"/run/containerd", true},
		{"/", false},
		{"", false},
		{"var/lib/kubelet", false},
		{"/var/lib/kubelet-evil", false},
		{"/var/lib/kubelet/../../etc/shadow", false},
		{"/home", false},
		{"/var/lib", false},
		{" /var/lib/kubelet", false}, // raw value, no trimming — matches the CEL rule
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := PathAllowed(tt.path); got != tt.want {
				t.Fatalf("PathAllowed(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestSocketPathAllowed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want bool
	}{
		{"/run/containerd/containerd.sock", true},
		{"/var/run/containerd/containerd.sock", true},
		{"/run/crio/crio.sock", true},
		{"/run/cri-dockerd/cri-dockerd.sock", true},
		{"/run/containerd/containerd", false},
		{"/var/run/docker.sock", false},
		{"/run/containerd/../docker.sock", false},
		{"/run/containerd-evil/x.sock", false},
		{"", false},
		{"run/containerd/containerd.sock", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := SocketPathAllowed(tt.path); got != tt.want {
				t.Fatalf("SocketPathAllowed(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestFilterAllowedItems(t *testing.T) {
	t.Parallel()
	in := []Item{
		{Type: TypeDiskHeadroom, Path: "/var/lib/kubelet"},
		{Type: TypeDiskHeadroom, Path: "/home"},
		{Type: TypeInodeHeadroom, Path: "/"},
		{Type: TypeKubeletHealthz},
		{Type: TypeNodeCondition},
		{Type: TypeContainerRuntime},
		{Type: TypeContainerRuntime, SocketPath: "/run/crio/crio.sock"},
		{Type: TypeContainerRuntime, SocketPath: "/var/run/docker.sock"},
	}
	got := FilterAllowedItems(in)
	want := []Item{
		{Type: TypeDiskHeadroom, Path: "/var/lib/kubelet"},
		{Type: TypeKubeletHealthz},
		{Type: TypeNodeCondition},
		{Type: TypeContainerRuntime},
		{Type: TypeContainerRuntime, SocketPath: "/run/crio/crio.sock"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d items %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestMountDirs(t *testing.T) {
	t.Parallel()
	got := MountDirs([]Item{
		{Type: TypeDiskHeadroom, Path: "/var/lib/kubelet/pods"},
		{Type: TypeInodeHeadroom, Path: "/var/lib/kubelet"},
		{Type: TypeDiskHeadroom, Path: "/var/log"},
		{Type: TypeKubeletHealthz},
		{Type: TypeContainerRuntime, SocketPath: "/run/containerd/containerd.sock"},
	})
	want := []string{"/var/lib/kubelet", "/var/log"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("MountDirs = %v, want %v (descendants collapsed, sockets excluded)", got, want)
	}
}

// TestAllowlistsMirrorCRDRules pins the Go allowlists to the CEL rules in the
// generated CRD: every prefix here must appear in the schema rule, and every
// literal in the rule must be here, so the admission-time and operator-time
// checks cannot drift apart silently.
func TestAllowlistsMirrorCRDRules(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "crd", "bases", "fathom.skaphos.io_nodehealthchecks.yaml"))
	if err != nil {
		t.Fatalf("read generated CRD: %v", err)
	}
	crd := string(raw)
	for _, pre := range AllowedPathPrefixes() {
		if !strings.Contains(crd, "'"+pre+"'") {
			t.Errorf("headroom prefix %q is allowed in Go but absent from the CRD path rule", pre)
		}
	}
	for _, dir := range AllowedSocketDirs() {
		if !strings.Contains(crd, "'"+dir+"'") {
			t.Errorf("socket dir %q is allowed in Go but absent from the CRD socketPath rule", dir)
		}
	}
	// Reverse direction: pull every quoted literal out of the two rules and
	// make sure the Go side knows it.
	known := map[string]bool{}
	for _, p := range AllowedPathPrefixes() {
		known[p] = true
	}
	for _, d := range AllowedSocketDirs() {
		known[d] = true
	}
	// Only the literals inside the allowlist array count: the same line also
	// carries `self.path != '/'` and `a + '/'`, which are not allowances.
	for _, line := range strings.Split(crd, "\n") {
		if !strings.Contains(line, ".exists(a,") {
			continue
		}
		open, close := strings.Index(line, "["), strings.Index(line, "]")
		if open < 0 || close < open {
			t.Fatalf("cannot locate the allowlist array in rule line %q", line)
		}
		for _, lit := range strings.Split(line[open:close], "'") {
			if strings.HasPrefix(lit, "/") && !known[lit] {
				t.Errorf("CRD rule allows %q but the Go allowlists do not", lit)
			}
		}
	}
}

// TestCheckTypesMirrorAPI pins the wire-contract type strings to the API enum.
// The package deliberately does not import the API constants (see the package
// comment), so this is what keeps the two in step.
func TestCheckTypesMirrorAPI(t *testing.T) {
	t.Parallel()
	pairs := map[string]fathomv1alpha1.NodeHealthCheckType{
		TypeDiskHeadroom:     fathomv1alpha1.NodeHealthCheckDiskHeadroom,
		TypeInodeHeadroom:    fathomv1alpha1.NodeHealthCheckInodeHeadroom,
		TypeNodeCondition:    fathomv1alpha1.NodeHealthCheckNodeCondition,
		TypeKubeletHealthz:   fathomv1alpha1.NodeHealthCheckKubeletHealthz,
		TypeContainerRuntime: fathomv1alpha1.NodeHealthCheckContainerRuntime,
	}
	for wire, api := range pairs {
		if wire != string(api) {
			t.Errorf("wire type %q != api type %q", wire, api)
		}
	}
	if DefaultProbeSocket := fathomv1alpha1.DefaultNodeHealthContainerRuntimeSocket; !SocketPathAllowed(DefaultProbeSocket) {
		t.Errorf("the API default socket %q is not allowed by SocketPathAllowed", DefaultProbeSocket)
	}
}

func TestItemPrivilegeFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ                       string
		hostNet, root, agentEvals bool
	}{
		{TypeDiskHeadroom, false, false, true},
		{TypeInodeHeadroom, false, false, true},
		{TypeNodeCondition, false, false, false},
		{TypeKubeletHealthz, true, false, true},
		{TypeContainerRuntime, false, true, true},
	}
	for _, tt := range tests {
		it := Item{Type: tt.typ}
		if it.NeedsHostNetwork() != tt.hostNet || it.NeedsRoot() != tt.root || it.AgentEvaluated() != tt.agentEvals {
			t.Errorf("%s: hostNet=%v root=%v agent=%v, want %v %v %v", tt.typ,
				it.NeedsHostNetwork(), it.NeedsRoot(), it.AgentEvaluated(), tt.hostNet, tt.root, tt.agentEvals)
		}
	}
}
