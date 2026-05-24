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



#### AddonCheck



AddonCheck is the Schema for the addonchecks API.



_Appears in:_
- [AddonCheckList](#addonchecklist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `AddonCheck` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AddonCheckSpec](#addoncheckspec)_ |  |  |  |
| `status` _[AddonCheckStatus](#addoncheckstatus)_ |  |  |  |


#### AddonCheckFamilyPolicy



AddonCheckFamilyPolicy configures one adapter-defined family of checks.



_Appears in:_
- [AddonCheckSpec](#addoncheckspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled gates execution of this family. |  | Optional: \{\} <br /> |
| `namespaces` _string array_ | Namespaces narrows this family to resources in specific namespaces. Empty<br />means all namespaces the adapter can read. |  | Optional: \{\} <br /> |
| `labelSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#labelselector-v1-meta)_ | LabelSelector narrows this family to resources matching the selector. |  | Optional: \{\} <br /> |
| `thresholds` _object (keys:string, values:string)_ | Thresholds carries adapter-specific string knobs, such as warnDays or<br />failDays. Adapter documentation defines the supported keys. |  | Optional: \{\} <br /> |


#### AddonCheckList



AddonCheckList contains a list of AddonCheck.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `AddonCheckList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AddonCheck](#addoncheck) array_ |  |  |  |


#### AddonCheckSpec



AddonCheckSpec defines the desired state of AddonCheck.



_Appears in:_
- [AddonCheck](#addoncheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `addonType` _string_ | AddonType selects the adapter responsible for this check, such as<br />cert-manager, coredns, or external-secrets. |  | MinLength: 1 <br /> |
| `interval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | Interval is the desired cadence between successful check runs. |  | Optional: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | Timeout bounds a single adapter run. |  | Optional: \{\} <br /> |
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
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions summarize whether the controller accepted and processed this<br />check specification. |  | Optional: \{\} <br /> |
| `lastRunTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastRunTime records when an adapter run last completed. |  | Optional: \{\} <br /> |
| `lastResult` _string_ | LastResult is the aggregate result from the most recent adapter run. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `lastReportName` _string_ | LastReportName names the HealthReport created for the most recent run. |  | Optional: \{\} <br /> |


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



ClusterHealth is the Schema for the clusterhealths API.



_Appears in:_
- [ClusterHealthList](#clusterhealthlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `ClusterHealth` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
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
| `name` _string_ | Name of the contributing HealthCheck. |  | MaxLength: 253 <br />MinLength: 1 <br /> |
| `result` _[HealthReportResult](#healthreportresult)_ | Result mirrors the contributing HealthCheck's Status.Result. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `summary` _string_ | Summary mirrors the contributing HealthCheck's Status.Summary. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | ObservedAt mirrors the contributing HealthCheck's<br />Status.SourceObservedAt, when present. |  | Optional: \{\} <br /> |


#### ClusterHealthList



ClusterHealthList contains a list of ClusterHealth.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `ClusterHealthList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[ClusterHealth](#clusterhealth) array_ |  |  |  |


#### ClusterHealthSpec



ClusterHealthSpec defines the desired state of ClusterHealth. ClusterHealth
is an aggregator: it rolls up the Status of selected HealthCheck resources
into a single worst-case Result for cluster-wide consumers (dashboards,
alerting, gates).



_Appears in:_
- [ClusterHealth](#clusterhealth)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#labelselector-v1-meta)_ | Selector selects the HealthChecks whose status this aggregate rolls up.<br />An empty or nil selector matches all HealthChecks in the same namespace<br />as this ClusterHealth. |  | Optional: \{\} <br /> |
| `description` _string_ | Description is a human-readable purpose for this aggregate. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |


#### ClusterHealthStatus



ClusterHealthStatus defines the observed state of ClusterHealth.



_Appears in:_
- [ClusterHealth](#clusterhealth)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent metadata.generation reconciled. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions summarize the controller's view of the aggregate. |  | Optional: \{\} <br /> |
| `result` _[HealthReportResult](#healthreportresult)_ | Result is the worst-case roll-up across the selected HealthChecks.<br />Empty when no HealthChecks match the selector. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `matchedCount` _integer_ | MatchedCount is the number of HealthChecks selected for this aggregate. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `children` _[ClusterHealthChildSummary](#clusterhealthchildsummary) array_ | Children summarizes each selected HealthCheck's contribution. |  | Optional: \{\} <br /> |
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | ObservedAt is when the aggregator last refreshed this status. |  | Optional: \{\} <br /> |


#### HealthCheck



HealthCheck is the Schema for the healthchecks API.



_Appears in:_
- [HealthCheckList](#healthchecklist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthCheck` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[HealthCheckSpec](#healthcheckspec)_ |  |  |  |
| `status` _[HealthCheckStatus](#healthcheckstatus)_ |  |  |  |


#### HealthCheckList



HealthCheckList contains a list of HealthCheck.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthCheckList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
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
| `checkRef` _[CheckTargetRef](#checktargetref)_ | CheckRef identifies the specialized check resource this HealthCheck wraps. |  |  |
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
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions summarize the controller's view of the wrapped check. |  | Optional: \{\} <br /> |
| `result` _[HealthReportResult](#healthreportresult)_ | Result is the outcome surfaced from the referenced check's most recent run. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br />Optional: \{\} <br /> |
| `summary` _string_ | Summary is a human-readable one-line outcome description. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |
| `sourceObservedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | SourceObservedAt is when the referenced check last completed. |  | Optional: \{\} <br /> |
| `lastReportName` _string_ | LastReportName names the most recent HealthReport produced by the<br />referenced check, when one exists. |  | MaxLength: 253 <br />Optional: \{\} <br /> |


#### HealthReport



HealthReport is the Schema for the healthreports API.



_Appears in:_
- [HealthReportList](#healthreportlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthReport` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[HealthReportSpec](#healthreportspec)_ |  |  |  |
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
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | ObservedAt is when this check completed. |  |  |
| `duration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | Duration is how long this check took. |  | Optional: \{\} <br /> |


#### HealthReportList



HealthReportList contains a list of HealthReport.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `fathom.skaphos.io/v1alpha1` | | |
| `kind` _string_ | `HealthReportList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
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
| `contractVersion` _string_ | ContractVersion is the adapter contract version used for this run. |  | Optional: \{\} <br /> |
| `result` _[HealthReportResult](#healthreportresult)_ | Result is the aggregate outcome across all checks. |  | Enum: [Pass Warn Fail Error Skipped Unknown] <br /> |
| `checks` _[HealthReportCheck](#healthreportcheck) array_ | Checks are the individual observations produced by the adapter. |  | Optional: \{\} <br /> |
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | ObservedAt is when the adapter run completed. |  |  |
| `duration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | Duration is the total adapter run duration. |  | Optional: \{\} <br /> |


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


