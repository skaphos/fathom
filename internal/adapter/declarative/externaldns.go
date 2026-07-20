/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// ExternalDNSDefinition is the declarative external-dns adapter (SKA-62).
// system_health verifies the external-dns controller Deployment and its pods;
// crd_health verifies the DNSEndpoint CRD. The Deployment is Required (absent
// -> Fail), but the CRD is Optional: the Helm chart ships it in crds/, but
// manifest-based installs and charts deployed with --skip-crds legitimately
// run without dnsendpoints.externaldns.k8s.io, and the CRD source is an
// opt-in feature — so absence scores Skipped with the absent marker.
//
// The Deployment name follows the Helm release fullname, so it is
// policy-overridable via the "deploymentName" threshold (the chart convention
// is a release named external-dns in the external-dns namespace).
//
// Deferred: a record_sync family scoring DNS record reconciliation outcomes.
// DNSEndpoint.status carries only observedGeneration — no conditions — so
// per-record outcomes are observable only via external-dns metrics/logs, which
// no shipped evaluator can express.
var ExternalDNSDefinition = AddonDefinition{
	AddonType:      "external-dns",
	AdapterVersion: "0.1.0",
	// Detect the installed external-dns version off the controller Deployment
	// (its app.kubernetes.io/version label, else the
	// registry.k8s.io/external-dns image tag). Detection-only: SupportedVersions
	// is left empty (external-dns is pre-1.0 and the exact range is
	// maintainer-owned), so external-dns never Warns on a version — gating is
	// opt-in (SKA-527).
	VersionSource: &VersionSource{FromFamily: adapter.Family("system_health")},
	RBAC: []adapter.PolicyRule{
		// Verbs mirror the engine's actual reads through the direct (uncached)
		// impersonating client: workloads and CRDs are fetched by name (get),
		// pods by label selector (list). Nothing watches — the client is
		// deliberately cache-free (see internal/adapter/impersonation) — so no
		// list/watch is granted where the engine never lists or watches.
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"},
			Justification: "Get the external-dns controller Deployment by name to score readiness. The name/namespace are policy-overridable (Helm release fullname) but always resolve to a single named Get; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"},
			Justification: "List the external-dns Pods by label selector for restart counts and readiness behind the Deployment. list (not get) because Pod names are dynamic; read-only."},
		{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get"},
			Justification: "Get the DNSEndpoint CRD by name to verify it is Established and serves a supported version. Deliberately NOT the DNSEndpoint objects themselves — no evaluator reads them (their status carries no conditions); read-only."},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("system_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDeployment,
				DefaultNamespace:        "external-dns",
				NameThresholdKey:        "deploymentName",
				DefaultName:             "external-dns",
				Component:               "external-dns",
				Absence:                 Required,
				CheckPods:               true,
				RestartWarnThresholdKey: "restartWarnCount",
				DefaultRestartWarn:      3,
			}},
		},
		{
			Name:           adapter.Family("crd_health"),
			DefaultEnabled: true,
			CRDs: []CRDCheck{{
				Names:             []string{"dnsendpoints.externaldns.k8s.io"},
				SupportedVersions: []string{"v1alpha1"},
				// The CRD source is opt-in, so a missing DNSEndpoint CRD is a
				// legitimate install shape — Skipped, not Fail (SKA-526).
				Absence:                   Optional,
				UnsupportedVersionOutcome: adapter.OutcomeWarn,
			}},
		},
	},
}

// NewExternalDNSEngine returns the declarative external-dns adapter. RBAC
// markers live on the package doc in definition.go.
func NewExternalDNSEngine() *Engine {
	return MustEngine(ExternalDNSDefinition)
}
