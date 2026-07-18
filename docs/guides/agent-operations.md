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
workflow when it has one. Use the repository `deploy` task only when the user
explicitly asks to test the current checkout.

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
```

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
reconcile it; do not run a parallel `helm upgrade` against the cluster. When a
direct installation is explicitly authorized, use this idempotent, pinned Helm
command. `--atomic` removes a failed release, while Helm intentionally leaves
installed CRDs in place.

```sh
kubectl --context "${KUBE_CONTEXT}" auth can-i create customresourcedefinitions.apiextensions.k8s.io
kubectl --context "${KUBE_CONTEXT}" auth can-i create clusterroles.rbac.authorization.k8s.io

helm upgrade --install fathom oci://ghcr.io/skaphos/charts/fathom-operator \
  --version "${FATHOM_VERSION}" \
  --namespace fathom-system \
  --create-namespace \
  --kube-context "${KUBE_CONTEXT}" \
  --atomic --wait --timeout 5m
```

Direct installation requires both permission checks to return `yes`; otherwise
stop and ask the user to use an appropriately authorized identity or GitOps
workflow.

After either delivery path reports success, verify the live rollout:

```sh
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  wait --for=condition=Available deployment \
  --selector app.kubernetes.io/instance=fathom --timeout=2m
kubectl --context "${KUBE_CONTEXT}" get crd \
  addonchecks.fathom.skaphos.io \
  healthchecks.fathom.skaphos.io \
  clusterhealths.fathom.skaphos.io \
  healthreports.fathom.skaphos.io \
  nodecertificatechecks.fathom.skaphos.io
```

For upgrades, read the release notes first. Helm installs CRDs only on the
initial install and does not upgrade them automatically; follow the CRD upgrade
notes in [Getting started](getting-started.md#1-install-the-operator).

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
parallel Helm uninstall. For a direct installation, delete agent-created
examples before uninstalling the release. Do not delete Fathom CRDs unless the
user explicitly authorizes deleting all Fathom custom resources and their
report history from the cluster.

```sh
kubectl --context "${KUBE_CONTEXT}" delete clusterhealth platform
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  delete healthcheck/coredns addoncheck/coredns-system-health
helm uninstall fathom --namespace fathom-system --kube-context "${KUBE_CONTEXT}"
```
