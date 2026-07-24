# Feature Specification: Adversarial Codebase Review for the v0.5.0 Release Gate

**Feature Branch**: `004-adversarial-review`

**Created**: 2026-07-24

**Status**: Draft

**Input**: User description: "Issue 217, we're going to do an adversarial review of this codebase and resolve critical/high findings"

**Tracking Issue**: [#217 — Release-gate: adversarial review of the codebase before v0.5.0 + address surfaced issues](https://github.com/skaphos/fathom/issues/217)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Multi-Perspective Adversarial Review Produces a Ranked Findings List (Priority: P1)

As the release manager for v0.5.0, I need a full, skeptical review of the entire
repository at the tip of `main`, performed from multiple independent
perspectives (security, correctness/reconcile-time behavior, API/CRD contract
stability, RBAC least-privilege, supply-chain/CI), so that I have a ranked,
evidence-backed list of confirmed defects before cutting the release.

Every candidate finding must survive an explicit refutation attempt before it
is accepted: for each candidate, an independent skeptical pass tries to prove
the finding wrong, and only findings that withstand that challenge are kept.
This prevents plausible-but-false findings from consuming milestone time.

**Why this priority**: The findings list is the foundation of the entire
release gate — no fixing, deferring, or coverage reporting can happen without
it. It is independently valuable even if no fixes follow, because it gives an
honest picture of release risk.

**Independent Test**: Can be fully tested by running the review and inspecting
the deliverable: a ranked list where every entry carries a severity, a precise
location (file and line), a concrete failure scenario, and a record of the
refutation attempt it survived.

**Acceptance Scenarios**:

1. **Given** the repository at the tip of `main` with all other v0.5.0
   milestone issues closed, **When** the adversarial review is executed,
   **Then** each of the five review perspectives produces its candidate
   findings independently, and the perspectives used are documented.
2. **Given** a candidate finding from any perspective, **When** it undergoes
   the adversarial refutation step, **Then** it is either confirmed (with the
   refutation attempt recorded) or discarded (with the reason it was refuted),
   and only confirmed findings appear in the final list.
3. **Given** the confirmed findings, **When** the final list is assembled,
   **Then** every entry is ranked by severity and includes severity,
   file:line, and a concrete failure scenario describing how the defect
   manifests.

---

### User Story 2 - Confirmed Critical/High Findings Are Fixed or Explicitly Deferred (Priority: P2)

As a maintainer, I need every confirmed critical- and high-severity finding to
be either fixed within this milestone or converted into a tracked follow-up
issue with a written rationale for deferring, so that v0.5.0 ships with no
silently ignored serious defects.

**Why this priority**: This is the "resolve" half of the release gate and the
core of the user's request. It depends on User Story 1's findings list but is
the action that actually reduces release risk.

**Independent Test**: Can be tested by cross-checking the confirmed findings
list against the milestone: every critical/high entry maps to either a merged
fix or a newly created follow-up issue containing a deferral rationale.
No critical/high finding lacks a disposition.

**Acceptance Scenarios**:

1. **Given** a confirmed critical or high finding, **When** the resolution
   phase completes, **Then** the finding has exactly one recorded disposition:
   a fix merged in this milestone, or a tracked follow-up issue with a written
   justification for deferral.
2. **Given** a fix for a confirmed finding, **When** it is prepared for merge,
   **Then** it includes a regression test that fails before the fix and passes
   after it, and the full local CI gate passes.
3. **Given** a fix that touches operator runtime behavior, **When** it is
   prepared for merge, **Then** an end-to-end verification run is recorded for
   it per the project's testing policy.
4. **Given** a confirmed medium or low finding, **When** the resolution phase
   completes, **Then** it is documented in the findings deliverable and
   triaged (fixed opportunistically, or noted for future work) without
   blocking the release gate.

---

### User Story 3 - Coverage Statement Eliminates Silent Gaps (Priority: P3)

As a future maintainer or auditor, I need a short written statement of what
was reviewed and what was intentionally out of scope, so that nobody later
mistakes an unreviewed area for a reviewed-and-clean one.

**Why this priority**: The coverage statement makes the review's guarantees
honest and durable. It is lightweight but required by the release gate's
acceptance criteria; without it the review's silence in an area is ambiguous.

**Independent Test**: Can be tested by reading the statement and verifying
that every top-level area of the repository is either listed as reviewed
(with the perspectives applied to it) or explicitly listed as out of scope
with a reason.

**Acceptance Scenarios**:

1. **Given** the completed review, **When** the coverage statement is written,
   **Then** it names the areas reviewed, the perspectives applied, and every
   intentional exclusion with its reason — leaving no repository area
   unaccounted for.
2. **Given** the coverage statement and the findings list, **When** issue #217
   is closed, **Then** both deliverables are attached to or referenced from
   the issue.

---

### Edge Cases

- **No findings survive refutation in a perspective**: the perspective is
  still documented in the coverage statement with an explicit "no confirmed
  findings" result — absence of findings is reported, never implied.
- **A finding's only complete fix would break the stable external contract**
  (CRD schema compatibility or the `ClusterHealth` external contract): the
  finding is deferred to a tracked follow-up with rationale rather than
  fixed in a contract-breaking way inside this milestone.
- **A fix introduces a regression**: the CI gate (lint, unit tests, static
  analysis, vulnerability scan, build) and — for runtime-behavior changes —
  the end-to-end suite must pass before the fix counts as a disposition;
  a failing fix reverts to "undispositioned" and blocks the gate.
- **Two perspectives report the same underlying defect**: duplicates are
  merged into a single finding before ranking so the count reflects distinct
  defects.
- **A finding is disputed during refutation but not clearly refuted**: it is
  kept, marked as contested, and dispositioned conservatively (treated at the
  higher of the proposed severities).
- **New commits land on `main` mid-review**: the review is anchored to a
  recorded commit; material changes after that point either restart the
  affected perspective or are explicitly listed as out of scope in the
  coverage statement.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The review MUST cover the whole repository at a recorded commit
  at the tip of `main`, after all other v0.5.0 milestone issues are closed.
- **FR-002**: The review MUST apply at least five independent perspectives:
  security, correctness/reconcile-time behavior, API/CRD contract stability,
  RBAC least-privilege, and supply-chain/CI integrity — each producing its
  candidate findings independently of the others.
- **FR-003**: Every candidate finding MUST undergo an adversarial refutation
  attempt; only findings that survive refutation are confirmed. Refuted
  candidates and their refutation reasons MUST be retained in the working
  record.
- **FR-004**: The confirmed findings deliverable MUST be a ranked list where
  each entry carries severity (critical/high/medium/low), file:line location,
  and a concrete failure scenario.
- **FR-005**: Every confirmed critical or high finding MUST receive exactly
  one disposition: a fix merged in this milestone, or a new tracked follow-up
  issue containing a written rationale for deferral.
- **FR-006**: Every merged fix MUST include a regression test that fails
  before the fix, and MUST pass the full local CI gate; fixes touching
  operator runtime behavior MUST additionally have an end-to-end verification
  run recorded per the project's testing policy.
- **FR-007**: Fixes MUST NOT break the stable external contracts: CRD schema
  compatibility against the latest release and the `ClusterHealth` external
  contract (derived only from `HealthCheck` status). A finding whose fix
  would require breaking these is deferred per FR-005.
- **FR-008**: Confirmed medium and low findings MUST be documented and
  triaged (fixed opportunistically or recorded for future work); they do not
  block the release gate.
- **FR-009**: A written coverage statement MUST enumerate what was reviewed,
  which perspectives were applied where, and every intentional exclusion with
  its reason.
- **FR-010**: The findings list and coverage statement MUST be recorded on
  issue #217 before it is closed, and #217 MUST be the final issue closed on
  the v0.5.0 milestone.

### Key Entities

- **Candidate Finding**: a suspected defect raised by one review perspective;
  attributes: perspective of origin, location, suspected severity, suspected
  failure scenario. Becomes either a Confirmed Finding or a Refuted Candidate.
- **Confirmed Finding**: a candidate that survived adversarial refutation;
  attributes: severity, file:line, failure scenario, surviving-refutation
  record, disposition (fix or deferral).
- **Disposition**: the resolution of a confirmed finding — either a merged
  fix (with its regression test and verification evidence) or a tracked
  follow-up issue (with deferral rationale).
- **Coverage Statement**: the written record of reviewed areas, applied
  perspectives, and intentional exclusions.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All five review perspectives complete and are documented; each
  reports either confirmed findings or an explicit "no confirmed findings"
  result — zero perspectives silently skipped.
- **SC-002**: 100% of confirmed critical and high findings have a recorded
  disposition (merged fix or tracked deferral with rationale) before the
  release gate closes; zero critical/high findings without disposition.
- **SC-003**: 100% of merged fixes carry a regression test that failed before
  the fix, and the project's full CI gate is green at the end of the work.
- **SC-004**: The coverage statement accounts for every top-level area of the
  repository — an independent reader can classify any given file as
  "reviewed" or "explicitly excluded" using the statement alone.
- **SC-005**: Issue #217 is closed with both deliverables attached, and it is
  the last issue closed on the v0.5.0 milestone.

## Assumptions

- The v0.5.0 milestone is otherwise clear: as of 2026-07-24, #217 is the only
  open issue on the milestone, so the fix-application phase may start
  immediately (verified against the issue tracker).
- Severity uses the conventional four-level scale (critical/high/medium/low),
  judged by exploitability/impact for security findings and by
  user-visible-damage likelihood for correctness findings.
- "Resolve" (from the user's request) means the disposition model of issue
  #217: fix in-milestone or defer via a tracked follow-up with rationale —
  not necessarily fixing every finding in place.
- Medium/low findings are in scope for documentation and triage but out of
  scope for mandatory in-milestone fixes.
- End-to-end verification for runtime-behavior fixes runs in the project's
  CI pipeline (local e2e is not available in this development environment);
  the recorded evidence is the CI run result.
- Generated artifacts (deep-copy code, OLM bundle metadata, generated
  manifests and API reference docs) are reviewed only for generation-input
  correctness, not hand-audited line-by-line; this is recorded in the
  coverage statement.
