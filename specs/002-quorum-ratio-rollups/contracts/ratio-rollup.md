# Contract: Ratio Rollup Thresholds and Report Entries

The externally observable surface this feature adds. Everything here is
covered by SC-002's compatibility guarantee: absent the reserved keys, no
observable output changes.

## Configuration surface (AddonCheck)

Reserved, engine-level threshold keys, valid on **any** family of **any**
adapter:

```yaml
spec:
  policy:
    certificates:
      thresholds:
        warnRatio: "1"     # Warn when degraded (Warn+Fail) fraction > 1%
        failRatio: "5%"    # Fail when unhealthy (Fail) fraction > 5%
```

- Value grammar: non-negative decimal, optional trailing `%`, range 0–100.
  `"5"`, `"5%"`, `"2.5"` are equivalent forms of the same percentage.
- Comparisons are **strict-exceed** ("above"): `"0"` means any single
  degraded/unhealthy resource escalates — identical to worst-of.
- Either key may be set alone; the omitted level keeps worst-of semantics
  for the outcomes it governs.
- The keys are consumed by the engine, not the adapter: adapters never see
  them in their advertised-key validation, and `ThresholdAdvertiser`
  adapters do not need to list them.

### Rejection (Accepted condition)

An unparsable or out-of-range value sets `Accepted=False` with reason
`InvalidPolicy` on the AddonCheck, naming the family and key. The run is
gated exactly like today's invalid-policy cases — never silently ignored.

## Verdict semantics

Per family with ≥1 reserved key set, per run:

| Condition (evaluated top-down) | Family verdict |
|--------------------------------|----------------|
| any check outcome is Error | Error (ratios skipped) |
| population (Pass+Warn+Fail) is 0 | Pass |
| `failRatio` set and Fail% > failRatio | Fail |
| `failRatio` unset and any Fail | Fail |
| `warnRatio` set and (Warn+Fail)% > warnRatio | Warn |
| `warnRatio` unset and any Warn | Warn |
| otherwise | Pass |

Skipped checks are excluded from the population and never escalate.

## Report surface (HealthReport)

Per ratio-evaluated family, one synthetic check entry is appended to
`spec.checks[]` (per-resource entries are unchanged):

```yaml
- family: certificates
  result: Pass
  targetRef: { apiVersion: skaphos.io/v1alpha1, kind: AddonCheck, ... }
  summary: 'ratio rollup: 1 unhealthy, 3 degraded of 200 evaluated, failRatio 5%, warnRatio 1 -> Pass'
  details:
    rollup: "ratio"          # discriminator — filter on this
    population: "200"
    unhealthy: "1"
    degraded: "3"
    warnRatio: "1"           # present only when configured
    failRatio: "5%"          # present only when configured
```

`spec.result` (and therefore `AddonCheck.status.lastResult`) folds
ratio-family verdicts with all other families' raw outcomes via the existing
worst-of fold.

## Metrics interplay (informative)

- `fathom_adapter_run_duration_seconds{outcome=...}` keeps the **raw**
  worst-of family observation (what the adapter saw).
- `fathom_check_result` (feature 001) reflects `status.lastResult` and is
  therefore **ratio-adjusted** — alert rules keying on it get the policy
  verdict.

## Explicit non-changes

- `ClusterHealth` derivation: untouched.
- Per-resource check entries in HealthReport: byte-identical.
- Adapters: no contract bump; `Request.Policy` passes thresholds through
  unchanged and adapters ignore keys they don't consume.
