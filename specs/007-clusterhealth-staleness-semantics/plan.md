# Implementation Plan: Cadence-Aware Staleness Semantics for ClusterHealth

**Branch**: `feature/277-clusterhealth-staleness` | **Date**: 2026-08-23 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/007-clusterhealth-staleness-semantics/spec.md`

## Summary

`ClusterHealth.Status.ObservedAt` takes the **newest** child's observation while
the verdict takes the **worst**, so a frozen child's stale `Fail` propagates while
a live sibling makes the aggregate report zero staleness. The same field feeds the
`fathom_check_last_run_timestamp_seconds` gauge, so the defect reaches the surface
operators alert on.

**Approach**: invert the derivation to the **stalest** child, and publish each
check's **effective cadence** as a new gauge so staleness can be expressed
relative to cadence instead of a hardcoded constant. Staleness stays a signal —
the verdict fold is untouched (D1). No timer requeue is needed, because a stalest
*timestamp* is a fact that never needs updating, unlike a stored judgment (R3).

Two spec statements did not survive research and are corrected here: `HealthCheck`
must be in scope or the aggregate has no cadence at all (R4), and the overdue
multiplier belongs in the shipped alert rules rather than operator config (R5).

## Technical Context

**Language/Version**: Go per `go.mod` (currently 1.27.0)

**Primary Dependencies**: controller-runtime, `k8s.io/*`, `prometheus/client_golang` — all existing; **no new dependencies**

**Storage**: Kubernetes CR status only. No `HealthReport` history is read (constitution).

**Testing**: envtest + Ginkgo/Gomega for controllers; stdlib `testing` for unit-level folds; Ginkgo e2e against kind

**Target Platform**: Kubernetes operator (Linux)

**Project Type**: Single Go module, kubebuilder v4 layout

**Performance Goals**: Aggregation stays O(children) with no additional API reads; one extra comparison per child in the loop that already builds `Status.Children`

**Constraints**: Bounded, idempotent reconcile; no timer requeue added; `ClusterHealth` derives only from `HealthCheck.status`

**Scale/Scope**: Aggregates gain a bounded child list with deterministic truncation; verdict and staleness are computed over the **full** selected set regardless of the cap

## Constitution Check

*GATE: evaluated before Phase 0, re-evaluated after Phase 1 design.*

| Principle / Constraint | Assessment | Status |
| --- | --- | --- |
| I — Explicit state over implicit behavior | Staleness becomes an explicit, documented property of the aggregate instead of an undocumented accident of fold order. Four contradictory descriptions are reconciled. | **PASS** |
| II — Git is the durable desired-state boundary | No live-resource mutation; all behavior lands as code and manifests. | **PASS** |
| `ClusterHealth` contract stability | Derivation remains exclusively from `HealthCheck.status`; `HealthReport` is not read. The field's *meaning* changes, handled as a breaking change with an ADR (FR-012). | **PASS** (with declared break) |
| Bounded, idempotent reconciliation | No timer requeue added (R3). One extra comparison per child in an existing loop; no new API reads per child. | **PASS** |
| Minimal RBAC | No new permissions. The cadence lookup reads check kinds the operator already watches and owns. | **PASS** |
| Configuration model | **No new operator option** after R5 — the multiplier is packaging, not runtime config. Nothing enters `Options`/`bindings()`. | **PASS** |
| CRD API versioning (maturity ratchet) | Bounding `Children[]` narrows an existing `v1alpha1` field. In-place breaking changes are forbidden at **beta/GA**; `v1alpha1` is alpha, so this is permitted — but it must land **before** #149 promotes to `v1`. | **PASS** (schedule-constrained) |
| Documentation standard | Generated API reference is regenerated, never hand-edited. A hard-to-reverse decision gets an immutable ADR (FR-012). | **PASS** |
| Repository governance | PR, DCO sign-off, full CI gate, e2e required for `internal/controller/*`. | **PASS** |

**No unjustified violations.** The one item needing explicit sanction is the CRD
compatibility gate flagging the `MaxItems` narrowing, which is recorded in
Complexity Tracking and handled through the repo's existing allowlist mechanism.

## Project Structure

### Documentation (this feature)

```text
specs/007-clusterhealth-staleness-semantics/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── clusterhealth-status.md
│   └── metrics.md
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
api/v1alpha1/
├── clusterhealth_types.go              # ObservedAt godoc; Children MaxItems
└── healthcheck_types.go                # (read-only: CheckRef drives cadence lookup)

internal/controller/
├── clusterhealth_controller.go         # stalest derivation; truncation; gauge feed
├── observe.go                          # observeCheck gains effective cadence
├── addoncheck_controller.go            # pass addonCheckInterval to observeCheck
├── dnscheck_controller.go              # pass dnsCheckInterval
├── nodecertificatecheck_controller.go  # pass nodeCertInterval
└── healthcheck_controller.go           # resolve + pass source check's cadence (R4)

internal/metrics/
└── metrics.go                          # fathom_check_interval_seconds; DeleteCheckSeries

config/
├── crd/bases/                          # regenerated
└── components/prometheus-rule/         # cadence-relative staleness rule

deploy/helm/fathom-operator/            # overdue multiplier value (R5)

docs/
├── adr/0005-*.md                       # semantic redefinition ADR (FR-012)
├── guides/monitoring.md                # correct the false guarantee; staleness framing
└── reference/api.md                    # regenerated

.crd-compat-allowlist.yaml              # sanctioned entry for the MaxItems narrowing

test/e2e/                               # aggregate staleness spec
```

**Structure Decision**: Standard kubebuilder layout, unchanged. The work is
concentrated in `internal/controller` (derivation + the shared `observeCheck`
seam) and `internal/metrics` (one new gauge), with generated artifacts and docs
following. No new packages.

## Implementation Phases

### Phase A — Staleness derivation (the reported defect)

Invert the fold in `clusterhealth_controller.go` from newest to stalest; a
never-observed child dominates and yields nil, which the gauge already renders as
the `0` never-ran sentinel. Clamp future timestamps. Correct the godoc, the
gauge-emission comment, and `monitoring.md` — in **staleness** framing per D3.
Regenerate the API reference.

*Delivers US1 standalone. No schema change. No dependency on later phases.*

### Phase B — Cadence publication

Add `fathom_check_interval_seconds`; extend `observeCheck` and
`metrics.ObserveCheck`; pass each kind's existing resolver at its call site; add
the cadence lookup for `HealthCheck` via `CheckRef` (R4); drop the new series in
`DeleteCheckSeries`. Aggregate cadence is the max across children.

*Delivers US2 and US3. Metrics only — no CRD schema, so no #149 deadline.*

### Phase C — Shipped alerting rules

Rewrite the staleness rule to be cadence-relative, removing the hardcoded 900s.
Add the multiplier as a chart value / component setting, default 3 (R5). Keep
`task verify-alert-rules` passing.

### Phase D — Child-list bound

`MaxItems` on `Children[]`, deterministic truncation ordered by severity then
staleness, `MatchedCount` guaranteed to remain the pre-truncation total. Add the
sanctioned `.crd-compat-allowlist.yaml` entry. Regenerate CRDs.

*The only schema-touching phase, and therefore the only one on the #149 critical
path. Separable into its own issue without affecting A–C.*

### Phase E — Release framing

ADR-0005 recording the semantic redefinition and why a parallel field was
rejected; `BREAKING CHANGE` footer on the landing commit.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| CRD compatibility gate will flag the `Children[]` narrowing as incompatible, requiring a sanctioned allowlist entry | Bounding a previously unbounded status list is the point of FR-016; any bound narrows the accepted set and trips the gate by construction | Leaving the list unbounded was the status quo the clarification session explicitly rejected. Working around the gate rather than sanctioning the entry would hide a real compatibility fact from the next reviewer. |
| A field's meaning changes without any schema signal | The gate diffs schemas; name, type and optionality are unchanged, so no automated check can detect a semantic redefinition | Adding a parallel field and deprecating the old one avoids the break, but adds CRD surface immediately before the `v1` freeze and leaves two fields where one is correct. Compensated instead by the breaking-change marker plus ADR (FR-012). |

## Risks

| Risk | Mitigation |
| --- | --- |
| **C1 and C2 amend the spec.** Research contradicted D2 (HealthCheck scope) and FR-015 (config home). | Both recorded in research.md with evidence; spec should be amended before `/speckit-tasks`. C2 *reduces* scope. |
| Max-cadence aggregation detects a frozen fast child on the slow child's timescale | Accepted and documented (R4). Per-check alerts still fire on the fast child's own cadence; the aggregate is a roll-up, not the sole signal. |
| Phase D puts the work on the #149 freeze critical path | Phases A–C carry no schema change and can land independently; D is separable into its own issue if the deadline pressure is unwelcome. |
| `CheckRef` kind coverage expands under #267 | Cadence lookup degrades gracefully — an unresolvable kind publishes no cadence rather than failing the reconcile. |
| Consumers reading `ObservedAt` as "newest" break silently | Breaking-change marker + ADR + release notes (FR-012, SC-009); no automated gate exists to catch it. |
