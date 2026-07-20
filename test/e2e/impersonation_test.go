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

// These specs assert the least-privilege RBAC posture the operator is deployed
// with (SKA-58): the operator ServiceAccount holds no addon reads of its own and
// may only impersonate the per-addon ServiceAccounts, each of which is scoped to
// its own declared reads. They exercise the real cluster authorizer via
// `kubectl auth can-i --as=...`, so they prove the deployed manifests actually
// restrict access — something envtest's superuser client cannot show. The per-
// addon adapter specs (coredns/certmanager/cilium/externalsecrets) separately
// prove the adapters still function end-to-end while impersonating.
var _ = Describe("Adapter RBAC impersonation posture", Label(utils.CoreLabel), func() {
	const (
		operatorNS = "fathom-system"
		operatorSA = "system:serviceaccount:fathom-system:fathom-controller-manager"
		ciliumSA   = "system:serviceaccount:fathom-system:fathom-addon-cilium"
	)

	// canI returns the trimmed "yes"/"no" from `kubectl auth can-i`. can-i exits
	// non-zero for "no", so the error is deliberately ignored — the answer is on
	// stdout, which utils.Run returns regardless of exit code.
	canI := func(as string, args ...string) string {
		full := append([]string{"auth", "can-i"}, args...)
		full = append(full, "--as="+as)
		out, _ := utils.Run(exec.Command("kubectl", full...))
		// The verdict is the final line; any warnings precede it. Take the last
		// non-empty line, trimmed — not the last whitespace token, which could be
		// a word from a warning.
		var verdict string
		for _, line := range strings.Split(out, "\n") {
			if s := strings.TrimSpace(line); s != "" {
				verdict = s
			}
		}
		return verdict
	}

	It("denies the operator ServiceAccount direct addon reads", func() {
		Expect(canI(operatorSA, "get", "deployments", "-A")).To(Equal("no"))
		Expect(canI(operatorSA, "list", "secrets", "-A")).To(Equal("no"))
	})

	It("lets the operator impersonate a per-addon ServiceAccount, but only those", func() {
		Expect(canI(operatorSA, "impersonate", "serviceaccounts/fathom-addon-cilium", "-n", operatorNS)).To(Equal("yes"))
		// resourceNames + namespace scoping: it cannot impersonate an arbitrary SA.
		Expect(canI(operatorSA, "impersonate", "serviceaccounts/default", "-n", "kube-system")).To(Equal("no"))
	})

	It("scopes each per-addon ServiceAccount to its own declared reads", func() {
		// Cilium declares deployments/daemonsets/pods/CRDs reads — but no secrets.
		Expect(canI(ciliumSA, "get", "deployments", "-A")).To(Equal("yes"))
		Expect(canI(ciliumSA, "get", "secrets", "-A")).To(Equal("no"))
	})

	It("creates a labeled ServiceAccount for every built-in addon", func() {
		out, err := utils.Run(exec.Command("kubectl", "get", "serviceaccounts",
			"-n", operatorNS, "-l", "fathom.skaphos.io/addon",
			"-o", "jsonpath={range .items[*]}{.metadata.labels.fathom\\.skaphos\\.io/addon}{\"\\n\"}{end}"))
		Expect(err).NotTo(HaveOccurred())
		for _, addon := range []string{"cert-manager", "cilium", "coredns", "external-secrets"} {
			Expect(out).To(ContainSubstring(addon), "expected a scoped ServiceAccount for addon %q", addon)
		}
	})
})
