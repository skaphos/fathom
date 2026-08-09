/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// TestGeneratedCRDsEmbedCadenceFloors pins the generated CRD schemas to the
// MinCheckInterval/MinCheckTimeout constants. The CEL markers on the spec
// types spell the floors as string literals, and the controllers clamp with
// the constants — this test is what keeps the two from drifting apart.
func TestGeneratedCRDsEmbedCadenceFloors(t *testing.T) {
	wantRules := []string{
		fmt.Sprintf("duration(self.interval) >= duration('%s')", fathomv1alpha1.MinCheckInterval),
		fmt.Sprintf("duration(self.timeout) >= duration('%s')", fathomv1alpha1.MinCheckTimeout),
	}
	crds := []string{
		"fathom.skaphos.io_addonchecks.yaml",
		"fathom.skaphos.io_nodecertificatechecks.yaml",
	}
	for _, name := range crds {
		path := filepath.Join("..", "..", "config", "crd", "bases", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read generated CRD %s: %v", path, err)
		}
		var crd apiextensionsv1.CustomResourceDefinition
		if err := yaml.Unmarshal(raw, &crd); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		specRules := specValidationRules(t, &crd)
		for _, want := range wantRules {
			found := false
			for _, rule := range specRules {
				if strings.Contains(rule, want) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: no spec-level validation rule contains %q; "+
					"either the CEL marker drifted from the api/v1alpha1 constants "+
					"or `task manifests` was not re-run (rules: %q)", name, want, specRules)
			}
		}
	}
}

// TestGeneratedCRDsDeclareTheFathomCategory pins FR-027: one listing command
// must surface every health-intent kind. A category on some kinds and not
// others is worse than none at all — `kubectl get fathom` would return a
// partial answer that looks complete.
//
// HealthReport is excluded on purpose. Reports are evidence, retained
// HistoryLimit-deep per check, so including them would bury the checks the
// grouping exists to surface. The exclusion is asserted too, so flipping it
// has to be a deliberate edit rather than a side effect of adding a marker.
func TestGeneratedCRDsDeclareTheFathomCategory(t *testing.T) {
	const category = "fathom"
	tests := map[string]bool{
		"fathom.skaphos.io_addonchecks.yaml":           true,
		"fathom.skaphos.io_healthchecks.yaml":          true,
		"fathom.skaphos.io_clusterhealths.yaml":        true,
		"fathom.skaphos.io_nodecertificatechecks.yaml": true,
		"fathom.skaphos.io_dnschecks.yaml":             true,
		"fathom.skaphos.io_healthreports.yaml":         false,
	}
	for name, want := range tests {
		path := filepath.Join("..", "..", "config", "crd", "bases", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read generated CRD %s: %v", path, err)
		}
		var crd apiextensionsv1.CustomResourceDefinition
		if err := yaml.Unmarshal(raw, &crd); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		got := slices.Contains(crd.Spec.Names.Categories, category)
		switch {
		case want && !got:
			t.Errorf("%s: missing the %q category (categories: %v); "+
				"either the marker was dropped or `task manifests` was not re-run. "+
				"A partial category makes `kubectl get %s` a misleading answer",
				name, category, crd.Spec.Names.Categories, category)
		case !want && got:
			t.Errorf("%s: unexpectedly carries the %q category. Reports are "+
				"evidence, not health intent; including them buries the checks "+
				"`kubectl get %s` exists to surface", name, category, category)
		}
	}
}

// TestEveryGeneratedCRDIsCategorised fails when a new kind is added without a
// deliberate choice about the category. The map above is the decision record;
// this asserts nothing escapes it.
func TestEveryGeneratedCRDIsCategorised(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "config", "crd", "bases"))
	if err != nil {
		t.Fatalf("read CRD bases: %v", err)
	}
	known := map[string]bool{
		"fathom.skaphos.io_addonchecks.yaml":           true,
		"fathom.skaphos.io_healthchecks.yaml":          true,
		"fathom.skaphos.io_clusterhealths.yaml":        true,
		"fathom.skaphos.io_nodecertificatechecks.yaml": true,
		"fathom.skaphos.io_dnschecks.yaml":             true,
		"fathom.skaphos.io_healthreports.yaml":         true,
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		if !known[entry.Name()] {
			t.Errorf("%s is a new CRD with no recorded category decision; add it to "+
				"TestGeneratedCRDsDeclareTheFathomCategory with an explicit true/false",
				entry.Name())
		}
	}
}

// specValidationRules returns the x-kubernetes-validations rule expressions
// attached to .spec of the CRD's single served version.
func specValidationRules(t *testing.T, crd *apiextensionsv1.CustomResourceDefinition) []string {
	t.Helper()
	if len(crd.Spec.Versions) != 1 {
		t.Fatalf("%s: expected exactly one version, got %d", crd.Name, len(crd.Spec.Versions))
	}
	schema := crd.Spec.Versions[0].Schema.OpenAPIV3Schema
	spec, ok := schema.Properties["spec"]
	if !ok {
		t.Fatalf("%s: schema has no .spec property", crd.Name)
	}
	rules := make([]string, 0, len(spec.XValidations))
	for _, v := range spec.XValidations {
		rules = append(rules, v.Rule)
	}
	return rules
}
