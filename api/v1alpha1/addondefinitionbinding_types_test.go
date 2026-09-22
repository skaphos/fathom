/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func runtimeBinding(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "fathom.skaphos.io/v1alpha1", "kind": "AddonDefinitionBinding",
		"metadata": map[string]any{"name": name, "namespace": "default"},
		"spec": map[string]any{
			"definitionRef":     map[string]any{"name": name, "uid": "definition-uid"},
			"serviceAccountRef": map[string]any{"name": "dedicated", "uid": "sa-uid"},
			"targetScope":       map[string]any{"namespaces": []any{"default"}},
		},
	}}
}

func TestAddonDefinitionBindingAdmission(t *testing.T) {
	requireAPIServer(t)
	for i, tc := range []struct {
		name   string
		mutate func(map[string]any)
		valid  bool
	}{
		{"disabled by default", func(map[string]any) {}, true},
		{"empty UID", func(s map[string]any) { s["definitionRef"].(map[string]any)["uid"] = "" }, false},
		{"oversize UID", func(s map[string]any) { s["serviceAccountRef"].(map[string]any)["uid"] = strings.Repeat("x", 129) }, false},
		{"identity mismatch", func(s map[string]any) { s["definitionRef"].(map[string]any)["name"] = "another" }, false},
		{"no scope", func(s map[string]any) { s["targetScope"] = map[string]any{} }, false},
		{"wildcard scope", func(s map[string]any) { s["targetScope"] = map[string]any{"namespaces": []any{"*"}} }, false},
		{"cluster only", func(s map[string]any) { s["targetScope"] = map[string]any{"allowClusterScoped": true} }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := runtimeBinding(fmt.Sprintf("runtime-binding-%d", i))
			tc.mutate(obj.Object["spec"].(map[string]any))
			err := k8sClient.Create(context.Background(), obj)
			if (err == nil) != tc.valid {
				t.Fatalf("accepted=%v want %v: %v", err == nil, tc.valid, err)
			}
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			enabled, found, err := unstructured.NestedBool(obj.Object, "spec", "enabled")
			if err != nil || !found || enabled {
				t.Fatalf("expected disabled default: found=%v enabled=%v err=%v", found, enabled, err)
			}
			original := obj.DeepCopy()
			if err := unstructured.SetNestedField(obj.Object, "recreated", "spec", "serviceAccountRef", "uid"); err != nil {
				t.Fatal(err)
			}
			if err := k8sClient.Update(context.Background(), obj); err == nil {
				t.Fatal("mutable SA UID accepted")
			}
			obj = original.DeepCopy()
			if err := unstructured.SetNestedField(obj.Object, "recreated", "spec", "definitionRef", "uid"); err != nil {
				t.Fatal(err)
			}
			if err := k8sClient.Update(context.Background(), obj); err == nil {
				t.Fatal("mutable definition UID accepted")
			}
		})
	}
}

func TestAddonDefinitionBindingStatusBounds(t *testing.T) {
	requireAPIServer(t)
	obj := runtimeBinding("runtime-status-bounds")
	if err := k8sClient.Create(context.Background(), obj); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
	condition := map[string]any{"type": "Ready", "status": "False", "reason": "AuthorizationRevoked", "message": "disabled", "observedGeneration": obj.GetGeneration(), "lastTransitionTime": "2026-09-20T00:00:00Z"}
	obj.Object["status"] = map[string]any{"observedGeneration": obj.GetGeneration(), "activeRuns": int64(0), "leaderIdentity": "leader", "leaderEpoch": map[string]any{"leaseUID": "lease", "holderIdentity": "leader", "acquireTime": "2026-09-20T00:00:00Z", "leaseTransitions": int64(0)}, "conditions": []any{condition}}
	if err := k8sClient.Status().Update(context.Background(), obj); err != nil {
		t.Fatal(err)
	}
	valid := obj.DeepCopy()
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"negative active runs", func(s map[string]any) { s["activeRuns"] = int64(-1) }},
		{"excess active runs", func(s map[string]any) { s["activeRuns"] = int64(5) }},
		{"negative generation", func(s map[string]any) { s["observedGeneration"] = int64(-1) }},
		{"empty lease UID", func(s map[string]any) { s["leaderEpoch"].(map[string]any)["leaseUID"] = "" }},
		{"negative transitions", func(s map[string]any) { s["leaderEpoch"].(map[string]any)["leaseTransitions"] = int64(-1) }},
		{"oversize leader", func(s map[string]any) { s["leaderIdentity"] = strings.Repeat("x", 254) }},
		{"oversize message", func(s map[string]any) {
			s["conditions"].([]any)[0].(map[string]any)["message"] = strings.Repeat("x", 1025)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := valid.DeepCopy()
			tc.mutate(invalid.Object["status"].(map[string]any))
			if err := k8sClient.Status().Update(context.Background(), invalid); err == nil {
				t.Fatal("invalid status accepted")
			}
		})
	}
}

func TestBindingMaximumContractAndStatusAdmission(t *testing.T) {
	requireAPIServer(t)
	obj := runtimeBinding(strings.Repeat("b", 63))
	spec := obj.Object["spec"].(map[string]any)
	spec["definitionRef"].(map[string]any)["uid"] = strings.Repeat("d", 128)
	spec["serviceAccountRef"] = map[string]any{"name": strings.Repeat("s", 253), "uid": strings.Repeat("u", 128)}
	ns := make([]any, 32)
	for i := range ns {
		ns[i] = fmt.Sprintf("ns-%02d-%s", i, strings.Repeat("n", 57))
	}
	spec["targetScope"] = map[string]any{"namespaces": ns, "allowClusterScoped": true}
	if err := k8sClient.Create(context.Background(), obj); err != nil {
		t.Fatalf("maximum binding admission/CEL cost: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
	conditions := make([]any, 8)
	for i := range conditions {
		conditions[i] = map[string]any{"type": fmt.Sprintf("%s%d", strings.Repeat("C", 63), i), "status": "Unknown", "reason": strings.Repeat("R", 128), "message": strings.Repeat("m", 1024), "observedGeneration": obj.GetGeneration(), "lastTransitionTime": "2026-09-20T00:00:00Z"}
	}
	obj.Object["status"] = map[string]any{"observedGeneration": obj.GetGeneration(), "activeRuns": int64(4), "leaderIdentity": strings.Repeat("l", 253), "leaderEpoch": map[string]any{"leaseUID": strings.Repeat("u", 128), "holderIdentity": strings.Repeat("h", 253), "acquireTime": "2026-09-20T00:00:00.123456Z", "leaseTransitions": int64(2147483647)}, "conditions": conditions}
	if err := k8sClient.Status().Update(context.Background(), obj); err != nil {
		t.Fatalf("maximum status admission/CEL cost: %v", err)
	}
	valid := obj.DeepCopy()
	for _, tc := range []struct {
		name  string
		path  []string
		value any
	}{
		{"lease UID", []string{"leaderEpoch", "leaseUID"}, strings.Repeat("u", 129)},
		{"holder", []string{"leaderEpoch", "holderIdentity"}, strings.Repeat("h", 254)},
		{"condition count", []string{"conditions"}, append(conditions, map[string]any{"type": "extra", "status": "True", "reason": "Extra", "message": "extra", "observedGeneration": int64(1), "lastTransitionTime": "2026-09-20T00:00:00Z"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := valid.DeepCopy()
			if err := unstructured.SetNestedField(invalid.Object, tc.value, append([]string{"status"}, tc.path...)...); err != nil {
				t.Fatal(err)
			}
			if err := k8sClient.Status().Update(context.Background(), invalid); err == nil {
				t.Fatal("over-limit status accepted")
			}
		})
	}
	for _, field := range []string{"type", "reason", "message"} {
		t.Run(field, func(t *testing.T) {
			invalid := valid.DeepCopy()
			condition := invalid.Object["status"].(map[string]any)["conditions"].([]any)[0].(map[string]any)
			condition[field] = condition[field].(string) + "x"
			if err := k8sClient.Status().Update(context.Background(), invalid); err == nil {
				t.Fatal("over-limit condition accepted")
			}
		})
	}
	// With the same UID, retargeting the service account name is still forbidden.
	invalid := valid.DeepCopy()
	if err := unstructured.SetNestedField(invalid.Object, "another", "spec", "serviceAccountRef", "name"); err != nil {
		t.Fatal(err)
	}
	if err := k8sClient.Update(context.Background(), invalid); err == nil {
		t.Fatal("same-UID service account retarget accepted")
	}
}

func TestBindingTypedSpecCannotReachSixteenKiB(t *testing.T) {
	// NUL escapes as six JSON bytes per UID byte, so this is larger than a
	// binding made with real Kubernetes UIDs. Every bounded name and namespace
	// is also at its maximum legal length and count.
	maxName := strings.Repeat("s", 63) + "." + strings.Repeat("s", 63) + "." +
		strings.Repeat("s", 63) + "." + strings.Repeat("s", 61)
	namespaces := make([]api.DefinitionDNSLabel, limits.MaxNamespaces)
	for i := range namespaces {
		namespaces[i] = api.DefinitionDNSLabel(fmt.Sprintf("n%02d%s", i, strings.Repeat("n", 60)))
	}
	name := strings.Repeat("d", limits.MaxIdentifierBytes)
	binding := &api.AddonDefinitionBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}, Spec: api.AddonDefinitionBindingSpec{
		DefinitionRef:     api.DefinitionReference{Name: api.DefinitionDNSLabel(name), UID: strings.Repeat("\x00", limits.MaxUIDBytes)},
		ServiceAccountRef: api.DefinitionObjectReference{Name: api.DefinitionResourceName(maxName), UID: strings.Repeat("\x00", limits.MaxUIDBytes)},
		Enabled:           true,
		TargetScope:       api.DefinitionBindingScope{Namespaces: namespaces, AllowClusterScoped: true},
	}}
	if err := limits.ValidateBinding(binding); err != nil {
		t.Fatalf("maximum typed binding fixture is invalid: %v", err)
	}
	raw, err := json.Marshal(binding.Spec)
	if err != nil {
		t.Fatal(err)
	}
	withoutFlags := binding.DeepCopy()
	withoutFlags.Spec.Enabled = false
	withoutFlags.Spec.TargetScope.AllowClusterScoped = false
	shorter, err := json.Marshal(withoutFlags.Spec)
	if err != nil {
		t.Fatal(err)
	}
	// Both bools have omitempty: false disappears, so true (used above) is
	// the maximum serialized form even though the word "false" is longer.
	if len(shorter) >= len(raw) || !strings.Contains(string(raw), `"enabled":true`) ||
		!strings.Contains(string(raw), `"allowClusterScoped":true`) ||
		strings.Contains(string(shorter), `"enabled"`) || strings.Contains(string(shorter), `"allowClusterScoped"`) {
		t.Fatalf("maximum bool encoding is not the true/true fixture: true=%d false=%d", len(raw), len(shorter))
	}
	if len(raw) >= 4200 || len(raw) >= limits.MaxBindingBytes {
		t.Fatalf("maximum typed spec is %d bytes; expected under 4200 and far below 16 KiB", len(raw))
	}
	t.Logf("maximum typed binding spec with escaped UIDs: %d bytes of %d-byte limit", len(raw), limits.MaxBindingBytes)
}
