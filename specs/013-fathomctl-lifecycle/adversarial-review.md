<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Adversarial Review: fathomctl Lifecycle Verbs (spec 013)

**Result: PASS** — 2026-10-12T04:33:50Z

No P0/P1/P2 finding remains unresolved; every P3 has a recorded disposition.
**Condition:** the `lifecycle` e2e shard (T052) has never executed — no
Linux/Cilium-capable environment was available locally — so the PR test plan
must say so and rely on the `kind e2e` CI run. This record is workflow
assistance, not a CI gate.

## Reviewer independence

A fresh reviewer subagent that did not write the implementation performed the
review, read-only, in three passes (initial review, re-review of the
remediation, confirmation of the follow-up fixes). The implementation
coordinator remediated every finding; the reviewer verified each fix.

## Base and scope

- Base: `origin/main` `039a0073b4a823404df8ecaf3b2fe5a3d9d7871e` — the PR
  target, validated as the merge-base of `HEAD`.
- Reviewed commits (`039a007..1a50dc5`): `018e5fe`, `9e169c0`, `7d5a130`,
  `5310b0b`, `26abfd7`, `ec6fd3a`, `420d8ac` (spec artifacts), `1d1b4ea`
  (install/upgrade and foundations), `1a50dc5` (uninstall/status, e2e shard,
  docs) — 67 files, +10,286/−81.
- Remediation reviewed as the working-tree diff on top of `1a50dc5`: 17 files,
  +452/−54 (diff sha256 prefix `5acb7aef834c4215`), committed with this
  record.
- Untracked/excluded: `graphify-out/` in the main checkout (generated graph
  output, unrelated to the feature).

## Verification

Run by the coordinator on macOS arm64, Go 1.27.2, after remediation:
`task fmt`; `task lint` (golangci-lint, 0 issues); every non-e2e package with
envtest 1.37 (`internal/cli` 87.4% coverage, `deploy/helm` 80.0%);
`scripts/check-coverage.sh`; `task staticcheck`; `go vet ./test/...`;
`reuse lint`. Earlier in the same scope: `task vuln` (0 reachable
vulnerabilities), `task build`, `task verify-generated`, `task crd-compat`,
`helm lint`. The F1 regression test was run red against the pre-fix code
("nil pointer evaluating interface {}.enabled") and green after.

### Limitations

- e2e not run: every shard installs Cilium first, and its `mount-bpf-fs` init
  container crash-loops under podman on macOS, so the node never becomes
  Ready. The lifecycle shard's real-cluster assertions (CRD writes before the
  new ReplicaSet, cleanup, Helm interop, purge/export/restore) are unverified.
- `task test` cannot resolve `setup-envtest@v0.25.2` in this environment (a
  pre-existing toolchain issue, the Taskfile line is unchanged); the identical
  test command ran with `setup-envtest@v0.25.1`.
- The reviewer read code and the Helm v4.3.0 source; it did not run tests.

## Findings and dispositions

| ID | Severity | Finding | Disposition |
|---|---|---|---|
| F1 | P1 | `upgrade` used Helm `ReuseValues`, which substitutes the previous chart's defaults: upgrading from v0.5.x would fail to render after the CRDs were changed, and Render validated different values than Upgrade applied. | Fixed — `ResetThenReuseValues` (`lifecycle_helm.go`); recovery text on a post-CRD failure; regression test `TestHelmReleaseClientUpgradeKeepsTheNewChartsDefaults` (red→green). |
| F2 | P1 | The rollout wait accepted the Deployment's Available condition while a surged new pod failed, so a broken upgrade exited 0 and `--rollback-on-failure` never fired. | Fixed — `rolloutComplete` with `kubectl rollout status` semantics and progress-deadline fail-fast (`lifecycle.go`); `TestRolloutComplete`, `TestUpgradeDetectsAStuckSurge`. |
| F3 | P2 | Uninstall cleaned up while the terminating manager could still recreate objects. | Fixed — `waitManagerGone` before cleanup; `TestWaitManagerGone`, `TestUninstallStopsBeforeCleanupWhileTheManagerRuns`. |
| F4 | P2 | A same-version `upgrade` dropped `-f`/`--set` and reported success. | Fixed — no-op only without user values; `TestUpgradeSameVersionWithValuesUpgrades`. |
| F5 | P2 | The e2e shard's fixed versions would read as a downgrade after a `Chart.yaml` bump. | Fixed — versions derive from `Chart.yaml` (`<appVersion>-e2e.1/.2`); Helm interop packages the chart at the CLI version. Unverified until the shard runs. |
| F6 | P2 | A failed first install was told to `helm rollback`, which cannot work. | Fixed — `pendingRefusal` points at uninstall-then-install for revision ≤1, pending-install and uninstalling; install failure carries recovery text; `TestPendingRefusalForAFailedFirstInstall`. |
| F7 | P3 | Preflight omitted `patch` and Secret reads; `--dry-run` needed write access; the table plan omitted values in effect. | Fixed — all three; `TestLifecyclePermissionsFollowTheFlags`, `TestUpgradeDryRunNeedsNoWritePermissions`. |
| F8 | P3 | A release mid-uninstall was invisible and its leftover operator treated as non-Helm. | Fixed — `ListUninstalling` in the mask; `TestUninstallFinishesAnInterruptedHelmUninstall`. |
| N1 | P2 | After an uninstall failed post-release-removal, a rerun said "nothing to remove" and orphaned operator-created objects. | Fixed — rerun detects and removes leftovers; actionable `next:`; `TestUninstallRerunCleansUpLeftovers`. |
| N2 | P3 | Post-upgrade-failure recovery suggested a retry that `release-pending` would refuse. | Fixed — says to `helm rollback` a failed latest revision first. |
| N3 | P3 | The rerun-after-failed-uninstall path cannot find the leader-election Lease (no release namespace or values). | Accepted — the Lease is inert without an operator and a reinstall takes it over once it expires. |
| N4 | P3 | On that path the permission preflight and `--delete-namespace` are skipped (no namespace is known). | Accepted — the cleanup is idempotent and safe to rerun; deleting a namespace the CLI cannot identify would be the riskier behaviour. Owner: spec 013 follow-up if users hit it. |
| N5 | P3 | On that path the CLI cannot wait for a manager it cannot identify. | Accepted — only reachable with a pod stuck beyond `--timeout`, which the error's force-delete hint covers. |
| N6 | P3 | `redactValues` does not descend into lists. | Accepted — no chart value carries a secret inside a list today. |
| N7 | P3 | `waitBounded` aborts on a transient API error instead of retrying. | Deferred — bounded and reported; revisit with real-cluster e2e data. |

## Re-review history

1. Initial review of `039a007..1a50dc5`: **BLOCKED** (F1, F2 P1; F3–F6 P2;
   F7, F8 P3).
2. Re-review of the F1–F8 remediation: **PASS** recommended; new N1 (P2) and
   N2 (P3) raised.
3. Confirmation of the N1/N2 fixes: **PASS**; P3 edge cases N3–N5 recorded
   above.
