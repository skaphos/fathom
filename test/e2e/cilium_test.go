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
	ciliumSamplePath = "config/samples/fathom_v1alpha1_addoncheck_cilium.yaml"
	ciliumSampleName = "cilium-sample"
	ciliumSampleNS   = "default"
	ciliumAddonNS    = "kube-system"

	ciliumOperatorDeployment = "cilium-operator"
	ciliumAgentDaemonSet     = "cilium"
)

// ciliumCRDChecks are the core Cilium CRDs the crd_health family probes; they
// mirror the adapter's ciliumCRDs list and the helmfile-installed Cilium CRDs.
var ciliumCRDChecks = []string{
	"ciliumnetworkpolicies.cilium.io",
	"ciliumclusterwidenetworkpolicies.cilium.io",
	"ciliumendpoints.cilium.io",
	"ciliumidentities.cilium.io",
	"ciliumnodes.cilium.io",
}

var _ = Describe("cilium AddonCheck", Ordered, func() {
	BeforeAll(func() {
		By("clearing any prior cilium AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", ciliumSamplePath,
			"--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the cilium AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", ciliumSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply cilium AddonCheck sample")
	})

	AfterAll(func() {
		By("cleaning up the cilium AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", ciliumSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(ciliumSampleName, ciliumSampleNS)
	})

	It("should produce a HealthReport with control_plane_health Pass for the cilium-operator Deployment", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(ciliumSampleName, ciliumSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			c := findCheck(report, "control_plane_health", "Deployment", ciliumOperatorDeployment)
			stopOnTerminalResult(c, "control_plane_health", "Deployment/"+ciliumOperatorDeployment)
			g.Expect(c).NotTo(BeNil(),
				"control_plane_health check missing for Deployment %s in HealthReport %q",
				ciliumOperatorDeployment, report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Pass"),
				"control_plane_health Deployment %s: got %q with summary %q",
				ciliumOperatorDeployment, c.Result, c.Summary)
			g.Expect(c.TargetRef.Namespace).To(Equal(ciliumAddonNS),
				"control_plane_health Deployment %s should target the kube-system namespace, got %q",
				ciliumOperatorDeployment, c.TargetRef.Namespace)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"cilium control_plane_health check did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with control_plane_health Pass for cilium-operator",
			latestReport.Metadata.Name))
	})

	It("should produce a HealthReport with agent_health Pass for the cilium agent DaemonSet", func() {
		verify := func(g Gomega) {
			report, err := latestHealthReport(ciliumSampleName, ciliumSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			c := findCheck(report, "agent_health", "DaemonSet", ciliumAgentDaemonSet)
			stopOnTerminalResult(c, "agent_health", "DaemonSet/"+ciliumAgentDaemonSet)
			g.Expect(c).NotTo(BeNil(),
				"agent_health check missing for DaemonSet %s in HealthReport %q",
				ciliumAgentDaemonSet, report.Metadata.Name)
			g.Expect(c.Result).To(Equal("Pass"),
				"agent_health DaemonSet %s: got %q with summary %q",
				ciliumAgentDaemonSet, c.Result, c.Summary)
			g.Expect(c.TargetRef.Namespace).To(Equal(ciliumAddonNS),
				"agent_health DaemonSet %s should target the kube-system namespace, got %q",
				ciliumAgentDaemonSet, c.TargetRef.Namespace)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"cilium agent_health check did not reach Pass within timeout")
	})

	It("should produce a HealthReport with crd_health Pass for the core Cilium CRDs", func() {
		verify := func(g Gomega) {
			report, err := latestHealthReport(ciliumSampleName, ciliumSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			for _, name := range ciliumCRDChecks {
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
			"cilium crd_health checks did not reach Pass within timeout")
	})

	It("should report Ready=True on the AddonCheck", func() {
		verify := func(g Gomega) {
			ready, err := addonCheckReadyTrue(ciliumSampleName, ciliumSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch AddonCheck status")
			g.Expect(ready).To(BeTrue(), "AddonCheck %s/%s did not reach Ready=True",
				ciliumSampleNS, ciliumSampleName)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
