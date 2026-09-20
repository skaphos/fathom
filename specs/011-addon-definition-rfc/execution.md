<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# RFC execution evidence

## Scope and setup — 2026-09-20

The implement skill treats quality checklists as read-only. Execution evidence
lives here rather than in `checklists/requirements.md`; task destinations were
updated accordingly. Its 16 quality items are checked; no checklist was edited.

- Branch: `docs/addon-definition-rfc`, initial HEAD `103e1a5`.
- Source baseline: `92c5d8b`; no source/configuration differences at initial HEAD.
- Prerequisites resolved `specs/011-addon-definition-rfc` with tasks present.
- Existing Git, Docker and Helm ignore files cover relevant local artifacts;
  documentation work introduces no new build outputs needing ignore changes.
- Git identity: Shawn Stratton, `shawn@skaphos.io`, matching origin `skaphos`.
- Initial quickstart whitespace, relative-file-link and template-marker checks passed.
- Initial `reuse lint` passed (629/629 files licensed).
- No extension hook configuration is installed.

## Governance

RFC decisions remain proposed. No acceptance, ADR creation, merge, issue update,
or runtime verification is claimed. See tasks T021–T027 for the remaining gates.

## Draft delivery and review

T004–T019 produced six explicitly Proposed RFC decisions and a complete draft
requirement/scenario map in `contracts/rfc-review.md`. The preserved source
baseline establishes current-state claims; the new binding resource, budgets,
publication behavior and authoring tools are proposed future work.

Synthesis: retain the typed engine, authorize runtime definitions through an
admin-owned UID-bound identity, and publish only attributable snapshots. The
strongest objection is operational complexity plus non-atomic authorization
changes. The load-bearing assumption is that binding authors are privileged
administrators, distinct from definition authors; the proposal now states that
same-UID edits inherit delegated read authority. Publication explicitly disclaims
linearizable revocation. A stronger requirement would invalidate this design.

Depth audit: anchored evidence PASS; concrete mechanisms PASS; no new evaluator
framework PASS; real alternatives and status-quo cost PASS; honest costs PASS;
falsifiability PASS; decision visibility PASS. These are author self-review
results, not independent security approval or evidence of runtime enforcement.

Review decisions still required: additional UID-bound resource and edit delegation,
collision suspension, observation-based revocation boundary, and initial caps.

## Validation before review publication

- Expanded quickstart Python checks: whitespace, relative Markdown file targets,
  and unresolved template markers all passed for feature docs, RFC and ADRs.
- `git diff --check`: passed.
- `reuse lint`: passed, 630/630 files with copyright and license information.
- Structural check: exactly six numbered decisions and 27 sequential task IDs;
  T001–T019 completed at first publication.
- Quality checklist, constitution and existing ADRs remain unchanged.
- No runtime source, manifests or Go dependencies changed; runtime/e2e tests were
  not run for this documentation-only work. #280 retains its e2e gate.
- Remote main and feature branch still matched the inspected baseline immediately
  before publication. No existing PR was found for this branch.

## Review publication

T020: [PR #349](https://github.com/skaphos/fathom/pull/349) opened and attached to
the task. Initial proposal revision: `7a860cb4dffb6c604103ba6a131beab865170f29`.
Author, committer and DCO trailer all use `Shawn Stratton <shawn@skaphos.io>`.
Review opened 2026-09-20, scheduled through 2026-09-25. The PR and RFC identify
Shawn Stratton as decider and request operator/API and RBAC/security perspectives.
No accepted decision or runtime implementation is implied by publication.

T021 is awaiting explicit acceptance or revision of the final PR proposal.
T022–T027 depend on that decision. T025 checks have been run before publication;
the task remains open because it also covers subsequent substantive edits.

## Acceptance — 2026-09-20

Shawn Stratton approved revision `80d1dd71033cd7cfa70a6680152d100118f8211a`
in the originating task and instructed updating the PR, watching Copilot and
merging when complete. This shortens the proposed review window to completion of
review/checks. T021 and T022 are complete: RFC status is accepted and ADR 0007
records the narrow supersession without changing ADR 0001. Technical decision
content is unchanged. Merge and implementation handoff remain pending.

## Copilot review clarifications

Seven comments on PR #349 were addressed with explicit roles, a same-UID
retargeting acceptance scenario, uncached APIReader fences, ordered runtime
conversion, binding drain status, preserved evidence wording, and portable
cross-repository links. Earlier local link checks passed because the sibling
checkout existed; the old links were not portable to a standalone checkout.
The accepted decisions remain unchanged; these spell out their enforcement and
observability obligations. Automated re-review is requested before merge.

## Final publication-fence correction

Copilot identified that binding resourceVersion includes status-only writes.
The RFC now explicitly uses binding UID plus spec generation and requires the
status subresource; active-run/drain bookkeeping cannot invalidate authority.
This corrects the mechanism for the already accepted spec-change fence. Missing
and invalid-definition matrix rows also explicitly preserve old observations,
revision and authority context while marking freshness Unavailable.

## Additional Copilot contract clarifications

The final pass requested explicit compile/evaluation panic containment and a
concrete binding GVK/reference/status contract. Both now spell out enforcement
of the accepted authority and failure-isolation requirements. Reference changes
use binding recreation; mutable enabled/scope changes invalidate generation.
HealthReports remain transition-only; unchanged-verdict revision changes are
visible in current status, while historical reports retain their original source.
These clarifications do not add a new execution architecture or redefine history.

## Merge and handoff — 2026-09-20

- [PR #349](https://github.com/skaphos/fathom/pull/349) merged at 18:01:32 UTC
  as `39b3dc5abd7c91b82c357164b98ae35bacfe7587` after all applicable checks passed.
  The workflow skipped runtime e2e for this documentation change; the kind-e2e
  gate passed. CI Summary was skipped by the workflow. No protection bypass used.
- All actionable Copilot threads were addressed and resolved, including binding
  generation fencing, panic containment, binding schema and report semantics.
- Readback: fetched main and compared both accepted RFC and ADR against reviewed
  head `d721000`; contents match. ADR 0001 and quality checklist were unchanged.
- [Epic #280](https://github.com/skaphos/fathom/issues/280) now references immutable
  merged RFC/ADR links and includes accepted implementation work, deferrals,
  #256 ownership/release dependency, and mandatory real-cluster e2e coverage.
  Actual body was read back and matched submitted content (ignoring terminal
  newline formatting). Implementation remains open and unshipped.
- T023–T025 are complete. This follow-up branch records that evidence without
  editing accepted decisions. T026 merge and T027 final readback/issue closure
  remain pending; their final evidence belongs in the follow-up PR record.

### T025 final post-edit validation — 2026-09-20

After the RFC/ADR acceptance edits and handoff evidence changes, the expanded
quickstart checks were rerun against `specs/011-addon-definition-rfc/`,
`docs/rfc/0001-addondefinition-crd.md`, and `docs/adr/`, including untracked
Markdown files:

- Whitespace Python check: PASS, no trailing whitespace.
- Relative Markdown file-target check: PASS, all local targets resolve;
  fragment anchors and remote URL availability are not checked by this script.
- Template-marker Python check: PASS, no unresolved markers.
- `git diff --check`: PASS, no whitespace errors.
- `reuse lint`: PASS, 631/631 files with copyright and license information;
  no missing/bad licenses or read errors.

These results were reconfirmed after adding this record for PR #350's Copilot
comment. They supersede the earlier 630-file pre-review validation count for
T025. Runtime/e2e tests were not run for these documentation-only changes.
