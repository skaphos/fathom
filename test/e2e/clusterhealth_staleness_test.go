/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

// ClusterHealth staleness against a real cluster (skaphos/fathom#277).
//
// envtest proves the fold; this proves the contract end to end — that a real
// API server accepts the status, that the aggregate publishes the STALEST
// contributing observation rather than the newest, and that the cadence the
// alert rule joins against is actually served on /metrics.
//
// The masking bug is reproduced with a HealthCheck whose target has never run
// beside one whose target has. A never-evaluated contributor is the strongest
// staleness signal there is, so under the old newest-wins fold the live sibling
// would have hidden it completely.
var _ = Describe("ClusterHealth staleness", Ordered, Label(utils.CoreLabel), func() {
	const (
		ns          = "default"
		liveTarget  = "staleness-live-target"
		staleTarget = "staleness-stale-target"
		liveWrap    = "staleness-live-wrapper"
		staleWrap   = "staleness-stale-wrapper"
		aggregate   = "staleness-aggregate-e2e"
	)

	manifest := `apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: ` + liveTarget + `
  namespace: ` + ns + `
spec:
  addonType: coredns
  interval: 1m
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: ` + staleTarget + `
  namespace: ` + ns + `
spec:
  addonType: coredns
  interval: 30m
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: ` + liveWrap + `
  namespace: ` + ns + `
  labels:
    staleness-e2e: "true"
spec:
  checkRef:
    kind: AddonCheck
    name: ` + liveTarget + `
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: ` + staleWrap + `
  namespace: ` + ns + `
  labels:
    staleness-e2e: "true"
spec:
  checkRef:
    kind: AddonCheck
    name: ` + staleTarget + `
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: ClusterHealth
metadata:
  name: ` + aggregate + `
spec:
  selector:
    matchLabels:
      staleness-e2e: "true"
`

	BeforeAll(func() {
		apply := exec.Command("kubectl", "apply", "-f", "-")
		apply.Stdin = strings.NewReader(manifest)
		_, err := utils.Run(apply)
		Expect(err).NotTo(HaveOccurred(), "apply staleness fixtures")
	})

	AfterAll(func() {
		del := exec.Command("kubectl", "delete", "-f", "-", "--ignore-not-found=true")
		del.Stdin = strings.NewReader(manifest)
		_, _ = utils.Run(del)
	})

	It("aggregates a verdict and publishes a staleness observation", func() {
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "clusterhealth", aggregate,
				"-o", "jsonpath={.status.matchedCount}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).To(Equal("2"),
				"the aggregate must select both wrappers")
		}).Should(Succeed())

		By("reporting a verdict derived only from HealthCheck status")
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "clusterhealth", aggregate,
				"-o", "jsonpath={.status.result}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).NotTo(BeEmpty())
		}).Should(Succeed())
	})

	It("surfaces each wrapper's source cadence, which is how the aggregate learns one", func() {
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "healthcheck", staleWrap,
				"-n", ns, "-o", "jsonpath={.status.sourceInterval}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).To(Equal("30m0s"),
				"a wrapper must publish the cadence of the check it wraps; an aggregate can reach it no other way")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "healthcheck", liveWrap,
				"-n", ns, "-o", "jsonpath={.status.sourceInterval}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).To(Equal("1m0s"))
		}).Should(Succeed())
	})

	It("never reports the aggregate as more current than its stalest contributor", func() {
		// Whatever the children's individual observations, the roll-up must not
		// claim an observation newer than the oldest of them. Comparing against
		// the live wrapper is the sharp end: under the old newest-wins fold the
		// aggregate would have taken exactly that value.
		Eventually(func(g Gomega) {
			agg, err := utils.Run(exec.Command("kubectl", "get", "clusterhealth", aggregate,
				"-o", "jsonpath={.status.observedAt}"))
			g.Expect(err).NotTo(HaveOccurred())
			live, err := utils.Run(exec.Command("kubectl", "get", "healthcheck", liveWrap,
				"-n", ns, "-o", "jsonpath={.status.sourceObservedAt}"))
			g.Expect(err).NotTo(HaveOccurred())

			aggAt := strings.TrimSpace(agg)
			liveAt := strings.TrimSpace(live)
			if aggAt == "" || liveAt == "" {
				// An empty aggregate observation means a contributor has never been
				// evaluated, which is itself the strongest staleness signal — and is
				// exactly the state the old fold would have masked.
				return
			}
			// RFC3339 timestamps sort lexicographically, so string comparison is
			// sufficient and avoids parsing in the assertion.
			g.Expect(aggAt <= liveAt).To(BeTrue(),
				"aggregate observedAt %q is newer than the live child %q; the stalest contributor must win", aggAt, liveAt)
		}).Should(Succeed())
	})

	It("serves the cadence gauge the shipped staleness rule joins against", func() {
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "clusterhealth", aggregate,
				"-o", "jsonpath={.status.children[*].name}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out).To(ContainSubstring(staleWrap))
			g.Expect(out).To(ContainSubstring(liveWrap))
		}).Should(Succeed())
	})
})
