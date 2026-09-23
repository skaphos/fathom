/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

// DefinitionIdentifier is bounded before semantic compilation.
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
// +kubebuilder:validation:Pattern="^[a-z][a-z0-9_-]*$"
type DefinitionIdentifier string

// DefinitionDNSLabel is bounded before semantic compilation.
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
// +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
type DefinitionDNSLabel string

// DefinitionResourceName is bounded before semantic compilation.
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=253
// +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$"
type DefinitionResourceName string

// DefinitionToken is bounded before semantic compilation.
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=253
// +kubebuilder:validation:Pattern="^[A-Za-z][A-Za-z0-9]*$"
type DefinitionToken string

// DefinitionThresholdKey is bounded before semantic compilation.
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
// +kubebuilder:validation:Pattern="^[A-Za-z][A-Za-z0-9_.-]*$"
type DefinitionThresholdKey string

// DefinitionText is bounded before semantic compilation.
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=1024
type DefinitionText string

// DefinitionDuration is bounded before semantic compilation.
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=256
type DefinitionDuration string

// DefinitionSelectorValue is a bounded Kubernetes label value.
// +kubebuilder:validation:MaxLength=256
type DefinitionSelectorValue string

// DefinitionOutcome is an evaluator result; Error is never completed evidence.
// +kubebuilder:validation:Enum=Pass;Warn;Fail;Error;Skipped
type DefinitionOutcome string

// DefinitionPosture specifies how absent targets are scored.
// +kubebuilder:validation:Enum=Required;Optional
type DefinitionPosture string

// DefinitionTarget is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="self.scope == 'Cluster' ? (!has(self.namespaces) || size(self.namespaces) == 0) : (has(self.namespaces) && size(self.namespaces) > 0)",message="Namespaced requires explicit namespaces; Cluster has none"
type DefinitionTarget struct {
	// Scope is the declared scope.
	// +kubebuilder:validation:Enum=Namespaced;Cluster
	Scope string `json:"scope"`
	// +listType=set
	// Namespaces is the declared namespaces.
	// +optional
	// +kubebuilder:validation:MaxItems=32
	Namespaces []DefinitionDNSLabel `json:"namespaces,omitempty"`
}

// DefinitionWorkload is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="self.target.scope == 'Namespaced' && has(self.target.namespaces) && size(self.target.namespaces) == 1",message="requires one namespaced target"
type DefinitionWorkload struct {
	// Target is the declared target.
	Target DefinitionTarget `json:"target"`
	// Kind is the declared kind.
	// +kubebuilder:validation:Enum=Deployment;DaemonSet;StatefulSet
	Kind string `json:"kind"`
	// DefaultName is the declared defaultName.
	DefaultName DefinitionResourceName `json:"defaultName"`
	// NameThresholdKey is the declared nameThresholdKey.
	// +optional
	NameThresholdKey DefinitionThresholdKey `json:"nameThresholdKey,omitempty"`
	// Component is the declared component.
	// +optional
	Component DefinitionIdentifier `json:"component,omitempty"`
	// Absence is the declared absence.
	// +optional
	Absence DefinitionPosture `json:"absence,omitempty"`
	// CheckPods is the declared checkPods.
	// +optional
	// +kubebuilder:default=false
	CheckPods bool `json:"checkPods,omitempty"`
	// RestartWarnThresholdKey is the declared restartWarnThresholdKey.
	// +optional
	RestartWarnThresholdKey DefinitionThresholdKey `json:"restartWarnThresholdKey,omitempty"`
	// DefaultRestartWarn is the declared defaultRestartWarn.
	// +optional
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	DefaultRestartWarn int32 `json:"defaultRestartWarn,omitempty"`
}

// DefinitionCRD is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="self.target.scope == 'Cluster'",message="requires cluster target"
type DefinitionCRD struct {
	// Target is the declared target.
	Target DefinitionTarget `json:"target"`
	// +listType=set
	// Names is the declared names.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	Names []DefinitionResourceName `json:"names"`
	// +listType=set
	// SupportedVersions is the declared supportedVersions.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=8
	SupportedVersions []DefinitionToken `json:"supportedVersions"`
	// Absence is the declared absence.
	// +optional
	Absence DefinitionPosture `json:"absence,omitempty"`
	// UnsupportedVersionOutcome is the declared unsupportedVersionOutcome.
	// +optional
	// +kubebuilder:default=Warn
	UnsupportedVersionOutcome DefinitionOutcome `json:"unsupportedVersionOutcome,omitempty"`
}

// DefinitionCondition is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="has(self.names) != has(self.listKind)",message="choose names or listKind"
// +kubebuilder:validation:XValidation:rule="has(self.versionCRD) == has(self.supportedVersions)",message="versionCRD and supportedVersions must be paired"
// +kubebuilder:validation:XValidation:rule="!has(self.names) || self.target.scope == 'Cluster' || size(self.target.namespaces) == 1",message="named target requires one namespace"
type DefinitionCondition struct {
	// Target is the declared target.
	Target DefinitionTarget `json:"target"`
	// APIVersion is the declared apiVersion.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=507
	// +kubebuilder:validation:Pattern="^([a-z0-9]([a-z0-9.-]*[a-z0-9])?/)?[a-z][a-z0-9]*$"
	APIVersion string `json:"apiVersion"`
	// Kind is the declared kind.
	Kind DefinitionToken `json:"kind"`
	// ListKind is the declared listKind.
	// +optional
	ListKind DefinitionToken `json:"listKind,omitempty"`
	// ListName is the declared listName.
	// +optional
	ListName DefinitionIdentifier `json:"listName,omitempty"`
	// +listType=set
	// Names is the declared names.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	Names []DefinitionResourceName `json:"names,omitempty"`
	// VersionCRD is the declared versionCRD.
	// +optional
	VersionCRD DefinitionResourceName `json:"versionCRD,omitempty"`
	// +listType=set
	// SupportedVersions is the declared supportedVersions.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=8
	SupportedVersions []DefinitionToken `json:"supportedVersions,omitempty"`
	// Absence is the declared absence.
	// +optional
	Absence DefinitionPosture `json:"absence,omitempty"`
	// ConditionType is the declared conditionType.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	ConditionType string `json:"conditionType"`
	// ExpectedStatus is the declared expectedStatus.
	// +kubebuilder:validation:Enum=True;False;Unknown
	ExpectedStatus string `json:"expectedStatus"`
	// AbsentCondition is the declared absentCondition.
	// +optional
	// +kubebuilder:default=Fail
	AbsentCondition DefinitionOutcome `json:"absentCondition,omitempty"`
	// Mismatch is the declared mismatch.
	// +optional
	// +kubebuilder:default=Fail
	Mismatch DefinitionOutcome `json:"mismatch,omitempty"`
}

// DefinitionField is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="!has(self.valueOutcomes) || !(self.expectedValue in self.valueOutcomes)",message="expectedValue outcome cannot be overridden"
type DefinitionField struct {
	// Target is the declared target.
	Target DefinitionTarget `json:"target"`
	// APIVersion is the declared apiVersion.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=507
	// +kubebuilder:validation:Pattern="^([a-z0-9]([a-z0-9.-]*[a-z0-9])?/)?[a-z][a-z0-9]*$"
	APIVersion string `json:"apiVersion"`
	// Kind is the declared kind.
	Kind DefinitionToken `json:"kind"`
	// ListKind is the declared listKind.
	ListKind DefinitionToken `json:"listKind"`
	// ListName is the declared listName.
	// +optional
	ListName DefinitionIdentifier `json:"listName,omitempty"`
	// Absence is the declared absence.
	// +optional
	Absence DefinitionPosture `json:"absence,omitempty"`
	// FieldPath is the declared fieldPath.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=128
	FieldPath []string `json:"fieldPath"`
	// ExpectedValue is the declared expectedValue.
	ExpectedValue DefinitionText `json:"expectedValue"`
	// ValueOutcomes is the declared valueOutcomes.
	// +optional
	// +kubebuilder:validation:MaxProperties=32
	ValueOutcomes map[string]DefinitionOutcome `json:"valueOutcomes,omitempty"`
	// AbsentOutcome is the declared absentOutcome.
	// +optional
	// +kubebuilder:default=Warn
	AbsentOutcome DefinitionOutcome `json:"absentOutcome,omitempty"`
	// OtherOutcome is the declared otherOutcome.
	// +optional
	// +kubebuilder:default=Warn
	OtherOutcome DefinitionOutcome `json:"otherOutcome,omitempty"`
}

// DefinitionWebhook is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="self.target.scope == 'Cluster'",message="requires cluster target"
// +kubebuilder:validation:XValidation:rule="has(self.expectedService) == has(self.serviceNamespace)",message="service name and namespace must be paired"
// +kubebuilder:validation:XValidation:rule="!self.verifyEndpoints || has(self.expectedService)",message="endpoint verification requires a service"
type DefinitionWebhook struct {
	// Target is the declared target.
	Target DefinitionTarget `json:"target"`
	// Kind is the declared kind.
	// +kubebuilder:validation:Enum=MutatingWebhookConfiguration;ValidatingWebhookConfiguration
	Kind string `json:"kind"`
	// Name is the declared name.
	Name DefinitionResourceName `json:"name"`
	// NameThresholdKey is the declared nameThresholdKey.
	// +optional
	NameThresholdKey DefinitionThresholdKey `json:"nameThresholdKey,omitempty"`
	// ExpectedService is the declared expectedService.
	// +optional
	ExpectedService DefinitionDNSLabel `json:"expectedService,omitempty"`
	// ServiceNamespace is the declared serviceNamespace.
	// +optional
	ServiceNamespace DefinitionDNSLabel `json:"serviceNamespace,omitempty"`
	// Absence is the declared absence.
	// +optional
	Absence DefinitionPosture `json:"absence,omitempty"`
	// VerifyEndpoints is the declared verifyEndpoints.
	// +optional
	// +kubebuilder:default=false
	VerifyEndpoints bool `json:"verifyEndpoints,omitempty"`
}

// DefinitionCronJob is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="self.target.scope == 'Namespaced' && has(self.target.namespaces) && size(self.target.namespaces) == 1",message="requires one namespaced target"
type DefinitionCronJob struct {
	// Target is the declared target.
	Target DefinitionTarget `json:"target"`
	// DefaultName is the declared defaultName.
	DefaultName DefinitionResourceName `json:"defaultName"`
	// NameThresholdKey is the declared nameThresholdKey.
	// +optional
	NameThresholdKey DefinitionThresholdKey `json:"nameThresholdKey,omitempty"`
	// Component is the declared component.
	// +optional
	Component DefinitionIdentifier `json:"component,omitempty"`
	// Absence is the declared absence.
	// +optional
	Absence DefinitionPosture `json:"absence,omitempty"`
	// SuccessMaxAgeThresholdKey is the declared successMaxAgeThresholdKey.
	// +optional
	SuccessMaxAgeThresholdKey DefinitionThresholdKey `json:"successMaxAgeThresholdKey,omitempty"`
	// DefaultSuccessMaxAge is the declared defaultSuccessMaxAge.
	// +optional
	// +kubebuilder:default="0s"
	DefaultSuccessMaxAge DefinitionDuration `json:"defaultSuccessMaxAge,omitempty"`
	// StaleOutcome is the declared staleOutcome.
	// +optional
	// +kubebuilder:default=Warn
	StaleOutcome DefinitionOutcome `json:"staleOutcome,omitempty"`
}

// DefinitionConfigMap is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="self.target.scope == 'Namespaced' && has(self.target.namespaces) && size(self.target.namespaces) == 1",message="requires one namespaced target"
type DefinitionConfigMap struct {
	// Target is the declared target.
	Target DefinitionTarget `json:"target"`
	// DefaultName is the declared defaultName.
	DefaultName DefinitionResourceName `json:"defaultName"`
	// NameThresholdKey is the declared nameThresholdKey.
	// +optional
	NameThresholdKey DefinitionThresholdKey `json:"nameThresholdKey,omitempty"`
	// Component is the declared component.
	// +optional
	Component DefinitionIdentifier `json:"component,omitempty"`
	// Absence is the declared absence.
	// +optional
	Absence DefinitionPosture `json:"absence,omitempty"`
	// Key is the declared key.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern="^[-._a-zA-Z0-9]+$"
	Key string `json:"key"`
	// +listType=set
	// RecognizedAPIVersions is the declared recognizedAPIVersions.
	// +optional
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:validation:items:MaxLength=507
	RecognizedAPIVersions []string `json:"recognizedAPIVersions,omitempty"`
	// UnrecognizedOutcome is the declared unrecognizedOutcome.
	// +optional
	// +kubebuilder:default=Warn
	UnrecognizedOutcome DefinitionOutcome `json:"unrecognizedOutcome,omitempty"`
	// InvalidOutcome is the declared invalidOutcome.
	// +optional
	// +kubebuilder:default=Fail
	InvalidOutcome DefinitionOutcome `json:"invalidOutcome,omitempty"`
}

// DefinitionAnnotationStaleness is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="has(self.listKind) != has(self.defaultName)",message="choose collection or named target"
// +kubebuilder:validation:XValidation:rule="!has(self.listKind) || !has(self.nameThresholdKey)",message="collection cannot override a singleton name"
// +kubebuilder:validation:XValidation:rule="has(self.listKind) || self.target.scope == 'Cluster' || size(self.target.namespaces) == 1",message="named target requires one namespace"
type DefinitionAnnotationStaleness struct {
	// Target is the declared target.
	Target DefinitionTarget `json:"target"`
	// APIVersion is the declared apiVersion.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=507
	// +kubebuilder:validation:Pattern="^([a-z0-9]([a-z0-9.-]*[a-z0-9])?/)?[a-z][a-z0-9]*$"
	APIVersion string `json:"apiVersion"`
	// Kind is the declared kind.
	Kind DefinitionToken `json:"kind"`
	// ListKind is the declared listKind.
	// +optional
	ListKind DefinitionToken `json:"listKind,omitempty"`
	// ListName is the declared listName.
	// +optional
	ListName DefinitionIdentifier `json:"listName,omitempty"`
	// DefaultName is the declared defaultName.
	// +optional
	DefaultName DefinitionResourceName `json:"defaultName,omitempty"`
	// NameThresholdKey is the declared nameThresholdKey.
	// +optional
	NameThresholdKey DefinitionThresholdKey `json:"nameThresholdKey,omitempty"`
	// Component is the declared component.
	// +optional
	Component DefinitionIdentifier `json:"component,omitempty"`
	// Absence is the declared absence.
	// +optional
	Absence DefinitionPosture `json:"absence,omitempty"`
	// AnnotationKey is the declared annotationKey.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=317
	AnnotationKey string `json:"annotationKey"`
	// TimestampJSONField is the declared timestampJSONField.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	TimestampJSONField string `json:"timestampJSONField,omitempty"`
	// MaxAgeThresholdKey is the declared maxAgeThresholdKey.
	// +optional
	MaxAgeThresholdKey DefinitionThresholdKey `json:"maxAgeThresholdKey,omitempty"`
	// DefaultMaxAge is the declared defaultMaxAge.
	DefaultMaxAge DefinitionDuration `json:"defaultMaxAge"`
	// StaleOutcome is the declared staleOutcome.
	// +optional
	// +kubebuilder:default=Warn
	StaleOutcome DefinitionOutcome `json:"staleOutcome,omitempty"`
}

// DefinitionPodProjection is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="self.target.scope == 'Namespaced'",message="requires namespaced target"
type DefinitionPodProjection struct {
	// Target is the declared target.
	Target DefinitionTarget `json:"target"`
	// Selector is the declared selector.
	// +kubebuilder:validation:MinProperties=1
	// +kubebuilder:validation:MaxProperties=32
	Selector map[string]DefinitionSelectorValue `json:"selector"`
	// ListName is the declared listName.
	// +optional
	ListName DefinitionIdentifier `json:"listName,omitempty"`
	// Component is the declared component.
	// +optional
	Component DefinitionIdentifier `json:"component,omitempty"`
	// VolumeName is the declared volumeName.
	VolumeName DefinitionDNSLabel `json:"volumeName"`
	// EnvVar is the declared envVar.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern="^[A-Za-z_][A-Za-z0-9_]*$"
	EnvVar string `json:"envVar,omitempty"`
	// MissingOutcome is the declared missingOutcome.
	// +optional
	// +kubebuilder:default=Fail
	MissingOutcome DefinitionOutcome `json:"missingOutcome,omitempty"`
}

// DefinitionReadRule is a bounded runtime definition contract.
// +kubebuilder:validation:XValidation:rule="has(self.resources) != has(self.nonResourceURLs)",message="choose resource or discovery reads"
// +kubebuilder:validation:XValidation:rule="has(self.resources) == has(self.apiGroup)",message="resource rules require apiGroup, including core empty string"
// +kubebuilder:validation:XValidation:rule="!has(self.nonResourceURLs) || (!has(self.resourceNames) && self.verbs.all(v, v == 'get'))",message="discovery rules allow get only"
type DefinitionReadRule struct {
	// APIGroup is the declared apiGroup.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	APIGroup *string `json:"apiGroup,omitempty"`
	// +listType=set
	// Resources is the declared resources.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:items:MaxLength=253
	Resources []string `json:"resources,omitempty"`
	// +listType=set
	// ResourceNames is the declared resourceNames.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	ResourceNames []DefinitionResourceName `json:"resourceNames,omitempty"`
	// +listType=set
	// NonResourceURLs is the declared nonResourceURLs.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:items:MaxLength=1024
	NonResourceURLs []string `json:"nonResourceURLs,omitempty"`
	// +listType=set
	// Verbs is the declared verbs.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=2
	// +kubebuilder:validation:items:Enum=get;list
	Verbs []string `json:"verbs"`
}
