/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package utils provides shared helpers for the e2e Ginkgo suite.
package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2" // nolint:revive,staticcheck
)

const (
	prometheusOperatorVersion = "v0.77.1"
	prometheusOperatorURL     = "https://github.com/prometheus-operator/prometheus-operator/" +
		"releases/download/%s/bundle.yaml"

	// E2EHelmfile is the project-relative path to the canonical addon-stack
	// helmfile used by the e2e suite. Kept as a const so tests and helpers
	// agree on the single source of truth for chart versions and values.
	E2EHelmfile = "test/e2e/fixtures/helmfile.yaml"

	// EnvAddons is the environment variable that selects which slice of the
	// tiered e2e addon stack a run targets (skaphos/fathom#178). See
	// ParseAddonSelection for the accepted values.
	EnvAddons = "E2E_ADDONS"

	// CoreLabel is the Ginkgo label carried by every spec in the core tier:
	// operator-infrastructure specs (Manager, RBAC impersonation,
	// NodeCertificateCheck, refresh-on-change) plus the specs of the core-tier
	// addons. It doubles as the E2E_ADDONS keyword that selects that tier.
	CoreLabel = "core"
)

// coreAddons are the addons whose charts carry the `tier: core` label in
// test/e2e/fixtures/helmfile.yaml (or, for CoreDNS, ship preinstalled with
// kind). They are installed on every stack sync regardless of E2E_ADDONS, and
// their specs carry both the "core" label and their own addon label.
var coreAddons = []string{"cilium", "coredns", "cert-manager", "external-secrets"}

// optInAddons are the addons installed only when named in E2E_ADDONS (each
// helmfile release carries an `addon: <name>` label and no `tier: core`).
// Growing this list is how a new non-core adapter joins the tiered stack; the
// drift guards in utils_test.go and scripts/e2e_shards_gate_test.go enforce
// that the helmfile labels and the CI shard planner stay in sync with it.
var optInAddons = []string{"external-dns", "metrics-server", "envoy-gateway", "istio", "argocd"}

// CoreAddons returns the addons in the always-on core tier.
func CoreAddons() []string { return append([]string(nil), coreAddons...) }

// OptInAddons returns the addons installed only when explicitly selected.
func OptInAddons() []string { return append([]string(nil), optInAddons...) }

// AddonSelection is a parsed E2E_ADDONS value: the slice of the tiered addon
// stack one e2e run installs and asserts against. The zero value selects the
// full stack.
type AddonSelection struct {
	// addons holds the normalized selection; nil means the full stack.
	addons []string
}

// ParseAddonSelection parses an E2E_ADDONS value. Accepted forms:
//
//   - "" or "all": the full stack — every release, every spec.
//   - "core": the core tier only.
//   - a comma-separated list of addon names (core-tier or opt-in), each of
//     which runs only that addon's specs; opt-in names additionally install
//     that addon's charts on top of the always-installed core tier.
//
// "core" may appear in the list alongside addon names. Unknown names are an
// error so a typo fails the run loudly instead of filtering it down to zero
// specs and reporting a hollow success.
func ParseAddonSelection(spec string) (AddonSelection, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "all" {
		return AddonSelection{}, nil
	}
	known := map[string]bool{CoreLabel: true}
	for _, a := range append(CoreAddons(), OptInAddons()...) {
		known[a] = true
	}
	var addons []string
	seen := map[string]bool{}
	for _, raw := range strings.Split(spec, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if !known[name] {
			return AddonSelection{}, fmt.Errorf(
				"unknown addon %q in %s=%q (known: %s, %s, all)",
				name, EnvAddons, spec, CoreLabel,
				strings.Join(append(CoreAddons(), OptInAddons()...), ", "))
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		addons = append(addons, name)
	}
	if addons == nil {
		return AddonSelection{}, fmt.Errorf("%s=%q selects nothing", EnvAddons, spec)
	}
	return AddonSelection{addons: addons}, nil
}

// HelmfileSelectors returns the helmfile --selector values that install this
// selection: nil for the full stack, otherwise `tier=core` (the core tier is
// always installed — Cilium is the cluster CNI, nothing schedules without it)
// plus one `addon=<name>` per selected opt-in addon.
func (s AddonSelection) HelmfileSelectors() []string {
	if s.addons == nil {
		return nil
	}
	optIn := map[string]bool{}
	for _, a := range optInAddons {
		optIn[a] = true
	}
	selectors := []string{"tier=core"}
	for _, a := range s.addons {
		if optIn[a] {
			selectors = append(selectors, "addon="+a)
		}
	}
	return selectors
}

// LabelFilter returns the Ginkgo label-filter expression that runs exactly the
// selected specs, or "" (no filter) for the full stack. Addon names double as
// spec labels, and "core" selects the operator-infrastructure specs plus the
// core-tier addon specs.
func (s AddonSelection) LabelFilter() string {
	return strings.Join(s.addons, " || ")
}

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %q\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%q failed with error %q: %w", command, string(output), err)
	}

	return string(output), nil
}

// SyncAddons applies the addon stack defined by test/e2e/fixtures/helmfile.yaml
// against the current kubeconfig context, restricted to the given helmfile
// selectors (none = the full stack; see AddonSelection.HelmfileSelectors).
// The call is idempotent: re-running against a cluster that already has the
// addons installed is a no-op for helmfile, so the Ginkgo suite can safely
// call this from BeforeSuite even when the Taskfile's e2e:cluster:addons step
// has already run.
//
// Requires `helmfile` and `helm` on PATH. Returns the combined stdout/stderr
// of the helmfile invocation alongside any error.
func SyncAddons(selectors ...string) (string, error) {
	projectDir, err := GetProjectDir()
	if err != nil {
		return "", err
	}
	args := []string{"-f", filepath.Join(projectDir, E2EHelmfile)}
	for _, sel := range selectors {
		args = append(args, "--selector", sel)
	}
	args = append(args, "sync")
	cmd := exec.Command("helmfile", args...)
	return Run(cmd)
}

// InstallPrometheusOperator installs the prometheus Operator to be used to export the enabled metrics.
func InstallPrometheusOperator() error {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "create", "-f", url)
	_, err := Run(cmd)
	return err
}

// UninstallPrometheusOperator uninstalls the prometheus
func UninstallPrometheusOperator() {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// IsPrometheusCRDsInstalled checks if any Prometheus CRDs are installed
// by verifying the existence of key CRDs related to Prometheus.
func IsPrometheusCRDsInstalled() bool {
	// List of common Prometheus CRDs
	prometheusCRDs := []string{
		"prometheuses.monitoring.coreos.com",
		"prometheusrules.monitoring.coreos.com",
		"prometheusagents.monitoring.coreos.com",
	}

	cmd := exec.Command("kubectl", "get", "crds", "-o", "custom-columns=NAME:.metadata.name")
	output, err := Run(cmd)
	if err != nil {
		return false
	}
	crdList := GetNonEmptyLines(output)
	for _, crd := range prometheusCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, fmt.Errorf("failed to get current working directory: %w", err)
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}

// UncommentCode searches for target in the file and remove the comment prefix
// of the target content. The target content may span multiple lines.
func UncommentCode(filename, target, prefix string) error {
	// false positive
	// nolint:gosec
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", filename, err)
	}
	strContent := string(content)

	idx := strings.Index(strContent, target)
	if idx < 0 {
		return fmt.Errorf("unable to find the code %q to be uncomment", target)
	}

	out := new(bytes.Buffer)
	_, err = out.Write(content[:idx])
	if err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(target))
	if !scanner.Scan() {
		return nil
	}
	for {
		if _, err = out.WriteString(strings.TrimPrefix(scanner.Text(), prefix)); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
		// Avoid writing a newline in case the previous line was the last in target.
		if !scanner.Scan() {
			break
		}
		if _, err = out.WriteString("\n"); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
	}

	if _, err = out.Write(content[idx+len(target):]); err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	// false positive
	// nolint:gosec
	if err = os.WriteFile(filename, out.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file %q: %w", filename, err)
	}

	return nil
}
