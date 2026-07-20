/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// TestDecideNodeCertRollup pins the transition-only contract (#157): a new
// HealthReport is persisted only when the aggregate result changes (or on the
// first roll-up); an unchanged result refreshes liveness on the interval cadence
// and is otherwise a no-op. The decision is pure, so it is exercised here without
// envtest.
func TestDecideNodeCertRollup(t *testing.T) {
	t.Parallel()

	const interval = time.Hour
	// A fixed reference instant keeps the relative offsets below deterministic.
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *metav1.Time {
		mt := metav1.NewTime(now.Add(d))
		return &mt
	}

	tests := []struct {
		name      string
		status    fathomv1alpha1.NodeCertificateCheckStatus
		aggregate string
		want      nodeCertRollupDecision
	}{
		{
			name:      "first roll-up with no prior report persists",
			status:    fathomv1alpha1.NodeCertificateCheckStatus{},
			aggregate: "Pass",
			want:      rollupPersist,
		},
		{
			name:      "prior report but nil LastRunTime persists",
			status:    fathomv1alpha1.NodeCertificateCheckStatus{LastReportName: "r1", LastResult: "Pass"},
			aggregate: "Pass",
			want:      rollupPersist,
		},
		{
			name:      "result transition persists immediately, not throttled by interval",
			status:    fathomv1alpha1.NodeCertificateCheckStatus{LastReportName: "r1", LastResult: "Pass", LastRunTime: at(0)},
			aggregate: "Fail",
			want:      rollupPersist,
		},
		{
			name:      "unchanged result within the interval is a no-op",
			status:    fathomv1alpha1.NodeCertificateCheckStatus{LastReportName: "r1", LastResult: "Pass", LastRunTime: at(-30 * time.Minute)},
			aggregate: "Pass",
			want:      rollupNoop,
		},
		{
			name:      "unchanged result at the interval boundary refreshes liveness",
			status:    fathomv1alpha1.NodeCertificateCheckStatus{LastReportName: "r1", LastResult: "Pass", LastRunTime: at(-interval)},
			aggregate: "Pass",
			want:      rollupRefreshLiveness,
		},
		{
			name:      "unchanged result past the interval refreshes liveness only",
			status:    fathomv1alpha1.NodeCertificateCheckStatus{LastReportName: "r1", LastResult: "Pass", LastRunTime: at(-2 * time.Hour)},
			aggregate: "Pass",
			want:      rollupRefreshLiveness,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := decideNodeCertRollup(&tc.status, tc.aggregate, interval, now); got != tc.want {
				t.Fatalf("decideNodeCertRollup = %v, want %v", got, tc.want)
			}
		})
	}
}
