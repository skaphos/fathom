/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package testutil_test

import (
	"encoding/json"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/runtime/testutil"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"go.yaml.in/yaml/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Independent parsers establish that the hostile fixtures really reach their
// advertised boundary. A wrong fixture count can make a budget test pass without
// exercising the cap, especially when document/sequence roots are counted.
func TestTraversalFixturesReachBoundary(t *testing.T) {
	for _, n := range []int{1, limits.MaxObjectNodes, limits.MaxObjectNodes + 1} {
		var value any
		if err := json.Unmarshal([]byte(testutil.JSONNodes(n)), &value); err != nil {
			t.Fatal(err)
		}
		count := 1
		if array, ok := value.([]any); ok {
			count += len(array)
		}
		if count != n {
			t.Fatalf("JSON nodes=%d want=%d", count, n)
		}
	}
	for _, n := range []int{2, limits.MaxYAMLNodes, limits.MaxYAMLNodes + 1} {
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(testutil.YAMLNodes(n)), &doc); err != nil {
			t.Fatal(err)
		}
		count := 1 + len(doc.Content) + len(doc.Content[0].Content)
		if count != n {
			t.Fatalf("YAML nodes=%d want=%d", count, n)
		}
	}
	for _, depth := range []int{limits.MaxSpecDepth, limits.MaxSpecDepth + 1} {
		var value any
		if err := json.Unmarshal([]byte(testutil.JSONDepth(depth)), &value); err != nil {
			t.Fatal(err)
		}
		got := 0
		for {
			array, ok := value.([]any)
			if !ok {
				break
			}
			value = array[0]
			got++
		}
		if got != depth {
			t.Fatalf("JSON depth=%d want=%d", got, depth)
		}
	}
}

func TestNamespaceAndUIDFixturesExerciseBindingLimits(t *testing.T) {
	for _, n := range []int{limits.MaxNamespaces, limits.MaxNamespaces + 1} {
		b := &api.AddonDefinitionBinding{ObjectMeta: metav1.ObjectMeta{Name: "custom", Namespace: "operator"}, Spec: api.AddonDefinitionBindingSpec{
			DefinitionRef: api.DefinitionReference{Name: "custom", UID: "definition"}, ServiceAccountRef: api.DefinitionObjectReference{Name: "dedicated", UID: "account"},
		}}
		for _, s := range testutil.Strings(n) {
			b.Spec.TargetScope.Namespaces = append(b.Spec.TargetScope.Namespaces, api.DefinitionDNSLabel(s))
		}
		if err := limits.ValidateBinding(b); (err == nil) != (n == limits.MaxNamespaces) {
			t.Fatalf("namespace count %d: %v", n, err)
		}
		b.Spec.TargetScope.Namespaces = []api.DefinitionDNSLabel{"target"}
		b.Spec.ServiceAccountRef.UID = testutil.Bytes(limits.MaxUIDBytes)
		if err := limits.ValidateBinding(b); err != nil {
			t.Fatal(err)
		}
		b.Spec.ServiceAccountRef.UID = testutil.Bytes(limits.MaxUIDBytes + 1)
		if err := limits.ValidateBinding(b); err == nil {
			t.Fatal("oversize UID accepted")
		}
	}
}
