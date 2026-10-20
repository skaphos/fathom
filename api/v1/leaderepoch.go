/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefinitionLeaderEpoch is a bounded runtime definition contract: the
// leadership epoch under which runtime definition work was admitted.
//
// It is owned by the stable API because AddonCheck evidence (and therefore
// HealthReport attribution) records it; the alpha AddonDefinitionBinding
// status references this type rather than the other way round, so the frozen
// v1 schema never depends on an alpha package (spec 014, research R2).
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
	AcquireTime metav1.MicroTime `json:"acquireTime"`
	// LeaseTransitions is the declared leaseTransitions.
	// +kubebuilder:validation:Minimum=0
	LeaseTransitions int32 `json:"leaseTransitions"`
}
