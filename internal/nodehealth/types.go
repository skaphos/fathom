/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package nodehealth holds the node-local health evaluation engine and the
// agent<->operator wire contract for NodeHealthCheck (#206).
//
// Like nodecert, the package is deliberately free of Kubernetes client and
// controller-runtime dependencies so the node-agent binary that imports it
// stays small and least-privilege: it measures filesystem headroom, probes the
// kubelet's localhost health endpoint, and dials the container-runtime socket
// — nothing more. Node conditions are NOT evaluated here: they live on the Node
// object, which the operator reads with its own identity, so the agent never
// needs a Node grant. The operator imports the same package for the shared
// NodeReport payload type and the path allowlist.
package nodehealth

import (
	"time"

	"github.com/skaphos/fathom/internal/nodecert"
)

// Outcome is the verdict for a single check on a single node. It is the same
// type nodecert uses so the operator folds both node-scoped kinds through one
// conversion.
type Outcome = nodecert.Outcome

const (
	OutcomePass    = nodecert.OutcomePass
	OutcomeWarn    = nodecert.OutcomeWarn
	OutcomeFail    = nodecert.OutcomeFail
	OutcomeError   = nodecert.OutcomeError
	OutcomeSkipped = nodecert.OutcomeSkipped
)

// Check types, as strings so the wire contract is self-describing without
// importing the API package. They mirror api/v1alpha1.NodeHealthCheckType and
// a test asserts the two sets stay identical.
const (
	TypeDiskHeadroom     = "DiskHeadroom"
	TypeInodeHeadroom    = "InodeHeadroom"
	TypeNodeCondition    = "NodeCondition"
	TypeKubeletHealthz   = "KubeletHealthz"
	TypeContainerRuntime = "ContainerRuntime"
)

// Item is one assertion the agent evaluates, with every threshold already
// resolved by the operator. Zero is a legal threshold ("never warn"), so the
// agent never infers a default from a zero value — the operator applies the
// API defaults before encoding the items into the DaemonSet arguments.
type Item struct {
	// Type is one of the Type* constants.
	Type string `json:"type"`
	// Path is the filesystem location a headroom check measures.
	Path string `json:"path,omitempty"`
	// WarnPercentFree is the free-percent at or below which a headroom check
	// is Warn.
	WarnPercentFree int32 `json:"warnPercentFree,omitempty"`
	// CriticalPercentFree is the free-percent at or below which a headroom
	// check is Fail.
	CriticalPercentFree int32 `json:"criticalPercentFree,omitempty"`
	// SocketPath is the CRI socket a ContainerRuntime check dials.
	SocketPath string `json:"socketPath,omitempty"`
}

// NeedsHostNetwork reports whether evaluating the item requires the agent pod
// to share the node's network namespace.
func (i Item) NeedsHostNetwork() bool { return i.Type == TypeKubeletHealthz }

// NeedsRoot reports whether evaluating the item requires the agent to run as
// root. The CRI socket is root-owned on every mainstream runtime.
func (i Item) NeedsRoot() bool { return i.Type == TypeContainerRuntime }

// AgentEvaluated reports whether the agent evaluates the item at all.
// NodeCondition is the operator's job.
func (i Item) AgentEvaluated() bool { return i.Type != TypeNodeCondition }

// CheckResult is the outcome for one check on one node. It is JSON-serialized
// into the per-node report ConfigMap the operator aggregates.
type CheckResult struct {
	// Type is the check type that produced this result.
	Type string `json:"type"`
	// Path identifies what was measured: the filesystem path for headroom
	// checks, the socket for ContainerRuntime, the condition type for
	// NodeCondition, empty for KubeletHealthz.
	Path string `json:"path,omitempty"`
	// Outcome is the verdict.
	Outcome Outcome `json:"outcome"`
	// Summary is a one-line human-readable description of the outcome.
	Summary string `json:"summary"`
	// PercentFree is the measured free percentage for a headroom check.
	PercentFree *float64 `json:"percentFree,omitempty"`
	// Total is the filesystem's total bytes or inodes for a headroom check.
	Total uint64 `json:"total,omitempty"`
	// Free is the filesystem's free bytes or inodes available to the agent for
	// a headroom check.
	Free uint64 `json:"free,omitempty"`
}

// NodeReport is the full per-node payload the agent writes into its report
// ConfigMap and the operator reads back.
type NodeReport struct {
	// Node is the name of the node the evaluation ran on.
	Node string `json:"node"`
	// CheckName is the NodeHealthCheck that scheduled the evaluation.
	CheckName string `json:"checkName"`
	// ObservedAt is when the evaluation completed.
	ObservedAt time.Time `json:"observedAt"`
	// Aggregate is the worst-case Outcome across Checks.
	Aggregate Outcome `json:"aggregate"`
	// Checks are the per-check results.
	Checks []CheckResult `json:"checks"`
	// Trigger is the on-demand run token the agent was started with (see
	// nodecert.EnvRunTrigger). Empty on routine ticks, so the operator treats
	// an empty value as "not this trigger".
	Trigger string `json:"trigger,omitempty"`
}

// outcomeRank orders outcomes for worst-case aggregation. It matches
// nodecert's ordering (Pass < Skipped < Warn < Fail < Error) so both node-scoped
// kinds fold identically; the function is duplicated rather than exported from
// nodecert because it is five lines and the alternative is widening that
// package's API for one caller.
func outcomeRank(o Outcome) int {
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

// WorstOutcome returns the highest-severity outcome across results, with the
// same fold the operator applies (api/v1alpha1.WorstResult): Skipped is
// informational — "this check did not apply on this node" — and never wins
// while any other outcome is present, so a Pass alongside a Skipped headroom
// path folds to Pass. An empty or all-Skipped slice yields OutcomeSkipped (the
// node observed nothing to grade).
func WorstOutcome(results []CheckResult) Outcome {
	var worst Outcome
	worstRank := 0
	for _, r := range results {
		if r.Outcome == OutcomeSkipped {
			continue
		}
		if rank := outcomeRank(r.Outcome); rank > worstRank {
			worst = r.Outcome
			worstRank = rank
		}
	}
	if worstRank == 0 {
		return OutcomeSkipped
	}
	return worst
}
