/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package adapter_test

import (
	"testing"

	"github.com/skaphos/fathom/pkg/adapter"
)

func TestOutcomeValid(t *testing.T) {
	tests := []struct {
		outcome adapter.Outcome
		want    bool
	}{
		{adapter.OutcomePass, true},
		{adapter.OutcomeWarn, true},
		{adapter.OutcomeFail, true},
		{adapter.OutcomeError, true},
		{adapter.OutcomeSkipped, true},
		{adapter.Outcome(""), false},
		{adapter.Outcome("pass"), false}, // case-sensitive by design
		{adapter.Outcome("Unknown"), false},
	}
	for _, tc := range tests {
		t.Run(string(tc.outcome), func(t *testing.T) {
			if got := tc.outcome.Valid(); got != tc.want {
				t.Fatalf("Outcome(%q).Valid() = %v, want %v", tc.outcome, got, tc.want)
			}
		})
	}
}

func TestFamilyOutcome(t *testing.T) {
	const fam = adapter.Family("system_health")
	const other = adapter.Family("dns_resolution")
	check := func(family adapter.Family, o adapter.Outcome) adapter.CheckResult {
		return adapter.CheckResult{Family: family, Outcome: o}
	}

	tests := []struct {
		name   string
		checks []adapter.CheckResult
		want   adapter.Outcome
	}{
		{"no checks at all", nil, adapter.OutcomePass},
		{"family has no checks", []adapter.CheckResult{check(other, adapter.OutcomeFail)}, adapter.OutcomePass},
		{"all pass", []adapter.CheckResult{check(fam, adapter.OutcomePass), check(fam, adapter.OutcomePass)}, adapter.OutcomePass},
		{"warn outranks pass", []adapter.CheckResult{check(fam, adapter.OutcomePass), check(fam, adapter.OutcomeWarn)}, adapter.OutcomeWarn},
		{"fail outranks warn", []adapter.CheckResult{check(fam, adapter.OutcomeWarn), check(fam, adapter.OutcomeFail)}, adapter.OutcomeFail},
		{"error counts as worst", []adapter.CheckResult{check(fam, adapter.OutcomePass), check(fam, adapter.OutcomeError)}, adapter.OutcomeError},
		{
			// A failure in another family must not taint this family's verdict.
			name:   "other family failure ignored",
			checks: []adapter.CheckResult{check(fam, adapter.OutcomePass), check(other, adapter.OutcomeError)},
			want:   adapter.OutcomePass,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := adapter.FamilyOutcome(tc.checks, fam); got != tc.want {
				t.Fatalf("FamilyOutcome(%v, %q) = %q, want %q", tc.checks, fam, got, tc.want)
			}
		})
	}
}

func TestMarkAbsentAndIsAbsent(t *testing.T) {
	// nil map: MarkAbsent allocates one and IsAbsent reports true.
	m := adapter.MarkAbsent(nil)
	if !adapter.IsAbsent(m) {
		t.Fatalf("IsAbsent(MarkAbsent(nil)) = false, want true (got %v)", m)
	}
	if m[adapter.DetailAbsent] != "true" {
		t.Fatalf("MarkAbsent(nil)[%q] = %q, want \"true\"", adapter.DetailAbsent, m[adapter.DetailAbsent])
	}

	// Existing details are preserved and the marker is added alongside them.
	m = adapter.MarkAbsent(map[string]string{"component": "app"})
	if m["component"] != "app" || !adapter.IsAbsent(m) {
		t.Fatalf("MarkAbsent(existing) preserved+marked: got %v", m)
	}

	// Absent nil or unmarked details are not absent.
	if adapter.IsAbsent(nil) {
		t.Fatal("IsAbsent(nil) = true, want false")
	}
	if adapter.IsAbsent(map[string]string{"skipReason": "FamilyDisabled"}) {
		t.Fatal("IsAbsent(non-absent details) = true, want false")
	}
}

func TestMarkVersionGate(t *testing.T) {
	// Unsupported: all three keys set, existing details preserved.
	m := adapter.MarkVersionGate(map[string]string{"component": "cilium"}, adapter.ReasonUnsupportedAddonVersion, "2.5.0", ">=1.0 <2.0")
	if m["component"] != "cilium" {
		t.Errorf("MarkVersionGate dropped existing details: %v", m)
	}
	if m[adapter.DetailVersionGate] != adapter.ReasonUnsupportedAddonVersion {
		t.Errorf("versionGate = %q, want %q", m[adapter.DetailVersionGate], adapter.ReasonUnsupportedAddonVersion)
	}
	if m[adapter.DetailDetectedVersion] != "2.5.0" || m[adapter.DetailSupportedVersions] != ">=1.0 <2.0" {
		t.Errorf("detected/supported = %q/%q", m[adapter.DetailDetectedVersion], m[adapter.DetailSupportedVersions])
	}

	// Unknown with no detected version: the detected key is omitted (not "").
	m = adapter.MarkVersionGate(nil, adapter.ReasonVersionUnknown, "", ">=1.0")
	if m[adapter.DetailVersionGate] != adapter.ReasonVersionUnknown {
		t.Errorf("versionGate = %q, want VersionUnknown", m[adapter.DetailVersionGate])
	}
	if _, ok := m[adapter.DetailDetectedVersion]; ok {
		t.Errorf("empty detected version should be omitted, got %q", m[adapter.DetailDetectedVersion])
	}
	if m[adapter.DetailSupportedVersions] != ">=1.0" {
		t.Errorf("supported = %q, want >=1.0", m[adapter.DetailSupportedVersions])
	}
}
