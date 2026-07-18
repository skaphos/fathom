/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// CiliumDefinition is the declarative Cilium adapter (it replaced the removed
// hand-written internal/adapter/cilium package):
// three families (control_plane_health, agent_health, crd_health) checking the
// cilium-operator Deployment and pods, the cilium agent DaemonSet and pods, and
// the core Cilium CRDs. The addon is Optional, so on a cluster that does not run
// Cilium every NotFound target scores Skipped and carries the adapter.DetailAbsent
// marker. The persisted verdict of such a run is Skipped, not Pass — only the
// metrics/tracing FamilyOutcome roll-up relabels an all-Skipped family as Pass.
//
// The RBAC field is the per-addon least-privilege source the RBAC generator
// emits Cilium's scoped read-only ClusterRole from (SKA-58).
var CiliumDefinition = AddonDefinition{
	AddonType:      "cilium",
	AdapterVersion: "0.2.0",
	Optional:       true,
	// Detect the installed Cilium version off the agent DaemonSet (its
	// app.kubernetes.io/version label, else the quay.io/cilium/cilium image tag).
	// Detection-only: SupportedVersions is left empty so Cilium never Warns on a
	// version — gating is opt-in once a maintainer confirms the range (SKA-527).
	VersionSource: &VersionSource{Kind: KindDaemonSet, Namespace: "kube-system", Name: "cilium"},
	RBAC: []adapter.PolicyRule{
		{APIGroups: []string{"apps"}, Resources: []string{"deployments", "daemonsets"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read the cilium-operator Deployment and the cilium agent DaemonSet to score readiness. list+watch because Cilium is Optional and may be absent, and the workload names are policy-overridable; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read the operator/agent Pods for restart counts and readiness behind their controllers. list is required because Pod names are dynamic; read-only."},
		{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read the core Cilium CRDs to verify they are Established and serve a supported version. list is needed to check several CRDs at once; read-only."},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("control_plane_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDeployment,
				DefaultNamespace:        "kube-system",
				NameThresholdKey:        "operatorDeploymentName",
				DefaultName:             "cilium-operator",
				Component:               "cilium-operator",
				CheckPods:               true,
				RestartWarnThresholdKey: "restartWarnCount",
				DefaultRestartWarn:      3,
			}},
		},
		{
			Name:           adapter.Family("agent_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDaemonSet,
				DefaultNamespace:        "kube-system",
				NameThresholdKey:        "agentDaemonSetName",
				DefaultName:             "cilium",
				Component:               "cilium-agent",
				CheckPods:               true,
				RestartWarnThresholdKey: "restartWarnCount",
				DefaultRestartWarn:      3,
			}},
		},
		{
			Name:           adapter.Family("crd_health"),
			DefaultEnabled: true,
			CRDs: []CRDCheck{{
				Names: []string{
					"ciliumnetworkpolicies.cilium.io",
					"ciliumclusterwidenetworkpolicies.cilium.io",
					"ciliumendpoints.cilium.io",
					"ciliumidentities.cilium.io",
					"ciliumnodes.cilium.io",
				},
				SupportedVersions:         []string{"v2", "v2alpha1"},
				UnsupportedVersionOutcome: adapter.OutcomeWarn,
			}},
		},
	},
}

// NewCiliumEngine returns the declarative Cilium adapter. It panics only on a
// programmer error in CiliumDefinition, which is caught by any test that
// constructs the engine. RBAC markers live on the package doc in definition.go.
func NewCiliumEngine() *Engine {
	return MustEngine(CiliumDefinition)
}
