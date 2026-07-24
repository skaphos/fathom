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
	azureWISamplePath = "config/samples/fathom_v1alpha1_addoncheck_azure_workload_identity.yaml"
	azureWISampleName = "azure-workload-identity-sample"
	azureWISampleNS   = "default"
	azureWIAddonNS    = "azure-workload-identity-system"
	azureWIDeployment = "azure-wi-webhook-controller-manager"
	azureWIWebhook    = "azure-wi-webhook-mutating-webhook-configuration"
	azureWIService    = "azure-wi-webhook-webhook-service"
)

var _ = Describe("azure-workload-identity AddonCheck", Ordered, Label("azure-workload-identity"), func() {
	BeforeAll(func() {
		By("clearing any prior azure-workload-identity AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", azureWISamplePath,
			"--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the azure-workload-identity AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", azureWISamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply azure-workload-identity AddonCheck sample")
	})

	AfterAll(func() {
		By("cleaning up the azure-workload-identity AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", azureWISamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(azureWISampleName, azureWISampleNS)
	})

	It("should produce a HealthReport with system_health Pass for the webhook Deployment", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(azureWISampleName, azureWISampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			c := findCheck(report, "system_health", "Deployment", azureWIDeployment)
			stopOnTerminalResult(c, "system_health", "Deployment/"+azureWIDeployment)
			g.Expect(c).NotTo(BeNil(),
				"system_health check missing for Deployment %s in HealthReport %q",
				azureWIDeployment, report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Pass"),
				"system_health Deployment %s: got %q with summary %q",
				azureWIDeployment, c.Result, c.Summary)
			g.Expect(c.TargetRef.Namespace).To(Equal(azureWIAddonNS),
				"system_health Deployment should target %s, got %q",
				azureWIAddonNS, c.TargetRef.Namespace)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"azure-workload-identity system_health Deployment check did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with system_health Pass for the webhook Deployment",
			latestReport.Metadata.Name))
	})

	It("should produce a HealthReport with webhook_wiring Pass for the configuration and its endpoints", func() {
		// The webhook manages its own serving cert and patches the caBundle at
		// startup, so a healthy install passes both the wiring check and the
		// backing-service endpoint-readiness check.
		verify := func(g Gomega) {
			report, err := latestHealthReport(azureWISampleName, azureWISampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			wiring := findCheck(report, "webhook_wiring", "MutatingWebhookConfiguration", azureWIWebhook)
			stopOnTerminalResult(wiring, "webhook_wiring", "MutatingWebhookConfiguration/"+azureWIWebhook)
			g.Expect(wiring).NotTo(BeNil(),
				"webhook_wiring check missing for MutatingWebhookConfiguration %s in HealthReport %q",
				azureWIWebhook, report.Metadata.Name)
			g.Expect(wiring.Result).To(Equal("Pass"),
				"webhook_wiring MutatingWebhookConfiguration %s: got %q with summary %q",
				azureWIWebhook, wiring.Result, wiring.Summary)

			endpoints := findCheck(report, "webhook_wiring", "EndpointSlice", azureWIService)
			stopOnTerminalResult(endpoints, "webhook_wiring", "EndpointSlice/"+azureWIService)
			g.Expect(endpoints).NotTo(BeNil(),
				"webhook_wiring endpoint-readiness check missing for service %s in HealthReport %q",
				azureWIService, report.Metadata.Name)
			g.Expect(endpoints.Result).To(Equal("Pass"),
				"webhook_wiring EndpointSlice %s: got %q with summary %q",
				azureWIService, endpoints.Result, endpoints.Summary)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"azure-workload-identity webhook_wiring checks did not reach Pass within timeout")
	})

	It("should emit Skipped projection_sanity when no pods opt in to the webhook", func() {
		// Nothing in the e2e stack carries the azure.workload.identity/use=true
		// label, so the adapter's empty-cluster contract is Skipped — quiet by
		// design, never a hollow Pass.
		verify := func(g Gomega) {
			report, err := latestHealthReport(azureWISampleName, azureWISampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			c := findCheck(report, "projection_sanity", "Pod", "workload-pods")
			g.Expect(c).NotTo(BeNil(),
				"projection_sanity empty-cluster Skipped entry missing from HealthReport %q",
				report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Skipped"),
				"projection_sanity on a cluster with no opted-in pods: got %q with summary %q",
				c.Result, c.Summary)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("should report Ready=True on the AddonCheck", func() {
		verify := func(g Gomega) {
			ready, err := addonCheckReadyTrue(azureWISampleName, azureWISampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch AddonCheck status")
			g.Expect(ready).To(BeTrue(), "AddonCheck %s/%s did not reach Ready=True",
				azureWISampleNS, azureWISampleName)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
