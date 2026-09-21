/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package addondefinition_test

import (
	"strings"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
)

func payloadCases() []api.DefinitionCheck {
	ns := api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{"default"}}
	cluster := api.DefinitionTarget{Scope: "Cluster"}
	return []api.DefinitionCheck{
		{Name: "workload", Kind: "Workload", Workload: &api.DefinitionWorkload{Target: ns, Kind: "Deployment", DefaultName: "controller"}},
		{Name: "crd", Kind: "CRD", CRD: &api.DefinitionCRD{Target: cluster, Names: []api.DefinitionResourceName{"widgets.example.org"}, SupportedVersions: []api.DefinitionToken{"v1"}}},
		{Name: "condition", Kind: "Condition", Condition: &api.DefinitionCondition{Target: cluster, APIVersion: "apiregistration.k8s.io/v1", Kind: "APIService", Names: []api.DefinitionResourceName{"v1.metrics.k8s.io"}, ConditionType: "Available", ExpectedStatus: "True"}},
		{Name: "field", Kind: "Field", Field: &api.DefinitionField{Target: ns, APIVersion: "example.org/v1", Kind: "Widget", ListKind: "WidgetList", FieldPath: []string{"status", "state"}, ExpectedValue: "Ready"}},
		{Name: "webhook", Kind: "Webhook", Webhook: &api.DefinitionWebhook{Target: cluster, Kind: "ValidatingWebhookConfiguration", Name: "validation"}},
		{Name: "cronjob", Kind: "CronJob", CronJob: &api.DefinitionCronJob{Target: ns, DefaultName: "job"}},
		{Name: "configmap", Kind: "ConfigMap", ConfigMap: &api.DefinitionConfigMap{Target: ns, DefaultName: "config", Key: "policy.yaml"}},
		{Name: "annotation", Kind: "AnnotationStaleness", AnnotationStaleness: &api.DefinitionAnnotationStaleness{Target: ns, APIVersion: "v1", Kind: "ConfigMap", DefaultName: "config", AnnotationKey: "example.org/last-check", DefaultMaxAge: "1m"}},
		{Name: "projection", Kind: "PodProjection", PodProjection: &api.DefinitionPodProjection{Target: ns, Selector: map[string]api.DefinitionSelectorValue{"app": "custom"}, VolumeName: "token"}},
	}
}

func TestAllPayloadKinds(t *testing.T) {
	for _, c := range payloadCases() {
		t.Run(c.Kind, func(t *testing.T) {
			d := validDefinition()
			d.Spec.Families[0].Checks = []api.DefinitionCheck{c}
			if err := definitions.Validate(d); err != nil {
				t.Fatal(err)
			}
			d.Spec.Families[0].Checks[0].Kind = "Expression"
			if err := definitions.Validate(d); err == nil {
				t.Fatal("unknown discriminator accepted")
			}
		})
	}
}

func TestPayloadConstraints(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index int
		edit  func(*api.DefinitionCheck)
	}{
		{"workload scope", 0, func(c *api.DefinitionCheck) { c.Workload.Target.Scope = "Cluster" }},
		{"empty CRD versions", 1, func(c *api.DefinitionCheck) { c.CRD.SupportedVersions = nil }},
		{"too many CRD versions", 1, func(c *api.DefinitionCheck) { c.CRD.SupportedVersions = make([]api.DefinitionToken, 9) }},
		{"condition named and collection", 2, func(c *api.DefinitionCheck) { c.Condition.ListKind = "APIServiceList" }},
		{"condition invalid predicate", 2, func(c *api.DefinitionCheck) { c.Condition.ExpectedStatus = "Ready" }},
		{"condition unpaired versions", 2, func(c *api.DefinitionCheck) { c.Condition.VersionCRD = "widgets.example.org" }},
		{"field unreachable override", 3, func(c *api.DefinitionCheck) {
			c.Field.ValueOutcomes = map[string]api.DefinitionOutcome{"Ready": "Fail"}
		}},
		{"field empty explicit outcome", 3, func(c *api.DefinitionCheck) { c.Field.ValueOutcomes = map[string]api.DefinitionOutcome{"Other": ""} }},
		{"field too deep", 3, func(c *api.DefinitionCheck) { c.Field.FieldPath = make([]string, 17) }},
		{"field byte bound", 3, func(c *api.DefinitionCheck) { c.Field.FieldPath = []string{strings.Repeat("é", 65)} }},
		{"webhook helper half specified", 4, func(c *api.DefinitionCheck) { c.Webhook.ExpectedService = "validation" }},
		{"webhook endpoints without service", 4, func(c *api.DefinitionCheck) { c.Webhook.VerifyEndpoints = true }},
		{"negative duration", 5, func(c *api.DefinitionCheck) { c.CronJob.DefaultSuccessMaxAge = "-1s" }},
		{"overflow duration", 5, func(c *api.DefinitionCheck) { c.CronJob.DefaultSuccessMaxAge = "99999999999999999999h" }},
		{"invalid configmap key", 6, func(c *api.DefinitionCheck) { c.ConfigMap.Key = "../policy" }},
		{"duplicate config versions", 6, func(c *api.DefinitionCheck) { c.ConfigMap.RecognizedAPIVersions = []string{"v1", "v1"} }},
		{"zero annotation age", 7, func(c *api.DefinitionCheck) { c.AnnotationStaleness.DefaultMaxAge = "0s" }},
		{"annotation collection singleton", 7, func(c *api.DefinitionCheck) { c.AnnotationStaleness.ListKind = "ConfigMapList" }},
		{"annotation key syntax", 7, func(c *api.DefinitionCheck) { c.AnnotationStaleness.AnnotationKey = "a/b/c" }},
		{"empty projection selector", 8, func(c *api.DefinitionCheck) { c.PodProjection.Selector = nil }},
		{"invalid projection label", 8, func(c *api.DefinitionCheck) {
			c.PodProjection.Selector = map[string]api.DefinitionSelectorValue{"app": "*"}
		}},
		{"invalid env", 8, func(c *api.DefinitionCheck) { c.PodProjection.EnvVar = "A-B" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := payloadCases()[tc.index]
			tc.edit(&c)
			d := validDefinition()
			d.Spec.Families[0].Checks = []api.DefinitionCheck{c}
			if err := definitions.Validate(d); err == nil {
				t.Fatal("invalid payload accepted")
			}
		})
	}
}
