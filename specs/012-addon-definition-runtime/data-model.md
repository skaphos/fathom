<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Data model

Normative source: [RFC sections 1–6](../../docs/rfc/0001-addondefinition-crd.md).
Names below define planned interfaces; they are not shipped API claims.

## AddonDefinition

Cluster-scoped `fathom.skaphos.io/v1alpha1`. metadata.name equals immutable
spec.addonType: exact lowercase DNS label, 1–63 ASCII characters, no normalization.
Required adapterVersion is SemVer; semanticsVersion initially 1. Optional defaults
false. supportedVersions requires an unambiguous family/workload/container
versionSource. Families have unique names, defaultEnabled=false, and ordered
uniquely named checks. Check kind selects exactly one of Workload, CRD, Condition,
Field, Webhook, CronJob, ConfigMap, AnnotationStaleness or PodProjection payloads.
APIService uses Condition. Payloads declare target scope; no executable expressions.
requestedReads is optional documentation/rendering input restricted to get/list,
exact resources and discovery URLs; it is neither a grant nor an authority ceiling.
Status exposes acceptance, revision and request-based permission diagnostics.
Exact definition status field layout is an implementation detail, bounded by the RFC.

## AddonDefinitionBinding

Namespaced alpha kind, restricted at activation to the configured operator namespace.
Name equals definitionRef.name. Required definitionRef and serviceAccountRef each
have name and UID; both entire reference objects are immutable. UIDs are nonempty
and at most 128 bytes; SA names at most 253 characters. enabled=false by default.
targetScope.namespaces is a unique set of at most 32 exact namespace names of at
most 63 characters; allowClusterScoped=false. At least one scope is required.
Enabled and scope edits change generation and invalidate captured authority.
Binding spec is at most 16 KiB. Recreated definition/SA requires binding recreation.

Status also carries leaderEpoch as specified in [leadership contract](contracts/leadership.md).
Status subresource: observedGeneration nonnegative int64; activeRuns 0–4;
leaderIdentity at most 253 bytes; conditions map keyed by type, at most 8.
Conditions require type (at most 64 bytes), True/False/Unknown status, reason
(at most 128 bytes), message (at most 1,024 bytes), transition time and generation.
Accepted, Ready and Drained are required condition types. Only the active leader
may acknowledge disabled, matching-generation, zero-active-runs drain. Missing or
old leader/generation status cannot authorize execution or acknowledge drain.

## Snapshot and publication context

Build and validate off-lock, then atomically swap. A runtime revision is exactly
(definition UID, generation, schema, semantics); publication additionally records
operator build and adapterVersion. Capture that revision, binding UID/spec generation,
SA UID and AddonCheck UID/generation/policy context. Deep-copy nested slices/maps;
keep declared evaluator order. Registry ownership prevents old deletion events
from removing a replacement. Collision state suppresses both dispatch candidates.
No runtime dispatch before informer synchronization and direct validation.

## Evidence and attempts

Current status stores lastSuccessfulEvaluation (completed Pass/Warn/Fail/Skipped evidence,
original observedAt, revision and authority), separately from latestAttemptAt,
attempt outcome and readiness. Unknown applies when no evidence exists. Freshness
is Current only for eligible input aged at most two effective intervals plus
timeout; otherwise Stale, Superseded or Unavailable with reason. Failed attempts
cannot renew the observation timestamp. HealthReport creation remains verdict-
transition-only; report context is immutable. HealthCheck mirrors current
readiness/freshness; ClusterHealth continues reading only HealthCheck.status.

All-Skipped is completed evidence, not a failed attempt: update observedAt and
revision/context, set verdict Skipped and coverage NoChecksEvaluated with message
“no checks evaluated”. Current freshness means recency, not Pass. Ready=True means
execution completed with eligible inputs, not a healthy verdict. Mixed results
retain the existing aggregate order; Skipped never turns a Warn/Fail into Pass.
A Pass→Skipped transition creates a report; Skipped→Skipped only updates status.
Zero enabled checks follows the engine's explicit Skipped sentinel. Historical
reports are retained under existing retention policy, never rewritten by this run.

See [payload contract](contracts/payloads.md) for all wire fields/defaults/mappings
and [leadership contract](contracts/leadership.md) for independently verified drain.

## Transitions and bounds

Every row of RFC section 5 is required, not just happy-path activation. The exact
numeric inventory is in [runtime contract](contracts/runtime.md); admission and
compiler enforce input bounds, and runtime counters cover discovered input and
policy overrides. Default-disabled rollout does not relax any runtime invariant.
