/*
SPDX-FileCopyrightText: 2026 Skaphos
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
