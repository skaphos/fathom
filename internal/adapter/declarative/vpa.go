/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// VpaDefinition is the declarative Vertical Pod Autoscaler adapter (SKA-509).
// Three families cover it:
//
//   - system_health — the recommender, updater, and admission-controller
//     Deployments and their pods.
//   - crd_health — the VerticalPodAutoscaler and VerticalPodAutoscalerCheckpoint
//     CRDs are Established and serve a supported version.
//   - recommendation_health — every VPA reports RecommendationProvided=True (the
//     recommender is producing sizing), and the vpa-webhook-config
//     MutatingWebhookConfiguration is present and wired. A VPA that has not yet
//     produced a recommendation (condition absent or not True) is a Warn, not a
//     Fail: it is often a young VPA or one with no matching pods, not a broken
//     install.
//
// The recommender is blind without metrics-server (relate to the metrics-server
// adapter); this adapter scores what VPA itself reports, not the upstream
// metrics pipeline. VPA is Optional: on a cluster that does not run VPA every
// NotFound target scores Skipped with the adapter.DetailAbsent marker. The VPA
// scan is cluster-wide (ClusterScoped) because VerticalPodAutoscaler objects
// live in the workload namespaces; an empty cluster yields a Skipped
// recommendation_health.
var VpaDefinition = AddonDefinition{
	AddonType:      "vpa",
	AdapterVersion: "0.1.0",
	Optional:       true,
	// Detect the installed VPA version off the recommender Deployment (its
	// app.kubernetes.io/version label, else the registry.k8s.io/autoscaling/vpa-recommender
	// image tag). Detection-only: SupportedVersions is left empty so VPA never
	// Warns on a version — gating is opt-in (SKA-527).
	VersionSource: &VersionSource{FromFamily: adapter.Family("system_health"), FromComponent: "vpa-recommender"},
	RBAC: []adapter.PolicyRule{
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"},
			Justification: "Get the three VPA Deployments (recommender, updater, admission-controller) by name to score readiness. get only — each is a single named Get and the impersonating client is cache-free, so no list/watch; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"},
			Justification: "List the VPA Pods by label selector for restart counts and readiness behind the Deployments. list (not get) because Pod names are dynamic; read-only."},
		{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get"},
			Justification: "Get each VPA CRD by name to verify it is Established and serves a supported version. get only — the check fetches each CRD by name in turn, never lists; read-only."},
		{APIGroups: []string{"autoscaling.k8s.io"}, Resources: []string{"verticalpodautoscalers"}, Verbs: []string{"list"},
			Justification: "List VerticalPodAutoscaler objects cluster-wide to score their RecommendationProvided condition. list only — the scan enumerates VPAs and never Gets one by name. Scoped to VerticalPodAutoscalers only — not the Checkpoints, which are read solely as CRDs — and read-only."},
		{APIGroups: []string{"admissionregistration.k8s.io"}, Resources: []string{"mutatingwebhookconfigurations"}, Verbs: []string{"get"},
			Justification: "Get the vpa-webhook-config MutatingWebhookConfiguration by name to verify it is present and its caBundle is populated (an unpopulated caBundle silently disables VPA's in-place pod sizing). get only — exactly one named configuration."},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("system_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{
				vpaDeployment("vpa-recommender"),
				vpaDeployment("vpa-updater"),
				vpaDeployment("vpa-admission-controller"),
			},
		},
		{
			Name:           adapter.Family("crd_health"),
			DefaultEnabled: true,
			CRDs: []CRDCheck{{
				Names: []string{
					"verticalpodautoscalers.autoscaling.k8s.io",
					"verticalpodautoscalercheckpoints.autoscaling.k8s.io",
				},
				SupportedVersions:         []string{"v1", "v1beta2"},
				UnsupportedVersionOutcome: adapter.OutcomeWarn,
			}},
		},
		{
			Name:           adapter.Family("recommendation_health"),
			DefaultEnabled: true,
			ManagedResources: []ConditionCheck{{
				APIVersion:        "autoscaling.k8s.io/v1",
				VersionCRD:        "verticalpodautoscalers.autoscaling.k8s.io",
				SupportedVersions: []string{"v1", "v1beta2"},
				Kind:              "VerticalPodAutoscaler",
				ListKind:          "VerticalPodAutoscalerList",
				ListName:          "verticalpodautoscalers",
				ClusterScoped:     true,
				ConditionType:     "RecommendationProvided",
				ExpectedStatus:    "True",
				AbsentCondition:   adapter.OutcomeWarn,
				Mismatch:          adapter.OutcomeWarn,
			}},
			Webhooks: []WebhookCheck{{
				Kind:             KindMutatingWebhookConfiguration,
				Name:             "vpa-webhook-config",
				NameThresholdKey: "webhookConfigurationName",
			}},
		},
	},
}

// vpaDeployment builds an Optional VPA controller Deployment check (with pods
// and a policy-overridable restart-warn threshold) in the kube-system namespace.
func vpaDeployment(name string) WorkloadCheck {
	return WorkloadCheck{
		Kind:                    KindDeployment,
		DefaultNamespace:        "kube-system",
		DefaultName:             name,
		Component:               name,
		CheckPods:               true,
		RestartWarnThresholdKey: "restartWarnCount",
		DefaultRestartWarn:      3,
	}
}

// NewVpaEngine returns the declarative VPA adapter. RBAC markers live on the
// package doc in definition.go.
func NewVpaEngine() *Engine {
	return MustEngine(VpaDefinition)
}
