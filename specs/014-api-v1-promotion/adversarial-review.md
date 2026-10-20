<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Adversarial review — spec 014, api/v1 promotion

**Result: PASS**

- Run 1 snapshot: 2026-10-20T17:42Z (two independent reviewers). Remediation
  snapshot: 2026-10-20T18:14Z. Re-review: see below.
- Reviewer independence: each review was a fresh `general-purpose` subagent
  with read-only instructions (no edits, no gates, no kind/docker); none had
  implemented the feature. The implementation coordinator (the worker that
  wrote the feature) verified every finding against the code, applied the
  remediation, and wrote only this record.
- This hook is workflow assistance, not CI enforcement: nothing in the
  repository mechanically gates on this file.

## Base evidence

- Target base: `main` = `3b83bb0b0093de8e6651a54629074fcff3503531`, equal to
  `origin/main` and to `git merge-base main HEAD`; the merge-base is an
  ancestor of `main`. Feature branch `feature/149-api-v1-promotion`, HEAD at
  review time `bc77908d1bd8261cf28a61d09cfd2b0b4058a6cf`.
- Delivery shape (research R16): four stacked PRs. Commits 6bbcdb6 (PR 1),
  e04a942 (PR 2) and bc77908 (PR 3) were committed; the retention work (PR 4)
  was the uncommitted working tree at review time.

## Commits merge-base..HEAD
- bc77908 feat(fathomctl): replace stored-version CRDs on upgrade and export alpha checks as v1 (spec 014)
- e04a942 feat(api)!: promote the check kinds to a stable fathom.skaphos.io/v1 (spec 014)
- 6bbcdb6 refactor: prepare the tree for the api/v1 promotion (spec 014, phase 2)
- 2871447 docs(spec): remediate spec 014 analysis findings
- ba3712f docs(spec): tasks for spec 014 api/v1 promotion
- 600847f docs(spec): plan spec 014 api/v1 promotion
- 8363b3d docs(spec): clarify spec 014 upgrade export, report retention and pruning
- 3be4904 docs(spec): resolve spec 014 clarifications
- 4420b2d docs(spec): draft spec 014 for the api/v1 promotion (#149, #262, #257)

## Scope snapshot 1 (run 1)

- Committed paths, merge-base..HEAD: 254 (full name-status in the coordinator's
  `review-snapshot.md`).
- Working tree: 37 modified tracked paths and 4 untracked paths, fingerprinted
  below. Exclusions: `graphify-out/*` (generated, reviewed by nobody);
  `config/manager/kustomization.yaml` (rewritten transiently by the concurrent
  e2e shard; clean at snapshot time).

## Fingerprints of dirty tracked and untracked files (sha256)
2009735fefdfbba42e0d72e1f19ee271d4c048f32deb40e713ccb49e84821753  api/v1/annotations.go
c68943138b9ab36678133fa3ae2e0541e913d809afd5beafe2ae2f84afc8c363  config/rbac/role.yaml
ae44ed05d74142b927b58c4f52e8aab3879c4edca143f0f72601d5c6c6a980bb  deploy/helm/fathom-operator/files/manager-rules.yaml
d2a5588d417ca08d876e3453a48f7ef5246287048c70e2a0a159c81b8b5a6720  docs/adr/0002-healthreport-as-first-class-crd.md
3bda39513a0025a99ffa4927d9a8322dffa48a281b559fdc8953f7325fc4e382  docs/guides/addon-checks.md
ac69d03b6ca1dd03c30973052ffd84466ba519f907f3963540a176994b12d70b  docs/guides/dns-checks.md
6e5c701f6d8734e757e05aa65091637c69cb3104e2d7a393ee5e86402c47de6c  docs/guides/monitoring.md
5df80c1dbd149ed1bb24c823f5421ae504ba403bf66a5e19dc50a02fb2539ae5  docs/guides/node-certificate-checks.md
f432a67db35883de7bc817b7463deec4307894f93b3514926c5f66795979a952  docs/guides/node-health-checks.md
c2a382c472c3615b8fd4d1030494fb49b7172dcd7970055f6691a6ce0287ac0e  docs/reference/configuration.md
6dc436b47ab83a25ec478515a922ec417db60f1b26f88e3f89037c49a7d7d656  docs/reference/operator-rbac.md
e22ae77ed223e4e1460bd09c7cbb23618af9ec8e2769be1d640218b566fed1cb  docs/reference/status-conditions.md
43022ba0da1bfb811b4fd3e2c566736bf403bfab58c6dff34298b76b090c80cf  graphify-out/GRAPH_REPORT.md
dcbbf95f0c49dad752ecffd2744aed2b82f46db95ad56f50fb7b929c0a7bd7bc  graphify-out/graph.html
4b3da42cc266990580d41269b5a749e239da9ebc17166981a54f352dcfc79d80  graphify-out/graph.json
b7ad397d6d27c19b7b064f8c93490a82f973fd932a5a7e2d468533190e79c278  graphify-out/manifest.json
9ebec56ddd5ec8a09a47a5e998af6cd56b091a80f9a345c5cf554b92134dd5c2  internal/app/options.go
df378765de9d175538cb256c65e31cbd4c6e59a673f8052c87c77d23971c3e20  internal/app/run.go
4880175f13f9041cd236f8e14123d59bbff5654fbffadd69008971b4eb49f80a  internal/app/runtime_wiring_test.go
df8deac557c4ad1cfda77728a750614e12dbbf3a2fffd0706dd696aeee30e4d0  internal/cli/reports.go
c5d656a31422fd83201072253248b969bd6e7e61dbf9c02e19888fbf5083f220  internal/cli/reports_test.go
7d3e81474a8d828125de3debd71174f007ef60c8779d82dc03778c1f7f810d58  internal/controller/addoncheck_controller.go
b9e472a28a6ab97923c8647ea9fce967971c765a7bd0aab6fc0d2ad5056ee420  internal/controller/addoncheck_controller_test.go
e4aa376a0122005a140bcf480e8a76acb6e1943c723a1ca9bbf3eb74c0b7ac66  internal/controller/dnscheck_controller.go
87a614ae87d62bb29dfad090cf8b8fb6fc8a99a081821f0c1e8dfd56cddb32e0  internal/controller/healthreport_history.go
c520d114fee6de78dfa20d0b2d0640a8ce2192df021366c0b1b93c1488e3a3dd  internal/controller/healthreport_history_test.go
0bbc6a31392a6c8a8b189986236eccbebb3e6dde65d287ed353a04a5be3f326d  internal/controller/nodecertificatecheck_controller.go
7850d19a6ea318b09fef839a5b714fc8644314a4a8926bcce6a986e122c4066e  internal/controller/nodecertificatecheck_helpers.go
0d1e071b59459af2c1b3a027d1eb672880a95088c1e4114eb43d6f376c4ba11d  internal/controller/nodehealthcheck_controller.go
877c894065849703bec9f179c89064bc4b3189d591a6728a2025fbfb4262322e  internal/controller/nodehealthcheck_controller_test.go
400120d9fb09970e97f495703e5486224781f55beb9c273f1aadb9d61769b4ee  internal/controller/nodehealthcheck_helpers.go
6907a2df742ad0ba5be9543c551f47013559d77ca282403ab6c6b92e25ffe25e  internal/metrics/contract_doc_test.go
77ea64201f7b5cfc36936e50fd07297ec6e93961f5bfc3f4680160eca263f5a8  internal/metrics/metrics.go
9f8452d28ecbcf84ec9fbd8507fe3bf4bbb3ee20c05af90bd45fb611ad1eb061  specs/014-api-v1-promotion/contracts/retention.md
28ed7717c0a3ca6be63d7ec7e8394f2c72cc0351b9b369edd21423d4ac54ed29  specs/014-api-v1-promotion/research.md
3fa27b5795985125c36ebc7b47d22b428b143805f2c9eaca577af18cfd1092df  specs/014-api-v1-promotion/tasks.md
715cac9bb4f8f823d6c2e36877108be3dad1f884d5fb63b1b4cc1a8dd5d6d03d  test/e2e/fathomctl_lifecycle_test.go
a671c1e4ffae386e6ae76e993c9e3d624dcb666f45fd360874113728ba66daeb  docs/adr/0010-healthreport-retention.md
84523ec4d902ff8fc3d91db93b42bad45127279fa7c756d2dcf70ec5a237933b  internal/app/options_retention_test.go
ab9e44c4770e2ed7f456f227a3c1727e20075f8d05cfeab9d1fc691393dcfb7a  internal/controller/healthreport_retention_controller.go
d6c2e54caeb780d231092bc928249620fede0a040473adff3d80a691a03ddd49  internal/controller/healthreport_retention_controller_test.go

## Verification performed

Reviewer A (commits 1–3): `go build ./...`, `go vet` on cli/scripts/api,
`go test ./internal/cli/` (replace/export/status/install/upgrade/purge cases),
`go test ./scripts/` (gate, upgrade-path, shards); a gate-bypass experiment
(a v1→v1 `maxLength` tightening on addonchecks passed the gate as SANCTIONED
under the committed `"*"` entry — finding A1); source checks of crdify v0.6.0
and apiextensions-apiserver v0.37.1; greps for alpha references; byte
comparison of the chart `crds/` with `config/crd/bases`.

Reviewer B (retention): `go build`, `go vet`, the focused envtest retention
specs (8 passed), the prune helper and app/cli/metrics unit tests.

Coordinator, before and after remediation: `task test` (full envtest), lint,
staticcheck, vuln, build, crd-compat (seven SANCTIONED servedVersionRemoval
findings, none INCOMPATIBLE, with the narrowed `"*"`), generator idempotence
over the tree; e2e on kind: lifecycle shard (E2E_ADDONS=lifecycle) 15 passed, 0 failed (alpha-to-v1 upgrade and retention scenarios, run three times across the remediation rounds); core shard (E2E_ADDONS=core) 83 passed, 0 failed, 44 skipped (other addon shards, left to CI). `verify-generated` runs against the
commits (it diffs the tree against HEAD, so it cannot pass on uncommitted hand
edits).

## Findings and dispositions (run 1)

| # | Sev | Finding | Disposition | Fix (file) |
|---|-----|---------|-------------|------------|
| A1 | P2 | `"*"` allowlist entries sanction any later incompatible v1 change (gate matched `*` against every finding on the CRD) | fix | `scripts/check-crd-compat.sh`: `*` matches only `servedVersionRemoval`; property findings need their own entry. Test `TestCompatGateStarDoesNotSanctionPropertyFindings` (tightened `maximum` on a v1→v1 CRD with a `*` entry → INCOMPATIBLE, exit 1, entry STALE). Allowlist header reworded. |
| A2 | P2 | Same-version `fathomctl upgrade` (release already 0.7.0 via `helm upgrade`, CRDs still store v1alpha1) dead-ends on `unsanctioned-incompatible` while `status` sends the user there | fix | `internal/cli/lifecycle_path.go` `sanctionedBy`: when from == to, the target release's own entry counts. `unsanctionedRefusal` for a replace now points at RELEASE.md "v1 API migration". Tests `TestUpgradeAtTheSameVersionStillSanctionsTheReplace`, `TestUpgradeReplaceWithoutASanctionPointsAtTheManualSequence`. |
| A3 | P3 | `export` renders an alpha object stored without `spec` as a spec-less v1 manifest, which v1 rejects at apply | fix | `internal/cli/export.go` `renderExported`: missing or null spec becomes `{}`. Test `TestExportGivesASpeclessObjectAnEmptySpec`. |
| A4 | P3 | Upgrade preflight never asks for `delete` on CRDs, which a replace needs; failure would be part-way | fix | `internal/cli/upgrade.go`: when the plan has a replace, a second preflight for `delete customresourcedefinitions` runs before any write. Test `TestUpgradeReplaceRefusesWithoutDeleteOnCRDs`. |
| A5 | P3 | Post-failure recovery text recommends `helm rollback` after a replace, which cannot restore a working operator | fix | `rollbackHint`/`upgradeRecovery(…, replaced)`: after a replace the hint names RELEASE.md's procedure and never spells the rollback command. Test `TestUpgradeFailureAfterAReplaceDoesNotSuggestHelmRollback`. |
| A6 | P3 | `status` contract/reference promised an `upgrade required:` prefix the code never emits; e2e asserted the absent string (vacuous); permission note stale | fix | Contract and `docs/reference/fathomctl.md` aligned to the code (`outdated`, reason `serves v1alpha1; …`); e2e asserts the positive string before the upgrade and its absence after; `status.go` help notes the inventory list. |
| A7 | P3 | `export` header omitted the source cluster context the contract promised | fix | `factory.contextName()` (`--context`, else the kubeconfig's current context); header says `from context <name>`; asserted in the new export test. |
| A8 | P3 | Pause remnants: `run.go` timeout hint and comment, `operator-rbac.md` DaemonSet `delete` justification ("a paused check"), PrometheusRule description in both rule files | fix | Reworded; DaemonSet `delete` re-justified by the provisioning-failure revocation (`revokeNodeCertAgent`). (`reports.go` lines cited turned out to be the generic `Skip` handling, not a pause branch: no change.) |
| A9 | P3 | RELEASE.md quoted crdify's error text for the manual path, not the API server's | fix | RELEASE.md quotes the server's `status.storedVersions[0]: Invalid value …` message. |
| A10 | P3 | e2e and Taskfile pin the scenario CLI to the newest upgrade-path entry rather than the newest stop | fix | `upgradeStopVersion()` and the Taskfile awk select the newest `stop: true` entry. |
| B1 | P2 | One refused stamp or delete aborted the whole namespace pass, halting window and cap enforcement for that namespace | fix | `healthreport_retention_controller.go`: writes are tolerant; failures are collected and returned after the rest of the pass. Test "keeps sweeping past a report it cannot stamp or delete" (a client that refuses one report's Patch/Delete). |
| B2 | P3 | Stamping was unbudgeted (a startup burst of up to cap×kinds PATCHes) | fix | Stamps count against the per-pass budget (`retentionWriteBudget`, 50 writes); test "stamps at most the budget per pass and continues shortly after". |
| B3 | P3 | `fathom_healthreports_retained` series never removed; `DeleteRetentionSeries` dead; ADR claim false | fix | An empty namespace drops its series; test "drops the namespace's gauge series once it holds no reports"; ADR 0010 reworded. |
| B4 | P3 | The cap could delete a live check's `status.lastReportName` report | fix | `checkExists` also returns the live check's latest report name; the cap spares it. Test "never caps away the report a live check's status still names". Contract, ADR, configuration reference updated. |
| B5 | P3 | Docs/design artifacts still described owner-referenced, garbage-collected reports and "requeue at once"; AGENTS.md reconciler list stale | fix | `docs/architecture.md`, `docs/guides/node-certificate-checks.md`, `data-model.md`, ADR 0010, `docs/reference/configuration.md`, two e2e comments, AGENTS.md updated. |
| B6 | P3 | T055 claimed e2e coverage (recreate-with-new-UID, cap prune, counters) the shard does not assert | fix (scope stated) | T055 reworded to what the shard proves; those behaviours are covered by the envtest retention suite. |

Reviewer-confirmed sound (no change): replace ordering and partial-failure
idempotency, install over leftover alpha CRDs, stop-skipped refusal, export
sanitisation and ordering, scheme/GV wiring, RBAC within the committed range,
generated artifacts, ClusterHealth derivation, v1 admission deltas, orphan
detection and UID boundary, stamp patch semantics and the `patch` verb, window/
cap/budget arithmetic, the HealthReport create watch as the resync trigger,
Event and metrics plumbing, options validation and precedence, the Helm
`config.data` path, the CLI column, builder label coverage.

## Remediation and scope snapshot 2

All sixteen findings were remediated in the working tree (dispositions above).
The committed-side fixes land as a `fix(cli)` commit on PR 3's branch; the
retention fixes land in PR 4's commit. Fingerprints after remediation:

## Fingerprints of dirty tracked and untracked files (sha256)
841fd5e546c4e111872ddd2b5c51d6e0d6caf9439ee3c0329844afd93ae4330a  .crd-compat-allowlist.yaml
b1644293577fca4e4314a5b3aa4e8f50c47da0c19a39b46c625c7682ccf239c8  AGENTS.md
b28598c4c37fc34e93536581544caa78e967fa52647d23124a422f46e7d75c2e  RELEASE.md
ce2975e354127d0c7aa842a49c234df0859b5df76f1ff8a0279568d78d2fccc5  Taskfile.yml
2009735fefdfbba42e0d72e1f19ee271d4c048f32deb40e713ccb49e84821753  api/v1/annotations.go
e08734fde92ee5f13c07b7a0c50732cf8d5d6c434c44dc0f920213f2328c2e84  config/components/prometheus-rule/prometheusrule.yaml
c68943138b9ab36678133fa3ae2e0541e913d809afd5beafe2ae2f84afc8c363  config/rbac/role.yaml
ae44ed05d74142b927b58c4f52e8aab3879c4edca143f0f72601d5c6c6a980bb  deploy/helm/fathom-operator/files/manager-rules.yaml
ce3861be504746cf367930aba169cc1a6e82799cf3308766d7b2459b3031ca0d  deploy/helm/fathom-operator/files/prometheus-rule-groups.yaml
d2a5588d417ca08d876e3453a48f7ef5246287048c70e2a0a159c81b8b5a6720  docs/adr/0002-healthreport-as-first-class-crd.md
64970b773712882131304088569323fe6c1ec2e8df1ee7d77eb693adb0d7b786  docs/architecture.md
3bda39513a0025a99ffa4927d9a8322dffa48a281b559fdc8953f7325fc4e382  docs/guides/addon-checks.md
ac69d03b6ca1dd03c30973052ffd84466ba519f907f3963540a176994b12d70b  docs/guides/dns-checks.md
6e5c701f6d8734e757e05aa65091637c69cb3104e2d7a393ee5e86402c47de6c  docs/guides/monitoring.md
34c048c95cc60d5b80c02f5eb40a5858b9c73a996c010585e32c117d5c755c0d  docs/guides/node-certificate-checks.md
f432a67db35883de7bc817b7463deec4307894f93b3514926c5f66795979a952  docs/guides/node-health-checks.md
b7b26dbf9ef31fc2b1188740331c85a12a160a2a4bd084d62e33f1fa3ff5f9ec  docs/reference/configuration.md
aa49e5eb3c3e7556e62f3a8dc0a19304ae402969e8550242b5090751adf9bcc4  docs/reference/fathomctl.md
a0a0c2d8f99c28561bdca1beae8fcd228bd8a48da00f9107379fa5dcfcf08146  docs/reference/operator-rbac.md
e22ae77ed223e4e1460bd09c7cbb23618af9ec8e2769be1d640218b566fed1cb  docs/reference/status-conditions.md
43022ba0da1bfb811b4fd3e2c566736bf403bfab58c6dff34298b76b090c80cf  graphify-out/GRAPH_REPORT.md
dcbbf95f0c49dad752ecffd2744aed2b82f46db95ad56f50fb7b929c0a7bd7bc  graphify-out/graph.html
4b3da42cc266990580d41269b5a749e239da9ebc17166981a54f352dcfc79d80  graphify-out/graph.json
b7ad397d6d27c19b7b064f8c93490a82f973fd932a5a7e2d468533190e79c278  graphify-out/manifest.json
9ebec56ddd5ec8a09a47a5e998af6cd56b091a80f9a345c5cf554b92134dd5c2  internal/app/options.go
df378765de9d175538cb256c65e31cbd4c6e59a673f8052c87c77d23971c3e20  internal/app/run.go
4880175f13f9041cd236f8e14123d59bbff5654fbffadd69008971b4eb49f80a  internal/app/runtime_wiring_test.go
6964861369b550d0cb7b79f6c8139bc66e21cc3e2ed906db5bbd6ad46847a9da  internal/cli/client.go
3385be365697375a0a1c33f22b66a8519a9d0174184482ef86706210c97f143c  internal/cli/export.go
2dd4569a2a2d19c6f7e7a91e383a46777e2fc73d1a04f0a04d8dab8225ae6f16  internal/cli/export_test.go
97c6a8fef12c3c5de62d3d6569139a494d437083992c33ba48ad64b2b2d85dd4  internal/cli/lifecycle_path.go
1fbf38b0a223d1714e1fcb333c6204aadb82724da072f0fe437225d0b8f49ecc  internal/cli/lifecycle_replace_test.go
df8deac557c4ad1cfda77728a750614e12dbbf3a2fffd0706dd696aeee30e4d0  internal/cli/reports.go
c5d656a31422fd83201072253248b969bd6e7e61dbf9c02e19888fbf5083f220  internal/cli/reports_test.go
1ac96864096f64b2af92cadc4ef2d8ad0c4f97e5204978c2793eba00d327211c  internal/cli/run.go
07b633d1e4d93074860512b4736f986ed6a3191ac1e91565d332d601df79308c  internal/cli/status.go
d8fc7c2b07490b488ed4e65cd3d46a5b345d5e384d1957b572b3158d62d475a4  internal/cli/upgrade.go
31a5798272d3a64f1b029b5763c68fa973c43b7c36a6cbee32ae1016e68f2504  internal/cli/upgrade_failure_test.go
7d3e81474a8d828125de3debd71174f007ef60c8779d82dc03778c1f7f810d58  internal/controller/addoncheck_controller.go
b9e472a28a6ab97923c8647ea9fce967971c765a7bd0aab6fc0d2ad5056ee420  internal/controller/addoncheck_controller_test.go
e4aa376a0122005a140bcf480e8a76acb6e1943c723a1ca9bbf3eb74c0b7ac66  internal/controller/dnscheck_controller.go
87a614ae87d62bb29dfad090cf8b8fb6fc8a99a081821f0c1e8dfd56cddb32e0  internal/controller/healthreport_history.go
c520d114fee6de78dfa20d0b2d0640a8ce2192df021366c0b1b93c1488e3a3dd  internal/controller/healthreport_history_test.go
0bbc6a31392a6c8a8b189986236eccbebb3e6dde65d287ed353a04a5be3f326d  internal/controller/nodecertificatecheck_controller.go
7850d19a6ea318b09fef839a5b714fc8644314a4a8926bcce6a986e122c4066e  internal/controller/nodecertificatecheck_helpers.go
0d1e071b59459af2c1b3a027d1eb672880a95088c1e4114eb43d6f376c4ba11d  internal/controller/nodehealthcheck_controller.go
877c894065849703bec9f179c89064bc4b3189d591a6728a2025fbfb4262322e  internal/controller/nodehealthcheck_controller_test.go
400120d9fb09970e97f495703e5486224781f55beb9c273f1aadb9d61769b4ee  internal/controller/nodehealthcheck_helpers.go
6907a2df742ad0ba5be9543c551f47013559d77ca282403ab6c6b92e25ffe25e  internal/metrics/contract_doc_test.go
77ea64201f7b5cfc36936e50fd07297ec6e93961f5bfc3f4680160eca263f5a8  internal/metrics/metrics.go
1a637edccd742a6304ca1380eeb71a466116891a8b25d4232638bd9f4559f6f4  scripts/check-crd-compat.sh
d12ba6d799c3c40a245ec44770404bf01ba9a2e7e67e62bb5cee40c7adbd70f5  scripts/check_crd_compat_test.go
a1fe09a011b3fdabf184c0dbfe65348da6fc019fd1e475040bb44b8aebc1cc93  specs/014-api-v1-promotion/contracts/cli-upgrade-export.md
019bfd7cb8641f6eca3d307c0120b1c66e0949cc1428277646b3d5f634e3cecd  specs/014-api-v1-promotion/contracts/retention.md
de5e295823238a9efca3dc055d69d9792843cde026b483948375f32739952edd  specs/014-api-v1-promotion/data-model.md
28ed7717c0a3ca6be63d7ec7e8394f2c72cc0351b9b369edd21423d4ac54ed29  specs/014-api-v1-promotion/research.md
f6e9daad8804a3d69bdd6a1ba6af1491b1120de9e19f568f42806f4becff8b4b  specs/014-api-v1-promotion/tasks.md
1a68c510f106278815a5a82735fc594853df7957c98d1fa535ddcb70776d912f  test/e2e/coredns_test.go
8ce78e1e87569b10152a7da9123fa8dc61197ce723eea939b9be7a9f0a837f0c  test/e2e/fathomctl_lifecycle_test.go
51d075c8750026f8651e9afe5c405296c2a6227e59c6e0842485e6f436690903  test/e2e/nodelocaldns_test.go
a1b341d2444367425048d61cca10b780ce9935aa817c44c1526546189e0b1695  docs/adr/0010-healthreport-retention.md
84523ec4d902ff8fc3d91db93b42bad45127279fa7c756d2dcf70ec5a237933b  internal/app/options_retention_test.go
8ac3b7a32033c778aae0856a0b6a15fda14a1ce917234659c645ca7bba85a449  internal/controller/healthreport_retention_controller.go
df494d286828b55138829bfd089f67c0990324ab6418bb5a9667c9800da03c76  internal/controller/healthreport_retention_controller_test.go

After snapshot 2 one line in `internal/cli/lifecycle_path.go` was reshaped for
a lint finding (QF1001): the same-version condition became a named boolean
`sameVersionReapply`; behaviour unchanged, `go test ./internal/cli/` and `task
lint` green.

## Re-review

A third independent reviewer (fresh subagent, read-only) re-reviewed the
remediated tree against scope snapshot 2: fingerprints verified (one mismatch,
`internal/cli/lifecycle_path.go`, was the QF1001 reshuffle noted above and was
reviewed on disk); `go build`, `go vet`, `go test ./internal/cli/ ./scripts/`,
the real-baseline gate run (`CRD_COMPAT_BASELINE_REF=v0.6.0`: seven
SANCTIONED, none INCOMPATIBLE, none STALE), the focused retention suite (12
specs), and the Taskfile stop selection executed against the real
`upgrade-path.yaml` (0.7.0).

Verdict on run 1: all sixteen findings FIXED, each with a test that fails
without its fix and with contract and reference text matching the code.
Recommendation: PASS.

Five new findings, all P3, each remediated in the same tree (run 2):

| # | Finding | Disposition | Fix |
|---|---------|-------------|-----|
| N1 | Residual `paused` comments in the AddonCheck, NodeCertificateCheck and HealthCheck reconcilers and the PrometheusRule kustomization | fix | Reworded to the behaviour that remains (adapterless checks, provisioning-failure revocation, invalid spec). |
| N2 | The CLI's `*` sanction still matched property findings, reachable through the new same-version window: CRDs applied by a later 0.7.x plus a 0.7.0 CLI re-apply would be rewritten under the #149 sanction | fix (alternative) | `upgrade` now applies the recorded-version downgrade refusal `install` already had (`recordedVersionRefusals` unconditional), so CRDs recorded by a newer release are refused before any sanction is judged, with a precise message. `*` keeps its documented upgrade-path meaning (alpha kinds may need it; the version window bounds it). Test `TestUpgradeRefusesCRDsRecordedByANewerRelease`; `TestSanctionedBy` unchanged. |
| N3 | The write budget counted successful writes only, so a locked-down namespace could issue unbounded refused writes per pass | fix | Attempts count against the budget (`remove` and the stamp path decrement first). Test "spends the budget on refused writes too" (60 refused deletes → exactly 50 attempts). Contract wording updated. |
| N4 | `docs/guides/monitoring.md` said the gauge falls to zero but not that an empty namespace's series are dropped | fix | Sentence added to the metric row. |
| N5 | Pre-existing (commit 1, not a remediation regression): the gate ignored crdify's CRD-level findings, so a scope flip passed as compatible | fix | The gate reads `crdValidation` entries as path `^` findings; `scope` is never sanctionable, `storedVersionRemoval` is sanctionable by `*` like the gate's own served-version finding. Test `TestCompatGateRefusesAScopeChange`. |

After run 2 the coordinator re-ran the unit gates (`task test`, lint,
staticcheck, vuln, build, crd-compat; generators idempotent), the focused
retention suite, and both e2e shards (below). No independent review of run 2's
five P3 fixes was requested: each is a bounded, test-covered change to code the
re-reviewer had just read, and the user can request a further pass on the PRs.

## Limitations

- Neither reviewer ran the kind e2e suite or the real events.k8s.io recorder,
  Helm render, or the `/metrics` endpoint; the coordinator's e2e runs cover the
  lifecycle (alpha-to-v1 upgrade and retention) and core shards only. The
  other addon shards are left to CI.
- The gate-bypass experiment used a fixture baseline, not a `v0.7.0` tag
  (which does not exist yet); the code path is the same.
- T086 (editing GitHub issues #149/#262/#257) is outward-facing and left for
  explicit confirmation; it is not part of the reviewed code.

## Re-review history

1. Run 1 (snapshot 1): two independent reviews → 3 × P2, 13 × P3; remediated.
2. Re-review (snapshot 2): all sixteen run-1 findings FIXED; five new P3 (N1–N5), remediated in the same tree with tests; unit gates and both e2e shards green afterwards.
