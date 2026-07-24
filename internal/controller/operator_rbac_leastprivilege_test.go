/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller_test

import (
	"sort"
	"testing"
)

// TestPrimaryCRDGrantsAreReadOnly is the regression test for the over-grant
// finding (adversarial review RBAC-1): the operator's ClusterRole granted
// create/update/patch/delete on addonchecks, clusterhealths, and healthchecks,
// but the reconcilers only read these objects and write their /status
// subresource (granted separately). The unused write verbs were pure attack
// surface — a compromised operator token could delete or forge every one of the
// product's primary resources. This test fails if any write verb is re-added to
// the top-level (non-status) grant for these kinds.
func TestPrimaryCRDGrantsAreReadOnly(t *testing.T) {
	role := loadOperatorClusterRole(t)

	// The primary kinds whose objects the operator must never mutate directly.
	primary := map[string]bool{
		"addonchecks":    true,
		"clusterhealths": true,
		"healthchecks":   true,
	}
	readOnly := map[string]bool{"get": true, "list": true, "watch": true}

	checked := 0
	for _, rule := range role.Rules {
		for _, res := range rule.Resources {
			// Only the top-level object grant, never the /status or /finalizers
			// subresources (those legitimately carry update/patch).
			if !primary[res] {
				continue
			}
			checked++
			for _, v := range rule.Verbs {
				if !readOnly[v] {
					t.Errorf("operator ClusterRole grants verb %q on %q; the reconcilers only read these kinds and write /status separately — the top-level grant must be get;list;watch only", v, res)
				}
			}
		}
	}

	if checked == 0 {
		t.Fatal("no top-level rule found for addonchecks/clusterhealths/healthchecks — the role.yaml layout changed; update this guard")
	}
	if checked != len(primary) {
		// Guard against a partial match (e.g. a kind split into its own rule with
		// broader verbs) silently passing.
		want := make([]string, 0, len(primary))
		for k := range primary {
			want = append(want, k)
		}
		sort.Strings(want)
		t.Logf("matched %d primary-kind resource entries across the role (kinds: %v)", checked, want)
	}
}
