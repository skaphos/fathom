/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// installInMemoryTracer swaps the global tracer provider for a synchronous,
// in-memory SDK provider and returns the exporter holding the recorded spans.
// The previous provider is restored on cleanup so this does not leak into the
// envtest-backed Ginkgo suite in the same package.
func installInMemoryTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return exporter
}

func spanByName(spans tracetest.SpanStubs, name string) (tracetest.SpanStub, bool) {
	for _, s := range spans {
		if s.Name == name {
			return s, true
		}
	}
	return tracetest.SpanStub{}, false
}

func attrValue(s tracetest.SpanStub, key string) (string, bool) {
	for _, kv := range s.Attributes {
		if string(kv.Key) == key {
			return kv.Value.AsString(), true
		}
	}
	return "", false
}

func newControllerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add fathom scheme: %v", err)
	}
	return scheme
}

// TestHealthCheckReconcile_EmitsSpan verifies a reconcile produces a span named
// for its kind, carrying the namespace/name attributes. A nil Tracer field
// exercises the global-provider fallback (the production default path).
func TestHealthCheckReconcile_EmitsSpan(t *testing.T) {
	exporter := installInMemoryTracer(t)
	scheme := newControllerScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &HealthCheckReconciler{Client: cl, Scheme: scheme}
	// The object does not exist: the reconcile takes the NotFound fast path and
	// returns cleanly, but the span is opened before the Get and is still
	// recorded — exactly the instrumentation we want to assert.
	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "team-a", Name: "web"},
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	spans := exporter.GetSpans()
	span, ok := spanByName(spans, "healthcheck.reconcile")
	if !ok {
		t.Fatalf("no healthcheck.reconcile span recorded; got %d spans", len(spans))
	}
	for key, want := range map[string]string{
		"fathom.kind":      "HealthCheck",
		"fathom.namespace": "team-a",
		"fathom.name":      "web",
	} {
		if got, ok := attrValue(span, key); !ok || got != want {
			t.Errorf("span attribute %q = %q (present=%v), want %q", key, got, ok, want)
		}
	}
}

// TestClusterHealthReconcile_EmitsSpan covers the second reconciler so the
// per-kind span naming is exercised for more than one controller.
func TestClusterHealthReconcile_EmitsSpan(t *testing.T) {
	exporter := installInMemoryTracer(t)
	scheme := newControllerScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &ClusterHealthReconciler{Client: cl, Scheme: scheme}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "all"},
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if _, ok := spanByName(exporter.GetSpans(), "clusterhealth.reconcile"); !ok {
		t.Fatal("no clusterhealth.reconcile span recorded")
	}
}
