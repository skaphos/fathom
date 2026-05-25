/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestMetricsAreValidCollectors(t *testing.T) {
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(ReconcileTotal))
	require.NoError(t, reg.Register(ReconcileDuration))
	require.NoError(t, reg.Register(AdapterRunDuration))
	require.NoError(t, reg.Register(AdapterRegistered))
}

func TestReconcileMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(ReconcileTotal, ReconcileDuration)

	ReconcileTotal.WithLabelValues("HealthCheck", "success").Inc()
	ReconcileTotal.WithLabelValues("HealthCheck", "error").Inc()
	ReconcileTotal.WithLabelValues("AddonCheck", "success").Add(3)

	ReconcileDuration.WithLabelValues("HealthCheck").Observe(0.042)
	ReconcileDuration.WithLabelValues("AddonCheck").Observe(1.234)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	var totalCount float64
	for _, mf := range mfs {
		if mf.GetName() == "fathom_reconcile_total" {
			for _, m := range mf.GetMetric() {
				totalCount += m.GetCounter().GetValue()
			}
		}
	}
	assert.Equal(t, 5.0, totalCount)
}

func TestAdapterMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(AdapterRunDuration, AdapterRegistered)

	AdapterRunDuration.WithLabelValues("coredns", "dns_resolution", "pass").Observe(0.015)
	AdapterRunDuration.WithLabelValues("cert-manager", "system_health", "fail").Observe(2.7)

	AdapterRegistered.WithLabelValues("coredns").Set(1)
	AdapterRegistered.WithLabelValues("cert-manager").Set(1)
	AdapterRegistered.WithLabelValues("external-secrets").Set(1)

	var m dto.Metric
	err := AdapterRegistered.WithLabelValues("coredns").Write(&m)
	require.NoError(t, err)
	assert.Equal(t, 1.0, *m.Gauge.Value)
}

func TestMetricsCanBeUsedFromOtherPackages(t *testing.T) {
	// This test documents the intended usage pattern from reconcilers and adapters.
	// We just exercise the vars to ensure they are exported and usable.
	ReconcileTotal.WithLabelValues("ClusterHealth", "success").Inc()
	AdapterRunDuration.WithLabelValues("external-secrets", "system_health", "pass").Observe(0.5)
	AdapterRegistered.WithLabelValues("external-secrets").Set(1)
}

func TestRecordAdapterRunHelper(t *testing.T) {
	AdapterRunDuration.Reset()

	reg := prometheus.NewRegistry()
	reg.MustRegister(AdapterRunDuration)

	RecordAdapterRun("coredns", "dns_resolution", "pass", 42*time.Millisecond)
	RecordAdapterRun("cert-manager", "system_health", "fail", 1*time.Second)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	var total uint64
	for _, mf := range mfs {
		if mf.GetName() == "fathom_adapter_run_duration_seconds" {
			for _, m := range mf.GetMetric() {
				total += m.GetHistogram().GetSampleCount()
			}
		}
	}
	assert.Equal(t, uint64(2), total)
}

func TestRecordReconcileHelper(t *testing.T) {
	ReconcileTotal.Reset()
	ReconcileDuration.Reset()

	// Simulate what a reconciler would do
	RecordReconcile("HealthCheck", "success", 123*time.Millisecond)
	RecordReconcile("AddonCheck", "error", 2*time.Second)

	mfs, err := ctrlmetrics.Registry.Gather()
	require.NoError(t, err)

	total := float64(0)
	for _, mf := range mfs {
		if mf.GetName() == "fathom_reconcile_total" {
			for _, m := range mf.GetMetric() {
				total += m.GetCounter().GetValue()
			}
		}
	}
	assert.Equal(t, 2.0, total)
}
