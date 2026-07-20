/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package utils

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestParseAddonSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		spec          string
		wantSelectors []string
		wantFilter    string
		wantErr       bool
	}{
		{name: "empty means full stack", spec: "", wantSelectors: nil, wantFilter: ""},
		{name: "all means full stack", spec: " all ", wantSelectors: nil, wantFilter: ""},
		{
			name:          "core tier only",
			spec:          "core",
			wantSelectors: []string{"tier=core"},
			wantFilter:    "core",
		},
		{
			name:          "single opt-in addon installs core too",
			spec:          "istio",
			wantSelectors: []string{"tier=core", "addon=istio"},
			wantFilter:    "istio",
		},
		{
			name: "core-tier addon needs no extra selector",
			spec: "cert-manager",
			// cert-manager's chart is in tier=core, so no addon= selector —
			// but only its own specs run.
			wantSelectors: []string{"tier=core"},
			wantFilter:    "cert-manager",
		},
		{
			name:          "coredns ships with kind",
			spec:          "coredns",
			wantSelectors: []string{"tier=core"},
			wantFilter:    "coredns",
		},
		{
			name:          "list mixes tiers and dedupes",
			spec:          "core, istio, external-dns, istio,",
			wantSelectors: []string{"tier=core", "addon=istio", "addon=external-dns"},
			wantFilter:    "core || istio || external-dns",
		},
		{name: "unknown addon", spec: "isto", wantErr: true},
		{name: "unknown addon in list", spec: "core,nope", wantErr: true},
		{name: "only separators", spec: " , ,", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sel, err := ParseAddonSelection(tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseAddonSelection(%q): expected error, got nil", tc.spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAddonSelection(%q): unexpected error: %v", tc.spec, err)
			}
			if got := sel.HelmfileSelectors(); !reflect.DeepEqual(got, tc.wantSelectors) {
				t.Errorf("HelmfileSelectors() = %#v, want %#v", got, tc.wantSelectors)
			}
			if got := sel.LabelFilter(); got != tc.wantFilter {
				t.Errorf("LabelFilter() = %q, want %q", got, tc.wantFilter)
			}
		})
	}
}

// TestHelmfileLabelsMatchAddonTiers is the drift guard between the tier/addon
// lists compiled into the suite (CoreAddons/OptInAddons) and the release
// labels in test/e2e/fixtures/helmfile.yaml (skaphos/fathom#178). It fails
// when a release is added without an `addon` label, when a core-tier chart is
// not labeled `tier: core`, or when an opt-in addon is known to only one side
// — so E2E_ADDONS can never silently select nothing.
func TestHelmfileLabelsMatchAddonTiers(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("..", "e2e", "fixtures", "helmfile.yaml"))
	if err != nil {
		t.Fatalf("read helmfile: %v", err)
	}
	var doc struct {
		Releases []struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"releases"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse helmfile: %v", err)
	}
	if len(doc.Releases) == 0 {
		t.Fatal("no releases parsed from helmfile.yaml")
	}

	coreCharted := map[string]bool{} // core-tier addons with a helmfile release
	optIn := map[string]bool{}
	for _, r := range doc.Releases {
		addon := r.Labels["addon"]
		if addon == "" {
			t.Errorf("release %q has no addon label", r.Name)
			continue
		}
		if r.Labels["tier"] == "core" {
			coreCharted[addon] = true
		} else {
			optIn[addon] = true
		}
	}

	for addon := range coreCharted {
		if optIn[addon] {
			t.Errorf("addon %q has releases both with and without tier=core", addon)
		}
	}

	// CoreDNS is core-tier but has no release: kind preinstalls it.
	wantCore := map[string]bool{"coredns": true}
	for a := range coreCharted {
		wantCore[a] = true
	}
	gotCore := map[string]bool{}
	for _, a := range CoreAddons() {
		gotCore[a] = true
	}
	if !reflect.DeepEqual(gotCore, wantCore) {
		t.Errorf("CoreAddons() = %v, helmfile tier=core (+coredns) = %v", gotCore, wantCore)
	}

	gotOptIn := map[string]bool{}
	for _, a := range OptInAddons() {
		gotOptIn[a] = true
	}
	if !reflect.DeepEqual(gotOptIn, optIn) {
		t.Errorf("OptInAddons() = %v, helmfile non-core addon labels = %v", gotOptIn, optIn)
	}
}

func TestGetNonEmptyLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string", "", nil},
		{"only newlines", "\n\n\n", nil},
		{"single line", "hello", []string{"hello"}},
		{
			name:  "interleaved blanks",
			input: "a\n\nb\nc\n\n",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "trailing newline",
			input: "x\ny\n",
			want:  []string{"x", "y"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := GetNonEmptyLines(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("GetNonEmptyLines(%q) = %#v, want %#v", tc.input, got, tc.want)
			}
		})
	}
}

func TestGetProjectDir(t *testing.T) {
	dir, err := GetProjectDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(dir, "/test/e2e") {
		t.Fatalf("GetProjectDir() returned a path still containing /test/e2e: %q", dir)
	}
	if dir == "" {
		t.Fatalf("GetProjectDir() returned empty path")
	}
}

func TestGetProjectDirStripsE2ESegment(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	root := t.TempDir()
	e2e := filepath.Join(root, "test", "e2e")
	if err := os.MkdirAll(e2e, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chdir(e2e); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	got, err := GetProjectDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// os.Getwd may resolve symlinks (e.g. /tmp -> /private/tmp on macOS),
	// so compare on the stripped suffix rather than the absolute prefix.
	if strings.HasSuffix(got, "/test/e2e") {
		t.Fatalf("expected /test/e2e to be stripped, got %q", got)
	}
}

func TestUncommentCode(t *testing.T) {
	t.Parallel()

	tmp := filepath.Join(t.TempDir(), "src.txt")
	original := "header\n// foo\n// bar\nfooter\n"
	if err := os.WriteFile(tmp, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := UncommentCode(tmp, "// foo\n// bar", "// "); err != nil {
		t.Fatalf("UncommentCode: %v", err)
	}

	got, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "header\nfoo\nbar\nfooter\n"
	if string(got) != want {
		t.Fatalf("UncommentCode produced %q, want %q", string(got), want)
	}
}

func TestUncommentCodeTargetMissing(t *testing.T) {
	t.Parallel()

	tmp := filepath.Join(t.TempDir(), "src.txt")
	if err := os.WriteFile(tmp, []byte("nothing to uncomment\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := UncommentCode(tmp, "// missing", "// ")
	if err == nil {
		t.Fatalf("expected error for missing target, got nil")
	}
}

func TestUncommentCodeFileMissing(t *testing.T) {
	t.Parallel()

	err := UncommentCode(filepath.Join(t.TempDir(), "does-not-exist"), "x", "// ")
	if err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
}

func TestRunPropagatesCommandFailure(t *testing.T) {
	// Use a portable failing command. `false` exits 1 on Linux and macOS.
	cmd := exec.Command("false")
	out, err := Run(cmd)
	if err == nil {
		t.Fatalf("expected error from failing command, got nil (output=%q)", out)
	}
}

func TestRunCapturesStdout(t *testing.T) {
	cmd := exec.Command("echo", "hello")
	out, err := Run(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected output to contain %q, got %q", "hello", out)
	}
}

// TestInstallersFailWithoutToolchain exercises the kubectl wrappers in an
// environment where the tool is not on PATH. Each wrapper should return a
// non-nil error (or false, for the "is installed" probe) without panicking.
// The point is structural coverage of the wrappers themselves; the actual
// install/uninstall flows are covered by the e2e suite.
func TestInstallersFailWithoutToolchain(t *testing.T) {
	// Point PATH at an empty directory so kubectl cannot be found,
	// regardless of what the host has installed.
	empty := t.TempDir()
	t.Setenv("PATH", empty)

	if err := InstallPrometheusOperator(); err == nil {
		t.Errorf("InstallPrometheusOperator: expected error with empty PATH, got nil")
	}
	if got := IsPrometheusCRDsInstalled(); got {
		t.Errorf("IsPrometheusCRDsInstalled: expected false with empty PATH, got true")
	}
	if _, err := SyncAddons("tier=core", "addon=istio"); err == nil {
		t.Errorf("SyncAddons: expected error with empty PATH, got nil")
	}

	// Uninstall* functions log via warnError and must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("uninstaller panicked: %v", r)
		}
	}()
	UninstallPrometheusOperator()
}

func TestWarnErrorDoesNotPanic(t *testing.T) {
	// warnError writes to GinkgoWriter; just ensure it doesn't panic on a
	// nil-free error value when no Ginkgo node is active.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("warnError panicked: %v", r)
		}
	}()
	warnError(os.ErrNotExist)
}
