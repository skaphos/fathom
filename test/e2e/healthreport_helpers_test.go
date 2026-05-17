/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

// healthReport mirrors the subset of HealthReport JSON the e2e specs assert
// on. Living in the test package rather than reusing api/v1alpha1 keeps the
// specs independent of schema churn — kubectl's JSON output is the stable
// interface the operator is contracted to produce.
type healthReport struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Checks []checkResult `json:"checks"`
	} `json:"spec"`
}

type checkResult struct {
	Family    string            `json:"family"`
	Result    string            `json:"result"`
	Summary   string            `json:"summary"`
	Details   map[string]string `json:"details"`
	TargetRef struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Namespace  string `json:"namespace"`
		Name       string `json:"name"`
	} `json:"targetRef"`
}

type healthReportList struct {
	Items []healthReport `json:"items"`
}

type eventList struct {
	Items []struct {
		InvolvedObject struct {
			Kind      string `json:"kind"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"involvedObject"`
	} `json:"items"`
}

// latestHealthReport returns the most-recently-created HealthReport owned by
// the named AddonCheck, parsed into the subset the e2e specs care about.
func latestHealthReport(sourceName, ns string) (healthReport, error) {
	cmd := exec.Command("kubectl", "get", "healthreport",
		"-n", ns,
		"-l", "fathom.skaphos.io/source-name="+sourceName,
		"--sort-by=.metadata.creationTimestamp",
		"-o", "json",
	)
	out, err := utils.Run(cmd)
	if err != nil {
		return healthReport{}, fmt.Errorf("kubectl get healthreport: %w", err)
	}
	var list healthReportList
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return healthReport{}, fmt.Errorf("unmarshal healthreport list: %w", err)
	}
	if len(list.Items) == 0 {
		return healthReport{}, fmt.Errorf("no HealthReports yet for %s/%s", ns, sourceName)
	}
	return list.Items[len(list.Items)-1], nil
}

// findCheck returns the first CheckResult in the report matching the given
// family/kind/name triple, or nil if none match.
func findCheck(report healthReport, family, targetKind, targetName string) *checkResult {
	for i := range report.Spec.Checks {
		c := &report.Spec.Checks[i]
		if c.Family == family && c.TargetRef.Kind == targetKind && c.TargetRef.Name == targetName {
			return c
		}
	}
	return nil
}

// dumpAddonCheckDiagnostics prints state useful for triaging a failed
// AddonCheck spec: controller logs, events in the AddonCheck's namespace,
// and the AddonCheck + HealthReports themselves.
func dumpAddonCheckDiagnostics(sampleName, sampleNamespace string) {
	By("dumping controller-manager logs")
	cmd := exec.Command("kubectl", "logs", "-l", "control-plane=controller-manager",
		"-n", namespace, "--tail=200")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n%s\n", out)
	}

	By("dumping events in the AddonCheck namespace")
	cmd = exec.Command("kubectl", "get", "events", "-n", sampleNamespace,
		"--sort-by=.lastTimestamp")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "%s events:\n%s\n", sampleNamespace, out)
	}

	By("dumping the AddonCheck and any HealthReports")
	cmd = exec.Command("kubectl", "get", "addoncheck,healthreport",
		"-n", sampleNamespace,
		"-l", "fathom.skaphos.io/source-name="+sampleName,
		"-o", "yaml")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "AddonCheck + HealthReports:\n%s\n", out)
	}
}

// stopOnTerminalResult aborts an Eventually loop when a check has reached a
// terminal failure outcome. "Error" means the adapter could not complete the
// check and "Fail" means it observed an unhealthy state — neither will change
// without a new reconcile. Since the AddonCheck samples ship with
// interval: 5m and the e2e Eventually windows are 3m, a terminal result on
// the first reconcile guarantees the spec will hang until timeout. Calling
// StopTrying("...").Now() converts that hang into an immediate failure that
// reports the actual observed state.
//
// Warn is not terminal: a warn-now check may legitimately recover on a
// subsequent reconcile within the same window (e.g., a restart count that
// stabilises). Skipped is an intentional outcome and never terminal.
func stopOnTerminalResult(c *checkResult, family, targetLabel string) {
	if c == nil {
		return
	}
	switch c.Result {
	case "Error", "Fail":
		StopTrying(fmt.Sprintf(
			"%s check for %s returned terminal result %q (will not change before AddonCheck.spec.interval expires): %s",
			family, targetLabel, c.Result, c.Summary,
		)).Now()
	}
}

// addonCheckReadyTrue reports whether the named AddonCheck's status has a
// Ready=True condition. Returns an error if the AddonCheck can't be fetched
// or its JSON can't be parsed.
func addonCheckReadyTrue(name, ns string) (bool, error) {
	cmd := exec.Command("kubectl", "get", "addoncheck", name,
		"-n", ns,
		"-o", "json",
	)
	out, err := utils.Run(cmd)
	if err != nil {
		return false, fmt.Errorf("kubectl get addoncheck: %w", err)
	}
	var ac struct {
		Status struct {
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &ac); err != nil {
		return false, fmt.Errorf("unmarshal addoncheck: %w", err)
	}
	for _, c := range ac.Status.Conditions {
		if c.Type == "Ready" {
			return c.Status == "True", nil
		}
	}
	return false, nil
}
