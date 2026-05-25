/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package metrics provides the custom Prometheus metrics for Fathom.
// These metrics are registered with the controller-runtime metrics registry
// so they are automatically exposed alongside the built-in metrics.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Reconcile metrics track the health and performance of the three main reconcilers.
var (
	// ReconcileTotal counts reconcile invocations by kind and outcome.
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fathom_reconcile_total",
			Help: "Total number of reconciles by kind and outcome.",
		},
		[]string{"kind", "outcome"},
	)

	// ReconcileDuration measures how long reconcile operations take.
	ReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fathom_reconcile_duration_seconds",
			Help:    "Duration of reconcile operations by kind in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"kind"},
	)
)

// Adapter metrics track registration and execution of addon adapters.
var (
	// AdapterRunDuration measures how long individual adapter Run calls take.
	AdapterRunDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fathom_adapter_run_duration_seconds",
			Help:    "Duration of adapter Run() calls by adapter, family, and outcome in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"adapter", "family", "outcome"},
	)

	// AdapterRegistered is a gauge that is set to 1 for each successfully registered adapter.
	AdapterRegistered = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fathom_adapter_registered",
			Help: "Indicates whether an adapter is registered (1 = registered).",
		},
		[]string{"adapter"},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		ReconcileTotal,
		ReconcileDuration,
		AdapterRunDuration,
		AdapterRegistered,
	)
}

// RecordReconcile is a convenience helper for reconcilers to record both
// the total count and duration of a reconcile operation.
func RecordReconcile(kind, outcome string, duration time.Duration) {
	ReconcileTotal.WithLabelValues(kind, outcome).Inc()
	ReconcileDuration.WithLabelValues(kind).Observe(duration.Seconds())
}

// RecordAdapterRun records the duration of a single adapter Run() invocation.
func RecordAdapterRun(adapter, family, outcome string, duration time.Duration) {
	AdapterRunDuration.WithLabelValues(adapter, family, outcome).Observe(duration.Seconds())
}
