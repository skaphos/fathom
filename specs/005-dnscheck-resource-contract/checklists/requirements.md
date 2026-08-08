# Specification Quality Checklist: DNSCheck Resource Contract

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-08-08
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

**Iteration 1 — 2026-08-08** — two items failed, both traceable to the same two
open questions: the admissible record-type set (Q1) and whether negative
assertions ship in this release (Q2). FR-003 could not be turned into a
pass/fail admission test while Q1 was open.

**Iteration 2 — 2026-08-08** — both questions answered; all items pass.

- **Q1 resolved**: the full record-type set (`A`, `AAAA`, `CNAME`, `SRV`,
  `PTR`) is admissible, and the resolution capability is widened in this same
  slice to evaluate all of them. The schema is not narrowed to today's
  capability; the capability is raised to meet the schema.
- **Q2 resolved**: negative assertions are in. Each target carries explicit
  polarity (FR-005), with the contradictory combination — negative assertion
  plus declared expected answers — rejected at write time.

**Consequences absorbed into the spec, not left implicit**

The Q1 answer propagated further than the record-type field alone. FR-013
forbids publishing a field the evaluation path does not honor, and
`spec.resolvers[]` was in exactly the same position as the record kinds: the
existing capability resolves against the ambient resolver only. Admitting
explicitly addressed resolvers while narrowing only the record types would have
left the honesty violation in place, one field over. Explicit-resolver support
is therefore in this slice too (FR-011), and the specification now spans a
contract half and a capability half — reflected in User Story 2, which is
priority P1 alongside User Story 1 rather than deferred.

Three requirements were added that the narrower scope would not have needed:

- **FR-014** — an unreachable resolver must never be read as satisfying a
  negative assertion. Without polarity this distinction was merely useful
  evidence; with it, conflating the two turns a network fault into false proof
  that a name is gone.
- **FR-021** — a failure arising from a negative assertion must name its
  polarity in the summary, so a deliberate "this must not resolve" failure is
  not triaged as a DNS outage.
- **FR-030** — widening the resolution capability must not change the outcome
  of any check already using it. The capability is shared, so this slice
  carries a regression obligation the schema-only scope did not.

**Scope note carried to planning**

This specification is wider than the issue that prompted it. Issue #265 covers
the contract; the capability half overlaps the runner work scoped to #266. The
split between the two issues needs restating before planning, so the boundary
in the tracker matches the boundary here.

**Non-blocking notes**

- "Written for non-technical stakeholders" is judged against house style. The
  specification uses platform-operator vocabulary (resolver, record kind,
  verdict, aggregate) consistent with the sibling specifications in `specs/`;
  it avoids language, framework, and code-level detail throughout. Marked pass.
- Content Quality holds a deliberate line: Kubernetes resource concepts appear
  because they are the product domain, while implementation mechanics —
  validation-expression language, generator tooling, marker syntax, Go types,
  the probe binary and its flags — appear nowhere in the specification.

**Iteration 3 — 2026-08-08, after `/speckit-clarify`** — all 16 items still
pass; no regressions. Re-run because clarify was executed out of order, after
plan and tasks rather than before.

Four questions were asked and integrated, adding FR-031 through FR-036, SC-008,
SC-009, two Edge Cases, and two acceptance scenarios:

- **Namespace of resolution (FR-031, SC-008).** The specification never said
  where a check's resolution runs from. Because a check is namespaced and its
  author may name any resolver address and any subject, running it from the
  operator's namespace would have let an author borrow an egress posture they
  do not otherwise hold. Confining it to the check's own namespace makes a
  check's reach equal to its author's existing reach, which is why no allowlist
  of resolver addresses is needed.
- **Metric surface (FR-032–034, SC-009).** Previously unstated, not even as an
  exclusion. Now: the generic check gauges plus a per-target gauge carrying the
  outcome as a label. The cardinality cost is real — 288 series per check at
  the caps — so SC-009 states the ceiling as a computable property rather than
  leaving it to emerge, and FR-034 makes raising a cap an explicit cardinality
  decision.
- **Vantage-point fan-out (FR-035).** FR-006 through FR-008 could be read as
  either fan-out or a palette of named definitions, differing threefold in
  evaluation and series count. Now explicitly fan-out with a per-target
  override — which is what the caps had already been sized for, so no numbers
  moved.
- **Pruning removed targets (FR-036).** Elevated in importance by the metric
  answer: without pruning, a target removed from the specification would keep
  reporting a verdict, and with per-target gauges that is an alert on something
  no longer declared that no edit could clear.

**Checklist item that needed work rather than re-judging**: "All functional
requirements have clear acceptance criteria" would have regressed, because
FR-035 arrived with no scenario to test it against. Two acceptance scenarios
were added to User Story 2 rather than marking the item passing on the strength
of the other requirements.

**Non-blocking note on numbering**: FR-031 through FR-036 are appended by
number but placed in the sections they belong to, so numbering is
non-monotonic within sections. The identifiers are stable and referenced from
`plan.md` and `tasks.md`; renumbering to restore monotonic order would churn
those references for a cosmetic gain and was not done.

## Notes

- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`
- All items pass as of iteration 3 (16/16). Clarifications must now be
  propagated into `plan.md` and `tasks.md`, which were generated before this
  pass; `/speckit-analyze` is the cross-artifact check that confirms it.
</content>
