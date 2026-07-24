# Data Model: Pre-1.0 CRD Validation Hardening

No new entities. This feature changes the *validation surface* of two existing
CRDs, adds two API constants, and introduces one committed configuration file
for CI. References: [research.md](research.md) decisions R2, R4–R6, R8.

## Changed: AddonCheck (`fathom.skaphos.io/v1alpha1`, namespaced)

### `spec` (type-level CEL, replacing the existing `> 0s` rules)

| Field | Rule | Error behavior |
|---|---|---|
| `spec.interval` | if set, `duration(self.interval) >= duration('10s')` | admission rejection naming field + minimum |
| `spec.timeout` | if set, `duration(self.timeout) >= duration('1s')` | admission rejection naming field + minimum |
| `spec.timeout` × `spec.interval` | existing `timeout <= interval` rule retained unchanged | unchanged |

### `spec.policy` (`map[string]AddonCheckFamilyPolicy`)

| Constraint | Mechanism | Value |
|---|---|---|
| Max families per policy | `MaxProperties` | 32 |
| Family-key format | CEL over keys | `^[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?$` (1–63 chars; underscores legal — existing families like `api_availability` must pass) |

### `spec.policy[*].namespaces` (`[]string`)

| Constraint | Mechanism | Value |
|---|---|---|
| Max entries | `MaxItems` | 64 |
| Entry length | `items:MaxLength` | 63 |
| Entry format | `items:Pattern` | DNS-1123 label: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` |

### `spec.policy[*].thresholds` (`map[string]ThresholdValue`)

*(As-built corrections: threshold keys are camelCase in the wild
(`warnDays`, `restartWarnCount`), so the key class allows uppercase; the
ratio keys are **percentages 0–100**, not decimals in [0,1] — that matches
`adapter.ParseRatioThresholds`, which was the authority all along; and the
value type is a named bounded string because the API server's CEL cost
estimator needs a `maxLength` on map values to price the shape rules.)*

| Constraint | Mechanism | Value |
|---|---|---|
| Max keys | `MaxProperties` | 16 |
| Key format | CEL over keys | `^[a-zA-Z0-9]([a-zA-Z0-9_-]{0,61}[a-zA-Z0-9])?$` |
| Value length | `MaxLength=64` on the named `ThresholdValue` string type | required by the CEL cost estimator |
| `warnDays`, `failDays` values | CEL | `^[0-9]{1,4}$` |
| `warnRatio`, `failRatio` values | CEL | percentage shape per parser grammar: `^[0-9]{1,3}([.][0-9]{1,2})?%?$` (range 0–100 stays with the parser) |
| any other key's value | none at admission | adapter-judged at reconcile (unchanged) |

### `spec.policy[*].labelSelector` (`*metav1.LabelSelector`)

*(As-built: the R6 fallback was taken — the CEL rule priced at 15.1x the
per-rule cost budget because the imported `metav1.LabelSelector` schema
carries no bounds. All selector validation is reconcile-time.)*

| Constraint | Mechanism | Value |
|---|---|---|
| selector structure (operators, values presence) | reconcile-time `LabelSelectorAsSelector` via `validateAddonCheckPolicy` → `Accepted=False/InvalidPolicy` | sole enforcement (R6 fallback taken; pinned by a regression spec asserting both halves of the split) |
| full label key/value grammar | reconcile-time (same path) | unchanged |

## Changed: NodeCertificateCheck (`fathom.skaphos.io/v1alpha1`, cluster-scoped)

| Field | Rule |
|---|---|
| `spec.interval` | if set, `>= 10s` (replaces `> 0s`) |
| `spec.timeout` | if set, `>= 1s` (replaces `> 0s`) |
| all other existing rules (paths allowlist, warnDays ≥ criticalDays, timeout ≤ interval) | retained unchanged |

## New: API constants (`api/v1alpha1`)

| Constant | Value | Consumers |
|---|---|---|
| `MinCheckInterval` | `10 * time.Second` | both controllers' clamp helpers; api-package test asserting CEL marker strings embed the same value |
| `MinCheckTimeout` | `1 * time.Second` | same |

## Changed: status semantics (no schema change)

| Surface | Addition |
|---|---|
| `Accepted` condition (both kinds) | new reason `SpecClamped`, status stays `True`; message format: `<field> <configured> is below the minimum <floor>; using <effective>` |
| Events (both kinds) | new Warning Event, reason `CadenceClamped`, same message |

State transitions: a clamped object is `Accepted=True/SpecClamped` while the
stored value is below the floor; any spec update passes the new admission
rules, so the transition out of `SpecClamped` is `Accepted=True/SpecAccepted`
on the first post-floor edit. No other condition flows change.

## New: `.crd-compat-allowlist.yaml` (repo root, committed)

```yaml
# Sanctioned CRD schema incompatibilities vs. the current release baseline.
# Every entry must cite a reason and a tracking issue. Entries that no longer
# match any gate finding are stale and produce a warning — prune them.
- crd: addonchecks.fathom.skaphos.io        # CRD metadata.name
  path: ^.spec.interval                      # crdify property path (or "*")
  reason: interval floor raised to 10s (sanctioned v1alpha1 churn)
  issue: https://github.com/skaphos/fathom/issues/152
```

| Field | Required | Meaning |
|---|---|---|
| `crd` | yes | CRD `metadata.name` the finding is on |
| `path` | yes | property path reported by crdify; `*` sanctions all findings on the CRD (discouraged; reserved for version-string bumps) |
| `reason` | yes | human rationale surfaced in gate output |
| `issue` | yes | tracking link |

Lifecycle: seeded with this feature's tightenings; matched entries are
reported as "sanctioned" in gate output; unmatched entries warn as stale;
pruned in the ordinary PR flow after each release advances the baseline.
