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
	argocdSamplePath = "config/samples/fathom_v1alpha1_addoncheck_argocd.yaml"
	argocdSampleName = "argocd-sample"
	argocdSampleNS   = "default"
	argocdAddonNS    = "argocd"
)

// argocdSystemHealthDeployments are the Deployments the argocd adapter expects
// in the argocd namespace; the helmfile fixture installs the chart with dex,
// applicationset, and notifications disabled, so exactly these (plus the
// application-controller StatefulSet) exist.
var argocdSystemHealthDeployments = []string{
	"argocd-repo-server",
	"argocd-server",
	"argocd-redis",
}

// argocdSystemHealthCRDs are the CRDs the argocd adapter probes.
var argocdSystemHealthCRDs = []string{
	"applications.argoproj.io",
	"applicationsets.argoproj.io",
	"appprojects.argoproj.io",
}

var _ = Describe("argocd AddonCheck", Ordered, Label("argocd"), func() {
	BeforeAll(func() {
		By("clearing any prior argocd AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", argocdSamplePath,
			"--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the argocd AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", argocdSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply argocd AddonCheck sample")
	})

	AfterAll(func() {
		By("cleaning up the argocd AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", argocdSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(argocdSampleName, argocdSampleNS)
	})

	It("should produce a HealthReport with system_health Pass for the control-plane workloads", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(argocdSampleName, argocdSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			sts := findCheck(report, "system_health", "StatefulSet", "argocd-application-controller")
			stopOnTerminalResult(sts, "system_health", "StatefulSet/argocd-application-controller")
			g.Expect(sts).NotTo(BeNil(),
				"system_health check missing for StatefulSet argocd-application-controller in HealthReport %q",
				report.Metadata.Name)
			g.Expect(sts.Result).To(Equal("Pass"),
				"system_health StatefulSet argocd-application-controller: got %q with summary %q",
				sts.Result, sts.Summary)

			for _, name := range argocdSystemHealthDeployments {
				c := findCheck(report, "system_health", "Deployment", name)
				stopOnTerminalResult(c, "system_health", "Deployment/"+name)
				g.Expect(c).NotTo(BeNil(),
					"system_health check missing for Deployment %s in HealthReport %q",
					name, report.Metadata.Name)
				g.Expect(c.Result).To(Equal("Pass"),
					"system_health Deployment %s: got %q with summary %q",
					name, c.Result, c.Summary)
				g.Expect(c.TargetRef.Namespace).To(Equal(argocdAddonNS),
					"system_health Deployment %s should target the argocd namespace, got %q",
					name, c.TargetRef.Namespace)
			}
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"argocd system_health workload checks did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with system_health Pass for the argocd control plane",
			latestReport.Metadata.Name))
	})

	It("should produce a HealthReport with system_health Pass for the Argo CD CRDs", func() {
		verify := func(g Gomega) {
			report, err := latestHealthReport(argocdSampleName, argocdSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			for _, name := range argocdSystemHealthCRDs {
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
			"argocd system_health CRD checks did not reach Pass within timeout")
	})

	It("should emit Skipped sync_health when no Application resources exist", func() {
		// The helmfile fixture installs Argo CD but declares no Applications,
		// so both sync_health FieldChecks exercise their empty-cluster Skipped
		// contract under their distinct list names.
		verify := func(g Gomega) {
			report, err := latestHealthReport(argocdSampleName, argocdSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			for _, listName := range []string{"applications-sync", "applications-health"} {
				c := findCheck(report, "sync_health", "Application", listName)
				g.Expect(c).NotTo(BeNil(),
					"sync_health empty-cluster Skipped entry %s missing from HealthReport %q",
					listName, report.Metadata.Name)
				g.Expect(c.Result).To(Equal("Skipped"),
					"sync_health %s on empty cluster: got %q with summary %q",
					listName, c.Result, c.Summary)
			}
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("should report Ready=True on the AddonCheck", func() {
		verify := func(g Gomega) {
			ready, err := addonCheckReadyTrue(argocdSampleName, argocdSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch AddonCheck status")
			g.Expect(ready).To(BeTrue(), "AddonCheck %s/%s did not reach Ready=True",
				argocdSampleNS, argocdSampleName)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
