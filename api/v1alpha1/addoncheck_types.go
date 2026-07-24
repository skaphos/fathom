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
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

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
