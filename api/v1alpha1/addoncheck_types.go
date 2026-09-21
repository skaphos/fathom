/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ThresholdValue is one adapter threshold knob value. It is an ordinary
// string on the wire; the MaxLength bound exists so the API server's CEL cost
// estimator has a real input size for the threshold shape rules below (an
// unbounded map value string prices those rules out of the per-CRD budget).
// +kubebuilder:validation:MaxLength=64
type ThresholdValue string

// AddonCheckFamilyPolicy configures one adapter-defined family of checks.
type AddonCheckFamilyPolicy struct {
	// Enabled gates execution of this family.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// Namespaces narrows this family to resources in specific namespaces. Empty
	// means all namespaces the adapter can read. Each entry must be a valid
	// namespace name (DNS-1123 label); at most 64 entries.
	// +optional
	// +kubebuilder:validation:MaxItems=64
	// +kubebuilder:validation:items:MaxLength=63
	// +kubebuilder:validation:items:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Namespaces []string `json:"namespaces,omitempty"`

	// LabelSelector narrows this family to resources matching the selector.
	// Selector structure and label syntax are validated at reconcile time and
	// reported through the Accepted condition (a CEL admission rule for the
	// structural checks exceeds the API server's per-CRD cost budget, because
	// the imported LabelSelector schema carries no size bounds the estimator
	// could use).
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

	// Thresholds carries adapter-specific string knobs, such as warnDays or
	// failDays. Adapter documentation defines the supported keys; unknown keys
	// are never rejected at admission. Keys documented as numeric are
	// shape-checked at admission: warnDays and failDays must be 1-4 digit
	// integers, warnRatio and failRatio must be percentage-shaped — at most
	// three integer digits, up to two decimals, optional trailing '%'. The
	// 0-100 range and cross-key semantics stay with the adapter and surface
	// via the Accepted condition. At most 16 keys.
	// +optional
	// +kubebuilder:validation:MaxProperties=16
	// +kubebuilder:validation:XValidation:rule="self.all(k, k.matches('^[a-zA-Z0-9]([a-zA-Z0-9_-]{0,61}[a-zA-Z0-9])?$'))",message="threshold keys must be 1-63 alphanumerics with interior '-' or '_'"
	// +kubebuilder:validation:XValidation:rule="self.all(k, !(k in ['warnDays','failDays']) || self[k].matches('^[0-9]{1,4}$'))",message="warnDays and failDays must be whole numbers of days (e.g. \"30\")"
	// +kubebuilder:validation:XValidation:rule="self.all(k, !(k in ['warnRatio','failRatio']) || self[k].matches('^[0-9]{1,3}([.][0-9]{1,2})?%?$'))",message="warnRatio and failRatio must be percentage values with at most two decimals (e.g. \"99.5\" or \"99.5%\")"
	Thresholds map[string]ThresholdValue `json:"thresholds,omitempty"`
}

// AddonCheckSpec defines the desired state of AddonCheck.
// +kubebuilder:validation:XValidation:rule="!has(self.timeout) || duration(self.timeout) >= duration('1s')",message="timeout must be at least 1s"
// +kubebuilder:validation:XValidation:rule="!has(self.interval) || duration(self.interval) >= duration('10s')",message="interval must be at least 10s"
// +kubebuilder:validation:XValidation:rule="!has(self.timeout) || !has(self.interval) || duration(self.timeout) <= duration(self.interval)",message="timeout must not exceed interval"
type AddonCheckSpec struct {
	// AddonType selects the adapter responsible for this check, such as
	// cert-manager, coredns, or external-secrets.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="addonType is immutable"
	AddonType string `json:"addonType"`

	// Interval is the cadence at which the adapter re-runs and the HealthReport
	// is refreshed. Defaults to 5m when unset. Must be at least 10s
	// (MinCheckInterval); the operator clamps stored objects that predate this
	// floor to it at runtime.
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`

	// Timeout bounds a single adapter run. Must be at least 1s
	// (MinCheckTimeout); the operator clamps stored objects that predate this
	// floor to it at runtime.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Paused prevents the controller from starting new adapter runs.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// Policy configures adapter-defined check families. A missing or empty policy
	// leaves family selection to the adapter defaults. Keys are adapter family
	// names: 1-63 lowercase alphanumerics with interior '-' or '_' (e.g.
	// system_health). Whether a well-formed key names a family the selected
	// adapter actually supports is judged at reconcile time via the Accepted
	// condition.
	// +optional
	// +kubebuilder:validation:MaxProperties=32
	// +kubebuilder:validation:XValidation:rule="self.all(k, k.matches('^[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?$'))",message="policy keys must be 1-63 lowercase alphanumerics with interior '-' or '_'"
	Policy map[string]AddonCheckFamilyPolicy `json:"policy,omitempty"`

	// HistoryLimit caps the number of HealthReports retained for this
	// AddonCheck. After each new HealthReport is created the controller
	// deletes the oldest reports until the total count is at or below this
	// limit. The minimum of 1 keeps Status.LastReportName referenceable.
	// +optional
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	HistoryLimit *int32 `json:"historyLimit,omitempty"`
}

// Completed-evidence, attempt and freshness vocabulary for runtime addon
// checks (T044 of specs/012-addon-definition-runtime).
//
// The three axes are deliberately independent, because conflating any two of
// them is how an old success gets presented as new coverage:
//
//   - the Ready condition says whether a run could EXECUTE and COMPLETE with
//     eligible inputs — contracts/runtime.md: "Ready denotes executable/
//     completed, freshness denotes recency, and neither means Pass";
//   - [AddonCheckEvidence] says what the last COMPLETED run observed, carrying
//     its own original observation time, revision and authority context;
//   - [AddonCheckEvidenceFreshness] says whether that observation is still
//     recent and still backed by eligible inputs.
//
// A failed attempt moves only the attempt fields. It never renews the
// evidence or its timestamp: "Attempt Error still preserves previous completed
// evidence."

// AddonCheckEvidenceVerdict is the aggregate verdict of a COMPLETED runtime
// evaluation.
//
// It is deliberately narrower than [AddonCheckStatus.LastResult]: Error and
// Unknown are not completed evidence. Unknown is the absence of evidence
// ("Unknown applies when no evidence exists"), and a run whose aggregate is
// Error could not determine health, so it is recorded as an attempt error that
// preserves whatever evidence was already stored.
// +kubebuilder:validation:Enum=Pass;Warn;Fail;Skipped
type AddonCheckEvidenceVerdict string

// Completed-evidence verdicts. Skipped is evidence, not a failure: a completed
// all-Skipped run replaces current evidence by the user's explicit
// clarification (contracts/runtime.md, "Clarification additions").
const (
	AddonCheckEvidenceVerdictPass    AddonCheckEvidenceVerdict = "Pass"
	AddonCheckEvidenceVerdictWarn    AddonCheckEvidenceVerdict = "Warn"
	AddonCheckEvidenceVerdictFail    AddonCheckEvidenceVerdict = "Fail"
	AddonCheckEvidenceVerdictSkipped AddonCheckEvidenceVerdict = "Skipped"
)

// AddonCheckEvidenceCoverage records what a completed run actually evaluated,
// so a Skipped verdict cannot be mistaken for an assessed-and-healthy one.
// +kubebuilder:validation:Enum=ChecksEvaluated;NoChecksEvaluated
type AddonCheckEvidenceCoverage string

const (
	// AddonCheckCoverageChecksEvaluated means at least one check produced a
	// health observation.
	AddonCheckCoverageChecksEvaluated AddonCheckEvidenceCoverage = "ChecksEvaluated"
	// AddonCheckCoverageNoChecksEvaluated means the run completed but every
	// check was Skipped, or no check ran at all. Its message is exactly
	// [AddonCheckNoChecksEvaluatedMessage].
	AddonCheckCoverageNoChecksEvaluated AddonCheckEvidenceCoverage = "NoChecksEvaluated"
)

// AddonCheckNoChecksEvaluatedMessage is the exact message a completed
// all-Skipped run records, fixed by the clarification in
// specs/012-addon-definition-runtime/spec.md.
const AddonCheckNoChecksEvaluatedMessage = "no checks evaluated"

// AddonCheckEvidenceFreshness describes the recency and eligibility of the
// stored completed evidence. It is derived, never authority: freshness says
// nothing about health, and Current does not mean Pass.
// +kubebuilder:validation:Enum=Current;Stale;Superseded;Unavailable
type AddonCheckEvidenceFreshness string

const (
	// AddonCheckEvidenceCurrent means the evidence was observed at most two
	// effective intervals plus one effective timeout ago, from inputs that are
	// still eligible.
	AddonCheckEvidenceCurrent AddonCheckEvidenceFreshness = "Current"
	// AddonCheckEvidenceStale means the evidence aged past that window. It
	// applies even when the stored verdict was Pass: "Evidence ages out |
	// Freshness=Stale even if stored verdict was Pass".
	AddonCheckEvidenceStale AddonCheckEvidenceFreshness = "Stale"
	// AddonCheckEvidenceSuperseded means the revision or context the evidence
	// was produced under has been replaced.
	AddonCheckEvidenceSuperseded AddonCheckEvidenceFreshness = "Superseded"
	// AddonCheckEvidenceUnavailable means the definition, binding or grants
	// that produced the evidence are gone, invalid or denied — or no evidence
	// has ever been recorded.
	AddonCheckEvidenceUnavailable AddonCheckEvidenceFreshness = "Unavailable"
)

// AddonCheckAttemptOutcome is the outcome of the LATEST attempt, which may be
// older evidence's failed successor.
// +kubebuilder:validation:Enum=Completed;Error
type AddonCheckAttemptOutcome string

const (
	// AddonCheckAttemptCompleted means the run executed to completion with
	// eligible inputs. It is not a verdict.
	AddonCheckAttemptCompleted AddonCheckAttemptOutcome = "Completed"
	// AddonCheckAttemptError means the attempt did not produce completed
	// evidence. Whatever evidence was already stored is preserved unchanged.
	AddonCheckAttemptError AddonCheckAttemptOutcome = "Error"
)

// AddonCheckEvidenceRevision is the immutable runtime revision a completed
// evaluation was produced by: the definition incarnation plus the publication
// provenance (data-model.md, "Snapshot and publication context").
type AddonCheckEvidenceRevision struct {
	// DefinitionUID is the AddonDefinition incarnation. A recreated definition
	// has a new UID and inherits no authority from the old one.
	// +optional
	// +kubebuilder:validation:MaxLength=128
	DefinitionUID string `json:"definitionUID,omitempty"`

	// DefinitionGeneration is the definition's spec generation.
	// +optional
	// +kubebuilder:validation:Minimum=0
	DefinitionGeneration int64 `json:"definitionGeneration,omitempty"`

	// SchemaVersion is the API schema version the definition was compiled from.
	// +optional
	// +kubebuilder:validation:MaxLength=63
	SchemaVersion string `json:"schemaVersion,omitempty"`

	// SemanticsVersion is the declared evaluation semantics version.
	// +optional
	// +kubebuilder:validation:Minimum=0
	SemanticsVersion int32 `json:"semanticsVersion,omitempty"`

	// AdapterVersion is the compiled adapter's own version.
	// +optional
	// +kubebuilder:validation:MaxLength=128
	AdapterVersion string `json:"adapterVersion,omitempty"`

	// OperatorBuild is the operator build that compiled the snapshot.
	// +optional
	// +kubebuilder:validation:MaxLength=128
	OperatorBuild string `json:"operatorBuild,omitempty"`
}

// AddonCheckEvidenceAuthority is the delegated authority and policy context a
// completed evaluation was attributed to. It is recorded so an operator can
// see which administrator-authorized incarnation produced a verdict, and so a
// later run under different authority cannot be mistaken for the same
// observation.
type AddonCheckEvidenceAuthority struct {
	// BindingUID is the AddonDefinitionBinding incarnation that authorized the
	// run.
	// +optional
	// +kubebuilder:validation:MaxLength=128
	BindingUID string `json:"bindingUID,omitempty"`

	// BindingGeneration is the binding's spec generation. Status-only binding
	// writes do not advance it and therefore do not invalidate evidence.
	// +optional
	// +kubebuilder:validation:Minimum=0
	BindingGeneration int64 `json:"bindingGeneration,omitempty"`

	// ServiceAccountUID is the dedicated reader incarnation the evaluation
	// impersonated.
	// +optional
	// +kubebuilder:validation:MaxLength=128
	ServiceAccountUID string `json:"serviceAccountUID,omitempty"`

	// CheckUID is this AddonCheck's incarnation.
	// +optional
	// +kubebuilder:validation:MaxLength=128
	CheckUID string `json:"checkUID,omitempty"`

	// CheckGeneration is the AddonCheck spec generation the run was attributed
	// to.
	// +optional
	// +kubebuilder:validation:Minimum=0
	CheckGeneration int64 `json:"checkGeneration,omitempty"`

	// PolicyDigest fingerprints spec.policy. The policy selects which families
	// run, so evidence produced under one policy is not interchangeable with
	// evidence produced under another at the same generation.
	// +optional
	// +kubebuilder:validation:MaxLength=128
	PolicyDigest string `json:"policyDigest,omitempty"`

	// LeaderEpoch is the leadership epoch observed when the run was admitted.
	// +optional
	LeaderEpoch *DefinitionLeaderEpoch `json:"leaderEpoch,omitempty"`
}

// AddonCheckEvidence is the last COMPLETED evaluation: its verdict, what it
// covered, and the original observation time, revision and authority context
// it was produced under.
//
// Nothing but another completed run replaces it. A failed attempt leaves every
// field here untouched — including ObservedAt, which a failed attempt may never
// renew.
type AddonCheckEvidence struct {
	// Verdict is the aggregate result of the completed run.
	Verdict AddonCheckEvidenceVerdict `json:"verdict"`

	// Coverage distinguishes an assessed verdict from a completed run that
	// evaluated nothing.
	Coverage AddonCheckEvidenceCoverage `json:"coverage"`

	// Message explains the coverage in one line. A completed all-Skipped run
	// records exactly [AddonCheckNoChecksEvaluatedMessage].
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Message string `json:"message,omitempty"`

	// ObservedAt is when the completed run finished. It is the evidence's
	// ORIGINAL observation time and is never advanced by a failed attempt.
	ObservedAt metav1.Time `json:"observedAt"`

	// Revision is the runtime definition revision that produced the evidence.
	// +optional
	Revision AddonCheckEvidenceRevision `json:"revision,omitempty"`

	// Authority is the delegated authority and policy context the evidence was
	// attributed to.
	// +optional
	Authority AddonCheckEvidenceAuthority `json:"authority,omitempty"`
}

// AddonCheckStatus defines the observed state of AddonCheck.
type AddonCheckStatus struct {
	// ObservedGeneration is the most recent metadata.generation reconciled by
	// the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions summarize whether the controller accepted and processed this
	// check specification.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastRunTime records when an adapter run last completed.
	// +optional
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// LastResult is the aggregate result from the most recent adapter run.
	// +kubebuilder:validation:Enum=Pass;Warn;Fail;Error;Skipped;Unknown
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// Absent is the number of checks in the most recent run whose target was not
	// installed — the required-absent Fails and optional-absent Skips alike. It
	// makes "not installed" queryable and distinct from "unhealthy" (a Fail whose
	// target exists) and "disabled" (a Skipped family). Zero when every checked
	// target is present (SKA-526).
	// +optional
	// +kubebuilder:validation:Minimum=0
	Absent int32 `json:"absent,omitempty"`

	// DetectedVersion is the installed addon release version detected on the most
	// recent run (from the addon workload's app.kubernetes.io/version label, else
	// its container image tag). Empty when the adapter does not detect versions or
	// the version was undetectable — the run then proceeds best-effort (SKA-527).
	// +optional
	DetectedVersion string `json:"detectedVersion,omitempty"`

	// LastReportName names the HealthReport created for the most recent run.
	// +optional
	LastReportName string `json:"lastReportName,omitempty"`

	// LastRunTrigger records the value of the fathom.skaphos.io/run-now
	// annotation most recently consumed to force an adapter run. The controller
	// re-runs the adapter whenever the annotation value differs from this, then
	// stores it here so a given on-demand trigger fires exactly once.
	// +optional
	LastRunTrigger string `json:"lastRunTrigger,omitempty"`

	// LastSuccessfulEvaluation is the last COMPLETED evaluation, with its own
	// original observation time, revision and authority context. It is replaced
	// only by another completed run; a failed attempt preserves it byte for
	// byte. Absent means no evidence exists at all, which reads as an Unknown
	// verdict rather than as a healthy or unhealthy one.
	// +optional
	LastSuccessfulEvaluation *AddonCheckEvidence `json:"lastSuccessfulEvaluation,omitempty"`

	// LatestAttemptAt is when the most recent attempt — successful or not —
	// finished. It advances on every attempt, which is precisely what makes it
	// distinguishable from LastSuccessfulEvaluation.ObservedAt.
	// +optional
	LatestAttemptAt *metav1.Time `json:"latestAttemptAt,omitempty"`

	// LatestAttemptOutcome records whether the most recent attempt completed
	// with eligible inputs. Completed is not a verdict and Error does not
	// invalidate stored evidence.
	// +optional
	LatestAttemptOutcome AddonCheckAttemptOutcome `json:"latestAttemptOutcome,omitempty"`

	// LatestAttemptReason is the contract reason for the most recent attempt's
	// outcome, chosen by the publication precedence order.
	// +optional
	// +kubebuilder:validation:MaxLength=128
	LatestAttemptReason string `json:"latestAttemptReason,omitempty"`

	// LatestAttemptMessage explains the most recent attempt's outcome.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	LatestAttemptMessage string `json:"latestAttemptMessage,omitempty"`

	// EvidenceFreshness describes the recency and eligibility of
	// LastSuccessfulEvaluation as of the most recent attempt. It is derived,
	// not authority, and it never implies a healthy verdict.
	// +optional
	EvidenceFreshness AddonCheckEvidenceFreshness `json:"evidenceFreshness,omitempty"`

	// EvidenceFreshnessReason explains a freshness that is not Current.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	EvidenceFreshnessReason string `json:"evidenceFreshnessReason,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=fathom

// AddonCheck is the Schema for the addonchecks API.
type AddonCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AddonCheckSpec   `json:"spec,omitempty"`
	Status AddonCheckStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AddonCheckList contains a list of AddonCheck.
type AddonCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddonCheck `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddonCheck{}, &AddonCheckList{})
}
