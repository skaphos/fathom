/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestE2E runs the end-to-end Ginkgo suite against a pre-prepared kind
// cluster. The Taskfile target `test-e2e` is responsible for creating
// the cluster, installing addons via helmfile, building and loading the
// operator and probe images, and deploying Fathom before this suite
// executes. Running `go test ./test/e2e/` directly against a cluster
// that lacks those preconditions will fail loudly inside the manager
// spec's BeforeAll.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting fathom e2e suite\n")
	RunSpecs(t, "e2e suite")
}
