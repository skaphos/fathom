/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package coredns

import (
	"testing"

	"github.com/skaphos/fathom/pkg/adapter"
)

// TestRBACRulesDeclaresProbeException locks CoreDNS's one write exception in
// place: the read-only RBAC guard only permits the probe-pod create;delete
// because it carries a WriteReason, and dropping it would silently break the
// DNS-resolution probe under the scoped ServiceAccount (SKA-58).
func TestRBACRulesDeclaresProbeException(t *testing.T) {
	var _ adapter.RBACDeclarer = Adapter{}

	rules := Adapter{}.RBACRules()
	if len(rules) == 0 {
		t.Fatal("RBACRules() returned no rules")
	}

	var found bool
	for _, r := range rules {
		if r.IsReadOnly() {
			continue
		}
		// Every non-read rule must justify itself, or the guard would fail it.
		if r.WriteReason == "" {
			t.Errorf("write rule %+v has no WriteReason", r)
		}
		if hasResource(r, "pods") && hasVerb(r, "create") && hasVerb(r, "delete") {
			found = true
		}
	}
	if !found {
		t.Error("expected a pods create;delete write exception for the probe pod")
	}
}

func hasResource(r adapter.PolicyRule, want string) bool {
	for _, res := range r.Resources {
		if res == want {
			return true
		}
	}
	return false
}

func hasVerb(r adapter.PolicyRule, want string) bool {
	for _, v := range r.Verbs {
		if v == want {
			return true
		}
	}
	return false
}
