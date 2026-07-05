/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HealthReportResult is the aggregate result for a report or individual check.
// +kubebuilder:validation:Enum=Pass;Warn;Fail;Error;Skipped;Unknown
type HealthReportResult string

const (
	HealthReportResultPass    HealthReportResult = "Pass"
	HealthReportResultWarn    HealthReportResult = "Warn"
	HealthReportResultFail    HealthReportResult = "Fail"
	HealthReportResultError   HealthReportResult = "Error"
	HealthReportResultSkipped HealthReportResult = "Skipped"
	HealthReportResultUnknown HealthReportResult = "Unknown"
)

// Severity returns the relative severity of this Result for worst-case
// aggregation across multiple Results. Higher value = worse. The ordering
// is: Pass(1) < Skipped(2) < Warn(3) < Unknown(4) < Fail(5) < Error(6).
// Empty or unrecognized values return 0; callers should treat that as
// "exclude from worst-case aggregation" (e.g. the input was never observed,
// like a HealthCheck that has not yet been reconciled).
//
// Unknown sits above Warn and below Fail rather than short-circuiting the
// aggregate: a known Fail is more actionable than an Unknown, so when both
// appear in the same set the aggregate reports Fail.
func (r HealthReportResult) Severity() int {
	switch r {
	case HealthReportResultPass:
		return 1
	case HealthReportResultSkipped:
		return 2
	case HealthReportResultWarn:
		return 3
	case HealthReportResultUnknown:
		return 4
	case HealthReportResultFail:
		return 5
	case HealthReportResultError:
		return 6
	default:
		return 0
	}
}

// HealthReportTargetRef identifies a Kubernetes object observed by a check.
type HealthReportTargetRef struct {
	// APIVersion is the target object's API version.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind is the target object's kind.
	// +optional
	// +kubebuilder:validation:MaxLength=63
	Kind string `json:"kind,omitempty"`

	// Namespace is the target object's namespace, if namespaced.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	Namespace string `json:"namespace,omitempty"`

	// Name is the target object's name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// HealthReportCheck records one adapter-emitted check result.
type HealthReportCheck struct {
	// Family is the adapter-defined check family that produced this result.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Family string `json:"family"`

	// Result is this check's outcome.
	Result HealthReportResult `json:"result"`

	// TargetRef is the observed resource for this check.
	TargetRef HealthReportTargetRef `json:"targetRef"`

	// Summary is a human-readable one-line outcome description.
	// +optional
	Summary string `json:"summary,omitempty"`

	// Details is adapter-defined structured context for the check.
	// +optional
	Details map[string]string `json:"details,omitempty"`

	// ObservedAt is when this check completed.
	ObservedAt metav1.Time `json:"observedAt"`

	// Duration is how long this check took.
	// +optional
	Duration *metav1.Duration `json:"duration,omitempty"`
}

// HealthReportSpec defines the desired state of HealthReport.
type HealthReportSpec struct {
	// SourceRef identifies the check resource that produced this report.
	SourceRef HealthReportTargetRef `json:"sourceRef"`

	// AddonType is the AddonCheck addon type used to select the adapter.
	// +optional
	AddonType string `json:"addonType,omitempty"`

	// AdapterName is the adapter identity that produced this report.
	// +optional
	AdapterName string `json:"adapterName,omitempty"`

	// AdapterVersion is the adapter implementation version.
	// +optional
	AdapterVersion string `json:"adapterVersion,omitempty"`

	// DetectedVersion is the installed addon release version detected for this
	// run, or empty when undetectable or not detected. Distinct from
	// AdapterVersion (the adapter's own version) — SKA-527.
	// +optional
	DetectedVersion string `json:"detectedVersion,omitempty"`

	// ContractVersion is the adapter contract version used for this run.
	// +optional
	ContractVersion string `json:"contractVersion,omitempty"`

	// Result is the aggregate outcome across all checks.
	Result HealthReportResult `json:"result"`

	// Checks are the individual observations produced by the adapter.
	// +optional
	Checks []HealthReportCheck `json:"checks,omitempty"`

	// ObservedAt is when the adapter run completed.
	ObservedAt metav1.Time `json:"observedAt"`

	// Duration is the total adapter run duration.
	// +optional
	Duration *metav1.Duration `json:"duration,omitempty"`
}

// HealthReportStatus defines the observed state of HealthReport.
type HealthReportStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// HealthReport is the Schema for the healthreports API.
type HealthReport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HealthReportSpec   `json:"spec,omitempty"`
	Status HealthReportStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HealthReportList contains a list of HealthReport.
type HealthReportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HealthReport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HealthReport{}, &HealthReportList{})
}
