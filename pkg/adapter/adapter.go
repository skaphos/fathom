/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package adapter

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter is the contract an addon adapter implements to be invoked by
// Beacon during AddonCheck reconciliation.
//
// Implementations must be safe for concurrent use: Beacon may invoke Run on
// the same Adapter instance from multiple goroutines for distinct AddonCheck
// resources.
type Adapter interface {
	// Name returns the stable identifier for this adapter, e.g. "cert-manager".
	// It is matched against AddonCheck.spec.addonType to select an adapter.
	Name() string

	// Version returns the adapter's own SemVer-formatted version, independent
	// of the contract version. Surfaced in HealthReport details for
	// auditability.
	Version() string

	// ContractVersion returns the SemVer-formatted contract version this
	// adapter was built against. Compared against Beacon's [ContractVersion]
	// via [EnsureCompatible] before Run is invoked.
	ContractVersion() string

	// Capabilities describes what this adapter can check. Beacon uses this to
	// validate AddonCheck policy and to route reconciliations.
	Capabilities() Capabilities

	// Run executes the configured checks and returns a Result. The returned
	// error is reserved for adapter-level failures (e.g. inability to reach
	// the API server); per-check failures must be reported as CheckResult
	// entries with [OutcomeFail] or [OutcomeError] inside the Result.
	//
	// Implementations must honor req.Timeout and the context's cancellation.
	Run(ctx context.Context, req Request) (Result, error)
}

// Capabilities advertises what addon kinds and check families an adapter
// supports. Beacon validates AddonCheck policy against this set.
type Capabilities struct {
	// AddonTypes are the addon-kind identifiers this adapter handles.
	// Most adapters declare exactly one (e.g. "cert-manager"); generic
	// adapters may declare several.
	AddonTypes []string

	// Families are the check families this adapter can run. Family names
	// are adapter-defined and are used as keys in Request.Policy.
	Families []Family
}

// Family is the adapter-defined identifier for a group of related checks
// (e.g. "system_health", "issuer_health"). Family names are scoped to the
// adapter that declares them; Beacon does not impose a global namespace.
type Family string

// Request carries everything an Adapter needs to perform a single Run.
// Beacon constructs and owns the Request; adapters should treat all fields
// as read-only.
type Request struct {
	// Client is a controller-runtime client scoped with the adapter's
	// least-privilege RBAC. Adapters must not assume cluster-admin access.
	Client client.Client

	// Logger is the contextual logger. Adapters should derive child loggers
	// with WithValues rather than mutating this one.
	Logger logr.Logger

	// Target identifies the AddonCheck (or other custom resource) driving
	// this Run. Surfaced in CheckResult so reports can be traced back to
	// their source policy.
	Target TargetRef

	// Policy is the per-family configuration parsed from
	// AddonCheck.spec.policy. Families absent from the map are disabled.
	Policy map[Family]FamilyPolicy

	// Timeout is the upper bound for this Run, mirroring the AddonCheck's
	// spec.timeout. Adapters should also honor ctx.Done() — Beacon may
	// cancel mid-Run for reasons unrelated to the timeout.
	Timeout time.Duration

	// ProbeImage is the operator-supplied default container image for
	// adapters that launch probe pods (see internal/probe). Adapters that
	// do not launch probe pods ignore it. Empty means "no operator-level
	// default was configured"; adapters that need a probe image must then
	// fall back to a per-AddonCheck override or an adapter-local default.
	//
	// Available since contract version 0.2.0.
	ProbeImage string
}

// FamilyPolicy is the configuration block for a single check family.
// Thresholds are intentionally untyped (string-keyed strings) so the
// contract does not need to evolve when an adapter introduces a new knob.
type FamilyPolicy struct {
	// Enabled gates execution of the family. A FamilyPolicy with
	// Enabled=false is equivalent to the family being absent from
	// Request.Policy.
	Enabled bool

	// Namespaces narrows the family to a specific set of namespaces.
	// An empty slice means "all namespaces the adapter can see".
	Namespaces []string

	// LabelSelector narrows the family to resources matching the selector.
	// Nil means no label-based narrowing.
	LabelSelector *metav1.LabelSelector

	// Thresholds carries family-specific knobs (e.g. {"warnDays":"14"}).
	// Adapter authors document the keys they consume.
	Thresholds map[string]string
}

// Result is the aggregate output of a single Adapter.Run invocation.
type Result struct {
	// Checks are the individual check outcomes produced by this Run, in the
	// order the adapter chose to emit them. May be empty if no enabled
	// family produced an observation.
	Checks []CheckResult

	// Duration is the wall-clock time the Run consumed. Reported separately
	// from per-check durations so callers can measure adapter overhead.
	Duration time.Duration
}

// CheckResult is the observation for a single check within a family.
type CheckResult struct {
	// Family is the family this check belongs to.
	Family Family

	// Outcome is the verdict for this check.
	Outcome Outcome

	// TargetRef is the resource the check observed. May differ from the
	// driving AddonCheck (e.g. a Certificate the cert-manager adapter
	// inspected on behalf of an AddonCheck).
	TargetRef TargetRef

	// Summary is a one-line, human-readable description of the outcome.
	// Required for non-Pass outcomes; recommended for Pass outcomes.
	Summary string

	// Details is a structured payload for machine consumption. Values are
	// strings to keep CheckResult serializable across process boundaries;
	// encode richer structure as JSON in a single key when needed.
	Details map[string]string

	// ObservedAt is the wall-clock time at which the check completed.
	ObservedAt time.Time

	// Duration is the wall-clock time this individual check consumed.
	Duration time.Duration
}

// Outcome is the verdict for a single check.
type Outcome string

// Outcome values. The split between [OutcomeFail] and [OutcomeError] is
// significant: Fail means the thing being checked is unhealthy; Error means
// the adapter could not determine its state. Aggregators should treat
// these differently.
const (
	// OutcomePass indicates the check observed a healthy state.
	OutcomePass Outcome = "Pass"
	// OutcomeWarn indicates a non-fatal anomaly (e.g. cert expiring soon).
	OutcomeWarn Outcome = "Warn"
	// OutcomeFail indicates the observed target is unhealthy.
	OutcomeFail Outcome = "Fail"
	// OutcomeError indicates the adapter itself could not complete the check.
	OutcomeError Outcome = "Error"
	// OutcomeSkipped indicates the check was intentionally not executed
	// (e.g. family disabled, target excluded by selector).
	OutcomeSkipped Outcome = "Skipped"
)

// Valid reports whether o is one of the defined Outcome constants.
func (o Outcome) Valid() bool {
	switch o {
	case OutcomePass, OutcomeWarn, OutcomeFail, OutcomeError, OutcomeSkipped:
		return true
	}
	return false
}

// TargetRef identifies a Kubernetes object without depending on a specific
// runtime.Object instance. Sufficient for HealthReport persistence.
type TargetRef struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}
