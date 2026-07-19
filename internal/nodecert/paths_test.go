/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodecert

import (
	"reflect"
	"strings"
	"testing"
)

func TestPathAllowed(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"under kubernetes prefix", "/etc/kubernetes/pki/apiserver.crt", true},
		{"exact allowed prefix", "/etc/kubernetes", true},
		{"kubelet pki", "/var/lib/kubelet/pki", true},
		{"etcd pki", "/etc/etcd/pki", true},
		{"var lib etcd", "/var/lib/etcd", true},
		{"rancher k3s tls", "/var/lib/rancher/k3s/server/tls", true},
		{"trailing slash still under prefix", "/etc/kubernetes/", true},

		{"host root rejected", "/", false},
		{"empty rejected", "", false},
		{"relative rejected", "etc/kubernetes/pki", false},
		// Leading whitespace must be rejected, not normalized: the CRD CEL rule
		// validates the raw value (a leading space fails startsWith('/')), so trimming
		// here would accept inputs the API server rejects and break Go<->CRD lockstep.
		{"leading space rejected", " /etc/kubernetes/pki", false},
		// A trailing space stays inside the prefix, so both PathAllowed and the CEL
		// rule accept it — asserted here to pin that the two agree at this boundary.
		{"trailing space matches CEL (allowed)", "/etc/kubernetes/pki ", true},
		{"traversal rejected", "/etc/kubernetes/../../root/.ssh", false},
		{"traversal embedded rejected", "/var/lib/kubelet/../../../etc/shadow", false},
		{"outside allowlist rejected", "/root/.ssh/id_rsa", false},
		{"sibling-prefix confusion rejected", "/etc/kubernetes-secrets/ca.crt", false},
		{"proc rejected", "/proc/1/environ", false},
		{"secrets mount rejected", "/var/run/secrets/kubernetes.io", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PathAllowed(tc.path); got != tc.want {
				t.Fatalf("PathAllowed(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// TestDefaultCertPathsAreAllowed guards the invariant that every built-in scan
// path stays inside the operator-approved allowlist — otherwise the default scan
// set would be silently dropped by FilterAllowedPaths / rejected at admission.
func TestDefaultCertPathsAreAllowed(t *testing.T) {
	for _, p := range DefaultCertPaths() {
		if !PathAllowed(p) {
			t.Errorf("default cert path %q is not under an allowed prefix %v", p, AllowedPathPrefixes())
		}
	}
}

func TestFilterAllowedPaths(t *testing.T) {
	in := []string{
		"/etc/kubernetes/pki",       // keep
		"/root/.ssh/id_rsa",         // drop: outside allowlist
		"/var/lib/kubelet/pki",      // keep
		"/etc/kubernetes/../secret", // drop: traversal
		"/",                         // drop: host root
		"relative/path",             // drop: not absolute
	}
	want := []string{"/etc/kubernetes/pki", "/var/lib/kubelet/pki"}
	if got := FilterAllowedPaths(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("FilterAllowedPaths(%v) = %v, want %v", in, got, want)
	}
}

// TestAllowedPathPrefixesMatchCRDRule keeps the Go allowlist in lockstep with the
// prefixes hard-coded in the NodeCertificateCheck CRD's x-kubernetes-validations
// rule. If this fails, update both together.
func TestAllowedPathPrefixesMatchCRDRule(t *testing.T) {
	want := []string{"/etc/kubernetes", "/var/lib/kubelet", "/etc/etcd", "/var/lib/etcd", "/var/lib/rancher"}
	got := AllowedPathPrefixes()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("AllowedPathPrefixes() = %v, want %v (keep in sync with the CRD CEL rule)", got, want)
	}
}
