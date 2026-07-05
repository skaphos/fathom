/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skaphos/fathom/pkg/adapter"
)

// TestEngine_EmitsSpan asserts the declarative engine's Run produces a span named
// "<addonType>.run" carrying the adapter, check-count, and per-family outcome
// attributes (SKA-293) — the tracing coverage the hand-written cilium adapter
// used to have before it was migrated onto the engine. An in-memory exporter on
// the global provider captures the span without envtest.
func TestEngine_EmitsSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	// Only control_plane_health is enabled, so agent_health / crd_health are
	// skipped and must not be tagged. An empty client makes the operator
	// Deployment absent (Optional -> Skipped), so the family still emits a check.
	req := adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "kube-system", Name: "cilium"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			"control_plane_health": {Enabled: true},
		},
	}

	result, err := NewCiliumEngine().Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Checks) == 0 {
		t.Fatal("expected control_plane_health to emit at least one check")
	}

	var span tracetest.SpanStub
	found := false
	for _, s := range exporter.GetSpans() {
		if s.Name == "cilium.run" {
			span = s
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no \"cilium.run\" span recorded; got %d spans", len(exporter.GetSpans()))
	}

	attrs := map[string]string{}
	for _, kv := range span.Attributes {
		attrs[string(kv.Key)] = kv.Value.String()
	}
	if attrs["fathom.adapter"] != "cilium" {
		t.Errorf("span fathom.adapter = %q, want cilium", attrs["fathom.adapter"])
	}
	if _, ok := attrs["fathom.adapter.check_count"]; !ok {
		t.Error("span missing fathom.adapter.check_count attribute")
	}
	if _, ok := attrs["fathom.outcome.control_plane_health"]; !ok {
		t.Error("span missing fathom.outcome.control_plane_health attribute")
	}
	if _, ok := attrs["fathom.outcome.agent_health"]; ok {
		t.Error("span should not tag a disabled family (agent_health)")
	}
}
