<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# fathomctl

`fathomctl` is the command-line client for Fathom. It reads the status the
operator publishes on every check and drives the on-demand run trigger the
operator already honours. Everything it shows is in the resources already;
`kubectl` remains fully sufficient. What `fathomctl` adds is one consistent
view of verdicts across every kind, and a `run --wait` you can put in a
pipeline.

Five verbs work with checks: `ls`, `describe`, `reports`, `run`, `version`.
There is no `pause` or `resume`: Fathom emits signal and does not own
suppression, so stopping a check means deleting it. Authoring checks stays in
Git; the CLI never creates or edits them. Four more verbs manage the operator
itself: `install`, `upgrade`, `uninstall` and `status` (see
[Install, upgrade and remove Fathom](#install-upgrade-and-remove-fathom)).
The flag-by-flag contract is in the
[fathomctl reference](../reference/fathomctl.md).

## Install

Each release attaches one archive per platform, a checksums file, a keyless
signature over the checksums, and build provenance for the archives.

```sh
VERSION=X.Y.Z            # the Fathom release you are pairing with
OS=linux ARCH=amd64      # darwin/windows, arm64 also published
BASE=https://github.com/skaphos/fathom/releases/download/v${VERSION}

curl -fsSLO "${BASE}/fathomctl_${VERSION}_${OS}_${ARCH}.tar.gz"
curl -fsSLO "${BASE}/fathomctl_${VERSION}_checksums.txt"
curl -fsSLO "${BASE}/fathomctl_${VERSION}_checksums.txt.sigstore.json"

cosign verify-blob \
  --bundle "fathomctl_${VERSION}_checksums.txt.sigstore.json" \
  --certificate-identity-regexp '^https://github.com/skaphos/fathom/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  "fathomctl_${VERSION}_checksums.txt"
sha256sum --check --ignore-missing "fathomctl_${VERSION}_checksums.txt"

tar -xzf "fathomctl_${VERSION}_${OS}_${ARCH}.tar.gz"
sudo install -m 0755 "fathomctl_${VERSION}_${OS}_${ARCH}/fathomctl" /usr/local/bin/fathomctl
fathomctl version --client
```

Windows archives are `.zip` and contain `fathomctl.exe`. See
[RELEASE.md](../../RELEASE.md#verify-a-fathomctl-download) for the full
verification procedure, including provenance.

`fathomctl` finds your cluster the way `kubectl` does: `--kubeconfig`, then
`$KUBECONFIG`, then `~/.kube/config`, then in-cluster configuration. Exit
codes follow `kubectl` too: `0` on success, `1` on any error.

## Which version am I talking to?

```sh
fathomctl version
```

```text
Client:   v0.6.0
Operator: v0.6.0 (fathom-system/fathom-controller-manager)
```

Offline, or without permission to read Deployments, the operator line reads
`unavailable (<reason>)` and the command still exits `0`. Keep the CLI within
one minor version of the operator; an older operator that does not yet honour
the run trigger for a kind shows up as a `run --wait` timeout, not an error
up front.

## Install, upgrade and remove Fathom

`fathomctl` is the primary way to install, upgrade and remove the operator.
It drives the same Helm chart that `helm install` uses, embedded in the
binary, as a Helm release `helm` can read; installing with Helm directly, as
in [Getting started](getting-started.md#1-install-the-operator), remains an
equivalent alternative. These verbs need cluster-admin-level rights; see
[Permissions](../reference/fathomctl.md#permissions).

```sh
fathomctl install --create-namespace   # into fathom-system, at this fathomctl's version
fathomctl status                       # installed, Available, CRDs 9/9 current
```

`install` applies every Fathom CRD, installs the operator with the images of
this `fathomctl`'s release (pinned by digest in release archives), and waits
until it is `Available`. Without `-n` it uses `fathom-system`, not your
kubeconfig context's namespace. `-f`/`--values` and `--set` take chart values
with Helm semantics; the image values are set by the CLI and cannot be
overridden.

### One version per fathomctl

Each `fathomctl` installs and upgrades to exactly its own version, from the
chart embedded in it. There is no `--version` flag and no chart download, so
the verbs work without registry access to the chart. To install or upgrade to
another release, download that release's `fathomctl`.

### Upgrade

```sh
fathomctl status             # where you are, and the upgrade path to this fathomctl
fathomctl upgrade --dry-run  # the plan: each CRD added, updated or unchanged; the images
fathomctl upgrade
```

`upgrade` applies the CRDs and waits for them to be `Established` before it
rolls out the new operator, keeping the release's values. It refuses, before
changing anything:

- an incompatible CRD change that no release sanctions, and any CRD scope
  change;
- a sanctioned incompatible change, until you repeat the command with
  `--accept-incompatible-crds` (the refusal lists each change with its reason
  and issue; `--yes` does not count);
- a skipped stop: a release that must be installed on the way to a later one.

A multi-step upgrade means running each intermediate release's `fathomctl`
in turn. The refusal prints the whole path and the first step, for example
with a hypothetical stop at v0.8.0:

```text
stop-skipped: v0.7.0 cannot be upgraded to v0.9.0 directly; upgrade through each stop in order. Upgrade path: v0.7.0 -> v0.8.0 -> v0.9.0
next: download fathomctl v0.8.0 and run "fathomctl upgrade -n fathom-system"
```

If the new operator is not `Available` within `--timeout`, the new release
stays in place and `upgrade` prints the recovery commands;
`--rollback-on-failure` rolls the operator back instead. CRDs are never
rolled back.

### Uninstall

```sh
fathomctl uninstall
```

This removes the release and everything the operator created at run time
(node-agent DaemonSets, probe Pods, per-check ServiceAccounts, Roles,
RoleBindings and NetworkPolicies, node-report ConfigMaps, the shared
admission policy and the leader-election Lease). It keeps every CRD, every
Fathom resource and the namespace, so a later `install` picks the resources
up again. Any `fathomctl` version can uninstall any install.

To remove everything, including your checks and their report history:

```sh
fathomctl uninstall --purge-crds --export fathom-backup.yaml
```

This shows how many resources of each kind will be deleted and asks once for
confirmation (`--yes` skips the prompt and is required without a terminal).
`--export` writes every resource to the file before anything is deleted, so
you can `kubectl apply -f fathom-backup.yaml` after a reinstall. Restored
resources carry no owner references: HealthReports no longer point at the
checks that wrote them and are not garbage-collected with those checks.
`--delete-namespace` also deletes the namespace, after listing anything in it
that Fathom did not create.

### Mixing fathomctl and Helm

`helm ls`, `helm history` and `helm rollback` work on a release `fathomctl`
made, and `fathomctl upgrade` works on a release made with `helm install`.
In the other direction, `helm upgrade` given no values reuses the previous
revision's, which include the image digests `fathomctl` pinned: moving a
`fathomctl`-made install to a newer chart with plain `helm upgrade` would run
the old images under the new chart. Pass `--reset-values` together with your
own values (`-f` or `--set`) when you do that.

### Not supported by these verbs

Installs made with kustomize (`dist/install.yaml`) or OLM are refused, never
taken over; manage them with the tool that installed them. One Fathom install
per cluster is supported, because the CRDs are shared.

### Air-gapped clusters

The chart is embedded, so only the images must be pullable from the cluster.
Image repositories are fixed by the release and cannot be overridden through
values, so configure the container runtime with a registry mirror for
`ghcr.io` that serves the same digests of `skaphos/fathom-operator`,
`skaphos/fathom-probe` and `skaphos/fathom-node-agent`.

## See every verdict

```sh
fathomctl ls -A
```

```text
KIND                  NAMESPACE      NAME               VERDICT  SUMMARY                          LAST RUN  NEXT RUN
AddonCheck            fathom-system  coredns            Pass     3 of 3 checks passed             2m        3m
DNSCheck              fathom-system  cluster-dns        Pass     2 of 2 pairs resolved            40s       20s
NodeCertificateCheck  fathom-system  node-certificates  Warn     apiserver.crt expires in 21d     12m       48m
HealthCheck           fathom-system  coredns            Pass     3 of 3 checks passed             2m        3m
ClusterHealth                        prod               Warn     4 matched, worst Warn            40s       -
```

`ls` groups every kind; `ls dnschecks` (or `ls dns`) lists one. ClusterHealth
is cluster-scoped and always appears. `-o json` emits the resources
unmodified inside a `List`, so the usual `jq` idioms work:

```sh
fathomctl ls -A -o json | jq -r '.items[] | select(.status.lastResult=="Fail") | .metadata.name'
```

## Understand a verdict

```sh
fathomctl describe dnscheck/cluster-dns
```

`describe` shows the spec as the controller applies it, the verdict and
summary, every condition with its reason, per-target results for a DNSCheck,
each contributing HealthCheck for a ClusterHealth, and a pointer to the
latest report. When a check is not doing what you expect, start here and
then follow the condition reason into
[Status and conditions](../reference/status-conditions.md).

## Walk the history

```sh
fathomctl reports addoncheck/coredns
fathomctl reports addoncheck/coredns --since 24h
fathomctl reports addoncheck/coredns --report coredns-3f9a1c2b
```

Reports are written when a verdict changes, not on every interval, so the
list is a transition log and a gap is not a missed run. The `CHANGE` column
says what moved: the verdict (`Pass→Fail`), some per-check results
(`2 check(s) changed`), or nothing.

## Check again, right now

```sh
fathomctl run addoncheck/coredns --wait
```

`run` writes a fresh token to the check's `fathom.skaphos.io/run-now`
annotation; the operator runs the check regardless of its interval and
records the token when the run completes. `--wait` blocks until that happens
and prints the verdict. In a pipeline, the exit code is the gate:

```sh
# Fails the job on Fail, Error, Unknown, a timeout, or a superseded trigger.
fathomctl run healthcheck/coredns --wait --timeout 3m
```

A HealthCheck run triggers the check it references; a ClusterHealth run
triggers the source behind every HealthCheck it selects, once each:

```sh
fathomctl run clusterhealth/prod --wait --yes
```

Above ten checks, `run` asks before writing anything; `--yes` skips the
prompt (required in CI, where there is no terminal) and `--dry-run` shows the
set without touching it. `--all` and `-l <selector>` trigger executable
checks in the namespace scope.

A NodeCertificateCheck run restarts one node-agent pod per node before the
scan; on a large cluster give `--wait` a longer `--timeout`. See
[Forcing a scan](node-certificate-checks.md#forcing-a-scan).

## Permissions

`install`, `upgrade` and `uninstall` need cluster-admin-level rights and
check them before changing anything; `status` needs only read access. The
exact lists are in [Permissions](../reference/fathomctl.md#permissions).

The read verbs need `get`, `list`, and `watch` on the Fathom kinds and
`healthreports`; `run` additionally needs `patch` on the three executable
kinds. Two ClusterRoles ship for exactly this, `fathomctl-viewer-role` and
`fathomctl-runner-role`; see
[RBAC for fathomctl users](../reference/fathomctl.md#rbac-for-fathomctl-users).
