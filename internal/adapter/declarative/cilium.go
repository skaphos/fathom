/*
SPDX-FileCopyrightText: 2026 Skaphos
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
// The RBAC field is documentation-only; the enforced +kubebuilder:rbac markers
// for all declarative adapters live on the package doc in definition.go.
var CiliumDefinition = AddonDefinition{
	AddonType:      "cilium",
	AdapterVersion: "0.2.0",
	Optional:       true,
	// Detect the installed Cilium version off the agent DaemonSet (its
	// app.kubernetes.io/version label, else the quay.io/cilium/cilium image tag).
	// Detection-only: SupportedVersions is left empty so Cilium never Warns on a
	// version — gating is opt-in once a maintainer confirms the range (SKA-527).
	VersionSource: &VersionSource{Kind: KindDaemonSet, Namespace: "kube-system", Name: "cilium"},
	RBAC: []RBACRule{
		{APIGroups: "apps", Resources: "deployments", Verbs: "get;list;watch"},
		{APIGroups: "apps", Resources: "daemonsets", Verbs: "get;list;watch"},
		{APIGroups: "", Resources: "pods", Verbs: "get;list;watch"},
		{APIGroups: "apiextensions.k8s.io", Resources: "customresourcedefinitions", Verbs: "get;list;watch"},
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
