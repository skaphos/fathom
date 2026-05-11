/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package v1alpha1

import "testing"

func TestHealthReportResultSeverity_OrderingAcrossEnumValues(t *testing.T) {
	// The ordering is load-bearing for both reconcilers (AddonCheck and
	// ClusterHealth aggregations call into Severity). If this changes,
	// downstream Result roll-ups change shape — bump deliberately.
	want := []HealthReportResult{
		HealthReportResultPass,
		HealthReportResultSkipped,
		HealthReportResultWarn,
		HealthReportResultUnknown,
		HealthReportResultFail,
		HealthReportResultError,
	}
	for i := 1; i < len(want); i++ {
		prev, curr := want[i-1], want[i]
		if prev.Severity() >= curr.Severity() {
			t.Errorf("severity not strictly increasing: %s(%d) >= %s(%d)",
				prev, prev.Severity(), curr, curr.Severity())
		}
	}
}

func TestHealthReportResultSeverity_EmptyAndUnrecognizedReturnZero(t *testing.T) {
	if HealthReportResult("").Severity() != 0 {
		t.Errorf("empty Result severity = %d, want 0", HealthReportResult("").Severity())
	}
	if HealthReportResult("not-a-real-outcome").Severity() != 0 {
		t.Errorf("unrecognized Result severity = %d, want 0", HealthReportResult("not-a-real-outcome").Severity())
	}
}

func TestHealthReportResultSeverity_PassIsLowestNonZero(t *testing.T) {
	// Pass=1 is the lowest "this Result was actually observed" rank; anything
	// returning 0 is excluded from worst-case aggregation entirely.
	if HealthReportResultPass.Severity() != 1 {
		t.Errorf("Pass severity = %d, want 1", HealthReportResultPass.Severity())
	}
}
