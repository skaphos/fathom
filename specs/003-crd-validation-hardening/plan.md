# Implementation Plan: Pre-1.0 CRD Validation Hardening

**Branch**: `feature/152-crd-validation-hardening` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/003-crd-validation-hardening/spec.md`

## Summary

Tighten the v1alpha1 schema while churn is still sanctioned (issue #152), in
three slices: (1) CEL admission floors on `spec.interval` (≥ 10s) and
`spec.timeout` (≥ 1s) for AddonCheck and NodeCertificateCheck, plus a
controller-side clamp surfaced via warning Event and status condition;
(2) structural admission validation of `AddonCheck.spec.policy` — bounded map
sizes, family-key format, DNS-1123 namespace entries, numeric threshold
values for the known numeric keys, and label-selector structure — expressed
as kubebuilder markers and CEL rules on the CRD itself, with the existing
reconcile-time `validateAddonCheckPolicy` kept as the semantic backstop;
(3) a CRD schema-compatibility CI gate that diffs `config/crd/bases` against
the latest release tag using pinned `sigs.k8s.io/crdify`, with a committed
allowlist file as the auditable override for sanctioned breaking changes.

## Technical Context

**Language/Version**: Go (per `go.mod`), kubebuilder v4 markers, controller-gen v0.21 (pinned in `tools/`)

**Primary Dependencies**: controller-runtime; CEL via `+kubebuilder:validation:XValidation` (no new runtime deps); new pinned tool `sigs.k8s.io/crdify` v0.6.0 in `tools/go.mod` for the CI gate

**Storage**: N/A (CRD schemas in etcd via the API server; no operator storage changes)

**Testing**: envtest (Ginkgo v2 + Gomega) for admission accept/reject matrices — envtest applies the real generated CRDs, so the API server enforces the CEL rules under test; stdlib table tests for clamp helpers; fixture-driven shell test for the gate script; full `test-e2e` run required (CRD types change, per AGENTS.md)

**Target Platform**: Kubernetes clusters ≥ the version pinned by `ENVTEST_K8S_VERSION` / kindest node (CEL validation rules and CEL duration functions are GA there)

**Project Type**: Kubernetes operator (single Go module + `tools/` module + CI scripts)

**Performance Goals**: No runtime hot paths touched; CEL rules must fit the API server's per-CRD cost budget (bounded via MaxProperties/MaxItems/MaxLength so cost estimation succeeds)

**Constraints**: All previously-valid manifests (samples, e2e fixtures) must still admit — notably existing family keys contain underscores (`api_availability`), so the family-key pattern must be `^[a-z0-9]([a-z0-9_-]*[a-z0-9])?$`-shaped, not DNS-1123; generated files only via tasks; no webhook

**Scale/Scope**: 2 CRD types touched (AddonCheck, NodeCertificateCheck); ~6 CEL rules + ~8 structural markers; 2 controller clamp seams; 1 script + 1 Taskfile task + 1 CI job; docs regeneration

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.0.0 — PASS (pre-Phase-0 and re-checked post-Phase-1). No violations; Complexity Tracking empty.*

| Principle / Constraint | Verdict | Notes |
|---|---|---|
| I. Explicit state over implicit behavior | PASS | Validation intent moves into the CRD schema itself — declared, not tribal; floors become documented API constants |
| II. Git is the durable desired-state boundary | PASS | Override allowlist is a committed, reviewed file (clarified 2026-07-23); no out-of-band gate state |
| III. Deterministic, reconstructible operation | PASS | CRDs regenerated only via pinned controller-gen; crdify pinned in `tools/go.mod`; gate compares deterministic generated artifacts |
| IV. Kubernetes-native, never obscured | PASS | Admission via CRD CEL (API-server enforced); clamp surfaced via status conditions + Events, the native channels |
| V. Compose, don't trap | PASS | No new cross-tool dependencies |
| VI. Explainable reconciliation, evidence-grade audit | PASS | Clamp carries field/configured/effective values in Event + condition; gate output names CRD/field/change; rejections carry actionable messages |
| VII. Read-only degradation over blindness | PASS | Clamped checks keep running (degraded-but-visible), never blank |
| VIII. Topology is deployment state | N/A | No topology surface touched |
| IX. Technical precision, honest scope | PASS | api-versioning.md "recommended" wording updated to describe the gate that now exists; docs state what CEL cannot express (semantic family validation stays reconcile-time) |
| ClusterHealth contract stability | PASS | Untouched |
| Bounded, idempotent reconciliation | PASS | Strengthened — floors/clamp bound reconcile cadence by construction |
| Minimal RBAC | PASS | No RBAC changes |
| Configuration model | PASS | Floors are API-contract constants in `api/v1alpha1`, not runtime config (a configurable floor would let deployments disagree with the published schema contract) |
| CRD API versioning standard | PASS (sanctioned) | Tightened validation is an incompatible change to v1alpha1 — explicitly sanctioned alpha churn per the standard and issue #152; the gate itself lands with the allowlist seeded for this feature's own changes |

## Project Structure

### Documentation (this feature)

```text
specs/003-crd-validation-hardening/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── crd-validation.md      # Admission accept/reject contract per field
│   ├── clamp-signal.md        # Event + condition contract for the runtime clamp
│   └── schema-compat-gate.md  # Gate script, allowlist format, CI wiring
└── tasks.md             # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
api/v1alpha1/
├── addoncheck_types.go            # floor CEL, policy bounds/CEL, floor constants
├── nodecertificatecheck_types.go  # floor CEL
└── (zz_generated.deepcopy.go — regenerated, never hand-edited)

config/crd/bases/                  # regenerated via `task manifests`
config/samples/                    # verify all still admit (FR-008)

internal/controller/
├── addoncheck_controller.go       # clamp in addonCheckInterval/addonCheckTimeout + Event/condition
├── nodecertificatecheck_controller.go  # clamp in nodeCertInterval/nodeCertTimeout + Event/condition
└── *_test.go / envtest suites     # admission matrices + clamp tests

scripts/
└── check-crd-compat.sh            # gate: baseline = latest release tag, crdify per CRD, allowlist filter

.crd-compat-allowlist.yaml         # committed override file (seeded with this feature's changes)
Taskfile.yml                       # new `crd-compat` task; crdify pinned via tools/go.mod
tools/go.mod                       # + sigs.k8s.io/crdify tool directive
.github/workflows/ci.yml           # new `crd-compat` job (fetch tags)
docs/reference/api-versioning.md   # gate is now implemented, not merely recommended
docs/reference/api.md              # regenerated via `task docs:api-ref`
test/e2e/                          # admission-rejection specs (optional thin layer; envtest is primary)
```

**Structure Decision**: Existing operator layout; no new packages. Validation
lives in `api/v1alpha1` markers (single source of truth → generated CRDs),
clamping in the two existing controllers next to the current
interval/timeout helpers, and the gate entirely in `scripts/` + `tools/` +
CI, mirroring `check-version-lockstep.sh` precedent.

## Complexity Tracking

*No constitution violations — table intentionally empty.*
