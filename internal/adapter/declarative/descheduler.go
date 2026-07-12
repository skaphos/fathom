/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"time"

	"github.com/skaphos/fathom/pkg/adapter"
)

// DeschedulerDefinition is the declarative descheduler adapter (SKA-510). The
// descheduler evicts pods so the scheduler can place them better; it runs either
// as a long-lived Deployment (leader-elected loop) or as a CronJob. Three
// families cover it:
//
//   - system_health — the Deployment OR the CronJob is healthy. A real install
//     is one or the other, so with the Optional posture whichever deployment mode
//     is not in use scores Skipped; the mode actually installed is scored.
//   - policy_validity — the DeschedulerPolicy ConfigMap holds a policy.yaml that
//     parses as YAML and declares a recognized descheduler apiVersion. This is the
//     check that catches the silent no-op: an unparseable policy means the
//     descheduler evicts nothing, with no other outward symptom.
//   - last_run — the CronJob's last scheduled run completed successfully and is
//     recent (not stuck). Skipped on a Deployment-mode install.
//
// The policy check is intentionally shape-level (valid YAML + recognized
// apiVersion); validating individual strategy/plugin names against the running
// descheduler release is version-coupled knowledge the generic engine does not
// carry (see ConfigMapCheck). descheduler is Optional: absent targets score
// Skipped with the adapter.DetailAbsent marker.
var DeschedulerDefinition = AddonDefinition{
	AddonType:      "descheduler",
	AdapterVersion: "0.1.0",
	Optional:       true,
	RBAC: []adapter.PolicyRule{
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list"},
			Justification: "Read the descheduler Deployment (long-lived deployment mode) to score readiness. list because descheduler is Optional and may run as a CronJob instead, and the name/namespace are policy-overridable; read-only."},
		{APIGroups: []string{"batch"}, Resources: []string{"cronjobs"}, Verbs: []string{"get", "list"},
			Justification: "Read the descheduler CronJob (CronJob deployment mode) to score presence, suspend state, and last-successful-run recency. list because it may run as a Deployment instead and the name is policy-overridable; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"},
			Justification: "List the descheduler Pods by label selector for restart counts and readiness behind the Deployment. list (not get) because Pod names are dynamic; read-only."},
		{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "list"},
			Justification: "Read the DeschedulerPolicy ConfigMap to verify its policy.yaml parses and declares a recognized apiVersion. list because the ConfigMap name/namespace are policy-overridable (Helm release fullname); read-only, and the descheduler policy holds no secret material."},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("system_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDeployment,
				DefaultNamespace:        "kube-system",
				NameThresholdKey:        "deploymentName",
				DefaultName:             "descheduler",
				Component:               "descheduler",
				CheckPods:               true,
				RestartWarnThresholdKey: "restartWarnCount",
				DefaultRestartWarn:      3,
			}},
			CronJobs: []CronJobCheck{{
				DefaultNamespace: "kube-system",
				NameThresholdKey: "cronJobName",
				DefaultName:      "descheduler",
				Component:        "descheduler",
			}},
		},
		{
			Name:           adapter.Family("policy_validity"),
			DefaultEnabled: true,
			ConfigMaps: []ConfigMapCheck{{
				DefaultNamespace:      "kube-system",
				NameThresholdKey:      "configMapName",
				DefaultName:           "descheduler",
				Component:             "descheduler",
				Key:                   "policy.yaml",
				RecognizedAPIVersions: []string{"descheduler/v1alpha1", "descheduler/v1alpha2"},
				UnrecognizedOutcome:   adapter.OutcomeWarn,
				InvalidOutcome:        adapter.OutcomeFail,
			}},
		},
		{
			Name:           adapter.Family("last_run"),
			DefaultEnabled: true,
			CronJobs: []CronJobCheck{{
				DefaultNamespace:          "kube-system",
				NameThresholdKey:          "cronJobName",
				DefaultName:               "descheduler",
				Component:                 "descheduler",
				SuccessMaxAgeThresholdKey: "successMaxAge",
				DefaultSuccessMaxAge:      24 * time.Hour,
				StaleOutcome:              adapter.OutcomeWarn,
			}},
		},
	},
}

// NewDeschedulerEngine returns the declarative descheduler adapter. RBAC markers
// live on the package doc in definition.go.
func NewDeschedulerEngine() *Engine {
	return MustEngine(DeschedulerDefinition)
}
