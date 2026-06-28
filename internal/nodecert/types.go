/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package nodecert holds the on-disk X.509 certificate scanning engine and the
// agent<->operator wire contract for NodeCertificateCheck (SKA-49 / SKA-519).
//
// The package is deliberately free of Kubernetes client and controller-runtime
// dependencies so the node-agent binary that imports it stays small and
// least-privilege: it parses certificates from the filesystem and classifies
// time-to-expiry, nothing more. The operator imports the same package for the
// shared NodeReport payload type and the host mount-path computation.
package nodecert

import "time"

// Outcome is the verdict for a single scanned certificate. It mirrors the
// small outcome set used elsewhere in Fathom (pkg/adapter.Outcome) but is
// duplicated here so this package and the node-agent binary do not depend on
// the adapter contract.
type Outcome string

const (
	// OutcomePass indicates the certificate is valid and not near expiry.
	OutcomePass Outcome = "Pass"
	// OutcomeWarn indicates the certificate expires within WarnDays.
	OutcomeWarn Outcome = "Warn"
	// OutcomeFail indicates the certificate is expired or expires within CriticalDays.
	OutcomeFail Outcome = "Fail"
	// OutcomeError indicates the path could not be read or parsed.
	OutcomeError Outcome = "Error"
	// OutcomeSkipped indicates the path does not exist or held no certificates.
	OutcomeSkipped Outcome = "Skipped"
)

// Severity ranks outcomes for worst-case aggregation. Higher is worse. The
// ordering matches HealthReportResult.Severity: Pass < Skipped < Warn < Fail <
// Error so a known Fail outranks an Error-as-could-not-determine only where the
// HealthReport layer decides; within a node scan we keep Error above Fail to
// surface unreadable paths. Callers aggregate with WorstOutcome.
func (o Outcome) severity() int {
	switch o {
	case OutcomePass:
		return 1
	case OutcomeSkipped:
		return 2
	case OutcomeWarn:
		return 3
	case OutcomeFail:
		return 4
	case OutcomeError:
		return 5
	default:
		return 0
	}
}

// WorstOutcome returns the highest-severity outcome across results. An empty
// slice yields OutcomeSkipped (the node observed nothing to report).
func WorstOutcome(results []CertResult) Outcome {
	worst := OutcomeSkipped
	worstRank := worst.severity()
	for _, r := range results {
		if rank := r.Outcome.severity(); rank > worstRank {
			worst = r.Outcome
			worstRank = rank
		}
	}
	return worst
}

// CertResult is the outcome for a single certificate, or for a single scanned
// path that yielded no certificate or an error. It is JSON-serialized into the
// per-node report ConfigMap the operator aggregates.
type CertResult struct {
	// Path is the absolute on-disk path the certificate was read from.
	Path string `json:"path"`
	// Source describes how the certificate was obtained, e.g. "file",
	// "dir:apiserver.crt", "kubeconfig:client:default", "kubeconfig:ca:default".
	Source string `json:"source,omitempty"`
	// Subject is the certificate subject distinguished name.
	Subject string `json:"subject,omitempty"`
	// Issuer is the certificate issuer distinguished name.
	Issuer string `json:"issuer,omitempty"`
	// SANs lists the subject alternative names (DNS names and IP addresses).
	SANs []string `json:"sans,omitempty"`
	// Serial is the hex-encoded certificate serial number.
	Serial string `json:"serial,omitempty"`
	// NotAfter is the certificate expiry time. Zero when no certificate was parsed.
	NotAfter time.Time `json:"notAfter,omitempty"`
	// DaysRemaining is whole days until expiry: ceiling for future expiry,
	// floor (negative) once expired.
	DaysRemaining int `json:"daysRemaining"`
	// Outcome is the verdict for this certificate.
	Outcome Outcome `json:"outcome"`
	// Summary is a one-line human-readable description of the outcome.
	Summary string `json:"summary"`
}

// NodeReport is the full per-node payload the agent writes into its report
// ConfigMap and the operator reads back. It is the wire contract between the
// node-agent DaemonSet and the NodeCertificateCheck controller.
type NodeReport struct {
	// Node is the name of the node the scan ran on.
	Node string `json:"node"`
	// CheckName is the NodeCertificateCheck that scheduled the scan.
	CheckName string `json:"checkName"`
	// ObservedAt is when the scan completed.
	ObservedAt time.Time `json:"observedAt"`
	// Aggregate is the worst-case Outcome across Certs.
	Aggregate Outcome `json:"aggregate"`
	// Certs are the per-certificate results.
	Certs []CertResult `json:"certs"`
}

// Thresholds carries the day-based expiry thresholds for classification.
type Thresholds struct {
	// WarnDays: a certificate with days-remaining at or below this is Warn.
	WarnDays int
	// CriticalDays: a certificate with days-remaining at or below this is Fail.
	CriticalDays int
}
