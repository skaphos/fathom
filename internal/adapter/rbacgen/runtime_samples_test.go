/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package rbacgen_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"sigs.k8s.io/yaml"

	"github.com/skaphos/fathom/internal/adapter/rbacgen"
	"github.com/skaphos/fathom/internal/app"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
)

func TestRuntimeInventoryMatchesOperator(t *testing.T) {
	var want []string
	for _, entry := range rbacgen.Collect(app.BuiltInAdapters()) {
		want = append(want, entry.Addon)
	}
	if !reflect.DeepEqual(want, definitions.BuiltinNames()) {
		t.Fatal("runtime inventory is stale; run task gen:runtime-definitions")
	}
	copy := definitions.BuiltinNames()
	copy[0] = "mutated"
	if definitions.BuiltinNames()[0] == "mutated" {
		t.Fatal("caller changed inventory")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg/addondefinition"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := rbacgen.WriteRuntimeInventory(root, app.BuiltInAdapters()); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(filepath.Join(root, "pkg/addondefinition/inventory_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("../../../pkg/addondefinition/inventory_generated.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(generated) != string(committed) {
		t.Fatal("generated source drift")
	}
}

func TestRuntimeSamplesMatchGenerated(t *testing.T) {
	root := t.TempDir()
	if err := rbacgen.WriteRuntimeSamples(root); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "config/samples/addondefinition")
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 9 {
		t.Fatalf("expected nine examples, got %d", len(files))
	}
	kinds := map[string]bool{}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		committed, err := os.ReadFile(filepath.Join("../../../config/samples/addondefinition", file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, committed) {
			t.Fatalf("sample drift: %s", file.Name())
		}
		var d api.AddonDefinition
		if err := yaml.UnmarshalStrict(data, &d); err != nil {
			t.Fatal(err)
		}
		if err := definitions.Validate(&d); err != nil {
			t.Fatal(err)
		}
		for _, name := range definitions.BuiltinNames() {
			if name == d.Name {
				t.Fatal("sample collides with built-in")
			}
		}
		kinds[d.Spec.Families[0].Checks[0].Kind] = true
	}
	if len(kinds) != 9 {
		t.Fatal("sample kinds are not distinct")
	}
}
