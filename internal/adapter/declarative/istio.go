/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// IstioDefinition is the declarative istio mesh adapter (SKA-60).
//
//   - system_health verifies the istiod control-plane Deployment and its pods,
//     plus the admission wiring istiod serves: the istio-sidecar-injector
//     MutatingWebhookConfiguration and the istio-validator-istio-system
//     ValidatingWebhookConfiguration, both of which must carry a populated
//     caBundle and point at the istiod Service. An unpopulated caBundle is
//     istio's classic silent failure: istiod has not patched (or cannot
//     patch) the bundle, so injection/validation requests fail TLS — or with
//     failurePolicy Ignore, pods are silently created uninjected.
//   - ztunnel_health verifies the ztunnel DaemonSet (ambient L4 node proxy).
//     Optional: a sidecar-mode mesh legitimately runs without it.
//   - istio_cni_health verifies the istio-cni-node DaemonSet. Optional: it is
//     required for ambient but opt-in for sidecar mode (where init-container
//     iptables is the default traffic-redirect mechanism).
//   - crd_health verifies the core networking.istio.io and security.istio.io
//     CRDs from the istio/base chart, per-group served versions (SKA-425).
//
// The plan-of-record (docs/design/addon-adapters-implementation-plan.md §4)
// originally routed istio through the Go escape hatch for "conditional
// topology + webhooks"; both gaps have since closed (per-check Optional
// absence, SKA-526, and the WebhookCheck evaluator shipped with this
// definition), so istio is declarative like the rest of Wave 2.
//
// Names assume the default revision in istio-system (istioctl / helm install
// without --revision): a revisioned or relocated control plane renames its
// Deployment and webhook configurations (istiod-<rev>,
// istio-sidecar-injector-<rev>, istio-validator-<rev>-<ns>). Point one
// AddonCheck per revision at the renamed objects via the deploymentName,
// injectorWebhookName, and validatorWebhookName thresholds; the expected
// backing-service namespace follows the family's policy namespace. One known
// gap: VersionSource is not threshold-aware (an engine-wide limitation shared
// with every declarative adapter), so a renamed istiod loses version
// detection — detection-only, never a wrong verdict. The base chart's
// istiod-default-validator is deliberately not checked: it exists only as a
// default-revision alias for the same istiod service the chart-owned
// validator already covers, so checking both would double-report one signal.
//
// Deferred, pending an active-probe evaluator: mesh_status
// (config-distribution / proxy-sync anomalies, PeerAuthentication mTLS-mode
// sanity). Proxy sync is observable only through istiod's XDS/debug
// endpoints or its Prometheus metrics — not the Kubernetes API — and
// PeerAuthentication carries no status conditions to score. When that probe
// lands, a thin Go adapter can compose this engine's passive families with
// it (the cert-manager passive+active split).
var IstioDefinition = AddonDefinition{
	AddonType:      "istio",
	AdapterVersion: "0.1.0",
	// Detect the installed istio version off the istiod Deployment (its
	// app.kubernetes.io/version label, else the istio/pilot image tag).
	// Detection-only: SupportedVersions is left empty so istio never Warns on
	// a version — gating is opt-in (SKA-527).
	VersionSource: &VersionSource{Kind: KindDeployment, Namespace: "istio-system", Name: "istiod"},
	RBAC: []adapter.PolicyRule{
		// Verbs mirror the engine's actual reads through the direct (uncached)
		// impersonating client: the istiod Deployment, the ztunnel/istio-cni
		// DaemonSets, each CRD, and both webhook configurations are fetched by
		// name (get); pods by label selector (list). Nothing watches — the
		// client is deliberately cache-free (see internal/adapter/impersonation)
		// — so no list/watch is granted where the engine never lists or watches.
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"},
			Justification: "Get the istiod Deployment by name to score control-plane readiness and detect the installed version. The name/namespace are policy-overridable (revisioned installs rename it istiod-<rev>) but always resolve to a single named Get; read-only."},
		{APIGroups: []string{"apps"}, Resources: []string{"daemonsets"}, Verbs: []string{"get"},
			Justification: "Get the ztunnel and istio-cni-node DaemonSets by name to score ambient data-plane readiness. Both are Optional (sidecar-mode meshes run without them) but always resolve to named Gets; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"},
			Justification: "List the istiod/ztunnel/istio-cni Pods by label selector for restart counts and readiness behind their controllers. list (not get) because Pod names are dynamic; read-only."},
		{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get"},
			Justification: "Get the core networking.istio.io and security.istio.io CRDs by name to verify they are Established and serve a supported version. get only — each CRD is fetched individually by name, never listed; read-only."},
		{APIGroups: []string{"admissionregistration.k8s.io"}, Resources: []string{"mutatingwebhookconfigurations", "validatingwebhookconfigurations"}, Verbs: []string{"get"},
			Justification: "Get the istio-sidecar-injector and istio-validator webhook configurations by name to verify istiod's admission wiring (caBundle populated, backed by the istiod Service). get only — exactly two named Gets; read-only."},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("system_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDeployment,
				DefaultNamespace:        "istio-system",
				NameThresholdKey:        "deploymentName",
				DefaultName:             "istiod",
				Component:               "istiod",
				Absence:                 Required,
				CheckPods:               true,
				RestartWarnThresholdKey: "restartWarnCount",
				DefaultRestartWarn:      3,
			}},
			Webhooks: []WebhookCheck{
				{
					Kind:             KindMutatingWebhookConfiguration,
					Name:             "istio-sidecar-injector",
					NameThresholdKey: "injectorWebhookName",
					ExpectedService:  "istiod",
					ServiceNamespace: "istio-system",
					Absence:          Required,
				},
				{
					Kind:             KindValidatingWebhookConfiguration,
					Name:             "istio-validator-istio-system",
					NameThresholdKey: "validatorWebhookName",
					ExpectedService:  "istiod",
					ServiceNamespace: "istio-system",
					Absence:          Required,
				},
			},
		},
		{
			Name:           adapter.Family("ztunnel_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDaemonSet,
				DefaultNamespace:        "istio-system",
				NameThresholdKey:        "daemonSetName",
				DefaultName:             "ztunnel",
				Component:               "ztunnel",
				Absence:                 Optional,
				CheckPods:               true,
				RestartWarnThresholdKey: "restartWarnCount",
				DefaultRestartWarn:      3,
			}},
		},
		{
			Name:           adapter.Family("istio_cni_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDaemonSet,
				DefaultNamespace:        "istio-system",
				NameThresholdKey:        "daemonSetName",
				DefaultName:             "istio-cni-node",
				Component:               "istio-cni",
				Absence:                 Optional,
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
					// Core traffic-management kinds istiod programs. v1 is the
					// storage version since istio 1.22; v1beta1/v1alpha3 remain
					// served for older clients.
					Names: []string{
						"virtualservices.networking.istio.io",
						"destinationrules.networking.istio.io",
						"gateways.networking.istio.io",
						"serviceentries.networking.istio.io",
					},
					SupportedVersions:         []string{"v1", "v1beta1", "v1alpha3"},
					Absence:                   Required,
					UnsupportedVersionOutcome: adapter.OutcomeWarn,
				},
				{
					// Core mesh-security kinds; security.istio.io never served
					// v1alpha3.
					Names: []string{
						"peerauthentications.security.istio.io",
						"authorizationpolicies.security.istio.io",
					},
					SupportedVersions:         []string{"v1", "v1beta1"},
					Absence:                   Required,
					UnsupportedVersionOutcome: adapter.OutcomeWarn,
				},
			},
		},
	},
}

// NewIstioEngine returns the declarative istio adapter. RBAC markers live on
// the package doc in definition.go.
func NewIstioEngine() *Engine {
	return MustEngine(IstioDefinition)
}
