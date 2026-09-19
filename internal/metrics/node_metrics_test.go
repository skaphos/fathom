/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/internal/nodehealth"
)

func TestObserveNodeCertificateReportUsesMinimumKnownExpiry(t *testing.T) {
	NodeCertificateExpiryDays.Reset()
	report := nodecert.NodeReport{Node: "node-a", Certs: []nodecert.CertResult{
		{Path: "/secret/unknown", DaysRemaining: -999},
		{Path: "/secret/later", NotAfter: time.Unix(2, 0), DaysRemaining: 30},
		{Path: "/secret/earliest", NotAfter: time.Unix(1, 0), DaysRemaining: -2},
		{Path: "/secret/latest", NotAfter: time.Unix(3, 0), DaysRemaining: 90},
	}}

	ObserveNodeCertificateReport("tenant-a", "certs", report)
	if got := testutil.ToFloat64(NodeCertificateExpiryDays.WithLabelValues("tenant-a", "certs", "node-a")); got != -2 {
		t.Fatalf("expiry = %v, want minimum known value -2", got)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(NodeCertificateExpiryDays)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		for _, metric := range family.Metric {
			if len(metric.Label) != 3 {
				t.Fatalf("certificate metric labels = %v, want namespace/check/node only", metric.Label)
			}
			for _, label := range metric.Label {
				if label.GetName() == "path" || label.GetName() == "subject" || label.GetName() == "issuer" {
					t.Fatalf("sensitive certificate label %q was exported", label.GetName())
				}
			}
		}
	}
}

func TestObserveNodeCertificateReportOmitsUnknownExpiryAndScopesDeletion(t *testing.T) {
	NodeCertificateExpiryDays.Reset()
	ObserveNodeCertificateReport("tenant-a", "unknown", nodecert.NodeReport{Node: "node-a", Certs: []nodecert.CertResult{{Path: "/x"}}})
	if got := testutil.CollectAndCount(NodeCertificateExpiryDays); got != 0 {
		t.Fatalf("unknown expiry emitted %d series, want none", got)
	}

	known := nodecert.NodeReport{Node: "node-a", Certs: []nodecert.CertResult{{NotAfter: time.Unix(1, 0), DaysRemaining: 7}}}
	ObserveNodeCertificateReport("tenant-a", "drop", known)
	ObserveNodeCertificateReport("tenant-a", "keep", known)
	ObserveNodeCertificateReport("tenant-b", "drop", known)
	DeleteNodeCertificateSeries("tenant-a", "drop")
	if got := testutil.CollectAndCount(NodeCertificateExpiryDays); got != 2 {
		t.Fatalf("scoped deletion left %d series, want 2 unrelated series", got)
	}
}

func TestObserveNodeHealthReportPreservesOneHotAndMeasurements(t *testing.T) {
	NodeHealthCheckResult.Reset()
	NodeHealthFilesystemFreePercent.Reset()
	free := 41.25
	report := nodehealth.NodeReport{Node: "node-a", Checks: []nodehealth.CheckResult{
		{Type: nodehealth.TypeDiskHeadroom, Path: "/var/lib/kubelet", Outcome: nodehealth.OutcomeWarn, PercentFree: &free},
		{Type: nodehealth.TypeKubeletHealthz, Outcome: nodehealth.OutcomePass},
	}}

	ObserveNodeHealthReport("tenant-a", "health", report)
	if got := testutil.ToFloat64(NodeHealthCheckResult.WithLabelValues("tenant-a", "health", "node-a", nodehealth.TypeDiskHeadroom, "/var/lib/kubelet", "Warn")); got != 1 {
		t.Fatalf("Warn series = %v, want 1", got)
	}
	if got := testutil.ToFloat64(NodeHealthCheckResult.WithLabelValues("tenant-a", "health", "node-a", nodehealth.TypeDiskHeadroom, "/var/lib/kubelet", "Pass")); got != 0 {
		t.Fatalf("Pass series = %v, want 0", got)
	}
	if got := testutil.ToFloat64(NodeHealthFilesystemFreePercent.WithLabelValues("tenant-a", "health", "node-a", "/var/lib/kubelet", "bytes")); got != free {
		t.Fatalf("free percent = %v, want %v", got, free)
	}
	if got := testutil.CollectAndCount(NodeHealthCheckResult); got != 2*len(checkResultValues) {
		t.Fatalf("result series = %d, want %d", got, 2*len(checkResultValues))
	}
}

func TestDeleteNodeHealthSeriesIsScopedToCheckIdentity(t *testing.T) {
	NodeHealthCheckResult.Reset()
	NodeHealthFilesystemFreePercent.Reset()
	free := 55.0
	report := nodehealth.NodeReport{Node: "node-a", Checks: []nodehealth.CheckResult{{
		Type: nodehealth.TypeInodeHeadroom, Path: "/var/log", Outcome: nodehealth.OutcomePass, PercentFree: &free,
	}}}
	ObserveNodeHealthReport("tenant-a", "drop", report)
	ObserveNodeHealthReport("tenant-a", "keep", report)
	ObserveNodeHealthReport("tenant-b", "drop", report)

	DeleteNodeHealthSeries("tenant-a", "drop")
	if got := testutil.CollectAndCount(NodeHealthCheckResult); got != 2*len(checkResultValues) {
		t.Fatalf("scoped deletion left %d result series, want %d", got, 2*len(checkResultValues))
	}
	if got := testutil.CollectAndCount(NodeHealthFilesystemFreePercent); got != 2 {
		t.Fatalf("scoped deletion left %d measurement series, want 2", got)
	}
}
