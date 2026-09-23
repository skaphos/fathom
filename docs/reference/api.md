# API Reference

## Packages
- [fathom.skaphos.io/v1alpha1](#fathomskaphosiov1alpha1)


## fathom.skaphos.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the fathom v1alpha1 API group.

### Resource Types
- [AddonCheck](#addoncheck)
- [AddonCheckList](#addonchecklist)
- [AddonDefinition](#addondefinition)
- [AddonDefinitionBinding](#addondefinitionbinding)
- [AddonDefinitionBindingList](#addondefinitionbindinglist)
- [AddonDefinitionList](#addondefinitionlist)
- [ClusterHealth](#clusterhealth)
- [ClusterHealthList](#clusterhealthlist)
- [DNSCheck](#dnscheck)
- [DNSCheckList](#dnschecklist)
- [HealthCheck](#healthcheck)
- [HealthCheckList](#healthchecklist)
- [HealthReport](#healthreport)
- [HealthReportList](#healthreportlist)
- [NodeCertificateCheck](#nodecertificatecheck)
- [NodeCertificateCheckList](#nodecertificatechecklist)
- [NodeHealthCheck](#nodehealthcheck)
- [NodeHealthCheckList](#nodehealthchecklist)



#### AddonCheck



AddonCheck is the Schema for the addonchecks API.



_Appears in:_
- [AddonCheckList](#addonchecklist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `AddonCheck` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AddonCheckSpec](#addoncheckspec)_ |  |  |  |
| `status` _[AddonCheckStatus](#addoncheckstatus)_ |  |  |  |


#### AddonCheckAttemptOutcome

_Underlying type:_ _string_

AddonCheckAttemptOutcome is the outcome of the LATEST attempt, which may be
older evidence's failed successor.

_Validation:_
- Enum: [Completed Error]

_Appears in:_
- [AddonCheckStatus](#addoncheckstatus)

| Field | Description |
| --- | --- |
| `Completed` | AddonCheckAttemptCompleted means the run executed to completion with<br />eligible inputs. It is not a verdict.<br /> |
| `Error` | AddonCheckAttemptError means the attempt did not produce completed<br />evidence. Whatever evidence was already stored is preserved unchanged.<br /> |


#### AddonCheckEvidence



AddonCheckEvidence is the last COMPLETED evaluation: its verdict, what it
covered, and the original observation time, revision and authority context
it was produced under.

Nothing but another completed run replaces it. A failed attempt leaves every
field here untouched — including ObservedAt, which a failed attempt may never
renew.



_Appears in:_
- [AddonCheckStatus](#addoncheckstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `verdict` _[AddonCheckEvidenceVerdict](#addoncheckevidenceverdict)_ | Verdict is the aggregate result of the completed run. |  | Enum: [Pass Warn Fail Skipped] <br /> |
| `coverage` _[AddonCheckEvidenceCoverage](#addoncheckevidencecoverage)_ | Coverage distinguishes an assessed verdict from a completed run that<br />evaluated nothing. |  | Enum: [ChecksEvaluated NoChecksEvaluated] <br /> |
| `message` _string_ | Message explains the coverage in one line. A completed all-Skipped run<br />records exactly [AddonCheckNoChecksEvaluatedMessage]. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | ObservedAt is when the completed run finished. It is the evidence's<br />ORIGINAL observation time and is never advanced by a failed attempt. |  |  |
| `revision` _[AddonCheckEvidenceRevision](#addoncheckevidencerevision)_ | Revision is the runtime definition revision that produced the evidence. |  | Optional: \{\} <br /> |
| `authority` _[AddonCheckEvidenceAuthority](#addoncheckevidenceauthority)_ | Authority is the delegated authority and policy context the evidence was<br />attributed to. |  | Optional: \{\} <br /> |


#### AddonCheckEvidenceAuthority



AddonCheckEvidenceAuthority is the delegated authority and policy context a
completed evaluation was attributed to. It is recorded so an operator can
see which administrator-authorized incarnation produced a verdict, and so a
later run under different authority cannot be mistaken for the same
observation.



_Appears in:_
- [AddonCheckEvidence](#addoncheckevidence)
- [HealthReportAttribution](#healthreportattribution)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bindingUID` _string_ | BindingUID is the AddonDefinitionBinding incarnation that authorized the<br />run. |  | MaxLength: 128 <br />Optional: \{\} <br /> |
| `bindingGeneration` _integer_ | BindingGeneration is the binding's spec generation. Status-only binding<br />writes do not advance it and therefore do not invalidate evidence. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `serviceAccountUID` _string_ | ServiceAccountUID is the dedicated reader incarnation the evaluation<br />impersonated. |  | MaxLength: 128 <br />Optional: \{\} <br /> |
| `checkUID` _string_ | CheckUID is this AddonCheck's incarnation. |  | MaxLength: 128 <br />Optional: \{\} <br /> |
| `checkGeneration` _integer_ | CheckGeneration is the AddonCheck spec generation the run was attributed<br />to. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `policyDigest` _string_ | PolicyDigest fingerprints spec.policy. The policy selects which families<br />run, so evidence produced under one policy is not interchangeable with<br />evidence produced under another at the same generation. |  | MaxLength: 128 <br />Optional: \{\} <br /> |
| `leaderEpoch` _[DefinitionLeaderEpoch](#definitionleaderepoch)_ | LeaderEpoch is the leadership epoch observed when the run was admitted. |  | Optional: \{\} <br /> |


#### AddonCheckEvidenceCoverage

_Underlying type:_ _string_

AddonCheckEvidenceCoverage records what a completed run actually evaluated,
so a Skipped verdict cannot be mistaken for an assessed-and-healthy one.

_Validation:_
- Enum: [ChecksEvaluated NoChecksEvaluated]

_Appears in:_
- [AddonCheckEvidence](#addoncheckevidence)
- [HealthReportAttribution](#healthreportattribution)

| Field | Description |
| --- | --- |
| `ChecksEvaluated` | AddonCheckCoverageChecksEvaluated means at least one check produced a<br />health observation.<br /> |
| `NoChecksEvaluated` | AddonCheckCoverageNoChecksEvaluated means the run completed but every<br />check was Skipped, or no check ran at all. Its message is exactly<br />[AddonCheckNoChecksEvaluatedMessage].<br /> |


#### AddonCheckEvidenceFreshness

_Underlying type:_ _string_

AddonCheckEvidenceFreshness describes the recency and eligibility of the
stored completed evidence. It is derived, never authority: freshness says
nothing about health, and Current does not mean Pass.

_Validation:_
- Enum: [Current Stale Superseded Unavailable]

_Appears in:_
- [AddonCheckStatus](#addoncheckstatus)
- [HealthCheckStatus](#healthcheckstatus)

| Field | Description |
| --- | --- |
| `Current` | AddonCheckEvidenceCurrent means the evidence was observed at most two<br />effective intervals plus one effective timeout ago, from inputs that are<br />still eligible.<br /> |
| `Stale` | AddonCheckEvidenceStale means the evidence aged past that window. It<br />applies even when the stored verdict was Pass: "Evidence ages out \|<br />Freshness=Stale even if stored verdict was Pass".<br /> |
| `Superseded` | AddonCheckEvidenceSuperseded means the revision or context the evidence<br />was produced under has been replaced.<br /> |
| `Unavailable` | AddonCheckEvidenceUnavailable means the definition, binding or grants<br />that produced the evidence are gone, invalid or denied — or no evidence<br />has ever been recorded.<br /> |


#### AddonCheckEvidenceRevision



AddonCheckEvidenceRevision is the immutable runtime revision a completed
evaluation was produced by: the definition incarnation plus the publication
provenance (data-model.md, "Snapshot and publication context").



_Appears in:_
- [AddonCheckEvidence](#addoncheckevidence)
- [HealthReportAttribution](#healthreportattribution)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `definitionUID` _string_ | DefinitionUID is the AddonDefinition incarnation. A recreated definition<br />has a new UID and inherits no authority from the old one. |  | MaxLength: 128 <br />Optional: \{\} <br /> |
| `definitionGeneration` _integer_ | DefinitionGeneration is the definition's spec generation. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `schemaVersion` _string_ | SchemaVersion is the API schema version the definition was compiled from. |  | MaxLength: 63 <br />Optional: \{\} <br /> |
| `semanticsVersion` _integer_ | SemanticsVersion is the declared evaluation semantics version. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `adapterVersion` _string_ | AdapterVersion is the compiled adapter's own version. |  | MaxLength: 128 <br />Optional: \{\} <br /> |
| `operatorBuild` _string_ | OperatorBuild is the operator build that compiled the snapshot. |  | MaxLength: 128 <br />Optional: \{\} <br /> |


#### AddonCheckEvidenceVerdict

_Underlying type:_ _string_

AddonCheckEvidenceVerdict is the aggregate verdict of a COMPLETED runtime
evaluation.

It is deliberately narrower than [AddonCheckStatus.LastResult]: Error and
Unknown are not completed evidence. Unknown is the absence of evidence
("Unknown applies when no evidence exists"), and a run whose aggregate is
Error could not determine health, so it is recorded as an attempt error that
preserves whatever evidence was already stored.

_Validation:_
- Enum: [Pass Warn Fail Skipped]

_Appears in:_
- [AddonCheckEvidence](#addoncheckevidence)

| Field | Description |
| --- | --- |
| `Pass` |  |
| `Warn` |  |
| `Fail` |  |
| `Skipped` |  |


#### AddonCheckFamilyPolicy



AddonCheckFamilyPolicy configures one adapter-defined family of checks.



_Appears in:_
- [AddonCheckSpec](#addoncheckspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled gates execution of this family. | true | Optional: \{\} <br /> |
| `namespaces` _string array_ | Namespaces narrows this family to resources in specific namespaces. Empty<br />means all namespaces the adapter can read. Each entry must be a valid<br />namespace name (DNS-1123 label); at most 64 entries. |  | MaxItems: 64 <br />items:MaxLength: 63 <br />items:Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `labelSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#labelselector-v1-meta)_ | LabelSelector narrows this family to resources matching the selector.<br />Selector structure and label syntax are validated at reconcile time and<br />reported through the Accepted condition (a CEL admission rule for the<br />structural checks exceeds the API server's per-CRD cost budget, because<br />the imported LabelSelector schema carries no size bounds the estimator<br />could use). |  | Optional: \{\} <br /> |
| `thresholds` _object (keys:string, values:[ThresholdValue](#thresholdvalue))_ | Thresholds carries adapter-specific string knobs, such as warnDays or<br />failDays. Adapter documentation defines the supported keys; unknown keys<br />are never rejected at admission. Keys documented as numeric are<br />shape-checked at admission: warnDays and failDays must be 1-4 digit<br />integers, warnRatio and failRatio must be percentage-shaped — at most<br />three integer digits, up to two decimals, optional trailing '%'. The<br />0-100 range and cross-key semantics stay with the adapter and surface<br />via the Accepted condition. At most 16 keys. |  | MaxProperties: 16 <br />Optional: \{\} <br /> |


#### AddonCheckList



AddonCheckList contains a list of AddonCheck.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `AddonCheckList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AddonCheck](#addoncheck) array_ |  |  |  |


#### AddonCheckSpec



AddonCheckSpec defines the desired state of AddonCheck.



_Appears in:_
- [AddonCheck](#addoncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `addonType` _string_ | AddonType selects the adapter responsible for this check, such as<br />cert-manager, coredns, or external-secrets. |  | MinLength: 1 <br /> |
| `interval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#duration-v1-meta)_ | Interval is the cadence at which the adapter re-runs and the HealthReport<br />is refreshed. Defaults to 5m when unset. Must be at least 10s<br />(MinCheckInterval); the operator clamps stored objects that predate this<br />floor to it at runtime. |  | Optional: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#duration-v1-meta)_ | Timeout bounds a single adapter run. Must be at least 1s<br />(MinCheckTimeout); the operator clamps stored objects that predate this<br />floor to it at runtime. |  | Optional: \{\} <br /> |
| `paused` _boolean_ | Paused prevents the controller from starting new adapter runs. |  | Optional: \{\} <br /> |
| `policy` _object (keys:string, values:[AddonCheckFamilyPolicy](#addoncheckfamilypolicy))_ | Policy configures adapter-defined check families. A missing or empty policy<br />leaves family selection to the adapter defaults. Keys are adapter family<br />names: 1-63 lowercase alphanumerics with interior '-' or '_' (e.g.<br />system_health). Whether a well-formed key names a family the selected<br />adapter actually supports is judged at reconcile time via the Accepted<br />condition. |  | MaxProperties: 32 <br />Optional: \{\} <br /> |
| `historyLimit` _integer_ | HistoryLimit caps the number of HealthReports retained for this<br />AddonCheck. After each new HealthReport is created the controller<br />deletes the oldest reports until the total count is at or below this<br />limit. The minimum of 1 keeps Status.LastReportName referenceable. | 10 | Minimum: 1 <br />Optional: \{\} <br /> |


#### AddonCheckStatus



AddonCheckStatus defines the observed state of AddonCheck.



_Appears in:_
- [AddonCheck](#addoncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent metadata.generation reconciled by<br />the controller. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | Conditions summarize whether the controller accepted and processed this<br />check specification. |  | Optional: \{\} <br /> |
| `lastRunTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | LastRunTime records when an adapter run last completed. |  | Optional: \{\} <br /> |
| `lastResult` _string_ | LastResult is the aggregate result from the most recent adapter run. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `absent` _integer_ | Absent is the number of checks in the most recent run whose target was not<br />installed — the required-absent Fails and optional-absent Skips alike. It<br />makes "not installed" queryable and distinct from "unhealthy" (a Fail whose<br />target exists) and "disabled" (a Skipped family). Zero when every checked<br />target is present (SKA-526). |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `detectedVersion` _string_ | DetectedVersion is the installed addon release version detected on the most<br />recent run (from the addon workload's app.kubernetes.io/version label, else<br />its container image tag). Empty when the adapter does not detect versions or<br />the version was undetectable — the run then proceeds best-effort (SKA-527). |  | Optional: \{\} <br /> |
| `lastReportName` _string_ | LastReportName names the HealthReport created for the most recent run. |  | Optional: \{\} <br /> |
| `lastRunTrigger` _string_ | LastRunTrigger records the value of the fathom.skaphos.io/run-now<br />annotation most recently consumed to force an adapter run. The controller<br />re-runs the adapter whenever the annotation value differs from this, then<br />stores it here so a given on-demand trigger fires exactly once. |  | Optional: \{\} <br /> |
| `lastSuccessfulEvaluation` _[AddonCheckEvidence](#addoncheckevidence)_ | LastSuccessfulEvaluation is the last COMPLETED evaluation, with its own<br />original observation time, revision and authority context. It is replaced<br />only by another completed run; a failed attempt preserves it byte for<br />byte. Absent means no evidence exists at all, which reads as an Unknown<br />verdict rather than as a healthy or unhealthy one. |  | Optional: \{\} <br /> |
| `latestAttemptAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | LatestAttemptAt is when the most recent attempt — successful or not —<br />finished. It advances on every attempt, which is precisely what makes it<br />distinguishable from LastSuccessfulEvaluation.ObservedAt. |  | Optional: \{\} <br /> |
| `latestAttemptOutcome` _[AddonCheckAttemptOutcome](#addoncheckattemptoutcome)_ | LatestAttemptOutcome records whether the most recent attempt completed<br />with eligible inputs. Completed is not a verdict and Error does not<br />invalidate stored evidence. |  | Enum: [Completed Error] <br />Optional: \{\} <br /> |
| `latestAttemptReason` _string_ | LatestAttemptReason is the contract reason for the most recent attempt's<br />outcome, chosen by the publication precedence order. |  | MaxLength: 128 <br />Optional: \{\} <br /> |
| `latestAttemptMessage` _string_ | LatestAttemptMessage explains the most recent attempt's outcome. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |
| `evidenceFreshness` _[AddonCheckEvidenceFreshness](#addoncheckevidencefreshness)_ | EvidenceFreshness describes the recency and eligibility of<br />LastSuccessfulEvaluation as of the most recent attempt. It is derived,<br />not authority, and it never implies a healthy verdict. |  | Enum: [Current Stale Superseded Unavailable] <br />Optional: \{\} <br /> |
| `evidenceFreshnessReason` _string_ | EvidenceFreshnessReason explains a freshness that is not Current. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |


#### AddonDefinition



AddonDefinition declares administrator-bound runtime health checks.



_Appears in:_
- [AddonDefinitionList](#addondefinitionlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `AddonDefinition` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AddonDefinitionSpec](#addondefinitionspec)_ |  |  |  |
| `status` _[AddonDefinitionStatus](#addondefinitionstatus)_ |  |  | Optional: \{\} <br /> |


#### AddonDefinitionBinding



AddonDefinitionBinding delegates a definition to a dedicated identity.



_Appears in:_
- [AddonDefinitionBindingList](#addondefinitionbindinglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `AddonDefinitionBinding` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AddonDefinitionBindingSpec](#addondefinitionbindingspec)_ |  |  |  |
| `status` _[AddonDefinitionBindingStatus](#addondefinitionbindingstatus)_ |  |  | Optional: \{\} <br /> |


#### AddonDefinitionBindingList



AddonDefinitionBindingList contains delegation bindings.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `AddonDefinitionBindingList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AddonDefinitionBinding](#addondefinitionbinding) array_ |  |  |  |


#### AddonDefinitionBindingSpec



AddonDefinitionBindingSpec is a bounded runtime definition contract.



_Appears in:_
- [AddonDefinitionBinding](#addondefinitionbinding)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `definitionRef` _[DefinitionReference](#definitionreference)_ | DefinitionRef is the declared definitionRef. |  |  |
| `serviceAccountRef` _[DefinitionObjectReference](#definitionobjectreference)_ | ServiceAccountRef is the declared serviceAccountRef. |  |  |
| `enabled` _boolean_ | Enabled is the declared enabled. | false | Optional: \{\} <br /> |
| `targetScope` _[DefinitionBindingScope](#definitionbindingscope)_ | TargetScope is the declared targetScope. |  |  |


#### AddonDefinitionBindingStatus



AddonDefinitionBindingStatus describes observation, never authorization.
Accepted records spec/identity validity; Ready records eligibility; Drained is
acknowledged only for disabled, observed-generation state with zero active
runs in the current leadership epoch. Consumers must independently verify the
epoch against the configured Lease before treating Drained as current.



_Appears in:_
- [AddonDefinitionBinding](#addondefinitionbinding)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the declared observedGeneration. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `activeRuns` _integer_ | ActiveRuns is the declared activeRuns. |  | Maximum: 4 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `leaderIdentity` _string_ | LeaderIdentity is the declared leaderIdentity. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `leaderEpoch` _[DefinitionLeaderEpoch](#definitionleaderepoch)_ | LeaderEpoch is the declared leaderEpoch. |  | Optional: \{\} <br /> |
| `conditions` _[DefinitionStatusCondition](#definitionstatuscondition) array_ | Conditions is the declared conditions. |  | MaxItems: 8 <br />Optional: \{\} <br /> |


#### AddonDefinitionList



AddonDefinitionList contains runtime definitions.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `AddonDefinitionList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AddonDefinition](#addondefinition) array_ |  |  |  |


#### AddonDefinitionSpec



AddonDefinitionSpec is a bounded runtime definition contract.



_Appears in:_
- [AddonDefinition](#addondefinition)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `addonType` _[DefinitionDNSLabel](#definitiondnslabel)_ | AddonType is the declared addonType. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br /> |
| `adapterVersion` _string_ | AdapterVersion is the declared adapterVersion. |  | MaxLength: 256 <br />MinLength: 1 <br /> |
| `semanticsVersion` _integer_ | SemanticsVersion is the declared semanticsVersion. |  | Enum: [1] <br /> |
| `optional` _boolean_ | Optional is the declared optional. | false | Optional: \{\} <br /> |
| `supportedVersions` _string_ | SupportedVersions is the declared supportedVersions. |  | MaxLength: 256 <br />Optional: \{\} <br /> |
| `versionSource` _[DefinitionVersionSource](#definitionversionsource)_ | VersionSource is the declared versionSource. |  | Optional: \{\} <br /> |
| `families` _[DefinitionFamily](#definitionfamily) array_ | Families is the declared families. |  | MaxItems: 16 <br />MinItems: 1 <br /> |
| `requestedReads` _[DefinitionReadRule](#definitionreadrule) array_ | RequestedReads is the declared requestedReads. |  | MaxItems: 32 <br />Optional: \{\} <br /> |


#### AddonDefinitionStatus



AddonDefinitionStatus is a bounded runtime definition contract.



_Appears in:_
- [AddonDefinition](#addondefinition)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the declared observedGeneration. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `revision` _string_ | Revision is the declared revision. |  | MaxLength: 256 <br />Optional: \{\} <br /> |
| `conditions` _[DefinitionStatusCondition](#definitionstatuscondition) array_ | Conditions is the declared conditions. |  | MaxItems: 8 <br />Optional: \{\} <br /> |


#### CheckTargetRef



CheckTargetRef references a supported specialized check resource
(AddonCheck, DNSCheck, NodeCertificateCheck, or NodeHealthCheck) whose status
a HealthCheck mirrors and surfaces for ClusterHealth aggregation.



_Appears in:_
- [HealthCheckSpec](#healthcheckspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | APIVersion of the target check resource. When empty, defaults to<br />fathom.skaphos.io/v1alpha1. |  | MaxLength: 317 <br />Optional: \{\} <br /> |
| `kind` _string_ | Kind of the target check resource: AddonCheck, DNSCheck,<br />NodeCertificateCheck, or NodeHealthCheck. |  | MaxLength: 63 <br />MinLength: 1 <br /> |
| `name` _string_ | Name of the target check resource. |  | MaxLength: 253 <br />MinLength: 1 <br /> |
| `namespace` _string_ | Namespace of the target check resource. When empty, the HealthCheck's<br />own namespace is used. |  | MaxLength: 253 <br />Optional: \{\} <br /> |


#### ClusterHealth



ClusterHealth is the Schema for the clusterhealths API. It is
cluster-scoped: one object rolls up HealthChecks across namespaces,
optionally narrowed by spec.namespaces (allowlist) or
spec.excludedNamespaces (denylist). See ClusterHealthSpec for precedence.



_Appears in:_
- [ClusterHealthList](#clusterhealthlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `ClusterHealth` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ClusterHealthSpec](#clusterhealthspec)_ |  |  |  |
| `status` _[ClusterHealthStatus](#clusterhealthstatus)_ |  |  |  |


#### ClusterHealthChildSummary



ClusterHealthChildSummary records one HealthCheck's contribution to the
aggregate. The aggregator never reads HealthReport history; it derives this
snapshot solely from HealthCheck.Status (per the AGENTS.md invariant).



_Appears in:_
- [ClusterHealthStatus](#clusterhealthstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `namespace` _string_ | Namespace of the contributing HealthCheck. |  | MaxLength: 63 <br />MinLength: 1 <br /> |
| `name` _string_ | Name of the contributing HealthCheck. |  | MaxLength: 253 <br />MinLength: 1 <br /> |
| `result` _[HealthReportResult](#healthreportresult)_ | Result mirrors the contributing HealthCheck's Status.Result. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `summary` _string_ | Summary mirrors the contributing HealthCheck's Status.Summary. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | ObservedAt mirrors the contributing HealthCheck's<br />Status.SourceObservedAt, when present. |  | Optional: \{\} <br /> |


#### ClusterHealthList



ClusterHealthList contains a list of ClusterHealth.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `ClusterHealthList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[ClusterHealth](#clusterhealth) array_ |  |  |  |


#### ClusterHealthSpec



ClusterHealthSpec defines the desired state of ClusterHealth. ClusterHealth
is an aggregator: it rolls up the Status of selected HealthCheck resources
into a single worst-case Result for cluster-wide consumers (dashboards,
alerting, gates).

Namespace scope uses allowlist-then-denylist precedence:

 1. If Namespaces is non-empty, only those namespaces are included
    (allowlist is definitive; ExcludedNamespaces is ignored).
 2. Else if ExcludedNamespaces is non-empty, every namespace except those
    listed is included (denylist).
 3. Else every namespace is in scope (open).

Cross-namespace HealthCheck.checkRef.namespace remains intentional: a
HealthCheck may mirror an AddonCheck in another namespace. Tenancy is
enforced by who can create those objects plus this aggregate's namespace
filter — not by forbidding cross-namespace refs.



_Appears in:_
- [ClusterHealth](#clusterhealth)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#labelselector-v1-meta)_ | Selector selects the HealthChecks whose status this aggregate rolls up.<br />An empty or nil selector matches every HealthCheck in the namespace<br />scope defined by Namespaces / ExcludedNamespaces. |  | Optional: \{\} <br /> |
| `namespaces` _string array_ | Namespaces is the allowlist of HealthCheck namespaces this aggregate<br />includes. When non-empty it is definitive: only listed namespaces are<br />considered and ExcludedNamespaces is ignored. Empty means "no allowlist"<br />(fall through to ExcludedNamespaces, then open). |  | MaxItems: 50 <br />items:MaxLength: 63 <br />items:MinLength: 1 <br />items:Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `excludedNamespaces` _string array_ | ExcludedNamespaces is the denylist of HealthCheck namespaces this<br />aggregate skips. Applied only when Namespaces is empty. Empty (with<br />Namespaces also empty) means open — every namespace is in scope. |  | MaxItems: 50 <br />items:MaxLength: 63 <br />items:MinLength: 1 <br />items:Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `description` _string_ | Description is a human-readable purpose for this aggregate. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |


#### ClusterHealthStatus



ClusterHealthStatus defines the observed state of ClusterHealth.



_Appears in:_
- [ClusterHealth](#clusterhealth)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent metadata.generation reconciled. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | Conditions summarize the controller's view of the aggregate. |  | Optional: \{\} <br /> |
| `result` _[HealthReportResult](#healthreportresult)_ | Result is the worst-case roll-up across the selected HealthChecks.<br />Unknown (with Ready=False, Reason=NoMatches) when no HealthChecks match<br />the selector; a selected child that has no verdict yet degrades the<br />roll-up to Unknown rather than being dropped, so a failure can never<br />silently vanish. Trust this value only when the Ready condition is True:<br />the InvalidSelector and ListFailed error paths leave it empty with<br />Ready=False. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `matchedCount` _integer_ | MatchedCount is the number of HealthChecks selected for this aggregate. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `children` _[ClusterHealthChildSummary](#clusterhealthchildsummary) array_ | Children summarizes each selected HealthCheck's contribution.<br />Capped at MaxClusterHealthChildren so a selector matching a large<br />population cannot grow the stored object without bound. When the selection<br />exceeds the cap the list is truncated, but Result and ObservedAt are still<br />computed from EVERY selected check — the cap limits what is reported, never<br />what is measured. MatchedCount stays the full pre-truncation total, so<br />matchedCount > len(children) is how a consumer detects truncation.<br />Truncation keeps the entries an operator actually needs: worst verdict<br />first, then stalest, then namespace/name for determinism. An arbitrary<br />alphabetical cut could otherwise hide the single failing or frozen child<br />that explains the roll-up. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | ObservedAt is the stalest observation backing this aggregate: the oldest<br />Status.SourceObservedAt across the selected HealthChecks. It answers "how<br />far back does my least-current evidence go", so a stale contributor makes<br />the whole roll-up read as stale even when a sibling is still running.<br />Empty when the aggregate matches nothing, or when any selected check has<br />never been evaluated — an unevaluated child is the strongest staleness<br />signal there is, and outranks every timestamp.<br />Alert on staleness relative to cadence rather than an absolute age; see<br />docs/guides/monitoring.md. |  | Optional: \{\} <br /> |


#### DNSCheck



DNSCheck is the Schema for the dnschecks API.



_Appears in:_
- [DNSCheckList](#dnschecklist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `DNSCheck` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DNSCheckSpec](#dnscheckspec)_ |  |  |  |
| `status` _[DNSCheckStatus](#dnscheckstatus)_ |  |  |  |


#### DNSCheckList



DNSCheckList contains a list of DNSCheck.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `DNSCheckList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[DNSCheck](#dnscheck) array_ |  |  |  |


#### DNSCheckSpec



DNSCheckSpec defines the desired state of DNSCheck.

A DNSCheck asserts that a set of names resolve — or deliberately do not —
from one or more vantage points, on a cadence. It reports a verdict per
(target, vantage point) pair and a single folded verdict for the check.

There is no field to pause a DNSCheck. Stopping a check means deleting it.
Per-target results are keyed by (name, recordType, resolver), so two targets
identical in all three would collide there. Rejecting the duplicate at write
time is far kinder than letting the controller fail a status update later
with an error that says nothing about the specification that caused it.



_Appears in:_
- [DNSCheck](#dnscheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `targets` _[DNSTarget](#dnstarget) array_ | Targets are the names this check asserts on. At least one is required —<br />a check with no targets would report a vacuous pass. |  | MaxItems: 16 <br />MinItems: 1 <br /> |
| `resolvers` _[DNSResolver](#dnsresolver) array_ | Resolvers are the vantage points resolution is performed from. When<br />empty, the check resolves through cluster DNS from an implicit vantage<br />point named "cluster".<br />A target that names no vantage point is checked against every entry<br />here, so the number of evaluations a check performs is<br />len(targets without an override) * max(1, len(resolvers)), bounded at<br />48. The max(1, …) is not a rounding nicety: an empty list still means one<br />vantage point — the implicit "cluster" one — so a check that declares no<br />resolvers evaluates every target once, not zero times. |  | MaxItems: 3 <br />Optional: \{\} <br /> |
| `interval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#duration-v1-meta)_ | Interval is the cadence at which the check re-runs. Defaults to 1m when<br />unset. Must be at least 10s (MinCheckInterval). |  | Optional: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#duration-v1-meta)_ | Timeout bounds a single evaluation. Defaults to 10s when unset. Must be<br />at least 1s (MinCheckTimeout) and must not exceed Interval. |  | Optional: \{\} <br /> |
| `historyLimit` _integer_ | HistoryLimit caps the number of HealthReports retained for this check.<br />The minimum of 1 keeps Status.LastReportName valid. | 10 | Minimum: 1 <br />Optional: \{\} <br /> |


#### DNSCheckStatus



DNSCheckStatus defines the observed state of DNSCheck.



_Appears in:_
- [DNSCheck](#dnscheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent metadata.generation reconciled by<br />the controller. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | Conditions summarize whether the controller accepted the spec. |  | Optional: \{\} <br /> |
| `lastRunTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | LastRunTime records when the check was last evaluated. |  | Optional: \{\} <br /> |
| `lastResult` _string_ | LastResult is the folded verdict across every (target, vantage point)<br />pair from the most recent evaluation — the most severe pair outcome. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `summary` _string_ | Summary is a human-readable one-line outcome. When a failure stems from<br />an absent assertion, it says so, so a deliberate "this must not resolve"<br />failure is not triaged as a DNS outage. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |
| `lastReportName` _string_ | LastReportName names the HealthReport capturing the current result. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `lastRunTrigger` _string_ | LastRunTrigger records the fathom.skaphos.io/run-now annotation value<br />most recently consumed, so a given on-demand trigger fires exactly once. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `targetResults` _[DNSTargetResult](#dnstargetresult) array_ | TargetResults holds one entry per (target, vantage point) pair. It is<br />rebuilt from the current spec on every evaluation rather than<br />accumulated, so a pair the spec no longer declares disappears instead of<br />freezing at its last verdict. |  | MaxItems: 48 <br />Optional: \{\} <br /> |
| `observedTargets` _integer_ | ObservedTargets is the number of (target, vantage point) pairs the most<br />recent evaluation covered. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### DNSRecordType

_Underlying type:_ _string_

DNSRecordType is the kind of DNS record a target expects.

Host is the default and is satisfied by an address of either family. A and
AAAA narrow to a single family and apply only when named explicitly — a
default of A would silently mean "IPv4 only" and fail an AAAA-only name for
a reason its author would not expect.

_Validation:_
- Enum: [Host A AAAA CNAME SRV PTR]

_Appears in:_
- [DNSTarget](#dnstarget)
- [DNSTargetResult](#dnstargetresult)

| Field | Description |
| --- | --- |
| `Host` |  |
| `A` |  |
| `AAAA` |  |
| `CNAME` |  |
| `SRV` |  |
| `PTR` |  |


#### DNSResolver



DNSResolver is a declared vantage point resolution is performed from.



_Appears in:_
- [DNSCheckSpec](#dnscheckspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name identifies this vantage point so targets can select it and results<br />can report it. The name "cluster" is reserved. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br /> |
| `from` _[DNSResolverSource](#dnsresolversource)_ | From is where this vantage point resolves: the cluster's own DNS<br />service, the node's resolver, or an explicitly addressed upstream. | Cluster | Enum: [Cluster Node Explicit] <br />Optional: \{\} <br /> |
| `address` _string_ | Address is the upstream resolver, as an IP with an optional port. It is<br />required when From is Explicit and rejected otherwise.<br />A hostname is not accepted: a resolver that must itself be resolved to be<br />reached cannot answer the question the check is asking.<br />The rule validates the host and the port separately. Checking only that<br />the value "looks addressy" would admit "10.0.0.10:abc" and<br />"[not-an-ip]:53", pushing a misconfiguration to runtime where it surfaces<br />as an unreachable resolver rather than as the typo it is. |  | MaxLength: 63 <br />Optional: \{\} <br /> |


#### DNSResolverSource

_Underlying type:_ _string_

DNSResolverSource is where a vantage point resolves from.

_Validation:_
- Enum: [Cluster Node Explicit]

_Appears in:_
- [DNSResolver](#dnsresolver)

| Field | Description |
| --- | --- |
| `Cluster` |  |
| `Node` |  |
| `Explicit` |  |


#### DNSTarget



DNSTarget is one subject plus the expectation attached to it.
The colon clause closes a gap the pattern alone leaves: the pattern must
admit IPv6 literals so a PTR subject can be one, which would otherwise let a
colon-bearing non-address such as "abc:def" through as an A target.



_Appears in:_
- [DNSCheckSpec](#dnscheckspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the subject to look up: a DNS name for forward record types, or<br />an IP address when RecordType is PTR. A trailing dot is accepted, and<br />SRV subjects may carry the conventional _service._proto labels.<br />A target checked against an Explicit vantage point must be fully<br />qualified. Such a pod runs with no cluster search domains, so a short<br />name that resolves fine through cluster DNS will not resolve there — a<br />failure that reads like an outage but is a missing suffix.<br />The pattern admits DNS names (optionally fully qualified, with the<br />underscore labels SRV needs) and IPv6 literals, because a PTR subject is<br />an address. It is deliberately coarse: the XValidation rules on this type<br />decide which of the two forms is legal for the declared record type. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^(_?[a-zA-Z0-9]([-a-zA-Z0-9]*[a-zA-Z0-9])?(\._?[a-zA-Z0-9]([-a-zA-Z0-9]*[a-zA-Z0-9])?)*\.?\|[0-9a-fA-F:]+)$` <br /> |
| `recordType` _[DNSRecordType](#dnsrecordtype)_ | RecordType is the kind of record expected. Defaults to Host, an address<br />lookup satisfied by either address family. | Host | Enum: [Host A AAAA CNAME SRV PTR] <br />Optional: \{\} <br /> |
| `expectedAnswers` _string array_ | ExpectedAnswers are answers the lookup must return. Matching is<br />containment, not equality: extra answers never fail the check, because<br />multi-address and round-robin records legitimately return supersets.<br />When empty, any non-empty answer satisfies the target. |  | MaxItems: 16 <br />items:MaxLength: 253 <br />Optional: \{\} <br /> |
| `absent` _boolean_ | Absent inverts the assertion: the target passes when the name does NOT<br />resolve. Use it to confirm a decommissioned name is really gone.<br />A resolver that cannot be reached never satisfies this — that is a<br />network fault, not evidence a name was retired, and it is reported as an<br />error rather than a pass. | false | Optional: \{\} <br /> |
| `resolver` _string_ | Resolver names the vantage point this target is checked from, either an<br />entry in spec.resolvers or the reserved name "cluster".<br />When empty the target is checked from EVERY declared vantage point, not<br />just the default one. Declaring three vantage points therefore triples<br />the cost of every target that does not name one: three probe runs, three<br />per-target results, and three sets of metric series. |  | MaxLength: 63 <br />Optional: \{\} <br /> |


#### DNSTargetResult



DNSTargetResult is the outcome for one (target, vantage point) pair on the
most recent evaluation. Identity is the name, record type, and resolver
together, so the same name checked from two vantage points produces two
distinct entries rather than colliding.



_Appears in:_
- [DNSCheckStatus](#dnscheckstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the subject that was looked up. |  | MaxLength: 253 <br /> |
| `recordType` _[DNSRecordType](#dnsrecordtype)_ | RecordType is the record kind that was queried, echoed so a result is<br />self-describing without cross-referencing the spec. |  | Enum: [Host A AAAA CNAME SRV PTR] <br /> |
| `resolver` _string_ | Resolver is the vantage point the query was issued from, or "cluster"<br />for the implicit one. |  | MaxLength: 63 <br /> |
| `result` _string_ | Result is the outcome for this pair. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br /> |
| `message` _string_ | Message says what was asked and what came back. |  | MaxLength: 512 <br />Optional: \{\} <br /> |
| `answers` _string array_ | Answers are the records returned, retained as the evidence behind the<br />verdict. |  | MaxItems: 16 <br />items:MaxLength: 253 <br />Optional: \{\} <br /> |
| `latencyMillis` _integer_ | LatencyMillis is how long the lookup took, as measured by the probe<br />inside its pod. It excludes pod scheduling and start-up, and is absent<br />when the probe reported no measurement (for example, a pair that was<br />never reached). It is recorded as evidence only; slow resolution is not<br />by itself a failure in this API version. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `runMillis` _integer_ | RunMillis is the wall time Fathom spent performing this pair: probe pod<br />scheduling, image pull, any admission-injected init containers, the<br />lookup itself, result collection and the probe Pod delete request (which<br />does not wait for termination). It is what the pair costs against the<br />run bound, not the probe's own runtime. When it dwarfs LatencyMillis, pod<br />start-up rather than DNS is what consumes the run bound (spec.timeout). |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### DefinitionAnnotationStaleness



DefinitionAnnotationStaleness is a bounded runtime definition contract.



_Appears in:_
- [DefinitionCheck](#definitioncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `target` _[DefinitionTarget](#definitiontarget)_ | Target is the declared target. |  |  |
| `apiVersion` _string_ | APIVersion is the declared apiVersion. |  | MaxLength: 507 <br />MinLength: 1 <br />Pattern: `^([a-z0-9]([a-z0-9.-]*[a-z0-9])?/)?[a-z][a-z0-9]*$` <br /> |
| `kind` _[DefinitionToken](#definitiontoken)_ | Kind is the declared kind. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9]*$` <br /> |
| `listKind` _[DefinitionToken](#definitiontoken)_ | ListKind is the declared listKind. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9]*$` <br />Optional: \{\} <br /> |
| `listName` _[DefinitionIdentifier](#definitionidentifier)_ | ListName is the declared listName. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br />Optional: \{\} <br /> |
| `defaultName` _[DefinitionResourceName](#definitionresourcename)_ | DefaultName is the declared defaultName. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `nameThresholdKey` _[DefinitionThresholdKey](#definitionthresholdkey)_ | NameThresholdKey is the declared nameThresholdKey. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9_.-]*$` <br />Optional: \{\} <br /> |
| `component` _[DefinitionIdentifier](#definitionidentifier)_ | Component is the declared component. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br />Optional: \{\} <br /> |
| `absence` _[DefinitionPosture](#definitionposture)_ | Absence is the declared absence. |  | Enum: [Required Optional] <br />Optional: \{\} <br /> |
| `annotationKey` _string_ | AnnotationKey is the declared annotationKey. |  | MaxLength: 317 <br />MinLength: 1 <br /> |
| `timestampJSONField` _string_ | TimestampJSONField is the declared timestampJSONField. |  | MaxLength: 128 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `maxAgeThresholdKey` _[DefinitionThresholdKey](#definitionthresholdkey)_ | MaxAgeThresholdKey is the declared maxAgeThresholdKey. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9_.-]*$` <br />Optional: \{\} <br /> |
| `defaultMaxAge` _[DefinitionDuration](#definitionduration)_ | DefaultMaxAge is the declared defaultMaxAge. |  | MaxLength: 256 <br />MinLength: 1 <br /> |
| `staleOutcome` _[DefinitionOutcome](#definitionoutcome)_ | StaleOutcome is the declared staleOutcome. | Warn | Enum: [Pass Warn Fail Error Skipped] <br />Optional: \{\} <br /> |


#### DefinitionBindingScope



DefinitionBindingScope is a bounded runtime definition contract.



_Appears in:_
- [AddonDefinitionBindingSpec](#addondefinitionbindingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `namespaces` _[DefinitionDNSLabel](#definitiondnslabel) array_ | Namespaces is the declared namespaces. |  | MaxItems: 32 <br />MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `allowClusterScoped` _boolean_ | AllowClusterScoped is the declared allowClusterScoped. | false | Optional: \{\} <br /> |


#### DefinitionCRD



DefinitionCRD is a bounded runtime definition contract.



_Appears in:_
- [DefinitionCheck](#definitioncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `target` _[DefinitionTarget](#definitiontarget)_ | Target is the declared target. |  |  |
| `names` _[DefinitionResourceName](#definitionresourcename) array_ | Names is the declared names. |  | MaxItems: 32 <br />MaxLength: 253 <br />MinItems: 1 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` <br /> |
| `supportedVersions` _[DefinitionToken](#definitiontoken) array_ | SupportedVersions is the declared supportedVersions. |  | MaxItems: 8 <br />MaxLength: 253 <br />MinItems: 1 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9]*$` <br /> |
| `absence` _[DefinitionPosture](#definitionposture)_ | Absence is the declared absence. |  | Enum: [Required Optional] <br />Optional: \{\} <br /> |
| `unsupportedVersionOutcome` _[DefinitionOutcome](#definitionoutcome)_ | UnsupportedVersionOutcome is the declared unsupportedVersionOutcome. | Warn | Enum: [Pass Warn Fail Error Skipped] <br />Optional: \{\} <br /> |


#### DefinitionCheck



DefinitionCheck is a bounded runtime definition contract.



_Appears in:_
- [DefinitionFamily](#definitionfamily)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _[DefinitionIdentifier](#definitionidentifier)_ | Name is the declared name. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br /> |
| `kind` _string_ | Kind is the declared kind. |  | Enum: [Workload CRD Condition Field Webhook CronJob ConfigMap AnnotationStaleness PodProjection] <br /> |
| `workload` _[DefinitionWorkload](#definitionworkload)_ | Workload is the declared workload. |  | Optional: \{\} <br /> |
| `crd` _[DefinitionCRD](#definitioncrd)_ | CRD is the declared crd. |  | Optional: \{\} <br /> |
| `condition` _[DefinitionCondition](#definitioncondition)_ | Condition is the declared condition. |  | Optional: \{\} <br /> |
| `field` _[DefinitionField](#definitionfield)_ | Field is the declared field. |  | Optional: \{\} <br /> |
| `webhook` _[DefinitionWebhook](#definitionwebhook)_ | Webhook is the declared webhook. |  | Optional: \{\} <br /> |
| `cronJob` _[DefinitionCronJob](#definitioncronjob)_ | CronJob is the declared cronJob. |  | Optional: \{\} <br /> |
| `configMap` _[DefinitionConfigMap](#definitionconfigmap)_ | ConfigMap is the declared configMap. |  | Optional: \{\} <br /> |
| `annotationStaleness` _[DefinitionAnnotationStaleness](#definitionannotationstaleness)_ | AnnotationStaleness is the declared annotationStaleness. |  | Optional: \{\} <br /> |
| `podProjection` _[DefinitionPodProjection](#definitionpodprojection)_ | PodProjection is the declared podProjection. |  | Optional: \{\} <br /> |


#### DefinitionCondition



DefinitionCondition is a bounded runtime definition contract.



_Appears in:_
- [DefinitionCheck](#definitioncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `target` _[DefinitionTarget](#definitiontarget)_ | Target is the declared target. |  |  |
| `apiVersion` _string_ | APIVersion is the declared apiVersion. |  | MaxLength: 507 <br />MinLength: 1 <br />Pattern: `^([a-z0-9]([a-z0-9.-]*[a-z0-9])?/)?[a-z][a-z0-9]*$` <br /> |
| `kind` _[DefinitionToken](#definitiontoken)_ | Kind is the declared kind. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9]*$` <br /> |
| `listKind` _[DefinitionToken](#definitiontoken)_ | ListKind is the declared listKind. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9]*$` <br />Optional: \{\} <br /> |
| `listName` _[DefinitionIdentifier](#definitionidentifier)_ | ListName is the declared listName. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br />Optional: \{\} <br /> |
| `names` _[DefinitionResourceName](#definitionresourcename) array_ | Names is the declared names. |  | MaxItems: 32 <br />MaxLength: 253 <br />MinItems: 1 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `versionCRD` _[DefinitionResourceName](#definitionresourcename)_ | VersionCRD is the declared versionCRD. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `supportedVersions` _[DefinitionToken](#definitiontoken) array_ | SupportedVersions is the declared supportedVersions. |  | MaxItems: 8 <br />MaxLength: 253 <br />MinItems: 1 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9]*$` <br />Optional: \{\} <br /> |
| `absence` _[DefinitionPosture](#definitionposture)_ | Absence is the declared absence. |  | Enum: [Required Optional] <br />Optional: \{\} <br /> |
| `conditionType` _string_ | ConditionType is the declared conditionType. |  | MaxLength: 253 <br />MinLength: 1 <br /> |
| `expectedStatus` _string_ | ExpectedStatus is the declared expectedStatus. |  | Enum: [True False Unknown] <br /> |
| `absentCondition` _[DefinitionOutcome](#definitionoutcome)_ | AbsentCondition is the declared absentCondition. | Fail | Enum: [Pass Warn Fail Error Skipped] <br />Optional: \{\} <br /> |
| `mismatch` _[DefinitionOutcome](#definitionoutcome)_ | Mismatch is the declared mismatch. | Fail | Enum: [Pass Warn Fail Error Skipped] <br />Optional: \{\} <br /> |


#### DefinitionConfigMap



DefinitionConfigMap is a bounded runtime definition contract.



_Appears in:_
- [DefinitionCheck](#definitioncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `target` _[DefinitionTarget](#definitiontarget)_ | Target is the declared target. |  |  |
| `defaultName` _[DefinitionResourceName](#definitionresourcename)_ | DefaultName is the declared defaultName. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` <br /> |
| `nameThresholdKey` _[DefinitionThresholdKey](#definitionthresholdkey)_ | NameThresholdKey is the declared nameThresholdKey. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9_.-]*$` <br />Optional: \{\} <br /> |
| `component` _[DefinitionIdentifier](#definitionidentifier)_ | Component is the declared component. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br />Optional: \{\} <br /> |
| `absence` _[DefinitionPosture](#definitionposture)_ | Absence is the declared absence. |  | Enum: [Required Optional] <br />Optional: \{\} <br /> |
| `key` _string_ | Key is the declared key. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[-._a-zA-Z0-9]+$` <br /> |
| `recognizedAPIVersions` _string array_ | RecognizedAPIVersions is the declared recognizedAPIVersions. |  | MaxItems: 8 <br />items:MaxLength: 507 <br />Optional: \{\} <br /> |
| `unrecognizedOutcome` _[DefinitionOutcome](#definitionoutcome)_ | UnrecognizedOutcome is the declared unrecognizedOutcome. | Warn | Enum: [Pass Warn Fail Error Skipped] <br />Optional: \{\} <br /> |
| `invalidOutcome` _[DefinitionOutcome](#definitionoutcome)_ | InvalidOutcome is the declared invalidOutcome. | Fail | Enum: [Pass Warn Fail Error Skipped] <br />Optional: \{\} <br /> |


#### DefinitionCronJob



DefinitionCronJob is a bounded runtime definition contract.



_Appears in:_
- [DefinitionCheck](#definitioncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `target` _[DefinitionTarget](#definitiontarget)_ | Target is the declared target. |  |  |
| `defaultName` _[DefinitionResourceName](#definitionresourcename)_ | DefaultName is the declared defaultName. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` <br /> |
| `nameThresholdKey` _[DefinitionThresholdKey](#definitionthresholdkey)_ | NameThresholdKey is the declared nameThresholdKey. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9_.-]*$` <br />Optional: \{\} <br /> |
| `component` _[DefinitionIdentifier](#definitionidentifier)_ | Component is the declared component. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br />Optional: \{\} <br /> |
| `absence` _[DefinitionPosture](#definitionposture)_ | Absence is the declared absence. |  | Enum: [Required Optional] <br />Optional: \{\} <br /> |
| `successMaxAgeThresholdKey` _[DefinitionThresholdKey](#definitionthresholdkey)_ | SuccessMaxAgeThresholdKey is the declared successMaxAgeThresholdKey. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9_.-]*$` <br />Optional: \{\} <br /> |
| `defaultSuccessMaxAge` _[DefinitionDuration](#definitionduration)_ | DefaultSuccessMaxAge is the declared defaultSuccessMaxAge. | 0s | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `staleOutcome` _[DefinitionOutcome](#definitionoutcome)_ | StaleOutcome is the declared staleOutcome. | Warn | Enum: [Pass Warn Fail Error Skipped] <br />Optional: \{\} <br /> |


#### DefinitionDNSLabel

_Underlying type:_ _string_

DefinitionDNSLabel is bounded before semantic compilation.

_Validation:_
- MaxLength: 63
- MinLength: 1
- Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`

_Appears in:_
- [AddonDefinitionSpec](#addondefinitionspec)
- [DefinitionBindingScope](#definitionbindingscope)
- [DefinitionPodProjection](#definitionpodprojection)
- [DefinitionReference](#definitionreference)
- [DefinitionTarget](#definitiontarget)
- [DefinitionVersionSource](#definitionversionsource)
- [DefinitionWebhook](#definitionwebhook)



#### DefinitionDuration

_Underlying type:_ _string_

DefinitionDuration is bounded before semantic compilation.

_Validation:_
- MaxLength: 256
- MinLength: 1

_Appears in:_
- [DefinitionAnnotationStaleness](#definitionannotationstaleness)
- [DefinitionCronJob](#definitioncronjob)



#### DefinitionFamily



DefinitionFamily is a bounded runtime definition contract.



_Appears in:_
- [AddonDefinitionSpec](#addondefinitionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _[DefinitionIdentifier](#definitionidentifier)_ | Name is the declared name. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br /> |
| `defaultEnabled` _boolean_ | DefaultEnabled is the declared defaultEnabled. | false | Optional: \{\} <br /> |
| `checks` _[DefinitionCheck](#definitioncheck) array_ | Checks is the declared checks. |  | MaxItems: 32 <br />MinItems: 1 <br /> |


#### DefinitionField



DefinitionField is a bounded runtime definition contract.



_Appears in:_
- [DefinitionCheck](#definitioncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `target` _[DefinitionTarget](#definitiontarget)_ | Target is the declared target. |  |  |
| `apiVersion` _string_ | APIVersion is the declared apiVersion. |  | MaxLength: 507 <br />MinLength: 1 <br />Pattern: `^([a-z0-9]([a-z0-9.-]*[a-z0-9])?/)?[a-z][a-z0-9]*$` <br /> |
| `kind` _[DefinitionToken](#definitiontoken)_ | Kind is the declared kind. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9]*$` <br /> |
| `listKind` _[DefinitionToken](#definitiontoken)_ | ListKind is the declared listKind. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9]*$` <br /> |
| `listName` _[DefinitionIdentifier](#definitionidentifier)_ | ListName is the declared listName. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br />Optional: \{\} <br /> |
| `absence` _[DefinitionPosture](#definitionposture)_ | Absence is the declared absence. |  | Enum: [Required Optional] <br />Optional: \{\} <br /> |
| `fieldPath` _string array_ | FieldPath is the declared fieldPath. |  | MaxItems: 16 <br />MinItems: 1 <br />items:MaxLength: 128 <br />items:MinLength: 1 <br /> |
| `expectedValue` _[DefinitionText](#definitiontext)_ | ExpectedValue is the declared expectedValue. |  | MaxLength: 1024 <br />MinLength: 1 <br /> |
| `valueOutcomes` _object (keys:string, values:[DefinitionOutcome](#definitionoutcome))_ | ValueOutcomes is the declared valueOutcomes. |  | MaxProperties: 32 <br />Optional: \{\} <br /> |
| `absentOutcome` _[DefinitionOutcome](#definitionoutcome)_ | AbsentOutcome is the declared absentOutcome. | Warn | Enum: [Pass Warn Fail Error Skipped] <br />Optional: \{\} <br /> |
| `otherOutcome` _[DefinitionOutcome](#definitionoutcome)_ | OtherOutcome is the declared otherOutcome. | Warn | Enum: [Pass Warn Fail Error Skipped] <br />Optional: \{\} <br /> |


#### DefinitionIdentifier

_Underlying type:_ _string_

DefinitionIdentifier is bounded before semantic compilation.

_Validation:_
- MaxLength: 63
- MinLength: 1
- Pattern: `^[a-z][a-z0-9_-]*$`

_Appears in:_
- [DefinitionAnnotationStaleness](#definitionannotationstaleness)
- [DefinitionCheck](#definitioncheck)
- [DefinitionCondition](#definitioncondition)
- [DefinitionConfigMap](#definitionconfigmap)
- [DefinitionCronJob](#definitioncronjob)
- [DefinitionFamily](#definitionfamily)
- [DefinitionField](#definitionfield)
- [DefinitionPodProjection](#definitionpodprojection)
- [DefinitionVersionSource](#definitionversionsource)
- [DefinitionWorkload](#definitionworkload)



#### DefinitionLeaderEpoch



DefinitionLeaderEpoch is a bounded runtime definition contract.



_Appears in:_
- [AddonCheckEvidenceAuthority](#addoncheckevidenceauthority)
- [AddonDefinitionBindingStatus](#addondefinitionbindingstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `leaseUID` _string_ | LeaseUID is the declared leaseUID. |  | MaxLength: 128 <br />MinLength: 1 <br /> |
| `holderIdentity` _string_ | HolderIdentity is the declared holderIdentity. |  | MaxLength: 253 <br />MinLength: 1 <br /> |
| `acquireTime` _[MicroTime](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#microtime-v1-meta)_ | AcquireTime is the declared acquireTime. |  |  |
| `leaseTransitions` _integer_ | LeaseTransitions is the declared leaseTransitions. |  | Minimum: 0 <br /> |


#### DefinitionObjectReference



DefinitionObjectReference is a bounded runtime definition contract.



_Appears in:_
- [AddonDefinitionBindingSpec](#addondefinitionbindingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _[DefinitionResourceName](#definitionresourcename)_ | Name is the declared name. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` <br /> |
| `uid` _string_ | UID is the declared uid. |  | MaxLength: 128 <br />MinLength: 1 <br /> |


#### DefinitionOutcome

_Underlying type:_ _string_

DefinitionOutcome is an evaluator result; Error is never completed evidence.

_Validation:_
- Enum: [Pass Warn Fail Error Skipped]

_Appears in:_
- [DefinitionAnnotationStaleness](#definitionannotationstaleness)
- [DefinitionCRD](#definitioncrd)
- [DefinitionCondition](#definitioncondition)
- [DefinitionConfigMap](#definitionconfigmap)
- [DefinitionCronJob](#definitioncronjob)
- [DefinitionField](#definitionfield)
- [DefinitionPodProjection](#definitionpodprojection)



#### DefinitionPodProjection



DefinitionPodProjection is a bounded runtime definition contract.



_Appears in:_
- [DefinitionCheck](#definitioncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `target` _[DefinitionTarget](#definitiontarget)_ | Target is the declared target. |  |  |
| `selector` _object (keys:string, values:[DefinitionSelectorValue](#definitionselectorvalue))_ | Selector is the declared selector. |  | MaxProperties: 32 <br />MinProperties: 1 <br /> |
| `listName` _[DefinitionIdentifier](#definitionidentifier)_ | ListName is the declared listName. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br />Optional: \{\} <br /> |
| `component` _[DefinitionIdentifier](#definitionidentifier)_ | Component is the declared component. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br />Optional: \{\} <br /> |
| `volumeName` _[DefinitionDNSLabel](#definitiondnslabel)_ | VolumeName is the declared volumeName. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br /> |
| `envVar` _string_ | EnvVar is the declared envVar. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[A-Za-z_][A-Za-z0-9_]*$` <br />Optional: \{\} <br /> |
| `missingOutcome` _[DefinitionOutcome](#definitionoutcome)_ | MissingOutcome is the declared missingOutcome. | Fail | Enum: [Pass Warn Fail Error Skipped] <br />Optional: \{\} <br /> |


#### DefinitionPosture

_Underlying type:_ _string_

DefinitionPosture specifies how absent targets are scored.

_Validation:_
- Enum: [Required Optional]

_Appears in:_
- [DefinitionAnnotationStaleness](#definitionannotationstaleness)
- [DefinitionCRD](#definitioncrd)
- [DefinitionCondition](#definitioncondition)
- [DefinitionConfigMap](#definitionconfigmap)
- [DefinitionCronJob](#definitioncronjob)
- [DefinitionField](#definitionfield)
- [DefinitionWebhook](#definitionwebhook)
- [DefinitionWorkload](#definitionworkload)



#### DefinitionReadRule



DefinitionReadRule is a bounded runtime definition contract.



_Appears in:_
- [AddonDefinitionSpec](#addondefinitionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiGroup` _string_ | APIGroup is the declared apiGroup. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `resources` _string array_ | Resources is the declared resources. |  | MaxItems: 32 <br />MinItems: 1 <br />items:MaxLength: 253 <br />Optional: \{\} <br /> |
| `resourceNames` _[DefinitionResourceName](#definitionresourcename) array_ | ResourceNames is the declared resourceNames. |  | MaxItems: 32 <br />MaxLength: 253 <br />MinItems: 1 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `nonResourceURLs` _string array_ | NonResourceURLs is the declared nonResourceURLs. |  | MaxItems: 32 <br />MinItems: 1 <br />items:MaxLength: 1024 <br />Optional: \{\} <br /> |
| `verbs` _string array_ | Verbs is the declared verbs. |  | MaxItems: 2 <br />MinItems: 1 <br />items:Enum: [get list] <br /> |


#### DefinitionReference



DefinitionReference identifies one canonical AddonDefinition revision owner.



_Appears in:_
- [AddonDefinitionBindingSpec](#addondefinitionbindingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _[DefinitionDNSLabel](#definitiondnslabel)_ | Name is the definition's canonical addon identity. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br /> |
| `uid` _string_ | UID prevents authority from surviving definition recreation. |  | MaxLength: 128 <br />MinLength: 1 <br /> |


#### DefinitionResourceName

_Underlying type:_ _string_

DefinitionResourceName is bounded before semantic compilation.

_Validation:_
- MaxLength: 253
- MinLength: 1
- Pattern: `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`

_Appears in:_
- [DefinitionAnnotationStaleness](#definitionannotationstaleness)
- [DefinitionCRD](#definitioncrd)
- [DefinitionCondition](#definitioncondition)
- [DefinitionConfigMap](#definitionconfigmap)
- [DefinitionCronJob](#definitioncronjob)
- [DefinitionObjectReference](#definitionobjectreference)
- [DefinitionReadRule](#definitionreadrule)
- [DefinitionWebhook](#definitionwebhook)
- [DefinitionWorkload](#definitionworkload)



#### DefinitionSelectorValue

_Underlying type:_ _string_

DefinitionSelectorValue is a bounded Kubernetes label value.

_Validation:_
- MaxLength: 256

_Appears in:_
- [DefinitionPodProjection](#definitionpodprojection)



#### DefinitionStatusCondition



DefinitionStatusCondition is a bounded runtime definition contract.



_Appears in:_
- [AddonDefinitionBindingStatus](#addondefinitionbindingstatus)
- [AddonDefinitionStatus](#addondefinitionstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Type is the declared type. |  | MaxLength: 64 <br />MinLength: 1 <br /> |
| `status` _[ConditionStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#conditionstatus-v1-meta)_ | Status is the declared status. |  | Enum: [True False Unknown] <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the declared observedGeneration. |  | Minimum: 0 <br /> |
| `lastTransitionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | LastTransitionTime is the declared lastTransitionTime. |  |  |
| `reason` _string_ | Reason is the declared reason. |  | MaxLength: 128 <br />MinLength: 1 <br /> |
| `message` _string_ | Message is the declared message. |  | MaxLength: 1024 <br /> |


#### DefinitionTarget



DefinitionTarget is a bounded runtime definition contract.



_Appears in:_
- [DefinitionAnnotationStaleness](#definitionannotationstaleness)
- [DefinitionCRD](#definitioncrd)
- [DefinitionCondition](#definitioncondition)
- [DefinitionConfigMap](#definitionconfigmap)
- [DefinitionCronJob](#definitioncronjob)
- [DefinitionField](#definitionfield)
- [DefinitionPodProjection](#definitionpodprojection)
- [DefinitionWebhook](#definitionwebhook)
- [DefinitionWorkload](#definitionworkload)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `scope` _string_ | Scope is the declared scope. |  | Enum: [Namespaced Cluster] <br /> |
| `namespaces` _[DefinitionDNSLabel](#definitiondnslabel) array_ | Namespaces is the declared namespaces. |  | MaxItems: 32 <br />MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |


#### DefinitionText

_Underlying type:_ _string_

DefinitionText is bounded before semantic compilation.

_Validation:_
- MaxLength: 1024
- MinLength: 1

_Appears in:_
- [DefinitionField](#definitionfield)



#### DefinitionThresholdKey

_Underlying type:_ _string_

DefinitionThresholdKey is bounded before semantic compilation.

_Validation:_
- MaxLength: 63
- MinLength: 1
- Pattern: `^[A-Za-z][A-Za-z0-9_.-]*$`

_Appears in:_
- [DefinitionAnnotationStaleness](#definitionannotationstaleness)
- [DefinitionConfigMap](#definitionconfigmap)
- [DefinitionCronJob](#definitioncronjob)
- [DefinitionWebhook](#definitionwebhook)
- [DefinitionWorkload](#definitionworkload)



#### DefinitionToken

_Underlying type:_ _string_

DefinitionToken is bounded before semantic compilation.

_Validation:_
- MaxLength: 253
- MinLength: 1
- Pattern: `^[A-Za-z][A-Za-z0-9]*$`

_Appears in:_
- [DefinitionAnnotationStaleness](#definitionannotationstaleness)
- [DefinitionCRD](#definitioncrd)
- [DefinitionCondition](#definitioncondition)
- [DefinitionField](#definitionfield)



#### DefinitionVersionSource



DefinitionVersionSource is a bounded runtime definition contract.



_Appears in:_
- [AddonDefinitionSpec](#addondefinitionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `fromFamily` _[DefinitionIdentifier](#definitionidentifier)_ | FromFamily is the declared fromFamily. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br /> |
| `fromComponent` _[DefinitionIdentifier](#definitionidentifier)_ | FromComponent is the declared fromComponent. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br /> |
| `container` _[DefinitionDNSLabel](#definitiondnslabel)_ | Container is the declared container. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |


#### DefinitionWebhook



DefinitionWebhook is a bounded runtime definition contract.



_Appears in:_
- [DefinitionCheck](#definitioncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `target` _[DefinitionTarget](#definitiontarget)_ | Target is the declared target. |  |  |
| `kind` _string_ | Kind is the declared kind. |  | Enum: [MutatingWebhookConfiguration ValidatingWebhookConfiguration] <br /> |
| `name` _[DefinitionResourceName](#definitionresourcename)_ | Name is the declared name. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` <br /> |
| `nameThresholdKey` _[DefinitionThresholdKey](#definitionthresholdkey)_ | NameThresholdKey is the declared nameThresholdKey. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9_.-]*$` <br />Optional: \{\} <br /> |
| `expectedService` _[DefinitionDNSLabel](#definitiondnslabel)_ | ExpectedService is the declared expectedService. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `serviceNamespace` _[DefinitionDNSLabel](#definitiondnslabel)_ | ServiceNamespace is the declared serviceNamespace. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `absence` _[DefinitionPosture](#definitionposture)_ | Absence is the declared absence. |  | Enum: [Required Optional] <br />Optional: \{\} <br /> |
| `verifyEndpoints` _boolean_ | VerifyEndpoints is the declared verifyEndpoints. | false | Optional: \{\} <br /> |


#### DefinitionWorkload



DefinitionWorkload is a bounded runtime definition contract.



_Appears in:_
- [DefinitionCheck](#definitioncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `target` _[DefinitionTarget](#definitiontarget)_ | Target is the declared target. |  |  |
| `kind` _string_ | Kind is the declared kind. |  | Enum: [Deployment DaemonSet StatefulSet] <br /> |
| `defaultName` _[DefinitionResourceName](#definitionresourcename)_ | DefaultName is the declared defaultName. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` <br /> |
| `nameThresholdKey` _[DefinitionThresholdKey](#definitionthresholdkey)_ | NameThresholdKey is the declared nameThresholdKey. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9_.-]*$` <br />Optional: \{\} <br /> |
| `component` _[DefinitionIdentifier](#definitionidentifier)_ | Component is the declared component. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z][a-z0-9_-]*$` <br />Optional: \{\} <br /> |
| `absence` _[DefinitionPosture](#definitionposture)_ | Absence is the declared absence. |  | Enum: [Required Optional] <br />Optional: \{\} <br /> |
| `checkPods` _boolean_ | CheckPods is the declared checkPods. | false | Optional: \{\} <br /> |
| `restartWarnThresholdKey` _[DefinitionThresholdKey](#definitionthresholdkey)_ | RestartWarnThresholdKey is the declared restartWarnThresholdKey. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[A-Za-z][A-Za-z0-9_.-]*$` <br />Optional: \{\} <br /> |
| `defaultRestartWarn` _integer_ | DefaultRestartWarn is the declared defaultRestartWarn. | 0 | Minimum: 0 <br />Optional: \{\} <br /> |


#### HealthCheck



HealthCheck is the Schema for the healthchecks API.



_Appears in:_
- [HealthCheckList](#healthchecklist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthCheck` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[HealthCheckSpec](#healthcheckspec)_ |  |  |  |
| `status` _[HealthCheckStatus](#healthcheckstatus)_ |  |  |  |


#### HealthCheckList



HealthCheckList contains a list of HealthCheck.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthCheckList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[HealthCheck](#healthcheck) array_ |  |  |  |


#### HealthCheckSpec



HealthCheckSpec defines the desired state of HealthCheck. A HealthCheck is
a thin wrapper that mirrors the status of a specialized check resource into
a uniform shape suitable for ClusterHealth aggregation. HealthCheck does not
execute checks itself.



_Appears in:_
- [HealthCheck](#healthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `checkRef` _[CheckTargetRef](#checktargetref)_ | CheckRef identifies the specialized check resource this HealthCheck wraps.<br />It is immutable: retargeting a wrapper would silently repoint its mirrored<br />status snapshot at a different check; replace the HealthCheck instead (SKA-576). |  |  |
| `description` _string_ | Description is a human-readable purpose for this HealthCheck. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |
| `paused` _boolean_ | Paused suspends mirroring of the referenced check's status into this<br />HealthCheck. The most recent Status snapshot is preserved while paused. |  | Optional: \{\} <br /> |


#### HealthCheckStatus



HealthCheckStatus defines the observed state of HealthCheck. The fields are
derived from the referenced check's status; consumers (notably
ClusterHealth) read this status without needing to understand any
specialized check schema.



_Appears in:_
- [HealthCheck](#healthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent metadata.generation reconciled. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | Conditions summarize the controller's view of the wrapped check. |  | Optional: \{\} <br /> |
| `result` _[HealthReportResult](#healthreportresult)_ | Result is the outcome surfaced from the referenced check's most recent run. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `summary` _string_ | Summary is a human-readable one-line outcome description. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |
| `sourceInterval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#duration-v1-meta)_ | SourceInterval is the cadence the referenced check is expected to run at,<br />after its own defaults and floor clamping. It is a fact about the wrapped<br />check, not a judgement about this one: it is what lets a ClusterHealth<br />aggregate judge staleness relative to cadence, since aggregates select<br />HealthChecks and never see the underlying checks (#277).<br />Empty when the cadence cannot be resolved — an unsupported checkRef kind,<br />a missing target, or a lookup failure — so consumers can tell "runs hourly"<br />from "cadence unknown" rather than reading an absent value as zero. |  | Optional: \{\} <br /> |
| `sourceObservedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | SourceObservedAt is when the referenced check last completed. |  | Optional: \{\} <br /> |
| `lastReportName` _string_ | LastReportName names the most recent HealthReport produced by the<br />referenced check, when one exists. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `sourceReady` _boolean_ | SourceReady mirrors whether the referenced check's most recent run could<br />EXECUTE and COMPLETE with eligible inputs. It is not a verdict:<br />contracts/runtime.md, "Ready denotes executable/completed, freshness<br />denotes recency, and neither means Pass". A false SourceReady beside a<br />Pass Result is the ordinary shape of preserved evidence — the last<br />completed run passed, and the most recent attempt could not run at all.<br />Nil when the referenced check has never published a readiness condition,<br />which is a different statement from "the check is not ready". |  | Optional: \{\} <br /> |
| `sourceReadyReason` _string_ | SourceReadyReason is the referenced check's own reason for that<br />readiness — UnknownAddonType, AuthorizationRevoked, AccessDenied,<br />RunCompleted and so on — so an operator can tell why a mirrored verdict<br />is not being refreshed without reading the wrapped check. |  | MaxLength: 128 <br />Optional: \{\} <br /> |
| `evidenceFreshness` _[AddonCheckEvidenceFreshness](#addoncheckevidencefreshness)_ | EvidenceFreshness mirrors the recency and eligibility of the completed<br />evidence behind Result, re-derived from the evidence's age at mirror<br />time. Empty for checks that publish no evidence (every built-in adapter),<br />which is "not applicable" rather than "unavailable".<br />This is the field that keeps a retained Pass from reading as a fresh<br />success: "Freshness=Stale even if stored verdict was Pass." |  | Enum: [Current Stale Superseded Unavailable] <br />Optional: \{\} <br /> |
| `evidenceFreshnessReason` _string_ | EvidenceFreshnessReason explains a freshness that is not Current. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |


#### HealthReport



HealthReport is the Schema for the healthreports API.



_Appears in:_
- [HealthReportList](#healthreportlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthReport` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[HealthReportSpec](#healthreportspec)_ | Spec is immutable: a HealthReport is a point-in-time history record.<br />The operator only ever creates reports (createOrReuseHealthReport);<br />mutating one after the fact would rewrite history (SKA-576). |  |  |
| `status` _[HealthReportStatus](#healthreportstatus)_ |  |  |  |


#### HealthReportAttribution



HealthReportAttribution records which runtime revision, under which
delegated authority, produced a report, and how much that run actually
covered (T045 of specs/012-addon-definition-runtime).

A HealthReport is history, and history is only useful while it stays
attributable: contracts/runtime.md requires that when a binding or grant
recovers, "history remains attributable", and the lifecycle matrix keeps
superseded evidence "with its original time/revision/context". Without this
block a stored report is just a verdict and a timestamp, so a Pass produced
under authority that has since been revoked, or under a definition revision
that has since been replaced, reads exactly like one produced under the
current one.

It reuses the AddonCheck evidence vocabulary deliberately rather than
restating it: the report is created FROM published evidence, and two
independent spellings of the same context would be free to drift apart —
which is precisely the divergence this field exists to make visible.

It is optional and absent on reports from built-in adapters, which carry no
runtime revision, no delegated binding and no dedicated identity to name.



_Appears in:_
- [HealthReportSpec](#healthreportspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `revision` _[AddonCheckEvidenceRevision](#addoncheckevidencerevision)_ | Revision is the runtime definition revision that produced the report:<br />the definition incarnation plus the operator build and adapter version<br />that compiled it. |  | Optional: \{\} <br /> |
| `authority` _[AddonCheckEvidenceAuthority](#addoncheckevidenceauthority)_ | Authority is the delegated binding, dedicated identity, check context and<br />leadership epoch the producing run was attributed to. |  | Optional: \{\} <br /> |
| `coverage` _[AddonCheckEvidenceCoverage](#addoncheckevidencecoverage)_ | Coverage distinguishes an assessed verdict from a completed run that<br />evaluated nothing, so a Skipped entry in history cannot be mistaken for<br />an assessed-and-healthy one. |  | Enum: [ChecksEvaluated NoChecksEvaluated] <br />Optional: \{\} <br /> |
| `message` _string_ | Message explains the coverage in one line. A report produced by a<br />completed all-Skipped run carries exactly<br />[AddonCheckNoChecksEvaluatedMessage]. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |


#### HealthReportCheck



HealthReportCheck records one adapter-emitted check result.



_Appears in:_
- [HealthReportSpec](#healthreportspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `family` _string_ | Family is the adapter-defined check family that produced this result. |  | MaxLength: 63 <br />MinLength: 1 <br /> |
| `result` _[HealthReportResult](#healthreportresult)_ | Result is this check's outcome. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br /> |
| `targetRef` _[HealthReportTargetRef](#healthreporttargetref)_ | TargetRef is the observed resource for this check. |  |  |
| `summary` _string_ | Summary is a human-readable one-line outcome description. |  | Optional: \{\} <br /> |
| `details` _object (keys:string, values:string)_ | Details is adapter-defined structured context for the check. |  | Optional: \{\} <br /> |
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | ObservedAt is when this check completed. |  |  |
| `duration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#duration-v1-meta)_ | Duration is how long this check took. |  | Optional: \{\} <br /> |


#### HealthReportList



HealthReportList contains a list of HealthReport.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthReportList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[HealthReport](#healthreport) array_ |  |  |  |


#### HealthReportResult

_Underlying type:_ _string_

HealthReportResult is the aggregate result for a report or individual check.

_Validation:_
- Enum: [Pass Warn Fail Error Skipped Unknown]

_Appears in:_
- [ClusterHealthChildSummary](#clusterhealthchildsummary)
- [ClusterHealthStatus](#clusterhealthstatus)
- [HealthCheckStatus](#healthcheckstatus)
- [HealthReportCheck](#healthreportcheck)
- [HealthReportSpec](#healthreportspec)

| Field | Description |
| --- | --- |
| `Pass` |  |
| `Warn` |  |
| `Fail` |  |
| `Error` |  |
| `Skipped` |  |
| `Unknown` |  |


#### HealthReportSpec



HealthReportSpec defines the desired state of HealthReport.



_Appears in:_
- [HealthReport](#healthreport)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sourceRef` _[HealthReportTargetRef](#healthreporttargetref)_ | SourceRef identifies the check resource that produced this report. |  |  |
| `addonType` _string_ | AddonType is the AddonCheck addon type used to select the adapter. |  | Optional: \{\} <br /> |
| `adapterName` _string_ | AdapterName is the adapter identity that produced this report. |  | Optional: \{\} <br /> |
| `adapterVersion` _string_ | AdapterVersion is the adapter implementation version. |  | Optional: \{\} <br /> |
| `detectedVersion` _string_ | DetectedVersion is the installed addon release version detected for this<br />run, or empty when undetectable or not detected. Distinct from<br />AdapterVersion (the adapter's own version) — SKA-527. |  | Optional: \{\} <br /> |
| `contractVersion` _string_ | ContractVersion is the adapter contract version used for this run. |  | Optional: \{\} <br /> |
| `result` _[HealthReportResult](#healthreportresult)_ | Result is the aggregate outcome across all checks. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br /> |
| `checks` _[HealthReportCheck](#healthreportcheck) array_ | Checks are the individual observations produced by the adapter. |  | Optional: \{\} <br /> |
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | ObservedAt is when the adapter run completed. |  |  |
| `duration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#duration-v1-meta)_ | Duration is the total adapter run duration. |  | Optional: \{\} <br /> |
| `attribution` _[HealthReportAttribution](#healthreportattribution)_ | Attribution names the runtime revision and delegated authority this<br />report was produced under. Absent for built-in adapters, which have<br />neither. |  | Optional: \{\} <br /> |


#### HealthReportStatus



HealthReportStatus defines the observed state of HealthReport.



_Appears in:_
- [HealthReport](#healthreport)



#### HealthReportTargetRef



HealthReportTargetRef identifies a Kubernetes object observed by a check.



_Appears in:_
- [HealthReportCheck](#healthreportcheck)
- [HealthReportSpec](#healthreportspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | APIVersion is the target object's API version. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `kind` _string_ | Kind is the target object's kind. |  | MaxLength: 63 <br />Optional: \{\} <br /> |
| `namespace` _string_ | Namespace is the target object's namespace, if namespaced. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `name` _string_ | Name is the target object's name. |  | MaxLength: 253 <br />MinLength: 1 <br /> |


#### NodeCertificateCheck



NodeCertificateCheck is the Schema for the nodecertificatechecks API.



_Appears in:_
- [NodeCertificateCheckList](#nodecertificatechecklist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `NodeCertificateCheck` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[NodeCertificateCheckSpec](#nodecertificatecheckspec)_ |  |  |  |
| `status` _[NodeCertificateCheckStatus](#nodecertificatecheckstatus)_ |  |  |  |


#### NodeCertificateCheckList



NodeCertificateCheckList contains a list of NodeCertificateCheck.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `NodeCertificateCheckList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[NodeCertificateCheck](#nodecertificatecheck) array_ |  |  |  |


#### NodeCertificateCheckSpec



NodeCertificateCheckSpec defines the desired state of NodeCertificateCheck.

A NodeCertificateCheck scans on-disk X.509 certificates on every selected
node and reports time-to-expiry before an expiring certificate can take the
cluster down. The operator runs the scan via a hardened, read-only node-agent
DaemonSet (one pod per node); each agent publishes its per-node result, and
the operator rolls those up into a single HealthReport and mirrors the
aggregate into Status.



_Appears in:_
- [NodeCertificateCheck](#nodecertificatecheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `paths` _string array_ | Paths is the set of on-disk certificate locations each node-agent scans.<br />Every entry is an absolute path to either a PEM-encoded certificate file<br />(read directly) or a directory (scanned recursively, to a bounded depth,<br />for *.crt, *.pem, and *.cert files). Files ending in .conf or .kubeconfig<br />are parsed as kubeconfigs and their embedded client/CA certificates are<br />extracted. Paths the non-root agent cannot read are reported as Skipped,<br />never Fail or Error; paths that do not exist on a node are omitted from the<br />report entirely, so absent distribution defaults do not flood it. When empty, a<br />distribution-agnostic default set covering common kubeadm, k3s/RKE2, etcd,<br />and kubelet certificate locations is used. The operator mounts the parent<br />directory of each configured path into the agent read-only; a configured<br />directory absent on a node is created empty by the kubelet (hostPath<br />DirectoryOrCreate), so prefer narrowing Paths on immutable-OS distributions.<br />To prevent a namespaced tenant from turning the privileged node-agent into a<br />confused deputy that mounts arbitrary host directories, every entry must be a<br />traversal-free absolute path (no "..", never the host root "/") under one of<br />the operator-approved certificate prefixes: /etc/kubernetes, /var/lib/kubelet,<br />/etc/etcd, /var/lib/etcd, /var/lib/rancher. Paths outside this allowlist are<br />rejected at admission. |  | MaxItems: 64 <br />items:MaxLength: 512 <br />Optional: \{\} <br /> |
| `warnDays` _integer_ | WarnDays is the days-to-expiry threshold at or below which a certificate<br />is reported as Warn. Must be greater than or equal to CriticalDays. | 30 | Minimum: 0 <br />Optional: \{\} <br /> |
| `criticalDays` _integer_ | CriticalDays is the days-to-expiry threshold at or below which a<br />certificate is reported as Fail. A certificate already past its notAfter<br />time is always Fail regardless of this value. | 7 | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeSelector` _object (keys:string, values:string)_ | NodeSelector restricts which nodes run the agent DaemonSet. An empty<br />selector targets every node. |  | Optional: \{\} <br /> |
| `tolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#toleration-v1-core) array_ | Tolerations are applied verbatim to the agent DaemonSet so it can schedule<br />onto nodes carrying arbitrary taints. It is empty by default. Control-plane<br />tolerations are NOT added here — use IncludeControlPlaneNodes for that, so<br />scheduling the privileged agent onto control-plane nodes is always an<br />explicit, auditable opt-in rather than a silent default. |  | Optional: \{\} <br /> |
| `includeControlPlaneNodes` _boolean_ | IncludeControlPlaneNodes opts the node-agent into scheduling on control-plane<br />nodes by adding tolerations for the standard control-plane and legacy master<br />taints (node-role.kubernetes.io/control-plane and .../master, Exists /<br />NoSchedule) on top of any Tolerations.<br />It defaults to false. The kubeadm apiserver, etcd, and front-proxy<br />certificates live on control-plane nodes, so set this to true to scan them —<br />but doing so mounts control-plane host paths into the agent, which is why it<br />is gated behind an explicit opt-in rather than applied by default. | false | Optional: \{\} <br /> |
| `interval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#duration-v1-meta)_ | Interval is the cadence at which each node-agent re-scans its<br />certificates and the operator refreshes the rolled-up HealthReport.<br />Defaults to 1h when unset. Must be at least 10s (MinCheckInterval); the<br />operator clamps stored objects that predate this floor to it at runtime. |  | Optional: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#duration-v1-meta)_ | Timeout bounds a single node-agent scan pass. Defaults to 30s when<br />unset. Must be at least 1s (MinCheckTimeout); the operator clamps stored<br />objects that predate this floor to it at runtime. |  | Optional: \{\} <br /> |
| `paused` _boolean_ | Paused stops the operator from running the agent DaemonSet and refreshing<br />reports. The agent DaemonSet is removed while paused; the most recent<br />Status snapshot is preserved. |  | Optional: \{\} <br /> |
| `historyLimit` _integer_ | HistoryLimit caps the number of HealthReports retained for this check.<br />After each new HealthReport the controller deletes the oldest reports<br />beyond the limit. The minimum of 1 keeps Status.LastReportName valid. | 10 | Minimum: 1 <br />Optional: \{\} <br /> |


#### NodeCertificateCheckStatus



NodeCertificateCheckStatus defines the observed state of NodeCertificateCheck.



_Appears in:_
- [NodeCertificateCheck](#nodecertificatecheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent metadata.generation reconciled by<br />the controller. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | Conditions summarize whether the controller accepted the spec and whether<br />the agent DaemonSet is rolled out and reporting. |  | Optional: \{\} <br /> |
| `lastRunTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | LastRunTime records when the operator last evaluated the node-agent<br />results (the latest poll). It is refreshed on the interval cadence even<br />when the aggregate result is unchanged, so downstream liveness stays fresh,<br />and does not imply a new HealthReport was written on every refresh. |  | Optional: \{\} <br /> |
| `lastResult` _string_ | LastResult is the aggregate result across all reporting nodes as of the<br />most recent evaluation. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `lastReportName` _string_ | LastReportName names the HealthReport capturing the current aggregate<br />result. A new HealthReport is written only when that result transitions, so<br />this name is stable across polls that observe the same result. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `desiredNodes` _integer_ | DesiredNodes is the number of nodes the agent DaemonSet targets<br />(DaemonSet status DesiredNumberScheduled). |  | Optional: \{\} <br /> |
| `reportingNodes` _integer_ | ReportingNodes is the number of nodes that have published a scan result<br />the operator consumed in the most recent roll-up. |  | Optional: \{\} <br /> |
| `lastRunTrigger` _string_ | LastRunTrigger records the fathom.skaphos.io/run-now annotation value<br />most recently consumed. A new value is carried to the node-agents through<br />their DaemonSet template, which restarts them; each agent stamps the value<br />into its report, and the operator records it here only once every desired<br />node's fresh report carries it. Until then the previous verdict is kept.<br />A given on-demand trigger therefore completes exactly once. |  | MaxLength: 253 <br />Optional: \{\} <br /> |


#### NodeHealthCheck



NodeHealthCheck is the Schema for the nodehealthchecks API.



_Appears in:_
- [NodeHealthCheckList](#nodehealthchecklist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `NodeHealthCheck` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[NodeHealthCheckSpec](#nodehealthcheckspec)_ |  |  |  |
| `status` _[NodeHealthCheckStatus](#nodehealthcheckstatus)_ |  |  |  |


#### NodeHealthCheckItem



NodeHealthCheckItem is one assertion made on every node in scope. Which
fields are legal depends on Type; the rules below reject a field on a type
that does not use it, so a misapplied threshold is a write-time error and
not a silently ignored one.
The relation is checked on the effective values: an omitted threshold
counts as its runtime default, so criticalPercentFree: 30 with warn omitted
(effective warn 20) and warnPercentFree: 0 with critical omitted (effective
critical 10) are rejected here rather than silently clamped at run time.
The path allowlist stops a namespaced tenant from turning the node-agent into
a confused deputy that mounts arbitrary host directories; it is mirrored in
internal/nodehealth so the operator re-checks it on clusters running an older
CRD. The host root is never allowed: statfs of /var/lib/kubelet reports the
root filesystem on any node where it is not a separate mount.



_Appears in:_
- [NodeHealthCheckSpec](#nodehealthcheckspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[NodeHealthCheckType](#nodehealthchecktype)_ | Type is the assertion this item makes. See NodeHealthCheckType for the<br />privilege each type costs the node-agent. |  | Enum: [DiskHeadroom InodeHeadroom NodeCondition KubeletHealthz ContainerRuntime] <br /> |
| `path` _string_ | Path is an existing directory on every selected node whose headroom is<br />measured. Regular files are not supported. Required for DiskHeadroom and<br />InodeHeadroom and rejected for every other type. The operator mounts it<br />into the agent read-only, so it must be a traversal-free absolute path<br />under one of the operator-approved prefixes. Admission validates the path's<br />syntax and prefix but cannot inspect a node's host filesystem.<br />The operator uses a non-creating hostPath Directory mount. If Path does<br />not exist or is not a directory on a node, Kubernetes cannot start<br />that node's agent pod;<br />AgentReady becomes False and coverage remains incomplete while the last<br />complete verdict is retained. This avoids mutating the host or measuring<br />the filesystem that would have held a newly created directory. |  | MaxLength: 512 <br />Optional: \{\} <br /> |
| `warnPercentFree` _integer_ | WarnPercentFree is the percentage of free space (or inodes) at or below<br />which the check is Warn. Applies only to the headroom types. Defaults to<br />20 (DefaultNodeHealthWarnPercentFree) when unset. Must be greater than or<br />equal to CriticalPercentFree. |  | Maximum: 100 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `criticalPercentFree` _integer_ | CriticalPercentFree is the percentage of free space (or inodes) at or<br />below which the check is Fail. Applies only to the headroom types.<br />Defaults to 10 (DefaultNodeHealthCriticalPercentFree) when unset. |  | Maximum: 100 <br />Minimum: 0 <br />Optional: \{\} <br /> |
| `conditions` _string array_ | Conditions names the node condition types a NodeCondition check asserts.<br />Ready is expected True; every other listed condition is expected False,<br />which is the healthy value for every pressure and unavailability<br />condition Kubernetes and the cloud providers define. Defaults to Ready,<br />MemoryPressure, DiskPressure, and PIDPressure when unset. A condition the<br />node does not report is Skipped, not Fail. Applies only to NodeCondition. |  | MaxItems: 16 <br />items:MaxLength: 63 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `socketPath` _string_ | SocketPath is the CRI socket a ContainerRuntime check dials. Defaults to<br />/run/containerd/containerd.sock when unset. The operator mounts the socket<br />itself (hostPath type Socket), so a node without a socket at this path<br />cannot schedule the agent and surfaces as incomplete coverage rather than<br />as a verdict. Applies only to ContainerRuntime. |  | MaxLength: 512 <br />Optional: \{\} <br /> |


#### NodeHealthCheckList



NodeHealthCheckList contains a list of NodeHealthCheck.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `NodeHealthCheckList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[NodeHealthCheck](#nodehealthcheck) array_ |  |  |  |


#### NodeHealthCheckSpec



NodeHealthCheckSpec defines the desired state of NodeHealthCheck.

A NodeHealthCheck asserts a set of node-local health signals on every
selected node — filesystem headroom, the node's own conditions, kubelet
health, and container-runtime liveness — and reports one verdict per node
plus a single folded verdict for the check. The node-local signals are
collected by the same hardened node-agent DaemonSet NodeCertificateCheck
uses; the operator rolls the per-node reports into a HealthReport and
mirrors the aggregate into Status.

There is no field to pause a NodeHealthCheck. Stopping a check means
deleting it (#262).
The comparison uses the effective interval: an omitted interval is 5m at
runtime, so timeout: 10m without an interval is rejected here rather than
admitted and silently capped. Keep the literal in step with
DefaultNodeHealthCheckInterval.
Per-node results and HealthReport checks are keyed by the item's identity —
its type plus what it measures: the path for the headroom types, the socket
for ContainerRuntime (its default counts as a value), nothing for the
rest — so two items with the same identity would collide there. Rejecting
the duplicate at write time is far kinder than a status update failing
later with an error that says nothing about the specification that caused
it. Two ContainerRuntime items with different sockets are distinct. The
pathless types key on their own type name rather than an empty string:
gofmt rewrites two adjacent apostrophes inside a doc comment as a
typographic quote, which would silently break the rule at CRD install.
A headroom path that is also a ContainerRuntime socket would be mounted
twice at one mountPath — as a Directory and as a Socket —
which the kubelet rejects, and a directory mount over a socket is
meaningless anyway. The socket's default counts as a value.



_Appears in:_
- [NodeHealthCheck](#nodehealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `checks` _[NodeHealthCheckItem](#nodehealthcheckitem) array_ | Checks are the assertions made on every node in scope. At least one is<br />required — a check with no items would report a vacuous pass. |  | MaxItems: 16 <br />MinItems: 1 <br /> |
| `nodeSelector` _object (keys:string, values:string)_ | NodeSelector restricts which nodes run the agent DaemonSet. An empty<br />selector targets every node. |  | Optional: \{\} <br /> |
| `tolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#toleration-v1-core) array_ | Tolerations are applied verbatim to the agent DaemonSet so it can schedule<br />onto nodes carrying arbitrary taints. Control-plane tolerations are NOT<br />added here — use IncludeControlPlaneNodes for that, so scheduling the<br />agent onto control-plane nodes is always an explicit, auditable opt-in.<br />An omitted list and an empty list mean the same thing: no tolerations.<br />The operator never distinguishes the two, so the nil-vs-empty round-trip<br />the sibling kind carries (#150) has no effect here. |  | Optional: \{\} <br /> |
| `includeControlPlaneNodes` _boolean_ | IncludeControlPlaneNodes opts the node-agent into scheduling on<br />control-plane nodes by adding tolerations for the standard control-plane<br />and legacy master taints on top of any Tolerations. It defaults to false:<br />a KubeletHealthz or ContainerRuntime check runs the agent with host<br />network or as root, so landing it on a control-plane node is a decision<br />the author makes, not a default. | false | Optional: \{\} <br /> |
| `metricsHostPort` _integer_ | MetricsHostPort is the legacy-named host port where a host-network agent<br />(one running a KubeletHealthz item) serves liveness. When unset, the operator<br />derives a port in 20000–22767 from the check's namespaced name; that<br />derivation is a hash, so two host-network checks scheduled on the same<br />node can collide, which surfaces as the second agent crash-looping<br />(AgentReady=False) and is reported on the AgentPrivileged condition. Set<br />this to resolve such a collision, or to avoid an unrelated host listener.<br />Ignored for a check without a host-network item. |  | Maximum: 65535 <br />Minimum: 1024 <br />Optional: \{\} <br /> |
| `interval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#duration-v1-meta)_ | Interval is the cadence at which the operator refreshes the rolled-up<br />HealthReport (the roll-up transition cadence). Defaults to 5m when<br />unset. Must be at least 10s (MinCheckInterval); the operator clamps<br />stored objects that predate this floor to it at runtime.<br />The runtime cadence is capped: the node-agent re-evaluates at<br />min(interval, 5m), the operator reconciles and refreshes<br />status.lastRunTime on that same cadence (so a silently dead agent is<br />noticed within minutes and a healthy check never reads as stale), and a<br />report counts as fresh for one full agent cycle — that cadence plus<br />three times the capped Timeout. A value above 5m therefore has no<br />further runtime effect; HealthReports are written only when the verdict<br />transitions, whatever the interval.<br />Headroom, kubelet, and runtime liveness change on the order of minutes,<br />so a long interval must not accept a measurement that old: a 24h<br />interval still detects a disk filling up within minutes. Unchanged<br />evaluations refresh only status liveness (LastRunTime); a new<br />HealthReport is written only when the verdict transitions. |  | Optional: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#duration-v1-meta)_ | Timeout bounds a single node-agent evaluation pass, including the kubelet<br />and runtime-socket probes. Defaults to 30s when unset. Must be at least 1s<br />(MinCheckTimeout) and must not exceed Interval. |  | Optional: \{\} <br /> |
| `historyLimit` _integer_ | HistoryLimit caps the number of HealthReports retained for this check.<br />The minimum of 1 keeps Status.LastReportName valid. | 10 | Minimum: 1 <br />Optional: \{\} <br /> |


#### NodeHealthCheckStatus



NodeHealthCheckStatus defines the observed state of NodeHealthCheck.



_Appears in:_
- [NodeHealthCheck](#nodehealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent metadata.generation reconciled by<br />the controller. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | Conditions summarize whether the controller accepted the spec, whether<br />the agent DaemonSet is rolled out and reporting, and whether the current<br />verdict was computed from a complete scan of the fleet. |  | Optional: \{\} <br /> |
| `lastRunTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | LastRunTime records when the operator last evaluated the node-agent<br />results. It is refreshed on the capped agent cadence, min(interval, 5m),<br />even when the aggregate is unchanged, so downstream liveness stays<br />fresh, and never moves backward: an incomplete window freezes it along<br />with the verdict. |  | Optional: \{\} <br /> |
| `lastResult` _string_ | LastResult is the aggregate result across every node in scope as of the<br />most recent complete evaluation. An incomplete window (a rollout, a node<br />joining, an agent restart) freezes it rather than clearing it; the<br />CoverageComplete condition says which. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `summary` _string_ | Summary is a human-readable one-line outcome naming how many nodes<br />passed and, when some did not, the worst of them. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |
| `lastReportName` _string_ | LastReportName names the HealthReport capturing the current aggregate<br />result. A new HealthReport is written only when that result transitions,<br />so this name is stable across polls that observe the same result. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `lastRunTrigger` _string_ | LastRunTrigger records the fathom.skaphos.io/run-now annotation value<br />most recently consumed. A new value is carried to the node-agents through<br />their DaemonSet template, which restarts them; each agent stamps the value<br />into its report, and the operator records it here only once every node in<br />scope has a fresh report carrying it. Until then the previous verdict is<br />kept. A given on-demand trigger therefore completes exactly once. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `desiredNodes` _integer_ | DesiredNodes is the number of nodes the agent DaemonSet targets<br />(DaemonSet status DesiredNumberScheduled). |  | Optional: \{\} <br /> |
| `reportingNodes` _integer_ | ReportingNodes is the number of fresh, well-formed reports the operator<br />found at its latest reconcile, including surplus reports from nodes that<br />have since left scope. It is refreshed every reconcile, so it can change<br />while LastResult and NodeResults are frozen across an incomplete window;<br />it is a liveness count, not the coverage signal — CoverageComplete is. |  | Optional: \{\} <br /> |
| `nodeResults` _[NodeHealthNodeResult](#nodehealthnoderesult) array_ | NodeResults holds one entry per node in the most recent complete<br />evaluation, sorted by node name, as bounded per-node detail. Capped at<br />100 entries, so in a larger fleet a node can be absent here although it<br />reported; the fleet-wide coverage signal is the CoverageComplete<br />condition, which names the nodes that have not reported. The verdict and<br />coverage are computed across every node before truncation. Like<br />LastResult it is frozen, not cleared, across an incomplete window. |  | MaxItems: 100 <br />Optional: \{\} <br /> |


#### NodeHealthCheckType

_Underlying type:_ _string_

NodeHealthCheckType selects what a NodeHealthCheck asserts on each node.

The five types are not equal in what they cost the agent. DiskHeadroom and
InodeHeadroom are a statfs over a read-only hostPath mount and keep the
hardened default profile. NodeCondition is evaluated by the operator, not
the agent, and needs a cluster-wide read on Node objects. KubeletHealthz
needs hostNetwork, because the kubelet's health endpoint binds to
localhost. ContainerRuntime needs the runtime socket mounted and the agent
running as root to open it. The operator grants each privilege only when a
check of that type is present, so a spec that declares only headroom checks
never pays for the others (#206).

_Validation:_
- Enum: [DiskHeadroom InodeHeadroom NodeCondition KubeletHealthz ContainerRuntime]

_Appears in:_
- [NodeHealthCheckItem](#nodehealthcheckitem)

| Field | Description |
| --- | --- |
| `DiskHeadroom` | NodeHealthCheckDiskHeadroom asserts the percentage of free bytes on the<br />filesystem holding Path stays above the thresholds.<br /> |
| `InodeHeadroom` | NodeHealthCheckInodeHeadroom asserts the percentage of free inodes on the<br />filesystem holding Path stays above the thresholds.<br /> |
| `NodeCondition` | NodeHealthCheckNodeCondition asserts the node's own status conditions<br />carry their healthy value (Ready=True; every pressure condition False).<br /> |
| `KubeletHealthz` | NodeHealthCheckKubeletHealthz asserts the kubelet answers its localhost<br />/healthz endpoint.<br /> |
| `ContainerRuntime` | NodeHealthCheckContainerRuntime asserts the container runtime accepts<br />connections on its CRI socket.<br /> |


#### NodeHealthNodeResult



NodeHealthNodeResult is the folded verdict for one node on the most recent
complete evaluation.



_Appears in:_
- [NodeHealthCheckStatus](#nodehealthcheckstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `node` _string_ | Node is the node the result belongs to. |  | MaxLength: 253 <br /> |
| `result` _string_ | Result is the worst outcome across every check evaluated on this node. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br /> |
| `message` _string_ | Message summarises the worst check on the node, so a failing node is<br />triaged without opening the HealthReport. |  | MaxLength: 512 <br />Optional: \{\} <br /> |
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#time-v1-meta)_ | ObservedAt is when the node's report was produced. |  | Optional: \{\} <br /> |


#### ThresholdValue

_Underlying type:_ _string_

ThresholdValue is one adapter threshold knob value. It is an ordinary
string on the wire; the MaxLength bound exists so the API server's CEL cost
estimator has a real input size for the threshold shape rules below (an
unbounded map value string prices those rules out of the per-CRD budget).

_Validation:_
- MaxLength: 64

_Appears in:_
- [AddonCheckFamilyPolicy](#addoncheckfamilypolicy)



