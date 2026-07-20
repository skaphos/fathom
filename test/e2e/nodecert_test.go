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
	nodeCertSamplePath = "test/e2e/fixtures/nodecertificatecheck.yaml"
	nodeCertSampleName = "nodecert-e2e"
	nodeCertSampleNS   = "default"
	nodeCertDaemonSet  = "nodecert-e2e-node-agent"
)

// This suite exercises the on-disk certificate scanner end to end against a real
// kind node (SKA-49 / SKA-519): the operator must create a node-agent DaemonSet,
// the agent must read the node's real kubeadm certificates over a read-only
// hostPath mount and publish a report ConfigMap, and the operator must roll those
// up into a HealthReport with status parity — none of which envtest can prove.
var _ = Describe("NodeCertificateCheck", Ordered, Label(utils.CoreLabel), func() {
	BeforeAll(func() {
		By("clearing any prior NodeCertificateCheck state")
		cmd := exec.Command("kubectl", "delete", "-f", nodeCertSamplePath, "--ignore-not-found=true", "--wait=true")
		_, _ = utils.Run(cmd)

		By("applying the NodeCertificateCheck fixture")
		cmd = exec.Command("kubectl", "apply", "-f", nodeCertSamplePath)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply NodeCertificateCheck fixture")
	})

	AfterAll(func() {
		By("cleaning up the NodeCertificateCheck")
		cmd := exec.Command("kubectl", "delete", "-f", nodeCertSamplePath, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		dumpNodeCertDiagnostics(nodeCertSampleName, nodeCertSampleNS)
	})

	It("should roll out the node-agent DaemonSet on the (control-plane) node", func() {
		verify := func(g Gomega) {
			ds, err := daemonSetRollout(nodeCertDaemonSet, nodeCertSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch node-agent DaemonSet")
			g.Expect(ds.desired).To(BeNumerically(">", 0), "DaemonSet scheduled on no nodes (tolerations/selector?)")
			g.Expect(ds.ready).To(Equal(ds.desired), "node-agent DaemonSet not fully ready (%d/%d)", ds.ready, ds.desired)
			// Assert the rollout actually settled rather than flapping: every pod
			// updated to the current template and the controller observed the
			// current generation. Before SKA-589 the DaemonSet churned into a
			// perpetual rolling restart and these never converged (timeout).
			g.Expect(ds.updated).To(Equal(ds.desired), "node-agent DaemonSet still rolling (%d/%d updated)", ds.updated, ds.desired)
			g.Expect(ds.observedGeneration).To(Equal(ds.generation), "DaemonSet rollout has not converged (observedGeneration %d != generation %d)", ds.observedGeneration, ds.generation)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"node-agent DaemonSet did not become ready within timeout")
	})

	It("should produce a HealthReport with node_certificate Pass for the apiserver cert", func() {
		var latest healthReport
		verify := func(g Gomega) {
			report, err := latestHealthReport(nodeCertSampleName, nodeCertSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch latest HealthReport")
			latest = report

			g.Expect(report.Spec.Checks).NotTo(BeEmpty(), "HealthReport %q has no checks yet", report.Metadata.Name)
			var nodeCertChecks int
			for _, c := range report.Spec.Checks {
				if c.Family != "node_certificate" {
					continue
				}
				nodeCertChecks++
				g.Expect(c.TargetRef.Kind).To(Equal("Node"), "node_certificate check should target a Node")
				// All scanned files (apiserver.crt, ca.crt) are long-lived on a
				// fresh kind cluster, so every result must be Pass.
				g.Expect(c.Result).To(Equal("Pass"),
					"node_certificate check for %s on node %s: got %q (%s)",
					c.Details["path"], c.TargetRef.Name, c.Result, c.Summary)
			}
			g.Expect(nodeCertChecks).To(BeNumerically(">", 0), "no node_certificate checks in HealthReport")
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"node_certificate HealthReport did not reach Pass within timeout")

		By(fmt.Sprintf("observed HealthReport %q with node_certificate Pass", latest.Metadata.Name))
	})

	It("should mirror Ready=True and a reporting-node count into NodeCertificateCheck status", func() {
		verify := func(g Gomega) {
			status, err := nodeCertStatus(nodeCertSampleName, nodeCertSampleNS)
			g.Expect(err).NotTo(HaveOccurred(), "failed to fetch NodeCertificateCheck status")
			g.Expect(status.readyTrue()).To(BeTrue(), "NodeCertificateCheck did not reach Ready=True")
			g.Expect(status.ReportingNodes).To(BeNumerically(">", 0), "no reporting nodes")
			g.Expect(status.LastReportName).NotTo(BeEmpty(), "status.lastReportName not set")
			g.Expect(status.LastResult).To(Equal("Pass"), "status.lastResult = %q", status.LastResult)
		}
		Eventually(verify, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})

type dsRollout struct {
	desired            int
	ready              int
	updated            int
	generation         int64
	observedGeneration int64
}

func daemonSetRollout(name, ns string) (dsRollout, error) {
	cmd := exec.Command("kubectl", "get", "daemonset", name, "-n", ns, "-o", "json")
	out, err := utils.Run(cmd)
	if err != nil {
		return dsRollout{}, fmt.Errorf("kubectl get daemonset: %w", err)
	}
	var ds struct {
		Metadata struct {
			Generation int64 `json:"generation"`
		} `json:"metadata"`
		Status struct {
			DesiredNumberScheduled int   `json:"desiredNumberScheduled"`
			NumberReady            int   `json:"numberReady"`
			UpdatedNumberScheduled int   `json:"updatedNumberScheduled"`
			ObservedGeneration     int64 `json:"observedGeneration"`
		} `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &ds); err != nil {
		return dsRollout{}, fmt.Errorf("unmarshal daemonset: %w", err)
	}
	return dsRollout{
		desired:            ds.Status.DesiredNumberScheduled,
		ready:              ds.Status.NumberReady,
		updated:            ds.Status.UpdatedNumberScheduled,
		generation:         ds.Metadata.Generation,
		observedGeneration: ds.Status.ObservedGeneration,
	}, nil
}

type nodeCertStatusView struct {
	Conditions []struct {
		Type   string `json:"type"`
		Status string `json:"status"`
	} `json:"conditions"`
	LastResult     string `json:"lastResult"`
	LastReportName string `json:"lastReportName"`
	ReportingNodes int    `json:"reportingNodes"`
}

func (s nodeCertStatusView) readyTrue() bool {
	for _, c := range s.Conditions {
		if c.Type == "Ready" {
			return c.Status == "True"
		}
	}
	return false
}

func nodeCertStatus(name, ns string) (nodeCertStatusView, error) {
	cmd := exec.Command("kubectl", "get", "nodecertificatecheck", name, "-n", ns, "-o", "json")
	out, err := utils.Run(cmd)
	if err != nil {
		return nodeCertStatusView{}, fmt.Errorf("kubectl get nodecertificatecheck: %w", err)
	}
	var obj struct {
		Status nodeCertStatusView `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return nodeCertStatusView{}, fmt.Errorf("unmarshal nodecertificatecheck: %w", err)
	}
	return obj.Status, nil
}

func dumpNodeCertDiagnostics(name, ns string) {
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
	cmd = exec.Command("kubectl", "logs", "-l", "fathom.skaphos.io/source-name="+name, "-n", ns, "--tail=100")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "node-agent logs:\n%s\n", out)
	}

	By("dumping the NodeCertificateCheck, report ConfigMaps, and HealthReports")
	cmd = exec.Command("kubectl", "get", "nodecertificatecheck,configmap,healthreport", "-n", ns,
		"-l", "fathom.skaphos.io/source-name="+name, "-o", "yaml")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "NodeCertificateCheck state:\n%s\n", out)
	}
}
