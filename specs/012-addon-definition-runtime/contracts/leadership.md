<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Leader election and drain verification

Implements FR-010 and the user's mandatory-election clarification. This is
observation-based operational verification, not distributed fencing or atomic RBAC
revocation. The active leader owns runtime admission, cancellation and drain status.

## Activation and identity

Use the existing manager Lease, explicitly in the configured operator namespace,
with configured LeaderElectionID (default `2d3dbc4f.skaphos.io`). Require an explicit
operator namespace for runtime loading, including local mode. If LeaderElect=false,
keep runtime inactive with AuthorizationUnavailable/LeaderElectionRequired diagnostics;
do not fail manager startup or disable built-ins. Diagnostics can be emitted at
startup without relying on a leader-gated controller that will never start.

Holder identity must be process-unique. Terminate on leadership loss; no reacquisition
within the same process. The runtime admission runnable starts only after election
and cache sync; losing its context closes admission, cancels workers and prevents
further evidence/drain publication. Use existing manager lifecycle integration,
not a second independently elected controller.

Binding status adds `leaderEpoch`: required for valid drain, object with leaseUID
(nonempty ≤128 bytes), holderIdentity (nonempty ≤253 bytes), acquireTime (timestamp),
and leaseTransitions (nonnegative int32). leaderIdentity equals holderIdentity.
Namespace/name of the Lease are trusted operator/CLI configuration, not supplied
by an untrusted definition or inferred solely from binding status. Epoch equality
uses all four fields; resourceVersion is excluded because normal renewal changes it.
A restarted holder, recreated Lease, or leadership handoff invalidates prior status.

## Drain acknowledgement

On acquisition, distrust persisted acknowledgements. Keep runtime admission closed
for at least 30 seconds using monotonic elapsed time, covering the maximum prior
runtime lifetime, while synchronizing definitions/bindings. This interval is an
operational grace period, not proof of exclusion for a suspended old process.
Old instances must check cancellation and live epoch before reads/publication.
No claim of linearizable fencing is added by the grace period.

For disabled bindings, acknowledge only after observing enabled=false directly,
cancelling/awaiting this session's work, and observing activeRuns=0. Re-read the
Lease and binding through uncached control-plane reads before publishing the
matching generation/epoch, Drained=True and Ready=False/AuthorizationRevoked.
Treat read failure or epoch mismatch as unverifiable, not drained. Re-enable/spec
edit removes acknowledgement eligibility; status writes do not change authority.

The same outer run deadline/counters cover pre/final control-plane validation and
runtime reads, even though manager and evaluator identities remain separate.
No evaluator ever receives the manager client. Lease loss/unknown epoch rejects
publication; failed fences never renew completed evidence.

## Independent CLI verification

`fathomctl definition drain` takes operator namespace and election ID, using its
uncached CLI client. It requires get on that exact Lease and the binding; no list,
watch, status-write or grant-write is needed. Administrators supply these read
permissions out-of-band. Existing operator leader-election Role already grants
Lease access; do not add cluster-wide Lease reads for this feature.

1. Directly read Lease L1. Require populated epoch, holderIdentity, renewTime and
   positive leaseDurationSeconds; otherwise return unverifiable.
2. Observe a strictly advancing renewTime with unchanged epoch within a 15-second
   monotonic deadline. Poll at most once per second, at most 16 Lease reads total;
   each request timeout is min(5 seconds, remaining deadline). This verifies live
   progress without interpreting remote timestamps using the CLI clock. Missing,
   denied, failed or changed-epoch reads fail closed; no progress means unverifiable.
3. Read binding, then Lease again inside the same deadline. Require the epoch
   unchanged, binding enabled=false, observedGeneration matching metadata.generation,
   activeRuns=0, Drained=True with matching condition generation, Ready=False with
   reason AuthorizationRevoked and matching generation, and matching leaderEpoch.
4. Return verified drained only for that observed epoch/generation. Any incomplete
   condition is not drained; absent authority evidence is unverifiable. No cached
   result survives another invocation. Lease resourceVersion changes alone are fine.

The 16-read ceiling includes the final Lease read; reserve capacity for it. Binding
read is one additional request. This CLI verification budget is separate from
runtime evaluation budgets and never mutates state. The 15-second timeout can
produce a conservative unverifiable result if renewal is slow; it cannot infer
success from an old timestamp. A leader can fail/change after the last read, so
output names the observed epoch and explicitly retains the RFC's race limitation.

## Required tests

Single instance with election disabled; mismatched configured namespace/ID;
missing/denied Lease; no renewal; clocks offset; ordinary renewal RV changes;
same epoch progressing; changed holder; recreated Lease UID; acquireTime/transition
change; old binding generation; missing conditions; leader loss during evaluation,
during drain and between CLI reads; takeover grace; zero active runs after panic;
restart identity uniqueness; no reacquisition; all CLI deadline/read ceilings.
