/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DNSRecordType is the kind of DNS record a target expects.
//
// Host is the default and is satisfied by an address of either family. A and
// AAAA narrow to a single family and apply only when named explicitly — a
// default of A would silently mean "IPv4 only" and fail an AAAA-only name for
// a reason its author would not expect.
// +kubebuilder:validation:Enum=Host;A;AAAA;CNAME;SRV;PTR
type DNSRecordType string

const (
	DNSRecordHost  DNSRecordType = "Host"
	DNSRecordA     DNSRecordType = "A"
	DNSRecordAAAA  DNSRecordType = "AAAA"
	DNSRecordCNAME DNSRecordType = "CNAME"
	DNSRecordSRV   DNSRecordType = "SRV"
	DNSRecordPTR   DNSRecordType = "PTR"
)

// DNSResolverSource is where a vantage point resolves from.
// +kubebuilder:validation:Enum=Cluster;Node;Explicit
type DNSResolverSource string

const (
	DNSResolverCluster  DNSResolverSource = "Cluster"
	DNSResolverNode     DNSResolverSource = "Node"
	DNSResolverExplicit DNSResolverSource = "Explicit"
)

// DefaultDNSResolverName is the reserved name of the implicit vantage point a
// DNSCheck resolves from when it declares none. Reserving it means a per-target
// result or a metric series labelled "cluster" always denotes the same thing,
// whether or not the check spelled its vantage points out.
const DefaultDNSResolverName = "cluster"

// DNSTarget is one subject plus the expectation attached to it.
// +kubebuilder:validation:XValidation:rule="!self.absent || !has(self.expectedAnswers) || size(self.expectedAnswers) == 0",message="a target asserting absent must not declare expectedAnswers"
// The colon clause closes a gap the pattern alone leaves: the pattern must
// admit IPv6 literals so a PTR subject can be one, which would otherwise let a
// colon-bearing non-address such as "abc:def" through as an A target.
// +kubebuilder:validation:XValidation:rule="self.recordType == 'PTR' ? isIP(self.name) : (!isIP(self.name) && !self.name.contains(':'))",message="a PTR target's name must be an IP address, and every other record type's name must be a DNS name"
type DNSTarget struct {
	// Name is the subject to look up: a DNS name for forward record types, or
	// an IP address when RecordType is PTR. A trailing dot is accepted, and
	// SRV subjects may carry the conventional _service._proto labels.
	//
	// A target checked against an Explicit vantage point must be fully
	// qualified. Such a pod runs with no cluster search domains, so a short
	// name that resolves fine through cluster DNS will not resolve there — a
	// failure that reads like an outage but is a missing suffix.
	// The pattern admits DNS names (optionally fully qualified, with the
	// underscore labels SRV needs) and IPv6 literals, because a PTR subject is
	// an address. It is deliberately coarse: the XValidation rules on this type
	// decide which of the two forms is legal for the declared record type.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^(_?[a-zA-Z0-9]([-a-zA-Z0-9]*[a-zA-Z0-9])?(\._?[a-zA-Z0-9]([-a-zA-Z0-9]*[a-zA-Z0-9])?)*\.?|[0-9a-fA-F:]+)$`
	Name string `json:"name"`

	// RecordType is the kind of record expected. Defaults to Host, an address
	// lookup satisfied by either address family.
	// +optional
	// +kubebuilder:default=Host
	RecordType DNSRecordType `json:"recordType,omitempty"`

	// ExpectedAnswers are answers the lookup must return. Matching is
	// containment, not equality: extra answers never fail the check, because
	// multi-address and round-robin records legitimately return supersets.
	// When empty, any non-empty answer satisfies the target.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:items:MaxLength=253
	ExpectedAnswers []string `json:"expectedAnswers,omitempty"`

	// Absent inverts the assertion: the target passes when the name does NOT
	// resolve. Use it to confirm a decommissioned name is really gone.
	//
	// A resolver that cannot be reached never satisfies this — that is a
	// network fault, not evidence a name was retired, and it is reported as an
	// error rather than a pass.
	// +optional
	// +kubebuilder:default=false
	Absent bool `json:"absent,omitempty"`

	// Resolver names the vantage point this target is checked from, either an
	// entry in spec.resolvers or the reserved name "cluster".
	//
	// When empty the target is checked from EVERY declared vantage point, not
	// just the default one. Declaring three vantage points therefore triples
	// the cost of every target that does not name one: three probe runs, three
	// per-target results, and three sets of metric series.
	// +optional
	// +kubebuilder:validation:MaxLength=63
	Resolver string `json:"resolver,omitempty"`
}

// DNSResolver is a declared vantage point resolution is performed from.
// +kubebuilder:validation:XValidation:rule="(self.from == 'Explicit') == has(self.address)",message="address is required when from is Explicit, and must be omitted otherwise"
// +kubebuilder:validation:XValidation:rule="self.name != 'cluster'",message="the resolver name \"cluster\" is reserved for the implicit vantage point"
type DNSResolver struct {
	// Name identifies this vantage point so targets can select it and results
	// can report it. The name "cluster" is reserved.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// From is where this vantage point resolves: the cluster's own DNS
	// service, the node's resolver, or an explicitly addressed upstream.
	// +optional
	// +kubebuilder:default=Cluster
	From DNSResolverSource `json:"from,omitempty"`

	// Address is the upstream resolver, as an IP with an optional port. It is
	// required when From is Explicit and rejected otherwise.
	//
	// A hostname is not accepted: a resolver that must itself be resolved to be
	// reached cannot answer the question the check is asking.
	// The rule validates the host and the port separately. Checking only that
	// the value "looks addressy" would admit "10.0.0.10:abc" and
	// "[not-an-ip]:53", pushing a misconfiguration to runtime where it surfaces
	// as an unreachable resolver rather than as the typo it is.
	// +optional
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:rule="isIP(self) || (self.startsWith('[') ? (self.indexOf(']:') > 1 && isIP(self.substring(1, self.indexOf(']:'))) && self.substring(self.indexOf(']:') + 2).matches('^[0-9]{1,5}$') && int(self.substring(self.indexOf(']:') + 2)) >= 1 && int(self.substring(self.indexOf(']:') + 2)) <= 65535) : (self.lastIndexOf(':') > 0 && isIP(self.substring(0, self.lastIndexOf(':'))) && self.substring(self.lastIndexOf(':') + 1).matches('^[0-9]{1,5}$') && int(self.substring(self.lastIndexOf(':') + 1)) >= 1 && int(self.substring(self.lastIndexOf(':') + 1)) <= 65535))",message="address must be an IP address with an optional port in 1-65535 (10.0.0.10, 10.0.0.10:53, 2001:db8::1, [2001:db8::1]:53), not a hostname"
	Address string `json:"address,omitempty"`
}

// DNSCheckSpec defines the desired state of DNSCheck.
//
// A DNSCheck asserts that a set of names resolve — or deliberately do not —
// from one or more vantage points, on a cadence. It reports a verdict per
// (target, vantage point) pair and a single folded verdict for the check.
//
// There is no field to pause a DNSCheck. Stopping a check means deleting it.
// +kubebuilder:validation:XValidation:rule="!has(self.timeout) || duration(self.timeout) >= duration('1s')",message="timeout must be at least 1s"
// +kubebuilder:validation:XValidation:rule="!has(self.interval) || duration(self.interval) >= duration('10s')",message="interval must be at least 10s"
// +kubebuilder:validation:XValidation:rule="!has(self.timeout) || !has(self.interval) || duration(self.timeout) <= duration(self.interval)",message="timeout must not exceed interval"
// +kubebuilder:validation:XValidation:rule="self.targets.all(t, !has(t.resolver) || t.resolver == 'cluster' || (has(self.resolvers) && self.resolvers.exists(r, r.name == t.resolver)))",message="each target's resolver must name a declared resolver or the reserved name \"cluster\""
// Per-target results are keyed by (name, recordType, resolver), so two targets
// identical in all three would collide there. Rejecting the duplicate at write
// time is far kinder than letting the controller fail a status update later
// with an error that says nothing about the specification that caused it.
// +kubebuilder:validation:XValidation:rule="self.targets.all(t, self.targets.filter(o, o.name == t.name && o.recordType == t.recordType && (has(o.resolver) ? (has(t.resolver) && o.resolver == t.resolver) : !has(t.resolver))).size() == 1)",message="targets must be unique by name, recordType, and resolver"
type DNSCheckSpec struct {
	// Targets are the names this check asserts on. At least one is required —
	// a check with no targets would report a vacuous pass.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	Targets []DNSTarget `json:"targets"`

	// Resolvers are the vantage points resolution is performed from. When
	// empty, the check resolves through cluster DNS from an implicit vantage
	// point named "cluster".
	//
	// A target that names no vantage point is checked against every entry
	// here, so the number of evaluations a check performs is
	// len(targets without an override) * max(1, len(resolvers)), bounded at
	// 48. The max(1, …) is not a rounding nicety: an empty list still means one
	// vantage point — the implicit "cluster" one — so a check that declares no
	// resolvers evaluates every target once, not zero times.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=3
	Resolvers []DNSResolver `json:"resolvers,omitempty"`

	// Interval is the cadence at which the check re-runs. Defaults to 1m when
	// unset. Must be at least 10s (MinCheckInterval).
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`

	// Timeout bounds a single evaluation. Defaults to 10s when unset. Must be
	// at least 1s (MinCheckTimeout) and must not exceed Interval.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// HistoryLimit caps the number of HealthReports retained for this check.
	// The minimum of 1 keeps Status.LastReportName valid.
	// +optional
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	HistoryLimit *int32 `json:"historyLimit,omitempty"`
}

// DNSTargetResult is the outcome for one (target, vantage point) pair on the
// most recent evaluation. Identity is the name, record type, and resolver
// together, so the same name checked from two vantage points produces two
// distinct entries rather than colliding.
type DNSTargetResult struct {
	// Name is the subject that was looked up.
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// RecordType is the record kind that was queried, echoed so a result is
	// self-describing without cross-referencing the spec.
	RecordType DNSRecordType `json:"recordType"`

	// Resolver is the vantage point the query was issued from, or "cluster"
	// for the implicit one.
	// +kubebuilder:validation:MaxLength=63
	Resolver string `json:"resolver"`

	// Result is the outcome for this pair.
	// +kubebuilder:validation:Enum=Pass;Warn;Fail;Error;Skipped;Unknown
	Result string `json:"result"`

	// Message says what was asked and what came back.
	// +optional
	// +kubebuilder:validation:MaxLength=512
	Message string `json:"message,omitempty"`

	// Answers are the records returned, retained as the evidence behind the
	// verdict.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:items:MaxLength=253
	Answers []string `json:"answers,omitempty"`

	// LatencyMillis is how long the lookup took. It is recorded as evidence
	// only; slow resolution is not by itself a failure in this API version.
	// +optional
	// +kubebuilder:validation:Minimum=0
	LatencyMillis int64 `json:"latencyMillis,omitempty"`
}

// DNSCheckStatus defines the observed state of DNSCheck.
type DNSCheckStatus struct {
	// ObservedGeneration is the most recent metadata.generation reconciled by
	// the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions summarize whether the controller accepted the spec.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// LastRunTime records when the check was last evaluated.
	// +optional
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// LastResult is the folded verdict across every (target, vantage point)
	// pair from the most recent evaluation — the most severe pair outcome.
	// +kubebuilder:validation:Enum=Pass;Warn;Fail;Error;Skipped;Unknown
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// Summary is a human-readable one-line outcome. When a failure stems from
	// an absent assertion, it says so, so a deliberate "this must not resolve"
	// failure is not triaged as a DNS outage.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Summary string `json:"summary,omitempty"`

	// LastReportName names the HealthReport capturing the current result.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	LastReportName string `json:"lastReportName,omitempty"`

	// LastRunTrigger records the fathom.skaphos.io/run-now annotation value
	// most recently consumed, so a given on-demand trigger fires exactly once.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	LastRunTrigger string `json:"lastRunTrigger,omitempty"`

	// TargetResults holds one entry per (target, vantage point) pair. It is
	// rebuilt from the current spec on every evaluation rather than
	// accumulated, so a pair the spec no longer declares disappears instead of
	// freezing at its last verdict.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=recordType
	// +listMapKey=resolver
	// +kubebuilder:validation:MaxItems=48
	TargetResults []DNSTargetResult `json:"targetResults,omitempty"`

	// ObservedTargets is the number of (target, vantage point) pairs the most
	// recent evaluation covered.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ObservedTargets int32 `json:"observedTargets,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=fathom
// +kubebuilder:printcolumn:name="Result",type=string,JSONPath=`.status.lastResult`
// +kubebuilder:printcolumn:name="Targets",type=integer,JSONPath=`.status.observedTargets`
// +kubebuilder:printcolumn:name="Last Run",type=date,JSONPath=`.status.lastRunTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DNSCheck is the Schema for the dnschecks API.
type DNSCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DNSCheckSpec   `json:"spec,omitempty"`
	Status DNSCheckStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DNSCheckList contains a list of DNSCheck.
type DNSCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSCheck `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DNSCheck{}, &DNSCheckList{})
}
