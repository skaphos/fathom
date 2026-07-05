/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// ExternalSecretsDefinition is the declarative External Secrets Operator adapter
// (it replaced the passive checks of the removed hand-written
// internal/adapter/externalsecrets package). system_health verifies the three
// ESO Deployments and their pods plus the four core ESO CRDs; secret_sync scores
// ExternalSecret Ready conditions in the external-secrets namespace, and is
// Skipped when no ExternalSecret resources exist. Deployments and CRDs are
// Required (absent -> Fail), matching the hand-written adapter.
//
// The managed list resolves the served version via VersionCRD (v1, falling back
// to v1beta1), matching the hand-written adapter. Deferred (not exercised by the
// empty-cluster e2e): the refreshTime staleness Warn, which needs a recency
// evaluator.
var ExternalSecretsDefinition = AddonDefinition{
	AddonType:      "external-secrets",
	AdapterVersion: "0.2.0",
	RBAC: []RBACRule{
		{APIGroups: "apps", Resources: "deployments", Verbs: "get;list;watch"},
		{APIGroups: "", Resources: "pods", Verbs: "get;list;watch"},
		{APIGroups: "apiextensions.k8s.io", Resources: "customresourcedefinitions", Verbs: "get;list;watch"},
		{APIGroups: "external-secrets.io", Resources: "externalsecrets", Verbs: "get;list;watch"},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("system_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{
				esoDeployment("external-secrets"),
				esoDeployment("external-secrets-webhook"),
				esoDeployment("external-secrets-cert-controller"),
			},
			CRDs: []CRDCheck{{
				Names: []string{
					"externalsecrets.external-secrets.io",
					"secretstores.external-secrets.io",
					"clustersecretstores.external-secrets.io",
					"clusterexternalsecrets.external-secrets.io",
				},
				SupportedVersions:         []string{"v1", "v1beta1"},
				Absence:                   Required,
				UnsupportedVersionOutcome: adapter.OutcomeWarn,
			}},
		},
		{
			Name:           adapter.Family("secret_sync"),
			DefaultEnabled: true,
			ManagedResources: []ConditionCheck{{
				APIVersion:        "external-secrets.io/v1",
				VersionCRD:        "externalsecrets.external-secrets.io",
				SupportedVersions: []string{"v1", "v1beta1"},
				Kind:              "ExternalSecret",
				ListKind:          "ExternalSecretList",
				ListName:          "externalsecrets",
				DefaultNamespace:  "external-secrets",
				ConditionType:     "Ready",
				ExpectedStatus:    "True",
			}},
		},
	},
}

// esoDeployment builds a Required ESO controller Deployment check (with pods and
// a policy-overridable restart-warn threshold) in the external-secrets namespace.
func esoDeployment(name string) WorkloadCheck {
	return WorkloadCheck{
		Kind:                    KindDeployment,
		DefaultNamespace:        "external-secrets",
		DefaultName:             name,
		Component:               name,
		Absence:                 Required,
		CheckPods:               true,
		RestartWarnThresholdKey: "restartWarnCount",
		DefaultRestartWarn:      3,
	}
}

// NewExternalSecretsEngine returns the declarative External Secrets adapter.
// RBAC markers (including the adapter-unique external-secrets.io read) live on
// the package doc in definition.go.
func NewExternalSecretsEngine() *Engine {
	return MustEngine(ExternalSecretsDefinition)
}
