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

	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/internal/nodehealth"
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

// Check metrics express the current verdict and staleness of every check
// resource the operator reconciles (AddonCheck, DNSCheck, NodeCertificateCheck,
// NodeHealthCheck, HealthCheck, ClusterHealth), so operators can alert on
// failing or stale checks
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

	// CheckLastRunTimestamp is the unix time of the evaluation backing the
	// check's current result, 0 until the first evaluation completes — so one
	// "time() - metric > N" rule catches never-ran and stopped-running checks
	// alike.
	//
	// For the wrapper kinds it follows the evidence chain rather than this
	// object's own reconcile: a HealthCheck carries its mirrored target's run
	// time, and a ClusterHealth the STALEST of its children, so a stale
	// contributor cannot hide behind a live sibling (#277).
	CheckLastRunTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fathom_check_last_run_timestamp_seconds",
			Help: "Unix time of the most recent completed evaluation backing a check's current result (0 = never evaluated).",
		},
		[]string{"kind", "name", "namespace"},
	)

	// CheckInterval publishes the cadence a check is currently expected to run
	// at, so staleness can be expressed relative to that cadence instead of a
	// hardcoded constant (#277).
	//
	// Without it a single threshold has to serve every kind, and there is no
	// value that works: 900s suits a 5m AddonCheck but fires continuously
	// against a 1h NodeCertificateCheck. Labels deliberately match
	// CheckLastRunTimestamp exactly so the two join with no relabeling:
	//
	//	time() - fathom_check_last_run_timestamp_seconds
	//	  > 3 * fathom_check_interval_seconds
	//
	// Left unset — not zero — for a check whose cadence cannot be resolved, so
	// such a check drops out of that join rather than appearing infinitely
	// overdue.
	CheckInterval = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fathom_check_interval_seconds",
			Help: "Cadence a check is currently expected to run at, after per-resource override and floor clamping. Absent when the cadence cannot be resolved.",
		},
		[]string{"kind", "name", "namespace"},
	)
)

// DNSCheck publishes one further gauge: the check-level result above says a
// check is failing, but a check may cover sixteen names from three vantage
// points, and an operator needs to alert on the one that broke rather than on
// the check as a whole (skaphos/fathom#266).
var (
	// DNSCheckTargetResult is the check-level result gauge one level down: a
	// one-hot state set per (target, vantage point) pair, so exactly one series
	// per pair is 1.
	//
	// Series count is bounded by the CRD schema alone — 16 targets × 3 vantage
	// points × 6 results = 288 per check — so the monitoring cost of a check is
	// computable from its specification before it is applied. Raising any of
	// those schema caps raises this ceiling and is a cardinality change, not
	// merely a limits change.
	//
	// The label set is deliberately NOT folded into CheckResult: that gauge is
	// consumed by cluster-wide dashboards across every kind, and adding target
	// labels there would multiply every other kind's cardinality.
	DNSCheckTargetResult = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fathom_dnscheck_target_result",
			Help: "Current result of one DNSCheck (target, vantage point) pair (one-hot: exactly one series per pair is 1).",
		},
		[]string{"namespace", "check", "name", "record_type", "resolver", "result"},
	)
)

// checkResultValues is the canonical result vocabulary, mirroring the
// api/v1alpha1 HealthReportResult constants. It remains a literal to keep this
// observability package independent of the Kubernetes API types. A unit test
// asserts it stays in sync, so a new result state cannot silently miss metrics.
var checkResultValues = []string{"Pass", "Warn", "Fail", "Error", "Skipped", "Unknown"}

// Node-detail metrics are projected by the operator from fresh, validated,
// in-scope node reports. They are served through the operator's authenticated
// metrics endpoint; node agents expose liveness only.
var (
	// NodeCertificateExpiryDays reports the earliest known certificate expiry
	// for one check and node. Negative once the certificate has expired. Paths,
	// subjects, and issuers are deliberately absent from the label set.
	NodeCertificateExpiryDays = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fathom_node_certificate_expiry_days",
			Help: "Minimum known days until certificate expiry for a NodeCertificateCheck and node (negative once expired).",
		},
		[]string{"namespace", "check", "node"},
	)

	// NodeHealthCheckResult is the per-check result one level below the
	// NodeHealthCheck's own verdict: a one-hot state set per (node, type, path),
	// so an operator alerts on the node and check that broke rather than on
	// the check as a whole (#206). Series count is bounded by the CRD schema:
	// 16 items × 6 results per node. Labels contain the check identity and the
	// schema-bounded node, type, and path dimensions; no summary text is used.
	NodeHealthCheckResult = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fathom_node_health_check_result",
			Help: "Current result of one NodeHealthCheck item on a node (one-hot: exactly one series per (node, type, path) is 1).",
		},
		[]string{"namespace", "check", "node", "type", "path", "result"},
	)

	// NodeHealthFilesystemFreePercent is the measured headroom behind a
	// DiskHeadroom or InodeHeadroom result, so a dashboard can graph the trend
	// rather than only the thresholded verdict. resource is "bytes" or
	// "inodes".
	NodeHealthFilesystemFreePercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fathom_node_health_filesystem_free_percent",
			Help: "Percentage of free bytes or inodes on the filesystem holding a NodeHealthCheck headroom path, by node, path, and resource.",
		},
		[]string{"namespace", "check", "node", "path", "resource"},
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
		CheckInterval,
		DNSCheckTargetResult,
		NodeCertificateExpiryDays,
		NodeHealthCheckResult,
		NodeHealthFilesystemFreePercent,
	)
}

// ObserveCheck mirrors a check's current status into the check gauges: the
// full one-hot result set plus the last-run timestamp. An empty or
// unrecognized result is coerced to "Unknown" (the sentinel for "not yet
// evaluated"), and a zero lastRun becomes 0 ("never ran"). Idempotent —
// reconcilers call it on every pass, whatever the exit path.
// interval is the check's effective cadence. A non-positive value means the
// cadence could not be resolved — for example a HealthCheck wrapping a kind the
// operator cannot yet look up — and leaves the series unset rather than
// asserting a cadence of zero, which would make the check read as permanently
// overdue in any cadence-relative rule (#277).
func ObserveCheck(kind, namespace, name, result string, lastRun time.Time, interval time.Duration) {
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
	if interval > 0 {
		CheckInterval.WithLabelValues(kind, name, namespace).Set(interval.Seconds())
	} else {
		// A cadence can stop being resolvable — for example, a wrapper's target is
		// deleted. Leaving the last known value behind would be worse
		// than never publishing one: the staleness rule would keep joining against
		// a cadence that no longer applies, so the check silently retains alert
		// coverage it should have lost. Withdraw the series instead.
		CheckInterval.DeleteLabelValues(kind, name, namespace)
	}
}

// DeleteCheckSeries removes every series ObserveCheck created for a check.
// Called when a reconcile observes the resource is gone, so a deleted check
// cannot keep asserting a result. An operator restart clears the registry
// wholesale; startup reconciles repopulate only checks that still exist.
func DeleteCheckSeries(kind, namespace, name string) {
	labels := prometheus.Labels{"kind": kind, "name": name, "namespace": namespace}
	CheckResult.DeletePartialMatch(labels)
	CheckLastRunTimestamp.DeletePartialMatch(labels)
	CheckInterval.DeletePartialMatch(labels)
}

// ObserveDNSTarget mirrors one (target, vantage point) pair's outcome into the
// per-target gauge as a one-hot set. An empty or unrecognized result is coerced
// to "Unknown", matching ObserveCheck.
//
// Callers rebuild rather than diff: DeleteDNSCheckTargetSeries first, then one
// ObserveDNSTarget per pair the specification currently declares. A pair the
// specification dropped is simply never re-set, so its series disappears with no
// removal detection — and that stays correct across an operator restart, which a
// diff against remembered state would not.
func ObserveDNSTarget(namespace, check, name, recordType, resolver, result string) {
	if !slices.Contains(checkResultValues, result) {
		result = "Unknown"
	}
	for _, value := range checkResultValues {
		current := 0.0
		if value == result {
			current = 1
		}
		DNSCheckTargetResult.WithLabelValues(namespace, check, name, recordType, resolver, value).Set(current)
	}
}

// DeleteDNSCheckTargetSeries removes every per-target series belonging to one
// DNSCheck. Called at the start of each evaluation to rebuild the set, and when
// a reconcile observes the check is gone so a deleted check cannot keep
// asserting per-target results.
func DeleteDNSCheckTargetSeries(namespace, check string) {
	DNSCheckTargetResult.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "check": check})
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

// ObserveNodeCertificateReport publishes the earliest known expiry from one
// accepted node report. A report with no nonzero expiry publishes no sample.
func ObserveNodeCertificateReport(namespace, check string, report nodecert.NodeReport) {
	var minimum int
	found := false
	for _, cert := range report.Certs {
		if cert.NotAfter.IsZero() || found && cert.DaysRemaining >= minimum {
			continue
		}
		minimum = cert.DaysRemaining
		found = true
	}
	if found {
		NodeCertificateExpiryDays.WithLabelValues(namespace, check, report.Node).Set(float64(minimum))
	}
}

// DeleteNodeCertificateSeries removes only one NodeCertificateCheck's detail
// series. Reconcilers call it at entry, then rebuild from accepted reports.
func DeleteNodeCertificateSeries(namespace, check string) {
	NodeCertificateExpiryDays.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "check": check})
}

// ObserveNodeHealthReport publishes every item in one accepted node report.
func ObserveNodeHealthReport(namespace, check string, report nodehealth.NodeReport) {
	for _, result := range report.Checks {
		ObserveNodeHealthCheck(namespace, check, report.Node, result.Type, result.Path, string(result.Outcome))
		if result.PercentFree == nil {
			continue
		}
		resource := "bytes"
		if result.Type == nodehealth.TypeInodeHeadroom {
			resource = "inodes"
		}
		ObserveNodeHealthFilesystem(namespace, check, report.Node, result.Path, resource, *result.PercentFree)
	}
}

// ObserveNodeHealthCheck mirrors one NodeHealthCheck item's outcome on one node
// into the per-check gauge as a one-hot set. An empty or unrecognized result is
// coerced to "Unknown", matching ObserveCheck. Reconcilers rebuild rather than
// diff: delete one check's series first, then observe every accepted report, so
// an item the spec dropped simply disappears.
func ObserveNodeHealthCheck(namespace, check, node, checkType, path, result string) {
	if !slices.Contains(checkResultValues, result) {
		result = "Unknown"
	}
	for _, value := range checkResultValues {
		current := 0.0
		if value == result {
			current = 1
		}
		NodeHealthCheckResult.WithLabelValues(namespace, check, node, checkType, path, value).Set(current)
	}
}

// ObserveNodeHealthFilesystem records the measured free percentage behind a
// headroom result.
func ObserveNodeHealthFilesystem(namespace, check, node, path, resource string, percentFree float64) {
	NodeHealthFilesystemFreePercent.WithLabelValues(namespace, check, node, path, resource).Set(percentFree)
}

// DeleteNodeHealthSeries removes only one NodeHealthCheck's detail series.
// Reconcilers call it at entry, then rebuild from accepted reports.
func DeleteNodeHealthSeries(namespace, check string) {
	labels := prometheus.Labels{"namespace": namespace, "check": check}
	NodeHealthCheckResult.DeletePartialMatch(labels)
	NodeHealthFilesystemFreePercent.DeletePartialMatch(labels)
}
