/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative_test

import (
	"context"
	"os"
	"strings"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/declarative"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/internal/adapter/runtime/testutil"
	"github.com/skaphos/fathom/pkg/adapter"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func TestRuntimeConfigMapParserBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		reject      bool
	}{
		{"bytes at", "value: " + strings.Repeat("x", limits.MaxYAMLBytes-7), false},
		{"bytes over", "value: " + strings.Repeat("x", limits.MaxYAMLBytes-6), true},
		{"nodes at", "items:\n" + strings.Repeat("- x\n", limits.MaxYAMLNodes-4), false},
		{"nodes over", "items:\n" + strings.Repeat("- x\n", limits.MaxYAMLNodes-3), true},
		{"depth at", "value: " + testutil.YAMLDepth(limits.MaxYAMLDepth-2), false},
		{"depth over", "value: " + testutil.YAMLDepth(limits.MaxYAMLDepth-1), true},
		{"alias", "value: &x [a]\nother: *x\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile("../../../config/samples/addondefinition/configmap.yaml")
			if err != nil {
				t.Fatal(err)
			}
			var d api.AddonDefinition
			if err := yaml.UnmarshalStrict(data, &d); err != nil {
				t.Fatal(err)
			}
			d.Spec.Families[0].DefaultEnabled = true
			payload := d.Spec.Families[0].Checks[0].ConfigMap
			scheme := runtime.NewScheme()
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: string(payload.DefaultName), Namespace: string(payload.Target.Namespaces[0])}, Data: map[string]string{payload.Key: tc.value}}).Build()
			engine, err := declarative.CompileRuntime(context.Background(), &d)
			if err != nil {
				t.Fatal(err)
			}
			b, done := execution.NewBudget(context.Background(), 0)
			defer done()
			_, err = engine.Run(declarative.WithExecutionBudget(b.Context(), b), adapter.Request{Client: c})
			if tc.reject {
				if err == nil || !strings.Contains(err.Error(), "InputLimitExceeded") {
					t.Fatalf("unsafe payload accepted: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeAnnotationByteBoundary(t *testing.T) {
	for _, size := range []int{limits.MaxAnnotationBytes, limits.MaxAnnotationBytes + 1} {
		data, err := os.ReadFile("../../../config/samples/addondefinition/annotationstaleness.yaml")
		if err != nil {
			t.Fatal(err)
		}
		var d api.AddonDefinition
		if err := yaml.UnmarshalStrict(data, &d); err != nil {
			t.Fatal(err)
		}
		d.Spec.Families[0].DefaultEnabled = true
		p := d.Spec.Families[0].Checks[0].AnnotationStaleness
		p.TimestampJSONField = "time"
		value := `{"time":"2000-01-01T00:00:00Z","padding":""}`
		value = strings.Replace(value, `"padding":""`, `"padding":"`+strings.Repeat("x", size-len(value))+`"`, 1)
		scheme := runtime.NewScheme()
		if err := corev1.AddToScheme(scheme); err != nil {
			t.Fatal(err)
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: string(p.DefaultName), Namespace: string(p.Target.Namespaces[0]), Annotations: map[string]string{p.AnnotationKey: value}}}).Build()
		engine, err := declarative.CompileRuntime(context.Background(), &d)
		if err != nil {
			t.Fatal(err)
		}
		b, done := execution.NewBudget(context.Background(), 0)
		_, err = engine.Run(declarative.WithExecutionBudget(b.Context(), b), adapter.Request{Client: c})
		done()
		if (err != nil) != (size > limits.MaxAnnotationBytes) {
			t.Fatalf("size=%d err=%v", size, err)
		}
	}
}

func TestRuntimeEvaluatorAndVersionHelperShareVisitBudget(t *testing.T) {
	for _, versionHelper := range []bool{false, true} {
		data, err := os.ReadFile("../../../config/samples/addondefinition/workload.yaml")
		if err != nil {
			t.Fatal(err)
		}
		var d api.AddonDefinition
		if err := yaml.UnmarshalStrict(data, &d); err != nil {
			t.Fatal(err)
		}
		d.Spec.Families[0].DefaultEnabled = true
		if versionHelper {
			d.Spec.VersionSource = &api.DefinitionVersionSource{FromFamily: "health", FromComponent: "workload"}
		}
		scheme := runtime.NewScheme()
		if err := appsv1.AddToScheme(scheme); err != nil {
			t.Fatal(err)
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "controller", Namespace: "default"}}).Build()
		engine, err := declarative.CompileRuntime(context.Background(), &d)
		if err != nil {
			t.Fatal(err)
		}
		b, done := execution.NewBudget(context.Background(), 0)
		if err := b.Visit(limits.MaxObjectVisits); err != nil {
			t.Fatal(err)
		}
		result, err := engine.Run(declarative.WithExecutionBudget(b.Context(), b), adapter.Request{Client: c})
		done()
		if err == nil || !strings.Contains(err.Error(), "WorkLimitExceeded") || len(result.Checks) != 0 {
			t.Fatalf("helper=%v result=%+v err=%v", versionHelper, result, err)
		}
	}
}

func TestScopedRuntimeRequiresSharedBudgetBeforeIO(t *testing.T) {
	data, err := os.ReadFile("../../../config/samples/addondefinition/configmap.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var d api.AddonDefinition
	if err := yaml.UnmarshalStrict(data, &d); err != nil {
		t.Fatal(err)
	}
	d.Spec.Families[0].DefaultEnabled = true
	engine, err := declarative.CompileRuntimeScoped(context.Background(), &d, api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"default"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.Run(context.Background(), adapter.Request{})
	if err == nil || !strings.Contains(err.Error(), "AuthorizationUnavailable") {
		t.Fatalf("missing budget accepted: %v", err)
	}
}
