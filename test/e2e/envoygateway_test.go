/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
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
	envoyGatewaySamplePath = "config/samples/fathom_v1alpha1_addoncheck_envoy_gateway.yaml"
	envoyGatewaySampleName = "envoy-gateway-sample"
	envoyGatewaySampleNS   = "default"
	envoyGatewayAddonNS    = "envoy-gateway-system"
)

// envoyGatewayCRDs are the CRDs the adapter's crd_health family asserts; the
// gateway-helm chart bundles the Gateway API core CRDs alongside Envoy
// Gateway's own EnvoyProxy CRD.
var envoyGatewayCRDs = []string{
	"gatewayclasses.gateway.networking.k8s.io",
	"gateways.gateway.networking.k8s.io",
	"httproutes.gateway.networking.k8s.io",
	"envoyproxies.gateway.envoyproxy.io",
}

var _ = Describe("envoy-gateway AddonCheck", Ordered, func() {
	BeforeAll(func() {
		By("clearing any prior envoy-gateway AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", envoyGatewaySamplePath,
			"--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the envoy-gateway AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", envoyGatewaySamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply envoy-gateway AddonCheck sample")
	})

	AfterAll(func() {
		By("cleaning up the envoy-gateway AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", envoyGatewaySamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(envoyGatewaySampleName, envoyGatewaySampleNS)
	})

	It("should produce a HealthReport with system_health Pass for the controller Deployment", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(envoyGatewaySampleName, envoyGatewaySampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			c := findCheck(report, "system_health", "Deployment", "envoy-gateway")
			stopOnTerminalResult(c, "system_health", "Deployment/envoy-gateway")
			g.Expect(c).NotTo(BeNil(),
				"system_health check missing for Deployment envoy-gateway in HealthReport %q",
				report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Pass"),
				"system_health Deployment envoy-gateway: got %q with summary %q",
				c.Result, c.Summary)
			g.Expect(c.TargetRef.Namespace).To(Equal(envoyGatewayAddonNS),
				"system_health Deployment should target envoy-gateway-system, got %q",
				c.TargetRef.Namespace)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"envoy-gateway system_health Deployment check did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with system_health Pass for the controller Deployment",
			latestReport.Metadata.Name))
	})

	It("should produce a HealthReport with crd_health Pass for the Gateway API and Envoy Gateway CRDs", func() {
		verify := func(g Gomega) {
			report, err := latestHealthReport(envoyGatewaySampleName, envoyGatewaySampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			for _, name := range envoyGatewayCRDs {
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
			"envoy-gateway crd_health CRD checks did not reach Pass within timeout")
	})

	It("should emit Skipped gateway_status when no Gateway objects exist", func() {
		// The fixture installs the controller but never declares a
		// GatewayClass/Gateway (a provisioned Gateway could not reach
		// Programmed=True on kind anyway — no LoadBalancer addresses), so the
		// adapter's empty-cluster contract is Skipped. The family carries two
		// ConditionChecks (Accepted, Programmed) over the same Gateway/gateways
		// target, so both rows are asserted explicitly by conditionType rather
		// than through findCheck, which would only ever see whichever comes
		// first.
		verify := func(g Gomega) {
			report, err := latestHealthReport(envoyGatewaySampleName, envoyGatewaySampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			for _, conditionType := range []string{"Accepted", "Programmed"} {
				c := findCheckByDetail(report, "gateway_status", "Gateway", "gateways", "conditionType", conditionType)
				g.Expect(c).NotTo(BeNil(),
					"gateway_status empty-cluster Skipped entry for conditionType %s missing from HealthReport %q",
					conditionType, report.Metadata.Name)
				g.Expect(c.Result).To(Equal("Skipped"),
					"gateway_status on empty cluster (conditionType %s): got %q with summary %q",
					conditionType, c.Result, c.Summary)
			}
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("should report Ready=True on the AddonCheck", func() {
		verify := func(g Gomega) {
			ready, err := addonCheckReadyTrue(envoyGatewaySampleName, envoyGatewaySampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch AddonCheck status")
			g.Expect(ready).To(BeTrue(), "AddonCheck %s/%s did not reach Ready=True",
				envoyGatewaySampleNS, envoyGatewaySampleName)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
