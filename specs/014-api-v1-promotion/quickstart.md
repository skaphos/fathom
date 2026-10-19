<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Quickstart: validating the `v1` promotion

Runnable checks that prove the feature end to end. Contracts:
[api-v1.md](contracts/api-v1.md), [cli-upgrade-export.md](contracts/cli-upgrade-export.md),
[retention.md](contracts/retention.md).

## Prerequisites

- Go per `go.mod`; `kind`, `docker`, `helm`, `helmfile` on `PATH`
  (mise shims for background runs).
- Network access from the kind node for the upgrade scenario, which pulls
  the published 0.6.0 chart and images.

## 1. Generated artifacts and gates

```sh
go -C tools tool task verify-generated   # api/v1 types -> CRDs, deepcopy, chart crds, api.md, rbac
go -C tools tool task crd-compat         # expect seven SANCTIONED servedVersionRemoval lines, zero INCOMPATIBLE
go -C tools tool task test               # unit + envtest, includes scripts/upgrade_path_gate_test.go
go -C tools tool task lint staticcheck vuln
```

Expected: every task exits 0; `crd-compat` prints one `SANCTIONED ^
(servedVersionRemoval)` per promoted CRD and no `STALE` entry for them.

## 2. Fresh install serves only `v1`

```sh
go -C tools tool task test-e2e E2E_ADDONS=fathom
```

The core shard applies the renamed `fathom_v1_*.yaml` samples and asserts
each reaches a verdict. Additional assertions in `test/e2e/crdvalidation_test.go`:

- `kubectl apply` of an alpha-version manifest for a promoted kind fails with
  "no matches for kind".
- replacing an AddonCheck with a copy lacking `spec` fails with
  `spec: Required value`.
- a HealthCheck with `checkRef.apiVersion: example.com/v1` fails naming the
  accepted value.
- two SSA managers each applying one AddonCheck condition leave both present.
- `kubectl get addonchecks,healthreports` shows the new columns.

## 3. Upgrade from the previous release

```sh
go -C tools tool task test-e2e E2E_ADDONS=lifecycle
```

The `upgrade` scenario, in order:

1. `helm install` 0.6.0 from `oci://ghcr.io/skaphos/charts/fathom-operator`
   with its published images; apply the 0.6.0 alpha samples; wait for reports.
2. `fathomctl export --output /tmp/v1.yaml` from the locally built CLI:
   exit 0, one document per check, every document at `v1`, no `paused`,
   `checkRef.apiVersion: fathom.skaphos.io/v1`.
3. `fathomctl upgrade`: exit 1, refusal `acknowledgement-required`, plan shows
   `replace` for seven CRDs with the inventory and counts.
4. `fathomctl upgrade --accept-incompatible-crds`: exit 0; output lists
   removed counts; `kubectl get crd addonchecks.fathom.skaphos.io -o jsonpath='{.spec.versions[*].name}'`
   prints `v1`; `status.storedVersions` is `[v1]`.
5. `kubectl apply -f /tmp/v1.yaml`: every object accepted; verdicts return
   within the sample intervals; `ClusterHealth` aggregates them.
6. `fathomctl export` again: exit 1, `nothing to export`.
7. `fathomctl status`: no "upgrade required" line.

## 4. Retention and the cap

Chart values for the scenario set `retention.report_window: 2m` and
`retention.report_cap_per_kind: 5` so pruning is observed inside the run:

1. Delete an AddonCheck that has reports; within one sweep its reports carry
   `fathom.skaphos.io/source-deleted-at`, `fathomctl reports AddonCheck/<name>`
   lists them as retained, and `fathom_healthreports_retained` is non-zero for
   the namespace.
2. Recreate the check with the same name; it runs, its new reports have a
   different `source-uid`, and `kubectl get healthreports -l fathom.skaphos.io/source-name=<name>`
   shows both generations; the old ones are untouched by the new check's
   history limit.
3. After the window the old reports are gone, the counter
   `fathom_healthreports_pruned_total{reason="window"}` increased, and one
   Event `ReportsPruned` exists on the namespace.
4. Drive a check past five reports: the oldest are removed with reason `cap`,
   one Event per pass.

## 5. Pause is gone

- `kubectl apply` of a NodeCertificateCheck with `spec.paused: true` fails
  with an unknown field.
- `fathomctl run` help text and `describe` output contain no "paused".
- `grep -ri paused docs/ README.md` finds only migration guidance and ADRs.

## 6. Documentation gates

- `docs/reference/api.md` regenerated from `api/` lists both group versions.
- `internal/metrics/contract_doc_test.go` passes with the two new rows in
  `docs/guides/monitoring.md`.
- `grep -rl 'fathom.skaphos.io/v1alpha1' docs README.md` returns only
  `docs/adr/*`, `docs/rfc/*`, the addon-definition guide, and the migration
  section of `RELEASE.md`.
