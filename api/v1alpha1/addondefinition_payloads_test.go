/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAddonDefinitionAllPayloadAdmission(t *testing.T) {
	requireAPIServer(t)
	for i, tc := range []struct {
		kind, field, scope, defaultField string
		defaultValue                     any
		payload                          map[string]any
	}{
		{"Workload", "workload", "Namespaced", "checkPods", false, map[string]any{"kind": "Deployment", "defaultName": "controller"}},
		{"CRD", "crd", "Cluster", "unsupportedVersionOutcome", "Warn", map[string]any{"names": []any{"widgets.example.org"}, "supportedVersions": []any{"v1"}}},
		{"Condition", "condition", "Cluster", "mismatch", "Fail", map[string]any{"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService", "names": []any{"v1.metrics.k8s.io"}, "conditionType": "Available", "expectedStatus": "True"}},
		{"Field", "field", "Namespaced", "absentOutcome", "Warn", map[string]any{"apiVersion": "example.org/v1", "kind": "Widget", "listKind": "WidgetList", "fieldPath": []any{"status", "phase"}, "expectedValue": "Ready"}},
		{"Webhook", "webhook", "Cluster", "verifyEndpoints", false, map[string]any{"kind": "ValidatingWebhookConfiguration", "name": "validation"}},
		{"CronJob", "cronJob", "Namespaced", "defaultSuccessMaxAge", "0s", map[string]any{"defaultName": "job"}},
		{"ConfigMap", "configMap", "Namespaced", "invalidOutcome", "Fail", map[string]any{"defaultName": "config", "key": "policy.yaml"}},
		{"AnnotationStaleness", "annotationStaleness", "Namespaced", "staleOutcome", "Warn", map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "defaultName": "config", "annotationKey": "example.org/observed", "defaultMaxAge": "1m"}},
		{"PodProjection", "podProjection", "Namespaced", "missingOutcome", "Fail", map[string]any{"selector": map[string]any{"app": "example"}, "volumeName": "token"}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			obj := runtimeDefinition(fmt.Sprintf("runtime-payload-%d", i))
			check := firstRuntimeCheck(obj.Object["spec"].(map[string]any))
			delete(check, "workload")
			check["kind"] = tc.kind
			target := map[string]any{"scope": tc.scope}
			if tc.scope == "Namespaced" {
				target["namespaces"] = []any{"default"}
			}
			tc.payload["target"] = target
			check[tc.field] = tc.payload
			if err := k8sClient.Create(context.Background(), obj); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			defaults := map[string]map[string]any{
				"Workload":            {"checkPods": false, "defaultRestartWarn": int64(0)},
				"CRD":                 {"unsupportedVersionOutcome": "Warn"},
				"Condition":           {"absentCondition": "Fail", "mismatch": "Fail"},
				"Field":               {"absentOutcome": "Warn", "otherOutcome": "Warn"},
				"Webhook":             {"verifyEndpoints": false},
				"CronJob":             {"defaultSuccessMaxAge": "0s", "staleOutcome": "Warn"},
				"ConfigMap":           {"unrecognizedOutcome": "Warn", "invalidOutcome": "Fail"},
				"AnnotationStaleness": {"staleOutcome": "Warn"},
				"PodProjection":       {"missingOutcome": "Fail"},
			}
			for key, want := range defaults[tc.kind] {
				got := firstRuntimeCheck(obj.Object["spec"].(map[string]any))[tc.field].(map[string]any)[key]
				if got != want {
					t.Errorf("default %s=%v want=%v", key, got, want)
				}
			}
			actual := firstRuntimeCheck(obj.Object["spec"].(map[string]any))[tc.field].(map[string]any)[tc.defaultField]
			if actual != tc.defaultValue {
				t.Fatalf("default %s=%v want=%v", tc.defaultField, actual, tc.defaultValue)
			}
		})
	}
}

// Exercise every required field, enum, and declared structural limit through
// the API server using valid generated examples, not Go zero-value objects.
func TestPayloadStructuralContractRejection(t *testing.T) {
	requireAPIServer(t)
	schema := runtimeSchema(t, "addondefinitions").Properties["spec"].Properties["families"].Items.Schema.Properties["checks"].Items.Schema
	fixtures := []struct {
		kind, field string
		payload     map[string]any
	}{
		{"Workload", "workload", map[string]any{"target": map[string]any{"scope": "Namespaced", "namespaces": []any{"default"}}, "kind": "Deployment", "defaultName": "controller"}},
		{"CRD", "crd", map[string]any{"target": map[string]any{"scope": "Cluster"}, "names": []any{"widgets.example.org"}, "supportedVersions": []any{"v1"}}},
		{"Condition", "condition", map[string]any{"target": map[string]any{"scope": "Cluster"}, "apiVersion": "example.org/v1", "kind": "Widget", "listKind": "WidgetList", "conditionType": "Ready", "expectedStatus": "True"}},
		{"Field", "field", map[string]any{"target": map[string]any{"scope": "Cluster"}, "apiVersion": "example.org/v1", "kind": "Widget", "listKind": "WidgetList", "fieldPath": []any{"status"}, "expectedValue": "Ready"}},
		{"Webhook", "webhook", map[string]any{"target": map[string]any{"scope": "Cluster"}, "kind": "ValidatingWebhookConfiguration", "name": "hook"}},
		{"CronJob", "cronJob", map[string]any{"target": map[string]any{"scope": "Namespaced", "namespaces": []any{"default"}}, "defaultName": "job"}},
		{"ConfigMap", "configMap", map[string]any{"target": map[string]any{"scope": "Namespaced", "namespaces": []any{"default"}}, "defaultName": "config", "key": "config.yaml"}},
		{"AnnotationStaleness", "annotationStaleness", map[string]any{"target": map[string]any{"scope": "Cluster"}, "apiVersion": "v1", "kind": "Node", "defaultName": "node", "annotationKey": "example.org/last", "defaultMaxAge": "1m"}},
		{"PodProjection", "podProjection", map[string]any{"target": map[string]any{"scope": "Namespaced", "namespaces": []any{"default"}}, "selector": map[string]any{"app": "custom"}, "volumeName": "token"}},
	}
	serial := 0
	for _, fixture := range fixtures {
		payloadSchema := schema.Properties[fixture.field]
		reject := func(label string, mutate func(map[string]any)) {
			t.Run(fixture.kind+"/"+label, func(t *testing.T) {
				serial++
				obj := runtimeDefinition(fmt.Sprintf("runtime-field-%d", serial))
				check := firstRuntimeCheck(obj.Object["spec"].(map[string]any))
				delete(check, "workload")
				check["kind"] = fixture.kind
				payload := (&unstructured.Unstructured{Object: fixture.payload}).DeepCopy().Object
				mutate(payload)
				check[fixture.field] = payload
				if err := k8sClient.Create(context.Background(), obj); err == nil {
					_ = k8sClient.Delete(context.Background(), obj)
					t.Fatal("invalid wire field admitted")
				} else {
					requiredReason := ""
					switch strings.Fields(label)[0] {
					case "length":
						requiredReason = "Too long"
					case "items", "map":
						requiredReason = "Too many"
					case "enum":
						requiredReason = "Unsupported value"
					case "missing":
						requiredReason = "Required value"
					}
					if requiredReason != "" && !strings.Contains(err.Error(), requiredReason) {
						t.Fatalf("rejected for an unrelated constraint: %v", err)
					}
				}
			})
		}
		for _, field := range payloadSchema.Required {
			reject("missing "+field, func(p map[string]any) { delete(p, field) })
		}
		for field, property := range payloadSchema.Properties {
			if len(property.Enum) > 0 {
				reject("enum "+field, func(p map[string]any) { p[field] = "NotAValidValue" })
			}
			if property.MaxLength != nil {
				reject("length "+field, func(p map[string]any) { p[field] = strings.Repeat("a", int(*property.MaxLength)+1) })
			}
			if property.Minimum != nil {
				reject("minimum "+field, func(p map[string]any) { p[field] = int64(*property.Minimum) - 1 })
			}
			if property.MaxItems != nil {
				reject("items "+field, func(p map[string]any) {
					items := make([]any, int(*property.MaxItems)+1)
					for i := range items {
						items[i] = fmt.Sprintf("v%d", i)
					}
					p[field] = items
				})
			}
			if property.MaxProperties != nil {
				reject("map "+field, func(p map[string]any) {
					values := map[string]any{}
					for i := 0; i <= int(*property.MaxProperties); i++ {
						values[fmt.Sprintf("key%d", i)] = "Warn"
					}
					p[field] = values
				})
			}
		}
	}
}
