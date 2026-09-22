/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
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
		if groups[fathomGroup] || groups["*"] {
			for resource := range resources {
				if !runtimeKinds[resource] && resource != "*" {
					continue
				}
				for verb := range verbs {
					if writeVerbs[verb] || verb == "*" {
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
		if (groups[coordinationGroup] || groups["*"]) && (resources["leases"] || resources["*"]) {
			t.Errorf("%s grants cluster-wide Lease access; the drain path must ride the namespaced leader-election Role instead",
				operatorRolePath)
		}

		// requestedReads is a bounded manifest and diagnostic, never an
		// effective-permission oracle -- diagnostics report the requests the
		// run actually made, so no access-review grant is required.
		if groups[authzGroup] || groups["*"] {
			for resource := range resources {
				if resource == "*" || strings.Contains(resource, "subjectaccessreviews") || resource == "selfsubjectrulesreviews" {
					t.Errorf("%s grants %s/%s; permission diagnostics must reflect requests actually made, not access reviews",
						operatorRolePath, authzGroup, resource)
				}
			}
		}

		// Escalation prevention: granting a runtime definition new permissions
		// would require the operator to hold them first. It must not.
		if groups[rbacGroup] || groups["*"] {
			for _, forbidden := range []string{"clusterroles", "clusterrolebindings"} {
				if resources[forbidden] || resources["*"] {
					t.Errorf("%s grants %s/%s; runtime definitions receive grants reviewed into Git, never minted by the operator",
						operatorRolePath, rbacGroup, forbidden)
				}
			}
			for _, forbidden := range []string{"bind", "escalate"} {
				if verbs[forbidden] || verbs["*"] {
					t.Errorf("%s grants the %q verb on %s; the operator must not be able to confer authority it does not hold",
						operatorRolePath, forbidden, rbacGroup)
				}
			}
		}
	}
}

func TestRuntimeLeaseAccessStaysInNamespacedElectionRole(t *testing.T) {
	const path = "../../config/rbac/leader_election_role.yaml"
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	role := &rbacv1.Role{}
	if err := yaml.UnmarshalStrict(raw, role); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if role.Kind != "Role" {
		t.Fatalf("%s has kind %q, want namespaced Role", path, role.Kind)
	}
	found := false
	for _, rule := range role.Rules {
		if set(rule.APIGroups)["coordination.k8s.io"] && set(rule.Resources)["leases"] && set(rule.Verbs)["get"] {
			found = true
		}
	}
	if !found {
		t.Errorf("%s must grant get on coordination.k8s.io/leases for direct drain fences", path)
	}

	const bindingPath = "../../config/rbac/leader_election_role_binding.yaml"
	raw, err = os.ReadFile(filepath.Clean(bindingPath))
	if err != nil {
		t.Fatalf("read %s: %v", bindingPath, err)
	}
	binding := &rbacv1.RoleBinding{}
	if err := yaml.UnmarshalStrict(raw, binding); err != nil {
		t.Fatalf("parse %s: %v", bindingPath, err)
	}
	if binding.Kind != "RoleBinding" || binding.RoleRef.Kind != "Role" || binding.RoleRef.Name != role.Name {
		t.Errorf("%s must bind the namespaced leader-election Role %q", bindingPath, role.Name)
	}
	if len(binding.Subjects) != 1 || binding.Subjects[0].Kind != "ServiceAccount" || binding.Subjects[0].Name != "controller-manager" {
		t.Errorf("%s must bind only the operator ServiceAccount", bindingPath)
	}
}

func TestHelmManagerRulesMatchGeneratedClusterRole(t *testing.T) {
	const path = "../../deploy/helm/fathom-operator/files/manager-rules.yaml"
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var chart struct {
		Rules []rbacv1.PolicyRule `json:"rules"`
	}
	if err := yaml.UnmarshalStrict(append([]byte("rules:\n"), raw...), &chart); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if !reflect.DeepEqual(chart.Rules, loadOperatorClusterRole(t).Rules) {
		t.Errorf("%s differs from generated operator ClusterRole; run task helm:sync", path)
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
