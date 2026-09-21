/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package addondefinition_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCanonicalDefinitionByteBoundary(t *testing.T) {
	d := validDefinition()
	d.Spec.Families = nil
	for f := 0; f < 8; f++ {
		family := api.DefinitionFamily{Name: api.DefinitionIdentifier(fmt.Sprintf("family-%d", f))}
		for c := 0; c < 32; c++ {
			check := payloadCases()[3]
			check.Name = api.DefinitionIdentifier(fmt.Sprintf("check-%d", c))
			check.Field.ExpectedValue = "a"
			family.Checks = append(family.Checks, check)
		}
		d.Spec.Families = append(d.Spec.Families, family)
	}
	raw, err := json.Marshal(d.Spec)
	if err != nil {
		t.Fatal(err)
	}
	remaining := definitions.MaxSpecBytes - len(raw)
	var expandable *api.DefinitionField
	for fi := range d.Spec.Families {
		for ci := range d.Spec.Families[fi].Checks {
			field := d.Spec.Families[fi].Checks[ci].Field
			extra := min(remaining, definitions.MaxStringBytes-1)
			field.ExpectedValue = api.DefinitionText(strings.Repeat("a", extra+1))
			remaining -= extra
			if len(field.ExpectedValue) < definitions.MaxStringBytes {
				expandable = field
			}
		}
	}
	if remaining != 0 || expandable == nil {
		t.Fatal("invalid exact-size fixture")
	}
	raw, err = json.Marshal(d.Spec)
	if err != nil || len(raw) != definitions.MaxSpecBytes {
		t.Fatalf("size=%d err=%v", len(raw), err)
	}
	if err := definitions.Validate(d); err != nil {
		t.Fatalf("at canonical byte cap: %v", err)
	}
	expandable.ExpectedValue += "a"
	if err := definitions.Validate(d); err == nil || !strings.Contains(err.Error(), "DefinitionTooLarge") {
		t.Fatalf("over cap rejected for wrong reason: %v", err)
	}
}

func TestDefinitionStructuralBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		edit  func(*api.AddonDefinition, int)
	}{
		{"families", 16, func(d *api.AddonDefinition, n int) {
			base := d.Spec.Families[0]
			d.Spec.Families = nil
			for i := 0; i < n; i++ {
				f := base
				f.Name = api.DefinitionIdentifier(fmt.Sprintf("family-%d", i))
				d.Spec.Families = append(d.Spec.Families, f)
			}
		}},
		{"checks", 32, func(d *api.AddonDefinition, n int) {
			d.Spec.Families[0].Checks = nil
			for i := 0; i < n; i++ {
				c := payloadCases()[0]
				c.Name = api.DefinitionIdentifier(fmt.Sprintf("check-%d", i))
				d.Spec.Families[0].Checks = append(d.Spec.Families[0].Checks, c)
			}
		}},
		{"family name", 63, func(d *api.AddonDefinition, n int) {
			d.Spec.Families[0].Name = api.DefinitionIdentifier(strings.Repeat("a", n))
		}},
		{"component", 63, func(d *api.AddonDefinition, n int) {
			d.Spec.Families[0].Checks[0].Workload.Component = api.DefinitionIdentifier(strings.Repeat("a", n))
		}},
		{"field segments", 16, func(d *api.AddonDefinition, n int) {
			c := payloadCases()[3]
			c.Field.FieldPath = make([]string, n)
			for i := range c.Field.FieldPath {
				c.Field.FieldPath[i] = "value"
			}
			d.Spec.Families[0].Checks = []api.DefinitionCheck{c}
		}},
		{"field segment bytes", 128, func(d *api.AddonDefinition, n int) {
			c := payloadCases()[3]
			c.Field.FieldPath = []string{strings.Repeat("a", n)}
			d.Spec.Families[0].Checks = []api.DefinitionCheck{c}
		}},
		{"string bytes", 1024, func(d *api.AddonDefinition, n int) {
			c := payloadCases()[3]
			c.Field.ExpectedValue = api.DefinitionText(strings.Repeat("a", n))
			d.Spec.Families[0].Checks = []api.DefinitionCheck{c}
		}},
		{"outcome map", 32, func(d *api.AddonDefinition, n int) {
			c := payloadCases()[3]
			c.Field.ValueOutcomes = map[string]api.DefinitionOutcome{}
			for i := 0; i < n; i++ {
				c.Field.ValueOutcomes[fmt.Sprintf("value-%d", i)] = "Warn"
			}
			d.Spec.Families[0].Checks = []api.DefinitionCheck{c}
		}},
		{"API versions", 8, func(d *api.AddonDefinition, n int) {
			c := payloadCases()[1]
			c.CRD.SupportedVersions = nil
			for i := 0; i < n; i++ {
				c.CRD.SupportedVersions = append(c.CRD.SupportedVersions, api.DefinitionToken(fmt.Sprintf("v%d", i)))
			}
			d.Spec.Families[0].Checks = []api.DefinitionCheck{c}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, over := range []bool{false, true} {
				d := validDefinition()
				n := tc.limit
				if over {
					n++
				}
				tc.edit(d, n)
				if err := definitions.Validate(d); (err != nil) != over {
					t.Fatalf("n=%d err=%v", n, err)
				}
			}
		})
	}
}

func TestRangeAndSelectorBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name           string
		valid, invalid string
	}{
		{"comparators", strings.TrimSpace(strings.Repeat(">=1 ", 16)), strings.TrimSpace(strings.Repeat(">=1 ", 17))},
		{"alternatives", strings.Repeat("1 || ", 7) + "1", strings.Repeat("1 || ", 8) + "1"},
		{"bytes", ">=1" + strings.Repeat(" ", 253), ">=1" + strings.Repeat(" ", 254)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := definitions.ValidateVersionRange(tc.valid); err != nil {
				t.Fatal(err)
			}
			if err := definitions.ValidateVersionRange(tc.invalid); err == nil {
				t.Fatal("over-limit range accepted")
			}
		})
	}
	for _, expressions := range []bool{false, true} {
		selector := &metav1.LabelSelector{}
		if expressions {
			e := metav1.LabelSelectorRequirement{Key: "app", Operator: metav1.LabelSelectorOpIn}
			for i := 0; i < 32; i++ {
				e.Values = append(e.Values, fmt.Sprintf("v%d", i))
			}
			selector.MatchExpressions = []metav1.LabelSelectorRequirement{e}
		} else {
			selector.MatchLabels = map[string]string{}
			for i := 0; i < 32; i++ {
				selector.MatchLabels[fmt.Sprintf("key%d", i)] = "value"
			}
		}
		if err := definitions.ValidateSelector(selector); err != nil {
			t.Fatal(err)
		}
		if expressions {
			selector.MatchExpressions[0].Values = append(selector.MatchExpressions[0].Values, "overflow")
		} else {
			selector.MatchExpressions = []metav1.LabelSelectorRequirement{{Key: "overflow", Operator: metav1.LabelSelectorOpExists}}
		}
		if err := definitions.ValidateSelector(selector); err == nil {
			t.Fatal("over-limit selector accepted")
		}
	}
}

func TestRuntimeReadDeclarationGrammar(t *testing.T) {
	core, group := "", "example.org"
	for _, tc := range []struct {
		name  string
		rule  api.DefinitionReadRule
		valid bool
	}{
		{"core get", api.DefinitionReadRule{APIGroup: &core, Resources: []string{"configmaps"}, ResourceNames: []api.DefinitionResourceName{"reviewed"}, Verbs: []string{"get"}}, true},
		{"group list", api.DefinitionReadRule{APIGroup: &group, Resources: []string{"widgets"}, Verbs: []string{"list"}}, true},
		{"exact discovery", api.DefinitionReadRule{NonResourceURLs: []string{"/api", "/api/v1", "/apis", "/apis/example.org", "/apis/example.org/v1"}, Verbs: []string{"get"}}, true},
		{"missing core group", api.DefinitionReadRule{Resources: []string{"pods"}, Verbs: []string{"get"}}, false},
		{"subresource", api.DefinitionReadRule{APIGroup: &core, Resources: []string{"pods/log"}, Verbs: []string{"get"}}, false},
		{"watch", api.DefinitionReadRule{APIGroup: &core, Resources: []string{"pods"}, Verbs: []string{"watch"}}, false},
		{"duplicate resource", api.DefinitionReadRule{APIGroup: &core, Resources: []string{"pods", "pods"}, Verbs: []string{"get"}}, false},
		{"duplicate name", api.DefinitionReadRule{APIGroup: &core, Resources: []string{"pods"}, ResourceNames: []api.DefinitionResourceName{"pod", "pod"}, Verbs: []string{"get"}}, false},
		{"duplicate verb", api.DefinitionReadRule{APIGroup: &core, Resources: []string{"pods"}, Verbs: []string{"get", "get"}}, false},
		{"resource and discovery", api.DefinitionReadRule{APIGroup: &core, Resources: []string{"pods"}, NonResourceURLs: []string{"/api"}, Verbs: []string{"get"}}, false},
		{"discovery resource names", api.DefinitionReadRule{NonResourceURLs: []string{"/api"}, ResourceNames: []api.DefinitionResourceName{"pod"}, Verbs: []string{"get"}}, false},
		{"discovery list", api.DefinitionReadRule{NonResourceURLs: []string{"/api"}, Verbs: []string{"list"}}, false},
		{"absolute URL", api.DefinitionReadRule{NonResourceURLs: []string{"https://example.org/apis"}, Verbs: []string{"get"}}, false},
		{"resource URL", api.DefinitionReadRule{NonResourceURLs: []string{"/api/v1/namespaces/default/secrets"}, Verbs: []string{"get"}}, false},
		{"wildcard URL", api.DefinitionReadRule{NonResourceURLs: []string{"/apis/*"}, Verbs: []string{"get"}}, false},
		{"query URL", api.DefinitionReadRule{NonResourceURLs: []string{"/api?x=y"}, Verbs: []string{"get"}}, false},
		{"encoded traversal", api.DefinitionReadRule{NonResourceURLs: []string{"/apis/%2e%2e"}, Verbs: []string{"get"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := validDefinition()
			d.Spec.RequestedReads = []api.DefinitionReadRule{tc.rule}
			if err := definitions.Validate(d); (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestDefinitionStringBytesAndUTF8(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		valid       bool
	}{
		{"multibyte at byte cap", strings.Repeat("é", 512), true},
		{"multibyte over byte cap", strings.Repeat("é", 512) + "x", false},
		{"invalid UTF8", string([]byte{0xff}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := validDefinition()
			c := payloadCases()[3]
			c.Field.ExpectedValue = api.DefinitionText(tc.value)
			d.Spec.Families[0].Checks = []api.DefinitionCheck{c}
			if err := definitions.Validate(d); (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
