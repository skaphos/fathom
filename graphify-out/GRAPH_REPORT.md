# Graph Report - fathom  (2026-08-23)

## Corpus Check
- 376 files · ~451,104 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 4578 nodes · 11305 edges · 330 communities (301 shown, 29 thin omitted)
- Extraction: 81% EXTRACTED · 19% INFERRED · 0% AMBIGUOUS · INFERRED: 2116 edges (avg confidence: 0.82)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `21f9f89a`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- .Run
- quorum-ratio-rollups/cmd/probe/main_test.go
- New
- .DeepCopy
- newFakeClient
- CheckResult
- newScheme
- quorum-ratio-rollups/internal/adapter/rbacgen/rbacgen.go
- Fathom Architecture
- fathom_check_result Gauge (one-hot current result)
- quorum-ratio-rollups/internal/controller/nodecertificatecheck_helpers.go
- observeCheck
- Family
- deploymentInNamespace
- detectAddonVersion
- .Register
- .Run
- quorum-ratio-rollups/internal/adapter/declarative/engine_test.go
- assertHasOutcome
- NewRootCommand
- internal/controller/tracing_test.go
- MustEngine
- quorum-ratio-rollups/internal/probe/pod_test.go
- Init
- Probe/Node-Agent Version Lockstep Gate
- quorum-ratio-rollups/internal/controller/healthreport_idempotency.go
- quorum-ratio-rollups/scripts/version_lockstep_gate_test.go
- internal/adapter/certmanager/adapter.go
- internal/adapter/certmanager/adapter_test.go
- cmd/probe/main_test.go
- FamilyDefinition
- .Run
- context.Context
- sigs.k8s.io/controller-runtime/pkg/client.Client
- assertHasDetail
- .Run
- .checkMetricsEndpoint
- newFakeClient
- assertHasDetail
- Tasks: DNSCheck Reconciler
- properties
- quorum-ratio-rollups/internal/adapter/declarative/evaluator.go
- Outcome
- k8s.io/api/core/v1.Pod
- k8s.io/apimachinery/pkg/apis/meta/v1.Time
- CertResult
- quorum-ratio-rollups/test/utils/utils.go
- TestScanAndPublishCreatesAndUpdatesConfigMap
- api/v1alpha1/deepcopy_test.go
- .DeepCopyInto
- common.sh
- Implementation Plan: Pre-1.0 CRD Validation Hardening
- observeCheck
- addoncheck_controller.go
- Implementation Plan: Quorum/Ratio Semantics for Managed-Resource Rollups
- Adversarial Review Findings — v0.5.0 Release Gate (#217)
- internal/app/run_happy_test.go
- assertHasOutcome
- Tasks: [FEATURE NAME]
- speckit-analyze/SKILL.md
- .Evaluate
- kedaHealthyObjects
- Quickstart Validation: Quorum/Ratio Rollups
- Tasks: DNSCheck Completion
- .Reconcile
- PolicyRule
- FamilyRatioVerdict
- Run
- .agents/skills/speckit-analyze/SKILL.md
- join
- test/utils/utils.go
- internal/adapter/rbacgen/rbacgen.go
- Tasks: Adversarial Codebase Review for the v0.5.0 Release Gate
- Tasks: Cadence-Aware Staleness Semantics for ClusterHealth
- DefaultOptions
- Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth
- SetRunningInClusterForTest
- Execution Steps
- newFakeClient
- TestAdapterClient
- newHealthCheckTargetRegistry
- image
- Load
- establishedCRD
- runArgoCD
- assertFamily
- New
- TestAnnotationStaleness_NamedLock
- NewRootCommand
- kedaHealthyObjects
- Feature Specification: DNSCheck Resource Contract
- Implementation Plan: Cadence-Aware Staleness Semantics for ClusterHealth
- Options
- .Evaluate
- properties
- test/e2e/healthreport_helpers_test.go
- MustEngine
- Sweeper
- Core Principles
- .Update
- 5. ClusterHealth staleness is the stalest child, and is a signal not a verdict
- dnscheck_validation_test.go
- Execution Steps
- DNS Checks
- Contract: `ClusterHealth.Status`
- internal/metrics/check_metrics_test.go
- testing.T
- quorum-ratio-rollups/internal/nodecert/paths.go
- Tasks: Pre-1.0 CRD Validation Hardening
- TestCommittedAddonRolesAreReadOnly
- quorum-ratio-rollups/internal/probe/sweeper_test.go
- Request
- operator_rbac_doc_test.go
- .Name
- .Run
- quorum-ratio-rollups/internal/nodecert/scan.go
- Tasks: DNSCheck Resource Contract
- Implementation Plan: DNSCheck Completion
- internal/adapter/crdutil/crdutil_test.go
- rbac
- enabled
- factory
- internal/adapter/declarative/annotation_test.go
- internal/adapter/declarative/field_test.go
- NewMetricsServerEngine
- runProjection
- .agents/skills/speckit-plan/SKILL.md
- crd_compat_gate_test.go
- Entity: `DNSCheck`
- HealthReportResult
- .agents/skills/speckit-specify/SKILL.md
- Repository Guidelines
- values.schema.json
- properties
- .agents/skills/speckit-tasks/SKILL.md
- Feature Specification: [FEATURE NAME]
- programmableAdapter
- quorum-ratio-rollups/internal/adapter/declarative/field_test.go
- k8s.io/apimachinery/pkg/runtime.Object
- GitHub Copilot Instructions for Fathom
- deploymentInNamespace
- TestEnvoyGateway_HealthyAndNoGatewaysSkipped
- Tasks: Quorum/Ratio Semantics for Managed-Resource Rollups
- Feature Specification: Adversarial Codebase Review for the v0.5.0 Release Gate
- speckit-plan/SKILL.md
- speckit-specify/SKILL.md
- speckit-tasks/SKILL.md
- test/e2e/nodecert_test.go
- TestExternalSecrets_HealthyAndEmptySyncSkipped
- time.Duration
- Core Principles
- Changed: AddonCheck (`fathom.skaphos.io/v1alpha1`, namespaced)
- Research: Pre-1.0 CRD Validation Hardening
- API / CRD Contract-Stability Candidates — commit cb845dd
- Correctness / Reconcile-Time Review — Candidates
- Implementation Plan: DNSCheck Resource Contract
- Phase 0 Research: DNSCheck Resource Contract
- Feature Specification: DNSCheck Completion
- RBAC / Least-Privilege Review — Fathom operator (commit cb845dd)
- Quickstart: Validating Cadence-Aware Staleness
- .IsReadOnly
- NewEngine
- TestCommittedAddonRolesAreReadOnly
- msHealthyObjects
- podInNamespace
- Implementation Plan: [FEATURE]
- Entities
- Phase 0 Research: Adversarial Codebase Review for the v0.5.0 Release Gate
- Quickstart: Validating the DNSCheck Resource Contract
- speckit-checklist/SKILL.md
- Phase 0 Research: Cadence-Aware Staleness Semantics for ClusterHealth
- ParseAddonSelection
- main
- healthcheck_controller.go
- .agents/skills/speckit-checklist/SKILL.md
- BuiltInAdapters
- quorum-ratio-rollups/internal/app/run_happy_test.go
- Research: Quorum/Ratio Semantics for Managed-Resource Rollups
- Quickstart Validation: Pre-1.0 CRD Validation Hardening
- Implementation Plan: Adversarial Codebase Review for the v0.5.0 Release Gate
- Quickstart: Validating the v0.5.0 Adversarial Review Release Gate
- Supply-Chain / CI-Integrity Review — Fathom @ cb845dd
- Phase 1 Data Model: Cadence-Aware Staleness Semantics
- Data Model: DNSCheck Completion
- speckit-clarify/SKILL.md
- speckit-implement/SKILL.md
- Research: DNSCheck Completion
- .agents/skills/speckit-clarify/SKILL.md
- .checkCRD
- Phase 1 Data Model: DNSCheck Reconciler
- .agents/skills/speckit-implement/SKILL.md
- webhookEntry
- Operator RBAC
- Specification Quality Checklist: Cadence-Aware Staleness Semantics for ClusterHealth
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
- Specification Quality Checklist: DNSCheck Completion
- .DeepCopy
- .DeepCopy
- Contract: HealthCheck Target Projection
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- speckit-taskstoissues/SKILL.md
- Quickstart Validation: DNSCheck Completion
- Feature Specification: DNSCheck Reconciler
- bindAddress
- quorum-ratio-rollups/internal/nodecert/scan_test.go
- check-version-lockstep.sh
- scripts/coverage_gate_test.go
- .agents/skills/speckit-constitution/SKILL.md
- Security Policy
- [CHECKLIST TYPE] Checklist: [FEATURE NAME]
- Contract: Runtime Clamp Signal
- Contract: DNSCheck Admission Validation
- Phase 3: User Story 1 — Declare DNS intent and have it validated (P1)
- Phase 0 Research: DNSCheck Reconciler
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- Contract: DNSCheck Metrics, Events, and RBAC
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- normalizeShell
- extraArgs
- Contract: DNSCheck Reconcile Loop
- User Scenarios & Testing *(mandatory)*
- check-coverage.sh
- e2e-shards.sh
- Contract: CRD Admission Validation
- Implementation Strategy
- .agents/skills/speckit-taskstoissues/SKILL.md
- test/e2e/observability_test.go
- Problem
- post-install.sh
- check-crd-compat.sh
- github.com/skaphos/fathom
- github.com/skaphos/fathom/tools
- .DeepCopy
- .DeepCopy
- Adapter
- New
- Implementation Plan: DNSCheck Reconciler
- SAUsername
- Quickstart Validation: DNSCheck Reconciler
- User Scenarios & Testing *(mandatory)*
- builder
- Implementation Strategy
- dnscheck_test.go
- Specification Quality Checklist: DNSCheck Reconciler
- Rationale: in-process interface over gRPC/OCI/plugin loaders
- quorum-ratio-rollups/internal/adapter/declarative/podprojection_test.go
- Implementation Strategy

## God Nodes (most connected - your core abstractions)
1. `assertHasOutcome()` - 142 edges
2. `assertHasOutcome()` - 122 edges
3. `CheckResult` - 108 edges
4. `newFakeClient()` - 80 edges
5. `newFakeClient()` - 76 edges
6. `New()` - 76 edges
7. `Family` - 73 edges
8. `join()` - 67 edges
9. `FamilyPolicy` - 57 edges
10. `assertHasDetail()` - 57 edges

## Surprising Connections (you probably didn't know these)
- `EnsureCompatible()` --semantically_similar_to--> `CRD Maturity Ladder (alpha/beta/GA)`  [INFERRED] [semantically similar]
  pkg/adapter/version.go → docs/reference/api-versioning.md
- `NewScheme()` --references--> `Fathom Architecture`  [EXTRACTED]
  internal/app/run.go → docs/architecture.md
- `BuiltInAdapters()` --references--> `Fathom Architecture`  [EXTRACTED]
  internal/app/run.go → docs/architecture.md
- `BuiltInAdapters()` --references--> `Authoring an Adapter Guide`  [EXTRACTED]
  internal/app/run.go → docs/authoring-adapters.md
- `AddonCheckReconciler` --references--> `ADR-0002 HealthReport as First-class CRD`  [EXTRACTED]
  internal/controller/addoncheck_controller.go → docs/adr/0002-healthreport-as-first-class-crd.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **AddonCheck → HealthCheck → ClusterHealth chain with HealthReport history** — readme_addoncheck, readme_healthcheck, readme_clusterhealth, readme_healthreport, readme_aggregation_chain [EXTRACTED 1.00]
- **Speckit artifact pipeline for feature 001 (spec → plan → tasks → quickstart)** — specs_001_alerting_observability_spec_alerting_observability, specs_001_alerting_observability_plan_alerting_observability_plan, specs_001_alerting_observability_tasks_alerting_observability_tasks, specs_001_alerting_observability_quickstart_validation [EXTRACTED 1.00]
- **Alerting-grade observability contract surface (gauges, events, alert rules)** — specs_001_alerting_observability_contracts_metrics_fathom_check_result, specs_001_alerting_observability_contracts_metrics_fathom_check_last_run_timestamp_seconds, specs_001_alerting_observability_contracts_metrics_alert_rules, specs_001_alerting_observability_contracts_events_resultchanged, specs_001_alerting_observability_contracts_events_failure_reasons [EXTRACTED 1.00]
- **AddonCheck -> HealthCheck -> ClusterHealth status aggregation chain** — api_v1alpha1_addoncheck_types_addoncheck, api_v1alpha1_healthcheck_types_healthcheck, api_v1alpha1_clusterhealth_types_clusterhealth, api_v1alpha1_healthreport_types_healthreport, controller_addoncheckreconciler, controller_healthcheckreconciler, controller_clusterhealthreconciler [EXTRACTED 1.00]
- **Probe-pod lifecycle: build, launch, parse, sweep orphans** — internal_probe_pod_pod, internal_probe_launcher_launcher, internal_probe_sweeper_sweeper, docs_architecture_probe_pod_model [EXTRACTED 1.00]

## Communities (330 total, 29 thin omitted)

### Community 0 - ".Run"
Cohesion: 0.07
Nodes (42): TestCountAbsent(), TestFamilyOutcome(), TestOutcomeValid(), TestClusterHealthCoversNamespace(), TestConfigMapCheck_AbsentInheritsOptional(), TestDescheduler_AbsentClusterAllSkipped(), NewEnvoyGatewayEngine(), egHealthyObjects() (+34 more)

### Community 1 - "quorum-ratio-rollups/cmd/probe/main_test.go"
Cohesion: 0.09
Nodes (41): main(), metricsMux(), publishGauges(), run(), runDNS(), runHTTPGet(), runTCPConnect(), runTCPListen() (+33 more)

### Community 2 - "New"
Cohesion: 0.11
Nodes (37): assertNoKind(), certManagerResource(), establishedCRDServing(), evictedPodNamed(), healthyDeployment(), mutatingWebhookConfiguration(), mutatingWebhookConfigurationNamed(), notReadyCertificate() (+29 more)

### Community 3 - ".DeepCopy"
Cohesion: 0.34
Nodes (18): deepCopyContract(), fullyPopulatedAddonCheck(), fullyPopulatedClusterHealth(), fullyPopulatedHealthCheck(), fullyPopulatedHealthReport(), fullyPopulatedNodeCertificateCheck(), runtimeObjectContract(), TestDeepCopy_AddonCheck() (+10 more)

### Community 4 - "newFakeClient"
Cohesion: 0.19
Nodes (39): adapterWithLauncher(), assertNoTarget(), daemonSetWithStatus(), dnsEndpointSlice(), dnsEndpointSliceNamed(), dnsService(), dnsServiceNamed(), healthyDeploymentNamed() (+31 more)

### Community 5 - "CheckResult"
Cohesion: 0.11
Nodes (41): firstNamespace(), containsString(), AnnotationStalenessCheck, ConfigMapCheck, CRDCheck, CronJobCheck, EvalContext, WebhookCheck (+33 more)

### Community 6 - "newScheme"
Cohesion: 0.12
Nodes (17): appFakeAdapter, TestAddToScheme(), TestDeepCopyIntoExercise(), TestSchemeBuilderRegisterReturnsSelf(), TestBuildManagerOptions_CertWatchers(), TestBuildManagerOptions_DefaultsHaveNoCertWatchers(), TestBuildManagerOptions_InsecureMetricsHasNoFilter(), TestBuildManagerOptions_MissingWebhookCert_Errors() (+9 more)

### Community 7 - "quorum-ratio-rollups/internal/adapter/rbacgen/rbacgen.go"
Cohesion: 0.22
Nodes (15): TestFilesRejectsIncompleteRule(), clusterRules(), Files(), groupsCell(), k8sObject, marshalDocs(), objectMeta, renderAddon() (+7 more)

### Community 8 - "Fathom Architecture"
Cohesion: 0.10
Nodes (58): version, AddonCheck CRD, ClusterHealth CRD (aggregate), HealthCheck CRD (thin wrapper), HealthReport CRD (immutable history), HealthReportResult Severity Enum, NodeCertificateCheck CRD, ADR-0001 In-process Adapter Contract (+50 more)

### Community 9 - "fathom_check_result Gauge (one-hot current result)"
Cohesion: 0.06
Nodes (49): ClusterHealth External Contract (derived only from HealthCheck.status), Cobra+Viper Configuration Model (flag → env → file → default), Run e2e After Major Changes Policy, AGENTS.md Repository Guidelines (CLAUDE.md symlink), SPDX Boilerplate Header, Breaking Change: ClusterHealth Made Cluster-Scoped (0.4.0), DCO Sign-Off Requirement, Contributor Safety Expectations (Bounded Work, Minimal RBAC) (+41 more)

### Community 10 - "quorum-ratio-rollups/internal/controller/nodecertificatecheck_helpers.go"
Cohesion: 0.15
Nodes (11): aggregateNodeReports(), controlPlaneTolerations(), healthReportForNodeCert(), joinPaths(), nodeOutcomeToResult(), pruneNodeCertHealthReports(), putIfNotEmpty(), resolveTolerations() (+3 more)

### Community 11 - "observeCheck"
Cohesion: 0.11
Nodes (27): ctrlRegistryGather(), gatherCheckSeries(), gatherOneHot(), TestDeleteCheckSeries(), TestObserveCheckFlipsResult(), TestObserveCheckOneHotInvariant(), TestObserveCheckSentinels(), checkGaugeValue() (+19 more)

### Community 12 - "Family"
Cohesion: 0.09
Nodes (42): endAdapterRunSpan(), skipped(), ThresholdAdvertiser, resolveFamily(), go.opentelemetry.io/otel/trace.Span, certManagerComponents(), endAdapterRunSpan(), familyEnabled() (+34 more)

### Community 13 - "deploymentInNamespace"
Cohesion: 0.21
Nodes (27): assertNoOutcome(), deploymentInNamespace(), absenceEngine(), deployEngine(), failedPod(), notReadyPod(), podWithRestarts(), runEngine() (+19 more)

### Community 14 - "detectAddonVersion"
Cohesion: 0.53
Nodes (5): versionAddress, detectAddonVersion(), imageTag(), pickImage(), TestImageTag()

### Community 15 - ".Register"
Cohesion: 0.42
Nodes (8): newFake(), TestCapabilities(), TestConcurrentAccess(), TestLookup(), TestRegister(), TestRegister_DuplicateAddonType(), TestRegister_PartialFailureLeavesRegistryUnchanged(), TestRegister_SameAdapterTwiceLogsNotice()

### Community 16 - ".Run"
Cohesion: 0.17
Nodes (45): assertCheck(), findCheck(), ksmService(), passingLauncher(), readyPod(), runRequest(), TestRun_AllFamiliesDisabledEmitsSentinelSkip(), TestRun_HealthyDeploymentAndEndpointsPass() (+37 more)

### Community 17 - "quorum-ratio-rollups/internal/adapter/declarative/engine_test.go"
Cohesion: 0.27
Nodes (21): NewCiliumEngine(), assertFamily(), assertHasDetail(), assertHasOutcome(), assertNoKind(), assertNoOutcome(), ciliumCRDNames(), daemonSetInNamespace() (+13 more)

### Community 18 - "assertHasOutcome"
Cohesion: 0.27
Nodes (21): assertHasOutcome(), runManaged(), TestCondition_ClusterScopedListsWithoutNamespace(), TestCondition_ConditionStatus(), TestCondition_InvalidAPIVersionErrors(), TestCondition_InvalidSelectorErrors(), TestCondition_ListErrorDescribesNamespaceScope(), TestCondition_ListNameFallsBackToKind() (+13 more)

### Community 19 - "NewRootCommand"
Cohesion: 0.23
Nodes (11): NewRootCommand(), signalContext(), TestSignalContext_PropagatesParentCancellation(), TestSignalContext_SIGINTCancels(), TestSignalContext_SIGTERMCancels(), TestSignalContext_StopReleasesContext(), TestNewRootCommand_BasicWiring(), TestNewRootCommand_HelpDoesNotErrorWithoutKubeconfig() (+3 more)

### Community 20 - "internal/controller/tracing_test.go"
Cohesion: 0.16
Nodes (18): TestListSelectedHealthChecks_ErrorNamesScope(), failingHealthCheckListClient, go.opentelemetry.io/otel/sdk/trace/tracetest.InMemoryExporter, go.opentelemetry.io/otel/sdk/trace/tracetest.SpanStub, go.opentelemetry.io/otel/sdk/trace/tracetest.SpanStubs, TestListSelectedHealthChecks_ErrorNamesScope(), attrValue(), installInMemoryTracer() (+10 more)

### Community 21 - "MustEngine"
Cohesion: 0.09
Nodes (32): TestMustEngine_PanicsOnInvalid(), crdAbsenceEngine(), TestCRD_AbsenceResolution(), cronJob(), runCronJob(), TestCronJobCheck(), TestCronJobCheck_AbsentInheritsOptional(), TestCronJobCheck_PerpetualFailurePastWindowWarns() (+24 more)

### Community 22 - "quorum-ratio-rollups/internal/probe/pod_test.go"
Cohesion: 0.25
Nodes (4): assertArgs(), TestPodBuildsHardenedDNSProbe(), TestPodBuildsHTTPGetArgs(), TestPodRejectsInvalidRequests()

### Community 23 - "Init"
Cohesion: 0.24
Nodes (10): Init(), restoreGlobalProvider(), TestInit_DisabledInstallsNoopProvider(), TestInit_EnabledInstallsRecordingProvider(), Config, Init(), ShutdownFunc, restoreGlobalProvider() (+2 more)

### Community 24 - "Probe/Node-Agent Version Lockstep Gate"
Cohesion: 0.40
Nodes (6): Kubernetes Test-Version Lockstep (envtest / kind / crd-ref-docs), Fathom Release History (Changelog), Conventional Commits Policy, Why Lockstep Is Automated: Human-Gated Contract Failed for 0.3.0/0.3.1 (SKA-579), Release Please Flow, Probe/Node-Agent Version Lockstep Gate

### Community 25 - "quorum-ratio-rollups/internal/controller/healthreport_idempotency.go"
Cohesion: 0.60
Nodes (4): createOrReuseHealthReport(), deterministicHealthReportName(), useDeterministicHealthReportName(), validateReusableHealthReport()

### Community 26 - "quorum-ratio-rollups/scripts/version_lockstep_gate_test.go"
Cohesion: 0.83
Nodes (3): runLockstep(), TestVersionLockstepDetectsDrift(), TestVersionLockstepInSync()

### Community 51 - "internal/adapter/certmanager/adapter.go"
Cohesion: 0.06
Nodes (56): certificateCheck(), certificateDetails(), conditionDetails(), conditionStatus(), conditionType(), daysRemaining(), daysThreshold(), dryRunCertificate() (+48 more)

### Community 52 - "internal/adapter/certmanager/adapter_test.go"
Cohesion: 0.12
Nodes (54): clientObject, k8s.io/api/admissionregistration/v1.MutatingWebhookConfiguration, k8s.io/api/admissionregistration/v1.ValidatingWebhookConfiguration, k8s.io/api/admissionregistration/v1.WebhookClientConfig, k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1.CustomResourceDefinition, New(), assertFamily(), assertHasDetail() (+46 more)

### Community 53 - "cmd/probe/main_test.go"
Cohesion: 0.08
Nodes (57): result, join(), lookupCNAME(), lookupIPs(), lookupSRV(), main(), missingAnswers(), normalizeAnswer() (+49 more)

### Community 54 - "FamilyDefinition"
Cohesion: 0.17
Nodes (12): CRDCheck, AddonDefinition, FamilyDefinition, VersionSource, AnnotationStalenessCheck, ConditionCheck, ConfigMapCheck, CronJobCheck (+4 more)

### Community 55 - ".Run"
Cohesion: 0.17
Nodes (40): clientObject, fakeLauncher, k8s.io/api/discovery/v1.EndpointSlice, adapterWithLauncher(), assertHasDetail(), assertHasOutcome(), assertNoOutcome(), dnsEndpointSlice() (+32 more)

### Community 56 - "context.Context"
Cohesion: 0.09
Nodes (45): NodeCertificateCheck, NodeCertificateCheckSpec, NodeCertificateCheckStatus, NodeCertificateCheckReconciler, nodeCertRollupDecision, context.Context, github.com/go-logr/logr.Logger, k8s.io/api/admissionregistration/v1.ValidatingAdmissionPolicySpec (+37 more)

### Community 57 - "sigs.k8s.io/controller-runtime/pkg/client.Client"
Cohesion: 0.06
Nodes (56): adapterOutcome(), boundedNodeList(), check(), deploymentAvailable(), dnsProbePodName(), dnsTargets(), familyForTarget(), int32Threshold() (+48 more)

### Community 58 - "assertHasDetail"
Cohesion: 0.18
Nodes (30): assertHasDetail(), TestRun_AbsentComponentsCarryMarker(), NewAzureWorkloadIdentityEngine(), healthyAzureWIObjects(), TestAzureWorkloadIdentity_AbsentWebhookFails(), TestAzureWorkloadIdentity_Capabilities(), TestAzureWorkloadIdentity_HealthyClusterAllPass(), TestAzureWorkloadIdentity_NoOptedInPodsProjectionSkipped() (+22 more)

### Community 59 - ".Run"
Cohesion: 0.25
Nodes (30): dnsProbePodName(), adapterWithLauncher(), assertHasDetail(), assertHasOutcome(), assertNoTarget(), daemonSetWithStatus(), Adapter, dnsProbeLauncher (+22 more)

### Community 60 - ".checkMetricsEndpoint"
Cohesion: 0.10
Nodes (27): certManagerComponents(), csvThreshold(), endpointTarget(), scrapeProbePodName(), servicePortDeclared(), servicePortList(), stringThreshold(), k8s.io/api/core/v1.Service (+19 more)

### Community 61 - "newFakeClient"
Cohesion: 0.15
Nodes (29): clientObject, Engine, NewCiliumEngine(), TestCondition_ResolveVersion(), assertNoKind(), ciliumCRDNames(), daemonSetInNamespace(), establishedCRD() (+21 more)

### Community 62 - "assertHasDetail"
Cohesion: 0.16
Nodes (33): Engine, NewAzureWorkloadIdentityEngine(), clientObject, healthyAzureWIObjects(), TestAzureWorkloadIdentity_AbsentWebhookFails(), TestAzureWorkloadIdentity_Capabilities(), TestAzureWorkloadIdentity_HealthyClusterAllPass(), TestAzureWorkloadIdentity_NoOptedInPodsProjectionSkipped() (+25 more)

### Community 63 - "Tasks: DNSCheck Reconciler"
Cohesion: 0.08
Nodes (26): Analysis remediation, Critical Path, Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation for User Story 4 (+18 more)

### Community 64 - "properties"
Cohesion: 0.05
Nodes (39): type, type, type, type, type, type, type, type (+31 more)

### Community 65 - "quorum-ratio-rollups/internal/adapter/declarative/evaluator.go"
Cohesion: 0.13
Nodes (4): Evaluator, durationThreshold(), isFutureTimestamp(), namespaceScope()

### Community 66 - "Outcome"
Cohesion: 0.25
Nodes (13): Posture, WorkloadKind, AnnotationStalenessCheck, ConditionCheck, ConfigMapCheck, CRDCheck, CronJobCheck, FieldCheck (+5 more)

### Community 67 - "k8s.io/api/core/v1.Pod"
Cohesion: 0.19
Nodes (30): k8s.io/api/apps/v1.StatefulSet, k8s.io/api/core/v1.Pod, assertNoOutcome(), podInNamespace(), absenceEngine(), deployEngine(), failedPod(), Engine (+22 more)

### Community 68 - "k8s.io/apimachinery/pkg/apis/meta/v1.Time"
Cohesion: 0.09
Nodes (36): AddonCheckList, ClusterHealth, ClusterHealthChildSummary, ClusterHealthList, ClusterHealthSpec, ClusterHealthStatus, DNSCheckList, CheckTargetRef (+28 more)

### Community 69 - "CertResult"
Cohesion: 0.05
Nodes (71): TestWorstResult(), WorstResult(), main(), metricsMux(), parseConfig(), publishGauges(), run(), sanitizeLabelValue() (+63 more)

### Community 70 - "quorum-ratio-rollups/test/utils/utils.go"
Cohesion: 0.13
Nodes (23): TestE2EShardPlannerKnowsEveryOptInAddon(), TestE2E(), AddonSelection, CoreAddons(), GetNonEmptyLines(), GetProjectDir(), InstallPrometheusOperator(), IsPrometheusCRDsInstalled() (+15 more)

### Community 71 - "TestScanAndPublishCreatesAndUpdatesConfigMap"
Cohesion: 0.11
Nodes (21): parseConfig(), splitCSV(), TestParseConfig(), TestSanitizeLabelValue(), TestScanAndPublishCreatesAndUpdatesConfigMap(), TestSplitCSV(), writeCert(), writeReportWithAnnotation() (+13 more)

### Community 72 - "api/v1alpha1/deepcopy_test.go"
Cohesion: 0.18
Nodes (27): AddonCheck, deepCopyContract(), fullyPopulatedAddonCheck(), fullyPopulatedClusterHealth(), fullyPopulatedDNSCheck(), fullyPopulatedHealthCheck(), fullyPopulatedHealthReport(), fullyPopulatedNodeCertificateCheck() (+19 more)

### Community 73 - ".DeepCopyInto"
Cohesion: 0.08
Nodes (11): ClusterHealthSpec, DNSCheck, DNSCheckList, DNSCheckSpec, DNSCheckStatus, DNSResolver, DNSTarget, DNSTargetResult (+3 more)

### Community 74 - "common.sh"
Cohesion: 0.08
Nodes (17): check-prerequisites.sh script, check_dir(), check_file(), get_feature_paths(), get_repo_root(), has_jq(), _persist_feature_json(), resolve_specify_init_dir() (+9 more)

### Community 75 - "Implementation Plan: Pre-1.0 CRD Validation Hardening"
Cohesion: 0.07
Nodes (27): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Pre-1.0 CRD Validation Hardening, Complexity Tracking, Constitution Check, Documentation (this feature) (+19 more)

### Community 76 - "observeCheck"
Cohesion: 0.20
Nodes (21): AddonCheckStatus, k8s.io/apimachinery/pkg/apis/meta/v1.Condition, k8s.io/apimachinery/pkg/apis/meta/v1.ConditionStatus, k8s.io/client-go/tools/events.FakeRecorder, acceptedCondition(), TestObserveCheckEmitsCadenceClampedOncePerEpisode(), checkGaugeValue(), createAddonCheckWithStatusForObservability() (+13 more)

### Community 77 - "addoncheck_controller.go"
Cohesion: 0.09
Nodes (41): AddonCheck, AddonCheckFamilyPolicy, AddonCheckSpec, WorstResult(), addonAdapterLookup, AddonCheckReconciler, familyRatioRollup, k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta (+33 more)

### Community 78 - "Implementation Plan: Quorum/Ratio Semantics for Managed-Resource Rollups"
Cohesion: 0.07
Nodes (25): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Quorum/Ratio Semantics for Managed-Resource Rollups, Complexity Tracking, Constitution Check, Documentation (this feature) (+17 more)

### Community 79 - "Adversarial Review Findings — v0.5.0 Release Gate (#217)"
Cohesion: 0.07
Nodes (24): Coverage Statement — v0.5.0 Release Gate (#217), Intentionally excluded, Perspective results (SC-001), Post-anchor deltas, Reviewed, Scope notes, Adversarial Review Findings — v0.5.0 Release Gate (#217), API-1: HealthCheck status.summary MaxLength=1024 wedges mirroring on long condition messages (high) (+16 more)

### Community 80 - "internal/app/run_happy_test.go"
Cohesion: 0.20
Nodes (11): crypto/tls.Config, testing.M, adapterName(), disableHTTP2(), firstEnvtestBinaryDir(), TestAdapterName_NilReturnsPlaceholder(), TestAdapterName_NonNilReturnsName(), TestDisableHTTP2() (+3 more)

### Community 81 - "assertHasOutcome"
Cohesion: 0.27
Nodes (21): ConditionCheck, runManaged(), TestCondition_ClusterScopedListsWithoutNamespace(), TestCondition_ConditionStatus(), TestCondition_InvalidAPIVersionErrors(), TestCondition_InvalidSelectorErrors(), TestCondition_ListErrorDescribesNamespaceScope(), TestCondition_ListNameFallsBackToKind() (+13 more)

### Community 82 - "Tasks: [FEATURE NAME]"
Cohesion: 0.07
Nodes (26): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation Strategy, Incremental Delivery, MVP First (User Story 1 Only) (+18 more)

### Community 83 - "speckit-analyze/SKILL.md"
Cohesion: 0.08
Nodes (25): 1. Initialize Analysis Context, 2. Load Artifacts (Progressive Disclosure), 3. Build Semantic Models, 4. Detection Passes (Token-Efficient Analysis), 5. Severity Assignment, 6. Produce Compact Analysis Report, 7. Provide Next Actions, 8. Offer Remediation (+17 more)

### Community 84 - ".Evaluate"
Cohesion: 0.16
Nodes (13): conditionStatus(), policySelector(), resourceAbsent(), ConditionCheck, FieldCheck, defaultOutcome(), conditionStatus(), ConditionCheck (+5 more)

### Community 85 - "kedaHealthyObjects"
Cohesion: 0.13
Nodes (20): Engine, WorkloadCheck, kedaDeployment(), NewKedaEngine(), conditionCR(), clientObject, kedaHealthyObjects(), TestKeda_AbsentClusterAllSkipped() (+12 more)

### Community 86 - "Quickstart Validation: Quorum/Ratio Rollups"
Cohesion: 0.08
Nodes (22): Configuration surface (AddonCheck), Contract: Ratio Rollup Thresholds and Report Entries, Explicit non-changes, Metrics interplay (informative), Rejection (Accepted condition), Report surface (HealthReport), Verdict semantics, Data Model: Quorum/Ratio Semantics for Managed-Resource Rollups (+14 more)

### Community 87 - "Tasks: DNSCheck Completion"
Cohesion: 0.07
Nodes (27): Dependencies and Execution Order, Documentation for User Story 4, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation Strategy, Incremental Delivery (+19 more)

### Community 88 - ".Reconcile"
Cohesion: 0.12
Nodes (19): ClusterHealthReconciler, go.opentelemetry.io/otel/trace.Tracer, k8s.io/apimachinery/pkg/labels.Selector, k8s.io/client-go/tools/events.EventRecorder, sigs.k8s.io/controller-runtime/pkg/handler.EventHandler, sigs.k8s.io/controller-runtime.Request, sigs.k8s.io/controller-runtime.Result, clearClusterHealthAggregateStatus() (+11 more)

### Community 89 - "PolicyRule"
Cohesion: 0.18
Nodes (13): hasResource(), hasVerb(), TestRBACRulesDeclaresDryRunException(), hasResource(), hasVerb(), TestRBACRulesDeclaresProbeException(), hasResource(), hasVerb() (+5 more)

### Community 90 - "FamilyRatioVerdict"
Cohesion: 0.20
Nodes (15): FamilyRatioVerdict(), RatioPercent, RatioRollup, RatioThresholds, Outcome, isDigits(), parseRatioPercent(), ParseRatioThresholds() (+7 more)

### Community 91 - "Run"
Cohesion: 0.27
Nodes (9): os/exec.Cmd, applyDNSCheck(), dnsCheckField(), ensureNamespaceActive(), eventuallyDNSResult(), addonCheckLastResult(), addonCheckReadyTrue(), dumpAddonCheckDiagnostics() (+1 more)

### Community 92 - ".agents/skills/speckit-analyze/SKILL.md"
Cohesion: 0.08
Nodes (25): 1. Initialize Analysis Context, 2. Load Artifacts (Progressive Disclosure), 3. Build Semantic Models, 4. Detection Passes (Token-Efficient Analysis), 5. Severity Assignment, 6. Produce Compact Analysis Report, 7. Provide Next Actions, 8. Offer Remediation (+17 more)

### Community 93 - "join"
Cohesion: 0.15
Nodes (27): serviceAccountToken(), join(), TestMain_ExitsNonZeroOnWriteError(), applyManifest(), scrapeOperatorMetrics(), newTestFlags(), TestDefaultOptions_MatchFlagDefaults(), TestLoad_ConfigOverridesDefault() (+19 more)

### Community 94 - "test/utils/utils.go"
Cohesion: 0.17
Nodes (18): GetNonEmptyLines(), GetProjectDir(), InstallPrometheusOperator(), IsPrometheusCRDsInstalled(), SyncAddons(), TestGetNonEmptyLines(), TestGetProjectDir(), TestGetProjectDirStripsE2ESegment() (+10 more)

### Community 95 - "internal/adapter/rbacgen/rbacgen.go"
Cohesion: 0.20
Nodes (18): TestFilesRejectsIncompleteRule(), TestModelGrantsAreJustified(), clusterRules(), Collect(), Files(), AddonRBAC, groupsCell(), marshalDocs() (+10 more)

### Community 96 - "Tasks: Adversarial Codebase Review for the v0.5.0 Release Gate"
Cohesion: 0.09
Nodes (21): Consolidation and refutation, Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation Strategy, Incremental Delivery, MVP First (US1 only), Notes, Parallel Example: User Story 1 (+13 more)

### Community 97 - "Tasks: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.08
Nodes (24): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Notes, Parallel Example: User Story 1 (+16 more)

### Community 98 - "DefaultOptions"
Cohesion: 0.20
Nodes (19): k8s.io/apimachinery/pkg/runtime.Scheme, sigs.k8s.io/controller-runtime.Options, sigs.k8s.io/controller-runtime/pkg/certwatcher.CertWatcher, DefaultOptions(), TestValidate(), TestValidate_MultipleErrorsAccumulate(), BuildManagerOptions(), NewScheme() (+11 more)

### Community 99 - "Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.10
Nodes (20): Assumptions, Clarifications, D1 — Staleness is a signal, never a verdict change, D2 — Cadence is published for self-scheduling kinds; the aggregate is fixed at its derivation, D3 — "Staleness" is the canonical term; "freshness" is not used, Dependencies and Constraints, Edge Cases, Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth (+12 more)

### Community 100 - "SetRunningInClusterForTest"
Cohesion: 0.15
Nodes (12): k8s.io/api/core/v1.ServiceAccount, defaultRunningInCluster(), inClusterFromConfigErr(), TestInClusterFromConfigErr(), RunningInCluster(), SetRunningInClusterForTest(), TestRunningInCluster_TestOverride(), TestDefaultControllers_InClusterRequiresNamespace() (+4 more)

### Community 101 - "Execution Steps"
Cohesion: 0.12
Nodes (15): 1. Initialize Convergence Context, 2. Load Artifacts (Progressive Disclosure), 3. Build the Intent Inventory, 4. Assess the Codebase and Classify Findings, 5. Assign Severity, 6. Present the In-Session Findings Summary, 7. Append Convergence Tasks (or report converged), 8. Provide Next Actions (Handoff) (+7 more)

### Community 102 - "newFakeClient"
Cohesion: 0.16
Nodes (31): dnsRequest(), Request, newFakeClient(), simulateKubelet(), TestLauncherRun_ConcurrentRunsAreIndependent(), TestLauncherRun_DeletesPodAfterRun(), TestLauncherRun_EmptyTerminationMessageIsError(), TestLauncherRun_FailedPhasePropagatesProbeJSON() (+23 more)

### Community 103 - "TestAdapterClient"
Cohesion: 0.15
Nodes (11): addonSA(), TestAdapterClient(), TestRunAddonCheckFailsClosedWhenNamespaceEmptyInCluster(), TestRunAddonCheckFailsClosedWithoutScopedClient(), TestDefaultControllers_InClusterRequiresNamespace(), defaultRunningInCluster(), inClusterFromConfigErr(), TestInClusterFromConfigErr() (+3 more)

### Community 104 - "newHealthCheckTargetRegistry"
Cohesion: 0.23
Nodes (14): healthCheckTargetIdentity, newHealthCheckTargetRegistry(), normalizeHealthCheckTarget(), TestAddonCheckTargetHandlerBoundsSummary(), TestAddonCheckTargetHandlerCompatibility(), TestDNSCheckTargetHandlerProjection(), TestHealthCheckTargetReferenceFailures(), TestNodeCertificateCheckTargetHandlerProjection() (+6 more)

### Community 105 - "image"
Cohesion: 0.11
Nodes (20): properties, required, type, properties, required, type, image, probeImage (+12 more)

### Community 106 - "Load"
Cohesion: 0.26
Nodes (19): github.com/spf13/pflag.FlagSet, sigs.k8s.io/controller-runtime/pkg/log/zap.Options, Load(), RegisterFlags(), newTestFlags(), TestDefaultOptions_MatchFlagDefaults(), TestLoad_ConfigOverridesDefault(), TestLoad_DNSCheckMaxConcurrentProbesPrecedence() (+11 more)

### Community 107 - "establishedCRD"
Cohesion: 0.25
Nodes (12): establishedCRD(), NewArgoCDEngine(), argoApp(), argocdHealthyObjects(), runArgoCD(), TestArgoCD_AbsentClusterFails(), TestArgoCD_ApplicationStateRollup(), TestArgoCD_HealthyWithSyncedApplication() (+4 more)

### Community 108 - "runArgoCD"
Cohesion: 0.22
Nodes (14): argocdDeployment(), Engine, WorkloadCheck, NewArgoCDEngine(), argoApp(), argocdHealthyObjects(), clientObject, runArgoCD() (+6 more)

### Community 109 - "assertFamily"
Cohesion: 0.26
Nodes (16): assertFamily(), Engine, NewIstioEngine(), clientObject, istioAmbientObjects(), istioCRDObjects(), istiodControlPlane(), istioHealthyObjects() (+8 more)

### Community 110 - "New"
Cohesion: 0.30
Nodes (10): New(), newFake(), TestCapabilities(), TestConcurrentAccess(), TestLookup(), TestRegister(), TestRegister_DuplicateAddonType(), TestRegister_PartialFailureLeavesRegistryUnchanged() (+2 more)

### Community 111 - "TestAnnotationStaleness_NamedLock"
Cohesion: 0.25
Nodes (12): daemonSetWithAnnotations(), lockCheck(), lockJSON(), nodeRebootCheck(), nodeWithAnnotations(), runAnnotation(), TestAnnotationStaleness_NamedLock(), TestAnnotationStaleness_NodeList() (+4 more)

### Community 112 - "NewRootCommand"
Cohesion: 0.13
Nodes (18): main(), runMain(), TestMain_BadFlagExitsNonZero(), TestMain_HelpExitsZero(), TestMain_RunsAsMainOnDemand(), context.CancelFunc, github.com/spf13/cobra.Command, NewRootCommand() (+10 more)

### Community 113 - "kedaHealthyObjects"
Cohesion: 0.20
Nodes (12): NewKedaEngine(), conditionCR(), kedaHealthyObjects(), TestKeda_AbsentClusterAllSkipped(), TestKeda_HealthyWithReadyScaledObject(), TestKeda_PausedScaledObjectWarns(), TestKeda_UnreadyScaledObjectFails(), NewVpaEngine() (+4 more)

### Community 114 - "Feature Specification: DNSCheck Resource Contract"
Cohesion: 0.11
Nodes (18): Assumptions, Clarifications, Dependencies, Edge Cases, Feature Specification: DNSCheck Resource Contract, Functional Requirements, Key Entities, Measurable Outcomes (+10 more)

### Community 115 - "Implementation Plan: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.13
Nodes (15): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Phases, Implementation Plan: Cadence-Aware Staleness Semantics for ClusterHealth, Phase A — Staleness derivation (the reported defect), Phase B — Cadence publication, Phase C — Shipped alerting rules (+7 more)

### Community 116 - "Options"
Cohesion: 0.39
Nodes (7): DNSCheckOptions, flagBinding, MetricsOptions, Options, TracingOptions, WebhookOptions, bindings()

### Community 117 - ".Evaluate"
Cohesion: 0.19
Nodes (13): PodProjectionCheck, k8s.io/api/core/v1.Container, capNames(), containerHasEnv(), formatSelector(), PodProjectionCheck, hasProjectedTokenVolume(), TestPodProjection_CapNames() (+5 more)

### Community 118 - "properties"
Cohesion: 0.12
Nodes (17): type, type, type, type, type, allOf, $comment, properties (+9 more)

### Community 119 - "test/e2e/healthreport_helpers_test.go"
Cohesion: 0.20
Nodes (12): checkResult, eventList, healthReport, healthReportList, addonCheckLastResult(), addonCheckReadyTrue(), dumpAddonCheckDiagnostics(), latestHealthReport() (+4 more)

### Community 120 - "MustEngine"
Cohesion: 0.11
Nodes (26): k8s.io/api/batch/v1.CronJob, k8s.io/api/core/v1.ConfigMap, TestMustEngine_PanicsOnInvalid(), configMap(), ConfigMapCheck, runConfigMap(), TestConfigMapCheck(), TestConfigMapCheck_AbsentInheritsOptional() (+18 more)

### Community 121 - "Sweeper"
Cohesion: 0.27
Nodes (7): k8s.io/api/core/v1.PodPhase, sigs.k8s.io/controller-runtime/pkg/client.Reader, orphanSince(), probePodSelector(), probeShaped(), terminalPhase(), Sweeper

### Community 122 - "Core Principles"
Cohesion: 0.12
Nodes (16): Core Principles, Development Workflow & Quality Gates, Engineering Constraints, Fathom Constitution, Fathom-Specific Constraints, Governance, I. Explicit State Over Implicit Behavior, II. Git Is the Durable Desired-State Boundary (+8 more)

### Community 123 - ".Update"
Cohesion: 0.20
Nodes (7): createAddonCheckWithStatusForObservability(), TestClusterHealthSelectsHealthCheck(), TestHealthCheckEventHandler(), conflictOnceStatusClient, conflictOnceStatusWriter, countingStatusWriter, sigs.k8s.io/controller-runtime/pkg/client.SubResourceWriter

### Community 124 - "5. ClusterHealth staleness is the stalest child, and is a signal not a verdict"
Cohesion: 0.14
Nodes (13): 5. ClusterHealth staleness is the stalest child, and is a signal not a verdict, Accepted tradeoff: detection latency in mixed aggregates, Alternative rejected: add a parallel field, Consequences, Context and Problem Statement, Decision, Rationale, References (+5 more)

### Community 125 - "dnscheck_validation_test.go"
Cohesion: 0.36
Nodes (10): duration(), envTestAssetsAvailable(), firstEnvTestBinaryDir(), requireAPIServer(), targets(), TestDNSCheckAdmission(), TestDNSCheckDefaults(), TestDNSCheckHasNoPauseField() (+2 more)

### Community 126 - "Execution Steps"
Cohesion: 0.12
Nodes (15): 1. Initialize Convergence Context, 2. Load Artifacts (Progressive Disclosure), 3. Build the Intent Inventory, 4. Assess the Codebase and Classify Findings, 5. Assign Severity, 6. Present the In-Session Findings Summary, 7. Append Convergence Tasks (or report converged), 8. Provide Next Actions (Handoff) (+7 more)

### Community 127 - "DNS Checks"
Cohesion: 0.14
Nodes (13): An explicit resolver is unreachable, Answers exist but the expectation fails, Check cluster DNS and aggregate the result, Choose a resolver, Cluster resolver, DNS Checks, Explicit resolver, Express expectations (+5 more)

### Community 128 - "Contract: `ClusterHealth.Status`"
Cohesion: 0.14
Nodes (12): Contract: `ClusterHealth.Status`, New guarantees, `status.children[]` — bounded and ordered, `status.matchedCount` — contract strengthened, `status.observedAt` — meaning inverted, `status.result` — explicitly unchanged, Unchanged, Alerting contract (+4 more)

### Community 129 - "internal/metrics/check_metrics_test.go"
Cohesion: 0.21
Nodes (19): github.com/prometheus/client_model/go.MetricFamily, ctrlRegistryGather(), gatherCheckSeries(), gatherDNSTargetSeries(), gatherOneHot(), TestCheckIntervalSeries(), TestCheckIntervalWithdrawnWhenUnresolvable(), TestCheckResultValuesMatchAPIVocabulary() (+11 more)

### Community 130 - "testing.T"
Cohesion: 0.05
Nodes (59): TestWorstResult(), TestAddToScheme(), TestDeepCopyIntoExercise(), TestDeepCopyRoundTrip(), TestGroupVersion(), TestSchemeBuilderRegisterReturnsSelf(), TestHealthReportResultSeverity_EmptyAndUnrecognizedReturnZero(), TestHealthReportResultSeverity_OrderingAcrossEnumValues() (+51 more)

### Community 131 - "quorum-ratio-rollups/internal/nodecert/paths.go"
Cohesion: 0.19
Nodes (14): resolveCertPaths(), TestResolveCertPathsFiltersDisallowed(), AllowedPathPrefixes(), DefaultCertPaths(), FilterAllowedPaths(), isCertFile(), isKubeconfigFile(), MinimalMountDirs() (+6 more)

### Community 132 - "Tasks: Pre-1.0 CRD Validation Hardening"
Cohesion: 0.12
Nodes (16): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Parallel Example: User Story 1, Parallel Opportunities, Phase 1: Setup (+8 more)

### Community 133 - "TestCommittedAddonRolesAreReadOnly"
Cohesion: 0.17
Nodes (12): RBACDeclarer, allowedWrites(), repoRoot(), TestCommittedAddonRolesAreReadOnly(), TestModelGrantsAreJustified(), TestUnjustifiedGrantsCatchesViolations(), AddonServiceAccountName(), IsReadVerb() (+4 more)

### Community 134 - "quorum-ratio-rollups/internal/probe/sweeper_test.go"
Cohesion: 0.32
Nodes (11): probeLabels(), probeShape(), sweepPod(), terminatedAt(), TestSweeper_LabelledPodNotMatchingProbeShapeIsSpared(), TestSweeper_LongRunningProbeIsNotReapedOnCreationAge(), TestSweeper_ReapsPodTerminatedLongerThanMinAge(), TestSweeper_StartSweepsImmediatelyAndStopsOnCancel() (+3 more)

### Community 135 - "Request"
Cohesion: 0.18
Nodes (13): fakeDNSLauncher, k8s.io/api/core/v1.Affinity, k8s.io/api/core/v1.PullPolicy, antiAffinity(), args(), boolPtr(), copyStringMap(), Request (+5 more)

### Community 136 - "operator_rbac_doc_test.go"
Cohesion: 0.28
Nodes (11): docRow, k8s.io/api/rbac/v1.ClusterRole, equalStrings(), loadJustificationRows(), loadOperatorClusterRole(), normalizeSet(), ruleKey(), splitList() (+3 more)

### Community 137 - ".Name"
Cohesion: 0.23
Nodes (9): TestAdapterMetadata(), fakePolicyAdapter, TestEngine_Metadata(), TestEnvoyGateway_AdapterMetadata(), TestExternalDNS_AdapterMetadata(), TestIstio_AdapterMetadata(), TestMetricsServer_AdapterMetadata(), Collect() (+1 more)

### Community 138 - ".Run"
Cohesion: 0.24
Nodes (6): extractResult(), Request, Result, hasTerminationMessage(), Launcher, LaunchError

### Community 139 - "quorum-ratio-rollups/internal/nodecert/scan.go"
Cohesion: 0.32
Nodes (14): minimalKubeconfig, classify(), classifyAll(), daysFromDuration(), errorResult(), parsePEMCertificates(), scanCertFile(), scanDir() (+6 more)

### Community 140 - "Tasks: DNSCheck Resource Contract"
Cohesion: 0.13
Nodes (15): Dependencies, Format: `[ID] [P?] [Story] Description`, Implementation strategy, Parallel opportunities, Path Conventions, Phase 1: Setup, Phase 2: Foundational (Blocking Prerequisites), Phase 4: User Story 2 — Every declared expectation is evaluated (P1) (+7 more)

### Community 141 - "Implementation Plan: DNSCheck Completion"
Cohesion: 0.17
Nodes (12): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: DNSCheck Completion, Implementation Strategy, Phase 0: Research Outcome, Phase 1: Design Outcome, Project Structure (+4 more)

### Community 142 - "internal/adapter/crdutil/crdutil_test.go"
Cohesion: 0.32
Nodes (7): k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1.ConditionStatus, k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1.CustomResourceDefinitionConditionType, crd(), crdWithServed(), TestEstablished(), TestPreferredServedVersion(), TestPreferredServedVersion_IgnoresUnservedEntries()

### Community 143 - "rbac"
Cohesion: 0.14
Nodes (14): type, type, type, annotations, create, name, rbac, serviceAccount (+6 more)

### Community 144 - "enabled"
Cohesion: 0.14
Nodes (14): properties, type, type, type, maximum, minimum, type, config (+6 more)

### Community 145 - "factory"
Cohesion: 0.20
Nodes (8): SAUsername(), TestClientForSetsImpersonationAndMemoizes(), TestSAUsername(), k8s.io/apimachinery/pkg/api/meta.RESTMapper, k8s.io/client-go/rest.Config, sigs.k8s.io/controller-runtime/pkg/client.Options, sync.Mutex, factory

### Community 146 - "internal/adapter/declarative/annotation_test.go"
Cohesion: 0.35
Nodes (10): k8s.io/api/core/v1.Node, daemonSetWithAnnotations(), AnnotationStalenessCheck, lockCheck(), lockJSON(), nodeRebootCheck(), nodeWithAnnotations(), runAnnotation() (+2 more)

### Community 147 - "internal/adapter/declarative/field_test.go"
Cohesion: 0.38
Nodes (11): gauge(), gaugeCheck(), FieldCheck, runFields(), TestField_InvalidSelectorErrors(), TestField_ListedObjectsScored(), TestField_NoMatchingObjectsSkipped(), TestField_NoMatchUsesAddonAbsencePosture() (+3 more)

### Community 148 - "NewMetricsServerEngine"
Cohesion: 0.32
Nodes (10): Engine, NewMetricsServerEngine(), apiService(), clientObject, msHealthyObjects(), TestMetricsServer_AdapterMetadata(), TestMetricsServer_HealthyPassesAllFamilies(), TestMetricsServer_MissingAPIServiceFails() (+2 more)

### Community 149 - "runProjection"
Cohesion: 0.51
Nodes (10): PodProjectionCheck, optedInPod(), runProjection(), TestPodProjection_AllInjectedPasses(), TestPodProjection_InactivePodsSkipped(), TestPodProjection_MissingEnvOnlyFails(), TestPodProjection_MissingVolumeFails(), TestPodProjection_NoOptedInPodsSkipped() (+2 more)

### Community 150 - ".agents/skills/speckit-plan/SKILL.md"
Cohesion: 0.18
Nodes (10): Completion Report, Done When, Key rules, Mandatory Post-Execution Hooks, Outline, Phase 0: Outline & Research, Phase 1: Design & Contracts, Phases (+2 more)

### Community 151 - "crd_compat_gate_test.go"
Cohesion: 0.47
Nodes (12): fixtureCRD(), runCRDCompat(), TestCRDCompatAddedOptionalFieldPasses(), TestCRDCompatAgainstBaseline(), TestCRDCompatAllowlistedChangePassesVisibly(), TestCRDCompatMalformedAllowlistFails(), TestCRDCompatNewCRDSkipped(), TestCRDCompatNoChangePasses() (+4 more)

### Community 152 - "Entity: `DNSCheck`"
Cohesion: 0.14
Nodes (14): Changes to existing types, `cmd/probe` — dns mode flags, `DNSCheckSpec`, `DNSCheckStatus`, `DNSResolver`, `DNSTarget`, `DNSTargetResult`, Entity: `DNSCheck` (+6 more)

### Community 153 - "HealthReportResult"
Cohesion: 0.06
Nodes (59): DNSCheck, DNSCheckSpec, DNSCheckStatus, DNSResolver, DNSTarget, DNSTargetResult, DNSCheckReconciler, dnsPair (+51 more)

### Community 154 - ".agents/skills/speckit-specify/SKILL.md"
Cohesion: 0.18
Nodes (10): Completion Report, Done When, For AI Generation, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, Quick Guidelines, Section Requirements (+2 more)

### Community 155 - "Repository Guidelines"
Cohesion: 0.14
Nodes (13): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Graphify, Project Structure & Module Organization (+5 more)

### Community 156 - "values.schema.json"
Cohesion: 0.15
Nodes (12): properties, required, type, nodeAgent, required, $schema, title, type (+4 more)

### Community 157 - "properties"
Cohesion: 0.15
Nodes (13): type, type, type, interval, labels, namespace, scrapeTimeout, serviceMonitor (+5 more)

### Community 158 - ".agents/skills/speckit-tasks/SKILL.md"
Cohesion: 0.18
Nodes (10): Checklist Format (REQUIRED), Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Phase Structure, Pre-Execution Checks, Task Generation Rules (+2 more)

### Community 159 - "Feature Specification: [FEATURE NAME]"
Cohesion: 0.15
Nodes (12): Assumptions, Edge Cases, Feature Specification: [FEATURE NAME], Functional Requirements, Key Entities *(include if feature involves data)*, Measurable Outcomes, Requirements *(mandatory)*, Success Criteria *(mandatory)* (+4 more)

### Community 160 - "programmableAdapter"
Cohesion: 0.09
Nodes (8): healthReportCount(), absentReportingAdapter, countingStatusClient, fakeAddonAdapter, programmableAdapter, versionReportingAdapter, healthReportCount(), Capabilities

### Community 161 - "quorum-ratio-rollups/internal/adapter/declarative/field_test.go"
Cohesion: 0.42
Nodes (10): gauge(), gaugeCheck(), runFields(), TestField_InvalidSelectorErrors(), TestField_ListedObjectsScored(), TestField_NoMatchingObjectsSkipped(), TestField_NoMatchUsesAddonAbsencePosture(), TestField_ScoreObject() (+2 more)

### Community 162 - "k8s.io/apimachinery/pkg/runtime.Object"
Cohesion: 0.16
Nodes (8): ClusterHealth, HealthReportList, NodeCertificateCheckList, k8s.io/apimachinery/pkg/runtime.Object, TestDeepCopyRoundTrip(), ClusterHealth, HealthReportList, NodeCertificateCheckList

### Community 163 - "GitHub Copilot Instructions for Fathom"
Cohesion: 0.17
Nodes (11): Codebase Shape, Commit and Branch Guidance, Documentation Expectations, GitHub Copilot Instructions for Fathom, Go and Repository Conventions, Knowledge Graph (`graphify-out/`), Pull Request Instructions, Safety Rules (+3 more)

### Community 164 - "deploymentInNamespace"
Cohesion: 0.17
Nodes (20): deploymentInNamespace(), Engine, NewExternalDNSEngine(), extdnsHealthyObjects(), clientObject, TestExternalDNS_AdapterMetadata(), TestExternalDNS_DeploymentNameThresholdOverride(), TestExternalDNS_HealthyPassesAllFamilies() (+12 more)

### Community 165 - "TestEnvoyGateway_HealthyAndNoGatewaysSkipped"
Cohesion: 0.27
Nodes (9): Engine, NewEnvoyGatewayEngine(), egHealthyObjects(), gatewayObject(), clientObject, TestEnvoyGateway_AdapterMetadata(), TestEnvoyGateway_GatewayConditionScoring(), TestEnvoyGateway_HealthyAndNoGatewaysSkipped() (+1 more)

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
Cohesion: 0.27
Nodes (8): esoDeployment(), Engine, WorkloadCheck, NewExternalSecretsEngine(), esoHealthyObjects(), clientObject, TestExternalSecrets_HealthyAndEmptySyncSkipped(), TestExternalSecrets_MissingDeploymentFails()

### Community 173 - "time.Duration"
Cohesion: 0.19
Nodes (20): time.Duration, runDNSProbe(), addonCheckInterval(), addonCheckTimeout(), cadenceClampMessages(), clampCadence(), durp(), findCondition() (+12 more)

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

### Community 181 - "Feature Specification: DNSCheck Completion"
Cohesion: 0.18
Nodes (11): Assumptions and Dependencies, Decisions and Tradeoffs, Feature Specification: DNSCheck Completion, Functional Requirements, Key Entities, Measurable Outcomes, Out of Scope, Overview (+3 more)

### Community 182 - "RBAC / Least-Privilege Review — Fathom operator (commit cb845dd)"
Cohesion: 0.20
Nodes (9): Positive observations (examined, no defect), RBAC-1: Operator ClusterRole grants create/update/patch/delete on the three primary CRDs the reconcilers never write (medium), RBAC-2: Runtime node-agent ClusterRole allows get/update of ANY ConfigMap in the operator namespace, though the agent writes exactly one (medium), RBAC-3: Node-agent NetworkPolicy egress permits TCP/443 + TCP/6443 to ANY destination, not just the API server (medium), RBAC-4: Operator holds cluster-wide create/get/list/update/watch on ClusterRoles and RoleBindings with no resourceNames (high), RBAC-5: Operator holds cluster-wide apps/daemonsets create/update/delete — a DaemonSet-on-every-node takeover primitive (high), RBAC-6: Impersonated addon ServiceAccounts hold cluster-wide pods create/delete, granting the operator (via impersonation) a pod-create capability its own role lacks (medium), RBAC-7: Unused finalizer subresource grants across all four reconcilers (low) (+1 more)

### Community 183 - "Quickstart: Validating Cadence-Aware Staleness"
Cohesion: 0.20
Nodes (9): Documentation checks, Full gate, Prerequisites, Quickstart: Validating Cadence-Aware Staleness, Scenario 1 — A frozen child cannot hide behind a healthy sibling (US1), Scenario 2 — A healthy slow child does not poison its aggregate (US2), Scenario 3 — One rule is correct at every cadence (US3), Scenario 4 — Never-observed and clock skew (+1 more)

### Community 184 - ".IsReadOnly"
Cohesion: 0.50
Nodes (4): hasResource(), hasVerb(), TestRBACRulesDeclaresDryRunException(), TestRBACRulesDeclaresProbeException()

### Community 185 - "NewEngine"
Cohesion: 0.19
Nodes (10): Engine, Engine, NewEngine(), TestNewEngine_Validation(), validVersionSource(), validWorkloadKind(), TestNewEngine_PodProjectionValidation(), TestNewEngine_VersionValidation() (+2 more)

### Community 186 - "TestCommittedAddonRolesAreReadOnly"
Cohesion: 0.48
Nodes (6): allowedWrites(), repoRoot(), TestCommittedAddonRolesAreReadOnly(), TestUnjustifiedGrantsCatchesViolations(), IsReadVerb(), writeKey

### Community 187 - "msHealthyObjects"
Cohesion: 0.47
Nodes (7): NewMetricsServerEngine(), apiService(), msHealthyObjects(), TestMetricsServer_HealthyPassesAllFamilies(), TestMetricsServer_MissingAPIServiceFails(), TestMetricsServer_MissingDeploymentFails(), TestMetricsServer_UnavailableAPIServiceFails()

### Community 188 - "podInNamespace"
Cohesion: 0.15
Nodes (24): assertFamily(), TestRun_EmptyClusterSkippedFamilyAttribution(), configMap(), runConfigMap(), TestConfigMapCheck(), TestConfigMapCheck_NoAPIVersionAssertionPassesAnyYAML(), NewDeschedulerEngine(), TestDescheduler_HealthyCronJobMode() (+16 more)

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

### Community 194 - "Phase 0 Research: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.20
Nodes (10): Phase 0 Research: Cadence-Aware Staleness Semantics for ClusterHealth, R1 — Effective cadence already exists as three parallel resolvers, R2 — `observeCheck` is a single seam; series deletion is a matching obligation, R3 — The stalest timestamp needs no timer, which is what makes D1 implementable, R4 — C1: the aggregate cannot be cadence-aware without `HealthCheck`, R5 — C2: the overdue multiplier does not belong in operator config, R6 — Staleness derivation and the never-observed case, R7 — Bounding `Children[]` is an incompatible narrowing (+2 more)

### Community 195 - "ParseAddonSelection"
Cohesion: 0.25
Nodes (7): TestE2EShardPlannerKnowsEveryOptInAddon(), TestE2E(), CoreAddons(), OptInAddons(), ParseAddonSelection(), TestHelmfileLabelsMatchAddonTiers(), TestParseAddonSelection()

### Community 196 - "main"
Cohesion: 0.38
Nodes (5): main(), runMain(), TestMain_ExitsNonZeroOnWriteError(), TestMain_RunsAsMainOnDemand(), TestMain_WritesArtifacts()

### Community 197 - "healthcheck_controller.go"
Cohesion: 0.12
Nodes (22): failingHealthCheckTargetClient, HealthCheckReconciler, healthCheckTargetHandler, healthCheckTargetReader, healthCheckTargetRegistry, healthCheckTargetSnapshot, transientAddonCheckGetClient, k8s.io/apimachinery/pkg/types.NamespacedName (+14 more)

### Community 198 - ".agents/skills/speckit-checklist/SKILL.md"
Cohesion: 0.25
Nodes (7): Anti-Examples: What NOT To Do, Checklist Purpose: "Unit Tests for English", Example Checklist Types & Sample Items, Execution Steps, Post-Execution Checks, Pre-Execution Checks, User Input

### Community 199 - "BuiltInAdapters"
Cohesion: 0.11
Nodes (21): Setupper, sigs.k8s.io/controller-runtime.Manager, sigs.k8s.io/controller-runtime/pkg/cache.Options, sigs.k8s.io/controller-runtime/pkg/healthz.Checker, sync/atomic.Bool, ClientFactory, New(), BuildAdapterRegistry() (+13 more)

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

### Community 206 - "Phase 1 Data Model: Cadence-Aware Staleness Semantics"
Cohesion: 0.25
Nodes (7): `children[]` truncation, Derived quantity: effective cadence, Entity: `ClusterHealth.Status`, Entity: `HealthCheck` (read-only in this feature), `observedAt` derivation, Phase 1 Data Model: Cadence-Aware Staleness Semantics, State transitions

### Community 207 - "Data Model: DNSCheck Completion"
Cohesion: 0.25
Nodes (7): CheckTargetRef, Data Model: DNSCheck Completion, Normalized target snapshot, Source relationships, State transitions, Target handler, Target identity

### Community 208 - "speckit-clarify/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 209 - "speckit-implement/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 210 - "Research: DNSCheck Completion"
Cohesion: 0.25
Nodes (8): R1: Projection architecture, R2: API-version identity, R3: Snapshot and failure semantics, R4: Source status normalization, R5: Watch wiring, R6: RBAC and generated artifacts, R7: Validation scope, Research: DNSCheck Completion

### Community 211 - ".agents/skills/speckit-clarify/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 212 - ".checkCRD"
Cohesion: 0.23
Nodes (9): Established(), PreferredServedVersion(), crd(), crdWithServed(), TestEstablished(), TestPreferredServedVersion(), TestPreferredServedVersion_IgnoresUnservedEntries(), Established() (+1 more)

### Community 213 - "Phase 1 Data Model: DNSCheck Reconciler"
Cohesion: 0.15
Nodes (13): 1. In-memory entities, 2. Mapping onto the frozen schema, 3. Metric series, 4. Lifecycle and ownership, 5. State transitions, Conditions, Existing, reused unchanged (FR-032), New (FR-033) (+5 more)

### Community 214 - ".agents/skills/speckit-implement/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 215 - "webhookEntry"
Cohesion: 0.40
Nodes (4): webhookEntry, k8s.io/api/admissionregistration/v1.ServiceReference, appendEntry(), appendEntry()

### Community 216 - "Operator RBAC"
Cohesion: 0.29
Nodes (6): Auxiliary roles shipped alongside the operator, Namespace-scoping analysis, Operator ClusterRole rules, Operator RBAC, Runtime-created RBAC, Why these grants are cluster-scoped

### Community 217 - "Specification Quality Checklist: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.29
Nodes (6): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Cadence-Aware Staleness Semantics for ClusterHealth, Validation Notes

### Community 218 - "runMain"
Cohesion: 0.48
Nodes (5): runMain(), TestMain_BadFlagExitsNonZero(), TestMain_HelpExitsZero(), TestMain_RunsAsMainOnDemand(), TestMain_WritesArtifacts()

### Community 219 - "quorum-ratio-rollups/internal/controller/policy_validation_test.go"
Cohesion: 0.33
Nodes (6): fakeAdvertisingAdapter, badSelector(), checkWithPolicy(), TestValidateAddonCheckPolicy(), TestValidateAddonCheckPolicy_DeterministicOrder(), TestValidateAddonCheckPolicy_ThresholdKeys()

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

### Community 230 - "Specification Quality Checklist: DNSCheck Completion"
Cohesion: 0.29
Nodes (5): Content Quality, Decision Depth, Notes, Requirement Completeness, Specification Quality Checklist: DNSCheck Completion

### Community 233 - "Contract: HealthCheck Target Projection"
Cohesion: 0.29
Nodes (7): Aggregation boundary, Compatibility, Contract: HealthCheck Target Projection, Reference failures, Successful projection, Supported references, Watch contract

### Community 239 - "speckit-taskstoissues/SKILL.md"
Cohesion: 0.40
Nodes (4): Outline, Post-Execution Checks, Pre-Execution Checks, User Input

### Community 240 - "Quickstart Validation: DNSCheck Completion"
Cohesion: 0.29
Nodes (7): 1. Verify generated contracts, 2. Verify projection and watch behavior, 3. Verify the real DNS aggregation chain, 4. Verify documentation and licensing, Expected completion signal, Prerequisites, Quickstart Validation: DNSCheck Completion

### Community 241 - "Feature Specification: DNSCheck Reconciler"
Cohesion: 0.15
Nodes (13): Assumptions, Clarifications, Feature Specification: DNSCheck Reconciler, Functional Requirements, Inherited Requirements, Key Entities, Measurable Outcomes, Numbering (+5 more)

### Community 242 - "bindAddress"
Cohesion: 0.40
Nodes (5): type, properties, type, bindAddress, healthProbe

### Community 243 - "quorum-ratio-rollups/internal/nodecert/scan_test.go"
Cohesion: 0.29
Nodes (12): Scan(), makeCertPEM(), TestScanBundleEmitsPerCert(), TestScanDefaultsWhenNoPaths(), TestScanDirectoryRecursiveAndIgnoresNonCerts(), TestScanKubeconfig(), TestScanMissingPathIsSilent(), TestScanPermissionDeniedIsSkippedNotError() (+4 more)

### Community 245 - "scripts/coverage_gate_test.go"
Cohesion: 0.83
Nodes (3): normalizeShell(), stripShellComment(), TestCoverageGateSkipsNoPackages()

### Community 246 - ".agents/skills/speckit-constitution/SKILL.md"
Cohesion: 0.33
Nodes (5): Outline, Post-Execution Checks, Pre-Execution Checks, Scope Guard, User Input

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

### Community 253 - "Phase 0 Research: DNSCheck Reconciler"
Cohesion: 0.17
Nodes (12): D10 — Configuration needs an integer binding, which does not exist yet, D1 — Probe pods carry no ownerReference today, and DNSCheck is the first kind that can give them one, D2 — Probe pod reads must not go through the manager cache, D3 — Fan-out is bounded-concurrency goroutines, not sequential iteration, D4 — One run deadline, derived per-pair bounds, D5 — Cadence is anchored to run start, with a floor, D6 — Unreached pairs are `Unknown`; truncation is also a condition, D7 — A new per-target gauge, rebuilt by delete-then-set (+4 more)

### Community 261 - "Contract: DNSCheck Metrics, Events, and RBAC"
Cohesion: 0.20
Nodes (10): 1. Check-level metrics (inherited, FR-032), 2. Per-target metric (new, FR-033), 3. Events (inherited, FR-112), 4. Conditions, 5. RBAC (FR-115 / inherited FR-037), Contract: DNSCheck Metrics, Events, and RBAC, Current state, Required (+2 more)

### Community 266 - "normalizeShell"
Cohesion: 0.83
Nodes (3): normalizeShell(), stripShellComment(), TestCoverageGateSkipsNoPackages()

### Community 267 - "extraArgs"
Cohesion: 0.50
Nodes (4): items, type, type, extraArgs

### Community 268 - "Contract: DNSCheck Reconcile Loop"
Cohesion: 0.20
Nodes (9): Contract: DNSCheck Reconcile Loop, Explicitly out of contract, Failure handling, Invocation, Ordered sequence, Outcome classification, Ownership, Status write discipline (+1 more)

### Community 269 - "User Scenarios & Testing *(mandatory)*"
Cohesion: 0.33
Nodes (6): Edge Cases, User Scenarios & Testing *(mandatory)*, User Story 1 - DNS contributes to cluster health (Priority: P1), User Story 2 - Every advertised target kind works (Priority: P1), User Story 3 - Invalid references fail explicitly and safely (Priority: P2), User Story 4 - DNSCheck is straightforward to author and diagnose (Priority: P2)

### Community 272 - "Contract: CRD Admission Validation"
Cohesion: 0.50
Nodes (4): AddonCheck `spec.policy`, Cadence floors (both kinds), Compatibility guarantee (FR-008), Contract: CRD Admission Validation

### Community 273 - "Implementation Strategy"
Cohesion: 0.50
Nodes (4): Implementation Strategy, Incremental Delivery, MVP First (User Story 1 Only), Single-Branch Note

### Community 274 - ".agents/skills/speckit-taskstoissues/SKILL.md"
Cohesion: 0.40
Nodes (4): Outline, Post-Execution Checks, Pre-Execution Checks, User Input

### Community 275 - "test/e2e/observability_test.go"
Cohesion: 0.18
Nodes (7): eventRow, getMetricsOutput(), tokenRequest, getMetricsOutput(), serviceAccountToken(), applyManifest(), scrapeOperatorMetrics()

### Community 276 - "Problem"
Cohesion: 0.40
Nodes (5): A naive fix is wrong, Four sources currently disagree, Problem, The cadence gap is already causing a second, live problem, The metric is wrong too, not just the status

### Community 309 - "Adapter"
Cohesion: 0.43
Nodes (3): sync.RWMutex, Registry, Adapter

### Community 310 - "New"
Cohesion: 0.50
Nodes (3): New(), TestAdapterMetadata(), TestRun_EmitsSpan()

### Community 312 - "Implementation Plan: DNSCheck Reconciler"
Cohesion: 0.25
Nodes (8): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: DNSCheck Reconciler, Project Structure, Source Code (repository root), Summary, Technical Context

### Community 313 - "SAUsername"
Cohesion: 0.67
Nodes (3): SAUsername(), TestClientForSetsImpersonationAndMemoizes(), TestSAUsername()

### Community 315 - "Quickstart Validation: DNSCheck Reconciler"
Cohesion: 0.29
Nodes (7): Level 1 — unit, no cluster, Level 2 — envtest, real API server, faked launcher, Level 3 — e2e on Kind, Level 4 — gates before the PR is ready, Manual review items no gate catches, Prerequisites, Quickstart Validation: DNSCheck Reconciler

### Community 316 - "User Scenarios & Testing *(mandatory)*"
Cohesion: 0.33
Nodes (6): Edge Cases, User Scenarios & Testing *(mandatory)*, User Story 1 - A declared check produces a verdict on its cadence (Priority: P1), User Story 2 - An operator sees which name failed, not just that the check failed (Priority: P2), User Story 3 - Result history is recorded without noise (Priority: P3), User Story 4 - Evaluation workloads never outlive their check (Priority: P3)

### Community 318 - "Implementation Strategy"
Cohesion: 0.50
Nodes (4): If the #149 deadline gets tight, Implementation Strategy, Incremental Delivery, MVP — User Story 1 only (T001–T016)

### Community 319 - "dnscheck_test.go"
Cohesion: 0.60
Nodes (3): dnsCheckPod, dnsCheckPodList, listProbePods()

### Community 321 - "Specification Quality Checklist: DNSCheck Reconciler"
Cohesion: 0.40
Nodes (5): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: DNSCheck Reconciler

### Community 325 - "quorum-ratio-rollups/internal/adapter/declarative/podprojection_test.go"
Cohesion: 0.58
Nodes (9): optedInPod(), runProjection(), TestPodProjection_AllInjectedPasses(), TestPodProjection_InactivePodsSkipped(), TestPodProjection_MissingEnvOnlyFails(), TestPodProjection_MissingVolumeFails(), TestPodProjection_NoOptedInPodsSkipped(), TestPodProjection_PolicyNamespacesScopeTheScan() (+1 more)

### Community 326 - "Implementation Strategy"
Cohesion: 0.50
Nodes (4): Implementation Strategy, Incremental Delivery, MVP First, Suggested PR Scope

## Knowledge Gaps
- **956 isolated node(s):** `post-install.sh script`, `common.sh script`, `$schema`, `title`, `type` (+951 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **29 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `New()` connect `New` to `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `newFakeClient`, `.DeepCopy`, `quorum-ratio-rollups/internal/probe/sweeper_test.go`, `.DeepCopy`, `.DeepCopy`, `.Name`, `.DeepCopy`, `observeCheck`, `Family`, `.DeepCopy`, `quorum-ratio-rollups/cmd/probe/main_test.go`, `.Register`, `.Run`, `.DeepCopy`, `factory`, `Init`, `newScheme`, `k8s.io/apimachinery/pkg/runtime.Object`, `.DeepCopy`, `.DeepCopy`, `assertHasDetail`, `podInNamespace`, `.DeepCopyInto`, `join`, `TestAdapterClient`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.Update`, `.DeepCopy`, `.DeepCopy`?**
  _High betweenness centrality (0.042) - this node is a cross-community bridge._
- **Why does `CheckResult` connect `CheckResult` to `Family`, `.Run`, `internal/adapter/declarative/annotation_test.go`, `internal/adapter/declarative/field_test.go`, `runProjection`, `time.Duration`, `internal/adapter/certmanager/adapter.go`, `internal/adapter/certmanager/adapter_test.go`, `.Run`, `sigs.k8s.io/controller-runtime/pkg/client.Client`, `.Run`, `.checkMetricsEndpoint`, `newFakeClient`, `assertHasDetail`, `k8s.io/api/core/v1.Pod`, `addoncheck_controller.go`, `assertHasOutcome`, `.checkCRD`, `.Evaluate`, `FamilyRatioVerdict`, `assertFamily`, `.Evaluate`, `MustEngine`?**
  _High betweenness centrality (0.042) - this node is a cross-community bridge._
- **Why does `Family` connect `Family` to `testing.T`, `CheckResult`, `.Name`, `internal/adapter/declarative/field_test.go`, `NewMetricsServerEngine`, `runProjection`, `programmableAdapter`, `deploymentInNamespace`, `TestEnvoyGateway_HealthyAndNoGatewaysSkipped`, `TestExternalSecrets_HealthyAndEmptySyncSkipped`, `internal/adapter/certmanager/adapter.go`, `internal/adapter/certmanager/adapter_test.go`, `FamilyDefinition`, `sigs.k8s.io/controller-runtime/pkg/client.Client`, `.checkMetricsEndpoint`, `newFakeClient`, `assertHasDetail`, `k8s.io/api/core/v1.Pod`, `addoncheck_controller.go`, `assertHasOutcome`, `kedaHealthyObjects`, `FamilyRatioVerdict`, `quorum-ratio-rollups/internal/controller/policy_validation_test.go`, `runArgoCD`, `assertFamily`, `New`, `MustEngine`?**
  _High betweenness centrality (0.023) - this node is a cross-community bridge._
- **Are the 108 inferred relationships involving `assertHasOutcome()` (e.g. with `TestAnnotationStaleness_NamedLock()` and `TestAnnotationStaleness_NodeList()`) actually correct?**
  _`assertHasOutcome()` has 108 INFERRED edges - model-reasoned connections that need verification._
- **Are the 110 inferred relationships involving `assertHasOutcome()` (e.g. with `TestAnnotationStaleness_NamedLock()` and `TestAnnotationStaleness_NodeList()`) actually correct?**
  _`assertHasOutcome()` has 110 INFERRED edges - model-reasoned connections that need verification._
- **What connects `post-install.sh script`, `common.sh script`, `$schema` to the rest of the system?**
  _956 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `.Run` be split into smaller, more focused modules?**
  _Cohesion score 0.07364114552893045 - nodes in this community are weakly interconnected._