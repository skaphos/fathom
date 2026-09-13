/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodehealth

import (
	"strings"
	"testing"
	"time"

	"github.com/skaphos/fathom/internal/nodecert"
)

func TestReportRoundTrip(t *testing.T) {
	t.Parallel()
	pct := 42.5
	in := NodeReport{
		Node:       "node-a",
		CheckName:  "nh",
		ObservedAt: time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC),
		Aggregate:  OutcomeWarn,
		Trigger:    "tok-1",
		Checks: []CheckResult{
			{Type: TypeDiskHeadroom, Path: "/var/lib/kubelet", Outcome: OutcomeWarn, Summary: "42.5% of bytes free", PercentFree: &pct, Total: 1000, Free: 425},
			{Type: TypeKubeletHealthz, Outcome: OutcomePass, Summary: "ok"},
		},
	}
	encoded, err := EncodeReport(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeReport(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if out.Node != in.Node || out.CheckName != in.CheckName || !out.ObservedAt.Equal(in.ObservedAt) || out.Aggregate != in.Aggregate || out.Trigger != in.Trigger {
		t.Fatalf("header round-trip mismatch: %+v", out)
	}
	if len(out.Checks) != 2 || out.Checks[0].PercentFree == nil || *out.Checks[0].PercentFree != pct || out.Checks[1].PercentFree != nil {
		t.Fatalf("checks round-trip mismatch: %+v", out.Checks)
	}
	if _, err := DecodeReport("{not json"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestItemsRoundTrip(t *testing.T) {
	t.Parallel()
	in := []Item{
		{Type: TypeDiskHeadroom, Path: "/var/log", WarnPercentFree: 20, CriticalPercentFree: 0},
		{Type: TypeContainerRuntime, SocketPath: "/run/crio/crio.sock"},
	}
	encoded, err := EncodeItems(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeItems(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0] != in[0] || out[1] != in[1] {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	// A zero threshold must survive: it means "never", not "unset".
	if out[0].CriticalPercentFree != 0 {
		t.Fatalf("zero threshold did not survive: %d", out[0].CriticalPercentFree)
	}
	if _, err := DecodeItems("nope"); err == nil {
		t.Fatal("expected decode error")
	}
}

// TestReportConfigMapNameIsKindQualified pins the fix for the name collision a
// second node-scoped kind would otherwise introduce: a NodeHealthCheck and a
// NodeCertificateCheck sharing a name in one namespace must write to different
// ConfigMaps, or each would authenticate — and overwrite — the other's report.
func TestReportConfigMapNameIsKindQualified(t *testing.T) {
	t.Parallel()
	ours := ReportConfigMapName("shared", "node-a")
	theirs := nodecert.NodeReportConfigMapName("shared", "node-a")
	if ours == theirs {
		t.Fatalf("NodeHealthCheck and NodeCertificateCheck report names collide: %q", ours)
	}
	if !strings.HasPrefix(ours, "nodehealth-shared-") {
		t.Fatalf("name %q is not kind-qualified", ours)
	}
	if ours != ReportConfigMapName("shared", "node-a") {
		t.Fatal("name is not deterministic")
	}
	if len(ours) > 253 {
		t.Fatalf("name too long: %d", len(ours))
	}
	if ReportConfigMapName("shared", "node-a") == ReportConfigMapName("shared", "node-b") {
		t.Fatal("distinct nodes produced the same name")
	}
}

// TestVerifyReportBindingSharesNodecertBindings pins that the SEC-1 structural
// bindings apply to this kind through the shared verifier, including the
// canonical-name rule against THIS kind's name — not nodecert's.
func TestVerifyReportBindingSharesNodecertBindings(t *testing.T) {
	t.Parallel()
	report := NodeReport{Node: "node-a", CheckName: "nh"}
	canonical := ReportConfigMapName("nh", "node-a")
	tests := []struct {
		name          string
		cmName        string
		annotatedNode string
		checkName     string
		report        NodeReport
		want          nodecert.ReportRejection
	}{
		{"accepted", canonical, "node-a", "nh", report, nodecert.ReportAccepted},
		{"wrong check", canonical, "node-a", "other", report, nodecert.RejectWrongCheck},
		{"missing node", canonical, "node-a", "nh", NodeReport{CheckName: "nh"}, nodecert.RejectMissingNode},
		{"missing annotation", canonical, "", "nh", report, nodecert.RejectMissingNodeAnnotation},
		{"node mismatch", canonical, "node-b", "nh", report, nodecert.RejectNodeMismatch},
		{"non-canonical name", "nh-node-a-deadbeef", "node-a", "nh", report, nodecert.RejectNonCanonicalName},
		// A report at nodecert's canonical name for the same check+node is an
		// off-name report for THIS kind: the two must never be confused.
		{"nodecert's canonical name is off-name here", nodecert.NodeReportConfigMapName("nh", "node-a"), "node-a", "nh", report, nodecert.RejectNonCanonicalName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := VerifyReportBinding(tt.cmName, tt.annotatedNode, tt.checkName, tt.report); got != tt.want {
				t.Fatalf("VerifyReportBinding = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWorstOutcomeFold pins the fold the operator relies on: Skipped is
// informational and never wins while a graded outcome exists; empty and
// all-Skipped inputs yield Skipped.
func TestWorstOutcomeFold(t *testing.T) {
	t.Parallel()
	r := func(o ...Outcome) []CheckResult {
		out := make([]CheckResult, 0, len(o))
		for _, x := range o {
			out = append(out, CheckResult{Outcome: x})
		}
		return out
	}
	tests := []struct {
		name string
		in   []CheckResult
		want Outcome
	}{
		{"empty", nil, OutcomeSkipped},
		{"all skipped", r(OutcomeSkipped, OutcomeSkipped), OutcomeSkipped},
		{"single pass", r(OutcomePass), OutcomePass},
		{"pass beats skipped", r(OutcomeSkipped, OutcomePass), OutcomePass},
		{"warn beats pass", r(OutcomePass, OutcomeWarn), OutcomeWarn},
		{"fail beats warn", r(OutcomeWarn, OutcomeFail, OutcomePass), OutcomeFail},
		{"error beats fail", r(OutcomeFail, OutcomeError), OutcomeError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := WorstOutcome(tt.in); got != tt.want {
				t.Fatalf("WorstOutcome = %s, want %s", got, tt.want)
			}
		})
	}
}
