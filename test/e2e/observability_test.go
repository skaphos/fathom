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

const (
	observabilityCheckName  = "observability-check"
	observabilityBrokenName = "observability-broken"
	observabilityCheckNS    = "default"
	// observabilityRoleBinding is separate from the base metrics spec's binding
	// so this suite stays self-contained whatever the spec ordering.
	observabilityRoleBinding = "fathom-observability-metrics-binding"
)

// observabilityCheckManifest is a minimal cert-manager AddonCheck (core tier,
// so its target is always installed). The "broken" variant points the
// system_health thresholds at a Deployment that does not exist, which makes
// the adapter report Fail — the induced-failure path for Warning events.
func observabilityCheckManifest(name, controllerName string) string {
	return fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: %s
  namespace: %s
spec:
  addonType: cert-manager
  interval: 5m
  timeout: 30s
  policy:
    system_health:
      enabled: true
      thresholds:
        controllerName: %q
`, name, observabilityCheckNS, controllerName)
}

func applyManifest(content, fileName string) {
	GinkgoHelper()
	path := filepath.Join("/tmp", fileName)
	Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to apply manifest %s", fileName)
}

// scrapeOperatorMetrics fetches the operator /metrics body through an
// ephemeral curl pod with its own uniquely named pod per call, so repeated
// scrapes in one spec (and reruns on a reused cluster) never collide.
func scrapeOperatorMetrics(podName string) string {
	GinkgoHelper()

	cmd := exec.Command("kubectl", "create", "clusterrolebinding", observabilityRoleBinding,
		"--clusterrole=fathom-metrics-reader",
		fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
	)
	// AlreadyExists across specs/reruns is fine.
	_, _ = utils.Run(cmd)

	token, err := serviceAccountToken()
	Expect(err).NotTo(HaveOccurred())
	Expect(token).NotTo(BeEmpty())

	cmd = exec.Command("kubectl", "delete", "pod", podName, "-n", namespace, "--ignore-not-found=true")
	_, _ = utils.Run(cmd)

	cmd = exec.Command("kubectl", "run", podName, "--restart=Never",
		"--namespace", namespace,
		"--image=curlimages/curl:latest",
		"--overrides",
		fmt.Sprintf(`{
			"spec": {
				"containers": [{
					"name": "curl",
					"image": "curlimages/curl:latest",
					"command": ["/bin/sh", "-c"],
					"args": ["curl -s -f -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
					"securityContext": {
						"allowPrivilegeEscalation": false,
						"capabilities": {"drop": ["ALL"]},
						"runAsNonRoot": true,
						"runAsUser": 1000,
						"seccompProfile": {"type": "RuntimeDefault"}
					}
				}],
				"serviceAccount": "%s"
			}
		}`, token, metricsServiceName, namespace, serviceAccountName))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create %s pod", podName)
	DeferCleanup(func() {
		cmd := exec.Command("kubectl", "delete", "pod", podName, "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "pods", podName, "-o", "jsonpath={.status.phase}", "-n", namespace)
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
	}, 3*time.Minute).Should(Succeed())

	cmd = exec.Command("kubectl", "logs", podName, "-n", namespace)
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from %s", podName)
	return output
}

var _ = Describe("Check observability", Ordered, Label(utils.CoreLabel), func() {
	BeforeAll(func() {
		By("clearing any prior observability AddonCheck state")
		for _, name := range []string{observabilityCheckName, observabilityBrokenName} {
			cmd := exec.Command("kubectl", "delete", "addoncheck", name,
				"-n", observabilityCheckNS, "--ignore-not-found=true", "--wait=true")
			_, _ = utils.Run(cmd)
		}

		By("applying a healthy and a deliberately broken AddonCheck")
		applyManifest(observabilityCheckManifest(observabilityCheckName, "cert-manager"), "observability-check.yaml")
		applyManifest(observabilityCheckManifest(observabilityBrokenName, "no-such-controller"), "observability-broken.yaml")
	})

	AfterAll(func() {
		for _, name := range []string{observabilityCheckName, observabilityBrokenName} {
			cmd := exec.Command("kubectl", "delete", "addoncheck", name,
				"-n", observabilityCheckNS, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		}
		cmd := exec.Command("kubectl", "delete", "clusterrolebinding", observabilityRoleBinding, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpAddonCheckDiagnostics(observabilityCheckName, observabilityCheckNS)
		dumpAddonCheckDiagnostics(observabilityBrokenName, observabilityCheckNS)
	})

	It("exports the one-hot result gauge and last-run timestamp for evaluated checks", func() {
		By("waiting for both checks to record a result")
		Eventually(func(g Gomega) {
			for name, want := range map[string]string{observabilityCheckName: "Pass", observabilityBrokenName: "Fail"} {
				cmd := exec.Command("kubectl", "get", "addoncheck", name, "-n", observabilityCheckNS,
					"-o", "jsonpath={.status.lastResult}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(want), "check %s lastResult", name)
			}
		}, 5*time.Minute).Should(Succeed())

		metricsOutput := scrapeOperatorMetrics("curl-metrics-observability")

		By("asserting the one-hot shape for the passing check")
		passSeries := func(result string, value int) string {
			return fmt.Sprintf(`fathom_check_result{kind="AddonCheck",name=%q,namespace=%q,result=%q} %d`,
				observabilityCheckName, observabilityCheckNS, result, value)
		}
		Expect(metricsOutput).To(ContainSubstring(passSeries("Pass", 1)))
		for _, other := range []string{"Warn", "Fail", "Error", "Skipped", "Unknown"} {
			Expect(metricsOutput).To(ContainSubstring(passSeries(other, 0)))
		}

		By("asserting the broken check reads Fail")
		Expect(metricsOutput).To(ContainSubstring(fmt.Sprintf(
			`fathom_check_result{kind="AddonCheck",name=%q,namespace=%q,result="Fail"} 1`,
			observabilityBrokenName, observabilityCheckNS)))

		By("asserting a non-zero last-run timestamp")
		Expect(metricsOutput).To(ContainSubstring(fmt.Sprintf(
			`fathom_check_last_run_timestamp_seconds{kind="AddonCheck",name=%q,namespace=%q}`,
			observabilityCheckName, observabilityCheckNS)))
		Expect(metricsOutput).NotTo(ContainSubstring(fmt.Sprintf(
			`fathom_check_last_run_timestamp_seconds{kind="AddonCheck",name=%q,namespace=%q} 0`,
			observabilityCheckName, observabilityCheckNS)))
	})

	It("records ResultChanged events, Warning for degradations, without per-run spam", func() {
		type eventRow struct{ evType, reason, message string }
		listEvents := func(name string) []eventRow {
			cmd := exec.Command("kubectl", "get", "events", "-n", observabilityCheckNS,
				"--field-selector", fmt.Sprintf("involvedObject.name=%s,reason=ResultChanged", name),
				"-o", `jsonpath={range .items[*]}{.type}|{.reason}|{.message}{"\n"}{end}`)
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			var rows []eventRow
			for _, line := range strings.Split(output, "\n") {
				parts := strings.SplitN(line, "|", 3)
				if len(parts) == 3 {
					rows = append(rows, eventRow{evType: parts[0], reason: parts[1], message: parts[2]})
				}
			}
			return rows
		}

		Eventually(func(g Gomega) {
			healthy := listEvents(observabilityCheckName)
			g.Expect(healthy).NotTo(BeEmpty(), "healthy check should have a ResultChanged event")
			g.Expect(healthy[0].evType).To(Equal("Normal"))
			g.Expect(healthy[0].message).To(ContainSubstring("from Unknown to Pass"))

			broken := listEvents(observabilityBrokenName)
			g.Expect(broken).NotTo(BeEmpty(), "broken check should have a ResultChanged event")
			g.Expect(broken[0].evType).To(Equal("Warning"))
			g.Expect(broken[0].message).To(ContainSubstring("to Fail"))
		}, 2*time.Minute).Should(Succeed())

		By("asserting transitions produce one event, not one per reconcile")
		Expect(listEvents(observabilityCheckName)).To(HaveLen(1))
	})

	It("removes a deleted check's series by the next scrape", func() {
		cmd := exec.Command("kubectl", "delete", "addoncheck", observabilityBrokenName,
			"-n", observabilityCheckNS, "--wait=true")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			metricsOutput := scrapeOperatorMetrics(fmt.Sprintf("curl-metrics-obs-del-%d", time.Now().Unix()))
			g.Expect(metricsOutput).NotTo(ContainSubstring(fmt.Sprintf(`name=%q`, observabilityBrokenName)))
			// The surviving check keeps its series.
			g.Expect(metricsOutput).To(ContainSubstring(fmt.Sprintf(`name=%q`, observabilityCheckName)))
		}, 2*time.Minute).Should(Succeed())
	})
})
