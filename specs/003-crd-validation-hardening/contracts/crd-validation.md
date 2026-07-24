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
| `thresholds: {warnDays: "banana"}` | **reject** (numeric) |
| `thresholds: {warnDays: "30"}` | accept |
| `thresholds: {failRatio: "1.5"}` | **reject** (ratio ∈ [0,1]) |
| `thresholds: {warnRatio: "0.9"}` | accept |
| `thresholds: {customKnob: "anything"}` | accept at admission (adapter judges at reconcile — MUST NOT be rejected for being unknown) |
| `matchExpressions` operator `Foo` | **reject** (operator enum) † |
| operator `In` with no `values` | **reject** † |
| operator `Exists` with `values: [x]` | **reject** † |
| valid selector | accept |
| well-formed family key unknown to the selected adapter | accept at admission; surfaces as `Accepted=False/InvalidPolicy` at reconcile (unchanged) |

† If the API server's CEL cost budget forces dropping the selector rule (R6
fallback), these three rows move to reconcile-time enforcement
(`Accepted=False/InvalidPolicy`) and the API reference must say so.

## Compatibility guarantee (FR-008)

Every manifest under `config/samples/` and every e2e fixture admits unchanged.
This is itself an asserted contract (envtest spec applying all samples).
