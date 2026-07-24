# Data Model: Quorum/Ratio Semantics for Managed-Resource Rollups

No CRD schema changes. All new data rides existing string-typed surfaces.

## RatioThresholds (in-memory, `pkg/adapter`)

Parsed from `FamilyPolicy.Thresholds` (reserved keys).

| Field | Source key | Type | Constraints |
|-------|-----------|------|-------------|
| WarnRatio | `warnRatio` | optional fixed-point percent | decimal in `[0, 100]`, optional trailing `%`; absent when key unset |
| FailRatio | `failRatio` | optional fixed-point percent | decimal in `[0, 100]`, optional trailing `%`; absent when key unset |

- `Configured()` is true when at least one field is set — the gate for ratio
  evaluation; families where both are unset take the untouched worst-of path.
- Parse errors are *controller-side validation errors*, never silently
  defaulted (FR-009). Values are stored fixed-point (e.g. hundredths of a
  percent) so comparisons are integer-exact.

## Family population classes (derived per run, per family)

From that family's `[]CheckResult`:

| Class | Definition | Role |
|-------|------------|------|
| population | outcomes in {Pass, Warn, Fail} | ratio denominator (FR-005: Skipped excluded) |
| unhealthy | outcomes = Fail | numerator for `failRatio` |
| degraded | outcomes in {Warn, Fail} | numerator for `warnRatio` |
| error-present | any outcome = Error | short-circuit flag (FR-004) |

## Family verdict derivation (state transitions)

```
any Error in family            → Error      (ratios not evaluated)
population == 0                → Pass
failRatio set  && unhealthy·100 > failRatio·population → Fail
failRatio unset && unhealthy > 0                       → Fail   (worst-of fallback)
warnRatio set  && degraded·100  > warnRatio·population → Warn
warnRatio unset && degraded > unhealthy(=any Warn)     → Warn   (worst-of fallback)
otherwise                                              → Pass
```

All comparisons strict-exceed; `"0"` therefore reproduces worst-of exactly.

## Rollup report entry (persisted, `HealthReport.spec.checks[]`)

One synthetic `HealthReportCheck` per ratio-evaluated family (only when
`Configured()`), alongside the untouched per-resource entries:

| Field | Value |
|-------|-------|
| `family` | the family name |
| `result` | the family verdict (Pass/Warn/Fail/Error) |
| `targetRef` | the driving AddonCheck |
| `summary` | e.g. `ratio rollup: 3 unhealthy, 3 degraded of 200 evaluated, failRatio 5 -> Pass` |
| `details.rollup` | literal `"ratio"` (discriminator for consumers) |
| `details.population` / `details.unhealthy` / `details.degraded` | decimal counts |
| `details.warnRatio` / `details.failRatio` | configured values, verbatim; omitted when unset |

## Overall aggregate (existing field, changed derivation)

`HealthReport.spec.result` = `WorstResult` over: ratio-evaluated families'
verdicts + every other check's raw outcome. With no ratio thresholds anywhere
this is exactly today's fold (SC-002). `AddonCheck.status.lastResult` mirrors
it unchanged; `ClusterHealth` derivation is untouched (FR-011).

## Validation rules (Accepted condition)

Extends `validateAddonCheckPolicy`:

| Input | Outcome |
|-------|---------|
| `warnRatio`/`failRatio` parseable, in range | valid; keys exempt from `unknownThresholdKeys` |
| unparsable value (`"banana"`) | `Accepted=False`, reason `InvalidPolicy`, message names family + key |
| out of range (`"150"`, `"-1"`) | same as above |
| reserved keys on any family | never reported as unknown keys, regardless of `ThresholdAdvertiser` |
