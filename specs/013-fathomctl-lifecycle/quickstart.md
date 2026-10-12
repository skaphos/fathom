<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Quickstart: Validating fathomctl Lifecycle Verbs

Run against a disposable kind cluster with cluster-admin. Commands assume a
`fathomctl` built from this branch (`go -C tools tool task fathomctl-build`)
and the operator, probe and node-agent images loaded into kind with the tag the
build expects. Expected results reference
[contracts/cli-lifecycle.md](contracts/cli-lifecycle.md).

## 1. Install, then confirm idempotency

```sh
fathomctl install -n fathom-system --create-namespace
fathomctl status                      # installed, Available, CRDs 9/9 current; exit 0
helm ls -n fathom-system              # release "fathom" listed
fathomctl install -n fathom-system    # "already installed ... nothing to do"; exit 0
```

## 2. Upgrade across a CRD schema change (CRDs first)

```sh
kubectl apply -f test/e2e/fixtures/lifecycle/older-crds/   # simulate an older schema
fathomctl status                      # CRDs outdated; exit 1
fathomctl upgrade --dry-run           # plan lists the CRD updates; nothing changes
fathomctl upgrade                     # CRDs Established before new manager Pods; exit 0
fathomctl status                      # CRDs current; exit 0
```

## 3. Refusals (unit-covered; spot-check with `--dry-run`)

- Sanctioned incompatible change without `--accept-incompatible-crds` → exit 1,
  lists change, reason, issue.
- Skipped stop → exit 1, prints the path and the `fathomctl` release to run next.
- Scope change → exit 1, explains delete-and-recreate.

## 4. Uninstall leaves no workloads, keeps data

```sh
kubectl apply -f config/samples/fathom_v1alpha1_nodecertificatecheck.yaml
kubectl apply -f config/samples/fathom_v1alpha1_dnscheck.yaml
fathomctl uninstall
kubectl get ds,pod,sa,netpol,role,rolebinding,cm -A -l fathom.skaphos.io/managed-by=fathom  # none
kubectl get validatingadmissionpolicy fathom-node-report-authenticity                          # NotFound
kubectl get crd | grep fathom.skaphos.io                                                       # 9 CRDs
kubectl get nodecertificatechecks,dnschecks -A                                                 # still present
kubectl get ns fathom-system                                                                   # kept
```

## 5. Purge with export, then restore

```sh
fathomctl install -n fathom-system
fathomctl uninstall --purge-crds --export fathom-backup.yaml   # TTY: prompt names counts per kind
fathomctl uninstall --purge-crds --export fathom-backup.yaml --yes
kubectl get crd | grep fathom.skaphos.io                       # none
fathomctl install -n fathom-system && kubectl apply -f fathom-backup.yaml   # resources return
```

## 6. Namespace deletion

```sh
fathomctl uninstall --delete-namespace --yes   # namespace removed when only Fathom objects remain
```

## Automated

```sh
go test ./internal/cli/... ./deploy/helm/... ./scripts/...
go -C tools tool task test-e2e E2E_ADDONS=lifecycle   # lifecycle shard on its own kind cluster
```
