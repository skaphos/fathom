/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeLoadingConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, config, env string
		flags             []string
		want              bool
	}{
		{name: "default disabled"},
		{name: "config", config: "runtimeLoading:\n  enabled: true\n", want: true},
		{name: "environment beats config", config: "runtimeLoading:\n  enabled: false\n", env: "true", want: true},
		{name: "flag beats environment", env: "true", flags: []string{"--runtime-loading-enabled=false"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FATHOM_RUNTIMELOADING_ENABLED", tc.env)
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			fs, zapOpts := newTestFlags(t)
			if err := fs.Parse(tc.flags); err != nil {
				t.Fatal(err)
			}
			got, err := Load(fs, *zapOpts, path, true)
			if err != nil {
				t.Fatal(err)
			}
			if got.RuntimeLoading.Enabled != tc.want {
				t.Fatalf("enabled=%v want %v", got.RuntimeLoading.Enabled, tc.want)
			}
		})
	}
}

func TestRuntimeLoadingAdmissionRequiresElection(t *testing.T) {
	for _, tc := range []struct {
		name              string
		enabled, election bool
		namespace, reason string
	}{
		{name: "disabled", election: true, namespace: "fathom-system", reason: "RuntimeLoadingDisabled"},
		{name: "election disabled", enabled: true, namespace: "fathom-system", reason: "LeaderElectionRequired"},
		{name: "namespace missing", enabled: true, election: true, reason: "OperatorNamespaceRequired"},
		{name: "eligible", enabled: true, election: true, namespace: "fathom-system"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultOptions()
			opts.RuntimeLoading.Enabled = tc.enabled
			opts.LeaderElect = tc.election
			opts.Namespace = tc.namespace
			if got := opts.RuntimeLoadingDisabledReason(); got != tc.reason {
				t.Fatalf("reason=%q want %q", got, tc.reason)
			}
			if err := opts.Validate(); err != nil {
				t.Fatalf("runtime precondition must not prevent built-in startup: %v", err)
			}
		})
	}
}
