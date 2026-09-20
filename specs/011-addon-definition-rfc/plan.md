<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Implementation Plan: Complete AddonDefinition Design RFC

**Branch**: `docs/addon-definition-rfc` | **Date**: 2026-09-19 | **Spec**: [spec.md](spec.md)

**Input**: `specs/011-addon-definition-rfc/spec.md`

## Summary

Complete [RFC 0001](../../docs/rfc/0001-addondefinition-crd.md) for
[#278](https://github.com/skaphos/fathom/issues/278), obtain recorded approval,
merge the accepted RFC, and update the existing
[#280](https://github.com/skaphos/fathom/issues/280) implementation epic. All
three external outcomes are required; a finished draft alone is insufficient.

Revise the six decision areas using source-backed research, explicit
authorization analysis, and a lifecycle/outcome matrix. Proposed recommendations
remain distinct from accepted decisions. This feature delivers documents and a
review outcome; CRD types, runtime loading, permission grants, and the adapter
version bump remain outside this feature.

## Technical Context

**Language/Version**: Markdown; existing Go 1.27.1 runtime per [go.mod](../../go.mod), inspected as evidence.

**Primary Dependencies**: Existing RFC 0001, ADR 0001, declarative engine,
registry, Kubernetes authorization/schema documentation, issues #278/#280/#256,
and the repository PR workflow. No new runtime dependency.

**Storage**: Versioned Markdown in Git; decision/merge evidence in the RFC PR;
implementation scope in the existing GitHub epic.

**Testing**: Requirement-to-decision review, source-reference checks, scenario
walkthroughs, relative-link and whitespace checks, `reuse lint`, and read-only
GitHub verification. Document checks do not establish runtime correctness.

**Target Platform**: Repository and GitHub review workflow. Kubernetes is the
subject of the design; this work does not install anything into a cluster.

**Project Type**: Architecture RFC completion and implementation handoff.

**Performance Goals**: All six decisions and every required lifecycle case have
a determinate review outcome. The RFC must specify finite runtime limits with
rationale; this phase makes no measured runtime performance claim.

**Constraints**: Preserve accepted ADRs, `ClusterHealth` derivation from
`HealthCheck.status`, GitOps operation, bounded reconciliation and least
privilege. No generated manifest edits, new API implementation or implicit approval.

**Scale/Scope**: One existing RFC, six principal decisions, 16 functional
requirements, a new decision record upon acceptance, and one existing epic
update. Select the next unused ADR number when authoring the record.

## Constitution Check

Initial gate passed before Phase 0: documentation scope needs no constitution
exception. Re-evaluate these same gates after Phase 1 and on substantive changes.

| Gate | Design obligation | Initial / post-design |
| --- | --- | --- |
| I, IV: explicit Kubernetes state | Describe identity, lifecycle, conditions and evidence; do not present proposed fields as shipped APIs. | Pass / Pass |
| II, III: reconstructible state | Keep definitions and authorization declarative; cover restart, collisions and revision races. | Pass / Pass |
| V: independent adoption | No consuming Skaphos control plane becomes a loading dependency; use the established evaluator model. | Pass / Pass |
| VI, VII: explainability and degradation | Require revision, failure reason, freshness and readable prior evidence without fabricated fresh success. | Pass / Pass |
| VIII: topology | State definition scope, target scope and identity namespace explicitly; labels alone do not establish authority. | Pass / Pass |
| IX: honest scope | Separate verified current behavior, proposed design, accepted design and shipped implementation. | Pass / Pass |
| Minimal RBAC and bounded work | Analyze every read path and privileged fallback; require finite size/work/time/retry limits. | Pass / Pass |
| Stable ClusterHealth contract | No new direct dependency on definitions or report history. | Pass / Pass |
| Governance and immutable ADRs | Record approval; preserve ADR 0001; add a linked decision record; DCO-signed Conventional Commits and PR merge. | Pass / Pass |

A later recommendation that weakens a constraint must reopen this gate.

## Project Structure

### Documentation (this feature)

```text
specs/011-addon-definition-rfc/
├── spec.md
├── checklists/requirements.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    └── rfc-review.md
```

`tasks.md` is produced by the subsequent `speckit-tasks` command.

### Source Code (repository root)

```text
docs/rfc/0001-addondefinition-crd.md          # existing proposal to revise later
docs/adr/0001-in-process-adapter-contract.md # immutable prior decision
docs/adr/                                  # new accepted decision record later
internal/adapter/declarative/               # read-only evidence
internal/adapter/registry/                  # read-only evidence
internal/adapter/impersonation/             # read-only evidence
internal/adapter/rbacgen/                   # read-only evidence
internal/controller/addoncheck_controller.go # read-only evidence
pkg/adapter/                               # read-only evidence
config/rbac/                               # read-only generated evidence
```

**Structure Decision**: Revise the existing RFC. Keep research, review contracts
and validation guidance here; do not create a competing RFC or deployable API.

## Phase 0: Research

Consolidate repository and primary-source findings in [research.md](research.md):

1. Distinguish permission to impersonate an identity from authority to assign
   that identity to a runtime definition, including local execution.
2. Check registry replacement, definition ownership, missing-adapter recovery,
   freshness and scope of existing validation.
3. Separate schema, evaluator semantics, addon version and Go adapter contract;
   identify the actual #256 compatibility decision.
4. Establish the review owner, ADR handling and objective evidence required for
   the approved/merged/handed-off completion condition.

Research resolves how to perform this documentation work. The six runtime
choices are the RFC work product; research recommendations do not approve them.

## Phase 1: Design Artifacts

- [data-model.md](data-model.md): conceptual entities the RFC must describe,
  plus decision/review/handoff records and their validation rules.
- [contracts/rfc-review.md](contracts/rfc-review.md): six-decision package,
  outcome matrix, requirement coverage and completion evidence. This is a
  review interface, not a Kubernetes API schema.
- [quickstart.md](quickstart.md): runnable document checks and read-only
  verification of eventual approval, merge and epic update.

## Execution Sequence After Planning

1. Refresh evidence if the base revision changed. Correct overstated current
   claims before drafting conclusions.
2. Revise RFC 0001 with six complete recommendations, real alternatives including
   the status quo, exact first-release scope, lifecycle outcomes, numeric limits
   with rationale, compatibility, rollout and reversibility.
3. Walk every scenario and map FR-001 through FR-016 to evidence. Resolve scope,
   authoring aids, identity binding and admission feasibility before decision.
4. Open the documentation PR. The RFC names Shawn Stratton as decider;
   [.github/CODEOWNERS](../../.github/CODEOWNERS) names `@mfacenet` as default
   owner. Include RBAC/security and operator/API review perspectives; roles need
   not be different people. Record actual review start/end dates when opening
   review. Proposed window: five business days, adjustable by a recorded decider
   decision; these dates are scheduling metadata, not an unresolved design choice.
5. Incorporate review outcomes. Substantive changes reopen affected decisions.
   Record explicit acceptance of the final proposal and add a concise ADR that
   links the RFC and explains whether ADR 0001 is extended or superseded. Keep
   ADR 0001 unchanged. Rejection or withdrawal is recorded and does not satisfy
   this feature's completion condition.
6. Merge through the repository PR process after required checks and recorded
   acceptance. Verify that the merged RFC contains the accepted text. Report the
   feature as awaiting approval or merge if either event is outstanding.
7. Update #280 with merged RFC/ADR references, scoped work, explicit deferrals,
   #256 disposition and implementation validation needs. Read it back before
   marking #278 complete.

This planning command stops after Phase 1. It does not open or merge a PR,
change RFC status, or update GitHub issues during this turn.

## Validation and Reversibility

Use [quickstart.md](quickstart.md) after drafting and substantive review changes.
Local checks establish document consistency; decision evidence establishes
acceptance. Runtime e2e is unnecessary for this document-only change; #280
retains its real-cluster validation requirement.

Before acceptance, revise or withdraw through the PR. After acceptance, record
changed architectural decisions in a superseding record instead of rewriting
accepted history. An epic update changes planning scope, not a cluster; later
corrections remain attributable issue revisions.

## Complexity Tracking

No constitution violations or additional runtime components are introduced.
