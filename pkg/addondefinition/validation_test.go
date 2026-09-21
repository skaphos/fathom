/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package addondefinition_test

import (
	"fmt"
	"strings"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validDefinition() *api.AddonDefinition {
	return &api.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: "custom-addon"}, Spec: api.AddonDefinitionSpec{
		AddonType: "custom-addon", AdapterVersion: "1.0.0", SemanticsVersion: 1,
		Families: []api.DefinitionFamily{{Name: "health", Checks: []api.DefinitionCheck{{Name: "controller", Kind: "Workload", Workload: &api.DefinitionWorkload{
			Target: api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{"default"}}, Kind: "Deployment", DefaultName: "controller",
		}}}}},
	}}
}

func TestDefinitionSemanticValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(*api.AddonDefinition)
		valid bool
	}{
		{"valid", func(*api.AddonDefinition) {}, true},
		{"unsupported semantics", func(d *api.AddonDefinition) { d.Spec.SemanticsVersion = 2 }, false},
		{"strict semver", func(d *api.AddonDefinition) { d.Spec.AdapterVersion = "latest" }, false},
		{"canonical identity", func(d *api.AddonDefinition) { d.Spec.AddonType = "CUSTOM-addon" }, false},
		{"missing source", func(d *api.AddonDefinition) { d.Spec.SupportedVersions = ">=1.0" }, false},
		{"reserved threshold", func(d *api.AddonDefinition) { d.Spec.Families[0].Checks[0].Workload.NameThresholdKey = "warnRatio" }, false},
		{"invalid namespace", func(d *api.AddonDefinition) {
			d.Spec.Families[0].Checks[0].Workload.Target.Namespaces = []api.DefinitionDNSLabel{"*"}
		}, false},
		{"negative restart", func(d *api.AddonDefinition) { d.Spec.Families[0].Checks[0].Workload.DefaultRestartWarn = -1 }, false},
		{"duplicate check", func(d *api.AddonDefinition) { f := &d.Spec.Families[0]; f.Checks = append(f.Checks, f.Checks[0]) }, false},
		{"mismatched payload", func(d *api.AddonDefinition) { d.Spec.Families[0].Checks[0].Kind = "CRD" }, false},
		{"too many families", func(d *api.AddonDefinition) {
			for len(d.Spec.Families) <= 16 {
				d.Spec.Families = append(d.Spec.Families, d.Spec.Families[0])
			}
		}, false},
		{"overlong version", func(d *api.AddonDefinition) { d.Spec.AdapterVersion = strings.Repeat("1", 257) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := validDefinition()
			tc.edit(d)
			if err := definitions.Validate(d); (err == nil) != tc.valid {
				t.Fatalf("valid=%v want=%v: %v", err == nil, tc.valid, err)
			}
		})
	}
}

func TestRequestedReadsRejectsAuthorityExpansion(t *testing.T) {
	core := ""
	for _, tc := range []struct {
		name  string
		rule  api.DefinitionReadRule
		valid bool
	}{
		{"exact core list", api.DefinitionReadRule{APIGroup: &core, Resources: []string{"pods"}, Verbs: []string{"get", "list"}}, true},
		{"exact discovery", api.DefinitionReadRule{NonResourceURLs: []string{"/api", "/apis/apps/v1"}, Verbs: []string{"get"}}, true},
		{"wildcard", api.DefinitionReadRule{APIGroup: &core, Resources: []string{"*"}, Verbs: []string{"get"}}, false},
		{"write", api.DefinitionReadRule{APIGroup: &core, Resources: []string{"pods"}, Verbs: []string{"create"}}, false},
		{"subresource", api.DefinitionReadRule{APIGroup: &core, Resources: []string{"pods/exec"}, Verbs: []string{"get"}}, false},
		{"arbitrary url", api.DefinitionReadRule{NonResourceURLs: []string{"https://example.com"}, Verbs: []string{"get"}}, false},
		{"query", api.DefinitionReadRule{NonResourceURLs: []string{"/api/v1?x=y"}, Verbs: []string{"get"}}, false},
		{"encoded traversal", api.DefinitionReadRule{NonResourceURLs: []string{"/apis/%2e%2e/v1"}, Verbs: []string{"get"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := validDefinition()
			d.Spec.RequestedReads = []api.DefinitionReadRule{tc.rule}
			if err := definitions.Validate(d); (err == nil) != tc.valid {
				t.Fatalf("valid=%v want=%v: %v", err == nil, tc.valid, err)
			}
		})
	}
}

func TestVersionComparatorLimitIncludesAllWhitespace(t *testing.T) {
	for _, separator := range []string{" ", ",", "\t", "\n", "\r", "\f"} {
		t.Run(fmt.Sprintf("separator-%q", separator), func(t *testing.T) {
			at := strings.TrimSuffix(strings.Repeat(">=1"+separator, 16), separator)
			over := at + separator + ">=1"
			if err := definitions.ValidateVersionRange(at); err != nil {
				t.Fatalf("at limit rejected: %v", err)
			}
			if err := definitions.ValidateVersionRange(over); err == nil {
				t.Fatal("17 comparators accepted")
			}
		})
	}
}
