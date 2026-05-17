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

	"github.com/skaphos/fathom/test/utils"
)

// TestE2E runs the end-to-end Ginkgo suite against a pre-prepared kind
// cluster. The Taskfile target `test-e2e` creates the cluster, builds and
// loads the operator and probe images, and deploys Fathom before this
// suite executes. The suite itself owns the addon-stack precondition:
// BeforeSuite invokes helmfile against test/e2e/fixtures/helmfile.yaml so
// re-running the suite against a reused cluster always re-syncs the
// canonical chart versions.
//
// Running `go test ./test/e2e/` directly against a cluster that lacks the
// operator deployment will fail loudly inside the manager spec's
// BeforeAll.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting fathom e2e suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("syncing the e2e addon stack via helmfile")
	out, err := utils.SyncAddons()
	Expect(err).NotTo(HaveOccurred(), "helmfile sync failed:\n%s", out)
})
