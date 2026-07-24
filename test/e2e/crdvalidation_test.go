/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

// Live-cluster smoke for the CRD admission hardening (issue #152): the envtest
// matrices in internal/controller cover the full boundary tables; this spec
// only proves the rules are enforced by a real API server with the shipped
// CRDs installed — the exact surface a user's kubectl apply hits.
var _ = Describe("CRD validation hardening", Ordered, Label(utils.CoreLabel), func() {
	applyManifest := func(manifest string) (string, error) {
		cmd := exec.Command("kubectl", "apply", "--dry-run=server", "-f", "-")
		cmd.Stdin = strings.NewReader(manifest)
		return utils.Run(cmd)
	}

	It("rejects a sub-floor interval at admission", func() {
		out, err := applyManifest(`apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: crd-validation-floor-e2e
  namespace: default
spec:
  addonType: coredns
  interval: 1ms
`)
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("interval must be at least 10s"))
	})

	It("rejects a sub-floor timeout at admission", func() {
		out, err := applyManifest(`apiVersion: fathom.skaphos.io/v1alpha1
kind: NodeCertificateCheck
metadata:
  name: crd-validation-timeout-e2e
spec:
  timeout: 500ms
`)
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("timeout must be at least 1s"))
	})

	It("rejects a non-numeric warnDays threshold at admission", func() {
		out, err := applyManifest(`apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: crd-validation-policy-e2e
  namespace: default
spec:
  addonType: cert-manager
  policy:
    certificates:
      thresholds:
        warnDays: "banana"
`)
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("warnDays and failDays must be whole numbers"))
	})
})
