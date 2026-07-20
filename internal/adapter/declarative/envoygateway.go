/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// EnvoyGatewayDefinition is the declarative Envoy Gateway adapter (SKA-507).
// system_health verifies the envoy-gateway controller Deployment and its pods;
// crd_health verifies the Gateway API core CRDs plus Envoy Gateway's own
// EnvoyProxy CRD; gateway_status scores every Gateway's Accepted and
// Programmed conditions across the policy namespaces (a cluster with no
// Gateway objects yet stays quiet — Skipped, NoMatchingObjects).
//
// Deferred, pending evaluators:
//   - The provisioned per-Gateway proxy Deployments are dynamically named
//     (envoy-<namespace>-<gateway>-<hash>), which WorkloadCheck's named-
//     singleton shape can't address; they need a label-selector workload
//     evaluator. Until then proxy fleet health is observed indirectly through
//     Gateway Programmed.
//   - HTTPRoute health: route conditions live under status.parents[] (one
//     condition set per parent Gateway), not top-level status.conditions, so
//     ConditionCheck can't express them yet.
var EnvoyGatewayDefinition = AddonDefinition{
	AddonType:      "envoy-gateway",
	AdapterVersion: "0.1.0",
	// Detect the installed Envoy Gateway version off the controller Deployment
	// (its app.kubernetes.io/version label, else the envoyproxy/gateway image
	// tag). Detection-only: SupportedVersions is left empty so Envoy Gateway
	// never Warns on a version — gating is opt-in (SKA-527).
	VersionSource: &VersionSource{FromFamily: adapter.Family("system_health")},
	RBAC: []adapter.PolicyRule{
		// Verbs mirror the engine's actual reads through the direct (uncached)
		// impersonating client: the Deployment and each CRD are fetched by name
		// (get), Pods and Gateways by list. Nothing watches — the client is
		// deliberately cache-free (see internal/adapter/impersonation) — so no
		// watch is granted where the engine never watches.
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"},
			Justification: "Get the envoy-gateway controller Deployment by name to score readiness. The name/namespace are policy-overridable (the chart hardcodes envoy-gateway, but repackaged installs may rename it) but always resolve to a single named Get; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"},
			Justification: "List the envoy-gateway Pods by label selector for restart counts and readiness behind the Deployment. list (not get) because Pod names are dynamic; read-only."},
		{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get"},
			Justification: "Get the Gateway API core CRDs and Envoy Gateway's EnvoyProxy CRD by name to verify they are Established and serve a supported version. get only — each CRD is fetched individually by name, never listed; read-only."},
		{APIGroups: []string{"gateway.networking.k8s.io"}, Resources: []string{"gateways"}, Verbs: []string{"list"},
			Justification: "List Gateway objects to score their Accepted and Programmed conditions. Deliberately only Gateways — not HTTPRoutes or the gateway.envoyproxy.io policy kinds, which no evaluator reads; read-only."},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("system_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDeployment,
				DefaultNamespace:        "envoy-gateway-system",
				NameThresholdKey:        "deploymentName",
				DefaultName:             "envoy-gateway",
				Component:               "envoy-gateway",
				Absence:                 Required,
				CheckPods:               true,
				RestartWarnThresholdKey: "restartWarnCount",
				DefaultRestartWarn:      3,
			}},
		},
		{
			Name:           adapter.Family("crd_health"),
			DefaultEnabled: true,
			CRDs: []CRDCheck{
				{
					// Gateway API core kinds Envoy Gateway programs. v1 is the
					// standard-channel storage version; v1beta1 remains served on
					// older Gateway API installs.
					Names: []string{
						"gatewayclasses.gateway.networking.k8s.io",
						"gateways.gateway.networking.k8s.io",
						"httproutes.gateway.networking.k8s.io",
					},
					SupportedVersions:         []string{"v1", "v1beta1"},
					Absence:                   Required,
					UnsupportedVersionOutcome: adapter.OutcomeWarn,
				},
				{
					// Envoy Gateway's own configuration CRD, bundled with the
					// gateway-helm chart.
					Names:                     []string{"envoyproxies.gateway.envoyproxy.io"},
					SupportedVersions:         []string{"v1alpha1"},
					Absence:                   Required,
					UnsupportedVersionOutcome: adapter.OutcomeWarn,
				},
			},
		},
		{
			Name:           adapter.Family("gateway_status"),
			DefaultEnabled: true,
			ManagedResources: []ConditionCheck{
				{
					APIVersion:        "gateway.networking.k8s.io/v1",
					VersionCRD:        "gateways.gateway.networking.k8s.io",
					SupportedVersions: []string{"v1", "v1beta1"},
					Kind:              "Gateway",
					ListKind:          "GatewayList",
					ListName:          "gateways",
					DefaultNamespace:  "envoy-gateway-system",
					ConditionType:     "Accepted",
					ExpectedStatus:    "True",
				},
				{
					APIVersion:        "gateway.networking.k8s.io/v1",
					VersionCRD:        "gateways.gateway.networking.k8s.io",
					SupportedVersions: []string{"v1", "v1beta1"},
					Kind:              "Gateway",
					ListKind:          "GatewayList",
					ListName:          "gateways",
					DefaultNamespace:  "envoy-gateway-system",
					ConditionType:     "Programmed",
					ExpectedStatus:    "True",
				},
			},
		},
	},
}

// NewEnvoyGatewayEngine returns the declarative Envoy Gateway adapter. RBAC
// markers live on the package doc in definition.go.
func NewEnvoyGatewayEngine() *Engine {
	return MustEngine(EnvoyGatewayDefinition)
}
