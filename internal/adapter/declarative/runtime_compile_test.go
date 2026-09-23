/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/declarative"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	result, err := engine.Run(runtimeTestContext(t), adapter.Request{Client: fake.NewClientBuilder().WithScheme(scheme).Build()})
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range result.Checks {
		if check.Outcome != adapter.OutcomeSkipped {
			t.Fatalf("optional missing target must inherit Skipped: %+v", check)
		}
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
			_, err = engine.Run(runtimeTestContext(t), adapter.Request{Policy: map[adapter.Family]adapter.FamilyPolicy{"health": tc.policy}})
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

func runtimeVersionDefinition() *api.AddonDefinition {
	return &api.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: "custom"}, Spec: api.AddonDefinitionSpec{AddonType: "custom", AdapterVersion: "1.0.0", SemanticsVersion: 1, SupportedVersions: ">=1.0.0", VersionSource: &api.DefinitionVersionSource{FromFamily: "health", FromComponent: "controller"}, Families: []api.DefinitionFamily{{Name: "health", DefaultEnabled: true, Checks: []api.DefinitionCheck{{Name: "controller", Kind: "Workload", Workload: &api.DefinitionWorkload{Target: api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{"default"}}, Kind: "Deployment", DefaultName: "controller"}}}}}}}
}

func TestRuntimeVersionSourceMustResolveExactlyOnce(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(*api.AddonDefinition)
		valid bool
	}{
		{"default component", func(*api.AddonDefinition) {}, true},
		{"explicit component", func(d *api.AddonDefinition) {
			d.Spec.Families[0].Checks[0].Workload.Component = "selected"
			d.Spec.VersionSource.FromComponent = "selected"
		}, true},
		{"missing family", func(d *api.AddonDefinition) { d.Spec.VersionSource.FromFamily = "missing" }, false},
		{"missing component", func(d *api.AddonDefinition) { d.Spec.VersionSource.FromComponent = "missing" }, false},
		{"missing reference", func(d *api.AddonDefinition) { d.Spec.VersionSource = nil }, false},
		{"ambiguous component", func(d *api.AddonDefinition) {
			c := d.Spec.Families[0].Checks[0]
			c.Name = "second"
			c.Workload = c.Workload.DeepCopy()
			c.Workload.Component = "controller"
			d.Spec.Families[0].Checks = append(d.Spec.Families[0].Checks, c)
		}, false},
		{"detect only", func(d *api.AddonDefinition) { d.Spec.SupportedVersions = "" }, true},
		{"nonworkload source", func(d *api.AddonDefinition) {
			d.Spec.Families[0].Checks[0] = api.DefinitionCheck{Name: "controller", Kind: "ConfigMap", ConfigMap: &api.DefinitionConfigMap{Target: api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{"default"}}, DefaultName: "controller", Key: "config"}}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := runtimeVersionDefinition()
			tc.edit(d)
			if _, err := declarative.CompileRuntime(context.Background(), d); (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestRuntimeScopeAppliesAfterPolicyResolutionBeforeReads(t *testing.T) {
	d := runtimeVersionDefinition()
	d.Spec.VersionSource = nil
	d.Spec.SupportedVersions = ""
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, namespace string
		valid           bool
	}{
		{"default unauthorized", "", false}, {"override authorized", "authorized", true}, {"override unauthorized", "other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, err := declarative.CompileRuntimeScoped(context.Background(), d, api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"authorized"}})
			if err != nil {
				t.Fatal(err)
			}
			req := adapter.Request{}
			if tc.namespace != "" {
				req.Policy = map[adapter.Family]adapter.FamilyPolicy{"health": {Enabled: true, Namespaces: []string{tc.namespace}}}
			}
			// Invalid scope must fail before accessing the nil client. The valid branch
			// uses a fake reader and verifies the effective namespace in its observation.
			if tc.valid {
				req.Client = fake.NewClientBuilder().WithScheme(scheme).Build()
			}
			result, err := engine.Run(runtimeTestContext(t), req)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && (len(result.Checks) != 1 || result.Checks[0].TargetRef.Namespace != "authorized") {
				t.Fatalf("override not used: %+v", result)
			}
		})
	}
}

// helperAReadCounter proves the refusal lands before any cluster read.
type helperAReadCounter struct {
	client.Client
	gets int
}

func (c *helperAReadCounter) Get(ctx context.Context, key client.ObjectKey, out client.Object, opts ...client.GetOption) error {
	c.gets++
	return c.Client.Get(ctx, key, out, opts...)
}

// Without the shared budget nothing enforces the per-object node and depth caps
// or the evidence cap, so every runtime execution must be refused before any
// read -- scoped or not.
func TestRuntimeExecutionRequiresSharedBudget(t *testing.T) {
	// A definition with no version source has no pre-step read, so Run's own
	// refusal is indistinguishable from the identical one in runtimeStep.Evaluate.
	// Retaining the version source keeps Engine.detectAndGateVersion in play: it
	// reads before any step runs and bypasses the workClient wrapper, so only the
	// fail-closed check at the top of Run can keep the read count at zero.
	versionless := func() *api.AddonDefinition {
		d := runtimeVersionDefinition()
		d.Spec.VersionSource = nil
		d.Spec.SupportedVersions = ""
		return d
	}
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		definition func() *api.AddonDefinition
		compile    func(*api.AddonDefinition) (adapter.Adapter, error)
	}{
		{"unscoped", versionless, func(d *api.AddonDefinition) (adapter.Adapter, error) {
			return declarative.CompileRuntime(context.Background(), d)
		}},
		{"scoped", versionless, func(d *api.AddonDefinition) (adapter.Adapter, error) {
			return declarative.CompileRuntimeScoped(context.Background(), d, api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"default"}})
		}},
		{"unscoped with version source", runtimeVersionDefinition, func(d *api.AddonDefinition) (adapter.Adapter, error) {
			return declarative.CompileRuntime(context.Background(), d)
		}},
		{"scoped with version source", runtimeVersionDefinition, func(d *api.AddonDefinition) (adapter.Adapter, error) {
			return declarative.CompileRuntimeScoped(context.Background(), d, api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"default"}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, err := tc.compile(tc.definition())
			if err != nil {
				t.Fatal(err)
			}
			c := &helperAReadCounter{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "controller", Namespace: "default"}}).Build()}
			result, err := engine.Run(context.Background(), adapter.Request{Client: c})
			if err == nil || !strings.Contains(err.Error(), "AuthorizationUnavailable") {
				t.Fatalf("unbudgeted run accepted: result=%+v err=%v", result, err)
			}
			if c.gets != 0 {
				t.Fatalf("unbudgeted run performed %d reads", c.gets)
			}
		})
	}
}

func runtimeTestContext(t *testing.T) context.Context {
	t.Helper()
	b, done := execution.NewBudget(context.Background(), 0)
	t.Cleanup(done)
	return declarative.WithExecutionBudget(b.Context(), b)
}
