/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package rbacgen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skaphos/fathom/internal/adapter/rbacgen"
	"github.com/skaphos/fathom/internal/app"
)

func TestWritePrunesStaleAddonFiles(t *testing.T) {
	root := t.TempDir()
	addonsDir := filepath.Join(root, "config", "rbac", "addons")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A leftover manifest from an adapter that no longer exists.
	ghost := filepath.Join(addonsDir, "addon-ghost.yaml")
	if err := os.WriteFile(ghost, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("seed ghost: %v", err)
	}

	if _, err := rbacgen.Write(root, app.BuiltInAdapters()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if _, err := os.Stat(ghost); !os.IsNotExist(err) {
		t.Errorf("stale addon-ghost.yaml was not pruned (stat err = %v)", err)
	}
	// A real addon's manifest must be (re)generated.
	if _, err := os.Stat(filepath.Join(addonsDir, "addon-coredns.yaml")); err != nil {
		t.Errorf("expected addon-coredns.yaml to be generated: %v", err)
	}
}
