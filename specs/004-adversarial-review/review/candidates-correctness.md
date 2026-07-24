# Correctness / Reconcile-Time Review — Candidates

Reviewer perspective: correctness / reconcile-time behavior only.
Commit under review: cb845dd (worktree `004-adversarial-review`).

Surfaces read in full: `internal/controller/` (all four reconcilers plus
`cadence.go`, `observe.go`, `healthreport_idempotency.go`, `tracing.go`,
`nodecertificatecheck_helpers.go`), `internal/adapter/declarative/engine.go`
+ `evaluator.go` + `versiondetect.go` (spot), `internal/adapter/registry/`,
`internal/adapter/crdutil/`, `internal/adapter/impersonation/`,
`internal/adapter/coredns/adapter.go` and `certmanager/adapter.go` (Run and
check bodies; kubestatemetrics/nodelocaldns follow the same shape),
`pkg/adapter/ratio.go`, `api/v1alpha1/aggregate.go` +
`healthreport_types.go` (Severity ladder), `internal/app/run.go`,
`internal/probe/launcher.go` (lifecycle only).

Checked and cleared (no entry): ClusterHealth constitution compliance — the
reconciler reads only `HealthCheck.Status`, never HealthReport (verified no
import/list of HealthReport in `clusterhealth_controller.go`); ratio math in
`FamilyRatioVerdict` / `RatioPercent.exceededBy` (integer cross-multiply,
strict-`>`, error short-circuit, empty-population fallback all correct and
unit-tested in `ratio_rollup_test.go`); `WorstResult` fold semantics
(Skipped informational, coerce-empty-to-Unknown); #148 old+new watch mapping
for label edits; deterministic HealthReport naming idempotency under
conflict retry (`createOrReuseHealthReport` + keyParts include prior report
name and both results, so flip-flops never collide); apiextensions scheme
registration (SKA-422 fix present in `NewScheme`); `scopedCacheOptions`
label filter vs the AddonCheck ServiceAccount list (SA deliberately
unfiltered — comment and code agree); impersonating clients are direct/
uncached so the ConfigMap cache filter cannot starve adapters; probe pod
cleanup uses a fresh `context.Background()` so a cancelled reconcile ctx
does not leak the pod; named-return `err` shadowing in
`ClusterHealthReconciler.Reconcile` (all return sites assign explicitly —
benign); `metav1.Time` second-truncation vs `RequeueAfter` (truncation
rounds down, so due-checks never stall).

---

### COR-1: Declarative engine silently truncates a Run on ctx expiry between families, persisting a wrong verdict from partial data (high)
- Location: `internal/adapter/declarative/engine.go:296-300`
- Failure scenario: An AddonCheck for any declarative-engine addon
  (externalsecrets, cilium, istio, keda, …) with `spec.timeout` = 30s
  (default). Family 1 (`controller_health`) completes its reads just before
  the deadline; the deadline then expires in the gap before family 2 (the
  family that is actually failing, e.g. `secretstore_health`) starts.
  `Run` hits `if ctxErr := ctx.Err(); ctxErr != nil { break }` and returns
  the partial check list **with `err == nil`**. The controller treats this
  as a complete run: `aggregateWithRatioRollups` folds only family 1's
  Passes, a Pass HealthReport is persisted (or `LastRunTime` refreshed on
  Pass), and the wrong verdict stands for a full interval. The same path
  fires on operator-shutdown ctx cancellation mid-run. Nothing in the
  report or status marks the run as truncated — no Error entry, no Skipped
  entry for the unevaluated families, no adapter error.
- Evidence:
  ```go
  if ctxErr := ctx.Err(); ctxErr != nil {
      // Honor cancellation between families; already-collected checks are
      // returned as-is, without an adapter-level error.
      break
  }
  ```
  and in `addoncheck_controller.go` the nil `runErr` means
  `aggregate` is computed purely from the partial `result.Checks`
  (`healthReportForAddonCheck`), then persisted.
- Refutation notes: The comment shows this is deliberate. If the deadline
  expires *inside* a family, evaluator client reads fail with the ctx error
  and emit per-target `OutcomeError` checks, so the aggregate goes Error —
  the silent-Pass window is only the inter-family gap. But that window
  recurs every interval when an early family reliably consumes ~the whole
  timeout (slow API server, many namespaces), and there is no marker at
  all distinguishing "ran 1 of 5 families" from "ran all 5". The
  hand-written adapters (coredns/certmanager/ksm/nodelocaldns) have no such
  break, so only engine-backed addons are affected. No engine test covers
  cancellation between families (`engine_test.go` has no cancel case).

### COR-2: NodeCertificateCheck ensure-failure paths set Ready=False in memory but never persist it — persistent provisioning failure leaves stale Ready=True in status forever (medium)
- Location: `internal/controller/nodecertificatecheck_controller.go:233-260` (all five `return ctrl.Result{}, err` sites before `finish`)
- Failure scenario: A check has been healthy (`Ready=True/Reporting`,
  `LastResult=Pass`). The operator's RBAC is later narrowed (or the
  node-agent ClusterRole update starts being rejected by an admission
  webhook / OPA policy). Every reconcile now takes
  `ensureNodeAgentClusterRole` → error → `setReady(False,
  "RBACProvisioningFailed")` → `return ctrl.Result{}, err`. `finish()` —
  the only place `Status().Update` is called — is never reached, so the
  API-visible status stays `Ready=True/Reporting` with the old Pass verdict
  indefinitely, while the failure is only visible as repeated Warning
  events (and even those re-fire every retry, because the persisted
  `before` conditions never contain the False condition, defeating
  `observeCheck`'s once-per-episode dedup). Dashboards and `kubectl get`
  show a healthy check that has not converged in hours.
- Evidence:
  ```go
  if err := r.ensureNodeAgentClusterRole(ctx); err != nil {
      r.setReady(&check, metav1.ConditionFalse, "RBACProvisioningFailed", err.Error())
      return ctrl.Result{}, err   // status never written
  }
  ```
  Contrast with `HealthCheckReconciler.Reconcile`, which persists status
  *before* returning the transient `mirrorErr` (healthcheck_controller.go:147-153),
  and with ClusterHealth's ListFailed path, which falls through to the
  update.
- Refutation notes: For genuinely transient errors (one failed API call)
  the next reconcile succeeds and status catches up, so the bug needs a
  *persistent* ensure failure to matter. One could argue conditions-for-
  errors should not be persisted mid-failure — but then setting them
  in-memory is pointless, and the sibling controllers chose the opposite
  (persist-then-return). The same shape exists in AddonCheck's
  `runAddonCheck` error return (addoncheck_controller.go:241-243), but
  there the only error sources are SetControllerReference and the
  HealthReport create — far less likely to persist.

### COR-3: Any incomplete report window (DaemonSet rollout, node join, agent restart) wipes the NodeCertificateCheck's last-known verdict instead of preserving it (medium)
- Location: `internal/controller/nodecertificatecheck_controller.go:275-277` and `:918-922` (`clearNodeCertRollupStatus`)
- Failure scenario: Steady state `LastResult=Pass`. A node is added (or the
  agent image is bumped, or one agent pod restarts). For the whole window
  where `nodeCertReportsComplete` is false — up to the new agent's first
  scan, or a full rollout — `clearNodeCertRollupStatus` sets
  `LastRunTime=nil`, `LastReportName=""`, `LastResult=""`. Consequences:
  (a) `observeCheck` records a result transition Pass→Unknown and later
  Unknown→Pass, so every routine rollout emits a pair of spurious
  ResultChanged events (Warning for the →Unknown edge, since Unknown ≥
  Warn severity); (b) the freshness gauge gets a zero time (reads as
  infinitely stale); (c) on recovery with an unchanged aggregate and
  generation, `decideNodeCertRollup` sees `LastReportName==""` →
  `rollupPersist`, recomputes the *same* deterministic name as the very
  first report (keyParts: UID, gen, "", "", aggregate) and reuses it —
  setting `LastRunTime` **backwards** to that old report's `ObservedAt`
  (`check.Status.LastRunTime = &persistedReport.Spec.ObservedAt`,
  line 842), a stale-liveness blip until the next interval refresh. This
  contradicts the preserve-last-good philosophy just established for
  transient conditions in #248.
- Evidence:
  ```go
  } else {
      clearNodeCertRollupStatus(&check)
  }
  ...
  func clearNodeCertRollupStatus(check *fathomv1alpha1.NodeCertificateCheck) {
      check.Status.LastRunTime = nil
      check.Status.LastReportName = ""
      check.Status.LastResult = ""
  }
  ```
- Refutation notes: The gate itself is justified (SKA-589: never stamp
  status from stale-template pods), and `AgentReady`/`Ready` conditions do
  say RollingOut/PartialReports — a careful consumer can tell. But nothing
  required *clearing* the mirrored verdict rather than freezing it; the
  paused path deliberately preserves the snapshot, showing preservation is
  the intended pattern. If the incomplete state persists (one flaky node),
  the verdict is lost indefinitely, not just for a blip. HealthCheck does
  not yet wrap NodeCertificateCheck, so this does not ripple into
  ClusterHealth today — which caps severity at medium.

### COR-4: nodeCertReportsComplete lets a departed node's leftover report stand in for a new node's missing one — rollup claims complete coverage while a live node was never scanned (medium)
- Location: `internal/controller/nodecertificatecheck_controller.go:908-916`
- Failure scenario: 3 nodes A/B/C, all reporting. C is replaced by D
  (cluster autoscaler churn). `DesiredNumberScheduled` is still 3, D's
  agent pod is Ready but has not yet published, and C's ConfigMap remains
  fresh for up to `interval+timeout` (default ~1h). `reportCount(3) >=
  desired(3)` and `nodeAgentRolledOut` is true, so
  `nodeCertReportsComplete` passes and the rollup aggregates {A, B, C} —
  a Pass verdict is persisted that includes a node that no longer exists
  and excludes D entirely. If D carries a near-expiry kubelet cert, the
  check reports Pass with `Ready=True/Reporting` for up to the freshness
  window. The count-based completeness test never verifies that the
  reporting node *set* covers the currently selected nodes.
- Evidence:
  ```go
  return nodeAgentRolledOut(ds) && int32(reportCount) >= ds.Status.DesiredNumberScheduled
  ```
  with reports keyed only by `report.Node` in `collectNodeReports` — no
  cross-check against the live Node list or the DaemonSet's pods.
- Refutation notes: Explicitly documented as a tradeoff ("tolerate
  transient node-count churn … accept reportCount >= desired", SKA-589) —
  the alternative (exact match) blanked the rollup for the whole freshness
  window, which COR-3 shows is worse. The window normally closes at D's
  first scan (agents scan on start), so minutes, not the full hour, unless
  D's agent is wedged — but a wedged agent is exactly the case a
  certificate check must not paper over. Fix direction would be comparing
  node-name sets (reports ⊇ scheduled nodes) rather than counts.

### COR-5: Status-update conflict after an adapter run causes a full re-run (including probe pods) on the immediate retry (low)
- Location: `internal/controller/addoncheck_controller.go:264-272` (update), `:278-289` (`addonCheckDueForRun`)
- Failure scenario: The coredns AddonCheck runs its DNS probe pods; before
  `Status().Update` lands, the fathom CLI writes a fresh `run-now`
  annotation (or any metadata write) — the update 409s and Reconcile
  returns the error. On the rate-limited retry the stored status still has
  the old `LastRunTime`, so `addonCheckDueForRun` is true and the adapter
  runs again end-to-end: new probe pods, new admission dry-runs, doubled
  API load, within the same interval. `observeCheck` also re-emits the
  ResultChanged event on the retry (the persisted `before` never saw the
  new result).
- Evidence: `if err := r.Status().Update(ctx, &check); err != nil { return ctrl.Result{}, err }`
  with no conflict-specific handling; due-ness is derived solely from
  persisted `LastRunTime`/`ObservedGeneration`.
- Refutation notes: HealthReport writes are idempotent under this retry
  (deterministic name → `createOrReuseHealthReport` reuses), and
  `status_conflict_test.go` pins that behavior — the *report* side was
  clearly thought through; only the side-effectful re-run remains.
  Conflicts are rare because only this controller writes AddonCheck
  status, and the blast radius is one extra bounded run. Adapter runs are
  required to be idempotent reads (plus ephemeral probe pods), so this is
  cost/noise, not wrongness.

### COR-6: History pruning can delete the just-created report on same-second CreationTimestamp ties, dangling Status.LastReportName (low)
- Location: `internal/controller/addoncheck_controller.go:533-544`; same pattern `internal/controller/nodecertificatecheck_helpers.go:253-262`
- Failure scenario: `historyLimit: 2` (schema minimum 1 permitted). A flapping
  addon transitions Pass→Fail→Pass within one second, producing two reports
  with identical (second-granularity) `CreationTimestamp`s plus one older
  report — 3 items, limit 2, excess 1. `sort.Slice` (unstable) with
  `Before()` (strict) leaves the order of the two tied reports at the mercy
  of List ordering (by hashed name, effectively arbitrary), so the pruner
  can delete the *newest* report — the one `Status.LastReportName` points
  at — while keeping its older twin. Consumers following `lastReportName`
  get a 404; the dangling pointer persists until the next transition.
- Evidence:
  ```go
  sort.Slice(reports.Items, func(i, j int) bool {
      return reports.Items[i].CreationTimestamp.Before(&reports.Items[j].CreationTimestamp)
  })
  excess := len(reports.Items) - limit
  for i := 0; i < excess; i++ { ... Delete ... }
  ```
  No name/UID tiebreaker and no exclusion of `Status.LastReportName`.
- Refutation notes: Needs a same-second double transition *and* the history
  at its cap *and* the unlucky tie order — rare in practice, and default
  `historyLimit=10` makes the tied pair unlikely to straddle the cut line.
  Self-heals on the next transition. Fix is a one-line tiebreak on Name or
  skipping the report named in status.

### COR-7: nodeCertReportFresh accepts reports timestamped up to maxAge in the future — a clock-skewed node's report stays "fresh" for 2×maxAge (low)
- Location: `internal/controller/nodecertificatecheck_controller.go:883-891`
- Failure scenario: A node with its clock 1h fast publishes a report with
  `ObservedAt = now+1h`. `maxAge = interval+timeout` ≈ 1h. The guard
  `report.ObservedAt.After(now.Add(maxAge))` only rejects stamps more than
  a full `maxAge` ahead, so the report is accepted now and remains "fresh"
  until `now.Sub(ObservedAt) > maxAge`, i.e. ~2h total — masking a
  subsequently dead agent on that node for an extra hour, during which the
  rollup keeps counting its (stale) verdict as coverage (compounds COR-4).
- Evidence:
  ```go
  if report.ObservedAt.After(now.Add(maxAge)) { return false }
  return now.Sub(report.ObservedAt) <= maxAge
  ```
  Contrast `internal/adapter/declarative/evaluator.go:134` which caps
  future tolerance at a dedicated `clockSkewGrace = 5m`, exactly to avoid
  this ("a naive now-minus-timestamp age goes negative for a future stamp
  and would otherwise slip under any freshness window").
- Refutation notes: Requires a badly skewed (or malicious) node; the VAP
  authenticity policy does not cover payload timestamps, but a hostile
  writer is a security-review concern. The evaluator's own 5-minute grace
  shows the project already standardized a tighter bound — this call site
  just didn't reuse it.

### COR-8: HealthCheck watch mapping swallows List errors silently — a mirrored status can go stale with zero trace (low)
- Location: `internal/controller/healthcheck_controller.go:261-264`
- Failure scenario: An AddonCheck status-update event arrives while the
  API server briefly errors the (cached, so rare — but possible pre-sync
  or under cache bypass) HealthCheck List; `healthChecksForAddonCheck`
  returns nil with **no log**, the event is dropped, and the wrapping
  HealthCheck keeps mirroring the old result until the *next* AddonCheck
  event — up to one full interval of a stale wrapper verdict feeding
  ClusterHealth. The sibling mapping in
  `clusterhealth_controller.go:369-374` logs exactly this loss
  ("Dropping the event here leaves a stale rollup published … so make the
  loss visible"); this one predates that fix.
- Evidence:
  ```go
  if err := r.List(ctx, &list); err != nil {
      return nil
  }
  ```
- Refutation notes: The list is served from the informer cache, which
  essentially never errors after sync, and the periodic AddonCheck requeue
  guarantees another event within one interval — so the stale window is
  bounded. Pure observability-of-loss gap, not a wrong computation; low.

### COR-9: Transient target-lookup path (#248) stamps the new ObservedGeneration over mirrored fields that belong to the previous checkRef (low)
- Location: `internal/controller/healthcheck_controller.go:108` with `:195-204`
- Failure scenario: A HealthCheck's `spec.checkRef` is edited from
  AddonCheck A to AddonCheck B (generation 2). The first reconcile's Get
  for B fails transiently. Per #248 the mirrored fields (A's Result,
  Summary, LastReportName, SourceObservedAt) are preserved — correct — but
  `Status.ObservedGeneration` was already set to 2 at the top of Reconcile
  and this status *is* persisted before returning the error. Until the
  retry succeeds, the object asserts "generation 2 observed" while
  `status.result`/`lastReportName` still describe target A: a consumer
  keying on observedGeneration == generation to trust the mirrored fields
  reads A's verdict as B's.
- Evidence: `hc.Status.ObservedGeneration = hc.Generation` (line 108,
  unconditional) plus the TargetLookupFailed branch that intentionally
  does not clear mirrored fields (lines 195-204), followed by the status
  update at 147-152 before `return ctrl.Result{}, mirrorErr`.
- Refutation notes: The window is one backoff retry in the common case,
  and `Ready=False/TargetLookupFailed` is simultaneously present, so a
  consumer honoring conditions is not misled. The pre-#248 behavior
  (clearing) had the worse failure mode this trade accepted. Only worth
  fixing if observedGeneration is documented as the mirrored-fields
  provenance signal.
