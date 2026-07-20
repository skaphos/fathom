/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// MetricsServerDefinition is the declarative metrics-server adapter (SKA-65).
// system_health verifies the metrics-server Deployment and its pods;
// api_availability verifies the aggregated v1beta1.metrics.k8s.io APIService
// reports Available=True — the check that actually distinguishes "pods are
// running" from "the resource-metrics API works" (an unreachable or
// TLS-misconfigured metrics-server keeps its pods Ready while the APIService
// goes Unavailable, taking `kubectl top` and HPA scaling with it).
//
// Everything is Required: a cluster that declares a metrics-server AddonCheck
// expects the metrics API, so absence — including a missing APIService object —
// is Fail with the absent marker (SKA-526).
var MetricsServerDefinition = AddonDefinition{
	AddonType:      "metrics-server",
	AdapterVersion: "0.1.0",
	// Detect the installed metrics-server version off the Deployment (its
	// app.kubernetes.io/version label, else the registry.k8s.io/metrics-server
	// image tag). Detection-only: SupportedVersions is left empty (the exact
	// supported range is maintainer-owned), so metrics-server never Warns on a
	// version — gating is opt-in (SKA-527).
	VersionSource: &VersionSource{FromFamily: adapter.Family("system_health")},
	RBAC: []adapter.PolicyRule{
		// Verbs mirror the engine's actual reads through the direct (uncached)
		// impersonating client: the Deployment and the APIService are fetched by
		// name (get), pods by label selector (list). Nothing watches — the
		// client is deliberately cache-free (see internal/adapter/impersonation)
		// — so no list/watch is granted where the engine never lists or watches.
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"},
			Justification: "Get the metrics-server Deployment by name to score readiness. The name/namespace are policy-overridable (Helm release fullname) but always resolve to a single named Get; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"},
			Justification: "List the metrics-server Pods by label selector for restart counts and readiness behind the Deployment. list (not get) because Pod names are dynamic; read-only."},
		{APIGroups: []string{"apiregistration.k8s.io"}, Resources: []string{"apiservices"}, Verbs: []string{"get"},
			Justification: "Get the v1beta1.metrics.k8s.io APIService by name to score aggregation availability. get only — the check fetches exactly one named APIService, so list/watch would be broader than the read the evaluator performs."},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("system_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDeployment,
				DefaultNamespace:        "kube-system",
				NameThresholdKey:        "deploymentName",
				DefaultName:             "metrics-server",
				Component:               "metrics-server",
				Absence:                 Required,
				CheckPods:               true,
				RestartWarnThresholdKey: "restartWarnCount",
				DefaultRestartWarn:      3,
			}},
		},
		{
			Name:           adapter.Family("api_availability"),
			DefaultEnabled: true,
			APIServices: []ConditionCheck{{
				APIVersion:     "apiregistration.k8s.io/v1",
				Kind:           "APIService",
				ListKind:       "APIServiceList",
				ListName:       "apiservices",
				ClusterScoped:  true,
				Names:          []string{"v1beta1.metrics.k8s.io"},
				Absence:        Required,
				ConditionType:  "Available",
				ExpectedStatus: "True",
			}},
		},
	},
}

// NewMetricsServerEngine returns the declarative metrics-server adapter. RBAC
// markers live on the package doc in definition.go.
func NewMetricsServerEngine() *Engine {
	return MustEngine(MetricsServerDefinition)
}
