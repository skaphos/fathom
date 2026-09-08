/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

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
)
