# Contract: DNSCheck Reconcile Loop

**Feature**: 006-dnscheck-reconciler

The behavioural contract for `DNSCheckReconciler.Reconcile`. This is what tests
assert and what a reader should be able to rely on without reading the body.

---

## Invocation

| Trigger | Behaviour |
|---|---|
| `DNSCheck` created / spec changed | evaluate on the next pass |
| `RequeueAfter` elapses | evaluate |
| Status-only change | no evaluation — status writes must not self-retrigger |
| `DNSCheck` deleted | withdraw all series, return; no evaluation |

Two runs of the same check are never in flight at once (FR-107b). The workqueue
already serialises reconciles per key; the contract records it because the
guarantee is load-bearing rather than incidental.

---

## Ordered sequence

```text
 1. start trace span; defer record reconcile duration + outcome
 2. Get the DNSCheck
      └─ NotFound → DeleteCheckSeries + withdraw per-target series → return
 3. snapshot status (`before`)
 4. defer observeCheck(before → after)   ← every exit path covered
 5. set observedGeneration
 6. set Accepted (reason SpecClamped when the stored cadence is sub-floor)
 7. plan the run:
      · expand pairs from the CURRENT spec
      · seed every pair Unknown
      · derive one run deadline from the declared bound
 8. fan out, bounded by the configured cap:
      · per-pair timeout = min(remaining budget, per-pair ceiling)
      · one probe pod per pair, owner-referenced to the check
      · a placement failure marks only that pair Error
 9. fold with WorstResult(coerceEmptyToUnknown=true)
10. set Complete=False when any pair is still Unknown
11. rollup: persist a HealthReport only when the verdict changed
12. rebuild per-target series (delete by check, then set current pairs)
13. write status if it differs from `before`
14. requeue: max(minGap, interval − elapsed since run start)
```

Step 4 must be deferred immediately after step 2 so that *every* exit — including
error returns — mirrors status into the gauges and events. The previous result
always comes from the fetched status, never from process memory; that is what
stops an operator restart from firing a false transition event.

---

## Timing

| Property | Guarantee | Requirement |
|---|---|---|
| Run duration | ≤ the declared bound, always | FR-104 |
| Per-pair bound | `min(remaining, ceiling)` — no pair starves a later one | FR-104, #150 |
| Run vs cadence | a run cannot outlast the cadence that scheduled it | FR-104a |
| Next run | one cadence from run **start** | FR-107 |
| Floor | `minGap` when the cadence is already due | FR-107a |
| In-flight pods | ≤ configured cap per check | FR-103a |
| Cluster ceiling | cap × reconcile concurrency, from config alone | FR-103a |

---

## Outcome classification

The distinction FR-105 turns on — a fault on Fathom's side is never reported as
a resolver's answer:

| Condition | Pair result | Verdict effect |
|---|---|---|
| Assertion satisfied | `Pass` | — |
| Assertion violated, incl. NXDOMAIN under a positive assertion | `Fail` | dominates `Unknown` |
| Negative assertion violated (name resolved but must not) | `Fail` | summary **must** name the polarity (FR-021) |
| `probe.LaunchError` (quota, admission, image, unschedulable) | `Error` | dominates everything |
| Pod ran, no usable result | `Error` | dominates everything |
| Deadline hit before the pair started | `Unknown` | degrades, loses to `Fail` |

Fold ordering is `Pass < Warn < Unknown < Fail < Error`, with `Skipped`
informational. `Skipped` is **never** used for an unreached pair — it would let a
mostly-unevaluated run report `Pass`.

---

## Status write discipline

- Written only when it differs from the pre-reconcile snapshot.
- `targetResults` is **replaced**, never merged — a pair the spec no longer
  declares cannot survive (FR-036).
- `lastRunTrigger` is not touched; #264 owns it.
- A `HealthReport` is written only on verdict change; an unchanged verdict
  refreshes `lastRunTime` only, throttled to the cadence.

---

## Failure handling

| Failure | Response |
|---|---|
| One pair's pod cannot be placed | that pair `Error`; the run continues (FR-103b) |
| Every pair fails to place | verdict `Error`; `Ready=False` with a launch reason |
| API error writing status | return the error; requeue with backoff; gauges already mirrored by the deferred `observeCheck` |
| Context cancelled (operator shutting down) | return promptly; pods are reclaimed by their owner reference or the sweeper |
| Operator dies mid-run | pods orphan; the leader-elected sweeper reaps them once terminal and older than `MinAge` |

---

## Ownership

Every probe pod carries an owner reference to its `DNSCheck` — legal only because
FR-031 places the pod in the check's own namespace, so owner and object share a
namespace. This is new: the probe package sets no owner reference today, and
`AddonCheck` cannot adopt the same approach because its pods land in the addon's
namespace (research D1).

Ownership and the sweeper are complementary, not redundant. Kubernetes does not
garbage-collect terminated pods that still have a live owner, so the sweeper
remains the only thing that reaps a crash orphan while its check still exists.

---

## Explicitly out of contract

- The run-now trigger (#264).
- Any pause or suspend surface (FR-019 forbids it).
- Mirroring into `ClusterHealth`.
- Changing `AddonCheck`'s or `NodeCertificateCheck`'s cadence anchoring, though
  both share the drift FR-107 avoids.
