/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeHealthCheckType selects what a NodeHealthCheck asserts on each node.
//
// The five types are not equal in what they cost the agent. DiskHeadroom and
// InodeHeadroom are a statfs over a read-only hostPath mount and keep the
// hardened default profile. NodeCondition is evaluated by the operator, not
// the agent, and needs a cluster-wide read on Node objects. KubeletHealthz
// needs hostNetwork, because the kubelet's health endpoint binds to
// localhost. ContainerRuntime needs the runtime socket mounted and the agent
// running as root to open it. The operator grants each privilege only when a
// check of that type is present, so a spec that declares only headroom checks
// never pays for the others (#206).
// +kubebuilder:validation:Enum=DiskHeadroom;InodeHeadroom;NodeCondition;KubeletHealthz;ContainerRuntime
type NodeHealthCheckType string

const (
	// NodeHealthCheckDiskHeadroom asserts the percentage of free bytes on the
	// filesystem holding Path stays above the thresholds.
	NodeHealthCheckDiskHeadroom NodeHealthCheckType = "DiskHeadroom"
	// NodeHealthCheckInodeHeadroom asserts the percentage of free inodes on the
	// filesystem holding Path stays above the thresholds.
	NodeHealthCheckInodeHeadroom NodeHealthCheckType = "InodeHeadroom"
	// NodeHealthCheckNodeCondition asserts the node's own status conditions
	// carry their healthy value (Ready=True; every pressure condition False).
	NodeHealthCheckNodeCondition NodeHealthCheckType = "NodeCondition"
	// NodeHealthCheckKubeletHealthz asserts the kubelet answers its localhost
	// /healthz endpoint.
	NodeHealthCheckKubeletHealthz NodeHealthCheckType = "KubeletHealthz"
	// NodeHealthCheckContainerRuntime asserts the container runtime accepts
	// connections on its CRI socket.
	NodeHealthCheckContainerRuntime NodeHealthCheckType = "ContainerRuntime"
)

// Runtime defaults for the per-check fields that the schema cannot default.
// The check item is a discriminated union, so a CRD default on a per-type
// field would populate it on every item — including types the field is
// rejected on — and break the union rule at admission. The agent, controller,
// and fathomctl apply these instead, so an unset field always means the same
// number everywhere.
const (
	// DefaultNodeHealthWarnPercentFree is the free-percent threshold at or
	// below which a headroom check is Warn when WarnPercentFree is unset.
	DefaultNodeHealthWarnPercentFree int32 = 20
	// DefaultNodeHealthCriticalPercentFree is the free-percent threshold at or
	// below which a headroom check is Fail when CriticalPercentFree is unset.
	DefaultNodeHealthCriticalPercentFree int32 = 10
	// DefaultNodeHealthContainerRuntimeSocket is the CRI socket a
	// ContainerRuntime check dials when SocketPath is unset.
	DefaultNodeHealthContainerRuntimeSocket = "/run/containerd/containerd.sock"

	// MaxNodeHealthNodeResults caps NodeHealthCheckStatus.NodeResults. It
	// mirrors the MaxItems marker on that field, and a test asserts the two stay
	// in step. The cap bounds only what is *reported*: the verdict and the
	// coverage signal are computed across every node before truncation, so a
	// large fleet stays correct — it simply stops enumerating every node.
	MaxNodeHealthNodeResults = 100
)

// DefaultNodeHealthConditions returns the node condition types a
// NodeCondition check asserts when Conditions is unset: Ready must be True and
// each pressure condition must be False.
func DefaultNodeHealthConditions() []string {
	return []string{"Ready", "MemoryPressure", "DiskPressure", "PIDPressure"}
}

// NodeHealthCheckItem is one assertion made on every node in scope. Which
// fields are legal depends on Type; the rules below reject a field on a type
// that does not use it, so a misapplied threshold is a write-time error and
// not a silently ignored one.
// +kubebuilder:validation:XValidation:rule="(self.type == 'DiskHeadroom' || self.type == 'InodeHeadroom') == has(self.path)",message="path is required for DiskHeadroom and InodeHeadroom, and must be omitted for every other type"
// +kubebuilder:validation:XValidation:rule="self.type == 'DiskHeadroom' || self.type == 'InodeHeadroom' || (!has(self.warnPercentFree) && !has(self.criticalPercentFree))",message="warnPercentFree and criticalPercentFree apply only to DiskHeadroom and InodeHeadroom"
// +kubebuilder:validation:XValidation:rule="!has(self.warnPercentFree) || !has(self.criticalPercentFree) || self.warnPercentFree >= self.criticalPercentFree",message="warnPercentFree must be greater than or equal to criticalPercentFree"
// +kubebuilder:validation:XValidation:rule="self.type == 'NodeCondition' || !has(self.conditions)",message="conditions applies only to NodeCondition"
// +kubebuilder:validation:XValidation:rule="self.type == 'ContainerRuntime' || !has(self.socketPath)",message="socketPath applies only to ContainerRuntime"
// The path allowlist stops a namespaced tenant from turning the node-agent into
// a confused deputy that mounts arbitrary host directories; it is mirrored in
// internal/nodehealth so the operator re-checks it on clusters running an older
// CRD. The host root is never allowed: statfs of /var/lib/kubelet reports the
// root filesystem on any node where it is not a separate mount.
// +kubebuilder:validation:XValidation:rule="!has(self.path) || (self.path.startsWith('/') && self.path != '/' && !self.path.contains('..') && ['/var/lib/kubelet','/var/lib/containerd','/var/lib/docker','/var/lib/etcd','/var/log','/var/lib/rancher','/etc/kubernetes','/run/containerd'].exists(a, self.path == a || self.path.startsWith(a + '/')))",message="path must be an absolute, traversal-free path under an allowed prefix (/var/lib/kubelet, /var/lib/containerd, /var/lib/docker, /var/lib/etcd, /var/log, /var/lib/rancher, /etc/kubernetes, /run/containerd)"
// +kubebuilder:validation:XValidation:rule="!has(self.socketPath) || (self.socketPath.endsWith('.sock') && !self.socketPath.contains('..') && ['/run/containerd','/var/run/containerd','/run/crio','/var/run/crio','/run/cri-dockerd','/var/run/cri-dockerd'].exists(a, self.socketPath.startsWith(a + '/')))",message="socketPath must be a traversal-free .sock path under an allowed runtime directory (/run/containerd, /var/run/containerd, /run/crio, /var/run/crio, /run/cri-dockerd, /var/run/cri-dockerd)"
type NodeHealthCheckItem struct {
	// Type is the assertion this item makes. See NodeHealthCheckType for the
	// privilege each type costs the node-agent.
	Type NodeHealthCheckType `json:"type"`

	// Path is a location on the filesystem whose headroom is measured. Required
	// for DiskHeadroom and InodeHeadroom and rejected for every other type. The
	// operator mounts it into the agent read-only, so it must be a traversal-free
	// absolute path under one of the operator-approved prefixes.
	//
	// The mounted directory is created empty by the kubelet when it does not
	// exist on a node (hostPath DirectoryOrCreate, as for NodeCertificateCheck),
	// so the measurement is then the headroom of the filesystem that would hold
	// it — usually the root filesystem — and the node's filesystem is mutated
	// by that directory. Prefer paths that exist on every node in scope. Only a
	// path that is missing beneath the mount (a subdirectory the agent cannot
	// stat) is reported Skipped.
	// +optional
	// +kubebuilder:validation:MaxLength=512
	Path string `json:"path,omitempty"`

	// WarnPercentFree is the percentage of free space (or inodes) at or below
	// which the check is Warn. Applies only to the headroom types. Defaults to
	// 20 (DefaultNodeHealthWarnPercentFree) when unset. Must be greater than or
	// equal to CriticalPercentFree.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	WarnPercentFree *int32 `json:"warnPercentFree,omitempty"`

	// CriticalPercentFree is the percentage of free space (or inodes) at or
	// below which the check is Fail. Applies only to the headroom types.
	// Defaults to 10 (DefaultNodeHealthCriticalPercentFree) when unset.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	CriticalPercentFree *int32 `json:"criticalPercentFree,omitempty"`

	// Conditions names the node condition types a NodeCondition check asserts.
	// Ready is expected True; every other listed condition is expected False,
	// which is the healthy value for every pressure and unavailability
	// condition Kubernetes and the cloud providers define. Defaults to Ready,
	// MemoryPressure, DiskPressure, and PIDPressure when unset. A condition the
	// node does not report is Skipped, not Fail. Applies only to NodeCondition.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=63
	Conditions []string `json:"conditions,omitempty"`

	// SocketPath is the CRI socket a ContainerRuntime check dials. Defaults to
	// /run/containerd/containerd.sock when unset. The operator mounts the socket
	// itself (hostPath type Socket), so a node without a socket at this path
	// cannot schedule the agent and surfaces as incomplete coverage rather than
	// as a verdict. Applies only to ContainerRuntime.
	// +optional
	// +kubebuilder:validation:MaxLength=512
	SocketPath string `json:"socketPath,omitempty"`
}

// NodeHealthCheckSpec defines the desired state of NodeHealthCheck.
//
// A NodeHealthCheck asserts a set of node-local health signals on every
// selected node — filesystem headroom, the node's own conditions, kubelet
// health, and container-runtime liveness — and reports one verdict per node
// plus a single folded verdict for the check. The node-local signals are
// collected by the same hardened node-agent DaemonSet NodeCertificateCheck
// uses; the operator rolls the per-node reports into a HealthReport and
// mirrors the aggregate into Status.
//
// There is no field to pause a NodeHealthCheck. Stopping a check means
// deleting it (#262).
// +kubebuilder:validation:XValidation:rule="!has(self.timeout) || duration(self.timeout) >= duration('1s')",message="timeout must be at least 1s"
// +kubebuilder:validation:XValidation:rule="!has(self.interval) || duration(self.interval) >= duration('10s')",message="interval must be at least 10s"
// +kubebuilder:validation:XValidation:rule="!has(self.timeout) || !has(self.interval) || duration(self.timeout) <= duration(self.interval)",message="timeout must not exceed interval"
// Per-node results and HealthReport checks are keyed by (type, path), so two
// items identical in both would collide there. Rejecting the duplicate at
// write time is far kinder than a status update failing later with an error
// that says nothing about the specification that caused it.
// +kubebuilder:validation:XValidation:rule="self.checks.all(c, self.checks.filter(o, o.type == c.type && (has(o.path) ? (has(c.path) && o.path == c.path) : !has(c.path))).size() == 1)",message="checks must be unique by type and path"
type NodeHealthCheckSpec struct {
	// Checks are the assertions made on every node in scope. At least one is
	// required — a check with no items would report a vacuous pass.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	Checks []NodeHealthCheckItem `json:"checks"`

	// NodeSelector restricts which nodes run the agent DaemonSet. An empty
	// selector targets every node.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations are applied verbatim to the agent DaemonSet so it can schedule
	// onto nodes carrying arbitrary taints. Control-plane tolerations are NOT
	// added here — use IncludeControlPlaneNodes for that, so scheduling the
	// agent onto control-plane nodes is always an explicit, auditable opt-in.
	//
	// An omitted list and an empty list mean the same thing: no tolerations.
	// The operator never distinguishes the two, so the nil-vs-empty round-trip
	// the sibling kind carries (#150) has no effect here.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// IncludeControlPlaneNodes opts the node-agent into scheduling on
	// control-plane nodes by adding tolerations for the standard control-plane
	// and legacy master taints on top of any Tolerations. It defaults to false:
	// a KubeletHealthz or ContainerRuntime check runs the agent with host
	// network or as root, so landing it on a control-plane node is a decision
	// the author makes, not a default.
	// +optional
	// +kubebuilder:default=false
	IncludeControlPlaneNodes *bool `json:"includeControlPlaneNodes,omitempty"`

	// Interval is the cadence at which the operator refreshes the rolled-up
	// HealthReport and the check's liveness. Defaults to 5m when unset. Must
	// be at least 10s (MinCheckInterval); the operator clamps stored objects
	// that predate this floor to it at runtime.
	//
	// The node-agent re-evaluates at min(interval, 5m), and a report counts as
	// fresh for that agent cadence plus Timeout — never for the full interval.
	// Headroom, kubelet, and runtime liveness change on the order of minutes,
	// so a long interval must not accept a measurement that old: a 24h
	// interval still detects a disk filling up within minutes, and simply
	// refreshes the HealthReport on its own cadence.
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`

	// Timeout bounds a single node-agent evaluation pass, including the kubelet
	// and runtime-socket probes. Defaults to 30s when unset. Must be at least 1s
	// (MinCheckTimeout) and must not exceed Interval.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// HistoryLimit caps the number of HealthReports retained for this check.
	// The minimum of 1 keeps Status.LastReportName valid.
	// +optional
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	HistoryLimit *int32 `json:"historyLimit,omitempty"`
}

// NodeHealthNodeResult is the folded verdict for one node on the most recent
// complete evaluation.
type NodeHealthNodeResult struct {
	// Node is the node the result belongs to.
	// +kubebuilder:validation:MaxLength=253
	Node string `json:"node"`

	// Result is the worst outcome across every check evaluated on this node.
	// +kubebuilder:validation:Enum=Pass;Warn;Fail;Error;Skipped;Unknown
	Result string `json:"result"`

	// Message summarises the worst check on the node, so a failing node is
	// triaged without opening the HealthReport.
	// +optional
	// +kubebuilder:validation:MaxLength=512
	Message string `json:"message,omitempty"`

	// ObservedAt is when the node's report was produced.
	// +optional
	ObservedAt *metav1.Time `json:"observedAt,omitempty"`
}

// NodeHealthCheckStatus defines the observed state of NodeHealthCheck.
type NodeHealthCheckStatus struct {
	// ObservedGeneration is the most recent metadata.generation reconciled by
	// the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions summarize whether the controller accepted the spec, whether
	// the agent DaemonSet is rolled out and reporting, and whether the current
	// verdict was computed from a complete scan of the fleet.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// LastRunTime records when the operator last evaluated the node-agent
	// results. It is refreshed on the interval cadence even when the aggregate
	// is unchanged, so downstream liveness stays fresh, and never moves
	// backward: an incomplete window freezes it along with the verdict.
	// +optional
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// LastResult is the aggregate result across every node in scope as of the
	// most recent complete evaluation. An incomplete window (a rollout, a node
	// joining, an agent restart) freezes it rather than clearing it; the
	// CoverageComplete condition says which.
	// +kubebuilder:validation:Enum=Pass;Warn;Fail;Error;Skipped;Unknown
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// Summary is a human-readable one-line outcome naming how many nodes
	// passed and, when some did not, the worst of them.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Summary string `json:"summary,omitempty"`

	// LastReportName names the HealthReport capturing the current aggregate
	// result. A new HealthReport is written only when that result transitions,
	// so this name is stable across polls that observe the same result.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	LastReportName string `json:"lastReportName,omitempty"`

	// LastRunTrigger records the fathom.skaphos.io/run-now annotation value
	// most recently consumed. A new value is carried to the node-agents through
	// their DaemonSet template, which restarts them; each agent stamps the value
	// into its report, and the operator records it here only once every node in
	// scope has a fresh report carrying it. Until then the previous verdict is
	// kept. A given on-demand trigger therefore completes exactly once.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	LastRunTrigger string `json:"lastRunTrigger,omitempty"`

	// DesiredNodes is the number of nodes the agent DaemonSet targets
	// (DaemonSet status DesiredNumberScheduled).
	// +optional
	DesiredNodes int32 `json:"desiredNodes,omitempty"`

	// ReportingNodes is the number of nodes that have published a fresh report
	// the operator consumed in the most recent roll-up.
	// +optional
	ReportingNodes int32 `json:"reportingNodes,omitempty"`

	// NodeResults holds one entry per node in the most recent complete
	// evaluation, sorted by node name. It is the explicit coverage signal: a
	// node in scope that is absent here has not reported, and the
	// CoverageComplete condition names it. Like LastResult it is frozen, not
	// cleared, across an incomplete window. Capped at 100 entries; the verdict
	// and coverage are computed across every node before truncation.
	// +optional
	// +listType=map
	// +listMapKey=node
	// +kubebuilder:validation:MaxItems=100
	NodeResults []NodeHealthNodeResult `json:"nodeResults,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=fathom
// +kubebuilder:printcolumn:name="Result",type=string,JSONPath=`.status.lastResult`
// +kubebuilder:printcolumn:name="Reporting",type=integer,JSONPath=`.status.reportingNodes`
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.status.desiredNodes`
// +kubebuilder:printcolumn:name="Last Run",type=date,JSONPath=`.status.lastRunTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NodeHealthCheck is the Schema for the nodehealthchecks API.
type NodeHealthCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeHealthCheckSpec   `json:"spec,omitempty"`
	Status NodeHealthCheckStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeHealthCheckList contains a list of NodeHealthCheck.
type NodeHealthCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeHealthCheck `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeHealthCheck{}, &NodeHealthCheckList{})
}
