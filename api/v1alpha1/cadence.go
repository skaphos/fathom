/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

import "time"

// Default cadences applied when a check's spec.interval or spec.timeout is
// unset. They live in the API package rather than the controllers so that
// clients (fathomctl) computing "next run" and "how long to wait" use the
// same values the controllers schedule with. The CRD field documentation
// quotes these numbers; the controllers apply them at runtime rather than as
// CRD defaults because a stored object may predate them.
const (
	// DefaultAddonCheckInterval is the cadence at which an AddonCheck's adapter
	// re-runs when Spec.Interval is unset. Periodic re-execution is what keeps
	// a HealthReport current: without it a check runs once and its result goes
	// stale the moment the underlying addon degrades.
	DefaultAddonCheckInterval = 5 * time.Minute
	// DefaultAddonCheckTimeout bounds one adapter run when Spec.Timeout is
	// unset.
	DefaultAddonCheckTimeout = 30 * time.Second

	// DefaultDNSCheckInterval is the cadence at which a DNSCheck re-evaluates
	// its targets when Spec.Interval is unset.
	DefaultDNSCheckInterval = time.Minute
	// DefaultDNSCheckTimeout bounds one whole DNSCheck run (every pair, not
	// each pair) when Spec.Timeout is unset.
	DefaultDNSCheckTimeout = 10 * time.Second

	// DefaultNodeCertificateCheckInterval is the node-agent re-scan cadence
	// and the operator's rollup cadence when Spec.Interval is unset.
	DefaultNodeCertificateCheckInterval = time.Hour
	// DefaultNodeCertificateCheckTimeout bounds a node-agent scan when
	// Spec.Timeout is unset.
	DefaultNodeCertificateCheckTimeout = 30 * time.Second
)
