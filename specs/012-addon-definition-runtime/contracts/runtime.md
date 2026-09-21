<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Runtime acceptance contract

Source: accepted [RFC 0001](../../../docs/rfc/0001-addondefinition-crd.md).
The tables below are copied exactly to make tests enumerate every accepted row.
They are design limits, not performance measurements. A change requires checking
the RFC, specification and tasks together.

## Numeric inventory

| Class | Proposed hard limit and enforcement | Rationale / failure |
| --- | --- | --- |
| Definition | 256 KiB canonical spec; 16 families; 32 checks/family; 512 total | Compiler plus schema structural limits; bounded compilation; DefinitionTooLarge |
| Strings/maps/lists | Strings ≤1,024 UTF-8 bytes and 1,024 code points unless tighter; maps ≤32 entries; nested lists ≤32 except listed limits; depth ≤8 | Schema maxLength/maxProperties/maxItems plus byte/depth compiler checks; InvalidDefinition |
| Names and identifiers | addon/family/component ≤63 ASCII characters; resource/group/path segments ≤253 bytes | Schema grammar; no implicit normalization |
| Binding and target scope | Binding spec ≤16 KiB; ≤32 exact namespace names, each ≤63 characters; no wildcard namespaces | Compile scope intersection before reads; ScopeDenied |
| Version expressions | Range ≤256 bytes, ≤16 comparators across ≤8 alternatives; candidate API versions ≤8 | Validate token counts before SemVer parse; InvalidVersionRange |
| Selectors and field paths | ≤32 selector terms, ≤32 values/term, ≤256-byte values; field path ≤16 literal segments of ≤128 bytes | No JSONPath filters or recursive descent; bounded parsing/traversal |
| YAML and annotations | ConfigMap value ≤64 KiB, ≤4,096 parsed nodes, depth ≤16, reject aliases; annotation values ≤1,024 bytes | Bounded streaming parse before expansion; InputLimitExceeded |
| API response | ≤2 MiB decoded response per request; ≤16 MiB cumulative per run; no response retained beyond its page | Limited reader after decompression before decode, including discovery/error bodies; ResponseLimitExceeded |
| Lists and work | ≤100 objects/page; ≤1,000 objects total/run; ≤100 API requests/run including discovery, auth helpers and retries; ≤100,000 object visits/run | Stream pages; hard counter before each operation; remaining continuation at cap is WorkLimitExceeded, never truncated success |
| Target object traversal | ≤32,768 JSON nodes and depth ≤64/object before evaluator traversal | Bounded decoder; nested pod/YAML loops share visit counter; InputLimitExceeded |
| Time | min(AddonCheck.spec.timeout, 30s); each request ≤min(5s, remaining); compilation ≤1s with cooperative checks | One deadline covers discovery/reads/evaluation/final validation; Timeout |
| Results | ≤1,000 entries and 256 KiB serialized evaluation evidence; each message ≤1,024 bytes, details ≤32 entries | Reserve a bounded failure summary; ResultLimitExceeded without partial healthy verdict |
| Compiled snapshot cache | ≤128 cached revisions/process, plus the ≤4 active-run snapshots; least-recently-used eviction of idle entries | Compile on demand within the worker budget; cache pressure does not disable unrelated definitions |
| Scheduling | ≤4 runtime runs/process, ≤1/definition, ≤1/check; ≤10 requests/s burst 20 across runtime transport | Separate runtime worker pool/limiter; deduplicated queue and round-robin admission by definition; built-ins retain their workers |
| Recovery/retry | Backoff 5s, 10s, 20s, 40s, then 60s; jitter 0–20%; missing input poll 60s; max one queued wake/check; event triggers coalesce | Retry continues while declared state exists, but rate, concurrent work and queue keys are bounded; no goroutine per failure |
| Internal request retry | ≤2 retries/request, each charged to request/time limits; pagination restart ≤1/run | No hidden retry loop outside budget; exhausted attempts report Error |


## Lifecycle matrix

| Event | Active state and visible outcome | Evidence and recovery |
| --- | --- | --- |
| Missing definition | No runtime snapshot; Ready=False/UnknownAddonType | Prior evidence retained with original time/revision/context, freshness=Unavailable; Unknown verdict only if none; watch plus bounded retry |
| Valid definition added | Activate only after valid binding and compilation | Requeue; Current only after a valid completed run |
| Edited to valid revision | Replace snapshot; old run becomes Superseded | Preserve old evidence with revision; evaluate new snapshot |
| Invalid edit stored | Remove eligibility; Accepted=False/InvalidDefinition | Old evidence retained with original time/revision/context, freshness=Unavailable; fix spec to recover; no stale adapter fallback |
| Unknown kind submitted | Admission rejects; stored revision unchanged | Existing valid snapshot remains; if legacy invalid storage is seen, previous row applies |
| Definition deleted | Remove matching UID only; Ready=False/DefinitionUnavailable | Keep original time and revision; recreation requires new UID binding |
| Same name recreated | New UID has no authority inherited from old binding | Ready=False/BindingMismatch until administrator authorizes new UID |
| Binding disabled/deleted or SA replaced | Revoke eligibility; Ready=False/AuthorizationRevoked or BindingMismatch | Cancel active work; final validation rejects changed context; explicit reauthorization recovers |
| RBAC denies an API read | No completed health result; Ready=False/AccessDenied | Old evidence retained with original observation/revision/context; freshness=Unavailable; bounded retries use actual current permissions |
| Revocation during run | Observed revocation cancels/rejects; unobserved race has limits described above | Latest attempt explains discard; no newly dated old verdict |
| Restart or partial informer sync | No runtime execution until synchronized and directly validated | Persisted evidence stays readable; no manager fallback; reconstruct from API objects |
| Binding/grants recover | Capture new authority context; Ready remains false until evaluation succeeds | New run supplies fresh evidence; history remains attributable |
| Edit during evaluation | Final revision/context mismatch discards completion | Superseded attempt recorded; enqueue current revision |
| Two valid runtime names claim identity | Name equality/uniqueness prohibits it | Invalid legacy collision fails closed; no winner by arrival order |
| New built-in collides | Conflict barrier; Ready=False/BuiltinCollision | Preserve evidence; explicit migration/deletion resolves barrier |
| Deadline, size, parser or read budget exhausted | Attempt Error with specific reason; Ready=False | No partial Pass; bounded retry and other definitions continue |
| Evidence ages out | Freshness=Stale even if stored verdict was Pass | Next eligible run can replace it; last observation time unchanged |


## Enforcement obligations

All evaluation requests, including discovery, helper reads, fallback versions,
retries and decompressed error bodies, use the bound identity, scope and shared
budgets. Control-plane metadata/final fences use uncached APIReader and never enter
the evaluator. Clear inherited impersonation headers; accept no arbitrary groups,
extras or usernames. Missing scope/factory/namespace fails closed. No new SAR
permission; diagnostics reflect requests actually made, even with metrics off.

Within-run failures stop at first failure in declared order. Publication precedence:
authority revoked/mismatched; definition or check superseded; invalid input;
recorded execution failure. Deadline wins over later response-limit errors after
cancellation. Ratio aggregation cannot average a failed family into Pass.
Compilation and evaluation recover panics in their executing goroutine, cancel
children and release slots; no unsupervised evaluator goroutines. Known input-
triggerable fatal process failures block release.

Admission CEL is operator-authored only: bound all iterated inputs, avoid nested
cross-products and test CRD installation plus maximum-sized objects against the
real API server's cost budgets. String character bounds do not replace UTF-8 byte
bounds. Internal retry and pagination restart counters share outer budgets.

## Authoring interface

Planned read-only commands:

- `fathomctl definition render --file <definition.yaml> --service-account <sa> --operator-namespace <ns> --operator-service-account <manager-sa>`
  emits definition, dedicated SA and read/impersonation grants plus an explicitly
  incomplete binding template until UIDs are known. It never invents UIDs or applies.
  The operator service account is explicit because deployment names vary; the
  renderer rejects using that same identity as the dedicated reader.
- `fathomctl definition bind --file <definition.yaml> --name <addon> --service-account <sa> --operator-namespace <ns>`
  reads existing objects and renders their exact UID references with enabled=false;
  target scope is the union of explicit primary/helper scopes in --file. The live
  definition must match the reviewed spec; mismatch fails instead of binding a
  different revision. Cluster-read helpers require allowClusterScoped; all explicit
  service helper namespaces are included. Binding remains disabled for review.
- `fathomctl definition collisions` uses this binary's bundled inventory, prints
  binary version/build and compares live definitions against it. Administrators
  must run the target release's CLI; no inventory-file input. Release builds must
  have version/build metadata; unknown development metadata fails preflight visibly.
  Exit 0 means no collision, 1 means collisions, 2 means inventory/read failure.
- `fathomctl definition drain --name <addon> --operator-namespace <ns> --leader-election-id <id>`
  performs the independent verification in [leadership.md](leadership.md); exit 0
  means verified drained, 1 not drained, 2 unverifiable/error. It never disables bindings.

Default leader election ID matches operator configuration; nondefault IDs must be
supplied explicitly. Rendered examples use non-built-in identities. Manifests are
reviewed into Git and applied by administrators. requestedReads remains a bounded
manifest/diagnostic, never an effective permission ceiling. Grant output includes
only necessary permissions and records any missing declared read information.
Rendering stays offline: unknown custom-resource plurals or scopes produce explicit
manual-completion diagnostics, never guessed resources or broader grants.
requestedReads alone cannot establish a resource scope, even when the binding
allows both cluster and namespaced targets.

## Clarification additions — 2026-09-20

The clarification answers in [spec.md](../spec.md#clarifications) govern implementation:
leader election is mandatory for runtime loading; completed all-Skipped runs replace
current evidence with Skipped and NoChecksEvaluated coverage; preflight requires
the target release's CLI. The copied RFC event/limit tables above remain unchanged.
The RFC prose's Pass/Warn/Fail enumeration is extended to include completed Skipped
by the user's explicit clarification. [decision supplement](decision-supplement.md)
records this refinement without rewriting the accepted ADR.

All-Skipped observedAt/revision/context advances after successful publication fences;
Ready denotes executable/completed, freshness denotes recency, and neither means
Pass. Attempt Error still preserves previous completed evidence. Mixed results
use existing aggregate semantics. No-change verdicts do not create reports.


## Coverage ledger

| Requirements | Story | Planned verification |
| --- | --- | --- |
| FR-001, FR-004 | US1 | Admission, conversion order, version references, render determinism, no writes |
| FR-002, FR-003, FR-005 | US2 | Dedicated UID identity, scoped discovery/helpers, local and metrics-off denial |
| FR-011, FR-012 | US2 | Every numeric row at/over cap, churn fairness, compile/evaluation panic isolation |
| FR-006–FR-010 | US3 | Every lifecycle row, context races, evidence aging, drain/leader change |
| FR-013–FR-015 | US4 | Default-off install, upgrade collision, #256 gate, rollback/re-enable |
| FR-016 | All | Unit/race/envtest plus full kind and generated/compatibility/licensing gates |

Tests must record concrete test names and outcomes in execution.md during
implementation; this planning ledger is not evidence those tests already pass.
