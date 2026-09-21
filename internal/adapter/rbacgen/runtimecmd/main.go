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
	if err := rbacgen.WriteRuntimeInventory(".", app.BuiltInAdapters()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := rbacgen.WriteRuntimeSamples("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
