<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Runtime add-on definitions (qualification preview)

`AddonDefinition` describes ordered, typed checks for one add-on. A namespaced
`AddonDefinitionBinding` delegates that definition to one dedicated reader
ServiceAccount. The built-in adapters remain compiled into the operator. Runtime
loading is off by default, and this workflow is **not yet qualified for release**:
the full Kind installation and rollback test is pending, and the separate
[#256 ratio-contract decision](https://github.com/skaphos/fathom/issues/256)
remains open. Use this guide to review the implementation; do not treat it as a
completed production validation.

The accepted [schema and payload contract](../../specs/012-addon-definition-runtime/contracts/payloads.md),
[numeric limits and lifecycle matrix](../../specs/012-addon-definition-runtime/contracts/runtime.md),
and [leader/drain contract](../../specs/012-addon-definition-runtime/contracts/leadership.md)
are the authoritative details. The [clarification supplement](../../specs/012-addon-definition-runtime/contracts/decision-supplement.md)
requires election even for one replica, treats completed all-Skipped runs as
current Skipped evidence, and makes the target release's CLI inventory decisive
for collision preflight.

## Stage 1: review and install prerequisites

Author exactly one cluster-scoped `AddonDefinition` document. Its
`metadata.name` must equal `spec.addonType`; declare `adapterVersion`, supported
`semanticsVersion: 1`, and ordered `families` and `checks`. Families run in
declaration order, then checks in their declared order. Use exact target
namespaces and explicitly declare cluster scope where needed, including helper
reads. A binding cannot expand a definition's scope.
The generated [Workload definition sample](../../config/samples/addondefinition/workload.yaml)
shows the typed payload for `example-workload`. Its `health` family is disabled
by default. To schedule it, create an `AddonCheck` in a check namespace with
the same add-on identity and enable that family:

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: example-workload-health
  namespace: default
spec:
  addonType: example-workload
  policy:
    health:
      enabled: true
```

The sample definition targets a `Deployment` named `controller` in `default`;
adjust its target and review the corresponding grants for your add-on.

Use the `fathomctl` binary built for the **target operator release**. Before an
upgrade, run `fathomctl definition collisions` against the live cluster. It
prints its version and build revision and compares live definition names and
`addonType` values with its bundled built-in inventory. Exit 0 means no
collision, 1 means collision, and 2 means inventory/version/build metadata or
cluster verification was unavailable. Resolve every collision before upgrade;
the canonical collision blocks both candidates until it is resolved.

Render review-only manifests offline:

```sh
fathomctl definition render --file definition.yaml \
  --service-account custom-reader \
  --operator-namespace fathom-system \
  --operator-service-account fathom-controller-manager > staged.yaml
```

The renderer reads no cluster objects and writes none. Its output includes the
definition, dedicated ServiceAccount, target read Roles or ClusterRoles and
bindings, and a namespaced Role that grants the **actual operator ServiceAccount**
`impersonate` only on `serviceaccounts` with `resourceNames: [custom-reader]`.
Target grants belong to the reader ServiceAccount. Review each rule against the
declared targets and helper reads. The renderer knows fixed built-in resource
mappings; `# MANUAL COMPLETION` comments flag unresolved custom resource
plurals, discovery needs, name overrides, and `requestedReads`. Verify scope,
plural, names and verbs independently, then add the narrow grants in Git.
Declarations alone do not grant access. The final binding template has empty
UIDs and must **not** be applied.

Review and apply only the definition, reader ServiceAccount, and verified grants
through the normal Git-reviewed deployment process. Inspect the live definition
after admission, including defaults. The reader account must be dedicated to
this definition and distinct from the operator account. Do not reuse a built-in
adapter reader account. The operator's ordinary ClusterRole does not gain addon
read permissions; it acts through this exact impersonation grant.

## Stage 2: capture live identity and opt in

After stage 1 objects exist, render a binding with their **live UIDs**:

```sh
fathomctl definition bind --file definition.yaml --name my-addon \
  --service-account custom-reader --operator-namespace fathom-system \
  > binding.yaml
```

`bind` reads the live definition and ServiceAccount, rejects deletion and
built-in collisions, and requires the live spec to match the reviewed file
exactly, including admission defaults. It prints a binding with `enabled: false`
and the target-scope union; it never applies it. Review and install that output
through Git. If either object is recreated, its UID changes and the old binding
does not authorize the replacement; generate and review a new binding.
Edits to the **same definition UID** intentionally inherit that binding, so
review spec changes and their new revision before applying them.

Runtime opt-in is Helm `runtimeLoading.enabled: true`, the operator's
`--runtime-loading-enabled` flag (or
`FATHOM_RUNTIMELOADING_ENABLED` / `runtimeLoading.enabled` through the standard
configuration model). The chart emits the flag only when its value is true;
false leaves the normal environment/config-file precedence intact. The
operator requires `--leader-elect=true` even with one replica, an explicit
operator namespace (`--namespace` or in-cluster `FATHOM_NAMESPACE`), and the
configured leader Lease in that namespace. If a prerequisite is absent, runtime
loading stays inactive and startup logs explain why. Enable the reviewed binding
by changing `spec.enabled` to `true` only after the deployment is configured.
Observe binding `Accepted`/`Ready` and the AddonCheck result before calling the
installation successful. Status is observation, never authority.

Runtime requests use the dedicated reader identity, declared target scope,
bounded direct API reads and the same run deadline for final authority checks.
Denied reads surface as diagnostics rather than widening to operator privileges.
The [runtime contract's numeric inventory](../../specs/012-addon-definition-runtime/contracts/runtime.md#numeric-inventory)
sets the exact limits: among them 256 KiB canonical spec, 16 families, 32 checks
per family, 512 total checks, 32 exact target namespaces, at most 100 requests,
1,000 objects and 100,000 visits per run, a 30-second outer deadline, and at
most four concurrent runtime runs per process. The table also covers parser,
response, evidence, cache, retry and per-request caps; exceeding a cap cannot
produce a partial healthy verdict.

## Changes, drain and rollback

An edited valid definition creates a new revision. Invalid edits, a missing
definition, lost input or revoked access preserve the previous verdict with its
original time and source while freshness becomes `Unavailable` or `Superseded`.
Old evidence is never redated as a new result. A completed run whose every check
is Skipped records **new** `Skipped` evidence with
`coverage: NoChecksEvaluated` and a fresh observation time; it does not retain a
prior Pass as current. Reports remain transition-only.

To revoke, first commit `spec.enabled: false` on the binding. Give the leader
time to cancel work and report `activeRuns: 0`, `Drained=True`, and
`Ready=False/AuthorizationRevoked` for the current binding generation. Then run:

```sh
fathomctl definition drain --name my-addon \
  --operator-namespace fathom-system \
  --leader-election-id 2d3dbc4f.skaphos.io
```

Use the deployment's actual Lease name if it differs. The CLI principal needs
`get` on that **one Lease** in the operator namespace and `get` on that binding;
grant those rights separately. It does not require status writes or grant
management. `drain` independently reads a progressing Lease renewal, the
binding, then the Lease again, and verifies one matching leadership epoch and
binding generation. It reads at most 16 Leases within 15 seconds. Exit 0 means
drained for the observed epoch, 1 means the binding was not acknowledged drained,
and 2 means verification failed or was unavailable. A leader can change after
the final observation: this is **not atomic revocation or a distributed fence**.
Inspect the printed epoch and recheck when making a later decision. Only after
the observed drain should the reader target grants and impersonation grant be
revoked.

For operator rollback, disable each binding and verify its drain **while the
current runtime loader and leader election are still running**. Revoke the
reader and impersonation grants, then export the complete runtime state before
turning runtime loading off or starting an older binary. Do not delete CRDs as a
shortcut. Fathom v0.5.1 does not have the newer
`status.lastSuccessfulEvaluation`, freshness, or `latestAttempt` fields; an older
binary may replace or remove those status fields. Reports are transition-only and
may not contain the newest same-verdict revision or evidence. Store the exported
files outside the cluster before replacing the binary.
Back up complete `AddonCheck` objects and status, not only `HealthReport` history:

```sh
backup_dir="fathom-runtime-backup-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$backup_dir"
kubectl get addonchecks.fathom.skaphos.io -A -o yaml >"$backup_dir/addonchecks.yaml"
kubectl get addonchecks.fathom.skaphos.io -A -o json >"$backup_dir/addonchecks.json"
kubectl get healthreports.fathom.skaphos.io -A -o yaml >"$backup_dir/healthreports.yaml"
kubectl get addondefinitions.fathom.skaphos.io -o yaml >"$backup_dir/addondefinitions.yaml"
kubectl get addondefinitionbindings.fathom.skaphos.io -A -o yaml >"$backup_dir/addondefinitionbindings.yaml"
```

Keep the JSON export unchanged; it supplies the exact pre-downgrade status for
each check. After an older-binary rollback, stop **all** old operator pods and
bring up the new binary with runtime loading still off. Keep bindings disabled
and the reader and impersonation grants revoked. Restore a check only if its
live UID and generation still match the exported object. The JSON Patch also
tests the current resourceVersion, so a concurrent write fails instead of
silently overwriting it. Run this once per affected AddonCheck, setting the
namespace, name and backup directory to the values used for the export:

```sh
check_namespace=example
check_name=my-addon
backup_dir=/secure/path/to/fathom-runtime-backup-YYYYMMDDTHHMMSSZ
snapshot_file=$(mktemp)
live_file=$(mktemp)
patch_file=$(mktemp)
jq -e --arg ns "$check_namespace" --arg name "$check_name" \
  '[.items[] | select(.metadata.namespace == $ns and .metadata.name == $name)]
   | if length == 1 then .[0] else error("expected one saved AddonCheck") end' \
  "$backup_dir/addonchecks.json" >"$snapshot_file" || exit 1
kubectl -n "$check_namespace" get addonchecks.fathom.skaphos.io "$check_name" \
  -o json >"$live_file" || exit 1
jq -n -e --slurpfile saved "$snapshot_file" --slurpfile live "$live_file" '
  $saved[0] as $s | $live[0] as $l |
  if $s.metadata.uid != $l.metadata.uid or
     $s.metadata.generation != $l.metadata.generation or
     ($s.status.lastSuccessfulEvaluation == null)
  then error("UID/generation changed or saved evidence is missing")
  else [
    {op:"test", path:"/metadata/uid", value:$l.metadata.uid},
    {op:"test", path:"/metadata/generation", value:$l.metadata.generation},
    {op:"test", path:"/metadata/resourceVersion", value:$l.metadata.resourceVersion},
    {op:"replace", path:"/status", value:$s.status}
  ] end' >"$patch_file" || exit 1
kubectl -n "$check_namespace" patch addonchecks.fathom.skaphos.io "$check_name" \
  --subresource=status --type=json --patch-file "$patch_file" || exit 1
kubectl -n "$check_namespace" get addonchecks.fathom.skaphos.io "$check_name" \
  -o json | jq -e --slurpfile saved "$snapshot_file" \
  '.status.lastSuccessfulEvaluation == $saved[0].status.lastSuccessfulEvaluation' || exit 1
rm -f "$snapshot_file" "$live_file" "$patch_file"
```

Review the restored `lastSuccessfulEvaluation.observedAt`, revision and authority
against the export; leave existing HealthReports untouched. If the UID,
generation or resourceVersion test fails, stop and investigate rather than
replacing the live object or retrying with a stale snapshot. The bounded
source-built v0.5.1 downgrade, guarded restore and fresh same-verdict re-enable
trial passed; see the [operations qualification record](../../specs/012-addon-definition-runtime/operations-qualification.md).
This is not a released chart upgrade qualification or historical new-builtin
test. Before re-enabling, use the target release's collision preflight again,
inspect live
definition and ServiceAccount UIDs and spec, review grants and binding scope,
confirm leader election/Lease configuration, and verify new binding readiness and
a fresh completed run. Recreate the binding through `definition bind` if a UID
changed. These steps do not resolve the separate #256 release dependency or
claim the pending full-cluster qualification has passed.

The new definition resources follow their own alpha schema track. The
[`v1alpha1` to `v1` freeze issue #149](https://github.com/skaphos/fathom/issues/149)
is separate and does not substitute for the #256 ratio-contract disposition.
