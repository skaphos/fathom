<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Contract: fathomctl Lifecycle Verbs

Global flags are unchanged (`--kubeconfig`, `--context`, `-n/--namespace`,
`--request-timeout`, `-o`). Exit codes: `0` success, `1` anything else. Every
refusal happens before the cluster is changed and ends with a `next:` line
holding the exact command to run.

Refusals common to install, upgrade and uninstall: `missing-permissions`
(a SelfSubjectAccessReview pre-check names each missing permission),
`multiple-installs` (names each Fathom release, FR-008), `not-helm`
(names kustomize or OLM, FR-006).

## Shared flags (install, upgrade)

| Flag | Default | Meaning |
|---|---|---|
| `-f, --values <file>` | — | Values file(s), repeatable, Helm semantics. |
| `--set k=v` / `--set-string` / `--set-json` | — | Helm-semantics overrides. |
| `--timeout <duration>` | `5m` | Bound on every wait (CRD Established, rollout). |
| `--release <name>` | `fathom` | Release name for `install`; discovered for the others. |

Image references are set by the CLI from its own digests and are not
overridable through these flags (they would contradict the exact-version
policy); an attempt is a usage error naming the reason.

## `fathomctl install`

`fathomctl install [-n <ns>] [--create-namespace] [shared flags]`

| Situation | Outcome |
|---|---|
| No install | CRDs applied, release installed at the CLI version, waits for Available → `0`. Prints version, release, namespace, image references. |
| Install at the CLI version | `already installed at vX; nothing to do` → `0`. |
| Older install | refuse `use upgrade` → `1`; `next: fathomctl upgrade -n <ns>`. |
| Newer install | refuse → `1`; names the matching `fathomctl` release. |
| Non-Helm install | refuse → `1`; names `kustomize`/`OLM`. |
| Namespace missing, no `--create-namespace` | refuse → `1`. |
| Not Available in time | `1`, unmet condition, recovery commands. |

## `fathomctl upgrade`

`fathomctl upgrade [--dry-run] [--accept-incompatible-crds] [--rollback-on-failure] [shared flags]`

Plan output (also for `--dry-run`, and `-o json|yaml` as the Upgrade plan
entity in [../data-model.md](../data-model.md)):

```text
Fathom upgrade plan: v0.6.0 -> v0.7.0 (release fathom in fathom-system)
Path:      direct
CRDs:
  addonchecks.fathom.skaphos.io        update
  dnschecks.fathom.skaphos.io          update   incompatible (sanctioned by v0.7.0, #401): ^.spec.targets maxItems 16 -> 8
  nodehealthchecks.fathom.skaphos.io   unchanged
Operator:  ghcr.io/skaphos/fathom-operator@sha256:… -> @sha256:…
```

| Situation | Outcome |
|---|---|
| Compatible changes | CRDs applied, then rollout, waits → `0`. |
| Sanctioned incompatible, no flag | refuse `acknowledgement-required` → `1`; lists each change with reason and issue; `next:` repeats the command with `--accept-incompatible-crds`. `--yes` does not count. |
| Unsanctioned incompatible | refuse `unsanctioned-incompatible` → `1`; CRD, path, crdify message; manual path. |
| Scope change | refuse `scope-change` → `1`; explains delete-and-recreate (which deletes every resource of that kind) and points at `uninstall --purge-crds --export`. |
| Stop skipped | refuse `stop-skipped` → `1`; `Upgrade path: v0.5.1 -> v0.6.0 -> v0.7.0`; `next: download fathomctl v0.6.0 and run "fathomctl upgrade -n fathom-system"`. Not bypassed by `--accept-incompatible-crds`. |
| CLI older than install | refuse `downgrade` → `1`; names the matching `fathomctl` release. |
| Release pending/failed | refuse `release-pending` → `1`; recovery via `helm rollback` or `helm history`. |
| `--rollback-on-failure` with any incompatible CRD change in the plan | usage refusal before any change → `1`. |
| Not Available in time | default: release left, `1`, unmet condition + recovery (`kubectl -n <ns> describe deploy/<name>`, longer `--timeout`, `helm rollback <release> -n <ns>`); with `--rollback-on-failure`: operator rolled back, then `1`. |

## `fathomctl uninstall`

`fathomctl uninstall [--purge-crds [--export <file>]] [--delete-namespace] [--yes]`

| Situation | Outcome |
|---|---|
| Plain | release removed; operator-created objects (data-model "Operator-created objects") removed; CRDs and custom resources kept; namespace kept and reported → `0`. |
| `--purge-crds` | prints counts per kind (nine kinds), mentions `--export` when absent, prompts on a TTY; `--yes` skips; no TTY and no `--yes` → refuse `1`. Then release + objects removed, every custom resource deleted, CRDs deleted and their removal awaited. |
| `--export <file>` | file written atomically first; on write failure nothing is deleted → `1`. |
| `--delete-namespace` | namespace deleted after the release; non-Fathom objects listed and confirmed first (same prompt/`--yes`). |
| No install | `nothing to remove` → `0`. |
| Non-Helm install | refuse → `1`. |
| CLI older than install | allowed (uninstall needs no chart of another version). |

## `fathomctl status`

`fathomctl status [-o table|json|yaml]` — read-only; needs get/list on Helm
release Secrets, CRDs and the manager Deployment.

```text
Fathom:     installed (helm release fathom in fathom-system, revision 3, by fathomctl)
Version:    v0.6.0 (images pinned by digest)
Operator:   Available
CRDs:       9/9 current
Upgrade to this fathomctl (v0.7.0): direct
```

Exit `0` when installed and current, or not installed; `1` when installed but
not Available, CRDs not current, the release is pending/failed, or more than
one release exists. JSON/YAML is the Install entity plus `crds[]` and
`upgradePath`.
