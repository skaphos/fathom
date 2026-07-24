/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodelocaldns

import (
	"testing"

	"github.com/skaphos/fathom/pkg/adapter"
)

// TestRBACRulesDeclaresProbeException locks NodeLocal DNSCache's one write
// exception in place: the RBAC guard only permits the probe-pod create;delete
// because it carries a Justification, and dropping it would silently break the
// DNS-resolution probe under the scoped ServiceAccount (SKA-58).
func TestRBACRulesDeclaresProbeException(t *testing.T) {
	var _ adapter.RBACDeclarer = Adapter{}

	rules := Adapter{}.RBACRules()
	if len(rules) == 0 {
		t.Fatal("RBACRules() returned no rules")
	}

	var found bool
	for _, r := range rules {
		// Every rule — read or write — must justify itself, or the guard fails it.
		if r.Justification == "" {
			t.Errorf("rule %+v has no Justification", r)
		}
		if r.IsReadOnly() {
			continue
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
