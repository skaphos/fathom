# Research: Quorum/Ratio Semantics for Managed-Resource Rollups

All decisions below were grounded in the current code (paths cited) rather
than re-derived; no NEEDS CLARIFICATION markers remained in the spec.

## R1: Evaluation locus — controller aggregation, helpers in `pkg/adapter`

**Decision**: Implement ratio math as pure exported helpers in `pkg/adapter`
(`ParseRatioThresholds`, `FamilyRatioVerdict`) and apply them in exactly one
behavioral location: the AddonCheck controller's per-run aggregation
(`aggregateHealthReportResult` / `healthReportForAddonCheck` in
`internal/controller/addoncheck_controller.go:694-784`). Aggregation becomes
family-aware: group `Result.Checks` by family; a family with ratio thresholds
contributes its ratio verdict, any other family contributes its raw outcomes
exactly as today; fold with the existing `WorstResult`.

**Rationale**: FR-008 requires one implementation covering every adapter.
The controller is the single choke point downstream of all adapters —
hand-written (cert-manager, coredns, kube-state-metrics, node-local-dns) and
declarative alike — and already owns `spec.policy` and the report write. Pure
helpers in `pkg/adapter` keep the math contract-adjacent, independently
testable, and available to future consumers.

**Alternatives considered**:
- *Per-adapter evaluation inside `Run`*: rejected — N reimplementations,
  violates FR-008, and adapters would each need policy-aware rollup code.
- *Declarative engine only (`internal/adapter/declarative`)*: rejected —
  misses the four hand-written adapters, which include noise-prone
  multi-resource families (e.g. cert-manager certificates).
- *New evaluator package `internal/rollup`*: rejected — the types it consumes
  (`CheckResult`, `Family`, `FamilyPolicy`) all live in `pkg/adapter`; a
  separate package adds a layer without adding meaning.

## R2: Threshold surface — reserved keys `warnRatio` / `failRatio`

**Decision**: Two reserved, engine-level threshold keys on the existing
`spec.policy.<family>.thresholds` map:

- `failRatio` — family verdict is Fail when the *unhealthy* fraction strictly
  exceeds this percentage.
- `warnRatio` — otherwise Warn when the *degraded* fraction strictly exceeds
  this percentage.

Accepted value format: a non-negative decimal with an optional trailing `%`
(`"5"`, `"5%"`, `"2.5"`), range `[0, 100]`. The controller treats these keys
as universally known: `unknownThresholdKeys`
(`internal/controller/addoncheck_controller.go:633`) must skip them so
`ThresholdAdvertiser`-implementing adapters don't reject them, and
`validateAddonCheckPolicy` gains value validation for them (parse failure or
out-of-range → `Accepted=False/InvalidPolicy`, satisfying FR-009/SC-005).
Reserved keys are stripped from the policy handed to `Request.Policy`? — No:
they are passed through unchanged (adapters ignore unknown keys by contract),
which keeps `addonCheckPolicy` untouched and the change additive.

**Rationale**: The spec mandates riding the existing string-map surface
(Assumptions). Reserved engine-level keys need no CRD schema change and no
adapter contract bump; the `ThresholdAdvertiser` seam already establishes the
precedent that key vocabularies are validated centrally.

**Alternatives considered**:
- *Typed CRD fields (`warnRatioPercent int`)*: rejected for this iteration —
  schema churn while #152 is still designing CRD validation hardening; can be
  promoted to typed fields at a future API version without breaking anything
  (the string keys stay as the v1alpha1 contract).
- *Percent-free fractions (`"0.05"`)*: rejected — the issue and spec speak in
  percentages ("Fail above 5%"), and mixed conventions invite 100× errors.

## R3: Verdict semantics

**Decision**: For a family with at least one ratio threshold configured:

1. Partition that family's checks: `population` = checks with outcome
   Pass/Warn/Fail; Skipped is excluded; any Error present → family verdict is
   **Error**, ratios are not evaluated (FR-004, constitution VII).
2. `unhealthy` = count of Fail; `degraded` = count of Warn + Fail.
3. Verdict: **Fail** if `failRatio` set and `unhealthy/population × 100 >
   failRatio`; else **Warn** if `warnRatio` set and `degraded/population ×
   100 > warnRatio`; else the *omitted-level fallback*: an unset `failRatio`
   keeps worst-of for Fail (any Fail → Fail); an unset `warnRatio` keeps
   worst-of for Warn (any Warn → Warn); both set and neither exceeded →
   **Pass**.
4. Empty population → **Pass** (matches `FamilyOutcome`'s no-checks
   behavior).

Comparisons are strict (`>`): `failRatio: "0"` reproduces worst-of exactly
(spec Assumption "Above is strict"). To avoid float-boundary surprises the
comparison is implemented as integer cross-multiplication
(`unhealthy × 100 > threshold × population` with the threshold scaled to a
fixed-point integer), keeping the verdict fully deterministic (constitution
III).

**Rationale**: Directly encodes FR-002/FR-003/FR-004/FR-005 with the
smallest possible state machine; the fallback rule makes partially-configured
families behave predictably instead of silently passing.

**Alternatives considered**: counting Error into the population (rejected —
averages away adapter blindness); counting Warn into `unhealthy` (rejected —
a warning fleet could trip a Fail threshold, violating the spec's
"Warn-only degradation … can reach Warn but never Fail through ratios").

## R4: Explainability — synthetic rollup entry in HealthReport

**Decision**: For each ratio-evaluated family, append one synthetic
`HealthReportCheck` to `Spec.Checks`: `Family` = the family,
`Result` = the family's ratio verdict, `TargetRef` = the driving AddonCheck,
`Summary` = human-readable one-liner, and `Details`:

```
rollup:      "ratio"
population:  "<n>"
unhealthy:   "<n>"
degraded:    "<n>"
warnRatio:   "<as configured>"   (present only when set)
failRatio:   "<as configured>"   (present only when set)
```

The overall report `Result` is folded from family verdicts (ratio families)
plus raw outcomes (all other families). No entry is emitted for families
without ratio thresholds, so default users see byte-identical reports
(SC-002).

**Rationale**: FR-010 requires counts + thresholds in the persisted record.
Synthetic check entries are an established pattern — the adapter-error entry
(`addoncheck_controller.go:727-735`) and the version-gate check (SKA-527)
both do this — and require **no HealthReport CRD schema change**.

**Alternatives considered**: a new `HealthReport.spec.families` field
(rejected — CRD schema churn for data expressible in the existing checks
list; revisit if a structured consumer appears); events (rejected — events
are ephemeral, FR-010 says "from the persisted report alone").

## R5: Metrics interplay

**Decision**: The adapter-side per-family metric label
(`fathom_adapter_run_duration_seconds{outcome}` via `adapter.FamilyOutcome`,
e.g. `internal/adapter/certmanager/adapter.go:205-211`) keeps reporting the
**raw worst-of observation**. The ratio-adjusted verdict surfaces through the
existing result chain: HealthReport `Result` → `AddonCheck.status.lastResult`
→ `fathom_check_result` (feature 001 contract).

**Rationale**: The run-duration label measures what the adapter observed;
the check-result gauge measures the policy verdict. Making adapter metrics
policy-aware would require touching every adapter (violating FR-008's
"no per-adapter reimplementation") for no alerting benefit — alert rules key
on `fathom_check_result`. This split is documented in the contract file.

## R6: Test & e2e strategy

**Decision**:
- `pkg/adapter/ratio_test.go`: external `_test` package (repo convention),
  table-driven over parse formats, boundary equality, omitted-level
  fallbacks, Error short-circuit, Skipped exclusion, empty population.
- `internal/controller/addoncheck_controller_test.go`: unit tests for
  family-aware aggregation, rollup entry emission, reserved-key validation
  (Accepted=False on `failRatio: "banana"` / `"150"`), and the no-thresholds
  regression path (unchanged aggregation, no rollup entries).
- Full `go -C tools tool task test-e2e` before the PR is ready — this change
  touches `internal/controller/*` and `pkg/adapter/*`, both on the AGENTS.md
  "requires e2e" list; shared-surface changes warrant the full stack, not a
  scoped `E2E_ADDONS` run.

**Rationale**: The mock seams (fake adapter in controller tests) cover the
policy plumbing without envtest cost; e2e catches reconcile-time integration
per the SKA-422/SKA-423 lesson recorded in AGENTS.md.
