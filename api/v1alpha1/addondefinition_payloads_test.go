/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1_test

import (
	"context"
	"fmt"
	"testing"
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
			actual := firstRuntimeCheck(obj.Object["spec"].(map[string]any))[tc.field].(map[string]any)[tc.defaultField]
			if actual != tc.defaultValue {
				t.Fatalf("default %s=%v want=%v", tc.defaultField, actual, tc.defaultValue)
			}
		})
	}
}
