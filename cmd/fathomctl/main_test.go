/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// runMainSubprocess re-executes the test binary so the child enters main()
// instead of the test harness. The child detects this by the TEST_MAIN_ARGS
// env var, which carries the os.Args main() should observe (unit-separator
// delimited). Same pattern as cmd/main_test.go.
const (
	envMainArgs = "TEST_MAIN_ARGS"
	argSep      = "\x1f"
)

func TestMain_RunsAsMainOnDemand(t *testing.T) {
	if raw, ok := os.LookupEnv(envMainArgs); ok {
		os.Args = strings.Split(raw, argSep)
		main()
		return
	}
	t.Skip("re-executed by sibling tests")
}

// TestMain_HelpExitsZero covers the successful path through
// cli.NewRootCommand().Execute(): cobra's --help handler returns nil and
// main() falls through without calling os.Exit.
func TestMain_HelpExitsZero(t *testing.T) {
	out, err := runMain(t, "fathomctl", "--help")
	if err != nil {
		t.Fatalf("main(--help) returned error: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "fathomctl") {
		t.Errorf("expected help text naming fathomctl, got:\n%s", out)
	}
}

// TestMain_BadFlagExitsNonZero drives the os.Exit(1) branch: an unknown flag
// makes Execute return an error, so main() exits 1 (kubectl convention).
func TestMain_BadFlagExitsNonZero(t *testing.T) {
	_, err := runMain(t, "fathomctl", "--definitely-not-a-real-flag")
	if err == nil {
		t.Fatalf("expected non-zero exit from main() on unknown flag, got nil")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
	}
}

func runMain(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMain_RunsAsMainOnDemand$")
	cmd.Env = append(os.Environ(), envMainArgs+"="+strings.Join(args, argSep))
	out, err := cmd.CombinedOutput()
	return string(out), err
}
