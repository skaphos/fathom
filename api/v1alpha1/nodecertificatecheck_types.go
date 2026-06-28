/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeCertificateCheckSpec defines the desired state of NodeCertificateCheck.
//
// A NodeCertificateCheck scans on-disk X.509 certificates on every selected
// node and reports time-to-expiry before an expiring certificate can take the
// cluster down. The operator runs the scan via a hardened, read-only node-agent
// DaemonSet (one pod per node); each agent publishes its per-node result, and
// the operator rolls those up into a single HealthReport and mirrors the
// aggregate into Status.
type NodeCertificateCheckSpec struct {
	// Paths is the set of on-disk certificate locations each node-agent scans.
	// Every entry is an absolute path to either a PEM-encoded certificate file
	// (read directly) or a directory (scanned recursively, to a bounded depth,
	// for *.crt, *.pem, and *.cert files). Files ending in .conf or .kubeconfig
	// are parsed as kubeconfigs and their embedded client/CA certificates are
	// extracted. Paths the non-root agent cannot read are reported as Skipped,
	// never Fail or Error; paths that do not exist on a node are omitted from the
	// report entirely, so absent distribution defaults do not flood it. When empty, a
	// distribution-agnostic default set covering common kubeadm, k3s/RKE2, etcd,
	// and kubelet certificate locations is used. The operator mounts the parent
	// directory of each configured path into the agent read-only; a configured
	// directory absent on a node is created empty by the kubelet (hostPath
	// DirectoryOrCreate), so prefer narrowing Paths on immutable-OS distributions.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MaxItems=64
	Paths []string `json:"paths,omitempty"`

	// WarnDays is the days-to-expiry threshold at or below which a certificate
	// is reported as Warn. Must be greater than or equal to CriticalDays.
	// +optional
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=0
	WarnDays *int32 `json:"warnDays,omitempty"`

	// CriticalDays is the days-to-expiry threshold at or below which a
	// certificate is reported as Fail. A certificate already past its notAfter
	// time is always Fail regardless of this value.
	// +optional
	// +kubebuilder:default=7
	// +kubebuilder:validation:Minimum=0
	CriticalDays *int32 `json:"criticalDays,omitempty"`

	// NodeSelector restricts which nodes run the agent DaemonSet. An empty
	// selector targets every node.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations are applied to the agent DaemonSet so it can schedule onto
	// tainted nodes. When nil, a default toleration set is applied so the agent
	// also lands on control-plane nodes (where kubeadm certificates live). Set
	// an explicit empty list to apply no tolerations.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Interval is the cadence at which each node-agent re-scans its
	// certificates and the operator refreshes the rolled-up HealthReport.
	// Defaults to 1h when unset.
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`

	// Timeout bounds a single node-agent scan pass. Defaults to 30s when unset.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Paused stops the operator from running the agent DaemonSet and refreshing
	// reports. The agent DaemonSet is removed while paused; the most recent
	// Status snapshot is preserved.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// HistoryLimit caps the number of HealthReports retained for this check.
	// After each new HealthReport the controller deletes the oldest reports
	// beyond the limit. The minimum of 1 keeps Status.LastReportName valid.
	// +optional
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	HistoryLimit *int32 `json:"historyLimit,omitempty"`
}

// NodeCertificateCheckStatus defines the observed state of NodeCertificateCheck.
type NodeCertificateCheckStatus struct {
	// ObservedGeneration is the most recent metadata.generation reconciled by
	// the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions summarize whether the controller accepted the spec and whether
	// the agent DaemonSet is rolled out and reporting.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// LastRunTime records when the operator last rolled up a HealthReport from
	// the node-agent results.
	// +optional
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// LastResult is the aggregate result across all reporting nodes from the
	// most recent roll-up.
	// +kubebuilder:validation:Enum=Pass;Warn;Fail;Error;Skipped;Unknown
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// LastReportName names the HealthReport created for the most recent roll-up.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	LastReportName string `json:"lastReportName,omitempty"`

	// DesiredNodes is the number of nodes the agent DaemonSet targets
	// (DaemonSet status DesiredNumberScheduled).
	// +optional
	DesiredNodes int32 `json:"desiredNodes,omitempty"`

	// ReportingNodes is the number of nodes that have published a scan result
	// the operator consumed in the most recent roll-up.
	// +optional
	ReportingNodes int32 `json:"reportingNodes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Result",type=string,JSONPath=`.status.lastResult`
// +kubebuilder:printcolumn:name="Reporting",type=integer,JSONPath=`.status.reportingNodes`
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.status.desiredNodes`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NodeCertificateCheck is the Schema for the nodecertificatechecks API.
type NodeCertificateCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeCertificateCheckSpec   `json:"spec,omitempty"`
	Status NodeCertificateCheckStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeCertificateCheckList contains a list of NodeCertificateCheck.
type NodeCertificateCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeCertificateCheck `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeCertificateCheck{}, &NodeCertificateCheckList{})
}
