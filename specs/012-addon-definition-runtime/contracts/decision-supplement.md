<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Accepted implementation clarifications — 2026-09-20

Authority: Shawn Stratton's three explicit option-A answers recorded in
[spec.md](../spec.md#clarifications). This supplement records the narrow follow-up
to RFC 0001; it does not modify immutable ADR 0007 or claim shipped behavior.

1. Runtime loading requires leader election, including one-instance deployments.
   Disabled election refuses runtime activation while preserving built-in behavior.
2. Completed all-Skipped runs are new Skipped evidence, with “no checks evaluated”
   coverage and fresh observation time. They replace prior current Pass; history
   remains transition-only. This extends the RFC section 5 prose enumeration
   of completed Pass/Warn/Fail evidence. Other authorization/evidence rules stand.
3. Collision preflight runs the target release's fathomctl and displays its version.
   Its own bundled compiled inventory is authoritative for that comparison; separate
   inventory file import is excluded.

Implementation planning reconciles the RFC wording by citing this supplement;
accepted ADRs and historical RFC text remain unchanged. Publish this supplement
with the feature planning PR so the clarified contract is durable and reviewable.

### Test contract clarification (2026-09-22)

Q: Must the full Docker/kind suite inject every budget and execution panic?

A: No. Deterministic component tests prove each numeric boundary at and over its
limit, impossible-under-normal-admission collision fixtures, injected recoverable
compile/evaluation panics, slot release and healthy peer progress. Docker/kind
tests prove real admission, permissions, delegated execution, lifecycle, drain,
rollback and hostile-input isolation. This keeps
fault-injection controls out of the shipped operator while preserving both exact
failure evidence and real authority/lifecycle evidence. The RFC numeric and
lifecycle tables remain unchanged.


### Offline renderer clarification — option A

The user selected offline rendering with safely determined grants and explicit
manual-completion diagnostics for unresolved custom-resource grants. No live
discovery, guessed pluralization, or scope broadening is authorized by this
decision. R1 is resolved as a design choice; renderer implementation and tests
remain required before T020 is complete.
