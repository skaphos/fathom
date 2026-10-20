/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The compat gate classifies only property-level crdify findings, and crdify
// reports nothing for two CRDs with no served version in common. Without a
// dedicated check, replacing a served version (v1alpha1 -> v1) would pass the
// gate silently; the servedVersionRemoval check makes it a finding that only
// an allowlist entry with path "*" can sanction (spec 014, research R4).

func absPath(t *testing.T, rel string) string {
	t.Helper()
	p, err := filepath.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// runCompatGate runs the gate over the fixture CRD directories with the given
// allowlist file (an absolute path) and returns its output and exit code.
func runCompatGate(t *testing.T, allowlist string) (string, int) {
	t.Helper()
	return runCompatGateDirs(t, absPath(t, "testdata/crd-compat/old"), absPath(t, "testdata/crd-compat/new"), allowlist)
}

func runCompatGateDirs(t *testing.T, oldDir, newDir, allowlist string) (string, int) {
	t.Helper()
	cmd := exec.Command("bash", "check-crd-compat.sh")
	cmd.Env = append(os.Environ(),
		"CRD_COMPAT_OLD_DIR="+oldDir,
		"CRD_COMPAT_NEW_DIR="+newDir,
		"CRD_COMPAT_ALLOWLIST="+allowlist,
	)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("run gate: %v\n%s", err, out)
		}
		code = exit.ExitCode()
	}
	return string(out), code
}

func fixtureAllowlist(t *testing.T, name string) string {
	t.Helper()
	return absPath(t, filepath.Join("testdata", "crd-compat", name))
}

// controller-gen writes two-space sequence entries, but the gate must not fail
// open on another valid layout: with fixtureCRD's four-space entries a
// v1alpha1 -> v1 swap used to parse as "no served versions" on both sides and
// pass as compatible.
func TestCompatGateReadsServedVersionsAtAnyIndent(t *testing.T) {
	alpha := fixtureCRD(baseProps)
	v1 := strings.Replace(alpha, "- name: v1alpha1", "- name: v1", 1)
	if v1 == alpha {
		t.Fatal("fixture no longer serves v1alpha1")
	}
	ok, out := runCRDCompat(t, writeFixture(t, alpha, v1, ""))
	if ok {
		t.Fatalf("a served-version swap in a four-space layout passed the gate:\n%s", out)
	}
	if !strings.Contains(out, "INCOMPATIBLE ^ (servedVersionRemoval)") {
		t.Errorf("expected the swap to be reported:\n%s", out)
	}
}

// A baseline serving two versions must lose neither to the parser: a trailing
// comment on `served: true` used to read as "not served", so a target that
// dropped that version produced no finding.
func TestCompatGateSeesEveryServedVersionOfAMultiVersionBaseline(t *testing.T) {
	alpha := strings.TrimRight(fixtureCRD(baseProps), "\n") + "\n"
	entry := alpha[strings.Index(alpha, "    - name: v1alpha1"):]
	second := strings.Replace(strings.Replace(entry, "- name: v1alpha1", "- name: v1", 1), "storage: true", "storage: false", 1)
	twoVersions := strings.Replace(alpha, "      served: true\n", "      served: true # legacy clients\n", 1) + second
	v1Only := strings.Replace(alpha, "- name: v1alpha1", "- name: v1", 1)
	ok, out := runCRDCompat(t, writeFixture(t, twoVersions, v1Only, ""))
	if ok {
		t.Fatalf("dropping the commented served version passed the gate:\n%s", out)
	}
	if !strings.Contains(out, "INCOMPATIBLE ^ (servedVersionRemoval)") || !strings.Contains(out, "version v1alpha1 is served at the baseline") {
		t.Errorf("expected the v1alpha1 removal to be reported:\n%s", out)
	}
}

// An unrecognised `served` spelling is unreadable, not false: the gate refuses
// to judge the CRD rather than compare a partial version set.
func TestCompatGateRefusesAnUnrecognisedServedValue(t *testing.T) {
	alpha := fixtureCRD(baseProps)
	odd := strings.Replace(alpha, "      served: true\n", "      served: yes\n", 1)
	if odd == alpha {
		t.Fatal("fixture no longer declares served: true")
	}
	ok, out := runCRDCompat(t, writeFixture(t, odd, odd, ""))
	if ok || !strings.Contains(out, "cannot read the served versions") {
		t.Fatalf("ok = %v, want a refusal naming the unreadable versions:\n%s", ok, out)
	}
}

// A CRD whose served versions cannot be read is refused, never judged as
// serving none: an unreadable layout must not become a silent pass.
func TestCompatGateRefusesACRDWhoseVersionsItCannotRead(t *testing.T) {
	alpha := fixtureCRD(baseProps)
	unserved := strings.Replace(alpha, "      served: true\n", "", 1)
	if unserved == alpha {
		t.Fatal("fixture no longer declares served: true")
	}
	ok, out := runCRDCompat(t, writeFixture(t, unserved, unserved, ""))
	if ok || !strings.Contains(out, "cannot read the served versions") {
		t.Fatalf("ok = %v, want a refusal naming the unreadable versions:\n%s", ok, out)
	}
}

func TestCompatGateReportsServedVersionRemoval(t *testing.T) {
	out, code := runCompatGate(t, fixtureAllowlist(t, "allowlist-widgets.yaml"))
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (gadgets swap is unsanctioned)\n%s", code, out)
	}
	for _, want := range []string{
		"widgets.example.com: 1 finding(s)",
		"SANCTIONED ^ (servedVersionRemoval): served version replaced in the fixture",
		"gadgets.example.com: 1 finding(s)",
		"INCOMPATIBLE ^ (servedVersionRemoval):",
		"version v1alpha1 is served at the baseline but not by the target",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "STALE") {
		t.Errorf("the widgets sanction matched, so nothing should be STALE:\n%s", out)
	}
}

func TestCompatGatePassesWhenEverySwapIsSanctioned(t *testing.T) {
	out, code := runCompatGate(t, fixtureAllowlist(t, "allowlist-both.yaml"))
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out)
	}
	if n := strings.Count(out, "SANCTIONED ^ (servedVersionRemoval)"); n != 2 {
		t.Errorf("want two sanctioned swaps, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "check-crd-compat: OK") {
		t.Errorf("gate did not report OK:\n%s", out)
	}
}

// An entry whose path names a property cannot sanction a served-version
// removal: the whole-CRD "*" sanction is the deliberate, reviewable shape, and
// the property entry is reported STALE because it matched nothing.
func TestCompatGateRequiresStarForServedVersionRemoval(t *testing.T) {
	allow := filepath.Join(t.TempDir(), "allowlist.yaml")
	if err := os.WriteFile(allow, []byte(
		"- crd: widgets.example.com\n  path: ^.spec\n  reason: property-level entry\n  issue: https://example.com/issues/2\n"+
			"- crd: gadgets.example.com\n  path: \"*\"\n  reason: ok\n  issue: https://example.com/issues/1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code := runCompatGate(t, allow)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (widgets swap only has a property-level entry)\n%s", code, out)
	}
	if !strings.Contains(out, "INCOMPATIBLE ^ (servedVersionRemoval)") {
		t.Errorf("widgets swap should be unsanctioned:\n%s", out)
	}
	if !strings.Contains(out, "STALE allowlist entry (matched nothing): widgets.example.com ^.spec") {
		t.Errorf("the property-level entry should be reported STALE:\n%s", out)
	}
}

// A "*" entry sanctions the served-version replacement only. Once both sides
// serve v1, a property tightening on the same CRD must still be an
// unsanctioned finding, or the seven 0.7.0 entries would cover every later
// incompatible v1 change until someone remembered to prune them.
func TestCompatGateStarDoesNotSanctionPropertyFindings(t *testing.T) {
	gadgets, err := os.ReadFile("testdata/crd-compat/new/example.com_gadgets.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tightened := strings.Replace(string(gadgets), "                type: integer\n", "                type: integer\n                maximum: 5\n", 1)
	if tightened == string(gadgets) {
		t.Fatal("fixture no longer has the size property to tighten")
	}
	oldDir, newDir := t.TempDir(), t.TempDir()
	for dir, body := range map[string]string{oldDir: string(gadgets), newDir: tightened} {
		if err := os.WriteFile(filepath.Join(dir, "example.com_gadgets.yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	allow := filepath.Join(t.TempDir(), "allowlist.yaml")
	if err := os.WriteFile(allow, []byte("- crd: gadgets.example.com\n  path: \"*\"\n  reason: stale swap entry\n  issue: https://example.com/issues/1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code := runCompatGateDirs(t, oldDir, newDir, allow)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (a \"*\" entry must not sanction a property finding)\n%s", code, out)
	}
	if !strings.Contains(out, "INCOMPATIBLE") || !strings.Contains(out, "maximum") {
		t.Errorf("the tightened maximum should be an unsanctioned finding:\n%s", out)
	}
	if !strings.Contains(out, "STALE allowlist entry (matched nothing): gadgets.example.com *") {
		t.Errorf("the \"*\" entry should be reported STALE once no version is replaced:\n%s", out)
	}
}

// crdify reports a scope flip as a CRD-level finding with no property path;
// the gate used to read only property comparisons and passed it as compatible.
// A flipped scope is a different resource, so no entry, not even "*", may
// sanction it (fathomctl refuses it the same way).
func TestCompatGateRefusesAScopeChange(t *testing.T) {
	gadgets, err := os.ReadFile("testdata/crd-compat/new/example.com_gadgets.yaml")
	if err != nil {
		t.Fatal(err)
	}
	flipped := strings.Replace(string(gadgets), "  scope: Namespaced\n", "  scope: Cluster\n", 1)
	if flipped == string(gadgets) {
		t.Fatal("fixture is no longer Namespaced")
	}
	oldDir, newDir := t.TempDir(), t.TempDir()
	for dir, body := range map[string]string{oldDir: string(gadgets), newDir: flipped} {
		if err := os.WriteFile(filepath.Join(dir, "example.com_gadgets.yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	allow := filepath.Join(t.TempDir(), "allowlist.yaml")
	if err := os.WriteFile(allow, []byte("- crd: gadgets.example.com\n  path: \"*\"\n  reason: swap\n  issue: https://example.com/issues/1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code := runCompatGateDirs(t, oldDir, newDir, allow)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (a scope change is never sanctionable)\n%s", code, out)
	}
	if !strings.Contains(out, "INCOMPATIBLE ^ (scope)") {
		t.Errorf("the scope flip should be an unsanctioned CRD-level finding:\n%s", out)
	}
}
