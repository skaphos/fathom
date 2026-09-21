<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Typed definition wire contract

Proposed first alpha schema for implementation, grounded in
[definition.go](../../../internal/adapter/declarative/definition.go) and
[engine.go](../../../internal/adapter/declarative/engine.go). This is an explicit
wire contract, not Go-struct serialization. Global caps in [runtime.md](runtime.md)
apply to every field; tighter bounds below win. Spec nesting depth counts the spec
object as depth 0 and increments for each nested object, array or scalar child;
this includes namespace-list elements and must remain ≤8. No evaluator or expression language
is added. Required means present and nonempty, unless a numeric zero is stated.

## Envelope, family and check

| Field | Type, presence, default and validation |
| --- | --- |
| spec.addonType | Required lowercase DNS label, 1–63 ASCII; immutable; equals metadata.name |
| spec.adapterVersion | Required SemVer string, ≤256 bytes; parsed after length check |
| spec.semanticsVersion | Required integer, supported value 1; unsupported stored values cannot compile |
| spec.optional | Boolean, default false |
| spec.supportedVersions | Optional string; empty disables addon-version gating; range limits apply; nonempty requires versionSource |
| spec.versionSource | Optional object: required fromFamily and fromComponent (identifiers ≤63); optional container (DNS label ≤63); resolves exactly one Workload payload/component in that family |
| spec.families | Required ordered list, 1–16 entries; unique name; no sorting during conversion |
| family.name | Required identifier, 1–63 ASCII matching `[a-z][a-z0-9_-]*`; preserves existing underscore family names |
| family.defaultEnabled | Boolean, default false |
| family.checks | Required ordered list, 1–32 entries; ≤512 total across definition |
| check.name | Required identifier, 1–63 ASCII matching `[a-z][a-z0-9_-]*`; unique within family; stable check identity |
| check.kind | Required enum: Workload, CRD, Condition, Field, Webhook, CronJob, ConfigMap, AnnotationStaleness, PodProjection |
| check payload | Exactly one object matching kind: workload, crd, condition, field, webhook, cronJob, configMap, annotationStaleness, podProjection |

There is no opaque payload, arbitrary JSON blob or preserve-unknown-fields island.
Fixed admission CEL rejects unknown kinds/mismatched unions; structural schemas
and the compiler validate the explicit field vocabulary. Normal Kubernetes unknown-
field pruning/strict validation behavior is not advertised as unconditional rejection.

## Common target and conversion rules

Every payload requires `target`: `{scope: Namespaced|Cluster, namespaces: []}`.
Namespaces is a set of exact DNS-label names, ≤32 entries, each ≤63 characters;
Cluster requires empty namespaces, Namespaced requires at least one. Namespaced
singleton evaluators require exactly one default namespace. Namespaced collection
checks may declare several. No all-namespaces request is permitted for a runtime
check. Effective namespaces come from nonempty family policy namespaces or the
payload defaults; all must be inside binding scope. Reject an out-of-scope request
rather than silently dropping targets and reporting partial coverage. Cluster
operations require allowClusterScoped. Discovery must confirm the declared scope.

`target.scope` maps to engine ClusterScoped where present; singleton namespace
maps to DefaultNamespace. Collection execution supplies the explicit namespace
list in its per-check policy. Namespace policy overrides are limited to one for
singletons, so firstNamespace never silently ignores extra runtime targets.
Helpers (pods, CRD resolution, services, EndpointSlices) require the same identity
and independently authorized target scope; a namespaced primary can legitimately
need cluster-scoped CRD discovery, which requires allowClusterScoped.

`absence`, where listed, is optional enum Required|Optional; omission inherits
spec.optional (Optional if true, otherwise Required). It maps to Absence. Outcome
fields use the existing enum Pass|Warn|Fail|Error|Skipped; absent values take the
listed default. An Error result is a failed attempt, never completed health evidence.
Strings use the global 1,024-byte/code-point ceiling unless a tighter limit applies.
Resource names are nonempty DNS subdomains ≤253 bytes; namespace names are DNS labels.
Component identifiers match check.name syntax, ≤63 ASCII; omitted component defaults
to check.name. Workload components must be unique within a family for version lookup.

Threshold-key fields are optional strings ≤63 ASCII matching `[A-Za-z][A-Za-z0-9_.-]*`;
omission disables that override. Reserved engine ratio keys cannot be used for
payload-specific overrides; apply #256's disposition before runtime release.
Validate resolved overrides against the same field type, scope and size limits
before any read. No implicit fallback after an invalid supplied override.
Duration fields are bounded strings parsed with the existing Go duration grammar,
≤256 bytes; values must fit signed 64-bit nanoseconds. These are observation-age
thresholds, distinct from the 30-second execution deadline.

Unless listed as special below, lower-camel fields map directly to the matching
exported engine field (apiVersion → APIVersion, recognizedAPIVersions →
RecognizedAPIVersions, envVar → EnvVar). Deep-copy all collections and assemble
an explicit ordered evaluator sequence; do not regroup runtime checks by bucket.

## Workload → WorkloadCheck

Required Namespaced singleton target; API fixed apps/v1.

| Field | Contract |
| --- | --- |
| kind | Required Deployment|DaemonSet|StatefulSet |
| defaultName | Required resource name |
| nameThresholdKey | Optional name override key |
| component | Optional identifier; defaults check.name |
| absence | Optional inherited posture |
| checkPods | Boolean default false; pod helper stays in effective namespace |
| restartWarnThresholdKey | Optional integer override key |
| defaultRestartWarn | int32 ≥0, default 0; existing restart comparison semantics |

## CRD → CRDCheck

Required Cluster target; fixed apiextensions.k8s.io/v1 CustomResourceDefinition.

| Field | Contract |
| --- | --- |
| names | Required unique resource names, 1–32 |
| supportedVersions | Required ordered unique API version tokens, 1–8, each ≤253 bytes |
| absence | Optional inherited posture |
| unsupportedVersionOutcome | Outcome, default Warn |

## Condition → ConditionCheck

Named mode when names is nonempty; otherwise collection mode. Namespaced named
mode requires one target namespace. APIService is this payload with
apiVersion=apiregistration.k8s.io/v1, kind=APIService and Cluster scope.

| Field | Contract |
| --- | --- |
| apiVersion | Required core version or group/version; each segment ≤253 bytes |
| kind | Required Kubernetes kind token, ≤253 bytes |
| listKind | Required in collection mode, absent in named mode; kind token ≤253 bytes |
| listName | Optional result label ≤63 ASCII; default check.name |
| names | Optional unique list of 1–32 resource names; omission selects collection |
| versionCRD | Optional CRD resource name; requires supportedVersions |
| supportedVersions | Ordered unique tokens, 1–8 when versionCRD set; otherwise absent |
| absence | Optional inherited posture; named mode |
| conditionType | Required nonempty string ≤253 bytes |
| expectedStatus | Required True|False|Unknown |
| absentCondition | Outcome, default Fail |
| mismatch | Outcome, default Fail |

Collection selectors come from bounded family policy.labelSelector; omitted means
all objects in the explicit effective namespace/cluster scope. Named mode ignores
label selectors, as the existing engine does. Fallback served-version lookup uses
the same scoped client/budgets. APIVersion's version remains fallback if no preferred
version is found, preserving current evaluator semantics.

## Field → FieldCheck

Collection only, explicit target scope. Reads string-valued scalar fields, matching
the existing evaluator; there is no automatic numeric/boolean string conversion.

| Field | Contract |
| --- | --- |
| apiVersion, kind, listKind | Required; syntax/bounds as Condition |
| listName | Optional ≤63 ASCII, default check.name |
| absence | Optional inherited posture for missing resource API |
| fieldPath | Required 1–16 literal segments, each 1–128 bytes; no expression syntax |
| expectedValue | Required nonempty string; matching value yields Pass |
| valueOutcomes | Optional map of observed strings to Outcome, ≤32 entries; key equal to expectedValue rejected as unreachable |
| absentOutcome | Outcome, default Warn for missing/empty/non-string field |
| otherOutcome | Outcome, default Warn |

Policy selectors and namespaces follow Condition collection rules.

## Webhook → WebhookCheck

Required Cluster target for admissionregistration.k8s.io/v1 configuration.

| Field | Contract |
| --- | --- |
| kind | Required MutatingWebhookConfiguration|ValidatingWebhookConfiguration |
| name | Required resource name |
| nameThresholdKey | Optional override key |
| expectedService | Optional service DNS-label name ≤63; requires serviceNamespace |
| serviceNamespace | Optional DNS label ≤63; requires expectedService; one policy namespace may override it |
| absence | Optional inherited posture |
| verifyEndpoints | Boolean default false; true requires expectedService and serviceNamespace |

For service helpers, the resolved serviceNamespace must be in the binding namespace
allowlist as well as having Cluster scope permitted for the primary. Zero or one
policy namespace is valid; multiple namespaces are rejected before reads.

## CronJob → CronJobCheck

Required Namespaced singleton target; fixed batch/v1 CronJob.

| Field | Contract |
| --- | --- |
| defaultName | Required resource name |
| nameThresholdKey | Optional override key |
| component | Optional identifier, default check.name |
| absence | Optional inherited posture |
| successMaxAgeThresholdKey | Optional duration override key |
| defaultSuccessMaxAge | Duration ≥0, default 0s disables recency checking |
| staleOutcome | Outcome, default Warn |

## ConfigMap → ConfigMapCheck

Required Namespaced singleton target; fixed v1 ConfigMap.

| Field | Contract |
| --- | --- |
| defaultName | Required resource name |
| nameThresholdKey | Optional override key |
| component | Optional identifier, default check.name |
| absence | Optional inherited posture |
| key | Required ConfigMap data-key syntax, ≤253 bytes |
| recognizedAPIVersions | Optional unique list ≤8 group/version strings; empty disables assertion |
| unrecognizedOutcome | Outcome, default Warn |
| invalidOutcome | Outcome, default Fail |

YAML value/node/depth/alias caps apply before parsing or expansion.

## AnnotationStaleness → AnnotationStalenessCheck

Named mode when listKind absent; collection mode otherwise. Namespaced named mode
requires one namespace; collection mode iterates explicit effective namespaces.

| Field | Contract |
| --- | --- |
| apiVersion, kind | Required; syntax/bounds as Condition |
| listKind | Optional kind token; present selects collection mode |
| listName | Optional ≤63 ASCII, default check.name |
| defaultName | Required in named mode; absent in collection mode |
| nameThresholdKey | Optional in named mode, absent in collection mode |
| component | Optional identifier, default check.name |
| absence | Optional inherited posture for named mode |
| annotationKey | Required Kubernetes qualified annotation key; prefix ≤253 bytes and name ≤63 |
| timestampJSONField | Optional literal top-level JSON key ≤128 bytes; absent means whole value is RFC3339; no nested expressions |
| maxAgeThresholdKey | Optional duration override key |
| defaultMaxAge | Required duration >0 |
| staleOutcome | Outcome, default Warn |

## PodProjection → PodProjectionCheck

Required Namespaced collection target; fixed v1 Pod. The payload selector is
authoritative; family policy.labelSelector cannot narrow the injection population.

| Field | Contract |
| --- | --- |
| selector | Required nonempty map of valid Kubernetes label keys/values, ≤32 entries; Kubernetes label grammar also applies |
| listName | Optional ≤63 ASCII, default check.name |
| component | Optional identifier, default check.name |
| volumeName | Required DNS label ≤63 |
| envVar | Optional nonempty C-identifier syntax when supplied, ≤253 bytes; absence disables assertion |
| missingOutcome | Outcome, default Fail |

## Declared reads and validation split

requestedReads is an optional list ≤32 of closed rule objects. Each selects exactly
one form: resource `{apiGroup, resources, verbs, resourceNames?}` or discovery
`{nonResourceURLs, verbs}`. apiGroup may be empty for core; resources is 1–32 exact
plural identifiers; resourceNames optional ≤32 exact names; verbs is a nonempty
unique subset of get/list. Discovery rules allow get only, 1–32 exact paths in
`/api`, `/apis`, `/api/<version>`, `/apis/<group>`, `/apis/<group>/<version>`;
segments use group/version grammar. No wildcard, query, fragment, traversal, write
verb or arbitrary URL. Namespace scope is derived from targets and binding, never
from an implicit grant here. Render only required get/list grants; incomplete
requestedReads produces a diagnostic, not authority denial for an otherwise
permitted evaluator request.

Admission checks structural lengths/counts/enums, unions and local relationships.
Compiler checks UTF-8 bytes, canonical serialized size/depth, SemVer/token limits,
component references, reserved keys and deterministic conversion. Live discovery
checks scope under delegated identity; local CEL cannot inspect other objects.
The implementation must test every row's valid, missing/default and invalid case,
then repeat resolved policy override constraints. Never raise an RFC cap to make
one maximum-size fixture fit a run; request/time limits can be tighter than schema.
