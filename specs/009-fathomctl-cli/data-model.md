<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Data Model: fathomctl CLI

## CLI-side types (`internal/cli`, unexported)

### GlobalOptions

Resolved once from the persistent flags; every verb reads it through the
client factory.

| Field | Source | Default | Rules |
|---|---|---|---|
| Kubeconfig | `--kubeconfig` | `""` (clientcmd discovery) | Path; passed as the explicit loading path |
| Context | `--context` | `""` (current context) | Kubeconfig context name |
| Namespace | `-n/--namespace` | `""` → context namespace, then `default` | Mutually exclusive with AllNamespaces |
| AllNamespaces | `-A/--all-namespaces` | `false` | |
| Output | `-o/--output` | `table` | One of `table`, `json`, `yaml`; parse-time validation |
| RequestTimeout | `--request-timeout` | `30s` | ≥ 0; applied to `rest.Config.Timeout` |

### kindDescriptor

One row per supported kind; the only place kind-specific knowledge lives.

| Field | Meaning |
|---|---|
| Kind, Resource, Aliases | `DNSCheck`, `dnschecks`, `{"dns"}` etc. Matching is case-insensitive over Kind, Resource, and Aliases |
| Namespaced | `false` only for ClusterHealth |
| Executable | `true` for AddonCheck, DNSCheck, NodeCertificateCheck |
| WritesReports | same set as Executable |
| New / NewList | constructors for the typed object and list |
| Paused(obj) | reads `spec.paused`; always `false` for DNSCheck and ClusterHealth |
| Snapshot(obj) | the normalisation in research R4 |
| DefaultTimeout(obj) | effective `spec.timeout` (schema default or exported constant) |
| Sources(ctx, client, obj) | derived kinds only: resolves executable targets (R7) |

Alias table:

| Kind | Resource | Aliases |
|---|---|---|
| AddonCheck | addonchecks | ac |
| DNSCheck | dnschecks | dns |
| NodeCertificateCheck | nodecertificatechecks | ncc |
| HealthCheck | healthchecks | hc |
| ClusterHealth | clusterhealths | ch |

### checkRef

A parsed target: `Kind` (descriptor), `Namespace`, `Name`. Produced from
`<kind>/<name>` or `<kind> <name>`; namespace comes from the global option
unless the kind is cluster-scoped.

### snapshot

The normalised verdict shared by `ls`, `describe`, and `run --wait`.

| Field | Type | Notes |
|---|---|---|
| Verdict | `HealthReportResult` | empty when the check has never run |
| Summary | string | bounded by the source field; truncated to the column width in tables only |
| LastRun | `*time.Time` | rendered as age (`5m`) in tables |
| NextRun | `*time.Time` | LastRun + interval; nil when no interval |
| ReportName | string | `status.lastReportName` |
| ConsumedTrigger | string | `status.lastRunTrigger`; empty for derived kinds |

### runTarget and runOutcome

`runTarget` is a resolved executable `checkRef` plus the `HealthCheck` or
`ClusterHealth` it was reached through (empty when addressed directly).
Targets are de-duplicated by (kind, namespace, name) before counting.

`runOutcome` per target: `Token`, `Written bool`, `Error string`, and after
`--wait` a `snapshot` plus `Superseded bool` and `TimedOut bool`. The
command's exit is `0` only if every outcome was written and, under `--wait`,
completed with a non-failing verdict.

### trigger token

`<RFC3339 UTC seconds>-<6 lowercase hex chars>`, e.g.
`2026-09-07T18:04:05Z-7f3a1c`. One token per `run` invocation, written to
every target in the invocation. Length is fixed at 27 characters, well under
the 253-character status bound.

### reportRow

For `reports` tables: `Name`, `ObservedAt`, `Result`, `Summary`, `Change`.
`Change` is one of `first`, `unchanged`, `<old>→<new>`, or
`<n> check(s) changed` computed against the next-older report.

### versionInfo

```
client:   string            # Version ldflag, else module version, else "devel"
operator: {version, namespace, deployment, image, error}   # error set ⇒ unavailable
```

## API changes (`api/v1alpha1`)

| Change | Kind | Compatibility |
|---|---|---|
| `AnnotationRunNow` constant | all | new exported constant, no schema change |
| `LastRunTrigger string` (`+optional`, MaxLength 253) | NodeCertificateCheckStatus | additive; same doc comment as AddonCheck/DNSCheck |
| Exported default cadence constants (`DefaultAddonCheckInterval`, `DefaultNodeCertificateCheckInterval`, per-kind default timeouts) | package | refactor; controllers switch to the exported names |

`crd-compat` must report the status addition as compatible; no allowlist
entry is expected.

## Wire contract change (`internal/nodecert`)

`NodeReport.Trigger string` (`json:"trigger,omitempty"`): the run-now token
the agent was started with, read from the `FATHOM_RUN_TRIGGER` environment
variable. Empty for routine ticks and for agents predating the field, so the
operator treats an empty value as "not this trigger" and older agents never
falsely complete a wait.

## Operator state transitions

### Executable checks (all three kinds)

```
annotation run-now = T, status.lastRunTrigger ≠ T
        │ reconcile
        ▼
   run performed (AddonCheck/DNSCheck) │ DaemonSet template stamped with T (NodeCertificateCheck)
        │ result written                │ every desired node reports trigger == T
        ▼                               ▼
   status.lastRunTrigger = T   (same status update as the verdict)
        │
        ▼
   periodic reconciles: annotation == lastRunTrigger ⇒ not due on that account;
   a run with no annotation never clears lastRunTrigger
```

NodeCertificateCheck while the rollout is in flight keeps its previous
verdict and `lastRunTrigger`; the `RolledOut` condition already reports the
rollout.

### CLI wait loop

```
write T ──► poll every 2s ──► annotation ≠ T ? ──yes──► superseded (exit 1)
                 │                 no
                 ▼
        status.lastRunTrigger == T ? ──yes──► snapshot → verdict → exit 0/1
                 │ no
                 ▼
        deadline reached ? ──yes──► timed out (exit 1, hint: operator version, paused, rollout)
                 │ no
                 └──────────── loop
```

## Release artifacts

| Artifact | Name |
|---|---|
| Archive | `fathomctl_<ver>_<os>_<arch>.tar.gz` (`.zip` for windows), containing `fathomctl[.exe]` and `LICENSE` |
| Checksums | `fathomctl_<ver>_checksums.txt` (sha256, one line per archive) |
| Signature bundle | `fathomctl_<ver>_checksums.txt.sigstore.json` (cosign keyless, bundle format) |
| Provenance | SLSA attestation over each archive via `actions/attest-build-provenance` |

`<ver>` is the release tag without the leading `v`; the binary reports
`v<ver>`.
