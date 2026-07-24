/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1_test

import (
	"fmt"
	"os"
	"path/filepath"
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
