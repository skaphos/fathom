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
// Ready message is the bounded one-line explanation the operator writes.
const readyCondition = "Ready"

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
		Verdict:    fathomv1alpha1.HealthReportResult(c.Status.LastResult),
		Summary:    readyMessage(c.Status.Conditions),
		LastRun:    c.Status.LastRunTime,
		NextRun:    nextRun(c.Status.LastRunTime, interval),
		ReportName: c.Status.LastReportName,
	}
}

func nodeCertificateCheckTimeout(o client.Object) time.Duration {
	c := o.(*fathomv1alpha1.NodeCertificateCheck)
	return effectiveDuration(c.Spec.Timeout, fathomv1alpha1.MinCheckTimeout, fathomv1alpha1.DefaultNodeCertificateCheckTimeout)
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

// noTimeout is the DefaultTimeout of derived kinds, which never run.
func noTimeout(client.Object) time.Duration { return 0 }
