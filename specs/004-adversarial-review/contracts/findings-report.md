# Deliverable Contracts: Findings Report, Refuted Record, Coverage Statement

**Date**: 2026-07-24 | **Plan**: [../plan.md](../plan.md) | **Data model**: [../data-model.md](../data-model.md)

These are the external interfaces of this feature: the formats consumed by
the release manager, issue #217 readers, and future auditors. The files live
under `specs/004-adversarial-review/review/`.

## 1. `review/findings.md` — ranked confirmed findings (deliverable 1)

Required header block:

```markdown
# Adversarial Review Findings — v0.5.0 Release Gate (#217)

**Anchor commit**: <full SHA>
**Review dates**: <start> – <end>
**Perspectives**: security, correctness, api-contract, rbac, supply-chain
```

Then one section per severity, critical first. Each finding:

```markdown
### <ID>: <one-line title> (<severity>)

- **Location**: `<file>:<line>` @ anchor commit
- **Perspective**: <lens> (duplicates merged: <IDs or "none">)
- **Failure scenario**: <concrete inputs/state → wrong outcome>
- **Refutation attempted**: <what the skeptical pass tried; why it did not refute>  <!-- append "; CONTESTED — treated at higher severity" when applicable -->
- **Disposition**: fixed (PR #NN) | deferred (issue #NN) | triaged — <note>
- **Evidence**: <for fixed: regression test + `task ci` result + e2e CI run link if runtime-behavior; for deferred: one-line rationale pointer>
```

Rules:

- Findings sorted by severity, then ID.
- Every critical/high entry MUST have a `fixed` or `deferred` disposition
  before #217 closes; `triaged` is valid only for medium/low.
- A severity section with no findings states `_No confirmed findings._`
- IDs are `SEC-n`, `COR-n`, `API-n`, `RBAC-n`, `SCM-n` per originating
  perspective (the first finder's ID survives a duplicate merge).

## 2. `review/refuted.md` — refuted candidates (working record, FR-003)

```markdown
# Refuted Candidates — v0.5.0 Release Gate (#217)

| ID | Location | Suspected issue (one line) | Refutation reason |
|----|----------|----------------------------|-------------------|
```

Every candidate that did not survive appears here — an empty table means
every candidate was confirmed, not that refutation was skipped.

## 3. `review/coverage.md` — coverage statement (deliverable 3)

```markdown
# Coverage Statement — v0.5.0 Release Gate (#217)

**Anchor commit**: <full SHA>

## Reviewed
| Area | Perspectives applied | Result |
|------|----------------------|--------|
| internal/controller/ | correctness (primary), security | <n> confirmed / no confirmed findings |
...

## Intentionally excluded
| Area | Reason |
|------|--------|
| zz_generated*.go, config/crd/bases, bundle/, docs/reference/api.md, graphify-out/ | generated; reviewed via generation inputs only |
...

## Post-anchor deltas
| Commit | Handling |
|--------|----------|
```

Rule: the union of `Reviewed` + `Intentionally excluded` areas MUST classify
every top-level path in the repository (SC-004). Perspectives with zero
confirmed findings still appear with the explicit result.

## 4. Deferral follow-up issues

Each deferred critical/high finding opens a GitHub issue with:

- Title: `<severity>: <finding title> (adversarial review #217, <ID>)`
- Labels: `security` when applicable, severity label, and the milestone it is
  proposed for.
- Body MUST contain: location, failure scenario, refutation record, and a
  **Deferral rationale** section explaining why it is not fixed in v0.5.0
  (e.g. requires a breaking CRD change, per FR-007).

## 5. Closing comment on #217

A single comment posted when the gate completes, containing: link to the
merged review deliverables (repo paths at a merged commit), the disposition
table for all critical/high findings (ID → fixed PR / deferred issue), the
perspective completion summary, and the statement that `task ci` is green.
#217 is then closed as the last open issue on the v0.5.0 milestone (SC-005).
