# Data Model: Adversarial Codebase Review for the v0.5.0 Release Gate

**Date**: 2026-07-24 | **Plan**: [plan.md](plan.md)

The feature's data lives in Markdown records under
`specs/004-adversarial-review/review/` and in GitHub issues/PRs. No runtime
types, storage, or CRDs are added. Field-level formats are specified in
[contracts/findings-report.md](contracts/findings-report.md).

## Entities

### ReviewRun

The single review execution for this release gate.

| Field | Description | Validation |
|---|---|---|
| `anchor_commit` | Full SHA of the `main` commit reviewed | MUST be recorded before any perspective starts (FR-001) |
| `date_started` / `date_completed` | Calendar dates of the run | required |
| `perspectives` | The five Perspective records | exactly the five from research R1 (FR-002) |
| `deliverables` | Paths of `findings.md`, `refuted.md`, `coverage.md` | all three MUST exist at completion |

### Perspective

One independent review lens.

| Field | Description | Validation |
|---|---|---|
| `name` | `security` \| `correctness` \| `api-contract` \| `rbac` \| `supply-chain` | fixed set |
| `primary_surface` | Directories/files owned by this lens | per research R1; union of primary surfaces + declared exclusions MUST cover the repository (SC-004) |
| `result` | Confirmed findings, or explicit `no confirmed findings` | never empty/implicit (SC-001) |

### CandidateFinding

A suspected defect raised by one perspective, before refutation.

| Field | Description | Validation |
|---|---|---|
| `id` | `<perspective-prefix>-<seq>` (e.g. `SEC-3`, `COR-1`) | unique within the run |
| `perspective` | Originating lens | required |
| `location` | `file:line` at the anchor commit | required; MUST point at non-generated source unless the finding is about generation inputs |
| `suspected_severity` | critical \| high \| medium \| low | per rubric R3 |
| `failure_scenario` | Concrete inputs/state → wrong outcome | required; "looks wrong" without a scenario is not a candidate |

**State transitions**:

```text
candidate ──refutation passes──▶ confirmed
candidate ──refuted(reason)────▶ refuted      (retained in refuted.md, terminal)
candidate ──inconclusive───────▶ contested    (treated as confirmed at the higher proposed severity)
```

### ConfirmedFinding

A candidate that survived refutation (includes `contested`).

| Field | Description | Validation |
|---|---|---|
| all CandidateFinding fields | severity now final | rubric R3 |
| `refutation_record` | What the skeptical pass attempted and why it failed to refute | required (FR-003) |
| `contested` | Whether refutation was inconclusive | contested findings use the higher proposed severity |
| `duplicates` | IDs merged into this finding | cross-perspective duplicates merge before ranking |
| `disposition` | See Disposition | REQUIRED for critical/high before gate close (FR-005); triage note for medium/low (FR-008) |

**Disposition state transitions** (critical/high only):

```text
open ──▶ fix-in-progress ──▶ fixed(PR#)        (PR merged, CI green, e2e evidence if runtime-behavior)
open ──────────────────────▶ deferred(issue#)  (follow-up issue with written rationale)
fix-in-progress ──fix fails gate──▶ open       (regression: disposition reverts, gate blocked)
```

### Disposition

The resolution of a confirmed finding.

| Field | Description | Validation |
|---|---|---|
| `kind` | `fixed` \| `deferred` \| `triaged` (medium/low only) | critical/high MUST be `fixed` or `deferred` |
| `reference` | PR number (fixed) or issue number (deferred) | required for fixed/deferred |
| `evidence` | For fixed: regression-test statement, `task ci` outcome, e2e CI run link when required (research R7) | FR-006 |
| `rationale` | For deferred: written justification | required; contract-breaking fixes are always deferred (FR-007) |

### CoverageStatement

The written scope record.

| Field | Description | Validation |
|---|---|---|
| `reviewed` | Area → perspectives applied | every top-level repo area appears exactly once across `reviewed` + `excluded` (SC-004) |
| `excluded` | Area → reason (e.g. generated artifacts: generation-input review only) | reasons mandatory (FR-009) |
| `deltas` | Commits after anchor and how each was handled | per spec edge case |

## Relationships

```text
ReviewRun 1──5 Perspective 1──* CandidateFinding
CandidateFinding ──▶ ConfirmedFinding (survives) │──▶ RefutedCandidate (terminal)
ConfirmedFinding 1──1 Disposition ──▶ PR | follow-up issue
ReviewRun 1──1 CoverageStatement
ReviewRun ──recorded on──▶ issue #217 (closes last on milestone, SC-005)
```
