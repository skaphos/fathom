/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"fmt"
	"os/exec"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

const (
	ksmSamplePath = "config/samples/fathom_v1alpha1_addoncheck_kube_state_metrics.yaml"
	ksmSampleName = "kube-state-metrics-sample"
	ksmSampleNS   = "default"
	ksmAddonNS    = "kube-system"

	// ksmMetricsEndpoint / ksmTelemetryEndpoint are the MetricsEndpoint check
	// target names the adapter derives from the sample's serviceName and port
	// thresholds — the fixture chart exposes both ports (selfMonitor enabled).
	ksmMetricsEndpoint   = "kube-state-metrics:8080"
	ksmTelemetryEndpoint = "kube-state-metrics:8081"
)

var _ = Describe("kube-state-metrics AddonCheck", Ordered, Label("kube-state-metrics"), func() {
	BeforeAll(func() {
		By("clearing any prior kube-state-metrics AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", ksmSamplePath,
			"--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the kube-state-metrics AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", ksmSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply kube-state-metrics AddonCheck sample")
	})

	AfterAll(func() {
		By("cleaning up the kube-state-metrics AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", ksmSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(ksmSampleName, ksmSampleNS)
	})

	It("should produce a HealthReport with system_health Pass for the kube-state-metrics Deployment", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(ksmSampleName, ksmSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			c := findCheck(report, "system_health", "Deployment", "kube-state-metrics")
			stopOnTerminalResult(c, "system_health", "Deployment/kube-state-metrics")
			g.Expect(c).NotTo(BeNil(),
				"system_health check missing for Deployment kube-state-metrics in HealthReport %q",
				report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Pass"),
				"system_health Deployment kube-state-metrics: got %q with summary %q",
				c.Result, c.Summary)
			g.Expect(c.TargetRef.Namespace).To(Equal(ksmAddonNS),
				"system_health Deployment should target kube-system, got %q",
				c.TargetRef.Namespace)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"kube-state-metrics system_health Deployment check did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with system_health Pass for the kube-state-metrics Deployment",
			latestReport.Metadata.Name))
	})

	It("should produce a HealthReport with metrics_endpoint Pass for the main /metrics scrape", func() {
		// The real "is KSM actually producing metrics" check: a probe pod
		// scrapes the Service's 8080 /metrics port and asserts the expected
		// kube_* families are present — this is what breaks (while pods stay
		// Ready) when the exporter serves an empty or partial body.
		verify := func(g Gomega) {
			report, err := latestHealthReport(ksmSampleName, ksmSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			c := findCheck(report, "metrics_endpoint", "MetricsEndpoint", ksmMetricsEndpoint)
			stopOnTerminalResult(c, "metrics_endpoint", "MetricsEndpoint/"+ksmMetricsEndpoint)
			g.Expect(c).NotTo(BeNil(),
				"metrics_endpoint check missing for %s in HealthReport %q",
				ksmMetricsEndpoint, report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Pass"),
				"metrics_endpoint %s: got %q with summary %q",
				ksmMetricsEndpoint, c.Result, c.Summary)

			latency, ok := c.Details["latencyMillis"]
			g.Expect(ok).To(BeTrue(), "latencyMillis detail missing")
			parsed, parseErr := strconv.Atoi(latency)
			g.Expect(parseErr).NotTo(HaveOccurred(), "latencyMillis %q is not an integer", latency)
			g.Expect(parsed).To(BeNumerically(">=", 0), "latencyMillis must be non-negative")
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"kube-state-metrics metrics_endpoint scrape did not reach Pass within timeout")
	})

	It("should produce a HealthReport with metrics_endpoint Pass for the self-telemetry scrape", func() {
		// The fixture enables selfMonitor, so the Service exposes the 8081
		// telemetry port and the check must genuinely scrape it (asserting
		// kube_state_metrics_build_info) rather than exercise its Skipped
		// contract.
		verify := func(g Gomega) {
			report, err := latestHealthReport(ksmSampleName, ksmSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			c := findCheck(report, "metrics_endpoint", "MetricsEndpoint", ksmTelemetryEndpoint)
			stopOnTerminalResult(c, "metrics_endpoint", "MetricsEndpoint/"+ksmTelemetryEndpoint)
			g.Expect(c).NotTo(BeNil(),
				"metrics_endpoint check missing for %s in HealthReport %q",
				ksmTelemetryEndpoint, report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Pass"),
				"metrics_endpoint %s: got %q with summary %q",
				ksmTelemetryEndpoint, c.Result, c.Summary)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"kube-state-metrics self-telemetry scrape did not reach Pass within timeout")
	})

	It("should report Ready=True on the AddonCheck", func() {
		verify := func(g Gomega) {
			ready, err := addonCheckReadyTrue(ksmSampleName, ksmSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch AddonCheck status")
			g.Expect(ready).To(BeTrue(), "AddonCheck %s/%s did not reach Ready=True",
				ksmSampleNS, ksmSampleName)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
