/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"os/exec"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

const runtimeChart = "../deploy/helm/fathom-operator"

func renderRuntimeChart(t *testing.T, args ...string) ([]map[string]any, string, error) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	cmd := exec.Command("helm", append([]string{"template", "runtime-test", runtimeChart}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, string(output), err
	}
	var objects []map[string]any
	for _, document := range strings.Split(string(output), "---\n") {
		if strings.TrimSpace(document) == "" {
			continue
		}
		var obj map[string]any
		if err := yaml.Unmarshal([]byte(document), &obj); err != nil {
			t.Fatalf("decode Helm output: %v", err)
		}
		objects = append(objects, obj)
	}
	return objects, string(output), nil
}

func helmObject(t *testing.T, objects []map[string]any, kind string) map[string]any {
	t.Helper()
	for _, obj := range objects {
		if obj["kind"] == kind {
			return obj
		}
	}
	t.Fatalf("Helm output has no %s", kind)
	return nil
}

func helmMap(t *testing.T, obj map[string]any, key string) map[string]any {
	t.Helper()
	child, ok := obj[key].(map[string]any)
	if !ok {
		t.Fatalf("%s is not a map: %T", key, obj[key])
	}
	return child
}

func managerArgs(t *testing.T, objects []map[string]any) []string {
	t.Helper()
	deployment := helmObject(t, objects, "Deployment")
	spec := helmMap(t, helmMap(t, deployment, "spec"), "template")
	pod := helmMap(t, spec, "spec")
	containers, ok := pod["containers"].([]any)
	if !ok || len(containers) != 1 {
		t.Fatalf("manager containers = %v", pod["containers"])
	}
	container, ok := containers[0].(map[string]any)
	if !ok {
		t.Fatalf("manager container has type %T", containers[0])
	}
	values, ok := container["args"].([]any)
	if !ok {
		t.Fatalf("manager args have type %T", container["args"])
	}
	args := make([]string, len(values))
	for i, value := range values {
		args[i], ok = value.(string)
		if !ok {
			t.Fatalf("manager arg %d has type %T", i, value)
		}
	}
	return args
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func assertRuntimeLoadingArg(t *testing.T, args []string, want bool) {
	t.Helper()
	if !want {
		for _, arg := range args {
			if arg == "--runtime-loading-enabled" || strings.HasPrefix(arg, "--runtime-loading-enabled=") {
				t.Errorf("runtime opt-in flag unexpectedly present: %v", args)
				return
			}
		}
		return
	}
	if hasArg(args, "--runtime-loading-enabled") {
		return
	}
	t.Errorf("runtime opt-in flag missing: %v", args)
}

func TestHelmRuntimeLoadingOptIn(t *testing.T) {
	tests := []struct {
		name, namespace string
		values          []string
		wantRuntime     bool
		wantElection    string
		wantLease       string
		wantMetrics     string
	}{
		{name: "default off", wantElection: "--leader-elect=true"},
		{name: "enabled in custom namespace and Lease", namespace: "custom-operator", values: []string{"--set", "runtimeLoading.enabled=true", "--set", "leaderElectionID=custom-lease"}, wantRuntime: true, wantElection: "--leader-elect=true", wantLease: "--leader-election-id=custom-lease"},
		{name: "disabled election keeps deployment renderable", values: []string{"--set", "runtimeLoading.enabled=true", "--set", "leaderElect=false"}, wantRuntime: true, wantElection: "--leader-elect=false"},
		{name: "metrics off with runtime on", values: []string{"--set", "runtimeLoading.enabled=true", "--set-string", "metrics.bindAddress=0"}, wantRuntime: true, wantElection: "--leader-elect=true", wantMetrics: "--metrics-bind-address=0"},
		{name: "config file may opt in when chart value false", values: []string{"--set", "config.enabled=true", "--set", "config.data.runtimeLoading.enabled=true"}, wantElection: "--leader-elect=true"},
		{name: "environment may opt in when chart value false", values: []string{"--set-json", `extraEnv=[{"name":"FATHOM_RUNTIMELOADING_ENABLED","value":"true"}]`}, wantElection: "--leader-elect=true"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{}, tc.values...)
			if tc.namespace != "" {
				args = append(args, "--namespace", tc.namespace)
			}
			objects, output, err := renderRuntimeChart(t, args...)
			if err != nil {
				t.Fatalf("helm template: %v\n%s", err, output)
			}
			manager := managerArgs(t, objects)
			assertRuntimeLoadingArg(t, manager, tc.wantRuntime)
			if !hasArg(manager, tc.wantElection) {
				t.Errorf("manager lacks %q: %v", tc.wantElection, manager)
			}
			if tc.wantLease != "" && !hasArg(manager, tc.wantLease) {
				t.Errorf("manager lacks %q: %v", tc.wantLease, manager)
			}
			if tc.wantMetrics != "" && !hasArg(manager, tc.wantMetrics) {
				t.Errorf("manager lacks %q: %v", tc.wantMetrics, manager)
			}
			if tc.namespace != "" {
				metadata := helmMap(t, helmObject(t, objects, "Deployment"), "metadata")
				if got := metadata["namespace"]; got != tc.namespace {
					t.Errorf("deployment namespace = %v, want %s", got, tc.namespace)
				}
				env := helmMap(t, helmObject(t, objects, "Deployment"), "spec")
				env = helmMap(t, env, "template")
				env = helmMap(t, env, "spec")
				containers := env["containers"].([]any)
				container := containers[0].(map[string]any)
				envs := container["env"].([]any)
				foundNamespace := false
				for _, item := range envs {
					entry := item.(map[string]any)
					if entry["name"] != "FATHOM_NAMESPACE" {
						continue
					}
					valueFrom := entry["valueFrom"].(map[string]any)
					fieldRef := valueFrom["fieldRef"].(map[string]any)
					if fieldRef["fieldPath"] != "metadata.namespace" {
						t.Errorf("FATHOM_NAMESPACE fieldPath = %v, want metadata.namespace", fieldRef["fieldPath"])
					}
					foundNamespace = true
				}
				if !foundNamespace {
					t.Error("deployment does not set FATHOM_NAMESPACE from the pod namespace")
				}
			}
			if tc.name == "config file may opt in when chart value false" {
				config := helmObject(t, objects, "ConfigMap")
				data := helmMap(t, config, "data")
				if !strings.Contains(data["config.yaml"].(string), "runtimeLoading:\n  enabled: true") {
					t.Errorf("config did not carry runtime opt-in: %v", data)
				}
			}
			if tc.name == "environment may opt in when chart value false" && !strings.Contains(output, "name: FATHOM_RUNTIMELOADING_ENABLED") {
				t.Error("deployment did not retain the runtime environment override")
			}
		})
	}
}

func TestHelmRuntimeLoadingRejectsString(t *testing.T) {
	_, output, err := renderRuntimeChart(t, "--set-string", "runtimeLoading.enabled=true")
	if err == nil || !strings.Contains(output, "/runtimeLoading/enabled") {
		t.Fatalf("schema accepted nonboolean runtimeLoading.enabled: err=%v output=%s", err, output)
	}
}
