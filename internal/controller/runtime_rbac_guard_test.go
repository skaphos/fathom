/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller_test

import (
	"strings"
	"testing"
)

// Runtime addon definitions deliberately add no authority beyond reading their
// own two kinds and writing their status. The absences below are the feature's
// security argument, so they are asserted against the generated ClusterRole
// rather than left to review: a marker added in a later change would otherwise
// widen the operator silently, and the justification-table guard next door only
// proves a rule is *documented*, not that a forbidden one is absent.
func TestRuntimeDefinitionsAddNoForbiddenOperatorGrants(t *testing.T) {
	role := loadOperatorClusterRole(t)

	const (
		fathomGroup       = "fathom.skaphos.io"
		coordinationGroup = "coordination.k8s.io"
		authzGroup        = "authorization.k8s.io"
		rbacGroup         = "rbac.authorization.k8s.io"
	)
	runtimeKinds := map[string]bool{"addondefinitions": true, "addondefinitionbindings": true}
	writeVerbs := map[string]bool{"create": true, "update": true, "patch": true, "delete": true, "deletecollection": true}

	for _, rule := range role.Rules {
		groups := set(rule.APIGroups)
		resources := set(rule.Resources)
		verbs := set(rule.Verbs)

		// A binding is an administrator's authorization that a definition may
		// borrow a dedicated identity. An operator that could write a binding
		// spec could enable a disabled one, retarget it at another identity, or
		// widen its namespace scope -- i.e. authorize itself.
		if groups[fathomGroup] {
			for resource := range resources {
				if !runtimeKinds[resource] {
					continue
				}
				for verb := range verbs {
					if writeVerbs[verb] {
						t.Errorf("%s grants %q on %s/%s; the operator must never write a definition or binding spec",
							operatorRolePath, verb, fathomGroup, resource)
					}
				}
			}
		}

		// The drain path reads the leader-election Lease in the configured
		// operator namespace, which the existing namespaced election Role
		// already grants. A ClusterRole Lease rule would turn a namespaced read
		// into cluster-wide reconnaissance.
		if groups[coordinationGroup] && resources["leases"] {
			t.Errorf("%s grants cluster-wide Lease access; the drain path must ride the namespaced leader-election Role instead",
				operatorRolePath)
		}

		// requestedReads is a bounded manifest and diagnostic, never an
		// effective-permission oracle -- diagnostics report the requests the
		// run actually made, so no access-review grant is required.
		if groups[authzGroup] {
			for resource := range resources {
				if strings.HasPrefix(resource, "subjectaccessreviews") || strings.HasPrefix(resource, "selfsubjectaccessreviews") {
					t.Errorf("%s grants %s/%s; permission diagnostics must reflect requests actually made, not access reviews",
						operatorRolePath, authzGroup, resource)
				}
			}
		}

		// Escalation prevention: granting a runtime definition new permissions
		// would require the operator to hold them first. It must not.
		if groups[rbacGroup] {
			for _, forbidden := range []string{"clusterroles", "clusterrolebindings"} {
				if resources[forbidden] {
					t.Errorf("%s grants %s/%s; runtime definitions receive grants reviewed into Git, never minted by the operator",
						operatorRolePath, rbacGroup, forbidden)
				}
			}
			for _, forbidden := range []string{"bind", "escalate"} {
				if verbs[forbidden] {
					t.Errorf("%s grants the %q verb on %s; the operator must not be able to confer authority it does not hold",
						operatorRolePath, forbidden, rbacGroup)
				}
			}
		}
	}
}

// The two runtime kinds must actually be readable, so the absences above cannot
// be satisfied by simply never granting anything.
func TestRuntimeDefinitionsAreReadable(t *testing.T) {
	role := loadOperatorClusterRole(t)

	want := map[string][]string{
		"addondefinitions":               {"get", "list", "watch"},
		"addondefinitionbindings":        {"get", "list", "watch"},
		"addondefinitions/status":        {"get", "patch", "update"},
		"addondefinitionbindings/status": {"get", "patch", "update"},
	}
	granted := map[string]map[string]bool{}
	for _, rule := range role.Rules {
		if !set(rule.APIGroups)["fathom.skaphos.io"] {
			continue
		}
		for _, resource := range rule.Resources {
			if _, ok := want[resource]; !ok {
				continue
			}
			if granted[resource] == nil {
				granted[resource] = map[string]bool{}
			}
			for verb := range set(rule.Verbs) {
				granted[resource][verb] = true
			}
		}
	}
	for resource, verbs := range want {
		for _, verb := range verbs {
			if !granted[resource][verb] {
				t.Errorf("%s does not grant %q on fathom.skaphos.io/%s, which the reconcilers require",
					operatorRolePath, verb, resource)
			}
		}
	}
}

func set(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}
