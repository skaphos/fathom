/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// White-box access is needed to inspect the private lowering boundary separately
// from evaluator algorithms, whose public behavioral suites already exist.
package declarative

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/pkg/adapter"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRuntimePayloadWireMappings(t *testing.T) {
	for _, tc := range []struct {
		kind, field, payload, want string
		cluster                    bool
	}{
		{"Workload", "workload", `{"kind":"StatefulSet","defaultName":"controller","component":"app","absence":"Optional","checkPods":true,"defaultRestartWarn":17,"nameThresholdKey":"name","restartWarnThresholdKey":"restart"}`, `{"Kind":"StatefulSet","DefaultNamespace":"team-a","DefaultName":"controller","Component":"app","Absence":"Optional","CheckPods":true,"DefaultRestartWarn":17,"NameThresholdKey":"","RestartWarnThresholdKey":""}`, false},
		{"CRD", "crd", `{"names":["widgets.example.org"],"supportedVersions":["v2","v1"],"absence":"Optional","unsupportedVersionOutcome":"Fail"}`, `{"Names":["widgets.example.org"],"SupportedVersions":["v2","v1"],"Absence":"Optional","UnsupportedVersionOutcome":"Fail"}`, true},
		{"Condition", "condition", `{"apiVersion":"example.org/v1","kind":"Widget","listKind":"WidgetList","listName":"objects","versionCRD":"widgets.example.org","supportedVersions":["v2","v1"],"absence":"Optional","conditionType":"Available","expectedStatus":"False","absentCondition":"Skipped","mismatch":"Warn"}`, `{"APIVersion":"example.org/v1","Kind":"Widget","ListKind":"WidgetList","ListName":"objects","VersionCRD":"widgets.example.org","SupportedVersions":["v2","v1"],"Absence":"Optional","ConditionType":"Available","ExpectedStatus":"False","AbsentCondition":"Skipped","Mismatch":"Warn","ClusterScoped":false,"DefaultNamespace":"team-a"}`, false},
		{"Field", "field", `{"apiVersion":"example.org/v1","kind":"Widget","listKind":"WidgetList","listName":"objects","absence":"Required","fieldPath":["status","phase"],"expectedValue":"Ready","valueOutcomes":{"Pending":"Skipped"},"absentOutcome":"Fail","otherOutcome":"Pass"}`, `{"APIVersion":"example.org/v1","Kind":"Widget","ListKind":"WidgetList","ListName":"objects","Absence":"Required","FieldPath":["status","phase"],"ExpectedValue":"Ready","ValueOutcomes":{"Pending":"Skipped"},"AbsentOutcome":"Fail","OtherOutcome":"Pass","ClusterScoped":true}`, true},
		{"Webhook", "webhook", `{"kind":"MutatingWebhookConfiguration","name":"hook","nameThresholdKey":"name","expectedService":"hook","serviceNamespace":"helpers","absence":"Optional","verifyEndpoints":true}`, `{"Kind":"MutatingWebhookConfiguration","Name":"hook","NameThresholdKey":"","ExpectedService":"hook","ServiceNamespace":"helpers","Absence":"Optional","VerifyEndpoints":true}`, true},
		{"CronJob", "cronJob", `{"defaultName":"job","nameThresholdKey":"name","component":"jobs","absence":"Optional","successMaxAgeThresholdKey":"age","defaultSuccessMaxAge":"2m","staleOutcome":"Fail"}`, `{"DefaultNamespace":"team-a","DefaultName":"job","NameThresholdKey":"","Component":"jobs","Absence":"Optional","SuccessMaxAgeThresholdKey":"","DefaultSuccessMaxAge":120000000000,"StaleOutcome":"Fail"}`, false},
		{"ConfigMap", "configMap", `{"defaultName":"config","nameThresholdKey":"name","component":"settings","absence":"Optional","key":"settings.yaml","recognizedAPIVersions":["example.org/v2","v1"],"unrecognizedOutcome":"Fail","invalidOutcome":"Warn"}`, `{"DefaultNamespace":"team-a","DefaultName":"config","NameThresholdKey":"","Component":"settings","Absence":"Optional","Key":"settings.yaml","RecognizedAPIVersions":["example.org/v2","v1"],"UnrecognizedOutcome":"Fail","InvalidOutcome":"Warn"}`, false},
		{"AnnotationStaleness", "annotationStaleness", `{"apiVersion":"v1","kind":"ConfigMap","defaultName":"config","nameThresholdKey":"name","component":"settings","absence":"Optional","listName":"objects","annotationKey":"example.org/last","timestampJSONField":"observed","maxAgeThresholdKey":"age","defaultMaxAge":"3m","staleOutcome":"Fail"}`, `{"APIVersion":"v1","Kind":"ConfigMap","DefaultNamespace":"team-a","DefaultName":"config","NameThresholdKey":"","Component":"settings","Absence":"Optional","ListName":"objects","AnnotationKey":"example.org/last","TimestampJSONField":"observed","MaxAgeThresholdKey":"","DefaultMaxAge":180000000000,"StaleOutcome":"Fail","ClusterScoped":false}`, false},
		{"PodProjection", "podProjection", `{"selector":{"app":"target"},"listName":"pods","component":"injection","volumeName":"token","envVar":"TOKEN","missingOutcome":"Warn"}`, `{"Selector":{"app":"target"},"ListName":"pods","Component":"injection","VolumeName":"token","EnvVar":"TOKEN","MissingOutcome":"Warn"}`, false},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal([]byte(tc.payload), &payload); err != nil {
				t.Fatal(err)
			}
			target := map[string]any{"scope": "Cluster"}
			if !tc.cluster {
				target = map[string]any{"scope": "Namespaced", "namespaces": []string{"team-a"}}
			}
			payload["target"] = target
			data, err := json.Marshal(map[string]any{"name": "check", "kind": tc.kind, tc.field: payload})
			if err != nil {
				t.Fatal(err)
			}
			var check api.DefinitionCheck
			if err := json.Unmarshal(data, &check); err != nil {
				t.Fatal(err)
			}
			d := &api.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: "custom"}, Spec: api.AddonDefinitionSpec{AddonType: "custom", AdapterVersion: "1.0.0", SemanticsVersion: 1, Families: []api.DefinitionFamily{{Name: "health", Checks: []api.DefinitionCheck{check}}}}}
			if err := definitions.Validate(d); err != nil {
				t.Fatal(err)
			}
			evaluator, namespaces := lowerCheck(check)
			if tc.cluster && len(namespaces) != 0 || !tc.cluster && !reflect.DeepEqual(namespaces, []string{"team-a"}) {
				t.Fatalf("namespace conversion: %v", namespaces)
			}
			actualJSON, err := json.Marshal(evaluator)
			if err != nil {
				t.Fatal(err)
			}
			var actual, want map[string]any
			if err := json.Unmarshal(actualJSON, &actual); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			for field, value := range want {
				if !reflect.DeepEqual(actual[field], value) {
					t.Errorf("%s=%v want=%v", field, actual[field], value)
				}
			}
		})
	}
}

func TestRuntimeResolvedPolicyIsRevalidated(t *testing.T) {
	for _, tc := range []struct {
		name, kind, field, payload string
		policy                     adapter.FamilyPolicy
	}{
		{"webhook helpers", "Webhook", "webhook", `{"target":{"scope":"Cluster"},"kind":"ValidatingWebhookConfiguration","name":"hook","expectedService":"hook","serviceNamespace":"helpers","verifyEndpoints":true}`, adapter.FamilyPolicy{Namespaces: []string{"a", "b"}}},
		{"cron negative age", "CronJob", "cronJob", `{"target":{"scope":"Namespaced","namespaces":["a"]},"defaultName":"job","successMaxAgeThresholdKey":"age"}`, adapter.FamilyPolicy{Thresholds: map[string]string{"age": "-1s"}}},
		{"annotation zero age", "AnnotationStaleness", "annotationStaleness", `{"target":{"scope":"Namespaced","namespaces":["a"]},"apiVersion":"v1","kind":"ConfigMap","defaultName":"config","annotationKey":"example.org/last","defaultMaxAge":"1m","maxAgeThresholdKey":"age"}`, adapter.FamilyPolicy{Thresholds: map[string]string{"age": "0s"}}},
		{"restart overflow", "Workload", "workload", `{"target":{"scope":"Namespaced","namespaces":["a"]},"kind":"Deployment","defaultName":"controller","restartWarnThresholdKey":"restart"}`, adapter.FamilyPolicy{Thresholds: map[string]string{"restart": "2147483648"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var check api.DefinitionCheck
			if err := json.Unmarshal([]byte(`{"name":"check","kind":"`+tc.kind+`","`+tc.field+`":`+tc.payload+`}`), &check); err != nil {
				t.Fatal(err)
			}
			d := &api.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: "custom"}, Spec: api.AddonDefinitionSpec{AddonType: "custom", AdapterVersion: "1.0.0", SemanticsVersion: 1, Families: []api.DefinitionFamily{{Name: "health", Checks: []api.DefinitionCheck{check}}}}}
			if err := definitions.Validate(d); err != nil {
				t.Fatal(err)
			}
			if err := resolveRuntimePolicy(context.Background(), d, map[adapter.Family]adapter.FamilyPolicy{"health": tc.policy}); err == nil {
				t.Fatal("invalid resolved override accepted")
			}
		})
	}
}
