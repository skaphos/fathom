/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package addondefinition contains the bounded, Kubernetes API based authoring
// contract shared by runtime compilation and the read-only CLI.
package addondefinition

import "time"

// Input bounds apply before parsing or compiling an untrusted definition.
const (
	MaxSpecBytes            = 256 << 10
	MaxFamilies             = 16
	MaxChecksPerFamily      = 32
	MaxChecks               = 512
	MaxStringBytes          = 1024
	MaxStringRunes          = 1024
	MaxMapEntries           = 32
	MaxListEntries          = 32
	MaxSpecDepth            = 8
	MaxIdentifierBytes      = 63
	MaxResourceSegmentBytes = 253
	MaxBindingBytes         = 16 << 10
	MaxNamespaces           = 32
	MaxUIDBytes             = 128
	MaxConditions           = 8
	MaxConditionTypeBytes   = 64
	MaxConditionReasonBytes = 128
	MaxLeaderIdentityBytes  = 253
	MaxAdapterVersionBytes  = 256
	MaxDurationBytes        = 256
	MaxVersionRangeBytes    = 256
	MaxVersionComparators   = 16
	MaxVersionAlternatives  = 8
	MaxAPIVersions          = 8
	MaxSelectorTerms        = 32
	MaxSelectorValues       = 32
	MaxSelectorValueBytes   = 256
	MaxFieldSegments        = 16
	MaxFieldSegmentBytes    = 128
	MaxYAMLBytes            = 64 << 10
	MaxYAMLNodes            = 4096
	MaxYAMLDepth            = 16
	MaxAnnotationBytes      = 1024
)

// Evaluation bounds are shared across all requests, helpers and retries in a run.
const (
	MaxResponseBytes       = 2 << 20
	MaxRunResponseBytes    = 16 << 20
	MaxPageObjects         = 100
	MaxRunObjects          = 1000
	MaxRunRequests         = 100
	MaxObjectVisits        = 100000
	MaxObjectNodes         = 32768
	MaxObjectDepth         = 64
	MaxResults             = 1000
	MaxEvidenceBytes       = 256 << 10
	MaxMessageBytes        = 1024
	MaxDetails             = 32
	MaxCachedRevisions     = 128
	MaxConcurrentRuns      = 4
	MaxRunsPerDefinition   = 1
	MaxRunsPerCheck        = 1
	RequestsPerSecond      = 10
	RequestBurst           = 20
	MaxRequestRetries      = 2
	MaxPaginationRestarts  = 1
	MaxQueuedWakesPerCheck = 1
	MaxRetryJitterPercent  = 20
	MaxRunDuration         = 30 * time.Second
	MaxRequestDuration     = 5 * time.Second
	MaxCompileDuration     = time.Second
	MissingInputPoll       = time.Minute
	MaxRetryBackoff        = time.Minute
	InitialRetryBackoff    = 5 * time.Second
	MinTakeoverGrace       = 30 * time.Second
)

// Drain verification uses an independent CLI budget, never evaluator credentials.
const (
	MaxDrainDuration   = 15 * time.Second
	MaxDrainLeaseReads = 16
	DrainPollInterval  = time.Second
)
