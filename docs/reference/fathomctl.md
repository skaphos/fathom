<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# fathomctl Reference

`fathomctl` is the command-line client for the Fathom operator. It reads the
status the operator writes and drives the on-demand run trigger the operator
already honours; it never bypasses the operator. The
[lifecycle verbs](#lifecycle-verbs) (`install`, `upgrade`, `uninstall`,
`status`) install, upgrade and remove the operator itself. For a
task-oriented introduction see the [fathomctl guide](../guides/fathomctl.md).

## Global flags

| Flag | Default | Notes |
| --- | --- | --- |
| `--kubeconfig <path>` | `$KUBECONFIG`, then `~/.kube/config`, then in-cluster | Same discovery as kubectl. |
| `--context <name>` | current context | |
| `-n, --namespace <ns>` | the context's namespace, else `default` | Usage error with `-A`. Ignored by cluster-scoped kinds. `install` defaults to `fathom-system` instead; see [install](#install). |
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
| `NodeHealthCheck` | `nodehealthchecks` | `nhc` | namespaced | yes |
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
NodeCertificateCheck, NodeHealthCheck, HealthCheck, ClusterHealth, with a
`KIND` column.
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
DNSCheck, desired and reporting nodes for NodeCertificateCheck and
NodeHealthCheck — for NodeHealthCheck also the per-node results, the
privilege posture, and the interval and timeout as the controller applies
them, naming the declared value when it was capped at the 5m agent cadence —
matched count for ClusterHealth), every condition with its reason and message,
per-target results for DNSCheck, per-node results for NodeHealthCheck, each
contributing HealthCheck for ClusterHealth, and a
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
the largest target's conservative wait estimate plus 30 seconds; `--timeout`
overrides it exactly. For most kinds that estimate is the effective
`spec.timeout`. A `NodeHealthCheck` estimate accounts for its serial
DaemonSet rollout: `max(status.desiredNodes, 1)` times a one-minute pod
startup/termination allowance plus twice the effective timeout. Evaluation
and publication are each bounded by
`min(spec.timeout, min(spec.interval, 5m))`, so a one-node 24h/24h check gets
an 11m30s default, never a day. While polling a `NodeHealthCheck`, the CLI
raises this implicit deadline if newer status reports a larger
`desiredNodes`; the deadline remains anchored to the original start, never
shrinks, and does not reset on each poll. If the controller does not publish
the new fleet size before the initial one-node or stale-status budget expires,
the CLI cannot infer it. The estimate also cannot bound scheduler or image
pull delays; use an explicit `--timeout` when the cluster needs more time.
An explicit timeout stays absolute and is never expanded. If
the annotation changes to a different value before the token is consumed,
the run is reported as **superseded**. If the operator
reports it can never run the check (`Ready=False` with `InvalidPolicy`, `ItemsRejected` (a NodeHealthCheck item the operator's allowlist refuses),
`MissingAdapter`, `AdapterLookupFailed`, `NoMatchingNodes`, or `Paused`),
`--wait` fails immediately with that reason instead of waiting out the
deadline. The condition must belong to the object's current generation, so a
stale reason from before a spec change does not abort the new run. Transient
API errors (rate limiting, a restarting API server) do
not end the wait; only a deleted check or a permission failure does. A
timeout message names the remaining likely causes: an operator older than
the CLI (compare with `fathomctl version`) or a node-agent rollout still in
progress.

A `NodeCertificateCheck` or `NodeHealthCheck` run restarts one node-agent pod
per node before the token can complete; see
[Forcing a scan](../guides/node-certificate-checks.md#forcing-a-scan) and
give `--wait` an explicit `--timeout` if the conservative default does not
cover the cluster's rollout conditions.

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

## Lifecycle verbs

`install`, `upgrade`, `uninstall` and `status` manage the operator as a Helm
release made from the `fathom-operator` chart embedded in the binary. Each
`fathomctl` installs and upgrades to exactly its own version: there is no
`--version` flag and no chart is fetched at run time, so the verbs work
without registry access to the chart. Only the images must be pullable.
Reaching another version means running that release's `fathomctl`.

Releases are stored with Helm's Secret driver in the release namespace and
labelled `fathom.skaphos.io/installed-by=fathomctl`, so `helm ls`,
`helm history` and `helm rollback` see them. An install made with
`helm install` can be upgraded or removed by `fathomctl`. The chart's objects
are server-side applied under the field manager `helm` — the name the `helm`
CLI itself uses — whatever the `fathomctl` binary is called, so the two tools
can take turns on one release without apply conflicts; the release label
`fathom.skaphos.io/installed-by=fathomctl` records which tool made it. CRDs
are written by `fathomctl` itself, under field manager `fathomctl`. Installs made by
kustomize (`dist/install.yaml`, `config/default`) or OLM are refused with
`not-helm`, never taken over.

The global flags apply unchanged. Exit codes are `0` on success and `1`
otherwise. Every refusal happens before the cluster is changed and is printed
as `<reason>: <detail>` followed by a `next:` line with the command to run
instead.

### Images and versions

The CLI always sets the repository, tag and digest of the operator, probe and
node-agent images (`image`, `probeImage`, `nodeAgent.image`) from its own
release, replacing any image values a previous revision recorded. Setting any
of those values with `-f`, `--set`, `--set-string` or `--set-json` is a usage
error. `image.pullPolicy` is not affected.

A release archive carries the release's image digests and installs by digest.
A build without them (a local `go -C tools tool task fathomctl-build`) is a
development build: it installs by tag, `v<version>`. A development build's
version must be SemVer. A bare commit hash or `dev` is refused with
`dev-version`; build with an explicit version:

```sh
FATHOMCTL_VERSION=0.7.0 go -C tools tool task fathomctl-build
```

### CRD handling

`install` and `upgrade` apply every Fathom CRD before the operator rollout,
then wait for each to be `Established` within `--timeout`. A CRD that already
exists is replaced whole rather than merged, so a CRD first created by `helm`
or `kubectl apply` reaches exactly this release's schema; labels and
annotations other tools set are kept, except
`kubectl.kubernetes.io/last-applied-configuration`. The CLI reads each CRD
back and does not start the rollout unless its spec matches.

Every CRD the CLI applies carries the annotation
`fathom.skaphos.io/version: <version>`. `status` uses it to judge whether the
CRDs match the operator, and `install` uses it as the installed version when
only CRDs remain from an earlier `uninstall`.

Before applying, the CLI compares each live CRD with the release's using
crdify, with the same rules as the repository's `crd-compat` gate
(`.crdify.yaml`). The chart's `upgrade-path.yaml` declares, per release,
which incompatible changes are sanctioned and whether the release is a stop
that must be installed on the way to a later one. Its `floor` is the oldest
version that data covers.

### Shared flags (install, upgrade)

| Flag | Default | Notes |
| --- | --- | --- |
| `-f, --values <file>` | — | Values file(s), Helm semantics. Repeatable. |
| `--set k=v`, `--set-string k=v`, `--set-json k=json` | — | Helm-semantics overrides. Repeatable. |
| `--timeout <duration>` | `5m` | Bound on every wait: CRDs `Established`, operator `Available`. Must be positive. |
| `--accept-incompatible-crds` | off | Apply incompatible CRD changes the release sanctions. `--yes` never implies it. |

Values are validated against the chart's schema before anything changes.

### install

```text
fathomctl install [-n <ns>] [--create-namespace] [--release <name>] [shared flags]
```

| Flag | Default | Notes |
| --- | --- | --- |
| `-n, --namespace <ns>` | `fathom-system` | Not the kubeconfig context's namespace. |
| `--create-namespace` | off | Without it, a missing namespace is refused with `namespace-missing`. |
| `--release <name>` | `fathom` | Helm release name. |

| Situation | Outcome |
| --- | --- |
| Nothing installed | CRDs applied, release installed, waits for the operator to be `Available`; prints version, release, namespace, revision and image references. Exit `0`. |
| Only Fathom CRDs remain | The upgrade checks run first, with the installed version taken from `fathom.skaphos.io/version` on the CRDs: `crd-downgrade` when a CRD records a newer release, otherwise `below-floor`, `stop-skipped`, `scope-change`, `unsanctioned-incompatible` or `acknowledgement-required` as for `upgrade`. A CRD recording no version is compared without an upgrade path, so any incompatible finding refuses. Existing resources are kept. |
| Installed at this version | `Fathom is already installed at vX; nothing to do`, exit `0`. |
| Installed at an older version | `use-upgrade`; `next: fathomctl upgrade -n <ns>`. |
| Installed at a newer version | `downgrade`; names the matching `fathomctl` release. |
| Release pending or failed | `release-pending`; `next:` `helm history` and `helm rollback`. |
| Operator not `Available` in time | Exit `1` with the unmet condition and recovery commands (`kubectl describe deploy`, `fathomctl status`, `fathomctl uninstall`). |

`-o json|yaml` prints `{release, namespace, revision, version, images,
pinning}`.

### upgrade

```text
fathomctl upgrade [--dry-run] [--rollback-on-failure] [shared flags]
```

| Flag | Default | Notes |
| --- | --- | --- |
| `--dry-run` | off | Print the plan and change nothing. Needs only read access: the write-permission preflight is skipped. |
| `--rollback-on-failure` | off | Roll the operator back to the previous revision if it is not `Available` within `--timeout`. CRDs are never rolled back. Refused with `rollback-refused` when the plan applies an incompatible CRD change. |

The release and namespace are discovered; `-n` adds a namespace to search.
The release keeps its recorded values; `-f` and `--set*` override them, and
the new chart's defaults apply to everything neither sets (Helm's
`--reset-then-reuse-values`, never `--reuse-values`, which would keep the old
chart's defaults). `upgrade` prints the plan first:

```text
Fathom upgrade plan: v0.6.0 -> v0.7.0 (release fathom in fathom-system)
Path:      direct
CRDs:
  addonchecks.fathom.skaphos.io        update
  dnschecks.fathom.skaphos.io          update      incompatible (sanctioned by v0.7.0 (<issue>)): ^.spec.targets <check>: <crdify message>
  nodehealthchecks.fathom.skaphos.io   unchanged
Operator:  ghcr.io/skaphos/fathom-operator@sha256:… -> ghcr.io/skaphos/fathom-operator@sha256:…
Values:    (in effect; the chart's defaults apply to everything else)
  image:
    …
```

The second CRD line is illustrative. With `-o json|yaml` the plan is printed
as `{from, to, release, namespace, path, stops, crds, refusals, operator,
values}`; values whose keys look like credentials are redacted.

| Situation | Outcome |
| --- | --- |
| Compatible changes | CRDs applied and `Established`, then the operator rolled out; waits until the rollout is complete (`kubectl rollout status` semantics: every replica updated and available, no old replica left — a surge whose new pod never becomes ready is a failure even while the old pod keeps the Deployment `Available`). Exit `0`. |
| Already at this version, CRDs unchanged, no `-f`/`--set*` | `Fathom is already at vX; nothing to do`, exit `0`. With value flags the release is upgraded in place. |
| `helm upgrade` fails after the CRDs were applied | Exit `1`; the CRDs stay on the new schema and the error names `helm history` and a retry. |
| Sanctioned incompatible change, no `--accept-incompatible-crds` | `acknowledgement-required`; lists each change with its reason and issue; `next:` repeats the command with the flag. |
| Incompatible change no release sanctions | `unsanctioned-incompatible`; names the CRD, path and crdify finding. |
| CRD scope change | `scope-change`; recreating the CRD deletes every resource of that kind; `next:` `fathomctl uninstall --purge-crds --export <file>`. Never sanctionable. |
| A declared stop between the installed version and this one | `stop-skipped`; prints the full path (`v0.7.0 -> v0.8.0 -> v0.9.0`); `next:` the first stop's `fathomctl`. `--accept-incompatible-crds` does not bypass it. |
| Installed below the `floor` | `below-floor`; `next:` a Helm upgrade to the latest patch of the floor's minor (CRDs applied with `kubectl`), then rerun. |
| Installed version newer than the CLI | `downgrade`; names the matching `fathomctl` release. |
| Release pending or failed | `release-pending`; `next:` `helm rollback`, or `fathomctl uninstall` then `install` when there is no earlier revision. |
| Nothing installed | `not-installed`; `next: fathomctl install`. |
| Operator not `Available` in time | The new release is left in place and `upgrade` exits `1` with the unmet condition and recovery lines: `kubectl -n <ns> describe deploy/<name>`, a retry with a longer `--timeout`, and `helm rollback <release> -n <ns>` (operator only; CRDs stay). With `--rollback-on-failure` the operator is rolled back instead, then exit `1`. |

### uninstall

```text
fathomctl uninstall [--purge-crds [--export <file>]] [--delete-namespace] [-y|--yes] [--timeout <duration>]
```

| Flag | Default | Notes |
| --- | --- | --- |
| `--purge-crds` | off | Also delete every Fathom custom resource and every `fathom.skaphos.io` CRD. |
| `--export <file>` | — | With `--purge-crds` only: write every Fathom resource to this file first. |
| `--delete-namespace` | off | Delete the release namespace after the operator is removed. |
| `-y, --yes` | off | Skip the confirmation prompt. |
| `--timeout <duration>` | `5m` | Bound on every wait: object deletion and CRD removal. |

A plain `uninstall` removes the release, waits until the operator's
Deployment and pods are gone (so a terminating operator cannot recreate
anything), then removes every object the operator created at run time and
waits until they are gone. An interrupted `helm uninstall` is finished the
same way, and rerunning `uninstall` after one that stopped part-way (for
example, an operator pod stuck terminating past `--timeout`) removes the
operator-created objects left behind instead of reporting nothing to remove:

- objects labelled `fathom.skaphos.io/managed-by=fathom` in any namespace:
  node-agent DaemonSets, Pods (including unowned probe Pods in `kube-system`),
  ServiceAccounts, Roles, RoleBindings, NetworkPolicies and ConfigMaps;
- the cluster-scoped ValidatingAdmissionPolicy and ValidatingAdmissionPolicyBinding
  `fathom-node-report-authenticity`;
- the leader-election Lease in the release namespace (the release's
  `leaderElectionID` value, default `2d3dbc4f.skaphos.io`).

CRDs, custom resources and the namespace are kept, so a later `install`
picks the resources up again. `uninstall` needs no chart, so any `fathomctl`
version can remove any install, and it proceeds on a pending or failed
release, reporting the state it found. With nothing installed it prints
`Fathom is not installed; nothing to remove.` and exits `0`; with only CRDs
left, `--purge-crds` still purges them.

`--purge-crds` prints a count for each of the nine kinds (AddonDefinition,
AddonDefinitionBinding, AddonCheck, DNSCheck, NodeCertificateCheck,
NodeHealthCheck, HealthCheck, ClusterHealth, HealthReport) and, without
`--export`, says nothing will be saved. It then deletes every resource and
every CRD and waits for their removal.

`--export <file>` writes one multi-document YAML file atomically (a temporary
file, then a rename) before anything is deleted. If writing fails, nothing is
deleted and the exit code is `1`. The operator can still write (a HealthReport,
say) until it stops, so once it has, the resources are listed again; if they
changed, the file is rewritten from that list before the purge, and the
deleted count comes from it. If that rewrite fails, the release is already
removed but no resource or CRD is deleted. Each resource is stripped of `status` and of
`uid`, `resourceVersion`, `managedFields`, `creationTimestamp`, `generation`,
`ownerReferences`, `finalizers` and the deletion fields, so
`kubectl apply -f <file>` recreates it after a reinstall. Restored resources
carry no owner references: HealthReports no longer point at their checks, so
they are not garbage-collected with them.

`--delete-namespace` lists every object in the namespace before anything is
removed and names those Fathom did not create. Objects Kubernetes creates in
every namespace are reported, not treated as someone else's. An object owned
by another object in the namespace (a Pod of a ReplicaSet, say) is reported
through its owner; one whose owner is not there is listed itself. If the identity
cannot list a kind in the namespace, the verb refuses with
`missing-permissions`, naming the kind, before anything changes: it would
otherwise delete objects it never showed. For the same reason, an API group
that discovery cannot reach (for example an unavailable aggregated API) stops
the uninstall.

Everything that needs consent is gathered into one confirmation before
anything changes: `--purge-crds` always needs it, and `--delete-namespace`
needs it when the namespace holds objects Fathom did not create (otherwise the
namespace is deleted without asking). On a terminal it is a `[y/N]` prompt;
`--yes` skips it; without a terminal and without `--yes` the verb refuses with
`confirmation-required`, printing what would have been deleted. Declining
changes nothing.

### status

```text
fathomctl status [-o table|json|yaml]
```

Read-only. Reports the managing tool, the Helm release, namespace and
revision, the version and images (pinned by digest or by tag), the operator's
`Available` condition, the state of each CRD and the upgrade path from the
installed version to this `fathomctl`'s:

```text
Fathom:     installed (helm release fathom in fathom-system, revision 3, by fathomctl)
Version:    v0.6.0 (images pinned by digest)
  operator   ghcr.io/skaphos/fathom-operator@sha256:…
  probe      ghcr.io/skaphos/fathom-probe@sha256:…
  node-agent ghcr.io/skaphos/fathom-node-agent@sha256:…
Operator:   Available
CRDs:       9/9 current
Upgrade to this fathomctl (v0.7.0): direct
```

CRD states, judged from the `fathom.skaphos.io/version` annotation against the
operator's version:

| State | Meaning | Fails `status` |
| --- | --- | --- |
| `current` | Recorded version equals the operator's. | no |
| `outdated` | Recorded version is older than the operator's. | yes |
| `newer` | Recorded version is newer than the operator's. | yes |
| `missing` | The CRD does not exist. | yes |
| `unknown` | No recorded version (applied by `helm` or `kubectl`) and this CLI is not the operator's release, so there is nothing to compare against; or the recorded version is not SemVer. | no |

When a CRD records no version and this CLI is the operator's release, its
embedded CRDs are the reference: `current` if they match, else `outdated`.

An operator installed by kustomize or OLM is reported as
`installed in <ns> by <tool>; not managed by fathomctl`.

Exit `0` when Fathom is installed and healthy, or not installed. Exit `1` when
the operator is not `Available`, a CRD is `outdated`, `newer` or `missing`,
the release is pending or failed, or more than one Fathom release exists.
`-o json|yaml` prints the install (`managedBy`, `release`, `namespace`,
`revision`, `releaseStatus`, `installedBy`, `version`, `images`, `pinning`,
`ready`, `readyReason`) plus `installed`, `crds[]`, `upgradePath`,
`cliVersion` and `problems`.

### Refusal reasons

| Reason | Verbs | Meaning |
| --- | --- | --- |
| `missing-permissions` | install, upgrade, uninstall | The kubeconfig identity lacks a permission the verb needs; each is named. See [Permissions](#permissions). |
| `multiple-installs` | all four | More than one Fathom release (or operator Deployment) exists, including a release beside an operator Deployment it does not own (for example a kustomize or OLM install); the CRDs are shared, so one install per cluster is supported. `status` reports it (in the `-o` format requested) and exits `1`. |
| `not-helm` | install, upgrade, uninstall | The operator was installed by kustomize, OLM or an unknown tool. |
| `release-pending` | install, upgrade | The release is pending, failed or mid-uninstall; recover with `helm history` / `helm rollback`, or — when there is no earlier revision — `fathomctl uninstall` and reinstall. |
| `namespace-missing` | install | The namespace does not exist and `--create-namespace` was not given. |
| `use-upgrade` | install | An older version is installed. |
| `downgrade` | install, upgrade | The installed version is newer than this CLI. |
| `not-installed` | upgrade | Nothing to upgrade. |
| `dev-version` | install, upgrade | This CLI's version is not SemVer. |
| `below-floor` | install, upgrade | The installed version predates the upgrade data's `floor`. |
| `stop-skipped` | install, upgrade | A declared stop lies between the installed version and this one. |
| `crd-downgrade` | install | Leftover CRDs record a newer release than this CLI. |
| `scope-change` | install, upgrade | A CRD changes scope. |
| `unsanctioned-incompatible` | install, upgrade | An incompatible CRD change no release sanctions. |
| `acknowledgement-required` | install, upgrade | Sanctioned incompatible changes need `--accept-incompatible-crds`. |
| `rollback-refused` | upgrade | `--rollback-on-failure` with an incompatible CRD change in the plan. |
| `unknown-kinds` | uninstall | `--purge-crds` found a `fathom.skaphos.io` CRD this `fathomctl` cannot enumerate — a kind it does not know, or one no longer served at `v1alpha1` (both from a newer release) — so it could not count or export its resources. Use the `fathomctl` from the installed release. |
| `confirmation-required` | uninstall | A destructive uninstall without a terminal and without `--yes`. |

Supplying image values is a usage error rather than a refusal reason; it also
exits `1` before anything changes.

### Permissions

`install`, `upgrade` and `uninstall` need cluster-admin-level rights: they
create and replace CRDs, cluster-scoped RBAC, the admission policy, and Helm's
release Secrets in the release namespace. They are not covered by the
`fathomctl-viewer-role` and `fathomctl-runner-role` described
[below](#rbac-for-fathomctl-users). Before changing anything each verb runs a
SelfSubjectAccessReview for every permission below and refuses with
`missing-permissions`, naming the ones the identity lacks:

| Verb | Checked permissions |
| --- | --- |
| `install`, `upgrade` | `create`, `update`, `get` on `customresourcedefinitions`; `create` on `clusterroles` and `clusterrolebindings`; in the release namespace, `create` on `roles`, `rolebindings`, `serviceaccounts`, `configmaps` and `services`, `create` and `update` on `deployments` and `secrets`, `get` and `list` on `secrets`, and `patch` on every chart object kind (server-side apply); with `--create-namespace`, `create` on `namespaces`. `upgrade --dry-run` checks none of these. |
| `uninstall` | in the release namespace, `delete` on `deployments`, `secrets`, `services` and `leases`; cluster-wide, `delete` on `clusterroles`, `clusterrolebindings`, `daemonsets`, `pods`, `serviceaccounts`, `configmaps`, `roles`, `rolebindings`, `networkpolicies`, `validatingadmissionpolicies` and `validatingadmissionpolicybindings`; with `--purge-crds`, `delete` on `customresourcedefinitions` and every Fathom kind; with `--delete-namespace`, `delete` on the namespace. A rerun after the release is already gone checks only the cluster-wide and `--purge-crds` permissions, since no release namespace remains |

The list covers every kind each verb writes or deletes. It does not check
Kubernetes' RBAC escalation rules: creating the chart's ClusterRoles also
requires holding the permissions they grant (or `escalate`/`bind`), which a
cluster-admin-level identity has.

`status` is read-only and needs exactly:

- `list` on `deployments` cluster-wide (to find the operator);
- `get` and `list` on `secrets` only in the operator's namespace and the `-n`
  namespace (Helm release records); no cluster-wide Secret access;
- `get` and `list` on `customresourcedefinitions`.

The other lifecycle verbs find the install with the same reads.

## Version skew

Keep the CLI within one minor version of the operator. The read verbs only
depend on the resource schema, so they degrade gracefully. `run` depends on
the operator honouring the run-now trigger for the kind in question: an
operator older than the CLI that does not yet consume the trigger for a kind
never writes `status.lastRunTrigger`, so `run --wait` times out rather than
failing up front. The timeout message points at `fathomctl version` for that
reason.

The lifecycle verbs do not tolerate skew: `install` and `upgrade` move the
operator to exactly the CLI's version and refuse an install newer than the
CLI. `status` and `uninstall` work with any version.

## RBAC for fathomctl users

The operator's own role is unchanged by the CLI. Two auxiliary ClusterRoles
ship under `config/rbac/` for people and CI jobs:

| Role | Grants | Use it for |
| --- | --- | --- |
| `fathomctl-viewer-role` | `get`, `list`, `watch` on the six check kinds and `healthreports` (plus `/status`); `get`, `list` on `apps/deployments` | `ls`, `describe`, `reports`, `version` |
| `fathomctl-runner-role` | the viewer rules plus `patch` on `addonchecks`, `dnschecks`, `nodecertificatechecks`, `nodehealthchecks` | everything, including `run` |

Neither role covers the lifecycle verbs; see [Permissions](#permissions).

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

## Runtime definition authoring (preview)

Runtime loading remains under implementation. These commands prepare and inspect
resources; they do not activate runtime checks or write to the cluster.

| Command | Purpose | Required flags |
| --- | --- | --- |
| `definition render` | Offline staged definition, dedicated SA, scoped grants, incomplete binding template | `--file`, `--service-account`, `--operator-namespace`, `--operator-service-account` |
| `definition bind` | Read live definition/SA UIDs and print a disabled binding | `--file`, `--name`, `--service-account`, `--operator-namespace` |
| `definition collisions` | Compare live definitions against the target binary's bundled built-in inventory | None; use target release binary |
| `definition drain` | Independently verify Lease renewal and current disabled-binding acknowledgement | `--name`, `--operator-namespace`; `--leader-election-id` for a nondefault Lease |

`render` never loads kubeconfig. Supply exactly one definition document; unknown
fields and oversized files are rejected. Custom-resource plural/scope mappings
that cannot be established offline produce manual-completion comments. Review
these grants before applying prerequisites. Do not apply the incomplete binding
at the end of the staged output: it intentionally has empty UIDs and scope.

`bind` requires the reviewed spec to match the live spec exactly, including API
admission defaults. It prints a disabled binding with the union of explicit
primary/helper scopes. It neither proves effective RBAC nor grants permissions.

`collisions` prints the binary version and source revision. Missing provenance or
inventory fails verification; there is no external inventory-file option. Exit
codes are 0 for no collisions, 1 for collisions, and 2 for verification/read errors.

`drain` performs at most 16 Lease reads and one binding read within 15 seconds,
with individual requests bounded to 5 seconds and renewal polling at most once
per second. It requires progressing renewal of the same leader epoch and matching
current-generation binding status. Exit codes are 0 for verified drained, 1 for
not drained, and 2 for unverifiable. A leader can change after the final read;
this is an observation, not an atomic revocation guarantee.
