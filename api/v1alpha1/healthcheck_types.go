/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CheckTargetRef references a supported specialized check resource
// (AddonCheck, DNSCheck, NodeCertificateCheck, or NodeHealthCheck) whose status
// a HealthCheck mirrors and surfaces for ClusterHealth aggregation.
type CheckTargetRef struct {
	// APIVersion of the target check resource. When empty, defaults to
	// fathom.skaphos.io/v1alpha1.
	// +optional
	// +kubebuilder:validation:MaxLength=317
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the target check resource: AddonCheck, DNSCheck,
	// NodeCertificateCheck, or NodeHealthCheck.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Kind string `json:"kind"`

	// Name of the target check resource.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// Namespace of the target check resource. When empty, the HealthCheck's
	// own namespace is used.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	Namespace string `json:"namespace,omitempty"`
}

// HealthCheckSpec defines the desired state of HealthCheck. A HealthCheck is
// a thin wrapper that mirrors the status of a specialized check resource into
// a uniform shape suitable for ClusterHealth aggregation. HealthCheck does not
// execute checks itself.
type HealthCheckSpec struct {
	// CheckRef identifies the specialized check resource this HealthCheck wraps.
	// It is immutable: retargeting a wrapper would silently repoint its mirrored
	// status snapshot at a different check; replace the HealthCheck instead (SKA-576).
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="checkRef is immutable"
	CheckRef CheckTargetRef `json:"checkRef"`

	// Description is a human-readable purpose for this HealthCheck.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Description string `json:"description,omitempty"`

	// Paused suspends mirroring of the referenced check's status into this
	// HealthCheck. The most recent Status snapshot is preserved while paused.
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// HealthCheckStatus defines the observed state of HealthCheck. The fields are
// derived from the referenced check's status; consumers (notably
// ClusterHealth) read this status without needing to understand any
// specialized check schema.
type HealthCheckStatus struct {
	// ObservedGeneration is the most recent metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions summarize the controller's view of the wrapped check.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// Result is the outcome surfaced from the referenced check's most recent run.
	// +optional
	Result HealthReportResult `json:"result,omitempty"`

	// Summary is a human-readable one-line outcome description.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Summary string `json:"summary,omitempty"`

	// SourceInterval is the cadence the referenced check is expected to run at,
	// after its own defaults and floor clamping. It is a fact about the wrapped
	// check, not a judgement about this one: it is what lets a ClusterHealth
	// aggregate judge staleness relative to cadence, since aggregates select
	// HealthChecks and never see the underlying checks (#277).
	//
	// Empty when the cadence cannot be resolved — an unsupported checkRef kind,
	// a missing target, or a lookup failure — so consumers can tell "runs hourly"
	// from "cadence unknown" rather than reading an absent value as zero.
	// +optional
	SourceInterval *metav1.Duration `json:"sourceInterval,omitempty"`

	// SourceObservedAt is when the referenced check last completed.
	// +optional
	SourceObservedAt *metav1.Time `json:"sourceObservedAt,omitempty"`

	// LastReportName names the most recent HealthReport produced by the
	// referenced check, when one exists.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	LastReportName string `json:"lastReportName,omitempty"`

	// SourceReady mirrors whether the referenced check's most recent run could
	// EXECUTE and COMPLETE with eligible inputs. It is not a verdict:
	// contracts/runtime.md, "Ready denotes executable/completed, freshness
	// denotes recency, and neither means Pass". A false SourceReady beside a
	// Pass Result is the ordinary shape of preserved evidence — the last
	// completed run passed, and the most recent attempt could not run at all.
	//
	// Nil when the referenced check has never published a readiness condition,
	// which is a different statement from "the check is not ready".
	// +optional
	SourceReady *bool `json:"sourceReady,omitempty"`

	// SourceReadyReason is the referenced check's own reason for that
	// readiness — UnknownAddonType, AuthorizationRevoked, AccessDenied,
	// RunCompleted and so on — so an operator can tell why a mirrored verdict
	// is not being refreshed without reading the wrapped check.
	// +optional
	// +kubebuilder:validation:MaxLength=128
	SourceReadyReason string `json:"sourceReadyReason,omitempty"`

	// EvidenceFreshness mirrors the recency and eligibility of the completed
	// evidence behind Result, re-derived from the evidence's age at mirror
	// time. Empty for checks that publish no evidence (every built-in adapter),
	// which is "not applicable" rather than "unavailable".
	//
	// This is the field that keeps a retained Pass from reading as a fresh
	// success: "Freshness=Stale even if stored verdict was Pass."
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
// +kubebuilder:printcolumn:name="Result",type=string,JSONPath=`.status.result`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.checkRef.kind`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.checkRef.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// HealthCheck is the Schema for the healthchecks API.
type HealthCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HealthCheckSpec   `json:"spec,omitempty"`
	Status HealthCheckStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HealthCheckList contains a list of HealthCheck.
type HealthCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HealthCheck `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HealthCheck{}, &HealthCheckList{})
}
