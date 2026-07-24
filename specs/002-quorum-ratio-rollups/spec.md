# Feature Specification: Quorum/Ratio Semantics for Managed-Resource Rollups

**Feature Branch**: `feature/159-quorum-ratio-rollups`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Quorum/ratio semantics for managed-resource rollups (GitHub issue #159): per-family threshold-driven ratio/quorum evaluator so a single broken managed CR doesn't fail the whole addon check via worst-of aggregation"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Isolated failures stop redding the fleet verdict (Priority: P1)

A platform operator runs an addon check over a large population of managed
resources (for example, hundreds of certificates, external secrets, or GitOps
applications). One resource in that population is broken — a single
misconfigured certificate that will never become ready. Today the whole addon
check reports **Fail**, which reds the dashboard and can block deploy gates,
even though 99%+ of the fleet is healthy. The operator configures a
fail-ratio threshold for that resource family; the addon verdict now reflects
the *proportion* of broken resources rather than the worst single resource,
while the broken resource itself remains individually visible in the report.

**Why this priority**: This is the core problem in issue #159 — worst-of
aggregation makes large-fleet addon checks unusable as a signal because a
single bad managed CR (which is a near-permanent condition in any large
fleet) keeps the verdict pinned at Fail. Without this, noise-prone families
cannot ship enabled.

**Independent Test**: Configure a fail-ratio threshold on one resource family
of an addon check against a population where exactly one resource is broken;
verify the family and overall verdicts stay below Fail while the broken
resource still appears as failed in the persisted report.

**Acceptance Scenarios**:

1. **Given** an addon check whose family covers 200 managed resources with a
   fail-ratio threshold of 5%, **When** 1 resource (0.5%) is unhealthy,
   **Then** the family verdict is not Fail and the overall addon result is
   not Fail on account of that family.
2. **Given** the same configuration, **When** the run completes, **Then** the
   persisted report still records the broken resource's individual failing
   result with its diagnostic details, unchanged by the rollup.
3. **Given** the same configuration, **When** 15 resources (7.5%) are
   unhealthy, **Then** the family verdict is Fail.

---

### User Story 2 - Graduated escalation between Warn and Fail (Priority: P2)

An operator wants early warning before degradation becomes an incident. They
configure two thresholds on a family: a lower warn-ratio and a higher
fail-ratio. As the degraded fraction of the fleet grows, the family verdict
escalates Pass → Warn → Fail at the configured boundaries, so dashboards and
alert rules can distinguish "background noise" from "spreading problem" from
"incident".

**Why this priority**: A single Fail/not-Fail boundary either hides growing
degradation or alerts too early. The issue explicitly calls for both levels
("Fail above N%, Warn above M%"), and the existing severity vocabulary
already distinguishes Warn from Fail.

**Independent Test**: Configure warn-ratio 2% and fail-ratio 10% on a family
and drive the degraded fraction through 0% → 5% → 15%; verify the family
verdict transitions Pass → Warn → Fail.

**Acceptance Scenarios**:

1. **Given** warn-ratio 2% and fail-ratio 10% over 100 resources, **When** 1
   resource is degraded (1%), **Then** the family verdict is Pass.
2. **Given** the same thresholds, **When** 5 resources are degraded (5%),
   **Then** the family verdict is Warn.
3. **Given** the same thresholds, **When** 12 resources are unhealthy (12%),
   **Then** the family verdict is Fail.
4. **Given** any ratio verdict, **When** an operator inspects the family
   result, **Then** the counts that produced it (degraded, unhealthy, total
   evaluated) and the thresholds applied are visible in the recorded result.

---

### User Story 3 - Existing checks keep exact worst-of behavior (Priority: P3)

An operator upgrades Fathom without touching any AddonCheck. Every check that
does not configure ratio thresholds behaves exactly as before: one failing
resource fails its family, worst-of aggregation across families is
unchanged, and no default thresholds are silently applied.

**Why this priority**: The AddonCheck policy surface is in production use
across eight shipped adapters; a silent change to verdict semantics on
upgrade would be a contract break. Ratio semantics must be strictly opt-in.

**Independent Test**: Run the existing adapter test suites (unit and e2e)
against the new version with no threshold configuration; all previously
expected verdicts are unchanged.

**Acceptance Scenarios**:

1. **Given** an AddonCheck with no ratio thresholds configured, **When** one
   of many managed resources is unhealthy, **Then** the family verdict is
   Fail, exactly as today.
2. **Given** any shipped adapter's default policy, **When** Fathom is
   upgraded to a version with this feature, **Then** no ratio thresholds are
   in effect unless the operator sets them.

---

### Edge Cases

- **Empty population**: a family that evaluates zero resources (after
  selection/narrowing) keeps its current verdict behavior (a family with no
  checks passes); ratio thresholds over zero resources never divide or
  escalate.
- **Small populations**: ratios apply arithmetically regardless of population
  size — 1 broken of 2 is 50% and will exceed most thresholds. This is
  accepted behavior; ratio thresholds are aimed at large fleets and
  operators of small populations simply leave them unset.
- **Adapter cannot determine state**: resources whose check outcome is
  Error (the adapter could not read or evaluate them) are never masked by
  ratio arithmetic — an Error in a family surfaces as Error, exactly as
  today. Ratio semantics only reinterpret Pass/Warn/Fail populations.
- **Skipped resources**: intentionally-skipped checks are excluded from the
  evaluated population and never raise a verdict, consistent with existing
  behavior.
- **Invalid threshold values**: a threshold that does not parse as a
  percentage in [0, 100] (for example `banana` or `150%`) must be surfaced
  to the operator as a rejected/not-accepted check configuration — never
  silently ignored or treated as 0%. (Validation mechanics coordinate with
  the pre-1.0 CRD validation hardening work, issue #152.)
- **Boundary equality**: a degraded fraction exactly equal to a threshold
  does not escalate; escalation requires strictly exceeding the threshold
  ("Fail **above** N%"), so `failRatio: 0` expresses "any failure fails the
  family" and reproduces worst-of.
- **Warn-only degradation**: resources reporting Warn count toward the
  warn-level fraction but not the fail-level fraction; a fleet with many
  warnings and no failures can reach Warn but never Fail through ratios.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Operators MUST be able to configure, per addon-check resource
  family, an optional pair of ratio thresholds — a warn level and a fail
  level — each expressed as a percentage of the family's evaluated
  population, through the existing per-family policy threshold surface of
  the AddonCheck resource.
- **FR-002**: When ratio thresholds are configured for a family, the family
  verdict MUST be derived from population fractions instead of worst-of:
  Fail when the fraction of unhealthy (failing) resources strictly exceeds
  the fail threshold; otherwise Warn when the fraction of degraded (warning
  or failing) resources strictly exceeds the warn threshold; otherwise Pass.
- **FR-003**: When either threshold is omitted, that escalation level MUST
  fall back to worst-of semantics for the outcomes it governs (an omitted
  fail threshold means any failing resource fails the family; an omitted
  warn threshold means any warning resource warns the family). A family with
  no thresholds configured MUST behave exactly as today, and the system MUST
  NOT ship implicit default thresholds for any family.
- **FR-004**: Error outcomes (the adapter could not determine a resource's
  state) MUST NOT be subject to ratio arithmetic: any Error in a family
  MUST surface at least an Error family verdict, preserving current
  behavior.
- **FR-005**: Skipped outcomes MUST be excluded from the evaluated
  population and MUST never raise a family verdict, preserving current
  behavior.
- **FR-006**: Ratio evaluation MUST only change family-level and overall
  rollup verdicts; every individual per-resource check result (outcome,
  message, details) MUST be recorded in the persisted report exactly as it
  is today.
- **FR-007**: The overall addon-check result MUST aggregate the
  ratio-adjusted family verdicts (worst across families), so a family held
  below Fail by its thresholds cannot fail the overall check by itself.
- **FR-008**: Ratio semantics MUST be provided once by the shared check
  engine and apply uniformly to every adapter family that evaluates a
  population of managed resources — current and future adapters alike —
  without per-adapter reimplementation.
- **FR-009**: Threshold values that do not parse as percentages in
  [0, 100] MUST cause the check configuration to be visibly rejected (the
  check's accepted-state reporting), not silently ignored.
- **FR-010**: When a ratio verdict is produced, the recorded family result
  MUST include the evaluated population size, the degraded and unhealthy
  counts, and the thresholds applied, so an operator can explain any verdict
  from the persisted report alone.
- **FR-011**: The external ClusterHealth contract MUST remain untouched: it
  continues to derive only from HealthCheck status, and this feature changes
  only how AddonCheck family and overall results are computed.

### Key Entities

- **Resource family**: an adapter-defined group of checks over a population
  of managed resources of one kind (certificates, external secrets, GitOps
  applications, …). The unit to which ratio thresholds attach.
- **Family policy thresholds**: the existing per-family key/value threshold
  surface on the AddonCheck resource, extended with the warn-ratio and
  fail-ratio keys.
- **Evaluated population**: the resources in a family whose checks produced
  Pass, Warn, or Fail on a run — the denominator for ratio arithmetic
  (Skipped excluded; any Error short-circuits ratios).
- **Family verdict**: the per-family rollup outcome (Pass/Warn/Fail/Error)
  after ratio evaluation; feeds the overall addon result and run metrics.
- **Health report**: the persisted per-run record; continues to carry every
  individual check result plus, for ratio-evaluated families, the counts
  and thresholds behind the verdict.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With a 5% fail-ratio configured, a single broken resource in a
  fleet of 100+ no longer turns the addon verdict Fail — the verdict stays
  Pass (or Warn where a warn threshold is crossed) while the broken resource
  remains individually identifiable in the report.
- **SC-002**: 100% of existing adapter test expectations pass unchanged when
  no thresholds are configured — zero verdict regressions on upgrade.
- **SC-003**: For any ratio-evaluated family, an operator can state why the
  verdict was produced (counts and thresholds) using only the persisted
  report, with no access to source code or logs.
- **SC-004**: The same two threshold keys work on every managed-resource
  family across all shipped adapters; enabling the noise-prone families
  gated on this feature (for example GitOps application fleets) requires
  configuration only, no new code per family.
- **SC-005**: A misconfigured threshold value is surfaced as a rejected
  check configuration on the next reconcile, and 0% of invalid values are
  silently ignored.

## Assumptions

- **Opt-in only**: no default ratio thresholds ship for any family; absent
  configuration, behavior is bit-for-bit today's worst-of. Adapter docs may
  *recommend* values for noise-prone families, but never apply them.
- **Ratio only, not absolute quorum**: this iteration covers
  percentage-of-population thresholds. Absolute-count semantics (for
  example "fail unless at least N healthy") are out of scope and can be
  layered onto the same surface later if a concrete need appears.
- **"Above" is strict**: escalation requires the degraded fraction to
  strictly exceed the threshold, matching the issue's "Fail above N%"
  wording and making `0` reproduce worst-of exactly.
- **Degradation classes**: the fail-level fraction counts failing resources
  only; the warn-level fraction counts warning and failing resources
  together. Error and Skipped outcomes are outside ratio arithmetic
  (short-circuit and excluded, respectively).
- **Threshold expression**: values are percentages of the evaluated
  population, carried on the existing string-valued per-family threshold
  surface; exact key names and accepted formats are settled at planning.
- **Families are independent**: ratio evaluation happens strictly within a
  family; one family's population never influences another family's
  verdict, preserving the existing independence guarantee.
- **Validation coordination**: strict schema-level validation of threshold
  values overlaps with the pre-1.0 CRD validation hardening (issue #152);
  this feature guarantees reconcile-time rejection visibility (FR-009) and
  defers schema-level enforcement mechanics to that work.
- **Applies wherever configured**: thresholds are honored on any family the
  operator sets them on; they are only *useful* for multi-resource
  families, and documentation will say so, but no family is special-cased.
