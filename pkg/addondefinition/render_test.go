/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package addondefinition_test

import (
	"bytes"
	"io"
	"testing"

	definitions "github.com/skaphos/fathom/pkg/addondefinition"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

func TestRenderStagedIdentityAndGrants(t *testing.T) {
	opts := definitions.RenderOptions{ServiceAccount: "custom-reader", OperatorNamespace: "fathom", OperatorServiceAccount: "operator", BuiltinNames: []string{"cert-manager"}}
	d := validDefinition()
	d.UID = "must-not-copy"
	d.ResourceVersion = "42"
	data, _, err := definitions.Render(d, opts)
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := definitions.Render(d, opts)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatal("nondeterministic output")
	}
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var binding, impersonation bool
	for {
		var obj unstructured.Unstructured
		err := decoder.Decode(&obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if obj.GetUID() != "" || obj.GetResourceVersion() != "" {
			t.Fatal("live metadata leaked into install output")
		}
		switch obj.GetKind() {
		case "AddonDefinitionBinding":
			binding = true
			enabled, _, _ := unstructured.NestedBool(obj.Object, "spec", "enabled")
			uid, _, _ := unstructured.NestedString(obj.Object, "spec", "definitionRef", "uid")
			saUID, _, _ := unstructured.NestedString(obj.Object, "spec", "serviceAccountRef", "uid")
			if enabled || uid != "" || saUID != "" {
				t.Fatal("binding invented authority")
			}
		case "Role":
			rules, _, _ := unstructured.NestedSlice(obj.Object, "rules")
			for _, raw := range rules {
				rule := raw.(map[string]any)
				verbs := rule["verbs"].([]any)
				if verbs[0] == "impersonate" {
					impersonation = true
					if obj.GetNamespace() != "fathom" || rule["resources"].([]any)[0] != "serviceaccounts" || rule["resourceNames"].([]any)[0] != "custom-reader" {
						t.Fatal("impersonation grant broadened")
					}
				}
			}
		case "ClusterRole":
			t.Fatal("namespaced workload needs no cluster role")
		}
	}
	if !binding || !impersonation {
		t.Fatal("missing staged objects")
	}
}

func TestRenderRejectsIdentityMistakes(t *testing.T) {
	for _, opts := range []definitions.RenderOptions{
		{ServiceAccount: "reader", OperatorNamespace: "fathom"},
		{ServiceAccount: "operator", OperatorNamespace: "fathom", OperatorServiceAccount: "operator"},
		{ServiceAccount: "reader", OperatorNamespace: "*", OperatorServiceAccount: "operator"},
		{ServiceAccount: "reader", OperatorNamespace: "fathom", OperatorServiceAccount: "operator", BuiltinNames: []string{"custom-addon"}},
	} {
		if _, _, err := definitions.Render(validDefinition(), opts); err == nil {
			t.Fatalf("accepted invalid options: %+v", opts)
		}
	}
}
