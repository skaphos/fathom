# Quickstart: Validating Cadence-Aware Staleness

**Feature**: 007-clusterhealth-staleness-semantics | **Date**: 2026-08-23

How to prove the feature works. Scenario 1 is the regression that must fail
before the fix.

## Prerequisites

```sh
go -C tools tool task --list        # tooling is pinned; never invoke tools directly
```

envtest binaries are bootstrapped by the `test` task. The e2e scenario
additionally needs `kind`, `docker`, `helm`, and `helmfile` on `PATH`.

## Scenario 1 — A frozen child cannot hide behind a healthy sibling (US1)

**The regression test. Must fail before the change and pass after.**

Setup: one `ClusterHealth` selecting two `HealthCheck` children — child A live and
advancing, child B frozen at `Fail` with an observation held in the past.

| Assertion | Expected |
| --- | --- |
| `status.result` | `Fail` — unchanged, from the worst-of fold |
| `status.observedAt` | Equals **child B's** (frozen) observation, not child A's |
| `fathom_check_last_run_timestamp_seconds{kind="ClusterHealth"}` | Agrees with `status.observedAt` |

Before the change the second and third assertions fail: both report child A's
timestamp, so the aggregate looks perfectly current while promoting `Fail`.

```sh
go -C tools tool task test
```

## Scenario 2 — A healthy slow child does not poison its aggregate (US2)

Setup: an aggregate whose children wrap a 5-minute check and a 1-hour check, both
healthy and observed within their own cadence.

| Assertion | Expected |
| --- | --- |
| `fathom_check_interval_seconds` for the aggregate | The **maximum** child cadence (3600) |
| The shipped staleness rule | Does **not** fire |

The failure this guards against is a fix that satisfies Scenario 1 by taking the
oldest child unconditionally — which would mark every mixed-cadence aggregate
permanently stale.

## Scenario 3 — One rule is correct at every cadence (US3)

Setup: an `AddonCheck` on its 5-minute default and a `NodeCertificateCheck` on its
1-hour default, both healthy; then freeze only the `AddonCheck`.

| Assertion | Expected |
| --- | --- |
| `fathom_check_interval_seconds` | `300` and `3600` respectively |
| Cadence-relative rule, both healthy | No alert |
| Cadence-relative rule, `AddonCheck` frozen | Fires for that check only |
| Any hardcoded threshold | None remains in the shipped rules |

Against the old `> 900` rule the healthy hourly check false-positives
continuously — that is the live bug this scenario retires.

```sh
go -C tools tool task verify-alert-rules
```

## Scenario 4 — Never-observed and clock skew

| Case | Expected |
| --- | --- |
| A selected child with no observation | Aggregate `observedAt` is `nil`; gauge reads the `0` sentinel; the staleness rule fires |
| A child reporting a future observation | Aggregate never reads as more current than now |
| No children match the selector | Existing `NoMatches` behavior, unchanged — not additionally reported stale |

## Scenario 5 — Truncation never hides the failure

Setup: an aggregate selecting more children than the cap, with the single failing
and the single frozen child placed where a naive alphabetical truncation would
drop them.

| Assertion | Expected |
| --- | --- |
| `status.result` | Computed from **all** children, not the truncated list |
| `status.observedAt` | Computed from **all** children |
| `status.matchedCount` | The full pre-truncation total |
| `len(status.children)` | At the cap |
| The failing and frozen children | Present in `children` — they sort first |
| Status write | Succeeds; reconciliation is not wedged |

Also verify an object stored **before** the cap existed, holding more children
than the maximum, reconciles to compliance on its next write rather than failing
validation.

## Full gate

```sh
go -C tools tool task ci                 # lint, test, staticcheck, vuln, build
go -C tools tool task verify-generated   # CRD + API reference must not be stale
go -C tools tool task crd-compat         # expects the sanctioned MaxItems allowlist entry
go -C tools tool task test-e2e           # REQUIRED: touches internal/controller/*
```

`test-e2e` rewrites `config/manager/kustomization.yaml` as a side effect — revert
it rather than committing it.

## Documentation checks

- `docs/guides/monitoring.md` no longer claims the aggregate carries "the freshest
  of its children", and describes the signal in **staleness** terms (D3).
- The CRD godoc and the gauge-emission comment agree with the implementation and
  with each other — all four descriptions reconciled (FR-010).
- `docs/reference/api.md` is regenerated, never hand-edited.
- ADR-0005 records the semantic redefinition and why a parallel field was rejected.
