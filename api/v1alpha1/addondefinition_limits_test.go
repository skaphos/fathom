/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	limits "github.com/skaphos/fathom/pkg/addondefinition"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func runtimeSchema(t *testing.T, kind string) apiextensionsv1.JSONSchemaProps {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "crd", "bases", "fathom.skaphos.io_"+kind+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatal(err)
	}
	return *crd.Spec.Versions[0].Schema.OpenAPIV3Schema
}

// The API package deliberately cannot import authoring constants. Pin generated
// literal bounds against them here, in an external test package.
func TestDefinitionSchemaLimitParity(t *testing.T) {
	def := runtimeSchema(t, "addondefinitions")
	binding := runtimeSchema(t, "addondefinitionbindings")
	spec := def.Properties["spec"]
	fam := spec.Properties["families"]
	checks := fam.Items.Schema.Properties["checks"]
	check := checks.Items.Schema
	field := check.Properties["field"]
	workload := check.Properties["workload"]
	target := workload.Properties["target"]
	status := binding.Properties["status"]
	bspec := binding.Properties["spec"]
	for _, tc := range []struct {
		name   string
		actual *int64
		want   int64
	}{
		{"families", fam.MaxItems, limits.MaxFamilies},
		{"checks per family", checks.MaxItems, limits.MaxChecksPerFamily},
		{"addon identity", spec.Properties["addonType"].MaxLength, limits.MaxIdentifierBytes},
		{"adapter version", spec.Properties["adapterVersion"].MaxLength, limits.MaxAdapterVersionBytes},
		{"range", spec.Properties["supportedVersions"].MaxLength, limits.MaxVersionRangeBytes},
		{"namespace count", target.Properties["namespaces"].MaxItems, limits.MaxNamespaces},
		{"namespace length", target.Properties["namespaces"].Items.Schema.MaxLength, limits.MaxIdentifierBytes},
		{"field segments", field.Properties["fieldPath"].MaxItems, limits.MaxFieldSegments},
		{"field segment length", field.Properties["fieldPath"].Items.Schema.MaxLength, limits.MaxFieldSegmentBytes},
		{"field outcome map", field.Properties["valueOutcomes"].MaxProperties, limits.MaxMapEntries},
		{"selector entries", check.Properties["podProjection"].Properties["selector"].MaxProperties, limits.MaxSelectorTerms},
		{"selector value", check.Properties["podProjection"].Properties["selector"].AdditionalProperties.Schema.MaxLength, limits.MaxSelectorValueBytes},
		{"UID", bspec.Properties["definitionRef"].Properties["uid"].MaxLength, limits.MaxUIDBytes},
		{"condition count", status.Properties["conditions"].MaxItems, limits.MaxConditions},
		{"condition message", status.Properties["conditions"].Items.Schema.Properties["message"].MaxLength, limits.MaxMessageBytes},
		{"leader identity", status.Properties["leaderIdentity"].MaxLength, limits.MaxLeaderIdentityBytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.actual == nil || *tc.actual != tc.want {
				t.Fatalf("schema limit=%v want %d", tc.actual, tc.want)
			}
		})
	}
}

func TestDefinitionMaximumCheckAdmissionCost(t *testing.T) {
	requireAPIServer(t)
	obj := runtimeDefinition("runtime-maximum-checks")
	spec := obj.Object["spec"].(map[string]any)
	prototype := firstRuntimeCheck(spec)
	families := make([]any, 16)
	for f := range families {
		checks := make([]any, 32)
		for c := range checks {
			check := (&unstructured.Unstructured{Object: prototype}).DeepCopy().Object
			check["name"] = fmt.Sprintf("check-%d", c)
			checks[c] = check
		}
		families[f] = map[string]any{"name": fmt.Sprintf("family-%d", f), "checks": checks}
	}
	spec["families"] = families
	if err := k8sClient.Create(context.Background(), obj); err != nil {
		t.Fatalf("512-check admission cost fixture: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })

	over := runtimeDefinition("runtime-over-maximum")
	overSpec := over.Object["spec"].(map[string]any)
	overSpec["families"] = append(families, map[string]any{"name": "one-too-many", "checks": families[0].(map[string]any)["checks"]})
	if err := k8sClient.Create(context.Background(), over); err == nil {
		_ = k8sClient.Delete(context.Background(), over)
		t.Fatal("17 families admitted")
	} else if !strings.Contains(err.Error(), "spec.families") {
		t.Fatalf("rejected for an unrelated reason: %v", err)
	}
}
