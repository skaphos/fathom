/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package testutil builds hostile input without relying on a Kubernetes cluster.
// Tests choose a boundary and feed the resulting value to the actual enforcer;
// these builders do not decide whether an input is permitted.
package testutil

import (
	"fmt"
	"strings"
	"time"

	limits "github.com/skaphos/fathom/pkg/addondefinition"
)

// Boundary pairs a configured threshold with its next higher value. Maximum
// bounds reject Over; minimum grace/poll intervals accept it. Enforcement tests
// must apply the contract direction rather than assuming every row is a maximum.
type Boundary struct {
	Name     string
	At, Over int64
	Task     string
}

// Boundaries enumerates each independently enforced numeric dimension. Time
// values are nanoseconds; rates/counts are integers, and byte caps are bytes.
func Boundaries() []Boundary {
	return []Boundary{
		{"MaxSpecBytes", int64(limits.MaxSpecBytes), int64(limits.MaxSpecBytes) + 1, "T017–T018"},
		{"MaxFamilies", int64(limits.MaxFamilies), int64(limits.MaxFamilies) + 1, "T017–T018"},
		{"MaxChecksPerFamily", int64(limits.MaxChecksPerFamily), int64(limits.MaxChecksPerFamily) + 1, "T017–T018"},
		{"MaxChecks", int64(limits.MaxChecks), int64(limits.MaxChecks) + 1, "T017–T018"},
		{"MaxStringBytes", int64(limits.MaxStringBytes), int64(limits.MaxStringBytes) + 1, "T017–T018"},
		{"MaxStringRunes", int64(limits.MaxStringRunes), int64(limits.MaxStringRunes) + 1, "T017–T018"},
		{"MaxMapEntries", int64(limits.MaxMapEntries), int64(limits.MaxMapEntries) + 1, "T017–T018"},
		{"MaxListEntries", int64(limits.MaxListEntries), int64(limits.MaxListEntries) + 1, "T017–T018"},
		{"MaxSpecDepth", int64(limits.MaxSpecDepth), int64(limits.MaxSpecDepth) + 1, "T017–T018"},
		{"MaxIdentifierBytes", int64(limits.MaxIdentifierBytes), int64(limits.MaxIdentifierBytes) + 1, "T017–T018"},
		{"MaxResourceSegmentBytes", int64(limits.MaxResourceSegmentBytes), int64(limits.MaxResourceSegmentBytes) + 1, "T017–T018"},
		{"MaxBindingBytes", int64(limits.MaxBindingBytes), int64(limits.MaxBindingBytes) + 1, "T017–T018"},
		{"MaxNamespaces", int64(limits.MaxNamespaces), int64(limits.MaxNamespaces) + 1, "T017–T018"},
		{"MaxUIDBytes", int64(limits.MaxUIDBytes), int64(limits.MaxUIDBytes) + 1, "T017–T018"},
		{"MaxConditions", int64(limits.MaxConditions), int64(limits.MaxConditions) + 1, "T017–T018"},
		{"MaxConditionTypeBytes", int64(limits.MaxConditionTypeBytes), int64(limits.MaxConditionTypeBytes) + 1, "T017–T018"},
		{"MaxConditionReasonBytes", int64(limits.MaxConditionReasonBytes), int64(limits.MaxConditionReasonBytes) + 1, "T017–T018"},
		{"MaxLeaderIdentityBytes", int64(limits.MaxLeaderIdentityBytes), int64(limits.MaxLeaderIdentityBytes) + 1, "T017–T018"},
		{"MaxAdapterVersionBytes", int64(limits.MaxAdapterVersionBytes), int64(limits.MaxAdapterVersionBytes) + 1, "T017–T018"},
		{"MaxDurationBytes", int64(limits.MaxDurationBytes), int64(limits.MaxDurationBytes) + 1, "T017–T018"},
		{"MaxVersionRangeBytes", int64(limits.MaxVersionRangeBytes), int64(limits.MaxVersionRangeBytes) + 1, "T017–T018"},
		{"MaxVersionComparators", int64(limits.MaxVersionComparators), int64(limits.MaxVersionComparators) + 1, "T017–T018"},
		{"MaxVersionAlternatives", int64(limits.MaxVersionAlternatives), int64(limits.MaxVersionAlternatives) + 1, "T017–T018"},
		{"MaxAPIVersions", int64(limits.MaxAPIVersions), int64(limits.MaxAPIVersions) + 1, "T017–T018"},
		{"MaxSelectorTerms", int64(limits.MaxSelectorTerms), int64(limits.MaxSelectorTerms) + 1, "T017–T018"},
		{"MaxSelectorValues", int64(limits.MaxSelectorValues), int64(limits.MaxSelectorValues) + 1, "T017–T018"},
		{"MaxSelectorValueBytes", int64(limits.MaxSelectorValueBytes), int64(limits.MaxSelectorValueBytes) + 1, "T017–T018"},
		{"MaxFieldSegments", int64(limits.MaxFieldSegments), int64(limits.MaxFieldSegments) + 1, "T017–T018"},
		{"MaxFieldSegmentBytes", int64(limits.MaxFieldSegmentBytes), int64(limits.MaxFieldSegmentBytes) + 1, "T017–T018"},
		{"MaxYAMLBytes", int64(limits.MaxYAMLBytes), int64(limits.MaxYAMLBytes) + 1, "T028"},
		{"MaxYAMLNodes", int64(limits.MaxYAMLNodes), int64(limits.MaxYAMLNodes) + 1, "T028"},
		{"MaxYAMLDepth", int64(limits.MaxYAMLDepth), int64(limits.MaxYAMLDepth) + 1, "T028"},
		{"MaxAnnotationBytes", int64(limits.MaxAnnotationBytes), int64(limits.MaxAnnotationBytes) + 1, "T028"},
		{"MaxResponseBytes", int64(limits.MaxResponseBytes), int64(limits.MaxResponseBytes) + 1, "T026–T027"},
		{"MaxRunResponseBytes", int64(limits.MaxRunResponseBytes), int64(limits.MaxRunResponseBytes) + 1, "T026–T027"},
		{"MaxPageObjects", int64(limits.MaxPageObjects), int64(limits.MaxPageObjects) + 1, "T026–T027"},
		{"MaxRunObjects", int64(limits.MaxRunObjects), int64(limits.MaxRunObjects) + 1, "T026–T027"},
		{"MaxRunRequests", int64(limits.MaxRunRequests), int64(limits.MaxRunRequests) + 1, "T026–T027"},
		{"MaxObjectVisits", int64(limits.MaxObjectVisits), int64(limits.MaxObjectVisits) + 1, "T026–T027"},
		{"MaxObjectNodes", int64(limits.MaxObjectNodes), int64(limits.MaxObjectNodes) + 1, "T026–T027"},
		{"MaxObjectDepth", int64(limits.MaxObjectDepth), int64(limits.MaxObjectDepth) + 1, "T026–T027"},
		{"MaxResults", int64(limits.MaxResults), int64(limits.MaxResults) + 1, "T030"},
		{"MaxEvidenceBytes", int64(limits.MaxEvidenceBytes), int64(limits.MaxEvidenceBytes) + 1, "T030"},
		{"MaxMessageBytes", int64(limits.MaxMessageBytes), int64(limits.MaxMessageBytes) + 1, "T030"},
		{"MaxDetails", int64(limits.MaxDetails), int64(limits.MaxDetails) + 1, "T030"},
		{"MaxCachedRevisions", int64(limits.MaxCachedRevisions), int64(limits.MaxCachedRevisions) + 1, "T032–T033"},
		{"MaxConcurrentRuns", int64(limits.MaxConcurrentRuns), int64(limits.MaxConcurrentRuns) + 1, "T032–T033"},
		{"MaxRunsPerDefinition", int64(limits.MaxRunsPerDefinition), int64(limits.MaxRunsPerDefinition) + 1, "T032–T033"},
		{"MaxRunsPerCheck", int64(limits.MaxRunsPerCheck), int64(limits.MaxRunsPerCheck) + 1, "T032–T033"},
		{"RequestsPerSecond", int64(limits.RequestsPerSecond), int64(limits.RequestsPerSecond) + 1, "T032–T033"},
		{"RequestBurst", int64(limits.RequestBurst), int64(limits.RequestBurst) + 1, "T032–T033"},
		{"MaxRequestRetries", int64(limits.MaxRequestRetries), int64(limits.MaxRequestRetries) + 1, "T029"},
		{"MaxPaginationRestarts", int64(limits.MaxPaginationRestarts), int64(limits.MaxPaginationRestarts) + 1, "T029"},
		{"MaxQueuedWakesPerCheck", int64(limits.MaxQueuedWakesPerCheck), int64(limits.MaxQueuedWakesPerCheck) + 1, "T032–T033"},
		{"MaxRetryJitterPercent", int64(limits.MaxRetryJitterPercent), int64(limits.MaxRetryJitterPercent) + 1, "T032–T033"},
		{"MaxRunDuration", int64(limits.MaxRunDuration), int64(limits.MaxRunDuration) + 1, "T029"},
		{"MaxRequestDuration", int64(limits.MaxRequestDuration), int64(limits.MaxRequestDuration) + 1, "T029"},
		{"MaxCompileDuration", int64(limits.MaxCompileDuration), int64(limits.MaxCompileDuration) + 1, "T029"},
		{"MissingInputPoll", int64(limits.MissingInputPoll), int64(limits.MissingInputPoll) + 1, "T032–T033"},
		{"MaxRetryBackoff", int64(limits.MaxRetryBackoff), int64(limits.MaxRetryBackoff) + 1, "T032–T033"},
		{"InitialRetryBackoff", int64(limits.InitialRetryBackoff), int64(limits.InitialRetryBackoff) + 1, "T032–T033"},
		{"MinTakeoverGrace", int64(limits.MinTakeoverGrace), int64(limits.MinTakeoverGrace) + 1, "T041"},
		{"MaxDrainDuration", int64(limits.MaxDrainDuration), int64(limits.MaxDrainDuration) + 1, "T043"},
		{"MaxDrainLeaseReads", int64(limits.MaxDrainLeaseReads), int64(limits.MaxDrainLeaseReads) + 1, "T043"},
		{"DrainPollInterval", int64(limits.DrainPollInterval), int64(limits.DrainPollInterval) + 1, "T043"},
	}
}

// Bytes builds an exact byte-count ASCII string.
func Bytes(n int) string { return strings.Repeat("x", n) }

// Strings builds n distinct short values for uniqueness-sensitive list tests.
func Strings(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("item-%d", i)
	}
	return out
}

// Map builds n unique entries without duplicate-key collapse during decoding.
func Map(n int) map[string]string {
	out := make(map[string]string, n)
	for _, key := range Strings(n) {
		out[key] = "value"
	}
	return out
}

// JSONDepth constructs a scalar at depth n, counting the root as zero.
func JSONDepth(n int) string { return strings.Repeat("[", n) + "0" + strings.Repeat("]", n) }

// JSONNodes constructs an array with exactly n nodes, including its root.
func JSONNodes(n int) string {
	if n < 1 {
		panic("node count must be positive")
	}
	if n == 1 {
		return "[]"
	}
	return "[" + strings.TrimSuffix(strings.Repeat("0,", n-1), ",") + "]"
}

// YAMLNodes includes both document and sequence roots in its node count.
func YAMLNodes(n int) string {
	if n < 2 {
		panic("YAML node count must include document and sequence")
	}
	if n == 2 {
		return "[]\n"
	}
	return strings.Repeat("- x\n", n-2)
}

// YAMLDepth is a nested flow sequence, with the value at depth n.
func YAMLDepth(n int) string { return strings.Repeat("[", n) + "x" + strings.Repeat("]", n) }

// AtAndOverDuration avoids wall-clock sleeps in deadline boundary tests.
func AtAndOverDuration(cap time.Duration) (time.Duration, time.Duration) {
	return cap, cap + time.Nanosecond
}
