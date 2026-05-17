/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

const (
	corednsSamplePath = "config/samples/fathom_v1alpha1_addoncheck_coredns.yaml"
	corednsSampleName = "coredns-sample"
	corednsNamespace  = "default"
	// dnsResolutionTarget is the well-known in-cluster Service the CoreDNS
	// sample probes by default. It's hardcoded in the sample manifest, so
	// the e2e check stays in lockstep with what the sample requests.
	dnsResolutionTarget = "kubernetes.default.svc.cluster.local"
)

var _ = Describe("CoreDNS AddonCheck", Ordered, func() {
	// The Taskfile's e2e:cluster:fathom step builds and loads the probe image
	// under the canonical tag the operator's DefaultProbeImage references,
	// so probe Pods resolve to the kind-loaded image via IfNotPresent and
	// do not need to pull from GHCR. Delete-then-apply ensures every test
	// run starts from a fresh CR, which triggers an immediate on-create
	// reconcile and clears any stale HealthReports / probe-pod events
	// from previous iterations on a reused cluster.
	BeforeAll(func() {
		By("clearing any prior CoreDNS AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", corednsSamplePath, "--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the CoreDNS AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", corednsSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply CoreDNS AddonCheck sample")
	})

	// Removing the AddonCheck owner-garbage-collects the HealthReports
	// (per ownerReferences in the reconciler), so the cleanup is a single
	// kubectl delete on the CR itself.
	AfterAll(func() {
		By("cleaning up the CoreDNS AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", corednsSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	// On failure, surface enough state to diagnose without re-running locally.
	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(corednsSampleName, corednsNamespace)
	})

	It("should produce a HealthReport with dns_resolution Pass and latencyMillis", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(corednsSampleName, corednsNamespace)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			dnsCheck := findCheck(report, "dns_resolution", "DNSName", dnsResolutionTarget)
			stopOnTerminalResult(dnsCheck, "dns_resolution", dnsResolutionTarget)
			g.Expect(dnsCheck).NotTo(BeNil(),
				"dns_resolution check for %q missing from HealthReport %q",
				dnsResolutionTarget, report.Metadata.Name)
			g.Expect(dnsCheck.Result).To(Equal("Pass"),
				"dns_resolution outcome: got %q with summary %q",
				dnsCheck.Result, dnsCheck.Summary)

			latency, ok := dnsCheck.Details["latencyMillis"]
			g.Expect(ok).To(BeTrue(), "latencyMillis detail missing")
			g.Expect(latency).NotTo(BeEmpty(), "latencyMillis detail is empty")
			parsed, parseErr := strconv.Atoi(latency)
			g.Expect(parseErr).NotTo(HaveOccurred(), "latencyMillis %q is not an integer", latency)
			g.Expect(parsed).To(BeNumerically(">=", 0), "latencyMillis must be non-negative")
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"CoreDNS dns_resolution check did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with dns_resolution Pass for %s",
			latestReport.Metadata.Name, dnsResolutionTarget))
	})

	// SKA-313 hardened probe-pod namespace defaulting so probe Pods land in
	// the AddonCheck's namespace rather than a hardcoded one. Probe Pods
	// are ephemeral (the launcher deletes them after the run), so we can't
	// reliably observe live pods after the reconcile. Events persist
	// beyond Pod deletion in the namespace where the Pod was created —
	// asserting a probe-pod creation event in `default` is the regression
	// coverage we need without racing the Pod's lifetime.
	It("should run probe Pods in the AddonCheck's namespace", func() {
		verify := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "events",
				"-n", corednsNamespace,
				"--field-selector", "involvedObject.kind=Pod,reason=Scheduled",
				"-o", "json",
			)
			out, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred(), "failed to list Pod-scheduled events in %s", corednsNamespace)

			var list eventList
			g.Expect(json.Unmarshal([]byte(out), &list)).To(Succeed())

			found := false
			for _, ev := range list.Items {
				if strings.HasPrefix(ev.InvolvedObject.Name, "fathom-dns-") &&
					ev.InvolvedObject.Namespace == corednsNamespace {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(),
				"no Pod-scheduled event for a fathom-dns-* probe Pod in %s; SKA-313 namespace defaulting may have regressed",
				corednsNamespace)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
