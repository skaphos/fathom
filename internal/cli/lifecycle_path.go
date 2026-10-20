/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"sigs.k8s.io/yaml"
)

// upgradePathDecl is the chart's upgrade-path.yaml (contracts/upgrade-path.md):
// a floor below which no upgrade data exists, then one entry per release that
// is a stop or sanctions an incompatible CRD change. Append-only.
type upgradePathDecl struct {
	Floor    string             `json:"floor"`
	Releases []pathReleaseEntry `json:"releases"`

	floor *semver.Version
}

type pathReleaseEntry struct {
	Version    string             `json:"version"`
	Stop       bool               `json:"stop,omitempty"`
	Sanctioned []sanctionedChange `json:"sanctioned,omitempty"`

	version *semver.Version
}

// sanctionedChange is one incompatible CRD change a release deliberately
// makes; reason and issue are shown verbatim in plans and refusals.
type sanctionedChange struct {
	CRD    string `json:"crd"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Issue  string `json:"issue"`
}

// scopeCheck is crdify's CRD-level scope validation. A scope change can never
// be applied in place, so no declaration can sanction it (FR-022).
const scopeCheck = "scope"

// parseUpgradePath parses and validates the declaration.
func parseUpgradePath(data []byte) (*upgradePathDecl, error) {
	var d upgradePathDecl
	if err := yaml.UnmarshalStrict(data, &d); err != nil {
		return nil, fmt.Errorf("parse upgrade-path declaration: %w", err)
	}
	if d.Floor == "" {
		return nil, errors.New("upgrade-path declaration: floor is required")
	}
	floor, err := semver.StrictNewVersion(d.Floor)
	if err != nil {
		return nil, fmt.Errorf("upgrade-path declaration: floor %q: %w", d.Floor, err)
	}
	d.floor = floor
	var prev *semver.Version
	for i := range d.Releases {
		e := &d.Releases[i]
		v, err := semver.StrictNewVersion(e.Version)
		if err != nil {
			return nil, fmt.Errorf("upgrade-path declaration: releases[%d].version %q: %w", i, e.Version, err)
		}
		if prev != nil && !v.GreaterThan(prev) {
			return nil, fmt.Errorf("upgrade-path declaration: releases[%d].version %s does not increase on %s", i, v, prev)
		}
		prev, e.version = v, v
		for j, s := range e.Sanctioned {
			var missing []string
			for field, value := range map[string]string{"crd": s.CRD, "path": s.Path, "reason": s.Reason, "issue": s.Issue} {
				if value == "" {
					missing = append(missing, field)
				}
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				return nil, fmt.Errorf("upgrade-path declaration: releases[%d].sanctioned[%d] is missing %s", i, j, strings.Join(missing, ", "))
			}
		}
	}
	return &d, nil
}

// pathResult is the upgrade path from A to B.
type pathResult struct {
	// BelowFloor is set when A predates every recorded upgrade datum.
	BelowFloor bool
	// Stops are the versions S with A < S < B that must be installed first,
	// ascending. Empty means direct.
	Stops []string
}

// Direct reports a path with no intermediate stop.
func (p pathResult) Direct() bool { return !p.BelowFloor && len(p.Stops) == 0 }

// render prints the path the way plans and status show it.
func (p pathResult) render(from, to string) string {
	if p.BelowFloor {
		return "below the upgrade floor"
	}
	if len(p.Stops) == 0 {
		return "direct"
	}
	parts := append([]string{"v" + from}, prefixV(p.Stops)...)
	return strings.Join(append(parts, "v"+to), " -> ")
}

// upgradePath computes the path from A to B (A < B) per the contract: below
// the floor is refused outright; otherwise every stop strictly between A and
// B must be installed first.
func (d *upgradePathDecl) upgradePath(from, to *semver.Version) pathResult {
	if from.LessThan(d.floor) {
		return pathResult{BelowFloor: true}
	}
	var res pathResult
	for _, e := range d.Releases {
		if e.Stop && e.version.GreaterThan(from) && e.version.LessThan(to) {
			res.Stops = append(res.Stops, e.version.String())
		}
	}
	return res
}

// oldestDirect is the oldest version `to` can be upgraded from directly: the
// later of the floor and the last stop below it (FR-027).
func (d *upgradePathDecl) oldestDirect(to *semver.Version) string {
	oldest := d.floor
	for _, e := range d.Releases {
		if e.Stop && e.version.LessThan(to) && e.version.GreaterThan(oldest) {
			oldest = e.version
		}
	}
	return oldest.String()
}

// sanctionedBy returns the entry that sanctions a finding on crd at path for
// an upgrade from A to B: one declared by a release with A < version <= B,
// matching the CRD and the path (or "*"). A nil from means the installed
// version is unknown, so every recorded release up to B counts. A
// same-version re-apply (A == B) counts B's own sanctions: `helm upgrade`
// moves the release without its CRDs, so the CRD transition B declares is
// still ahead of the cluster and `status` sends the user here to make it
// (CRDs recorded by a newer release are refused before sanctions are judged,
// so the window cannot license rewriting a newer schema). Scope findings
// are never sanctioned.
func (d *upgradePathDecl) sanctionedBy(crd, path, check string, from, to *semver.Version) (*sanctionedChange, string) {
	if check == scopeCheck {
		return nil, ""
	}
	for i := len(d.Releases) - 1; i >= 0; i-- {
		e := &d.Releases[i]
		if e.version.GreaterThan(to) {
			continue
		}
		sameVersionReapply := from != nil && from.Equal(to) && e.version.Equal(to)
		if from != nil && !e.version.GreaterThan(from) && !sameVersionReapply {
			continue
		}
		for j := range e.Sanctioned {
			s := &e.Sanctioned[j]
			if s.CRD == crd && (s.Path == "*" || s.Path == path) {
				return s, e.version.String()
			}
		}
	}
	return nil, ""
}

func prefixV(vs []string) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = "v" + v
	}
	return out
}
