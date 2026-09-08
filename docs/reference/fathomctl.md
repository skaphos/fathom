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

Every verb that takes a check accepts `<kind>/<name>`, `<kind> <name>`, or
`<kind>/<namespace>/<name>`. The last form is what every verb prints, so any
reference in `fathomctl` output can be pasted back as an argument; an inline
namespace overrides `-n`. A namespaced check addressed under `-A` without an
inline namespace is an error (`-A` scopes listing, not a single target). The
kind may be spelled as the Kind, its lowercase form, the plural resource
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

## ls

```text
fathomctl ls [kind] [-l <selector>]
```

Lists checks with the verdict the operator last published. With no kind it
lists every kind, grouped in the order AddonCheck, DNSCheck,
NodeCertificateCheck, HealthCheck, ClusterHealth, with a `KIND` column.
ClusterHealth is cluster-scoped and is always included regardless of `-n`.
With a kind, only that kind is listed and the `KIND` column is dropped.

Columns: `NAMESPACE` (only with `-A`, blank for ClusterHealth), `NAME`,
`VERDICT` (`-` for a check that has never run), `SUMMARY` (truncated to 80
columns; `describe` has the full text), `LAST RUN` (age), `NEXT RUN` (time
until the next scheduled run, `now` when overdue, `-` when the kind has no
interval).

An empty result prints `No checks found in namespace <ns>.` (or `...in any
namespace.`) and exits `0`. `-o json` and `-o yaml` emit a `kind: List`
envelope whose items are the resources unmodified, with `apiVersion` and
`kind` set, so `fathomctl ls -A -o json | jq '.items[] | select(.status.lastResult=="Fail")'`
works the way it does with kubectl.

## describe

```text
fathomctl describe <kind>/<name>
```

Shows one check in full: identity, the spec that drives it (interval and
timeout as the controller applies them, marked `(default)` or `clamped`
when the spec is unset or below the floor), the normalised verdict, summary,
last and next run, the consumed run-now trigger, kind-specific status
(detected version and absent count for AddonCheck, observed targets for
DNSCheck, desired and reporting nodes for NodeCertificateCheck, matched count
for ClusterHealth), every condition with its reason and message, per-target
results for DNSCheck, each contributing HealthCheck for ClusterHealth, and a
`Latest report:` pointer into `fathomctl reports`.

A missing check is an error (`AddonCheck "x" not found in namespace y`),
exit `1`. `-o json` and `-o yaml` emit the resource unmodified.

## reports

```text
fathomctl reports <kind>/<name> [--limit N] [--since <duration>] [--report <name>]
```

Lists a check's HealthReport history newest-first with `NAME`, `OBSERVED`
(age), `RESULT`, a derived `SUMMARY`, and `CHANGE`: `first`, `unchanged`,
`<old>→<new>` when the verdict changed, or `<n> check(s) changed` when the
verdict held but per-check results moved. The change column is computed over
the full history, so the oldest row shown still compares to the report before
it.

Reports are written when a verdict changes, not on every interval; a gap is
not a missed run. `--limit` defaults to 10; `--since` keeps reports observed
within that window. `--report <name>` prints one report in full: source,
result, observation time, adapter, and every check with its target and
summary. The named report must belong to the addressed check; a report of
another check in the same namespace is refused rather than printed under the
wrong banner.

Only executable checks write reports. `reports healthcheck/<name>` follows
`spec.checkRef` to the source and says so on stderr; `reports
clusterhealth/<name>` fails and lists the sources to query. A check with no
history prints `No reports yet for <check>.` and exits `0`. `-o json` and
`-o yaml` emit a `List` of the reports unmodified (or the single report with
`--report`).

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
token is consumed, the run is reported as **superseded**. If the operator
reports it can never run the check (`Ready=False` with `InvalidPolicy`,
`MissingAdapter`, `AdapterLookupFailed`, `NoMatchingNodes`, or `Paused`),
`--wait` fails immediately with that reason instead of waiting out the
deadline. Transient API errors (rate limiting, a restarting API server) do
not end the wait; only a deleted check or a permission failure does. A
timeout message names the remaining likely causes: an operator older than
the CLI (compare with `fathomctl version`) or a node-agent rollout still in
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

## version

```text
fathomctl version [--client]
```

Prints the CLI version and, when a cluster is reachable and Fathom is
installed, the operator's version with the namespace and Deployment it was
read from:

```text
Client:   v0.6.0
Operator: v0.6.0 (fathom-system/fathom-controller-manager)
```

The operator is located by the `control-plane=controller-manager` label on
its Deployment (both the kustomize and Helm installs apply it), in `-n` when
given or across all namespaces otherwise, and confirmed to be Fathom's by its
`app.kubernetes.io/name` label or `fathom-operator` image. The version comes
from the `app.kubernetes.io/version` label (Helm sets it), else the manager
image tag, else a shortened image digest. Any failure, including no
kubeconfig or no permission to list Deployments, is reported as
`Operator: unavailable (<reason>)` with exit `0`. `--client` skips the
cluster entirely. `-o json` emits `{client, operator{version, namespace,
deployment, image, error}}`; `operator` is omitted with `--client`.

A locally built binary reports a `git describe` version; a release archive
reports the release tag.

## Version skew

Keep the CLI within one minor version of the operator. The read verbs only
depend on the resource schema, so they degrade gracefully. `run` depends on
the operator honouring the run-now trigger for the kind in question: an
operator older than the CLI that does not yet consume the trigger for a kind
never writes `status.lastRunTrigger`, so `run --wait` times out rather than
failing up front. The timeout message points at `fathomctl version` for that
reason.

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
