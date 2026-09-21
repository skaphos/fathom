/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative_test

import (
	"context"
	"os"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/declarative"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type runtimePageClient struct {
	client.Client
	calls  int
	expire bool
}

func (c *runtimePageClient) List(_ context.Context, out client.ObjectList, opts ...client.ListOption) error {
	o := new(client.ListOptions).ApplyOptions(opts)
	c.calls++
	list := out.(*unstructured.UnstructuredList)
	if c.calls == 1 {
		list.Items = []unstructured.Unstructured{{Object: map[string]any{"apiVersion": "example.org/v1", "kind": "Widget", "metadata": map[string]any{"name": "first"}, "status": map[string]any{"state": "Ready"}}}}
		list.SetContinue("next")
		return nil
	}
	if c.calls == 2 && o.Continue != "next" {
		panic("missing continuation")
	}
	if c.expire && c.calls == 2 {
		return apierrors.NewResourceExpired("expired")
	}
	list.Items = []unstructured.Unstructured{{Object: map[string]any{"apiVersion": "example.org/v1", "kind": "Widget", "metadata": map[string]any{"name": "last"}, "status": map[string]any{"state": "Broken"}}}}
	return nil
}
func TestRuntimeFieldConsumesFinalPageAndRestartsTransactionally(t *testing.T) {
	for _, expire := range []bool{false, true} {
		data, err := os.ReadFile("../../../config/samples/addondefinition/field.yaml")
		if err != nil {
			t.Fatal(err)
		}
		var d api.AddonDefinition
		if err := yaml.UnmarshalStrict(data, &d); err != nil {
			t.Fatal(err)
		}
		d.Spec.Families[0].DefaultEnabled = true
		engine, err := declarative.CompileRuntime(context.Background(), &d)
		if err != nil {
			t.Fatal(err)
		}
		b, done := execution.NewBudget(context.Background(), 0)
		c := &runtimePageClient{expire: expire}
		result, err := engine.Run(declarative.WithExecutionBudget(b.Context(), b), adapter.Request{Client: c})
		done()
		if err != nil {
			t.Fatal(err)
		}
		want := 2
		if expire {
			want = 1
		}
		if len(result.Checks) != want || result.Checks[want-1].Outcome == adapter.OutcomePass {
			t.Fatalf("partial/obsolete evidence: %+v", result.Checks)
		}
	}
}

type missingRuntimeAPI struct{ client.Client }

func (missingRuntimeAPI) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "example.org", Kind: "Widget"}}
}
func TestRuntimeMissingOptionalAPIIsCompletedSkipped(t *testing.T) {
	data, err := os.ReadFile("../../../config/samples/addondefinition/field.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var d api.AddonDefinition
	if err := yaml.UnmarshalStrict(data, &d); err != nil {
		t.Fatal(err)
	}
	d.Spec.Optional = true
	d.Spec.Families[0].DefaultEnabled = true
	engine, err := declarative.CompileRuntime(context.Background(), &d)
	if err != nil {
		t.Fatal(err)
	}
	b, done := execution.NewBudget(context.Background(), 0)
	defer done()
	result, err := engine.Run(declarative.WithExecutionBudget(b.Context(), b), adapter.Request{Client: missingRuntimeAPI{}})
	if err != nil || len(result.Checks) != 1 || result.Checks[0].Outcome != adapter.OutcomeSkipped {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
