/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package metrics provides the custom Prometheus metrics for Fathom.
// These metrics are registered with the controller-runtime metrics registry
// so they are automatically exposed alongside the built-in metrics.
package metrics

import (
	"slices"
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

// Check metrics express the current verdict and freshness of every check
// resource the operator reconciles (AddonCheck, HealthCheck, ClusterHealth,
// NodeCertificateCheck), so operators can alert on failing or stale checks
// without bridging CRD status into their monitoring stack (skaphos/fathom#154).
var (
	// CheckResult is a one-hot state set: for every existing check there is one
	// series per result value, and exactly one of them is 1. Series exist from
	// the moment a check is first observed (result "Unknown" until the first
	// evaluation completes) and are removed when the check is deleted.
	CheckResult = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fathom_check_result",
			Help: "Current result of a check by kind, name, namespace, and result (one-hot: exactly one series per check is 1).",
		},
		[]string{"kind", "name", "namespace", "result"},
	)

	// CheckLastRunTimestamp is the unix time of the freshest completed
	// evaluation backing the check's current result, 0 until the first
	// evaluation completes — so one "time() - metric > N" rule catches
	// never-ran and stopped-running checks alike.
	CheckLastRunTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fathom_check_last_run_timestamp_seconds",
			Help: "Unix time of the most recent completed evaluation backing a check's current result (0 = never evaluated).",
		},
		[]string{"kind", "name", "namespace"},
	)
)

// checkResultValues is the canonical result vocabulary, mirroring the
// api/v1alpha1 HealthReportResult constants. It is deliberately a literal —
// importing the API package here would drag apimachinery into every binary
// that serves these metrics (the node-agent imports this package) — and a
// unit test asserts it stays in sync with the API constants, so a new result
// state cannot silently miss the metric.
var checkResultValues = []string{"Pass", "Warn", "Fail", "Error", "Skipped", "Unknown"}

// Node-agent metrics are set by the node-agent DaemonSet (cmd/node-agent), which
// imports this package and serves ctrlmetrics.Registry on its own metrics port.
// In the operator process the gauge is registered but never set, so it emits no
// series there.
var (
	// NodeCertificateExpiryDays reports days-until-expiry for each on-disk
	// certificate a node-agent scans. Negative once a certificate has expired.
	// Labelled only by node and certificate path: the agent's /metrics endpoint
	// is unauthenticated, so the sensitive subject/issuer distinguished names are
	// deliberately NOT exposed here — they live in the HealthReport instead. This
	// also keeps label cardinality bounded.
	NodeCertificateExpiryDays = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fathom_node_certificate_expiry_days",
			Help: "Days until on-disk certificate expiry on a node (negative once expired), by node and path.",
		},
		[]string{"node", "path"},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		ReconcileTotal,
		ReconcileDuration,
		AdapterRunDuration,
		AdapterRegistered,
		CheckResult,
		CheckLastRunTimestamp,
		NodeCertificateExpiryDays,
	)
}

// ObserveCheck mirrors a check's current status into the check gauges: the
// full one-hot result set plus the last-run timestamp. An empty or
// unrecognized result is coerced to "Unknown" (the sentinel for "not yet
// evaluated"), and a zero lastRun becomes 0 ("never ran"). Idempotent —
// reconcilers call it on every pass, whatever the exit path.
func ObserveCheck(kind, namespace, name, result string, lastRun time.Time) {
	if !slices.Contains(checkResultValues, result) {
		result = "Unknown"
	}
	for _, value := range checkResultValues {
		current := 0.0
		if value == result {
			current = 1
		}
		CheckResult.WithLabelValues(kind, name, namespace, value).Set(current)
	}
	ts := 0.0
	if !lastRun.IsZero() {
		ts = float64(lastRun.Unix())
	}
	CheckLastRunTimestamp.WithLabelValues(kind, name, namespace).Set(ts)
}

// DeleteCheckSeries removes every series ObserveCheck created for a check.
// Called when a reconcile observes the resource is gone, so a deleted check
// cannot keep asserting a result. An operator restart clears the registry
// wholesale; startup reconciles repopulate only checks that still exist.
func DeleteCheckSeries(kind, namespace, name string) {
	labels := prometheus.Labels{"kind": kind, "name": name, "namespace": namespace}
	CheckResult.DeletePartialMatch(labels)
	CheckLastRunTimestamp.DeletePartialMatch(labels)
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
