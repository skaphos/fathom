# Graph Report - fathom  (2026-08-08)

## Corpus Check
- 322 files · ~365,734 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 4339 nodes · 10394 edges · 307 communities (270 shown, 37 thin omitted)
- Extraction: 78% EXTRACTED · 22% INFERRED · 0% AMBIGUOUS · INFERRED: 2270 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `e1da7fee`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- .Run
- quorum-ratio-rollups/cmd/probe/main_test.go
- quorum-ratio-rollups/internal/adapter/certmanager/adapter.go
- .DeepCopy
- New
- .Get
- newScheme
- quorum-ratio-rollups/internal/adapter/rbacgen/rbacgen.go
- Add-on Checks Guide (Adapter Catalog)
- fathom_check_result Gauge (one-hot current result)
- quorum-ratio-rollups/internal/controller/nodecertificatecheck_helpers.go
- observeCheck
- .Update
- deploymentInNamespace
- MustEngine
- .Register
- .Run
- podInNamespace
- assertHasOutcome
- NewRootCommand
- newControllerScheme
- quorum-ratio-rollups/internal/adapter/declarative/podprojection_test.go
- quorum-ratio-rollups/internal/probe/pod_test.go
- Init
- Probe/Node-Agent Version Lockstep Gate
- quorum-ratio-rollups/internal/controller/healthreport_idempotency.go
- quorum-ratio-rollups/scripts/version_lockstep_gate_test.go
- CheckResult
- internal/adapter/certmanager/adapter_test.go
- cmd/probe/main_test.go
- .detectAndGateVersion
- .Run
- .Reconcile
- TargetRef
- assertHasDetail
- .Run
- .checkMetricsEndpoint
- newFakeClient
- assertHasDetail
- FamilyPolicy
- properties
- .Evaluate
- Family
- deploymentInNamespace
- HealthReportResult
- scanAndPublish
- quorum-ratio-rollups/test/utils/utils.go
- Write
- api/v1alpha1/deepcopy_test.go
- .DeepCopyInto
- common.sh
- Implementation Plan: Pre-1.0 CRD Validation Hardening
- observeCheck
- addoncheck_controller.go
- Implementation Plan: Quorum/Ratio Semantics for Managed-Resource Rollups
- Adversarial Review Findings — v0.5.0 Release Gate (#217)
- BuiltInAdapters Registration
- assertHasOutcome
- Tasks: [FEATURE NAME]
- speckit-analyze/SKILL.md
- .Evaluate
- kedaHealthyObjects
- Quickstart Validation: Quorum/Ratio Rollups
- CertResult
- .Reconcile
- PolicyRule
- RatioThresholds
- Command
- MustEngine
- join
- test/utils/utils_test.go
- internal/adapter/rbacgen/rbacgen.go
- Tasks: Adversarial Codebase Review for the v0.5.0 Release Gate
- Registry
- NewScheme (scheme registration)
- HealthCheckReconciler
- SetRunningInClusterForTest
- internal/controller/nodecertificatecheck_helpers.go
- newFakeClient
- TestAdapterClient
- .Reconcile
- image
- Load
- internal/nodecert/scan_test.go
- runArgoCD
- internal/adapter/declarative/istio_test.go
- New
- internal/probe/sweeper_test.go
- NewRootCommand
- kedaHealthyObjects
- Feature Specification: DNSCheck Resource Contract
- dnscheck_types.go
- Options
- .podInjected
- properties
- test/e2e/healthreport_helpers_test.go
- runConfigMap
- Sweeper
- Core Principles
- Fathom Architecture
- addoncheck_types.go
- dnscheck_validation_test.go
- Execution Steps
- AddonCheckReconciler
- internal/nodecert/paths.go
- internal/metrics/check_metrics_test.go
- Probe Pod Manifest Builder Pod(Request)
- quorum-ratio-rollups/internal/nodecert/paths.go
- Tasks: Pre-1.0 CRD Validation Hardening
- TestCommittedAddonRolesAreReadOnly
- establishedCRD
- pod.go
- operator_rbac_doc_test.go
- .Name
- .Run
- quorum-ratio-rollups/internal/nodecert/scan.go
- Tasks: DNSCheck Resource Contract
- validateAddonCheckPolicy
- PreferredServedVersion
- rbac
- enabled
- TestClientForSetsImpersonationAndMemoizes
- internal/adapter/declarative/annotation_test.go
- internal/adapter/declarative/field_test.go
- NewMetricsServerEngine
- runProjection
- writeFile
- crd_compat_gate_test.go
- Entity: `DNSCheck`
- quorum-ratio-rollups/internal/probe/sweeper_test.go
- NodeCertificateCheck
- Repository Guidelines
- values.schema.json
- properties
- TestEnvoyGateway_HealthyAndNoGatewaysSkipped
- Feature Specification: [FEATURE NAME]
- internal/controller/addoncheck_controller_test.go
- healthcheck_types.go
- Object
- GitHub Copilot Instructions for Fathom
- NewExternalDNSEngine
- writeNodeReportForCheck
- Tasks: Quorum/Ratio Semantics for Managed-Resource Rollups
- Feature Specification: Adversarial Codebase Review for the v0.5.0 Release Gate
- speckit-plan/SKILL.md
- speckit-specify/SKILL.md
- speckit-tasks/SKILL.md
- test/e2e/nodecert_test.go
- TestExternalSecrets_HealthyAndEmptySyncSkipped
- cadence_test.go
- Core Principles
- Changed: AddonCheck (`fathom.skaphos.io/v1alpha1`, namespaced)
- Research: Pre-1.0 CRD Validation Hardening
- API / CRD Contract-Stability Candidates — commit cb845dd
- Correctness / Reconcile-Time Review — Candidates
- Implementation Plan: DNSCheck Resource Contract
- Phase 0 Research: DNSCheck Resource Contract
- RecordReconcile
- RBAC / Least-Privilege Review — Fathom operator (commit cb845dd)
- Capabilities
- .IsReadOnly
- ClusterHealthReconciler
- TestCommittedAddonRolesAreReadOnly
- msHealthyObjects
- writeNodeReportForCheck
- Implementation Plan: [FEATURE]
- Entities
- Phase 0 Research: Adversarial Codebase Review for the v0.5.0 Release Gate
- Quickstart: Validating the DNSCheck Resource Contract
- speckit-checklist/SKILL.md
- runMain
- .Run
- runMain
- summarizeFromConditions
- createOrReuseHealthReport
- IsReadVerb
- quorum-ratio-rollups/internal/app/run_happy_test.go
- Research: Quorum/Ratio Semantics for Managed-Resource Rollups
- Quickstart Validation: Pre-1.0 CRD Validation Hardening
- Implementation Plan: Adversarial Codebase Review for the v0.5.0 Release Gate
- Quickstart: Validating the v0.5.0 Adversarial Review Release Gate
- Supply-Chain / CI-Integrity Review — Fathom @ cb845dd
- api/v1alpha1/groupversion_info_test.go
- validation_test.go
- speckit-clarify/SKILL.md
- speckit-implement/SKILL.md
- programmableAdapter
- .Get
- PreferredServedVersion
- Engine
- noMatchListClient
- webhookEntry
- Operator RBAC
- nodeAgentTemplateNeedsWrite
- runMain
- quorum-ratio-rollups/internal/controller/policy_validation_test.go
- Specification Quality Checklist: Adversarial Codebase Review for the v0.5.0 Release Gate
- Specification Quality Checklist: DNSCheck Resource Contract
- Contract: Probe `dns` Mode CLI
- speckit-constitution/SKILL.md
- Network policies
- Contract: CRD Schema-Compatibility Gate
- Deliverable Contracts: Findings Report, Refuted Record, Coverage Statement
- Security review candidates — Fathom (commit cb845dd)
- quorum-ratio-rollups/pkg/adapter/adapter_test.go
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- speckit-taskstoissues/SKILL.md
- absentReportingAdapter
- fakeAddonAdapter
- bindAddress
- .severity
- check-version-lockstep.sh
- scripts/coverage_gate_test.go
- runLockstep
- Security Policy
- [CHECKLIST TYPE] Checklist: [FEATURE NAME]
- Contract: Runtime Clamp Signal
- Contract: DNSCheck Admission Validation
- Phase 3: User Story 1 — Declare DNS intent and have it validated (P1)
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- normalizeShell
- extraArgs
- countAbsent
- internal/controller/suite_test.go
- check-coverage.sh
- e2e-shards.sh
- Contract: CRD Admission Validation
- Implementation Strategy
- TestWorstResult
- quorum-ratio-rollups/test/e2e/e2e_test.go
- requestNames
- post-install.sh
- check-crd-compat.sh
- github.com/skaphos/fathom
- github.com/skaphos/fathom/tools

## God Nodes (most connected - your core abstractions)
1. `assertHasOutcome()` - 142 edges
2. `assertHasOutcome()` - 122 edges
3. `CheckResult` - 108 edges
4. `newFakeClient()` - 80 edges
5. `newFakeClient()` - 77 edges
6. `New()` - 76 edges
7. `Family` - 73 edges
8. `Probe Pod Manifest Builder Pod(Request)` - 72 edges
9. `join()` - 67 edges
10. `FamilyPolicy` - 66 edges

## Surprising Connections (you probably didn't know these)
- `EnsureCompatible SemVer Handshake` --semantically_similar_to--> `CRD Maturity Ladder (alpha/beta/GA)`  [INFERRED] [semantically similar]
  pkg/adapter/version.go → docs/reference/api-versioning.md
- `Adapter` --rationale_for--> `Rationale: in-process interface over gRPC/OCI/plugin loaders`  [EXTRACTED]
  pkg/adapter/adapter.go → docs/adr/0001-in-process-adapter-contract.md
- `Options / bindings() Configuration Table` --references--> `Configuration Reference`  [EXTRACTED]
  internal/app/options.go → docs/reference/configuration.md
- `HealthCheck CRD (thin wrapper)` --rationale_for--> `Rationale: uniform wrapper preserves aggregator contract`  [EXTRACTED]
  api/v1alpha1/healthcheck_types.go → docs/adr/0004-healthcheck-as-wrapper.md
- `Registry` --references--> `ADR-0001 In-process Adapter Contract`  [EXTRACTED]
  internal/adapter/registry/registry.go → docs/adr/0001-in-process-adapter-contract.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **AddonCheck → HealthCheck → ClusterHealth chain with HealthReport history** — readme_addoncheck, readme_healthcheck, readme_clusterhealth, readme_healthreport, readme_aggregation_chain [EXTRACTED 1.00]
- **Speckit artifact pipeline for feature 001 (spec → plan → tasks → quickstart)** — specs_001_alerting_observability_spec_alerting_observability, specs_001_alerting_observability_plan_alerting_observability_plan, specs_001_alerting_observability_tasks_alerting_observability_tasks, specs_001_alerting_observability_quickstart_validation [EXTRACTED 1.00]
- **Alerting-grade observability contract surface (gauges, events, alert rules)** — specs_001_alerting_observability_contracts_metrics_fathom_check_result, specs_001_alerting_observability_contracts_metrics_fathom_check_last_run_timestamp_seconds, specs_001_alerting_observability_contracts_metrics_alert_rules, specs_001_alerting_observability_contracts_events_resultchanged, specs_001_alerting_observability_contracts_events_failure_reasons [EXTRACTED 1.00]
- **Declarative adapter stack: definition -> engine -> contract -> registration -> scoped RBAC** — internal_adapter_declarative_definition_addondefinition, docs_authoring_adapters_declarative_engine, adapter_adapter, internal_app_run_builtinadapters, docs_reference_rbac_adapter_impersonation [EXTRACTED 1.00]
- **AddonCheck -> HealthCheck -> ClusterHealth status aggregation chain** — api_v1alpha1_addoncheck_types_addoncheck, api_v1alpha1_healthcheck_types_healthcheck, api_v1alpha1_clusterhealth_types_clusterhealth, api_v1alpha1_healthreport_types_healthreport, controller_addoncheckreconciler, controller_healthcheckreconciler, controller_clusterhealthreconciler [EXTRACTED 1.00]
- **Probe-pod lifecycle: build, launch, parse, sweep orphans** — internal_probe_pod_pod, internal_probe_launcher_launcher, internal_probe_sweeper_sweeper, docs_architecture_probe_pod_model [EXTRACTED 1.00]

## Communities (307 total, 37 thin omitted)

### Community 0 - ".Run"
Cohesion: 0.07
Nodes (51): TestCountAbsent(), TestRun_EmitsSpan(), TestAzureWorkloadIdentity_AbsentWebhookFails(), TestClusterHealthCoversNamespace(), configMap(), runConfigMap(), TestConfigMapCheck(), TestConfigMapCheck_AbsentInheritsOptional() (+43 more)

### Community 1 - "quorum-ratio-rollups/cmd/probe/main_test.go"
Cohesion: 0.11
Nodes (34): runDNS(), runHTTPGet(), runTCPConnect(), runTCPListen(), scanMetricFamilies(), splitComma(), captureResult(), claimAndReleasePort() (+26 more)

### Community 2 - "quorum-ratio-rollups/internal/adapter/certmanager/adapter.go"
Cohesion: 0.14
Nodes (33): adapterOutcome(), certificateCheck(), certificateDetails(), certManagerComponents(), check(), conditionDetails(), conditionStatus(), conditionType() (+25 more)

### Community 3 - ".DeepCopy"
Cohesion: 0.34
Nodes (18): deepCopyContract(), fullyPopulatedAddonCheck(), fullyPopulatedClusterHealth(), fullyPopulatedHealthCheck(), fullyPopulatedHealthReport(), fullyPopulatedNodeCertificateCheck(), runtimeObjectContract(), TestDeepCopy_AddonCheck() (+10 more)

### Community 4 - "New"
Cohesion: 0.09
Nodes (77): adapterWithLauncher(), assertNoKind(), assertNoTarget(), certManagerResource(), daemonSetWithStatus(), dnsEndpointSlice(), dnsEndpointSliceNamed(), dnsService() (+69 more)

### Community 5 - ".Get"
Cohesion: 0.09
Nodes (49): firstNamespace(), containsString(), ConditionCheck, ConfigMapCheck, CRDCheck, CronJobCheck, EvalContext, WebhookCheck (+41 more)

### Community 6 - "newScheme"
Cohesion: 0.09
Nodes (22): appFakeAdapter, TestAddToScheme(), TestDeepCopyIntoExercise(), TestSchemeBuilderRegisterReturnsSelf(), Context, Request, Result, TestBuildAdapterRegistry_RegistersBuiltInAdapters() (+14 more)

### Community 7 - "quorum-ratio-rollups/internal/adapter/rbacgen/rbacgen.go"
Cohesion: 0.22
Nodes (15): TestFilesRejectsIncompleteRule(), clusterRules(), Files(), groupsCell(), k8sObject, marshalDocs(), objectMeta, renderAddon() (+7 more)

### Community 8 - "Add-on Checks Guide (Adapter Catalog)"
Cohesion: 0.25
Nodes (24): AddonCheck CRD, ClusterHealth CRD (aggregate), HealthCheck CRD (thin wrapper), HealthReport CRD (immutable history), HealthReportResult Severity Enum, NodeCertificateCheck CRD, Rationale: CRD history without external storage dependency, Aggregation / Status-Mirror Chain (+16 more)

### Community 9 - "fathom_check_result Gauge (one-hot current result)"
Cohesion: 0.06
Nodes (49): ClusterHealth External Contract (derived only from HealthCheck.status), Cobra+Viper Configuration Model (flag → env → file → default), Run e2e After Major Changes Policy, AGENTS.md Repository Guidelines (CLAUDE.md symlink), SPDX Boilerplate Header, Breaking Change: ClusterHealth Made Cluster-Scoped (0.4.0), DCO Sign-Off Requirement, Contributor Safety Expectations (Bounded Work, Minimal RBAC) (+41 more)

### Community 10 - "quorum-ratio-rollups/internal/controller/nodecertificatecheck_helpers.go"
Cohesion: 0.12
Nodes (13): TestWorstResult(), WorstResult(), aggregateNodeReports(), controlPlaneTolerations(), healthReportForNodeCert(), joinPaths(), nodeOutcomeToResult(), pruneNodeCertHealthReports() (+5 more)

### Community 11 - "observeCheck"
Cohesion: 0.11
Nodes (27): ctrlRegistryGather(), gatherCheckSeries(), gatherOneHot(), TestDeleteCheckSeries(), TestObserveCheckFlipsResult(), TestObserveCheckOneHotInvariant(), TestObserveCheckSentinels(), checkGaugeValue() (+19 more)

### Community 12 - ".Update"
Cohesion: 0.18
Nodes (9): createAddonCheckWithStatusForObservability(), TestClusterHealthSelectsHealthCheck(), TestHealthCheckEventHandler(), conflictOnceStatusClient, conflictOnceStatusWriter, Client, Context, Object (+1 more)

### Community 13 - "deploymentInNamespace"
Cohesion: 0.21
Nodes (27): assertNoOutcome(), deploymentInNamespace(), absenceEngine(), deployEngine(), failedPod(), notReadyPod(), podWithRestarts(), runEngine() (+19 more)

### Community 14 - "MustEngine"
Cohesion: 0.17
Nodes (19): TestMustEngine_PanicsOnInvalid(), crdAbsenceEngine(), TestCRD_AbsenceResolution(), cronJob(), runCronJob(), TestCronJobCheck(), TestCronJobCheck_AbsentInheritsOptional(), TestCronJobCheck_PerpetualFailurePastWindowWarns() (+11 more)

### Community 15 - ".Register"
Cohesion: 0.23
Nodes (11): GroupVersion, newFake(), TestCapabilities(), TestConcurrentAccess(), TestLookup(), TestRegister(), TestRegister_DuplicateAddonType(), TestRegister_PartialFailureLeavesRegistryUnchanged() (+3 more)

### Community 16 - ".Run"
Cohesion: 0.13
Nodes (57): assertCheck(), findCheck(), ksmService(), passingLauncher(), readyPod(), runRequest(), TestRun_AllFamiliesDisabledEmitsSentinelSkip(), TestRun_HealthyDeploymentAndEndpointsPass() (+49 more)

### Community 17 - "podInNamespace"
Cohesion: 0.13
Nodes (35): daemonSetWithAnnotations(), lockCheck(), lockJSON(), nodeRebootCheck(), nodeWithAnnotations(), runAnnotation(), TestAnnotationStaleness_NamedLock(), TestAnnotationStaleness_NodeList() (+27 more)

### Community 18 - "assertHasOutcome"
Cohesion: 0.18
Nodes (30): assertHasOutcome(), runManaged(), TestCondition_ClusterScopedListsWithoutNamespace(), TestCondition_ConditionStatus(), TestCondition_InvalidAPIVersionErrors(), TestCondition_InvalidSelectorErrors(), TestCondition_ListErrorDescribesNamespaceScope(), TestCondition_ListNameFallsBackToKind() (+22 more)

### Community 19 - "NewRootCommand"
Cohesion: 0.23
Nodes (11): NewRootCommand(), signalContext(), TestSignalContext_PropagatesParentCancellation(), TestSignalContext_SIGINTCancels(), TestSignalContext_SIGTERMCancels(), TestSignalContext_StopReleasesContext(), TestNewRootCommand_BasicWiring(), TestNewRootCommand_HelpDoesNotErrorWithoutKubeconfig() (+3 more)

### Community 20 - "newControllerScheme"
Cohesion: 0.13
Nodes (22): TestListSelectedHealthChecks_ErrorNamesScope(), failingHealthCheckListClient, InMemoryExporter, Client, T, TestListSelectedHealthChecks_ErrorNamesScope(), attrValue(), Scheme (+14 more)

### Community 21 - "quorum-ratio-rollups/internal/adapter/declarative/podprojection_test.go"
Cohesion: 0.24
Nodes (16): NewEngine(), TestNewEngine_Validation(), validVersionSource(), validWorkloadKind(), optedInPod(), runProjection(), TestNewEngine_PodProjectionValidation(), TestPodProjection_AllInjectedPasses() (+8 more)

### Community 22 - "quorum-ratio-rollups/internal/probe/pod_test.go"
Cohesion: 0.25
Nodes (4): assertArgs(), TestPodBuildsHardenedDNSProbe(), TestPodBuildsHTTPGetArgs(), TestPodRejectsInvalidRequests()

### Community 23 - "Init"
Cohesion: 0.22
Nodes (12): Context, Init(), T, restoreGlobalProvider(), TestInit_DisabledInstallsNoopProvider(), TestInit_EnabledInstallsRecordingProvider(), Config, Init() (+4 more)

### Community 24 - "Probe/Node-Agent Version Lockstep Gate"
Cohesion: 0.40
Nodes (6): Kubernetes Test-Version Lockstep (envtest / kind / crd-ref-docs), Fathom Release History (Changelog), Conventional Commits Policy, Why Lockstep Is Automated: Human-Gated Contract Failed for 0.3.0/0.3.1 (SKA-579), Release Please Flow, Probe/Node-Agent Version Lockstep Gate

### Community 25 - "quorum-ratio-rollups/internal/controller/healthreport_idempotency.go"
Cohesion: 0.60
Nodes (4): createOrReuseHealthReport(), deterministicHealthReportName(), useDeterministicHealthReportName(), validateReusableHealthReport()

### Community 26 - "quorum-ratio-rollups/scripts/version_lockstep_gate_test.go"
Cohesion: 0.83
Nodes (3): runLockstep(), TestVersionLockstepDetectsDrift(), TestVersionLockstepInSync()

### Community 51 - "CheckResult"
Cohesion: 0.06
Nodes (65): CheckResult, dryRunCertificate(), dryRunCreate(), dryRunIssuer(), familyEnabled(), includesKind(), mutatingWebhookClients(), restartWarnCount() (+57 more)

### Community 52 - "internal/adapter/certmanager/adapter_test.go"
Cohesion: 0.11
Nodes (63): clientObject, New(), assertFamily(), assertHasDetail(), assertHasOutcome(), assertNoKind(), assertNoOutcome(), certManagerResource() (+55 more)

### Community 53 - "cmd/probe/main_test.go"
Cohesion: 0.10
Nodes (59): Context, result, Reader, join(), lookupCNAME(), lookupIPs(), lookupSRV(), main() (+51 more)

### Community 54 - ".detectAndGateVersion"
Cohesion: 0.06
Nodes (52): CRDCheck, FamilyDefinition, Posture, versionAddress, VersionSource, WorkloadKind, AnnotationStalenessCheck, ConditionCheck (+44 more)

### Community 55 - ".Run"
Cohesion: 0.12
Nodes (54): clientObject, fakeLauncher, New(), adapterWithLauncher(), assertHasDetail(), assertHasOutcome(), assertNoOutcome(), dnsEndpointSlice() (+46 more)

### Community 56 - ".Reconcile"
Cohesion: 0.09
Nodes (39): NodeCertificateCheckReconciler, nodeCertRollupDecision, TestNodeCertCadenceHelpersClamp(), admissionPolicyUnsupported(), checkForReportConfigMap(), clearNodeCertRollupStatus(), decideNodeCertRollup(), Client (+31 more)

### Community 57 - "TargetRef"
Cohesion: 0.07
Nodes (38): boundedNodeList(), TargetRef, TestBoundedNodeListCapsAtMax(), podTarget(), adapterOutcome(), boundedNodeList(), check(), dnsProbePodName() (+30 more)

### Community 58 - "assertHasDetail"
Cohesion: 0.12
Nodes (42): assertFamily(), assertHasDetail(), NewAzureWorkloadIdentityEngine(), healthyAzureWIObjects(), TestAzureWorkloadIdentity_Capabilities(), TestAzureWorkloadIdentity_HealthyClusterAllPass(), TestAzureWorkloadIdentity_NoOptedInPodsProjectionSkipped(), TestAzureWorkloadIdentity_UnpopulatedCABundleFails() (+34 more)

### Community 59 - ".Run"
Cohesion: 0.17
Nodes (42): New(), adapterWithLauncher(), assertHasDetail(), assertHasOutcome(), assertNoTarget(), daemonSetWithStatus(), Adapter, Client (+34 more)

### Community 60 - ".checkMetricsEndpoint"
Cohesion: 0.10
Nodes (34): endpointTarget(), Result, scrapeProbePodName(), servicePortDeclared(), servicePortList(), adapterOutcome(), csvThreshold(), deploymentAvailable() (+26 more)

### Community 61 - "newFakeClient"
Cohesion: 0.12
Nodes (39): clientObject, Engine, NewCiliumEngine(), TestDescheduler_HealthyDeploymentMode(), assertFamily(), assertNoKind(), ciliumCRDNames(), daemonSetInNamespace() (+31 more)

### Community 62 - "assertHasDetail"
Cohesion: 0.14
Nodes (41): Engine, NewAzureWorkloadIdentityEngine(), clientObject, T, healthyAzureWIObjects(), TestAzureWorkloadIdentity_AbsentWebhookFails(), TestAzureWorkloadIdentity_Capabilities(), TestAzureWorkloadIdentity_HealthyClusterAllPass() (+33 more)

### Community 63 - "FamilyPolicy"
Cohesion: 0.10
Nodes (31): FamilyPolicy, Adapter, dnsProbeLauncher, adapterOutcome(), check(), deploymentAvailable(), dnsProbePodName(), dnsTargets() (+23 more)

### Community 64 - "properties"
Cohesion: 0.05
Nodes (39): type, type, type, type, type, type, type, type (+31 more)

### Community 65 - ".Evaluate"
Cohesion: 0.10
Nodes (15): policyNamespaces(), TestPolicyNamespaceResolution(), AnnotationStalenessCheck, Evaluator, durationThreshold(), isFutureTimestamp(), namespaceScope(), AnnotationStalenessCheck (+7 more)

### Community 66 - "Family"
Cohesion: 0.09
Nodes (28): Family, Outcome, Request, ThresholdAdvertiser, fakeAdvertisingAdapter, endRunSpan(), resolveFamily(), endRunSpan() (+20 more)

### Community 67 - "deploymentInNamespace"
Cohesion: 0.19
Nodes (33): assertNoOutcome(), deploymentInNamespace(), Deployment, Outcome, absenceEngine(), deployEngine(), failedPod(), Deployment (+25 more)

### Community 68 - "HealthReportResult"
Cohesion: 0.10
Nodes (27): Condition, LabelSelector, ListMeta, ObjectMeta, Time, TypeMeta, ClusterHealth, ClusterHealthChildSummary (+19 more)

### Community 69 - "scanAndPublish"
Cohesion: 0.12
Nodes (29): Context, Duration, Time, main(), metricsMux(), parseConfig(), publishGauges(), run() (+21 more)

### Community 70 - "quorum-ratio-rollups/test/utils/utils.go"
Cohesion: 0.11
Nodes (27): TestE2EShardPlannerKnowsEveryOptInAddon(), TestE2E(), AddonSelection, CoreAddons(), GetNonEmptyLines(), GetProjectDir(), InstallPrometheusOperator(), IsPrometheusCRDsInstalled() (+19 more)

### Community 71 - "Write"
Cohesion: 0.11
Nodes (24): main(), metricsMux(), parseConfig(), publishGauges(), run(), sanitizeLabelValue(), scanAndPublish(), splitCSV() (+16 more)

### Community 72 - "api/v1alpha1/deepcopy_test.go"
Cohesion: 0.19
Nodes (29): deepCopyContract(), fullyPopulatedAddonCheck(), fullyPopulatedClusterHealth(), fullyPopulatedDNSCheck(), fullyPopulatedHealthCheck(), fullyPopulatedHealthReport(), fullyPopulatedNodeCertificateCheck(), AddonCheck (+21 more)

### Community 73 - ".DeepCopyInto"
Cohesion: 0.08
Nodes (11): ClusterHealthSpec, DNSCheck, DNSCheckList, DNSCheckSpec, DNSCheckStatus, DNSResolver, DNSTarget, DNSTargetResult (+3 more)

### Community 74 - "common.sh"
Cohesion: 0.09
Nodes (14): check-prerequisites.sh script, check_dir(), check_file(), get_feature_paths(), get_repo_root(), has_jq(), _persist_feature_json(), resolve_specify_init_dir() (+6 more)

### Community 75 - "Implementation Plan: Pre-1.0 CRD Validation Hardening"
Cohesion: 0.07
Nodes (27): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Pre-1.0 CRD Validation Hardening, Complexity Tracking, Constitution Check, Documentation (this feature) (+19 more)

### Community 76 - "observeCheck"
Cohesion: 0.15
Nodes (27): AddonCheckStatus, FakeRecorder, TestObserveCheckEmitsCadenceClampedOncePerEpisode(), checkGaugeValue(), createAddonCheckWithStatusForObservability(), drainEvents(), gatherGaugeValue(), Condition (+19 more)

### Community 77 - "addoncheck_controller.go"
Cohesion: 0.16
Nodes (25): WorstResult(), familyRatioRollup, HealthReportCheck, HealthReportTargetRef, aggregateHealthReportResult(), aggregateWithRatioRollups(), copyStringMap(), HealthReport (+17 more)

### Community 78 - "Implementation Plan: Quorum/Ratio Semantics for Managed-Resource Rollups"
Cohesion: 0.07
Nodes (25): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Quorum/Ratio Semantics for Managed-Resource Rollups, Complexity Tracking, Constitution Check, Documentation (this feature) (+17 more)

### Community 79 - "Adversarial Review Findings — v0.5.0 Release Gate (#217)"
Cohesion: 0.07
Nodes (24): Coverage Statement — v0.5.0 Release Gate (#217), Intentionally excluded, Perspective results (SC-001), Post-anchor deltas, Reviewed, Scope notes, Adversarial Review Findings — v0.5.0 Release Gate (#217), API-1: HealthCheck status.summary MaxLength=1024 wedges mirroring on long condition messages (high) (+16 more)

### Community 80 - "BuiltInAdapters Registration"
Cohesion: 0.13
Nodes (25): Setupper, Bool, Checker, adapterName(), BuildAdapterRegistry(), BuiltInAdapters Registration, DefaultControllers(), disableHTTP2() (+17 more)

### Community 81 - "assertHasOutcome"
Cohesion: 0.25
Nodes (26): ConditionCheck, T, Unstructured, runManaged(), TestCondition_ClusterScopedListsWithoutNamespace(), TestCondition_ConditionStatus(), TestCondition_InvalidAPIVersionErrors(), TestCondition_InvalidSelectorErrors() (+18 more)

### Community 82 - "Tasks: [FEATURE NAME]"
Cohesion: 0.07
Nodes (26): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation Strategy, Incremental Delivery, MVP First (User Story 1 Only) (+18 more)

### Community 83 - "speckit-analyze/SKILL.md"
Cohesion: 0.08
Nodes (25): 1. Initialize Analysis Context, 2. Load Artifacts (Progressive Disclosure), 3. Build Semantic Models, 4. Detection Passes (Token-Efficient Analysis), 5. Severity Assignment, 6. Produce Compact Analysis Report, 7. Provide Next Actions, 8. Offer Remediation (+17 more)

### Community 84 - ".Evaluate"
Cohesion: 0.12
Nodes (16): conditionStatus(), policySelector(), resourceAbsent(), FieldCheck, defaultOutcome(), conditionStatus(), LabelSelector, Selector (+8 more)

### Community 85 - "kedaHealthyObjects"
Cohesion: 0.14
Nodes (21): Engine, WorkloadCheck, kedaDeployment(), NewKedaEngine(), conditionCR(), clientObject, T, Unstructured (+13 more)

### Community 86 - "Quickstart Validation: Quorum/Ratio Rollups"
Cohesion: 0.08
Nodes (22): Configuration surface (AddonCheck), Contract: Ratio Rollup Thresholds and Report Entries, Explicit non-changes, Metrics interplay (informative), Rejection (Accepted condition), Report surface (HealthReport), Verdict semantics, Data Model: Quorum/Ratio Semantics for Managed-Resource Rollups (+14 more)

### Community 87 - "CertResult"
Cohesion: 0.27
Nodes (21): Certificate, classify(), classifyAll(), daysFromDuration(), errorResult(), Duration, Time, parsePEMCertificates() (+13 more)

### Community 88 - ".Reconcile"
Cohesion: 0.14
Nodes (17): clearClusterHealthAggregateStatus(), clusterHealthCoversNamespace(), clusterHealthSelectsHealthCheck(), ClusterHealth, Context, HealthCheck, LabelSelector, Request (+9 more)

### Community 89 - "PolicyRule"
Cohesion: 0.13
Nodes (17): PolicyRule, T, hasResource(), hasVerb(), TestRBACRulesDeclaresDryRunException(), T, hasResource(), hasVerb() (+9 more)

### Community 90 - "RatioThresholds"
Cohesion: 0.18
Nodes (18): RatioPercent, RatioRollup, RatioThresholds, ratioThresholdsByFamily(), FamilyRatioVerdict(), Outcome, isDigits(), parseRatioPercent() (+10 more)

### Community 91 - "Command"
Cohesion: 0.18
Nodes (19): Cmd, Command, eventRow, getMetricsOutput(), serviceAccountToken(), addonCheckLastResult(), addonCheckReadyTrue(), dumpAddonCheckDiagnostics() (+11 more)

### Community 92 - "MustEngine"
Cohesion: 0.15
Nodes (19): CronJob, AddonDefinition, crdAbsenceEngine(), Engine, T, TestCRD_AbsenceResolution(), cronJob(), CronJobCheck (+11 more)

### Community 93 - "join"
Cohesion: 0.17
Nodes (19): serviceAccountToken(), join(), applyManifest(), scrapeOperatorMetrics(), newTestFlags(), TestDefaultOptions_MatchFlagDefaults(), TestLoad_ConfigOverridesDefault(), TestLoad_EnvOverridesConfig() (+11 more)

### Community 94 - "test/utils/utils_test.go"
Cohesion: 0.14
Nodes (20): T, TestE2EShardPlannerKnowsEveryOptInAddon(), T, TestE2E(), CoreAddons(), OptInAddons(), ParseAddonSelection(), T (+12 more)

### Community 95 - "internal/adapter/rbacgen/rbacgen.go"
Cohesion: 0.19
Nodes (19): T, TestFilesRejectsIncompleteRule(), clusterRules(), Collect(), Files(), Adapter, groupsCell(), marshalDocs() (+11 more)

### Community 96 - "Tasks: Adversarial Codebase Review for the v0.5.0 Release Gate"
Cohesion: 0.09
Nodes (21): Consolidation and refutation, Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation Strategy, Incremental Delivery, MVP First (US1 only), Notes, Parallel Example: User Story 1 (+13 more)

### Community 97 - "Registry"
Cohesion: 0.15
Nodes (16): version, Rationale: in-process interface over gRPC/OCI/plugin loaders, CRD API Versioning Standard, CRD Maturity Ladder (alpha/beta/GA), Adapter, Logger, ContractVersion Constant, EnsureCompatible SemVer Handshake (+8 more)

### Community 98 - "NewScheme (scheme registration)"
Cohesion: 0.25
Nodes (20): CertWatcher, DefaultOptions(), BuildManagerOptions(), Scheme, NewScheme (scheme registration), T, TestBuildAdapterRegistry_RegistersBuiltInAdapters(), TestBuildAdapterRegistry_WrapsRegistrationErrors() (+12 more)

### Community 99 - "HealthCheckReconciler"
Cohesion: 0.12
Nodes (16): HealthCheckReconciler, clearMirroredHealthCheckStatus(), Client, Context, EventRecorder, HealthCheck, Manager, Object (+8 more)

### Community 100 - "SetRunningInClusterForTest"
Cohesion: 0.12
Nodes (16): defaultRunningInCluster(), inClusterFromConfigErr(), T, TestInClusterFromConfigErr(), RunningInCluster(), SetRunningInClusterForTest(), T, TestRunningInCluster_TestOverride() (+8 more)

### Community 101 - "internal/controller/nodecertificatecheck_helpers.go"
Cohesion: 0.13
Nodes (19): aggregateNodeReports(), controlPlaneTolerations(), Client, Context, HealthReport, Logger, Outcome, Time (+11 more)

### Community 102 - "newFakeClient"
Cohesion: 0.30
Nodes (20): dnsRequest(), Client, PodPhase, Request, T, newFakeClient(), simulateKubelet(), TestLauncherRun_ConcurrentRunsAreIndependent() (+12 more)

### Community 103 - "TestAdapterClient"
Cohesion: 0.13
Nodes (13): addonSA(), TestAdapterClient(), TestRunAddonCheckFailsClosedWhenNamespaceEmptyInCluster(), TestRunAddonCheckFailsClosedWithoutScopedClient(), fakeClientFactory, TestDefaultControllers_InClusterRequiresNamespace(), defaultRunningInCluster(), inClusterFromConfigErr() (+5 more)

### Community 104 - ".Reconcile"
Cohesion: 0.18
Nodes (16): addonAdapterLookup, addonCheckDueForRun(), addonCheckInterval(), addonCheckTargetRef(), addonCheckTimeout(), Adapter, AddonCheck, Context (+8 more)

### Community 105 - "image"
Cohesion: 0.11
Nodes (20): properties, required, type, properties, required, type, image, probeImage (+12 more)

### Community 106 - "Load"
Cohesion: 0.31
Nodes (19): Load(), FlagSet, T, newTestFlags(), TestDefaultOptions_MatchFlagDefaults(), TestLoad_ConfigOverridesDefault(), TestLoad_EnvOverridesConfig(), TestLoad_FlagOverridesEverything() (+11 more)

### Community 107 - "internal/nodecert/scan_test.go"
Cohesion: 0.28
Nodes (19): Scan(), T, Time, makeCertPEM(), TestClassifyBoundaries(), TestDaysFromDuration(), TestEncodeDecodeReportRoundTrip(), TestMinimalMountDirs() (+11 more)

### Community 108 - "runArgoCD"
Cohesion: 0.20
Nodes (17): argocdDeployment(), Engine, WorkloadCheck, NewArgoCDEngine(), argoApp(), argocdHealthyObjects(), clientObject, Result (+9 more)

### Community 109 - "internal/adapter/declarative/istio_test.go"
Cohesion: 0.27
Nodes (17): Engine, NewIstioEngine(), clientObject, T, istioAmbientObjects(), istioCRDObjects(), istiodControlPlane(), istioHealthyObjects() (+9 more)

### Community 110 - "New"
Cohesion: 0.23
Nodes (14): New(), Context, Request, Result, T, newFake(), TestCapabilities(), TestConcurrentAccess() (+6 more)

### Community 111 - "internal/probe/sweeper_test.go"
Cohesion: 0.27
Nodes (18): Duration, PodPhase, Scheme, T, newScheme(), probeLabels(), probeShape(), sweepPod() (+10 more)

### Community 112 - "NewRootCommand"
Cohesion: 0.22
Nodes (15): CancelFunc, Context, NewRootCommand(), signalContext(), T, TestSignalContext_PropagatesParentCancellation(), TestSignalContext_SIGINTCancels(), TestSignalContext_SIGTERMCancels() (+7 more)

### Community 113 - "kedaHealthyObjects"
Cohesion: 0.20
Nodes (12): NewKedaEngine(), conditionCR(), kedaHealthyObjects(), TestKeda_AbsentClusterAllSkipped(), TestKeda_HealthyWithReadyScaledObject(), TestKeda_PausedScaledObjectWarns(), TestKeda_UnreadyScaledObjectFails(), NewVpaEngine() (+4 more)

### Community 114 - "Feature Specification: DNSCheck Resource Contract"
Cohesion: 0.11
Nodes (18): Assumptions, Clarifications, Dependencies, Edge Cases, Feature Specification: DNSCheck Resource Contract, Functional Requirements, Key Entities, Measurable Outcomes (+10 more)

### Community 115 - "dnscheck_types.go"
Cohesion: 0.19
Nodes (15): Condition, Duration, ListMeta, ObjectMeta, Time, TypeMeta, DNSCheck, DNSCheckList (+7 more)

### Community 116 - "Options"
Cohesion: 0.18
Nodes (14): flagBinding, MetricsOptions, Options, TracingOptions, WebhookOptions, factory, Client, Mutex (+6 more)

### Community 117 - ".podInjected"
Cohesion: 0.16
Nodes (12): PodProjectionCheck, capNames(), containerHasEnv(), formatSelector(), Container, PodProjectionCheck, hasProjectedTokenVolume(), capNames() (+4 more)

### Community 118 - "properties"
Cohesion: 0.12
Nodes (17): type, type, type, type, type, allOf, $comment, properties (+9 more)

### Community 119 - "test/e2e/healthreport_helpers_test.go"
Cohesion: 0.20
Nodes (12): checkResult, eventList, healthReport, healthReportList, addonCheckLastResult(), addonCheckReadyTrue(), dumpAddonCheckDiagnostics(), latestHealthReport() (+4 more)

### Community 120 - "runConfigMap"
Cohesion: 0.21
Nodes (14): configMap(), ConfigMap, ConfigMapCheck, T, runConfigMap(), TestConfigMapCheck(), TestConfigMapCheck_AbsentInheritsOptional(), TestConfigMapCheck_NoAPIVersionAssertionPassesAnyYAML() (+6 more)

### Community 121 - "Sweeper"
Cohesion: 0.16
Nodes (13): Client, Context, Duration, Logger, PodPhase, Reader, Selector, Time (+5 more)

### Community 122 - "Core Principles"
Cohesion: 0.12
Nodes (16): Core Principles, Development Workflow & Quality Gates, Engineering Constraints, Fathom Constitution, Fathom-Specific Constraints, Governance, I. Explicit State Over Implicit Behavior, II. Git Is the Durable Desired-State Boundary (+8 more)

### Community 123 - "Fathom Architecture"
Cohesion: 0.28
Nodes (16): Adapter, ADR-0001 In-process Adapter Contract, ADR-0002 HealthReport as First-class CRD, ADR-0003 Probe-pod Model, Rationale: representative network topology without a DaemonSet, ADR-0004 HealthCheck as Thin Wrapper, Rationale: uniform wrapper preserves aggregator contract, ClusterHealth-from-HealthCheck.status-only Invariant (+8 more)

### Community 124 - "addoncheck_types.go"
Cohesion: 0.17
Nodes (14): Condition, Duration, LabelSelector, ListMeta, ObjectMeta, Time, TypeMeta, AddonCheck (+6 more)

### Community 125 - "dnscheck_validation_test.go"
Cohesion: 0.23
Nodes (15): duration(), envTestAssetsAvailable(), firstEnvTestBinaryDir(), DNSCheck, Duration, M, T, requireAPIServer() (+7 more)

### Community 126 - "Execution Steps"
Cohesion: 0.12
Nodes (15): 1. Initialize Convergence Context, 2. Load Artifacts (Progressive Disclosure), 3. Build the Intent Inventory, 4. Assess the Codebase and Classify Findings, 5. Assign Severity, 6. Present the In-Session Findings Summary, 7. Append Convergence Tasks (or report converged), 8. Provide Next Actions (Handoff) (+7 more)

### Community 127 - "AddonCheckReconciler"
Cohesion: 0.23
Nodes (14): AddonCheckReconciler, Absence Semantics (Required/Optional, MarkAbsent), Declarative Adapter Engine (MustEngine), Authoring an Adapter Guide, Version Detection and SupportedVersions Gating, Rationale: five shaping decisions (declarative-first epic), Addon Adapters Implementation Plan (v2), Per-addon Least-Privilege ServiceAccount Impersonation (+6 more)

### Community 128 - "internal/nodecert/paths.go"
Cohesion: 0.22
Nodes (14): resolveCertPaths(), TestResolveCertPathsFiltersDisallowed(), AllowedPathPrefixes(), DefaultCertPaths(), FilterAllowedPaths(), isCertFile(), isKubeconfigFile(), MinimalMountDirs() (+6 more)

### Community 129 - "internal/metrics/check_metrics_test.go"
Cohesion: 0.28
Nodes (13): ctrlRegistryGather(), gatherCheckSeries(), gatherOneHot(), T, TestCheckResultValuesMatchAPIVocabulary(), TestDeleteCheckSeries(), TestObserveCheckFlipsResult(), TestObserveCheckOneHotInvariant() (+5 more)

### Community 130 - "Probe Pod Manifest Builder Pod(Request)"
Cohesion: 0.34
Nodes (15): Probe Pod Manifest Builder Pod(Request), assertArgs(), T, TestParseResult(), TestPodBuildsDNSAssertionArgs(), TestPodBuildsHardenedDNSProbe(), TestPodBuildsHTTPGetArgs(), TestPodLeavesDNSPolicyAloneByDefault() (+7 more)

### Community 131 - "quorum-ratio-rollups/internal/nodecert/paths.go"
Cohesion: 0.19
Nodes (14): resolveCertPaths(), TestResolveCertPathsFiltersDisallowed(), AllowedPathPrefixes(), DefaultCertPaths(), FilterAllowedPaths(), isCertFile(), isKubeconfigFile(), MinimalMountDirs() (+6 more)

### Community 132 - "Tasks: Pre-1.0 CRD Validation Hardening"
Cohesion: 0.12
Nodes (16): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Parallel Example: User Story 1, Parallel Opportunities, Phase 1: Setup (+8 more)

### Community 133 - "TestCommittedAddonRolesAreReadOnly"
Cohesion: 0.18
Nodes (12): RBACDeclarer, allowedWrites(), repoRoot(), TestCommittedAddonRolesAreReadOnly(), TestModelGrantsAreJustified(), TestUnjustifiedGrantsCatchesViolations(), AddonServiceAccountName(), IsReadVerb() (+4 more)

### Community 134 - "establishedCRD"
Cohesion: 0.25
Nodes (12): establishedCRD(), NewArgoCDEngine(), argoApp(), argocdHealthyObjects(), runArgoCD(), TestArgoCD_AbsentClusterFails(), TestArgoCD_ApplicationStateRollup(), TestArgoCD_HealthyWithSyncedApplication() (+4 more)

### Community 135 - "pod.go"
Cohesion: 0.19
Nodes (14): Affinity, antiAffinity(), args(), boolPtr(), copyStringMap(), Duration, Result, Toleration (+6 more)

### Community 136 - "operator_rbac_doc_test.go"
Cohesion: 0.25
Nodes (13): ClusterRole, docRow, equalStrings(), T, loadJustificationRows(), loadOperatorClusterRole(), normalizeSet(), ruleKey() (+5 more)

### Community 137 - ".Name"
Cohesion: 0.24
Nodes (8): fakePolicyAdapter, TestEngine_Metadata(), TestEnvoyGateway_AdapterMetadata(), TestExternalDNS_AdapterMetadata(), TestIstio_AdapterMetadata(), TestMetricsServer_AdapterMetadata(), Collect(), getFirstFoundEnvTestBinaryDir()

### Community 138 - ".Run"
Cohesion: 0.20
Nodes (9): extractResult(), Client, Context, Duration, Request, Result, hasTerminationMessage(), Launcher (+1 more)

### Community 139 - "quorum-ratio-rollups/internal/nodecert/scan.go"
Cohesion: 0.32
Nodes (14): minimalKubeconfig, classify(), classifyAll(), daysFromDuration(), errorResult(), parsePEMCertificates(), scanCertFile(), scanDir() (+6 more)

### Community 140 - "Tasks: DNSCheck Resource Contract"
Cohesion: 0.13
Nodes (15): Dependencies, Format: `[ID] [P?] [Story] Description`, Implementation strategy, Parallel opportunities, Path Conventions, Phase 1: Setup, Phase 2: Foundational (Blocking Prerequisites), Phase 4: User Story 2 — Every declared expectation is evaluated (P1) (+7 more)

### Community 141 - "validateAddonCheckPolicy"
Cohesion: 0.27
Nodes (13): AddonCheckFamilyPolicy, unknownThresholdKeys(), validateAddonCheckPolicy(), badSelector(), checkWithPolicy(), AddonCheck, LabelSelector, T (+5 more)

### Community 142 - "PreferredServedVersion"
Cohesion: 0.23
Nodes (12): CustomResourceDefinitionConditionType, Established(), CustomResourceDefinition, PreferredServedVersion(), crd(), crdWithServed(), ConditionStatus, CustomResourceDefinition (+4 more)

### Community 143 - "rbac"
Cohesion: 0.14
Nodes (14): type, type, type, annotations, create, name, rbac, serviceAccount (+6 more)

### Community 144 - "enabled"
Cohesion: 0.14
Nodes (14): properties, type, type, type, maximum, minimum, type, config (+6 more)

### Community 145 - "TestClientForSetsImpersonationAndMemoizes"
Cohesion: 0.19
Nodes (10): SAUsername(), TestClientForSetsImpersonationAndMemoizes(), TestSAUsername(), ClientFactory, Manager, New(), SAUsername(), T (+2 more)

### Community 146 - "internal/adapter/declarative/annotation_test.go"
Cohesion: 0.26
Nodes (13): daemonSetWithAnnotations(), AnnotationStalenessCheck, DaemonSet, Node, T, Time, lockCheck(), lockJSON() (+5 more)

### Community 147 - "internal/adapter/declarative/field_test.go"
Cohesion: 0.37
Nodes (13): gauge(), gaugeCheck(), FieldCheck, T, Unstructured, runFields(), TestField_InvalidSelectorErrors(), TestField_ListedObjectsScored() (+5 more)

### Community 148 - "NewMetricsServerEngine"
Cohesion: 0.30
Nodes (12): Engine, NewMetricsServerEngine(), apiService(), clientObject, T, Unstructured, msHealthyObjects(), TestMetricsServer_AdapterMetadata() (+4 more)

### Community 149 - "runProjection"
Cohesion: 0.43
Nodes (13): PodProjectionCheck, T, optedInPod(), runProjection(), TestNewEngine_PodProjectionValidation(), TestPodProjection_AllInjectedPasses(), TestPodProjection_CapNames(), TestPodProjection_InactivePodsSkipped() (+5 more)

### Community 150 - "writeFile"
Cohesion: 0.35
Nodes (13): Scan(), makeCertPEM(), TestScanBundleEmitsPerCert(), TestScanDefaultsWhenNoPaths(), TestScanDirectoryRecursiveAndIgnoresNonCerts(), TestScanKubeconfig(), TestScanMissingPathIsSilent(), TestScanPermissionDeniedIsSkippedNotError() (+5 more)

### Community 151 - "crd_compat_gate_test.go"
Cohesion: 0.53
Nodes (13): fixtureCRD(), T, runCRDCompat(), TestCRDCompatAddedOptionalFieldPasses(), TestCRDCompatAgainstBaseline(), TestCRDCompatAllowlistedChangePassesVisibly(), TestCRDCompatMalformedAllowlistFails(), TestCRDCompatNewCRDSkipped() (+5 more)

### Community 152 - "Entity: `DNSCheck`"
Cohesion: 0.14
Nodes (14): Changes to existing types, `cmd/probe` — dns mode flags, `DNSCheckSpec`, `DNSCheckStatus`, `DNSResolver`, `DNSTarget`, `DNSTargetResult`, Entity: `DNSCheck` (+6 more)

### Community 153 - "quorum-ratio-rollups/internal/probe/sweeper_test.go"
Cohesion: 0.32
Nodes (11): probeLabels(), probeShape(), sweepPod(), terminatedAt(), TestSweeper_LabelledPodNotMatchingProbeShapeIsSpared(), TestSweeper_LongRunningProbeIsNotReapedOnCreationAge(), TestSweeper_ReapsPodTerminatedLongerThanMinAge(), TestSweeper_StartSweepsImmediatelyAndStopsOnCancel() (+3 more)

### Community 154 - "NodeCertificateCheck"
Cohesion: 0.21
Nodes (11): Condition, Duration, ListMeta, ObjectMeta, Time, Toleration, TypeMeta, NodeCertificateCheck (+3 more)

### Community 155 - "Repository Guidelines"
Cohesion: 0.15
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 156 - "values.schema.json"
Cohesion: 0.15
Nodes (12): properties, required, type, nodeAgent, required, $schema, title, type (+4 more)

### Community 157 - "properties"
Cohesion: 0.15
Nodes (13): type, type, type, interval, labels, namespace, scrapeTimeout, serviceMonitor (+5 more)

### Community 158 - "TestEnvoyGateway_HealthyAndNoGatewaysSkipped"
Cohesion: 0.26
Nodes (11): Engine, NewEnvoyGatewayEngine(), egHealthyObjects(), gatewayObject(), clientObject, T, Unstructured, TestEnvoyGateway_AdapterMetadata() (+3 more)

### Community 159 - "Feature Specification: [FEATURE NAME]"
Cohesion: 0.15
Nodes (12): Assumptions, Edge Cases, Feature Specification: [FEATURE NAME], Functional Requirements, Key Entities *(include if feature involves data)*, Measurable Outcomes, Requirements *(mandatory)*, Success Criteria *(mandatory)* (+4 more)

### Community 160 - "internal/controller/addoncheck_controller_test.go"
Cohesion: 0.20
Nodes (8): healthReportCount(), countingStatusClient, countingStatusWriter, Client, NamespacedName, Object, SubResourceWriter, healthReportCount()

### Community 161 - "healthcheck_types.go"
Cohesion: 0.24
Nodes (10): Condition, ListMeta, ObjectMeta, Time, TypeMeta, CheckTargetRef, HealthCheck, HealthCheckList (+2 more)

### Community 162 - "Object"
Cohesion: 0.23
Nodes (6): Object, HealthReportList, NodeCertificateCheckList, TestDeepCopyRoundTrip(), HealthReportList, NodeCertificateCheckList

### Community 163 - "GitHub Copilot Instructions for Fathom"
Cohesion: 0.17
Nodes (11): Codebase Shape, Commit and Branch Guidance, Documentation Expectations, GitHub Copilot Instructions for Fathom, Go and Repository Conventions, Knowledge Graph (`graphify-out/`), Pull Request Instructions, Safety Rules (+3 more)

### Community 164 - "NewExternalDNSEngine"
Cohesion: 0.33
Nodes (10): Engine, NewExternalDNSEngine(), extdnsHealthyObjects(), clientObject, T, TestExternalDNS_AdapterMetadata(), TestExternalDNS_DeploymentNameThresholdOverride(), TestExternalDNS_HealthyPassesAllFamilies() (+2 more)

### Community 165 - "writeNodeReportForCheck"
Cohesion: 0.36
Nodes (11): Context, NamespacedName, NodeCertificateCheck, Time, newNodeCertReconciler(), nodeCertHealthReportCount(), setNodeAgentDaemonSetStatus(), setNodeAgentDaemonSetStatusFull() (+3 more)

### Community 166 - "Tasks: Quorum/Ratio Semantics for Managed-Resource Rollups"
Cohesion: 0.17
Nodes (11): Dependencies, Format: `[ID] [P?] [Story] Description`, Implementation strategy, Parallel execution examples, Phase 1: Setup, Phase 2: Foundational (blocking prerequisites), Phase 3: User Story 1 — Isolated failures stop redding the fleet verdict (P1) 🎯 MVP, Phase 4: User Story 2 — Graduated escalation between Warn and Fail (P2) (+3 more)

### Community 167 - "Feature Specification: Adversarial Codebase Review for the v0.5.0 Release Gate"
Cohesion: 0.17
Nodes (12): Assumptions, Edge Cases, Feature Specification: Adversarial Codebase Review for the v0.5.0 Release Gate, Functional Requirements, Key Entities, Measurable Outcomes, Requirements *(mandatory)*, Success Criteria *(mandatory)* (+4 more)

### Community 168 - "speckit-plan/SKILL.md"
Cohesion: 0.18
Nodes (10): Completion Report, Done When, Key rules, Mandatory Post-Execution Hooks, Outline, Phase 0: Outline & Research, Phase 1: Design & Contracts, Phases (+2 more)

### Community 169 - "speckit-specify/SKILL.md"
Cohesion: 0.18
Nodes (10): Completion Report, Done When, For AI Generation, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, Quick Guidelines, Section Requirements (+2 more)

### Community 170 - "speckit-tasks/SKILL.md"
Cohesion: 0.18
Nodes (10): Checklist Format (REQUIRED), Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Phase Structure, Pre-Execution Checks, Task Generation Rules (+2 more)

### Community 171 - "test/e2e/nodecert_test.go"
Cohesion: 0.24
Nodes (8): dsRollout, nodeCertStatusView, daemonSetRollout(), dumpNodeCertDiagnostics(), nodeCertStatus(), daemonSetRollout(), dumpNodeCertDiagnostics(), nodeCertStatus()

### Community 172 - "TestExternalSecrets_HealthyAndEmptySyncSkipped"
Cohesion: 0.25
Nodes (9): esoDeployment(), Engine, WorkloadCheck, NewExternalSecretsEngine(), esoHealthyObjects(), clientObject, T, TestExternalSecrets_HealthyAndEmptySyncSkipped() (+1 more)

### Community 173 - "cadence_test.go"
Cohesion: 0.35
Nodes (10): acceptedCondition(), durp(), findCondition(), Condition, Duration, T, TestAddonCheckCadenceHelpersClamp(), TestCadenceClampMessages() (+2 more)

### Community 174 - "Core Principles"
Cohesion: 0.18
Nodes (10): Core Principles, Governance, [PRINCIPLE_1_NAME], [PRINCIPLE_2_NAME], [PRINCIPLE_3_NAME], [PRINCIPLE_4_NAME], [PRINCIPLE_5_NAME], [PROJECT_NAME] Constitution (+2 more)

### Community 175 - "Changed: AddonCheck (`fathom.skaphos.io/v1alpha1`, namespaced)"
Cohesion: 0.18
Nodes (11): Changed: AddonCheck (`fathom.skaphos.io/v1alpha1`, namespaced), Changed: NodeCertificateCheck (`fathom.skaphos.io/v1alpha1`, cluster-scoped), Changed: status semantics (no schema change), Data Model: Pre-1.0 CRD Validation Hardening, New: API constants (`api/v1alpha1`), New: `.crd-compat-allowlist.yaml` (repo root, committed), `spec.policy[*].labelSelector` (`*metav1.LabelSelector`), `spec.policy` (`map[string]AddonCheckFamilyPolicy`) (+3 more)

### Community 176 - "Research: Pre-1.0 CRD Validation Hardening"
Cohesion: 0.18
Nodes (11): R10. Documentation deltas, R1. Admission mechanism: CRD-embedded CEL, no VAP, no webhook, R2. Floor rules and where the constants live, R3. Runtime clamp and its observability, R4. Policy map bounds and family-key format, R5. Numeric threshold validation in CEL, R6. Label-selector structural CEL, with controller backstop, R7. Schema-compat gate: crdify, pinned, against the latest release tag (+3 more)

### Community 177 - "API / CRD Contract-Stability Candidates — commit cb845dd"
Cohesion: 0.18
Nodes (10): API-1: HealthCheck.status.summary MaxLength=1024 can wedge mirroring on long condition messages (high), API-2: #241 retroactively reserves `warnRatio`/`failRatio` from the adapter-owned thresholds namespace without a ContractVersion bump (high), API-3: Top-level `spec` is optional on every CRD, so the CEL immutability contracts are bypassable via remove-then-re-add (medium), API-4: AddonCheck `status.conditions` is an atomic list — missing `listType=map` / patch-merge markers, inconsistent with every sibling kind (medium), API-5: CheckTargetRef schema docs advertise target kinds the controller rejects — including NodeCertificateCheck, which therefore cannot reach ClusterHealth (medium), API-6: `checkRef.apiVersion` is unbounded and silently ignored by the controller (medium), API-7: `timeout <= interval` CEL invariant silently unenforced when interval is unset (low), API-8: Ratio-threshold field doc misattributes range validation to the adapter; out-of-range value disables the entire check (low) (+2 more)

### Community 178 - "Correctness / Reconcile-Time Review — Candidates"
Cohesion: 0.18
Nodes (10): COR-1: Declarative engine silently truncates a Run on ctx expiry between families, persisting a wrong verdict from partial data (high), COR-2: NodeCertificateCheck ensure-failure paths set Ready=False in memory but never persist it — persistent provisioning failure leaves stale Ready=True in status forever (medium), COR-3: Any incomplete report window (DaemonSet rollout, node join, agent restart) wipes the NodeCertificateCheck's last-known verdict instead of preserving it (medium), COR-4: nodeCertReportsComplete lets a departed node's leftover report stand in for a new node's missing one — rollup claims complete coverage while a live node was never scanned (medium), COR-5: Status-update conflict after an adapter run causes a full re-run (including probe pods) on the immediate retry (low), COR-6: History pruning can delete the just-created report on same-second CreationTimestamp ties, dangling Status.LastReportName (low), COR-7: nodeCertReportFresh accepts reports timestamped up to maxAge in the future — a clock-skewed node's report stays "fresh" for 2×maxAge (low), COR-8: HealthCheck watch mapping swallows List errors silently — a mirrored status can go stale with zero trace (low) (+2 more)

### Community 179 - "Implementation Plan: DNSCheck Resource Contract"
Cohesion: 0.18
Nodes (11): Complexity Tracking, Constitution Check, Decisions resolved during planning, Documentation (this feature), Implementation Plan: DNSCheck Resource Contract, Implementation sequence, Project Structure, Risks (+3 more)

### Community 180 - "Phase 0 Research: DNSCheck Resource Contract"
Cohesion: 0.18
Nodes (10): Phase 0 Research: DNSCheck Resource Contract, R1 — How to evaluate the record kinds, R2 — How the three resolver vantage points are realized, R3 — Outcome mapping, including negative assertions, R4 — Keeping CEL inside the cost budget, R5 — Subject syntax validation, R6 — How many probe pods a run costs, and the bounds that follow, R7 — Where answer-matching and polarity are evaluated (+2 more)

### Community 181 - "RecordReconcile"
Cohesion: 0.31
Nodes (9): Duration, RecordReconcile(), T, TestAdapterMetrics(), TestMetricsAreValidCollectors(), TestMetricsCanBeUsedFromOtherPackages(), TestReconcileMetrics(), TestRecordAdapterRunHelper() (+1 more)

### Community 182 - "RBAC / Least-Privilege Review — Fathom operator (commit cb845dd)"
Cohesion: 0.20
Nodes (9): Positive observations (examined, no defect), RBAC-1: Operator ClusterRole grants create/update/patch/delete on the three primary CRDs the reconcilers never write (medium), RBAC-2: Runtime node-agent ClusterRole allows get/update of ANY ConfigMap in the operator namespace, though the agent writes exactly one (medium), RBAC-3: Node-agent NetworkPolicy egress permits TCP/443 + TCP/6443 to ANY destination, not just the API server (medium), RBAC-4: Operator holds cluster-wide create/get/list/update/watch on ClusterRoles and RoleBindings with no resourceNames (high), RBAC-5: Operator holds cluster-wide apps/daemonsets create/update/delete — a DaemonSet-on-every-node takeover primitive (high), RBAC-6: Impersonated addon ServiceAccounts hold cluster-wide pods create/delete, granting the operator (via impersonation) a pod-create capability its own role lacks (medium), RBAC-7: Unused finalizer subresource grants across all four reconcilers (low) (+1 more)

### Community 184 - ".IsReadOnly"
Cohesion: 0.50
Nodes (4): hasResource(), hasVerb(), TestRBACRulesDeclaresDryRunException(), TestRBACRulesDeclaresProbeException()

### Community 185 - "ClusterHealthReconciler"
Cohesion: 0.25
Nodes (7): ClusterHealthReconciler, EventHandler, Client, EventRecorder, Manager, Scheme, Tracer

### Community 186 - "TestCommittedAddonRolesAreReadOnly"
Cohesion: 0.42
Nodes (8): allowedWrites(), T, repoRoot(), TestCommittedAddonRolesAreReadOnly(), TestModelGrantsAreJustified(), TestUnjustifiedGrantsCatchesViolations(), UnjustifiedGrants(), writeKey

### Community 187 - "msHealthyObjects"
Cohesion: 0.47
Nodes (7): NewMetricsServerEngine(), apiService(), msHealthyObjects(), TestMetricsServer_HealthyPassesAllFamilies(), TestMetricsServer_MissingAPIServiceFails(), TestMetricsServer_MissingDeploymentFails(), TestMetricsServer_UnavailableAPIServiceFails()

### Community 188 - "writeNodeReportForCheck"
Cohesion: 0.31
Nodes (7): nodeCertHealthReportCount(), setNodeAgentDaemonSetStatus(), setNodeAgentDaemonSetStatusFull(), writeNodeReport(), writeNodeReportAt(), writeNodeReportForCheck(), agentResourceName()

### Community 189 - "Implementation Plan: [FEATURE]"
Cohesion: 0.22
Nodes (8): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: [FEATURE], Project Structure, Source Code (repository root), Summary, Technical Context

### Community 190 - "Entities"
Cohesion: 0.22
Nodes (9): CandidateFinding, ConfirmedFinding, CoverageStatement, Data Model: Adversarial Codebase Review for the v0.5.0 Release Gate, Disposition, Entities, Perspective, Relationships (+1 more)

### Community 191 - "Phase 0 Research: Adversarial Codebase Review for the v0.5.0 Release Gate"
Cohesion: 0.22
Nodes (9): Phase 0 Research: Adversarial Codebase Review for the v0.5.0 Release Gate, R1. Review perspective set and surface assignment, R2. Independence and refutation protocol, R3. Severity rubric, R4. Anchoring and drift handling, R5. Tool-assisted evidence, R6. Disposition workflow for confirmed findings, R7. e2e evidence for runtime-behavior fixes (+1 more)

### Community 192 - "Quickstart: Validating the DNSCheck Resource Contract"
Cohesion: 0.22
Nodes (9): 1. Regenerate and verify the generated surface, 2. Prove the CEL rules fit the cost budget — do this early, 3. Validate the admission contract (User Story 1), 4. Validate the resolution capability (User Story 2), 5. Prove the shared path did not move (FR-030), 6. Full local CI, Prerequisites, Quickstart: Validating the DNSCheck Resource Contract (+1 more)

### Community 193 - "speckit-checklist/SKILL.md"
Cohesion: 0.25
Nodes (7): Anti-Examples: What NOT To Do, Checklist Purpose: "Unit Tests for English", Example Checklist Types & Sample Items, Execution Steps, Post-Execution Checks, Pre-Execution Checks, User Input

### Community 194 - "runMain"
Cohesion: 0.43
Nodes (6): main(), T, runMain(), TestMain_BadFlagExitsNonZero(), TestMain_HelpExitsZero(), TestMain_RunsAsMainOnDemand()

### Community 195 - ".Run"
Cohesion: 0.46
Nodes (3): Context, Request, Result

### Community 196 - "runMain"
Cohesion: 0.43
Nodes (6): main(), T, runMain(), TestMain_ExitsNonZeroOnWriteError(), TestMain_RunsAsMainOnDemand(), TestMain_WritesArtifacts()

### Community 197 - "summarizeFromConditions"
Cohesion: 0.36
Nodes (6): Condition, summarizeFromConditions(), truncateSummary(), T, TestSummarizeFromConditionsBoundsLength(), TestSummarizeFromConditionsNoReadyCondition()

### Community 198 - "createOrReuseHealthReport"
Cohesion: 0.39
Nodes (7): createOrReuseHealthReport(), deterministicHealthReportName(), Client, Context, HealthReport, useDeterministicHealthReportName(), validateReusableHealthReport()

### Community 199 - "IsReadVerb"
Cohesion: 0.36
Nodes (6): AddonServiceAccountName(), IsReadVerb(), T, TestAddonServiceAccountName(), TestIsReadVerb(), TestPolicyRuleIsReadOnly()

### Community 200 - "quorum-ratio-rollups/internal/app/run_happy_test.go"
Cohesion: 0.29
Nodes (4): firstEnvtestBinaryDir(), TestMain(), TestRun_HappyPath_DefaultControllers(), TestRun_HappyPath_NoControllers()

### Community 201 - "Research: Quorum/Ratio Semantics for Managed-Resource Rollups"
Cohesion: 0.25
Nodes (7): R1: Evaluation locus — controller aggregation, helpers in `pkg/adapter`, R2: Threshold surface — reserved keys `warnRatio` / `failRatio`, R3: Verdict semantics, R4: Explainability — synthetic rollup entry in HealthReport, R5: Metrics interplay, R6: Test & e2e strategy, Research: Quorum/Ratio Semantics for Managed-Resource Rollups

### Community 202 - "Quickstart Validation: Pre-1.0 CRD Validation Hardening"
Cohesion: 0.25
Nodes (8): 1. Regenerate and verify no drift, 2. Admission floors (live cluster), 3. Policy validation (live cluster), 4. Runtime clamp (envtest-covered; live check optional), 5. Schema-compat gate, 6. Full e2e (required before PR is ready — CRD types changed), Prerequisites, Quickstart Validation: Pre-1.0 CRD Validation Hardening

### Community 203 - "Implementation Plan: Adversarial Codebase Review for the v0.5.0 Release Gate"
Cohesion: 0.25
Nodes (8): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: Adversarial Codebase Review for the v0.5.0 Release Gate, Project Structure, Source Code (repository root), Summary, Technical Context

### Community 204 - "Quickstart: Validating the v0.5.0 Adversarial Review Release Gate"
Cohesion: 0.25
Nodes (8): 1. Milestone precondition (FR-001), 2. Deliverables exist and follow the contract (FR-004, FR-009), 3. Every critical/high finding is dispositioned (FR-005, SC-002), 4. Fix quality gates (FR-006, SC-003), 5. Refutation record (FR-003), 6. Gate closure (FR-010, SC-005), Prerequisites, Quickstart: Validating the v0.5.0 Adversarial Review Release Gate

### Community 205 - "Supply-Chain / CI-Integrity Review — Fathom @ cb845dd"
Cohesion: 0.25
Nodes (7): Checked and clean (no candidate), SCM-1: SBOMs are published as release assets only — not attached to images as OCI referrers nor signed (medium), SCM-2: `release.yml` publishes a fully-signed, provenanced release from *any* `v*` tag with no guard that the commit is on `main` / came from the release flow (medium), SCM-3: release/e2e tool binaries are verified only against a checksums file fetched from the same release URL — no signature/provenance check (low), SCM-4: `checkout` never sets `persist-credentials: false`; the write-scoped token/app token stays in `.git/config` across later tool steps (low), SCM-5: coverage gate silently excludes any package whose import path contains `/e2e` as a substring, not just `test/e2e` (low), Supply-Chain / CI-Integrity Review — Fathom @ cb845dd

### Community 206 - "api/v1alpha1/groupversion_info_test.go"
Cohesion: 0.48
Nodes (6): T, TestAddToScheme(), TestDeepCopyIntoExercise(), TestDeepCopyRoundTrip(), TestGroupVersion(), TestSchemeBuilderRegisterReturnsSelf()

### Community 207 - "validation_test.go"
Cohesion: 0.48
Nodes (6): CustomResourceDefinition, T, specValidationRules(), TestEveryGeneratedCRDIsCategorised(), TestGeneratedCRDsDeclareTheFathomCategory(), TestGeneratedCRDsEmbedCadenceFloors()

### Community 208 - "speckit-clarify/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 209 - "speckit-implement/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 210 - "programmableAdapter"
Cohesion: 0.33
Nodes (3): programmableAdapter, Mutex, Outcome

### Community 211 - ".Get"
Cohesion: 0.29
Nodes (5): transientAddonCheckGetClient, Client, Context, Object, ObjectKey

### Community 212 - "PreferredServedVersion"
Cohesion: 0.43
Nodes (6): PreferredServedVersion(), crd(), crdWithServed(), TestEstablished(), TestPreferredServedVersion(), TestPreferredServedVersion_IgnoresUnservedEntries()

### Community 214 - "noMatchListClient"
Cohesion: 0.38
Nodes (5): failingListClient, noMatchListClient, Client, Context, ObjectList

### Community 215 - "webhookEntry"
Cohesion: 0.33
Nodes (5): webhookEntry, appendEntry(), WebhookClientConfig, ServiceReference, appendEntry()

### Community 216 - "Operator RBAC"
Cohesion: 0.29
Nodes (6): Auxiliary roles shipped alongside the operator, Namespace-scoping analysis, Operator ClusterRole rules, Operator RBAC, Runtime-created RBAC, Why these grants are cluster-scoped

### Community 217 - "nodeAgentTemplateNeedsWrite"
Cohesion: 0.24
Nodes (5): nodeAgentTemplateNeedsWrite(), T, TestNodeAgentTemplateNeedsWrite(), T, TestDecideNodeCertRollup()

### Community 218 - "runMain"
Cohesion: 0.48
Nodes (5): runMain(), TestMain_BadFlagExitsNonZero(), TestMain_ExitsNonZeroOnWriteError(), TestMain_HelpExitsZero(), TestMain_WritesArtifacts()

### Community 219 - "quorum-ratio-rollups/internal/controller/policy_validation_test.go"
Cohesion: 0.48
Nodes (5): badSelector(), checkWithPolicy(), TestValidateAddonCheckPolicy(), TestValidateAddonCheckPolicy_DeterministicOrder(), TestValidateAddonCheckPolicy_ThresholdKeys()

### Community 221 - "Specification Quality Checklist: Adversarial Codebase Review for the v0.5.0 Release Gate"
Cohesion: 0.29
Nodes (5): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Adversarial Codebase Review for the v0.5.0 Release Gate

### Community 222 - "Specification Quality Checklist: DNSCheck Resource Contract"
Cohesion: 0.29
Nodes (6): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: DNSCheck Resource Contract, Validation Notes

### Community 224 - "Contract: Probe `dns` Mode CLI"
Cohesion: 0.29
Nodes (7): Answer matching, Contract: Probe `dns` Mode CLI, Flags, Invariants, Outcome mapping, Per-record-kind behavior, Vantage point is not a probe flag

### Community 225 - "speckit-constitution/SKILL.md"
Cohesion: 0.33
Nodes (5): Outline, Post-Execution Checks, Pre-Execution Checks, Scope Guard, User Input

### Community 226 - "Network policies"
Cohesion: 0.33
Nodes (5): Network policies, Node-agent DaemonSet (runtime-managed, always on), Operator (static, opt-in), Probe pods (deliberately no Fathom-shipped policy), The `metrics: enabled` label contract

### Community 227 - "Contract: CRD Schema-Compatibility Gate"
Cohesion: 0.33
Nodes (6): Algorithm (normative), Contract: CRD Schema-Compatibility Gate, Exit codes / output, Initial state, Invocation, Pass/fail matrix (asserted by the fixture shell test)

### Community 228 - "Deliverable Contracts: Findings Report, Refuted Record, Coverage Statement"
Cohesion: 0.33
Nodes (6): 1. `review/findings.md` — ranked confirmed findings (deliverable 1), 2. `review/refuted.md` — refuted candidates (working record, FR-003), 3. `review/coverage.md` — coverage statement (deliverable 3), 4. Deferral follow-up issues, 5. Closing comment on #217, Deliverable Contracts: Findings Report, Refuted Record, Coverage Statement

### Community 229 - "Security review candidates — Fathom (commit cb845dd)"
Cohesion: 0.33
Nodes (5): SEC-1: Report-authenticity ValidatingAdmissionPolicy exempts every non-`*-node-agent` writer (medium), SEC-2: node-agent `/metrics` endpoint is unauthenticated and leaks per-node certificate inventory (medium), SEC-3: node-agent hostPath mounts use `DirectoryOrCreate`, letting a check seed root-owned dirs on every node (low), SEC-4: User-controlled DNS targets and resolver address drive arbitrary DNS egress from probe pods (low), Security review candidates — Fathom (commit cb845dd)

### Community 239 - "speckit-taskstoissues/SKILL.md"
Cohesion: 0.40
Nodes (4): Outline, Post-Execution Checks, Pre-Execution Checks, User Input

### Community 242 - "bindAddress"
Cohesion: 0.40
Nodes (5): type, properties, type, bindAddress, healthProbe

### Community 243 - ".severity"
Cohesion: 0.60
Nodes (3): TestHealthReportResultSeverity_EmptyAndUnrecognizedReturnZero(), TestHealthReportResultSeverity_OrderingAcrossEnumValues(), TestHealthReportResultSeverity_PassIsLowestNonZero()

### Community 245 - "scripts/coverage_gate_test.go"
Cohesion: 0.60
Nodes (4): T, normalizeShell(), stripShellComment(), TestCoverageGateSkipsNoPackages()

### Community 246 - "runLockstep"
Cohesion: 0.80
Nodes (4): T, runLockstep(), TestVersionLockstepDetectsDrift(), TestVersionLockstepInSync()

### Community 247 - "Security Policy"
Cohesion: 0.40
Nodes (4): Reporting a vulnerability, Security Policy, Supported versions, What to expect

### Community 248 - "[CHECKLIST TYPE] Checklist: [FEATURE NAME]"
Cohesion: 0.40
Nodes (4): [Category 1], [Category 2], [CHECKLIST TYPE] Checklist: [FEATURE NAME], Notes

### Community 249 - "Contract: Runtime Clamp Signal"
Cohesion: 0.40
Nodes (5): Behavior, Condition, Contract: Runtime Clamp Signal, Event, Idempotence

### Community 251 - "Contract: DNSCheck Admission Validation"
Cohesion: 0.40
Nodes (5): Accept / reject matrix, Contract: DNSCheck Admission Validation, Fully populated object, Minimal accepted object, Non-guarantees

### Community 252 - "Phase 3: User Story 1 — Declare DNS intent and have it validated (P1)"
Cohesion: 0.40
Nodes (5): Generation and the cost gate, Phase 3: User Story 1 — Declare DNS intent and have it validated (P1), Tests, Types, Validation rules

### Community 266 - "normalizeShell"
Cohesion: 0.83
Nodes (3): normalizeShell(), stripShellComment(), TestCoverageGateSkipsNoPackages()

### Community 267 - "extraArgs"
Cohesion: 0.50
Nodes (4): items, type, type, extraArgs

### Community 268 - "countAbsent"
Cohesion: 0.50
Nodes (3): T, TestCountAbsent(), countAbsent()

### Community 272 - "Contract: CRD Admission Validation"
Cohesion: 0.50
Nodes (4): AddonCheck `spec.policy`, Cadence floors (both kinds), Compatibility guarantee (FR-008), Contract: CRD Admission Validation

### Community 273 - "Implementation Strategy"
Cohesion: 0.50
Nodes (4): Implementation Strategy, Incremental Delivery, MVP First (User Story 1 Only), Single-Branch Note

## Knowledge Gaps
- **599 isolated node(s):** `post-install.sh script`, `common.sh script`, `$schema`, `title`, `type` (+594 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **37 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `CheckResult` connect `CheckResult` to `quorum-ratio-rollups/internal/adapter/certmanager/adapter.go`, `.Get`, `countAbsent`, `.Run`, `internal/adapter/declarative/annotation_test.go`, `internal/adapter/declarative/field_test.go`, `runProjection`, `internal/adapter/certmanager/adapter_test.go`, `.detectAndGateVersion`, `.Run`, `TargetRef`, `.Run`, `.checkMetricsEndpoint`, `newFakeClient`, `assertHasDetail`, `FamilyPolicy`, `.Evaluate`, `Family`, `deploymentInNamespace`, `addoncheck_controller.go`, `assertHasOutcome`, `.Evaluate`, `RatioThresholds`, `MustEngine`, `runConfigMap`?**
  _High betweenness centrality (0.117) - this node is a cross-community bridge._
- **Why does `New()` connect `New` to `.Run`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `observeCheck`, `.Update`, `quorum-ratio-rollups/cmd/probe/main_test.go`, `newScheme`, `.Register`, `.Run`, `.DeepCopy`, `TestClientForSetsImpersonationAndMemoizes`, `Init`, `quorum-ratio-rollups/internal/probe/sweeper_test.go`, `Object`, `.detectAndGateVersion`, `quorum-ratio-rollups/test/utils/utils.go`, `.DeepCopyInto`, `TestAdapterClient`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`?**
  _High betweenness centrality (0.086) - this node is a cross-community bridge._
- **Why does `Family` connect `Family` to `.Get`, `.Name`, `validateAddonCheckPolicy`, `internal/adapter/declarative/field_test.go`, `NewMetricsServerEngine`, `runProjection`, `TestEnvoyGateway_HealthyAndNoGatewaysSkipped`, `NewExternalDNSEngine`, `TestExternalSecrets_HealthyAndEmptySyncSkipped`, `CheckResult`, `internal/adapter/certmanager/adapter_test.go`, `.detectAndGateVersion`, `Capabilities`, `TargetRef`, `.checkMetricsEndpoint`, `newFakeClient`, `assertHasDetail`, `FamilyPolicy`, `.Run`, `deploymentInNamespace`, `addoncheck_controller.go`, `assertHasOutcome`, `kedaHealthyObjects`, `RatioThresholds`, `runArgoCD`, `internal/adapter/declarative/istio_test.go`, `New`?**
  _High betweenness centrality (0.083) - this node is a cross-community bridge._
- **Are the 108 inferred relationships involving `assertHasOutcome()` (e.g. with `TestAnnotationStaleness_NamedLock()` and `TestAnnotationStaleness_NodeList()`) actually correct?**
  _`assertHasOutcome()` has 108 INFERRED edges - model-reasoned connections that need verification._
- **Are the 110 inferred relationships involving `assertHasOutcome()` (e.g. with `TestAnnotationStaleness_NamedLock()` and `TestAnnotationStaleness_NodeList()`) actually correct?**
  _`assertHasOutcome()` has 110 INFERRED edges - model-reasoned connections that need verification._
- **Are the 68 inferred relationships involving `newFakeClient()` (e.g. with `TestRun_EmitsSpan()` and `runAnnotation()`) actually correct?**
  _`newFakeClient()` has 68 INFERRED edges - model-reasoned connections that need verification._
- **Are the 63 inferred relationships involving `newFakeClient()` (e.g. with `runAnnotation()` and `runArgoCD()`) actually correct?**
  _`newFakeClient()` has 63 INFERRED edges - model-reasoned connections that need verification._