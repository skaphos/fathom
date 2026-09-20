<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# RFC Review Contract: AddonDefinition

This is the review interface for revising
[RFC 0001](../../../docs/rfc/0001-addondefinition-crd.md). It defines evidence
and acceptance criteria, not a Kubernetes API or implemented behavior. All six
runtime choices remain proposals until Shawn Stratton records approval of the
final revision; `@mfacenet` owns review routing.

## Six Principal Decision Topics

The RFC must expose exactly these six decision records. Each record must contain
a selected proposal, relevant alternatives, rationale, costs, source evidence,
an observable example, deferrals, and status `Proposed` until approval. Across
the RFC, the alternatives analysis must include at least two real alternatives
and the status quo.

| Topic | Required review output |
| --- | --- |
| Schema | First-release typed vocabulary, field relationships, scope, validation, unknown-kind outcome, authoring assistance/samples, and no new expression-language recommendation. |
| RBAC | Comparison of pregrants, separate administrator grants, bounded operator grants, and trusted publishers; explicit binding and action establishing authority; declared-permission semantics; metrics-independent diagnostics. |
| Versions | Separate schema, evaluator, addon, and Go adapter contracts; compatibility rules and explicit #256 disposition for `warnRatio`/`failRatio`. |
| Identity | Exact canonical identity, global runtime conflict enforcement, runtime/built-in precedence, upgrade collision handling, and visible outcomes; local CEL is insufficient for global uniqueness. |
| Lifecycle | Missing/add/edit/delete/recreate/invalid/restart/revocation/recovery and in-flight edit/revocation outcomes, with immutable revision and publication fencing. |
| Bounds | Numeric size/work/time/retry/concurrency/result limits, enforcement points, failure outcomes, rationale, and per-definition/check blast radius. |

The revised RFC must settle first-release scope, CLI rendering or other RBAC
assistance, generated samples, the choice to avoid a new expression language,
actual global conflict enforcement, identity-scoped discovery with no manager
fallback exception, minimum requested permissions versus the effective grant
union, and diagnostics independent of metrics. This contract deliberately does
not choose missing numeric runtime thresholds.

## Requirement Traceability

| Requirements | Required output |
| --- | --- |
| FR-001 | Proposal/acceptance labels and final-revision approval evidence. |
| FR-002 | The six complete records above, each with alternatives, rationale, consequences, and an observable example. |
| FR-003 | Schema vocabulary, relationships, validation, unknown kinds, and finite input/work bounds. |
| FR-004–006 | Four-model RBAC comparison, explicit administrator binding, all read paths, fail-closed execution, and declared-permission semantics. |
| FR-007 | Independent contract tracks and bounded #256 resolution or deferral. |
| FR-008 | Canonical identity, runtime conflict, built-in precedence, and upgrade collision result. |
| FR-009–011 | Complete lifecycle table, coherent revision/auth context, publication fence, prior evidence age/freshness, and recovery. |
| FR-012 | Numeric limits, enforcement, retry/concurrency policy, and isolated failure. |
| FR-013–014 | Settled scope, RBAC aid and samples, plus explicit first-release non-goals and execution topology. |
| FR-015 | New ADR relationship to immutable ADR 0001. |
| FR-016 | Merged RFC/ADR references and actual #280 update/readback with accepted work and deferrals. |

## Scenario Review Matrix

Every row needs a single derivable outcome in the final RFC. “Evidence” means
text, table entry, source citation, or recorded governance artifact a reviewer
can inspect; it does not mean proof of runtime implementation.

| Scenario | Evidence expected | Outcome criterion |
| --- | --- | --- |
| Author names a broadly granted identity | Binding rule, impersonation analysis, admin action | Definition cannot borrow grants from operator impersonation alone. |
| Scoped client or binding absent | In-cluster and local execution paths | Fail closed; no manager or kubeconfig privilege inheritance. |
| Discovery plus addon/core reads | Per-path identity trace | Every request is assigned to the same declared identity; no FR-005 discovery exception. |
| Requested permission is narrower than actual grants | Additive-RBAC analysis and selected diagnostic basis | RFC chooses minimum request or effective union and states diagnostic limits. |
| Metrics are disabled | Installation source for access-review diagnostics | Diagnostic behavior and permissions do not depend on metrics configuration. |
| Four grant models compared | Privilege delta, administrator step, operating cost | Selected model and rejected alternatives are reviewable without implying automatic grants. |
| Definition is missing or added | Lifecycle row, visible condition, retry/watch trigger | Active state and recovery are deterministic; prior success is not refreshed. |
| Definition is edited during evaluation | UID/generation snapshot and publication check | Inputs, verdict, and evidence use one revision; superseded result has a visible disposition. |
| Authorization revoked in flight | Authorization-context model and publication fence | No atomic-revocation claim; obsolete completion cannot publish fresh success. |
| Definition deleted | Owner-aware removal and evidence policy | Authority ends; last-known evidence remains aged and visibly unavailable/stale. |
| Same name is recreated | UID-based identity and restart path | New object cannot receive evidence or authority from the deleted UID. |
| Definition is invalid or unknown-kind | Admission/reconciliation validation split | Responsible definition visibly fails and unrelated definitions continue. |
| Restart sees definition and grant at different times | Reconstructible-state and fail-closed ordering | No transient manager-client fallback; recovery is deterministic. |
| Authorization later recovers | Requeue trigger and evidence transition | New attempt records a new context; old evidence remains attributable. |
| Two runtime objects claim one identity | Named global arbitration enforcement point | Exactly one deterministic visible result; schema CEL alone is not claimed sufficient. |
| Upgrade adds a colliding built-in | Upgrade backstop and precedence policy | Existing runtime state is neither silently replaced nor accepted ambiguously. |
| Built-in replacement is requested | First-release scope and migration policy | Inclusion or deferral is explicit; built-in pack remains available as decided. |
| Cluster versus namespace scope | Alternatives, target topology, authority ownership | One first-release scope is chosen, including cross-namespace behavior. |
| RBAC assistance and generated samples | CLI/rendering and docs-only alternatives | First-release inclusion and non-goals are separately settled. |
| Existing typed evaluators mapped | Vocabulary inventory including APIService reuse | Supported families are exact; no new user expression language is recommended. |
| Schema or evaluator semantics change | Independent version tracks | Compatibility and upgrade behavior do not rely on the Go contract version alone. |
| Reserved ratio keys meet #256 | Issue dependency/deferral record | Owner, dependency, resolution condition, and compatibility consequence are explicit. |
| Large list or response | Object/page/result-volume bound and enforcement point | A numeric RFC limit and visible isolated failure exist; timeout alone is insufficient. |
| Multiple limits hit or failures repeat | Precedence, retry/backoff, concurrency policy | Deterministic reason, bounded retry, and progress for unrelated work. |
| Evidence exceeds freshness window | Timestamp, source revision, freshness rule | Stale evidence remains inspectable and is never current success. |
| `ClusterHealth` consumes outcomes | Contract trace to `HealthCheck.status` | No direct definition or report-history input is introduced. |

## Review and Approval Lifecycle

Allowed states are `Draft`, `In Review`, `Accepted`, `Rejected`, and
`Withdrawn`. Draft enters In Review with the actual forum, dates, revision, and
required perspectives recorded. A substantive change returns the affected final
revision to review and requires renewed approval. Accepted content is immutable;
later substantive change uses a superseding RFC. Rejected and Withdrawn remain
historical records and do not complete this feature.

Approval must identify Shawn Stratton's decision and the exact final revision.
Authorship, ownership, a merge event, silence, or this contract cannot stand in
for approval. The accepted RFC produces a merged accepted ADR that relates the
decision to immutable [ADR 0001](../../../docs/adr/0001-in-process-adapter-contract.md).

## Completion and Handoff Evidence

Completion requires all of the following, with no implementation claim:

1. Recorded approval of the final RFC revision and a merged accepted RFC.
2. A merged accepted RFC-linked ADR; ADR 0001 remains unchanged.
3. The existing #280 epic actually updated with accepted scope, deferrals,
   implementation validation, merged RFC/ADR links, and an explicit #256 disposition.
4. Read-back of #280 confirming the update; no duplicate implementation epic.
5. #278 closed or updated only after the preceding evidence exists.

Local Markdown, link, SPDX, and whitespace checks establish artifact quality.
They do not establish approval, merge, issue mutation, runtime correctness, or
deployment. Implementation and real-cluster validation remain future #280 work.
