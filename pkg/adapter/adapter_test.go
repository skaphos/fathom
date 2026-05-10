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
