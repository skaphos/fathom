# Graph Report - chore+deps-refresh-2026-11  (2026-11-07)

## Corpus Check
- 780 files · ~894,799 words
- Verdict: corpus is large enough that graph structure adds value.
- Unclassified: 15 file(s) not represented in the graph (top: (none) 10, .node-agent 1, .probe 1)

## Summary
- 9245 nodes · 23290 edges · 542 communities (447 shown, 95 thin omitted)
- Extraction: 85% EXTRACTED · 15% INFERRED · 0% AMBIGUOUS · INFERRED: 3488 edges (avg confidence: 0.83)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `3b83bb0b`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- newRuntimeCheckFixture
- quorum-ratio-rollups/cmd/probe/main_test.go
- NodeCertificateCheck
- .DeepCopy
- Item
- helperDContractRows
- time.Time
- hostile_input_test.go
- Fathom Documentation Index
- fathom_check_result Gauge (one-hot current result)
- NodeHealthCheck
- observeCheck
- lifecycleDefinition
- deploymentInNamespace
- fakeFactory
- quorum-ratio-rollups/internal/adapter/certmanager/adapter_test.go
- context.Context
- quorum-ratio-rollups/internal/adapter/declarative/engine_test.go
- assertHasOutcome
- NewRootCommand
- .Run
- sigs.k8s.io/controller-runtime/pkg/reconcile.Request
- quorum-ratio-rollups/internal/probe/pod_test.go
- Init
- Probe/Node-Agent Version Lockstep Gate
- quorum-ratio-rollups/internal/controller/healthreport_idempotency.go
- quorum-ratio-rollups/scripts/version_lockstep_gate_test.go
- internal/adapter/certmanager/adapter.go
- internal/adapter/certmanager/adapter_test.go
- cmd/probe/main_test.go
- DefaultOptions
- AddonCheck
- ClusterHealthReconciler
- HealthReportResult
- assertHasDetail
- Execute
- .Run
- newFakeClient
- assertHasDetail
- Tasks: DNSCheck Reconciler
- properties
- Tasks: fathomctl CLI
- Outcome
- deploymentInNamespace
- .DeepCopyInto
- versions
- quorum-ratio-rollups/test/utils/utils.go
- TestScanAndPublishCreatesAndUpdatesConfigMap
- api/v1alpha1/deepcopy_test.go
- .DeepCopy
- common.sh
- Implementation Plan: Pre-1.0 CRD Validation Hardening
- observeCheck
- newScheme
- Implementation Plan: Quorum/Ratio Semantics for Managed-Resource Rollups
- Adversarial Review Findings — v0.5.0 Release Gate (#217)
- nodehealthcheck_helpers.go
- Scan
- Tasks: [FEATURE NAME]
- .claude/skills/speckit-analyze/SKILL.md
- internal/adapter/coredns/adapter_test.go
- github.com/spf13/cobra.Command
- Quickstart Validation: Quorum/Ratio Rollups
- Tasks: DNSCheck Completion
- cli/run.go
- nodeagent_metrics_test.go
- time.Duration
- versions
- .agents/skills/speckit-analyze/SKILL.md
- join
- test/utils/utils.go
- FamilyPolicy
- Tasks: Adversarial Codebase Review for the v0.5.0 Release Gate
- Tasks: Cadence-Aware Staleness Semantics for ClusterHealth
- NodeReport
- Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth
- RuntimeLeadership
- Execution Steps
- ObserveCheck
- internal/controller/nodecertificatecheck_controller_test.go
- execVerb
- properties
- Load
- definition_drain_test.go
- k8s.io/api/core/v1.Pod
- assertFamily
- clientVersion
- PolicyRule
- fakeReleaseClient
- factory
- Feature Specification: DNSCheck Resource Contract
- Implementation Plan: Cadence-Aware Staleness Semantics for ClusterHealth
- runInstall
- cmd/node-agent/main.go
- properties
- test/e2e/healthreport_helpers_test.go
- runProjection
- Guard
- Core Principles
- .Evaluate
- 5. ClusterHealth staleness is the stalest child, and is a signal not a verdict
- internal/controller/nodecertificatecheck_helpers_test.go
- Execution Steps
- DNS Checks
- Contract: `ClusterHealth.Status`
- .Diagnostics
- testing.T
- quorum-ratio-rollups/internal/controller/nodecertificatecheck_helpers.go
- Tasks: Pre-1.0 CRD Validation Hardening
- k8s.io/api/apps/v1.Deployment
- renderRuntimeChart
- fathomctl Reference
- versions
- healthcheck_controller.go
- runReports
- quorum-ratio-rollups/internal/nodecert/paths.go
- Tasks: DNSCheck Resource Contract
- Implementation Plan: DNSCheck Completion
- Research: fathomctl CLI
- annotations
- enabled
- NewBudget
- parseVersion
- addoncheck_controller.go
- NewMetricsServerEngine
- runtime_leadership_test.go
- .agents/skills/speckit-plan/SKILL.md
- upgradePathDecl
- Entity: `DNSCheck`
- k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1.CustomResourceDefinition
- .agents/skills/speckit-specify/SKILL.md
- Repository Guidelines
- values.schema.json
- properties
- .agents/skills/speckit-tasks/SKILL.md
- Feature Specification: [FEATURE NAME]
- k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured
- Tasks: fathomctl Lifecycle Verbs
- .RBACRules
- GitHub Copilot Instructions for Fathom
- internal/nodecert/paths.go
- Feature Specification: Node-agent metrics security
- Tasks: Quorum/Ratio Semantics for Managed-Resource Rollups
- Feature Specification: Adversarial Codebase Review for the v0.5.0 Release Gate
- .claude/skills/speckit-plan/SKILL.md
- .claude/skills/speckit-specify/SKILL.md
- .claude/skills/speckit-tasks/SKILL.md
- Registry
- SetRunningInClusterForTest
- Tasks: Node-agent metrics security
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
- newFakeClient
- Validate
- nodehealth/paths.go
- Fathom Architecture
- NewRootCommand
- Implementation Plan: [FEATURE]
- Entities
- Phase 0 Research: Adversarial Codebase Review for the v0.5.0 Release Gate
- Quickstart: Validating the DNSCheck Resource Contract
- .claude/skills/speckit-checklist/SKILL.md
- Phase 0 Research: Cadence-Aware Staleness Semantics for ClusterHealth
- upgrade_path_gate_test.go
- Quickstart Validation: fathomctl CLI
- assertHasOutcome
- .agents/skills/speckit-checklist/SKILL.md
- .Update
- fathomctl_lifecycle_test.go
- Research: Quorum/Ratio Semantics for Managed-Resource Rollups
- Quickstart Validation: Pre-1.0 CRD Validation Hardening
- Implementation Plan: Adversarial Codebase Review for the v0.5.0 Release Gate
- Quickstart: Validating the v0.5.0 Adversarial Review Release Gate
- Supply-Chain / CI-Integrity Review — Fathom @ cb845dd
- Phase 1 Data Model: Cadence-Aware Staleness Semantics
- Data Model: DNSCheck Completion
- .claude/skills/speckit-clarify/SKILL.md
- .claude/skills/speckit-implement/SKILL.md
- Research: DNSCheck Completion
- .agents/skills/speckit-clarify/SKILL.md
- MustEngine
- Phase 1 Data Model: DNSCheck Reconciler
- .agents/skills/speckit-implement/SKILL.md
- Implementation Plan: fathomctl CLI
- fathomctl
- Specification Quality Checklist: Cadence-Aware Staleness Semantics for ClusterHealth
- addondefinition/validation.go
- sigs.k8s.io/controller-runtime.Manager
- Specification Quality Checklist: Adversarial Codebase Review for the v0.5.0 Release Gate
- Specification Quality Checklist: DNSCheck Resource Contract
- Contract: Probe `dns` Mode CLI
- .claude/skills/speckit-constitution/SKILL.md
- Node Health Checks
- Contract: CRD Schema-Compatibility Gate
- Deliverable Contracts: Findings Report, Refuted Record, Coverage Statement
- Security review candidates — Fathom (commit cb845dd)
- Specification Quality Checklist: DNSCheck Completion
- CheckResult
- Feature Specification: fathomctl CLI
- Contract: HealthCheck Target Projection
- k8s.io/apimachinery/pkg/apis/meta/v1.Time
- versions
- Research: fathomctl Lifecycle Verbs
- Research: Node-agent metrics security
- versions
- .claude/skills/speckit-taskstoissues/SKILL.md
- Quickstart Validation: DNSCheck Completion
- Feature Specification: DNSCheck Reconciler
- bindAddress
- sigs.k8s.io/controller-runtime/pkg/client.Reader
- check-version-lockstep.sh
- scripts/coverage_gate_test.go
- .agents/skills/speckit-constitution/SKILL.md
- Security Policy
- [CHECKLIST TYPE] Checklist: [FEATURE NAME]
- Contract: Runtime Clamp Signal
- Contract: DNSCheck Admission Validation
- Phase 3: User Story 1 — Declare DNS intent and have it validated (P1)
- Phase 0 Research: DNSCheck Reconciler
- Feature Specification: fathomctl Lifecycle Verbs
- definitionE2EAPIFixture
- Implementation Plan: Runtime Addon Definitions
- .DeepCopy
- sync.Mutex
- versions
- RFC 1. AddonDefinition as a CRD — make adapters installable, not compiled in
- Contract: DNSCheck Metrics, Events, and RBAC
- 0008-fathomctl-lifecycle-via-helm-library.md
- Network policies
- quorum-ratio-rollups/api/v1alpha1/groupversion_info_test.go
- .DeepCopyInto
- quorum-ratio-rollups/internal/adapter/declarative/istio_test.go
- projectedLabels
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
- runtime_wiring_test.go
- versions
- github.com/skaphos/fathom
- github.com/skaphos/fathom/tools
- versions
- kedaHealthyObjects
- New
- CompileRuntime
- Implementation Plan: DNSCheck Reconciler
- addondefinition_payloads.go
- Quickstart Validation: DNSCheck Reconciler
- User Scenarios & Testing *(mandatory)*
- .checkCRD
- quorum-ratio-rollups/internal/nodecert/scan_test.go
- dnscheck_test.go
- Specification Quality Checklist: DNSCheck Reconciler
- Rationale: in-process interface over gRPC/OCI/plugin loaders
- versions
- properties
- Implementation Strategy
- CLI-side types (`internal/cli`, unexported)
- factory
- Contract: fathomctl command surface
- Data Model: fathomctl CLI
- internal/adapter/rbacgen/rbacgen.go
- Specification Quality Checklist: fathomctl CLI
- Contract: on-demand run trigger (operator side)
- User Scenarios & Testing *(mandatory)*
- 8. Drive the Fathom lifecycle from fathomctl through the Helm library
- internal/adapter/declarative/field_test.go
- TestEnvoyGateway_HealthyAndNoGatewaysSkipped
- Adversarial Review: fathomctl Lifecycle Verbs (spec 013)
- internal/adapter/declarative/annotation_test.go
- NewExternalDNSEngine
- BuiltinNames
- Implementation Plan: fathomctl Lifecycle Verbs
- Quickstart: Validating fathomctl Lifecycle Verbs
- fathomctl-dist.sh
- versions
- with
- task
- Feature Specification: Complete AddonDefinition Design RFC
- .agents/skills/speckit-adversarial-review/SKILL.md
- .claude/skills/speckit-adversarial-review/SKILL.md
- runtime.md
- Feature Specification: Runtime Addon Definitions
- subjects
- 010-node-agent-metrics-security/data-model.md
- Proposed PR shape
- crd_compat_gate_test.go
- k8s.io/apimachinery/pkg/types.NamespacedName
- 0001-addondefinition-crd.md
- Typed definition wire contract
- Data Model: fathomctl Lifecycle Verbs
- properties
- Design Entities
- RFC execution evidence
- Implementation Plan: Complete AddonDefinition Design RFC
- quorum-ratio-rollups/internal/probe/sweeper_test.go
- internal/controller/tracing_test.go
- Runtime qualification evidence
- AddonDefinition
- sigs.k8s.io/controller-runtime/pkg/client.Client
- properties
- Research: Complete AddonDefinition Design RFC
- Tasks: Complete AddonDefinition Design RFC
- Final checkpoint checks and development-cluster validation
- Research and decisions
- Tasks: Runtime Addon Definitions
- properties
- properties
- fathomctl_test.go
- Specification Quality Checklist: fathomctl Lifecycle Verbs
- 7. Load typed addon definitions under explicit administrator authority
- properties
- RFC Review Contract: AddonDefinition
- image
- properties
- Run
- Runtime acceptance contract
- Contract: fathomctl Lifecycle Verbs
- User Scenarios & Testing *(mandatory)*
- Specification Quality Checklist: Runtime Addon Definitions
- Data model
- with
- Operator RBAC
- Adversarial review — implementation checkpoint
- properties
- .Name
- properties
- .Run
- .DeepCopy
- addondefinition_test.go
- operator_rbac_doc_test.go
- vars
- spec
- internal/nodecert/scan_test.go
- .DeepCopyInto
- tasks
- runUpgrade
- internal/app/run_happy_test.go
- DefinitionReference
- runArgoCD
- Dependencies & Execution Order
- .DeepCopy
- ControlTargets
- .DeepCopyInto
- normalizeShell
- Node-agent DaemonSet (on-disk cert scanner)
- MustEngine
- Adapter
- crd_kustomization_test.go
- check-ghcr-public.sh
- DefinitionVersionSource
- DNSResolver
- quorum-ratio-rollups/internal/controller/policy_validation_test.go
- port
- DNSTarget
- NodeHealthCheckItem
- with
- values
- options.go
- Pod
- Load
- env
- Budget
- internal/metrics/metrics.go
- verify-public
- TestAnnotationStaleness_NamedLock
- NewScheduler
- Chart.yaml
- jobs
- with
- requests
- validateAddonCheckPolicy
- name
- with
- snapshot_test.go
- releases
- addoncheck_types.go
- shard
- internal/adapter/declarative/versiondetect_test.go
- Implementation execution evidence
- runDescribe
- runTarget
- env
- TestExternalSecrets_HealthyAndEmptySyncSkipped
- quorum-ratio-rollups/internal/controller/tracing_test.go
- e2e
- e2e.yml
- steps
- env
- runMain
- quorum-ratio-rollups/internal/app/run_happy_test.go
- fathomctl-build
- runLs
- cRecordingContext
- build
- summary
- env
- plan
- steps
- User Scenarios & Testing *(mandatory)*
- .DeepCopy
- .DeepCopy
- steps
- steps
- lint
- reuse
- on
- on
- checkGauges
- Opt-in preview qualification guide
- cmds
- test-e2e
- fathomctl-dist
- repositories
- .DeepCopyInto
- .DeepCopy
- .DeepCopy
- .DeepCopy
- Runtime add-on definitions (qualification preview)
- Dependencies & Execution Order
- bundle-build
- config
- pilot
- DefinitionBindingScope
- DefinitionObjectReference
- DefinitionTarget
- DNSTargetResult
- on
- bundle-push
- catalog-build
- catalog-push
- clean
- crd-compat
- default
- docker-build
- docker-buildx-push
- docker-push
- e2e:cluster:addons
- e2e:cluster:down
- e2e:cluster:samples
- fmt
- gen:addon-rbac
- generate
- helm:sync
- images:refresh
- manifests
- node-agent-build
- node-agent-docker-build
- node-agent-docker-buildx-push
- node-agent-docker-push
- probe-build
- probe-docker-build
- probe-docker-buildx-push
- probe-docker-push
- test
- verify-alert-rules
- verify-generated
- verify-version-lockstep
- vet
- vuln
- image
- webhook
- global
- operator
- selfMonitor

## God Nodes (most connected - your core abstractions)
1. `assertHasOutcome()` - 142 edges
2. `assertHasOutcome()` - 122 edges
3. `CheckResult` - 109 edges
4. `newRuntimeCheckFixture()` - 87 edges
5. `execVerb()` - 85 edges
6. `newFakeClient()` - 80 edges
7. `New()` - 76 edges
8. `Family` - 76 edges
9. `newFakeClient()` - 76 edges
10. `NewBudget()` - 68 edges

## Surprising Connections (you probably didn't know these)
- `EnsureCompatible()` --semantically_similar_to--> `CRD Maturity Ladder (alpha/beta/GA)`  [INFERRED] [semantically similar]
  pkg/adapter/version.go → docs/reference/api-versioning.md
- `HealthCheckReconciler` --references--> `Fathom Architecture`  [EXTRACTED]
  internal/controller/healthcheck_controller.go → docs/architecture.md
- `NodeCertificateCheckReconciler` --references--> `Fathom Architecture`  [EXTRACTED]
  internal/controller/nodecertificatecheck_controller.go → docs/architecture.md
- `NodeCertificateCheckReconciler` --references--> `Node-agent DaemonSet (on-disk cert scanner)`  [EXTRACTED]
  internal/controller/nodecertificatecheck_controller.go → docs/guides/node-certificate-checks.md
- `BuiltInAdapters()` --references--> `Fathom Architecture`  [EXTRACTED]
  internal/app/run.go → docs/architecture.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **AddonCheck → HealthCheck → ClusterHealth chain with HealthReport history** — readme_addoncheck, readme_healthcheck, readme_clusterhealth, readme_healthreport, readme_aggregation_chain [EXTRACTED 1.00]
- **Alerting-grade observability contract surface (gauges, events, alert rules)** — specs_001_alerting_observability_contracts_metrics_fathom_check_result, specs_001_alerting_observability_contracts_metrics_fathom_check_last_run_timestamp_seconds, specs_001_alerting_observability_contracts_metrics_alert_rules, specs_001_alerting_observability_contracts_events_resultchanged, specs_001_alerting_observability_contracts_events_failure_reasons [EXTRACTED 1.00]
- **Probe-pod lifecycle: build, launch, parse, sweep orphans** — internal_probe_pod_pod, internal_probe_launcher_launcher, internal_probe_sweeper_sweeper, docs_architecture_probe_pod_model [EXTRACTED 1.00]
- **Speckit artifact pipeline for feature 001 (spec → plan → tasks → quickstart)** — specs_001_alerting_observability_spec_alerting_observability, specs_001_alerting_observability_plan_alerting_observability_plan, specs_001_alerting_observability_tasks_alerting_observability_tasks, specs_001_alerting_observability_quickstart_validation [EXTRACTED 1.00]

## Communities (542 total, 95 thin omitted)

### Community 0 - "newRuntimeCheckFixture"
Cohesion: 0.06
Nodes (103): healthReportCount(), AddonCheckEvidence, absentReportingAdapter, countingStatusClient, countingStatusWriter, failingReportCreateClient, fakeAddonAdapter, fakeEvaluatorFactory (+95 more)

### Community 1 - "quorum-ratio-rollups/cmd/probe/main_test.go"
Cohesion: 0.09
Nodes (41): main(), metricsMux(), publishGauges(), run(), runDNS(), runHTTPGet(), runTCPConnect(), runTCPListen() (+33 more)

### Community 2 - "NodeCertificateCheck"
Cohesion: 0.08
Nodes (38): NodeCertificateCheck, NodeCertificateCheckSpec, NodeCertificateCheckStatus, NodeCertificateCheckReconciler, nodeCertRollupDecision, reportRejection, ensureScopedReportRBAC(), admissionPolicyUnsupported() (+30 more)

### Community 3 - ".DeepCopy"
Cohesion: 0.29
Nodes (19): deepCopyContract(), fullyPopulatedAddonCheck(), fullyPopulatedClusterHealth(), fullyPopulatedHealthCheck(), fullyPopulatedHealthReport(), fullyPopulatedNodeCertificateCheck(), runtimeObjectContract(), TestDeepCopy_AddonCheck() (+11 more)

### Community 4 - "Item"
Cohesion: 0.10
Nodes (28): nodeHealthMetricsPort(), joinNodeHealthArgs(), nodeHealthAgentItems(), nodeHealthHostMetricsPort(), nodeHealthNeedsHostNetwork(), nodeHealthNeedsRoot(), nodeHealthReportCoversSpec(), nodeHealthSocketPaths() (+20 more)

### Community 5 - "helperDContractRows"
Cohesion: 0.09
Nodes (45): helperDBudget(), helperDCacheRecompiles(), helperDChecks(), helperDContractRows(), helperDDrain(), helperDEnqueue(), helperDEvidence(), helperDGrants() (+37 more)

### Community 6 - "time.Time"
Cohesion: 0.12
Nodes (33): TestHealthReportResultSeverity_EmptyAndUnrecognizedReturnZero(), TestHealthReportResultSeverity_OrderingAcrossEnumValues(), TestHealthReportResultSeverity_PassIsLowestNonZero(), reportWriterClient(), writeReportAtName(), writeReportWithAnnotation(), nodeAgentClient(), writeNodeReport() (+25 more)

### Community 7 - "hostile_input_test.go"
Cohesion: 0.24
Nodes (22): helperEClient(), helperEFailureReason(), helperEFieldCheck(), helperEGoroutineBaseline(), helperEGoroutinesSettle(), helperEGuard(), helperEHealthyDefinition(), helperEHostileDefinition() (+14 more)

### Community 8 - "Fathom Documentation Index"
Cohesion: 0.25
Nodes (23): AddonCheck CRD, ClusterHealth CRD (aggregate), HealthCheck CRD (thin wrapper), HealthReport CRD (immutable history), HealthReportResult Severity Enum, NodeCertificateCheck CRD, ADR-0002 HealthReport as First-class CRD, Rationale: CRD history without external storage dependency (+15 more)

### Community 9 - "fathom_check_result Gauge (one-hot current result)"
Cohesion: 0.06
Nodes (49): ClusterHealth External Contract (derived only from HealthCheck.status), Cobra+Viper Configuration Model (flag → env → file → default), Run e2e After Major Changes Policy, AGENTS.md Repository Guidelines (CLAUDE.md symlink), SPDX Boilerplate Header, Breaking Change: ClusterHealth Made Cluster-Scoped (0.4.0), DCO Sign-Off Requirement, Contributor Safety Expectations (Bounded Work, Minimal RBAC) (+41 more)

### Community 10 - "NodeHealthCheck"
Cohesion: 0.13
Nodes (16): NodeHealthCheck, NodeHealthCheckStatus, NodeHealthCheckReconciler, addonCheckDueForRun(), consumeNodeHealthRunTrigger(), nodeHealthTemplateToken(), observeNodeHealthReports(), nodeHealthAgentLabels() (+8 more)

### Community 11 - "observeCheck"
Cohesion: 0.11
Nodes (27): ctrlRegistryGather(), gatherCheckSeries(), gatherOneHot(), TestDeleteCheckSeries(), TestObserveCheckFlipsResult(), TestObserveCheckOneHotInvariant(), TestObserveCheckSentinels(), checkGaugeValue() (+19 more)

### Community 12 - "lifecycleDefinition"
Cohesion: 0.05
Nodes (115): AddonDefinitionStatus, DefinitionStatusCondition, AddonDefinitionBinding, AddonDefinitionBindingSpec, AddonDefinitionBindingStatus, DefinitionLeaderEpoch, DefinitionObjectReference, DefinitionReference (+107 more)

### Community 13 - "deploymentInNamespace"
Cohesion: 0.14
Nodes (38): assertNoOutcome(), NewArgoCDEngine(), argoApp(), argocdHealthyObjects(), runArgoCD(), TestArgoCD_AbsentClusterFails(), TestArgoCD_ApplicationStateRollup(), TestArgoCD_HealthyWithSyncedApplication() (+30 more)

### Community 14 - "fakeFactory"
Cohesion: 0.09
Nodes (50): TestDefinitionBindReviewedUIDs(), squash(), TestCadenceRendering(), TestDescribe_NeverRunAndNotFound(), TestDescribe_PerKind(), TestDescribe_StructuredIsUnmodified(), lsFixtures(), TestLs_AllNamespacesAndNeverRun() (+42 more)

### Community 15 - "quorum-ratio-rollups/internal/adapter/certmanager/adapter_test.go"
Cohesion: 0.09
Nodes (76): adapterWithLauncher(), assertFamily(), assertNoKind(), assertNoTarget(), certManagerResource(), daemonSetWithStatus(), dnsEndpointSlice(), dnsEndpointSliceNamed() (+68 more)

### Community 16 - "context.Context"
Cohesion: 0.06
Nodes (36): endAdapterRunSpan(), skipped(), ThresholdAdvertiser, gateAdapter, legacyRatioAdapter, runtimeCheckAdapter, resolveFamily(), endAdapterRunSpan() (+28 more)

### Community 17 - "quorum-ratio-rollups/internal/adapter/declarative/engine_test.go"
Cohesion: 0.27
Nodes (21): NewCiliumEngine(), assertFamily(), assertHasDetail(), assertHasOutcome(), assertNoKind(), assertNoOutcome(), ciliumCRDNames(), daemonSetInNamespace() (+13 more)

### Community 18 - "assertHasOutcome"
Cohesion: 0.18
Nodes (30): assertHasOutcome(), runManaged(), TestCondition_ClusterScopedListsWithoutNamespace(), TestCondition_ConditionStatus(), TestCondition_InvalidAPIVersionErrors(), TestCondition_InvalidSelectorErrors(), TestCondition_ListErrorDescribesNamespaceScope(), TestCondition_ListNameFallsBackToKind() (+22 more)

### Community 19 - "NewRootCommand"
Cohesion: 0.23
Nodes (11): NewRootCommand(), signalContext(), TestSignalContext_PropagatesParentCancellation(), TestSignalContext_SIGINTCancels(), TestSignalContext_SIGTERMCancels(), TestSignalContext_StopReleasesContext(), TestNewRootCommand_BasicWiring(), TestNewRootCommand_HelpDoesNotErrorWithoutKubeconfig() (+3 more)

### Community 20 - ".Run"
Cohesion: 0.05
Nodes (67): TestCountAbsent(), establishedCRD(), establishedCRDServing(), TestCheckCRD_HonorsPerCRDVersions(), TestFamilyOutcome(), TestOutcomeValid(), TestRun_EmitsSpan(), TestAzureWorkloadIdentity_AbsentWebhookFails() (+59 more)

### Community 21 - "sigs.k8s.io/controller-runtime/pkg/reconcile.Request"
Cohesion: 0.21
Nodes (6): bindingRequests(), bindingsForServiceAccountReference(), requestNames(), checkForReportConfigMap(), checkForNodeHealthReportConfigMap(), TestCheckForNodeHealthReportConfigMap()

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
Cohesion: 0.07
Nodes (51): certificateCheck(), certificateDetails(), check(), conditionDetails(), conditionStatus(), conditionType(), daysRemaining(), daysThreshold() (+43 more)

### Community 52 - "internal/adapter/certmanager/adapter_test.go"
Cohesion: 0.10
Nodes (54): clientObject, webhookEntry, New(), assertFamily(), assertHasDetail(), assertHasOutcome(), assertNoKind(), assertNoOutcome() (+46 more)

### Community 53 - "cmd/probe/main_test.go"
Cohesion: 0.08
Nodes (60): absoluteDNSName(), result, join(), lookupCNAME(), lookupIPs(), lookupSRV(), main(), missingAnswers() (+52 more)

### Community 54 - "DefaultOptions"
Cohesion: 0.10
Nodes (30): appFakeAdapter, DefaultOptions(), TestValidate(), TestValidate_MetricsProjectedLabels(), TestValidate_MultipleErrorsAccumulate(), BuildAdapterRegistry(), BuildManagerOptions(), NewScheme() (+22 more)

### Community 55 - "AddonCheck"
Cohesion: 0.08
Nodes (44): AddonCheck, AddonCheckRuntimeRunner, publicationCandidate, publicationRank, RuntimeAttempt, RuntimeDispatchResolver, RuntimeEvaluatorFactory, RuntimeExecutionSession (+36 more)

### Community 56 - "ClusterHealthReconciler"
Cohesion: 0.11
Nodes (19): ClusterHealthReconciler, clearClusterHealthAggregateStatus(), clusterHealthCoversNamespace(), clusterHealthSelectsHealthCheck(), namespaceScope(), rankSeverity(), selectorFromSpec(), truncateChildren() (+11 more)

### Community 57 - "HealthReportResult"
Cohesion: 0.05
Nodes (66): DNSCheck, DNSCheckSpec, DNSCheckStatus, DNSResolver, DNSTarget, DNSTargetResult, targets(), DNSCheckReconciler (+58 more)

### Community 58 - "assertHasDetail"
Cohesion: 0.16
Nodes (31): assertHasDetail(), NewAzureWorkloadIdentityEngine(), healthyAzureWIObjects(), TestAzureWorkloadIdentity_Capabilities(), TestAzureWorkloadIdentity_HealthyClusterAllPass(), TestAzureWorkloadIdentity_NoOptedInPodsProjectionSkipped(), TestAzureWorkloadIdentity_UnpopulatedCABundleFails(), TestIstio_RelocatedControlPlanePasses() (+23 more)

### Community 59 - "Execute"
Cohesion: 0.16
Nodes (23): FailureSummary(), Budget, SealResult(), checkResult(), resultBudget(), TestCompletedVerdictsAreEvidenceButEmptyRunIsNot(), TestEvidenceStringBudgetRejectsBeforeMarshal(), TestResultBoundaries() (+15 more)

### Community 60 - ".Run"
Cohesion: 0.17
Nodes (45): assertCheck(), findCheck(), ksmService(), passingLauncher(), readyPod(), runRequest(), TestRun_AllFamiliesDisabledEmitsSentinelSkip(), TestRun_HealthyDeploymentAndEndpointsPass() (+37 more)

### Community 61 - "newFakeClient"
Cohesion: 0.15
Nodes (27): clientObject, NewCiliumEngine(), TestCondition_ResolveVersion(), assertNoKind(), ciliumCRDNames(), daemonSetInNamespace(), establishedCRD(), healthyObjects() (+19 more)

### Community 62 - "assertHasDetail"
Cohesion: 0.23
Nodes (24): TestAzureWorkloadIdentity_UnpopulatedCABundleFails(), assertHasDetail(), TestIstio_RelocatedControlPlanePasses(), boolPtr(), endpointSlice(), mutatingConfig(), runWebhook(), runWebhookPolicy() (+16 more)

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
Cohesion: 0.08
Nodes (19): programmableAdapter, AddonDefinition, Evaluator, FamilyDefinition, Posture, runtimeStep, VersionSource, WorkloadKind (+11 more)

### Community 67 - "deploymentInNamespace"
Cohesion: 0.13
Nodes (35): TestArgoCD_PolicyOverridesWorkloadNames(), TestArgoCD_UnavailableRepoServerFails(), NewDeschedulerEngine(), TestDescheduler_AbsentClusterAllSkipped(), TestDescheduler_HealthyCronJobMode(), TestDescheduler_HealthyDeploymentMode(), TestDescheduler_InvalidPolicyFails(), assertNoOutcome() (+27 more)

### Community 68 - ".DeepCopyInto"
Cohesion: 0.03
Nodes (26): AddonCheckEvidence, AddonCheckEvidenceAuthority, AddonCheckEvidenceRevision, AddonDefinitionBindingSpec, AddonDefinitionBindingStatus, AddonDefinitionSpec, AddonDefinitionStatus, DefinitionAnnotationStaleness (+18 more)

### Community 69 - "versions"
Cohesion: 0.05
Nodes (36): jsonPath, name, type, controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations (+28 more)

### Community 70 - "quorum-ratio-rollups/test/utils/utils.go"
Cohesion: 0.12
Nodes (23): TestE2EShardPlannerKnowsEveryOptInAddon(), TestE2E(), AddonSelection, CoreAddons(), GetNonEmptyLines(), GetProjectDir(), InstallPrometheusOperator(), IsPrometheusCRDsInstalled() (+15 more)

### Community 71 - "TestScanAndPublishCreatesAndUpdatesConfigMap"
Cohesion: 0.18
Nodes (14): parseConfig(), splitCSV(), TestParseConfig(), TestSanitizeLabelValue(), TestScanAndPublishCreatesAndUpdatesConfigMap(), TestSplitCSV(), writeCert(), writeReportWithAnnotation() (+6 more)

### Community 72 - "api/v1alpha1/deepcopy_test.go"
Cohesion: 0.15
Nodes (25): deepCopyContract(), fullyPopulatedAddonCheck(), fullyPopulatedClusterHealth(), fullyPopulatedDNSCheck(), fullyPopulatedHealthCheck(), fullyPopulatedHealthReport(), fullyPopulatedNodeCertificateCheck(), fullyPopulatedNodeHealthCheck() (+17 more)

### Community 73 - ".DeepCopy"
Cohesion: 0.06
Nodes (11): AddonDefinition, AddonDefinitionBinding, AddonDefinitionBindingList, AddonDefinitionList, DNSCheck, DNSCheckList, HealthCheck, NodeHealthCheck (+3 more)

### Community 74 - "common.sh"
Cohesion: 0.13
Nodes (29): check-prerequisites.sh script, check_dir(), check_file(), find_specify_root(), format_speckit_command(), get_current_branch(), get_feature_paths(), get_invoke_separator() (+21 more)

### Community 75 - "Implementation Plan: Pre-1.0 CRD Validation Hardening"
Cohesion: 0.07
Nodes (27): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Pre-1.0 CRD Validation Hardening, Complexity Tracking, Constitution Check, Documentation (this feature) (+19 more)

### Community 76 - "observeCheck"
Cohesion: 0.17
Nodes (22): acceptedCondition(), durp(), findCondition(), TestAddonCheckCadenceHelpersClamp(), TestCadenceClampMessages(), TestNodeCertCadenceHelpersClamp(), TestObserveCheckEmitsCadenceClampedOncePerEpisode(), TestSetAddonCheckAcceptedClamped() (+14 more)

### Community 77 - "newScheme"
Cohesion: 0.09
Nodes (24): addonSA(), TestAdapterClient(), TestRunAddonCheckFailsClosedWhenNamespaceEmptyInCluster(), TestRunAddonCheckFailsClosedWithoutScopedClient(), TestDefaultControllers_InClusterRequiresNamespace(), defaultRunningInCluster(), inClusterFromConfigErr(), TestInClusterFromConfigErr() (+16 more)

### Community 78 - "Implementation Plan: Quorum/Ratio Semantics for Managed-Resource Rollups"
Cohesion: 0.07
Nodes (25): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Quorum/Ratio Semantics for Managed-Resource Rollups, Complexity Tracking, Constitution Check, Documentation (this feature) (+17 more)

### Community 79 - "Adversarial Review Findings — v0.5.0 Release Gate (#217)"
Cohesion: 0.07
Nodes (24): Coverage Statement — v0.5.0 Release Gate (#217), Intentionally excluded, Perspective results (SC-001), Post-anchor deltas, Reviewed, Scope notes, Adversarial Review Findings — v0.5.0 Release Gate (#217), API-1: HealthCheck status.summary MaxLength=1024 wedges mirroring on long condition messages (high) (+16 more)

### Community 80 - "nodehealthcheck_helpers.go"
Cohesion: 0.06
Nodes (58): DefaultNodeHealthConditions(), NodeHealthCheckItem, NodeHealthCheckSpec, NodeHealthNodeResult, nodeHealthEvaluation, controlPlaneTolerations(), nodeOutcomeToResult(), putIfNotEmpty() (+50 more)

### Community 81 - "Scan"
Cohesion: 0.14
Nodes (25): containerRuntime(), StatfsGuard, headroom(), kubeletHealthz(), NewStatfsGuard(), probeTimedOut(), Scan(), fakeStatfs() (+17 more)

### Community 82 - "Tasks: [FEATURE NAME]"
Cohesion: 0.07
Nodes (26): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation Strategy, Incremental Delivery, MVP First (User Story 1 Only) (+18 more)

### Community 83 - ".claude/skills/speckit-analyze/SKILL.md"
Cohesion: 0.08
Nodes (25): 1. Initialize Analysis Context, 2. Load Artifacts (Progressive Disclosure), 3. Build Semantic Models, 4. Detection Passes (Token-Efficient Analysis), 5. Severity Assignment, 6. Produce Compact Analysis Report, 7. Provide Next Actions, 8. Offer Remediation (+17 more)

### Community 84 - "internal/adapter/coredns/adapter_test.go"
Cohesion: 0.15
Nodes (38): clientObject, New(), adapterWithLauncher(), assertHasDetail(), assertHasOutcome(), assertNoOutcome(), dnsEndpointSlice(), dnsEndpointSliceNamed() (+30 more)

### Community 85 - "github.com/spf13/cobra.Command"
Cohesion: 0.15
Nodes (13): globalOptions, newDefinitionBindCommand(), newDefinitionCollisionsCommand(), newDefinitionDrainCommand(), newDefinitionCommand(), newDefinitionRenderCommand(), readDefinition(), commandContext() (+5 more)

### Community 86 - "Quickstart Validation: Quorum/Ratio Rollups"
Cohesion: 0.08
Nodes (22): Configuration surface (AddonCheck), Contract: Ratio Rollup Thresholds and Report Entries, Explicit non-changes, Metrics interplay (informative), Rejection (Accepted condition), Report surface (HealthReport), Verdict semantics, Data Model: Quorum/Ratio Semantics for Managed-Resource Rollups (+14 more)

### Community 87 - "Tasks: DNSCheck Completion"
Cohesion: 0.07
Nodes (27): Dependencies and Execution Order, Documentation for User Story 4, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation Strategy, Incremental Delivery (+19 more)

### Community 88 - "cli/run.go"
Cohesion: 0.12
Nodes (34): checkRef, kindDescriptor, runOptions, runOutcome, sourceResolution, executableKinds(), kindByName(), kindNamesForHelp() (+26 more)

### Community 89 - "nodeagent_metrics_test.go"
Cohesion: 0.18
Nodes (18): agentEndpoint, anonymousStatusCommand(), assertAgentDoesNotServeMetrics(), assertCertificateMetrics(), assertNodeHealthMetrics(), authenticatedStatusCommand(), daemonSetAgentEndpoint(), ensureMetricsTestNamespace() (+10 more)

### Community 90 - "time.Duration"
Cohesion: 0.20
Nodes (25): lsRow, snapshot, cadence(), addonCheckSnapshot(), addonCheckTimeout(), clusterHealthSnapshot(), dnsCheckSnapshot(), dnsCheckTimeout() (+17 more)

### Community 91 - "versions"
Cohesion: 0.05
Nodes (36): jsonPath, name, type, controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations (+28 more)

### Community 92 - ".agents/skills/speckit-analyze/SKILL.md"
Cohesion: 0.08
Nodes (25): 1. Initialize Analysis Context, 2. Load Artifacts (Progressive Disclosure), 3. Build Semantic Models, 4. Detection Passes (Token-Efficient Analysis), 5. Severity Assignment, 6. Produce Compact Analysis Report, 7. Provide Next Actions, 8. Offer Remediation (+17 more)

### Community 93 - "join"
Cohesion: 0.13
Nodes (27): serviceAccountToken(), join(), applyManifest(), scrapeOperatorMetrics(), newTestFlags(), TestDefaultOptions_MatchFlagDefaults(), TestLoad_ConfigOverridesDefault(), TestLoad_EnvOverridesConfig() (+19 more)

### Community 94 - "test/utils/utils.go"
Cohesion: 0.10
Nodes (27): TestE2EShardPlannerClassifiesPaths(), TestE2EShardPlannerIncludesTheLifecycleShard(), TestE2EShardPlannerKnowsEveryOptInAddon(), TestE2E(), lifecycleVersions(), CoreAddons(), GetProjectDir(), InstallPrometheusOperator() (+19 more)

### Community 95 - "FamilyPolicy"
Cohesion: 0.05
Nodes (57): adapterOutcome(), certManagerComponents(), csvThreshold(), dnsProbePodName(), dnsTargets(), endpointTarget(), familyForTarget(), int32Threshold() (+49 more)

### Community 96 - "Tasks: Adversarial Codebase Review for the v0.5.0 Release Gate"
Cohesion: 0.09
Nodes (21): Consolidation and refutation, Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation Strategy, Incremental Delivery, MVP First (US1 only), Notes, Parallel Example: User Story 1 (+13 more)

### Community 97 - "Tasks: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.08
Nodes (24): Format: `[ID] [P?] [Story] Description`, If the #149 deadline gets tight, Implementation, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation Strategy, Incremental Delivery (+16 more)

### Community 98 - "NodeReport"
Cohesion: 0.16
Nodes (18): ensureNode(), healthyNodeConditions(), newNHC(), newNodeHealthReconciler(), nodeHealthAgentClient(), scheduleNodeHealthAgentPods(), setNodeHealthDaemonSetStatus(), writeNodeHealthReport() (+10 more)

### Community 99 - "Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.10
Nodes (20): Assumptions, Clarifications, D1 — Staleness is a signal, never a verdict change, D2 — Cadence is published for self-scheduling kinds; the aggregate is fixed at its derivation, D3 — "Staleness" is the canonical term; "freshness" is not used, Dependencies and Constraints, Edge Cases, Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth (+12 more)

### Community 100 - "RuntimeLeadership"
Cohesion: 0.11
Nodes (4): DrainAcknowledgement, RuntimeLeadership, processUniqueHolderIdentity(), sleepContext()

### Community 101 - "Execution Steps"
Cohesion: 0.12
Nodes (15): 1. Initialize Convergence Context, 2. Load Artifacts (Progressive Disclosure), 3. Build the Intent Inventory, 4. Assess the Codebase and Classify Findings, 5. Assign Severity, 6. Present the In-Session Findings Summary, 7. Append Convergence Tasks (or report converged), 8. Provide Next Actions (Handoff) (+7 more)

### Community 102 - "ObserveCheck"
Cohesion: 0.14
Nodes (32): ctrlRegistryGather(), gatherCheckSeries(), gatherDNSTargetSeries(), gatherOneHot(), TestCheckIntervalSeries(), TestCheckIntervalWithdrawnWhenUnresolvable(), TestCheckResultValuesMatchAPIVocabulary(), TestDeleteCheckSeries() (+24 more)

### Community 103 - "internal/controller/nodecertificatecheck_controller_test.go"
Cohesion: 0.05
Nodes (30): fakeDNSLauncher, dnsProbeLauncher, adapterOutcome(), dnsProbePodName(), runDNSProbe(), TestDNSProbePodNameIsDNS1123Compliant(), adapterOutcome(), adapterOutcome() (+22 more)

### Community 104 - "execVerb"
Cohesion: 0.06
Nodes (97): eventLog, lifecycleEnv, seedRelease(), TestImageRepositoriesMatchTheChart(), TestInstallDevelopmentBuildPinsByTag(), TestInstallFreshCluster(), TestInstallOntoCompatibleLeftoverCRDsKeepsResources(), TestInstallRefusals() (+89 more)

### Community 105 - "properties"
Cohesion: 0.14
Nodes (16): pattern, type, properties, properties, required, type, digest, probeImage (+8 more)

### Community 106 - "Load"
Cohesion: 0.21
Nodes (20): Load(), RegisterFlags(), newTestFlags(), TestDefaultOptions_MatchFlagDefaults(), TestLoad_ConfigOverridesDefault(), TestLoad_DNSCheckMaxConcurrentProbesPrecedence(), TestLoad_EnvOverridesConfig(), TestLoad_FlagOverridesEverything() (+12 more)

### Community 107 - "definition_drain_test.go"
Cohesion: 0.11
Nodes (31): drainReader, writeRecorder, ExitCode(), TestDefinitionCollisions(), leaseEpoch(), sameEpoch(), documentedLeaderElectionID(), drainCommandFactory() (+23 more)

### Community 108 - "k8s.io/api/core/v1.Pod"
Cohesion: 0.08
Nodes (32): boundedNodeList(), maxRestartCount(), podReady(), podTarget(), check(), familyForTarget(), maxRestartCount(), podReady() (+24 more)

### Community 109 - "assertFamily"
Cohesion: 0.26
Nodes (14): assertFamily(), NewIstioEngine(), istioAmbientObjects(), istioCRDObjects(), istiodControlPlane(), istioHealthyObjects(), istioInjectorConfig(), istioValidatorConfig() (+6 more)

### Community 110 - "clientVersion"
Cohesion: 0.13
Nodes (21): commandError, crdStatus, operatorVersion, statusReport, versionInfo, releaseInfo(), cliChartVersion(), normalizeChartVersion() (+13 more)

### Community 111 - "PolicyRule"
Cohesion: 0.16
Nodes (13): hasResource(), hasVerb(), TestRBACRulesDeclaresDryRunException(), hasResource(), hasVerb(), TestRBACRulesDeclaresProbeException(), hasResource(), hasVerb() (+5 more)

### Community 112 - "fakeReleaseClient"
Cohesion: 0.13
Nodes (11): fakeReleaseClient, helmRelease, helmReleaseClient, recordingLister, releaseRequest, valueFlags, asRelease(), chartMetadata() (+3 more)

### Community 113 - "factory"
Cohesion: 0.15
Nodes (5): stubClientConfig, defaultClientConfig(), factory, newScheme(), TestDefaultClientConfig_RealKubeconfig()

### Community 114 - "Feature Specification: DNSCheck Resource Contract"
Cohesion: 0.17
Nodes (12): Assumptions, Clarifications, Dependencies, Feature Specification: DNSCheck Resource Contract, Functional Requirements, Key Entities, Measurable Outcomes, Out of Scope (+4 more)

### Community 115 - "Implementation Plan: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.13
Nodes (15): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Phases, Implementation Plan: Cadence-Aware Staleness Semantics for ClusterHealth, Phase A — Staleness derivation (the reported defect), Phase B — Cadence publication, Phase C — Shipped alerting rules (+7 more)

### Community 116 - "runInstall"
Cohesion: 0.11
Nodes (27): imagePinning, installResult, lifecycleClients, lifecycleOptions, releaseClient, usageError, imageReferences(), imageValues() (+19 more)

### Community 117 - "cmd/node-agent/main.go"
Cohesion: 0.08
Nodes (52): boundedContext(), certificatePass(), certificatePassWithClock(), TestHealthzReflectsProgress(), TestLivenessFollowsPublication(), TestMetricsEndpointIsNotServed(), TestOnceFailsWhenThePassDoesNotPublish(), TestParseConfigHealthMode() (+44 more)

### Community 118 - "properties"
Cohesion: 0.12
Nodes (17): type, type, type, type, type, allOf, $comment, properties (+9 more)

### Community 119 - "test/e2e/healthreport_helpers_test.go"
Cohesion: 0.16
Nodes (15): checkResult, eventList, healthReport, healthReportList, addonCheckLastResult(), addonCheckReadyTrue(), dumpAddonCheckDiagnostics(), latestHealthReport() (+7 more)

### Community 120 - "runProjection"
Cohesion: 0.21
Nodes (15): NewAzureWorkloadIdentityEngine(), healthyAzureWIObjects(), TestAzureWorkloadIdentity_AbsentWebhookFails(), TestAzureWorkloadIdentity_Capabilities(), TestAzureWorkloadIdentity_HealthyClusterAllPass(), TestAzureWorkloadIdentity_NoOptedInPodsProjectionSkipped(), optedInPod(), runProjection() (+7 more)

### Community 121 - "Guard"
Cohesion: 0.06
Nodes (36): helperBUrequest, injectedHeaders, RuntimeFactory, runtimeIdentityTransport, TestRuntimeControlReaderSharesBudgetWithoutSharingIdentity(), RuntimeAuthority, hasFailureReason(), NewRuntimeFactory() (+28 more)

### Community 122 - "Core Principles"
Cohesion: 0.12
Nodes (16): Core Principles, Development Workflow & Quality Gates, Engineering Constraints, Fathom Constitution, Fathom-Specific Constraints, Governance, I. Explicit State Over Implicit Behavior, II. Git Is the Durable Desired-State Boundary (+8 more)

### Community 123 - ".Evaluate"
Cohesion: 0.12
Nodes (18): PodProjectionCheck, versionAddress, capNames(), containerHasEnv(), formatSelector(), PodProjectionCheck, hasProjectedTokenVolume(), TestPodProjection_CapNames() (+10 more)

### Community 124 - "5. ClusterHealth staleness is the stalest child, and is a signal not a verdict"
Cohesion: 0.15
Nodes (13): 5. ClusterHealth staleness is the stalest child, and is a signal not a verdict, Accepted tradeoff: detection latency in mixed aggregates, Alternative rejected: add a parallel field, Consequences, Context and Problem Statement, Decision, Rationale, References (+5 more)

### Community 125 - "internal/controller/nodecertificatecheck_helpers_test.go"
Cohesion: 0.09
Nodes (35): runtimeSchema(), TestDefinitionCanonicalByteCapAfterAdmissionDefaults(), TestDefinitionMaximumCheckAdmissionCost(), TestDefinitionSchemaLimitParity(), TestAddonDefinitionAllPayloadAdmission(), TestPayloadStructuralContractRejection(), firstRuntimeCheck(), runtimeDefinition() (+27 more)

### Community 126 - "Execution Steps"
Cohesion: 0.12
Nodes (15): 1. Initialize Convergence Context, 2. Load Artifacts (Progressive Disclosure), 3. Build the Intent Inventory, 4. Assess the Codebase and Classify Findings, 5. Assign Severity, 6. Present the In-Session Findings Summary, 7. Append Convergence Tasks (or report converged), 8. Provide Next Actions (Handoff) (+7 more)

### Community 127 - "DNS Checks"
Cohesion: 0.14
Nodes (14): An explicit resolver is unreachable, Answers exist but the expectation fails, Check cluster DNS and aggregate the result, Choose a resolver, Cluster resolver, DNS Checks, Explicit resolver, Express expectations (+6 more)

### Community 128 - "Contract: `ClusterHealth.Status`"
Cohesion: 0.14
Nodes (12): Contract: `ClusterHealth.Status`, New guarantees, `status.children[]` — bounded and ordered, `status.matchedCount` — contract strengthened, `status.observedAt` — meaning inverted, `status.result` — explicitly unchanged, Unchanged, Alerting contract (+4 more)

### Community 129 - ".Diagnostics"
Cohesion: 0.33
Nodes (6): declaresRead(), Guard, unresolvedReads(), observedRead, PermissionDiagnostic, permissionObservations

### Community 130 - "testing.T"
Cohesion: 0.03
Nodes (86): TestAddToScheme(), TestDeepCopyIntoExercise(), TestDeepCopyRoundTrip(), TestGroupVersion(), TestSchemeBuilderRegisterReturnsSelf(), TestHealthReportResultSeverity_EmptyAndUnrecognizedReturnZero(), TestHealthReportResultSeverity_OrderingAcrossEnumValues(), TestHealthReportResultSeverity_PassIsLowestNonZero() (+78 more)

### Community 131 - "quorum-ratio-rollups/internal/controller/nodecertificatecheck_helpers.go"
Cohesion: 0.13
Nodes (11): TestWorstResult(), WorstResult(), aggregateNodeReports(), controlPlaneTolerations(), healthReportForNodeCert(), joinPaths(), nodeOutcomeToResult(), pruneNodeCertHealthReports() (+3 more)

### Community 132 - "Tasks: Pre-1.0 CRD Validation Hardening"
Cohesion: 0.12
Nodes (16): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Parallel Example: User Story 1, Parallel Opportunities, Phase 1: Setup (+8 more)

### Community 133 - "k8s.io/api/apps/v1.Deployment"
Cohesion: 0.15
Nodes (20): permission, permissionScope, refusalList, deploymentAvailable(), deploymentAvailable(), deploymentAvailable(), describeManager(), foreignInstall() (+12 more)

### Community 134 - "renderRuntimeChart"
Cohesion: 0.12
Nodes (36): chartAppVersion(), managerImages(), TestHelmDigestsReplaceTags(), TestHelmImagesDefaultToTheAppVersionTag(), TestHelmRejectsMalformedDigests(), helmAnnotations(), helmObjectNamed(), TestHelmDynatracePresetExplicitInsecureSkipVerify() (+28 more)

### Community 135 - "fathomctl Reference"
Cohesion: 0.07
Nodes (27): Bulk confirmation and --dry-run, CRD handling, describe, Exit codes, fathomctl Reference, Global flags, Images and versions, install (+19 more)

### Community 136 - "versions"
Cohesion: 0.06
Nodes (35): jsonPath, name, type, controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations (+27 more)

### Community 137 - "healthcheck_controller.go"
Cohesion: 0.09
Nodes (39): CheckTargetRef, HealthCheck, HealthCheckSpec, HealthCheckStatus, HealthCheckReconciler, healthCheckTargetHandler, healthCheckTargetIdentity, healthCheckTargetReader (+31 more)

### Community 138 - "runReports"
Cohesion: 0.10
Nodes (29): describer, objectList, outputFormat, reportsOptions, table, timestamp(), renderLsTable(), age() (+21 more)

### Community 139 - "quorum-ratio-rollups/internal/nodecert/paths.go"
Cohesion: 0.19
Nodes (14): resolveCertPaths(), TestResolveCertPathsFiltersDisallowed(), AllowedPathPrefixes(), DefaultCertPaths(), FilterAllowedPaths(), isCertFile(), isKubeconfigFile(), MinimalMountDirs() (+6 more)

### Community 140 - "Tasks: DNSCheck Resource Contract"
Cohesion: 0.13
Nodes (15): Dependencies, Format: `[ID] [P?] [Story] Description`, Implementation strategy, Parallel opportunities, Path Conventions, Phase 1: Setup, Phase 2: Foundational (Blocking Prerequisites), Phase 4: User Story 2 — Every declared expectation is evaluated (P1) (+7 more)

### Community 141 - "Implementation Plan: DNSCheck Completion"
Cohesion: 0.17
Nodes (12): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: DNSCheck Completion, Implementation Strategy, Phase 0: Research Outcome, Phase 1: Design Outcome, Project Structure (+4 more)

### Community 142 - "Research: fathomctl CLI"
Cohesion: 0.14
Nodes (14): R10: Operator version discovery, R11: Distribution, R12: RBAC for CLI users, R13: End-to-end coverage, R1: Binary and package layout, R2: Cluster client, R3: Kind addressing and aliases, R4: Verdict normalisation per kind (+6 more)

### Community 143 - "annotations"
Cohesion: 0.14
Nodes (14): additionalProperties, type, type, type, annotations, create, name, rbac (+6 more)

### Community 144 - "enabled"
Cohesion: 0.18
Nodes (11): properties, type, type, type, config, data, enabled, runtimeLoading (+3 more)

### Community 145 - "NewBudget"
Cohesion: 0.17
Nodes (43): NewBudget(), TestControlAndDelegatedRequestsShareBudget(), TestControlAndDelegatedRequestsShareResponseBytes(), diagnosticDefinition(), TestPermissionDeclarationHelpersAndUnknownMappings(), TestPermissionDeclarationsAreAdvisory(), TestPermissionDiscoverySuccessDoesNotClaimTargetAccess(), TestPermissionFailuresSurviveLaterSuccess() (+35 more)

### Community 146 - "parseVersion"
Cohesion: 0.31
Nodes (8): version, contractVersionAtLeast(), ensureCompatible(), parseVersion(), TestContractVersionParses(), TestEnsureCompatible(), TestEnsureCompatible_PreStableHost(), TestParseVersion()

### Community 147 - "addoncheck_controller.go"
Cohesion: 0.08
Nodes (41): TestWorstResult(), WorstResult(), addonAdapterLookup, familyRatioRollup, addonCheckPolicy(), aggregateHealthReportResult(), aggregateWithRatioRollups(), builtinAdapterFor() (+33 more)

### Community 148 - "NewMetricsServerEngine"
Cohesion: 0.32
Nodes (8): NewMetricsServerEngine(), apiService(), msHealthyObjects(), TestMetricsServer_AdapterMetadata(), TestMetricsServer_HealthyPassesAllFamilies(), TestMetricsServer_MissingAPIServiceFails(), TestMetricsServer_MissingDeploymentFails(), TestMetricsServer_UnavailableAPIServiceFails()

### Community 149 - "runtime_leadership_test.go"
Cohesion: 0.17
Nodes (30): exitRecorder, fakeClock, NewRuntimeLeadership(), beginSyncedSession(), epochOf(), heldLease(), newFakeClock(), newTestLeadership() (+22 more)

### Community 150 - ".agents/skills/speckit-plan/SKILL.md"
Cohesion: 0.18
Nodes (10): Completion Report, Done When, Key rules, Mandatory Post-Execution Hooks, Outline, Phase 0: Outline & Research, Phase 1: Design & Contracts, Phases (+2 more)

### Community 151 - "upgradePathDecl"
Cohesion: 0.17
Nodes (13): pathReleaseEntry, pathResult, sanctionedChange, upgradePathDecl, parseUpgradePath(), prefixV(), mustParsePath(), TestEmbeddedUpgradePathParses() (+5 more)

### Community 152 - "Entity: `DNSCheck`"
Cohesion: 0.14
Nodes (14): Changes to existing types, `cmd/probe` — dns mode flags, `DNSCheckSpec`, `DNSCheckStatus`, `DNSResolver`, `DNSTarget`, `DNSTargetResult`, Entity: `DNSCheck` (+6 more)

### Community 153 - "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1.CustomResourceDefinition"
Cohesion: 0.11
Nodes (32): crdFinding, Established(), crd(), crdWithServed(), TestEstablished(), TestPreferredServedVersion(), TestPreferredServedVersion_IgnoresUnservedEntries(), addLegacyProperty() (+24 more)

### Community 154 - ".agents/skills/speckit-specify/SKILL.md"
Cohesion: 0.18
Nodes (10): Completion Report, Done When, For AI Generation, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, Quick Guidelines, Section Requirements (+2 more)

### Community 155 - "Repository Guidelines"
Cohesion: 0.14
Nodes (13): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Graphify, Project Structure & Module Organization (+5 more)

### Community 156 - "values.schema.json"
Cohesion: 0.40
Nodes (4): required, $schema, title, type

### Community 157 - "properties"
Cohesion: 0.10
Nodes (21): $comment, items, type, type, additionalProperties, type, type, properties (+13 more)

### Community 158 - ".agents/skills/speckit-tasks/SKILL.md"
Cohesion: 0.18
Nodes (10): Checklist Format (REQUIRED), Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Phase Structure, Pre-Execution Checks, Task Generation Rules (+2 more)

### Community 159 - "Feature Specification: [FEATURE NAME]"
Cohesion: 0.15
Nodes (12): Assumptions, Edge Cases, Feature Specification: [FEATURE NAME], Functional Requirements, Key Entities *(include if feature involves data)*, Measurable Outcomes, Requirements *(mandatory)*, Success Criteria *(mandatory)* (+4 more)

### Community 160 - "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured"
Cohesion: 0.12
Nodes (31): install, managedBy, namespaceEntry, namespaceInventory, uninstallOptions, gatewayObject(), deleteAndWait(), deleteOperatorCreated() (+23 more)

### Community 161 - "Tasks: fathomctl Lifecycle Verbs"
Cohesion: 0.07
Nodes (27): Chart digests and embedding, CRD comparison and application, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation for User Story 4, Implementation Strategy (+19 more)

### Community 162 - ".RBACRules"
Cohesion: 0.11
Nodes (23): RBACDeclarer, allowedWrites(), repoRoot(), TestCommittedAddonRolesAreReadOnly(), TestModelGrantsAreJustified(), TestUnjustifiedGrantsCatchesViolations(), allowedWrites(), repoRoot() (+15 more)

### Community 163 - "GitHub Copilot Instructions for Fathom"
Cohesion: 0.17
Nodes (11): Codebase Shape, Commit and Branch Guidance, Documentation Expectations, GitHub Copilot Instructions for Fathom, Go and Repository Conventions, Knowledge Graph (`graphify-out/`), Pull Request Instructions, Safety Rules (+3 more)

### Community 164 - "internal/nodecert/paths.go"
Cohesion: 0.18
Nodes (16): resolveCertPaths(), TestResolveCertPathsFiltersDisallowed(), AllowedPathPrefixes(), DefaultCertPaths(), FilterAllowedPaths(), isCertFile(), isKubeconfigFile(), MinimalMountDirs() (+8 more)

### Community 165 - "Feature Specification: Node-agent metrics security"
Cohesion: 0.18
Nodes (11): Assumptions, Edge Cases, Feature Specification: Node-agent metrics security, Functional Requirements, Key Entities, Measurable Outcomes, Requirements, Success Criteria (+3 more)

### Community 166 - "Tasks: Quorum/Ratio Semantics for Managed-Resource Rollups"
Cohesion: 0.17
Nodes (11): Dependencies, Format: `[ID] [P?] [Story] Description`, Implementation strategy, Parallel execution examples, Phase 1: Setup, Phase 2: Foundational (blocking prerequisites), Phase 3: User Story 1 — Isolated failures stop redding the fleet verdict (P1) 🎯 MVP, Phase 4: User Story 2 — Graduated escalation between Warn and Fail (P2) (+3 more)

### Community 167 - "Feature Specification: Adversarial Codebase Review for the v0.5.0 Release Gate"
Cohesion: 0.17
Nodes (12): Assumptions, Edge Cases, Feature Specification: Adversarial Codebase Review for the v0.5.0 Release Gate, Functional Requirements, Key Entities, Measurable Outcomes, Requirements *(mandatory)*, Success Criteria *(mandatory)* (+4 more)

### Community 168 - ".claude/skills/speckit-plan/SKILL.md"
Cohesion: 0.18
Nodes (10): Completion Report, Done When, Key rules, Mandatory Post-Execution Hooks, Outline, Phase 0: Outline & Research, Phase 1: Design & Contracts, Phases (+2 more)

### Community 169 - ".claude/skills/speckit-specify/SKILL.md"
Cohesion: 0.18
Nodes (10): Completion Report, Done When, For AI Generation, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, Quick Guidelines, Section Requirements (+2 more)

### Community 170 - ".claude/skills/speckit-tasks/SKILL.md"
Cohesion: 0.18
Nodes (10): Checklist Format (REQUIRED), Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Phase Structure, Pre-Execution Checks, Task Generation Rules (+2 more)

### Community 171 - "Registry"
Cohesion: 0.06
Nodes (56): buildRuntimeSnapshot(), DispatchBarrier, Registry, Resolution, RuntimeEntry, RuntimeProvenance, RuntimeRevision, New() (+48 more)

### Community 172 - "SetRunningInClusterForTest"
Cohesion: 0.16
Nodes (11): defaultRunningInCluster(), inClusterFromConfigErr(), TestInClusterFromConfigErr(), RunningInCluster(), SetRunningInClusterForTest(), TestRunningInCluster_TestOverride(), TestDefaultControllers_InClusterRequiresNamespace(), addonSA() (+3 more)

### Community 173 - "Tasks: Node-agent metrics security"
Cohesion: 0.06
Nodes (26): 6. Serve node metrics through the authenticated operator endpoint, Consequences, Considered Options, Context and Problem Statement, Decision Outcome, Links, Specification Quality Checklist: Node-agent metrics security, Access (+18 more)

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

### Community 184 - "newFakeClient"
Cohesion: 0.15
Nodes (30): dnsRequest(), newFakeClient(), simulateKubelet(), TestLauncherRun_ConcurrentRunsAreIndependent(), TestLauncherRun_DeletesPodAfterRun(), TestLauncherRun_EmptyTerminationMessageIsError(), TestLauncherRun_FailedPhasePropagatesProbeJSON(), TestLauncherRun_InvalidRequestReturnsError() (+22 more)

### Community 185 - "Validate"
Cohesion: 0.10
Nodes (30): GrantPlan, RenderOptions, ScopedGrant, TestRuntimePayloadWireMappings(), TestRuntimeResolvedPolicyIsRevalidated(), TestCanonicalDefinitionByteBoundary(), TestDefinitionResourceSegmentAndIdentifierBoundaries(), TestDefinitionStringBytesAndUTF8() (+22 more)

### Community 186 - "nodehealth/paths.go"
Cohesion: 0.19
Nodes (14): AllowedPathPrefixes(), AllowedSocketDirs(), AllowedSocketFiles(), Canonical(), MountDirs(), PathAllowed(), SocketPathAllowed(), TestAllowlistsMirrorCRDRules() (+6 more)

### Community 187 - "Fathom Architecture"
Cohesion: 0.19
Nodes (24): ADR-0001 In-process Adapter Contract, ADR-0003 Probe-pod Model, Rationale: representative network topology without a DaemonSet, Fathom Architecture, Probe-Pod Model (active in-cluster checks), Absence Semantics (Required/Optional, MarkAbsent), Declarative Adapter Engine (MustEngine), Authoring an Adapter Guide (+16 more)

### Community 188 - "NewRootCommand"
Cohesion: 0.13
Nodes (16): main(), runMain(), TestMain_BadFlagExitsNonZero(), TestMain_HelpExitsZero(), TestMain_RunsAsMainOnDemand(), NewRootCommand(), signalContext(), TestSignalContext_PropagatesParentCancellation() (+8 more)

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

### Community 193 - ".claude/skills/speckit-checklist/SKILL.md"
Cohesion: 0.25
Nodes (7): Anti-Examples: What NOT To Do, Checklist Purpose: "Unit Tests for English", Example Checklist Types & Sample Items, Execution Steps, Post-Execution Checks, Pre-Execution Checks, User Input

### Community 194 - "Phase 0 Research: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.20
Nodes (10): Phase 0 Research: Cadence-Aware Staleness Semantics for ClusterHealth, R1 — Effective cadence already exists as three parallel resolvers, R2 — `observeCheck` is a single seam; series deletion is a matching obligation, R3 — The stalest timestamp needs no timer, which is what makes D1 implementable, R4 — C1: the aggregate cannot be cadence-aware without `HealthCheck`, R5 — C2: the overdue multiplier does not belong in operator config, R6 — Staleness derivation and the never-observed case, R7 — Bounding `Children[]` is an incompatible narrowing (+2 more)

### Community 195 - "upgrade_path_gate_test.go"
Cohesion: 0.29
Nodes (15): latestReleaseTag(), parseSanctioned(), parseUpgradePathDoc(), readUpgradePath(), releasedHistoryViolations(), sanctionedFindings(), TestParseSanctionedReadsTheGateReport(), TestReleasedHistoryViolations() (+7 more)

### Community 196 - "Quickstart Validation: fathomctl CLI"
Cohesion: 0.17
Nodes (8): 1. Build and generated artifacts, 2. Unit coverage without a cluster, 3. Distribution script, 4. Real cluster (required: controllers and CRD types change), 5. Release verification (after the first tagged release), 6. Documentation gates, Prerequisites, Quickstart Validation: fathomctl CLI

### Community 197 - "assertHasOutcome"
Cohesion: 0.27
Nodes (20): runManaged(), TestCondition_ClusterScopedListsWithoutNamespace(), TestCondition_ConditionStatus(), TestCondition_InvalidAPIVersionErrors(), TestCondition_InvalidSelectorErrors(), TestCondition_ListErrorDescribesNamespaceScope(), TestCondition_ListNameFallsBackToKind(), TestCondition_NamedClusterScopedGet() (+12 more)

### Community 198 - ".agents/skills/speckit-checklist/SKILL.md"
Cohesion: 0.25
Nodes (7): Anti-Examples: What NOT To Do, Checklist Purpose: "Unit Tests for English", Example Checklist Types & Sample Items, Execution Steps, Post-Execution Checks, Pre-Execution Checks, User Input

### Community 199 - ".Update"
Cohesion: 0.17
Nodes (10): createAddonCheckWithStatusForObservability(), conflictOnceStatusClient, conflictOnceStatusWriter, nodeCertHealthReportCount(), setNodeAgentDaemonSetStatus(), setNodeAgentDaemonSetStatusFull(), writeNodeReport(), writeNodeReportAt() (+2 more)

### Community 200 - "fathomctl_lifecycle_test.go"
Cohesion: 0.33
Nodes (9): lifecycleCLI, agentDaemonSets(), applySampleChecks(), buildLifecycleCLI(), checksPresent(), kubectl(), lifecycleCRDNames(), managedObjects() (+1 more)

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

### Community 208 - ".claude/skills/speckit-clarify/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 209 - ".claude/skills/speckit-implement/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 210 - "Research: DNSCheck Completion"
Cohesion: 0.25
Nodes (8): R1: Projection architecture, R2: API-version identity, R3: Snapshot and failure semantics, R4: Source status normalization, R5: Watch wiring, R6: RBAC and generated artifacts, R7: Validation scope, Research: DNSCheck Completion

### Community 211 - ".agents/skills/speckit-clarify/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 212 - "MustEngine"
Cohesion: 0.09
Nodes (24): Engine, TestMustEngine_PanicsOnInvalid(), configMap(), runConfigMap(), TestConfigMapCheck(), TestConfigMapCheck_AbsentInheritsOptional(), TestConfigMapCheck_NoAPIVersionAssertionPassesAnyYAML(), crdAbsenceEngine() (+16 more)

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
Cohesion: 0.13
Nodes (15): Air-gapped clusters, Check again, right now, fathomctl, Install, Install, upgrade and remove Fathom, Mixing fathomctl and Helm, Not supported by these verbs, One version per fathomctl (+7 more)

### Community 217 - "Specification Quality Checklist: Cadence-Aware Staleness Semantics for ClusterHealth"
Cohesion: 0.29
Nodes (6): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Cadence-Aware Staleness Semantics for ClusterHealth, Validation Notes

### Community 218 - "addondefinition/validation.go"
Cohesion: 0.13
Nodes (39): EvalContext, Guard, firstError(), labelName(), requiredName(), resourceType(), selectorLabels(), validateAnnotation() (+31 more)

### Community 219 - "sigs.k8s.io/controller-runtime.Manager"
Cohesion: 0.10
Nodes (11): runtimeDispatchGate, runtimeGateFixture, runtimeWiring, setupperFunc, managerServiceAccountName(), refreshStartupClaims(), serviceAccountFromToken(), setupWithContext() (+3 more)

### Community 221 - "Specification Quality Checklist: Adversarial Codebase Review for the v0.5.0 Release Gate"
Cohesion: 0.29
Nodes (5): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Adversarial Codebase Review for the v0.5.0 Release Gate

### Community 222 - "Specification Quality Checklist: DNSCheck Resource Contract"
Cohesion: 0.29
Nodes (6): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: DNSCheck Resource Contract, Validation Notes

### Community 224 - "Contract: Probe `dns` Mode CLI"
Cohesion: 0.29
Nodes (7): Answer matching, Contract: Probe `dns` Mode CLI, Flags, Invariants, Outcome mapping, Per-record-kind behavior, Vantage point is not a probe flag

### Community 225 - ".claude/skills/speckit-constitution/SKILL.md"
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
Cohesion: 0.05
Nodes (51): deploymentAvailable(), firstNamespace(), conditionStatus(), policySelector(), resourceAbsent(), containsString(), AnnotationStalenessCheck, ConditionCheck (+43 more)

### Community 232 - "Feature Specification: fathomctl CLI"
Cohesion: 0.15
Nodes (13): Assumptions and Dependencies, Clarifications, Decisions and Tradeoffs, Feature Specification: fathomctl CLI, Functional Requirements, Key Entities, Measurable Outcomes, Out of Scope (+5 more)

### Community 233 - "Contract: HealthCheck Target Projection"
Cohesion: 0.29
Nodes (7): Aggregation boundary, Compatibility, Contract: HealthCheck Target Projection, Reference failures, Successful projection, Supported references, Watch contract

### Community 234 - "k8s.io/apimachinery/pkg/apis/meta/v1.Time"
Cohesion: 0.09
Nodes (27): AddonCheckList, AddonDefinitionList, AddonDefinitionBindingList, ClusterHealth, ClusterHealthChildSummary, ClusterHealthList, ClusterHealthSpec, ClusterHealthStatus (+19 more)

### Community 235 - "versions"
Cohesion: 0.06
Nodes (35): jsonPath, name, type, controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations (+27 more)

### Community 236 - "Research: fathomctl Lifecycle Verbs"
Cohesion: 0.13
Nodes (15): R10. Export format for `--purge-crds --export`, R11. Namespace deletion check, R12. Enforcing the CLI import boundary, R13. End-to-end testing, R14. Cost: binary size and dependency surface, R1. Helm as a library: `helm.sh/helm/v4` v4.3.0, R2. Embedding the chart, R3. Image digests (+7 more)

### Community 237 - "Research: Node-agent metrics security"
Cohesion: 0.29
Nodes (6): Boundary and prior art, Decision: reduce certificate inventory, Decision: serve node metrics through the operator, Decision: use existing metric projection lifecycle, Research: Node-agent metrics security, Verification

### Community 238 - "versions"
Cohesion: 0.06
Nodes (35): jsonPath, name, type, controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations (+27 more)

### Community 239 - ".claude/skills/speckit-taskstoissues/SKILL.md"
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

### Community 243 - "sigs.k8s.io/controller-runtime/pkg/client.Reader"
Cohesion: 0.09
Nodes (38): activeAgentReportNames(), clearNodeAgentAccess(), clearScopedReportAccess(), clearSharedAgentBindingAccess(), deleteOwnedNodeAgentDaemonSet(), scopedReportAccessName(), newFilteredNodeAgentClient(), nodeAgentName() (+30 more)

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

### Community 254 - "Feature Specification: fathomctl Lifecycle Verbs"
Cohesion: 0.14
Nodes (14): Analysis remediation 2026-10-11, Assumptions, Clarifications, Decisions and Tradeoffs, Feature Specification: fathomctl Lifecycle Verbs, Functional Requirements, Key Entities, Measurable Outcomes (+6 more)

### Community 255 - "definitionE2EAPIFixture"
Cohesion: 0.11
Nodes (8): definitionE2EAPIFixture, definitionE2EHeldRead, TestDefinitionE2EFirstIPv4(), definitionE2EAPICertificate(), definitionE2EFirstIPv4(), definitionE2EHostGateway(), definitionE2ENewAPIFixture(), definitionE2EApply()

### Community 256 - "Implementation Plan: Runtime Addon Definitions"
Cohesion: 0.09
Nodes (22): Acceptance Criteria, Assumptions and Unknowns, Complexity Tracking, Constitution Check, Delivery shape (2026-09-22), Done When, Goal, Implementation Plan: Runtime Addon Definitions (+14 more)

### Community 258 - "sync.Mutex"
Cohesion: 0.10
Nodes (8): lockedBuffer, recordingCache, recordingClient, recordingIndexer, recordingManager, runtimeRunnables(), checkID, projection

### Community 259 - "versions"
Cohesion: 0.06
Nodes (35): jsonPath, name, type, controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations (+27 more)

### Community 260 - "RFC 1. AddonDefinition as a CRD — make adapters installable, not compiled in"
Cohesion: 0.10
Nodes (20): 1. Schema — Accepted, 2. Authority — Accepted, 3. Versions — Accepted, 4. Identity and precedence — Accepted, 5. Loading, lifecycle and evidence — Accepted, 6. Bounds and failure isolation — Accepted, Acceptance record, All evaluation paths (+12 more)

### Community 261 - "Contract: DNSCheck Metrics, Events, and RBAC"
Cohesion: 0.20
Nodes (10): 1. Check-level metrics (inherited, FR-032), 2. Per-target metric (new, FR-033), 3. Events (inherited, FR-112), 4. Conditions, 5. RBAC (FR-115 / inherited FR-037), Contract: DNSCheck Metrics, Events, and RBAC, Current state, Required (+2 more)

### Community 262 - "0008-fathomctl-lifecycle-via-helm-library.md"
Cohesion: 0.30
Nodes (3): Contract: Upgrade-Path Declaration, Gates, Path algorithm

### Community 263 - "Network policies"
Cohesion: 0.33
Nodes (5): Network policies, Node-agent DaemonSet (runtime-managed, always on), Operator (static, opt-in), Probe pods (deliberately no Fathom-shipped policy), The `metrics: enabled` label contract

### Community 264 - "quorum-ratio-rollups/api/v1alpha1/groupversion_info_test.go"
Cohesion: 0.40
Nodes (3): TestAddToScheme(), TestDeepCopyIntoExercise(), TestSchemeBuilderRegisterReturnsSelf()

### Community 265 - ".DeepCopyInto"
Cohesion: 0.10
Nodes (14): AddonCheckList, ClusterHealth, ClusterHealthSpec, HealthCheckList, HealthReport, NodeCertificateCheck, NodeCertificateCheckStatus, AddonCheckList (+6 more)

### Community 266 - "quorum-ratio-rollups/internal/adapter/declarative/istio_test.go"
Cohesion: 0.32
Nodes (12): NewIstioEngine(), istioAmbientObjects(), istioCRDObjects(), istiodControlPlane(), istioHealthyObjects(), istioInjectorConfig(), istioValidatorConfig(), TestIstio_AmbientDataPlanePasses() (+4 more)

### Community 267 - "projectedLabels"
Cohesion: 0.17
Nodes (12): items, type, maxLength, minLength, type, $comment, items, maxItems (+4 more)

### Community 268 - "Contract: DNSCheck Reconcile Loop"
Cohesion: 0.20
Nodes (9): Contract: DNSCheck Reconcile Loop, Explicitly out of contract, Failure handling, Invocation, Ordered sequence, Outcome classification, Ownership, Status write discipline (+1 more)

### Community 269 - "User Scenarios & Testing *(mandatory)*"
Cohesion: 0.33
Nodes (6): Edge Cases, User Scenarios & Testing *(mandatory)*, User Story 1 - DNS contributes to cluster health (Priority: P1), User Story 2 - Every advertised target kind works (Priority: P1), User Story 3 - Invalid references fail explicitly and safely (Priority: P2), User Story 4 - DNSCheck is straightforward to author and diagnose (Priority: P2)

### Community 270 - "check-coverage.sh"
Cohesion: 0.83
Nodes (3): check-coverage.sh script, skip_pkg(), threshold_for_pkg()

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

### Community 284 - "runtime_wiring_test.go"
Cohesion: 0.08
Nodes (27): Setupper, BuiltInAdapters(), DefaultControllers(), defaultControllers(), newUncachedProbeClient(), operatorVersion(), readyzCheck(), Run() (+19 more)

### Community 288 - "versions"
Cohesion: 0.06
Nodes (35): jsonPath, name, type, controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations (+27 more)

### Community 307 - "versions"
Cohesion: 0.06
Nodes (35): jsonPath, name, type, controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations (+27 more)

### Community 308 - "kedaHealthyObjects"
Cohesion: 0.13
Nodes (14): kedaDeployment(), NewKedaEngine(), conditionCR(), kedaHealthyObjects(), TestKeda_AbsentClusterAllSkipped(), TestKeda_HealthyWithReadyScaledObject(), TestKeda_PausedScaledObjectWarns(), TestKeda_UnreadyScaledObjectFails() (+6 more)

### Community 309 - "New"
Cohesion: 0.10
Nodes (21): AddonCheckFamilyPolicy, AddonCheckSpec, HealthCheckSpec, HealthReportCheck, HealthReportSpec, NodeCertificateCheckSpec, New(), newFake() (+13 more)

### Community 310 - "CompileRuntime"
Cohesion: 0.18
Nodes (19): TestRuntimeCollectionPayloadsFollowContinuation(), CompileRuntime(), runtimeTestContext(), runtimeVersionDefinition(), TestRuntimeCompilationPreservesOrderAndSnapshot(), TestRuntimeCompilerRejectsUnsupportedAndCancelledInput(), TestRuntimeExecutionRequiresSharedBudget(), TestRuntimePolicyRejectsInvalidOverridesBeforeReads() (+11 more)

### Community 312 - "Implementation Plan: DNSCheck Reconciler"
Cohesion: 0.25
Nodes (8): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: DNSCheck Reconciler, Project Structure, Source Code (repository root), Summary, Technical Context

### Community 313 - "addondefinition_payloads.go"
Cohesion: 0.13
Nodes (29): DefinitionAnnotationStaleness, DefinitionCondition, DefinitionConfigMap, DefinitionCRD, DefinitionCronJob, DefinitionField, DefinitionPodProjection, DefinitionReadRule (+21 more)

### Community 315 - "Quickstart Validation: DNSCheck Reconciler"
Cohesion: 0.29
Nodes (7): Level 1 — unit, no cluster, Level 2 — envtest, real API server, faked launcher, Level 3 — e2e on Kind, Level 4 — gates before the PR is ready, Manual review items no gate catches, Prerequisites, Quickstart Validation: DNSCheck Reconciler

### Community 316 - "User Scenarios & Testing *(mandatory)*"
Cohesion: 0.33
Nodes (6): Edge Cases, User Scenarios & Testing *(mandatory)*, User Story 1 - A declared check produces a verdict on its cadence (Priority: P1), User Story 2 - An operator sees which name failed, not just that the check failed (Priority: P2), User Story 3 - Result history is recorded without noise (Priority: P3), User Story 4 - Evaluation workloads never outlive their check (Priority: P3)

### Community 317 - ".checkCRD"
Cohesion: 0.27
Nodes (8): Established(), PreferredServedVersion(), crd(), crdWithServed(), TestEstablished(), TestPreferredServedVersion(), TestPreferredServedVersion_IgnoresUnservedEntries(), PreferredServedVersion()

### Community 318 - "quorum-ratio-rollups/internal/nodecert/scan_test.go"
Cohesion: 0.16
Nodes (26): minimalKubeconfig, classify(), classifyAll(), daysFromDuration(), errorResult(), parsePEMCertificates(), Scan(), scanCertFile() (+18 more)

### Community 319 - "dnscheck_test.go"
Cohesion: 0.60
Nodes (3): dnsCheckPod, dnsCheckPodList, listProbePods()

### Community 321 - "Specification Quality Checklist: DNSCheck Reconciler"
Cohesion: 0.40
Nodes (5): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: DNSCheck Reconciler

### Community 323 - "versions"
Cohesion: 0.06
Nodes (35): jsonPath, name, type, controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations (+27 more)

### Community 325 - "properties"
Cohesion: 0.22
Nodes (9): properties, type, properties, type, dynatrace, integrations, sumologic, properties (+1 more)

### Community 326 - "Implementation Strategy"
Cohesion: 0.50
Nodes (4): Implementation Strategy, Incremental Delivery, MVP First, Suggested PR Scope

### Community 330 - "CLI-side types (`internal/cli`, unexported)"
Cohesion: 0.22
Nodes (9): checkRef, CLI-side types (`internal/cli`, unexported), GlobalOptions, kindDescriptor, reportRow, runTarget and runOutcome, snapshot, trigger token (+1 more)

### Community 331 - "factory"
Cohesion: 0.15
Nodes (9): SAUsername(), TestClientForSetsImpersonationAndMemoizes(), TestSAUsername(), factory, ClientFactory, New(), SAUsername(), TestClientForSetsImpersonationAndMemoizes() (+1 more)

### Community 332 - "Contract: fathomctl command surface"
Cohesion: 0.25
Nodes (8): Contract: fathomctl command surface, `fathomctl describe <kind>/<name>`, `fathomctl ls [kind] [-l selector]`, `fathomctl reports <kind>/<name> [--limit N] [--since <duration>] [--report <name>]`, `fathomctl run (<kind>/<name> | --all | -l selector) [--wait] [--timeout <duration>] [--yes] [--dry-run]`, `fathomctl version [--client]`, Global flags, Help and completion

### Community 333 - "Data Model: fathomctl CLI"
Cohesion: 0.25
Nodes (7): API changes (`api/v1alpha1`), CLI wait loop, Data Model: fathomctl CLI, Executable checks (all three kinds), Operator state transitions, Release artifacts, Wire contract change (`internal/nodecert`)

### Community 334 - "internal/adapter/rbacgen/rbacgen.go"
Cohesion: 0.11
Nodes (33): TestFilesRejectsIncompleteRule(), TestFilesRejectsIncompleteRule(), TestModelGrantsAreJustified(), clusterRules(), Collect(), Files(), AddonRBAC, groupsCell() (+25 more)

### Community 335 - "Specification Quality Checklist: fathomctl CLI"
Cohesion: 0.29
Nodes (5): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: fathomctl CLI

### Community 336 - "Contract: on-demand run trigger (operator side)"
Cohesion: 0.29
Nodes (7): Annotation, Consumption, Contract: on-demand run trigger (operator side), Derived kinds, Kind-specific behaviour, Status field, Test obligations

### Community 337 - "User Scenarios & Testing *(mandatory)*"
Cohesion: 0.29
Nodes (7): Edge Cases, User Scenarios & Testing *(mandatory)*, User Story 1 - Ask Fathom to validate now and get the answer (Priority: P1), User Story 2 - See every verdict in the cluster at a glance (Priority: P1), User Story 3 - Understand why a check has the verdict it has (Priority: P2), User Story 4 - See how a check's verdict has changed over time (Priority: P2), User Story 5 - Install the CLI, trust it, and know what I am talking to (Priority: P3)

### Community 338 - "8. Drive the Fathom lifecycle from fathomctl through the Helm library"
Cohesion: 0.25
Nodes (8): 8. Drive the Fathom lifecycle from fathomctl through the Helm library, Consequences, Considered Options, Context and Problem Statement, Decision Drivers, Decision Outcome, Links, Pros and Cons of the Options

### Community 339 - "internal/adapter/declarative/field_test.go"
Cohesion: 0.38
Nodes (10): gauge(), gaugeCheck(), runFields(), TestField_InvalidSelectorErrors(), TestField_ListedObjectsScored(), TestField_NoMatchingObjectsSkipped(), TestField_NoMatchUsesAddonAbsencePosture(), TestField_ScoreObject() (+2 more)

### Community 340 - "TestEnvoyGateway_HealthyAndNoGatewaysSkipped"
Cohesion: 0.29
Nodes (6): NewEnvoyGatewayEngine(), egHealthyObjects(), TestEnvoyGateway_AdapterMetadata(), TestEnvoyGateway_GatewayConditionScoring(), TestEnvoyGateway_HealthyAndNoGatewaysSkipped(), TestEnvoyGateway_MissingDeploymentFails()

### Community 341 - "Adversarial Review: fathomctl Lifecycle Verbs (spec 013)"
Cohesion: 0.25
Nodes (7): Adversarial Review: fathomctl Lifecycle Verbs (spec 013), Base and scope, Findings and dispositions, Limitations, Re-review history, Reviewer independence, Verification

### Community 342 - "internal/adapter/declarative/annotation_test.go"
Cohesion: 0.40
Nodes (8): daemonSetWithAnnotations(), lockCheck(), lockJSON(), nodeRebootCheck(), nodeWithAnnotations(), runAnnotation(), TestAnnotationStaleness_NamedLock(), TestAnnotationStaleness_NodeList()

### Community 343 - "NewExternalDNSEngine"
Cohesion: 0.31
Nodes (7): NewExternalDNSEngine(), extdnsHealthyObjects(), TestExternalDNS_AdapterMetadata(), TestExternalDNS_DeploymentNameThresholdOverride(), TestExternalDNS_HealthyPassesAllFamilies(), TestExternalDNS_MissingCRDSkippedOptional(), TestExternalDNS_MissingDeploymentFails()

### Community 344 - "BuiltinNames"
Cohesion: 0.24
Nodes (8): runtimeSampleChecks(), TestRuntimeInventoryMatchesOperator(), TestRuntimeSamplesMatchGenerated(), WriteRuntimeInventory(), WriteRuntimeSamples(), main(), run(), BuiltinNames()

### Community 345 - "Implementation Plan: fathomctl Lifecycle Verbs"
Cohesion: 0.25
Nodes (8): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: fathomctl Lifecycle Verbs, Project Structure, Source Code (repository root), Summary, Technical Context

### Community 346 - "Quickstart: Validating fathomctl Lifecycle Verbs"
Cohesion: 0.25
Nodes (8): 1. Install, then confirm idempotency, 2. Upgrade across a CRD schema change (CRDs first), 3. Refusals (unit-covered; spot-check with `--dry-run`), 4. Uninstall leaves no workloads, keeps data, 5. Purge with export, then restore, 6. Namespace deletion, Automated, Quickstart: Validating fathomctl Lifecycle Verbs

### Community 348 - "versions"
Cohesion: 0.06
Nodes (35): jsonPath, name, type, controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations (+27 more)

### Community 351 - "with"
Cohesion: 0.06
Nodes (34): concurrency, cancel-in-progress, group, GH_REPO, GH_TOKEN, jobs, release-please, name (+26 more)

### Community 352 - "task"
Cohesion: 0.06
Nodes (34): cmds, desc, cmds, desc, cmds, desc, cmds, desc (+26 more)

### Community 353 - "Feature Specification: Complete AddonDefinition Design RFC"
Cohesion: 0.13
Nodes (15): Assumptions, Clarifications, Context and Scope, Edge Cases, Feature Specification: Complete AddonDefinition Design RFC, Functional Requirements, Key Entities *(include if feature involves data)*, Measurable Outcomes (+7 more)

### Community 354 - ".agents/skills/speckit-adversarial-review/SKILL.md"
Cohesion: 0.29
Nodes (6): Establish the Review Contract, Findings, Independent Review, Purpose and Boundaries, Result and Re-review, User Input

### Community 355 - ".claude/skills/speckit-adversarial-review/SKILL.md"
Cohesion: 0.29
Nodes (6): Establish the Review Contract, Findings, Independent Review, Purpose and Boundaries, Result and Re-review, User Input

### Community 356 - "runtime.md"
Cohesion: 0.18
Nodes (9): Accepted implementation clarifications — 2026-09-20, Offline renderer clarification — option A, Test contract clarification (2026-09-22), Activation and identity, Drain acknowledgement, Independent CLI verification, Leader election and drain verification, Required tests (+1 more)

### Community 357 - "Feature Specification: Runtime Addon Definitions"
Cohesion: 0.12
Nodes (16): Assumptions, Clarifications, Edge Cases, Feature Specification: Runtime Addon Definitions, Functional Requirements, Key Entities, Measurable Outcomes, Requirements (+8 more)

### Community 358 - "subjects"
Cohesion: 0.29
Nodes (7): reader, subjects, properties, type, $comment, items, type

### Community 360 - "Proposed PR shape"
Cohesion: 0.29
Nodes (7): Delivery preparation, Milestone 1 description, Milestone 2 description, Milestone 3 description, Milestone 4 description, Proposed PR shape, Validation and release dependencies

### Community 361 - "crd_compat_gate_test.go"
Cohesion: 0.47
Nodes (12): fixtureCRD(), runCRDCompat(), TestCRDCompatAddedOptionalFieldPasses(), TestCRDCompatAgainstBaseline(), TestCRDCompatAllowlistedChangePassesVisibly(), TestCRDCompatMalformedAllowlistFails(), TestCRDCompatNewCRDSkipped(), TestCRDCompatNoChangePasses() (+4 more)

### Community 362 - "k8s.io/apimachinery/pkg/types.NamespacedName"
Cohesion: 0.11
Nodes (13): runtimeWorkerPool, fakeRuntimeQueue, Scheduler, Scheduler, runtimeDisposition(), healthReportCount(), nodeCertHealthReportCount(), nodeHealthHealthReportCount() (+5 more)

### Community 363 - "0001-addondefinition-crd.md"
Cohesion: 0.18
Nodes (9): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Complete AddonDefinition Design RFC, AddonDefinition RFC quickstart, Eventual review and handoff checks, Local checks (+1 more)

### Community 364 - "Typed definition wire contract"
Cohesion: 0.15
Nodes (13): AnnotationStaleness → AnnotationStalenessCheck, Common target and conversion rules, Condition → ConditionCheck, ConfigMap → ConfigMapCheck, CRD → CRDCheck, CronJob → CronJobCheck, Declared reads and validation split, Envelope, family and check (+5 more)

### Community 365 - "Data Model: fathomctl Lifecycle Verbs"
Cohesion: 0.29
Nodes (7): CRD set, Data Model: fathomctl Lifecycle Verbs, Install, Operator-created objects, Purge export, Upgrade path declaration (`upgrade-path.yaml`), Upgrade plan

### Community 366 - "properties"
Cohesion: 0.06
Nodes (32): controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations, name, kind, listKind (+24 more)

### Community 367 - "Design Entities"
Cohesion: 0.17
Nodes (12): Addon definition, Built-in definition, Conceptual Data Model: AddonDefinition RFC, Definition revision, Design Entities, Evaluation evidence, Evaluation identity, Governance Entities (+4 more)

### Community 368 - "RFC execution evidence"
Cohesion: 0.17
Nodes (12): Acceptance — 2026-09-20, Additional Copilot contract clarifications, Copilot review clarifications, Draft delivery and review, Final publication-fence correction, Governance, Merge and handoff — 2026-09-20, Review publication (+4 more)

### Community 369 - "Implementation Plan: Complete AddonDefinition Design RFC"
Cohesion: 0.17
Nodes (12): Complexity Tracking, Constitution Check, Documentation (this feature), Execution Sequence After Planning, Implementation Plan: Complete AddonDefinition Design RFC, Phase 0: Research, Phase 1: Design Artifacts, Project Structure (+4 more)

### Community 370 - "quorum-ratio-rollups/internal/probe/sweeper_test.go"
Cohesion: 0.32
Nodes (11): probeLabels(), probeShape(), sweepPod(), terminatedAt(), TestSweeper_LabelledPodNotMatchingProbeShapeIsSpared(), TestSweeper_LongRunningProbeIsNotReapedOnCreationAge(), TestSweeper_ReapsPodTerminatedLongerThanMinAge(), TestSweeper_StartSweepsImmediatelyAndStopsOnCancel() (+3 more)

### Community 371 - "internal/controller/tracing_test.go"
Cohesion: 0.29
Nodes (7): TestListSelectedHealthChecks_ErrorNamesScope(), attrValue(), installInMemoryTracer(), newControllerScheme(), spanByName(), TestClusterHealthReconcile_EmitsSpan(), TestHealthCheckReconcile_EmitsSpan()

### Community 372 - "Runtime qualification evidence"
Cohesion: 0.33
Nodes (6): Current qualification state, Focused security review, Functional requirements, Lifecycle contract rows, Numeric contract rows, Runtime qualification evidence

### Community 373 - "AddonDefinition"
Cohesion: 0.12
Nodes (22): AddonDefinition, DefinitionBindingScope, runtimeAdapter, compileRuntime(), CompileRuntimeScoped(), firstRuntimeNamespace(), lowerCheck(), lowerRuntime() (+14 more)

### Community 374 - "sigs.k8s.io/controller-runtime/pkg/client.Client"
Cohesion: 0.05
Nodes (26): selectiveStartupReader, failingHealthCheckListClient, failingHealthCheckTargetClient, fakeClientFactory, healthReportBlindClient, transientAddonCheckGetClient, collectionPageClient, ExecutionBudget (+18 more)

### Community 375 - "properties"
Cohesion: 0.06
Nodes (32): controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations, name, kind, listKind (+24 more)

### Community 376 - "Research: Complete AddonDefinition Design RFC"
Cohesion: 0.20
Nodes (10): Execution refresh — 2026-09-20, R1. Finish the existing RFC through an evidenced decision, R2. Treat the permission boundary as a design obligation, R3. Separate requested permissions, effective access and diagnostics, R4. Make lifecycle and evidence coherence explicit, R5. Separate local validation from global identity arbitration, R6. Bound evaluated work, not just definition size, R7. Keep version tracks and follow-on scope separate (+2 more)

### Community 377 - "Tasks: Complete AddonDefinition Design RFC"
Cohesion: 0.20
Nodes (10): Dependencies and execution order, Implementation strategy, Parallel opportunities, Phase 1: Setup, Phase 2: Foundational evidence, Phase 3: US1 — Review a Complete Safety Boundary (P1), Phase 4: US2 — Predict Definition Lifecycle Outcomes (P1), Phase 5: US3 — Implement from an Unambiguous Contract (P2) (+2 more)

### Community 378 - "Final checkpoint checks and development-cluster validation"
Cohesion: 0.06
Nodes (33): Actual prior-binary operations qualification, Authoring CLI and generated inventory — T021/T022, Combined ContractVersion 1.1 qualification result — 2026-09-22, ContractVersion 1.1 integration status — 2026-09-22 (historical in-progress snapshot), Delivery decision and separate #256 compatibility proof, Dependency-watch fix and renewed verification, Feature PR publication — 2026-09-22, Final checkpoint checks and development-cluster validation (+25 more)

### Community 379 - "Research and decisions"
Cohesion: 0.20
Nodes (10): Clarification research — 2026-09-20, CLI layering, Compatibility and remaining measurements, Evidence and history, Existing verification and generators, Immutable ownership and publication, Pinned lifecycle verification — implementation T006, Research and decisions (+2 more)

### Community 380 - "Tasks: Runtime Addon Definitions"
Cohesion: 0.18
Nodes (11): Dependencies and parallel opportunities, Implementation strategy, Phase 1: Setup, Phase 2: Foundation, Phase 3: US1 — Author installable coverage (P1), Phase 4: US2 — Delegate bounded execution (P1), Phase 5: US3 — Preserve evidence across change (P1), Phase 6: US4 — Install and operate safely (P2) (+3 more)

### Community 381 - "properties"
Cohesion: 0.06
Nodes (32): controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations, name, kind, listKind (+24 more)

### Community 382 - "properties"
Cohesion: 0.06
Nodes (32): controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations, name, kind, listKind (+24 more)

### Community 383 - "fathomctl_test.go"
Cohesion: 0.33
Nodes (3): fathomctlRunOutcome, fathomctl(), kubectlJSONPath()

### Community 384 - "Specification Quality Checklist: fathomctl Lifecycle Verbs"
Cohesion: 0.33
Nodes (5): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: fathomctl Lifecycle Verbs

### Community 385 - "7. Load typed addon definitions under explicit administrator authority"
Cohesion: 0.22
Nodes (8): 7. Load typed addon definitions under explicit administrator authority, Consequences, Considered Options, Context and Problem Statement, Decision Drivers, Decision Outcome, Links, Pros and Cons of the Options

### Community 386 - "properties"
Cohesion: 0.06
Nodes (31): controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations, name, categories, kind (+23 more)

### Community 387 - "RFC Review Contract: AddonDefinition"
Cohesion: 0.25
Nodes (8): Accepted decision and handoff evidence, Completion and Handoff Evidence, Draft walkthrough — 2026-09-20, Requirement Traceability, Review and Approval Lifecycle, RFC Review Contract: AddonDefinition, Scenario Review Matrix, Six Principal Decision Topics

### Community 388 - "image"
Cohesion: 0.29
Nodes (7): required, type, properties, required, type, image, nodeAgent

### Community 389 - "properties"
Cohesion: 0.06
Nodes (31): controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations, name, categories, kind (+23 more)

### Community 390 - "Run"
Cohesion: 0.11
Nodes (23): dsRollout, nodeCertStatusView, nodeHealthStatusView, rbacRoleBinding, rbacSubject, applyDNSCheck(), dnsCheckField(), ensureNamespaceActive() (+15 more)

### Community 391 - "Runtime acceptance contract"
Cohesion: 0.25
Nodes (8): Authoring interface, Clarification additions — 2026-09-20, Coverage ledger, Enforcement obligations, Lifecycle matrix, Numeric inventory, Runtime acceptance contract, Test-layer clarification — 2026-09-22

### Community 392 - "Contract: fathomctl Lifecycle Verbs"
Cohesion: 0.33
Nodes (6): Contract: fathomctl Lifecycle Verbs, `fathomctl install`, `fathomctl status`, `fathomctl uninstall`, `fathomctl upgrade`, Shared flags (install, upgrade)

### Community 393 - "User Scenarios & Testing *(mandatory)*"
Cohesion: 0.33
Nodes (6): Edge Cases, User Scenarios & Testing *(mandatory)*, User Story 1 - Install Fathom with one command and know it worked (Priority: P1), User Story 2 - Upgrade safely, CRDs first (Priority: P1), User Story 3 - Uninstall and leave nothing running (Priority: P2), User Story 4 - Ask what is installed (Priority: P2)

### Community 394 - "Specification Quality Checklist: Runtime Addon Definitions"
Cohesion: 0.33
Nodes (5): Content Quality, Feature Readiness, Notes, Requirement Completeness, Specification Quality Checklist: Runtime Addon Definitions

### Community 395 - "Data model"
Cohesion: 0.33
Nodes (6): AddonDefinition, AddonDefinitionBinding, Data model, Evidence and attempts, Snapshot and publication context, Transitions and bounds

### Community 396 - "with"
Cohesion: 0.07
Nodes (31): needs, permissions, runs-on, steps, jobs, install, lint, permissions (+23 more)

### Community 397 - "Operator RBAC"
Cohesion: 0.33
Nodes (6): Auxiliary roles shipped alongside the operator, Namespace-scoping analysis, Operator ClusterRole rules, Operator RBAC, Runtime-created RBAC, Why these grants are cluster-scoped

### Community 398 - "Adversarial review — implementation checkpoint"
Cohesion: 0.50
Nodes (4): Adversarial review — implementation checkpoint, Reproduced and fixed findings, Resolved design decision, Verified boundaries and limitations

### Community 399 - "properties"
Cohesion: 0.06
Nodes (30): controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations, name, kind, listKind (+22 more)

### Community 400 - ".Name"
Cohesion: 0.14
Nodes (20): TestAdapterMetadata(), fakePolicyAdapter, TestEngine_Metadata(), TestEnvoyGateway_AdapterMetadata(), NewExternalDNSEngine(), extdnsHealthyObjects(), TestExternalDNS_AdapterMetadata(), TestExternalDNS_DeploymentNameThresholdOverride() (+12 more)

### Community 401 - "properties"
Cohesion: 0.06
Nodes (30): controller-gen.kubebuilder.io/version, apiVersion, kind, metadata, annotations, name, kind, listKind (+22 more)

### Community 402 - ".Run"
Cohesion: 0.28
Nodes (26): adapterWithLauncher(), assertHasDetail(), assertHasOutcome(), assertNoTarget(), daemonSetWithStatus(), healthyObjects(), newFakeClient(), passingDNSLauncher() (+18 more)

### Community 405 - "addondefinition_test.go"
Cohesion: 0.17
Nodes (13): definitionE2EAttributedTo(), definitionE2EBinding(), definitionE2EBindingFor(), definitionE2ECheckStatus(), definitionE2EDefinition(), definitionE2EExpectAbsent(), definitionE2EFrozenEvidence(), definitionE2EGetJSON() (+5 more)

### Community 406 - "operator_rbac_doc_test.go"
Cohesion: 0.16
Nodes (16): docRow, equalStrings(), loadJustificationRows(), loadOperatorClusterRole(), normalizeSet(), ruleKey(), splitList(), splitTableRow() (+8 more)

### Community 407 - "vars"
Cohesion: 0.07
Nodes (31): vars, sh, vars, BUNDLE_IMG, CATALOG_IMG, CONTROLLER_GEN, CRD_REF_DOCS, E2E_ADDONS (+23 more)

### Community 408 - "spec"
Cohesion: 0.08
Nodes (25): apiVersion, enabled, thresholds, kind, app.kubernetes.io/managed-by, app.kubernetes.io/name, metadata, labels (+17 more)

### Community 409 - "internal/nodecert/scan_test.go"
Cohesion: 0.12
Nodes (14): makeCertPEM(), TestDaysFromDuration(), TestEncodeDecodeReportRoundTrip(), TestNodeReportConfigMapNameDeterministicAndSafe(), TestScanBundleEmitsPerCert(), TestScanDefaultsWhenNoPaths(), TestScanDirectoryRecursiveAndIgnoresNonCerts(), TestScanKubeconfig() (+6 more)

### Community 411 - "tasks"
Cohesion: 0.08
Nodes (23): cmds, desc, cmds, desc, cmds, desc, cmds, desc (+15 more)

### Community 412 - "runUpgrade"
Cohesion: 0.21
Nodes (20): crdPlan, operatorChange, planRequest, refusal, upgradeOptions, upgradePlan, notHelmRefusal(), pendingRefusal() (+12 more)

### Community 413 - "internal/app/run_happy_test.go"
Cohesion: 0.20
Nodes (9): adapterName(), disableHTTP2(), firstEnvtestBinaryDir(), TestAdapterName_NilReturnsPlaceholder(), TestAdapterName_NonNilReturnsName(), TestDisableHTTP2(), TestMain(), TestRun_HappyPath_DefaultControllers() (+1 more)

### Community 415 - "runArgoCD"
Cohesion: 0.24
Nodes (9): argocdDeployment(), NewArgoCDEngine(), argoApp(), argocdHealthyObjects(), runArgoCD(), TestArgoCD_AbsentClusterFails(), TestArgoCD_ApplicationStateRollup(), TestArgoCD_HealthyWithSyncedApplication() (+1 more)

### Community 416 - "Dependencies & Execution Order"
Cohesion: 0.40
Nodes (5): Dependencies & Execution Order, Parallel Opportunities, Phase Dependencies, User Story Dependencies, Within Each User Story

### Community 418 - "ControlTargets"
Cohesion: 0.17
Nodes (13): RuntimeControlReader, NewRuntimeControlReader(), newRuntimeControlReader(), TestRuntimeControlReaderRequiresExplicitBoundary(), NewControlGuard(), controlTargets(), serviceAccountControlTargets(), TestControlGuardAllowsOnlyTheResolvedServiceAccount() (+5 more)

### Community 420 - "normalizeShell"
Cohesion: 0.83
Nodes (3): normalizeShell(), stripShellComment(), TestCoverageGateSkipsNoPackages()

### Community 422 - "Node-agent DaemonSet (on-disk cert scanner)"
Cohesion: 0.50
Nodes (4): Fathom Prometheus Metrics Surface, Rationale: path allowlist prevents confused-deputy host reads, Node-agent DaemonSet (on-disk cert scanner), Node-report Authenticity ValidatingAdmissionPolicy

### Community 423 - "MustEngine"
Cohesion: 0.09
Nodes (40): TestMustEngine_PanicsOnInvalid(), crdAbsenceEngine(), TestCRD_AbsenceResolution(), cronJob(), runCronJob(), TestCronJobCheck(), TestCronJobCheck_PerpetualFailurePastWindowWarns(), TestCronJobCheck_SuccessMaxAgeThresholdOverride() (+32 more)

### Community 424 - "Adapter"
Cohesion: 0.21
Nodes (14): NewCache(), revision(), TestCacheCanceledCompilationIsNotCachedOrPinned(), TestCacheIdleLRUAndActiveBounds(), TestCachePinsSurviveIdleEvictionAndCompileOutsideLock(), TestCacheRejectsIncompleteRevisionKey(), TestCacheRevisionIsolationAndPanicCleanup(), successfulCompiler() (+6 more)

### Community 425 - "crd_kustomization_test.go"
Cohesion: 0.83
Nodes (3): crdKustomizationDrift(), TestCRDKustomizationDriftDetection(), TestCRDKustomizationListsEveryBase()

### Community 426 - "check-ghcr-public.sh"
Cohesion: 0.83
Nodes (3): anon_status(), check_package(), check-ghcr-public.sh script

### Community 429 - "quorum-ratio-rollups/internal/controller/policy_validation_test.go"
Cohesion: 0.48
Nodes (5): badSelector(), checkWithPolicy(), TestValidateAddonCheckPolicy(), TestValidateAddonCheckPolicy_DeterministicOrder(), TestValidateAddonCheckPolicy_ThresholdKeys()

### Community 430 - "port"
Cohesion: 0.29
Nodes (7): maximum, minimum, type, port, service, properties, type

### Community 433 - "with"
Cohesion: 0.10
Nodes (20): with, cosign-release, fetch-depth, files, format, generate_release_notes, go-version-file, image (+12 more)

### Community 434 - "values"
Cohesion: 0.10
Nodes (20): enabled, create, replicas, enabled, mode, enabled, name, values (+12 more)

### Community 435 - "options.go"
Cohesion: 0.15
Nodes (8): DNSCheckOptions, flagBinding, MetricsOptions, Options, RuntimeLoadingOptions, TracingOptions, WebhookOptions, bindings()

### Community 436 - "Pod"
Cohesion: 0.19
Nodes (17): antiAffinity(), copyStringMap(), Pod(), assertArgs(), TestParseResult(), TestPodBuildsDNSAssertionArgs(), TestPodBuildsHardenedDNSProbe(), TestPodBuildsHTTPGetArgs() (+9 more)

### Community 437 - "Load"
Cohesion: 0.17
Nodes (13): bufferedFiles(), Files(), helmIgnoreRules(), Load(), NormalizeVersion(), ReadFile(), TestLoadAcceptsGitDescribeVersions(), TestLoadAppliesHelmIgnore() (+5 more)

### Community 438 - "env"
Cohesion: 0.11
Nodes (18): BUNDLE_IMG, BUNDLE_REF, CATALOG_IMG, CATALOG_REF, CHART_REF, COSIGN_YES, FATHOM_NODE_AGENT_DIGEST, FATHOM_OPERATOR_DIGEST (+10 more)

### Community 439 - "Budget"
Cohesion: 0.19
Nodes (3): Budget, minTime(), Failure

### Community 440 - "internal/metrics/metrics.go"
Cohesion: 0.16
Nodes (15): TestNodeDetailMetricsWithdrawAtReconcileEntryWithoutAffectingSiblingCheck(), TestNodeDetailProjectionIncludesAcceptedPartialFleetAndExcludesDepartedNodes(), TestPausedNodeCertificateCheckWithdrawsDetailMetrics(), DeleteNodeCertificateSeries(), DeleteNodeHealthSeries(), init(), newCheckGauges(), ObserveNodeCertificateReport() (+7 more)

### Community 441 - "verify-public"
Cohesion: 0.14
Nodes (16): jobs, release, verify-public, name, permissions, attestations, contents, id-token (+8 more)

### Community 442 - "TestAnnotationStaleness_NamedLock"
Cohesion: 0.23
Nodes (13): daemonSetWithAnnotations(), lockCheck(), lockJSON(), nodeRebootCheck(), nodeWithAnnotations(), runAnnotation(), TestAnnotationStaleness_NamedLock(), TestAnnotationStaleness_NodeList() (+5 more)

### Community 443 - "NewScheduler"
Cohesion: 0.42
Nodes (14): TestRuntimePoolRecoversHandlerPanicAndPreservesPeer(), TestRuntimePoolStopsAndReleasesAllWorkers(), NewScheduler(), admit(), enqueue(), TestSchedulerBackoffAndMissingInputs(), TestSchedulerCoalescesChurnAndKeepsCheckSlot(), TestSchedulerConcurrentAdmissionAndWakeCoalescing() (+6 more)

### Community 444 - "Chart.yaml"
Cohesion: 0.14
Nodes (13): apiVersion, appVersion, description, home, keywords, kubeVersion, maintainers, name (+5 more)

### Community 445 - "jobs"
Cohesion: 0.16
Nodes (13): if, name, permissions, runs-on, jobs, dco, test, name (+5 more)

### Community 446 - "with"
Cohesion: 0.14
Nodes (14): quality, name, permissions, runs-on, steps, with, cache-dependency-path, fetch-depth (+6 more)

### Community 447 - "requests"
Cohesion: 0.14
Nodes (14): resources, resources, resources, resources, cpu, memory, requests, resources (+6 more)

### Community 448 - "validateAddonCheckPolicy"
Cohesion: 0.27
Nodes (10): fakeAdvertisingAdapter, legacyAdvertisingAdapter, validateAddonCheckPolicy(), badSelector(), checkWithPolicy(), TestSetAddonCheckAccepted(), TestValidateAddonCheckPolicy(), TestValidateAddonCheckPolicy_DeterministicOrder() (+2 more)

### Community 449 - "name"
Cohesion: 0.24
Nodes (13): steps, steps, version-lockstep, if, name, run, uses, steps (+5 more)

### Community 450 - "with"
Cohesion: 0.17
Nodes (12): with, cluster_name, config, fetch-depth, go-version-file, if-no-files-found, kubectl_version, name (+4 more)

### Community 451 - "snapshot_test.go"
Cohesion: 0.24
Nodes (11): descriptorFor(), dur(), ptrTime(), samePtrTime(), sameTime(), TestNodeHealthCheckPassTimeoutBudgetsEvaluationAndPublication(), TestNodeHealthCheckTimeoutIsCappedAtTheAgentCadence(), TestNodeHealthCheckWaitEstimateSaturates() (+3 more)

### Community 452 - "releases"
Cohesion: 0.17
Nodes (12): addon, tier, releases, chart, createNamespace, labels, name, namespace (+4 more)

### Community 453 - "addoncheck_types.go"
Cohesion: 0.24
Nodes (8): AddonCheckEvidenceAuthority, AddonCheckEvidenceRevision, AddonCheckFamilyPolicy, AddonCheckSpec, AddonCheckStatus, createAddonCheckWithStatusForObservability(), AddonCheckAttemptOutcome, ThresholdValue

### Community 454 - "shard"
Cohesion: 0.18
Nodes (11): E2E_ADDONS, shard, shard, env, if, name, needs, runs-on (+3 more)

### Community 455 - "internal/adapter/declarative/versiondetect_test.go"
Cohesion: 0.35
Nodes (9): deploymentWithImage(), deploymentWithMetaVersion(), deploymentWithTemplateVersion(), TestDetectAddonVersion(), TestDetectAndGateVersion(), TestDetectAndGateVersion_FollowsPolicyOverrides(), TestDetectAndGateVersion_GateTargetsResolvedAddress(), TestEngine_SurfacesDetectedVersion() (+1 more)

### Community 456 - "Implementation execution evidence"
Cohesion: 0.18
Nodes (11): Adversarial review and integration progress, Baseline verification, Foundation progress, Implementation execution evidence, Lifecycle cases, Numeric boundaries, Payload field coverage, Requirement coverage ledger (+3 more)

### Community 457 - "runDescribe"
Cohesion: 0.29
Nodes (5): conditionsOf(), formatMap(), int32PtrString(), newDescribeCommand(), runDescribe()

### Community 458 - "runTarget"
Cohesion: 0.49
Nodes (7): runTarget, waitResult, blockedBy(), classifyWaitResult(), factory, inspectRunObject(), pollRun()

### Community 459 - "env"
Cohesion: 0.20
Nodes (10): BASE_REF, BUILD_RESULT, DCO_RESULT, GENERATED_RESULT, LINT_RESULT, QUALITY_RESULT, REUSE_RESULT, TEST_RESULT (+2 more)

### Community 460 - "TestExternalSecrets_HealthyAndEmptySyncSkipped"
Cohesion: 0.27
Nodes (5): esoDeployment(), NewExternalSecretsEngine(), esoHealthyObjects(), TestExternalSecrets_HealthyAndEmptySyncSkipped(), TestExternalSecrets_MissingDeploymentFails()

### Community 461 - "quorum-ratio-rollups/internal/controller/tracing_test.go"
Cohesion: 0.42
Nodes (7): TestListSelectedHealthChecks_ErrorNamesScope(), attrValue(), installInMemoryTracer(), newControllerScheme(), spanByName(), TestClusterHealthReconcile_EmitsSpan(), TestHealthCheckReconcile_EmitsSpan()

### Community 462 - "e2e"
Cohesion: 0.22
Nodes (9): if, name, needs, permissions, runs-on, e2e, permissions, contents (+1 more)

### Community 463 - "e2e.yml"
Cohesion: 0.25
Nodes (7): concurrency, cancel-in-progress, group, env, HELMFILE_VERSION, jobs, name

### Community 464 - "steps"
Cohesion: 0.36
Nodes (8): steps, steps, steps, id, if, name, run, uses

### Community 465 - "env"
Cohesion: 0.25
Nodes (8): env, BUNDLE_NAME, CATALOG_NAME, IMAGE_NAME, IMAGE_NAMESPACE, NODE_AGENT_NAME, PROBE_NAME, REGISTRY

### Community 466 - "runMain"
Cohesion: 0.43
Nodes (6): runMain(), TestMain_BadFlagExitsNonZero(), TestMain_ExitsNonZeroOnWriteError(), TestMain_HelpExitsZero(), TestMain_RunsAsMainOnDemand(), TestMain_WritesArtifacts()

### Community 467 - "quorum-ratio-rollups/internal/app/run_happy_test.go"
Cohesion: 0.29
Nodes (4): firstEnvtestBinaryDir(), TestMain(), TestRun_HappyPath_DefaultControllers(), TestRun_HappyPath_NoControllers()

### Community 468 - "fathomctl-build"
Cohesion: 0.25
Nodes (8): cmds, desc, vars, sh, sh, fathomctl-build, FATHOMCTL_REVISION, FATHOMCTL_VERSION

### Community 469 - "runLs"
Cohesion: 0.52
Nodes (5): lsOptions, emptyListMessage(), newLsCommand(), runLs(), selectorListOption()

### Community 471 - "build"
Cohesion: 0.33
Nodes (6): if, name, needs, permissions, runs-on, build

### Community 472 - "summary"
Cohesion: 0.33
Nodes (6): summary, if, name, needs, permissions, runs-on

### Community 473 - "env"
Cohesion: 0.33
Nodes (6): BASE_REF, EVENT_NAME, PLAN_RESULT, SHARD_RESULT, SHARDS, env

### Community 474 - "plan"
Cohesion: 0.33
Nodes (6): plan, shards, name, outputs, permissions, runs-on

### Community 475 - "steps"
Cohesion: 0.47
Nodes (6): steps, id, name, run, uses, steps

### Community 476 - "User Scenarios & Testing *(mandatory)*"
Cohesion: 0.33
Nodes (6): Edge Cases, User Scenarios & Testing *(mandatory)*, User Story 1 - Declare DNS resolution intent and have it validated (Priority: P1), User Story 2 - Every declared expectation is actually evaluated (Priority: P1), User Story 3 - Learn that resolution is failing, with evidence (Priority: P2), User Story 4 - Roll DNS health into the cluster-wide verdict (Priority: P3)

### Community 479 - "steps"
Cohesion: 0.40
Nodes (5): name, permissions, runs-on, steps, crd-compat

### Community 480 - "steps"
Cohesion: 0.40
Nodes (5): name, permissions, runs-on, steps, generated

### Community 481 - "lint"
Cohesion: 0.40
Nodes (5): lint, name, permissions, runs-on, steps

### Community 482 - "reuse"
Cohesion: 0.40
Nodes (5): reuse, name, permissions, runs-on, steps

### Community 483 - "on"
Cohesion: 0.40
Nodes (5): on, pull_request, push, branches, branches

### Community 484 - "on"
Cohesion: 0.40
Nodes (5): on, pull_request, push, branches, branches

### Community 486 - "Opt-in preview qualification guide"
Cohesion: 0.40
Nodes (5): Component and repository checks, Opt-in preview qualification guide, Real-cluster scenarios, Release evidence, Repository checks

### Community 487 - "cmds"
Cohesion: 0.40
Nodes (5): cmd, if, cmds, desc, e2e:cluster:fathom

### Community 488 - "test-e2e"
Cohesion: 0.40
Nodes (5): defer, task, test-e2e, cmds, desc

### Community 489 - "fathomctl-dist"
Cohesion: 0.40
Nodes (5): cmds, desc, vars, fathomctl-dist, FATHOMCTL_DIST_DIR

### Community 490 - "repositories"
Cohesion: 0.40
Nodes (4): repositories, name, oci, url

### Community 495 - "Runtime add-on definitions (qualification preview)"
Cohesion: 0.50
Nodes (4): Changes, drain and rollback, Runtime add-on definitions (qualification preview), Stage 1: review and install prerequisites, Stage 2: capture live identity and opt in

### Community 496 - "Dependencies & Execution Order"
Cohesion: 0.50
Nodes (4): Dependencies & Execution Order, Parallel Opportunities, Phase Dependencies, Story Dependencies

### Community 497 - "bundle-build"
Cohesion: 0.50
Nodes (4): cmds, deps, desc, bundle-build

### Community 498 - "config"
Cohesion: 0.50
Nodes (4): dnsDomain, dnsServer, localDns, config

### Community 499 - "pilot"
Cohesion: 0.50
Nodes (4): autoscaleEnabled, replicaCount, resources, pilot

### Community 504 - "on"
Cohesion: 0.67
Nodes (3): on, push, tags

### Community 505 - "bundle-push"
Cohesion: 0.67
Nodes (3): cmds, desc, bundle-push

### Community 506 - "catalog-build"
Cohesion: 0.67
Nodes (3): cmds, desc, catalog-build

### Community 507 - "catalog-push"
Cohesion: 0.67
Nodes (3): cmds, desc, catalog-push

### Community 508 - "clean"
Cohesion: 0.67
Nodes (3): cmds, desc, clean

### Community 509 - "crd-compat"
Cohesion: 0.67
Nodes (3): cmds, desc, crd-compat

### Community 510 - "default"
Cohesion: 0.67
Nodes (3): cmds, desc, default

### Community 511 - "docker-build"
Cohesion: 0.67
Nodes (3): cmds, desc, docker-build

### Community 512 - "docker-buildx-push"
Cohesion: 0.67
Nodes (3): cmds, desc, docker-buildx-push

### Community 513 - "docker-push"
Cohesion: 0.67
Nodes (3): cmds, desc, docker-push

### Community 514 - "e2e:cluster:addons"
Cohesion: 0.67
Nodes (3): cmds, desc, e2e:cluster:addons

### Community 515 - "e2e:cluster:down"
Cohesion: 0.67
Nodes (3): cmds, desc, e2e:cluster:down

### Community 516 - "e2e:cluster:samples"
Cohesion: 0.67
Nodes (3): cmds, desc, e2e:cluster:samples

### Community 517 - "fmt"
Cohesion: 0.67
Nodes (3): cmds, desc, fmt

### Community 518 - "gen:addon-rbac"
Cohesion: 0.67
Nodes (3): cmds, desc, gen:addon-rbac

### Community 519 - "generate"
Cohesion: 0.67
Nodes (3): cmds, desc, generate

### Community 520 - "helm:sync"
Cohesion: 0.67
Nodes (3): cmds, desc, helm:sync

### Community 521 - "images:refresh"
Cohesion: 0.67
Nodes (3): cmds, desc, images:refresh

### Community 522 - "manifests"
Cohesion: 0.67
Nodes (3): cmds, desc, manifests

### Community 523 - "node-agent-build"
Cohesion: 0.67
Nodes (3): cmds, desc, node-agent-build

### Community 524 - "node-agent-docker-build"
Cohesion: 0.67
Nodes (3): cmds, desc, node-agent-docker-build

### Community 525 - "node-agent-docker-buildx-push"
Cohesion: 0.67
Nodes (3): cmds, desc, node-agent-docker-buildx-push

### Community 526 - "node-agent-docker-push"
Cohesion: 0.67
Nodes (3): cmds, desc, node-agent-docker-push

### Community 527 - "probe-build"
Cohesion: 0.67
Nodes (3): cmds, desc, probe-build

### Community 528 - "probe-docker-build"
Cohesion: 0.67
Nodes (3): cmds, desc, probe-docker-build

### Community 529 - "probe-docker-buildx-push"
Cohesion: 0.67
Nodes (3): cmds, desc, probe-docker-buildx-push

### Community 530 - "probe-docker-push"
Cohesion: 0.67
Nodes (3): cmds, desc, probe-docker-push

### Community 531 - "test"
Cohesion: 0.67
Nodes (3): test, cmds, desc

### Community 532 - "verify-alert-rules"
Cohesion: 0.67
Nodes (3): verify-alert-rules, cmds, desc

### Community 533 - "verify-generated"
Cohesion: 0.67
Nodes (3): verify-generated, cmds, desc

### Community 534 - "verify-version-lockstep"
Cohesion: 0.67
Nodes (3): verify-version-lockstep, cmds, desc

### Community 535 - "vet"
Cohesion: 0.67
Nodes (3): vet, cmds, desc

### Community 536 - "vuln"
Cohesion: 0.67
Nodes (3): vuln, cmds, desc

### Community 537 - "image"
Cohesion: 0.67
Nodes (3): pullPolicy, tag, image

### Community 538 - "webhook"
Cohesion: 0.67
Nodes (3): webhook, create, resources

## Knowledge Gaps
- **28 isolated node(s):** `github.com/skaphos/fathom`, `github.com/skaphos/fathom/tools`, `Guard`, `ConfigMapCheck`, `CRDCheck` (+23 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 2861 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **95 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Fathom Documentation Index` connect `Fathom Documentation Index` to `7. Load typed addon definitions under explicit administrator authority`, `0008-fathomctl-lifecycle-via-helm-library.md`, `Network policies`, `0001-addondefinition-crd.md`, `Tasks: Node-agent metrics security`, `Fathom Architecture`?**
  _High betweenness centrality (0.054) - this node is a cross-community bridge._
- **Are the 108 inferred relationships involving `assertHasOutcome()` (e.g. with `TestAnnotationStaleness_NamedLock()` and `TestAnnotationStaleness_NodeList()`) actually correct?**
  _`assertHasOutcome()` has 108 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/skaphos/fathom`, `github.com/skaphos/fathom/tools`, `Guard` to the rest of the system?**
  _28 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `newRuntimeCheckFixture` be split into smaller, more focused modules?**
  _Cohesion score 0.059239373601789706 - nodes in this community are weakly interconnected._
- **Why does `Fathom Architecture` connect `Fathom Architecture` to `NodeCertificateCheck`, `Fathom Documentation Index`, `healthcheck_controller.go`, `Pod`, `DefaultOptions`, `ClusterHealthReconciler`, `runtime_wiring_test.go`?**
  _High betweenness centrality (0.040) - this node is a cross-community bridge._
- **Should `quorum-ratio-rollups/cmd/probe/main_test.go` be split into smaller, more focused modules?**
  _Cohesion score 0.08985507246376812 - nodes in this community are weakly interconnected._
- **Why does `New()` connect `New` to `.DeepCopy`, `quorum-ratio-rollups/cmd/probe/main_test.go`, `.DeepCopy`, `.DeepCopyInto`, `observeCheck`, `quorum-ratio-rollups/internal/adapter/certmanager/adapter_test.go`, `.Name`, `context.Context`, `.DeepCopy`, `.Run`, `Init`, `.DeepCopyInto`, `.DeepCopy`, `.DeepCopyInto`, `.Run`, `.Update`, `.DeepCopy`, `factory`, `newScheme`, `.DeepCopy`, `.DeepCopy`, `join`, `.DeepCopyInto`, `.DeepCopy`, `.DeepCopy`, `.DeepCopy`, `quorum-ratio-rollups/internal/probe/sweeper_test.go`?**
  _High betweenness centrality (0.027) - this node is a cross-community bridge._