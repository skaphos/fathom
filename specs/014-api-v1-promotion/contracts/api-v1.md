<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Contract: `fathom.skaphos.io/v1`

The stable API surface this feature freezes. Field-level detail is the
generated reference (`docs/reference/api.md`); this contract states the
promises and the admission behaviour that tests assert.

## Promises

1. `v1` is the only served and the only stored version of the seven kinds.
   A request at `fathom.skaphos.io/v1alpha1` for any of them is answered by
   the API server with "no matches for kind" after upgrade.
2. From the release that ships `v1`, no field is removed, renamed, retyped or
   re-meant, no validation is tightened, and no default changes its meaning
   under `v1`. Compatible additions remain allowed. The compat gate reports
   any violation as unsanctioned.
3. The seven kinds carry forward every alpha default, floor, immutability
   rule, scope rule and bound unchanged.
4. `ClusterHealth.status` is derived only from `HealthCheck.status`.

## Admission behaviour added at `v1`

| Case | Server response |
|---|---|
| object of any promoted kind without `spec` | rejected: `spec: Required value` |
| update replacing an object with one whose `spec` is absent | rejected (same rule), so `self == oldSelf` rules cannot be bypassed |
| `spec.paused` on AddonCheck, HealthCheck or NodeCertificateCheck | rejected: unknown field under strict field validation (`kubectl` default); pruned by the server otherwise |
| `HealthCheck.spec.checkRef.apiVersion` not `fathom.skaphos.io/v1` | rejected: unsupported value, accepted value listed |
| `HealthCheck.spec.checkRef.apiVersion` empty | accepted; the controller treats it as `fathom.skaphos.io/v1` |
| two server-side-apply managers each own one `AddonCheck` condition type | both conditions present after each apply |

## Default list columns

| Kind | Columns |
|---|---|
| AddonCheck | Result, Addon, Last Run, Age |
| HealthReport | Result, Source, Name, Observed, Age |
| others | unchanged from alpha |

## Labels and annotations the operator writes on HealthReport

| Key | Meaning |
|---|---|
| `fathom.skaphos.io/source-kind` | kind of the check that produced the report |
| `fathom.skaphos.io/source-name` | name of that check |
| `fathom.skaphos.io/source-uid` | UID of that check instance |
| `fathom.skaphos.io/source-deleted-at` | annotation; when the operator first found the check gone |

Reports carry no owner reference. See [retention.md](retention.md).

## Removed surface

- `spec.paused` on three kinds; the `Paused` condition on three kinds; the
  `AgentReady=False/Paused` reason.
- `fathomctl` no longer reports a `paused` outcome or prints a Paused field.

## Version axes

| Axis | Value at this release | Value at 1.0.0 | Relationship |
|---|---|---|---|
| operator release | 0.7.0 | 1.0.0 | SemVer per release-please |
| CRD version (seven kinds) | `v1` | `v1` | frozen from 0.7.0 |
| CRD version (definition kinds) | `v1alpha1` | `v1alpha1` | own track |
| adapter contract | 1.1.0 | 1.1.0 unless changed on its own merits | independent axis |
