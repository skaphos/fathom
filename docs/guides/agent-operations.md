<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Operating Fathom with automation and AI agents

This is a runbook for automation and AI agents asked to **install or operate
Fathom on a cluster**. It is intentionally prescriptive: it favours pinned
releases, explicit approval before touching a cluster, and read-only discovery
over assumptions.

If you are a human getting started, read [Getting started](getting-started.md)
instead — it is the same workflow written for a person, with more explanation
and fewer guardrails. This document is what a person would point an agent at.

> **Not to be confused with [`AGENTS.md`](../../AGENTS.md)** (symlinked as
> `CLAUDE.md`), which briefs agents *contributing to the Fathom codebase* —
> build, test, and coding guardrails. This page is about *running* Fathom
> against a live cluster.

Prefer the released Helm chart, delivered through the cluster's existing GitOps
workflow when it has one. For a direct installation, use the `fathomctl`
release matching the approved version: its `install`, `upgrade`, `uninstall`
and `status` verbs drive that chart, apply CRDs before the operator, and
refuse unsafe transitions before changing anything. Use the repository
`deploy` task only when the user explicitly asks to test the current checkout.

## 1. Confirm the target and release

- Get an explicit target cluster and an exact chart version approved by the
  user or already pinned in its authorized GitOps source. Do not infer a
  production target or install an unpinned `latest` release.
- If live access is authorized, pass the context to every command instead of
  changing the caller's current context. Repeat the workflow separately for
  each approved cluster.
- Fathom validates existing add-ons; it does not install them.

```sh
export KUBE_CONTEXT="your-approved-context"
export FATHOM_VERSION="X.Y.Z"  # released chart version, without a leading v

# Run only when the user has authorized live access.
kubectl --context "${KUBE_CONTEXT}" cluster-info
helm version --short

# For a direct installation: the fathomctl of exactly that release.
fathomctl version --client   # must print v${FATHOM_VERSION}
```

Each `fathomctl` installs and upgrades to exactly its own version; there is no
`--version` flag. If `fathomctl version --client` does not match the approved
version, download the matching release's `fathomctl` (see the
[fathomctl guide](fathomctl.md#install)) rather than using the one at hand.

Stop and ask for direction if the context is not the approved target.

## 2. Discover add-ons and the cluster delivery model

Before choosing `AddonCheck`s or changing the cluster, ask the user:

- Are the cluster add-ons defined and reconciled through GitOps?
- If so, which GitOps repositories and paths contain them, and may the agent
  read those repositories?
- Should Fathom and its health-check resources also be delivered through
  GitOps? If so, which repository, path, and review workflow should be used?

When GitOps repositories are available, treat them as the desired-state source
of truth. Scan the relevant Argo CD Applications, Flux HelmReleases and
Kustomizations, Helm values, Helmfile definitions, or plain manifests to
identify supported add-ons and their configured namespaces, release names,
workload names, Services, modes, and versions. Use those values to configure
Fathom thresholds instead of assuming upstream defaults. Do not read Secret
payloads, change the repositories, or apply resources directly unless the user
has authorized that action.

Desired state may differ from live state. Ask before comparing it with the
cluster. If add-ons are not managed through GitOps, repository access is
incomplete, or the user requests live verification, ask permission to run
read-only discovery. Start with Helm releases:

```sh
helm list --all-namespaces --kube-context "${KUBE_CONTEXT}"
kubectl --context "${KUBE_CONTEXT}" get crd
```

Inspect only the workloads and configuration needed to resolve ambiguous
add-on names or namespaces. Summarize the discovered add-ons and proposed check
families for user confirmation before creating health checks.

## 3. Install and verify the operator

Follow the delivery model confirmed above. For GitOps-managed clusters, add a
pinned Fathom chart release to the authorized repository and let its controller
reconcile it; do not run a parallel `fathomctl` or `helm upgrade` against the
cluster. When a direct installation is explicitly authorized, use the
matching release's `fathomctl`. Start read-only:

```sh
fathomctl --context "${KUBE_CONTEXT}" status
```

`Fathom: not installed` means install; an installed older version means
upgrade (below). An install made with kustomize or OLM is reported as not
managed by `fathomctl`, and `install`, `upgrade` and `uninstall` refuse it:
stop and ask the user how it should be managed. More than one Fathom release
also exits `1`; stop and ask.

```sh
fathomctl --context "${KUBE_CONTEXT}" install -n fathom-system --create-namespace
```

`install` checks the identity's permissions before changing anything and
refuses with `missing-permissions`, naming each missing one; if so, stop and
ask the user for an appropriately authorized identity or GitOps workflow. It
waits until the operator is `Available` (bounded by `--timeout`, default
`5m`) and is idempotent: on an install at the same version it prints
`nothing to do` and exits `0`.

After either delivery path reports success, verify the live state.
`status` exits `0` only when the operator is `Available` and every CRD
matches its version:

```sh
fathomctl --context "${KUBE_CONTEXT}" status
```

If `fathomctl` is not in use, the equivalent Helm command and checks are:

```sh
kubectl --context "${KUBE_CONTEXT}" auth can-i create customresourcedefinitions.apiextensions.k8s.io
kubectl --context "${KUBE_CONTEXT}" auth can-i create clusterroles.rbac.authorization.k8s.io

helm upgrade --install fathom oci://ghcr.io/skaphos/charts/fathom-operator \
  --version "${FATHOM_VERSION}" \
  --namespace fathom-system \
  --create-namespace \
  --kube-context "${KUBE_CONTEXT}" \
  --atomic --wait --timeout 5m

kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  wait --for=condition=Available deployment \
  --selector app.kubernetes.io/instance=fathom --timeout=2m
```

`--atomic` removes a failed release, while Helm intentionally leaves installed
CRDs in place and never upgrades them; with Helm, follow the CRD notes in
[Getting started](getting-started.md#1-install-the-operator).

### Upgrade runbook

Read the release notes first, then use the target release's `fathomctl`:

```sh
fathomctl --context "${KUBE_CONTEXT}" status                # current version and the path to this fathomctl
fathomctl --context "${KUBE_CONTEXT}" upgrade --dry-run     # the plan; changes nothing
# Show the plan to the user and get approval, then:
fathomctl --context "${KUBE_CONTEXT}" upgrade
fathomctl --context "${KUBE_CONTEXT}" status                # exit 0: Available, CRDs current
```

`upgrade` applies the CRDs and waits for them to be `Established` before it
rolls out the operator; no manual CRD step is needed. Every refusal happens
before anything changes and ends with a `next:` line. Do not work around a
refusal:

- `acknowledgement-required`: the release sanctions incompatible CRD changes,
  listed with reason and issue. Existing resources may fail the new
  validation. Show the list to the user; add `--accept-incompatible-crds`
  only with their explicit approval.
- `stop-skipped`: an intermediate release must be installed first. Run that
  release's `fathomctl upgrade`, then this one. Never bypassable.
- `unsanctioned-incompatible`, `scope-change`, `below-floor`: stop and report
  the message and its `next:` line to the user.

If the operator is not `Available` within `--timeout`, the new release stays
in place and `upgrade` exits `1` with recovery commands. Report them; do not
roll back without approval. `--rollback-on-failure` rolls back only the
operator, never the CRDs, and is refused when the plan includes an
incompatible CRD change.

## 4. Create a first cluster health signal

Use the confirmed GitOps or live-discovery results to create checks only for
add-ons that exist, with thresholds matching their actual object names and
namespaces. For GitOps-managed clusters, put these resources in the authorized
repository and let the GitOps controller apply them. For an explicitly
authorized direct workflow, the following baseline checks CoreDNS, mirrors its
status through a `HealthCheck`, and limits the cluster-wide aggregate to
wrappers carrying the `platform` label.

```sh
kubectl --context "${KUBE_CONTEXT}" apply -f - <<'EOF'
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: coredns-system-health
  namespace: fathom-system
spec:
  addonType: coredns
  interval: 5m
  timeout: 30s
  policy:
    system_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        deploymentName: "coredns"
        serviceName: "kube-dns"
        restartWarnCount: "3"
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: coredns
  namespace: fathom-system
  labels:
    fathom.skaphos.io/aggregate: platform
spec:
  description: "Mirror CoreDNS health for cluster aggregation."
  checkRef:
    apiVersion: fathom.skaphos.io/v1alpha1
    kind: AddonCheck
    name: coredns-system-health
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: ClusterHealth
metadata:
  name: platform
spec:
  description: "Worst-case platform health across selected checks."
  namespaces:
    - fathom-system
  selector:
    matchLabels:
      fathom.skaphos.io/aggregate: platform
EOF

kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  wait --for=condition=Ready addoncheck/coredns-system-health --timeout=2m
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  wait --for=condition=Ready healthcheck/coredns --timeout=2m
kubectl --context "${KUBE_CONTEXT}" \
  wait --for=condition=Ready clusterhealth/platform --timeout=2m
```

If the distribution renames CoreDNS resources, adjust the thresholds before
applying the example. For other supported add-ons and check families, use the
manifests in [`config/samples/`](../../config/samples/) and the
[Add-on checks guide](addon-checks.md).

## 5. Read, refresh, and troubleshoot results

```sh
# Current check, normalized status, and cluster-wide verdict.
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  get addoncheck,healthcheck
kubectl --context "${KUBE_CONTEXT}" get clusterhealth platform

# Immutable run history and per-target details.
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  get healthreport \
  --selector 'fathom.skaphos.io/source-kind=AddonCheck,fathom.skaphos.io/source-name=coredns-system-health' \
  --sort-by=.metadata.creationTimestamp

# Force one immediate run by changing the one-shot annotation value.
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  annotate addoncheck coredns-system-health \
  fathom.skaphos.io/run-now="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite

# Inspect conditions and operator logs when a result is unexpected.
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  describe addoncheck coredns-system-health
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  logs --selector app.kubernetes.io/instance=fathom \
  --container manager --tail=100
```

Treat `Pass` and `Skipped` as green outcomes, investigate `Warn` and `Unknown`,
and treat `Fail` and `Error` as unhealthy. Read `status.conditions` and the
newest `HealthReport` before changing a check or the cluster. `ClusterHealth`
is cluster-scoped and derives its verdict only from selected `HealthCheck`
status; it never aggregates `HealthReport` history directly.

## 6. Remove only what was authorized

Remove GitOps-managed resources from their source repository and allow the
controller to prune them; do not fight reconciliation with direct deletes or a
parallel uninstall. For a direct installation, delete agent-created
examples before uninstalling the release:

```sh
kubectl --context "${KUBE_CONTEXT}" delete clusterhealth platform
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  delete healthcheck/coredns addoncheck/coredns-system-health
fathomctl --context "${KUBE_CONTEXT}" uninstall
```

A plain `uninstall` removes the release and every object the operator created
at run time (node-agent DaemonSets, probe Pods, per-check RBAC and
NetworkPolicies, ConfigMaps, the admission policy and the leader-election
Lease), and keeps the CRDs, every remaining Fathom resource and the
namespace. `helm uninstall` alone leaves the operator-created objects behind.

Do not delete Fathom CRDs unless the user explicitly authorizes deleting all
Fathom custom resources and their report history from the cluster. When they
do, run the purge once without `--yes`. Without a terminal it refuses with
`confirmation-required` and changes nothing, and the refusal lists how many
resources of each kind would be deleted. Show that to the user, then rerun
with `--yes` only after their approval:

```sh
fathomctl --context "${KUBE_CONTEXT}" uninstall --purge-crds --export fathom-backup.yaml
# After explicit approval of the listed counts:
fathomctl --context "${KUBE_CONTEXT}" uninstall --purge-crds --export fathom-backup.yaml --yes
```

The export is written before anything is deleted; if it cannot be written,
nothing is deleted. Re-apply it with `kubectl apply -f fathom-backup.yaml`
after a reinstall; restored resources carry no owner references. Add
`--delete-namespace` only when the user has authorized deleting the
namespace. Without `--yes` it refuses, listing them, when the namespace holds
objects Fathom did not create; when it holds only Fathom's own objects, it
deletes the namespace without asking.
