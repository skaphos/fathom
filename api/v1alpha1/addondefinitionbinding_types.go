/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DefinitionObjectReference is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="reference is immutable"
type DefinitionObjectReference struct {
	// Name is the declared name.
	Name DefinitionResourceName `json:"name"`
	// UID is the declared uid.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	UID string `json:"uid"`
}

// DefinitionReference identifies one canonical AddonDefinition revision owner.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="reference is immutable"
type DefinitionReference struct {
	// Name is the definition's canonical addon identity.
	Name DefinitionDNSLabel `json:"name"`
	// UID prevents authority from surviving definition recreation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	UID string `json:"uid"`
}

// DefinitionBindingScope is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="self.allowClusterScoped || (has(self.namespaces) && size(self.namespaces) > 0)",message="at least one scope is required"
type DefinitionBindingScope struct {
	// +listType=set
	// Namespaces is the declared namespaces.
	// +optional
	// +kubebuilder:validation:MaxItems=32
	Namespaces []DefinitionDNSLabel `json:"namespaces,omitempty"`
	// AllowClusterScoped is the declared allowClusterScoped.
	// +optional
	// +kubebuilder:default=false
	AllowClusterScoped bool `json:"allowClusterScoped,omitempty"`
}

// AddonDefinitionBindingSpec is a bounded runtime definition contract.
type AddonDefinitionBindingSpec struct {
	// DefinitionRef is the declared definitionRef.
	DefinitionRef DefinitionReference `json:"definitionRef"`
	// ServiceAccountRef is the declared serviceAccountRef.
	ServiceAccountRef DefinitionObjectReference `json:"serviceAccountRef"`
	// Enabled is the declared enabled.
	// +optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
	// TargetScope is the declared targetScope.
	TargetScope DefinitionBindingScope `json:"targetScope"`
}

// DefinitionLeaderEpoch is a bounded runtime definition contract.
type DefinitionLeaderEpoch struct {
	// LeaseUID is the declared leaseUID.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	LeaseUID string `json:"leaseUID"`
	// HolderIdentity is the declared holderIdentity.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	HolderIdentity string `json:"holderIdentity"`
	// AcquireTime is the declared acquireTime.
	AcquireTime metav1.Time `json:"acquireTime"`
	// LeaseTransitions is the declared leaseTransitions.
	// +kubebuilder:validation:Minimum=0
	LeaseTransitions int32 `json:"leaseTransitions"`
}

// AddonDefinitionBindingStatus describes observation, never authorization.
// Accepted records spec/identity validity; Ready records eligibility; Drained is
// acknowledged only for disabled, observed-generation state with zero active
// runs in the current leadership epoch. Consumers must independently verify the
// epoch against the configured Lease before treating Drained as current.
type AddonDefinitionBindingStatus struct {
	// ObservedGeneration is the declared observedGeneration.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ActiveRuns is the declared activeRuns.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4
	ActiveRuns int32 `json:"activeRuns,omitempty"`
	// LeaderIdentity is the declared leaderIdentity.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	LeaderIdentity string `json:"leaderIdentity,omitempty"`
	// LeaderEpoch is the declared leaderEpoch.
	// +optional
	LeaderEpoch *DefinitionLeaderEpoch `json:"leaderEpoch,omitempty"`
	// +listType=map
	// +listMapKey=type
	// Conditions is the declared conditions.
	// +optional
	// +kubebuilder:validation:MaxItems=8
	Conditions []DefinitionStatusCondition `json:"conditions,omitempty"`
}

// AddonDefinitionBinding delegates a definition to a dedicated identity.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:validation:XValidation:rule="self.metadata.name == self.spec.definitionRef.name",message="name must equal definitionRef.name"
type AddonDefinitionBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AddonDefinitionBindingSpec `json:"spec"`
	// +optional
	Status AddonDefinitionBindingStatus `json:"status,omitempty"`
}

// AddonDefinitionBindingList contains delegation bindings.
// +kubebuilder:object:root=true
type AddonDefinitionBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddonDefinitionBinding `json:"items"`
}

func init() { SchemeBuilder.Register(&AddonDefinitionBinding{}, &AddonDefinitionBindingList{}) }
