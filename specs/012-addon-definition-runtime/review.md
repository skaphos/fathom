<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Adversarial review — implementation checkpoint

Scope: current uncommitted schema, semantic validator, compiler, configuration,
and Podman test-harness changes. This is not the final runtime security review:
the authority/transport, lifecycle, evidence, CLI and qualification work remains
unchecked in tasks.md. Verdict: **not ready for runtime activation or release**.

## Resolved design decision

**R1 — Offline grant generation cannot infer arbitrary resource identity/scope.**
A custom evaluator supplies kind and apiVersion. Kubernetes discovery determines
its plural resource and scope; pluralization cannot be guessed safely. Meanwhile
requestedReads carries exact resource plurals but does not associate individual
rules with target scopes. For a definition mixing cluster and namespaced checks,
binding-scope union alone cannot justify a cluster-wide grant for every declared
resource. T020–T022 must not silently broaden grants. The user selected offline partial output with explicit diagnostics (option A).
The renderer must flag unresolved custom-resource grants for manual completion. Runtime requestedReads remains non-authoritative.

T020 now implements this decision in a pure grant planner and staged YAML renderer.
Regression tests cover mixed-scope isolation, helper grants, and manual-completion
diagnostics. CLI exposure and generated inventory remain pending in T021/T022.

## Reproduced and fixed findings

- **R2 — Comparator-count bypass through whitespace.** The initial range counter
  split on space/tab/newline/comma while the SemVer parser also accepted carriage
  returns and form feeds. Seventeen `>=1` comparators separated by either passed
  the sixteen-comparator cap. `TestVersionComparatorLimitIncludesAllWhitespace`
  reproduced both cases, then passed after using Unicode whitespace splitting.
- **R3 — Empty explicit outcome accepted offline.** `valueOutcomes` used the
  helper that permits omitted optional outcome defaults, accepting an explicit
  empty map value that admission rejects. `TestPayloadConstraints/field_empty_explicit_outcome`
  failed before the fix. Explicit map values now require a real outcome.
- **R4 — Ignored schema marker.** The attempted additionalProperties length marker
  did not bound PodProjection selector values in generated YAML. A constrained
  value type now emits the bound; `TestDefinitionSchemaLimitParity` checks it.
- **R5 — Maximum-size fixture could reject for the wrong reason.** Admission
  overwrites an unstructured object's map; modifying a stale map then changing
  metadata.name could test identity mismatch instead of the family cap. The
  over-limit object is now constructed separately and the test checks that the
  error identifies spec.families.
- **R6 — Binding definition name too broad.** A shared SA/reference type allowed
  253-character DNS subdomains for definitionRef.name. The definition reference
  now has its own DNS-label type (63 characters) while the SA reference retains
  the resource-name bound. Both whole reference objects remain immutable.

## Verified boundaries and limitations

- Compiler owns a deep copy and preserves mixed ConfigMap/CronJob declaration
  order; existing built-in evaluator tests pass.
- Invalid singleton namespace/name overrides fail before reads. The nil-client
  regression makes any accidental fallback read fail visibly.
- All nine payload branches have semantic and API admission fixtures. Binding
  UID immutability, status bounds, strict rejection/pruning behavior, and the
  512-check admission-cost fixture exercise the real pinned envtest API server.
  Latest test outcomes are tracked in execution.md; do not infer completion of
  every contract row from these representative cases.
- No runtime adapter is registered by the manager. The compiled adapter alone
  is not a secure execution boundary: the planned scoped impersonated client,
  counters, supervisor and publication fences are still required.
- Separate issue #256 remains an explicit release dependency.


Additional real-cluster evidence: both new CRD schemas passed server-side dry-run
on the user-designated Kubernetes 1.36.2 development cluster without persisting
resources. Existing built-in checks there were healthy. Runtime authority and
publication remain untested because that implementation is not installed.
