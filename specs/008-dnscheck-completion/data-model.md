<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Data Model: DNSCheck Completion

This feature changes no aggregate status schema. It clarifies
`CheckTargetRef`, uses the three existing specialized statuses, and introduces
private controller-side normalization types.

## CheckTargetRef

Public immutable value embedded in `HealthCheck.spec`.

| Field | Required | Validation and normalization |
|---|---|---|
| `apiVersion` | No | Maximum 317 characters. Empty normalizes to `fathom.skaphos.io/v1alpha1`; a nonempty value is matched exactly. |
| `kind` | Yes | 1–63 characters; supported values are `AddonCheck`, `DNSCheck`, and `NodeCertificateCheck`. Unsupported values are admitted but rejected in status so future kinds do not require schema relaxation. |
| `name` | Yes | 1–253 characters; exact target object name. |
| `namespace` | No | At most 253 characters. Empty normalizes to the wrapper namespace; a nonempty namespace is exact. |

The entire `checkRef` remains immutable through the existing transition rule.

## Target identity

Private normalized identity used for handler selection and watch matching.

```text
TargetIdentity
├── apiVersion: normalized group/version
├── kind: exact specialized resource kind
├── namespace: effective namespace
└── name: exact object name
```

Identity equality requires all four fields. A source event carries its handler's
known API version and kind plus the object's namespace/name; it cannot match an
unsupported-version reference.

## Target handler

Private compiled-in descriptor, one per supported kind.

| Property | Purpose |
|---|---|
| API version | Registry key and watch-matching identity |
| Kind | Registry key, status explanation, and watch-matching identity |
| Object prototype | Concrete controller-runtime watch type |
| Read/project function | Performs one typed Get and returns a normalized snapshot |

The handler set is deterministic and contains exactly three entries. It is not
a runtime plugin mechanism.

## Normalized target snapshot

Private all-or-nothing projection result applied to existing
`HealthCheckStatus` fields.

| Snapshot field | Source | Destination |
|---|---|---|
| Result | `status.lastResult` | `HealthCheck.status.result` |
| Summary | Ready condition message, rune-truncated | `HealthCheck.status.summary` |
| Observed time | `status.lastRunTime` | `HealthCheck.status.sourceObservedAt` |
| Report name | `status.lastReportName` | `HealthCheck.status.lastReportName` |
| Effective interval | Existing kind-specific cadence helper | `HealthCheck.status.sourceInterval` and metrics |

An existing target with no completed run yields the source's empty optional
facts; the wrapper does not synthesize a result. Kind-specific fields such as
DNS answers or node counts are not projected.

## Source relationships

```text
AddonCheck ───────────────┐
DNSCheck ─────────────────┼─ one selected source → HealthCheck status
NodeCertificateCheck ─────┘                         │
                                                    ├─→ ClusterHealth roll-up
                                                    └─→ existing metrics/events

Specialized check ──creates on result change──→ HealthReport
ClusterHealth never reads HealthReport or specialized checks directly.
```

Each HealthCheck references exactly one source. Multiple HealthChecks may
reference the same source. A ClusterHealth may select zero or more wrappers
through its existing selector and namespace rules.

## State transitions

| Trigger | Ready condition | Snapshot action | Reconcile result |
|---|---|---|---|
| Supported target read succeeds | `True/TargetMirrored` | Replace all projected fields | Success |
| Unsupported API version | `False/UnsupportedAPIVersion` | Clear all projected fields | Terminal success |
| Unsupported kind | `False/UnsupportedKind` | Clear all projected fields | Terminal success |
| Target absent/deleted | `False/TargetNotFound` | Clear all projected fields | Terminal success |
| Target read has transient error | `False/TargetLookupFailed` | Preserve prior fields | Return error for retry |
| HealthCheck paused | `False/Paused` | Preserve prior fields | Success |
| Source resource version changes | No direct mutation | Enqueue exact referencing wrappers | Event-driven reconcile |

Status updates remain conditional on semantic difference, so repeated reads of
unchanged source status produce no write loop.
