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

Five verbs: `ls`, `describe`, `reports`, `run`, `version`. There is no
`pause` or `resume`: Fathom emits signal and does not own suppression, so
stopping a check means deleting it. Authoring checks stays in Git; the CLI
never creates or edits them. The flag-by-flag contract is in the
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

The read verbs need `get`, `list`, and `watch` on the Fathom kinds and
`healthreports`; `run` additionally needs `patch` on the three executable
kinds. Two ClusterRoles ship for exactly this, `fathomctl-viewer-role` and
`fathomctl-runner-role`; see
[RBAC for fathomctl users](../reference/fathomctl.md#rbac-for-fathomctl-users).
