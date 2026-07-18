/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// runMainSubprocess re-executes the test binary so the child enters main()
// instead of the test harness. The child detects this by the TEST_MAIN_ARGS
// env var, which carries the os.Args main() should observe (NUL-separated).
const (
	envMainArgs = "TEST_MAIN_ARGS"
	argSep      = "\x1f" // ASCII unit separator
)

func TestMain_RunsAsMainOnDemand(t *testing.T) {
	// When this env var is set the test binary acts as the operator binary.
	if raw, ok := os.LookupEnv(envMainArgs); ok {
		os.Args = strings.Split(raw, argSep)
		main()
		return
	}
	t.Skip("re-executed by sibling tests")
}

// TestMain_HelpExitsZero exercises the happy-path branch of main(): cobra's
// builtin --help handler returns nil and main() falls through without calling
// os.Exit. This covers the successful path through app.NewRootCommand().Execute().
func TestMain_HelpExitsZero(t *testing.T) {
	out, err := runMain(t, "fathom", "--help")
	if err != nil {
		t.Fatalf("main(--help) returned error: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "Usage") && !strings.Contains(out, "fathom") {
		t.Errorf("expected help text in output, got:\n%s", out)
	}
}

// TestMain_BadFlagExitsNonZero drives the os.Exit(1) branch: an unknown flag
// makes cobra's Execute return an error, so main() exits non-zero.
func TestMain_BadFlagExitsNonZero(t *testing.T) {
	_, err := runMain(t, "fathom", "--definitely-not-a-real-flag")
	if err == nil {
		t.Fatalf("expected non-zero exit from main() on unknown flag, got nil")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, got 0")
	}
}

func runMain(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMain_RunsAsMainOnDemand$")
	cmd.Env = append(os.Environ(), envMainArgs+"="+strings.Join(args, argSep))
	out, err := cmd.CombinedOutput()
	return string(out), err
}
