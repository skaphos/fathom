/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package declarative provides a reusable Engine that implements
// pkg/adapter.Adapter from an in-memory AddonDefinition, plus a small set of
// Evaluator primitives that hoist the read-and-compare logic shared across the
// hand-written adapters. An addon adapter is expressed as data (an
// AddonDefinition literal) rather than bespoke Go, and the Engine reproduces
// the hand-written adapters' behavior check-for-check.
//
// The engine commits to one uniform semantics for the conventions that drifted
// across the hand-written adapters:
//
//   - Absence of a named singleton is declared per component via Posture:
//     Required -> Fail, Optional -> Skipped.
//   - A managed-resource list that matches zero objects -> Skipped.
//   - An unsupported served CRD version -> Warn by default (per-component
//     override via UnsupportedVersionOutcome).
//   - Family is a first-class explicit field on every emitted CheckResult.
//   - Skipped results carry Details["skipReason"] for machine consumption.
//   - The OutcomeError vs OutcomeFail split is preserved: transport/selector
//     errors -> Error; an object that exists but is unhealthy -> Fail.
package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// Posture declares how the absence of a named singleton target is scored.
type Posture string

const (
	// Required scores a NotFound target as OutcomeFail.
	Required Posture = "Required"
	// Optional scores a NotFound target as OutcomeSkipped (rolls up green).
	Optional Posture = "Optional"
)

// WorkloadKind selects the controller kind a WorkloadCheck reads.
type WorkloadKind string

const (
	// KindDeployment reads an apps/v1 Deployment.
	KindDeployment WorkloadKind = "Deployment"
	// KindDaemonSet reads an apps/v1 DaemonSet.
	KindDaemonSet WorkloadKind = "DaemonSet"
	// KindStatefulSet reads an apps/v1 StatefulSet.
	KindStatefulSet WorkloadKind = "StatefulSet"
)

// AddonDefinition is the complete declarative description of one addon adapter.
//
// The Engine treats it as read-only: Run only reads the definition and copies
// per-run scalars onto the stack, so many goroutines may call Run concurrently.
// The copy the Engine holds is shallow — Families/RBAC share backing arrays with
// the caller's value — so callers MUST NOT mutate a definition (or its nested
// slices) after passing it to NewEngine/MustEngine. In practice definitions are
// compiled-in literals that are never mutated.
type AddonDefinition struct {
	// AddonType is the adapter identity: Name(), the AddonTypes[0] capability,
	// and the AddonCheck.spec.addonType match key.
	AddonType string

	// AdapterVersion is the adapter's own SemVer (Adapter.Version()).
	AdapterVersion string

	// Families are evaluated in slice order. Families[0] is the "primary"
	// family under which the all-disabled sentinel Skipped is emitted.
	Families []FamilyDefinition

	// RBAC holds the +kubebuilder:rbac marker bodies this definition needs.
	// Recorded for documentation and codegen; the engine does not enforce
	// them at runtime (the real markers must live on a scanned Go source file).
	RBAC []RBACRule
}

// RBACRule is a least-privilege grant the definition's evaluators require.
type RBACRule struct {
	// APIGroups is the API group, "" for core (e.g. "apps",
	// "apiextensions.k8s.io").
	APIGroups string
	// Resources is the resource plural (e.g. "deployments").
	Resources string
	// Verbs is the semicolon-separated verb list (e.g. "get;list;watch").
	Verbs string
}

// FamilyDefinition is one policy-keyed check family. Each typed component slice
// contributes zero or more CheckResults, all tagged with Name. The engine
// evaluates components in a fixed within-family order: Workloads, then CRDs,
// then ManagedResources, then APIServices (Webhooks are reserved).
type FamilyDefinition struct {
	// Name is the adapter-defined family identifier and the Request.Policy key.
	Name adapter.Family
	// DefaultEnabled gates the family when no policy entry is present.
	DefaultEnabled bool

	// Workloads verify controller singletons (Deployment/DaemonSet/StatefulSet).
	Workloads []WorkloadCheck
	// CRDs verify a fixed list of CustomResourceDefinitions.
	CRDs []CRDCheck
	// ManagedResources score a status.conditions predicate over listed CRs.
	ManagedResources []ConditionCheck

	// APIServices are declared for cert-manager parity later. They map onto the
	// ConditionCheck evaluator (apiregistration.k8s.io APIService, Available=True)
	// and are not exercised by Cilium.
	APIServices []ConditionCheck
	// Webhooks are reserved; their evaluator ships in a later PR.
	Webhooks []WebhookCheck
}

// evaluators returns the family's components in the fixed within-family order.
// Webhooks are omitted: their evaluator is not shipped in this increment.
func (f FamilyDefinition) evaluators() []Evaluator {
	evals := make([]Evaluator, 0,
		len(f.Workloads)+len(f.CRDs)+len(f.ManagedResources)+len(f.APIServices))
	for _, w := range f.Workloads {
		evals = append(evals, w)
	}
	for _, c := range f.CRDs {
		evals = append(evals, c)
	}
	for _, m := range f.ManagedResources {
		evals = append(evals, m)
	}
	for _, a := range f.APIServices {
		evals = append(evals, a)
	}
	return evals
}

// WorkloadCheck evaluates one controller singleton and, optionally, its pods.
// It generalizes the byte-identical checkDeployment/checkPods logic across
// Deployment / DaemonSet / StatefulSet.
type WorkloadCheck struct {
	// Kind selects the controller kind read.
	Kind WorkloadKind
	// DefaultNamespace is used when policy.Namespaces is empty.
	DefaultNamespace string
	// NameThresholdKey overrides the workload name from policy thresholds;
	// "" disables the override.
	NameThresholdKey string
	// DefaultName is the workload name when no threshold override applies.
	DefaultName string
	// Component is the label value recorded in Details["component"].
	Component string
	// Absence scores a NotFound target (Required -> Fail, Optional -> Skipped).
	Absence Posture

	// CheckPods enables the kind-independent selector -> ready -> restart-warn
	// sub-check.
	CheckPods bool
	// RestartWarnThresholdKey overrides the restart-warn count from policy
	// thresholds; "" disables the override.
	RestartWarnThresholdKey string
	// DefaultRestartWarn is the restart-warn count when no override applies.
	DefaultRestartWarn int32
}

// CRDCheck verifies a fixed, cluster-scoped list of CRDs are established and
// serve a supported version. It takes no namespace, selector, or thresholds.
type CRDCheck struct {
	// Names is the CRD list; one CheckResult is emitted per name.
	Names []string
	// SupportedVersions is a descending-preference slice (e.g. {"v2","v2alpha1"}).
	SupportedVersions []string
	// Absence scores a NotFound CRD (Required -> Fail, Optional -> Skipped).
	Absence Posture
	// UnsupportedVersionOutcome scores an established CRD serving no recognized
	// version; defaults to OutcomeWarn when empty.
	UnsupportedVersionOutcome adapter.Outcome
}

// ConditionCheck lists managed CRs (or APIServices) across namespaces and
// scores a status.conditions[type]==expectedStatus predicate. This is the
// cert-manager passive-check primitive (issuer/cert Ready, APIService
// Available). It is unused by Cilium.
type ConditionCheck struct {
	// APIVersion is the group/version of the listed objects (e.g.
	// "cert-manager.io/v1"). Its version is the fallback when VersionCRD
	// resolution finds nothing.
	APIVersion string
	// VersionCRD, when set, names the CRD (e.g.
	// "externalsecrets.external-secrets.io") whose preferred served version among
	// SupportedVersions overrides APIVersion's version before listing -- so the
	// check keeps working on clusters that serve only a legacy version. When
	// empty, APIVersion's version is used verbatim.
	VersionCRD string
	// SupportedVersions is the ordered candidate list for VersionCRD resolution
	// (e.g. []string{"v1", "v1beta1"}); ignored when VersionCRD is empty.
	SupportedVersions []string
	// Kind is the object kind (e.g. "Issuer").
	Kind string
	// ListKind is the list kind (e.g. "IssuerList").
	ListKind string
	// ListName is the stable placeholder Name used on list-level CheckResults
	// (invalid-selector, list-failure, and no-matching-objects), so they don't
	// collapse onto (Kind, Name=""). Matches the hand-written adapters'
	// convention of a resource-plural label (e.g. "issuers"). Defaults to Kind.
	ListName string
	// ClusterScoped lists without a namespace when true.
	ClusterScoped bool
	// DefaultNamespace is used when policy.Namespaces is empty and the objects
	// are namespaced.
	DefaultNamespace string
	// ConditionType is the status condition inspected (e.g. "Ready").
	ConditionType string
	// ExpectedStatus is the required condition status (e.g. "True").
	ExpectedStatus string
	// AbsentCondition scores a missing condition; defaults to OutcomeFail.
	AbsentCondition adapter.Outcome
	// Mismatch scores a condition whose status differs from ExpectedStatus;
	// defaults to OutcomeFail. An empty result set across all namespaces is
	// always OutcomeSkipped.
	Mismatch adapter.Outcome
}

// WebhookCheck is reserved for cert-manager's ValidatingWebhookConfiguration /
// MutatingWebhookConfiguration client validation. It is declared here to fix
// the schema shape; its evaluator ships in a later PR.
type WebhookCheck struct {
	// Kind is "ValidatingWebhookConfiguration" or "MutatingWebhookConfiguration".
	Kind string
	// Name is the webhook configuration name.
	Name string
	// ExpectedService is the backing service name.
	ExpectedService string
	// ServiceNamespace is the backing service namespace.
	ServiceNamespace string
	// Absence scores a NotFound configuration.
	Absence Posture
}
