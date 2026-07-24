/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// ArgoCDDefinition is the declarative Argo CD adapter (skaphos/fathom#191).
//
//   - system_health verifies the four control-plane workloads of a default
//     install in the argocd namespace — the argocd-application-controller
//     StatefulSet plus the argocd-repo-server, argocd-server, and argocd-redis
//     Deployments — and that the three core CRDs (Application, ApplicationSet,
//     AppProject) are Established and serve v1alpha1.
//   - sync_health scores every Application's status.sync.status and
//     status.health.status (Argo CD reports them as nested scalar fields, not
//     status.conditions — hence FieldCheck): Synced/Healthy -> Pass;
//     Degraded and Missing -> Fail (the app's workloads are broken or gone);
//     OutOfSync, Progressing, Suspended, and Unknown -> Warn (drift, a rollout
//     in flight, a deliberate pause, or an indeterminate state — surfaced, not
//     failed). A cluster with no Applications is Skipped, quiet by design.
//
// Strictly read-only: the adapter only lists Applications through the
// Kubernetes API and never annotates, syncs, refreshes, or touches the Argo CD
// API server, so it can never trigger reconciliation work. Aggregation is the
// engine's usual worst-of roll-up; per-family quorum semantics is separate,
// unimplemented work (skaphos/fathom#159).
//
// Names assume a default install (official manifests, or the argo-cd chart
// with its default nameOverride) in the argocd namespace. A renamed install is
// reachable through policy: namespaces redirects the workloads and the
// per-component name thresholds override the renamed objects. Not covered: the
// HA variant's redis-ha topology (argocd-redis-ha-haproxy Deployment plus a
// redis-ha StatefulSet) replaces the single argocd-redis Deployment with
// differently-kinded workloads, which a name override cannot express — point
// redisName at argocd-redis-ha-haproxy to cover the proxy Deployment. Also not
// checked: the optional dex, applicationset, and notifications controllers,
// which not every install runs.
var ArgoCDDefinition = AddonDefinition{
	AddonType:      "argocd",
	AdapterVersion: "0.1.0",
	// Detect the installed Argo CD version off the application-controller
	// StatefulSet (its app.kubernetes.io/version label, else the
	// quay.io/argoproj/argocd image tag). Detection-only: SupportedVersions is
	// left empty so Argo CD never Warns on a version — gating is opt-in
	// (SKA-527).
	VersionSource: &VersionSource{FromFamily: adapter.Family("system_health"), FromComponent: "argocd-application-controller"},
	RBAC: []adapter.PolicyRule{
		// Verbs mirror the engine's actual reads through the direct (uncached)
		// impersonating client: named Gets for the workloads and CRDs, label
		// lists for pods, and one Application list. Nothing watches, and nothing
		// mutates — this adapter must never be able to trigger a sync or refresh.
		{APIGroups: []string{"apps"}, Resources: []string{"statefulsets"}, Verbs: []string{"get"},
			Justification: "Get the argocd-application-controller StatefulSet by name to score control-plane readiness and detect the installed version. The name/namespace are policy-overridable but always resolve to a single named Get; read-only."},
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"},
			Justification: "Get the argocd-repo-server, argocd-server, and argocd-redis Deployments by name to score control-plane readiness. get only — three named Gets; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"},
			Justification: "List the Argo CD control-plane Pods by label selector for restart counts and readiness behind their controllers. list (not get) because Pod names are dynamic; read-only."},
		{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get"},
			Justification: "Get the Application, ApplicationSet, and AppProject CRDs by name to verify they are Established and serve a supported version. get only — each CRD is fetched individually by name, never listed; read-only."},
		{APIGroups: []string{"argoproj.io"}, Resources: []string{"applications"}, Verbs: []string{"list"},
			Justification: "List Applications across the policy namespaces to score status.sync.status and status.health.status. list only, and scoped to Applications alone — deliberately NOT AppProjects or ApplicationSets, and no update/patch verb, so the adapter can never trigger a sync or refresh (both require writing to the Application or calling the Argo CD API); strictly read-only."},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("system_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{
				{
					Kind:                    KindStatefulSet,
					DefaultNamespace:        "argocd",
					NameThresholdKey:        "applicationControllerName",
					DefaultName:             "argocd-application-controller",
					Component:               "argocd-application-controller",
					Absence:                 Required,
					CheckPods:               true,
					RestartWarnThresholdKey: "restartWarnCount",
					DefaultRestartWarn:      3,
				},
				argocdDeployment("argocd-repo-server", "repoServerName"),
				argocdDeployment("argocd-server", "serverName"),
				argocdDeployment("argocd-redis", "redisName"),
			},
			CRDs: []CRDCheck{{
				Names: []string{
					"applications.argoproj.io",
					"applicationsets.argoproj.io",
					"appprojects.argoproj.io",
				},
				SupportedVersions:         []string{"v1alpha1"},
				Absence:                   Required,
				UnsupportedVersionOutcome: adapter.OutcomeWarn,
			}},
		},
		{
			Name:           adapter.Family("sync_health"),
			DefaultEnabled: true,
			Fields: []FieldCheck{
				{
					APIVersion:    "argoproj.io/v1alpha1",
					Kind:          "Application",
					ListKind:      "ApplicationList",
					ListName:      "applications-sync",
					FieldPath:     []string{"status", "sync", "status"},
					ExpectedValue: "Synced",
					// OutOfSync is drift, not breakage (auto-sync apps pass
					// through it on every deploy), and Unknown means Argo CD
					// itself cannot compare — both Warn. Anything else (a future
					// Argo CD state) falls to the default Warn.
					ValueOutcomes: map[string]adapter.Outcome{
						"OutOfSync": adapter.OutcomeWarn,
						"Unknown":   adapter.OutcomeWarn,
					},
				},
				{
					APIVersion:    "argoproj.io/v1alpha1",
					Kind:          "Application",
					ListKind:      "ApplicationList",
					ListName:      "applications-health",
					FieldPath:     []string{"status", "health", "status"},
					ExpectedValue: "Healthy",
					// Degraded (workloads failed) and Missing (resources absent
					// from the cluster) are hard Fails. Progressing (rollout in
					// flight), Suspended (deliberate pause), and Unknown are
					// surfaced as Warn, not failed.
					ValueOutcomes: map[string]adapter.Outcome{
						"Degraded":    adapter.OutcomeFail,
						"Missing":     adapter.OutcomeFail,
						"Progressing": adapter.OutcomeWarn,
						"Suspended":   adapter.OutcomeWarn,
						"Unknown":     adapter.OutcomeWarn,
					},
				},
			},
		},
	},
}

// argocdDeployment builds a Required Argo CD control-plane Deployment check
// (with pods, a policy-overridable name, and a restart-warn threshold) in the
// argocd namespace.
func argocdDeployment(name, nameKey string) WorkloadCheck {
	return WorkloadCheck{
		Kind:                    KindDeployment,
		DefaultNamespace:        "argocd",
		NameThresholdKey:        nameKey,
		DefaultName:             name,
		Component:               name,
		Absence:                 Required,
		CheckPods:               true,
		RestartWarnThresholdKey: "restartWarnCount",
		DefaultRestartWarn:      3,
	}
}

// NewArgoCDEngine returns the declarative Argo CD adapter. RBAC markers live on
// the package doc in definition.go.
func NewArgoCDEngine() *Engine {
	return MustEngine(ArgoCDDefinition)
}
