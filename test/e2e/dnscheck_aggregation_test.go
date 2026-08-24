/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

var _ = Describe("DNSCheck aggregation", Ordered, Label(utils.CoreLabel, "dnscheck"), func() {
	const (
		ns        = "dnscheck-aggregation-e2e"
		dnsName   = "aggregate-source"
		health    = "aggregate-wrapper"
		aggregate = "dnscheck-aggregate-e2e"
	)

	manifest := `apiVersion: v1
kind: Namespace
metadata:
  name: ` + ns + `
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: DNSCheck
metadata:
  name: ` + dnsName + `
  namespace: ` + ns + `
spec:
  interval: 1m
  timeout: 30s
  targets:
    - name: kubernetes.default.svc.cluster.local
      recordType: Host
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: ` + health + `
  namespace: ` + ns + `
  labels:
    dnscheck-aggregation-e2e: "true"
spec:
  checkRef:
    apiVersion: fathom.skaphos.io/v1alpha1
    kind: DNSCheck
    name: ` + dnsName + `
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: ClusterHealth
metadata:
  name: ` + aggregate + `
spec:
  namespaces:
    - ` + ns + `
  selector:
    matchLabels:
      dnscheck-aggregation-e2e: "true"
`

	kubectlField := func(resource, name, namespace, path string) string {
		args := []string{"get", resource, name}
		if namespace != "" {
			args = append(args, "-n", namespace)
		}
		args = append(args, "-o", "jsonpath="+path)
		out, err := utils.Run(exec.Command("kubectl", args...))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(out)
	}

	reportCount := func() int {
		out, err := utils.Run(exec.Command("kubectl", "get", "healthreport", "-n", ns,
			"-l", "fathom.skaphos.io/source-name="+dnsName,
			"-o", "jsonpath={.items[*].metadata.name}"))
		if err != nil || strings.TrimSpace(out) == "" {
			return 0
		}
		return len(strings.Fields(out))
	}

	BeforeAll(func() {
		apply := exec.Command("kubectl", "apply", "-f", "-")
		apply.Stdin = strings.NewReader(manifest)
		_, err := utils.Run(apply)
		Expect(err).NotTo(HaveOccurred(), "apply DNS aggregation fixtures")
	})

	AfterAll(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "clusterhealth", aggregate, "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", ns, "--ignore-not-found=true", "--wait=false"))
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		out, _ := utils.Run(exec.Command("kubectl", "get", "dnscheck,healthcheck,healthreport", "-n", ns, "-o", "yaml"))
		_, _ = fmt.Fprintf(GinkgoWriter, "DNS aggregation namespaced resources:\n%s\n", out)
		out, _ = utils.Run(exec.Command("kubectl", "get", "clusterhealth", aggregate, "-o", "yaml"))
		_, _ = fmt.Fprintf(GinkgoWriter, "DNS aggregation ClusterHealth:\n%s\n", out)
	})

	It("propagates Pass and Fail evidence without duplicate history", func() {
		By("waiting for the DNS source, wrapper, and aggregate to pass")
		Eventually(func(g Gomega) {
			g.Expect(kubectlField("dnscheck", dnsName, ns, "{.status.lastResult}")).To(Equal("Pass"))
			g.Expect(kubectlField("healthcheck", health, ns, "{.status.result}")).To(Equal("Pass"))
			g.Expect(kubectlField("clusterhealth", aggregate, "", "{.status.result}")).To(Equal("Pass"))
		}, 3*time.Minute, 3*time.Second).Should(Succeed())

		assertEvidenceParity := func(g Gomega, want string) {
			dnsObserved := kubectlField("dnscheck", dnsName, ns, "{.status.lastRunTime}")
			dnsReport := kubectlField("dnscheck", dnsName, ns, "{.status.lastReportName}")
			healthObserved := kubectlField("healthcheck", health, ns, "{.status.sourceObservedAt}")
			healthReport := kubectlField("healthcheck", health, ns, "{.status.lastReportName}")
			healthSummary := kubectlField("healthcheck", health, ns, "{.status.summary}")
			g.Expect(kubectlField("healthcheck", health, ns, "{.status.result}")).To(Equal(want))
			g.Expect(dnsObserved).NotTo(BeEmpty())
			g.Expect(healthObserved).To(Equal(dnsObserved))
			g.Expect(healthReport).To(Equal(dnsReport))
			g.Expect(utf8.RuneCountInString(healthSummary)).To(BeNumerically("<=", 1024))
			g.Expect(kubectlField("clusterhealth", aggregate, "", "{.status.matchedCount}")).To(Equal("1"))
			g.Expect(kubectlField("clusterhealth", aggregate, "", "{.status.children[0].result}")).To(Equal(want))
			g.Expect(kubectlField("clusterhealth", aggregate, "", "{.status.children[0].summary}")).To(Equal(healthSummary))
			g.Expect(kubectlField("clusterhealth", aggregate, "", "{.status.children[0].observedAt}")).To(Equal(healthObserved))
		}
		Eventually(func(g Gomega) { assertEvidenceParity(g, "Pass") }, time.Minute, 2*time.Second).Should(Succeed())
		Eventually(reportCount, time.Minute, 2*time.Second).Should(Equal(1))

		By("changing the expectation so the resolvable name must be absent")
		patch := `{"spec":{"targets":[{"name":"kubernetes.default.svc.cluster.local","recordType":"Host","absent":true}]}}`
		_, err := utils.Run(exec.Command("kubectl", "patch", "dnscheck", dnsName, "-n", ns, "--type=merge", "-p", patch))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			g.Expect(kubectlField("dnscheck", dnsName, ns, "{.status.lastResult}")).To(Equal("Fail"))
			g.Expect(kubectlField("clusterhealth", aggregate, "", "{.status.result}")).To(Equal("Fail"))
			assertEvidenceParity(g, "Fail")
		}, 3*time.Minute, 3*time.Second).Should(Succeed())
		Eventually(reportCount, time.Minute, 2*time.Second).Should(Equal(2))

		By("triggering an unchanged reconciliation and verifying history remains change-only")
		previousRun := kubectlField("dnscheck", dnsName, ns, "{.status.lastRunTime}")
		annotation := "fathom.skaphos.io/e2e-reconcile=" + strconv.FormatInt(time.Now().UnixNano(), 10)
		_, err = utils.Run(exec.Command("kubectl", "annotate", "dnscheck", dnsName, "-n", ns, annotation, "--overwrite"))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() string {
			return kubectlField("dnscheck", dnsName, ns, "{.status.lastRunTime}")
		}, 3*time.Minute, 3*time.Second).ShouldNot(Equal(previousRun))
		Consistently(reportCount, 15*time.Second, 2*time.Second).Should(Equal(2))
	})
})
