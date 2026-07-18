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
)

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
	if _, err := SyncAddons(); err == nil {
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
