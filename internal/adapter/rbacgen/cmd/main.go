/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Command rbacgen regenerates the per-addon RBAC manifests, the operator
// impersonate Role, the Helm data file, and the docs matrix from the compiled-in
// adapters' declared RBAC (SKA-58). It is wired as the `gen:addon-rbac` task and
// gated by `verify-generated`; run it from the repo root.
package main

import (
	"fmt"
	"os"

	"github.com/skaphos/fathom/internal/adapter/rbacgen"
	"github.com/skaphos/fathom/internal/app"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	paths, err := rbacgen.Write(root, app.BuiltInAdapters())
	if err != nil {
		fmt.Fprintln(os.Stderr, "rbacgen:", err)
		os.Exit(1)
	}
	for _, p := range paths {
		fmt.Println("wrote", p)
	}
}
