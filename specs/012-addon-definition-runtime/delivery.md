<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Delivery preparation

Runtime release remains blocked by the separate #256 compatibility decision and
the remaining PR/release gates. The isolated combined full suite passed. No pull request was opened or
published by this qualification run. The descriptions below are review sections
for the one feature PR and retain existing checkpoint commits as landmarks.

## Proposed PR shape

Publish one feature PR, organized into four review sections matching the four
milestones below. Existing checkpoint commits (`9ea1104`, `b4646f9` and
`7c19435`) remain useful review landmarks, but they are not separate PR heads
requiring independent CI. The #256 compatibility work is a separate prerequisite
PR, [draft PR #351](https://github.com/skaphos/fathom/pull/351), with the chosen
ContractVersion 1.1 decision validated separately. It is not merged; integration
verification and T054 remain open.

| Review section | Scope | Evidence |
| --- | --- | --- |
| 1. Schema and authoring | Typed resources, validation, rendering and read-only CLI | Component/API tests and generated checks |
| 2. Authority and bounded execution | Delegated identity, limits, panic isolation and scheduling | Deterministic component tests and race checks |
| 3. Lifecycle and evidence | Wiring, publication fences, history, election and drain | Manager/envtest, lifecycle tests and live scenarios |
| 4. Qualification and delivery | Default-off packaging, hostile-input isolation, rollback and operations | 105/105 full Kind suite, CI, race, security and operations records |

The feature PR description should link the exact validation records, DCO identity,
the separate #256 prerequisite, and the release gate status.

### Milestone 1 description

Add typed AddonDefinition and UID-bound AddonDefinitionBinding APIs plus an
offline renderer and read-only bind, collision and drain CLI commands. The
renderer emits only grants it can determine safely and flags custom-resource
grants requiring review. This checkpoint does not activate runtime execution.

### Milestone 2 description

Run runtime evaluators through dedicated ServiceAccount impersonation with exact
scope checks, bounded requests and parsing, supervised failures, fair scheduling
and immutable revision caching. Missing authority fails closed; manager
credentials never substitute for the delegated reader. This checkpoint is an
execution harness, with real-cluster acceptance dependent on later integration.

### Milestone 3 description

Integrate definition and binding reconciliation, leadership and drain, final
publication fences, completed evidence and transition-only history behind a
default-off loader. Preserve original evidence across failed attempts and mirror
readiness and freshness through HealthCheck without changing ClusterHealth's
status-only source contract.

### Milestone 4 description

Expose the opt-in loader in Helm and document reviewed UID installation, exact
grants, drain, rollback and required backups. Add real Docker/kind coverage for
delegation, identity changes, hostile input, held API reads, history and
operations, while component tests prove exact limits and injected panics.

Documented integration fixes include ServiceAccount and peer-binding changes now trigger
readiness reconciliation; runtime reports use the exact published evidence rather
than a potentially stale cached object, and unchanged-verdict retries reuse a
report after a report-pointer write conflict. Startup now checks provisional
definition UID and generation claims before controller startup, so stored
collisions fail closed; definition recreation then recovers through the normal
identity and generation path. The host-mode manager now requires and uses the
explicit operator namespace for leader election. These fixes remove startup and
recovery races and make behavior predictable for operators. The approved test
split keeps exact boundaries, injected panics and impossible collisions in
deterministic component tests, while real permissions, execution, lifecycle,
drain, rollback and hostile-input isolation run in Docker/Kind. Runtime loading
remains default-off.

## Validation and release dependencies

Use [execution.md](execution.md) for exact commands and outcomes,
[qualification.md](qualification.md) for the requirement-to-test map, and
[operations-qualification.md](operations-qualification.md) for the bounded
v0.5.1 downgrade/restore/re-enable and cross-version collision evidence. Keep
failed and superseded trials distinguishable from the final suite. The latest
completed historical trial had 104 passes and one failure; the isolated final
run passed all 105 specs. The failed run remains historical evidence.

[#256](https://github.com/skaphos/fathom/issues/256) remains a separate release
decision; do not implement or close it through these PRs. An exact Dockerfile
v0.5.1 downgrade confirmed that the older binary strips the new AddonCheck status
fields. A guarded restore under the current default-off binary preserved original
evidence and unchanged reports; fresh same-verdict re-enable, cross-version
stored-collision handling and the T063 host-mode fix also passed in bounded
trials. Full status exports and the tested restoration procedure are recorded in
the operations qualification record. The 105/105 full Kind suite, CI, race
checks, security review and bounded operations evidence support the feature PR;
no released chart upgrade is claimed, and #256 remains a separate release gate.

T059 is draft PR preparation only: no PR was published and no working-branch commit
was made by this qualification run. Local author/committer configuration was
verified as Shawn Stratton `shawn@skaphos.io`, matching the existing feature
commits. Any future commit must retain the mandatory Signed-off-by trailer;
cryptographic signing is preferred.
