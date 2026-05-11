/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AddonCheckFamilyPolicy configures one adapter-defined family of checks.
type AddonCheckFamilyPolicy struct {
	// Enabled gates execution of this family.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Namespaces narrows this family to resources in specific namespaces. Empty
	// means all namespaces the adapter can read.
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// LabelSelector narrows this family to resources matching the selector.
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

	// Thresholds carries adapter-specific string knobs, such as warnDays or
	// failDays. Adapter documentation defines the supported keys.
	// +optional
	Thresholds map[string]string `json:"thresholds,omitempty"`
}

// AddonCheckSpec defines the desired state of AddonCheck.
type AddonCheckSpec struct {
	// AddonType selects the adapter responsible for this check, such as
	// cert-manager, coredns, or external-secrets.
	// +kubebuilder:validation:MinLength=1
	AddonType string `json:"addonType"`

	// Interval is the desired cadence between successful check runs.
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`

	// Timeout bounds a single adapter run.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Paused prevents the controller from starting new adapter runs.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// Policy configures adapter-defined check families. A missing or empty policy
	// leaves family selection to the adapter defaults.
	// +optional
	Policy map[string]AddonCheckFamilyPolicy `json:"policy,omitempty"`
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

	// LastReportName names the HealthReport created for the most recent run.
	// +optional
	LastReportName string `json:"lastReportName,omitempty"`
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
