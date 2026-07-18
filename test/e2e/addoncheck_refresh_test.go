/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

// This spec exercises periodic refresh-on-change (SKA-525 / SKA-528): a short-
// interval AddonCheck must flip its result when the underlying addon degrades
// and recover when it heals — all without any edit to the AddonCheck spec, i.e.
// driven purely by the reconciler's interval requeue. It degrades external-
// secrets by scaling its controller Deployment to zero, then restores it.
//
// Serial so the transient scale-to-zero cannot race another spec's assertions;
// Ordered so setup/teardown bracket the single behavioural It.
var _ = Describe("AddonCheck periodic refresh-on-change", Ordered, Serial, func() {
	const (
		checkName    = "addoncheck-refresh-e2e"
		checkNS      = "default"
		targetDeploy = "external-secrets"
		targetNS     = "external-secrets"
	)

	// A dedicated AddonCheck with a short interval so a re-run lands well inside
	// the Eventually windows below (the shipped samples use interval: 5m).
	manifest := `apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: ` + checkName + `
  namespace: ` + checkNS + `
spec:
  addonType: external-secrets
  interval: 15s
  timeout: 10s
  policy:
    system_health:
      enabled: true
`

	var originalReplicas string

	BeforeAll(func() {
		By("recording the external-secrets deployment replica count")
		out, err := utils.Run(exec.Command("kubectl", "get", "deploy", targetDeploy,
			"-n", targetNS, "-o", "jsonpath={.spec.replicas}"))
		Expect(err).NotTo(HaveOccurred(), "failed to read external-secrets replicas")
		originalReplicas = strings.TrimSpace(out)
		if originalReplicas == "" || originalReplicas == "0" {
			originalReplicas = "1"
		}

		By("applying the short-interval refresh AddonCheck")
		apply := exec.Command("kubectl", "apply", "-f", "-")
		apply.Stdin = strings.NewReader(manifest)
		_, err = utils.Run(apply)
		Expect(err).NotTo(HaveOccurred(), "failed to apply refresh AddonCheck")
	})

	AfterAll(func() {
		By("restoring the external-secrets deployment and deleting the refresh AddonCheck")
		_, _ = utils.Run(exec.Command("kubectl", "scale", "deploy", targetDeploy,
			"-n", targetNS, "--replicas="+originalReplicas))
		_, _ = utils.Run(exec.Command("kubectl", "rollout", "status", "deploy", targetDeploy,
			"-n", targetNS, "--timeout=120s"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "addoncheck", checkName,
			"-n", checkNS, "--ignore-not-found=true"))
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			dumpAddonCheckDiagnostics(checkName, checkNS)
		}
	})

	It("refreshes the result when the addon degrades and recovers, with no spec edit", func() {
		By("waiting for the initial healthy (Pass) result")
		Eventually(func(g Gomega) {
			res, err := addonCheckLastResult(checkName, checkNS)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(res).To(Equal("Pass"))
		}, 2*time.Minute, 5*time.Second).Should(Succeed(),
			"refresh AddonCheck never reached Pass initially")

		By("degrading the addon (scale external-secrets to 0) without touching the AddonCheck")
		_, err := utils.Run(exec.Command("kubectl", "scale", "deploy", targetDeploy,
			"-n", targetNS, "--replicas=0"))
		Expect(err).NotTo(HaveOccurred())

		By("expecting a periodic re-run to flip the result off Pass (no spec edit)")
		Eventually(func(g Gomega) {
			res, err := addonCheckLastResult(checkName, checkNS)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(res).NotTo(BeEmpty())
			g.Expect(res).NotTo(Equal("Pass"), "result did not refresh after the addon degraded")
		}, 90*time.Second, 5*time.Second).Should(Succeed(),
			"AddonCheck did not refresh to a non-Pass result after degradation")

		By("restoring the addon and expecting the result to recover to Pass")
		_, err = utils.Run(exec.Command("kubectl", "scale", "deploy", targetDeploy,
			"-n", targetNS, "--replicas="+originalReplicas))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			res, err := addonCheckLastResult(checkName, checkNS)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(res).To(Equal("Pass"), "result did not recover to Pass after the addon was restored")
		}, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"AddonCheck did not recover to Pass")
	})
})
