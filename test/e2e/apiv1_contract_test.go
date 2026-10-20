/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

// Live-cluster proof of the stable v1 contract
// (specs/014-api-v1-promotion/contracts/api-v1.md): the shipped CRDs serve v1
// alone, the admission rules v1 added hold against a real API server through
// the exact surface a user's kubectl hits, and the aggregate still derives
// only from HealthCheck status. The envtest matrix in
// internal/controller/crd_v1_contract_test.go covers the boundary tables.
var _ = Describe("v1 API contract", Ordered, Label(utils.CoreLabel), func() {
	const ns = "apiv1-contract-e2e"

	kubectl := func(stdin string, args ...string) (string, error) {
		cmd := exec.Command("kubectl", args...)
		if stdin != "" {
			cmd.Stdin = strings.NewReader(stdin)
		}
		return utils.Run(cmd)
	}
	dryRun := func(manifest string, extra ...string) (string, error) {
		return kubectl(manifest, append([]string{"apply", "--dry-run=server", "-f", "-"}, extra...)...)
	}

	BeforeAll(func() {
		_, _ = kubectl("", "create", "namespace", ns)
	})
	AfterAll(func() {
		_, _ = kubectl("", "delete", "namespace", ns, "--ignore-not-found", "--wait=false")
	})

	It("no longer serves the alpha version of a promoted kind", func() {
		out, err := dryRun(`apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: alpha-rejected
  namespace: ` + ns + `
spec:
  addonType: coredns
`)
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring(`no matches for kind "AddonCheck" in version "fathom.skaphos.io/v1alpha1"`))
	})

	It("requires spec at the top level", func() {
		out, err := dryRun(`apiVersion: fathom.skaphos.io/v1
kind: AddonCheck
metadata:
  name: no-spec
  namespace: ` + ns + `
`)
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("spec: Required value"))
	})

	It("refuses the remove-then-re-add bypass of checkRef immutability", func() {
		_, err := kubectl(`apiVersion: fathom.skaphos.io/v1
kind: HealthCheck
metadata:
  name: bypass
  namespace: `+ns+`
spec:
  checkRef:
    kind: AddonCheck
    name: original
`, "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred())

		// Step one of the bypass: replace the object with one that has no spec.
		out, err := kubectl(`apiVersion: fathom.skaphos.io/v1
kind: HealthCheck
metadata:
  name: bypass
  namespace: `+ns+`
`, "replace", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("spec: Required value"))

		got, err := kubectl("", "get", "healthcheck", "bypass", "-n", ns, "-o", "jsonpath={.spec.checkRef.name}")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(got)).To(Equal("original"))
	})

	It("rejects a checkRef.apiVersion that is not fathom.skaphos.io/v1", func() {
		for _, apiVersion := range []string{"example.com/v1", "fathom.skaphos.io/v1alpha1"} {
			out, err := dryRun(fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1
kind: HealthCheck
metadata:
  name: foreign-ref
  namespace: %s
spec:
  checkRef:
    apiVersion: %s
    kind: AddonCheck
    name: target
`, ns, apiVersion))
			Expect(err).To(HaveOccurred(), apiVersion)
			Expect(out).To(ContainSubstring(`supported values: "fathom.skaphos.io/v1"`), apiVersion)
		}
	})

	It("keeps AddonCheck conditions from two server-side-apply managers", func() {
		_, err := kubectl(`apiVersion: fathom.skaphos.io/v1
kind: AddonCheck
metadata:
  name: ssa
  namespace: `+ns+`
spec:
  addonType: never-runs
`, "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred())
		for _, m := range []string{"manager-a", "manager-b"} {
			_, err := kubectl(fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1
kind: AddonCheck
metadata:
  name: ssa
  namespace: %s
status:
  conditions:
  - type: %s
    status: "True"
    reason: Test
    message: %s
    lastTransitionTime: "2026-01-01T00:00:00Z"
`, ns, strings.ToUpper(m[:1])+m[1:], m), "apply", "--server-side", "--subresource=status", "--field-manager="+m, "-f", "-")
			Expect(err).NotTo(HaveOccurred(), m)
		}
		got, err := kubectl("", "get", "addoncheck", "ssa", "-n", ns, "-o", "jsonpath={.status.conditions[*].type}")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(ContainSubstring("Manager-a"))
		Expect(got).To(ContainSubstring("Manager-b"))
	})

	It("shows a verdict and age in the default columns of AddonCheck and HealthReport", func() {
		out, err := kubectl("", "get", "addonchecks", "-n", ns)
		Expect(err).NotTo(HaveOccurred())
		header := strings.ToUpper(strings.SplitN(out, "\n", 2)[0])
		for _, col := range []string{"RESULT", "ADDON", "LAST RUN", "AGE"} {
			Expect(header).To(ContainSubstring(col))
		}
		out, err = kubectl("", "get", "healthreports", "-n", ns, "--ignore-not-found")
		Expect(err).NotTo(HaveOccurred())
		if out == "" {
			// kubectl prints no header for an empty list; the column contract is
			// proven on the CRD's printer columns instead.
			cols, err := kubectl("", "get", "crd", "healthreports.fathom.skaphos.io", "-o",
				"jsonpath={.spec.versions[0].additionalPrinterColumns[*].name}")
			Expect(err).NotTo(HaveOccurred())
			out = cols
		}
		header = strings.ToUpper(strings.SplitN(out, "\n", 2)[0])
		for _, col := range []string{"RESULT", "SOURCE", "NAME", "OBSERVED", "AGE"} {
			Expect(header).To(ContainSubstring(col))
		}
	})

	It("rejects spec.paused under strict field validation and prunes it otherwise", func() {
		manifest := `apiVersion: fathom.skaphos.io/v1
kind: NodeCertificateCheck
metadata:
  name: paused-field
  namespace: ` + ns + `
spec:
  paused: true
`
		out, err := dryRun(manifest, "--validate=strict")
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring(`unknown field "spec.paused"`))

		_, err = kubectl(manifest, "apply", "--validate=false", "-f", "-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, _ = kubectl("", "delete", "nodecertificatecheck", "paused-field", "-n", ns, "--ignore-not-found")
		})
		got, err := kubectl("", "get", "nodecertificatecheck", "paused-field", "-n", ns, "-o", "jsonpath={.spec.paused}")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(got)).To(BeEmpty(), "the server prunes the retired field")
	})

	It("derives ClusterHealth only from HealthCheck status, never from HealthReports", func() {
		_, err := kubectl(`apiVersion: fathom.skaphos.io/v1
kind: AddonCheck
metadata:
  name: source
  namespace: `+ns+`
spec:
  addonType: coredns
  interval: 1m
---
apiVersion: fathom.skaphos.io/v1
kind: HealthCheck
metadata:
  name: wrapper
  namespace: `+ns+`
spec:
  checkRef:
    kind: AddonCheck
    name: source
---
apiVersion: fathom.skaphos.io/v1
kind: ClusterHealth
metadata:
  name: apiv1-contract-aggregate
spec:
  namespaces: [`+ns+`]
`, "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, _ = kubectl("", "delete", "clusterhealth", "apiv1-contract-aggregate", "--ignore-not-found")
		})

		var before string
		Eventually(func(g Gomega) {
			out, err := kubectl("", "get", "clusterhealth", "apiv1-contract-aggregate", "-o", "jsonpath={.status.result}")
			g.Expect(err).NotTo(HaveOccurred())
			before = strings.TrimSpace(out)
			g.Expect(before).NotTo(BeEmpty())
			g.Expect(before).NotTo(Equal("Fail"))
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		// A report nobody's status refers to must not move the aggregate.
		_, err = kubectl(`apiVersion: fathom.skaphos.io/v1
kind: HealthReport
metadata:
  name: forged-report
  namespace: `+ns+`
  labels:
    fathom.skaphos.io/source-kind: AddonCheck
    fathom.skaphos.io/source-name: source
spec:
  sourceRef:
    apiVersion: fathom.skaphos.io/v1
    kind: AddonCheck
    namespace: `+ns+`
    name: source
  result: Fail
  observedAt: "2026-01-01T00:00:00Z"
`, "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred())
		Consistently(func(g Gomega) {
			out, err := kubectl("", "get", "clusterhealth", "apiv1-contract-aggregate", "-o", "jsonpath={.status.result}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).To(Equal(before))
		}, 30*time.Second, 5*time.Second).Should(Succeed())
	})
})
