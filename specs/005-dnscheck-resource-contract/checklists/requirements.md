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

## Notes

- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`
- All items pass as of iteration 2. Ready for `/speckit-plan`.
</content>
