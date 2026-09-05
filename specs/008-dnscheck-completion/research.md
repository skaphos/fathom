<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Research: DNSCheck Completion

## R1: Projection architecture

**Decision**: Use a private, explicit registry of typed target handlers keyed
by normalized API version and kind. Each handler reads its concrete resource
and returns a private normalized snapshot consumed by the existing
HealthCheck status-writing path.

**Rationale**: The three status types expose equivalent concepts but do not
implement a shared interface. Typed handlers preserve compile-time schema
checks and isolate kind-specific cadence access while a single orchestration
path preserves identical terminal/transient semantics. The registry is an
in-tree extension boundary, not a public plugin API.

**Alternatives considered**:

- A larger kind switch in `mirrorTarget`: smallest initial diff, but repeats
  lookup/error/projection behavior and grows one branch for every future kind.
- `unstructured.Unstructured` plus JSON paths: reduces typed functions but
  trades compile-time safety for runtime field-name coupling.
- Go reflection or a public status interface: specialized CRD structs cannot
  satisfy it without API-oriented boilerplate, and a public extension surface
  is outside scope.
- Direct specialized-check aggregation in ClusterHealth: violates ADR-0004
  and the constitution's stable aggregation boundary.

## R2: API-version identity

**Decision**: Normalize an empty `checkRef.apiVersion` to
`fathom.skaphos.io/v1alpha1`; key handlers and watch matching by the exact
normalized string plus kind. Reject any other nonempty value before reading a
target. Add a finite length bound, but no one-value enum.

**Rationale**: Empty values already document the current-version default and
must remain compatible. Exact matching makes the field meaningful and avoids
reading a current object for a future or foreign contract. Avoiding an enum
keeps a future multi-version conversion path possible without first relaxing
admission.

**Alternatives considered**:

- Ignore `apiVersion`: ambiguous and silently binds references to the current
  type. The bound is 317 characters: the maximum 253-character Kubernetes API
  group, one slash, and a 63-character version.
- Require it immediately: breaks existing resources that rely on the
  documented empty default.
- Admission-enum only `v1alpha1`: clear today, but creates an unnecessary CRD
  migration when another served version is added.

## R3: Snapshot and failure semantics

**Decision**: Preserve the existing AddonCheck rules for every handler. A
successful typed read replaces the whole normalized snapshot. Unsupported
identity and NotFound clear it. A non-NotFound read error leaves it untouched,
sets `Ready=False`, and returns the error. Pausing remains outside handlers and
preserves the snapshot.

**Rationale**: Terminal reference failures must not leave a stale healthy
verdict contributing to ClusterHealth. Transient control-plane failure is not
new health evidence, so retaining the last readable snapshot satisfies the
read-only degradation principle. Semantic status comparison keeps retries
idempotent.

**Alternatives considered**:

- Clear on every error: turns API-server availability into false aggregate
  health and discards last-known evidence.
- Preserve on NotFound: a deleted or misspelled target could remain healthy
  indefinitely.
- Synthesize `Unknown`: invents a source verdict and changes existing wrapper
  semantics.

## R4: Source status normalization

**Decision**: Normalize the shared status facts only:
`LastResult`, `LastRunTime`, `LastReportName`, Ready-condition summary, and the
kind's effective interval. Reuse `addonCheckInterval`, `dnsCheckInterval`, and
`nodeCertInterval`; keep the 1,024-rune summary truncation at the wrapper
boundary.

**Rationale**: All three CRDs already publish these facts and use the shared
result vocabulary. Existing helpers encode defaulting and legacy floor
clamping, preventing the wrapper from drifting from the source controller.
Kind-specific evidence remains on the specialized resource/HealthReport and
does not leak new fields into the aggregate contract.

**Alternatives considered**:

- Copy full kind-specific result structures: expands and couples
  HealthCheckStatus.
- Derive cadence directly from raw spec fields: duplicates default and clamp
  policy.
- Read HealthReport for richer evidence: violates the ClusterHealth/status
  separation and adds an unnecessary lookup.

## R5: Watch wiring

**Decision**: Register typed watches for AddonCheck, DNSCheck, and
NodeCertificateCheck using `ResourceVersionChangedPredicate`. Route each to a
shared list-and-filter mapper parameterized by normalized API version and kind.
Match effective namespace and name exactly.

**Rationale**: This generalizes the proven AddonCheck pattern with low
cognitive load. Parameterizing identity prevents unrelated kinds or versions
from enqueueing wrappers. Resource-version filtering includes status and
deletion-relevant changes without a custom predicate that could miss evidence.

**Alternatives considered**:

- Dynamic/unstructured watch: unnecessary for three compiled-in kinds and
  weakens type safety.
- Field indexes: useful at much larger wrapper counts, but adds manager-index
  setup and migration complexity without a stated scale requirement. It can be
  introduced later without changing the external contract.
- Poll HealthChecks only: delays propagation and adds recurring load.

## R6: RBAC and generated artifacts

**Decision**: Add HealthCheck-controller RBAC markers for read-only DNSCheck
and NodeCertificateCheck access, regenerate manifests, and require the rendered
aggregate role to show no net new permissions beyond those already needed by
their existing controllers.

**Rationale**: Controller-local markers document actual dependencies and keep
generation correct if controllers are split later. The deployed operator role
already reads/watches these resources, so completion should not broaden its
authority.

**Alternatives considered**:

- Rely only on markers elsewhere: works in the current aggregate role but
  obscures the HealthCheck controller's dependency.
- Add wildcard Fathom permissions: violates least privilege.

## R7: Validation scope

**Decision**: Use envtest for all target handlers and failure/watch branches,
plus one core-tier Kind scenario for DNSCheck → HealthCheck → ClusterHealth
Pass/Fail propagation and history idempotency. Retain existing DNS resolution
e2e scenarios rather than duplicating them.

**Rationale**: Envtest is fast and can write source status directly. A real
cluster is still mandatory because controller registration, watches, probe
execution, CRDs, and aggregation must operate together. This split provides
fast branch coverage and one meaningful end-to-end proof.

**Alternatives considered**:

- Envtest only: cannot prove real probe or manager watch behavior.
- Duplicate every resolver scenario through ClusterHealth: slow and tests the
  same DNS behavior twice.
- NodeCertificateCheck e2e aggregation in this feature: existing node-agent e2e
  remains valid; the missing release blocker specifically requires DNS
  composition, while typed NodeCertificateCheck projection is covered directly.
