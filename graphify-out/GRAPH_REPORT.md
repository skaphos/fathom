# Graph Report - fix+155-nodecert-report-spoofing  (2026-07-19)

## Corpus Check
- 189 files · ~192,578 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 3236 nodes · 6306 edges · 228 communities (214 shown, 14 thin omitted)
- Extraction: 85% EXTRACTED · 15% INFERRED · 0% AMBIGUOUS · INFERRED: 944 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `2e983381`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- .DeepCopyInto
- EvalContext
- PreferredServedVersion
- nodecertificatecheck_helpers.go
- adapter.go
- addoncheck_controller.go
- Monitoring and Alerting Guide
- CHANGELOG.md
- adapter_test.go
- newFakeClient
- Pod
- Built In Adapter Catalog
- Families and check types
- adapter_test.go
- properties
- writeFile
- addoncheck_controller_test.go
- Resource Types
- .DeepCopyInto
- Run
- assertHasOutcome
- main_test.go
- Release Process
- Release Process
- Release Process
- Release Process
- Release Process
- Release Process
- rbacgen.go
- Fathom Architecture
- paths.go
- Addon Adapters — Implementation Plan (v2)
- Container: manager
- join
- MustEngine
- Node Certificate Checks
- deepcopy_test.go
- IsReadVerb
- Capabilities
- Adapter
- CRD API Versioning Standard
- Getting Started
- Monitoring & Alerting
- .Run
- .Reconcile
- kedaHealthyObjects
- runEngine
- NewScheme
- podInNamespace
- assertHasDetail
- New
- adapter.go
- Configuration Reference
- Release Process
- TestAddToScheme
- NewRootCommand
- fathom
- Addon adapter RBAC
- Release Process
- annotation_test.go
- Release Process
- AddonCheck CRD (fathom.skaphos.io)
- .desiredDaemonSet
- Family
- deploymentInNamespace
- fathom
- fathom
- fathom
- fathom
- fathom
- AddonCheck Example
- README.md
- fathom
- .Reconcile
- 3. Probe-pod model for active in-cluster network checks
- image
- TestDescheduler_HealthyDeploymentMode
- Repository Guidelines
- Code Map
- Status and Conditions Reference
- 1. In-process Go interface as the AddonAdapter contract
- 2. HealthReport as a first-class custom resource
- Repository Guidelines
- Repository Guidelines
- Repository Guidelines
- definition.go
- Repository Guidelines
- Repository Guidelines
- Repository Guidelines
- Repository Guidelines
- Repository Guidelines
- Concepts
- 4. HealthCheck as a thin wrapper over a specialized check
- Fathom End-to-End Fixtures
- Repository Guidelines
- Repository Guidelines
- Repository Guidelines
- Repository Guidelines
- Repository Guidelines
- Repository Guidelines
- Repository Guidelines
- .Evaluate
- Repository Guidelines
- Scorecard OLM patch
- clusterhealth_types.go
- CheckResult
- Fathom Release Process
- properties
- newControllerScheme
- ClusterRole: clusterhealth-admin-role
- healthReportForAddonCheck
- README.md
- .Run
- msHealthyObjects
- policy_validation_test.go
- Addon Adapters Implementation Plan v2
- Supply-Chain Verification
- Supply-Chain Verification
- Options
- NewExternalDNSEngine
- TestEnvoyGateway_HealthyAndNoGatewaysSkipped
- healthreport_types.go
- BuiltInAdapters
- FamilyOutcome
- TestExternalSecrets_HealthyAndEmptySyncSkipped
- Contributing Guidelines
- Fathom Documentation
- addoncheck_types.go
- config/components/prometheus
- nodecertificatecheck_types.go
- Contributing Guidelines
- Contributing Guidelines
- Contributing Guidelines
- Contributing Guidelines
- rbac
- Contributing Guidelines
- CONTRIBUTING.md
- Contributing Guidelines
- Contributing Guidelines
- enabled
- NodeReport
- addoncheck_impersonation_test.go
- HealthCheckReconciler
- SetRunningInClusterForTest
- CLAUDE.md
- conflictOnceStatusClient
- Fathom User Guides
- config/rbac/kustomization.yaml
- healthcheck_types.go
- healthreport_types_test.go
- normalizeShell
- .runAddonCheck
- PolicyRule
- values.schema.json
- Init
- properties
- app.NewRootCommand (cobra root)
- Service controller-manager-metrics-service
- builder
- TestE2E
- RecordAdapterRun
- Supply-Chain Verification
- serviceAccountToken helper
- Controller envtest suite
- config/samples/kustomization.yaml
- ClusterRole: metrics-reader
- SPDX boilerplate (Go)
- GitHub Copilot Instructions for Fathom
- FamilyDefinition
- noMatchListClient
- factory
- Supply-Chain Verification
- detectAddonVersion
- ClusterHealthReconciler
- TestCommittedAddonRolesAreReadOnly
- Contributing Guidelines
- programmableAdapter
- createOrReuseHealthReport
- [0.3.1](https://github.com/skaphos/fathom/compare/v0.3.0...v0.3.1) (2026-07-06)
- Changelog
- Operating Fathom with automation and AI agents
- Managed Node Agent DaemonSet
- [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)
- endReconcileSpan
- CLAUDE.md (symlink to AGENTS.md)
- config/crd/kustomization.yaml
- config/crd/kustomizeconfig.yaml
- [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)
- [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)
- appendEntry
- bindAddress
- SAUsername
- check-version-lockstep.sh
- AddonCheck
- [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)
- extraArgs
- [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)
- countAbsent
- [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)
- check-coverage.sh
- [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)
- post-install.sh
- github.com/skaphos/fathom
- github.com/skaphos/fathom/tools

## God Nodes (most connected - your core abstractions)
1. `assertHasOutcome()` - 90 edges
2. `CheckResult` - 76 edges
3. `newFakeClient()` - 65 edges
4. `join()` - 54 edges
5. `Family` - 47 edges
6. `FamilyPolicy` - 36 edges
7. `assertHasDetail()` - 35 edges
8. `New()` - 33 edges
9. `Pod()` - 33 edges
10. `MustEngine()` - 29 edges

## Surprising Connections (you probably didn't know these)
- `Fathom CLI Entrypoint` --references--> `Taskfile.yml (task runner)`  [INFERRED]
  cmd/main.go → Taskfile.yml
- `Probe pod hardening defaults` --semantically_similar_to--> `Hardening profile defaults (non-root, drop caps, RO rootfs)`  [INFERRED] [semantically similar]
  README.md → docs/adr/0003-probe-pod-model.md
- `TestAddToScheme` --calls--> `NewScheme()`  [INFERRED]
  api/v1alpha1/groupversion_info_test.go → internal/app/run.go
- `TestSchemeBuilderRegisterReturnsSelf()` --calls--> `NewScheme()`  [INFERRED]
  api/v1alpha1/groupversion_info_test.go → internal/app/run.go
- `main()` --calls--> `Write()`  [INFERRED]
  cmd/main.go → internal/adapter/rbacgen/rbacgen.go

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Prometheus ServiceMonitor opt-in overlay path** — kustomize_default, kustomize_default_prometheus_component, kustomize_prometheus_component, prometheus_servicemonitor, prometheus_tls_patch [EXTRACTED 0.90]
- **Fathom CRD admin role family (healthcheck/healthreport/clusterhealth)** — config_rbac_healthcheck_admin_role, config_rbac_healthreport_admin_role, config_rbac_clusterhealth_admin_role [INFERRED 0.85]
- **HealthReport RBAC tier triple (admin/editor/viewer)** — config_rbac_healthreport_admin_role, config_rbac_healthreport_editor_role, config_rbac_healthreport_viewer_role [EXTRACTED 1.00]
- **HealthCheck RBAC tier triple (admin/editor/viewer)** — config_rbac_healthcheck_admin_role, config_rbac_healthcheck_editor_role, config_rbac_healthcheck_viewer_role [EXTRACTED 1.00]
- **AddonCheck -> HealthCheck -> ClusterHealth aggregation chain** — crd_addoncheck, crd_healthcheck, crd_clusterhealth, crd_healthreport [EXTRACTED 0.90]
- **Probe pod model: ADR, hardening, RBAC, sample usage** — adr_0003_decision_single_shot_pod, adr_0003_hardening_profile, rbac_pods_verbs, sample_addoncheck_coredns, readme_probe_pods [EXTRACTED 0.85]
- **Fathom Status and History Flow** — readme_fathom, docs_architecture_status_chain, readme_node_certificates [EXTRACTED 1.00]
- **Adapter Extension Flow** — authoring_paths, authoring_rbac, docs_architecture_extension_models [EXTRACTED 1.00]
- **Fathom Documentation Navigation** — docs_index, docs_architecture_overview, docs_code_map, authoring_guide, docs_reference_api_versioning_standard [EXTRACTED 1.00]
- **Sensor Projection Aggregate Flow** — docs_reference_status_conditions_addoncheck_sensor, docs_reference_status_conditions_healthcheck_projection, docs_reference_status_conditions_clusterhealth_rollup, docs_reference_status_conditions_healthreport_history [EXTRACTED 1.00]
- **Distributed Check Execution Surfaces** — docs_guides_concepts_probe_pods, node_certificate_agent_daemonset, node_certificate_scan_engine [EXTRACTED 1.00]
- **Declarative Addon Architecture** — addon_adapters_declarative_engine, addon_adapters_evaluator_library, addon_adapters_scoped_impersonation, addon_adapters_version_gating [EXTRACTED 1.00]

## Communities (228 total, 14 thin omitted)

### Community 0 - ".DeepCopyInto"
Cohesion: 0.08
Nodes (11): Object, AddonCheckList, ClusterHealthChildSummary, ClusterHealthList, HealthCheck, HealthCheckList, HealthReport, HealthReportList (+3 more)

### Community 1 - "EvalContext"
Cohesion: 0.12
Nodes (30): EvalContext, GroupVersionKind, containsString(), absenceOutcome(), deploymentAvailable(), derefReplicas(), durationThreshold(), effectiveAbsence() (+22 more)

### Community 2 - "PreferredServedVersion"
Cohesion: 0.23
Nodes (12): CustomResourceDefinitionConditionType, Established(), CustomResourceDefinition, PreferredServedVersion(), crd(), crdWithServed(), ConditionStatus, CustomResourceDefinition (+4 more)

### Community 3 - "nodecertificatecheck_helpers.go"
Cohesion: 0.13
Nodes (23): nodeCertReportMaxAge(), agentLabels(), aggregateNodeReports(), controlPlaneTolerations(), Client, Context, Duration, HealthReport (+15 more)

### Community 4 - "adapter.go"
Cohesion: 0.13
Nodes (38): FamilyPolicy, crdSupport, webhookClient, CreateOptions, certificateCheck(), certificateDetails(), certManagerComponents(), conditionDetails() (+30 more)

### Community 5 - "addoncheck_controller.go"
Cohesion: 0.22
Nodes (16): addonAdapterLookup, addonCheckDueForRun(), addonCheckInterval(), addonCheckPolicy(), addonCheckTargetRef(), addonCheckTimeout(), copyStringMap(), Adapter (+8 more)

### Community 6 - "Monitoring and Alerting Guide"
Cohesion: 0.19
Nodes (17): Required and Optional Absence Semantics, Add-on Checks Guide, Periodic and On Demand Adapter Execution, Fathom Concepts Guide, Sensor to Verdict Status Flow, Getting Started Guide, ClusterHealth Deployment Gate, Monitoring and Alerting Guide (+9 more)

### Community 7 - "CHANGELOG.md"
Cohesion: 0.18
Nodes (10): [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), Bug Fixes, Changelog, Features, [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), [0.3.1](https://github.com/skaphos/fathom/compare/v0.3.0...v0.3.1) (2026-07-06), Bug Fixes, Bug Fixes (+2 more)

### Community 8 - "adapter_test.go"
Cohesion: 0.14
Nodes (57): clientObject, New(), assertFamily(), assertHasDetail(), assertHasOutcome(), assertNoKind(), certManagerResource(), establishedCRD() (+49 more)

### Community 9 - "newFakeClient"
Cohesion: 0.15
Nodes (32): clientObject, Engine, NewCiliumEngine(), assertFamily(), assertNoKind(), assertNoOutcome(), ciliumCRD(), ciliumCRDNames() (+24 more)

### Community 10 - "Pod"
Cohesion: 0.06
Nodes (59): Affinity, coredns.Adapter.checkDNSResolution, coredns.Adapter.checkSystemHealth, dnsProbeLauncher interface, Rationale: probe pod for workload-perspective DNS (ADR-0003), coredns.Adapter.Run, fakeLauncher (test double), Launcher.bestEffortDelete (+51 more)

### Community 11 - "Built In Adapter Catalog"
Cohesion: 0.09
Nodes (35): Built In Adapter Catalog, Add-on Checks, cert-manager, Cilium, code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (spec:), code:yaml (spec:), code:yaml (spec:) (+27 more)

### Community 12 - "Families and check types"
Cohesion: 0.08
Nodes (34): Absence semantics, `AnnotationStalenessCheck` — a timestamp annotation's age, Authoring an Adapter, code:go (declarative.AddonDefinition{), code:go (// internal/adapter/declarative/externaldns.go), code:go (func (MyAdapter) Name() string            { return "my-addon), code:go ({APIGroups: []string{"apps"}, Resources: []string{"deploymen), code:console ($ go -C tools tool task gen:addon-rbac) (+26 more)

### Community 13 - "adapter_test.go"
Cohesion: 0.15
Nodes (46): clientObject, fakeLauncher, EndpointSlice, adapterWithLauncher(), assertHasDetail(), assertHasOutcome(), dnsEndpointSlice(), dnsEndpointSliceNamed() (+38 more)

### Community 14 - "properties"
Cohesion: 0.05
Nodes (39): type, type, type, type, type, type, type, type (+31 more)

### Community 15 - "writeFile"
Cohesion: 0.06
Nodes (79): Certificate, Context, Duration, Time, main(), metricsMux(), parseConfig(), publishGauges() (+71 more)

### Community 16 - "addoncheck_controller_test.go"
Cohesion: 0.11
Nodes (9): absentReportingAdapter, countingStatusClient, countingStatusWriter, fakeAddonAdapter, Client, NamespacedName, Object, SubResourceWriter (+1 more)

### Community 17 - "Resource Types"
Cohesion: 0.08
Nodes (31): AddonCheck API, AddonCheckFamilyPolicy, AddonCheckList, AddonCheckSpec, AddonCheckStatus, API Reference, CheckTargetRef, ClusterHealth API (+23 more)

### Community 18 - ".DeepCopyInto"
Cohesion: 0.12
Nodes (11): AddonCheckFamilyPolicy, AddonCheckSpec, AddonCheckStatus, ClusterHealth, ClusterHealthSpec, ClusterHealthStatus, HealthCheckStatus, HealthReportCheck (+3 more)

### Community 19 - "Run"
Cohesion: 0.07
Nodes (52): Cmd, Command, checkResult, corednsCheck, corednsHealthReport, dsRollout, eventList, healthReport (+44 more)

### Community 20 - "assertHasOutcome"
Cohesion: 0.23
Nodes (27): ConditionCheck, T, Unstructured, runManaged(), TestCondition_ClusterScopedListsWithoutNamespace(), TestCondition_ConditionStatus(), TestCondition_InvalidAPIVersionErrors(), TestCondition_InvalidSelectorErrors() (+19 more)

### Community 21 - "main_test.go"
Cohesion: 0.22
Nodes (25): Context, result, main(), run(), runDNS(), runTCPConnect(), runTCPListen(), claimAndReleasePort() (+17 more)

### Community 22 - "Release Process"
Cohesion: 0.15
Nodes (13): 1. Land Releasable Commits on `main`, 2. Run Local Release Checks, 3. Review and Merge the Release PR, 4. Tag-Triggered Publish, 5. Verify the Release, code:bash (operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bund), code:bash (IMG=ghcr.io/skaphos/fathom-operator@sha256:<digest> \), Image Pinning and Deploy-by-Digest Contract (+5 more)

### Community 23 - "Release Process"
Cohesion: 0.08
Nodes (25): 1. Land Releasable Commits on `main`, 2. Run Local Release Checks, 3. Review and Merge the Release PR, 4. Tag-Triggered Publish, 5. Verify the Release, code:bash (operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bund), code:bash (IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z), code:bash (cosign verify \) (+17 more)

### Community 24 - "Release Process"
Cohesion: 0.08
Nodes (25): 1. Land Releasable Commits on `main`, 2. Run Local Release Checks, 3. Review and Merge the Release PR, 4. Tag-Triggered Publish, 5. Verify the Release, code:bash (operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bund), code:bash (IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z), code:bash (cosign verify \) (+17 more)

### Community 25 - "Release Process"
Cohesion: 0.12
Nodes (15): 1. Land Releasable Commits on `main`, 2. Run Local Release Checks, 3. Review and Merge the Release PR, 4. Tag-Triggered Publish, 5. Verify the Release, code:bash (operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bund), code:yaml (components:), code:bash (IMG=ghcr.io/skaphos/fathom-operator@sha256:<digest> \) (+7 more)

### Community 26 - "Release Process"
Cohesion: 0.08
Nodes (25): 1. Land Releasable Commits on `main`, 2. Run Local Release Checks, 3. Review and Merge the Release PR, 4. Tag-Triggered Publish, 5. Verify the Release, code:bash (operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bund), code:bash (IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z), code:bash (cosign verify \) (+17 more)

### Community 27 - "Release Process"
Cohesion: 0.08
Nodes (25): 1. Land Releasable Commits on `main`, 2. Run Local Release Checks, 3. Review and Merge the Release PR, 4. Tag-Triggered Publish, 5. Verify the Release, code:bash (operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bund), code:bash (IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z), code:bash (cosign verify \) (+17 more)

### Community 28 - "rbacgen.go"
Cohesion: 0.15
Nodes (23): T, TestFilesRejectsIncompleteRule(), clusterRules(), Collect(), Files(), Adapter, groupsCell(), marshalDocs() (+15 more)

### Community 29 - "Fathom Architecture"
Cohesion: 0.07
Nodes (34): Adapter Authoring Guide, Declarative-First Adapter Paths, Per-Add-on Least-Privilege RBAC, AddonCheckReconciler, Built-in adapters, ClusterHealthReconciler, code:block1 (Pass(1) < Skipped(2) < Warn(3) < Unknown(4) < Fail(5) < Erro), code:block2 (adapter run                 mirror                   aggrega) (+26 more)

### Community 30 - "paths.go"
Cohesion: 0.17
Nodes (17): resolveCertPaths(), boolPtr(), T, TestResolveCertPathsFiltersDisallowed(), TestResolveTolerations(), AllowedPathPrefixes(), DefaultCertPaths(), FilterAllowedPaths() (+9 more)

### Community 31 - "Addon Adapters — Implementation Plan (v2)"
Cohesion: 0.10
Nodes (20): 0. Decisions that shape this plan, 1.1 Shipped, 1.2 Corrections to the record (from the v1 review), 1. Where we are today (and what v1 got wrong), 2.1 Declarative-first adapter (Decision 2), 2.2 Execution model (Decision 1), 2.3 Absence semantics (Decision 3), 2.4 Per-addon least-privilege client (Decision 4) (+12 more)

### Community 32 - "Container: manager"
Cohesion: 0.10
Nodes (21): Kustomization: config/manager, Args: --leader-elect --health-probe-bind-address=:8081, Container: manager, ContainerSecurityContext: drop ALL caps, no priv escalation, Deployment: controller-manager, Image: controller:latest, Liveness probe: /healthz:8081, Namespace: system (+13 more)

### Community 33 - "join"
Cohesion: 0.32
Nodes (20): join(), Load(), FlagSet, T, newTestFlags(), TestDefaultOptions_MatchFlagDefaults(), TestLoad_ConfigOverridesDefault(), TestLoad_EnvOverridesConfig() (+12 more)

### Community 34 - "MustEngine"
Cohesion: 0.13
Nodes (20): CronJob, AddonDefinition, crdAbsenceEngine(), Engine, T, TestCRD_AbsenceResolution(), cronJob(), CronJobCheck (+12 more)

### Community 35 - "Node Certificate Checks"
Cohesion: 0.12
Nodes (18): A minimal check, Cadence, pausing, and history, Choosing which nodes run the agent, code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (kubectl apply -f node-certificates.yaml), code:block3 (NAME                RESULT   REPORTING   DESIRED   AGE), code:block4 (/etc/kubernetes/pki              # apiserver, kubelet-client), code:yaml (spec:) (+10 more)

### Community 36 - "deepcopy_test.go"
Cohesion: 0.22
Nodes (25): deepCopyContract(), fullyPopulatedAddonCheck(), fullyPopulatedClusterHealth(), fullyPopulatedHealthCheck(), fullyPopulatedHealthReport(), fullyPopulatedNodeCertificateCheck(), AddonCheck, ClusterHealth (+17 more)

### Community 37 - "IsReadVerb"
Cohesion: 0.27
Nodes (7): RBACDeclarer, AddonServiceAccountName(), IsReadVerb(), T, TestAddonServiceAccountName(), TestIsReadVerb(), TestPolicyRuleIsReadOnly()

### Community 38 - "Capabilities"
Cohesion: 0.14
Nodes (16): Adapter, Capabilities, Outcome, Request, Result, ThresholdAdvertiser, Client, Duration (+8 more)

### Community 39 - "Adapter"
Cohesion: 0.27
Nodes (8): Adapter, check(), deploymentAvailable(), Client, Context, Deployment, Outcome, Time

### Community 40 - "CRD API Versioning Standard"
Cohesion: 0.11
Nodes (18): Alpha — `vNalphaM`, Beta — `vNbetaM`, code:block1 (/api/**                       @skaphos/maintainers), Conversion, CRD API Versioning Standard, Deprecation and removal, Enforcement and tooling, Introducing a new version (+10 more)

### Community 41 - "Getting Started"
Cohesion: 0.12
Nodes (17): 1. Install the operator, 2. Declare your first check, 3. Read the result, 4. Roll checks up into one verdict, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:block10 (AddonCheck ──runs──▶ status + HealthReport (history)), code:sh (kubectl -n fathom-system get deploy,pod), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+9 more)

### Community 42 - "Monitoring & Alerting"
Cohesion: 0.11
Nodes (18): 1. Read status with `kubectl`, 2. Scrape Prometheus metrics, 3. Tracing, 4. Alerting patterns, 5. Deployment gates, Add-on check results — read the status, not a metric, Certificate expiry (the clean case), code:sh (# One verdict for the namespace:) (+10 more)

### Community 43 - ".Run"
Cohesion: 0.15
Nodes (18): Adapter, check(), crdEstablished(), crdServesV1(), deploymentAvailable(), endAdapterRunSpan(), Client, Context (+10 more)

### Community 44 - ".Reconcile"
Cohesion: 0.16
Nodes (14): NodeCertificateCheckReconciler, checkForReportConfigMap(), Client, ConfigMap, Context, Logger, Manager, Object (+6 more)

### Community 45 - "kedaHealthyObjects"
Cohesion: 0.13
Nodes (23): Engine, WorkloadCheck, kedaDeployment(), NewKedaEngine(), conditionCR(), clientObject, T, Unstructured (+15 more)

### Community 46 - "runEngine"
Cohesion: 0.28
Nodes (20): absenceEngine(), deployEngine(), Engine, T, notReadyPod(), podWithRestarts(), runEngine(), statefulSet() (+12 more)

### Community 47 - "NewScheme"
Cohesion: 0.25
Nodes (19): CertWatcher, DefaultOptions(), BuildManagerOptions(), Scheme, NewScheme(), T, TestBuildAdapterRegistry_RegistersBuiltInAdapters(), TestBuildAdapterRegistry_WrapsRegistrationErrors() (+11 more)

### Community 48 - "podInNamespace"
Cohesion: 0.32
Nodes (18): podInNamespace(), Engine, NewIstioEngine(), clientObject, T, istioAmbientObjects(), istioCRDObjects(), istiodControlPlane() (+10 more)

### Community 49 - "assertHasDetail"
Cohesion: 0.26
Nodes (23): assertHasDetail(), MutatingWebhook, MutatingWebhookConfiguration, T, ValidatingWebhookConfiguration, WebhookCheck, mutatingConfig(), runWebhook() (+15 more)

### Community 50 - "New"
Cohesion: 0.11
Nodes (26): version, Adapter, Logger, New(), Context, Request, Result, T (+18 more)

### Community 51 - "adapter.go"
Cohesion: 0.19
Nodes (19): dnsProbeLauncher, coredns.Adapter, adapterOutcome (probe.Outcome mapping), dnsProbePodName, dnsTargets(), familyForTarget(), familyPolicy(), firstNamespace() (+11 more)

### Community 52 - "Configuration Reference"
Cohesion: 0.12
Nodes (16): Adding a New Option, code:block1 (command-line flag  >  environment variable  >  config file  ), code:block2 (FATHOM_<VIPER_KEY with "." replaced by "_", upper-cased>), code:yaml (metrics:), code:block4 (per-AddonCheck probeImage threshold  >  --probe-image (Reque), Config File, Configuration Reference, Environment Variables (+8 more)

### Community 53 - "Release Process"
Cohesion: 0.08
Nodes (27): 1. Land Releasable Commits on `main`, 2. Run Local Release Checks, 3. Review and Merge the Release PR, 4. Tag-Triggered Publish, 5. Verify the Release, code:bash (operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bund), code:yaml (components:), code:bash (IMG=ghcr.io/skaphos/fathom-operator@sha256:<digest> \) (+19 more)

### Community 54 - "TestAddToScheme"
Cohesion: 0.48
Nodes (6): T, TestAddToScheme, TestDeepCopyIntoExercise(), TestDeepCopyRoundTrip, TestGroupVersion, TestSchemeBuilderRegisterReturnsSelf()

### Community 55 - "NewRootCommand"
Cohesion: 0.10
Nodes (28): CancelFunc, main(), T, runMain(), TestMain_BadFlagExitsNonZero(), TestMain_HelpExitsZero(), TestMain_RunsAsMainOnDemand(), main() (+20 more)

### Community 56 - "fathom"
Cohesion: 0.14
Nodes (14): AddonCheck Example, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (go -C tools tool task probe-build), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+6 more)

### Community 57 - "Addon adapter RBAC"
Cohesion: 0.14
Nodes (14): Addon adapter RBAC, cert-manager, cilium, coredns, descheduler, envoy-gateway, external-dns, external-secrets (+6 more)

### Community 58 - "Release Process"
Cohesion: 0.08
Nodes (25): 1. Land Releasable Commits on `main`, 2. Run Local Release Checks, 3. Review and Merge the Release PR, 4. Tag-Triggered Publish, 5. Verify the Release, code:bash (operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bund), code:bash (IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z), code:bash (cosign verify \) (+17 more)

### Community 59 - "annotation_test.go"
Cohesion: 0.17
Nodes (20): daemonSetWithAnnotations(), AnnotationStalenessCheck, DaemonSet, T, Time, lockCheck(), lockJSON(), nodeRebootCheck() (+12 more)

### Community 60 - "Release Process"
Cohesion: 0.13
Nodes (15): 1. Land Releasable Commits on `main`, 2. Run Local Release Checks, 3. Review and Merge the Release PR, 4. Tag-Triggered Publish, 5. Verify the Release, code:bash (operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bund), code:yaml (components:), code:bash (IMG=ghcr.io/skaphos/fathom-operator@sha256:<digest> \) (+7 more)

### Community 61 - "AddonCheck CRD (fathom.skaphos.io)"
Cohesion: 0.06
Nodes (41): ADR-0001: In-process adapter contract, ContractVersion handshake at boot, Decision: in-process Go interface (option A), Option B: out-of-process gRPC plugins (rejected), Option C: OCI bundle adapters as Pods (rejected), Option D: Go plugin .so (rejected, fragile), Rationale: single OLM bundle, no sidecar, ADR-0002: HealthReport as first-class CRD (+33 more)

### Community 62 - ".desiredDaemonSet"
Cohesion: 0.16
Nodes (14): admissionPolicyUnsupported(), clearNodeCertRollupStatus(), ConditionStatus, DaemonSet, NodeCertificateCheck, nodeAgentRolledOut(), nodeAgentSpecHash(), nodeAgentTemplateNeedsWrite() (+6 more)

### Community 63 - "Family"
Cohesion: 0.16
Nodes (7): Family, fakeAdvertisingAdapter, fakePolicyAdapter, versionReportingAdapter, Context, Request, Result

### Community 64 - "deploymentInNamespace"
Cohesion: 0.30
Nodes (14): deploymentInNamespace(), Deployment, deploymentWithImage(), deploymentWithMetaVersion(), deploymentWithTemplateVersion(), Deployment, Engine, T (+6 more)

### Community 65 - "fathom"
Cohesion: 0.14
Nodes (14): AddonCheck Example, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (go -C tools tool task probe-build), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+6 more)

### Community 66 - "fathom"
Cohesion: 0.14
Nodes (14): AddonCheck Example, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (go -C tools tool task probe-build), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+6 more)

### Community 67 - "fathom"
Cohesion: 0.14
Nodes (14): AddonCheck Example, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (go -C tools tool task probe-build), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+6 more)

### Community 68 - "fathom"
Cohesion: 0.14
Nodes (14): AddonCheck Example, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (go -C tools tool task probe-build), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+6 more)

### Community 69 - "fathom"
Cohesion: 0.14
Nodes (14): AddonCheck Example, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (go -C tools tool task probe-build), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+6 more)

### Community 70 - "AddonCheck Example"
Cohesion: 0.11
Nodes (19): AddonCheck Example, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (go -C tools tool task probe-build), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+11 more)

### Community 71 - "README.md"
Cohesion: 0.16
Nodes (14): AddonCheck Example, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (go -C tools tool task probe-build), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+6 more)

### Community 72 - "fathom"
Cohesion: 0.14
Nodes (14): AddonCheck Example, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (go -C tools tool task probe-build), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+6 more)

### Community 73 - ".Reconcile"
Cohesion: 0.16
Nodes (14): clearClusterHealthAggregateStatus(), clusterHealthCoversNamespace(), ClusterHealth, Context, HealthCheck, LabelSelector, Object, Request (+6 more)

### Community 74 - "3. Probe-pod model for active in-cluster network checks"
Cohesion: 0.15
Nodes (13): 3. Probe-pod model for active in-cluster network checks, A. In-process net code from the operator pod, B. Sidecar container in the operator Deployment, C. DaemonSet probe agent on every node, Consequences, Considered Options, Context and Problem Statement, D. Single-shot probe Pod per check (+5 more)

### Community 75 - "image"
Cohesion: 0.11
Nodes (20): properties, required, type, properties, required, type, image, probeImage (+12 more)

### Community 76 - "TestDescheduler_HealthyDeploymentMode"
Cohesion: 0.21
Nodes (15): configMap(), ConfigMap, ConfigMapCheck, T, runConfigMap(), TestConfigMapCheck(), TestConfigMapCheck_AbsentInheritsOptional(), TestConfigMapCheck_NoAPIVersionAssertionPassesAnyYAML() (+7 more)

### Community 77 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 78 - "Code Map"
Cohesion: 0.15
Nodes (12): `api/v1alpha1/` — CRD types, Build and codegen, `cmd/` — entrypoints, Code Map, `config/` — manifests and packaging, `internal/adapter/` — registry and built-in adapters, `internal/app/` — process plumbing, `internal/controller/` — reconcilers (+4 more)

### Community 79 - "Status and Conditions Reference"
Cohesion: 0.15
Nodes (13): AddonCheck, ClusterHealth, code:sh (kubectl -n fathom-system get addoncheck cert-manager-system-), code:block2 (Pass < Skipped < Warn < Unknown < Fail < Error), code:sh (kubectl -n fathom-system annotate addoncheck cert-manager-sy), code:sh (kubectl -n fathom-system get configmap \), HealthCheck, HealthReport (+5 more)

### Community 80 - "1. In-process Go interface as the AddonAdapter contract"
Cohesion: 0.17
Nodes (12): 1. In-process Go interface as the AddonAdapter contract, A. In-process Go interface, B. Out-of-process gRPC plugins, C. OCI bundle adapters launched as Pods per run, Consequences, Considered Options, Context and Problem Statement, D. Go `plugin` package (+4 more)

### Community 81 - "2. HealthReport as a first-class custom resource"
Cohesion: 0.17
Nodes (12): 2. HealthReport as a first-class custom resource, A. Status conditions on the source check only, B. HealthReport as a first-class CRD, C. External time-series store, Consequences, Considered Options, Context and Problem Statement, D. Kubernetes Events (+4 more)

### Community 82 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 83 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 84 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 85 - "definition.go"
Cohesion: 0.20
Nodes (16): Posture, RBACRule, VersionSource, WorkloadKind, ConfigMapCheck, CRDCheck, CronJobCheck, AnnotationStalenessCheck (+8 more)

### Community 86 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 87 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 88 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 89 - "Repository Guidelines"
Cohesion: 0.15
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 90 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 91 - "Concepts"
Cohesion: 0.18
Nodes (11): Adapter Check Families, code:block1 (Pass < Skipped < Warn < Unknown < Fail < Error), code:block2 (runs                       mirror                      aggre), Concepts, How status flows, Next steps, Results and severity, The resource kinds (+3 more)

### Community 92 - "4. HealthCheck as a thin wrapper over a specialized check"
Cohesion: 0.18
Nodes (11): 4. HealthCheck as a thin wrapper over a specialized check, A. HealthCheck as a thin wrapper, B. Discriminator-typed unified CRD, C. Delete HealthCheck and ClusterHealth, Consequences, Considered Options, Context and Problem Statement, Decision Drivers (+3 more)

### Community 93 - "Fathom End-to-End Fixtures"
Cohesion: 0.15
Nodes (11): code:sh (# 1. Create the kind cluster.), code:sh (# Status conditions live on the AddonCheck.), code:sh (kubectl get pods -l app.kubernetes.io/managed-by=fathom -A -), code:sh (go -C tools tool task probe-docker-build PROBE_IMG=ghcr.io/s), Fathom End-to-End Fixtures, Inspecting AddonCheck Results, Layout, Prerequisites (+3 more)

### Community 94 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 95 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 96 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 97 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 98 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 99 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 100 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 101 - ".Evaluate"
Cohesion: 0.18
Nodes (11): conditionStatus(), ConditionCheck, LabelSelector, Outcome, Selector, Time, Unstructured, policySelector() (+3 more)

### Community 102 - "Repository Guidelines"
Cohesion: 0.17
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 103 - "Scorecard OLM patch"
Cohesion: 0.20
Nodes (12): Kustomization: config/manifests, Kustomization: config/scorecard, Image: quay.io/operator-framework/scorecard-test:v1.42.2, Scorecard base Configuration, Scorecard basic patch, Scorecard OLM patch, Scorecard test: basic-check-spec, Scorecard test: olm-bundle-validation (+4 more)

### Community 104 - "clusterhealth_types.go"
Cohesion: 0.12
Nodes (20): ClusterHealth CRD type, Condition, LabelSelector, ListMeta, ObjectMeta, Time, TypeMeta, ClusterHealth (+12 more)

### Community 105 - "CheckResult"
Cohesion: 0.29
Nodes (9): CheckResult, TargetRef, AnnotationStalenessCheck, GroupVersion, Time, Unstructured, skippedResult(), Outcome (+1 more)

### Community 106 - "Fathom Release Process"
Cohesion: 0.18
Nodes (13): Cobra and Viper Configuration Model, Fathom Repository Guidelines, CRD Compatibility and Version Lifecycle, Fathom Release History, CLAUDE Agent Briefing Symlink, Fathom Contributor Workflow, AddonCheck HealthCheck ClusterHealth Chain, Node Certificate Checks (+5 more)

### Community 107 - "properties"
Cohesion: 0.12
Nodes (17): type, type, type, type, type, allOf, $comment, properties (+9 more)

### Community 108 - "newControllerScheme"
Cohesion: 0.17
Nodes (17): failingHealthCheckListClient, InMemoryExporter, Client, Context, ObjectList, T, TestListSelectedHealthChecks_ErrorNamesScope(), attrValue() (+9 more)

### Community 109 - "ClusterRole: clusterhealth-admin-role"
Cohesion: 0.29
Nodes (11): API Group: fathom.skaphos.io, ClusterRole: clusterhealth-admin-role, ClusterRole: healthcheck-admin-role, ClusterRole: healthcheck-editor-role, ClusterRole: healthcheck-viewer-role, ClusterRole: healthreport-admin-role, ClusterRole: healthreport-editor-role, ClusterRole: healthreport-viewer-role (+3 more)

### Community 110 - "healthReportForAddonCheck"
Cohesion: 0.21
Nodes (13): HealthReportCheck, aggregateHealthReportResult, HealthReport, Outcome, Time, healthReportChecks(), healthReportForAddonCheck, healthReportResult (outcome mapping) (+5 more)

### Community 112 - ".Run"
Cohesion: 0.17
Nodes (15): adapterName(), disableHTTP2(), Adapter, firstEnvtestBinaryDir(), T, TestAdapterName_NilReturnsPlaceholder(), TestAdapterName_NonNilReturnsName(), TestDisableHTTP2() (+7 more)

### Community 113 - "msHealthyObjects"
Cohesion: 0.32
Nodes (12): Engine, NewMetricsServerEngine(), apiService(), clientObject, T, Unstructured, msHealthyObjects(), TestMetricsServer_AdapterMetadata() (+4 more)

### Community 114 - "policy_validation_test.go"
Cohesion: 0.21
Nodes (13): AddonCheckFamilyPolicy, badSelector(), checkWithPolicy(), AddonCheck, Context, LabelSelector, Request, Result (+5 more)

### Community 115 - "Addon Adapters Implementation Plan v2"
Cohesion: 0.15
Nodes (14): Declarative Addon Engine, Addon Evaluator Library, Go Adapter Escape Hatch, Addon Adapters Implementation Plan v2, Managed Resource Quorum Semantics, Per Addon Scoped Impersonating Client, Addon Version Detection and Gating, Workload Vantage Probe Pods (+6 more)

### Community 116 - "Supply-Chain Verification"
Cohesion: 0.20
Nodes (10): code:bash (IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z), code:bash (cosign verify \), code:bash (gh attestation verify "oci://${IMAGE}" --owner skaphos), code:bash (cosign verify-attestation \), code:bash (gh release download "vX.Y.Z" --repo skaphos/fathom --pattern), code:bash (syft "${IMAGE}" -o spdx-json), Inspect the SBOM, Supply-Chain Verification (+2 more)

### Community 117 - "Supply-Chain Verification"
Cohesion: 0.20
Nodes (14): code:bash (IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z), code:bash (cosign verify \), code:bash (gh attestation verify "oci://${IMAGE}" --owner skaphos), code:bash (cosign verify-attestation \), code:bash (gh release download "vX.Y.Z" --repo skaphos/fathom --pattern), code:bash (syft "${IMAGE}" -o spdx-json), Inspect the SBOM, Supply-Chain Verification (+6 more)

### Community 118 - "Options"
Cohesion: 0.36
Nodes (8): flagBinding, MetricsOptions, Options, TracingOptions, WebhookOptions, bindings(), FlagSet, RegisterFlags()

### Community 119 - "NewExternalDNSEngine"
Cohesion: 0.36
Nodes (10): Engine, NewExternalDNSEngine(), extdnsHealthyObjects(), clientObject, T, TestExternalDNS_AdapterMetadata(), TestExternalDNS_DeploymentNameThresholdOverride(), TestExternalDNS_HealthyPassesAllFamilies() (+2 more)

### Community 120 - "TestEnvoyGateway_HealthyAndNoGatewaysSkipped"
Cohesion: 0.29
Nodes (11): Engine, NewEnvoyGatewayEngine(), egHealthyObjects(), gatewayObject(), clientObject, T, Unstructured, TestEnvoyGateway_AdapterMetadata() (+3 more)

### Community 121 - "healthreport_types.go"
Cohesion: 0.18
Nodes (13): Duration, ListMeta, ObjectMeta, Time, TypeMeta, HealthReport, HealthReportCheck, HealthReportList (+5 more)

### Community 122 - "BuiltInAdapters"
Cohesion: 0.16
Nodes (15): appFakeAdapter, Setupper, Bool, Checker, BuildAdapterRegistry(), BuiltInAdapters(), DefaultControllers(), Context (+7 more)

### Community 123 - "FamilyOutcome"
Cohesion: 0.21
Nodes (11): endAdapterRunSpan(), Request, Result, Span, endRunSpan(), Context, Request, Result (+3 more)

### Community 124 - "TestExternalSecrets_HealthyAndEmptySyncSkipped"
Cohesion: 0.26
Nodes (9): esoDeployment(), Engine, WorkloadCheck, NewExternalSecretsEngine(), esoHealthyObjects(), clientObject, T, TestExternalSecrets_HealthyAndEmptySyncSkipped() (+1 more)

### Community 125 - "Contributing Guidelines"
Cohesion: 0.29
Nodes (7): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Safety Expectations, Testing

### Community 126 - "Fathom Documentation"
Cohesion: 0.25
Nodes (7): Architecture Decision Records, Contents, Design & planning, Fathom Documentation, Guides — for platform teams, Other repository docs, Reference & internals

### Community 127 - "addoncheck_types.go"
Cohesion: 0.15
Nodes (17): AddonCheck CRD type, Condition, Duration, LabelSelector, ListMeta, ObjectMeta, Time, TypeMeta (+9 more)

### Community 128 - "config/components/prometheus"
Cohesion: 0.36
Nodes (8): config/default kustomization, manager_metrics_patch (8443 HTTPS), Prometheus component opt-in, config/components/prometheus, controller-manager-metrics-monitor ServiceMonitor, ServiceMonitor cert-manager TLS patch, Default deployment topology, ServiceMonitor opt-in rationale

### Community 129 - "nodecertificatecheck_types.go"
Cohesion: 0.19
Nodes (12): Condition, Duration, ListMeta, ObjectMeta, Time, Toleration, TypeMeta, NodeCertificateCheck (+4 more)

### Community 130 - "Contributing Guidelines"
Cohesion: 0.29
Nodes (7): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Safety Expectations, Testing

### Community 131 - "Contributing Guidelines"
Cohesion: 0.29
Nodes (7): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Safety Expectations, Testing

### Community 132 - "Contributing Guidelines"
Cohesion: 0.29
Nodes (7): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Safety Expectations, Testing

### Community 133 - "Contributing Guidelines"
Cohesion: 0.29
Nodes (7): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Safety Expectations, Testing

### Community 134 - "rbac"
Cohesion: 0.14
Nodes (14): type, type, type, annotations, create, name, rbac, serviceAccount (+6 more)

### Community 135 - "Contributing Guidelines"
Cohesion: 0.29
Nodes (7): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Safety Expectations, Testing

### Community 136 - "CONTRIBUTING.md"
Cohesion: 0.25
Nodes (7): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Safety Expectations, Testing

### Community 137 - "Contributing Guidelines"
Cohesion: 0.29
Nodes (7): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Safety Expectations, Testing

### Community 138 - "Contributing Guidelines"
Cohesion: 0.29
Nodes (7): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Safety Expectations, Testing

### Community 139 - "enabled"
Cohesion: 0.14
Nodes (14): properties, type, type, type, maximum, minimum, type, config (+6 more)

### Community 140 - "NodeReport"
Cohesion: 0.60
Nodes (5): Duration, Time, nodeCertReportFresh(), DecodeReport(), NodeReport

### Community 141 - "addoncheck_impersonation_test.go"
Cohesion: 0.29
Nodes (8): fakeClientFactory, addonSA(), Client, T, TestAdapterClient(), TestRunAddonCheckFailsClosedWhenNamespaceEmptyInCluster(), TestRunAddonCheckFailsClosedWithoutScopedClient(), ServiceAccount

### Community 142 - "HealthCheckReconciler"
Cohesion: 0.15
Nodes (14): HealthCheckReconciler, clearMirroredHealthCheckStatus(), Client, Condition, Context, HealthCheck, Manager, Object (+6 more)

### Community 143 - "SetRunningInClusterForTest"
Cohesion: 0.16
Nodes (10): defaultRunningInCluster(), inClusterFromConfigErr(), T, TestInClusterFromConfigErr(), RunningInCluster(), SetRunningInClusterForTest(), T, TestRunningInCluster_TestOverride() (+2 more)

### Community 144 - "CLAUDE.md"
Cohesion: 0.15
Nodes (12): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+4 more)

### Community 145 - "conflictOnceStatusClient"
Cohesion: 0.25
Nodes (6): conflictOnceStatusClient, conflictOnceStatusWriter, Client, Context, Object, SubResourceWriter

### Community 146 - "Fathom User Guides"
Cohesion: 0.33
Nodes (5): Automating Fathom, Conventions used in these guides, Fathom User Guides, How to use Fathom, Start here

### Community 147 - "config/rbac/kustomization.yaml"
Cohesion: 0.50
Nodes (5): ClusterRole clusterhealth-editor-role, ClusterRole clusterhealth-viewer-role, config/rbac/kustomization.yaml, ServiceAccount controller-manager, ClusterRoleBinding manager-rolebinding

### Community 148 - "healthcheck_types.go"
Cohesion: 0.13
Nodes (14): Condition, ListMeta, ObjectMeta, Time, TypeMeta, CheckTargetRef, HealthCheck, HealthCheckList (+6 more)

### Community 149 - "healthreport_types_test.go"
Cohesion: 0.60
Nodes (4): T, TestHealthReportResultSeverity_EmptyAndUnrecognizedReturnZero(), TestHealthReportResultSeverity_OrderingAcrossEnumValues(), TestHealthReportResultSeverity_PassIsLowestNonZero()

### Community 150 - "normalizeShell"
Cohesion: 0.60
Nodes (4): T, normalizeShell(), stripShellComment(), TestCoverageGateSkipsNoPackages()

### Community 151 - ".runAddonCheck"
Cohesion: 0.23
Nodes (9): AddonCheckReconciler, addonAdapterLookup interface, Client, Context, Logger, Manager, Scheme, Tracer (+1 more)

### Community 152 - "PolicyRule"
Cohesion: 0.23
Nodes (9): PolicyRule, T, hasResource(), hasVerb(), TestRBACRulesDeclaresDryRunException(), T, hasResource(), hasVerb() (+1 more)

### Community 153 - "values.schema.json"
Cohesion: 0.15
Nodes (12): properties, required, type, nodeAgent, required, $schema, title, type (+4 more)

### Community 154 - "Init"
Cohesion: 0.33
Nodes (8): Context, Init(), T, restoreGlobalProvider(), TestInit_DisabledInstallsNoopProvider(), TestInit_EnabledInstallsRecordingProvider(), Config, ShutdownFunc

### Community 155 - "properties"
Cohesion: 0.15
Nodes (13): type, type, type, interval, labels, namespace, scrapeTimeout, serviceMonitor (+5 more)

### Community 156 - "app.NewRootCommand (cobra root)"
Cohesion: 0.50
Nodes (4): app.NewRootCommand (cobra root), signalAwareContext, Fathom CLI Entrypoint, Taskfile.yml (task runner)

### Community 157 - "Service controller-manager-metrics-service"
Cohesion: 0.50
Nodes (3): Service controller-manager-metrics-service, NetworkPolicy allow-metrics-traffic, config/network-policy/kustomization.yaml

### Community 158 - "builder"
Cohesion: 0.40
Nodes (3): GroupVersion, SchemeBuilder, builder

### Community 159 - "TestE2E"
Cohesion: 0.50
Nodes (3): e2e BeforeSuite/AfterSuite, T, TestE2E()

### Community 160 - "RecordAdapterRun"
Cohesion: 0.24
Nodes (10): Duration, RecordAdapterRun(), RecordReconcile(), T, TestAdapterMetrics(), TestMetricsAreValidCollectors(), TestMetricsCanBeUsedFromOtherPackages(), TestReconcileMetrics() (+2 more)

### Community 161 - "Supply-Chain Verification"
Cohesion: 0.17
Nodes (12): code:bash (IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z), code:bash (cosign verify \), code:bash (gh attestation verify "oci://${IMAGE}" --owner skaphos), code:bash (cosign verify-attestation \), code:bash (gh release download "vX.Y.Z" --repo skaphos/fathom --pattern), code:bash (syft "${IMAGE}" -o spdx-json), code:yaml (components:), Default Deployment Topology (+4 more)

### Community 163 - "Controller envtest suite"
Cohesion: 0.40
Nodes (5): getFirstFoundEnvTestBinaryDir, Controller envtest suite, getFirstFoundEnvTestBinaryDir(), T, TestControllers()

### Community 167 - "GitHub Copilot Instructions for Fathom"
Cohesion: 0.17
Nodes (11): Codebase Shape, Commit and Branch Guidance, Documentation Expectations, GitHub Copilot Instructions for Fathom, Go and Repository Conventions, Knowledge Graph (`graphify-out/`), Pull Request Instructions, Safety Rules (+3 more)

### Community 168 - "FamilyDefinition"
Cohesion: 0.20
Nodes (9): CRDCheck, Evaluator, FamilyDefinition, AnnotationStalenessCheck, ConditionCheck, ConfigMapCheck, CronJobCheck, WebhookCheck (+1 more)

### Community 170 - "noMatchListClient"
Cohesion: 0.27
Nodes (7): failingListClient, noMatchListClient, Client, Context, Object, ObjectList, ObjectKey

### Community 171 - "factory"
Cohesion: 0.24
Nodes (8): ClientFactory, factory, Client, Manager, Mutex, Scheme, New(), RESTMapper

### Community 174 - "Supply-Chain Verification"
Cohesion: 0.20
Nodes (10): code:bash (IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z), code:bash (cosign verify \), code:bash (gh attestation verify "oci://${IMAGE}" --owner skaphos), code:bash (cosign verify-attestation \), code:bash (gh release download "vX.Y.Z" --repo skaphos/fathom --pattern), code:bash (syft "${IMAGE}" -o spdx-json), Inspect the SBOM, Supply-Chain Verification (+2 more)

### Community 177 - "detectAddonVersion"
Cohesion: 0.33
Nodes (7): Container, detectAddonVersion(), Client, Context, imageTag(), pickImage(), validSupportedVersions()

### Community 178 - "ClusterHealthReconciler"
Cohesion: 0.22
Nodes (8): ClusterHealthReconciler, clusterHealthsForHealthCheck (watch map), Client, Manager, Scheme, Tracer, ClusterHealthReconciler.SetupWithManager, ClusterHealth Controller envtest suite

### Community 179 - "TestCommittedAddonRolesAreReadOnly"
Cohesion: 0.42
Nodes (8): allowedWrites(), T, repoRoot(), TestCommittedAddonRolesAreReadOnly(), TestModelGrantsAreJustified(), TestUnjustifiedGrantsCatchesViolations(), UnjustifiedGrants(), writeKey

### Community 180 - "Contributing Guidelines"
Cohesion: 0.25
Nodes (7): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Safety Expectations, Testing

### Community 181 - "programmableAdapter"
Cohesion: 0.29
Nodes (3): programmableAdapter, Mutex, Outcome

### Community 182 - "createOrReuseHealthReport"
Cohesion: 0.39
Nodes (7): createOrReuseHealthReport(), deterministicHealthReportName(), Client, Context, HealthReport, useDeterministicHealthReportName(), validateReusableHealthReport()

### Community 183 - "[0.3.1](https://github.com/skaphos/fathom/compare/v0.3.0...v0.3.1) (2026-07-06)"
Cohesion: 0.29
Nodes (7): [0.3.1](https://github.com/skaphos/fathom/compare/v0.3.0...v0.3.1) (2026-07-06), Bug Fixes, [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), Bug Fixes, Bug Fixes, Changelog, Features

### Community 184 - "Changelog"
Cohesion: 0.29
Nodes (7): [0.4.0](https://github.com/skaphos/fathom/compare/v0.3.1...v0.4.0) (2026-07-15), [0.4.1](https://github.com/skaphos/fathom/compare/v0.4.0...v0.4.1) (2026-07-18), ⚠ BREAKING CHANGES, Bug Fixes, Bug Fixes, Changelog, Features

### Community 185 - "Operating Fathom with automation and AI agents"
Cohesion: 0.29
Nodes (7): 1. Confirm the target and release, 2. Discover add-ons and the cluster delivery model, 3. Install and verify the operator, 4. Create a first cluster health signal, 5. Read, refresh, and troubleshoot results, 6. Remove only what was authorized, Operating Fathom with automation and AI agents

### Community 186 - "Managed Node Agent DaemonSet"
Cohesion: 0.33
Nodes (6): Node Certificate Expiry Gauge, Node Agent Image Configuration, NodeCertificateCheck Fresh Coverage, Managed Node Agent DaemonSet, Minimal Read Only HostPath Surface, On Disk X.509 Scan Engine

### Community 187 - "[0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)"
Cohesion: 0.40
Nodes (6): [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), [0.3.1](https://github.com/skaphos/fathom/compare/v0.3.0...v0.3.1) (2026-07-06), Bug Fixes, Bug Fixes, Changelog, Features

### Community 188 - "endReconcileSpan"
Cohesion: 0.40
Nodes (4): endReconcileSpan(), Span, Tracer, reconcilerTracer()

### Community 193 - "[0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)"
Cohesion: 0.33
Nodes (6): [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), [0.3.1](https://github.com/skaphos/fathom/compare/v0.3.0...v0.3.1) (2026-07-06), Bug Fixes, Bug Fixes, Changelog, Features

### Community 194 - "[0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)"
Cohesion: 0.40
Nodes (5): [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), Bug Fixes, Features, Bug Fixes, Features

### Community 195 - "appendEntry"
Cohesion: 0.50
Nodes (4): webhookEntry, appendEntry(), WebhookClientConfig, ServiceReference

### Community 196 - "bindAddress"
Cohesion: 0.40
Nodes (5): type, properties, type, bindAddress, healthProbe

### Community 197 - "SAUsername"
Cohesion: 0.60
Nodes (4): SAUsername(), T, TestClientForSetsImpersonationAndMemoizes(), TestSAUsername()

### Community 200 - "[0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)"
Cohesion: 0.50
Nodes (4): [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), Bug Fixes, Changelog, Features

### Community 201 - "extraArgs"
Cohesion: 0.50
Nodes (4): items, type, type, extraArgs

### Community 202 - "[0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)"
Cohesion: 0.50
Nodes (4): [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), Bug Fixes, Changelog, Features

### Community 203 - "countAbsent"
Cohesion: 0.50
Nodes (3): T, TestCountAbsent(), countAbsent()

### Community 204 - "[0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)"
Cohesion: 0.50
Nodes (4): [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), Bug Fixes, Changelog, Features

### Community 206 - "[0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)"
Cohesion: 0.50
Nodes (4): [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), Bug Fixes, Changelog, Features

## Knowledge Gaps
- **887 isolated node(s):** `post-install.sh script`, `$schema`, `title`, `type`, `probeImage` (+882 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **14 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `join()` connect `join` to `EvalContext`, `nodecertificatecheck_helpers.go`, `Controller envtest suite`, `addoncheck_controller.go`, `.Run`, `writeFile`, `NewScheme`, `.Run`, `policy_validation_test.go`, `TestCommittedAddonRolesAreReadOnly`, `Run`, `main_test.go`, `Options`, `NewRootCommand`, `createOrReuseHealthReport`, `normalizeShell`, `rbacgen.go`?**
  _High betweenness centrality (0.079) - this node is a cross-community bridge._
- **Why does `CheckResult` connect `CheckResult` to `EvalContext`, `adapter.go`, `adapter_test.go`, `newFakeClient`, `adapter_test.go`, `assertHasOutcome`, `MustEngine`, `Capabilities`, `Adapter`, `.Run`, `runEngine`, `detectAddonVersion`, `assertHasDetail`, `adapter.go`, `annotation_test.go`, `Family`, `countAbsent`, `TestDescheduler_HealthyDeploymentMode`, `.Evaluate`, `healthReportForAddonCheck`, `FamilyOutcome`?**
  _High betweenness centrality (0.077) - this node is a cross-community bridge._
- **Why does `Family` connect `Family` to `EvalContext`, `adapter.go`, `addoncheck_controller.go`, `adapter_test.go`, `newFakeClient`, `assertHasOutcome`, `Capabilities`, `FamilyDefinition`, `.Run`, `kedaHealthyObjects`, `runEngine`, `podInNamespace`, `assertHasDetail`, `New`, `adapter.go`, `TestDescheduler_HealthyDeploymentMode`, `CheckResult`, `msHealthyObjects`, `NewExternalDNSEngine`, `TestEnvoyGateway_HealthyAndNoGatewaysSkipped`, `FamilyOutcome`, `TestExternalSecrets_HealthyAndEmptySyncSkipped`?**
  _High betweenness centrality (0.043) - this node is a cross-community bridge._
- **Are the 78 inferred relationships involving `assertHasOutcome()` (e.g. with `TestAnnotationStaleness_NamedLock()` and `TestAnnotationStaleness_NodeList()`) actually correct?**
  _`assertHasOutcome()` has 78 INFERRED edges - model-reasoned connections that need verification._
- **Are the 51 inferred relationships involving `newFakeClient()` (e.g. with `runAnnotation()` and `runManaged()`) actually correct?**
  _`newFakeClient()` has 51 INFERRED edges - model-reasoned connections that need verification._
- **Are the 51 inferred relationships involving `join()` (e.g. with `.Validate()` and `.checkCRD()`) actually correct?**
  _`join()` has 51 INFERRED edges - model-reasoned connections that need verification._
- **Are the 19 inferred relationships involving `Family` (e.g. with `.Run()` and `.Run()`) actually correct?**
  _`Family` has 19 INFERRED edges - model-reasoned connections that need verification._