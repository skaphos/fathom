# Implementation Plan: DNSCheck Resource Contract

**Branch**: `feature/265-dnscheck-crd-type` | **Date**: 2026-08-08 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/005-dnscheck-resource-contract/spec.md`

## Summary

Add the `DNSCheck` kind at `v1alpha1` — an operator declares which names must
(or must not) resolve, from which vantage points, with which expected answers —
and widen the probe's `dns` mode so every declared field is actually honored.

The two halves ship together because of FR-013: the published schema must not
name a capability that does not exist. Phase 0 changed the shape of the second
half substantially. The assumption going in was that explicit-resolver support
had to be built; it is **already implemented** as `Request.DNSNameservers`
(`internal/probe/pod.go:70-76`), in use by the nodelocaldns adapter. What is
actually missing is narrower than expected:

- **Probe binary**: record-kind selection, expected-answer matching, negative
  assertions. Standard library only — `net.Resolver` covers every kind, and
  the default `Host` kind *is* the existing path, so the shared consumer is
  untouched by construction.
- **Pod builder**: one `dnsPolicy: Default` case for the node vantage point.
  Cluster and explicit already work.
- **API**: the new kind, its validation, and its printer columns.

No controller. Nothing evaluates a `DNSCheck` on a cadence until #266.

## Technical Context

**Language/Version**: Go 1.26.5 (`go.mod`)

**Primary Dependencies**: kubebuilder v4 markers + `controller-gen`;
`sigs.k8s.io/controller-runtime`; `k8s.io/apimachinery` v0.36.3. **No new
dependency** — the probe stays standard-library-only (research R1)

**Storage**: Kubernetes API server (CRD); no external store

**Testing**: envtest + Ginkgo/Gomega for the admission matrix; stdlib `testing`
table tests for the probe

**Target Platform**: Kubernetes ≥ the version pinned by `ENVTEST_K8S_VERSION`;
probe runs as a distroless, non-root, read-only pod

**Project Type**: Kubernetes operator (single Go module)

**Performance Goals**: worst case 48 probe pods per evaluation (16 targets × 3
resolvers, research R6); CEL validation cost within the API server's per-CRD
budget (research R4)

**Constraints**: every string and list bounded (FR-018); no behavior change to
the existing shared probe path (FR-030); generated artifacts produced only by
pinned tasks

**Scale/Scope**: one new CRD (~4 Go types), ~6 new probe flags/branches, one
pod-builder case, one `PROJECT` entry

## Constitution Check

*GATE: evaluated before Phase 0, re-evaluated after Phase 1 design. Both passes
recorded.*

| Principle | Verdict | Basis |
|-----------|---------|-------|
| I. Explicit State Over Implicit Behavior | **Pass** | DNS health intent becomes a first-class declared object with lifecycle and status, rather than annotations or an adapter threshold |
| II. Git Is the Durable Desired-State Boundary | **Pass** | CRD and samples render from this repository via pinned tasks; GitOps-compatible |
| III. Deterministic, Reconstructible Operation | **Pass** | Deep-copy, CRD, samples, and API reference generated only by task wrappers; never hand-edited |
| IV. Kubernetes-Native, Never Obscured | **Pass** | Plain CRD plus API-server admission validation — no side-channel, no webhook. Validation is CRD-schema-level, the cheapest tier in the preference order |
| V. Compose, Don't Trap | **Pass** | No new cross-tool dependency; no dependency on any control plane that consumes Fathom |
| VI. Explainable Reconciliation, Evidence-Grade Audit | **Pass** | Per-target results carry the subject, the record kind, the vantage point, the answers returned, and the message that decided the verdict (FR-012, FR-022) |
| VII. Read-Only Degradation Over Blindness | **Pass** | Status is additive and readable regardless; an unreachable resolver is reported as state, never as a blank — and FR-014 forbids the specific blindness of reading it as proof |
| VIII. Topology Is Deployment State | **Pass, and strengthened** | Resolver vantage point (cluster / node / explicit upstream) is modelled as data, not reconstructed from convention. This is the principle most directly served: "where you resolved from" is deployment state |
| IX. Technical Precision, Honest Scope | **Pass — and the driving constraint** | FR-013 and SC-005 exist to enforce it. The whole reason the capability half is in this slice is that shipping the schema alone would overclaim |

**Fathom-specific constraints:**

| Constraint | Verdict | Basis |
|------------|---------|-------|
| `ClusterHealth` contract stability | **Pass** | Untouched. `DNSCheck` is not mirrorable until #267; nothing here reads `HealthReport` history |
| Bounded, idempotent reconciliation | **Pass** | No reconcile loop in this slice. The bound that *is* set here is the schema cap on targets × resolvers (research R6), which is what bounds the future loop |
| Minimal RBAC | **Pass — by adding none** | See below |
| Configuration model | **N/A** | No new operator options; nothing added to `Options`/`bindings()` |

**On RBAC — no grant is added in this slice, deliberately.** Issue #265's
acceptance list mentions regenerating RBAC, but nothing in the operator reads
or writes a `DNSCheck` until the controller lands. Adding
`+kubebuilder:rbac` markers now would grant a permission with no consumer,
which the minimal-RBAC constraint forbids and which YAGNI argues against
independently. The grant belongs with #266, together with its justification row
in `docs/reference/operator-rbac.md`. Regenerating RBAC here is therefore a
no-op, and that is the correct outcome rather than an oversight.

**Post-Phase-1 re-evaluation**: no verdict changed. The design added no
webhook, no new dependency, no cluster-wide permission, and no new
configuration surface. The one design decision that touched a shared code path
(`internal/probe.Request`) is governed by FR-030 and carries explicit
regression coverage in [quickstart.md](quickstart.md) step 5.

## Project Structure

### Documentation (this feature)

```text
specs/005-dnscheck-resource-contract/
├── spec.md                        # Feature specification
├── plan.md                        # This file
├── research.md                    # Phase 0 — 8 decisions, all unknowns resolved
├── data-model.md                  # Phase 1 — entities, fields, bounds, rules
├── quickstart.md                  # Phase 1 — how to validate both halves
├── checklists/
│   └── requirements.md            # Spec quality checklist (passing)
├── contracts/
│   ├── dnscheck-admission.md      # 34-row accept/reject matrix
│   └── probe-dns-cli.md           # Probe dns-mode CLI + outcome mapping
└── tasks.md                       # Phase 2 — created by /speckit-tasks
```

### Source Code (repository root)

```text
api/v1alpha1/
├── dnscheck_types.go              # NEW — DNSCheck, Spec, Status, DNSTarget,
│                                  #       DNSResolver, DNSTargetResult, enums
├── dnscheck_validation_test.go    # NEW — envtest admission matrix (34 rows)
├── zz_generated.deepcopy.go       # regenerated, never hand-edited
└── deepcopy_test.go               # extended — fullyPopulatedDNSCheck

cmd/probe/
├── main.go                        # MODIFIED — record kinds, expected answers,
│                                  #            negative assertions
└── main_test.go                   # extended — per-kind + polarity matrix

internal/probe/
├── pod.go                         # MODIFIED — DNSFrom vantage selector,
│                                  #            dnsPolicy: Default for Node
└── pod_test.go                    # extended — incl. FR-030 regression

config/
├── crd/bases/fathom.skaphos.io_dnschecks.yaml   # generated
└── samples/fathom_v1alpha1_dnscheck.yaml        # generated

docs/reference/api.md              # generated via task docs:api-ref
PROJECT                            # one new resource entry
```

**Structure Decision**: standard kubebuilder layout, unchanged. The new kind
joins the existing five in `api/v1alpha1` — physical package location is
independent of the version track (research R8). The probe changes extend the
existing `dns` mode rather than adding a mode, so `internal/probe` keeps its
single shared path and its existing consumers.

## Implementation sequence

Ordering is driven by which failures are cheapest to discover early.

1. **Types + markers** — `dnscheck_types.go` with every bound and rule.
2. **Generate and install (early gate)** — `task manifests generate` then
   `task install`. The CEL cost budget is rejected at install time, so this
   runs *before* the matrix is written rather than after (quickstart step 2).
   If it fails, the fix is tighter bounds, and tighter bounds change the types.
3. **Admission matrix** — the 34 rows in `contracts/dnscheck-admission.md`.
4. **Probe capability** — record kinds, matching, polarity, with the two
   research-R1 traps covered by name.
5. **Pod builder** — `DNSFrom` selector; FR-030 regression first.
6. **Regenerate the rest** — samples, `docs/reference/api.md`, `crd-compat`.

Steps 4 and 5 are independent of 1–3 and can proceed in parallel.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| CEL cost exceeds the per-CRD budget | Medium — this repo has hit it before | Gate at step 2, before tests depend on the rules; tighten `MaxItems`/`MaxLength`, which reduces estimated cost directly (research R4) |
| FR-030 regression on the shared probe path | Medium — live consumer in `nodelocaldns` | Default path keeps `LookupHost` semantics (both families); regression test plus scoped e2e before any other probe change |
| `LookupCNAME` / `LookupSRV` misuse | High if unguarded — both compile and return wrong answers | Named test cases (research R1, quickstart step 4); called out in the CLI contract |
| Probe-pod churn at the bounds | Low at 48 worst case | Bounded at the schema; batching recorded as the next move with its trigger (research R6) |
| `isIP()` unavailable on the envtest API server | Low | Regex fallback delivers the same matrix; the matrix is the contract, not the mechanism |

## Decisions resolved during planning

**The default record kind — resolved: add `Host`, default to it.**

Defaulting `recordType` to `A` would have meant a default check verifies IPv4
only, failing an AAAA-only name for a reason its author would not expect. It
would also have left two subtly different defaults in the system: the resource
defaulting to `A` (IPv4) while the probe's existing no-flag path is
`LookupHost` (either family).

`Host` is now the sixth enum value and the default on both sides, so the CRD
default and the probe default are one behaviour. `A` and `AAAA` narrow to a
single family only when named explicitly.

This also turns FR-030 from a constraint that must be defended into one that
falls out of the design: the existing `nodelocaldns` caller passes no record
type, receives `Host`, and behaves exactly as before. Had `A` been the default,
that caller would have silently narrowed to IPv4 — precisely the regression
FR-030 forbids.

Propagated to `spec.md` (FR-003, Clarifications, Assumptions),
`data-model.md`, and both contracts. No open decisions remain.

## Complexity Tracking

No constitution violations require justification. One scope note, recorded for
traceability rather than as a deviation:

| Item | Why | Note |
|------|-----|------|
| Slice spans two tracker issues | FR-013 forbids publishing a field the evaluation path ignores, and `spec.resolvers[]` was in the same position as the record kinds | Boundary restated on #265 and #266; the reconciler remains wholly in #266 |
</content>
