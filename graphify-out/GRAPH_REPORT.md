# Graph Report - fathom  (2026-09-20)

## Corpus Check
- 547 files · ~670,693 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 6337 nodes · 15797 edges · 420 communities (390 shown, 30 thin omitted)
- Extraction: 83% EXTRACTED · 17% INFERRED · 0% AMBIGUOUS · INFERRED: 2753 edges (avg confidence: 0.83)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `c9def0b2`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- .Run
- quorum-ratio-rollups/cmd/probe/main_test.go
- context.Context
- .DeepCopy
- NodeHealthCheck
- .Evaluate
- Scheduler
- internal/controller/nodecertificatecheck_helpers.go
- Add-on Checks Guide (Adapter Catalog)
- fathom_check_result Gauge (one-hot current result)
- dnscheck_plan.go
- observeCheck
- MustEngine
- deploymentInNamespace
- fakeFactory
- New
- sigs.k8s.io/controller-runtime/pkg/client.Client
- quorum-ratio-rollups/internal/adapter/declarative/engine_test.go
- assertHasOutcome
- NewRootCommand
- Result
- newRootCommand
- quorum-ratio-rollups/internal/probe/pod_test.go
- Init
- Probe/Node-Agent Version Lockstep Gate
- quorum-ratio-rollups/internal/controller/healthreport_idempotency.go
- quorum-ratio-rollups/scripts/version_lockstep_gate_test.go
- internal/adapter/certmanager/adapter.go
- internal/adapter/certmanager/adapter_test.go
- cmd/probe/main_test.go
- DefaultOptions
- FamilyPolicy
- .Run
- HealthReportResult
- assertHasDetail
- resultBudget
- internal/adapter/coredns/adapter.go
- newFakeClient
- assertHasDetail
- Tasks: DNSCheck Reconciler
- properties
- Tasks: fathomctl CLI
- Outcome
- deploymentInNamespace
- .DeepCopyInto
- k8s.io/api/core/v1.Pod
- quorum-ratio-rollups/test/utils/utils.go
- main
- api/v1alpha1/deepcopy_test.go
- .DeepCopy
- common.sh
- Implementation Plan: Pre-1.0 CRD Validation Hardening
- observeCheck
- newScheme
- Implementation Plan: Quorum/Ratio Semantics for Managed-Resource Rollups
- Adversarial Review Findings — v0.5.0 Release Gate (#217)
- github.com/spf13/cobra.Command
- Family
- Tasks: [FEATURE NAME]
- speckit-analyze/SKILL.md
- quorum-ratio-rollups/internal/adapter/declarative/evaluator.go
- kedaHealthyObjects
- Quickstart Validation: Quorum/Ratio Rollups
- Tasks: DNSCheck Completion
- cli/run.go
- nodeagent_metrics_test.go
- time.Duration
- Run
- .agents/skills/speckit-analyze/SKILL.md
- join
- test/utils/utils.go
- k8s.io/apimachinery/pkg/apis/meta/v1.TypeMeta
- Tasks: Adversarial Codebase Review for the v0.5.0 Release Gate
- Tasks: Cadence-Aware Staleness Semantics for ClusterHealth
- FamilyRatioVerdict
- Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth
- dnscheck_types.go
- Execution Steps
- CertResult
- TestAdapterClient
- BuiltInAdapters
- image
- Load
- AddonDefinitionBinding
- .Run
- assertFamily
- cli/version.go
- PolicyRule
- TestCommittedAddonRolesAreReadOnly
- HealthReport
- Feature Specification: DNSCheck Resource Contract
- Implementation Plan: Cadence-Aware Staleness Semantics for ClusterHealth
- healthcheck_controller.go
- cmd/node-agent/main.go
- properties
- test/e2e/healthreport_helpers_test.go
- runProjection
- Guard
- Core Principles
- clampCadence
- 5. ClusterHealth staleness is the stalest child, and is a signal not a verdict
- requireAPIServer
- Execution Steps
- DNS Checks
- Contract: `ClusterHealth.Status`
- TestRuntimeInventoryMatchesOperator
- testing.T
- quorum-ratio-rollups/internal/controller/nodecertificatecheck_helpers.go
- Tasks: Pre-1.0 CRD Validation Hardening
- .Name
- internal/adapter/rbacgen/rbacgen.go
- fathomctl Reference
- operator_rbac_doc_test.go
- Sweeper
- runReports
- AddonCheck
- Tasks: DNSCheck Resource Contract
- Implementation Plan: DNSCheck Completion
- Research: fathomctl CLI
- rbac
- enabled
- NewBudget
- EnsureCompatible
- quorum-ratio-rollups/internal/probe/sweeper_test.go
- NewMetricsServerEngine
- internal/metrics/check_metrics_test.go
- .agents/skills/speckit-plan/SKILL.md
- establishedCRD
- Entity: `DNSCheck`
- crd_compat_gate_test.go
- .agents/skills/speckit-specify/SKILL.md
- Repository Guidelines
- values.schema.json
- properties
- .agents/skills/speckit-tasks/SKILL.md
- Feature Specification: [FEATURE NAME]
- Capabilities
- Fathom Architecture
- runArgoCD
- GitHub Copilot Instructions for Fathom
- truncateChildren
- Feature Specification: Node-agent metrics security
- Tasks: Quorum/Ratio Semantics for Managed-Resource Rollups
- Feature Specification: Adversarial Codebase Review for the v0.5.0 Release Gate
- speckit-plan/SKILL.md
- speckit-specify/SKILL.md
- speckit-tasks/SKILL.md
- assertPodNetworkHealthAgentSecurity
- SetRunningInClusterForTest
- 6. Serve node metrics through the authenticated operator endpoint
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
- TestExternalSecrets_HealthyAndEmptySyncSkipped
- validDefinition
- nodehealth/paths.go
- Authoring an Adapter Guide
- Tasks: Node-agent metrics security
- Implementation Plan: [FEATURE]
- Entities
- Phase 0 Research: Adversarial Codebase Review for the v0.5.0 Release Gate
- Quickstart: Validating the DNSCheck Resource Contract
- speckit-checklist/SKILL.md
- Phase 0 Research: Cadence-Aware Staleness Semantics for ClusterHealth
- 010-node-agent-metrics-security/contracts/metrics.md
- Quickstart Validation: fathomctl CLI
- assertHasOutcome
- .agents/skills/speckit-checklist/SKILL.md
- .detectAndGateVersion
- Budget
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
- newFakeClient
- Phase 1 Data Model: DNSCheck Reconciler
- .agents/skills/speckit-implement/SKILL.md
- Implementation Plan: fathomctl CLI
- fathomctl
- Specification Quality Checklist: Cadence-Aware Staleness Semantics for ClusterHealth
- payloads.go
- quorum-ratio-rollups/internal/controller/policy_validation_test.go
- Specification Quality Checklist: Adversarial Codebase Review for the v0.5.0 Release Gate
- Specification Quality Checklist: DNSCheck Resource Contract
- Contract: Probe `dns` Mode CLI
- speckit-constitution/SKILL.md
- Node Health Checks
- Contract: CRD Schema-Compatibility Gate
- Deliverable Contracts: Findings Report, Refuted Record, Coverage Statement
- Security review candidates — Fathom (commit cb845dd)
- Specification Quality Checklist: DNSCheck Completion
- CheckResult
- Feature Specification: fathomctl CLI
- Contract: HealthCheck Target Projection
- .DeepCopyObject
- quorum-ratio-rollups/internal/nodecert/paths.go
- TestCommittedAddonRolesAreReadOnly
- Research: Node-agent metrics security
- addondefinition_payloads.go
- speckit-taskstoissues/SKILL.md
- Quickstart Validation: DNSCheck Completion
- Feature Specification: DNSCheck Reconciler
- bindAddress
- agentResourceName
- check-version-lockstep.sh
- scripts/coverage_gate_test.go
- .agents/skills/speckit-constitution/SKILL.md
- Security Policy
- [CHECKLIST TYPE] Checklist: [FEATURE NAME]
- Contract: Runtime Clamp Signal
- Contract: DNSCheck Admission Validation
- Phase 3: User Story 1 — Declare DNS intent and have it validated (P1)
- Phase 0 Research: DNSCheck Reconciler
- Adapter
- AddonDefinition
- Implementation Plan: Runtime Addon Definitions
- .DeepCopy
- NodeReport
- quorum-ratio-rollups/internal/nodecert/scan_test.go
- RFC 1. AddonDefinition as a CRD — make adapters installable, not compiled in
- Contract: DNSCheck Metrics, Events, and RBAC
- Implementation Plan: Node-agent metrics security
- main
- quorum-ratio-rollups/internal/adapter/declarative/istio_test.go
- .DeepCopyInto
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
- .Update
- .Evaluate
- .waitForRunWithDeadlineGrowth
- NewRootCommand
- Implementation Plan: DNSCheck Reconciler
- DefinitionCheck
- Quickstart Validation: DNSCheck Reconciler
- User Scenarios & Testing *(mandatory)*
- quorum-ratio-rollups/internal/adapter/crdutil/crdutil_test.go
- webhookEntry
- dnscheck_test.go
- Specification Quality Checklist: DNSCheck Reconciler
- Rationale: in-process interface over gRPC/OCI/plugin loaders
- TestDescheduler_HealthyDeploymentMode
- Implementation Strategy
- CLI-side types (`internal/cli`, unexported)
- .DeepCopy
- Contract: fathomctl command surface
- Data Model: fathomctl CLI
- quorum-ratio-rollups/internal/adapter/rbacgen/rbacgen.go
- Specification Quality Checklist: fathomctl CLI
- Contract: on-demand run trigger (operator side)
- User Scenarios & Testing *(mandatory)*
- fathomctl_test.go
- .DeepCopy
- New
- Consequences
- k8s.io/api/apps/v1.Deployment
- MustEngine
- internal/app/run_happy_test.go
- internal/adapter/declarative/field_test.go
- internal/controller/tracing_test.go
- fathomctl-dist.sh
- Feature Specification: Complete AddonDefinition Design RFC
- k8s.io/apimachinery/pkg/runtime.Scheme
- 010-node-agent-metrics-security/spec.md
- runtime.md
- Feature Specification: Runtime Addon Definitions
- TestEnvoyGateway_HealthyAndNoGatewaysSkipped
- 010-node-agent-metrics-security/data-model.md
- Adversarial review — implementation checkpoint
- quorum-ratio-rollups/internal/nodecert/scan.go
- Options
- 0001-addondefinition-crd.md
- Typed definition wire contract
- .Register
- Validate
- Design Entities
- RFC execution evidence
- Implementation Plan: Complete AddonDefinition Design RFC
- NewExternalDNSEngine
- fixtures.go
- Established
- .IsReadOnly
- quorum-ratio-rollups/internal/app/run_happy_test.go
- .severity
- Research: Complete AddonDefinition Design RFC
- Tasks: Complete AddonDefinition Design RFC
- Implementation execution evidence
- Research and decisions
- Tasks: Runtime Addon Definitions
- DefinitionBindingScope
- sigs.k8s.io/controller-runtime.Manager
- TestEnvoyGateway_HealthyAndNoGatewaysSkipped
- successfulCompiler
- 7. Load typed addon definitions under explicit administrator authority
- .DeepCopy
- RFC Review Contract: AddonDefinition
- .DeepCopy
- Operator RBAC
- runMain
- Runtime acceptance contract
- .DeepCopy
- Specification Quality Checklist: Complete AddonDefinition Design RFC
- Specification Quality Checklist: Runtime Addon Definitions
- Data model
- Validation guide
- Network policies
- .DeepCopy
- TestClientForSetsImpersonationAndMemoizes
- DefinitionReadRule
- SAUsername
- .DeepCopy
- TestRBACRulesDeclaresProbeException
- Implementation Strategy
- nodeHealthStatus
- DefinitionReference
- .DeepCopy
- DNSTargetResult
- .DeepCopy
- .DeepCopy
- .DeepCopy
- .DeepCopy
- Dependencies & Execution Order
- DefinitionTarget
- DNSResolver
- DNSTarget

## God Nodes (most connected - your core abstractions)
1. `assertHasOutcome()` - 142 edges
2. `assertHasOutcome()` - 122 edges
3. `CheckResult` - 108 edges
4. `newFakeClient()` - 80 edges
5. `newFakeClient()` - 76 edges
6. `New()` - 76 edges
7. `Family` - 75 edges
8. `join()` - 67 edges
9. `FamilyPolicy` - 61 edges
10. `assertHasDetail()` - 57 edges

## Surprising Connections (you probably didn't know these)
- `EnsureCompatible()` --semantically_similar_to--> `CRD Maturity Ladder (alpha/beta/GA)`  [INFERRED] [semantically similar]
  pkg/adapter/version.go → docs/reference/api-versioning.md
- `HealthCheck CRD (thin wrapper)` --rationale_for--> `Rationale: uniform wrapper preserves aggregator contract`  [EXTRACTED]
  api/v1alpha1/healthcheck_types.go → docs/adr/0004-healthcheck-as-wrapper.md
- `NewScheme()` --references--> `Fathom Architecture`  [EXTRACTED]
  internal/app/run.go → docs/architecture.md
- `BuiltInAdapters()` --references--> `Fathom Architecture`  [EXTRACTED]
  internal/app/run.go → docs/architecture.md
- `BuiltInAdapters()` --references--> `Authoring an Adapter Guide`  [EXTRACTED]
  internal/app/run.go → docs/authoring-adapters.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **AddonCheck → HealthCheck → ClusterHealth chain with HealthReport history** — readme_addoncheck, readme_healthcheck, readme_clusterhealth, readme_healthreport, readme_aggregation_chain [EXTRACTED 1.00]
- **AddonCheck -> HealthCheck -> ClusterHealth status aggregation chain** — api_v1alpha1_addoncheck_types_addoncheck, api_v1alpha1_healthcheck_types_healthcheck, api_v1alpha1_clusterhealth_types_clusterhealth, api_v1alpha1_healthreport_types_healthreport, controller_addoncheckreconciler, controller_healthcheckreconciler, controller_clusterhealthreconciler [EXTRACTED 1.00]
- **Alerting-grade observability contract surface (gauges, events, alert rules)** — specs_001_alerting_observability_contracts_metrics_fathom_check_result, specs_001_alerting_observability_contracts_metrics_fathom_check_last_run_timestamp_seconds, specs_001_alerting_observability_contracts_metrics_alert_rules, specs_001_alerting_observability_contracts_events_resultchanged, specs_001_alerting_observability_contracts_events_failure_reasons [EXTRACTED 1.00]
- **Probe-pod lifecycle: build, launch, parse, sweep orphans** — internal_probe_pod_pod, internal_probe_launcher_launcher, internal_probe_sweeper_sweeper, docs_architecture_probe_pod_model [EXTRACTED 1.00]
- **Speckit artifact pipeline for feature 001 (spec → plan → tasks → quickstart)** — specs_001_alerting_observability_spec_alerting_observability, specs_001_alerting_observability_plan_alerting_observability_plan, specs_001_alerting_observability_tasks_alerting_observability_tasks, specs_001_alerting_observability_quickstart_validation [EXTRACTED 1.00]

## Communities (420 total, 30 thin omitted)

### Community 0 - ".Run"
Cohesion: 0.06
Nodes (59): TestCountAbsent(), TestFamilyOutcome(), TestOutcomeValid(), TestRun_EmitsSpan(), daemonSetWithAnnotations(), lockCheck(), lockJSON(), nodeRebootCheck() (+51 more)

### Community 1 - "quorum-ratio-rollups/cmd/probe/main_test.go"
Cohesion: 0.11
Nodes (34): runDNS(), runHTTPGet(), runTCPConnect(), runTCPListen(), scanMetricFamilies(), splitComma(), captureResult(), claimAndReleasePort() (+26 more)

### Community 2 - "context.Context"
Cohesion: 0.07
Nodes (54): NodeCertificateCheck, NodeCertificateCheckSpec, NodeCertificateCheckStatus, NodeCertificateCheckReconciler, reportRejection, context.Context, github.com/go-logr/logr.Logger, k8s.io/api/admissionregistration/v1.ValidatingAdmissionPolicySpec (+46 more)

### Community 3 - ".DeepCopy"
Cohesion: 0.34
Nodes (18): deepCopyContract(), fullyPopulatedAddonCheck(), fullyPopulatedClusterHealth(), fullyPopulatedHealthCheck(), fullyPopulatedHealthReport(), fullyPopulatedNodeCertificateCheck(), runtimeObjectContract(), TestDeepCopy_AddonCheck() (+10 more)

### Community 4 - "NodeHealthCheck"
Cohesion: 0.05
Nodes (78): DefaultNodeHealthConditions(), NodeHealthCheck, NodeHealthCheckItem, NodeHealthCheckSpec, NodeHealthCheckStatus, NodeHealthNodeResult, nodeCertRollupDecision, NodeHealthCheckReconciler (+70 more)

### Community 5 - ".Evaluate"
Cohesion: 0.17
Nodes (11): conditionStatus(), policySelector(), resourceAbsent(), FieldCheck, defaultOutcome(), conditionStatus(), policySelector(), resourceAbsent() (+3 more)

### Community 6 - "Scheduler"
Cohesion: 0.15
Nodes (22): container/list.Element, container/list.List, Scheduler, TestRuntimePoolRecoversHandlerPanicAndPreservesPeer(), TestRuntimePoolStopsAndReleasesAllWorkers(), Scheduler, NewScheduler(), admit() (+14 more)

### Community 7 - "internal/controller/nodecertificatecheck_helpers.go"
Cohesion: 0.18
Nodes (16): k8s.io/api/core/v1.Toleration, aggregateNodeReports(), controlPlaneTolerations(), healthReportForNodeCert(), joinPaths(), nodeOutcomeToResult(), putIfNotEmpty(), resolveCertPaths() (+8 more)

### Community 8 - "Add-on Checks Guide (Adapter Catalog)"
Cohesion: 0.27
Nodes (22): AddonCheck CRD, ClusterHealth CRD (aggregate), HealthCheck CRD (thin wrapper), HealthReport CRD (immutable history), HealthReportResult Severity Enum, NodeCertificateCheck CRD, Rationale: CRD history without external storage dependency, Aggregation / Status-Mirror Chain (+14 more)

### Community 9 - "fathom_check_result Gauge (one-hot current result)"
Cohesion: 0.06
Nodes (49): ClusterHealth External Contract (derived only from HealthCheck.status), Cobra+Viper Configuration Model (flag → env → file → default), Run e2e After Major Changes Policy, AGENTS.md Repository Guidelines (CLAUDE.md symlink), SPDX Boilerplate Header, Breaking Change: ClusterHealth Made Cluster-Scoped (0.4.0), DCO Sign-Off Requirement, Contributor Safety Expectations (Bounded Work, Minimal RBAC) (+41 more)

### Community 10 - "dnscheck_plan.go"
Cohesion: 0.14
Nodes (27): dnsPair, dnsVantagePoint, countUnreachedDNSPairs(), dnsCheckShouldPersistReport(), dnsNameserverAddress(), dnsTargetResults(), dnsVantagePoints(), expandDNSPairs() (+19 more)

### Community 11 - "observeCheck"
Cohesion: 0.11
Nodes (27): ctrlRegistryGather(), gatherCheckSeries(), gatherOneHot(), TestDeleteCheckSeries(), TestObserveCheckFlipsResult(), TestObserveCheckOneHotInvariant(), TestObserveCheckSentinels(), checkGaugeValue() (+19 more)

### Community 12 - "MustEngine"
Cohesion: 0.10
Nodes (24): AddonDefinition, Engine, k8s.io/api/batch/v1.CronJob, TestMustEngine_PanicsOnInvalid(), crdAbsenceEngine(), Engine, TestCRD_AbsenceResolution(), cronJob() (+16 more)

### Community 13 - "deploymentInNamespace"
Cohesion: 0.14
Nodes (38): assertNoOutcome(), NewArgoCDEngine(), argoApp(), argocdHealthyObjects(), runArgoCD(), TestArgoCD_AbsentClusterFails(), TestArgoCD_ApplicationStateRollup(), TestArgoCD_HealthyWithSyncedApplication() (+30 more)

### Community 14 - "fakeFactory"
Cohesion: 0.10
Nodes (52): TestDefinitionBindReviewedUIDs(), squash(), TestCadenceRendering(), TestDescribe_NeverRunAndNotFound(), TestDescribe_PerKind(), TestDescribe_StructuredIsUnmodified(), execVerb(), factory (+44 more)

### Community 15 - "New"
Cohesion: 0.10
Nodes (76): adapterWithLauncher(), assertFamily(), assertNoKind(), assertNoTarget(), certManagerResource(), daemonSetWithStatus(), dnsEndpointSlice(), dnsEndpointSliceNamed() (+68 more)

### Community 16 - "sigs.k8s.io/controller-runtime/pkg/client.Client"
Cohesion: 0.05
Nodes (44): failingHealthCheckTargetClient, fakeClientFactory, transientAddonCheckGetClient, collectionPageClient, failingListClient, missingRuntimeAPI, noMatchListClient, oversizedEvidenceClient (+36 more)

### Community 17 - "quorum-ratio-rollups/internal/adapter/declarative/engine_test.go"
Cohesion: 0.27
Nodes (21): NewCiliumEngine(), assertFamily(), assertHasDetail(), assertHasOutcome(), assertNoKind(), assertNoOutcome(), ciliumCRDNames(), daemonSetInNamespace() (+13 more)

### Community 18 - "assertHasOutcome"
Cohesion: 0.18
Nodes (30): assertHasOutcome(), runManaged(), TestCondition_ClusterScopedListsWithoutNamespace(), TestCondition_ConditionStatus(), TestCondition_InvalidAPIVersionErrors(), TestCondition_InvalidSelectorErrors(), TestCondition_ListErrorDescribesNamespaceScope(), TestCondition_ListNameFallsBackToKind() (+22 more)

### Community 19 - "NewRootCommand"
Cohesion: 0.23
Nodes (11): NewRootCommand(), signalContext(), TestSignalContext_PropagatesParentCancellation(), TestSignalContext_SIGINTCancels(), TestSignalContext_SIGTERMCancels(), TestSignalContext_StopReleasesContext(), TestNewRootCommand_BasicWiring(), TestNewRootCommand_HelpDoesNotErrorWithoutKubeconfig() (+3 more)

### Community 20 - "Result"
Cohesion: 0.14
Nodes (20): endAdapterRunSpan(), ThresholdAdvertiser, go.opentelemetry.io/otel/trace.Span, endAdapterRunSpan(), endAdapterRunSpan(), endRunSpan(), endAdapterRunSpan(), endAdapterRunSpan() (+12 more)

### Community 21 - "newRootCommand"
Cohesion: 0.08
Nodes (28): globalOptions, stubClientConfig, io.Reader, k8s.io/client-go/rest.Config, k8s.io/client-go/tools/clientcmd/api.Config, k8s.io/client-go/tools/clientcmd.ClientConfig, k8s.io/client-go/tools/clientcmd.ConfigAccess, defaultClientConfig() (+20 more)

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
Nodes (63): certificateCheck(), certificateDetails(), certManagerComponents(), check(), conditionDetails(), conditionStatus(), conditionType(), daysRemaining() (+55 more)

### Community 52 - "internal/adapter/certmanager/adapter_test.go"
Cohesion: 0.11
Nodes (55): clientObject, k8s.io/api/admissionregistration/v1.MutatingWebhookConfiguration, k8s.io/api/admissionregistration/v1.ValidatingWebhookConfiguration, k8s.io/api/admissionregistration/v1.WebhookClientConfig, k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1.CustomResourceDefinition, New(), assertFamily(), assertHasDetail() (+47 more)

### Community 53 - "cmd/probe/main_test.go"
Cohesion: 0.08
Nodes (60): absoluteDNSName(), result, join(), lookupCNAME(), lookupIPs(), lookupSRV(), main(), missingAnswers() (+52 more)

### Community 54 - "DefaultOptions"
Cohesion: 0.18
Nodes (19): sigs.k8s.io/controller-runtime.Options, sigs.k8s.io/controller-runtime/pkg/certwatcher.CertWatcher, DefaultOptions(), TestValidate(), TestValidate_MultipleErrorsAccumulate(), BuildManagerOptions(), NewScheme(), TestBuildManagerOptions_CertWatchers() (+11 more)

### Community 55 - "FamilyPolicy"
Cohesion: 0.12
Nodes (27): endpointTarget(), scrapeProbePodName(), servicePortDeclared(), servicePortList(), k8s.io/api/core/v1.Service, adapterOutcome(), csvThreshold(), endpointTarget() (+19 more)

### Community 56 - ".Run"
Cohesion: 0.16
Nodes (42): clientObject, k8s.io/api/discovery/v1.EndpointSlice, New(), adapterWithLauncher(), assertHasDetail(), assertHasOutcome(), assertNoOutcome(), dnsEndpointSlice() (+34 more)

### Community 57 - "HealthReportResult"
Cohesion: 0.12
Nodes (22): TestWorstResult(), WorstResult(), DNSCheck, DNSCheckReconciler, dnsPairOutcome, dnsProbeRunner, k8s.io/apimachinery/pkg/apis/meta/v1.OwnerReference, sigs.k8s.io/controller-runtime/pkg/reconcile.Result (+14 more)

### Community 58 - "assertHasDetail"
Cohesion: 0.19
Nodes (29): assertHasDetail(), NewAzureWorkloadIdentityEngine(), healthyAzureWIObjects(), TestAzureWorkloadIdentity_Capabilities(), TestAzureWorkloadIdentity_HealthyClusterAllPass(), TestAzureWorkloadIdentity_NoOptedInPodsProjectionSkipped(), TestAzureWorkloadIdentity_UnpopulatedCABundleFails(), TestIstio_RelocatedControlPlanePasses() (+21 more)

### Community 59 - "resultBudget"
Cohesion: 0.17
Nodes (21): FailureSummary(), Budget, SealResult(), checkResult(), resultBudget(), TestCompletedVerdictsAreEvidenceButEmptyRunIsNot(), TestResultBoundaries(), TestResultCountsUTF8BytesAndJSONEscaping() (+13 more)

### Community 60 - "internal/adapter/coredns/adapter.go"
Cohesion: 0.12
Nodes (19): Adapter, dnsProbeLauncher, adapterOutcome(), check(), deploymentAvailable(), dnsProbePodName(), dnsTargets(), familyForTarget() (+11 more)

### Community 61 - "newFakeClient"
Cohesion: 0.11
Nodes (38): clientObject, daemonSetWithAnnotations(), AnnotationStalenessCheck, lockCheck(), lockJSON(), nodeRebootCheck(), nodeWithAnnotations(), runAnnotation() (+30 more)

### Community 62 - "assertHasDetail"
Cohesion: 0.16
Nodes (33): Engine, NewAzureWorkloadIdentityEngine(), clientObject, healthyAzureWIObjects(), TestAzureWorkloadIdentity_AbsentWebhookFails(), TestAzureWorkloadIdentity_Capabilities(), TestAzureWorkloadIdentity_HealthyClusterAllPass(), TestAzureWorkloadIdentity_NoOptedInPodsProjectionSkipped() (+25 more)

### Community 63 - "Tasks: DNSCheck Reconciler"
Cohesion: 0.08
Nodes (26): Analysis remediation, Critical Path, Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation for User Story 4 (+18 more)

### Community 64 - "properties"
Cohesion: 0.05
Nodes (39): type, type, type, type, type, type, type, type (+31 more)

### Community 65 - "Tasks: fathomctl CLI"
Cohesion: 0.08
Nodes (23): CLI: `run` and `--wait` (#263, contracts/cli-commands.md), Dependencies & Execution Order, Engine: generalise the trigger (#264, contracts/run-trigger.md), Format: `[ID] [P?] [Story] Description`, Implementation Strategy, Incremental Delivery (recommended PR sequence), MVP First (US1 only), Notes (+15 more)

### Community 66 - "Outcome"
Cohesion: 0.10
Nodes (27): CRDCheck, AddonDefinition, Evaluator, FamilyDefinition, Posture, runtimeStep, VersionSource, WorkloadKind (+19 more)

### Community 67 - "deploymentInNamespace"
Cohesion: 0.17
Nodes (34): k8s.io/api/apps/v1.StatefulSet, TestArgoCD_PolicyOverridesWorkloadNames(), TestArgoCD_UnavailableRepoServerFails(), TestDescheduler_InvalidPolicyFails(), assertNoOutcome(), deploymentInNamespace(), podInNamespace(), TestExternalDNS_DeploymentNameThresholdOverride() (+26 more)

### Community 68 - ".DeepCopyInto"
Cohesion: 0.04
Nodes (22): AddonDefinitionBindingSpec, AddonDefinitionBindingStatus, AddonDefinitionSpec, AddonDefinitionStatus, DefinitionAnnotationStaleness, DefinitionCheck, DefinitionCondition, DefinitionConfigMap (+14 more)

### Community 69 - "k8s.io/api/core/v1.Pod"
Cohesion: 0.06
Nodes (56): adapterOutcome(), boundedNodeList(), csvThreshold(), dnsProbePodName(), dnsTargets(), familyForTarget(), int32Threshold(), maxRestartCount() (+48 more)

### Community 70 - "quorum-ratio-rollups/test/utils/utils.go"
Cohesion: 0.13
Nodes (23): TestE2EShardPlannerKnowsEveryOptInAddon(), TestE2E(), AddonSelection, CoreAddons(), GetNonEmptyLines(), GetProjectDir(), InstallPrometheusOperator(), IsPrometheusCRDsInstalled() (+15 more)

### Community 71 - "main"
Cohesion: 0.13
Nodes (21): main(), metricsMux(), parseConfig(), publishGauges(), run(), sanitizeLabelValue(), scanAndPublish(), splitCSV() (+13 more)

### Community 72 - "api/v1alpha1/deepcopy_test.go"
Cohesion: 0.15
Nodes (32): AddonCheck, deepCopyContract(), fullyPopulatedAddonCheck(), fullyPopulatedClusterHealth(), fullyPopulatedDNSCheck(), fullyPopulatedHealthCheck(), fullyPopulatedHealthReport(), fullyPopulatedNodeCertificateCheck() (+24 more)

### Community 73 - ".DeepCopy"
Cohesion: 0.05
Nodes (16): AddonCheck, AddonCheckSpec, AddonDefinition, AddonDefinitionBinding, AddonDefinitionBindingList, AddonDefinitionList, DNSCheck, DNSCheckList (+8 more)

### Community 74 - "common.sh"
Cohesion: 0.08
Nodes (17): check-prerequisites.sh script, check_dir(), check_file(), get_feature_paths(), get_repo_root(), has_jq(), _persist_feature_json(), resolve_specify_init_dir() (+9 more)

### Community 75 - "Implementation Plan: Pre-1.0 CRD Validation Hardening"
Cohesion: 0.07
Nodes (27): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Pre-1.0 CRD Validation Hardening, Complexity Tracking, Constitution Check, Documentation (this feature) (+19 more)

### Community 76 - "observeCheck"
Cohesion: 0.22
Nodes (20): AddonCheckStatus, k8s.io/apimachinery/pkg/apis/meta/v1.Condition, k8s.io/client-go/tools/events.FakeRecorder, acceptedCondition(), TestObserveCheckEmitsCadenceClampedOncePerEpisode(), checkGaugeValue(), createAddonCheckWithStatusForObservability(), drainEvents() (+12 more)

### Community 77 - "newScheme"
Cohesion: 0.15
Nodes (14): appFakeAdapter, TestBuildManagerOptions_CertWatchers(), TestBuildManagerOptions_DefaultsHaveNoCertWatchers(), TestBuildManagerOptions_InsecureMetricsHasNoFilter(), TestBuildManagerOptions_MissingWebhookCert_Errors(), TestBuildManagerOptions_ScopesCacheByManagedByLabel(), TestNewScheme_RegistersAPIExtensions(), TestNewScheme_RegistersFathomTypes() (+6 more)

### Community 78 - "Implementation Plan: Quorum/Ratio Semantics for Managed-Resource Rollups"
Cohesion: 0.07
Nodes (25): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Quorum/Ratio Semantics for Managed-Resource Rollups, Complexity Tracking, Constitution Check, Documentation (this feature) (+17 more)

### Community 79 - "Adversarial Review Findings — v0.5.0 Release Gate (#217)"
Cohesion: 0.07
Nodes (24): Coverage Statement — v0.5.0 Release Gate (#217), Intentionally excluded, Perspective results (SC-001), Post-anchor deltas, Reviewed, Scope notes, Adversarial Review Findings — v0.5.0 Release Gate (#217), API-1: HealthCheck status.summary MaxLength=1024 wedges mirroring on long condition messages (high) (+16 more)

### Community 80 - "github.com/spf13/cobra.Command"
Cohesion: 0.15
Nodes (22): lsOptions, github.com/spf13/cobra.Command, factory, newDefinitionBindCommand(), factory, newDefinitionCollisionsCommand(), factory, newDefinitionDrainCommand() (+14 more)

### Community 81 - "Family"
Cohesion: 0.25
Nodes (19): fakeAdvertisingAdapter, familyRatioRollup, k8s.io/apimachinery/pkg/apis/meta/v1.Time, aggregateHealthReportResult(), aggregateWithRatioRollups(), copyStringMap(), healthReportChecks(), healthReportForAddonCheck() (+11 more)

### Community 82 - "Tasks: [FEATURE NAME]"
Cohesion: 0.07
Nodes (26): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation Strategy, Incremental Delivery, MVP First (User Story 1 Only) (+18 more)

### Community 83 - "speckit-analyze/SKILL.md"
Cohesion: 0.08
Nodes (25): 1. Initialize Analysis Context, 2. Load Artifacts (Progressive Disclosure), 3. Build Semantic Models, 4. Detection Passes (Token-Efficient Analysis), 5. Severity Assignment, 6. Produce Compact Analysis Report, 7. Provide Next Actions, 8. Offer Remediation (+17 more)

### Community 84 - "quorum-ratio-rollups/internal/adapter/declarative/evaluator.go"
Cohesion: 0.15
Nodes (7): AnnotationStalenessCheck, durationThreshold(), isFutureTimestamp(), namespaceScope(), AnnotationStalenessCheck, EvalContext, durationThreshold()

### Community 85 - "kedaHealthyObjects"
Cohesion: 0.13
Nodes (20): Engine, WorkloadCheck, kedaDeployment(), NewKedaEngine(), conditionCR(), clientObject, kedaHealthyObjects(), TestKeda_AbsentClusterAllSkipped() (+12 more)

### Community 86 - "Quickstart Validation: Quorum/Ratio Rollups"
Cohesion: 0.08
Nodes (22): Configuration surface (AddonCheck), Contract: Ratio Rollup Thresholds and Report Entries, Explicit non-changes, Metrics interplay (informative), Rejection (Accepted condition), Report surface (HealthReport), Verdict semantics, Data Model: Quorum/Ratio Semantics for Managed-Resource Rollups (+14 more)

### Community 87 - "Tasks: DNSCheck Completion"
Cohesion: 0.07
Nodes (27): Dependencies and Execution Order, Documentation for User Story 4, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation Strategy, Incremental Delivery (+19 more)

### Community 88 - "cli/run.go"
Cohesion: 0.10
Nodes (37): checkRef, kindDescriptor, runOptions, runOutcome, sourceResolution, executableKinds(), kindByName(), kindNamesForHelp() (+29 more)

### Community 89 - "nodeagent_metrics_test.go"
Cohesion: 0.18
Nodes (20): agentEndpoint, github.com/prometheus/client_model/go.Metric, github.com/prometheus/client_model/go.MetricFamily, anonymousStatusCommand(), assertAgentDoesNotServeMetrics(), assertCertificateMetrics(), assertNodeHealthMetrics(), authenticatedStatusCommand() (+12 more)

### Community 90 - "time.Duration"
Cohesion: 0.13
Nodes (38): lsRow, snapshot, sigs.k8s.io/controller-runtime/pkg/client.Object, time.Duration, cadence(), int32PtrString(), addonCheckSnapshot(), addonCheckTimeout() (+30 more)

### Community 91 - "Run"
Cohesion: 0.21
Nodes (15): rbacRoleBinding, rbacSubject, os/exec.Cmd, Gomega, applyDNSCheck(), dnsCheckField(), ensureNamespaceActive(), eventuallyDNSResult() (+7 more)

### Community 92 - ".agents/skills/speckit-analyze/SKILL.md"
Cohesion: 0.08
Nodes (25): 1. Initialize Analysis Context, 2. Load Artifacts (Progressive Disclosure), 3. Build Semantic Models, 4. Detection Passes (Token-Efficient Analysis), 5. Severity Assignment, 6. Produce Compact Analysis Report, 7. Provide Next Actions, 8. Offer Remediation (+17 more)

### Community 93 - "join"
Cohesion: 0.13
Nodes (26): serviceAccountToken(), join(), applyManifest(), scrapeOperatorMetrics(), newTestFlags(), TestDefaultOptions_MatchFlagDefaults(), TestLoad_ConfigOverridesDefault(), TestLoad_EnvOverridesConfig() (+18 more)

### Community 94 - "test/utils/utils.go"
Cohesion: 0.11
Nodes (26): TestE2EShardPlannerClassifiesPaths(), TestE2EShardPlannerKnowsEveryOptInAddon(), TestE2E(), CoreAddons(), GetNonEmptyLines(), GetProjectDir(), InstallPrometheusOperator(), IsPrometheusCRDsInstalled() (+18 more)

### Community 95 - "k8s.io/apimachinery/pkg/apis/meta/v1.TypeMeta"
Cohesion: 0.10
Nodes (28): AddonCheckList, AddonDefinitionList, ClusterHealth, ClusterHealthList, DNSCheckList, CheckTargetRef, HealthCheck, HealthCheckList (+20 more)

### Community 96 - "Tasks: Adversarial Codebase Review for the v0.5.0 Release Gate"
Cohesion: 0.09
Nodes (21): Consolidation and refutation, Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation Strategy, Incremental Delivery, MVP First (US1 only), Notes, Parallel Example: User Story 1 (+13 more)

### Community 97 - "Tasks: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.08
Nodes (24): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Notes, Parallel Example: User Story 1 (+16 more)

### Community 98 - "FamilyRatioVerdict"
Cohesion: 0.18
Nodes (17): ratioThresholdsByFamily(), FamilyRatioVerdict(), RatioPercent, RatioRollup, RatioThresholds, CheckResult, Outcome, isDigits() (+9 more)

### Community 99 - "Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.10
Nodes (20): Assumptions, Clarifications, D1 — Staleness is a signal, never a verdict change, D2 — Cadence is published for self-scheduling kinds; the aggregate is fixed at its derivation, D3 — "Staleness" is the canonical term; "freshness" is not used, Dependencies and Constraints, Edge Cases, Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth (+12 more)

### Community 100 - "dnscheck_types.go"
Cohesion: 0.16
Nodes (13): DNSCheckSpec, DNSCheckStatus, DNSResolver, DNSTarget, DNSTargetResult, targets(), createDNSCheck(), dnsHealthReports() (+5 more)

### Community 101 - "Execution Steps"
Cohesion: 0.12
Nodes (15): 1. Initialize Convergence Context, 2. Load Artifacts (Progressive Disclosure), 3. Build the Intent Inventory, 4. Assess the Codebase and Classify Findings, 5. Assign Severity, 6. Present the In-Session Findings Summary, 7. Append Convergence Tasks (or report converged), 8. Provide Next Actions (Handoff) (+7 more)

### Community 102 - "CertResult"
Cohesion: 0.11
Nodes (41): crypto/x509.Certificate, AllowedPathPrefixes(), FilterAllowedPaths(), isCertFile(), isKubeconfigFile(), MinimalMountDirs(), PathAllowed(), TestAllowedPathPrefixesMatchCRDRule() (+33 more)

### Community 103 - "TestAdapterClient"
Cohesion: 0.15
Nodes (11): addonSA(), TestAdapterClient(), TestRunAddonCheckFailsClosedWhenNamespaceEmptyInCluster(), TestRunAddonCheckFailsClosedWithoutScopedClient(), TestDefaultControllers_InClusterRequiresNamespace(), defaultRunningInCluster(), inClusterFromConfigErr(), TestInClusterFromConfigErr() (+3 more)

### Community 104 - "BuiltInAdapters"
Cohesion: 0.15
Nodes (18): Setupper, sigs.k8s.io/controller-runtime/pkg/cache.Options, sigs.k8s.io/controller-runtime/pkg/healthz.Checker, sync/atomic.Bool, BuildAdapterRegistry(), BuiltInAdapters(), DefaultControllers(), newUncachedProbeClient() (+10 more)

### Community 105 - "image"
Cohesion: 0.11
Nodes (20): properties, required, type, properties, required, type, image, probeImage (+12 more)

### Community 106 - "Load"
Cohesion: 0.25
Nodes (20): github.com/spf13/pflag.FlagSet, sigs.k8s.io/controller-runtime/pkg/log/zap.Options, Load(), RegisterFlags(), newTestFlags(), TestDefaultOptions_MatchFlagDefaults(), TestLoad_ConfigOverridesDefault(), TestLoad_DNSCheckMaxConcurrentProbesPrecedence() (+12 more)

### Community 107 - "AddonDefinitionBinding"
Cohesion: 0.17
Nodes (17): AddonDefinitionBinding, AddonDefinitionBindingList, AddonDefinitionBindingSpec, AddonDefinitionBindingStatus, DefinitionLeaderEpoch, DefinitionObjectReference, DefinitionReference, drainReader (+9 more)

### Community 108 - ".Run"
Cohesion: 0.06
Nodes (95): assertCheck(), findCheck(), ksmService(), passingLauncher(), readyPod(), runRequest(), TestRun_AllFamiliesDisabledEmitsSentinelSkip(), TestRun_HealthyDeploymentAndEndpointsPass() (+87 more)

### Community 109 - "assertFamily"
Cohesion: 0.26
Nodes (16): assertFamily(), Engine, NewIstioEngine(), clientObject, istioAmbientObjects(), istioCRDObjects(), istiodControlPlane(), istioHealthyObjects() (+8 more)

### Community 110 - "cli/version.go"
Cohesion: 0.11
Nodes (21): commandError, operatorVersion, versionInfo, runtime/debug.BuildInfo, ExitCode(), releaseInfo(), TestDefinitionCollisions(), buildSetting() (+13 more)

### Community 111 - "PolicyRule"
Cohesion: 0.22
Nodes (10): hasResource(), hasVerb(), TestRBACRulesDeclaresDryRunException(), hasResource(), hasVerb(), TestRBACRulesDeclaresProbeException(), hasResource(), hasVerb() (+2 more)

### Community 112 - "TestCommittedAddonRolesAreReadOnly"
Cohesion: 0.17
Nodes (13): RBACDeclarer, allowedWrites(), repoRoot(), TestCommittedAddonRolesAreReadOnly(), TestModelGrantsAreJustified(), TestUnjustifiedGrantsCatchesViolations(), UnjustifiedGrants(), AddonServiceAccountName() (+5 more)

### Community 113 - "HealthReport"
Cohesion: 0.24
Nodes (11): HealthReport, HealthReportCheck, HealthReportList, HealthReportSpec, HealthReportStatus, HealthReportTargetRef, k8s.io/apimachinery/pkg/apis/meta/v1.Duration, k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta (+3 more)

### Community 114 - "Feature Specification: DNSCheck Resource Contract"
Cohesion: 0.11
Nodes (18): Assumptions, Clarifications, Dependencies, Edge Cases, Feature Specification: DNSCheck Resource Contract, Functional Requirements, Key Entities, Measurable Outcomes (+10 more)

### Community 115 - "Implementation Plan: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.13
Nodes (15): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Phases, Implementation Plan: Cadence-Aware Staleness Semantics for ClusterHealth, Phase A — Staleness derivation (the reported defect), Phase B — Cadence publication, Phase C — Shipped alerting rules (+7 more)

### Community 116 - "healthcheck_controller.go"
Cohesion: 0.08
Nodes (39): HealthCheckReconciler, healthCheckTargetHandler, healthCheckTargetIdentity, healthCheckTargetReader, healthCheckTargetRegistry, healthCheckTargetSnapshot, k8s.io/apimachinery/pkg/types.NamespacedName, sigs.k8s.io/controller-runtime/pkg/reconcile.Request (+31 more)

### Community 117 - "cmd/node-agent/main.go"
Cohesion: 0.05
Nodes (87): boundedContext(), certificatePass(), certificatePassWithClock(), TestHealthzReflectsProgress(), TestLivenessFollowsPublication(), TestMetricsEndpointIsNotServed(), TestOnceFailsWhenThePassDoesNotPublish(), TestParseConfigHealthMode() (+79 more)

### Community 118 - "properties"
Cohesion: 0.12
Nodes (17): type, type, type, type, type, allOf, $comment, properties (+9 more)

### Community 119 - "test/e2e/healthreport_helpers_test.go"
Cohesion: 0.16
Nodes (15): checkResult, eventList, healthReport, healthReportList, addonCheckLastResult(), addonCheckReadyTrue(), dumpAddonCheckDiagnostics(), latestHealthReport() (+7 more)

### Community 120 - "runProjection"
Cohesion: 0.38
Nodes (12): capNames(), PodProjectionCheck, optedInPod(), runProjection(), TestPodProjection_AllInjectedPasses(), TestPodProjection_CapNames(), TestPodProjection_InactivePodsSkipped(), TestPodProjection_MissingEnvOnlyFails() (+4 more)

### Community 121 - "Guard"
Cohesion: 0.14
Nodes (14): k8s.io/apimachinery/pkg/runtime/schema.GroupVersionKind, net/http.Request, net/http.Response, net/http.RoundTripper, injectedHeaders, runtimeIdentityTransport, Guard, urlQuery() (+6 more)

### Community 122 - "Core Principles"
Cohesion: 0.12
Nodes (16): Core Principles, Development Workflow & Quality Gates, Engineering Constraints, Fathom Constitution, Fathom-Specific Constraints, Governance, I. Explicit State Over Implicit Behavior, II. Git Is the Durable Desired-State Boundary (+8 more)

### Community 123 - "clampCadence"
Cohesion: 0.19
Nodes (15): addonCheckInterval(), addonCheckTimeout(), cadenceClampMessages(), clampCadence(), durp(), findCondition(), TestAddonCheckCadenceHelpersClamp(), TestCadenceClampMessages() (+7 more)

### Community 124 - "5. ClusterHealth staleness is the stalest child, and is a signal not a verdict"
Cohesion: 0.25
Nodes (8): 5. ClusterHealth staleness is the stalest child, and is a signal not a verdict, Context and Problem Statement, Decision, Rationale, References, Why not degrade a stale `Fail` to `Unknown`, Why not the oldest child, unconditionally, Why status holds a timestamp rather than a judgement

### Community 125 - "requireAPIServer"
Cohesion: 0.11
Nodes (31): runtimeSchema(), TestDefinitionMaximumCheckAdmissionCost(), TestDefinitionSchemaLimitParity(), TestAddonDefinitionAllPayloadAdmission(), TestPayloadStructuralContractRejection(), firstRuntimeCheck(), runtimeDefinition(), TestAddonDefinitionAdmission() (+23 more)

### Community 126 - "Execution Steps"
Cohesion: 0.12
Nodes (15): 1. Initialize Convergence Context, 2. Load Artifacts (Progressive Disclosure), 3. Build the Intent Inventory, 4. Assess the Codebase and Classify Findings, 5. Assign Severity, 6. Present the In-Session Findings Summary, 7. Append Convergence Tasks (or report converged), 8. Provide Next Actions (Handoff) (+7 more)

### Community 127 - "DNS Checks"
Cohesion: 0.14
Nodes (14): An explicit resolver is unreachable, Answers exist but the expectation fails, Check cluster DNS and aggregate the result, Choose a resolver, Cluster resolver, DNS Checks, Explicit resolver, Express expectations (+6 more)

### Community 128 - "Contract: `ClusterHealth.Status`"
Cohesion: 0.14
Nodes (12): Contract: `ClusterHealth.Status`, New guarantees, `status.children[]` — bounded and ordered, `status.matchedCount` — contract strengthened, `status.observedAt` — meaning inverted, `status.result` — explicitly unchanged, Unchanged, Alerting contract (+4 more)

### Community 129 - "TestRuntimeInventoryMatchesOperator"
Cohesion: 0.25
Nodes (7): runtimeSampleChecks(), TestRuntimeInventoryMatchesOperator(), TestRuntimeSamplesMatchGenerated(), WriteRuntimeInventory(), WriteRuntimeSamples(), main(), BuiltinNames()

### Community 130 - "testing.T"
Cohesion: 0.04
Nodes (71): TestAddToScheme(), TestDeepCopyIntoExercise(), TestDeepCopyRoundTrip(), TestGroupVersion(), TestSchemeBuilderRegisterReturnsSelf(), TestHealthReportResultSeverity_EmptyAndUnrecognizedReturnZero(), TestHealthReportResultSeverity_OrderingAcrossEnumValues(), TestHealthReportResultSeverity_PassIsLowestNonZero() (+63 more)

### Community 131 - "quorum-ratio-rollups/internal/controller/nodecertificatecheck_helpers.go"
Cohesion: 0.15
Nodes (11): aggregateNodeReports(), controlPlaneTolerations(), healthReportForNodeCert(), joinPaths(), nodeOutcomeToResult(), pruneNodeCertHealthReports(), putIfNotEmpty(), resolveTolerations() (+3 more)

### Community 132 - "Tasks: Pre-1.0 CRD Validation Hardening"
Cohesion: 0.17
Nodes (12): Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Parallel Example: User Story 1, Phase 1: Setup, Phase 2: Foundational (Blocking Prerequisites), Phase 3: User Story 1 - Reject dangerously short check cadences at admission (Priority: P1) 🎯 MVP (+4 more)

### Community 133 - ".Name"
Cohesion: 0.14
Nodes (21): TestAdapterMetadata(), fakePolicyAdapter, TestEngine_Metadata(), TestEnvoyGateway_AdapterMetadata(), NewExternalDNSEngine(), extdnsHealthyObjects(), TestExternalDNS_AdapterMetadata(), TestExternalDNS_DeploymentNameThresholdOverride() (+13 more)

### Community 134 - "internal/adapter/rbacgen/rbacgen.go"
Cohesion: 0.19
Nodes (20): TestFilesRejectsIncompleteRule(), clusterRules(), Collect(), Files(), AddonRBAC, groupsCell(), marshalDocs(), renderAddon() (+12 more)

### Community 135 - "fathomctl Reference"
Cohesion: 0.12
Nodes (17): Bulk confirmation and --dry-run, describe, Exit codes, fathomctl Reference, Global flags, Kind names and aliases, ls, On-demand trigger contract (+9 more)

### Community 136 - "operator_rbac_doc_test.go"
Cohesion: 0.25
Nodes (13): docRow, k8s.io/api/rbac/v1.ClusterRole, equalStrings(), loadJustificationRows(), loadOperatorClusterRole(), normalizeSet(), ruleKey(), splitList() (+5 more)

### Community 137 - "Sweeper"
Cohesion: 0.31
Nodes (6): k8s.io/api/core/v1.PodPhase, orphanSince(), probePodSelector(), probeShaped(), terminalPhase(), Sweeper

### Community 138 - "runReports"
Cohesion: 0.09
Nodes (33): describer, objectList, outputFormat, reportsOptions, table, io.Writer, text/tabwriter.Writer, formatMap() (+25 more)

### Community 139 - "AddonCheck"
Cohesion: 0.12
Nodes (23): AddonCheck, AddonCheckFamilyPolicy, AddonCheckSpec, addonAdapterLookup, AddonCheckReconciler, TestCountAbsent(), addonCheckDueForRun(), addonCheckPolicy() (+15 more)

### Community 140 - "Tasks: DNSCheck Resource Contract"
Cohesion: 0.13
Nodes (15): Dependencies, Format: `[ID] [P?] [Story] Description`, Implementation strategy, Parallel opportunities, Path Conventions, Phase 1: Setup, Phase 2: Foundational (Blocking Prerequisites), Phase 4: User Story 2 — Every declared expectation is evaluated (P1) (+7 more)

### Community 141 - "Implementation Plan: DNSCheck Completion"
Cohesion: 0.17
Nodes (12): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: DNSCheck Completion, Implementation Strategy, Phase 0: Research Outcome, Phase 1: Design Outcome, Project Structure (+4 more)

### Community 142 - "Research: fathomctl CLI"
Cohesion: 0.14
Nodes (14): R10: Operator version discovery, R11: Distribution, R12: RBAC for CLI users, R13: End-to-end coverage, R1: Binary and package layout, R2: Cluster client, R3: Kind addressing and aliases, R4: Verdict normalisation per kind (+6 more)

### Community 143 - "rbac"
Cohesion: 0.14
Nodes (14): type, type, type, annotations, create, name, rbac, serviceAccount (+6 more)

### Community 144 - "enabled"
Cohesion: 0.14
Nodes (14): properties, type, type, type, maximum, minimum, type, config (+6 more)

### Community 145 - "NewBudget"
Cohesion: 0.10
Nodes (38): ExecutionBudget, executionBudgetKey, TestRuntimeCollectionPayloadsFollowContinuation(), CompileRuntime(), runtimeTestContext(), runtimeVersionDefinition(), TestRuntimeCompilationPreservesOrderAndSnapshot(), TestRuntimeCompilerRejectsUnsupportedAndCancelledInput() (+30 more)

### Community 146 - "EnsureCompatible"
Cohesion: 0.24
Nodes (11): version, CRD API Versioning Standard, CRD Maturity Ladder (alpha/beta/GA), ContractVersion Constant, EnsureCompatible(), ensureCompatible(), parseVersion(), TestContractVersionParses() (+3 more)

### Community 147 - "quorum-ratio-rollups/internal/probe/sweeper_test.go"
Cohesion: 0.32
Nodes (11): probeLabels(), probeShape(), sweepPod(), terminatedAt(), TestSweeper_LabelledPodNotMatchingProbeShapeIsSpared(), TestSweeper_LongRunningProbeIsNotReapedOnCreationAge(), TestSweeper_ReapsPodTerminatedLongerThanMinAge(), TestSweeper_StartSweepsImmediatelyAndStopsOnCancel() (+3 more)

### Community 148 - "NewMetricsServerEngine"
Cohesion: 0.32
Nodes (10): Engine, NewMetricsServerEngine(), apiService(), clientObject, msHealthyObjects(), TestMetricsServer_AdapterMetadata(), TestMetricsServer_HealthyPassesAllFamilies(), TestMetricsServer_MissingAPIServiceFails() (+2 more)

### Community 149 - "internal/metrics/check_metrics_test.go"
Cohesion: 0.24
Nodes (19): ctrlRegistryGather(), gatherCheckSeries(), gatherDNSTargetSeries(), gatherOneHot(), TestCheckIntervalSeries(), TestCheckIntervalWithdrawnWhenUnresolvable(), TestCheckResultValuesMatchAPIVocabulary(), TestDeleteCheckSeries() (+11 more)

### Community 150 - ".agents/skills/speckit-plan/SKILL.md"
Cohesion: 0.18
Nodes (10): Completion Report, Done When, Key rules, Mandatory Post-Execution Hooks, Outline, Phase 0: Outline & Research, Phase 1: Design & Contracts, Phases (+2 more)

### Community 151 - "establishedCRD"
Cohesion: 0.18
Nodes (14): establishedCRD(), TestCondition_ResolveVersion(), NewKedaEngine(), conditionCR(), kedaHealthyObjects(), TestKeda_AbsentClusterAllSkipped(), TestKeda_HealthyWithReadyScaledObject(), TestKeda_PausedScaledObjectWarns() (+6 more)

### Community 152 - "Entity: `DNSCheck`"
Cohesion: 0.14
Nodes (14): Changes to existing types, `cmd/probe` — dns mode flags, `DNSCheckSpec`, `DNSCheckStatus`, `DNSResolver`, `DNSTarget`, `DNSTargetResult`, Entity: `DNSCheck` (+6 more)

### Community 153 - "crd_compat_gate_test.go"
Cohesion: 0.47
Nodes (12): fixtureCRD(), runCRDCompat(), TestCRDCompatAddedOptionalFieldPasses(), TestCRDCompatAgainstBaseline(), TestCRDCompatAllowlistedChangePassesVisibly(), TestCRDCompatMalformedAllowlistFails(), TestCRDCompatNewCRDSkipped(), TestCRDCompatNoChangePasses() (+4 more)

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

### Community 160 - "Capabilities"
Cohesion: 0.09
Nodes (7): healthReportCount(), absentReportingAdapter, countingStatusClient, fakeAddonAdapter, programmableAdapter, versionReportingAdapter, Capabilities

### Community 161 - "Fathom Architecture"
Cohesion: 0.33
Nodes (7): ADR-0002 HealthReport as First-class CRD, ADR-0004 HealthCheck as Thin Wrapper, Rationale: uniform wrapper preserves aggregator contract, ClusterHealth-from-HealthCheck.status-only Invariant, Fathom Architecture, Fathom Documentation Index, Probe Orphan Sweeper

### Community 162 - "runArgoCD"
Cohesion: 0.24
Nodes (12): argocdDeployment(), Engine, WorkloadCheck, NewArgoCDEngine(), argoApp(), argocdHealthyObjects(), clientObject, runArgoCD() (+4 more)

### Community 163 - "GitHub Copilot Instructions for Fathom"
Cohesion: 0.17
Nodes (11): Codebase Shape, Commit and Branch Guidance, Documentation Expectations, GitHub Copilot Instructions for Fathom, Go and Repository Conventions, Knowledge Graph (`graphify-out/`), Pull Request Instructions, Safety Rules (+3 more)

### Community 164 - "truncateChildren"
Cohesion: 0.24
Nodes (12): ClusterHealthChildSummary, ClusterHealthSpec, ClusterHealthStatus, rankSeverity(), truncateChildren(), childAt(), TestRankSeverityCoercesEmptyToUnknown(), TestTruncateChildrenCapsAtTheLimit() (+4 more)

### Community 165 - "Feature Specification: Node-agent metrics security"
Cohesion: 0.18
Nodes (11): Assumptions, Edge Cases, Feature Specification: Node-agent metrics security, Functional Requirements, Key Entities, Measurable Outcomes, Requirements, Success Criteria (+3 more)

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

### Community 171 - "assertPodNetworkHealthAgentSecurity"
Cohesion: 0.21
Nodes (9): dsRollout, nodeCertStatusView, daemonSetRollout(), dumpNodeCertDiagnostics(), nodeCertStatus(), assertPodNetworkHealthAgentSecurity(), daemonSetRollout(), dumpNodeCertDiagnostics() (+1 more)

### Community 172 - "SetRunningInClusterForTest"
Cohesion: 0.15
Nodes (12): k8s.io/api/core/v1.ServiceAccount, defaultRunningInCluster(), inClusterFromConfigErr(), TestInClusterFromConfigErr(), RunningInCluster(), SetRunningInClusterForTest(), TestRunningInCluster_TestOverride(), TestDefaultControllers_InClusterRequiresNamespace() (+4 more)

### Community 173 - "6. Serve node metrics through the authenticated operator endpoint"
Cohesion: 0.25
Nodes (6): 6. Serve node metrics through the authenticated operator endpoint, Consequences, Considered Options, Context and Problem Statement, Decision Outcome, Links

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

### Community 184 - "TestExternalSecrets_HealthyAndEmptySyncSkipped"
Cohesion: 0.27
Nodes (8): esoDeployment(), Engine, WorkloadCheck, NewExternalSecretsEngine(), esoHealthyObjects(), clientObject, TestExternalSecrets_HealthyAndEmptySyncSkipped(), TestExternalSecrets_MissingDeploymentFails()

### Community 185 - "validDefinition"
Cohesion: 0.11
Nodes (24): GrantPlan, RenderOptions, ScopedGrant, k8s.io/api/rbac/v1.PolicyRule, TestCanonicalDefinitionByteBoundary(), TestDefinitionStringBytesAndUTF8(), TestDefinitionStructuralBoundaries(), TestRuntimeReadDeclarationGrammar() (+16 more)

### Community 186 - "nodehealth/paths.go"
Cohesion: 0.19
Nodes (14): AllowedPathPrefixes(), AllowedSocketDirs(), AllowedSocketFiles(), Canonical(), MountDirs(), PathAllowed(), SocketPathAllowed(), TestAllowlistsMirrorCRDRules() (+6 more)

### Community 187 - "Authoring an Adapter Guide"
Cohesion: 0.24
Nodes (18): ADR-0001 In-process Adapter Contract, ADR-0003 Probe-pod Model, Rationale: representative network topology without a DaemonSet, Probe-Pod Model (active in-cluster checks), Absence Semantics (Required/Optional, MarkAbsent), Declarative Adapter Engine (MustEngine), Authoring an Adapter Guide, Version Detection and SupportedVersions Gating (+10 more)

### Community 188 - "Tasks: Node-agent metrics security"
Cohesion: 0.25
Nodes (8): Dependencies and parallel work, Implementation strategy, Phase 1: Setup, Phase 2: Foundational contract, Phase 3: US1 — Protect node inventory, Phase 4: US2 — Keep monitoring useful, Phase 5: Review and validation, Tasks: Node-agent metrics security

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

### Community 195 - "010-node-agent-metrics-security/contracts/metrics.md"
Cohesion: 0.29
Nodes (5): Access, Lifecycle and migration, Node metrics contract, Series, Validation guide

### Community 196 - "Quickstart Validation: fathomctl CLI"
Cohesion: 0.17
Nodes (8): 1. Build and generated artifacts, 2. Unit coverage without a cluster, 3. Distribution script, 4. Real cluster (required: controllers and CRD types change), 5. Release verification (after the first tagged release), 6. Documentation gates, Prerequisites, Quickstart Validation: fathomctl CLI

### Community 197 - "assertHasOutcome"
Cohesion: 0.27
Nodes (21): ConditionCheck, runManaged(), TestCondition_ClusterScopedListsWithoutNamespace(), TestCondition_ConditionStatus(), TestCondition_InvalidAPIVersionErrors(), TestCondition_InvalidSelectorErrors(), TestCondition_ListErrorDescribesNamespaceScope(), TestCondition_ListNameFallsBackToKind() (+13 more)

### Community 198 - ".agents/skills/speckit-checklist/SKILL.md"
Cohesion: 0.25
Nodes (7): Anti-Examples: What NOT To Do, Checklist Purpose: "Unit Tests for English", Example Checklist Types & Sample Items, Execution Steps, Post-Execution Checks, Pre-Execution Checks, User Input

### Community 199 - ".detectAndGateVersion"
Cohesion: 0.19
Nodes (12): versionAddress, resolveFamily(), resolveFamily(), detectAddonVersion(), Engine, imageTag(), pickImage(), TestImageTag() (+4 more)

### Community 200 - "Budget"
Cohesion: 0.19
Nodes (4): context.CancelCauseFunc, Budget, minTime(), Failure

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

### Community 212 - "newFakeClient"
Cohesion: 0.16
Nodes (30): dnsRequest(), Request, newFakeClient(), simulateKubelet(), TestLauncherRun_ConcurrentRunsAreIndependent(), TestLauncherRun_DeletesPodAfterRun(), TestLauncherRun_EmptyTerminationMessageIsError(), TestLauncherRun_FailedPhasePropagatesProbeJSON() (+22 more)

### Community 213 - "Phase 1 Data Model: DNSCheck Reconciler"
Cohesion: 0.15
Nodes (13): 1. In-memory entities, 2. Mapping onto the frozen schema, 3. Metric series, 4. Lifecycle and ownership, 5. State transitions, Conditions, Existing, reused unchanged (FR-032), New (FR-033) (+5 more)

### Community 214 - ".agents/skills/speckit-implement/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 215 - "Implementation Plan: fathomctl CLI"
Cohesion: 0.15
Nodes (13): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: fathomctl CLI, Implementation Strategy, Phase 0: Research Outcome, Phase 1: Design Outcome, Project Structure (+5 more)

### Community 216 - "fathomctl"
Cohesion: 0.25
Nodes (8): Check again, right now, fathomctl, Install, Permissions, See every verdict, Understand a verdict, Walk the history, Which version am I talking to?

### Community 217 - "Specification Quality Checklist: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.29
Nodes (6): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Cadence-Aware Staleness Semantics for ClusterHealth, Validation Notes

### Community 218 - "payloads.go"
Cohesion: 0.33
Nodes (22): firstError(), labelName(), requiredName(), resourceType(), selectorLabels(), validateAnnotation(), validateCheck(), validateCondition() (+14 more)

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

### Community 226 - "Node Health Checks"
Cohesion: 0.17
Nodes (12): A minimal check, Cadence and freshness, Coverage, freezing, and what the conditions mean, Node Health Checks, Node scope and control-plane nodes, On-demand runs, Projecting into cluster health, The check types (+4 more)

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

### Community 231 - "CheckResult"
Cohesion: 0.10
Nodes (41): deploymentAvailable(), firstNamespace(), containsString(), Established(), PreferredServedVersion(), ConditionCheck, ConfigMapCheck, CRDCheck (+33 more)

### Community 232 - "Feature Specification: fathomctl CLI"
Cohesion: 0.15
Nodes (13): Assumptions and Dependencies, Clarifications, Decisions and Tradeoffs, Feature Specification: fathomctl CLI, Functional Requirements, Key Entities, Measurable Outcomes, Out of Scope (+5 more)

### Community 233 - "Contract: HealthCheck Target Projection"
Cohesion: 0.29
Nodes (7): Aggregation boundary, Compatibility, Contract: HealthCheck Target Projection, Reference failures, Successful projection, Supported references, Watch contract

### Community 234 - ".DeepCopyObject"
Cohesion: 0.22
Nodes (6): NodeCertificateCheckList, TestAddToScheme(), TestDeepCopyIntoExercise(), TestDeepCopyRoundTrip(), TestSchemeBuilderRegisterReturnsSelf(), NodeCertificateCheckList

### Community 235 - "quorum-ratio-rollups/internal/nodecert/paths.go"
Cohesion: 0.19
Nodes (14): resolveCertPaths(), TestResolveCertPathsFiltersDisallowed(), AllowedPathPrefixes(), DefaultCertPaths(), FilterAllowedPaths(), isCertFile(), isKubeconfigFile(), MinimalMountDirs() (+6 more)

### Community 236 - "TestCommittedAddonRolesAreReadOnly"
Cohesion: 0.20
Nodes (11): allowedWrites(), repoRoot(), TestCommittedAddonRolesAreReadOnly(), TestModelGrantsAreJustified(), TestUnjustifiedGrantsCatchesViolations(), AddonServiceAccountName(), IsReadVerb(), TestAddonServiceAccountName() (+3 more)

### Community 237 - "Research: Node-agent metrics security"
Cohesion: 0.29
Nodes (6): Boundary and prior art, Decision: reduce certificate inventory, Decision: serve node metrics through the operator, Decision: use existing metric projection lifecycle, Research: Node-agent metrics security, Verification

### Community 238 - "addondefinition_payloads.go"
Cohesion: 0.33
Nodes (21): DefinitionAnnotationStaleness, DefinitionCondition, DefinitionConfigMap, DefinitionCRD, DefinitionCronJob, DefinitionField, DefinitionPodProjection, DefinitionReadRule (+13 more)

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

### Community 243 - "agentResourceName"
Cohesion: 0.09
Nodes (32): TestNodeDetailMetricsWithdrawAtReconcileEntryWithoutAffectingSiblingCheck(), TestNodeDetailProjectionIncludesAcceptedPartialFleetAndExcludesDepartedNodes(), TestPausedNodeCertificateCheckWithdrawsDetailMetrics(), TestEnsureAgentRBACDrainsLegacyBindingsWithoutClusterRoleRequests(), observeNodeCertificateReports(), agentResourceName(), newProvisioningScheme(), TestNodeCertAuthenticityUnavailableFailsClosed() (+24 more)

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

### Community 254 - "Adapter"
Cohesion: 0.21
Nodes (8): k8s.io/apimachinery/pkg/types.UID, sync.RWMutex, Registry, Adapter, Cache, cachedRevision, RevisionKey, Snapshot

### Community 255 - "AddonDefinition"
Cohesion: 0.14
Nodes (21): AddonDefinition, DefinitionBindingScope, runtimeAdapter, compileRuntime(), CompileRuntimeScoped(), firstRuntimeNamespace(), Engine, lowerCheck() (+13 more)

### Community 256 - "Implementation Plan: Runtime Addon Definitions"
Cohesion: 0.10
Nodes (21): Acceptance Criteria, Assumptions and Unknowns, Complexity Tracking, Constitution Check, Current State, Done When, Goal, Implementation Plan: Runtime Addon Definitions (+13 more)

### Community 258 - "NodeReport"
Cohesion: 0.08
Nodes (41): k8s.io/api/core/v1.NodeCondition, ensureNode(), healthyNodeConditions(), newNodeHealthReconciler(), nodeHealthAgentClient(), writeNodeHealthReport(), writeNodeHealthReportObject(), writeTriggeredNodeHealthReport() (+33 more)

### Community 259 - "quorum-ratio-rollups/internal/nodecert/scan_test.go"
Cohesion: 0.27
Nodes (13): Scan(), makeCertPEM(), TestClassifyBoundaries(), TestScanBundleEmitsPerCert(), TestScanDefaultsWhenNoPaths(), TestScanDirectoryRecursiveAndIgnoresNonCerts(), TestScanKubeconfig(), TestScanMissingPathIsSilent() (+5 more)

### Community 260 - "RFC 1. AddonDefinition as a CRD — make adapters installable, not compiled in"
Cohesion: 0.10
Nodes (20): 1. Schema — Accepted, 2. Authority — Accepted, 3. Versions — Accepted, 4. Identity and precedence — Accepted, 5. Loading, lifecycle and evidence — Accepted, 6. Bounds and failure isolation — Accepted, Acceptance record, All evaluation paths (+12 more)

### Community 261 - "Contract: DNSCheck Metrics, Events, and RBAC"
Cohesion: 0.20
Nodes (10): 1. Check-level metrics (inherited, FR-032), 2. Per-target metric (new, FR-033), 3. Events (inherited, FR-112), 4. Conditions, 5. RBAC (FR-115 / inherited FR-037), Contract: DNSCheck Metrics, Events, and RBAC, Current state, Required (+2 more)

### Community 262 - "Implementation Plan: Node-agent metrics security"
Cohesion: 0.33
Nodes (6): Complexity Tracking, Constitution Check, Implementation Plan: Node-agent metrics security, Project Structure, Summary, Technical Context

### Community 263 - "main"
Cohesion: 0.38
Nodes (5): main(), runMain(), TestMain_BadFlagExitsNonZero(), TestMain_HelpExitsZero(), TestMain_RunsAsMainOnDemand()

### Community 264 - "quorum-ratio-rollups/internal/adapter/declarative/istio_test.go"
Cohesion: 0.32
Nodes (12): NewIstioEngine(), istioAmbientObjects(), istioCRDObjects(), istiodControlPlane(), istioHealthyObjects(), istioInjectorConfig(), istioValidatorConfig(), TestIstio_AmbientDataPlanePasses() (+4 more)

### Community 265 - ".DeepCopyInto"
Cohesion: 0.08
Nodes (18): AddonCheckStatus, ClusterHealthList, ClusterHealthStatus, HealthCheckSpec, HealthCheckStatus, HealthReportCheck, NodeCertificateCheck, NodeCertificateCheckSpec (+10 more)

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

### Community 271 - "e2e-shards.sh"
Cohesion: 0.83
Nodes (3): emit_all(), e2e-shards.sh script, shard_for_file()

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

### Community 307 - ".Update"
Cohesion: 0.15
Nodes (12): createAddonCheckWithStatusForObservability(), conflictOnceStatusClient, conflictOnceStatusWriter, countingStatusWriter, sigs.k8s.io/controller-runtime/pkg/client.SubResourceWriter, nodeCertHealthReportCount(), setNodeAgentDaemonSetStatus(), setNodeAgentDaemonSetStatusFull() (+4 more)

### Community 308 - ".Evaluate"
Cohesion: 0.16
Nodes (15): PodProjectionCheck, k8s.io/api/core/v1.Container, TestPolicyNamespaceResolution(), namespaceScope(), policyNamespaces(), containerHasEnv(), formatSelector(), PodProjectionCheck (+7 more)

### Community 309 - ".waitForRunWithDeadlineGrowth"
Cohesion: 0.49
Nodes (7): runTarget, waitResult, blockedBy(), classifyWaitResult(), factory, inspectRunObject(), pollRun()

### Community 310 - "NewRootCommand"
Cohesion: 0.13
Nodes (17): main(), runMain(), TestMain_BadFlagExitsNonZero(), TestMain_HelpExitsZero(), TestMain_RunsAsMainOnDemand(), context.CancelFunc, NewRootCommand(), signalContext() (+9 more)

### Community 312 - "Implementation Plan: DNSCheck Reconciler"
Cohesion: 0.25
Nodes (8): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: DNSCheck Reconciler, Project Structure, Source Code (repository root), Summary, Technical Context

### Community 313 - "DefinitionCheck"
Cohesion: 0.12
Nodes (21): AddonDefinitionSpec, AddonDefinitionStatus, DefinitionCheck, DefinitionFamily, DefinitionStatusCondition, DefinitionAnnotationStaleness, DefinitionCondition, DefinitionConfigMap (+13 more)

### Community 315 - "Quickstart Validation: DNSCheck Reconciler"
Cohesion: 0.29
Nodes (7): Level 1 — unit, no cluster, Level 2 — envtest, real API server, faked launcher, Level 3 — e2e on Kind, Level 4 — gates before the PR is ready, Manual review items no gate catches, Prerequisites, Quickstart Validation: DNSCheck Reconciler

### Community 316 - "User Scenarios & Testing *(mandatory)*"
Cohesion: 0.33
Nodes (6): Edge Cases, User Scenarios & Testing *(mandatory)*, User Story 1 - A declared check produces a verdict on its cadence (Priority: P1), User Story 2 - An operator sees which name failed, not just that the check failed (Priority: P2), User Story 3 - Result history is recorded without noise (Priority: P3), User Story 4 - Evaluation workloads never outlive their check (Priority: P3)

### Community 317 - "quorum-ratio-rollups/internal/adapter/crdutil/crdutil_test.go"
Cohesion: 0.47
Nodes (5): crd(), crdWithServed(), TestEstablished(), TestPreferredServedVersion(), TestPreferredServedVersion_IgnoresUnservedEntries()

### Community 318 - "webhookEntry"
Cohesion: 0.40
Nodes (4): webhookEntry, k8s.io/api/admissionregistration/v1.ServiceReference, appendEntry(), appendEntry()

### Community 319 - "dnscheck_test.go"
Cohesion: 0.60
Nodes (3): dnsCheckPod, dnsCheckPodList, listProbePods()

### Community 321 - "Specification Quality Checklist: DNSCheck Reconciler"
Cohesion: 0.40
Nodes (5): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: DNSCheck Reconciler

### Community 325 - "TestDescheduler_HealthyDeploymentMode"
Cohesion: 0.22
Nodes (11): configMap(), ConfigMapCheck, runConfigMap(), TestConfigMapCheck(), TestConfigMapCheck_AbsentInheritsOptional(), TestConfigMapCheck_NoAPIVersionAssertionPassesAnyYAML(), Engine, NewDeschedulerEngine() (+3 more)

### Community 326 - "Implementation Strategy"
Cohesion: 0.50
Nodes (4): Implementation Strategy, Incremental Delivery, MVP First, Suggested PR Scope

### Community 330 - "CLI-side types (`internal/cli`, unexported)"
Cohesion: 0.22
Nodes (9): checkRef, CLI-side types (`internal/cli`, unexported), GlobalOptions, kindDescriptor, reportRow, runTarget and runOutcome, snapshot, trigger token (+1 more)

### Community 332 - "Contract: fathomctl command surface"
Cohesion: 0.25
Nodes (8): Contract: fathomctl command surface, `fathomctl describe <kind>/<name>`, `fathomctl ls [kind] [-l selector]`, `fathomctl reports <kind>/<name> [--limit N] [--since <duration>] [--report <name>]`, `fathomctl run (<kind>/<name> | --all | -l selector) [--wait] [--timeout <duration>] [--yes] [--dry-run]`, `fathomctl version [--client]`, Global flags, Help and completion

### Community 333 - "Data Model: fathomctl CLI"
Cohesion: 0.25
Nodes (7): API changes (`api/v1alpha1`), CLI wait loop, Data Model: fathomctl CLI, Executable checks (all three kinds), Operator state transitions, Release artifacts, Wire contract change (`internal/nodecert`)

### Community 334 - "quorum-ratio-rollups/internal/adapter/rbacgen/rbacgen.go"
Cohesion: 0.29
Nodes (12): TestFilesRejectsIncompleteRule(), clusterRules(), Files(), groupsCell(), marshalDocs(), renderAddon(), renderDocs(), renderHelmData() (+4 more)

### Community 335 - "Specification Quality Checklist: fathomctl CLI"
Cohesion: 0.29
Nodes (5): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: fathomctl CLI

### Community 336 - "Contract: on-demand run trigger (operator side)"
Cohesion: 0.29
Nodes (7): Annotation, Consumption, Contract: on-demand run trigger (operator side), Derived kinds, Kind-specific behaviour, Status field, Test obligations

### Community 337 - "User Scenarios & Testing *(mandatory)*"
Cohesion: 0.29
Nodes (7): Edge Cases, User Scenarios & Testing *(mandatory)*, User Story 1 - Ask Fathom to validate now and get the answer (Priority: P1), User Story 2 - See every verdict in the cluster at a glance (Priority: P1), User Story 3 - Understand why a check has the verdict it has (Priority: P2), User Story 4 - See how a check's verdict has changed over time (Priority: P2), User Story 5 - Install the CLI, trust it, and know what I am talking to (Priority: P3)

### Community 338 - "fathomctl_test.go"
Cohesion: 0.33
Nodes (3): fathomctlRunOutcome, fathomctl(), kubectlJSONPath()

### Community 340 - "New"
Cohesion: 0.27
Nodes (11): New(), newFake(), TestCapabilities(), TestConcurrentAccess(), TestLookup(), TestRegister(), TestRegister_DuplicateAddonType(), TestRegister_PartialFailureLeavesRegistryUnchanged() (+3 more)

### Community 341 - "Consequences"
Cohesion: 0.40
Nodes (5): Accepted tradeoff: detection latency in mixed aggregates, Alternative rejected: add a parallel field, Consequences, Terminology, This is a breaking behavioural change with no schema signal

### Community 342 - "k8s.io/api/apps/v1.Deployment"
Cohesion: 0.26
Nodes (13): k8s.io/api/apps/v1.Deployment, deploymentAvailable(), deploymentWithImage(), deploymentWithMetaVersion(), deploymentWithTemplateVersion(), Engine, TestDetectAddonVersion(), TestDetectAndGateVersion() (+5 more)

### Community 343 - "MustEngine"
Cohesion: 0.11
Nodes (31): TestMustEngine_PanicsOnInvalid(), crdAbsenceEngine(), TestCRD_AbsenceResolution(), endRunSpan(), MustEngine(), NewEngine(), TestNewEngine_Validation(), validVersionSource() (+23 more)

### Community 344 - "internal/app/run_happy_test.go"
Cohesion: 0.20
Nodes (11): crypto/tls.Config, testing.M, adapterName(), disableHTTP2(), firstEnvtestBinaryDir(), TestAdapterName_NilReturnsPlaceholder(), TestAdapterName_NonNilReturnsName(), TestDisableHTTP2() (+3 more)

### Community 345 - "internal/adapter/declarative/field_test.go"
Cohesion: 0.38
Nodes (11): gauge(), gaugeCheck(), FieldCheck, runFields(), TestField_InvalidSelectorErrors(), TestField_ListedObjectsScored(), TestField_NoMatchingObjectsSkipped(), TestField_NoMatchUsesAddonAbsencePosture() (+3 more)

### Community 346 - "internal/controller/tracing_test.go"
Cohesion: 0.16
Nodes (18): TestListSelectedHealthChecks_ErrorNamesScope(), failingHealthCheckListClient, go.opentelemetry.io/otel/sdk/trace/tracetest.InMemoryExporter, go.opentelemetry.io/otel/sdk/trace/tracetest.SpanStub, go.opentelemetry.io/otel/sdk/trace/tracetest.SpanStubs, TestListSelectedHealthChecks_ErrorNamesScope(), attrValue(), installInMemoryTracer() (+10 more)

### Community 353 - "Feature Specification: Complete AddonDefinition Design RFC"
Cohesion: 0.13
Nodes (15): Assumptions, Clarifications, Context and Scope, Edge Cases, Feature Specification: Complete AddonDefinition Design RFC, Functional Requirements, Key Entities *(include if feature involves data)*, Measurable Outcomes (+7 more)

### Community 354 - "k8s.io/apimachinery/pkg/runtime.Scheme"
Cohesion: 0.35
Nodes (9): k8s.io/apimachinery/pkg/runtime.Scheme, RuntimeAuthority, RuntimeFactory, NewRuntimeFactory(), ResolveRuntimeAuthority(), authorityFixture(), TestRuntimeAuthorityRejectsIdentityConfusion(), TestRuntimeDiscoveryUsesOnlyBoundIdentity() (+1 more)

### Community 356 - "runtime.md"
Cohesion: 0.22
Nodes (7): Accepted implementation clarifications — 2026-09-20, Offline renderer clarification — option A, Activation and identity, Drain acknowledgement, Independent CLI verification, Leader election and drain verification, Required tests

### Community 357 - "Feature Specification: Runtime Addon Definitions"
Cohesion: 0.13
Nodes (15): Assumptions, Clarifications, Edge Cases, Feature Specification: Runtime Addon Definitions, Functional Requirements, Key Entities, Measurable Outcomes, Requirements (+7 more)

### Community 358 - "TestEnvoyGateway_HealthyAndNoGatewaysSkipped"
Cohesion: 0.27
Nodes (9): Engine, NewEnvoyGatewayEngine(), egHealthyObjects(), gatewayObject(), clientObject, TestEnvoyGateway_AdapterMetadata(), TestEnvoyGateway_GatewayConditionScoring(), TestEnvoyGateway_HealthyAndNoGatewaysSkipped() (+1 more)

### Community 360 - "Adversarial review — implementation checkpoint"
Cohesion: 0.40
Nodes (4): Adversarial review — implementation checkpoint, Reproduced and fixed findings, Resolved design decision, Verified boundaries and limitations

### Community 361 - "quorum-ratio-rollups/internal/nodecert/scan.go"
Cohesion: 0.35
Nodes (13): minimalKubeconfig, classify(), classifyAll(), daysFromDuration(), errorResult(), parsePEMCertificates(), scanCertFile(), scanDir() (+5 more)

### Community 362 - "Options"
Cohesion: 0.31
Nodes (8): DNSCheckOptions, flagBinding, MetricsOptions, Options, RuntimeLoadingOptions, TracingOptions, WebhookOptions, bindings()

### Community 363 - "0001-addondefinition-crd.md"
Cohesion: 0.32
Nodes (4): AddonDefinition RFC quickstart, Eventual review and handoff checks, Local checks, Prerequisites

### Community 364 - "Typed definition wire contract"
Cohesion: 0.15
Nodes (13): AnnotationStaleness → AnnotationStalenessCheck, Common target and conversion rules, Condition → ConditionCheck, ConfigMap → ConfigMapCheck, CRD → CRDCheck, CronJob → CronJobCheck, Declared reads and validation split, Envelope, family and check (+5 more)

### Community 365 - ".Register"
Cohesion: 0.26
Nodes (10): k8s.io/apimachinery/pkg/runtime/schema.GroupVersion, k8s.io/apimachinery/pkg/runtime.SchemeBuilder, newFake(), TestConcurrentAccess(), TestLookup(), TestRegister(), TestRegister_DuplicateAddonType(), TestRegister_PartialFailureLeavesRegistryUnchanged() (+2 more)

### Community 366 - "Validate"
Cohesion: 0.15
Nodes (21): reflect.Value, EvalContext, TestRuntimePayloadWireMappings(), TestRuntimeResolvedPolicyIsRevalidated(), TestRangeAndSelectorBoundaries(), apiVersion(), boundedSpec(), boundedString() (+13 more)

### Community 367 - "Design Entities"
Cohesion: 0.17
Nodes (12): Addon definition, Built-in definition, Conceptual Data Model: AddonDefinition RFC, Definition revision, Design Entities, Evaluation evidence, Evaluation identity, Governance Entities (+4 more)

### Community 368 - "RFC execution evidence"
Cohesion: 0.17
Nodes (12): Acceptance — 2026-09-20, Additional Copilot contract clarifications, Copilot review clarifications, Draft delivery and review, Final publication-fence correction, Governance, Merge and handoff — 2026-09-20, Review publication (+4 more)

### Community 369 - "Implementation Plan: Complete AddonDefinition Design RFC"
Cohesion: 0.17
Nodes (12): Complexity Tracking, Constitution Check, Documentation (this feature), Execution Sequence After Planning, Implementation Plan: Complete AddonDefinition Design RFC, Phase 0: Research, Phase 1: Design Artifacts, Project Structure (+4 more)

### Community 370 - "NewExternalDNSEngine"
Cohesion: 0.33
Nodes (8): Engine, NewExternalDNSEngine(), extdnsHealthyObjects(), clientObject, TestExternalDNS_AdapterMetadata(), TestExternalDNS_HealthyPassesAllFamilies(), TestExternalDNS_MissingCRDSkippedOptional(), TestExternalDNS_MissingDeploymentFails()

### Community 371 - "fixtures.go"
Cohesion: 0.21
Nodes (12): AtAndOverDuration(), Boundaries(), Bytes(), JSONDepth(), JSONNodes(), Map(), Strings(), TestNamespaceAndUIDFixturesExerciseBindingLimits() (+4 more)

### Community 372 - "Established"
Cohesion: 0.24
Nodes (8): k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1.ConditionStatus, k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1.CustomResourceDefinitionConditionType, Established(), crd(), crdWithServed(), TestEstablished(), TestPreferredServedVersion(), TestPreferredServedVersion_IgnoresUnservedEntries()

### Community 373 - ".IsReadOnly"
Cohesion: 0.50
Nodes (4): hasResource(), hasVerb(), TestRBACRulesDeclaresDryRunException(), TestRBACRulesDeclaresProbeException()

### Community 374 - "quorum-ratio-rollups/internal/app/run_happy_test.go"
Cohesion: 0.29
Nodes (4): firstEnvtestBinaryDir(), TestMain(), TestRun_HappyPath_DefaultControllers(), TestRun_HappyPath_NoControllers()

### Community 375 - ".severity"
Cohesion: 0.28
Nodes (5): TestWorstResult(), WorstResult(), TestHealthReportResultSeverity_EmptyAndUnrecognizedReturnZero(), TestHealthReportResultSeverity_OrderingAcrossEnumValues(), TestHealthReportResultSeverity_PassIsLowestNonZero()

### Community 376 - "Research: Complete AddonDefinition Design RFC"
Cohesion: 0.20
Nodes (10): Execution refresh — 2026-09-20, R1. Finish the existing RFC through an evidenced decision, R2. Treat the permission boundary as a design obligation, R3. Separate requested permissions, effective access and diagnostics, R4. Make lifecycle and evidence coherence explicit, R5. Separate local validation from global identity arbitration, R6. Bound evaluated work, not just definition size, R7. Keep version tracks and follow-on scope separate (+2 more)

### Community 377 - "Tasks: Complete AddonDefinition Design RFC"
Cohesion: 0.20
Nodes (10): Dependencies and execution order, Implementation strategy, Parallel opportunities, Phase 1: Setup, Phase 2: Foundational evidence, Phase 3: US1 — Review a Complete Safety Boundary (P1), Phase 4: US2 — Predict Definition Lifecycle Outcomes (P1), Phase 5: US3 — Implement from an Unambiguous Contract (P2) (+2 more)

### Community 378 - "Implementation execution evidence"
Cohesion: 0.10
Nodes (20): Adversarial review and integration progress, Authoring CLI and generated inventory — T021/T022, Baseline verification, Final checkpoint checks and development-cluster validation, Foundation progress, Implementation execution evidence, Lifecycle cases, Numeric boundaries (+12 more)

### Community 379 - "Research and decisions"
Cohesion: 0.20
Nodes (10): Clarification research — 2026-09-20, CLI layering, Compatibility and remaining measurements, Evidence and history, Existing verification and generators, Immutable ownership and publication, Pinned lifecycle verification — implementation T006, Research and decisions (+2 more)

### Community 380 - "Tasks: Runtime Addon Definitions"
Cohesion: 0.20
Nodes (10): Dependencies and parallel opportunities, Implementation strategy, Phase 1: Setup, Phase 2: Foundation, Phase 3: US1 — Author installable coverage (P1), Phase 4: US2 — Delegate bounded execution (P1), Phase 5: US3 — Preserve evidence across change (P1), Phase 6: US4 — Install and operate safely (P2) (+2 more)

### Community 382 - "sigs.k8s.io/controller-runtime.Manager"
Cohesion: 0.29
Nodes (3): sigs.k8s.io/controller-runtime.Manager, ClientFactory, New()

### Community 383 - "TestEnvoyGateway_HealthyAndNoGatewaysSkipped"
Cohesion: 0.39
Nodes (6): NewEnvoyGatewayEngine(), egHealthyObjects(), gatewayObject(), TestEnvoyGateway_GatewayConditionScoring(), TestEnvoyGateway_HealthyAndNoGatewaysSkipped(), TestEnvoyGateway_MissingDeploymentFails()

### Community 384 - "successfulCompiler"
Cohesion: 0.61
Nodes (7): NewCache(), revision(), TestCacheCanceledCompilationIsNotCachedOrPinned(), TestCacheIdleLRUAndActiveBounds(), TestCachePinsSurviveIdleEvictionAndCompileOutsideLock(), TestCacheRevisionIsolationAndPanicCleanup(), successfulCompiler()

### Community 385 - "7. Load typed addon definitions under explicit administrator authority"
Cohesion: 0.22
Nodes (8): 7. Load typed addon definitions under explicit administrator authority, Consequences, Considered Options, Context and Problem Statement, Decision Drivers, Decision Outcome, Links, Pros and Cons of the Options

### Community 387 - "RFC Review Contract: AddonDefinition"
Cohesion: 0.25
Nodes (8): Accepted decision and handoff evidence, Completion and Handoff Evidence, Draft walkthrough — 2026-09-20, Requirement Traceability, Review and Approval Lifecycle, RFC Review Contract: AddonDefinition, Scenario Review Matrix, Six Principal Decision Topics

### Community 389 - "Operator RBAC"
Cohesion: 0.33
Nodes (6): Auxiliary roles shipped alongside the operator, Namespace-scoping analysis, Operator ClusterRole rules, Operator RBAC, Runtime-created RBAC, Why these grants are cluster-scoped

### Community 390 - "runMain"
Cohesion: 0.43
Nodes (6): runMain(), TestMain_BadFlagExitsNonZero(), TestMain_ExitsNonZeroOnWriteError(), TestMain_HelpExitsZero(), TestMain_RunsAsMainOnDemand(), TestMain_WritesArtifacts()

### Community 391 - "Runtime acceptance contract"
Cohesion: 0.29
Nodes (7): Authoring interface, Clarification additions — 2026-09-20, Coverage ledger, Enforcement obligations, Lifecycle matrix, Numeric inventory, Runtime acceptance contract

### Community 393 - "Specification Quality Checklist: Complete AddonDefinition Design RFC"
Cohesion: 0.33
Nodes (5): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Complete AddonDefinition Design RFC

### Community 394 - "Specification Quality Checklist: Runtime Addon Definitions"
Cohesion: 0.33
Nodes (5): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Runtime Addon Definitions

### Community 395 - "Data model"
Cohesion: 0.33
Nodes (6): AddonDefinition, AddonDefinitionBinding, Data model, Evidence and attempts, Snapshot and publication context, Transitions and bounds

### Community 396 - "Validation guide"
Cohesion: 0.33
Nodes (5): Implementation checks, Planning checks now, Real-cluster scenarios, Release evidence, Validation guide

### Community 397 - "Network policies"
Cohesion: 0.33
Nodes (5): Network policies, Node-agent DaemonSet (runtime-managed, always on), Operator (static, opt-in), Probe pods (deliberately no Fathom-shipped policy), The `metrics: enabled` label contract

### Community 399 - "TestClientForSetsImpersonationAndMemoizes"
Cohesion: 0.50
Nodes (3): SAUsername(), TestClientForSetsImpersonationAndMemoizes(), TestSAUsername()

### Community 401 - "SAUsername"
Cohesion: 0.67
Nodes (3): SAUsername(), TestClientForSetsImpersonationAndMemoizes(), TestSAUsername()

### Community 403 - "TestRBACRulesDeclaresProbeException"
Cohesion: 0.83
Nodes (3): hasResource(), hasVerb(), TestRBACRulesDeclaresProbeException()

### Community 405 - "Implementation Strategy"
Cohesion: 0.50
Nodes (4): If the #149 deadline gets tight, Implementation Strategy, Incremental Delivery, MVP — User Story 1 only (T001–T016)

### Community 406 - "nodeHealthStatus"
Cohesion: 0.50
Nodes (3): nodeHealthStatusView, dumpNodeHealthDiagnostics(), nodeHealthStatus()

### Community 415 - "Dependencies & Execution Order"
Cohesion: 0.50
Nodes (4): Dependencies & Execution Order, Parallel Opportunities, Phase Dependencies, Within Each User Story

## Knowledge Gaps
- **1321 isolated node(s):** `post-install.sh script`, `common.sh script`, `$schema`, `title`, `type` (+1316 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **30 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Fathom Documentation Index` connect `Fathom Architecture` to `Add-on Checks Guide (Adapter Catalog)`, `0001-addondefinition-crd.md`, `Network policies`, `EnsureCompatible`, `Authoring an Adapter Guide`?**
  _High betweenness centrality (0.056) - this node is a cross-community bridge._
- **Why does `New()` connect `New` to `.Run`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.Name`, `quorum-ratio-rollups/cmd/probe/main_test.go`, `.DeepCopy`, `.DeepCopyInto`, `observeCheck`, `.DeepCopy`, `TestClientForSetsImpersonationAndMemoizes`, `.DeepCopy`, `quorum-ratio-rollups/internal/probe/sweeper_test.go`, `Init`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `.Update`, `.detectAndGateVersion`, `.DeepCopy`, `.DeepCopy`, `newScheme`, `.DeepCopy`, `New`, `join`, `TestAdapterClient`, `.DeepCopyObject`, `.Run`, `.Register`?**
  _High betweenness centrality (0.046) - this node is a cross-community bridge._
- **Why does `Fathom Architecture` connect `Fathom Architecture` to `context.Context`, `testing.T`, `Add-on Checks Guide (Adapter Catalog)`, `BuiltInAdapters`, `AddonCheck`, `EnsureCompatible`, `healthcheck_controller.go`, `DefaultOptions`, `Authoring an Adapter Guide`, `k8s.io/apimachinery/pkg/apis/meta/v1.TypeMeta`?**
  _High betweenness centrality (0.038) - this node is a cross-community bridge._
- **Are the 108 inferred relationships involving `assertHasOutcome()` (e.g. with `TestAnnotationStaleness_NamedLock()` and `TestAnnotationStaleness_NodeList()`) actually correct?**
  _`assertHasOutcome()` has 108 INFERRED edges - model-reasoned connections that need verification._
- **Are the 110 inferred relationships involving `assertHasOutcome()` (e.g. with `TestAnnotationStaleness_NamedLock()` and `TestAnnotationStaleness_NodeList()`) actually correct?**
  _`assertHasOutcome()` has 110 INFERRED edges - model-reasoned connections that need verification._
- **What connects `post-install.sh script`, `common.sh script`, `$schema` to the rest of the system?**
  _1321 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `.Run` be split into smaller, more focused modules?**
  _Cohesion score 0.05822784810126582 - nodes in this community are weakly interconnected._