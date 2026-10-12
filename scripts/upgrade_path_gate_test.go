/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"
	"sigs.k8s.io/yaml"
)

// The upgrade-path declaration is what every later fathomctl reads to decide
// which upgrades it may perform (skaphos/fathom#330, contracts/upgrade-path.md).
// It must not fall behind the crd-compat allowlist, and released entries must
// never change, or a CLI built later would plan against rewritten history.

const upgradePathFile = "deploy/helm/fathom-operator/upgrade-path.yaml"

type upgradePathDoc struct {
	Floor    string `json:"floor"`
	Releases []struct {
		Version    string `json:"version"`
		Stop       bool   `json:"stop,omitempty"`
		Sanctioned []struct {
			CRD    string `json:"crd"`
			Path   string `json:"path"`
			Reason string `json:"reason"`
			Issue  string `json:"issue"`
		} `json:"sanctioned,omitempty"`
	} `json:"releases"`
}

func parseUpgradePathDoc(t *testing.T, raw []byte) upgradePathDoc {
	t.Helper()
	var doc upgradePathDoc
	if err := yaml.UnmarshalStrict(raw, &doc); err != nil {
		t.Fatalf("parse upgrade-path declaration: %v", err)
	}
	return doc
}

func readUpgradePath(t *testing.T) upgradePathDoc {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", upgradePathFile))
	if err != nil {
		t.Fatalf("read %s: %v", upgradePathFile, err)
	}
	return parseUpgradePathDoc(t, raw)
}

// latestReleaseTag is the highest v* tag, as the crd-compat gate picks it.
func latestReleaseTag(t *testing.T) (string, *semver.Version) {
	t.Helper()
	out, err := exec.Command("git", "tag", "--list", "v*", "--sort=-v:refname").Output()
	if err != nil {
		t.Skipf("git tags unavailable: %v", err)
	}
	tag := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if tag == "" {
		t.Skip("no v* release tag; nothing is released yet")
	}
	return tag, semver.MustParse(strings.TrimPrefix(tag, "v"))
}

func TestUpgradePathDeclarationIsWellFormed(t *testing.T) {
	doc := readUpgradePath(t)
	if _, err := semver.StrictNewVersion(doc.Floor); err != nil {
		t.Fatalf("floor %q: %v", doc.Floor, err)
	}
	var prev *semver.Version
	for i, r := range doc.Releases {
		v, err := semver.StrictNewVersion(r.Version)
		if err != nil {
			t.Fatalf("releases[%d].version %q: %v", i, r.Version, err)
		}
		if prev != nil && !v.GreaterThan(prev) {
			t.Errorf("releases[%d].version %s does not increase on %s", i, v, prev)
		}
		prev = v
		for j, s := range r.Sanctioned {
			if s.CRD == "" || s.Path == "" || s.Reason == "" || s.Issue == "" {
				t.Errorf("releases[%d].sanctioned[%d] needs crd, path, reason and issue: %+v", i, j, s)
			}
		}
	}
}

// Released entries are append-only: anything at or below the latest tag must
// match that tag's copy exactly.
func TestUpgradePathReleasedEntriesAreUnchanged(t *testing.T) {
	tag, tagVersion := latestReleaseTag(t)
	raw, err := exec.Command("git", "show", tag+":"+upgradePathFile).Output()
	if err != nil {
		t.Skipf("%s has no %s (it predates the declaration)", tag, upgradePathFile)
	}
	released := parseUpgradePathDoc(t, raw)
	current := readUpgradePath(t)
	if current.Floor != released.Floor {
		t.Errorf("floor changed from %s (at %s) to %s; it is never changed once released", released.Floor, tag, current.Floor)
	}
	byVersion := map[string]any{}
	for _, r := range current.Releases {
		byVersion[r.Version] = r
	}
	for _, r := range released.Releases {
		if semver.MustParse(r.Version).GreaterThan(tagVersion) {
			continue
		}
		got, ok := byVersion[r.Version]
		if !ok {
			t.Errorf("released entry %s was removed", r.Version)
			continue
		}
		if !reflect.DeepEqual(got, any(r)) {
			t.Errorf("released entry %s was edited:\n got %+v\nwant %+v", r.Version, got, r)
		}
	}
}

// sanctionedFindings runs the crd-compat gate and returns its SANCTIONED
// findings as "crd path". The report prints each CRD as a heading line and
// its findings indented beneath it.
func sanctionedFindings(t *testing.T, env map[string]string) []string {
	t.Helper()
	ok, out := runCRDCompat(t, env)
	if !ok {
		t.Fatalf("crd-compat gate failed; fix it first:\n%s", out)
	}
	return parseSanctioned(out)
}

func parseSanctioned(report string) []string {
	var findings []string
	crd := ""
	sc := bufio.NewScanner(strings.NewReader(report))
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, "finding(s)"):
			crd = strings.SplitN(trimmed, ":", 2)[0]
		case strings.HasPrefix(trimmed, "SANCTIONED "):
			path := strings.Fields(strings.TrimPrefix(trimmed, "SANCTIONED "))[0]
			findings = append(findings, crd+" "+path)
		}
	}
	return findings
}

func TestParseSanctionedReadsTheGateReport(t *testing.T) {
	report := `check-crd-compat: baseline is release tag v0.5.1
  OK addonchecks.fathom.skaphos.io: compatible
  clusterhealths.fathom.skaphos.io: 1 finding(s)
    SANCTIONED ^.status.children (maxItems): capped — https://example.test/277
  healthchecks.fathom.skaphos.io: 2 finding(s)
    SANCTIONED ^.spec.checkRef.apiVersion (maxLength): bounded — https://example.test/267
    INCOMPATIBLE ^.spec.other (type):
check-crd-compat: OK`
	got := parseSanctioned(report)
	want := []string{"clusterhealths.fathom.skaphos.io ^.status.children", "healthchecks.fathom.skaphos.io ^.spec.checkRef.apiVersion"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseSanctioned = %v, want %v", got, want)
	}
}

// Every change the crd-compat gate sanctions for the upcoming release is
// recorded in the declaration's single unreleased entry, so fathomctl can
// tell users about it and require --accept-incompatible-crds.
func TestUpgradePathRecordsEverySanctionedFinding(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the crd-compat gate")
	}
	_, tagVersion := latestReleaseTag(t)
	doc := readUpgradePath(t)
	unreleased := map[string]bool{}
	var unreleasedVersions []string
	for _, r := range doc.Releases {
		if !semver.MustParse(r.Version).GreaterThan(tagVersion) {
			continue
		}
		unreleasedVersions = append(unreleasedVersions, r.Version)
		for _, s := range r.Sanctioned {
			unreleased[s.CRD+" "+s.Path] = true
			unreleased[s.CRD+" *"] = true
		}
	}
	if len(unreleasedVersions) > 1 {
		t.Errorf("more than one unreleased entry (%v); the upcoming release has exactly one", unreleasedVersions)
	}
	for _, f := range sanctionedFindings(t, nil) {
		crd := strings.SplitN(f, " ", 2)[0]
		if !unreleased[f] && !unreleased[crd+" *"] {
			t.Errorf("crd-compat sanctions %q but %s has no unreleased entry for it; add it (RELEASE.md, upgrade-path upkeep)", f, upgradePathFile)
		}
	}
}

// In a release PR, release-please has moved the manifest ahead of the latest
// tag: the unreleased entry, if any, must be for exactly that version.
func TestUpgradePathUnreleasedEntryMatchesTheReleasePR(t *testing.T) {
	_, tagVersion := latestReleaseTag(t)
	raw, err := os.ReadFile(filepath.Join("..", ".release-please-manifest.json"))
	if err != nil {
		t.Fatalf("read release-please manifest: %v", err)
	}
	var manifest map[string]string
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode release-please manifest: %v", err)
	}
	next := semver.MustParse(manifest["."])
	if !next.GreaterThan(tagVersion) {
		t.Skip("not a release PR: the manifest records the last release")
	}
	for _, r := range readUpgradePath(t).Releases {
		v := semver.MustParse(r.Version)
		if v.GreaterThan(tagVersion) && !v.Equal(next) {
			t.Errorf("unreleased entry %s does not match the release being cut (%s)", r.Version, next)
		}
	}
}
