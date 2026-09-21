/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative_test

import (
	"context"
	"errors"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/declarative"
	"github.com/skaphos/fathom/pkg/adapter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRuntimeCompilationPreservesOrderAndSnapshot(t *testing.T) {
	ns := api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{"default"}}
	d := &api.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: "custom"}, Spec: api.AddonDefinitionSpec{AddonType: "custom", AdapterVersion: "1.0.0", SemanticsVersion: 1, Optional: true, Families: []api.DefinitionFamily{{Name: "health", DefaultEnabled: true, Checks: []api.DefinitionCheck{
		{Name: "first", Kind: "ConfigMap", ConfigMap: &api.DefinitionConfigMap{Target: ns, DefaultName: "first", Key: "config"}},
		{Name: "second", Kind: "CronJob", CronJob: &api.DefinitionCronJob{Target: ns, DefaultName: "second"}},
	}}}}}
	engine, err := declarative.CompileRuntime(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	d.Spec.Families[0].Checks[0].ConfigMap.DefaultName = "mutated"
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background(), adapter.Request{Client: fake.NewClientBuilder().WithScheme(scheme).Build()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Checks) != 2 {
		t.Fatalf("checks=%#v", result.Checks)
	}
	if result.Checks[0].TargetRef.Name != "first" || result.Checks[1].TargetRef.Name != "second" {
		t.Fatalf("declared order/snapshot changed: %#v", result.Checks)
	}
}

func TestRuntimePolicyRejectsInvalidOverridesBeforeReads(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy adapter.FamilyPolicy
	}{
		{"multiple singleton namespaces", adapter.FamilyPolicy{Enabled: true, Namespaces: []string{"one", "two"}}},
		{"wildcard namespace", adapter.FamilyPolicy{Enabled: true, Namespaces: []string{"*"}}},
		{"empty name", adapter.FamilyPolicy{Enabled: true, Thresholds: map[string]string{"name": ""}}},
		{"invalid name", adapter.FamilyPolicy{Enabled: true, Thresholds: map[string]string{"name": "../other"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &api.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: "custom"}, Spec: api.AddonDefinitionSpec{AddonType: "custom", AdapterVersion: "1.0.0", SemanticsVersion: 1, Families: []api.DefinitionFamily{{Name: "health", DefaultEnabled: true, Checks: []api.DefinitionCheck{{Name: "controller", Kind: "Workload", Workload: &api.DefinitionWorkload{Target: api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{"default"}}, Kind: "Deployment", DefaultName: "controller", NameThresholdKey: "name"}}}}}}}
			engine, err := declarative.CompileRuntime(context.Background(), d)
			if err != nil {
				t.Fatal(err)
			}
			// A nil client makes any accidental read panic instead of masking fallback.
			_, err = engine.Run(context.Background(), adapter.Request{Policy: map[adapter.Family]adapter.FamilyPolicy{"health": tc.policy}})
			if err == nil {
				t.Fatal("invalid override accepted")
			}
		})
	}
}

func TestRuntimeCompilerRejectsUnsupportedAndCancelledInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := declarative.CompileRuntime(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled compile: %v", err)
	}
	d := &api.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: "custom"}, Spec: api.AddonDefinitionSpec{AddonType: "custom", AdapterVersion: "1.0.0", SemanticsVersion: 2}}
	if _, err := declarative.CompileRuntime(context.Background(), d); err == nil {
		t.Fatal("unsupported semantics accepted")
	}
}
