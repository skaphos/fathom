/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package addondefinition_test

import (
	"fmt"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
)

func TestTargetScopeUnion(t *testing.T) {
	d := validDefinition()
	d.Spec.Families[0].Checks = payloadCases()
	scope, err := definitions.TargetScope(d)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.AllowClusterScoped || len(scope.Namespaces) != 1 || scope.Namespaces[0] != "default" {
		t.Fatalf("scope=%+v", scope)
	}
	projection := payloadCases()[8]
	projection.PodProjection.Target.Namespaces = nil
	for i := 0; i < definitions.MaxNamespaces; i++ {
		projection.PodProjection.Target.Namespaces = append(projection.PodProjection.Target.Namespaces, api.DefinitionDNSLabel(fmt.Sprintf("ns-%02d", i)))
	}
	d.Spec.Families[0].Checks = []api.DefinitionCheck{projection}
	scope, err = definitions.TargetScope(d)
	if err != nil || len(scope.Namespaces) != definitions.MaxNamespaces {
		t.Fatalf("at limit: %+v %v", scope, err)
	}
	extra := payloadCases()[0]
	d.Spec.Families[0].Checks = append(d.Spec.Families[0].Checks, extra)
	if _, err := definitions.TargetScope(d); err == nil {
		t.Fatal("union bypassed binding namespace cap")
	}
}

func TestDefinitionScopeIntersectionRejectsPartialCoverage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		checks []api.DefinitionCheck
		scope  api.DefinitionBindingScope
		valid  bool
	}{
		{"namespaced", []api.DefinitionCheck{payloadCases()[0]}, api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"default"}}, true},
		{"missing primary namespace", []api.DefinitionCheck{payloadCases()[0]}, api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"other"}}, false},
		{"cluster", []api.DefinitionCheck{payloadCases()[1]}, api.DefinitionBindingScope{AllowClusterScoped: true}, true},
		{"no cluster permission", []api.DefinitionCheck{payloadCases()[1]}, api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"default"}}, false},
		{"mixed partial coverage", payloadCases(), api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"default"}}, false},
		{"mixed authorized", payloadCases(), api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"default"}, AllowClusterScoped: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := validDefinition()
			d.Spec.Families[0].Checks = tc.checks
			if err := definitions.ValidateScope(d, tc.scope); (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
	d := validDefinition()
	hook := payloadCases()[4]
	hook.Webhook.ExpectedService = "hook"
	hook.Webhook.ServiceNamespace = "helpers"
	hook.Webhook.VerifyEndpoints = true
	d.Spec.Families[0].Checks = []api.DefinitionCheck{hook}
	if err := definitions.ValidateScope(d, api.DefinitionBindingScope{AllowClusterScoped: true}); err == nil {
		t.Fatal("unauthorized helper namespace accepted")
	}
	if err := definitions.ValidateScope(d, api.DefinitionBindingScope{AllowClusterScoped: true, Namespaces: []api.DefinitionDNSLabel{"helpers"}}); err != nil {
		t.Fatal(err)
	}
}
