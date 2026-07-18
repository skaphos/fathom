/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package app

import (
	"io"
	"strings"
	"testing"
)

// TestNewRootCommand_BasicWiring locks the cobra wiring NewRootCommand
// constructs: command name, RunE existence, persistent flags (which
// future subcommands inherit). The RunE itself is not invoked here
// (it needs a kubeconfig); its signal/context handoff is covered by
// the dedicated signalContext tests in root_test.go.
func TestNewRootCommand_BasicWiring(t *testing.T) {
	cmd := NewRootCommand()
	if cmd == nil {
		t.Fatal("NewRootCommand returned nil")
	}
	if cmd.Use != "fathom" {
		t.Errorf("Use = %q, want \"fathom\"", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("Short is empty")
	}
	if cmd.Long == "" {
		t.Error("Long is empty")
	}
	if cmd.RunE == nil {
		t.Error("RunE is nil")
	}
	if !cmd.SilenceUsage {
		t.Error("SilenceUsage should be true (setup failures aren't usage errors)")
	}
}

// TestNewRootCommand_RegistersConfigFlag confirms the persistent --config
// flag is registered and points at the documented default. This is part
// of the operator's user-facing surface: the flag name and default path
// appear in deployment manifests and docs.
func TestNewRootCommand_RegistersConfigFlag(t *testing.T) {
	cmd := NewRootCommand()
	flag := cmd.PersistentFlags().Lookup("config")
	if flag == nil {
		t.Fatal("--config persistent flag missing")
	}
	if flag.DefValue != DefaultConfigPath {
		t.Errorf("--config default = %q, want %q", flag.DefValue, DefaultConfigPath)
	}
}

// TestNewRootCommand_RegistersFathomFlags confirms RegisterFlags wired
// the operator's configuration flags onto persistent flags so future
// subcommands inherit them. We sample one well-known flag per concern
// rather than enumerate all flags (which is the job of options_test.go).
func TestNewRootCommand_RegistersFathomFlags(t *testing.T) {
	cmd := NewRootCommand()
	wantFlags := []string{
		"metrics-bind-address",
		"health-probe-bind-address",
		"leader-elect",
	}
	for _, name := range wantFlags {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("persistent flag --%s missing", name)
		}
	}
}

// TestNewRootCommand_HelpDoesNotErrorWithoutKubeconfig confirms that
// invoking --help short-circuits cobra's RunE — so the help text is
// reachable even when no kubeconfig is configured. (RunE would fail
// at ctrl.GetConfig otherwise; cobra runs help instead of RunE when
// --help is passed.)
func TestNewRootCommand_HelpDoesNotErrorWithoutKubeconfig(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--help"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute --help: %v", err)
	}
	if !strings.Contains(out.String(), "fathom") {
		t.Errorf("--help output should mention command name; got: %q", out.String())
	}
}

// TestNewRootCommand_RunE_SurfacesKubeconfigError exercises RunE up to
// the ctrl.GetConfig step. With no KUBECONFIG, no in-cluster service
// account, and no default ~/.kube/config (HOME pointed at a temp dir),
// ctrl.GetConfig returns an error and RunE wraps it. This also covers
// Load(...) on a configExplicit=false code path (the default-config-file
// branch).
func TestNewRootCommand_RunE_SurfacesKubeconfigError(t *testing.T) {
	t.Setenv("KUBECONFIG", "")
	t.Setenv("HOME", t.TempDir())

	cmd := NewRootCommand()
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute: expected error from missing kubeconfig, got nil")
	}
	if !strings.Contains(err.Error(), "load kubeconfig") {
		t.Errorf("error should wrap kubeconfig load failure; got: %v", err)
	}
}
