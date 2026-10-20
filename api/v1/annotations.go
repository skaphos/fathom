/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1

// AnnotationRunNow is the on-demand run trigger. Setting it on an executable
// check (AddonCheck, DNSCheck, NodeCertificateCheck) to a value that differs
// from the check's status.lastRunTrigger forces a run out of band from the
// interval. The controller records the consumed value in
// status.lastRunTrigger so a given trigger fires exactly once; writers
// (fathomctl run, automation) must therefore write a fresh value each time,
// such as a timestamp plus a nonce, never a constant. A run that carries no
// annotation never clears the recorded value, so a spent token cannot fire
// again by being re-applied.
//
// Derived kinds (HealthCheck, ClusterHealth) ignore the annotation; a client
// that wants to re-check them triggers their sources instead.
const AnnotationRunNow = "fathom.skaphos.io/run-now"

// LabelHealthReportSourceKind and LabelHealthReportSourceName pin a
// HealthReport to the check that produced it. Every controller stamps the
// pair on the reports it writes, and readers (retention pruning, fathomctl
// reports) select on it, so history for one check is a label query rather
// than a scan of every report in the namespace. Kind disambiguates name
// collisions across kinds.
const (
	LabelHealthReportSourceKind = "fathom.skaphos.io/source-kind"
	LabelHealthReportSourceName = "fathom.skaphos.io/source-name"
	// LabelHealthReportSourceUID pins a report to one *instance* of its check:
	// a check deleted and recreated under the same name gets a new UID, so its
	// predecessor's retained history is neither counted against it nor pruned
	// by it (spec 014, FR-013b).
	LabelHealthReportSourceUID = "fathom.skaphos.io/source-uid"
)

// AnnotationHealthReportSourceDeletedAt is stamped (RFC 3339, once, never
// rewritten) on a report the first time the operator finds its check gone.
// Retention measures the window from it; it is lifecycle provenance about a
// deleted owner, not a health verdict — those stay in spec and status (ADR
// 0010). Reports carry no owner reference, so deleting a check keeps them
// for the configured window.
const AnnotationHealthReportSourceDeletedAt = "fathom.skaphos.io/source-deleted-at"
