/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DefinitionCheck is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="has(self.workload) == (self.kind == 'Workload')",message="payload must match kind Workload"
// +kubebuilder:validation:XValidation:rule="has(self.crd) == (self.kind == 'CRD')",message="payload must match kind CRD"
// +kubebuilder:validation:XValidation:rule="has(self.condition) == (self.kind == 'Condition')",message="payload must match kind Condition"
// +kubebuilder:validation:XValidation:rule="has(self.field) == (self.kind == 'Field')",message="payload must match kind Field"
// +kubebuilder:validation:XValidation:rule="has(self.webhook) == (self.kind == 'Webhook')",message="payload must match kind Webhook"
// +kubebuilder:validation:XValidation:rule="has(self.cronJob) == (self.kind == 'CronJob')",message="payload must match kind CronJob"
// +kubebuilder:validation:XValidation:rule="has(self.configMap) == (self.kind == 'ConfigMap')",message="payload must match kind ConfigMap"
// +kubebuilder:validation:XValidation:rule="has(self.annotationStaleness) == (self.kind == 'AnnotationStaleness')",message="payload must match kind AnnotationStaleness"
// +kubebuilder:validation:XValidation:rule="has(self.podProjection) == (self.kind == 'PodProjection')",message="payload must match kind PodProjection"
type DefinitionCheck struct {
	// Name is the declared name.
	Name DefinitionIdentifier `json:"name"`
	// Kind is the declared kind.
	// +kubebuilder:validation:Enum=Workload;CRD;Condition;Field;Webhook;CronJob;ConfigMap;AnnotationStaleness;PodProjection
	Kind string `json:"kind"`
	// Workload is the declared workload.
	// +optional
	Workload *DefinitionWorkload `json:"workload,omitempty"`
	// CRD is the declared crd.
	// +optional
	CRD *DefinitionCRD `json:"crd,omitempty"`
	// Condition is the declared condition.
	// +optional
	Condition *DefinitionCondition `json:"condition,omitempty"`
	// Field is the declared field.
	// +optional
	Field *DefinitionField `json:"field,omitempty"`
	// Webhook is the declared webhook.
	// +optional
	Webhook *DefinitionWebhook `json:"webhook,omitempty"`
	// CronJob is the declared cronJob.
	// +optional
	CronJob *DefinitionCronJob `json:"cronJob,omitempty"`
	// ConfigMap is the declared configMap.
	// +optional
	ConfigMap *DefinitionConfigMap `json:"configMap,omitempty"`
	// AnnotationStaleness is the declared annotationStaleness.
	// +optional
	AnnotationStaleness *DefinitionAnnotationStaleness `json:"annotationStaleness,omitempty"`
	// PodProjection is the declared podProjection.
	// +optional
	PodProjection *DefinitionPodProjection `json:"podProjection,omitempty"`
}

// DefinitionFamily is a bounded runtime definition contract.
type DefinitionFamily struct {
	// Name is the declared name.
	Name DefinitionIdentifier `json:"name"`
	// DefaultEnabled is the declared defaultEnabled.
	// +optional
	// +kubebuilder:default=false
	DefaultEnabled bool `json:"defaultEnabled,omitempty"`
	// +listType=map
	// +listMapKey=name
	// Checks is the declared checks.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	Checks []DefinitionCheck `json:"checks"`
}

// DefinitionVersionSource is a bounded runtime definition contract.
type DefinitionVersionSource struct {
	// FromFamily is the declared fromFamily.
	FromFamily DefinitionIdentifier `json:"fromFamily"`
	// FromComponent is the declared fromComponent.
	FromComponent DefinitionIdentifier `json:"fromComponent"`
	// Container is the declared container.
	// +optional
	Container DefinitionDNSLabel `json:"container,omitempty"`
}

// AddonDefinitionSpec is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="!has(self.supportedVersions) || size(self.supportedVersions) == 0 || has(self.versionSource)",message="version gate requires versionSource"
type AddonDefinitionSpec struct {
	// AddonType is the declared addonType.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="addonType is immutable"
	AddonType DefinitionDNSLabel `json:"addonType"`
	// AdapterVersion is the declared adapterVersion.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	AdapterVersion string `json:"adapterVersion"`
	// SemanticsVersion is the declared semanticsVersion.
	// +kubebuilder:validation:Enum=1
	SemanticsVersion int32 `json:"semanticsVersion"`
	// Optional is the declared optional.
	// +optional
	// +kubebuilder:default=false
	Optional bool `json:"optional,omitempty"`
	// SupportedVersions is the declared supportedVersions.
	// +optional
	// +kubebuilder:validation:MaxLength=256
	SupportedVersions string `json:"supportedVersions,omitempty"`
	// VersionSource is the declared versionSource.
	// +optional
	VersionSource *DefinitionVersionSource `json:"versionSource,omitempty"`
	// +listType=map
	// +listMapKey=name
	// Families is the declared families.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	Families []DefinitionFamily `json:"families"`
	// RequestedReads is the declared requestedReads.
	// +optional
	// +kubebuilder:validation:MaxItems=32
	RequestedReads []DefinitionReadRule `json:"requestedReads,omitempty"`
}

// DefinitionStatusCondition is a bounded runtime definition contract.
type DefinitionStatusCondition struct {
	// Type is the declared type.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	Type string `json:"type"`
	// Status is the declared status.
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status metav1.ConditionStatus `json:"status"`
	// ObservedGeneration is the declared observedGeneration.
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration"`
	// LastTransitionTime is the declared lastTransitionTime.
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	// Reason is the declared reason.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Reason string `json:"reason"`
	// Message is the declared message.
	// +kubebuilder:validation:MaxLength=1024
	Message string `json:"message"`
}

// AddonDefinitionStatus is a bounded runtime definition contract.
type AddonDefinitionStatus struct {
	// ObservedGeneration is the declared observedGeneration.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Revision is the declared revision.
	// +optional
	// +kubebuilder:validation:MaxLength=256
	Revision string `json:"revision,omitempty"`
	// +listType=map
	// +listMapKey=type
	// Conditions is the declared conditions.
	// +optional
	// +kubebuilder:validation:MaxItems=8
	Conditions []DefinitionStatusCondition `json:"conditions,omitempty"`
}

// AddonDefinition declares administrator-bound runtime health checks.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == self.spec.addonType",message="name must equal addonType"
type AddonDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AddonDefinitionSpec `json:"spec"`
	// +optional
	Status AddonDefinitionStatus `json:"status,omitempty"`
}

// AddonDefinitionList contains runtime definitions.
// +kubebuilder:object:root=true
type AddonDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddonDefinition `json:"items"`
}

func init() { SchemeBuilder.Register(&AddonDefinition{}, &AddonDefinitionList{}) }
