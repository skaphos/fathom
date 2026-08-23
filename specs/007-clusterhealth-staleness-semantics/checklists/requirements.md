# Specification Quality Checklist: Cadence-Aware Staleness Semantics for ClusterHealth

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-08-23
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Validation Notes

**Iteration 1 (2026-08-23)** — two issues found and fixed:

1. *Implementation detail leak*: the first draft framed requirements around the
   three candidate directions named in issue #277. Those are mechanisms, not
   outcomes. Rewritten as outcome requirements so planning selects the mechanism.
2. *Unverifiable success criterion*: an early SC referenced a specific status
   field name. Replaced with operator-perspective criteria.

**Iteration 2 (2026-08-23)** — both open clarifications resolved into scope
decisions D1 and D2, recorded in the spec:

- **D1 (was FR-013)**: staleness is a **signal only**; `Status.Result` is never
  modified. Driven by three findings in the code: `Unknown` (severity 4) ranks
  *below* `Fail` (5), so degrading a stale `Fail` would silence the shipped
  `result=~"Fail|Error"` alert; the aggregate reconciler never requeues on a
  timer, so a frozen child produces no event to trigger a degrade; and a
  time-dependent verdict makes `Result` correct only at write time.
- **D1 corollary — a spec correction**: the same decaying-value argument applies
  to a computed staleness judgment in status. The previous FR-003 ("expose how
  many children are overdue") was therefore **wrong** and has been rewritten to
  require non-decaying evidence only. This reverses direction (2) as framed in
  the issue.
- **D2 (was FR-014)**: scope follows the cadence-ownership split found in the
  code, not the issue's "aggregate vs all kinds" framing. `AddonCheck`,
  `DNSCheck`, and `NodeCertificateCheck` own a `spec.interval`; `HealthCheck` and
  `ClusterHealth` do not. Cadence is published for the three; the aggregate is
  fixed at its derivation. No kind gains a staleness status field (FR-013).

**Consequence worth re-checking at plan time**: the resolved scope is expected to
add **zero CRD fields**, which removes this work from the #149 `v1alpha1` → `v1`
freeze critical path (SC-008). If planning finds a new field unavoidable, that
assumption inverts and the work becomes freeze-blocking. This is flagged in
Dependencies and Constraints as the highest-value item to verify first.

**New finding folded in during iteration 2**: the defect also reaches the
exported gauge, not just status — `Status.ObservedAt` is fed directly into
`fathom_check_last_run_timestamp_seconds` beneath a comment asserting the
guarantee that fails. Captured as FR-009 and US1 scenario 2. This raised the
count of mutually inconsistent descriptions from three to four (FR-010).

All checklist items now pass. Ready for `/speckit-plan`.

## Notes

- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`
