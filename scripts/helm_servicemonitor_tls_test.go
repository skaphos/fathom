/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"reflect"
	"strings"
	"testing"
)

// serviceMonitorEndpoint returns the single endpoint of the rendered
// ServiceMonitor.
func serviceMonitorEndpoint(t *testing.T, objects []map[string]any) map[string]any {
	t.Helper()
	monitor := helmObjectNamed(objects, "ServiceMonitor", "")
	if monitor == nil {
		t.Fatal("no ServiceMonitor rendered")
	}
	endpoints, _ := helmMap(t, monitor, "spec")["endpoints"].([]any)
	if len(endpoints) != 1 {
		t.Fatalf("ServiceMonitor endpoints = %v", endpoints)
	}
	return endpoints[0].(map[string]any)
}

// The scraper forwards its ServiceAccount token, so the chart no longer
// defaults to skipping verification of the serving certificate.
func TestHelmServiceMonitorRefusesUnverifiedTLSByDefault(t *testing.T) {
	_, out, err := renderRuntimeChart(t, "--set", "metrics.serviceMonitor.enabled=true")
	if err == nil {
		t.Fatal("the ServiceMonitor rendered token forwarding without certificate verification")
	}
	if !strings.Contains(out, "certSecretName") || !strings.Contains(out, "tlsConfig") {
		t.Errorf("render error does not name both remedies:\n%s", out)
	}
}

// With a mounted CA-signed metrics certificate the chart verifies against its
// ca.crt and the metrics Service's DNS name without further configuration.
func TestHelmServiceMonitorVerifiesAgainstTheMetricsCertificate(t *testing.T) {
	objects, out, err := renderRuntimeChart(t,
		"--set", "metrics.serviceMonitor.enabled=true",
		"--set", "metrics.certSecretName=fathom-metrics-cert",
		"--set", "metrics.certPath=/tmp/k8s-metrics-server/metrics-certs")
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	want := map[string]any{
		"ca":                 map[string]any{"secret": map[string]any{"name": "fathom-metrics-cert", "key": "ca.crt"}},
		"serverName":         "runtime-test-fathom-operator-metrics.default.svc",
		"insecureSkipVerify": false,
	}
	if got := serviceMonitorEndpoint(t, objects)["tlsConfig"]; !reflect.DeepEqual(got, want) {
		t.Errorf("tlsConfig\n got %v\nwant %v", got, want)
	}
}

// An explicit tlsConfig is used verbatim. Regression: the old default
// (insecureSkipVerify: true) deep-merged into a user's own tlsConfig, so
// setting only caFile silently kept verification off.
func TestHelmServiceMonitorExplicitTLSIsVerbatim(t *testing.T) {
	objects, out, err := renderRuntimeChart(t,
		"--set", "metrics.serviceMonitor.enabled=true",
		"--set", "metrics.serviceMonitor.tlsConfig.caFile=/etc/prometheus/ca.crt",
		"--set", "metrics.serviceMonitor.tlsConfig.serverName=fathom-metrics.monitoring.svc")
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	want := map[string]any{"caFile": "/etc/prometheus/ca.crt", "serverName": "fathom-metrics.monitoring.svc"}
	if got := serviceMonitorEndpoint(t, objects)["tlsConfig"]; !reflect.DeepEqual(got, want) {
		t.Errorf("tlsConfig\n got %v\nwant %v (no inherited insecureSkipVerify)", got, want)
	}

	objects, out, err = renderRuntimeChart(t,
		"--set", "metrics.serviceMonitor.enabled=true",
		"--set", "metrics.serviceMonitor.tlsConfig.insecureSkipVerify=true")
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	if got := serviceMonitorEndpoint(t, objects)["tlsConfig"]; !reflect.DeepEqual(got, map[string]any{"insecureSkipVerify": true}) {
		t.Errorf("explicit opt-out tlsConfig = %v", got)
	}
}

// The CA Secret can only be referenced from the ServiceMonitor's own
// namespace, so a ServiceMonitor placed elsewhere needs an explicit tlsConfig.
func TestHelmServiceMonitorInAnotherNamespaceNeedsExplicitTLS(t *testing.T) {
	_, out, err := renderRuntimeChart(t,
		"--set", "metrics.serviceMonitor.enabled=true",
		"--set", "metrics.serviceMonitor.namespace=monitoring",
		"--set", "metrics.certSecretName=fathom-metrics-cert",
		"--set", "metrics.certPath=/tmp/k8s-metrics-server/metrics-certs")
	if err == nil {
		t.Fatal("a ServiceMonitor outside the release namespace referenced the release-namespace CA Secret")
	}
	if !strings.Contains(out, "release namespace") {
		t.Errorf("render error does not explain the namespace constraint:\n%s", out)
	}
}

// Plaintext metrics carry no token and no TLS, so nothing is required.
func TestHelmServiceMonitorPlaintextNeedsNoTLS(t *testing.T) {
	objects, out, err := renderRuntimeChart(t,
		"--set", "metrics.serviceMonitor.enabled=true",
		"--set", "metrics.secure=false", "--set", "metrics.allowInsecure=true")
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	endpoint := serviceMonitorEndpoint(t, objects)
	for _, key := range []string{"tlsConfig", "bearerTokenFile"} {
		if _, ok := endpoint[key]; ok {
			t.Errorf("plaintext ServiceMonitor endpoint carries %s", key)
		}
	}
	if endpoint["scheme"] != "http" {
		t.Errorf("scheme = %v, want http", endpoint["scheme"])
	}
}
