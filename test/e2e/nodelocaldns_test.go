/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
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
	nodeLocalDNSSamplePath = "config/samples/fathom_v1alpha1_addoncheck_node_local_dns.yaml"
	nodeLocalDNSSampleName = "node-local-dns-sample"
	nodeLocalDNSNamespace  = "default"
	// nodeLocalDNSResolutionTarget is the in-cluster Service the sample probes
	// by default. It's hardcoded in the sample manifest, so the e2e check stays
	// in lockstep with what the sample requests.
	nodeLocalDNSResolutionTarget = "kubernetes.default.svc.cluster.local"
	// nodeLocalDNSListenAddress is the cache listen address pinned by both the
	// sample's listenAddress threshold and the helmfile fixture's
	// config.localDns value — the upstream NodeLocal DNSCache convention.
	nodeLocalDNSListenAddress = "169.254.20.10"
)

var _ = Describe("NodeLocal DNSCache AddonCheck", Ordered, Label("node-local-dns"), func() {
	// Delete-then-apply ensures every test run starts from a fresh CR, which
	// triggers an immediate on-create reconcile and clears any stale
	// HealthReports / probe-pod events from previous iterations on a reused
	// cluster. The probe image is kind-loaded by e2e:cluster:fathom under the
	// operator's compiled-in default tag, same as the CoreDNS suite.
	BeforeAll(func() {
		By("clearing any prior NodeLocal DNSCache AddonCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", nodeLocalDNSSamplePath, "--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the NodeLocal DNSCache AddonCheck sample")
		cmd = exec.Command("kubectl", "apply", "-f", nodeLocalDNSSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply NodeLocal DNSCache AddonCheck sample")
	})

	// Removing the AddonCheck owner-garbage-collects the HealthReports (per
	// ownerReferences in the reconciler), so the cleanup is a single delete.
	AfterAll(func() {
		By("cleaning up the NodeLocal DNSCache AddonCheck")
		cmd := exec.Command("kubectl", "delete", "-f", nodeLocalDNSSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	// On failure, surface enough state to diagnose without re-running locally.
	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(nodeLocalDNSSampleName, nodeLocalDNSNamespace)
	})

	It("should report the DaemonSet ready with full per-node coverage", func() {
		verify := func(g Gomega) {
			report, err := latestHealthReport(nodeLocalDNSSampleName, nodeLocalDNSNamespace)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")

			dsCheck := findCheck(report, "system_health", "DaemonSet", "node-local-dns")
			g.Expect(dsCheck).NotTo(BeNil(), "system_health DaemonSet check missing from HealthReport %q", report.Metadata.Name)
			g.Expect(dsCheck.Result).To(Equal("Pass"),
				"DaemonSet check outcome: got %q with summary %q", dsCheck.Result, dsCheck.Summary)

			coverage := findCheck(report, "system_health", "Node", "nodes")
			g.Expect(coverage).NotTo(BeNil(), "per-node coverage check missing from HealthReport %q", report.Metadata.Name)
			g.Expect(coverage.Result).To(Equal("Pass"),
				"node coverage outcome: got %q with summary %q (missingNodes=%q)",
				coverage.Result, coverage.Summary, coverage.Details["missingNodes"])

			schedulable, parseErr := strconv.Atoi(coverage.Details["schedulableNodes"])
			g.Expect(parseErr).NotTo(HaveOccurred(), "schedulableNodes %q is not an integer", coverage.Details["schedulableNodes"])
			g.Expect(schedulable).To(BeNumerically(">=", 1), "coverage check must observe at least one schedulable node")
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"NodeLocal DNSCache system_health did not reach Pass within timeout")
	})

	It("should produce a HealthReport with dns_resolution Pass through the node-local cache", func() {
		var latestReport healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(nodeLocalDNSSampleName, nodeLocalDNSNamespace)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latestReport = report

			dnsCheck := findCheck(report, "dns_resolution", "DNSName", nodeLocalDNSResolutionTarget)
			stopOnTerminalResult(dnsCheck, "dns_resolution", nodeLocalDNSResolutionTarget)
			g.Expect(dnsCheck).NotTo(BeNil(),
				"dns_resolution check for %q missing from HealthReport %q",
				nodeLocalDNSResolutionTarget, report.Metadata.Name)
			g.Expect(dnsCheck.Result).To(Equal("Pass"),
				"dns_resolution outcome: got %q with summary %q",
				dnsCheck.Result, dnsCheck.Summary)

			// The nameserver detail proves the probe was pinned to the cache's
			// listen address rather than the cluster's default resolver path.
			g.Expect(dnsCheck.Details["nameserver"]).To(Equal(nodeLocalDNSListenAddress),
				"probe was not pinned to the node-local cache listen address")

			latency, ok := dnsCheck.Details["latencyMillis"]
			g.Expect(ok).To(BeTrue(), "latencyMillis detail missing")
			g.Expect(latency).NotTo(BeEmpty(), "latencyMillis detail is empty")
			parsed, parseErr := strconv.Atoi(latency)
			g.Expect(parseErr).NotTo(HaveOccurred(), "latencyMillis %q is not an integer", latency)
			g.Expect(parsed).To(BeNumerically(">=", 0), "latencyMillis must be non-negative")
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"NodeLocal DNSCache dns_resolution check did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with dns_resolution Pass for %s via %s",
			latestReport.Metadata.Name, nodeLocalDNSResolutionTarget, nodeLocalDNSListenAddress))
	})

	// Probe pods are ephemeral (the launcher deletes them after the run), so
	// live pods can't be observed reliably. Events outlive the pod in the
	// namespace where it was created — a probe-pod creation event in `default`
	// asserts the pods run in the AddonCheck's namespace (SKA-313 semantics,
	// mirrored from the CoreDNS suite).
	It("should run probe Pods in the AddonCheck's namespace", func() {
		verify := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "events",
				"-n", nodeLocalDNSNamespace,
				"--field-selector", "involvedObject.kind=Pod,reason=Scheduled",
				"-o", "json",
			)
			out, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred(), "failed to list Pod-scheduled events in %s", nodeLocalDNSNamespace)

			var list eventList
			g.Expect(json.Unmarshal([]byte(out), &list)).To(Succeed())

			found := false
			for _, ev := range list.Items {
				if strings.HasPrefix(ev.InvolvedObject.Name, "fathom-nldns-") &&
					ev.InvolvedObject.Namespace == nodeLocalDNSNamespace {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(),
				"no Pod-scheduled event for a fathom-nldns-* probe Pod in %s",
				nodeLocalDNSNamespace)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
