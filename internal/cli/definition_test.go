/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
)

func TestDefinitionRenderOffline(t *testing.T) {
	file := filepath.Join(t.TempDir(), "definition.yaml")
	data := `apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonDefinition
metadata:
  name: example-addon
spec:
  addonType: example-addon
  adapterVersion: 1.0.0
  semanticsVersion: 1
  families:
  - name: health
    checks:
    - name: controller
      kind: Workload
      workload:
        target:
          scope: Namespaced
          namespaces: [example]
        kind: Deployment
        defaultName: controller
`
	if err := os.WriteFile(file, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	f := newFactory()
	f.clientConfig = func(*globalOptions) clientcmd.ClientConfig { t.Fatal("offline command loaded kubeconfig"); return nil }
	out, _, err := execVerb(f, "definition", "render", "--file", file, "--service-account", "reader", "--operator-namespace", "fathom", "--operator-service-account", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "kind: AddonDefinitionBinding") || !strings.Contains(out, "enabled: false") {
		t.Fatal(out)
	}
	if err := os.WriteFile(file, []byte(data+"  arbitraryField: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := execVerb(f, "definition", "render", "--file", file, "--service-account", "reader", "--operator-namespace", "fathom", "--operator-service-account", "operator"); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestDefinitionRejectsMultipleDocuments(t *testing.T) {
	file := filepath.Join(t.TempDir(), "multi.yaml")
	if err := os.WriteFile(file, []byte("kind: AddonDefinition\n---\nkind: Secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readDefinition(file); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("unexpected error: %v", err)
	}
}
