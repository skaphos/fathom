# Graph Report - declarative-engine  (2026-07-04)

## Corpus Check
- 115 files · ~127,468 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 1629 nodes · 2734 edges · 99 communities (85 shown, 14 thin omitted)
- Extraction: 87% EXTRACTED · 13% INFERRED · 0% AMBIGUOUS · INFERRED: 365 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `af34f18e`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- [[_COMMUNITY_Community 0|Community 0]]
- [[_COMMUNITY_Community 1|Community 1]]
- [[_COMMUNITY_Community 2|Community 2]]
- [[_COMMUNITY_Community 3|Community 3]]
- [[_COMMUNITY_Community 4|Community 4]]
- [[_COMMUNITY_Community 5|Community 5]]
- [[_COMMUNITY_Community 6|Community 6]]
- [[_COMMUNITY_Community 7|Community 7]]
- [[_COMMUNITY_Community 8|Community 8]]
- [[_COMMUNITY_Community 9|Community 9]]
- [[_COMMUNITY_Community 10|Community 10]]
- [[_COMMUNITY_Community 11|Community 11]]
- [[_COMMUNITY_Community 12|Community 12]]
- [[_COMMUNITY_Community 13|Community 13]]
- [[_COMMUNITY_Community 14|Community 14]]
- [[_COMMUNITY_Community 15|Community 15]]
- [[_COMMUNITY_Community 16|Community 16]]
- [[_COMMUNITY_Community 17|Community 17]]
- [[_COMMUNITY_Community 18|Community 18]]
- [[_COMMUNITY_Community 19|Community 19]]
- [[_COMMUNITY_Community 20|Community 20]]
- [[_COMMUNITY_Community 21|Community 21]]
- [[_COMMUNITY_Community 22|Community 22]]
- [[_COMMUNITY_Community 23|Community 23]]
- [[_COMMUNITY_Community 24|Community 24]]
- [[_COMMUNITY_Community 25|Community 25]]
- [[_COMMUNITY_Community 26|Community 26]]
- [[_COMMUNITY_Community 27|Community 27]]
- [[_COMMUNITY_Community 28|Community 28]]
- [[_COMMUNITY_Community 29|Community 29]]
- [[_COMMUNITY_Community 30|Community 30]]
- [[_COMMUNITY_Community 31|Community 31]]
- [[_COMMUNITY_Community 32|Community 32]]
- [[_COMMUNITY_Community 33|Community 33]]
- [[_COMMUNITY_Community 34|Community 34]]
- [[_COMMUNITY_Community 35|Community 35]]
- [[_COMMUNITY_Community 36|Community 36]]
- [[_COMMUNITY_Community 37|Community 37]]
- [[_COMMUNITY_Community 38|Community 38]]
- [[_COMMUNITY_Community 39|Community 39]]
- [[_COMMUNITY_Community 40|Community 40]]
- [[_COMMUNITY_Community 41|Community 41]]
- [[_COMMUNITY_Community 42|Community 42]]
- [[_COMMUNITY_Community 43|Community 43]]
- [[_COMMUNITY_Community 44|Community 44]]
- [[_COMMUNITY_Community 45|Community 45]]
- [[_COMMUNITY_Community 46|Community 46]]
- [[_COMMUNITY_Community 47|Community 47]]
- [[_COMMUNITY_Community 48|Community 48]]
- [[_COMMUNITY_Community 49|Community 49]]
- [[_COMMUNITY_Community 50|Community 50]]
- [[_COMMUNITY_Community 51|Community 51]]
- [[_COMMUNITY_Community 52|Community 52]]
- [[_COMMUNITY_Community 53|Community 53]]
- [[_COMMUNITY_Community 54|Community 54]]
- [[_COMMUNITY_Community 55|Community 55]]
- [[_COMMUNITY_Community 56|Community 56]]
- [[_COMMUNITY_Community 57|Community 57]]
- [[_COMMUNITY_Community 58|Community 58]]
- [[_COMMUNITY_Community 59|Community 59]]
- [[_COMMUNITY_Community 60|Community 60]]
- [[_COMMUNITY_Community 61|Community 61]]
- [[_COMMUNITY_Community 62|Community 62]]
- [[_COMMUNITY_Community 63|Community 63]]
- [[_COMMUNITY_Community 64|Community 64]]
- [[_COMMUNITY_Community 66|Community 66]]
- [[_COMMUNITY_Community 67|Community 67]]
- [[_COMMUNITY_Community 68|Community 68]]
- [[_COMMUNITY_Community 69|Community 69]]
- [[_COMMUNITY_Community 70|Community 70]]
- [[_COMMUNITY_Community 71|Community 71]]
- [[_COMMUNITY_Community 75|Community 75]]
- [[_COMMUNITY_Community 76|Community 76]]
- [[_COMMUNITY_Community 77|Community 77]]
- [[_COMMUNITY_Community 78|Community 78]]
- [[_COMMUNITY_Community 79|Community 79]]
- [[_COMMUNITY_Community 91|Community 91]]
- [[_COMMUNITY_Community 93|Community 93]]
- [[_COMMUNITY_Community 94|Community 94]]
- [[_COMMUNITY_Community 95|Community 95]]
- [[_COMMUNITY_Community 97|Community 97]]
- [[_COMMUNITY_Community 98|Community 98]]

## God Nodes (most connected - your core abstractions)
1. `join()` - 39 edges
2. `New()` - 30 edges
3. `Resource Types` - 27 edges
4. `writeFile()` - 22 edges
5. `healthyObjects()` - 20 edges
6. `newFakeClient()` - 19 edges
7. `NewScheme()` - 18 edges
8. `Load()` - 18 edges
9. `newFakeClient()` - 18 edges
10. `NodeCertificateCheckReconciler` - 17 edges

## Surprising Connections (you probably didn't know these)
- `Fathom CLI Entrypoint` --references--> `Taskfile.yml (task runner)`  [INFERRED]
  cmd/main.go → Taskfile.yml
- `Probe pod hardening defaults` --semantically_similar_to--> `Hardening profile defaults (non-root, drop caps, RO rootfs)`  [INFERRED] [semantically similar]
  README.md → docs/adr/0003-probe-pod-model.md
- `writeCert()` --calls--> `writeFile()`  [INFERRED]
  cmd/node-agent/main_test.go → internal/nodecert/scan_test.go
- `scanAndPublish()` --calls--> `Scan()`  [INFERRED]
  cmd/node-agent/main.go → internal/nodecert/scan.go
- `writeResult()` --calls--> `writeFile()`  [INFERRED]
  cmd/probe/main.go → internal/nodecert/scan_test.go

## Hyperedges (group relationships)
- **Worst-case severity aggregation pipeline** — healthreport_types_Severity, addoncheck_controller_aggregateHealthReportResult, clusterhealth_controller_aggregate [INFERRED 0.90]
- **Status mirror chain: AddonCheck -> HealthCheck -> ClusterHealth** — addoncheck_controller_AddonCheckReconciler, healthcheck_controller_HealthCheckReconciler, clusterhealth_controller_ClusterHealthReconciler [INFERRED 0.90]
- **DNS probe dispatch: coredns adapter -> launcher -> pod** — coredns_adapter_checkDNSResolution, coredns_adapter_runDNSProbe, probe_launcher_Run [EXTRACTED 1.00]
- **AddonCheck -> HealthCheck -> ClusterHealth aggregation chain** — crd_addoncheck, crd_healthcheck, crd_clusterhealth, crd_healthreport [EXTRACTED 0.90]
- **Prometheus ServiceMonitor opt-in overlay path** — kustomize_default, kustomize_default_prometheus_component, kustomize_prometheus_component, prometheus_servicemonitor, prometheus_tls_patch [EXTRACTED 0.90]
- **Probe pod model: ADR, hardening, RBAC, sample usage** — adr_0003_decision_single_shot_pod, adr_0003_hardening_profile, rbac_pods_verbs, sample_addoncheck_coredns, readme_probe_pods [EXTRACTED 0.85]

## Communities (99 total, 14 thin omitted)

### Community 0 - "Community 0"
Cohesion: 0.05
Nodes (27): New(), AddonCheck, AddonCheckFamilyPolicy, AddonCheckList, AddonCheckSpec, AddonCheckStatus, CheckTargetRef, ClusterHealth (+19 more)

### Community 1 - "Community 1"
Cohesion: 0.05
Nodes (82): flagBinding, MetricsOptions, Options, bindings(), Load(), RegisterFlags(), newTestFlags(), TestDefaultOptions_MatchFlagDefaults() (+74 more)

### Community 2 - "Community 2"
Cohesion: 0.07
Nodes (41): Adapter, FamilyOutcome(), TestFamilyOutcome(), TestOutcomeValid(), Capabilities, CheckResult, Family, FamilyPolicy (+33 more)

### Community 3 - "Community 3"
Cohesion: 0.06
Nodes (44): appFakeAdapter, DefaultOptions(), TestValidate(), TestValidate_MultipleErrorsAccumulate(), adapterName(), BuildAdapterRegistry(), BuildManagerOptions(), builtInAdapters() (+36 more)

### Community 4 - "Community 4"
Cohesion: 0.09
Nodes (36): Adapter, certificateCheck(), certificateDetails(), certManagerComponents(), check(), conditionDetails(), conditionStatus(), conditionType() (+28 more)

### Community 5 - "Community 5"
Cohesion: 0.07
Nodes (23): addonAdapterLookup, addonCheckDueForRun(), addonCheckInterval(), addonCheckPolicy(), addonCheckTargetRef(), addonCheckTimeout(), aggregateHealthReportResult(), copyStringMap() (+15 more)

### Community 6 - "Community 6"
Cohesion: 0.09
Nodes (31): Established(), PreferredServedVersion(), crd(), crdWithServed(), TestEstablished(), TestPreferredServedVersion(), TestPreferredServedVersion_IgnoresUnservedEntries(), Adapter (+23 more)

### Community 7 - "Community 7"
Cohesion: 0.06
Nodes (42): ADR-0001: In-process adapter contract, ContractVersion handshake at boot, Decision: in-process Go interface (option A), Option B: out-of-process gRPC plugins (rejected), Option C: OCI bundle adapters as Pods (rejected), Option D: Go plugin .so (rejected, fragile), Rationale: single OLM bundle, no sidecar, ADR-0002: HealthReport as first-class CRD (+34 more)

### Community 8 - "Community 8"
Cohesion: 0.1
Nodes (32): runManaged(), TestCondition_ClusterScopedListsWithoutNamespace(), TestCondition_ConditionStatus(), TestCondition_InvalidAPIVersionErrors(), TestCondition_InvalidSelectorErrors(), TestCondition_ListNameFallsBackToKind(), TestCondition_NoMatchingObjectsSkipped(), TestCondition_ScoreObject() (+24 more)

### Community 9 - "Community 9"
Cohesion: 0.14
Nodes (38): assertFamily(), assertHasDetail(), assertHasOutcome(), assertNoKind(), certManagerResource(), establishedCRD(), healthyDeployment(), healthyObjects() (+30 more)

### Community 10 - "Community 10"
Cohesion: 0.07
Nodes (16): AddonDefinition, conditionStatus(), ConditionCheck, CRDCheck, EvalContext, Evaluator, absenceOutcome(), derefReplicas() (+8 more)

### Community 11 - "Community 11"
Cohesion: 0.11
Nodes (32): Launcher, extractResult(), hasTerminationMessage(), dnsRequest(), newFakeClient(), simulateKubelet(), TestLauncherRun_ConcurrentRunsAreIndependent(), TestLauncherRun_DeletesPodAfterRun() (+24 more)

### Community 12 - "Community 12"
Cohesion: 0.09
Nodes (28): writeNodeReport(), config, main(), metricsMux(), parseConfig(), publishGauges(), run(), sanitizeLabelValue() (+20 more)

### Community 13 - "Community 13"
Cohesion: 0.13
Nodes (15): agentLabels(), agentResourceName(), aggregateNodeReports(), healthReportForNodeCert(), joinPaths(), mergeLabels(), nodeCertInterval(), nodeCertTimeout() (+7 more)

### Community 14 - "Community 14"
Cohesion: 0.2
Nodes (31): adapterWithLauncher(), assertHasDetail(), assertHasOutcome(), dnsEndpointSlice(), dnsEndpointSliceNamed(), dnsService(), dnsServiceNamed(), healthyDeployment() (+23 more)

### Community 15 - "Community 15"
Cohesion: 0.17
Nodes (32): assertFamily(), assertHasDetail(), assertHasOutcome(), assertNoOutcome(), ciliumCRD(), daemonSetInNamespace(), deploymentInNamespace(), establishedCiliumCRD() (+24 more)

### Community 16 - "Community 16"
Cohesion: 0.12
Nodes (20): Adapter, adapterOutcome(), check(), deploymentAvailable(), dnsProbePodName(), dnsTargets(), endAdapterRunSpan(), familyForTarget() (+12 more)

### Community 17 - "Community 17"
Cohesion: 0.06
Nodes (30): AddonCheck, AddonCheckFamilyPolicy, AddonCheckList, AddonCheckSpec, AddonCheckStatus, API Reference, CheckTargetRef, ClusterHealth (+22 more)

### Community 18 - "Community 18"
Cohesion: 0.12
Nodes (17): version, EnsureCompatible(), parseVersion(), TestContractVersionParses(), TestEnsureCompatible(), TestParseVersion(), fakeAdapter, Registry (+9 more)

### Community 19 - "Community 19"
Cohesion: 0.08
Nodes (28): AddonCheck Example, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (go -C tools tool task probe-build), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+20 more)

### Community 20 - "Community 20"
Cohesion: 0.18
Nodes (23): assertHasDetail(), assertHasOutcome(), assertNoCheckFor(), establishedCRD(), establishedCRDWithVersions(), externalSecret(), externalSecretWithVersion(), healthyDeployment() (+15 more)

### Community 21 - "Community 21"
Cohesion: 0.15
Nodes (13): Adapter, check(), deploymentAvailable(), desiredReplicas(), endAdapterRunSpan(), familyPolicy(), int32Threshold(), maxRestartCount() (+5 more)

### Community 22 - "Community 22"
Cohesion: 0.15
Nodes (23): main(), run(), runDNS(), runTCPConnect(), runTCPListen(), claimAndReleasePort(), TestJoin(), TestRunDispatchesToDNS() (+15 more)

### Community 23 - "Community 23"
Cohesion: 0.08
Nodes (24): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+16 more)

### Community 24 - "Community 24"
Cohesion: 0.08
Nodes (24): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Configuration Model, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines (+16 more)

### Community 25 - "Community 25"
Cohesion: 0.14
Nodes (17): NewRootCommand(), signalAwareContext(), signalContext(), TestSignalContext_PropagatesParentCancellation(), TestSignalContext_SIGINTCancels(), TestSignalContext_SIGTERMCancels(), TestSignalContext_StopReleasesContext(), TestNewRootCommand_BasicWiring() (+9 more)

### Community 26 - "Community 26"
Cohesion: 0.09
Nodes (21): AddonCheckReconciler, Built-in adapters, ClusterHealthReconciler, code:block1 (Pass(1) < Skipped(2) < Warn(3) < Unknown(4) < Fail(5) < Erro), code:block2 (adapter run                 mirror                   aggrega), code:block3 (+-----------------------------+), Fathom Architecture, HealthCheckReconciler (+13 more)

### Community 27 - "Community 27"
Cohesion: 0.1
Nodes (20): 0. Decisions that shape this plan, 1.1 Shipped, 1.2 Corrections to the record (from the v1 review), 1. Where we are today (and what v1 got wrong), 2.1 Declarative-first adapter (Decision 2), 2.2 Execution model (Decision 1), 2.3 Absence semantics (Decision 3), 2.4 Per-addon least-privilege client (Decision 4) (+12 more)

### Community 28 - "Community 28"
Cohesion: 0.1
Nodes (21): Kustomization: config/manager, Args: --leader-elect --health-probe-bind-address=:8081, Container: manager, ContainerSecurityContext: drop ALL caps, no priv escalation, Deployment: controller-manager, Image: controller:latest, Liveness probe: /healthz:8081, Namespace: system (+13 more)

### Community 29 - "Community 29"
Cohesion: 0.1
Nodes (19): A minimal check, Cadence, pausing, and history, Choosing which nodes run the agent, code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:sh (kubectl apply -f node-certificates.yaml), code:block3 (NAME                RESULT   REPORTING   DESIRED   AGE), code:block4 (/etc/kubernetes/pki              # apiserver, kubelet-client), code:yaml (spec:) (+11 more)

### Community 30 - "Community 30"
Cohesion: 0.1
Nodes (19): Adapter catalog, Add-on Checks, cert-manager, Cilium, code:yaml (apiVersion: fathom.skaphos.io/v1alpha1), code:yaml (spec:), code:yaml (spec:), code:yaml (spec:) (+11 more)

### Community 31 - "Community 31"
Cohesion: 0.31
Nodes (18): deepCopyContract(), fullyPopulatedAddonCheck(), fullyPopulatedClusterHealth(), fullyPopulatedHealthCheck(), fullyPopulatedHealthReport(), fullyPopulatedNodeCertificateCheck(), runtimeObjectContract(), TestDeepCopy_AddonCheck() (+10 more)

### Community 32 - "Community 32"
Cohesion: 0.11
Nodes (18): 1. Install the operator, 2. Declare your first check, 3. Read the result, 4. Roll checks up into one verdict, code:sh (helm install fathom oci://ghcr.io/skaphos/charts/fathom-oper), code:block10 (AddonCheck ──runs──▶ status + HealthReport (history)), code:sh (kubectl -n fathom-system get deploy,pod), code:yaml (apiVersion: fathom.skaphos.io/v1alpha1) (+10 more)

### Community 33 - "Community 33"
Cohesion: 0.11
Nodes (18): 1. Read status with `kubectl`, 2. Scrape Prometheus metrics, 3. Tracing, 4. Alerting patterns, 5. Deployment gates, Add-on check results — read the status, not a metric, Certificate expiry (the clean case), code:sh (# One verdict for the namespace:) (+10 more)

### Community 34 - "Community 34"
Cohesion: 0.14
Nodes (5): newFakeClientWithErrors(), TestRun_CRDGetErrorReportsError(), TestRun_DaemonSetGetErrorReportsError(), TestRun_DeploymentGetErrorReportsError(), TestRun_PodListErrorReportsError()

### Community 35 - "Community 35"
Cohesion: 0.12
Nodes (6): checkResult, corednsCheck, corednsHealthReport, eventList, healthReport, healthReportList

### Community 36 - "Community 36"
Cohesion: 0.13
Nodes (15): 1. Land Releasable Commits on `main`, 2. Run Local Release Checks, 3. Review and Merge the Release PR, 4. Tag-Triggered Publish, 5. Verify the Release, code:bash (operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bund), code:yaml (components:), code:bash (IMG=ghcr.io/skaphos/fathom-operator@sha256:<digest> \) (+7 more)

### Community 37 - "Community 37"
Cohesion: 0.13
Nodes (14): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Safety Expectations, Testing, Branching and Commits (+6 more)

### Community 38 - "Community 38"
Cohesion: 0.14
Nodes (13): 1. Land Releasable Commits on `main`, 2. Run Local Release Checks, 3. Review and Merge the Release PR, 4. Tag-Triggered Publish, 5. Verify the Release, code:bash (operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bund), code:bash (IMG=ghcr.io/skaphos/fathom-operator@sha256:<digest> \), Image Pinning and Deploy-by-Digest Contract (+5 more)

### Community 39 - "Community 39"
Cohesion: 0.14
Nodes (13): Adding a New Option, code:block1 (command-line flag  >  environment variable  >  config file  ), code:block2 (FATHOM_<VIPER_KEY with "." replaced by "_", upper-cased>), code:yaml (metrics:), code:block4 (per-AddonCheck probeImage threshold  >  --probe-image (Reque), Config File, Configuration Reference, Environment Variables (+5 more)

### Community 40 - "Community 40"
Cohesion: 0.14
Nodes (13): 3. Probe-pod model for active in-cluster network checks, A. In-process net code from the operator pod, B. Sidecar container in the operator Deployment, C. DaemonSet probe agent on every node, Consequences, Considered Options, Context and Problem Statement, D. Single-shot probe Pod per check (+5 more)

### Community 41 - "Community 41"
Cohesion: 0.15
Nodes (12): `api/v1alpha1/` — CRD types, Build and codegen, `cmd/` — entrypoints, Code Map, `config/` — manifests and packaging, `internal/adapter/` — registry and built-in adapters, `internal/app/` — process plumbing, `internal/controller/` — reconcilers (+4 more)

### Community 42 - "Community 42"
Cohesion: 0.15
Nodes (12): 1. In-process Go interface as the AddonAdapter contract, A. In-process Go interface, B. Out-of-process gRPC plugins, C. OCI bundle adapters launched as Pods per run, Consequences, Considered Options, Context and Problem Statement, D. Go `plugin` package (+4 more)

### Community 43 - "Community 43"
Cohesion: 0.15
Nodes (12): 2. HealthReport as a first-class custom resource, A. Status conditions on the source check only, B. HealthReport as a first-class CRD, C. External time-series store, Consequences, Considered Options, Context and Problem Statement, D. Kubernetes Events (+4 more)

### Community 44 - "Community 44"
Cohesion: 0.18
Nodes (13): AddonCheckReconciler, AddonCheckReconciler.Reconcile, addonAdapterLookup interface, aggregateHealthReportResult, healthReportForAddonCheck, healthReportResult (outcome mapping), labels fathom.skaphos.io/source-{kind,name}, AddonCheckReconciler.pruneHealthReportHistory (+5 more)

### Community 45 - "Community 45"
Cohesion: 0.17
Nodes (11): Check families, code:block1 (Pass < Skipped < Warn < Unknown < Fail < Error), code:block2 (runs                       mirror                      aggre), Concepts, How status flows, Next steps, Results and severity, The resource kinds (+3 more)

### Community 46 - "Community 46"
Cohesion: 0.17
Nodes (11): 4. HealthCheck as a thin wrapper over a specialized check, A. HealthCheck as a thin wrapper, B. Discriminator-typed unified CRD, C. Delete HealthCheck and ClusterHealth, Consequences, Considered Options, Context and Problem Statement, Decision Drivers (+3 more)

### Community 47 - "Community 47"
Cohesion: 0.17
Nodes (11): code:sh (# 1. Create the kind cluster.), code:sh (# Status conditions live on the AddonCheck.), code:sh (kubectl get pods -l app.kubernetes.io/managed-by=fathom -A -), code:sh (go -C tools tool task probe-docker-build PROBE_IMG=ghcr.io/s), Fathom End-to-End Fixtures, Inspecting AddonCheck Results, Layout, Prerequisites (+3 more)

### Community 48 - "Community 48"
Cohesion: 0.2
Nodes (12): Image: quay.io/operator-framework/scorecard-test:v1.42.2, Kustomization: config/manifests, Kustomization: config/scorecard, Scorecard base Configuration, Scorecard basic patch, Scorecard OLM patch, Scorecard test: basic-check-spec, Scorecard test: olm-bundle-validation (+4 more)

### Community 49 - "Community 49"
Cohesion: 0.18
Nodes (12): coredns.Adapter, coredns.Adapter.Run, coredns.Adapter.checkDNSResolution, coredns.Adapter.checkSystemHealth, dnsProbeLauncher interface, dnsProbePodName, Rationale: probe pod for workload-perspective DNS (ADR-0003), runDNSProbe (+4 more)

### Community 50 - "Community 50"
Cohesion: 0.21
Nodes (12): config/default kustomization, manager_metrics_patch (8443 HTTPS), Prometheus component opt-in, config/components/prometheus, controller-manager-metrics-monitor ServiceMonitor, ServiceMonitor cert-manager TLS patch, Default deployment topology, GHCR operator/bundle/catalog images (+4 more)

### Community 51 - "Community 51"
Cohesion: 0.29
Nodes (11): API Group: fathom.skaphos.io, ClusterRole: clusterhealth-admin-role, ClusterRole: healthcheck-admin-role, ClusterRole: healthcheck-editor-role, ClusterRole: healthcheck-viewer-role, ClusterRole: healthreport-admin-role, ClusterRole: healthreport-editor-role, ClusterRole: healthreport-viewer-role (+3 more)

### Community 52 - "Community 52"
Cohesion: 0.22
Nodes (11): AddonCheck CRD type, AddonCheckFamilyPolicy, AddonCheckSpec, AddonCheckStatus, HealthCheckReconciler.Reconcile, HealthCheckReconciler.mirrorTarget, summarizeFromConditions, CheckTargetRef (+3 more)

### Community 53 - "Community 53"
Cohesion: 0.2
Nodes (10): code:bash (IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z), code:bash (cosign verify \), code:bash (gh attestation verify "oci://${IMAGE}" --owner skaphos), code:bash (cosign verify-attestation \), code:bash (gh release download "vX.Y.Z" --repo skaphos/fathom --pattern), code:bash (syft "${IMAGE}" -o spdx-json), Inspect the SBOM, Supply-Chain Verification (+2 more)

### Community 54 - "Community 54"
Cohesion: 0.22
Nodes (10): ClusterHealthReconciler, ClusterHealthReconciler.Reconcile, ClusterHealthReconciler.SetupWithManager, ClusterHealthReconciler.aggregate, clusterHealthsForHealthCheck (watch map), Invariant: ClusterHealth never reads HealthReport (ADR-0004), selectorFromSpec, ClusterHealth Controller envtest suite (+2 more)

### Community 55 - "Community 55"
Cohesion: 0.22
Nodes (8): [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), Bug Fixes, Changelog, Features, [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17), Bug Fixes, Changelog, Features

### Community 56 - "Community 56"
Cohesion: 0.22
Nodes (9): code:bash (cosign verify \), code:bash (gh attestation verify "oci://${IMAGE}" --owner skaphos), code:bash (cosign verify-attestation \), code:bash (gh release download "vX.Y.Z" --repo skaphos/fathom --pattern), code:bash (syft "${IMAGE}" -o spdx-json), Inspect the SBOM, Supply-Chain Verification, Verify build provenance (+1 more)

### Community 57 - "Community 57"
Cohesion: 0.22
Nodes (8): ClusterHealthChildSummary, ClusterHealthStatus, TestDeepCopyRoundTrip, HealthReportResult enum, HealthReportResult.Severity(), TestHealthReportResultSeverity_EmptyAndUnrecognizedReturnZero, TestHealthReportResultSeverity_OrderingAcrossEnumValues, TestHealthReportResultSeverity_PassIsLowestNonZero

### Community 58 - "Community 58"
Cohesion: 0.25
Nodes (7): Architecture Decision Records, Contents, Design & planning, Fathom Documentation, Guides — for platform teams, Other repository docs, Reference & internals

### Community 60 - "Community 60"
Cohesion: 0.33
Nodes (6): Launcher.Run, Launcher.bestEffortDelete, extractResult, Rationale: pollHeadroom outlasts probe ActiveDeadline, simulateKubelet test helper, Launcher.waitForCompletion

### Community 61 - "Community 61"
Cohesion: 0.4
Nodes (4): Conventions used in these guides, Fathom User Guides, How to use Fathom, Start here

### Community 62 - "Community 62"
Cohesion: 0.5
Nodes (5): ClusterRole clusterhealth-editor-role, ClusterRole clusterhealth-viewer-role, ServiceAccount controller-manager, ClusterRoleBinding manager-rolebinding, config/rbac/kustomization.yaml

### Community 63 - "Community 63"
Cohesion: 0.4
Nodes (5): HealthCheckReconciler, HealthCheckReconciler.SetupWithManager, healthChecksForAddonCheck (watch map), Rationale: HealthCheck as wrapper (ADR-0004), HealthCheck Controller envtest suite

### Community 64 - "Community 64"
Cohesion: 0.4
Nodes (5): TestAddToScheme, HealthReport CRD type, HealthReportCheck, HealthReportSpec, HealthReportTargetRef

### Community 66 - "Community 66"
Cohesion: 0.83
Nodes (3): restoreGlobalProvider(), TestInit_DisabledInstallsNoopProvider(), TestInit_EnabledInstallsRecordingProvider()

### Community 68 - "Community 68"
Cohesion: 0.5
Nodes (4): app.NewRootCommand (cobra root), signalAwareContext, Taskfile.yml (task runner), Fathom CLI Entrypoint

### Community 69 - "Community 69"
Cohesion: 0.5
Nodes (3): NetworkPolicy allow-metrics-traffic, Service controller-manager-metrics-service, config/network-policy/kustomization.yaml

### Community 71 - "Community 71"
Cohesion: 0.67
Nodes (3): code:bash (IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z), code:yaml (components:), Default Deployment Topology

## Knowledge Gaps
- **406 isolated node(s):** `config`, `version`, `Adapter`, `Capabilities`, `Request` (+401 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **14 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewScheme()` connect `Community 3` to `Community 2`, `Community 9`, `Community 11`, `Community 14`, `Community 15`, `Community 20`?**
  _High betweenness centrality (0.133) - this node is a cross-community bridge._
- **Why does `join()` connect `Community 1` to `Community 3`, `Community 4`, `Community 6`, `Community 10`, `Community 12`, `Community 13`, `Community 20`, `Community 21`, `Community 22`, `Community 25`?**
  _High betweenness centrality (0.120) - this node is a cross-community bridge._
- **Why does `New()` connect `Community 0` to `Community 16`, `Community 11`, `Community 14`?**
  _High betweenness centrality (0.080) - this node is a cross-community bridge._
- **Are the 37 inferred relationships involving `join()` (e.g. with `normalizeShell()` and `runMain()`) actually correct?**
  _`join()` has 37 INFERRED edges - model-reasoned connections that need verification._
- **Are the 29 inferred relationships involving `New()` (e.g. with `.DeepCopy()` and `.DeepCopyInto()`) actually correct?**
  _`New()` has 29 INFERRED edges - model-reasoned connections that need verification._
- **Are the 15 inferred relationships involving `writeFile()` (e.g. with `writeCert()` and `writeResult()`) actually correct?**
  _`writeFile()` has 15 INFERRED edges - model-reasoned connections that need verification._
- **What connects `config`, `version`, `Adapter` to the rest of the system?**
  _406 weakly-connected nodes found - possible documentation gaps or missing edges._