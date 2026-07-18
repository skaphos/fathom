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
	externalSecretsSamplePath = "config/samples/fathom_v1alpha1_addoncheck_external_secrets.yaml"
	externalSecretsSampleName = "external-secrets-sample"
	externalSecretsSampleNS   = "default"
	externalSecretsAddonNS    = "external-secrets"
)

// externalSecretsSystemHealthDeployments are the Deployments the ESO
// adapter expects in the external-secrets namespace; the helmfile fixture
// enables webhook and cert-controller alongside the main controller.
var externalSecretsSystemHealthDeployments = []string{
	"external-secrets",
	"external-secrets-webhook",
	"external-secrets-cert-controller",
}

// externalSecretsSystemHealthCRDs are the CRDs the ESO adapter probes.
// Generators and PushSecret are intentionally excluded — see comments on
// the adapter's `crds` var.
var externalSecretsSystemHealthCRDs = []string{
	"externalsecrets.external-secrets.io",
	"secretstores.external-secrets.io",
	"clustersecretstores.external-secrets.io",
	"clusterexternalsecrets.external-secrets.io",
}

var _ = Describe("external-secrets AddonCheck", Ordered, func() {
	BeforeAll(func() {
		By("clearing any prior external-secrets AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", externalSecretsSamplePath,
			"--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the external-secrets AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", externalSecretsSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply external-secrets AddonCheck sample")
	})

	AfterAll(func() {
		By("cleaning up the external-secrets AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", externalSecretsSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(externalSecretsSampleName, externalSecretsSampleNS)
	})

	It("should produce a HealthReport with system_health Pass for ESO Deployments", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(externalSecretsSampleName, externalSecretsSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			for _, name := range externalSecretsSystemHealthDeployments {
				c := findCheck(report, "system_health", "Deployment", name)
				stopOnTerminalResult(c, "system_health", "Deployment/"+name)
				g.Expect(c).NotTo(BeNil(),
					"system_health check missing for Deployment %s in HealthReport %q",
					name, report.Metadata.Name)
				g.Expect(c.Result).To(Equal("Pass"),
					"system_health Deployment %s: got %q with summary %q",
					name, c.Result, c.Summary)
				g.Expect(c.TargetRef.Namespace).To(Equal(externalSecretsAddonNS),
					"system_health Deployment %s should target the external-secrets namespace, got %q",
					name, c.TargetRef.Namespace)
			}
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"ESO system_health Deployment checks did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with system_health Pass for required Deployments",
			latestReport.Metadata.Name))
	})

	It("should produce a HealthReport with system_health Pass for required ESO CRDs", func() {
		verify := func(g Gomega) {
			report, err := latestHealthReport(externalSecretsSampleName, externalSecretsSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			for _, name := range externalSecretsSystemHealthCRDs {
				c := findCheck(report, "system_health", "CustomResourceDefinition", name)
				stopOnTerminalResult(c, "system_health", "CRD/"+name)
				g.Expect(c).NotTo(BeNil(),
					"system_health check missing for CRD %s in HealthReport %q",
					name, report.Metadata.Name)
				g.Expect(c.Result).To(Equal("Pass"),
					"system_health CRD %s: got %q with summary %q",
					name, c.Result, c.Summary)
			}
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"ESO system_health CRD checks did not reach Pass within timeout")
	})

	It("should emit Skipped secret_sync when no ExternalSecret resources exist", func() {
		// The helmfile fixture installs ESO but never creates
		// ExternalSecret resources, so the adapter's empty-cluster
		// contract is Skipped — see checkSecretSync in the adapter.
		verify := func(g Gomega) {
			report, err := latestHealthReport(externalSecretsSampleName, externalSecretsSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			c := findCheck(report, "secret_sync", "ExternalSecret", "externalsecrets")
			g.Expect(c).NotTo(BeNil(),
				"secret_sync empty-cluster Skipped entry missing from HealthReport %q",
				report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Skipped"),
				"secret_sync on empty cluster: got %q with summary %q",
				c.Result, c.Summary)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("should report Ready=True on the AddonCheck", func() {
		verify := func(g Gomega) {
			ready, err := addonCheckReadyTrue(externalSecretsSampleName, externalSecretsSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch AddonCheck status")
			g.Expect(ready).To(BeTrue(), "AddonCheck %s/%s did not reach Ready=True",
				externalSecretsSampleNS, externalSecretsSampleName)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
