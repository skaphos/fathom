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
// images under (E2E_LIFECYCLE_VERSIONS), by default pre-releases of the
// chart's appVersion so a release bump never makes the chart look newer
// than the CLI.
func lifecycleVersions() (string, string) {
	// The Taskfile appends the upgrade stop as a third version (see
	// upgradeStopVersion); the first two are this suite's pair.
	if v := strings.Fields(os.Getenv("E2E_LIFECYCLE_VERSIONS")); len(v) >= 2 {
		return v[0], v[1]
	}
	root, err := utils.GetProjectDir()
	Expect(err).NotTo(HaveOccurred())
	raw, err := os.ReadFile(filepath.Join(root, "deploy", "helm", "fathom-operator", "Chart.yaml"))
	Expect(err).NotTo(HaveOccurred())
	var app string
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, ok := strings.CutPrefix(line, "appVersion:"); ok {
			app = strings.Trim(strings.Fields(rest)[0], `"`)
		}
	}
	Expect(app).NotTo(BeEmpty(), "Chart.yaml has no appVersion")
	return app + "-e2e.1", app + "-e2e.2"
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
	for _, sample := range []string{"fathom_v1_dnscheck.yaml", "fathom_v1_nodecertificatecheck.yaml"} {
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
			// kubectl omits managedFields from -o json unless asked.
			raw, err := kubectl("get", "crd", name, "-o", "json", "--show-managed-fields")
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
		Expect(latestCRDWrite.IsZero()).To(BeFalse(), "no CRD has a fathomctl managedFields entry")
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
		// Package the chart at the CLI's version, as the release workflow does,
		// so the release stays at the version the loaded images carry.
		_, err := utils.Run(exec.Command("helm", "package", filepath.Join(root, "deploy", "helm", "fathom-operator"),
			"--version", v2.version, "--app-version", v2.version, "-d", tmp))
		Expect(err).NotTo(HaveOccurred())
		pkg := filepath.Join(tmp, "fathom-operator-"+v2.version+".tgz")
		_, err = utils.Run(exec.Command("helm", "upgrade", lifecycleRelease, pkg,
			"-n", lifecycleNS, "--reuse-values", "--wait", "--timeout", "5m"))
		Expect(err).NotTo(HaveOccurred(), "helm could not upgrade a fathomctl-made release")
		status, err := v2.run("status")
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(ContainSubstring("helm release " + lifecycleRelease))

		// A value change makes this a real fathomctl revision on top of helm's.
		_, err = v2.run("upgrade", "--set", "leaderElect=true", "--timeout", "5m")
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

// upgradeStopVersion is the newest release declared in the chart's
// upgrade-path.yaml: the stop the alpha-to-v1 scenario upgrades to. The CLI
// for that scenario is built at exactly this version, because a
// `<appVersion>-e2e.N` build sorts below it and the 0.7.0 sanctions, matched
// by A < entry <= B, would never apply (spec 014, research R14).
func upgradeStopVersion() string {
	root, err := utils.GetProjectDir()
	Expect(err).NotTo(HaveOccurred())
	raw, err := os.ReadFile(filepath.Join(root, "deploy", "helm", "fathom-operator", "upgrade-path.yaml"))
	Expect(err).NotTo(HaveOccurred())
	version, stop := "", ""
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trimmed, "- version:"); ok {
			version = strings.TrimSpace(rest)
		}
		if trimmed == "stop: true" {
			stop = version
		}
	}
	Expect(stop).NotTo(BeEmpty(), "upgrade-path.yaml declares no stop release; the alpha-to-v1 scenario has nothing to upgrade to")
	return stop
}

const (
	v1UpgradeChart     = "oci://ghcr.io/skaphos/charts/fathom-operator"
	v1UpgradeFrom      = "0.6.0"
	v1UpgradeChecksNS  = "v1-upgrade-e2e"
	v1UpgradeAggregate = "platform"
)

// The alpha-to-v1 upgrade (spec 014, FR-022, quickstart §3): a genuine 0.6.0
// install from the published chart, with alpha resources and report history,
// is exported, replaced and recreated by the stop release's fathomctl. It
// runs in the lifecycle shard, needs GHCR for the 0.6.0 chart and images, and
// skips with a clear message when the registry is unreachable.
var _ = Describe("fathomctl alpha-to-v1 upgrade", Ordered, Label(utils.LifecycleLabel), func() {
	var cli lifecycleCLI
	var tmp, export string

	v1UpgradeCRD := func(kind string) string { return strings.ToLower(kind) + "s.fathom.skaphos.io" }
	servedVersions := func(kind string) string {
		out, err := kubectl("get", "crd", v1UpgradeCRD(kind), "-o", "jsonpath={.spec.versions[*].name}")
		Expect(err).NotTo(HaveOccurred())
		return strings.TrimSpace(out)
	}

	BeforeAll(func() {
		if _, err := utils.Run(exec.Command("helm", "show", "chart", v1UpgradeChart, "--version", v1UpgradeFrom)); err != nil {
			Skip(fmt.Sprintf("the published %s chart %s is unreachable (no registry access?): %v", v1UpgradeFrom, v1UpgradeChart, err))
		}
		tmp = GinkgoT().TempDir()
		export = filepath.Join(tmp, "fathom-v1.yaml")
		cli = buildLifecycleCLI(tmp, upgradeStopVersion())
		_, err := kubectl("create", "namespace", v1UpgradeChecksNS)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterAll(func() {
		if cli.bin != "" {
			_, _ = cli.run("uninstall", "--purge-crds", "--delete-namespace", "--yes", "--timeout", "3m")
		}
		_, _ = kubectl("delete", "namespace", v1UpgradeChecksNS, "--ignore-not-found", "--wait=false")
	})

	It("starts from the published "+v1UpgradeFrom+" release with alpha resources and history", func() {
		_, err := utils.Run(exec.Command("helm", "install", lifecycleRelease, v1UpgradeChart, "--version", v1UpgradeFrom,
			"-n", lifecycleNS, "--create-namespace", "--wait", "--timeout", "5m"))
		Expect(err).NotTo(HaveOccurred(), "helm could not install the published %s chart", v1UpgradeFrom)
		Expect(servedVersions("AddonCheck")).To(Equal("v1alpha1"), "the %s chart must serve the alpha version", v1UpgradeFrom)

		root, _ := utils.GetProjectDir()
		_, err = kubectl("apply", "-n", v1UpgradeChecksNS, "-f", filepath.Join(root, "test", "e2e", "fixtures", "lifecycle", "v0.6.0-samples"))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() int {
			out, _ := kubectl("get", "healthreports", "-n", v1UpgradeChecksNS, "-o", "name")
			return len(utils.GetNonEmptyLines(out))
		}, 5*time.Minute, 5*time.Second).Should(BeNumerically(">", 0), "the %s operator produced no report history", v1UpgradeFrom)
	})

	It("exports the alpha checks as v1 manifests without touching the cluster", func() {
		out, err := cli.run("export", "--output", export)
		Expect(err).NotTo(HaveOccurred(), out)
		raw, err := os.ReadFile(export)
		Expect(err).NotTo(HaveOccurred())
		text := string(raw)
		// Top-level documents only: the HealthCheck's nested checkRef.apiVersion
		// is indented and must not be counted.
		documents := 0
		for _, line := range strings.Split(text, "\n") {
			if line == "apiVersion: fathom.skaphos.io/v1" {
				documents++
			}
		}
		Expect(documents).To(Equal(5), "one v1 document per alpha check:\n%s", text)
		Expect(text).NotTo(ContainSubstring("v1alpha1"))
		Expect(text).NotTo(ContainSubstring("paused"))
		Expect(text).NotTo(ContainSubstring("kind: HealthReport"))
		Expect(text).NotTo(ContainSubstring("status:"))
		Expect(text).NotTo(ContainSubstring("resourceVersion"))
		Expect(servedVersions("AddonCheck")).To(Equal("v1alpha1"), "export must change nothing")
	})

	It("refuses the upgrade with the inventory until the removal is acknowledged", func() {
		out, err := cli.run("upgrade", "-n", lifecycleNS)
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("acknowledgement-required"))
		Expect(out).To(ContainSubstring("replace crd/" + v1UpgradeCRD("AddonCheck") + " (v1alpha1 -> v1): 1 objects will be removed"))
		Expect(out).To(ContainSubstring("  " + v1UpgradeChecksNS + "/coredns-sample"))
		Expect(out).To(ContainSubstring("and their HealthReport history"))
		Expect(servedVersions("AddonCheck")).To(Equal("v1alpha1"), "a refused upgrade must change nothing")
		status, _ := cli.run("status", "-n", lifecycleNS)
		Expect(status).To(ContainSubstring("serves v1alpha1; run fathomctl export, then fathomctl upgrade"), "status names the procedure while a CRD still serves alpha")
	})

	It("replaces the seven CRDs with the flag and rolls the new operator out", func() {
		out, err := cli.run("upgrade", "-n", lifecycleNS, "--accept-incompatible-crds", "--timeout", "5m")
		Expect(err).NotTo(HaveOccurred(), out)
		Expect(out).To(ContainSubstring("replaced crd/" + v1UpgradeCRD("AddonCheck") + ": removed 1 objects"))
		for _, kind := range []string{"AddonCheck", "HealthCheck", "ClusterHealth", "HealthReport", "NodeCertificateCheck", "DNSCheck", "NodeHealthCheck"} {
			Expect(servedVersions(kind)).To(Equal("v1"), "%s", kind)
			stored, _ := kubectl("get", "crd", v1UpgradeCRD(kind), "-o", "jsonpath={.status.storedVersions[*]}")
			Expect(strings.TrimSpace(stored)).To(Equal("v1"), "%s storedVersions", kind)
		}
		for _, kind := range []string{"AddonDefinition", "AddonDefinitionBinding"} {
			Expect(servedVersions(kind)).To(Equal("v1alpha1"), "%s must stay alpha", kind)
		}
		checks, _ := kubectl("get", "dnscheck,addoncheck,healthcheck,nodecertificatecheck", "-n", v1UpgradeChecksNS, "-o", "name")
		Expect(utils.GetNonEmptyLines(checks)).To(BeEmpty(), "alpha resources do not survive the replacement")
		reports, _ := kubectl("get", "healthreports", "-n", v1UpgradeChecksNS, "-o", "name")
		Expect(utils.GetNonEmptyLines(reports)).To(BeEmpty(), "alpha report history goes with its CRD")
	})

	It("recreates the checks from the export and they reach a verdict", func() {
		_, err := kubectl("apply", "-f", export)
		Expect(err).NotTo(HaveOccurred(), "the exported manifests must admit against v1 unchanged")
		Eventually(func(g Gomega) {
			out, err := kubectl("get", "dnscheck", "cluster-dns", "-n", v1UpgradeChecksNS, "-o", "jsonpath={.status.lastResult}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).NotTo(BeEmpty())
		}, 5*time.Minute, 5*time.Second).Should(Succeed(), "the recreated DNSCheck never ran")
		Eventually(func(g Gomega) {
			out, err := kubectl("get", "clusterhealth", v1UpgradeAggregate, "-o", "jsonpath={.status.result}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).NotTo(BeEmpty())
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "the recreated ClusterHealth never aggregated")
	})

	It("keeps a deleted check's reports for the window, then prunes them with a metric and an Event", func() {
		// A short window and a tiny cap so pruning is observed inside the run
		// (quickstart §4); a values-only upgrade at the same version.
		out, err := cli.run("upgrade", "-n", lifecycleNS, "--timeout", "5m",
			"--set", "config.enabled=true", "--set", "config.data.retention.report_window=90s", "--set", "config.data.retention.report_cap_per_kind=5")
		Expect(err).NotTo(HaveOccurred(), out)

		before, err := kubectl("get", "healthreports", "-n", v1UpgradeChecksNS, "-l", "fathom.skaphos.io/source-name=cluster-dns", "-o", "name")
		Expect(err).NotTo(HaveOccurred())
		Expect(utils.GetNonEmptyLines(before)).NotTo(BeEmpty(), "the recreated DNSCheck produced no history to retain")
		_, err = kubectl("delete", "dnscheck", "cluster-dns", "-n", v1UpgradeChecksNS)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			out, err := kubectl("get", "healthreports", "-n", v1UpgradeChecksNS, "-l", "fathom.skaphos.io/source-name=cluster-dns",
				"-o", `jsonpath={range .items[*]}{.metadata.annotations.fathom\.skaphos\.io/source-deleted-at}{"\n"}{end}`)
			g.Expect(err).NotTo(HaveOccurred())
			stamps := utils.GetNonEmptyLines(out)
			g.Expect(stamps).To(HaveLen(len(utils.GetNonEmptyLines(before))), "every report of the deleted check is stamped, none deleted yet")
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		listing, err := cli.run("reports", "dnscheck/cluster-dns", "-n", v1UpgradeChecksNS)
		Expect(err).NotTo(HaveOccurred(), listing)
		Expect(listing).To(ContainSubstring("retained (check deleted"))

		Eventually(func(g Gomega) {
			out, _ := kubectl("get", "healthreports", "-n", v1UpgradeChecksNS, "-l", "fathom.skaphos.io/source-name=cluster-dns", "-o", "name")
			g.Expect(utils.GetNonEmptyLines(out)).To(BeEmpty(), "the window elapsed; the reports must be gone")
		}, 5*time.Minute, 10*time.Second).Should(Succeed())
		events, err := kubectl("get", "events", "-n", v1UpgradeChecksNS, "--field-selector", "reason=ReportsPruned", "-o", "jsonpath={.items[*].message}")
		Expect(err).NotTo(HaveOccurred())
		Expect(events).To(ContainSubstring("pruned"))
		Expect(events).To(ContainSubstring("window:"))
	})

	It("has nothing left to export and a clean status", func() {
		out, err := cli.run("export")
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("nothing to export"))
		status, err := cli.run("status", "-n", lifecycleNS)
		Expect(err).NotTo(HaveOccurred(), status)
		Expect(status).NotTo(ContainSubstring("serves v1alpha1"))
		Expect(status).To(ContainSubstring(fmt.Sprintf("CRDs:       %d/%d current", lifecycleCRDs, lifecycleCRDs)))
	})
})
