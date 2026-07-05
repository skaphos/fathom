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
//   - Absence of a named singleton is declared per component via Posture, or
//     per addon via AddonDefinition.Optional. Required (the default when neither
//     is set) -> Fail; the explicit Optional opt-out -> Skipped. Either way the
//     result carries the adapter.DetailAbsent marker, so "not installed" is
//     queryable independent of the verdict (SKA-526).
//   - A managed-resource list that matches zero objects -> Skipped.
//   - An unsupported served CRD version -> Warn by default (per-component
//     override via UnsupportedVersionOutcome).
//   - Family is a first-class explicit field on every emitted CheckResult.
//   - Skipped results carry Details["skipReason"] for machine consumption.
//   - The OutcomeError vs OutcomeFail split is preserved: transport/selector
//     errors -> Error; an object that exists but is unhealthy -> Fail.
//
// RBAC: the +kubebuilder:rbac markers below are the union of reads every
// declarative AddonDefinition performs. controller-gen aggregates them into the
// operator's aggregate config/rbac/role.yaml. Each definition's own RBAC field
// (Engine.RBACRules) is the per-addon least-privilege source the RBAC generator
// emits scoped roles from (SKA-58); these union markers are retained only until
// the operator drops the addon reads from its own role in favour of
// impersonation. They live on the package doc -- one scanned location owning the
// declarative reads -- because controller-gen did not reliably collect
// equivalent markers placed on individual engine constructors.
//
// +kubebuilder:rbac:groups=apps,resources=deployments;daemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=external-secrets.io,resources=externalsecrets,verbs=get;list;watch
package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// Posture declares how the absence of a named singleton target is scored.
type Posture string

const (
	// Required scores a NotFound target as OutcomeFail. It is the default: an
	// unset component Posture on a non-Optional addon resolves to Required.
	Required Posture = "Required"
	// Optional scores a NotFound target as OutcomeSkipped — the explicit opt-out
	// for an addon (or component) that may legitimately be absent on a cluster.
	// The persisted verdict of an all-Optional-absent run is Skipped, not Pass;
	// only the metrics/tracing FamilyOutcome roll-up relabels Skipped as Pass.
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

	// Optional makes every component default to the Optional Posture (a NotFound
	// target -> Skipped) instead of the required-by-default. It is the addon-wide
	// opt-out for addons that may legitimately be absent on a cluster (e.g. Cilium
	// on a non-Cilium cluster). A component's own non-empty Posture always wins
	// over this default (SKA-526).
	Optional bool

	// VersionSource, when set, names the workload whose app.kubernetes.io/version
	// label (or, failing that, container image tag) reports the installed addon
	// release version. The engine detects it once per Run and surfaces it as
	// Result.DetectedVersion. Nil disables detection (SKA-527).
	VersionSource *VersionSource

	// SupportedVersions is the addon release-version semver RANGE (Masterminds /
	// Helm grammar, e.g. ">=1.14 <2.0") the detected version is gated against.
	// Empty disables gating: a detected version is still surfaced, but never
	// produces a Warn. Requires VersionSource to be set to have any effect. This
	// is the addon *release* version — distinct from the per-CRD served-API
	// versions in CRDCheck/ConditionCheck.SupportedVersions (SKA-527).
	SupportedVersions string

	// Families are evaluated in slice order. Families[0] is the "primary"
	// family under which the all-disabled sentinel Skipped is emitted.
	Families []FamilyDefinition

	// RBAC holds the least-privilege grants this definition's evaluators need.
	// The RBAC generator emits a per-addon read-only ClusterRole from them
	// (surfaced via Engine.RBACRules), and the reconciler impersonates that
	// addon's ServiceAccount when it runs the engine — so these rules are the
	// engine's real, enforced blast radius, not documentation (SKA-58).
	RBAC []adapter.PolicyRule
}

// VersionSource identifies the workload whose app.kubernetes.io/version label
// (primary) or container image tag (fallback) reports the installed addon
// release version, for engine version detection (SKA-527). It carries only the
// addressing fields — it is not a health check.
type VersionSource struct {
	// Kind is the controller kind read (Deployment/DaemonSet/StatefulSet).
	Kind WorkloadKind
	// Namespace is the workload's namespace.
	Namespace string
	// Name is the workload's name.
	Name string
	// Container selects which container's image supplies the fallback tag by
	// name; "" uses the first container. The primary source is the version
	// label, so this only matters for a multi-container workload whose first
	// container is not the addon itself.
	Container string
}

// defaultPosture is the Posture a component inherits when it declares none:
// Optional when the addon opted out via Optional, otherwise Required.
func (d AddonDefinition) defaultPosture() Posture {
	if d.Optional {
		return Optional
	}
	return Required
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
