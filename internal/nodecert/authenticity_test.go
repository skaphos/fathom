/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodecert

import (
	"testing"
	"time"
)

// TestVerifyReportBinding pins the structural bindings a genuine node report
// always satisfies (SEC-1). The checks are ordered most-benign-first, so each
// case here also asserts which reason wins when several could apply.
func TestVerifyReportBinding(t *testing.T) {
	t.Parallel()

	const check = "node-certificates"
	const node = "node-a"
	canonical := NodeReportConfigMapName(check, node)

	report := func(mutate func(*NodeReport)) NodeReport {
		r := NodeReport{Node: node, CheckName: check, ObservedAt: time.Now(), Aggregate: OutcomePass}
		if mutate != nil {
			mutate(&r)
		}
		return r
	}

	tests := []struct {
		name          string
		cmName        string
		annotatedNode string
		report        NodeReport
		want          ReportRejection
		wantForgery   bool
	}{
		{
			name:          "well-bound report is accepted",
			cmName:        canonical,
			annotatedNode: node,
			report:        report(nil),
			want:          ReportAccepted,
		},
		{
			name:          "payload naming another check is skipped, not flagged",
			cmName:        canonical,
			annotatedNode: node,
			report:        report(func(r *NodeReport) { r.CheckName = "other-check" }),
			want:          RejectWrongCheck,
		},
		{
			name:          "payload without a node cannot be attributed",
			cmName:        canonical,
			annotatedNode: node,
			report:        report(func(r *NodeReport) { r.Node = "" }),
			want:          RejectMissingNode,
		},
		{
			name:          "missing annotation means admission bound nothing",
			cmName:        canonical,
			annotatedNode: "",
			report:        report(nil),
			want:          RejectMissingNodeAnnotation,
		},
		{
			// The compromised-agent case from #155: the writer's node claim pins
			// the annotation, so the payload is what has to lie.
			name:          "payload disagreeing with the bound annotation is forgery",
			cmName:        canonical,
			annotatedNode: "node-b",
			report:        report(nil),
			want:          RejectNodeMismatch,
			wantForgery:   true,
		},
		{
			// The SEC-1 case admission alone does not close: a second ConfigMap
			// competing with the node's real report.
			name:          "self-consistent report at a non-canonical name is forgery",
			cmName:        "attacker-supplied-name",
			annotatedNode: node,
			report:        report(nil),
			want:          RejectNonCanonicalName,
			wantForgery:   true,
		},
		{
			name:          "canonical name is derived from the check, not the label",
			cmName:        NodeReportConfigMapName("a-different-check", node),
			annotatedNode: node,
			report:        report(nil),
			want:          RejectNonCanonicalName,
			wantForgery:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := VerifyReportBinding(tc.cmName, tc.annotatedNode, check, tc.report)
			if got != tc.want {
				t.Errorf("VerifyReportBinding = %q, want %q", got, tc.want)
			}
			if got.IndicatesForgery() != tc.wantForgery {
				t.Errorf("%q.IndicatesForgery() = %v, want %v", got, got.IndicatesForgery(), tc.wantForgery)
			}
		})
	}
}

// TestReportAcceptedIsNotForgery guards the zero value: an accepted report must
// never be reported as a forgery signal.
func TestReportAcceptedIsNotForgery(t *testing.T) {
	t.Parallel()
	if ReportAccepted.IndicatesForgery() {
		t.Error("ReportAccepted must not indicate forgery")
	}
}
