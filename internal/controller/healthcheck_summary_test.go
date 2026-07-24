/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"strings"
	"testing"
	"unicode/utf8"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSummarizeFromConditionsBoundsLength is the regression test for the mirror
// wedge: a source Ready condition Message longer than the HealthCheckStatus
// .Summary MaxLength (1024) must be truncated at the mirror boundary so the
// status update the controller then issues stays admissible. Before the fix
// summarizeFromConditions returned the message verbatim, so an over-long
// adapter error deterministically failed every HealthCheck status update and
// froze the mirrored verdict.
func TestSummarizeFromConditionsBoundsLength(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		wantLenLTE  int
		wantVerbatm bool
	}{
		{
			name:        "short message passes through verbatim",
			message:     "AddonCheck mirrored the referenced check's status.",
			wantLenLTE:  healthCheckSummaryMaxLen,
			wantVerbatm: true,
		},
		{
			name:        "message exactly at the bound is verbatim",
			message:     strings.Repeat("a", healthCheckSummaryMaxLen),
			wantLenLTE:  healthCheckSummaryMaxLen,
			wantVerbatm: true,
		},
		{
			name:       "over-long ASCII message is truncated to the bound",
			message:    strings.Repeat("x", 32768),
			wantLenLTE: healthCheckSummaryMaxLen,
		},
		{
			name:       "over-long multi-byte message stays within the rune bound",
			message:    strings.Repeat("→", 4000),
			wantLenLTE: healthCheckSummaryMaxLen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conds := []metav1.Condition{{
				Type:    healthCheckConditionReady,
				Message: tt.message,
			}}
			got := summarizeFromConditions(conds)

			if n := utf8.RuneCountInString(got); n > tt.wantLenLTE {
				t.Fatalf("summary rune length = %d, want <= %d", n, tt.wantLenLTE)
			}
			if tt.wantVerbatm && got != tt.message {
				t.Fatalf("summary = %q, want verbatim %q", got, tt.message)
			}
			if !tt.wantVerbatm && got == tt.message {
				t.Fatalf("summary was not truncated for an over-long message")
			}
		})
	}
}

// TestSummarizeFromConditionsNoReadyCondition covers the no-Ready-condition
// path returning an empty summary.
func TestSummarizeFromConditionsNoReadyCondition(t *testing.T) {
	conds := []metav1.Condition{{Type: "SomethingElse", Message: "ignored"}}
	if got := summarizeFromConditions(conds); got != "" {
		t.Fatalf("summary = %q, want empty when no Ready condition present", got)
	}
}
