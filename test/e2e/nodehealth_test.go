/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

const (
	nodeHealthSamplePath     = "test/e2e/fixtures/nodehealthcheck.yaml"
	nodeHealthFailSamplePath = "test/e2e/fixtures/nodehealthcheck-fail.yaml"
	nodeHealthSampleName     = "nodehealth-e2e"
	nodeHealthSampleNS       = "default"
	nodeHealthDaemonSet      = "nodehealth-e2e-node-health-agent"
)

// This suite exercises NodeHealthCheck end to end against a real kind node
// (#206): the operator must provision a node-agent DaemonSet in health mode —
// with the host network and root privileges the privileged check types cost —
// the agent must measure the node's real filesystem, kubelet, and containerd
// socket, the operator must grade the real Node object's conditions and roll
// everything into a HealthReport, and an incomplete-coverage window must
// freeze the verdict rather than flap it to Unknown. None of that is provable
// in envtest.
var _ = Describe("NodeHealthCheck", Ordered, Label(utils.CoreLabel), func() {
	BeforeAll(func() {
		By("clearing any prior NodeHealthCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", nodeHealthSamplePath, "--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the NodeHealthCheck fixture")
		cmd = exec.Command("kubectl", "apply", "-f", nodeHealthSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply NodeHealthCheck fixture")
	})

	AfterAll(func() {
		By("cleaning up the NodeHealthCheck")
		cmd := exec.Command("kubectl", "delete", "-f", nodeHealthSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpNodeHealthDiagnostics(nodeHealthSampleName, nodeHealthSampleNS)
	})

	It("should roll out the privileged node-agent DaemonSet on the (control-plane) node", func() {
		verify := func(g Gomega) {
			ds, err := daemonSetRollout(nodeHealthDaemonSet, nodeHealthSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch node-agent DaemonSet")
			g.Expect(ds.desired).To(BeNumerically(">", 0), "DaemonSet scheduled on no nodes (tolerations/selector?)")
			g.Expect(ds.ready).To(Equal(ds.desired), "node-agent DaemonSet not fully ready (%d/%d)", ds.ready, ds.desired)
			g.Expect(ds.updated).To(Equal(ds.desired), "node-agent DaemonSet still rolling (%d/%d updated)", ds.updated, ds.desired)
			g.Expect(ds.observedGeneration).To(Equal(ds.generation), "DaemonSet rollout has not converged")
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"node-agent DaemonSet did not become ready within timeout")

		By("confirming the privileged posture the fixture's check types require")
		cmd := exec.Command("kubectl", "get", "daemonset", nodeHealthDaemonSet, "-n", nodeHealthSampleNS,
			"-o", "jsonpath={.spec.template.spec.hostNetwork} {.spec.template.spec.securityContext.runAsUser}")
		out, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("true 0"), "KubeletHealthz needs hostNetwork and ContainerRuntime needs root")
	})

	It("should produce a HealthReport with node_health Pass for every check type", func() {
		var latest healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(nodeHealthSampleName, nodeHealthSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latest = report
			g.Expect(report.Spec.Checks).NotTo(BeEmpty(), "HealthReport %q has no checks yet", report.Metadata.Name)

			seen := map[string]bool{}
			for _, c := range report.Spec.Checks {
				if c.Family != "node_health" {
					continue
				}
				g.Expect(c.TargetRef.Kind).To(Equal("Node"), "node_health check should target a Node")
				seen[c.Details["type"]] = true
				// Skipped is legal for a condition the node does not report;
				// anything else must be Pass on a healthy kind node.
				g.Expect(c.Result).To(BeElementOf("Pass", "Skipped"),
					"node_health %s %s on %s: got %q (%s)", c.Details["type"], c.Details["path"], c.TargetRef.Name, c.Result, c.Summary)
			}
			for _, typ := range []string{"DiskHeadroom", "InodeHeadroom", "NodeCondition", "KubeletHealthz", "ContainerRuntime"} {
				g.Expect(seen).To(HaveKey(typ), "no %s check in HealthReport", typ)
			}
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"node_health HealthReport did not reach Pass within timeout")
		By(fmt.Sprintf("observed HealthReport %q with node_health Pass for every type", latest.Metadata.Name))
	})

	It("should mirror Ready=True, per-node results, and the privileged posture into status", func() {
		verify := func(g Gomega) {
			status, err := nodeHealthStatus(nodeHealthSampleName, nodeHealthSampleNS)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(status.condition("Ready")).To(Equal("True/Reporting"))
			g.Expect(status.condition("CoverageComplete")).To(Equal("True/AllNodesReporting"))
			g.Expect(status.condition("ReportsAuthentic")).To(Equal("True/AllReportsBound"))
			g.Expect(status.condition("AgentPrivileged")).To(Equal("True/HostNetworkAndRoot"))
			g.Expect(status.LastResult).To(Equal("Pass"))
			g.Expect(status.LastReportName).NotTo(BeEmpty())
			g.Expect(status.ReportingNodes).To(BeNumerically(">", 0))
			g.Expect(status.NodeResults).NotTo(BeEmpty())
			g.Expect(status.NodeResults[0].Result).To(Equal("Pass"))
			g.Expect(status.Summary).To(Equal(fmt.Sprintf("%d of %d node(s) passed", status.ReportingNodes, status.ReportingNodes)))
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("should fail the node when a headroom threshold is deliberately unsatisfiable, and recover", func() {
		before, err := nodeHealthStatus(nodeHealthSampleName, nodeHealthSampleNS)
		Expect(err).NotTo(HaveOccurred())

		By("raising the DiskHeadroom critical threshold to 100% free")
		cmd := exec.Command("kubectl", "apply", "-f", nodeHealthFailSamplePath)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			status, err := nodeHealthStatus(nodeHealthSampleName, nodeHealthSampleNS)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(status.LastResult).To(Equal("Fail"))
			g.Expect(status.Summary).To(ContainSubstring("DiskHeadroom /var/lib/kubelet"))
			g.Expect(status.LastReportName).NotTo(Equal(before.LastReportName), "a result transition must write a new HealthReport")
			g.Expect(status.NodeResults[0].Result).To(Equal("Fail"))
			g.Expect(status.NodeResults[0].Message).To(ContainSubstring("criticalPercentFree 100"))
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "threshold change did not produce a Fail")

		By("restoring the satisfiable threshold")
		cmd = exec.Command("kubectl", "apply", "-f", nodeHealthSamplePath)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			status, err := nodeHealthStatus(nodeHealthSampleName, nodeHealthSampleNS)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(status.LastResult).To(Equal("Pass"))
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "did not recover to Pass")
	})

	It("should freeze the verdict, not flap to Unknown, across an incomplete-coverage window (COR-3)", func() {
		frozen, err := nodeHealthStatus(nodeHealthSampleName, nodeHealthSampleNS)
		Expect(err).NotTo(HaveOccurred())
		Expect(frozen.LastResult).To(Equal("Pass"))

		By("selecting a node label nothing carries, so the DaemonSet targets zero nodes")
		cmd := exec.Command("kubectl", "patch", "nodehealthcheck", nodeHealthSampleName, "-n", nodeHealthSampleNS,
			"--type=merge", "-p", `{"spec":{"nodeSelector":{"fathom.skaphos.io/e2e-no-such-node":"true"}}}`)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			status, err := nodeHealthStatus(nodeHealthSampleName, nodeHealthSampleNS)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(status.condition("CoverageComplete")).To(Equal("False/NoMatchingNodes"))
			g.Expect(status.condition("Ready")).To(Equal("False/NoMatchingNodes"))
			// The whole point: the last complete verdict is retained.
			g.Expect(status.LastResult).To(Equal("Pass"), "an incomplete window must freeze the verdict")
			g.Expect(status.LastReportName).To(Equal(frozen.LastReportName), "no new HealthReport during the gap")
			g.Expect(status.NodeResults).To(Equal(frozen.NodeResults), "per-node results frozen with the verdict")
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "coverage gap not reported, or verdict not frozen")

		// Across the whole window the verdict must never have read as anything
		// but the frozen one — sample it repeatedly.
		Consistently(func(g Gomega) {
			status, err := nodeHealthStatus(nodeHealthSampleName, nodeHealthSampleNS)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(status.LastResult).To(Equal("Pass"))
		}, 40*time.Second, 5*time.Second).Should(Succeed(), "verdict flapped during the coverage gap")

		By("restoring the selector")
		cmd = exec.Command("kubectl", "patch", "nodehealthcheck", nodeHealthSampleName, "-n", nodeHealthSampleNS,
			"--type=json", "-p", `[{"op":"remove","path":"/spec/nodeSelector"}]`)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			status, err := nodeHealthStatus(nodeHealthSampleName, nodeHealthSampleNS)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(status.condition("CoverageComplete")).To(Equal("True/AllNodesReporting"))
			g.Expect(status.condition("Ready")).To(Equal("True/Reporting"))
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "coverage did not recover")
	})
})

type nodeHealthStatusView struct {
	Conditions []struct {
		Type   string `json:"type"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"conditions"`
	LastResult     string `json:"lastResult"`
	LastReportName string `json:"lastReportName"`
	Summary        string `json:"summary"`
	ReportingNodes int    `json:"reportingNodes"`
	NodeResults    []struct {
		Node    string `json:"node"`
		Result  string `json:"result"`
		Message string `json:"message"`
	} `json:"nodeResults"`
}

// condition returns "Status/Reason" for the named condition, or "" when absent.
func (s nodeHealthStatusView) condition(typ string) string {
	for _, c := range s.Conditions {
		if c.Type == typ {
			return c.Status + "/" + c.Reason
		}
	}
	return ""
}

func nodeHealthStatus(name, ns string) (nodeHealthStatusView, error) {
	cmd := exec.Command("kubectl", "get", "nodehealthcheck", name, "-n", ns, "-o", "json")
	out, err := utils.Run(cmd)
	if err != nil {
		return nodeHealthStatusView{}, fmt.Errorf("kubectl get nodehealthcheck: %w", err)
	}
	var obj struct {
		Status nodeHealthStatusView `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return nodeHealthStatusView{}, fmt.Errorf("unmarshal nodehealthcheck: %w", err)
	}
	return obj.Status, nil
}

func dumpNodeHealthDiagnostics(name, ns string) {
	By("dumping controller-manager logs")
	cmd := exec.Command("kubectl", "logs", "-l", "control-plane=controller-manager", "-n", namespace, "--tail=200")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n%s\n", out)
	}

	By("dumping the node-agent DaemonSet, pods, and logs")
	cmd = exec.Command("kubectl", "get", "daemonset,pod", "-n", ns, "-l", "fathom.skaphos.io/source-name="+name, "-o", "wide")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "node-agent workloads:\n%s\n", out)
	}
	cmd = exec.Command("kubectl", "logs", "-l", "fathom.skaphos.io/source-name="+name+",fathom.skaphos.io/source-kind=NodeHealthCheck", "-n", ns, "--tail=100")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "node-agent logs:\n%s\n", out)
	}
	cmd = exec.Command("kubectl", "get", "events", "-n", ns, "--sort-by=.lastTimestamp")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "%s events:\n%s\n", ns, out)
	}

	By("dumping the NodeHealthCheck, report ConfigMaps, and HealthReports")
	cmd = exec.Command("kubectl", "get", "nodehealthcheck,configmap,healthreport", "-n", ns,
		"-l", "fathom.skaphos.io/source-name="+name, "-o", "yaml")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "NodeHealthCheck state:\n%s\n", out)
	}
	cmd = exec.Command("kubectl", "get", "nodehealthcheck", name, "-n", ns, "-o", "yaml")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "NodeHealthCheck object:\n%s\n", out)
	}
}
