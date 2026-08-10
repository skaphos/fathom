/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

// dnsResolutionNamespace is deliberately distinct from the lifecycle suite's
// namespace. Sharing one meant that suite's AfterAll deletion (--wait=false)
// could still be terminating when this suite's BeforeAll ran, and creating a
// resource in a terminating namespace is Forbidden.
const dnsResolutionNamespace = "dnscheck-resolution-e2e"

// ensureNamespaceActive creates the namespace and waits for it to be usable,
// tolerating a previous run's copy still terminating on a reused cluster.
func ensureNamespaceActive(name string) {
	Eventually(func(g Gomega) {
		_, _ = utils.Run(exec.Command("kubectl", "create", "namespace", name))
		phase, err := utils.Run(exec.Command("kubectl", "get", "namespace", name,
			"-o", "jsonpath={.status.phase}"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(phase)).To(Equal("Active"))
	}, 2*time.Minute, 3*time.Second).Should(Succeed())
}

// applyDNSCheck writes a DNSCheck manifest, applies it, and schedules removal.
func applyDNSCheck(name, spec string) {
	manifest := fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: DNSCheck
metadata:
  name: %s
  namespace: %s
spec:
%s`, name, dnsResolutionNamespace, spec)
	path := filepath.Join(os.TempDir(), "fathom-e2e-"+name+".yaml")
	Expect(os.WriteFile(path, []byte(manifest), 0o600)).To(Succeed())
	DeferCleanup(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "-f", path, "--ignore-not-found=true"))
		_ = os.Remove(path)
	})
	_, err := utils.Run(exec.Command("kubectl", "apply", "-f", path))
	Expect(err).NotTo(HaveOccurred())
}

// dnsCheckField reads one jsonpath expression off a DNSCheck.
func dnsCheckField(name, jsonPath string) string {
	out, err := utils.Run(exec.Command("kubectl", "get", "dnscheck", name,
		"-n", dnsResolutionNamespace, "-o", "jsonpath="+jsonPath))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// eventuallyDNSResult waits for a check to settle on a verdict, reporting the
// summary on failure so a wrong verdict explains itself.
func eventuallyDNSResult(name, want string) {
	Eventually(func(g Gomega) {
		g.Expect(dnsCheckField(name, "{.status.lastResult}")).To(Equal(want),
			"summary was %q", dnsCheckField(name, "{.status.summary}"))
	}, 3*time.Minute, 3*time.Second).Should(Succeed())
}

var _ = Describe("DNSCheck resolution", Ordered, Label(utils.CoreLabel, "dnscheck"), func() {
	BeforeAll(func() {
		ensureNamespaceActive(dnsResolutionNamespace)
	})

	AfterAll(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", dnsResolutionNamespace,
			"--ignore-not-found=true", "--wait=false"))
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		out, _ := utils.Run(exec.Command("kubectl", "get", "dnschecks", "-n", dnsResolutionNamespace, "-o", "yaml"))
		_, _ = fmt.Fprintf(GinkgoWriter, "dnschecks:\n%s\n", out)
	})

	// T046 — SC-008 and FR-031, the most important row in this file. The check
	// lives in a namespace the operator does not own, and resolution must happen
	// THERE: that is what makes a check author's reach exactly their own, and
	// what puts the query under that namespace's NetworkPolicy.
	It("resolves a real cluster name from the check's own namespace", func() {
		applyDNSCheck("in-cluster", `  interval: 1m
  timeout: 30s
  targets:
    - name: kubernetes.default.svc.cluster.local
      recordType: Host
`)
		eventuallyDNSResult("in-cluster", "Pass")

		Expect(dnsCheckField("in-cluster", "{.status.observedTargets}")).To(Equal("1"))
		Expect(dnsCheckField("in-cluster", "{.status.targetResults[0].resolver}")).To(Equal("cluster"))

		By("confirming a probe Pod was scheduled in the check's namespace, not the operator's")
		out, err := utils.Run(exec.Command("kubectl", "get", "events",
			"-n", dnsResolutionNamespace, "--field-selector", "reason=Scheduled", "-o", "json"))
		Expect(err).NotTo(HaveOccurred())
		var list eventList
		Expect(json.Unmarshal([]byte(out), &list)).To(Succeed())
		found := false
		for _, ev := range list.Items {
			if strings.HasPrefix(ev.InvolvedObject.Name, "fathom-dnscheck-") {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(),
			"no fathom-dnscheck-* Pod was scheduled in %s; resolution may have run elsewhere (SC-008)",
			dnsResolutionNamespace)
	})

	// T047 — FR-025 and FR-012. `.invalid` is guaranteed non-resolvable
	// (RFC 6761), so one name exercises both assertion polarities against a real
	// resolver.
	It("fails a positive assertion on a dead name and passes the same name as absent", func() {
		applyDNSCheck("polarity", `  interval: 1m
  timeout: 30s
  targets:
    - name: nothing-here.invalid.
      recordType: A
`)
		// FR-025: a resolver's negative answer is the check's finding, not an
		// operator-side fault. Error here would mask unrelated failures.
		eventuallyDNSResult("polarity", "Fail")

		applyDNSCheck("polarity-absent", `  interval: 1m
  timeout: 30s
  targets:
    - name: nothing-here.invalid.
      recordType: A
      absent: true
`)
		eventuallyDNSResult("polarity-absent", "Pass")
	})

	// T048 — FR-014's sharpest case, against a real network rather than a fake
	// launcher. An unreachable resolver is a network fault, never proof that a
	// name was retired.
	It("never lets an unreachable resolver satisfy a negative assertion", func() {
		applyDNSCheck("unreachable-absent", `  interval: 1m
  timeout: 20s
  resolvers:
    - name: blackhole
      from: Explicit
      address: 10.255.255.1
  targets:
    - name: retired.example.com.
      recordType: A
      absent: true
`)
		Eventually(func(g Gomega) {
			result := dnsCheckField("unreachable-absent", "{.status.lastResult}")
			g.Expect(result).NotTo(BeEmpty())
			g.Expect(result).NotTo(Equal("Pass"),
				"an unreachable resolver reported a satisfied absence assertion (FR-014); summary %q",
				dnsCheckField("unreachable-absent", "{.status.summary}"))
			g.Expect(result).To(BeElementOf("Fail", "Error", "Unknown"))
		}, 3*time.Minute, 3*time.Second).Should(Succeed())
	})

	// T050 — SC-101. A bound too small for the pair count must truncate visibly
	// rather than overrun, and must not report the pairs it did reach as the
	// whole story.
	It("truncates a run that cannot reach every pair", func() {
		applyDNSCheck("truncating", `  interval: 1m
  timeout: 5s
  resolvers:
    - name: blackhole
      from: Explicit
      address: 10.255.255.1
  targets:
    - name: a.example.com.
      recordType: A
    - name: b.example.com.
      recordType: A
    - name: c.example.com.
      recordType: A
    - name: d.example.com.
      recordType: A
    - name: e.example.com.
      recordType: A
    - name: f.example.com.
      recordType: A
`)
		By("waiting for the first run to report")
		Eventually(func(g Gomega) {
			g.Expect(dnsCheckField("truncating", "{.status.lastResult}")).NotTo(BeEmpty())
		}, 3*time.Minute, 3*time.Second).Should(Succeed())

		By("expecting Complete=False, naming how many pairs went unreached")
		Eventually(func(g Gomega) {
			reason := dnsCheckField("truncating", `{.status.conditions[?(@.type=="Complete")].reason}`)
			g.Expect(reason).To(Equal("RunTruncated"),
				"summary %q", dnsCheckField("truncating", "{.status.summary}"))
		}, 2*time.Minute, 3*time.Second).Should(Succeed())

		Expect(dnsCheckField("truncating", "{.status.summary}")).To(ContainSubstring("not reached"))
		Expect(dnsCheckField("truncating", "{.status.lastResult}")).NotTo(Equal("Pass"),
			"a truncated run must never report the pairs it reached as the whole story")
	})
})
