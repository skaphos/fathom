/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func TestDefinitionCanonicalByteCapAfterAdmissionDefaults(t *testing.T) {
	requireAPIServer(t)
	const name = "runtime-canonical-byte-cap"
	spec := api.AddonDefinitionSpec{AddonType: name, AdapterVersion: "1.0.0", SemanticsVersion: 1}
	for f := 0; f < 8; f++ {
		family := api.DefinitionFamily{Name: api.DefinitionIdentifier(fmt.Sprintf("family-%d", f))}
		for c := 0; c < 32; c++ {
			family.Checks = append(family.Checks, api.DefinitionCheck{
				Name: api.DefinitionIdentifier(fmt.Sprintf("check-%d", c)), Kind: "Field",
				Field: &api.DefinitionField{
					Target:     api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{"default"}},
					APIVersion: "example.org/v1", Kind: "Widget", ListKind: "WidgetList",
					FieldPath: []string{"status"}, ExpectedValue: "a", AbsentOutcome: "Warn", OtherOutcome: "Warn",
				},
			})
		}
		spec.Families = append(spec.Families, family)
	}
	defaultedWire := func() (map[string]any, error) {
		wire, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
		if err != nil {
			return nil, err
		}
		// The CRD adds these defaults. Include their serialized cost before
		// filling strings, so admission cannot move the stored wire spec past
		// the 256 KiB boundary even when typed omitempty later drops false.
		wire["optional"] = false
		for _, family := range wire["families"].([]any) {
			family.(map[string]any)["defaultEnabled"] = false
		}
		return wire, nil
	}
	wireSpec, err := defaultedWire()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(wireSpec)
	if err != nil {
		t.Fatal(err)
	}
	remaining := limits.MaxSpecBytes - len(raw)
	for fi := range spec.Families {
		for ci := range spec.Families[fi].Checks {
			field := spec.Families[fi].Checks[ci].Field
			extra := min(remaining, limits.MaxStringBytes-1)
			field.ExpectedValue = api.DefinitionText(strings.Repeat("a", extra+1))
			remaining -= extra
		}
	}
	if remaining != 0 {
		t.Fatalf("typed fixture cannot reach the %d-byte cap: %d bytes remain", limits.MaxSpecBytes, remaining)
	}
	wireSpec, err = defaultedWire()
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(wireSpec)
	if err != nil || len(raw) != limits.MaxSpecBytes {
		t.Fatalf("pre-admission defaulted wire spec bytes=%d want %d: %v", len(raw), limits.MaxSpecBytes, err)
	}
	obj := runtimeDefinition(name)
	obj.Object["spec"] = wireSpec
	if err := k8sClient.Create(context.Background(), obj); err != nil {
		t.Fatalf("CREATE exact-byte definition: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
	stored := runtimeDefinition(name)
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: name}, stored); err != nil {
		t.Fatal(err)
	}
	storedJSON, err := json.Marshal(stored.Object["spec"])
	if err != nil || len(storedJSON) != limits.MaxSpecBytes {
		t.Fatalf("stored defaulted wire spec bytes=%d want %d: %v", len(storedJSON), limits.MaxSpecBytes, err)
	}
	var admitted api.AddonDefinitionSpec
	if err := json.Unmarshal(storedJSON, &admitted); err != nil {
		t.Fatal(err)
	}
	defaultedJSON, err := json.Marshal(admitted)
	if err != nil || len(defaultedJSON) > limits.MaxSpecBytes {
		t.Fatalf("post-default typed canonical spec bytes=%d exceeds %d: %v", len(defaultedJSON), limits.MaxSpecBytes, err)
	}
	if err := limits.Validate(&api.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: admitted}); err != nil {
		t.Fatalf("CREATE returned a definition the compiler rejects at the exact byte cap: %v", err)
	}
}
