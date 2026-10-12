/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"

	helmchart "github.com/skaphos/fathom/deploy/helm"
)

const testUpgradePath = `
floor: 0.5.0
releases:
  - version: 0.6.0
    sanctioned:
      - crd: clusterhealths.fathom.skaphos.io
        path: ^.status.children
        reason: children capped
        issue: https://example.test/277
  - version: 0.8.0
    stop: true
    sanctioned:
      - crd: dnschecks.fathom.skaphos.io
        path: "*"
        reason: whole CRD reworked
        issue: https://example.test/401
  - version: 0.10.0
    stop: true
`

func mustParsePath(t *testing.T, raw string) *upgradePathDecl {
	t.Helper()
	d, err := parseUpgradePath([]byte(raw))
	if err != nil {
		t.Fatalf("parseUpgradePath: %v", err)
	}
	return d
}

func v(s string) *semver.Version { return semver.MustParse(s) }

func TestParseUpgradePathRejectsInvalidDeclarations(t *testing.T) {
	cases := map[string]struct{ raw, want string }{
		"missing floor":  {"releases: []", "floor is required"},
		"bad floor":      {"floor: five", "floor"},
		"non-increasing": {"floor: 0.5.0\nreleases:\n  - version: 0.7.0\n  - version: 0.6.0", "does not increase"},
		"duplicate":      {"floor: 0.5.0\nreleases:\n  - version: 0.7.0\n  - version: 0.7.0", "does not increase"},
		"bad version":    {"floor: 0.5.0\nreleases:\n  - version: v0.7", "releases[0].version"},
		"missing fields": {"floor: 0.5.0\nreleases:\n  - version: 0.7.0\n    sanctioned:\n      - crd: a.b\n        path: ^.spec", "missing issue, reason"},
		"unknown field":  {"floor: 0.5.0\nmaxSkip: 2", "unknown field"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseUpgradePath([]byte(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestUpgradePath(t *testing.T) {
	d := mustParsePath(t, testUpgradePath)
	cases := []struct {
		from, to   string
		belowFloor bool
		stops      []string
		rendered   string
	}{
		{from: "0.5.1", to: "0.6.0", rendered: "direct"},
		{from: "0.5.0", to: "0.7.0", rendered: "direct"}, // A equal to the floor is allowed
		{from: "0.3.1", to: "0.6.0", belowFloor: true, rendered: "below the upgrade floor"},
		{from: "0.6.0", to: "0.9.0", stops: []string{"0.8.0"}, rendered: "v0.6.0 -> v0.8.0 -> v0.9.0"},
		{from: "0.6.0", to: "0.11.0", stops: []string{"0.8.0", "0.10.0"}, rendered: "v0.6.0 -> v0.8.0 -> v0.10.0 -> v0.11.0"},
		{from: "0.8.0", to: "0.9.0", rendered: "direct"},  // a stop equal to A is not a step
		{from: "0.7.0", to: "0.8.0", rendered: "direct"},  // a stop equal to B is not a step
		{from: "0.8.0", to: "0.10.0", rendered: "direct"}, // both ends are stops
	}
	for _, tc := range cases {
		t.Run(tc.from+"->"+tc.to, func(t *testing.T) {
			got := d.upgradePath(v(tc.from), v(tc.to))
			if got.BelowFloor != tc.belowFloor || !reflect.DeepEqual(got.Stops, tc.stops) {
				t.Errorf("upgradePath = %+v, want belowFloor=%v stops=%v", got, tc.belowFloor, tc.stops)
			}
			if r := got.render(tc.from, tc.to); r != tc.rendered {
				t.Errorf("render = %q, want %q", r, tc.rendered)
			}
		})
	}
}

func TestOldestDirect(t *testing.T) {
	d := mustParsePath(t, testUpgradePath)
	for to, want := range map[string]string{"0.6.0": "0.5.0", "0.8.0": "0.5.0", "0.9.0": "0.8.0", "0.11.0": "0.10.0"} {
		if got := d.oldestDirect(v(to)); got != want {
			t.Errorf("oldestDirect(%s) = %s, want %s", to, got, want)
		}
	}
}

func TestSanctionedBy(t *testing.T) {
	d := mustParsePath(t, testUpgradePath)
	cases := []struct {
		name, crd, path, check string
		from                   *semver.Version
		to                     string
		wantRelease            string
	}{
		{"exact path in range", "clusterhealths.fathom.skaphos.io", "^.status.children", "maxItems", v("0.5.1"), "0.6.0", "0.6.0"},
		{"other path", "clusterhealths.fathom.skaphos.io", "^.spec.selector", "maxItems", v("0.5.1"), "0.6.0", ""},
		{"other crd", "healthchecks.fathom.skaphos.io", "^.status.children", "maxItems", v("0.5.1"), "0.6.0", ""},
		{"already past the release", "clusterhealths.fathom.skaphos.io", "^.status.children", "maxItems", v("0.6.0"), "0.7.0", ""},
		{"release after the target", "dnschecks.fathom.skaphos.io", "^.spec.targets", "maxItems", v("0.6.0"), "0.7.0", ""},
		{"wildcard", "dnschecks.fathom.skaphos.io", "^.spec.targets", "maxItems", v("0.7.0"), "0.8.0", "0.8.0"},
		{"unknown installed version", "clusterhealths.fathom.skaphos.io", "^.status.children", "maxItems", nil, "0.9.0", "0.6.0"},
		{"scope is never sanctioned", "dnschecks.fathom.skaphos.io", "", scopeCheck, v("0.7.0"), "0.8.0", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, release := d.sanctionedBy(tc.crd, tc.path, tc.check, tc.from, v(tc.to))
			if release != tc.wantRelease {
				t.Errorf("sanctioned by %q, want %q", release, tc.wantRelease)
			}
			if (s != nil) != (tc.wantRelease != "") {
				t.Errorf("entry = %+v, want one iff a release sanctions it", s)
			}
		})
	}
}

// The declaration shipped in the chart must always parse: every fathomctl
// reads it before any upgrade.
func TestEmbeddedUpgradePathParses(t *testing.T) {
	raw, err := helmchart.ReadFile(helmchart.UpgradePathFile)
	if err != nil {
		t.Fatalf("read embedded declaration: %v", err)
	}
	d, err := parseUpgradePath(raw)
	if err != nil {
		t.Fatalf("embedded declaration: %v", err)
	}
	if d.Floor != "0.5.0" {
		t.Errorf("floor = %s, want 0.5.0", d.Floor)
	}
}
