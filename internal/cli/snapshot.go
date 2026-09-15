/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"fmt"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// readyCondition is the condition every kind uses for its operational
// summary. AddonCheck and NodeCertificateCheck carry no summary field, so the
// Ready message is the bounded one-line explanation the operator writes;
// NodeHealthCheck falls back to it until its own summary is populated.
const readyCondition = "Ready"

const (
	// nodeAgentPodRolloutMargin estimates serial DaemonSet termination,
	// scheduling, image startup, and readiness for one node. It is deliberately
	// conservative, but cannot guarantee a finite scheduler or image-pull bound.
	nodeAgentPodRolloutMargin = time.Minute
	maxDuration               = time.Duration(1<<63 - 1)
)

// snapshot is the normalised view of a check that ls, describe, and
// run --wait all render. There is exactly one extractor per kind (below), so
// the three verbs can never disagree about what a check's verdict is.
type snapshot struct {
	// Verdict is empty when the check has never run; renderers show "-".
	Verdict fathomv1alpha1.HealthReportResult
	// Summary is the bounded human-readable line from status.
	Summary string
	// LastRun is when the check last completed (or, for derived kinds, when
	// its source last did).
	LastRun *metav1.Time
	// NextRun is LastRun plus the effective interval; nil when the kind has no
	// interval of its own or has never run.
	NextRun *time.Time
	// ReportName is status.lastReportName.
	ReportName string
	// ConsumedTrigger is status.lastRunTrigger; empty for derived kinds.
	ConsumedTrigger string
}

// effectiveDuration mirrors the controllers' cadence rule: an unset or
// non-positive value takes the default, a positive value below the admission
// floor is raised to the floor, anything else is used as declared.
func effectiveDuration(d *metav1.Duration, floor, def time.Duration) time.Duration {
	if d == nil || d.Duration <= 0 {
		return def
	}
	if d.Duration < floor {
		return floor
	}
	return d.Duration
}

func readyMessage(conditions []metav1.Condition) string {
	if c := apimeta.FindStatusCondition(conditions, readyCondition); c != nil {
		return c.Message
	}
	return ""
}

func nextRun(last *metav1.Time, interval time.Duration) *time.Time {
	if last == nil || last.IsZero() || interval <= 0 {
		return nil
	}
	t := last.Add(interval)
	return &t
}

func addonCheckSnapshot(o client.Object) snapshot {
	c := o.(*fathomv1alpha1.AddonCheck)
	interval := effectiveDuration(c.Spec.Interval, fathomv1alpha1.MinCheckInterval, fathomv1alpha1.DefaultAddonCheckInterval)
	return snapshot{
		Verdict:         fathomv1alpha1.HealthReportResult(c.Status.LastResult),
		Summary:         readyMessage(c.Status.Conditions),
		LastRun:         c.Status.LastRunTime,
		NextRun:         nextRun(c.Status.LastRunTime, interval),
		ReportName:      c.Status.LastReportName,
		ConsumedTrigger: c.Status.LastRunTrigger,
	}
}

func addonCheckTimeout(o client.Object) time.Duration {
	c := o.(*fathomv1alpha1.AddonCheck)
	return effectiveDuration(c.Spec.Timeout, fathomv1alpha1.MinCheckTimeout, fathomv1alpha1.DefaultAddonCheckTimeout)
}

func dnsCheckSnapshot(o client.Object) snapshot {
	c := o.(*fathomv1alpha1.DNSCheck)
	interval := effectiveDuration(c.Spec.Interval, fathomv1alpha1.MinCheckInterval, fathomv1alpha1.DefaultDNSCheckInterval)
	return snapshot{
		Verdict:         fathomv1alpha1.HealthReportResult(c.Status.LastResult),
		Summary:         c.Status.Summary,
		LastRun:         c.Status.LastRunTime,
		NextRun:         nextRun(c.Status.LastRunTime, interval),
		ReportName:      c.Status.LastReportName,
		ConsumedTrigger: c.Status.LastRunTrigger,
	}
}

// dnsCheckTimeout mirrors the controller's run bound: the whole run is capped
// at the interval so one run can never overlap the next.
func dnsCheckTimeout(o client.Object) time.Duration {
	c := o.(*fathomv1alpha1.DNSCheck)
	bound := effectiveDuration(c.Spec.Timeout, fathomv1alpha1.MinCheckTimeout, fathomv1alpha1.DefaultDNSCheckTimeout)
	interval := effectiveDuration(c.Spec.Interval, fathomv1alpha1.MinCheckInterval, fathomv1alpha1.DefaultDNSCheckInterval)
	return min(bound, interval)
}

func nodeCertificateCheckSnapshot(o client.Object) snapshot {
	c := o.(*fathomv1alpha1.NodeCertificateCheck)
	interval := effectiveDuration(c.Spec.Interval, fathomv1alpha1.MinCheckInterval, fathomv1alpha1.DefaultNodeCertificateCheckInterval)
	return snapshot{
		Verdict:         fathomv1alpha1.HealthReportResult(c.Status.LastResult),
		Summary:         readyMessage(c.Status.Conditions),
		LastRun:         c.Status.LastRunTime,
		NextRun:         nextRun(c.Status.LastRunTime, interval),
		ReportName:      c.Status.LastReportName,
		ConsumedTrigger: c.Status.LastRunTrigger,
	}
}

func nodeCertificateCheckTimeout(o client.Object) time.Duration {
	c := o.(*fathomv1alpha1.NodeCertificateCheck)
	return effectiveDuration(c.Spec.Timeout, fathomv1alpha1.MinCheckTimeout, fathomv1alpha1.DefaultNodeCertificateCheckTimeout)
}

// nodeHealthCheckCadence is the cadence the controller actually runs a
// NodeHealthCheck at: spec.interval capped at the agent cadence. Status is
// refreshed and the agent re-evaluates on it, so "next run" follows it, not
// a 24h spec.interval.
func nodeHealthCheckCadence(c *fathomv1alpha1.NodeHealthCheck) time.Duration {
	interval := effectiveDuration(c.Spec.Interval, fathomv1alpha1.MinCheckInterval, fathomv1alpha1.DefaultNodeHealthCheckInterval)
	return min(interval, fathomv1alpha1.MaxNodeHealthCheckAgentInterval)
}

func nodeHealthCheckSnapshot(o client.Object) snapshot {
	c := o.(*fathomv1alpha1.NodeHealthCheck)
	interval := nodeHealthCheckCadence(c)
	// NodeHealthCheck writes its own bounded summary once it has rolled up;
	// before that the Ready message is the best one-line explanation.
	summary := c.Status.Summary
	if summary == "" {
		summary = readyMessage(c.Status.Conditions)
	}
	return snapshot{
		Verdict:         fathomv1alpha1.HealthReportResult(c.Status.LastResult),
		Summary:         summary,
		LastRun:         c.Status.LastRunTime,
		NextRun:         nextRun(c.Status.LastRunTime, interval),
		ReportName:      c.Status.LastReportName,
		ConsumedTrigger: c.Status.LastRunTrigger,
	}
}

// nodeHealthCheckTimeout mirrors the controller's effective agent timeout:
// the agent re-evaluates at min(interval, MaxNodeHealthCheckAgentInterval)
// and a pass is bounded by min(timeout, that cadence). Reporting the raw
// spec.timeout made `run --wait` on a 24h/24h check wait a day for a pass the
// agent stops at five minutes.
func nodeHealthCheckTimeout(o client.Object) time.Duration {
	c := o.(*fathomv1alpha1.NodeHealthCheck)
	timeout := effectiveDuration(c.Spec.Timeout, fathomv1alpha1.MinCheckTimeout, fathomv1alpha1.DefaultNodeHealthCheckTimeout)
	return min(timeout, nodeHealthCheckCadence(c))
}

// nodeHealthCheckPassTimeout is the bound on one whole agent pass — the
// evaluation and then the publication of its report, each bounded by the
// effective timeout — which is what `run --wait` must budget for. Budgeting a
// single timeout let a pass whose API write was merely slow be reported as a
// timed-out run.
func nodeHealthCheckPassTimeout(o client.Object) time.Duration {
	return 2 * nodeHealthCheckTimeout(o)
}

// nodeHealthCheckWaitEstimate accounts for the controller's serial
// DaemonSet rollout: each desired node may consume a pod-rollout margin and a
// complete evaluation-and-publication pass. Before status is populated, one
// node is the least surprising estimate for a newly created check.
func nodeHealthCheckWaitEstimate(o client.Object) time.Duration {
	c := o.(*fathomv1alpha1.NodeHealthCheck)
	nodes := max(int64(c.Status.DesiredNodes), 1)
	perNode := saturatingDurationAdd(nodeAgentPodRolloutMargin, nodeHealthCheckPassTimeout(c))
	return saturatingDurationMultiply(perNode, nodes)
}

func saturatingDurationAdd(a, b time.Duration) time.Duration {
	if a >= maxDuration-b {
		return maxDuration
	}
	return a + b
}

func saturatingDurationMultiply(d time.Duration, n int64) time.Duration {
	if d == 0 || n == 0 {
		return 0
	}
	if n > int64(maxDuration/d) {
		return maxDuration
	}
	return d * time.Duration(n)
}

func healthCheckSnapshot(o client.Object) snapshot {
	c := o.(*fathomv1alpha1.HealthCheck)
	var interval time.Duration
	if c.Status.SourceInterval != nil {
		interval = c.Status.SourceInterval.Duration
	}
	return snapshot{
		Verdict:    c.Status.Result,
		Summary:    c.Status.Summary,
		LastRun:    c.Status.SourceObservedAt,
		NextRun:    nextRun(c.Status.SourceObservedAt, interval),
		ReportName: c.Status.LastReportName,
	}
}

func clusterHealthSnapshot(o client.Object) snapshot {
	c := o.(*fathomv1alpha1.ClusterHealth)
	summary := fmt.Sprintf("%d matched", c.Status.MatchedCount)
	if c.Status.Result != "" {
		summary += ", worst " + string(c.Status.Result)
	}
	return snapshot{
		Verdict: c.Status.Result,
		Summary: summary,
		LastRun: c.Status.ObservedAt,
	}
}

// noWaitEstimate is used by derived kinds, which never run themselves.
func noWaitEstimate(client.Object) time.Duration { return 0 }
