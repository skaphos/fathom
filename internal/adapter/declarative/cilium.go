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
// the core Cilium CRDs. Every workload and CRD is Optional, so a cluster that
// does not run Cilium rolls up green (NotFound -> Skipped).
//
// The RBAC field is documentation-only; the enforced +kubebuilder:rbac markers
// for all declarative adapters live in rbac.go.
var CiliumDefinition = AddonDefinition{
	AddonType:      "cilium",
	AdapterVersion: "0.1.0",
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
				Absence:                 Optional,
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
				Absence:                 Optional,
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
				Absence:                   Optional,
				UnsupportedVersionOutcome: adapter.OutcomeWarn,
			}},
		},
	},
}

// NewCiliumEngine returns the declarative Cilium adapter. It panics only on a
// programmer error in CiliumDefinition, which is caught by any test that
// constructs the engine. RBAC markers live in rbac.go.
func NewCiliumEngine() *Engine {
	return MustEngine(CiliumDefinition)
}
