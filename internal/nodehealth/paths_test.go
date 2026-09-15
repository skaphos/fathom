/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodehealth_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodehealth"
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
			if got := nodehealth.PathAllowed(tt.path); got != tt.want {
				t.Fatalf("nodehealth.PathAllowed(%q) = %v, want %v", tt.path, got, tt.want)
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
		{"/run/k3s/containerd/containerd.sock", true}, // k3s / RKE2
		{"/var/run/cri-dockerd.sock", true},           // cri-dockerd's actual default: a file directly in /var/run
		{"/run/cri-dockerd.sock", true},
		{"/run/docker.sock", false}, // no directory allowance for /run itself
		{"/var/run/cri-dockerd.sock/x.sock", false},
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
			if got := nodehealth.SocketPathAllowed(tt.path); got != tt.want {
				t.Fatalf("nodehealth.SocketPathAllowed(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMountDirs(t *testing.T) {
	t.Parallel()
	got := nodehealth.MountDirs([]nodehealth.Item{
		{Type: nodehealth.TypeDiskHeadroom, Path: "/var/lib/kubelet/pods"},
		{Type: nodehealth.TypeInodeHeadroom, Path: "/var/lib/kubelet"},
		{Type: nodehealth.TypeDiskHeadroom, Path: "/var/log"},
		{Type: nodehealth.TypeKubeletHealthz},
		{Type: nodehealth.TypeContainerRuntime, SocketPath: "/run/containerd/containerd.sock"},
	})
	want := []string{"/var/lib/kubelet", "/var/lib/kubelet/pods", "/var/log"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("nodehealth.MountDirs = %v, want %v (every requested directory, parents first, sockets excluded)", got, want)
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
	for _, pre := range nodehealth.AllowedPathPrefixes() {
		if !strings.Contains(crd, "'"+pre+"'") {
			t.Errorf("headroom prefix %q is allowed in Go but absent from the CRD path rule", pre)
		}
	}
	for _, dir := range nodehealth.AllowedSocketDirs() {
		if !strings.Contains(crd, "'"+dir+"'") {
			t.Errorf("socket dir %q is allowed in Go but absent from the CRD socketPath rule", dir)
		}
	}
	for _, file := range nodehealth.AllowedSocketFiles() {
		if !strings.Contains(crd, "'"+file+"'") {
			t.Errorf("socket file %q is allowed in Go but absent from the CRD socketPath rule", file)
		}
	}
	// Reverse direction: every path-like literal in an allowlist rule must be
	// known on the Go side. The rule lines also carry `self.path != '/'` and
	// `a + '/'`; a lone "/" is not an allowance and is skipped.
	known := map[string]bool{}
	for _, p := range nodehealth.AllowedPathPrefixes() {
		known[p] = true
	}
	for _, d := range nodehealth.AllowedSocketDirs() {
		known[d] = true
	}
	for _, f := range nodehealth.AllowedSocketFiles() {
		known[f] = true
	}
	for _, line := range strings.Split(crd, "\n") {
		if !strings.Contains(line, ".exists(") {
			continue
		}
		for _, lit := range strings.Split(line, "'") {
			if len(lit) > 1 && strings.HasPrefix(lit, "/") && !known[lit] {
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
		nodehealth.TypeDiskHeadroom:     fathomv1alpha1.NodeHealthCheckDiskHeadroom,
		nodehealth.TypeInodeHeadroom:    fathomv1alpha1.NodeHealthCheckInodeHeadroom,
		nodehealth.TypeNodeCondition:    fathomv1alpha1.NodeHealthCheckNodeCondition,
		nodehealth.TypeKubeletHealthz:   fathomv1alpha1.NodeHealthCheckKubeletHealthz,
		nodehealth.TypeContainerRuntime: fathomv1alpha1.NodeHealthCheckContainerRuntime,
	}
	for wire, api := range pairs {
		if wire != string(api) {
			t.Errorf("wire type %q != api type %q", wire, api)
		}
	}
	if DefaultProbeSocket := fathomv1alpha1.DefaultNodeHealthContainerRuntimeSocket; !nodehealth.SocketPathAllowed(DefaultProbeSocket) {
		t.Errorf("the API default socket %q is not allowed by nodehealth.SocketPathAllowed", DefaultProbeSocket)
	}
}

func TestItemPrivilegeFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ                       string
		hostNet, root, agentEvals bool
	}{
		{nodehealth.TypeDiskHeadroom, false, false, true},
		{nodehealth.TypeInodeHeadroom, false, false, true},
		{nodehealth.TypeNodeCondition, false, false, false},
		{nodehealth.TypeKubeletHealthz, true, false, true},
		{nodehealth.TypeContainerRuntime, false, true, true},
	}
	for _, tt := range tests {
		it := nodehealth.Item{Type: tt.typ}
		if it.NeedsHostNetwork() != tt.hostNet || it.NeedsRoot() != tt.root || it.AgentEvaluated() != tt.agentEvals {
			t.Errorf("%s: hostNet=%v root=%v agent=%v, want %v %v %v", tt.typ,
				it.NeedsHostNetwork(), it.NeedsRoot(), it.AgentEvaluated(), tt.hostNet, tt.root, tt.agentEvals)
		}
	}
}

// TestMountDirsKeepsDottedDirectories pins that a headroom path is always a
// directory: a dot in its last segment (/var/log/app.v1) must not turn it
// into its parent, and a requested nested directory is its own mount so an
// absent one is created and measured rather than Skipped. The host root is
// never mounted.
func TestMountDirsKeepsDottedDirectories(t *testing.T) {
	t.Parallel()
	got := nodehealth.MountDirs([]nodehealth.Item{
		{Type: nodehealth.TypeDiskHeadroom, Path: "/var/log/app.v1"},
		{Type: nodehealth.TypeInodeHeadroom, Path: "/var/log/app.v1/nested"},
		{Type: nodehealth.TypeDiskHeadroom, Path: "/"},
	})
	if strings.Join(got, ",") != "/var/log/app.v1,/var/log/app.v1/nested" {
		t.Fatalf("MountDirs = %v, want the dotted directory and its nested child as their own mounts, root dropped", got)
	}
}
