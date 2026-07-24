# Contract: CRD Admission Validation

The externally observable accept/reject behavior of the API server for
AddonCheck and NodeCertificateCheck after this feature. This is the contract
the envtest matrices assert (research R9) and the API reference documents.
Rule definitions: [data-model.md](../data-model.md).

## Cadence floors (both kinds)

| Manifest fragment | Verdict | Message must name |
|---|---|---|
| `interval: 1ms` | **reject** | `interval`, `10s` |
| `interval: 9s` | **reject** | `interval`, `10s` |
| `interval: 10s` | accept | — |
| `interval` unset | accept (default applies) | — |
| `timeout: 500ms` | **reject** | `timeout`, `1s` |
| `timeout: 999ms` | **reject** | `timeout`, `1s` |
| `timeout: 1s` | accept | — |
| `timeout: 5s`, `interval: 5m` | accept (fail-fast stays legal) | — |
| `timeout: 1s`, `interval: 10s` | accept (both at own floor) | — |
| `timeout: 30s`, `interval: 10s` | **reject** (existing cross-field rule) | timeout ≤ interval |
| `timeout` unset | accept (default applies) | — |

Applies identically on CREATE and UPDATE. Objects already stored below a
floor are not re-validated by the API server; they are the clamp contract's
domain ([clamp-signal.md](clamp-signal.md)).

## AddonCheck `spec.policy`

| Manifest fragment | Verdict |
|---|---|
| no `policy` / `policy: {}` | accept (adapter defaults, unchanged) |
| 33 family keys | **reject** (max 32) |
| family key `api_availability` | accept (underscores legal) |
| family key `Certificates` / `-bad` / `bad-` / 64 chars | **reject** (key format) |
| `namespaces` with 65 entries | **reject** (max 64) |
| `namespaces: [Prod_NS]` | **reject** (DNS-1123 label) |
| `namespaces: [kube-system]` | accept |
| namespace entry of 64 chars | **reject** (max 63) |
| `thresholds` with 17 keys | **reject** (max 16) |
| `thresholds: {warnDays: "banana"}` | **reject** (whole number required) |
| `thresholds: {warnDays: "30"}` | accept |
| `thresholds: {failRatio: "banana"}` | **reject** (percentage shape) |
| `thresholds: {warnRatio: "1000"}` | **reject** (percentage shape) |
| `thresholds: {warnRatio: "99.5"}`, `{failRatio: "80%"}`, `{failRatio: "1.5"}` | accept — ratios are **percentages 0–100** per `ParseRatioThresholds` (as-built correction: the draft's [0,1]-decimal assumption was wrong) |
| threshold value over 64 chars | **reject** (`ThresholdValue` MaxLength) |
| camelCase threshold keys (`restartWarnCount`) | accept (live convention) |
| `thresholds: {customKnob: "anything"}` | accept at admission (adapter judges at reconcile — MUST NOT be rejected for being unknown) |
| `matchExpressions` operator `Foo` | accept at admission; `Accepted=False/InvalidPolicy` at reconcile † |
| operator `In` with no `values` | accept at admission; `Accepted=False/InvalidPolicy` at reconcile † |
| operator `Exists` with `values: [x]` | accept at admission; `Accepted=False/InvalidPolicy` at reconcile † |
| valid selector | accept |
| well-formed family key unknown to the selected adapter | accept at admission; surfaces as `Accepted=False/InvalidPolicy` at reconcile (unchanged) |

† As built, the R6 fallback WAS taken: the selector CEL rule priced at 15.1x
the API server's per-rule cost budget (the imported `metav1.LabelSelector`
schema is unbounded), so selector structure is enforced at reconcile time.
A regression spec pins both halves of this split, and the README and field
docs state it.

## Compatibility guarantee (FR-008)

Every manifest under `config/samples/` and every e2e fixture admits unchanged.
This is itself an asserted contract (envtest spec applying all samples).
