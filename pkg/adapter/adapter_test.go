/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package adapter

import "testing"

func TestOutcomeValid(t *testing.T) {
	tests := []struct {
		outcome Outcome
		want    bool
	}{
		{OutcomePass, true},
		{OutcomeWarn, true},
		{OutcomeFail, true},
		{OutcomeError, true},
		{OutcomeSkipped, true},
		{Outcome(""), false},
		{Outcome("pass"), false}, // case-sensitive by design
		{Outcome("Unknown"), false},
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
	const fam = Family("system_health")
	const other = Family("dns_resolution")
	check := func(family Family, o Outcome) CheckResult {
		return CheckResult{Family: family, Outcome: o}
	}

	tests := []struct {
		name   string
		checks []CheckResult
		want   Outcome
	}{
		{"no checks at all", nil, OutcomePass},
		{"family has no checks", []CheckResult{check(other, OutcomeFail)}, OutcomePass},
		{"all pass", []CheckResult{check(fam, OutcomePass), check(fam, OutcomePass)}, OutcomePass},
		{"warn outranks pass", []CheckResult{check(fam, OutcomePass), check(fam, OutcomeWarn)}, OutcomeWarn},
		{"fail outranks warn", []CheckResult{check(fam, OutcomeWarn), check(fam, OutcomeFail)}, OutcomeFail},
		{"error counts as worst", []CheckResult{check(fam, OutcomePass), check(fam, OutcomeError)}, OutcomeError},
		{
			// A failure in another family must not taint this family's verdict.
			name:   "other family failure ignored",
			checks: []CheckResult{check(fam, OutcomePass), check(other, OutcomeError)},
			want:   OutcomePass,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FamilyOutcome(tc.checks, fam); got != tc.want {
				t.Fatalf("FamilyOutcome(%v, %q) = %q, want %q", tc.checks, fam, got, tc.want)
			}
		})
	}
}
