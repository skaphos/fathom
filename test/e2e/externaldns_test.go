/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

const (
	externalDNSSamplePath = "config/samples/fathom_v1alpha1_addoncheck_external_dns.yaml"
	externalDNSSampleName = "external-dns-sample"
	externalDNSSampleNS   = "default"
	externalDNSAddonNS    = "external-dns"

	// externalDNSEndpointCRD is the opt-in CRD-source CRD. The helmfile fixture
	// installs the chart defaults, which do NOT include it — the crd_health spec
	// below asserts the adapter's Optional-absence contract against that.
	externalDNSEndpointCRD = "dnsendpoints.externaldns.k8s.io"
)

var _ = Describe("external-dns AddonCheck", Ordered, func() {
	BeforeAll(func() {
		By("clearing any prior external-dns AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", externalDNSSamplePath,
			"--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the external-dns AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", externalDNSSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply external-dns AddonCheck sample")
	})

	AfterAll(func() {
		By("cleaning up the external-dns AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", externalDNSSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(externalDNSSampleName, externalDNSSampleNS)
	})

	It("should produce a HealthReport with system_health Pass for the controller Deployment", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(externalDNSSampleName, externalDNSSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			c := findCheck(report, "system_health", "Deployment", "external-dns")
			stopOnTerminalResult(c, "system_health", "Deployment/external-dns")
			g.Expect(c).NotTo(BeNil(),
				"system_health check missing for Deployment external-dns in HealthReport %q",
				report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Pass"),
				"system_health Deployment external-dns: got %q with summary %q",
				c.Result, c.Summary)
			g.Expect(c.TargetRef.Namespace).To(Equal(externalDNSAddonNS),
				"system_health Deployment should target the external-dns namespace, got %q",
				c.TargetRef.Namespace)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"external-dns system_health Deployment check did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with system_health Pass for the controller Deployment",
			latestReport.Metadata.Name))
	})

	It("should emit Skipped crd_health with the absent marker when the DNSEndpoint CRD is not installed", func() {
		// The DNSEndpoint CRD is an opt-in external-dns feature the chart-default
		// fixture never installs, so the definition's component-level Optional
		// posture must score it Skipped — with Details["absent"] so "not
		// installed" stays queryable (SKA-526) — rather than failing the addon.
		verify := func(g Gomega) {
			report, err := latestHealthReport(externalDNSSampleName, externalDNSSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			c := findCheck(report, "crd_health", "CustomResourceDefinition", externalDNSEndpointCRD)
			g.Expect(c).NotTo(BeNil(),
				"crd_health check missing for CRD %s in HealthReport %q",
				externalDNSEndpointCRD, report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Skipped"),
				"crd_health on a CRD-less install: got %q with summary %q",
				c.Result, c.Summary)
			g.Expect(c.Details).To(HaveKeyWithValue("absent", "true"),
				"crd_health Skipped result should carry the absent marker")
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("should report Ready=True on the AddonCheck", func() {
		verify := func(g Gomega) {
			ready, err := addonCheckReadyTrue(externalDNSSampleName, externalDNSSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch AddonCheck status")
			g.Expect(ready).To(BeTrue(), "AddonCheck %s/%s did not reach Ready=True",
				externalDNSSampleNS, externalDNSSampleName)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
