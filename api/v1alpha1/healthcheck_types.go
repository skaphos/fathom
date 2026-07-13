/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CheckTargetRef references a specialized check resource (AddonCheck,
// DNSCheck, NodeHealthCheck, NodeCertificateCheck, ReachabilityCheck) whose
// status a HealthCheck mirrors and surfaces for ClusterHealth aggregation.
type CheckTargetRef struct {
	// APIVersion of the target check resource. When empty, defaults to
	// fathom.skaphos.io/v1alpha1.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the target check resource (e.g., AddonCheck, DNSCheck).
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

	// SourceObservedAt is when the referenced check last completed.
	// +optional
	SourceObservedAt *metav1.Time `json:"sourceObservedAt,omitempty"`

	// LastReportName names the most recent HealthReport produced by the
	// referenced check, when one exists.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	LastReportName string `json:"lastReportName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
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
