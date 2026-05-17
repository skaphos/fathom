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
	certManagerSamplePath = "config/samples/fathom_v1alpha1_addoncheck.yaml"
	certManagerSampleName = "addoncheck-sample"
	certManagerSampleNS   = "default"
	certManagerAddonNS    = "cert-manager"
)

// certManagerSystemHealthPasses lists the targets the cert-manager
// system_health family must report Pass on against a vanilla helmfile
// install. ValidatingWebhookConfiguration is cluster-scoped, so its
// expected namespace is empty. The cert-manager adapter also emits
// Pod-level checks per Deployment and an optional webhook-probe
// Certificate when policy enables webhookProbe — those are not
// enumerated here because the live Pod and Certificate names are
// non-deterministic.
var certManagerSystemHealthPasses = []struct {
	kind      string
	name      string
	namespace string
}{
	{"Deployment", "cert-manager", certManagerAddonNS},
	{"Deployment", "cert-manager-webhook", certManagerAddonNS},
	{"Deployment", "cert-manager-cainjector", certManagerAddonNS},
	{"Service", "cert-manager-webhook", certManagerAddonNS},
	{"ValidatingWebhookConfiguration", "cert-manager-webhook", ""},
}

var _ = Describe("cert-manager AddonCheck", Ordered, func() {
	// Delete-then-apply ensures every run starts from a fresh CR, which
	// triggers an immediate on-create reconcile and clears stale
	// HealthReports from previous iterations on a reused cluster.
	BeforeAll(func() {
		By("clearing any prior cert-manager AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", certManagerSamplePath,
			"--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the cert-manager AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", certManagerSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply cert-manager AddonCheck sample")
	})

	AfterAll(func() {
		By("cleaning up the cert-manager AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", certManagerSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(certManagerSampleName, certManagerSampleNS)
	})

	It("should produce a HealthReport with system_health Pass for required components", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(certManagerSampleName, certManagerSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			for _, want := range certManagerSystemHealthPasses {
				c := findCheck(report, "system_health", want.kind, want.name)
				stopOnTerminalResult(c, "system_health", fmt.Sprintf("%s/%s", want.kind, want.name))
				g.Expect(c).NotTo(BeNil(),
					"system_health check missing for %s/%s in HealthReport %q",
					want.kind, want.name, report.Metadata.Name)
				g.Expect(c.Result).To(Equal("Pass"),
					"system_health check for %s/%s: got %q with summary %q",
					want.kind, want.name, c.Result, c.Summary)
				g.Expect(c.TargetRef.Namespace).To(Equal(want.namespace),
					"system_health check for %s/%s: expected namespace %q, got %q",
					want.kind, want.name, want.namespace, c.TargetRef.Namespace)
			}
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"cert-manager system_health checks did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with system_health Pass for required components",
			latestReport.Metadata.Name))
	})

	It("should emit Skipped entries for empty Issuer and Certificate lists", func() {
		// The helmfile fixture installs cert-manager but never creates
		// Issuers, ClusterIssuers, or Certificates, so the adapter's
		// empty-cluster contract is Skipped.
		//
		// Caveat: certmanager/adapter.go's `skipped()` helper hardcodes
		// Family=system_health for every skipped entry, including the
		// ones emitted by checkIssuers and checkCertificates. So the
		// empty-cluster Skipped entries appear under family
		// "system_health" rather than "issuer_health" / "certificate_health".
		// This test asserts the actual emission contract; if the adapter
		// is fixed to tag families correctly, update these assertions
		// alongside the adapter change.
		verify := func(g Gomega) {
			report, err := latestHealthReport(certManagerSampleName, certManagerSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			issuer := findCheck(report, "system_health", "Issuer", "issuers")
			g.Expect(issuer).NotTo(BeNil(),
				"empty-cluster Skipped entry for Issuer/issuers missing from HealthReport %q",
				report.Metadata.Name)
			g.Expect(issuer.Result).To(Equal("Skipped"),
				"Issuer/issuers on empty cluster: got %q with summary %q",
				issuer.Result, issuer.Summary)

			cert := findCheck(report, "system_health", "Certificate", "certificates")
			g.Expect(cert).NotTo(BeNil(),
				"empty-cluster Skipped entry for Certificate/certificates missing from HealthReport %q",
				report.Metadata.Name)
			g.Expect(cert.Result).To(Equal("Skipped"),
				"Certificate/certificates on empty cluster: got %q with summary %q",
				cert.Result, cert.Summary)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("should report Ready=True on the AddonCheck", func() {
		verify := func(g Gomega) {
			ready, err := addonCheckReadyTrue(certManagerSampleName, certManagerSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch AddonCheck status")
			g.Expect(ready).To(BeTrue(), "AddonCheck %s/%s did not reach Ready=True",
				certManagerSampleNS, certManagerSampleName)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
