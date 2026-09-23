/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Command runtimecmd generates the release's offline authoring inventory.
package main

import (
	"fmt"
	"os"

	"github.com/skaphos/fathom/internal/adapter/rbacgen"
	"github.com/skaphos/fathom/internal/app"
)

func main() {
	if err := run("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root string) error {
	if err := rbacgen.WriteRuntimeInventory(root, app.BuiltInAdapters()); err != nil {
		return fmt.Errorf("write runtime inventory: %w", err)
	}
	if err := rbacgen.WriteRuntimeSamples(root); err != nil {
		return fmt.Errorf("write runtime samples: %w", err)
	}
	return nil
}
