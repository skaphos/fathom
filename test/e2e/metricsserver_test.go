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
	metricsServerSamplePath = "config/samples/fathom_v1alpha1_addoncheck_metrics_server.yaml"
	metricsServerSampleName = "metrics-server-sample"
	metricsServerSampleNS   = "default"
	metricsServerAddonNS    = "kube-system"

	// metricsServerAPIService is the aggregated resource-metrics APIService the
	// api_availability family asserts Available=True against.
	metricsServerAPIService = "v1beta1.metrics.k8s.io"
)

var _ = Describe("metrics-server AddonCheck", Ordered, func() {
	BeforeAll(func() {
		By("clearing any prior metrics-server AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", metricsServerSamplePath,
			"--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the metrics-server AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", metricsServerSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply metrics-server AddonCheck sample")
	})

	AfterAll(func() {
		By("cleaning up the metrics-server AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", metricsServerSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(metricsServerSampleName, metricsServerSampleNS)
	})

	It("should produce a HealthReport with system_health Pass for the metrics-server Deployment", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(metricsServerSampleName, metricsServerSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			c := findCheck(report, "system_health", "Deployment", "metrics-server")
			stopOnTerminalResult(c, "system_health", "Deployment/metrics-server")
			g.Expect(c).NotTo(BeNil(),
				"system_health check missing for Deployment metrics-server in HealthReport %q",
				report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Pass"),
				"system_health Deployment metrics-server: got %q with summary %q",
				c.Result, c.Summary)
			g.Expect(c.TargetRef.Namespace).To(Equal(metricsServerAddonNS),
				"system_health Deployment should target kube-system, got %q",
				c.TargetRef.Namespace)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"metrics-server system_health Deployment check did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with system_health Pass for the metrics-server Deployment",
			latestReport.Metadata.Name))
	})

	It("should produce a HealthReport with api_availability Pass for the aggregated APIService", func() {
		// The real aggregation check: v1beta1.metrics.k8s.io must exist and
		// report Available=True — this is what breaks (while pods stay Ready)
		// when the aggregated API is TLS-misconfigured or unreachable.
		verify := func(g Gomega) {
			report, err := latestHealthReport(metricsServerSampleName, metricsServerSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			c := findCheck(report, "api_availability", "APIService", metricsServerAPIService)
			stopOnTerminalResult(c, "api_availability", "APIService/"+metricsServerAPIService)
			g.Expect(c).NotTo(BeNil(),
				"api_availability check missing for APIService %s in HealthReport %q",
				metricsServerAPIService, report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Pass"),
				"api_availability APIService %s: got %q with summary %q",
				metricsServerAPIService, c.Result, c.Summary)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"metrics-server api_availability check did not reach Pass within timeout")
	})

	It("should report Ready=True on the AddonCheck", func() {
		verify := func(g Gomega) {
			ready, err := addonCheckReadyTrue(metricsServerSampleName, metricsServerSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch AddonCheck status")
			g.Expect(ready).To(BeTrue(), "AddonCheck %s/%s did not reach Ready=True",
				metricsServerSampleNS, metricsServerSampleName)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
