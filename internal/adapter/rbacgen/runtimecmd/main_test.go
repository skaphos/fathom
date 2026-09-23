/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGeneratesInventoryAndSamples(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg/addondefinition"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := run(root); err != nil {
		t.Fatalf("run: %v", err)
	}

	inventory, err := os.ReadFile(filepath.Join(root, "pkg/addondefinition/inventory_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Package addondefinition", "func BuiltinNames()", `"cilium"`} {
		if !strings.Contains(string(inventory), want) {
			t.Errorf("generated inventory missing %q", want)
		}
	}
	samples, err := filepath.Glob(filepath.Join(root, "config/samples/addondefinition/*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 9 {
		t.Errorf("generated %d samples, want one for each of the nine runtime check kinds", len(samples))
	}
}

func TestRunReportsInventoryWriteFailure(t *testing.T) {
	t.Parallel()
	err := run(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "write runtime inventory") {
		t.Fatalf("run error = %v, want inventory write failure", err)
	}
}

func TestRunReportsSampleWriteFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg/addondefinition"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "config/samples"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config/samples/addondefinition"), []byte("blocked"), 0644); err != nil {
		t.Fatal(err)
	}
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "write runtime samples") {
		t.Fatalf("run error = %v, want sample write failure", err)
	}
}
