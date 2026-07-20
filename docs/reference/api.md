# API Reference

## Packages
- [fathom.skaphos.io/v1alpha1](#fathomskaphosiov1alpha1)


## fathom.skaphos.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the fathom v1alpha1 API group.

### Resource Types
- [AddonCheck](#addoncheck)
- [AddonCheckList](#addonchecklist)
- [ClusterHealth](#clusterhealth)
- [ClusterHealthList](#clusterhealthlist)
- [HealthCheck](#healthcheck)
- [HealthCheckList](#healthchecklist)
- [HealthReport](#healthreport)
- [HealthReportList](#healthreportlist)
- [NodeCertificateCheck](#nodecertificatecheck)
- [NodeCertificateCheckList](#nodecertificatechecklist)



#### AddonCheck



AddonCheck is the Schema for the addonchecks API.



_Appears in:_
- [AddonCheckList](#addonchecklist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `AddonCheck` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AddonCheckSpec](#addoncheckspec)_ |  |  |  |
| `status` _[AddonCheckStatus](#addoncheckstatus)_ |  |  |  |


#### AddonCheckFamilyPolicy



AddonCheckFamilyPolicy configures one adapter-defined family of checks.



_Appears in:_
- [AddonCheckSpec](#addoncheckspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled gates execution of this family. | true | Optional: \{\} <br /> |
| `namespaces` _string array_ | Namespaces narrows this family to resources in specific namespaces. Empty<br />means all namespaces the adapter can read. |  | Optional: \{\} <br /> |
| `labelSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#labelselector-v1-meta)_ | LabelSelector narrows this family to resources matching the selector. |  | Optional: \{\} <br /> |
| `thresholds` _object (keys:string, values:string)_ | Thresholds carries adapter-specific string knobs, such as warnDays or<br />failDays. Adapter documentation defines the supported keys. |  | Optional: \{\} <br /> |


#### AddonCheckList



AddonCheckList contains a list of AddonCheck.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `AddonCheckList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AddonCheck](#addoncheck) array_ |  |  |  |


#### AddonCheckSpec



AddonCheckSpec defines the desired state of AddonCheck.



_Appears in:_
- [AddonCheck](#addoncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `addonType` _string_ | AddonType selects the adapter responsible for this check, such as<br />cert-manager, coredns, or external-secrets. |  | MinLength: 1 <br /> |
| `interval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | Interval is the cadence at which the adapter re-runs and the HealthReport<br />is refreshed. Defaults to 5m when unset. |  | Optional: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | Timeout bounds a single adapter run. |  | Optional: \{\} <br /> |
| `paused` _boolean_ | Paused prevents the controller from starting new adapter runs. |  | Optional: \{\} <br /> |
| `policy` _object (keys:string, values:[AddonCheckFamilyPolicy](#addoncheckfamilypolicy))_ | Policy configures adapter-defined check families. A missing or empty policy<br />leaves family selection to the adapter defaults. |  | Optional: \{\} <br /> |
| `historyLimit` _integer_ | HistoryLimit caps the number of HealthReports retained for this<br />AddonCheck. After each new HealthReport is created the controller<br />deletes the oldest reports until the total count is at or below this<br />limit. The minimum of 1 keeps Status.LastReportName referenceable. | 10 | Minimum: 1 <br />Optional: \{\} <br /> |


#### AddonCheckStatus



AddonCheckStatus defines the observed state of AddonCheck.



_Appears in:_
- [AddonCheck](#addoncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent metadata.generation reconciled by<br />the controller. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ | Conditions summarize whether the controller accepted and processed this<br />check specification. |  | Optional: \{\} <br /> |
| `lastRunTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | LastRunTime records when an adapter run last completed. |  | Optional: \{\} <br /> |
| `lastResult` _string_ | LastResult is the aggregate result from the most recent adapter run. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `absent` _integer_ | Absent is the number of checks in the most recent run whose target was not<br />installed — the required-absent Fails and optional-absent Skips alike. It<br />makes "not installed" queryable and distinct from "unhealthy" (a Fail whose<br />target exists) and "disabled" (a Skipped family). Zero when every checked<br />target is present (SKA-526). |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `detectedVersion` _string_ | DetectedVersion is the installed addon release version detected on the most<br />recent run (from the addon workload's app.kubernetes.io/version label, else<br />its container image tag). Empty when the adapter does not detect versions or<br />the version was undetectable — the run then proceeds best-effort (SKA-527). |  | Optional: \{\} <br /> |
| `lastReportName` _string_ | LastReportName names the HealthReport created for the most recent run. |  | Optional: \{\} <br /> |
| `lastRunTrigger` _string_ | LastRunTrigger records the value of the fathom.skaphos.io/run-now<br />annotation most recently consumed to force an adapter run. The controller<br />re-runs the adapter whenever the annotation value differs from this, then<br />stores it here so a given on-demand trigger fires exactly once. |  | Optional: \{\} <br /> |


#### CheckTargetRef



CheckTargetRef references a specialized check resource (AddonCheck,
DNSCheck, NodeHealthCheck, NodeCertificateCheck, ReachabilityCheck) whose
status a HealthCheck mirrors and surfaces for ClusterHealth aggregation.



_Appears in:_
- [HealthCheckSpec](#healthcheckspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | APIVersion of the target check resource. When empty, defaults to<br />fathom.skaphos.io/v1alpha1. |  | Optional: \{\} <br /> |
| `kind` _string_ | Kind of the target check resource (e.g., AddonCheck, DNSCheck). |  | MaxLength: 63 <br />MinLength: 1 <br /> |
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
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
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
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | ObservedAt mirrors the contributing HealthCheck's<br />Status.SourceObservedAt, when present. |  | Optional: \{\} <br /> |


#### ClusterHealthList



ClusterHealthList contains a list of ClusterHealth.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `ClusterHealthList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
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
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#labelselector-v1-meta)_ | Selector selects the HealthChecks whose status this aggregate rolls up.<br />An empty or nil selector matches every HealthCheck in the namespace<br />scope defined by Namespaces / ExcludedNamespaces. |  | Optional: \{\} <br /> |
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
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ | Conditions summarize the controller's view of the aggregate. |  | Optional: \{\} <br /> |
| `result` _[HealthReportResult](#healthreportresult)_ | Result is the worst-case roll-up across the selected HealthChecks.<br />Unknown (with Ready=False, Reason=NoMatches) when no HealthChecks match<br />the selector; a selected child that has no verdict yet degrades the<br />roll-up to Unknown rather than being dropped, so a failure can never<br />silently vanish. Trust this value only when the Ready condition is True:<br />the InvalidSelector and ListFailed error paths leave it empty with<br />Ready=False. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `matchedCount` _integer_ | MatchedCount is the number of HealthChecks selected for this aggregate. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `children` _[ClusterHealthChildSummary](#clusterhealthchildsummary) array_ | Children summarizes each selected HealthCheck's contribution. |  | Optional: \{\} <br /> |
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | ObservedAt is when the aggregator last refreshed this status. |  | Optional: \{\} <br /> |


#### HealthCheck



HealthCheck is the Schema for the healthchecks API.



_Appears in:_
- [HealthCheckList](#healthchecklist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthCheck` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[HealthCheckSpec](#healthcheckspec)_ |  |  |  |
| `status` _[HealthCheckStatus](#healthcheckstatus)_ |  |  |  |


#### HealthCheckList



HealthCheckList contains a list of HealthCheck.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthCheckList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
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
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ | Conditions summarize the controller's view of the wrapped check. |  | Optional: \{\} <br /> |
| `result` _[HealthReportResult](#healthreportresult)_ | Result is the outcome surfaced from the referenced check's most recent run. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `summary` _string_ | Summary is a human-readable one-line outcome description. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |
| `sourceObservedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | SourceObservedAt is when the referenced check last completed. |  | Optional: \{\} <br /> |
| `lastReportName` _string_ | LastReportName names the most recent HealthReport produced by the<br />referenced check, when one exists. |  | MaxLength: 253 <br />Optional: \{\} <br /> |


#### HealthReport



HealthReport is the Schema for the healthreports API.



_Appears in:_
- [HealthReportList](#healthreportlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthReport` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[HealthReportSpec](#healthreportspec)_ | Spec is immutable: a HealthReport is a point-in-time history record.<br />The operator only ever creates reports (createOrReuseHealthReport);<br />mutating one after the fact would rewrite history (SKA-576). |  |  |
| `status` _[HealthReportStatus](#healthreportstatus)_ |  |  |  |


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
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | ObservedAt is when this check completed. |  |  |
| `duration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | Duration is how long this check took. |  | Optional: \{\} <br /> |


#### HealthReportList



HealthReportList contains a list of HealthReport.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthReportList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
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
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | ObservedAt is when the adapter run completed. |  |  |
| `duration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | Duration is the total adapter run duration. |  | Optional: \{\} <br /> |


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
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[NodeCertificateCheckSpec](#nodecertificatecheckspec)_ |  |  |  |
| `status` _[NodeCertificateCheckStatus](#nodecertificatecheckstatus)_ |  |  |  |


#### NodeCertificateCheckList



NodeCertificateCheckList contains a list of NodeCertificateCheck.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `NodeCertificateCheckList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
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
| `tolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#toleration-v1-core) array_ | Tolerations are applied verbatim to the agent DaemonSet so it can schedule<br />onto nodes carrying arbitrary taints. It is empty by default. Control-plane<br />tolerations are NOT added here — use IncludeControlPlaneNodes for that, so<br />scheduling the privileged agent onto control-plane nodes is always an<br />explicit, auditable opt-in rather than a silent default. |  | Optional: \{\} <br /> |
| `includeControlPlaneNodes` _boolean_ | IncludeControlPlaneNodes opts the node-agent into scheduling on control-plane<br />nodes by adding tolerations for the standard control-plane and legacy master<br />taints (node-role.kubernetes.io/control-plane and .../master, Exists /<br />NoSchedule) on top of any Tolerations.<br />It defaults to false. The kubeadm apiserver, etcd, and front-proxy<br />certificates live on control-plane nodes, so set this to true to scan them —<br />but doing so mounts control-plane host paths into the agent, which is why it<br />is gated behind an explicit opt-in rather than applied by default. | false | Optional: \{\} <br /> |
| `interval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | Interval is the cadence at which each node-agent re-scans its<br />certificates and the operator refreshes the rolled-up HealthReport.<br />Defaults to 1h when unset. |  | Optional: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | Timeout bounds a single node-agent scan pass. Defaults to 30s when unset. |  | Optional: \{\} <br /> |
| `paused` _boolean_ | Paused stops the operator from running the agent DaemonSet and refreshing<br />reports. The agent DaemonSet is removed while paused; the most recent<br />Status snapshot is preserved. |  | Optional: \{\} <br /> |
| `historyLimit` _integer_ | HistoryLimit caps the number of HealthReports retained for this check.<br />After each new HealthReport the controller deletes the oldest reports<br />beyond the limit. The minimum of 1 keeps Status.LastReportName valid. | 10 | Minimum: 1 <br />Optional: \{\} <br /> |


#### NodeCertificateCheckStatus



NodeCertificateCheckStatus defines the observed state of NodeCertificateCheck.



_Appears in:_
- [NodeCertificateCheck](#nodecertificatecheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent metadata.generation reconciled by<br />the controller. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ | Conditions summarize whether the controller accepted the spec and whether<br />the agent DaemonSet is rolled out and reporting. |  | Optional: \{\} <br /> |
| `lastRunTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | LastRunTime records when the operator last rolled up a HealthReport from<br />the node-agent results. |  | Optional: \{\} <br /> |
| `lastResult` _string_ | LastResult is the aggregate result across all reporting nodes from the<br />most recent roll-up. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `lastReportName` _string_ | LastReportName names the HealthReport created for the most recent roll-up. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `desiredNodes` _integer_ | DesiredNodes is the number of nodes the agent DaemonSet targets<br />(DaemonSet status DesiredNumberScheduled). |  | Optional: \{\} <br /> |
| `reportingNodes` _integer_ | ReportingNodes is the number of nodes that have published a scan result<br />the operator consumed in the most recent roll-up. |  | Optional: \{\} <br /> |


