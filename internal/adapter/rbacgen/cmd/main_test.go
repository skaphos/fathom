/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runMainSubprocess re-executes the test binary so the child enters main()
// instead of the test harness. The child detects this by the TEST_MAIN_ARGS
// env var, which carries the os.Args main() should observe (NUL-separated).
// Because the child inherits GOCOVERDIR from `go test`, main()'s coverage is
// merged back into this package's profile — mirrors cmd/main_test.go.
const (
	envMainArgs = "TEST_MAIN_ARGS"
	argSep      = "\x1f" // ASCII unit separator
)

// TestMain_RunsAsMainOnDemand acts as the rbacgen binary when TEST_MAIN_ARGS is
// set, so sibling tests can drive main() in a subprocess.
func TestMain_RunsAsMainOnDemand(t *testing.T) {
	if raw, ok := os.LookupEnv(envMainArgs); ok {
		os.Args = strings.Split(raw, argSep)
		main()
		return
	}
	t.Skip("re-executed by sibling tests")
}

// TestMain_WritesArtifacts exercises the happy path: main() takes the root from
// os.Args[1], generates every artifact under it, prints the written paths, and
// falls through without calling os.Exit.
func TestMain_WritesArtifacts(t *testing.T) {
	root := t.TempDir()
	out, err := runMain(t, "rbacgen", root)
	if err != nil {
		t.Fatalf("main(%s) returned error: %v\noutput:\n%s", root, err, out)
	}
	if !strings.Contains(out, "wrote") {
		t.Errorf("expected 'wrote' lines in output, got:\n%s", out)
	}
	// Sanity: a real per-addon manifest landed under the given root, proving the
	// os.Args[1] root override reached rbacgen.Write.
	if _, err := os.Stat(filepath.Join(root, "config", "rbac", "addons", "addon-coredns.yaml")); err != nil {
		t.Errorf("expected addon-coredns.yaml under root: %v", err)
	}
}

// TestMain_ExitsNonZeroOnWriteError drives the fail-closed os.Exit(1) branch. A
// regular file as root makes MkdirAll under it fail (ENOTDIR), so rbacgen.Write
// errors and main() reports it and exits non-zero.
func TestMain_ExitsNonZeroOnWriteError(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	out, err := runMain(t, "rbacgen", notADir)
	if err == nil {
		t.Fatalf("expected non-zero exit from main() on write error, got nil\noutput:\n%s", out)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, got 0")
	}
	if !strings.Contains(out, "rbacgen:") {
		t.Errorf("expected 'rbacgen:' error prefix in output, got:\n%s", out)
	}
}

func runMain(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMain_RunsAsMainOnDemand$")
	cmd.Env = append(os.Environ(), envMainArgs+"="+strings.Join(args, argSep))
	out, err := cmd.CombinedOutput()
	return string(out), err
}
