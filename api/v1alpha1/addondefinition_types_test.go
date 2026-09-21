/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1_test

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Use unstructured wire objects so unknown fields and missing defaults exercise
// the installed schema, not Go zero values or the fake client's permissiveness.
func runtimeDefinition(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "fathom.skaphos.io/v1alpha1", "kind": "AddonDefinition",
		"metadata": map[string]any{"name": name},
		"spec": map[string]any{
			"addonType": name, "adapterVersion": "1.0.0", "semanticsVersion": int64(1),
			"families": []any{map[string]any{"name": "health", "checks": []any{map[string]any{
				"name": "controller", "kind": "Workload", "workload": map[string]any{
					"target": map[string]any{"scope": "Namespaced", "namespaces": []any{"default"}},
					"kind":   "Deployment", "defaultName": "controller",
				},
			}}}},
		},
	}}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "fathom.skaphos.io", Version: "v1alpha1", Kind: "AddonDefinition"})
	return obj
}

func TestAddonDefinitionAdmission(t *testing.T) {
	requireAPIServer(t)
	cases := []struct {
		name   string
		mutate func(map[string]any)
		valid  bool
	}{
		{"valid", func(map[string]any) {}, true},
		{"unsupported semantics", func(s map[string]any) { s["semanticsVersion"] = int64(2) }, false},
		{"identity mismatch", func(s map[string]any) { s["addonType"] = "other" }, false},
		{"no families", func(s map[string]any) { s["families"] = []any{} }, false},
		{"unknown kind", func(s map[string]any) { firstRuntimeCheck(s)["kind"] = "Expression" }, false},
		{"missing payload", func(s map[string]any) { delete(firstRuntimeCheck(s), "workload") }, false},
		{"mismatched kind", func(s map[string]any) { firstRuntimeCheck(s)["kind"] = "Field" }, false},
		{"duplicate checks", func(s map[string]any) {
			f := s["families"].([]any)[0].(map[string]any)
			f["checks"] = []any{firstRuntimeCheck(s), firstRuntimeCheck(s)}
		}, false},
		{"duplicate families", func(s map[string]any) { f := s["families"].([]any)[0]; s["families"] = []any{f, f} }, false},
		{"wildcard namespace", func(s map[string]any) {
			firstRuntimeCheck(s)["workload"].(map[string]any)["target"] = map[string]any{"scope": "Namespaced", "namespaces": []any{"*"}}
		}, false},
		{"singleton multi namespace", func(s map[string]any) {
			firstRuntimeCheck(s)["workload"].(map[string]any)["target"] = map[string]any{"scope": "Namespaced", "namespaces": []any{"a", "b"}}
		}, false},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := runtimeDefinition(fmt.Sprintf("runtime-admission-%d", i))
			tc.mutate(obj.Object["spec"].(map[string]any))
			err := k8sClient.Create(context.Background(), obj)
			if (err == nil) != tc.valid {
				t.Fatalf("accepted=%v want %v: %v", err == nil, tc.valid, err)
			}
			if err == nil {
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			}
		})
	}
}

func firstRuntimeCheck(s map[string]any) map[string]any {
	return s["families"].([]any)[0].(map[string]any)["checks"].([]any)[0].(map[string]any)
}

func TestAddonDefinitionUnknownFieldHandling(t *testing.T) {
	requireAPIServer(t)
	obj := runtimeDefinition("runtime-strict-fields")
	firstRuntimeCheck(obj.Object["spec"].(map[string]any))["script"] = "return healthy"
	if err := k8sClient.Create(context.Background(), obj, &client.CreateOptions{FieldValidation: "Strict"}); err == nil {
		_ = k8sClient.Delete(context.Background(), obj)
		t.Fatal("strict unknown field accepted")
	}
	obj = runtimeDefinition("runtime-pruned-fields")
	firstRuntimeCheck(obj.Object["spec"].(map[string]any))["script"] = "return healthy"
	if err := k8sClient.Create(context.Background(), obj, &client.CreateOptions{FieldValidation: "Ignore"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
	if _, exists := firstRuntimeCheck(obj.Object["spec"].(map[string]any))["script"]; exists {
		t.Fatal("opaque executable field survived pruning")
	}
}
