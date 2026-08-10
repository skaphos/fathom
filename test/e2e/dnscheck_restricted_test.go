/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
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

// dnsRestrictedNamespace enforces the `restricted` Pod Security Standard, the
// posture a security-conscious tenant namespace actually runs.
const dnsRestrictedNamespace = "dnscheck-restricted-e2e"

// T049 — the last first-contact risk research flagged.
//
// probe.Pod() builds a hardened pod and unit tests assert its security context
// field by field, but a field-by-field assertion is not the same claim as "the
// admission controller accepts it". Pod Security admission evaluates the whole
// pod, in a namespace the operator does not own, and it is the only thing that
// can answer that. If this fails, DNSCheck is unusable in exactly the
// namespaces most likely to want it.
var _ = Describe("DNSCheck under restricted Pod Security", Ordered, Label(utils.CoreLabel, "dnscheck"), func() {
	BeforeAll(func() {
		By("creating a namespace that enforces the restricted Pod Security Standard")
		ensureNamespaceActive(dnsRestrictedNamespace)
		for _, label := range []string{
			"pod-security.kubernetes.io/enforce=restricted",
			"pod-security.kubernetes.io/enforce-version=latest",
			"pod-security.kubernetes.io/audit=restricted",
			"pod-security.kubernetes.io/warn=restricted",
		} {
			_, err := utils.Run(exec.Command("kubectl", "label", "namespace",
				dnsRestrictedNamespace, label, "--overwrite"))
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterAll(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", dnsRestrictedNamespace,
			"--ignore-not-found=true", "--wait=false"))
	})

	It("admits the probe Pod and resolves normally", func() {
		manifest := fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: DNSCheck
metadata:
  name: restricted
  namespace: %s
spec:
  interval: 1m
  timeout: 30s
  targets:
    - name: kubernetes.default.svc.cluster.local
      recordType: Host
`, dnsRestrictedNamespace)
		path := filepath.Join(os.TempDir(), "fathom-e2e-restricted.yaml")
		Expect(os.WriteFile(path, []byte(manifest), 0o600)).To(Succeed())
		DeferCleanup(func() {
			_, _ = utils.Run(exec.Command("kubectl", "delete", "-f", path, "--ignore-not-found=true"))
			_ = os.Remove(path)
		})
		_, err := utils.Run(exec.Command("kubectl", "apply", "-f", path))
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			out, getErr := utils.Run(exec.Command("kubectl", "get", "dnscheck", "restricted",
				"-n", dnsRestrictedNamespace, "-o", "jsonpath={.status.lastResult}"))
			g.Expect(getErr).NotTo(HaveOccurred())

			summary, _ := utils.Run(exec.Command("kubectl", "get", "dnscheck", "restricted",
				"-n", dnsRestrictedNamespace, "-o", "jsonpath={.status.summary}"))
			// A PSA rejection surfaces as a per-pair Error naming "violates
			// PodSecurity", which is a far more useful failure message than a
			// bare timeout — so assert against it explicitly.
			g.Expect(summary).NotTo(ContainSubstring("violates PodSecurity"),
				"the hardened probe Pod was rejected by restricted Pod Security admission")
			g.Expect(strings.TrimSpace(out)).To(Equal("Pass"), "summary was %q", summary)
		}, 3*time.Minute, 3*time.Second).Should(Succeed())
	})
})
