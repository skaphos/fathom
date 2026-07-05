/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package rbacgen_test

import (
	"testing"

	"github.com/skaphos/fathom/internal/adapter/rbacgen"
	"github.com/skaphos/fathom/pkg/adapter"
)

// TestFilesRejectsIncompleteRule proves the codegen guardrail fails fast on a
// PolicyRule that would render an unapplyable manifest — one missing apiGroups,
// resources, or verbs — instead of emitting RBAC the API server rejects at apply
// time. The core group is the "" element, so a rule with APIGroups: []string{""}
// is complete and must NOT trip the guard.
func TestFilesRejectsIncompleteRule(t *testing.T) {
	cases := []struct {
		name    string
		rule    adapter.PolicyRule
		wantErr bool
	}{
		{
			name:    "complete core-group rule",
			rule:    adapter.PolicyRule{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "list"}, Justification: "read config"},
			wantErr: false,
		},
		{
			name:    "missing apiGroups",
			rule:    adapter.PolicyRule{Resources: []string{"configmaps"}, Verbs: []string{"get"}, Justification: "read config"},
			wantErr: true,
		},
		{
			name:    "missing resources",
			rule:    adapter.PolicyRule{APIGroups: []string{""}, Verbs: []string{"get"}, Justification: "read config"},
			wantErr: true,
		},
		{
			name:    "missing verbs",
			rule:    adapter.PolicyRule{APIGroups: []string{""}, Resources: []string{"configmaps"}, Justification: "read config"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addons := []rbacgen.AddonRBAC{{
				Addon:          "probe-addon",
				ServiceAccount: "addon-probe-addon",
				Rules:          []adapter.PolicyRule{tc.rule},
			}}
			_, err := rbacgen.Files(addons)
			if tc.wantErr && err == nil {
				t.Fatalf("Files() returned nil error, want an error for an incomplete rule")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Files() = %v, want no error for a complete rule", err)
			}
		})
	}
}
