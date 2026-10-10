/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func helmObjectNamed(objects []map[string]any, kind, suffix string) map[string]any {
	for _, obj := range objects {
		meta, _ := obj["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		if obj["kind"] == kind && strings.HasSuffix(name, suffix) {
			return obj
		}
	}
	return nil
}

// Monitoring objects are opt-in: a default install must not require the
// prometheus-operator CRDs or grant anything beyond today's RBAC (#276).
func TestHelmMonitoringIsOffByDefault(t *testing.T) {
	objects, out, err := renderRuntimeChart(t)
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	for _, kind := range []string{"PrometheusRule", "ServiceMonitor"} {
		if helmObjectNamed(objects, kind, "") != nil {
			t.Errorf("default render contains a %s", kind)
		}
	}
	if helmObjectNamed(objects, "ClusterRoleBinding", "-metrics-reader") != nil {
		t.Error("default render binds the metrics-reader role to someone")
	}
}

// The chart's PrometheusRule is the kustomize sample, byte-for-byte in
// meaning: both install paths ship the same worked example.
func TestHelmPrometheusRuleRendersTheKustomizeSample(t *testing.T) {
	objects, out, err := renderRuntimeChart(t, "--set", "metrics.prometheusRule.enabled=true",
		"--set", "metrics.prometheusRule.labels.release=kps")
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	rule := helmObjectNamed(objects, "PrometheusRule", "")
	if rule == nil {
		t.Fatal("metrics.prometheusRule.enabled=true rendered no PrometheusRule")
	}
	if got := helmMap(t, helmMap(t, rule, "metadata"), "labels")["release"]; got != "kps" {
		t.Errorf("rule selector label release = %v, want kps", got)
	}

	raw, err := os.ReadFile(filepath.Join("..", "config", "components", "prometheus-rule", "prometheusrule.yaml"))
	if err != nil {
		t.Fatalf("read kustomize sample: %v", err)
	}
	var sample map[string]any
	if err := yaml.Unmarshal(raw, &sample); err != nil {
		t.Fatalf("decode kustomize sample: %v", err)
	}
	want := helmMap(t, sample, "spec")["groups"]
	if got := helmMap(t, rule, "spec")["groups"]; !reflect.DeepEqual(got, want) {
		t.Errorf("chart rule groups drifted from the kustomize sample; run `task helm:sync`\n got %v\nwant %v", got, want)
	}
}

// Supplying groups replaces the sample rather than appending to it, so an
// adopter's own policy is never mixed with Fathom's example.
func TestHelmPrometheusRuleCustomGroupsReplaceTheSample(t *testing.T) {
	objects, out, err := renderRuntimeChart(t, "--set", "metrics.prometheusRule.enabled=true",
		"--set", "metrics.prometheusRule.groups[0].name=mine",
		"--set", "metrics.prometheusRule.groups[0].rules[0].alert=MyFathomAlert",
		"--set", "metrics.prometheusRule.groups[0].rules[0].expr=fathom_check_result{result=\"Fail\"} == 1")
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	rule := helmObjectNamed(objects, "PrometheusRule", "")
	if rule == nil {
		t.Fatal("no PrometheusRule rendered")
	}
	groups, _ := helmMap(t, rule, "spec")["groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["name"] != "mine" {
		t.Errorf("groups = %v, want only the custom group", groups)
	}
	if strings.Contains(out, "FathomCheckStale") {
		t.Error("custom groups were rendered alongside the sample")
	}
}

// reader.subjects grants exactly get on /metrics: the binding must point at
// the metrics-reader role and nothing broader.
func TestHelmReaderSubjectsBindOnlyTheMetricsReaderRole(t *testing.T) {
	objects, out, err := renderRuntimeChart(t,
		"--set", "metrics.reader.subjects[0].kind=ServiceAccount",
		"--set", "metrics.reader.subjects[0].name=collector",
		"--set", "metrics.reader.subjects[0].namespace=observability")
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	binding := helmObjectNamed(objects, "ClusterRoleBinding", "-metrics-reader")
	if binding == nil {
		t.Fatal("reader.subjects rendered no metrics-reader binding")
	}
	roleRef := helmMap(t, binding, "roleRef")
	if name, _ := roleRef["name"].(string); !strings.HasSuffix(name, "-metrics-reader") || roleRef["kind"] != "ClusterRole" {
		t.Errorf("binding roleRef = %v, want the metrics-reader ClusterRole", roleRef)
	}
	role := helmObjectNamed(objects, "ClusterRole", roleRef["name"].(string))
	if role == nil {
		t.Fatal("bound role is not rendered")
	}
	rules, _ := role["rules"].([]any)
	if len(rules) != 1 || !reflect.DeepEqual(rules[0].(map[string]any)["nonResourceURLs"], []any{"/metrics"}) ||
		!reflect.DeepEqual(rules[0].(map[string]any)["verbs"], []any{"get"}) {
		t.Errorf("metrics-reader role grants %v, want only get on /metrics", rules)
	}
	subjects, _ := binding["subjects"].([]any)
	if len(subjects) != 1 || subjects[0].(map[string]any)["namespace"] != "observability" {
		t.Errorf("subjects = %v", subjects)
	}
}

// A ServiceAccount subject without a namespace would bind nothing useful; the
// schema refuses it at install time instead.
func TestHelmReaderSubjectSchemaRequiresServiceAccountNamespace(t *testing.T) {
	_, out, err := renderRuntimeChart(t,
		"--set", "metrics.reader.subjects[0].kind=ServiceAccount",
		"--set", "metrics.reader.subjects[0].name=collector")
	if err == nil {
		t.Fatal("a ServiceAccount subject without a namespace was accepted")
	}
	if !strings.Contains(out, "namespace") {
		t.Errorf("schema error does not name the missing namespace:\n%s", out)
	}
}

func helmAnnotations(t *testing.T, objects []map[string]any) map[string]any {
	t.Helper()
	svc := helmObjectNamed(objects, "Service", "-metrics")
	if svc == nil {
		t.Fatal("no metrics Service rendered")
	}
	annotations, _ := helmMap(t, svc, "metadata")["annotations"].(map[string]any)
	return annotations
}

// The ActiveGate forwards its own (typically cluster-wide) ServiceAccount token
// to the metrics endpoint, so the preset refuses to render token forwarding over
// unverified TLS unless the adopter supplies a CA or explicitly accepts it.
func TestHelmDynatracePresetRefusesUnverifiedTokenForwarding(t *testing.T) {
	_, out, err := renderRuntimeChart(t, "--set", "metrics.integrations.dynatrace.enabled=true")
	if err == nil {
		t.Fatal("the Dynatrace preset rendered token forwarding without a CA or an explicit insecureSkipVerify")
	}
	if !strings.Contains(out, "caConfigMap") || !strings.Contains(out, "insecureSkipVerify") {
		t.Errorf("render error does not name both remedies:\n%s", out)
	}
}

// With a CA ConfigMap the preset verifies the serving certificate, grants the
// ActiveGate get on exactly that ConfigMap, and binds it to the metrics-reader
// role. Explicit service annotations are kept alongside the preset's keys and
// win over a key the preset also sets (here `path`).
func TestHelmDynatracePresetVerifiesWithACAConfigMap(t *testing.T) {
	objects, out, err := renderRuntimeChart(t,
		"--set", "metrics.integrations.dynatrace.enabled=true",
		"--set", "metrics.integrations.dynatrace.activeGateServiceAccount.namespace=dt",
		"--set", "metrics.integrations.dynatrace.caConfigMap.name=fathom-metrics-ca",
		// --set-json: --set-string would parse the braces of the JSON value.
		"--set-json", `metrics.service.annotations={"metrics.dynatrace.com/filter":"{\"mode\":\"include\",\"names\":[\"fathom_check_result\"]}","metrics.dynatrace.com/path":"/custom-metrics"}`)
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	want := map[string]any{
		"metrics.dynatrace.com/scrape":               "true",
		"metrics.dynatrace.com/port":                 "8443",
		"metrics.dynatrace.com/path":                 "/custom-metrics", // explicit annotation wins
		"metrics.dynatrace.com/secure":               "true",
		"metrics.dynatrace.com/tls.ca.crt":           "configmap:default:fathom-metrics-ca:ca.crt",
		"metrics.dynatrace.com/insecure_skip_verify": "false",
		"metrics.dynatrace.com/http.auth":            "builtin:default",
		"metrics.dynatrace.com/filter":               `{"mode":"include","names":["fathom_check_result"]}`,
	}
	if got := helmAnnotations(t, objects); !reflect.DeepEqual(got, want) {
		t.Errorf("metrics Service annotations\n got %v\nwant %v", got, want)
	}

	activeGate := []any{map[string]any{"kind": "ServiceAccount", "name": "dynatrace-activegate", "namespace": "dt"}}
	binding := helmObjectNamed(objects, "ClusterRoleBinding", "-metrics-reader")
	if binding == nil {
		t.Fatal("the Dynatrace preset did not bind the ActiveGate to the metrics-reader role")
	}
	if got := binding["subjects"]; !reflect.DeepEqual(got, activeGate) {
		t.Errorf("metrics-reader subjects = %v, want %v", got, activeGate)
	}

	role := helmObjectNamed(objects, "Role", "-dynatrace-metrics-ca")
	if role == nil {
		t.Fatal("no Role lets the ActiveGate read the CA ConfigMap")
	}
	wantRules := []any{map[string]any{
		"apiGroups": []any{""}, "resources": []any{"configmaps"},
		"resourceNames": []any{"fathom-metrics-ca"}, "verbs": []any{"get"},
	}}
	if got := role["rules"]; !reflect.DeepEqual(got, wantRules) {
		t.Errorf("CA Role rules = %v, want get on the one ConfigMap", got)
	}
	roleBinding := helmObjectNamed(objects, "RoleBinding", "-dynatrace-metrics-ca")
	if roleBinding == nil || !reflect.DeepEqual(roleBinding["subjects"], activeGate) {
		t.Errorf("CA RoleBinding = %v, want it bound to the ActiveGate", roleBinding)
	}
}

// Accepting unverified TLS is possible, but only as an explicit choice.
func TestHelmDynatracePresetExplicitInsecureSkipVerify(t *testing.T) {
	objects, out, err := renderRuntimeChart(t,
		"--set", "metrics.integrations.dynatrace.enabled=true",
		"--set", "metrics.integrations.dynatrace.insecureSkipVerify=true")
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	annotations := helmAnnotations(t, objects)
	if annotations["metrics.dynatrace.com/insecure_skip_verify"] != "true" {
		t.Errorf("insecure_skip_verify = %v, want true when explicitly accepted", annotations["metrics.dynatrace.com/insecure_skip_verify"])
	}
	if _, ok := annotations["metrics.dynatrace.com/tls.ca.crt"]; ok {
		t.Error("tls.ca.crt set without a CA ConfigMap")
	}
	if helmObjectNamed(objects, "Role", "-dynatrace-metrics-ca") != nil {
		t.Error("CA ConfigMap access granted without a CA ConfigMap")
	}
}

// With plaintext metrics there is no token to send and nothing to authorize:
// the preset must not ask Dynatrace for TLS or auth, and no binding renders.
func TestHelmDynatracePresetWithPlaintextMetrics(t *testing.T) {
	objects, out, err := renderRuntimeChart(t,
		"--set", "metrics.integrations.dynatrace.enabled=true",
		"--set", "metrics.secure=false", "--set", "metrics.allowInsecure=true")
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	annotations := helmAnnotations(t, objects)
	if annotations["metrics.dynatrace.com/secure"] != "false" {
		t.Errorf("secure = %v, want false for plaintext metrics", annotations["metrics.dynatrace.com/secure"])
	}
	for _, key := range []string{"metrics.dynatrace.com/http.auth", "metrics.dynatrace.com/insecure_skip_verify"} {
		if _, ok := annotations[key]; ok {
			t.Errorf("%s set although metrics are plaintext", key)
		}
	}
	if helmObjectNamed(objects, "ClusterRoleBinding", "-metrics-reader") != nil {
		t.Error("a metrics-reader binding rendered although metrics are plaintext")
	}
}

// The Sumo Logic preset renders the ServiceMonitor on its own and labels it
// for the collection's Target Allocator; explicit labels still win.
func TestHelmSumoLogicPresetRendersALabelledServiceMonitor(t *testing.T) {
	objects, out, err := renderRuntimeChart(t,
		"--set", "metrics.integrations.sumologic.enabled=true",
		"--set", "metrics.integrations.sumologic.releaseName=sumo",
		"--set", "metrics.serviceMonitor.labels.team=platform")
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	monitor := helmObjectNamed(objects, "ServiceMonitor", "")
	if monitor == nil {
		t.Fatal("the Sumo Logic preset rendered no ServiceMonitor")
	}
	labels := helmMap(t, helmMap(t, monitor, "metadata"), "labels")
	if labels["release"] != "sumo" || labels["team"] != "platform" {
		t.Errorf("ServiceMonitor labels = %v, want release=sumo and team=platform", labels)
	}
}

func TestHelmMetricsServiceAnnotations(t *testing.T) {
	objects, out, err := renderRuntimeChart(t,
		"--set-string", `metrics.service.annotations.metrics\.dynatrace\.com/scrape=true`)
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	svc := helmObjectNamed(objects, "Service", "-metrics")
	if svc == nil {
		t.Fatal("no metrics Service rendered")
	}
	annotations := helmMap(t, helmMap(t, svc, "metadata"), "annotations")
	if annotations["metrics.dynatrace.com/scrape"] != "true" {
		t.Errorf("metrics Service annotations = %v", annotations)
	}
}
