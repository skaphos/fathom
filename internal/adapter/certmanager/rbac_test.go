/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package certmanager

import (
	"testing"

	"github.com/skaphos/fathom/pkg/adapter"
)

// TestRBACRulesDeclaresDryRunException locks cert-manager's one write exception
// in place: the read-only RBAC guard only permits the certificates;issuers create
// because it carries a WriteReason. Dropping it would silently break the
// admission dry-run probe under the scoped ServiceAccount (SKA-58).
func TestRBACRulesDeclaresDryRunException(t *testing.T) {
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
		if r.WriteReason == "" {
			t.Errorf("write rule %+v has no WriteReason", r)
		}
		if hasVerb(r, "create") && hasResource(r, "certificates") && hasResource(r, "issuers") {
			found = true
		}
	}
	if !found {
		t.Error("expected a certificates;issuers create write exception for the dry-run probe")
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
