# Feature Specification: Pre-1.0 CRD Validation Hardening

**Feature Branch**: `feature/152-crd-validation-hardening`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Pre-1.0 CRD validation hardening from GitHub issue skaphos/fathom#152: add interval/timeout lower-bound CEL validation (floor, e.g. >= 10s) to AddonCheck and NodeCertificateCheck; validate spec.policy (family-name map keys, namespaces DNS-1123 pattern + MaxItems, thresholds like warnDays, structurally-invalid label selectors) via CEL/VAP where feasible; add a CRD schema-compatibility CI gate diffing config/crd/bases against the previous release per docs/reference/api-versioning.md"

## Clarifications

### Session 2026-07-23

- Q: What lower bound should apply to `spec.timeout` (interval keeps its 10s floor either way)? → A: `timeout ≥ 1s` — blocks the 1ms-typo class on both fields but keeps legitimate fail-fast timeouts (e.g. `timeout: 5s` with `interval: 5m`) valid.
- Q: How should the operator surface that it clamped a stored check's interval/timeout to the floor? → A: Both a warning Event on the check and the check's condition machinery (accepted-with-degradation message naming the field, spec value, and effective value).
- Q: How should a sanctioned breaking CRD change be acknowledged past the schema-compat CI gate? → A: A committed, code-reviewed allowlist file in the repository that the gate consults (keyed by CRD/field, pruned when the release baseline advances) — the override lands in git history per the constitution's Git-as-audit-trail principle.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Reject dangerously short check cadences at admission (Priority: P1)

A cluster operator creates or edits an AddonCheck or NodeCertificateCheck and
mistypes the cadence — for example `interval: 1ms` instead of `1m`. Today the
API server accepts any positive duration, and the typo produces a hot
reconcile loop that launches probe workloads continuously, starving every
other check and loading the cluster. With this feature, the API server rejects
the resource at admission with a clear message stating the minimum allowed
cadence, and even a value that somehow bypasses admission is clamped to a safe
floor by the operator at runtime.

**Why this priority**: This is the only gap in the set that can actively
damage a cluster (runaway pod creation, control-plane load) rather than merely
confuse a user. It is also the cheapest to hit — a single-character typo.

**Independent Test**: Can be fully tested by applying manifests with
`interval`/`timeout` values below, at, and above the floor and confirming
rejection or acceptance; runtime clamping is testable by driving the
controller with a stored object that predates the rule.

**Acceptance Scenarios**:

1. **Given** a cluster with the updated CRDs installed, **When** a user
   applies an AddonCheck with `interval: 1ms`, **Then** the API server rejects
   it with a message that names the field and the minimum allowed value.
2. **Given** a cluster with the updated CRDs installed, **When** a user
   applies a NodeCertificateCheck with `timeout: 500ms` (below the 1s timeout
   floor), **Then** the API server rejects it with a message naming the field
   and the minimum.
3. **Given** a valid AddonCheck with `interval: 5m` and `timeout: 1m`,
   **When** it is applied, **Then** it is accepted unchanged.
4. **Given** a stored check object whose interval is below the floor (created
   before the rule existed or applied while the gate was absent), **When** the
   operator reconciles it, **Then** the effective cadence used at runtime is
   never below the floor, the check continues to run rather than erroring, and
   the clamp is visible as a warning Event on the check and in its status
   conditions (naming the field, configured value, and effective value).

---

### User Story 2 - Catch invalid policy configuration at admission, not at reconcile (Priority: P2)

A platform user configures `spec.policy` on an AddonCheck — narrowing a check
family to namespaces, tuning thresholds such as `warnDays`, or adding a label
selector. Today the schema accepts structurally invalid input (a namespace
name that is not a legal Kubernetes namespace, `warnDays: "banana"`, a label
selector with an invalid operator, an unbounded namespace list), and the user
only discovers the mistake later, by noticing the check's `Accepted` condition
went false after the first reconcile. With this feature, structurally invalid
policy is rejected at admission where the schema can express the rule, so the
feedback arrives at `kubectl apply` time.

**Why this priority**: Wrong-but-accepted configuration is a silent failure —
the user believes the health check is narrowed or tuned as requested when it
is not running at all. It does not damage the cluster, so it ranks below the
cadence floor.

**Independent Test**: Can be fully tested by applying AddonCheck manifests
with each class of invalid policy value and confirming admission-time
rejection, plus valid manifests confirming acceptance; adapter-semantic
errors (e.g., an unknown family name for the selected adapter) still surface
through the existing `Accepted` condition.

**Acceptance Scenarios**:

1. **Given** the updated CRDs, **When** a user applies an AddonCheck whose
   policy lists a namespace name that is not a valid Kubernetes namespace name
   (e.g., `Prod_NS`), **Then** the API server rejects it with a message
   identifying the invalid entry.
2. **Given** the updated CRDs, **When** a user applies an AddonCheck whose
   policy namespace list exceeds the documented maximum size, **Then** the API
   server rejects it.
3. **Given** the updated CRDs, **When** a user applies an AddonCheck with a
   threshold value that must be numeric set to a non-numeric string (e.g.,
   `warnDays: "banana"`), **Then** the mistake is rejected at admission where
   the rule is expressible, and in every case is reported on the resource with
   a message naming the offending key and value before any check runs with it.
4. **Given** the updated CRDs, **When** a user applies an AddonCheck whose
   policy label selector is structurally invalid (e.g., an operator that
   requires values given none), **Then** it is rejected at admission where
   expressible, and otherwise reported on the resource before any check runs.
5. **Given** the updated CRDs, **When** a user applies an AddonCheck whose
   policy uses a family-name key that violates the documented key format,
   **Then** it is rejected at admission; a well-formed key unknown to the
   selected adapter continues to be reported via the existing `Accepted`
   condition.
6. **Given** an AddonCheck with a fully valid policy, **When** it is applied,
   **Then** it is accepted and behaves exactly as before this feature.

---

### User Story 3 - Block accidental incompatible schema changes in CI (Priority: P3)

A contributor changes a CRD type and regenerates manifests. The project's API
versioning standard forbids incompatible schema changes under an unchanged
served version (outside sanctioned alpha churn), but nothing enforces this
today — the standard itself recommends a CI check that does not exist. With
this feature, every pull request is checked: the generated CRD schemas are
compared against those of the most recent release, and a change that would
break existing stored objects or clients under an unchanged API version fails
the check with an explanation, while compatible additions pass.

**Why this priority**: This is the compensating control that makes the first
two changes (and all future schema evolution) safe to ship: once 1.0 declares
the schema stable, this gate is what prevents a later patch release from
accidentally breaking it. It is last because it protects future changes rather
than fixing a live gap in the current schema.

**Independent Test**: Can be fully tested in CI (or locally) by running the
gate against (a) an unchanged schema — passes; (b) a compatible addition — a
new optional field passes; (c) an incompatible change — e.g., removing a
field, tightening validation on an existing field, or changing a type — fails
with a message identifying the offending CRD and change.

**Acceptance Scenarios**:

1. **Given** a pull request that does not change any CRD schema, **When** CI
   runs, **Then** the compatibility gate passes.
2. **Given** a pull request that adds a new optional field to a CRD, **When**
   CI runs, **Then** the gate passes.
3. **Given** a pull request that removes a field, renames a field, changes a
   field type, or tightens validation on an existing served version, **When**
   CI runs, **Then** the gate fails and its output identifies the CRD, the
   field, and the nature of the incompatibility.
4. **Given** a deliberately sanctioned breaking change (v1alpha1 churn or a
   new API version), **When** the contributor adds the corresponding entry to
   the committed override allowlist in the same pull request, **Then** the
   gate passes with the override visible in the reviewed diff — and the entry
   is pruned once the release baseline advances past it.

---

### Edge Cases

- **Pre-existing stored objects below the new floor**: schema validation
  applies to writes, not to objects already stored. A check created before the
  floor existed must keep working — the operator clamps its effective cadence
  to the floor at runtime instead of erroring (see US1 scenario 4). This
  ratchet also means the very validation tightened by this feature is itself
  an incompatible schema change, which is sanctioned only because the API is
  still in its alpha churn window — the CI gate (US3) must therefore be
  introduced with a baseline that accounts for this feature's own changes.
- **Floor vs. cross-field rule interaction**: the existing rule "timeout must
  not exceed interval" must continue to hold together with the new floors;
  manifests with `timeout: 10s, interval: 10s` (both at their minimum-legal
  intersection) and `timeout: 1s, interval: 10s` (each at its own floor)
  remain valid.
- **Unset optional fields**: `interval` and `timeout` are optional with safe
  defaults; the floor must only constrain values that are actually set, never
  make the fields required.
- **Adapter-specific threshold keys**: threshold knobs are adapter-defined;
  only keys documented as numeric (e.g., `warnDays`, `failDays`) get numeric
  admission validation. Unknown or adapter-specific keys must not be rejected
  at admission solely for being unknown — family/key semantics remain the
  adapter's to judge via the `Accepted` condition.
- **First release after the gate lands**: the compatibility gate compares
  against the most recent release; the first run must handle a baseline that
  predates the gate itself (and predates this feature's sanctioned schema
  changes) without producing a permanently red check — this feature's own
  validation tightening enters the committed override allowlist and is pruned
  once a release containing it becomes the baseline.
- **New CRD kinds**: a brand-new CRD file with no counterpart in the previous
  release must pass the gate (nothing to be incompatible with).
- **Empty or absent policy**: an AddonCheck with no `spec.policy` (or an empty
  map) remains valid and continues to defer family selection to adapter
  defaults.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST reject, at admission time, any AddonCheck or
  NodeCertificateCheck whose `spec.interval` or `spec.timeout` is set to a
  duration below its minimum floor, with an error message naming the field and
  the minimum value. The floors are **10 seconds for `interval`** and **1
  second for `timeout`** (see Assumptions and Clarifications).
- **FR-002**: The operator MUST additionally clamp effective interval and
  timeout values to their respective floors at runtime, so stored objects that predate
  the admission rule (or bypass it) can never drive reconciliation or probe
  scheduling faster than the floor. Clamping MUST be surfaced on the resource
  in two ways, never silently: a warning Event on the check, and the check's
  status conditions (the check remains accepted, with a message naming the
  clamped field, the configured value, and the effective value used).
- **FR-003**: The system MUST reject, at admission time, AddonCheck policy
  namespace entries that are not valid Kubernetes namespace names (DNS-1123
  label format), and MUST bound the namespace list to a documented maximum
  number of entries.
- **FR-004**: The system MUST bound and format-check AddonCheck policy
  family-name map keys at admission (documented key character set and length
  cap, and a cap on the number of families per policy). Semantic validity of a
  well-formed family name for the selected adapter remains a reconcile-time
  concern surfaced via the existing `Accepted` condition.
- **FR-005**: For threshold keys documented as numeric (at minimum `warnDays`
  and `failDays`), the system MUST reject non-numeric values before any check
  runs with them — at admission where the schema can express the rule.
  Threshold keys not documented as numeric MUST NOT be rejected at admission
  solely for being unrecognized.
- **FR-006**: Structurally invalid label selectors in AddonCheck policy
  (invalid match-expression operator, operator/values combinations that are
  contradictory, invalid label key/value syntax) MUST be rejected at admission
  where expressible; whatever structural checks cannot be expressed in the
  schema MUST continue to be caught by reconcile-time validation and reported
  via the `Accepted` condition before any check executes with the bad
  selector.
- **FR-007**: Every validation gap in FR-003 through FR-006 that cannot be
  expressed at admission MUST still be caught by the existing reconcile-time
  policy validation, so no invalid policy configuration is ever silently
  acted upon; the division between admission-time and reconcile-time
  enforcement MUST be documented.
- **FR-008**: All previously valid manifests that respect the new rules MUST
  continue to be accepted unchanged — the shipped default/sample manifests and
  end-to-end fixtures MUST pass admission without modification (unless a
  fixture exists specifically to exercise a now-rejected value, in which case
  it is updated as part of this feature).
- **FR-009**: Continuous integration MUST include a check on every pull
  request that compares the generated CRD schemas against those published in
  the most recent release and fails when a change is incompatible for an
  unchanged served API version (field removal or rename, type change, newly
  tightened validation on existing fields, enum narrowing), while passing
  compatible additions (new optional fields, new kinds, loosened validation,
  documentation changes).
- **FR-010**: The compatibility gate MUST produce output that identifies the
  affected CRD, field path, and nature of each incompatibility. Its override
  mechanism for sanctioned breaking changes (alpha-version churn or a new API
  version) MUST be a committed, code-reviewed allowlist file in the repository
  that the gate consults — entries identify the accepted incompatibility (CRD
  and field) and are pruned once the release baseline advances past them — so
  every override lands in git history and the gate never forces either silent
  rule-breaking or permanent red status.
- **FR-011**: The validation rules added by this feature MUST be reflected in
  user-facing documentation: field documentation for the floor and policy
  constraints, and contributor documentation for the compatibility gate and
  its override path (updating the API versioning reference's "recommended"
  wording to reflect that the gate now exists).
- **FR-012**: Each new validation rule MUST ship with direct test coverage:
  admission-level tests exercising accept/reject boundaries for FR-001 and
  FR-003 through FR-006, runtime tests for the clamp in FR-002, and gate tests
  for FR-009's pass/fail matrix.

### Key Entities

- **AddonCheck**: namespaced health-check resource whose spec carries
  `interval`, `timeout`, and a per-family `policy` map (namespaces, label
  selector, string thresholds). The main subject of both the cadence floor and
  the policy validation.
- **NodeCertificateCheck**: cluster-scoped certificate-expiry check whose spec
  carries `interval` and `timeout`. Subject of the cadence floor only — its
  threshold fields (`warnDays`, `criticalDays`) are already strongly typed and
  it has no `policy` map.
- **Check family policy**: the per-family configuration block inside
  AddonCheck policy — enablement, namespace narrowing, label selector,
  adapter-specific thresholds. The unit whose structure this feature
  validates.
- **CRD schema baseline**: the set of generated CRD schemas as published in
  the most recent release; the reference point the CI compatibility gate
  compares each pull request against.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of attempts to create or update a check with a cadence or
  timeout below its floor are rejected at apply time with a message that names
  the offending field and the minimum — verified across the boundary matrix
  (below/at/above each field's floor, each field, both kinds).
- **SC-002**: No configuration — regardless of how it entered the cluster —
  can make the operator reconcile a check or launch probe workloads more
  frequently than the floor allows.
- **SC-003**: For each validated policy mistake class (invalid namespace name,
  oversized namespace list, malformed family key, non-numeric numeric
  threshold, structurally invalid selector), the user receives an error
  identifying the mistake at apply time where expressible — and in all cases
  before any check executes with the invalid configuration.
- **SC-004**: All shipped sample manifests and the full end-to-end fixture
  suite pass admission and behave identically after the change — zero
  regressions for valid configurations.
- **SC-005**: A pull request carrying an incompatible CRD schema change under
  an unchanged API version cannot merge with green checks unless the
  documented override is visibly invoked; compatible schema changes and
  no-schema-change pull requests pass the gate without manual action.
- **SC-006**: The compatibility gate's failure output alone is sufficient for
  a contributor to locate the incompatible change (CRD, field, change type)
  without re-deriving the diff by hand.

## Assumptions

- **Floor values**: the minimum for `interval` is **10 seconds** (the example
  value proposed in the issue) and for `timeout` is **1 second** (clarified
  2026-07-23) — the hot-loop hazard is cadence-driven, while a short timeout
  only bounds a single run, so fail-fast timeouts like `5s` stay legal.
  Nothing in the current defaults conflicts with either floor (defaults are
  minutes-scale). If a future check type genuinely needs sub-10s cadence, that
  is a deliberate schema decision for that type, not a reason to lower the
  global floor now.
- **Both belt and suspenders**: the issue offers "CEL floor and/or a
  controller-side clamp"; this spec requires **both**, because admission
  validation cannot protect against objects stored before the rule existed,
  and the clamp alone gives no apply-time feedback.
- **Sanctioned breakage window**: tightening validation on v1alpha1 is an
  incompatible change, sanctioned because the API is explicitly in its alpha
  churn window ("must land while v1alpha1 churn is sanctioned" — issue #152)
  and the project's versioning standard designates alpha as the place for
  churn. Existing stored objects below the floor are handled by the runtime
  clamp rather than migration.
- **Scope of policy validation**: only AddonCheck has a `spec.policy`;
  NodeCertificateCheck and HealthCheck have no equivalent, and HealthCheck has
  no interval/timeout fields, so the cadence floor applies to AddonCheck and
  NodeCertificateCheck only — matching the issue.
- **Admission mechanism**: validation is expressed in the CRD schema itself
  wherever possible (structural constraints and CEL rules), because the CRD
  travels with every install; cluster-scoped admission-policy objects are a
  fallback only for rules the schema cannot express, and introducing a webhook
  is out of scope. The exact split is a planning decision.
- **Compatibility baseline**: "previous release" means the most recent
  published release tag (currently v0.4.1); the gate re-baselines
  automatically as releases are cut. The comparison method/tool is a planning
  decision.
- **Adapter threshold contract**: `warnDays` and `failDays` are the
  numeric threshold keys known today; the documented set of
  admission-validated keys can grow as adapters do, and growth is a compatible
  change.
- **Out of scope**: conversion webhooks, a v1alpha2/v1beta1 version bump,
  validation of adapter semantic knowledge at admission (which family names a
  given adapter supports), and retroactive migration of stored objects.
