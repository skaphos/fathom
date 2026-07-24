# Graph Report - .  (2026-07-23)

## Corpus Check
- Large corpus: 221 files · ~333,113 words. Semantic extraction will be expensive (many Claude tokens). Consider running on a subfolder, or use --no-semantic to run AST-only.

## Summary
- 1431 nodes · 3828 edges · 51 communities detected
- Extraction: 69% EXTRACTED · 31% INFERRED · 0% AMBIGUOUS · INFERRED: 1203 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- [[_COMMUNITY_Adapter Run & Probe Harness|Adapter Run & Probe Harness]]
- [[_COMMUNITY_CLI Entrypoints & Scan Tests|CLI Entrypoints & Scan Tests]]
- [[_COMMUNITY_Adapter Check Engine|Adapter Check Engine]]
- [[_COMMUNITY_CRD Types & DeepCopy|CRD Types & DeepCopy]]
- [[_COMMUNITY_Adapter Test Helpers|Adapter Test Helpers]]
- [[_COMMUNITY_Declarative Evaluators|Declarative Evaluators]]
- [[_COMMUNITY_Manager Wiring & Sweeper Tests|Manager Wiring & Sweeper Tests]]
- [[_COMMUNITY_Adapter RBAC Metadata|Adapter RBAC Metadata]]
- [[_COMMUNITY_Documentation & Guides|Documentation & Guides]]
- [[_COMMUNITY_Feature Specs & Health Contracts|Feature Specs & Health Contracts]]
- [[_COMMUNITY_Node Cert Scan Engine|Node Cert Scan Engine]]
- [[_COMMUNITY_Check Observability Metrics|Check Observability Metrics]]
- [[_COMMUNITY_Controller Report Writing|Controller Report Writing]]
- [[_COMMUNITY_Workload E2E Helpers|Workload E2E Helpers]]
- [[_COMMUNITY_Adapter Version Detection|Adapter Version Detection]]
- [[_COMMUNITY_Adapter Registry|Adapter Registry]]
- [[_COMMUNITY_Telemetry Adapter Tests|Telemetry Adapter Tests]]
- [[_COMMUNITY_Declarative Engine Tests|Declarative Engine Tests]]
- [[_COMMUNITY_Condition Check Tests|Condition Check Tests]]
- [[_COMMUNITY_Root Command & Signals|Root Command & Signals]]
- [[_COMMUNITY_Reconciler Tracing Tests|Reconciler Tracing Tests]]
- [[_COMMUNITY_Pod Projection Tests|Pod Projection Tests]]
- [[_COMMUNITY_Probe Pod Builder Tests|Probe Pod Builder Tests]]
- [[_COMMUNITY_Tracing Init|Tracing Init]]
- [[_COMMUNITY_Release Process & Lockstep|Release Process & Lockstep]]
- [[_COMMUNITY_HealthReport Idempotency|HealthReport Idempotency]]
- [[_COMMUNITY_Lockstep Gate Tests|Lockstep Gate Tests]]
- [[_COMMUNITY_Reconciler Tracing|Reconciler Tracing]]
- [[_COMMUNITY_ClusterHealth Controller Tests|ClusterHealth Controller Tests]]
- [[_COMMUNITY_NodeLocalDNS E2E|NodeLocalDNS E2E]]
- [[_COMMUNITY_Impersonation E2E|Impersonation E2E]]
- [[_COMMUNITY_Istio E2E|Istio E2E]]
- [[_COMMUNITY_AddonCheck Refresh E2E|AddonCheck Refresh E2E]]
- [[_COMMUNITY_Kube-State-Metrics E2E|Kube-State-Metrics E2E]]
- [[_COMMUNITY_Cilium E2E|Cilium E2E]]
- [[_COMMUNITY_ExternalDNS E2E|ExternalDNS E2E]]
- [[_COMMUNITY_Argo CD E2E|Argo CD E2E]]
- [[_COMMUNITY_Azure Workload Identity E2E|Azure Workload Identity E2E]]
- [[_COMMUNITY_Metrics-Server E2E|Metrics-Server E2E]]
- [[_COMMUNITY_Envoy Gateway E2E|Envoy Gateway E2E]]
- [[_COMMUNITY_Cert-Manager E2E|Cert-Manager E2E]]
- [[_COMMUNITY_CoreDNS E2E|CoreDNS E2E]]
- [[_COMMUNITY_Annotation Check|Annotation Check]]
- [[_COMMUNITY_CRD Check|CRD Check]]
- [[_COMMUNITY_CronJob Check|CronJob Check]]
- [[_COMMUNITY_HealthCheck Controller Tests|HealthCheck Controller Tests]]
- [[_COMMUNITY_HealthReport Idempotency Tests|HealthReport Idempotency Tests]]
- [[_COMMUNITY_Impersonation Envtest|Impersonation Envtest]]
- [[_COMMUNITY_CRD Validation Tests|CRD Validation Tests]]
- [[_COMMUNITY_Generated DeepCopy|Generated DeepCopy]]
- [[_COMMUNITY_Package Doc|Package Doc]]

## God Nodes (most connected - your core abstractions)
1. `assertHasOutcome()` - 142 edges
2. `newFakeClient()` - 80 edges
3. `New()` - 76 edges
4. `join()` - 67 edges
5. `assertHasDetail()` - 57 edges
6. `newFakeClient()` - 48 edges
7. `healthyObjects()` - 38 edges
8. `deploymentInNamespace()` - 32 edges
9. `MustEngine()` - 32 edges
10. `podInNamespace()` - 28 edges

## Surprising Connections (you probably didn't know these)
- `EnsureCompatible SemVer Handshake` --semantically_similar_to--> `CRD Maturity Ladder (alpha/beta/GA)`  [INFERRED] [semantically similar]
  pkg/adapter/version.go → docs/reference/api-versioning.md
- `Fathom Architecture` --references--> `NewScheme (scheme registration)`  [EXTRACTED]
  docs/architecture.md → internal/app/run.go
- `Code Map` --references--> `Options / bindings() Configuration Table`  [EXTRACTED]
  docs/code-map.md → internal/app/options.go
- `Configuration Reference` --references--> `Options / bindings() Configuration Table`  [EXTRACTED]
  docs/reference/configuration.md → internal/app/options.go
- `Rationale: in-process interface over gRPC/OCI/plugin loaders` --rationale_for--> `Adapter Registry`  [EXTRACTED]
  docs/adr/0001-in-process-adapter-contract.md → internal/adapter/registry/registry.go

## Hyperedges (group relationships)
- **AddonCheck → HealthCheck → ClusterHealth chain with HealthReport history** — readme_addoncheck, readme_healthcheck, readme_clusterhealth, readme_healthreport, readme_aggregation_chain [EXTRACTED 1.00]
- **Speckit artifact pipeline for feature 001 (spec → plan → tasks → quickstart)** — spec_alerting_observability, plan_alerting_observability_plan, tasks_alerting_observability_tasks, quickstart_validation [EXTRACTED 1.00]
- **Alerting-grade observability contract surface (gauges, events, alert rules)** — metrics_fathom_check_result, metrics_fathom_check_last_run_timestamp_seconds, metrics_alert_rules, events_resultchanged, events_failure_reasons [EXTRACTED 1.00]
- **Declarative adapter stack: definition -> engine -> contract -> registration -> scoped RBAC** — definition_addondefinition, authoring_adapters_declarative_engine, adapter_adapter, run_builtinadapters, rbac_adapter_impersonation [EXTRACTED 1.00]
- **AddonCheck -> HealthCheck -> ClusterHealth status aggregation chain** — addoncheck_types_addoncheck, healthcheck_types_healthcheck, clusterhealth_types_clusterhealth, healthreport_types_healthreport, addoncheck_controller_addoncheckreconciler, healthcheck_controller_healthcheckreconciler, clusterhealth_controller_clusterhealthreconciler [EXTRACTED 1.00]
- **Probe-pod lifecycle: build, launch, parse, sweep orphans** — pod_pod, launcher_launcher, sweeper_sweeper, architecture_probe_pod_model [EXTRACTED 1.00]

## Communities

### Community 0 - "Adapter Run & Probe Harness"
Cohesion: 0.03
Nodes (157): TestCountAbsent(), assertFamily(), assertHasDetail(), TestRun_EmitsSpan(), daemonSetWithAnnotations(), lockCheck(), lockJSON(), nodeRebootCheck() (+149 more)

### Community 1 - "CLI Entrypoints & Scan Tests"
Cohesion: 0.03
Nodes (134): normalizeShell(), stripShellComment(), TestCoverageGateSkipsNoPackages(), eventRow, TestE2EShardPlannerKnowsEveryOptInAddon(), TestE2E(), getMetricsOutput(), serviceAccountToken() (+126 more)

### Community 2 - "Adapter Check Engine"
Cohesion: 0.04
Nodes (71): adapterOutcome(), boundedNodeList(), certificateCheck(), certificateDetails(), certManagerComponents(), check(), conditionDetails(), conditionStatus() (+63 more)

### Community 3 - "CRD Types & DeepCopy"
Cohesion: 0.03
Nodes (44): deepCopyContract(), fullyPopulatedAddonCheck(), fullyPopulatedClusterHealth(), fullyPopulatedHealthCheck(), fullyPopulatedHealthReport(), fullyPopulatedNodeCertificateCheck(), runtimeObjectContract(), TestDeepCopy_AddonCheck() (+36 more)

### Community 4 - "Adapter Test Helpers"
Cohesion: 0.09
Nodes (83): adapterWithLauncher(), assertHasOutcome(), assertNoKind(), assertNoTarget(), certManagerResource(), daemonSetWithStatus(), dnsEndpointSlice(), dnsEndpointSliceNamed() (+75 more)

### Community 5 - "Declarative Evaluators"
Cohesion: 0.05
Nodes (34): conditionStatus(), policySelector(), resourceAbsent(), TestCondition_ResolveVersion(), TestResourceAbsent_DoesNotTreatMissingNamespaceAsMissingAPI(), Established(), PreferredServedVersion(), crd() (+26 more)

### Community 6 - "Manager Wiring & Sweeper Tests"
Cohesion: 0.04
Nodes (48): addonSA(), TestAdapterClient(), TestRunAddonCheckFailsClosedWhenNamespaceEmptyInCluster(), TestRunAddonCheckFailsClosedWithoutScopedClient(), appFakeAdapter, fakeClientFactory, programmableAdapter, SAUsername() (+40 more)

### Community 7 - "Adapter RBAC Metadata"
Cohesion: 0.05
Nodes (49): PolicyRule, RBACDeclarer, fakeAdvertisingAdapter, fakePolicyAdapter, TestEngine_Metadata(), TestEnvoyGateway_AdapterMetadata(), TestExternalDNS_AdapterMetadata(), TestFilesRejectsIncompleteRule() (+41 more)

### Community 8 - "Documentation & Guides"
Cohesion: 0.11
Nodes (61): ADR-0001 In-process Adapter Contract, Rationale: in-process interface over gRPC/OCI/plugin loaders, ADR-0002 HealthReport as First-class CRD, Rationale: CRD history without external storage dependency, ADR-0003 Probe-pod Model, Rationale: representative network topology without a DaemonSet, ADR-0004 HealthCheck as Thin Wrapper, Rationale: uniform wrapper preserves aggregator contract (+53 more)

### Community 9 - "Feature Specs & Health Contracts"
Cohesion: 0.06
Nodes (49): ClusterHealth External Contract (derived only from HealthCheck.status), Cobra+Viper Configuration Model (flag → env → file → default), Run e2e After Major Changes Policy, AGENTS.md Repository Guidelines (CLAUDE.md symlink), SPDX Boilerplate Header, Breaking Change: ClusterHealth Made Cluster-Scoped (0.4.0), DCO Sign-Off Requirement, Contributor Safety Expectations (Bounded Work, Minimal RBAC) (+41 more)

### Community 10 - "Node Cert Scan Engine"
Cohesion: 0.07
Nodes (38): minimalKubeconfig, ScanOptions, aggregateNodeReports(), controlPlaneTolerations(), healthReportForNodeCert(), joinPaths(), nodeOutcomeToResult(), pruneNodeCertHealthReports() (+30 more)

### Community 11 - "Check Observability Metrics"
Cohesion: 0.09
Nodes (32): TestFamilyOutcome(), TestOutcomeValid(), TestWorstResult(), WorstResult(), ctrlRegistryGather(), gatherCheckSeries(), gatherOneHot(), TestDeleteCheckSeries() (+24 more)

### Community 12 - "Controller Report Writing"
Cohesion: 0.06
Nodes (18): healthReportCount(), createAddonCheckWithStatusForObservability(), TestClusterHealthSelectsHealthCheck(), TestHealthCheckEventHandler(), absentReportingAdapter, conflictOnceStatusClient, conflictOnceStatusWriter, countingStatusClient (+10 more)

### Community 13 - "Workload E2E Helpers"
Cohesion: 0.13
Nodes (37): assertNoOutcome(), NewArgoCDEngine(), argoApp(), argocdHealthyObjects(), runArgoCD(), TestArgoCD_AbsentClusterFails(), TestArgoCD_ApplicationStateRollup(), TestArgoCD_HealthyWithSyncedApplication() (+29 more)

### Community 14 - "Adapter Version Detection"
Cohesion: 0.09
Nodes (25): Engine, versionAddress, endRunSpan(), NewEngine(), TestNewEngine_Validation(), validVersionSource(), validWorkloadKind(), resolveFamily() (+17 more)

### Community 15 - "Adapter Registry"
Cohesion: 0.1
Nodes (16): DeleteCheckSeries(), RecordReconcile(), TestAdapterMetrics(), TestMetricsAreValidCollectors(), TestRecordAdapterRunHelper(), TestRecordReconcileHelper(), fakeAdapter, newFake() (+8 more)

### Community 16 - "Telemetry Adapter Tests"
Cohesion: 0.33
Nodes (21): assertCheck(), findCheck(), ksmService(), passingLauncher(), readyPod(), runRequest(), TestRun_AllFamiliesDisabledEmitsSentinelSkip(), TestRun_HealthyDeploymentAndEndpointsPass() (+13 more)

### Community 17 - "Declarative Engine Tests"
Cohesion: 0.25
Nodes (22): NewCiliumEngine(), clientObject, assertFamily(), assertHasDetail(), assertHasOutcome(), assertNoKind(), assertNoOutcome(), ciliumCRDNames() (+14 more)

### Community 18 - "Condition Check Tests"
Cohesion: 0.2
Nodes (20): runManaged(), TestCondition_ClusterScopedListsWithoutNamespace(), TestCondition_ConditionStatus(), TestCondition_InvalidAPIVersionErrors(), TestCondition_InvalidSelectorErrors(), TestCondition_ListErrorDescribesNamespaceScope(), TestCondition_ListNameFallsBackToKind(), TestCondition_NamedClusterScopedGet() (+12 more)

### Community 19 - "Root Command & Signals"
Cohesion: 0.23
Nodes (11): NewRootCommand(), signalContext(), TestSignalContext_PropagatesParentCancellation(), TestSignalContext_SIGINTCancels(), TestSignalContext_SIGTERMCancels(), TestSignalContext_StopReleasesContext(), TestNewRootCommand_BasicWiring(), TestNewRootCommand_HelpDoesNotErrorWithoutKubeconfig() (+3 more)

### Community 20 - "Reconciler Tracing Tests"
Cohesion: 0.36
Nodes (8): TestListSelectedHealthChecks_ErrorNamesScope(), failingHealthCheckListClient, attrValue(), installInMemoryTracer(), newControllerScheme(), spanByName(), TestClusterHealthReconcile_EmitsSpan(), TestHealthCheckReconcile_EmitsSpan()

### Community 21 - "Pod Projection Tests"
Cohesion: 0.58
Nodes (9): optedInPod(), runProjection(), TestPodProjection_AllInjectedPasses(), TestPodProjection_InactivePodsSkipped(), TestPodProjection_MissingEnvOnlyFails(), TestPodProjection_MissingVolumeFails(), TestPodProjection_NoOptedInPodsSkipped(), TestPodProjection_PolicyNamespacesScopeTheScan() (+1 more)

### Community 22 - "Probe Pod Builder Tests"
Cohesion: 0.25
Nodes (4): assertArgs(), TestPodBuildsHardenedDNSProbe(), TestPodBuildsHTTPGetArgs(), TestPodRejectsInvalidRequests()

### Community 23 - "Tracing Init"
Cohesion: 0.36
Nodes (6): Config, Init(), ShutdownFunc, restoreGlobalProvider(), TestInit_DisabledInstallsNoopProvider(), TestInit_EnabledInstallsRecordingProvider()

### Community 24 - "Release Process & Lockstep"
Cohesion: 0.4
Nodes (6): Kubernetes Test-Version Lockstep (envtest / kind / crd-ref-docs), Fathom Release History (Changelog), Conventional Commits Policy, Why Lockstep Is Automated: Human-Gated Contract Failed for 0.3.0/0.3.1 (SKA-579), Release Please Flow, Probe/Node-Agent Version Lockstep Gate

### Community 25 - "HealthReport Idempotency"
Cohesion: 0.6
Nodes (4): createOrReuseHealthReport(), deterministicHealthReportName(), useDeterministicHealthReportName(), validateReusableHealthReport()

### Community 26 - "Lockstep Gate Tests"
Cohesion: 0.83
Nodes (3): runLockstep(), TestVersionLockstepDetectsDrift(), TestVersionLockstepInSync()

### Community 27 - "Reconciler Tracing"
Cohesion: 0.67
Nodes (0): 

### Community 28 - "ClusterHealth Controller Tests"
Cohesion: 1.0
Nodes (0): 

### Community 29 - "NodeLocalDNS E2E"
Cohesion: 1.0
Nodes (0): 

### Community 30 - "Impersonation E2E"
Cohesion: 1.0
Nodes (0): 

### Community 31 - "Istio E2E"
Cohesion: 1.0
Nodes (0): 

### Community 32 - "AddonCheck Refresh E2E"
Cohesion: 1.0
Nodes (0): 

### Community 33 - "Kube-State-Metrics E2E"
Cohesion: 1.0
Nodes (0): 

### Community 34 - "Cilium E2E"
Cohesion: 1.0
Nodes (0): 

### Community 35 - "ExternalDNS E2E"
Cohesion: 1.0
Nodes (0): 

### Community 36 - "Argo CD E2E"
Cohesion: 1.0
Nodes (0): 

### Community 37 - "Azure Workload Identity E2E"
Cohesion: 1.0
Nodes (0): 

### Community 38 - "Metrics-Server E2E"
Cohesion: 1.0
Nodes (0): 

### Community 39 - "Envoy Gateway E2E"
Cohesion: 1.0
Nodes (0): 

### Community 40 - "Cert-Manager E2E"
Cohesion: 1.0
Nodes (0): 

### Community 41 - "CoreDNS E2E"
Cohesion: 1.0
Nodes (0): 

### Community 42 - "Annotation Check"
Cohesion: 1.0
Nodes (0): 

### Community 43 - "CRD Check"
Cohesion: 1.0
Nodes (0): 

### Community 44 - "CronJob Check"
Cohesion: 1.0
Nodes (0): 

### Community 45 - "HealthCheck Controller Tests"
Cohesion: 1.0
Nodes (0): 

### Community 46 - "HealthReport Idempotency Tests"
Cohesion: 1.0
Nodes (0): 

### Community 47 - "Impersonation Envtest"
Cohesion: 1.0
Nodes (0): 

### Community 48 - "CRD Validation Tests"
Cohesion: 1.0
Nodes (0): 

### Community 49 - "Generated DeepCopy"
Cohesion: 1.0
Nodes (0): 

### Community 50 - "Package Doc"
Cohesion: 1.0
Nodes (0): 

## Knowledge Gaps
- **58 isolated node(s):** `config`, `result`, `eventRow`, `tokenRequest`, `dsRollout` (+53 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **Thin community `ClusterHealth Controller Tests`** (2 nodes): `requestNames()`, `clusterhealth_controller_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `NodeLocalDNS E2E`** (1 nodes): `nodelocaldns_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Impersonation E2E`** (1 nodes): `impersonation_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Istio E2E`** (1 nodes): `istio_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `AddonCheck Refresh E2E`** (1 nodes): `addoncheck_refresh_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Kube-State-Metrics E2E`** (1 nodes): `kubestatemetrics_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Cilium E2E`** (1 nodes): `cilium_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `ExternalDNS E2E`** (1 nodes): `externaldns_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Argo CD E2E`** (1 nodes): `argocd_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Azure Workload Identity E2E`** (1 nodes): `azureworkloadidentity_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Metrics-Server E2E`** (1 nodes): `metricsserver_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Envoy Gateway E2E`** (1 nodes): `envoygateway_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Cert-Manager E2E`** (1 nodes): `certmanager_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `CoreDNS E2E`** (1 nodes): `coredns_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Annotation Check`** (1 nodes): `annotation.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `CRD Check`** (1 nodes): `crd.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `CronJob Check`** (1 nodes): `cronjob.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `HealthCheck Controller Tests`** (1 nodes): `healthcheck_controller_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `HealthReport Idempotency Tests`** (1 nodes): `healthreport_idempotency_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Impersonation Envtest`** (1 nodes): `impersonation_envtest_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `CRD Validation Tests`** (1 nodes): `crd_validation_test.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Generated DeepCopy`** (1 nodes): `zz_generated.deepcopy.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Package Doc`** (1 nodes): `doc.go`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `New()` connect `Adapter Test Helpers` to `Adapter Run & Probe Harness`, `CLI Entrypoints & Scan Tests`, `CRD Types & DeepCopy`, `Manager Wiring & Sweeper Tests`, `Check Observability Metrics`, `Controller Report Writing`, `Adapter Version Detection`, `Adapter Registry`, `Telemetry Adapter Tests`, `Tracing Init`?**
  _High betweenness centrality (0.209) - this node is a cross-community bridge._
- **Why does `join()` connect `CLI Entrypoints & Scan Tests` to `Adapter Check Engine`, `Declarative Evaluators`, `Manager Wiring & Sweeper Tests`, `Adapter RBAC Metadata`, `Node Cert Scan Engine`, `Telemetry Adapter Tests`, `Probe Pod Builder Tests`, `HealthReport Idempotency`?**
  _High betweenness centrality (0.136) - this node is a cross-community bridge._
- **Why does `assertHasOutcome()` connect `Adapter Test Helpers` to `Adapter Run & Probe Harness`, `Workload E2E Helpers`, `Adapter Version Detection`, `Condition Check Tests`, `Pod Projection Tests`?**
  _High betweenness centrality (0.091) - this node is a cross-community bridge._
- **Are the 108 inferred relationships involving `assertHasOutcome()` (e.g. with `TestCRD_AbsenceResolution()` and `TestCronJobCheck()`) actually correct?**
  _`assertHasOutcome()` has 108 INFERRED edges - model-reasoned connections that need verification._
- **Are the 68 inferred relationships involving `newFakeClient()` (e.g. with `TestEngine_EmitsSpan()` and `runCronJob()`) actually correct?**
  _`newFakeClient()` has 68 INFERRED edges - model-reasoned connections that need verification._
- **Are the 75 inferred relationships involving `New()` (e.g. with `runDNS()` and `runTCPConnect()`) actually correct?**
  _`New()` has 75 INFERRED edges - model-reasoned connections that need verification._
- **Are the 64 inferred relationships involving `join()` (e.g. with `runMain()` and `TestScanAndPublishCreatesAndUpdatesConfigMap()`) actually correct?**
  _`join()` has 64 INFERRED edges - model-reasoned connections that need verification._