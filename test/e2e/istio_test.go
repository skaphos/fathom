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
	istioSamplePath = "config/samples/fathom_v1alpha1_addoncheck_istio.yaml"
	istioSampleName = "istio-sample"
	istioSampleNS   = "default"
	istioAddonNS    = "istio-system"
)

// istioCRDs are the CRDs the adapter's crd_health family asserts; the
// istio/base chart installs them.
var istioCRDs = []string{
	"virtualservices.networking.istio.io",
	"destinationrules.networking.istio.io",
	"gateways.networking.istio.io",
	"serviceentries.networking.istio.io",
	"peerauthentications.security.istio.io",
	"authorizationpolicies.security.istio.io",
}

var _ = Describe("istio AddonCheck", Ordered, func() {
	BeforeAll(func() {
		By("clearing any prior istio AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", istioSamplePath,
			"--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the istio AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", istioSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply istio AddonCheck sample")
	})

	AfterAll(func() {
		By("cleaning up the istio AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", istioSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(istioSampleName, istioSampleNS)
	})

	It("should produce a HealthReport with system_health Pass for istiod and both webhook configurations", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(istioSampleName, istioSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			c := findCheck(report, "system_health", "Deployment", "istiod")
			stopOnTerminalResult(c, "system_health", "Deployment/istiod")
			g.Expect(c).NotTo(BeNil(),
				"system_health check missing for Deployment istiod in HealthReport %q",
				report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Pass"),
				"system_health Deployment istiod: got %q with summary %q",
				c.Result, c.Summary)
			g.Expect(c.TargetRef.Namespace).To(Equal(istioAddonNS),
				"system_health Deployment should target istio-system, got %q",
				c.TargetRef.Namespace)

			// The admission wiring istiod serves: both webhook configurations
			// present, caBundle patched (by istiod at startup), backed by the
			// istiod Service. This is the check that distinguishes "istiod
			// pods Ready" from "injection/validation actually admitted".
			for kind, name := range map[string]string{
				"MutatingWebhookConfiguration":   "istio-sidecar-injector",
				"ValidatingWebhookConfiguration": "istio-validator-istio-system",
			} {
				w := findCheck(report, "system_health", kind, name)
				stopOnTerminalResult(w, "system_health", kind+"/"+name)
				g.Expect(w).NotTo(BeNil(),
					"system_health check missing for %s %s in HealthReport %q",
					kind, name, report.Metadata.Name)
				g.Expect(w.Result).To(Equal("Pass"),
					"system_health %s %s: got %q with summary %q",
					kind, name, w.Result, w.Summary)
			}
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"istio system_health checks did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with system_health Pass for istiod and its webhooks",
			latestReport.Metadata.Name))
	})

	It("should produce a HealthReport with crd_health Pass for the istio CRDs", func() {
		verify := func(g Gomega) {
			report, err := latestHealthReport(istioSampleName, istioSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			for _, name := range istioCRDs {
				c := findCheck(report, "crd_health", "CustomResourceDefinition", name)
				stopOnTerminalResult(c, "crd_health", "CRD/"+name)
				g.Expect(c).NotTo(BeNil(),
					"crd_health check missing for CRD %s in HealthReport %q",
					name, report.Metadata.Name)
				g.Expect(c.Result).To(Equal("Pass"),
					"crd_health CRD %s: got %q with summary %q",
					name, c.Result, c.Summary)
			}
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"istio crd_health CRD checks did not reach Pass within timeout")
	})

	It("should emit Skipped with the absent marker for the ambient families on a sidecar-mode install", func() {
		// The fixture installs base + istiod only, so ztunnel and
		// istio-cni-node are genuinely absent — the Optional-absence contract
		// (SKA-526) asserted against a real cluster rather than a fake client.
		verify := func(g Gomega) {
			report, err := latestHealthReport(istioSampleName, istioSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			for family, name := range map[string]string{
				"ztunnel_health":   "ztunnel",
				"istio_cni_health": "istio-cni-node",
			} {
				c := findCheck(report, family, "DaemonSet", name)
				stopOnTerminalResult(c, family, "DaemonSet/"+name)
				g.Expect(c).NotTo(BeNil(),
					"%s check missing for DaemonSet %s in HealthReport %q",
					family, name, report.Metadata.Name)
				g.Expect(c.Result).To(Equal("Skipped"),
					"%s DaemonSet %s on a sidecar-mode install: got %q with summary %q",
					family, name, c.Result, c.Summary)
				g.Expect(c.Details["absent"]).To(Equal("true"),
					"%s DaemonSet %s should carry the absent marker", family, name)
			}
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("should report Ready=True on the AddonCheck", func() {
		verify := func(g Gomega) {
			ready, err := addonCheckReadyTrue(istioSampleName, istioSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch AddonCheck status")
			g.Expect(ready).To(BeTrue(), "AddonCheck %s/%s did not reach Ready=True",
				istioSampleNS, istioSampleName)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
