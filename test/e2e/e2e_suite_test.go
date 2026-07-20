/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"fmt"
	"os"
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
// The stack is tiered (skaphos/fathom#178): E2E_ADDONS scopes both the
// helmfile sync and the specs that run, so a per-adapter run only pays for
// the core tier plus its own addon. Unset (or "all") keeps the historical
// full-stack behaviour. An explicit -ginkgo.label-filter flag overrides the
// filter derived from E2E_ADDONS but not the addon install set.
//
// Running `go test ./test/e2e/` directly against a cluster that lacks the
// operator deployment will fail loudly inside the manager spec's
// BeforeAll.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	selection, err := utils.ParseAddonSelection(os.Getenv(utils.EnvAddons))
	if err != nil {
		t.Fatal(err)
	}
	suiteCfg, reporterCfg := GinkgoConfiguration()
	if suiteCfg.LabelFilter == "" {
		suiteCfg.LabelFilter = selection.LabelFilter()
	}
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting fathom e2e suite (label filter %q)\n",
		suiteCfg.LabelFilter)
	RunSpecs(t, "e2e suite", suiteCfg, reporterCfg)
}

var _ = BeforeSuite(func() {
	selection, err := utils.ParseAddonSelection(os.Getenv(utils.EnvAddons))
	Expect(err).NotTo(HaveOccurred())

	By("syncing the e2e addon stack via helmfile")
	out, err := utils.SyncAddons(selection.HelmfileSelectors()...)
	Expect(err).NotTo(HaveOccurred(), "helmfile sync failed:\n%s", out)
})
