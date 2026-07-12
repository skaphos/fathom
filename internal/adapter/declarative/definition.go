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
// RBAC: declarative adapters' cluster reads are NOT granted to the operator
// ServiceAccount. Each definition's own RBAC field (surfaced via
// Engine.RBACRules) is the per-addon least-privilege source the RBAC generator
// emits a scoped read-only ClusterRole from, into config/rbac/addons/; the
// operator only impersonates that ServiceAccount at run time (SKA-58). There are
// deliberately no +kubebuilder:rbac markers here — the engine's reads are scoped
// per addon, not aggregated onto the operator role.
package declarative

import (
	"time"

	"github.com/skaphos/fathom/pkg/adapter"
)

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

// Webhook configuration kinds a WebhookCheck can read
// (admissionregistration.k8s.io/v1).
const (
	// KindMutatingWebhookConfiguration reads a MutatingWebhookConfiguration.
	KindMutatingWebhookConfiguration = "MutatingWebhookConfiguration"
	// KindValidatingWebhookConfiguration reads a ValidatingWebhookConfiguration.
	KindValidatingWebhookConfiguration = "ValidatingWebhookConfiguration"
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
// then ManagedResources, then APIServices, then Webhooks, then CronJobs, then
// ConfigMaps, then Annotations.
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

	// APIServices map onto the ConditionCheck evaluator, typically in named
	// mode against apiregistration.k8s.io APIService Available=True (see the
	// metrics-server definition).
	APIServices []ConditionCheck
	// Webhooks verify admission webhook configurations are present and wired
	// (caBundle populated, expected backing service).
	Webhooks []WebhookCheck

	// CronJobs verify a batch/v1 CronJob singleton: present, not suspended, and
	// (optionally) its last successful run is recent — the descheduler
	// system_health-as-CronJob and last_run signals.
	CronJobs []CronJobCheck
	// ConfigMaps verify a named ConfigMap holds a data key that parses as YAML
	// (and, optionally, carries a recognized policy apiVersion) — the descheduler
	// policy_validity signal that catches the silent no-op of an unparseable
	// DeschedulerPolicy.
	ConfigMaps []ConfigMapCheck
	// Annotations score the staleness of a timestamp carried in an object's
	// metadata annotation — the kured reboot_state signal for a lock that has
	// been held too long (wedged) or a node that has waited on a reboot window
	// beyond a threshold.
	Annotations []AnnotationStalenessCheck
}

// evaluators returns the family's components in the fixed within-family order.
func (f FamilyDefinition) evaluators() []Evaluator {
	evals := make([]Evaluator, 0,
		len(f.Workloads)+len(f.CRDs)+len(f.ManagedResources)+len(f.APIServices)+
			len(f.Webhooks)+len(f.CronJobs)+len(f.ConfigMaps)+len(f.Annotations))
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
	for _, w := range f.Webhooks {
		evals = append(evals, w)
	}
	for _, c := range f.CronJobs {
		evals = append(evals, c)
	}
	for _, c := range f.ConfigMaps {
		evals = append(evals, c)
	}
	for _, a := range f.Annotations {
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

// ConditionCheck scores a status.conditions[type]==expectedStatus predicate,
// in one of two modes:
//
//   - List mode (Names empty): list managed CRs across the resolved namespaces
//     and score every match. This is the cert-manager passive-check primitive
//     (issuer/cert Ready). An empty result set is Skipped — quiet by design.
//   - Named mode (Names set): Get each named singleton and score it. A NotFound
//     name is scored by Absence (Required by default -> Fail) with the absent
//     marker — the right shape for objects whose existence is itself the check,
//     like an aggregated APIService (metrics-server's v1beta1.metrics.k8s.io).
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
	// Names, when non-empty, switches the check from list-and-score to
	// get-by-name: each named object is fetched and scored individually.
	// policy.LabelSelector does not apply to named gets; a namespaced kind
	// resolves its namespace via firstNamespace (like a WorkloadCheck
	// singleton).
	Names []string
	// Absence scores a NotFound named object (Required -> Fail, Optional ->
	// Skipped), always tagged with the adapter.DetailAbsent marker. Only
	// meaningful in named mode; list mode keeps its NoMatchingObjects skip.
	Absence Posture
	// ClusterScoped lists (or, in named mode, gets) without a namespace when true.
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

// WebhookCheck verifies one named admission webhook configuration (a
// cluster-scoped singleton) is present and wired: at least one webhooks[]
// entry, every entry's clientConfig.caBundle populated, and — when
// ExpectedService is set — every entry pointing at the expected in-cluster
// backing service. An unpopulated caBundle is the classic silent failure of
// CA-injecting addons (istiod, cert-manager's cainjector): the API server
// cannot trust the webhook, so admission either errors or, with failurePolicy
// Ignore, silently stops being enforced.
type WebhookCheck struct {
	// Kind is KindValidatingWebhookConfiguration or
	// KindMutatingWebhookConfiguration.
	Kind string
	// Name is the webhook configuration name.
	Name string
	// NameThresholdKey overrides the configuration name from policy
	// thresholds; "" disables the override. Covers renamed configurations —
	// e.g. istio's revision- and namespace-suffixed
	// istio-validator-<rev>-<ns>.
	NameThresholdKey string
	// ExpectedService is the backing service name every entry's clientConfig
	// must reference; "" disables the service assertion. Set it together with
	// ServiceNamespace (enforced at engine construction).
	ExpectedService string
	// ServiceNamespace is the backing service namespace when policy names no
	// namespaces; the first policy namespace overrides it, because the
	// backing service lives wherever the addon was installed.
	ServiceNamespace string
	// Absence scores a NotFound configuration (Required -> Fail, Optional ->
	// Skipped), always tagged with the adapter.DetailAbsent marker.
	Absence Posture
}

// CronJobCheck evaluates one batch/v1 CronJob singleton. It scores three
// distinct failure modes an addon that runs as a scheduled Job (the descheduler
// CronJob deployment mode) can silently fall into: the CronJob is absent, it is
// suspended (spec.suspend), or — when DefaultSuccessMaxAge is set — its last
// successful run is stale (status.lastSuccessfulTime older than the window, or
// it has scheduled a Job that never completed successfully). A CronJob that has
// simply not fired yet is healthy. Unlike WorkloadCheck it reads no pods: a
// CronJob has no long-running replica set to inspect.
type CronJobCheck struct {
	// DefaultNamespace is used when policy.Namespaces is empty.
	DefaultNamespace string
	// NameThresholdKey overrides the CronJob name from policy thresholds; ""
	// disables the override.
	NameThresholdKey string
	// DefaultName is the CronJob name when no threshold override applies.
	DefaultName string
	// Component is the label value recorded in Details["component"].
	Component string
	// Absence scores a NotFound CronJob (Required -> Fail, Optional -> Skipped).
	Absence Posture

	// SuccessMaxAgeThresholdKey overrides the last-success staleness window from
	// policy thresholds (a Go duration, e.g. "24h"); "" disables the override.
	SuccessMaxAgeThresholdKey string
	// DefaultSuccessMaxAge is the maximum age of status.lastSuccessfulTime before
	// a run is scored stale. Zero disables the recency check entirely — the
	// CronJob is scored on existence and suspend only (the system_health mode).
	DefaultSuccessMaxAge time.Duration
	// StaleOutcome scores a stale or never-successful CronJob; defaults to
	// OutcomeWarn.
	StaleOutcome adapter.Outcome
}

// ConfigMapCheck verifies a named ConfigMap holds a well-formed policy document.
// It scores the silent no-op failure mode where an addon keeps running against
// a ConfigMap whose policy key is missing or unparseable (the descheduler
// deschedules nothing when its DeschedulerPolicy fails to parse). The check is
// deliberately shape-level: it confirms the key exists, the value parses as
// YAML, and — when RecognizedAPIVersions is set — that the document declares a
// recognized policy apiVersion. It does not validate individual strategy or
// plugin names against a specific addon release; that is version-coupled
// addon-specific knowledge the generic engine does not carry.
type ConfigMapCheck struct {
	// DefaultNamespace is used when policy.Namespaces is empty.
	DefaultNamespace string
	// NameThresholdKey overrides the ConfigMap name from policy thresholds; ""
	// disables the override.
	NameThresholdKey string
	// DefaultName is the ConfigMap name when no threshold override applies.
	DefaultName string
	// Component is the label value recorded in Details["component"].
	Component string
	// Absence scores a NotFound ConfigMap (Required -> Fail, Optional -> Skipped).
	Absence Posture

	// Key is the data key holding the policy document (e.g. "policy.yaml").
	Key string
	// RecognizedAPIVersions, when non-empty, requires the parsed document's
	// top-level apiVersion to be one of these values; otherwise the check scores
	// UnrecognizedOutcome. Empty skips the apiVersion assertion.
	RecognizedAPIVersions []string
	// UnrecognizedOutcome scores a document whose apiVersion is not recognized;
	// defaults to OutcomeWarn.
	UnrecognizedOutcome adapter.Outcome
	// InvalidOutcome scores a missing key or a value that does not parse as YAML;
	// defaults to OutcomeFail.
	InvalidOutcome adapter.Outcome
}

// AnnotationStalenessCheck scores the age of a timestamp carried in an object's
// metadata annotation. It generalizes the "how long has this been held / how
// long has this been waiting" question that has no status.conditions to read:
//
//   - Named mode (ListKind empty): Get one object (e.g. the kured DaemonSet that
//     carries the reboot lock in weave.works/kured-node-lock). A NotFound object
//     is scored by Absence. When the annotation is absent the object is healthy
//     (nothing held) -> Pass. When present, its timestamp is parsed and a value
//     older than the window is scored StaleOutcome (a wedged lock).
//   - List mode (ListKind set): list objects of the kind (e.g. Nodes carrying
//     weave.works/kured-most-recent-reboot-needed) and score only those that
//     carry the annotation. Objects without it are not emitted — a cluster with
//     no node waiting on a reboot is quiet. An empty match set is Skipped.
type AnnotationStalenessCheck struct {
	// APIVersion is the group/version of the object read (e.g. "apps/v1", "v1").
	APIVersion string
	// Kind is the object kind (e.g. "DaemonSet", "Node").
	Kind string
	// ListKind, when set, switches the check to list mode (e.g. "NodeList").
	ListKind string
	// ListName is the stable placeholder Name on list-level results (list
	// failure, no-matching-objects); defaults to Kind.
	ListName string
	// ClusterScoped lists (or gets) without a namespace when true.
	ClusterScoped bool
	// DefaultNamespace is used when policy.Namespaces is empty and the object is
	// namespaced.
	DefaultNamespace string
	// NameThresholdKey overrides the named-mode object name from policy
	// thresholds; "" disables the override.
	NameThresholdKey string
	// DefaultName is the named-mode object name when no threshold override
	// applies. Ignored in list mode.
	DefaultName string
	// Component is the label value recorded in Details["component"].
	Component string
	// Absence scores a NotFound named object (Required -> Fail, Optional ->
	// Skipped). Only meaningful in named mode.
	Absence Posture

	// AnnotationKey is the metadata annotation inspected (e.g.
	// "weave.works/kured-node-lock").
	AnnotationKey string
	// TimestampJSONField, when set, parses the annotation value as a JSON object
	// and reads this field as an RFC3339 timestamp (the kured lock stores its
	// acquisition time under "created"); otherwise the whole annotation value is
	// parsed as an RFC3339 timestamp.
	TimestampJSONField string
	// MaxAgeThresholdKey overrides the staleness window from policy thresholds (a
	// Go duration, e.g. "1h"); "" disables the override.
	MaxAgeThresholdKey string
	// DefaultMaxAge is the maximum age of the parsed timestamp before the value
	// is scored stale.
	DefaultMaxAge time.Duration
	// StaleOutcome scores a timestamp older than the window; defaults to
	// OutcomeWarn.
	StaleOutcome adapter.Outcome
}
