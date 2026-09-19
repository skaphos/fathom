/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/internal/nodehealth"
)

func TestNodeDetailProjectionIncludesAcceptedPartialFleetAndExcludesDepartedNodes(t *testing.T) {
	metrics.NodeCertificateExpiryDays.Reset()
	metrics.NodeHealthCheckResult.Reset()
	metrics.NodeHealthFilesystemFreePercent.Reset()
	expected := map[string]struct{}{"node-live": {}, "node-missing": {}}

	known := time.Unix(1, 0)
	observeNodeCertificateReports("tenant-a", "certs", []nodecert.NodeReport{
		{Node: "node-live", Certs: []nodecert.CertResult{{NotAfter: known, DaysRemaining: 5}}},
		{Node: "node-departed", Certs: []nodecert.CertResult{{NotAfter: known, DaysRemaining: -10}}},
	}, expected)
	if got := testutil.ToFloat64(metrics.NodeCertificateExpiryDays.WithLabelValues("tenant-a", "certs", "node-live")); got != 5 {
		t.Fatalf("live-node expiry = %v, want 5", got)
	}
	if got := testutil.CollectAndCount(metrics.NodeCertificateExpiryDays); got != 1 {
		t.Fatalf("partial fleet emitted %d certificate series, want only accepted live-node evidence", got)
	}

	free := 23.5
	observeNodeHealthReports("tenant-a", "health", []nodehealth.NodeReport{
		{Node: "node-live", Checks: []nodehealth.CheckResult{{Type: nodehealth.TypeDiskHeadroom, Path: "/var/lib/kubelet", Outcome: nodehealth.OutcomeWarn, PercentFree: &free}}},
		{Node: "node-departed", Checks: []nodehealth.CheckResult{{Type: nodehealth.TypeKubeletHealthz, Outcome: nodehealth.OutcomeFail}}},
	}, expected)
	if got := testutil.ToFloat64(metrics.NodeHealthCheckResult.WithLabelValues("tenant-a", "health", "node-live", nodehealth.TypeDiskHeadroom, "/var/lib/kubelet", "Warn")); got != 1 {
		t.Fatalf("live-node Warn series = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(metrics.NodeHealthCheckResult); got != len(checkResultValuesForTest) {
		t.Fatalf("partial fleet emitted %d health series, want one live item's one-hot set", got)
	}
}

var checkResultValuesForTest = [...]string{"Pass", "Warn", "Fail", "Error", "Skipped", "Unknown"}

func TestNodeDetailMetricsWithdrawAtReconcileEntryWithoutAffectingSiblingCheck(t *testing.T) {
	metrics.NodeCertificateExpiryDays.Reset()
	metrics.NodeHealthCheckResult.Reset()
	metrics.NodeHealthFilesystemFreePercent.Reset()
	scheme := newProvisioningScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	known := time.Unix(1, 0)
	certReport := nodecert.NodeReport{Node: "node-a", Certs: []nodecert.CertResult{{NotAfter: known, DaysRemaining: 3}}}
	metrics.ObserveNodeCertificateReport("tenant-a", "gone", certReport)
	metrics.ObserveNodeCertificateReport("tenant-a", "keep", certReport)

	certReconciler := &NodeCertificateCheckReconciler{Client: cl, Scheme: scheme}
	if _, err := certReconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "tenant-a", Name: "gone"}}); err != nil {
		t.Fatal(err)
	}
	if got := testutil.CollectAndCount(metrics.NodeCertificateExpiryDays); got != 1 {
		t.Fatalf("deleted check withdrawal left %d certificate series, want sibling only", got)
	}

	healthReport := nodehealth.NodeReport{Node: "node-a", Checks: []nodehealth.CheckResult{{Type: nodehealth.TypeKubeletHealthz, Outcome: nodehealth.OutcomePass}}}
	metrics.ObserveNodeHealthReport("tenant-a", "gone", healthReport)
	metrics.ObserveNodeHealthReport("tenant-a", "keep", healthReport)
	healthReconciler := &NodeHealthCheckReconciler{Client: cl, Scheme: scheme}
	if _, err := healthReconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "tenant-a", Name: "gone"}}); err != nil {
		t.Fatal(err)
	}
	if got := testutil.CollectAndCount(metrics.NodeHealthCheckResult); got != len(checkResultValuesForTest) {
		t.Fatalf("deleted check withdrawal left %d health series, want sibling only", got)
	}
}

func TestPausedNodeCertificateCheckWithdrawsDetailMetrics(t *testing.T) {
	metrics.NodeCertificateExpiryDays.Reset()
	scheme := newProvisioningScheme(t)
	check := &fathomv1alpha1.NodeCertificateCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "paused", Namespace: "tenant-a", UID: "check-uid"},
		Spec:       fathomv1alpha1.NodeCertificateCheckSpec{Paused: true},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check).
		WithStatusSubresource(&fathomv1alpha1.NodeCertificateCheck{}).Build()
	metrics.ObserveNodeCertificateReport(check.Namespace, check.Name, nodecert.NodeReport{
		Node: "node-a", Certs: []nodecert.CertResult{{NotAfter: time.Unix(1, 0), DaysRemaining: 3}},
	})

	r := &NodeCertificateCheckReconciler{Client: cl, Scheme: scheme}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(check)}); err != nil {
		t.Fatal(err)
	}
	if got := testutil.CollectAndCount(metrics.NodeCertificateExpiryDays); got != 0 {
		t.Fatalf("paused check left %d certificate series, want none", got)
	}
}
