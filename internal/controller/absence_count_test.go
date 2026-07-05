/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"

	"github.com/skaphos/fathom/pkg/adapter"
)

// TestCountAbsent verifies status.absent counts every check carrying the absent
// marker — required-absent Fails and optional-absent Skips alike — and ignores
// present-but-unhealthy Fails and ordinary Skips (SKA-526).
func TestCountAbsent(t *testing.T) {
	requiredAbsent := adapter.CheckResult{Outcome: adapter.OutcomeFail, Details: adapter.MarkAbsent(nil)}
	optionalAbsent := adapter.CheckResult{Outcome: adapter.OutcomeSkipped, Details: adapter.MarkAbsent(map[string]string{"component": "x"})}
	presentUnhealthy := adapter.CheckResult{Outcome: adapter.OutcomeFail, Details: map[string]string{"component": "y"}}
	disabledSkip := adapter.CheckResult{Outcome: adapter.OutcomeSkipped, Details: map[string]string{"skipReason": "FamilyDisabled"}}
	healthy := adapter.CheckResult{Outcome: adapter.OutcomePass}

	tests := []struct {
		name   string
		checks []adapter.CheckResult
		want   int32
	}{
		{"empty", nil, 0},
		{"none absent", []adapter.CheckResult{presentUnhealthy, disabledSkip, healthy}, 0},
		{"required-absent and optional-absent both count", []adapter.CheckResult{requiredAbsent, optionalAbsent, presentUnhealthy, healthy}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := countAbsent(tc.checks); got != tc.want {
				t.Fatalf("countAbsent(%v) = %d, want %d", tc.checks, got, tc.want)
			}
		})
	}
}
