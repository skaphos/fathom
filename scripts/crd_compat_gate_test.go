/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runCRDCompat runs scripts/check-crd-compat.sh with the given environment
// overrides and reports success and combined output. Like the other gate
// tests, the script re-anchors to the repo root via BASH_SOURCE, so running
// it from the scripts/ package directory exercises the CI invocation.
func runCRDCompat(t *testing.T, env map[string]string) (bool, string) {
	t.Helper()

	cmd := exec.Command("bash", "check-crd-compat.sh")
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	return err == nil, string(out)
}

// fixtureCRD renders a minimal valid CRD manifest for gate fixtures. extra is
// spliced into the spec properties block verbatim.
func fixtureCRD(extraProps string) string {
	return `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.fixtures.skaphos.io
spec:
  group: fixtures.skaphos.io
  names:
    kind: Widget
    listKind: WidgetList
    plural: widgets
    singular: widget
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
` + extraProps
}

const baseProps = `                size:
                  type: string
`

// writeFixture lays out an old/new CRD pair plus an allowlist in a temp dir
// and returns the environment overrides pointing the gate at them.
func writeFixture(t *testing.T, oldCRD, newCRD, allowlist string) map[string]string {
	t.Helper()

	dir := t.TempDir()
	oldDir := filepath.Join(dir, "old")
	newDir := filepath.Join(dir, "new")
	for d, content := range map[string]string{oldDir: oldCRD, newDir: newCRD} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if content == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(d, "fixtures.skaphos.io_widgets.yaml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	allowlistPath := filepath.Join(dir, "allowlist.yaml")
	if err := os.WriteFile(allowlistPath, []byte(allowlist), 0o644); err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"CRD_COMPAT_OLD_DIR":   oldDir,
		"CRD_COMPAT_NEW_DIR":   newDir,
		"CRD_COMPAT_ALLOWLIST": allowlistPath,
	}
}

// TestCRDCompatAgainstBaseline is the live gate run CI performs: the current
// config/crd/bases against the latest release tag with the committed
// allowlist. It must pass — this feature's own sanctioned tightenings are
// seeded in .crd-compat-allowlist.yaml. (With no v* tag reachable, e.g. a
// shallow clone, the script passes with a no-baseline notice, so this stays
// green there too.)
func TestCRDCompatAgainstBaseline(t *testing.T) {
	ok, out := runCRDCompat(t, nil)
	if !ok {
		t.Fatalf("check-crd-compat.sh failed against the release baseline:\n%s", out)
	}
}

func TestCRDCompatNoChangePasses(t *testing.T) {
	crd := fixtureCRD(baseProps)
	ok, out := runCRDCompat(t, writeFixture(t, crd, crd, ""))
	if !ok || !strings.Contains(out, "compatible") {
		t.Fatalf("identical schemas must pass (ok=%v):\n%s", ok, out)
	}
}

func TestCRDCompatAddedOptionalFieldPasses(t *testing.T) {
	newCRD := fixtureCRD(baseProps + `                color:
                  type: string
`)
	ok, out := runCRDCompat(t, writeFixture(t, fixtureCRD(baseProps), newCRD, ""))
	if !ok {
		t.Fatalf("adding an optional field is compatible and must pass:\n%s", out)
	}
}

func TestCRDCompatNewCRDSkipped(t *testing.T) {
	env := writeFixture(t, "", fixtureCRD(baseProps), "")
	ok, out := runCRDCompat(t, env)
	if !ok || !strings.Contains(out, "NEW fixtures.skaphos.io_widgets.yaml") {
		t.Fatalf("a CRD with no baseline counterpart must be skipped with a notice (ok=%v):\n%s", ok, out)
	}
}

func TestCRDCompatTightenedValidationFails(t *testing.T) {
	newCRD := fixtureCRD(`                size:
                  type: string
                  maxLength: 10
`)
	ok, out := runCRDCompat(t, writeFixture(t, fixtureCRD(baseProps), newCRD, ""))
	if ok {
		t.Fatalf("tightened validation must fail the gate:\n%s", out)
	}
	if !strings.Contains(out, "INCOMPATIBLE ^.spec.size") || !strings.Contains(out, "maxLength") {
		t.Fatalf("failure output must name the property and change (SC-006):\n%s", out)
	}
	if !strings.Contains(out, ".crd-compat-allowlist.yaml") {
		t.Fatalf("failure output must point at the allowlist mechanism:\n%s", out)
	}
}

func TestCRDCompatRemovedFieldFails(t *testing.T) {
	ok, out := runCRDCompat(t, writeFixture(t, fixtureCRD(baseProps), fixtureCRD(`                other:
                  type: string
`), ""))
	if ok {
		t.Fatalf("removing a field must fail the gate:\n%s", out)
	}
	if !strings.Contains(out, "INCOMPATIBLE") {
		t.Fatalf("failure output must classify the finding:\n%s", out)
	}
}

func TestCRDCompatAllowlistedChangePassesVisibly(t *testing.T) {
	newCRD := fixtureCRD(`                size:
                  type: string
                  maxLength: 10
`)
	allowlist := `- crd: widgets.fixtures.skaphos.io
  path: ^.spec.size
  reason: fixture-sanctioned tightening
  issue: https://github.com/skaphos/fathom/issues/152
`
	ok, out := runCRDCompat(t, writeFixture(t, fixtureCRD(baseProps), newCRD, allowlist))
	if !ok {
		t.Fatalf("an allowlisted change must pass:\n%s", out)
	}
	if !strings.Contains(out, "SANCTIONED ^.spec.size") || !strings.Contains(out, "fixture-sanctioned tightening") {
		t.Fatalf("sanctioned findings must stay visible with their reason:\n%s", out)
	}
}

func TestCRDCompatStaleAllowlistEntryWarns(t *testing.T) {
	crd := fixtureCRD(baseProps)
	allowlist := `- crd: widgets.fixtures.skaphos.io
  path: ^.spec.gone
  reason: sanctioned change already released
  issue: https://github.com/skaphos/fathom/issues/152
`
	ok, out := runCRDCompat(t, writeFixture(t, crd, crd, allowlist))
	if !ok {
		t.Fatalf("a stale allowlist entry must not fail the gate:\n%s", out)
	}
	if !strings.Contains(out, "STALE allowlist entry") || !strings.Contains(out, "^.spec.gone") {
		t.Fatalf("stale entries must be reported for pruning:\n%s", out)
	}
}

func TestCRDCompatMalformedAllowlistFails(t *testing.T) {
	crd := fixtureCRD(baseProps)
	ok, out := runCRDCompat(t, writeFixture(t, crd, crd, "- crd: widgets.fixtures.skaphos.io\n  path: ^.spec\n"))
	if ok {
		t.Fatalf("an allowlist entry missing reason/issue must fail loudly:\n%s", out)
	}
	if !strings.Contains(out, "missing") {
		t.Fatalf("the error must say what is missing:\n%s", out)
	}
}
