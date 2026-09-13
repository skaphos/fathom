/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

var (
	snapNow  = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	snapLast = metav1.NewTime(snapNow.Add(-2 * time.Minute))
	ready    = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Message: "3 of 3 checks passed"}}
)

func dur(d time.Duration) *metav1.Duration { return &metav1.Duration{Duration: d} }

func TestSnapshot_PerKind(t *testing.T) {
	tests := []struct {
		name        string
		obj         client.Object
		want        snapshot
		wantTimeout time.Duration
	}{
		{
			name: "AddonCheck with status",
			obj: &fathomv1alpha1.AddonCheck{
				Spec: fathomv1alpha1.AddonCheckSpec{Interval: dur(10 * time.Minute), Timeout: dur(45 * time.Second)},
				Status: fathomv1alpha1.AddonCheckStatus{
					LastResult: "Pass", Conditions: ready, LastRunTime: &snapLast,
					LastReportName: "coredns-abc", LastRunTrigger: "tok-1",
				},
			},
			want: snapshot{
				Verdict: "Pass", Summary: "3 of 3 checks passed", LastRun: &snapLast,
				NextRun: ptrTime(snapLast.Add(10 * time.Minute)), ReportName: "coredns-abc", ConsumedTrigger: "tok-1",
			},
			wantTimeout: 45 * time.Second,
		},
		{
			name:        "AddonCheck never run uses defaults and empty verdict",
			obj:         &fathomv1alpha1.AddonCheck{},
			want:        snapshot{},
			wantTimeout: fathomv1alpha1.DefaultAddonCheckTimeout,
		},
		{
			name: "AddonCheck sub-floor interval is clamped like the controller",
			obj: &fathomv1alpha1.AddonCheck{
				Spec:   fathomv1alpha1.AddonCheckSpec{Interval: dur(time.Second), Timeout: dur(time.Millisecond)},
				Status: fathomv1alpha1.AddonCheckStatus{LastRunTime: &snapLast},
			},
			want:        snapshot{LastRun: &snapLast, NextRun: ptrTime(snapLast.Add(fathomv1alpha1.MinCheckInterval))},
			wantTimeout: fathomv1alpha1.MinCheckTimeout,
		},
		{
			name: "DNSCheck uses status.summary and caps timeout at interval",
			obj: &fathomv1alpha1.DNSCheck{
				Spec: fathomv1alpha1.DNSCheckSpec{Interval: dur(30 * time.Second), Timeout: dur(5 * time.Minute)},
				Status: fathomv1alpha1.DNSCheckStatus{
					LastResult: "Fail", Summary: "1 of 2 targets failed", LastRunTime: &snapLast,
					LastReportName: "dns-1", LastRunTrigger: "tok-2",
				},
			},
			want: snapshot{
				Verdict: "Fail", Summary: "1 of 2 targets failed", LastRun: &snapLast,
				NextRun: ptrTime(snapLast.Add(30 * time.Second)), ReportName: "dns-1", ConsumedTrigger: "tok-2",
			},
			wantTimeout: 30 * time.Second,
		},
		{
			name:        "DNSCheck defaults",
			obj:         &fathomv1alpha1.DNSCheck{},
			want:        snapshot{},
			wantTimeout: fathomv1alpha1.DefaultDNSCheckTimeout,
		},
		{
			name: "NodeCertificateCheck uses Ready message and hourly default",
			obj: &fathomv1alpha1.NodeCertificateCheck{
				Status: fathomv1alpha1.NodeCertificateCheckStatus{
					LastResult: "Warn", Conditions: ready, LastRunTime: &snapLast, LastReportName: "ncc-1", LastRunTrigger: "tok-3",
				},
			},
			want: snapshot{
				Verdict: "Warn", Summary: "3 of 3 checks passed", LastRun: &snapLast,
				NextRun: ptrTime(snapLast.Add(fathomv1alpha1.DefaultNodeCertificateCheckInterval)), ReportName: "ncc-1", ConsumedTrigger: "tok-3",
			},
			wantTimeout: fathomv1alpha1.DefaultNodeCertificateCheckTimeout,
		},
		{
			name: "NodeHealthCheck prefers its own summary and the 5m default",
			obj: &fathomv1alpha1.NodeHealthCheck{
				Status: fathomv1alpha1.NodeHealthCheckStatus{
					LastResult: "Fail", Summary: "1 of 2 node(s) passed; worst: node-b DiskHeadroom /var/lib/kubelet: 8.2% of bytes free", Conditions: ready,
					LastRunTime: &snapLast, LastReportName: "nhc-1", LastRunTrigger: "tok-4",
				},
			},
			want: snapshot{
				Verdict: "Fail", Summary: "1 of 2 node(s) passed; worst: node-b DiskHeadroom /var/lib/kubelet: 8.2% of bytes free", LastRun: &snapLast,
				NextRun: ptrTime(snapLast.Add(fathomv1alpha1.DefaultNodeHealthCheckInterval)), ReportName: "nhc-1", ConsumedTrigger: "tok-4",
			},
			wantTimeout: fathomv1alpha1.DefaultNodeHealthCheckTimeout,
		},
		{
			name: "NodeHealthCheck falls back to the Ready message before its first roll-up",
			obj: &fathomv1alpha1.NodeHealthCheck{
				Status: fathomv1alpha1.NodeHealthCheckStatus{Conditions: ready},
			},
			want:        snapshot{Summary: "3 of 3 checks passed"},
			wantTimeout: fathomv1alpha1.DefaultNodeHealthCheckTimeout,
		},
		{
			name: "HealthCheck mirrors source observation and interval",
			obj: &fathomv1alpha1.HealthCheck{
				Status: fathomv1alpha1.HealthCheckStatus{
					Result: "Pass", Summary: "mirrored", SourceObservedAt: &snapLast,
					SourceInterval: dur(time.Minute), LastReportName: "hc-1",
				},
			},
			want: snapshot{
				Verdict: "Pass", Summary: "mirrored", LastRun: &snapLast,
				NextRun: ptrTime(snapLast.Add(time.Minute)), ReportName: "hc-1",
			},
			wantTimeout: 0,
		},
		{
			name:        "HealthCheck without source interval has no next run",
			obj:         &fathomv1alpha1.HealthCheck{Status: fathomv1alpha1.HealthCheckStatus{SourceObservedAt: &snapLast}},
			want:        snapshot{LastRun: &snapLast},
			wantTimeout: 0,
		},
		{
			name: "ClusterHealth derives its summary",
			obj: &fathomv1alpha1.ClusterHealth{
				Status: fathomv1alpha1.ClusterHealthStatus{Result: "Fail", MatchedCount: 4, ObservedAt: &snapLast},
			},
			want:        snapshot{Verdict: "Fail", Summary: "4 matched, worst Fail", LastRun: &snapLast},
			wantTimeout: 0,
		},
		{
			name:        "ClusterHealth never evaluated",
			obj:         &fathomv1alpha1.ClusterHealth{},
			want:        snapshot{Summary: "0 matched"},
			wantTimeout: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := descriptorFor(t, tt.obj)
			got := d.Snapshot(tt.obj)
			if got.Verdict != tt.want.Verdict || got.Summary != tt.want.Summary ||
				got.ReportName != tt.want.ReportName || got.ConsumedTrigger != tt.want.ConsumedTrigger {
				t.Errorf("snapshot = %+v, want %+v", got, tt.want)
			}
			if !sameTime(got.LastRun, tt.want.LastRun) {
				t.Errorf("LastRun = %v, want %v", got.LastRun, tt.want.LastRun)
			}
			if !samePtrTime(got.NextRun, tt.want.NextRun) {
				t.Errorf("NextRun = %v, want %v", got.NextRun, tt.want.NextRun)
			}
			if to := d.DefaultTimeout(tt.obj); to != tt.wantTimeout {
				t.Errorf("DefaultTimeout = %s, want %s", to, tt.wantTimeout)
			}
		})
	}
}

// TestSnapshot_EveryKindWired guards the descriptor table: a kind added
// without a Snapshot or DefaultTimeout would panic at first use.
func TestSnapshot_EveryKindWired(t *testing.T) {
	for _, k := range kinds {
		if k.Snapshot == nil || k.DefaultTimeout == nil {
			t.Errorf("%s: Snapshot/DefaultTimeout not wired", k.Kind)
			continue
		}
		_ = k.Snapshot(k.New())
		_ = k.DefaultTimeout(k.New())
	}
}

func descriptorFor(t *testing.T, o client.Object) *kindDescriptor {
	t.Helper()
	switch o.(type) {
	case *fathomv1alpha1.AddonCheck:
		return kindByName("AddonCheck")
	case *fathomv1alpha1.DNSCheck:
		return kindByName("DNSCheck")
	case *fathomv1alpha1.NodeCertificateCheck:
		return kindByName("NodeCertificateCheck")
	case *fathomv1alpha1.NodeHealthCheck:
		return kindByName("NodeHealthCheck")
	case *fathomv1alpha1.HealthCheck:
		return kindByName("HealthCheck")
	case *fathomv1alpha1.ClusterHealth:
		return kindByName("ClusterHealth")
	}
	t.Fatalf("no descriptor for %T", o)
	return nil
}

func ptrTime(t time.Time) *time.Time { return &t }

func sameTime(a, b *metav1.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(b)
}

func samePtrTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
