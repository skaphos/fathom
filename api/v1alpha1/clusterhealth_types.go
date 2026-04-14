/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClusterHealthSpec defines the desired state of ClusterHealth.
type ClusterHealthSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ClusterHealth. Edit clusterhealth_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// ClusterHealthStatus defines the observed state of ClusterHealth.
type ClusterHealthStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ClusterHealth is the Schema for the clusterhealths API.
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
