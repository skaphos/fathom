<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# fathomctl Reference

`fathomctl` is the command-line client for the Fathom operator. It reads the
status the operator writes and drives the on-demand run trigger the operator
already honours; it never bypasses the operator. For a task-oriented
introduction see the [fathomctl guide](../guides/fathomctl.md).

## Global flags

| Flag | Default | Notes |
| --- | --- | --- |
| `--kubeconfig <path>` | `$KUBECONFIG`, then `~/.kube/config`, then in-cluster | Same discovery as kubectl. |
| `--context <name>` | current context | |
| `-n, --namespace <ns>` | the context's namespace, else `default` | Usage error with `-A`. Ignored by cluster-scoped kinds. |
| `-A, --all-namespaces` | off | |
| `-o, --output table\|json\|yaml` | `table` | `json` and `yaml` emit the underlying resources unmodified. |
| `--request-timeout <duration>` | `30s` | Applied to every API request. |

There is no config file and no `FATHOM_*` environment mapping: a client tool
has no ConfigMap to mount, and the operator's configuration model is not
duplicated for a handful of flags.

## Kind names and aliases

Every verb that takes a check accepts `<kind>/<name>` or `<kind> <name>`.
The kind may be spelled as the Kind, its lowercase form, the plural resource
name, or a CLI alias:

| Kind | Resource | Alias | Scope | Runs itself? |
| --- | --- | --- | --- | --- |
| `AddonCheck` | `addonchecks` | `ac` | namespaced | yes |
| `DNSCheck` | `dnschecks` | `dns` | namespaced | yes |
| `NodeCertificateCheck` | `nodecertificatechecks` | `ncc` | namespaced | yes |
| `HealthCheck` | `healthchecks` | `hc` | namespaced | no (mirrors a source) |
| `ClusterHealth` | `clusterhealths` | `ch` | cluster | no (aggregates) |

The aliases are **CLI-only**. The CRDs declare no `shortNames`, so
`kubectl get ac` does not work; use the resource name with kubectl.

## Exit codes

`fathomctl` follows kubectl: `0` on success, `1` on any error. The kind of
failure (usage, permission, cluster, timeout, failing verdict) is distinguished
by the message on stderr, not by the code. For `run --wait`, success means
every trigger was accepted and every verdict is `Pass`, `Warn`, or `Skipped`;
`Fail`, `Error`, `Unknown`, a timeout, or a superseded trigger exit `1`. That is
what makes `run --wait` usable as a gate in CI.

## run

```text
fathomctl run (<kind>/<name> | --all | -l <selector>) [--wait] [--timeout <d>] [--yes] [--dry-run]
```

`run` writes a fresh token to the `fathom.skaphos.io/run-now` annotation of
every executable check it resolves. The token is the UTC request time plus a
short random suffix (`2026-09-07T18:04:05Z-7f3a1c`): unique across concurrent
invocations, sortable, readable in status, and carrying no caller identity.
The write names `fathomctl` as its field manager, so `managedFields` records
which tool wrote it; who ran it is the API audit log's job.

### Propagation from HealthCheck and ClusterHealth

Derived kinds have no work of their own to redo, so `run` triggers what they
observe:

- `run healthcheck/<name>` triggers the executable check named by
  `spec.checkRef`. A paused HealthCheck is refused: it would not mirror the
  fresh result.
- `run clusterhealth/<name>` triggers the source behind every HealthCheck in
  `status.children` (the operator's own record of selection, capped by the
  controller). A shared source is triggered once. A child that is missing,
  paused, or references an unsupported kind is reported and skipped; the rest
  proceed, and the skip makes the exit code `1`.

`--all` and `-l` select **executable kinds only**, within the `-n`/`-A`
scope; derived kinds are never selected this way.

### Bulk confirmation and --dry-run

Before writing anything, `run` resolves the full set and prints the count.
Above 10 checks it asks for confirmation on a terminal; without a terminal
and without `--yes` it aborts with exit `1` and writes nothing. `--yes`
skips the prompt. `--dry-run` prints the resolved set and exits `0` without
writing.

### --wait

With `--wait`, `run` polls each target every 2 seconds until
`status.lastRunTrigger` equals the token it wrote, which every controller
sets in the same status update as the run's verdict. The default timeout is
the largest target's effective `spec.timeout` plus 30 seconds; `--timeout`
overrides it. If the annotation changes to a different value before the
token is consumed, the run is reported as **superseded**. A timeout message
names the likely causes: an operator older than the CLI (compare with
`fathomctl version`), a paused check, or a node-agent rollout still in
progress.

A `NodeCertificateCheck` run restarts one node-agent pod per node before the
token can complete; see
[Forcing a scan](../guides/node-certificate-checks.md#forcing-a-scan) and
give `--wait` a `--timeout` that covers the rollout on large clusters.

### Output

Table output shows one row per target (`TARGET`, `STATUS`, or with `--wait`
`TARGET`, `VERDICT`, `SUMMARY`). `-o json` and `-o yaml` emit the per-target
outcomes:

```json
[
  {
    "target": "addoncheck/fathom-system/coredns",
    "via": "healthcheck/fathom-system/coredns",
    "token": "2026-09-07T18:04:05Z-7f3a1c",
    "triggered": true,
    "verdict": "Pass",
    "summary": "3 of 3 checks passed"
  }
]
```

`via` is present when the target was reached through a derived kind;
`skipped`, `error`, `superseded`, and `timedOut` explain a non-success.

## RBAC for fathomctl users

The operator's own role is unchanged by the CLI. Two auxiliary ClusterRoles
ship under `config/rbac/` for people and CI jobs:

| Role | Grants | Use it for |
| --- | --- | --- |
| `fathomctl-viewer-role` | `get`, `list`, `watch` on the five check kinds and `healthreports` (plus `/status`); `get`, `list` on `apps/deployments` | `ls`, `describe`, `reports`, `version` |
| `fathomctl-runner-role` | the viewer rules plus `patch` on `addonchecks`, `dnschecks`, `nodecertificatechecks` | everything, including `run` |

`run` writes only the trigger annotation, but Kubernetes RBAC cannot scope
`patch` to metadata, so the runner role also permits spec edits; bind it to
subjects trusted to trigger runs. The Deployment read serves only
`fathomctl version`; without it the CLI still works and reports the operator
as unavailable. See
[Operator RBAC](operator-rbac.md#auxiliary-roles-shipped-alongside-the-operator)
for where these sit relative to the operator's grants.

## On-demand trigger contract

The contract `run` drives is documented once for every kind in
[Status and conditions: On-demand runs](status-conditions.md#on-demand-runs).
