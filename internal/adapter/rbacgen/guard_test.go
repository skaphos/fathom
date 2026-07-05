/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package rbacgen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/skaphos/fathom/internal/adapter/rbacgen"
	"github.com/skaphos/fathom/internal/app"
	"github.com/skaphos/fathom/pkg/adapter"
)

// writeKey uniquely identifies one (addon, apiGroup, resource, verb) write grant
// so the model's declared exceptions can be matched against the committed roles.
type writeKey struct{ addon, group, resource, verb string }

// allowedWrites builds the set of write grants the adapters declared with a
// WriteReason — the single source of the write-exception allowlist.
func allowedWrites(addons []rbacgen.AddonRBAC) map[writeKey]bool {
	allowed := map[writeKey]bool{}
	for _, a := range addons {
		for _, r := range a.Rules {
			if r.WriteReason == "" {
				continue
			}
			for _, v := range r.Verbs {
				if adapter.IsReadVerb(v) {
					continue
				}
				for _, g := range r.APIGroups {
					for _, res := range r.Resources {
						allowed[writeKey{a.Addon, g, res, v}] = true
					}
				}
			}
		}
	}
	return allowed
}

// TestModelWritesAreJustified enforces the core invariant directly on the
// compiled-in adapters' declared rules: no write verb without a WriteReason.
func TestModelWritesAreJustified(t *testing.T) {
	if v := rbacgen.UnjustifiedWrites(rbacgen.Collect(app.BuiltInAdapters())); len(v) > 0 {
		t.Errorf("unjustified writes in built-in adapters:\n%s", strings.Join(v, "\n"))
	}
}

// TestUnjustifiedWritesCatchesViolations proves the guard actually flags a rogue
// write — a write verb with no WriteReason, and an addon with no rules — so a
// passing TestModelWritesAreJustified is meaningful rather than vacuous.
func TestUnjustifiedWritesCatchesViolations(t *testing.T) {
	bad := []rbacgen.AddonRBAC{
		{Addon: "rogue", ServiceAccount: "addon-rogue", Rules: []adapter.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get", "delete"}}, // delete, no reason
		}},
		{Addon: "empty", ServiceAccount: "addon-empty"}, // no rules at all
		{Addon: "ok", ServiceAccount: "addon-ok", Rules: []adapter.PolicyRule{
			{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"create"}, WriteReason: "probe pod"},
		}},
	}
	got := rbacgen.UnjustifiedWrites(bad)
	if len(got) != 2 {
		t.Fatalf("expected 2 violations (rogue delete + empty addon), got %d: %v", len(got), got)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "rogue") || !strings.Contains(joined, "empty") {
		t.Errorf("violations should name the rogue and empty addons, got:\n%s", joined)
	}
	if strings.Contains(joined, "\"ok\"") {
		t.Errorf("the justified addon must not be flagged, got:\n%s", joined)
	}
}

// TestCommittedAddonRolesAreReadOnly re-parses the committed per-addon
// ClusterRoles under config/rbac/addons/ and fails if any grants a write verb
// that the model does not justify with a WriteReason. This guards the shipped
// artifact independently of the generator, so a hand-edit that broadens a role
// cannot pass review (SKA-58).
func TestCommittedAddonRolesAreReadOnly(t *testing.T) {
	root := repoRoot(t)
	allowed := allowedWrites(rbacgen.Collect(app.BuiltInAdapters()))

	dir := filepath.Join(root, "config", "rbac", "addons")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	var checkedRoles int
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "addon-") || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, doc := range strings.Split(string(body), "\n---\n") {
			var obj struct {
				Kind     string `json:"kind"`
				Metadata struct {
					Labels map[string]string `json:"labels"`
				} `json:"metadata"`
				Rules []struct {
					APIGroups []string `json:"apiGroups"`
					Resources []string `json:"resources"`
					Verbs     []string `json:"verbs"`
				} `json:"rules"`
			}
			if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
				t.Fatalf("parse doc in %s: %v", e.Name(), err)
			}
			if obj.Kind != "ClusterRole" {
				continue
			}
			checkedRoles++
			addon := obj.Metadata.Labels[adapter.AddonLabel]
			if addon == "" {
				t.Errorf("%s: ClusterRole has no %s label", e.Name(), adapter.AddonLabel)
			}
			for _, r := range obj.Rules {
				for _, v := range r.Verbs {
					if adapter.IsReadVerb(v) {
						continue
					}
					for _, g := range r.APIGroups {
						for _, res := range r.Resources {
							if !allowed[writeKey{addon, g, res, v}] {
								t.Errorf("%s: unjustified write %q on group=%q resource=%q for addon %q — the committed role grants a write the adapter does not declare with a WriteReason", e.Name(), v, g, res, addon)
							}
						}
					}
				}
			}
		}
	}
	if checkedRoles == 0 {
		t.Fatalf("no committed addon ClusterRoles found under %s — did generation run?", dir)
	}
}

// repoRoot walks up from the test's working directory to the module root
// (the directory containing go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root (go.mod) above the test directory")
		}
		dir = parent
	}
}
