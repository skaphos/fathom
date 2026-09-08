/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
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

const (
	fathomctlNS      = "fathomctl-e2e"
	fathomctlBin     = "bin/fathomctl"
	fathomctlAddon   = "coredns-sample" // from config/samples, applied into fathomctlNS
	fathomctlDNS     = "fathomctl-dns"
	fathomctlNodeCrt = "fathomctl-ncc"
	fathomctlCH      = "fathomctl-e2e"
	fathomctlPaused  = "fathomctl-paused"
)

// fathomctlRunOutcome mirrors the `run -o json` element the CLI contracts in
// docs/reference/fathomctl.md. Declared locally, like the other e2e views, so
// the spec asserts the published shape rather than importing internal types.
type fathomctlRunOutcome struct {
	Target    string `json:"target"`
	Via       string `json:"via"`
	Token     string `json:"token"`
	Triggered bool   `json:"triggered"`
	Skipped   string `json:"skipped"`
	Error     string `json:"error"`
	Verdict   string `json:"verdict"`
	Summary   string `json:"summary"`
	TimedOut  bool   `json:"timedOut"`
}

// fathomctl runs the freshly built binary against the current kubeconfig
// context (the Kind cluster). Unlike utils.Run it keeps stdout separate from
// stderr, because the CLI prints progress to stderr and `-o json` must parse:
// on success it returns stdout alone; on failure it returns both, and the
// error wraps the *exec.ExitError so the exit code is recoverable.
func fathomctl(args ...string) (string, error) {
	dir, _ := utils.GetProjectDir()
	cmd := exec.Command(fathomctlBin, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_, _ = fmt.Fprintf(GinkgoWriter, "running: fathomctl %s\n", strings.Join(args, " "))
	out, err := cmd.Output()
	if stderr.Len() > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "fathomctl stderr:\n%s\n", stderr.String())
	}
	if err != nil {
		return string(out) + stderr.String(), fmt.Errorf("fathomctl %s failed: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return string(out), nil
}

func fathomctlExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func kubectlJSONPath(kind, name, ns, path string) (string, error) {
	args := []string{"get", kind, name, "-o", "jsonpath=" + path}
	if ns != "" {
		args = append(args, "-n", ns)
	}
	return utils.Run(exec.Command("kubectl", args...))
}

func fathomctlFixtureManifest() string {
	return fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: DNSCheck
metadata:
  name: %[2]s
  namespace: %[1]s
spec:
  interval: 30s
  timeout: 20s
  targets:
    - name: kubernetes.default.svc.cluster.local
      recordType: A
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: NodeCertificateCheck
metadata:
  name: %[3]s
  namespace: %[1]s
spec:
  includeControlPlaneNodes: true
  paths:
    - /etc/kubernetes/pki/apiserver.crt
    - /etc/kubernetes/pki/ca.crt
  interval: 30s
  timeout: 20s
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: %[6]s
  namespace: %[1]s
spec:
  addonType: coredns
  paused: true
  policy:
    system_health:
      enabled: true
      namespaces: [kube-system]
      thresholds:
        deploymentName: "coredns"
        serviceName: "kube-dns"
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: hc-addon
  namespace: %[1]s
spec:
  checkRef: {kind: AddonCheck, name: %[4]s}
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: hc-dns
  namespace: %[1]s
spec:
  checkRef: {kind: DNSCheck, name: %[2]s}
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: hc-ncc
  namespace: %[1]s
spec:
  checkRef: {kind: NodeCertificateCheck, name: %[3]s}
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: ClusterHealth
metadata:
  name: %[5]s
spec:
  namespaces: [%[1]s]
`, fathomctlNS, fathomctlDNS, fathomctlNodeCrt, fathomctlAddon, fathomctlCH, fathomctlPaused)
}

var _ = Describe("fathomctl", Ordered, Label(utils.CoreLabel, "fathomctl"), func() {
	var manifest string

	BeforeAll(func() {
		By("building fathomctl from the working tree")
		_, err := utils.Run(exec.Command("go", "build", "-o", fathomctlBin, "./cmd/fathomctl"))
		Expect(err).NotTo(HaveOccurred(), "go build ./cmd/fathomctl failed")

		By("creating the fathomctl e2e namespace and fixtures")
		_, _ = utils.Run(exec.Command("kubectl", "create", "namespace", fathomctlNS))
		_, err = utils.Run(exec.Command("kubectl", "apply", "-n", fathomctlNS, "-f", corednsSamplePath))
		Expect(err).NotTo(HaveOccurred(), "apply CoreDNS sample into %s", fathomctlNS)
		manifest = filepath.Join(os.TempDir(), "fathom-e2e-fathomctl.yaml")
		Expect(os.WriteFile(manifest, []byte(fathomctlFixtureManifest()), 0o600)).To(Succeed())
		_, err = utils.Run(exec.Command("kubectl", "apply", "-f", manifest))
		Expect(err).NotTo(HaveOccurred(), "apply fathomctl fixtures")

		By("waiting for every executable check to produce a first verdict")
		Eventually(func(g Gomega) {
			for _, kn := range [][2]string{{"addoncheck", fathomctlAddon}, {"dnscheck", fathomctlDNS}, {"nodecertificatecheck", fathomctlNodeCrt}} {
				result, err := kubectlJSONPath(kn[0], kn[1], fathomctlNS, "{.status.lastResult}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result).NotTo(BeEmpty(), "%s/%s has no verdict yet", kn[0], kn[1])
			}
		}, 5*time.Minute, 5*time.Second).Should(Succeed())

		By("waiting for the ClusterHealth to select the three wrappers")
		Eventually(func(g Gomega) {
			matched, err := kubectlJSONPath("clusterhealth", fathomctlCH, "", "{.status.matchedCount}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(matched).To(Equal("3"), "ClusterHealth matched %q wrappers", matched)
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		By("removing the fathomctl e2e fixtures")
		_, _ = utils.Run(exec.Command("kubectl", "delete", "clusterhealth", fathomctlCH, "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", fathomctlNS, "--ignore-not-found=true", "--wait=false"))
		if manifest != "" {
			_ = os.Remove(manifest)
		}
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		By("dumping fathomctl diagnostics")
		for _, args := range [][]string{
			{"get", "addonchecks,dnschecks,nodecertificatechecks,healthchecks", "-n", fathomctlNS, "-o", "yaml"},
			{"get", "clusterhealth", fathomctlCH, "-o", "yaml"},
			{"get", "pods", "-n", fathomctlNS, "-o", "wide"},
			{"logs", "-l", "control-plane=controller-manager", "-n", namespace, "--tail=200"},
		} {
			out, _ := utils.Run(exec.Command("kubectl", args...))
			_, _ = fmt.Fprintf(GinkgoWriter, "kubectl %s:\n%s\n", strings.Join(args, " "), out)
		}
	})

	// FR-019 to FR-021, FR-024, SC-002, SC-008: a forced run on every
	// executable kind completes, the CLI's exit follows the verdict, and the
	// operator records exactly the token the CLI wrote.
	DescribeTable("run --wait triggers a fresh run and records the token",
		func(kind, name string, checkTimeout time.Duration, bound time.Duration) {
			start := time.Now()
			out, err := fathomctl("run", kind+"/"+name, "-n", fathomctlNS, "--wait", "--timeout", "5m", "-o", "json")
			elapsed := time.Since(start)
			Expect(err).NotTo(HaveOccurred(), "run --wait exited non-zero:\n%s", out)

			var outcomes []fathomctlRunOutcome
			Expect(json.Unmarshal([]byte(out), &outcomes)).To(Succeed(), "run -o json output:\n%s", out)
			Expect(outcomes).To(HaveLen(1))
			o := outcomes[0]
			Expect(o.Triggered).To(BeTrue())
			Expect(o.Token).To(MatchRegexp(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z-[0-9a-f]{6}$`))
			Expect(o.Verdict).To(BeElementOf("Pass", "Warn", "Skipped"), "verdict %q (%s)", o.Verdict, o.Summary)

			consumed, err := kubectlJSONPath(kind, name, fathomctlNS, "{.status.lastRunTrigger}")
			Expect(err).NotTo(HaveOccurred())
			Expect(consumed).To(Equal(o.Token), "operator recorded a different token than the CLI wrote")

			Expect(elapsed).To(BeNumerically("<", bound),
				"SC-002: %s/%s took %s, bound is spec.timeout %s + margin", kind, name, elapsed, checkTimeout)
		},
		Entry("AddonCheck", "addoncheck", fathomctlAddon, 30*time.Second, 30*time.Second+30*time.Second),
		Entry("DNSCheck", "dnscheck", fathomctlDNS, 20*time.Second, 20*time.Second+30*time.Second),
		// The node-agent restarts before it can scan; SC-002 qualifies this kind
		// by the Kind cluster's single-node rollout, bounded here generously.
		Entry("NodeCertificateCheck", "nodecertificatecheck", fathomctlNodeCrt, 20*time.Second, 3*time.Minute),
	)

	// SC-008: a consumed token never fires again, even when re-applied after
	// being removed. AddonCheck is the kind that gates runs on the token (a
	// DNSCheck evaluates on every reconcile by design).
	It("does not run again when the consumed token is re-applied", func() {
		token, err := kubectlJSONPath("addoncheck", fathomctlAddon, fathomctlNS, "{.status.lastRunTrigger}")
		Expect(err).NotTo(HaveOccurred())
		Expect(token).NotTo(BeEmpty(), "the previous spec must have left a consumed token")
		before, err := kubectlJSONPath("addoncheck", fathomctlAddon, fathomctlNS, "{.status.lastRunTime}")
		Expect(err).NotTo(HaveOccurred())

		_, err = utils.Run(exec.Command("kubectl", "annotate", "addoncheck", fathomctlAddon, "-n", fathomctlNS, "fathom.skaphos.io/run-now-"))
		Expect(err).NotTo(HaveOccurred())
		_, err = utils.Run(exec.Command("kubectl", "annotate", "addoncheck", fathomctlAddon, "-n", fathomctlNS, "fathom.skaphos.io/run-now="+token, "--overwrite"))
		Expect(err).NotTo(HaveOccurred())

		Consistently(func(g Gomega) {
			after, err := kubectlJSONPath("addoncheck", fathomctlAddon, fathomctlNS, "{.status.lastRunTime}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(after).To(Equal(before), "a re-applied consumed token must not cause a run")
		}, 30*time.Second, 5*time.Second).Should(Succeed())
	})

	// FR-025: a ClusterHealth run fans out to the source behind every selected
	// wrapper, once each, with one token.
	It("fans out from a ClusterHealth to every source once", func() {
		out, err := fathomctl("run", "clusterhealth/"+fathomctlCH, "--yes", "-o", "json")
		Expect(err).NotTo(HaveOccurred(), "run clusterhealth exited non-zero:\n%s", out)

		var outcomes []fathomctlRunOutcome
		Expect(json.Unmarshal([]byte(out), &outcomes)).To(Succeed(), out)
		Expect(outcomes).To(HaveLen(3), "one outcome per source, none duplicated:\n%s", out)
		token := outcomes[0].Token
		targets := map[string]bool{}
		for _, o := range outcomes {
			Expect(o.Triggered).To(BeTrue(), "%+v", o)
			Expect(o.Token).To(Equal(token), "every source must receive the same token")
			Expect(o.Via).To(HavePrefix("healthcheck/" + fathomctlNS + "/"))
			targets[o.Target] = true
		}
		Expect(targets).To(HaveLen(3))

		for _, kn := range [][2]string{{"addoncheck", fathomctlAddon}, {"dnscheck", fathomctlDNS}, {"nodecertificatecheck", fathomctlNodeCrt}} {
			written, err := kubectlJSONPath(kn[0], kn[1], fathomctlNS, `{.metadata.annotations.fathom\.skaphos\.io/run-now}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(written).To(Equal(token), "%s/%s did not receive the fan-out token", kn[0], kn[1])
		}
	})

	// FR-023: a paused check is refused before anything is written.
	It("refuses to trigger a paused check and exits 1", func() {
		out, err := fathomctl("run", "addoncheck/"+fathomctlPaused, "-n", fathomctlNS)
		Expect(fathomctlExitCode(err)).To(Equal(1), "expected exit 1:\n%s", out)
		Expect(out).To(ContainSubstring("paused"))

		written, err := kubectlJSONPath("addoncheck", fathomctlPaused, fathomctlNS, `{.metadata.annotations.fathom\.skaphos\.io/run-now}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(written).To(BeEmpty(), "a paused check must not be written")
	})
})
