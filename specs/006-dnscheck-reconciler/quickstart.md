# Quickstart Validation: DNSCheck Reconciler

**Feature**: 006-dnscheck-reconciler · **Date**: 2026-08-09

How to prove this feature works end to end. Scenarios map to the spec's user
stories and success criteria; details live in
[contracts/](./contracts/) and [data-model.md](./data-model.md) rather than being
repeated here.

---

## Prerequisites

| Need | Check |
|---|---|
| Go toolchain | `go version` — `go.mod` requires 1.26.5 |
| Task runner | `go -C tools tool task --list` |
| envtest assets | bootstrapped by the `test` task into `bin/k8s/` |
| Kind + Docker | `kind version && docker info` — e2e only |
| mise shims on PATH | `~/.local/share/mise/shims` for `helmfile`/`yamllint` in background runs |

---

## Level 1 — unit, no cluster

The pure helpers in `internal/controller/dnscheck_plan.go` are table-testable
with no API server. Run first; they are the fastest signal on the arithmetic
that the clarify session decided.

```bash
go -C tools tool task test
```

| Assert | Requirement |
|---|---|
| Pair expansion: a target naming a resolver yields 1 pair; naming none yields one per vantage point; no resolvers at all yields a single `cluster` pair | FR-035, FR-007, FR-038 |
| Expansion at the caps yields exactly 48 pairs | SC-009 |
| Per-pair bound is `min(remaining, ceiling)` and strictly decreases across a run | FR-104, #150 |
| No pair receives zero or negative budget while budget remains | FR-104 |
| `RequeueAfter` = `interval − elapsed` with slack, and `minGap` without | FR-107, FR-107a, SC-108 |
| Truncation fold: 8 `Pass` + 40 `Unknown` → verdict `Unknown`, not `Pass` | FR-106 |
| Truncation fold: 1 `Fail` + 47 `Unknown` → verdict `Fail` | FR-106 |
| Negative-assertion failure summary names the polarity | FR-021 |

The 8-Pass/40-Unknown row is the one that matters most: it is the case where
choosing `Skipped` would have reported green on one-sixth of the evidence.

---

## Level 2 — envtest, real API server, faked launcher

Injecting a fake launcher keeps these hermetic while still exercising real
admission, status subresources, and owner references.

| Scenario | Assert | Requirement |
|---|---|---|
| Apply a check, let one cadence elapse | `status.lastResult` set, `observedTargets` = planned pairs, `observedGeneration` = generation | US1, FR-109 |
| One target resolvable, one not, both positive | verdict `Fail` — **not** `Error` | FR-025 |
| Launcher returns `LaunchError` for one pair | that pair `Error`; the others still evaluated | FR-103b |
| Remove a target, reconcile again | its `targetResults` entry gone; its series withdrawn | FR-036 |
| Stable verdict over several runs | exactly one `HealthReport`; `lastRunTime` still advances | FR-111, SC-103 |
| Verdict changes | exactly one new `HealthReport` + one `ResultChanged` event | FR-111, FR-112 |
| Fake launcher slower than the bound | some pairs `Unknown`, `Complete=False` naming the count | FR-106, FR-106a |
| Delete the check | check-level and per-target series both withdrawn | FR-114 |
| Probe pod built for a pair | carries an owner reference to the `DNSCheck` | FR-113, D1 |
| Concurrency cap = N | never more than N launcher calls in flight | FR-103a |

The in-flight assertion needs the fake launcher to record concurrent entries —
a counter incremented on entry and decremented on exit, with the observed maximum
asserted. A cap that is silently ignored would otherwise pass every other test
here.

---

## Level 3 — e2e on Kind

**Required** — `internal/controller/*` and `internal/probe/*` both change, which
AGENTS.md lists as mandating e2e. envtest cannot catch any of the following.

```bash
go -C tools tool task test-e2e
```

| Scenario | Assert | Requirement |
|---|---|---|
| `DNSCheck` in a namespace the operator does not own, naming `kubernetes.default.svc.cluster.local` | verdict `Pass`; the probe pod ran **in the check's namespace** | FR-031, SC-008 |
| Same, inspecting where queries originate | zero queries from any other namespace | SC-008 |
| Target under `.invalid` (RFC 6761, guaranteed non-resolvable), positive assertion | verdict `Fail`, not `Error` | FR-025 |
| Same target with `absent: true` | verdict `Pass` | FR-012 |
| Delete the check mid-run | pods removed by the owner-reference cascade | FR-114 |
| Kill the operator mid-run, restart | no unreclaimed pod survives the sweeper's period | FR-113, SC-102 |
| Namespace with `restricted` PodSecurity | probe pod admitted, or the failure surfaces as a per-pair `Error` with an actionable message | research risk 2 |
| Check with many pairs and a deliberately short bound | run truncates; `Complete=False`; **no** overrun past the bound | SC-101 |
| Measure per-pair cost at a realistic pair count | feeds the sizing guidance FR-104b requires | FR-104b, SC-107 |

The last row is a deliverable, not just an assertion: FR-104b requires publishing
the relationship between pair count and the bound needed to reach every pair, and
that number has to be measured rather than estimated. The 1–3s figure used
throughout planning is an estimate and must not ship as documentation.

---

## Level 4 — gates before the PR is ready

```bash
go -C tools tool task lint
go -C tools tool task ci
```

| Gate | Why it matters here |
|---|---|
| `verify-generated` | `config/rbac/role.yaml` must be regenerated from the new marker, never hand-edited |
| `crd-compat` | should be a no-op — the schema is frozen; a diff means something changed that should not have |
| `reuse lint` | new files need SPDX coverage |
| coverage gate | new packages must clear `COVERAGE_MIN_DEFAULT` |
| `helm:sync` | chart CRDs are generated and gated |

### Manual review items no gate catches

- The RBAC justification document exists and covers all six points in
  [contracts/metrics-and-events.md](./contracts/metrics-and-events.md#the-justification-document).
- The rendered ClusterRole grants `create;get;list;delete` on pods and **no**
  `watch`, no `pods/exec`, no `pods/log`, no `pods/portforward`.
- Documentation states plainly that a maximum-size check does not complete at
  the default bound (FR-104b) — honest scope, per constitution principle IX.
