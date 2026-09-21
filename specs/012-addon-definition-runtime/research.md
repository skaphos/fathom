<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Research and decisions

Source baseline: main `3e7eafd`, 2026-09-20. Design authority is accepted RFC 0001
and ADR 0007. Read-only planning research checked implementation seams directly;
no new external technology choice or runtime behavior is asserted.

## Typed compilation and deterministic order

Decision: retain the existing typed engine and add a runtime ordered sequence.
Rationale: `internal/adapter/declarative/definition.go` groups checks into buckets
and shares nested data; runtime definitions need deep copies and declared ordering.
`engine.go` provides semantic construction validation to extend.
Alternatives: bucket round-trip silently reorders mixed checks; another evaluator
framework or arbitrary expressions contradict accepted scope.

## Separate runtime authority path

Decision: dedicated identity-scoped discovery and bounded read transport, plus
uncached control-plane validation. Reuse evaluation logic, not unsafe client fallbacks.
Rationale: `internal/adapter/impersonation/factory.go` uses manager REST mapping and
username-only caching; `internal/controller/addoncheck_controller.go` permits local
or missing-factory manager fallback. `internal/app/run.go` label-filters some caches.
Alternatives: manager discovery/reads violate the new authority boundary; effective
RBAC union inspection adds privilege and cannot prove future authorization. Actual
request diagnostics are the accepted alternative and work with metrics off.

## Immutable ownership and publication

Decision: extend `internal/adapter/registry/registry.go` with separate runtime
snapshot ownership, removal and collision state, preserving built-in registration.
Rationale: current same-name Register is an idempotent no-op and has no replacement.
Use UID/generation/context fencing and per-check serialization/CAS at publication.
Alternatives: mutable instances mix revisions; last-writer-wins cannot reconstruct
collision decisions. Kubernetes cannot provide atomic revocation across all reads
and status writes; preserve the RFC's explicitly limited observation guarantee.

## Evidence and history

Decision: separate completed evidence from attempts in AddonCheck status and mirror
freshness through HealthCheck. Keep HealthReport transition-only.
Rationale: current runAddonCheck updates LastRunTime even on failure; runtime input
loss must not renew old evidence. ClusterHealth remains HealthCheck-only.
Alternative: creating history for every revision changes accepted history semantics.

## CLI layering

Decision: introduce `pkg/addondefinition` for API-dependent pure rendering and
inventory data, with a pinned generator for compiled inventory; runtime compiler
stays internal. `internal/cli` uses this package and injectable Kubernetes reads.
Rationale: repository rules prohibit CLI imports of internal/app, internal/adapter
and internal/controller. Existing BuiltInAdapters/rbacgen cannot be imported there.
Alternatives: duplicate built-in inventory drifts; relaxing CLI imports contradicts
repository boundaries. This package is proposed implementation organization,
not a new evaluator interface or new architectural decision.

## Existing verification and generators

Extend registry concurrency tests, impersonation factory tests, declarative engine
and version suites, controller impersonation/envtest tests, CLI fake-client tests,
and `test/e2e/impersonation_test.go` patterns. Use Taskfile wrappers for generate,
manifests, gen:addon-rbac, helm:sync, docs:api-ref, verify-generated, crd-compat and
full test-e2e. New task wrappers must pin any new generator.

## Compatibility and remaining measurements

#256 remains Shawn Stratton's separately coordinated release dependency: decision,
older adapter ratio-key collision test and migration/rejection behavior are required.
No settled design questions remain. CEL worst-size admission cost, compiler deadline
cooperation, bounded decoding and fair scheduling are implementation measurements,
not unresolved requirements; failed evidence blocks release or requires a design
revision. No arbitrary threshold increase is authorized by this plan.

## Clarification research — 2026-09-20

Decision: require election and verify drain against the configured manager Lease,
not binding status alone. run.go currently supplies LeaderElection/ID; options.go
already defaults election on. leader_election_role.yaml grants Lease access and
internal/cli/client.go provides uncached reads. The pinned client-go leader-election
source explicitly disclaims fencing and uses observed progression rather than
trusting remote clock accuracy. Same-holder updates preserve acquisition metadata:
therefore process-unique holders, no reacquisition and explicit epoch comparison
are necessary. See contracts/leadership.md for the bounded renewal observation,
takeover grace, status schema and conservative failure behavior.

Alternatives rejected: status-only validation trusts stale leader claims; comparing
Lease resourceVersion rejects harmless renewals; absolute remote timestamps rely
on clock synchronization; election-free runtime contradicts the user's answer.
The exact controller-runtime manager integration API must be checked against the
pinned v0.25.1 source during implementation (research found only v0.25.0 locally).
That is a compatibility verification task, not permission to weaken the contract.

Decision: completed Skipped is attributable current evidence with NoChecksEvaluated
coverage. Ready means execution readiness/completion and Current means recency;
neither converts Skipped to Pass. Mixed outcomes retain existing aggregate semantics.
The explicit user clarification extends RFC section 5's Pass/Warn/Fail enumeration;
contracts/decision-supplement.md records that refinement, leaving the ADR immutable.

Decision: target-version CLI owns collision inventory; do not implement inventory
file import. Emit binary version/build and fail visibly when unavailable. This
avoids a second artifact trust/versioning interface, at the cost of downloading
the target CLI before upgrade.

Decision: contracts/payloads.md enumerates all nine wire payloads, required fields,
defaults, scope and policy override behavior against definition.go/engine.go. Explicit
namespace lists replace all-namespace runtime reads; the declared binding remains
the authority ceiling. Built-in evaluator defaults/order remain unchanged. No
external evaluator framework or new expression runtime is introduced.

## Pinned lifecycle verification — implementation T006

Verified the downloaded controller-runtime **v0.25.1** source, not the earlier
v0.25.0 research copy. `pkg/leaderelection/leader_election.go` constructs a
hostname plus random UUID identity. `manager.Options.LeaderElectionResourceLockInterface`
allows a supplied Lease lock with an identity known to runtime session code;
when supplied, construct the lock with the explicit operator namespace and the
configured election ID because the manager's corresponding options are ignored.
`pkg/manager/internal.go:initLeaderElector` starts election runnables after
acquisition, reports “leader election lost” to the manager error channel on loss,
and sets the shutdown grace to zero. The manager does not reacquire the Lease
in the same Start invocation. `NeedLeaderElection` routes runtime runnables to
that group. Its runnable context is the manager internal context, so a runtime
session must separately enforce its admission cancellation and direct Lease
checks; it must not assume that context is the election callback context.

The client-go version is **v0.37.0** (go.mod). Epoch checks still require direct
Lease observation, process-unique holder identity, termination on loss, and the
30-second monotonic takeover grace. These supported seams do not provide fencing
against suspended processes. T041 remains responsible for integration tests.
