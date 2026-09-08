/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// fathomctl is the command-line client for the Fathom operator. The command
// tree lives in internal/cli; this entrypoint only wires auth plugins and the
// exit code.
package main

import (
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// so exec-based and cloud kubeconfigs work exactly as they do for kubectl.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/skaphos/fathom/internal/cli"
)

func main() {
	// kubectl convention: 0 on success, 1 on any error. cobra has already
	// printed the error to stderr by the time Execute returns.
	if err := cli.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
