<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Contract: HealthCheck Target Projection

## Supported references

For this release, `HealthCheck.spec.checkRef` resolves these exact identities:

| Effective API version | Kind |
|---|---|
| `fathom.skaphos.io/v1alpha1` | `AddonCheck` |
| `fathom.skaphos.io/v1alpha1` | `DNSCheck` |
| `fathom.skaphos.io/v1alpha1` | `NodeCertificateCheck` |

An omitted API version means the value in the table. An omitted namespace
means the HealthCheck's namespace. Case is significant for every string.
The API-version value is limited to 317 characters and is not enumerated.

`NodeHealthCheck` and `ReachabilityCheck` are not part of this contract.

## Successful projection

After the target is read, the wrapper MUST replace its normalized snapshot
from one coherent source status read:

- `result` is the source `lastResult` using the existing shared enum;
- `sourceObservedAt` is the source `lastRunTime`;
- `lastReportName` is the source `lastReportName`, if any;
- `summary` is the source Ready-condition message, capped at 1,024 Unicode
  code points with the existing ellipsis behavior;
- `sourceInterval` is the source's effective defaulted and floor-clamped
  cadence; and
- Ready is `True` with reason `TargetMirrored`.

If the source exists but has never completed, optional source facts remain
empty. The wrapper MUST NOT manufacture `Unknown` or another verdict.

## Reference failures

| Failure | Ready reason | Snapshot | Retry |
|---|---|---|---|
| Unsupported nonempty API version | `UnsupportedAPIVersion` | Cleared | No error retry |
| Unsupported kind | `UnsupportedKind` | Cleared | No error retry |
| Target NotFound | `TargetNotFound` | Cleared | No error retry; source creation/update watch triggers reconciliation |
| Other target read error | `TargetLookupFailed` | Preserved | Return error to controller-runtime |
| Wrapper paused | `Paused` | Preserved | No target read |

Condition messages MUST identify the failing reference or supported contract
without exceeding the existing Kubernetes condition-message bound.

## Watch contract

Each supported concrete source type is watched with resource-version change
filtering. A source event enqueues every HealthCheck, and only HealthChecks,
whose normalized API version, kind, effective namespace, and name all match the
source. Wrapper namespace is the reconcile request namespace; it need not equal
an explicitly referenced source namespace.

Deletion is observable through the typed watch and leads to the NotFound
transition. An event for a different kind with the same name does not match.
An event for the current source does not match a wrapper that explicitly names
an unsupported API version.

## Aggregation boundary

ClusterHealth continues to watch and read HealthCheck only. It consumes the
existing normalized status and does not gain DNS-, certificate-, or
HealthReport-specific behavior. Specialized controllers retain sole ownership
of HealthReport creation and change-only history semantics.

## Compatibility

- Existing AddonCheck references with empty API version behave unchanged.
- `checkRef` remains immutable.
- No HealthCheck or ClusterHealth status field is added, removed, or retyped.
- Unsupported kinds remain admitted so future releases can add handlers
  without relaxing the CRD schema, but current reconciliation reports them
  truthfully as unsupported.
