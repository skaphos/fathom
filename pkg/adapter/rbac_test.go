/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package adapter_test

import (
	"testing"

	"github.com/skaphos/fathom/pkg/adapter"
)

func TestAddonServiceAccountName(t *testing.T) {
	if got, want := adapter.AddonServiceAccountName("cilium"), "addon-cilium"; got != want {
		t.Fatalf("AddonServiceAccountName(cilium) = %q, want %q", got, want)
	}
	if got, want := adapter.AddonServiceAccountName("cert-manager"), "addon-cert-manager"; got != want {
		t.Fatalf("AddonServiceAccountName(cert-manager) = %q, want %q", got, want)
	}
	// It composes from the exported prefix so the generator and reconciler agree.
	if adapter.AddonServiceAccountName("x") != adapter.AddonServiceAccountPrefix+"x" {
		t.Fatalf("AddonServiceAccountName must be AddonServiceAccountPrefix + addon")
	}
}

func TestIsReadVerb(t *testing.T) {
	for _, v := range []string{"get", "list", "watch"} {
		if !adapter.IsReadVerb(v) {
			t.Errorf("IsReadVerb(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"create", "update", "patch", "delete", "deletecollection", "*", "impersonate", ""} {
		if adapter.IsReadVerb(v) {
			t.Errorf("IsReadVerb(%q) = true, want false", v)
		}
	}
}

func TestPolicyRuleIsReadOnly(t *testing.T) {
	tests := []struct {
		name string
		rule adapter.PolicyRule
		want bool
	}{
		{
			name: "all reads",
			rule: adapter.PolicyRule{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list", "watch"}},
			want: true,
		},
		{
			name: "empty verbs is vacuously read-only",
			rule: adapter.PolicyRule{APIGroups: []string{""}, Resources: []string{"pods"}},
			want: true,
		},
		{
			name: "a single write verb makes it not read-only",
			rule: adapter.PolicyRule{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "create"}},
			want: false,
		},
		{
			name: "write verb even with a WriteReason is still not read-only",
			rule: adapter.PolicyRule{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"delete"}, WriteReason: "probe teardown"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.IsReadOnly(); got != tt.want {
				t.Fatalf("IsReadOnly() = %v, want %v", got, tt.want)
			}
		})
	}
}
