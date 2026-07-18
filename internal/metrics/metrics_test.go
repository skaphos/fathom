/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Adapter outcome label values mirror pkg/adapter.Outcome ("Pass"/"Warn"/
// "Fail"/"Error"); these tests use the same casing the adapters emit so the
// documented label set matches production.
//
// Each test that writes to the package-level collectors Resets them first so
// the suite stays order-independent (the collectors are process-global).

func TestMetricsAreValidCollectors(t *testing.T) {
	reg := prometheus.NewRegistry()
	for _, c := range []prometheus.Collector{ReconcileTotal, ReconcileDuration, AdapterRunDuration, AdapterRegistered} {
		if err := reg.Register(c); err != nil {
			t.Fatalf("register collector: %v", err)
		}
	}
}

func TestReconcileMetrics(t *testing.T) {
	ReconcileTotal.Reset()
	ReconcileDuration.Reset()

	reg := prometheus.NewRegistry()
	reg.MustRegister(ReconcileTotal, ReconcileDuration)

	ReconcileTotal.WithLabelValues("HealthCheck", "success").Inc()
	ReconcileTotal.WithLabelValues("HealthCheck", "error").Inc()
	ReconcileTotal.WithLabelValues("AddonCheck", "success").Add(3)

	ReconcileDuration.WithLabelValues("HealthCheck").Observe(0.042)
	ReconcileDuration.WithLabelValues("AddonCheck").Observe(1.234)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	var totalCount float64
	for _, mf := range mfs {
		if mf.GetName() == "fathom_reconcile_total" {
			for _, m := range mf.GetMetric() {
				totalCount += m.GetCounter().GetValue()
			}
		}
	}
	if totalCount != 5.0 {
		t.Errorf("fathom_reconcile_total sum = %v, want 5", totalCount)
	}
}

func TestAdapterMetrics(t *testing.T) {
	AdapterRunDuration.Reset()
	AdapterRegistered.Reset()

	reg := prometheus.NewRegistry()
	reg.MustRegister(AdapterRunDuration, AdapterRegistered)

	AdapterRunDuration.WithLabelValues("coredns", "dns_resolution", "Pass").Observe(0.015)
	AdapterRunDuration.WithLabelValues("cert-manager", "system_health", "Fail").Observe(2.7)

	AdapterRegistered.WithLabelValues("coredns").Set(1)
	AdapterRegistered.WithLabelValues("cert-manager").Set(1)
	AdapterRegistered.WithLabelValues("external-secrets").Set(1)

	var m dto.Metric
	if err := AdapterRegistered.WithLabelValues("coredns").Write(&m); err != nil {
		t.Fatalf("write gauge: %v", err)
	}
	if got := m.GetGauge().GetValue(); got != 1.0 {
		t.Errorf("fathom_adapter_registered{adapter=coredns} = %v, want 1", got)
	}
}

func TestMetricsCanBeUsedFromOtherPackages(t *testing.T) {
	// Documents the intended usage from reconcilers and adapters: the vars are
	// exported and writable. Reset first so this does not perturb other tests.
	ReconcileTotal.Reset()
	AdapterRunDuration.Reset()
	AdapterRegistered.Reset()

	ReconcileTotal.WithLabelValues("ClusterHealth", "success").Inc()
	AdapterRunDuration.WithLabelValues("external-secrets", "system_health", "Pass").Observe(0.5)
	AdapterRegistered.WithLabelValues("external-secrets").Set(1)
}

func TestRecordAdapterRunHelper(t *testing.T) {
	AdapterRunDuration.Reset()

	reg := prometheus.NewRegistry()
	reg.MustRegister(AdapterRunDuration)

	RecordAdapterRun("coredns", "dns_resolution", "Pass", 42*time.Millisecond)
	RecordAdapterRun("cert-manager", "system_health", "Fail", 1*time.Second)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	var total uint64
	for _, mf := range mfs {
		if mf.GetName() == "fathom_adapter_run_duration_seconds" {
			for _, m := range mf.GetMetric() {
				total += m.GetHistogram().GetSampleCount()
			}
		}
	}
	if total != 2 {
		t.Errorf("adapter run histogram sample count = %d, want 2", total)
	}
}

func TestRecordReconcileHelper(t *testing.T) {
	ReconcileTotal.Reset()
	ReconcileDuration.Reset()

	RecordReconcile("HealthCheck", "success", 123*time.Millisecond)
	RecordReconcile("AddonCheck", "error", 2*time.Second)

	mfs, err := ctrlmetrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	var total float64
	for _, mf := range mfs {
		if mf.GetName() == "fathom_reconcile_total" {
			for _, m := range mf.GetMetric() {
				total += m.GetCounter().GetValue()
			}
		}
	}
	if total != 2.0 {
		t.Errorf("fathom_reconcile_total sum = %v, want 2", total)
	}
}
