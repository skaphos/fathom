/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"bytes"
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

// The fathomctl lifecycle verbs end to end (skaphos/fathom#330, spec 013).
// This runs only in the `lifecycle` shard (E2E_ADDONS=lifecycle), whose
// cluster has the images loaded under the names fathomctl installs but no
// pre-deployed operator: the verbs refuse a kustomize-managed one. Refusal
// paths (stops, scope, unsanctioned, acknowledgement) are unit-covered with
// synthetic declarations; here the real cluster proves ordering, cleanup and
// Helm interop.

const (
	lifecycleNS       = "fathom-system"
	lifecycleChecksNS = "lifecycle-e2e"
	lifecycleRelease  = "fathom"
	lifecycleCRDs     = 9
)

// lifecycleCLI is one fathomctl build at one version.
type lifecycleCLI struct {
	version string
	bin     string
}

func (c lifecycleCLI) run(args ...string) (string, error) {
	cmd := exec.Command(c.bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_, _ = fmt.Fprintf(GinkgoWriter, "running: fathomctl(%s) %s\n", c.version, strings.Join(args, " "))
	out, err := cmd.Output()
	if stderr.Len() > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "stderr:\n%s\n", stderr.String())
	}
	if err != nil {
		return string(out) + stderr.String(), fmt.Errorf("fathomctl %s: %w\n%s%s", strings.Join(args, " "), err, out, stderr.String())
	}
	return string(out), nil
}

// lifecycleVersions are the two versions the Taskfile tagged and loaded the
// images under (E2E_LIFECYCLE_VERSIONS).
func lifecycleVersions() (string, string) {
	v := strings.Fields(os.Getenv("E2E_LIFECYCLE_VERSIONS"))
	if len(v) != 2 {
		v = []string{"0.5.99-e2e.1", "0.5.99-e2e.2"}
	}
	return v[0], v[1]
}

func buildLifecycleCLI(dir, version string) lifecycleCLI {
	root, err := utils.GetProjectDir()
	Expect(err).NotTo(HaveOccurred())
	bin := filepath.Join(dir, "fathomctl-"+version)
	cmd := exec.Command("go", "build", "-o", bin,
		"-ldflags", "-X github.com/skaphos/fathom/internal/cli.Version=v"+version, "./cmd/fathomctl")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "build fathomctl %s:\n%s", version, out)
	return lifecycleCLI{version: version, bin: bin}
}

func kubectl(args ...string) (string, error) {
	return utils.Run(exec.Command("kubectl", args...))
}

func lifecycleCRDNames() []string {
	out, err := kubectl("get", "crd", "-o", "jsonpath={range .items[?(@.spec.group==\"fathom.skaphos.io\")]}{.metadata.name}{\"\\n\"}{end}")
	Expect(err).NotTo(HaveOccurred())
	return utils.GetNonEmptyLines(out)
}

// managedObjects lists every object carrying the operator's managed-by label.
func managedObjects() []string {
	out, err := kubectl("get", "ds,pod,sa,netpol,role,rolebinding,cm", "-A",
		"-l", "fathom.skaphos.io/managed-by=fathom", "-o", "name")
	Expect(err).NotTo(HaveOccurred())
	return utils.GetNonEmptyLines(out)
}

func applySampleChecks() {
	root, _ := utils.GetProjectDir()
	for _, sample := range []string{"fathom_v1alpha1_dnscheck.yaml", "fathom_v1alpha1_nodecertificatecheck.yaml"} {
		_, err := kubectl("apply", "-n", lifecycleChecksNS, "-f", filepath.Join(root, "config", "samples", sample))
		Expect(err).NotTo(HaveOccurred())
	}
}

func checksPresent() bool {
	_, errDNS := kubectl("get", "dnscheck", "cluster-dns", "-n", lifecycleChecksNS)
	_, errNCC := kubectl("get", "nodecertificatecheck", "node-certificates", "-n", lifecycleChecksNS)
	return errDNS == nil && errNCC == nil
}

// agentDaemonSets are the node-agent DaemonSets the operator made for checks.
func agentDaemonSets() []string {
	out, _ := kubectl("get", "ds", "-n", lifecycleChecksNS, "-l", "fathom.skaphos.io/managed-by=fathom", "-o", "name")
	return utils.GetNonEmptyLines(out)
}

var _ = Describe("fathomctl lifecycle", Ordered, Label(utils.LifecycleLabel), func() {
	var v1, v2 lifecycleCLI
	var tmp string

	BeforeAll(func() {
		tmp = GinkgoT().TempDir()
		ver1, ver2 := lifecycleVersions()
		v1 = buildLifecycleCLI(tmp, ver1)
		v2 = buildLifecycleCLI(tmp, ver2)
		_, err := kubectl("create", "namespace", lifecycleChecksNS)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		_, _ = v2.run("uninstall", "--purge-crds", "--delete-namespace", "--yes", "--timeout", "3m")
		_, _ = kubectl("delete", "namespace", lifecycleChecksNS, "--ignore-not-found", "--wait=false")
	})

	It("installs CRDs and the operator with one command", func() {
		out, err := v1.run("install", "--create-namespace", "--timeout", "5m")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("Installed Fathom v" + v1.version))
		Expect(lifecycleCRDNames()).To(HaveLen(lifecycleCRDs))
		helmOut, err := utils.Run(exec.Command("helm", "ls", "-n", lifecycleNS, "-o", "json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(helmOut).To(ContainSubstring(`"name":"` + lifecycleRelease + `"`))
	})

	It("reports a healthy install quickly", func() {
		start := time.Now()
		out, err := v1.run("status")
		Expect(err).NotTo(HaveOccurred())
		Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second), "status must answer in under 5s")
		Expect(out).To(ContainSubstring(fmt.Sprintf("CRDs:       %d/%d current", lifecycleCRDs, lifecycleCRDs)))
		Expect(out).To(ContainSubstring("Operator:   Available"))
	})

	It("is idempotent", func() {
		out, err := v1.run("install")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("nothing to do"))
	})

	It("upgrades CRDs before the new operator rolls out", func() {
		root, _ := utils.GetProjectDir()
		// kubectl replace: a foreign field manager now owns the older schema.
		_, err := kubectl("replace", "-f", filepath.Join(root, "test", "e2e", "fixtures", "lifecycle", "older-crds"))
		Expect(err).NotTo(HaveOccurred())
		_, err = v1.run("status")
		Expect(err).To(HaveOccurred(), "status must fail while a CRD is outdated")

		before, err := kubectl("get", "crd", "dnschecks.fathom.skaphos.io", "-o", "jsonpath={.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.historyLimit.type}")
		Expect(err).NotTo(HaveOccurred())
		Expect(before).To(BeEmpty())

		plan, err := v2.run("upgrade", "--dry-run")
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(MatchRegexp(`dnschecks\.fathom\.skaphos\.io\s+update`))
		after, _ := kubectl("get", "crd", "dnschecks.fathom.skaphos.io", "-o", "jsonpath={.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.historyLimit.type}")
		Expect(after).To(BeEmpty(), "--dry-run changed a CRD")

		_, err = v2.run("upgrade", "--timeout", "5m")
		Expect(err).NotTo(HaveOccurred())
		after, _ = kubectl("get", "crd", "dnschecks.fathom.skaphos.io", "-o", "jsonpath={.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.historyLimit.type}")
		Expect(after).To(Equal("integer"), "the removed property was not restored")

		// Every CRD write by fathomctl precedes the new manager ReplicaSet (SC-002).
		var latestCRDWrite time.Time
		for _, name := range lifecycleCRDNames() {
			version, _ := kubectl("get", "crd", name, "-o", "jsonpath={.metadata.annotations.fathom\\.skaphos\\.io/version}")
			Expect(version).To(Equal(v2.version), "%s not stamped", name)
			raw, err := kubectl("get", "crd", name, "-o", "json")
			Expect(err).NotTo(HaveOccurred())
			var crd struct {
				Metadata struct {
					ManagedFields []struct {
						Manager string    `json:"manager"`
						Time    time.Time `json:"time"`
					} `json:"managedFields"`
				} `json:"metadata"`
			}
			Expect(json.Unmarshal([]byte(raw), &crd)).To(Succeed())
			for _, mf := range crd.Metadata.ManagedFields {
				if mf.Manager == "fathomctl" && mf.Time.After(latestCRDWrite) {
					latestCRDWrite = mf.Time
				}
			}
		}
		Expect(latestCRDWrite.IsZero()).To(BeFalse())
		rsTimes, err := kubectl("get", "rs", "-n", lifecycleNS, "-l", "app.kubernetes.io/instance="+lifecycleRelease,
			"--sort-by=.metadata.creationTimestamp", "-o", "jsonpath={range .items[*]}{.metadata.creationTimestamp}{\"\\n\"}{end}")
		Expect(err).NotTo(HaveOccurred())
		lines := utils.GetNonEmptyLines(rsTimes)
		Expect(len(lines)).To(BeNumerically(">=", 2), "the upgrade did not roll out a new ReplicaSet")
		newest, err := time.Parse(time.RFC3339, lines[len(lines)-1])
		Expect(err).NotTo(HaveOccurred())
		Expect(latestCRDWrite.After(newest)).To(BeFalse(), "a CRD was written (%s) after the new ReplicaSet was created (%s)", latestCRDWrite, newest)

		out, err := v2.run("status")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(fmt.Sprintf("%d/%d current", lifecycleCRDs, lifecycleCRDs)))
	})

	It("uninstalls leaving nothing running and every resource kept", func() {
		applySampleChecks()
		Eventually(agentDaemonSets, 3*time.Minute, 2*time.Second).ShouldNot(BeEmpty(), "the operator never created a node-agent DaemonSet")

		out, err := v2.run("uninstall", "--timeout", "3m")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("Kept namespace " + lifecycleNS))
		Eventually(managedObjects, time.Minute, 2*time.Second).Should(BeEmpty())
		_, err = kubectl("get", "validatingadmissionpolicy", "fathom-node-report-authenticity")
		Expect(err).To(HaveOccurred(), "the admission policy survived uninstall")
		Expect(lifecycleCRDNames()).To(HaveLen(lifecycleCRDs))
		Expect(checksPresent()).To(BeTrue(), "a check resource was deleted")
		_, err = kubectl("get", "namespace", lifecycleNS)
		Expect(err).NotTo(HaveOccurred())
	})

	It("reinstalls onto the kept CRDs and picks the checks up", func() {
		_, err := v2.run("install", "--timeout", "5m")
		Expect(err).NotTo(HaveOccurred())
		Eventually(agentDaemonSets, 3*time.Minute, 2*time.Second).ShouldNot(BeEmpty(), "the reinstalled operator did not pick the checks up")
	})

	It("interoperates with helm in both directions (SC-007)", func() {
		root, _ := utils.GetProjectDir()
		_, err := utils.Run(exec.Command("helm", "upgrade", lifecycleRelease, filepath.Join(root, "deploy", "helm", "fathom-operator"),
			"-n", lifecycleNS, "--reuse-values", "--wait", "--timeout", "5m"))
		Expect(err).NotTo(HaveOccurred(), "helm could not upgrade a fathomctl-made release")
		status, _ := v2.run("status")
		Expect(status).To(ContainSubstring("helm release " + lifecycleRelease))

		_, err = v2.run("upgrade", "--timeout", "5m")
		Expect(err).NotTo(HaveOccurred(), "fathomctl could not upgrade a helm-upgraded release")
		history, err := utils.Run(exec.Command("helm", "history", lifecycleRelease, "-n", lifecycleNS, "-o", "json"))
		Expect(err).NotTo(HaveOccurred())
		var revisions []map[string]any
		Expect(json.Unmarshal([]byte(history), &revisions)).To(Succeed())
		Expect(len(revisions)).To(BeNumerically(">=", 3), "helm history must show the fathomctl and helm revisions")
	})

	It("purges with an export that restores after a reinstall", func() {
		export := filepath.Join(tmp, "fathom-backup.yaml")
		_, err := v2.run("uninstall", "--purge-crds", "--export", export, "--yes", "--timeout", "3m")
		Expect(err).NotTo(HaveOccurred())
		Expect(lifecycleCRDNames()).To(BeEmpty())
		raw, err := os.ReadFile(export)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).NotTo(ContainSubstring("ownerReferences"))
		exportedReports := strings.Count(string(raw), "kind: HealthReport")

		_, err = v2.run("install", "--timeout", "5m")
		Expect(err).NotTo(HaveOccurred())
		_, err = kubectl("apply", "-f", export)
		Expect(err).NotTo(HaveOccurred())
		Expect(checksPresent()).To(BeTrue())
		// Restored reports carry no owner references, so the garbage
		// collector must leave them alone (FR-032).
		countReports := func() int {
			out, _ := kubectl("get", "healthreports", "-n", lifecycleChecksNS, "-o", "name")
			return len(utils.GetNonEmptyLines(out))
		}
		Consistently(countReports, 20*time.Second, 2*time.Second).Should(BeNumerically(">=", exportedReports))
	})
})
