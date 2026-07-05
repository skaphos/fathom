/*
SPDX-FileCopyrightText: 2026 Skaphos
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
	VersionSource: &VersionSource{Kind: KindDeployment, Namespace: "envoy-gateway-system", Name: "envoy-gateway"},
	RBAC: []adapter.PolicyRule{
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read the envoy-gateway controller Deployment to score readiness. list+watch because the name/namespace are policy-overridable (Helm release convention); read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read the envoy-gateway Pods for restart counts and readiness behind the Deployment. list is required because Pod names are dynamic; read-only."},
		{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read the Gateway API core CRDs and Envoy Gateway's EnvoyProxy CRD to verify they are Established and serve a supported version. list is needed to check several CRDs; read-only."},
		{APIGroups: []string{"gateway.networking.k8s.io"}, Resources: []string{"gateways"}, Verbs: []string{"get", "list", "watch"},
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
