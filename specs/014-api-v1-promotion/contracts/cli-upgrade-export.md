<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Contract: `fathomctl export` and the `replace` CRD action

Extends `specs/013-fathomctl-lifecycle/contracts/cli-lifecycle.md` and
`upgrade-path.md`. Everything not stated here is unchanged.

## `fathomctl export`

Read-only. Renders the live alpha check resources as `v1` manifests.

```text
fathomctl export [--output <file>] [--namespace <ns>] [global flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--output` | stdout | write the multi-document YAML to this file atomically |
| `--namespace` | all namespaces | restrict namespaced kinds to one namespace |

Behaviour:

1. Reads the Fathom CRDs. For each of ClusterHealth, AddonCheck, DNSCheck,
   NodeCertificateCheck, NodeHealthCheck and HealthCheck, notes which version
   the live CRD serves.
2. If none of them serves `v1alpha1`: prints
   `nothing to export: no Fathom CRD serves v1alpha1 (already upgraded?)` to
   stderr, exit 1, no output written.
3. Lists every object of each alpha-served kind (unstructured, at
   `v1alpha1`), in kind order ClusterHealth, AddonCheck, DNSCheck,
   NodeCertificateCheck, NodeHealthCheck, HealthCheck, then namespace, then
   name.
4. Renders each object: `apiVersion` set to `fathom.skaphos.io/v1`;
   `spec.paused` removed; `spec.checkRef.apiVersion` set to
   `fathom.skaphos.io/v1` if present; `status` removed; metadata reduced to
   `name`, `namespace`, `labels`, `annotations` (the existing
   `sanitizeForExport` plus the `kubectl.kubernetes.io/last-applied-configuration`
   annotation).
5. Writes the documents `---`-separated, preceded by a comment header naming
   the source cluster context and the time.
6. An object that cannot be rendered (list failure, decode failure) is
   reported on stderr as `cannot export <kind>/<ns>/<name>: <reason>`; the
   rest are still written; exit 1.
7. Exit 0 when every object rendered. Never writes to the cluster.

HealthReports, AddonDefinitions and AddonDefinitionBindings are never
exported by this verb.

## `upgrade` and `install`: the `replace` action

Chosen for a CRD when the target drops any version listed in the live CRD's
`status.storedVersions`.

| Condition | Result |
|---|---|
| no upgrade-path entry with `path: "*"` sanctions the CRD on the path | refusal `unsanctioned-incompatible` |
| sanctioned, `--accept-incompatible-crds` absent | refusal `acknowledgement-required`; the plan output lists the inventory below so the administrator sees what the flag will remove |
| sanctioned, flag present | the CRD is deleted, its deletion awaited, the target created and awaited `Established`; the removed object count is printed |
| the release is a `stop` on the path | existing `stop-skipped` rules apply first |

Inventory printed before any change, once per CRD to be replaced:

```text
replace crd/addonchecks.fathom.skaphos.io (v1alpha1 -> v1): 12 objects will be removed
  default/coredns
  default/cert-manager
  ...
  and their HealthReport history
```

The inventory is listed at the live served version; if the live CRD cannot
be listed the plan says so and still refuses without the flag. With the flag,
a CRD that is gone by the time of deletion is treated as already replaced.

`--dry-run` prints the plan, including the inventory, and changes nothing.

`--rollback-on-failure` with any `replace` action is a usage refusal (as it
already is for incompatible changes).

## `status`

Reports the served version per Fathom CRD. On a cluster where a promoted kind
still serves `v1alpha1`, `status` prints
`upgrade required: <crd> serves v1alpha1; run fathomctl export, then fathomctl upgrade`.

## Exit codes

Unchanged: `0` success, `1` any refusal or error.

## Documentation

`docs/reference/fathomctl.md` gains a top-level `## export` section beside
`ls`, and the Lifecycle verbs section gains the `replace` row under CRD
handling and the new refusal reason in the Refusal reasons table.
