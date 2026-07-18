/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// KedaDefinition is the declarative KEDA adapter (SKA-508). KEDA is
// event-driven autoscaling; three families cover it:
//
//   - system_health — the keda-operator, metrics-apiserver, and admission-webhooks
//     Deployments and their pods, plus the keda-admission ValidatingWebhookConfiguration
//     (an unpopulated caBundle silently disables ScaledObject admission validation).
//   - crd_health — the four core KEDA CRDs are Established and serve a supported version.
//   - scaling_health — every ScaledObject reports Ready=True (KEDA created the managed
//     HPA and the triggers resolved) and is not Paused. Ready mismatch is a Fail: an
//     un-Ready ScaledObject is not autoscaling its workload. A paused ScaledObject is a
//     Warn — surfaced, not failed, because pausing is a deliberate operator action.
//
// KEDA is Optional: on a cluster that does not run KEDA every NotFound target
// scores Skipped and carries the adapter.DetailAbsent marker. The ScaledObject
// scan is cluster-wide (ClusterScoped) because ScaledObjects live in the
// workload namespaces, not KEDA's own; an empty cluster yields a Skipped
// scaling_health.
var KedaDefinition = AddonDefinition{
	AddonType:      "keda",
	AdapterVersion: "0.1.0",
	Optional:       true,
	// Detect the installed KEDA version off the keda-operator Deployment (its
	// app.kubernetes.io/version label, else the ghcr.io/kedacore/keda image tag).
	// Detection-only: SupportedVersions is left empty so KEDA never Warns on a
	// version — gating is opt-in once a maintainer confirms the range (SKA-527).
	VersionSource: &VersionSource{Kind: KindDeployment, Namespace: "keda", Name: "keda-operator"},
	RBAC: []adapter.PolicyRule{
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"},
			Justification: "Get the three KEDA Deployments (operator, metrics-apiserver, admission-webhooks) by name to score readiness. get only — each is a single named Get (a policy-overridable name still resolves to one Get) and the impersonating client is cache-free, so no list/watch; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"},
			Justification: "List the KEDA Pods by label selector for restart counts and readiness behind the Deployments. list (not get) because Pod names are dynamic; read-only."},
		{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get"},
			Justification: "Get each KEDA CRD by name to verify it is Established and serves a supported version. get only — the check fetches each CRD by name in turn, never lists; read-only."},
		{APIGroups: []string{"keda.sh"}, Resources: []string{"scaledobjects"}, Verbs: []string{"list"},
			Justification: "List ScaledObjects cluster-wide to score their Ready/Paused conditions. list only — the scan enumerates ScaledObjects and never Gets one by name. Scoped to the keda.sh group and to ScaledObjects only — not TriggerAuthentications, which can reference trigger credentials — and read-only."},
		{APIGroups: []string{"admissionregistration.k8s.io"}, Resources: []string{"validatingwebhookconfigurations"}, Verbs: []string{"get"},
			Justification: "Get the keda-admission ValidatingWebhookConfiguration by name to verify it is present and its caBundle is populated. get only — the check fetches exactly one named configuration."},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("system_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{
				kedaDeployment("keda-operator"),
				kedaDeployment("keda-operator-metrics-apiserver"),
				kedaDeployment("keda-admission-webhooks"),
			},
			Webhooks: []WebhookCheck{{
				Kind:             KindValidatingWebhookConfiguration,
				Name:             "keda-admission",
				NameThresholdKey: "webhookConfigurationName",
			}},
		},
		{
			Name:           adapter.Family("crd_health"),
			DefaultEnabled: true,
			CRDs: []CRDCheck{{
				Names: []string{
					"scaledobjects.keda.sh",
					"scaledjobs.keda.sh",
					"triggerauthentications.keda.sh",
					"clustertriggerauthentications.keda.sh",
				},
				SupportedVersions:         []string{"v1alpha1"},
				UnsupportedVersionOutcome: adapter.OutcomeWarn,
			}},
		},
		{
			Name:           adapter.Family("scaling_health"),
			DefaultEnabled: true,
			ManagedResources: []ConditionCheck{
				{
					APIVersion:      "keda.sh/v1alpha1",
					Kind:            "ScaledObject",
					ListKind:        "ScaledObjectList",
					ListName:        "scaledobjects",
					ClusterScoped:   true,
					ConditionType:   "Ready",
					ExpectedStatus:  "True",
					AbsentCondition: adapter.OutcomeWarn,
				},
				{
					APIVersion:      "keda.sh/v1alpha1",
					Kind:            "ScaledObject",
					ListKind:        "ScaledObjectList",
					ListName:        "scaledobjects-paused",
					ClusterScoped:   true,
					ConditionType:   "Paused",
					ExpectedStatus:  "False",
					AbsentCondition: adapter.OutcomePass,
					Mismatch:        adapter.OutcomeWarn,
				},
			},
		},
	},
}

// kedaDeployment builds an Optional KEDA controller Deployment check (with pods
// and a policy-overridable restart-warn threshold) in the keda namespace.
func kedaDeployment(name string) WorkloadCheck {
	return WorkloadCheck{
		Kind:                    KindDeployment,
		DefaultNamespace:        "keda",
		DefaultName:             name,
		Component:               name,
		CheckPods:               true,
		RestartWarnThresholdKey: "restartWarnCount",
		DefaultRestartWarn:      3,
	}
}

// NewKedaEngine returns the declarative KEDA adapter. RBAC markers live on the
// package doc in definition.go.
func NewKedaEngine() *Engine {
	return MustEngine(KedaDefinition)
}
