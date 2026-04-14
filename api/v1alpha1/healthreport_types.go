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

// HealthReportSpec defines the desired state of HealthReport.
type HealthReportSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of HealthReport. Edit healthreport_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// HealthReportStatus defines the observed state of HealthReport.
type HealthReportStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
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
