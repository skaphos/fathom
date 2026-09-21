/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package addondefinition_test

import (
	"reflect"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
)

func TestOfflineGrantsDoNotBroadenMixedScope(t *testing.T) {
	d := validDefinition()
	d.Spec.Families[0].Checks = payloadCases()
	group := "example.org"
	d.Spec.RequestedReads = []api.DefinitionReadRule{{APIGroup: &group, Resources: []string{"widgets"}, Verbs: []string{"list"}}}
	original := d.DeepCopy()
	plan, err := definitions.PlanGrants(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Diagnostics) == 0 {
		t.Fatal("custom resource requires manual completion")
	}
	for _, grant := range plan.Grants {
		if grant.Rule.APIGroups[0] == group {
			t.Fatal("requestedReads inferred authority or scope")
		}
		if grant.Rule.Resources[0] == "deployments" && grant.Namespace != "default" {
			t.Fatal("namespaced grant broadened")
		}
		for _, v := range grant.Rule.Verbs {
			if v != "get" && v != "list" {
				t.Fatal("non-read grant")
			}
		}
	}
	again, err := definitions.PlanGrants(d)
	if err != nil || !reflect.DeepEqual(plan, again) || !reflect.DeepEqual(d, original) {
		t.Fatal("nondeterministic or mutating planner")
	}
}

func TestOfflineWebhookHelperAndOverride(t *testing.T) {
	d := validDefinition()
	c := payloadCases()[4]
	c.Webhook.VerifyEndpoints = true
	c.Webhook.ExpectedService = "validation"
	c.Webhook.ServiceNamespace = "helpers"
	c.Webhook.NameThresholdKey = "webhookName"
	d.Spec.Families[0].Checks = []api.DefinitionCheck{c}
	plan, err := definitions.PlanGrants(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Grants) != 2 {
		t.Fatalf("grants=%+v", plan.Grants)
	}
	var endpoints, webhook bool
	for _, g := range plan.Grants {
		switch g.Rule.Resources[0] {
		case "endpointslices":
			endpoints = g.Namespace == "helpers" && reflect.DeepEqual(g.Rule.Verbs, []string{"list"})
		case "validatingwebhookconfigurations":
			webhook = g.Namespace == "" && reflect.DeepEqual(g.Rule.ResourceNames, []string{"validation"})
		}
	}
	if !endpoints || !webhook || len(plan.Diagnostics) == 0 {
		t.Fatalf("unsafe helper or missing override diagnostic: %+v", plan)
	}
}
