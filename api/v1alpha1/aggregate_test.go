/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

import "testing"

func TestWorstResult(t *testing.T) {
	const bogus HealthReportResult = "Nonsense" // unrecognized, Severity 0

	tests := []struct {
		name    string
		results []HealthReportResult
		coerce  bool
		want    HealthReportResult
	}{
		// Empty / informational-only sets fold to Skipped.
		{"empty slice", nil, false, HealthReportResultSkipped},
		{"empty slice coerced", nil, true, HealthReportResultSkipped},
		{"all skipped", []HealthReportResult{HealthReportResultSkipped, HealthReportResultSkipped}, false, HealthReportResultSkipped},
		{"all empty uncoerced", []HealthReportResult{"", ""}, false, HealthReportResultSkipped},
		{"unrecognized only", []HealthReportResult{bogus}, false, HealthReportResultSkipped},

		// Skipped is informational: it never wins while a participating result exists (#160).
		{"pass plus skipped", []HealthReportResult{HealthReportResultPass, HealthReportResultSkipped}, false, HealthReportResultPass},
		{"skipped plus fail", []HealthReportResult{HealthReportResultSkipped, HealthReportResultFail}, false, HealthReportResultFail},

		// Ordinary worst-of over participating results.
		{"all pass", []HealthReportResult{HealthReportResultPass, HealthReportResultPass}, false, HealthReportResultPass},
		{"pass warn fail", []HealthReportResult{HealthReportResultPass, HealthReportResultWarn, HealthReportResultFail}, false, HealthReportResultFail},
		{"fail beats unknown", []HealthReportResult{HealthReportResultUnknown, HealthReportResultFail}, false, HealthReportResultFail},
		{"error wins", []HealthReportResult{HealthReportResultFail, HealthReportResultError}, false, HealthReportResultError},

		// Empty is informational unless coerced.
		{"empty plus pass uncoerced", []HealthReportResult{"", HealthReportResultPass}, false, HealthReportResultPass},

		// Coercion promotes empty -> participating Unknown (ClusterHealth, #161).
		{"empty coerced to unknown", []HealthReportResult{""}, true, HealthReportResultUnknown},
		{"empty coerced but fail wins", []HealthReportResult{"", HealthReportResultFail}, true, HealthReportResultFail},
		{"empty coerced beats skipped", []HealthReportResult{"", HealthReportResultSkipped}, true, HealthReportResultUnknown},
		{"empty coerced with pass", []HealthReportResult{"", HealthReportResultPass}, true, HealthReportResultUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorstResult(tt.results, tt.coerce); got != tt.want {
				t.Errorf("WorstResult(%v, coerce=%v) = %q, want %q", tt.results, tt.coerce, got, tt.want)
			}
		})
	}
}
