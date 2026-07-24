# Research: Pre-1.0 CRD Validation Hardening

All Technical Context unknowns resolved. Each decision below records what was
chosen, why, and what was rejected.

## R1. Admission mechanism: CRD-embedded CEL, no VAP, no webhook

**Decision**: Express every admission-time rule in the CRD schema itself —
kubebuilder structural markers (`MaxItems`, `MaxProperties`, `items:Pattern`,
`items:MaxLength`) plus `+kubebuilder:validation:XValidation` CEL rules on the
Go types. Do not introduce ValidatingAdmissionPolicy objects or a webhook for
this feature.

**Rationale**: The CRD travels with every install path (kustomize `task
install`, OLM bundle, Helm chart) — schema-embedded rules cannot be forgotten
by an installer. Both target CRDs already carry spec-level CEL
(`duration(self.interval) > duration('0s')`, path allowlists), so this extends
an established pattern. VAP objects are a second artifact with their own
lifecycle/RBAC and are unnecessary while every needed rule is expressible in
schema CEL. A webhook is explicitly out of scope per the spec.

**Alternatives considered**: VAP (rejected: adds a deliverable that can drift
from the CRD and can be uninstalled independently; note the operator *does*
provision a VAP for node-agent hardening, but that guards runtime behavior,
not schema shape); admission webhook (rejected: cert management + availability
burden disproportionate to structural checks); controller-only validation
(rejected: exactly the "validation black hole" the issue names — feedback
arrives after apply).

## R2. Floor rules and where the constants live

**Decision**: Replace the `> duration('0s')` CEL rules with
`>= duration('10s')` for `interval` and `>= duration('1s')` for `timeout`, on
both `AddonCheckSpec` and `NodeCertificateCheckSpec` (keeping the existing
`timeout <= interval` cross-field rule). Declare exported constants in
`api/v1alpha1` (e.g. `MinCheckInterval = 10 * time.Second`,
`MinCheckTimeout = time.Second`) next to the types whose CEL encodes them, and
have the controllers' clamp logic reference those constants so schema and
clamp cannot drift apart silently (a comment on the constants points at the
CEL markers; an api-package unit test asserts the marker strings contain the
constant values).

**Rationale**: Clarification session fixed the values (10s/1s). Constants in
the API package are importable by both controllers and by tests; runtime
config for the floor was rejected because the floor is part of the published
API contract — a per-deployment floor would let the operator disagree with
the schema the cluster admits against.

**Alternatives considered**: `Minimum`-style numeric markers (inapplicable —
`metav1.Duration` is a string in schema); defaulting instead of rejecting
(rejected: silent mutation of user intent at admission; CRD defaulting also
cannot express "raise to floor").

## R3. Runtime clamp and its observability

**Decision**: Clamp in the existing cadence helpers —
`addonCheckInterval`/`addonCheckTimeout` and
`nodeCertInterval`/`nodeCertTimeout` — raising any set-but-below-floor value
to the floor (unset values keep their existing defaults, all ≥ the floors).
When a clamp occurs, the reconciler (which owns the `events.EventRecorder`
and condition writes — the helpers stay pure) emits a **Warning Event**
(reason `CadenceClamped`) and sets the `Accepted` condition **True** with
reason `SpecClamped`, message naming the field, configured value, and
effective value. The node-agent DaemonSet args (`--interval`) receive the
clamped value.

**Rationale**: Clarification session chose Event + condition. The helpers are
the single funnel every cadence read already goes through (requeue period,
run deadline, DaemonSet args), so clamping there is complete by construction.
`Accepted=True/SpecClamped` (not False) matches the spec: the check *remains
accepted* and running — degraded-but-visible, per constitution VII.

**Alternatives considered**: `Accepted=False` (rejected: would stop a check
that can safely run); a new condition type (rejected: `Accepted` already
carries spec-quality verdicts; a new type expands the status contract for no
consumer); log-only (rejected in clarification).

## R4. Policy map bounds and family-key format

**Decision**: On `AddonCheckSpec.Policy`: `MaxProperties=32` and a CEL rule
over keys — `self.all(k, k.matches('^[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?$'))`
(1–63 chars, lowercase alphanumerics with `-` and `_` interior). On
`AddonCheckFamilyPolicy.Namespaces`: `MaxItems=64`, `items:MaxLength=63`,
`items:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` (DNS-1123 label). On
`Thresholds`: `MaxProperties=16` plus a CEL key-shape rule (same family-key
character class).

**Rationale**: Live family names use underscores (`api_availability`,
`addon_version`, `control_plane_health` — 26 families surveyed), so DNS-1123
for keys would reject every existing policy; the chosen class is the smallest
superset covering reality. Namespace *values* genuinely are namespace names →
DNS-1123 label is exact. Bounds (32/64/16) are an order of magnitude above
observed usage (samples use 1–3 families, ≤ 3 namespaces) and exist both as
abuse limits and to give the CEL cost estimator finite input sizes.

**Alternatives considered**: DNS-1123 for family keys (rejected: breaks FR-008
against every existing sample/fixture); no caps (rejected: CEL cost estimation
over unbounded maps risks exceeding the per-CRD budget, and unbounded lists
are the abuse vector FR-003/FR-004 close).

## R5. Numeric threshold validation in CEL

**Decision**: Field-level CEL on `Thresholds`: keys in
`['warnDays','failDays']` must match `^[0-9]{1,4}$`; keys in
`['warnRatio','failRatio']` must match `^(0(\\.[0-9]{1,4})?|1(\\.0{1,4})?)$`
(decimal in [0,1]). All other keys pass at admission (adapter-defined; the
`ThresholdAdvertiser`/`unknownThresholdKeys` reconcile-time check keeps
judging key *names*, and `ParseRatioThresholds` remains the semantic parser).

**Rationale**: The survey found exactly four numeric keys in tree today
(`warnDays`/`failDays` integers; `warnRatio`/`failRatio` ratios parsed by
`adapter.ParseRatioThresholds`). Validating known-numeric keys by shape at
admission kills the `warnDays: "banana"` class (FR-005) without rejecting
unknown adapter keys (explicit FR-005 prohibition). Cross-key semantics
(warn ≥ fail) stay reconcile-time — expressible in CEL but adapter-semantic,
and the spec assigns semantics to the adapter.

**Alternatives considered**: Typing thresholds as a structured object
(rejected: `map[string]string` is the deliberate adapter-extensible contract;
restructuring it is a bigger API break than this feature sanctions); rejecting
unknown keys at admission (rejected by spec).

## R6. Label-selector structural CEL, with controller backstop

**Decision**: Field-level CEL on `LabelSelector`: every
`matchExpressions` entry's `operator` must be one of
`In|NotIn|Exists|DoesNotExist`; `In`/`NotIn` require non-empty `values`;
`Exists`/`DoesNotExist` require empty/absent `values`. Full label key/value
*syntax* (prefix/name grammar, 253+63 char split) stays reconcile-time via the
existing `metav1.LabelSelectorAsSelector` check in
`validateAddonCheckPolicy`. If the API server's CEL cost estimator rejects
the nested rule (map → selector → expressions without an items bound on the
imported `metav1.LabelSelector` type), the fallback is dropping only this rule
and relying on the documented reconcile-time backstop — FR-006 explicitly
permits this split.

**Rationale**: Operator-enum and values-presence errors are the common
hand-authoring mistakes and are cheaply expressible; label-name grammar in CEL
regex is possible but costly and duplicates upstream logic the controller
already runs. The imported `metav1.LabelSelector` schema cannot take
kubebuilder bounds markers from this repo, which is what makes rule cost the
one open risk — hence the explicit fallback.

**Alternatives considered**: Full selector grammar in CEL (rejected: cost +
duplication); no admission check (rejected: misses FR-006's cheap wins).

## R7. Schema-compat gate: crdify, pinned, against the latest release tag

**Decision**: Add `sigs.k8s.io/crdify` (v0.6.0, latest) as a pinned tool in
`tools/go.mod`. New script `scripts/check-crd-compat.sh`: resolve baseline =
highest reachable semver tag (`git tag --list 'v*' --sort=-v:refname | head
-1`); for each `config/crd/bases/*.yaml` present at the baseline, run
`crdify file://<old> file://<new>` (old extracted via `git show
<tag>:<path>`); CRD files new since the baseline are skipped (nothing to be
incompatible with); failures are matched against the committed allowlist and
surviving failures fail the script with crdify's per-property output
(CRD, field path, change) plus a pointer to the allowlist mechanism. New
Taskfile task `crd-compat` wraps it; new `ci.yml` job runs it with
`fetch-depth: 0` (tags required).

**Rationale**: crdify is the kubernetes-sigs tool purpose-built for exactly
this comparison (served-version property diffs classified by compatibility);
adopting it over a hand-rolled differ follows the constitution's
adopt-before-build rule. Go-module pinning matches every other tool in
`tools/`. Git-tag baseline (vs. GitHub release asset download) keeps the gate
hermetic — no network beyond the clone, works in forks, and `release-please`
tags are the release identity anyway. Script + task + job mirrors the
existing `check-version-lockstep.sh` precedent.

**Alternatives considered**: kube-api-linter (rejected: lints marker style on
Go types, does not diff released schemas — complementary, not sufficient);
hand-rolled YAML diff (rejected: compatibility classification — enum
narrowing, validation tightening, type changes — is the hard part and crdify
already encodes it); downloading release artifacts from GitHub (rejected:
network dependency + auth in CI, breaks forks).

## R8. Override allowlist format and lifecycle

**Decision**: `.crd-compat-allowlist.yaml` at the repo root: a list of
entries `{crd, path, reason, issue}` (CRD name, property path or `*`,
human rationale, tracking link). The gate treats a crdify failure as
sanctioned iff an entry matches its CRD + path. The file ships **seeded with
this feature's own tightenings** (interval/timeout floors, policy bounds —
required, since the v0.4.x baseline predates them). Entries are pruned when
the baseline advances past them; the gate warns on allowlist entries that
matched nothing (stale entries), keeping the file self-cleaning.

**Rationale**: Clarification session chose a committed, reviewed file
(constitution II — the override lands in git history). Keying by CRD +
property path makes an entry exactly as wide as the sanctioned change.
Stale-entry warnings implement the spec's "pruned once the baseline advances"
without a manual audit.

**Alternatives considered**: PR label / commit trailer (rejected in
clarification: approval outside the reviewed diff); versioned allowlist keyed
by baseline tag (rejected: stale-entry warning achieves pruning with less
ceremony).

## R9. Test strategy per requirement

**Decision**:
- **CEL admission matrices (FR-001, FR-003–FR-006, FR-012)**: envtest specs in
  `internal/controller` — envtest installs the real generated
  `config/crd/bases` CRDs, so creates/updates hit genuine API-server CEL
  evaluation. Table-driven accept/reject cases per field boundary
  (below/at/above each floor; each policy mistake class; valid samples).
- **Clamp (FR-002)**: unit tests on the cadence helpers (pure functions,
  no envtest) + reconciler-level envtest assertions that a stored-but-legacy
  object (created against relaxed CRDs is impossible in envtest, so the test
  drives the helper path and asserts Event + condition via the recorder fake /
  status read).
- **FR-008 regression**: an envtest spec that applies every
  `config/samples/fathom_v1alpha1_addoncheck_*.yaml` and asserts admission.
- **Gate (FR-009/FR-010)**: fixture-driven shell test
  (`scripts/check-crd-compat_test.sh` style, run in CI next to the gate job)
  exercising: no change → pass; added optional field → pass; removed field →
  fail naming the field; allowlisted removal → pass with visible notice;
  stale allowlist entry → warning.
- **e2e**: full `task test-e2e` (CRD schema change → mandatory per AGENTS.md);
  add one admission-rejection spec to the core tier as a live-cluster smoke.

**Rationale**: envtest is the cheapest layer that runs real CEL; the coverage
gate ratchets only if new code carries tests next to it; AGENTS.md makes the
e2e run non-optional for `api/v1alpha1` changes.

**Alternatives considered**: unit-testing CEL strings by re-parsing with
cel-go (rejected: tests the test's CEL environment, not the API server's);
kind-only validation testing (rejected: slow feedback for a wide matrix).

## R10. Documentation deltas

**Decision**: Regenerate `docs/reference/api.md` (`task docs:api-ref` —
field docs change with the new markers); update
`docs/reference/api-versioning.md` "Enforcement and tooling" to describe the
now-real gate + allowlist (removing "Recommended:"); README validation notes
for the floors and policy constraints; AGENTS.md gains the `crd-compat` task
in the command list.

**Rationale**: FR-011; documentation-standard drift gates already enforce the
generated reference.
