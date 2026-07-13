/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterHealthSpec defines the desired state of ClusterHealth. ClusterHealth
// is an aggregator: it rolls up the Status of selected HealthCheck resources
// into a single worst-case Result for cluster-wide consumers (dashboards,
// alerting, gates).
type ClusterHealthSpec struct {
	// Selector selects the HealthChecks whose status this aggregate rolls up.
	// An empty or nil selector matches every HealthCheck in scope (all
	// namespaces, or those listed in Namespaces).
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Namespaces narrows the aggregate to HealthChecks in these namespaces.
	// Empty means all namespaces.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=63
	// +kubebuilder:validation:items:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Namespaces []string `json:"namespaces,omitempty"`

	// Description is a human-readable purpose for this aggregate.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Description string `json:"description,omitempty"`
}

// ClusterHealthChildSummary records one HealthCheck's contribution to the
// aggregate. The aggregator never reads HealthReport history; it derives this
// snapshot solely from HealthCheck.Status (per the AGENTS.md invariant).
type ClusterHealthChildSummary struct {
	// Namespace of the contributing HealthCheck.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Namespace string `json:"namespace"`

	// Name of the contributing HealthCheck.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// Result mirrors the contributing HealthCheck's Status.Result.
	// +optional
	Result HealthReportResult `json:"result,omitempty"`

	// Summary mirrors the contributing HealthCheck's Status.Summary.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Summary string `json:"summary,omitempty"`

	// ObservedAt mirrors the contributing HealthCheck's
	// Status.SourceObservedAt, when present.
	// +optional
	ObservedAt *metav1.Time `json:"observedAt,omitempty"`
}

// ClusterHealthStatus defines the observed state of ClusterHealth.
type ClusterHealthStatus struct {
	// ObservedGeneration is the most recent metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions summarize the controller's view of the aggregate.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// Result is the worst-case roll-up across the selected HealthChecks.
	// Empty when no HealthChecks match the selector.
	// +optional
	Result HealthReportResult `json:"result,omitempty"`

	// MatchedCount is the number of HealthChecks selected for this aggregate.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MatchedCount int32 `json:"matchedCount,omitempty"`

	// Children summarizes each selected HealthCheck's contribution.
	// +optional
	// +listType=map
	// +listMapKey=namespace
	// +listMapKey=name
	Children []ClusterHealthChildSummary `json:"children,omitempty"`

	// ObservedAt is when the aggregator last refreshed this status.
	// +optional
	ObservedAt *metav1.Time `json:"observedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Result",type=string,JSONPath=`.status.result`
// +kubebuilder:printcolumn:name="Matched",type=integer,JSONPath=`.status.matchedCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterHealth is the Schema for the clusterhealths API. It is
// cluster-scoped: one object rolls up HealthChecks across all namespaces
// (optionally narrowed by spec.namespaces).
type ClusterHealth struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterHealthSpec   `json:"spec,omitempty"`
	Status ClusterHealthStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterHealthList contains a list of ClusterHealth.
type ClusterHealthList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterHealth `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterHealth{}, &ClusterHealthList{})
}
