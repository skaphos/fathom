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
