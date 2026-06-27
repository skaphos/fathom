/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package cilium

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skaphos/fathom/pkg/adapter"
)

// TestRun_EmitsSpan asserts an adapter Run produces a span named "cilium.run"
// carrying the adapter, aggregate outcome, and check-count attributes. Only the
// control_plane_health family is enabled, so a disabled family must not be
// tagged. A nil-injected global provider with an in-memory exporter captures the
// span without envtest.
func TestRun_EmitsSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	req := adapter.Request{
		Client: newFakeClient(t, healthyObjects()...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "kube-system", Name: "cilium"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilyControlPlaneHealth: {Enabled: true},
		},
		Timeout: 5 * time.Second,
	}

	result, err := New().Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Checks) == 0 {
		t.Fatal("expected control_plane_health to emit at least one check")
	}

	spans := exporter.GetSpans()
	var span tracetest.SpanStub
	found := false
	for _, s := range spans {
		if s.Name == Name+".run" {
			span = s
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no %q span recorded; got %d spans", Name+".run", len(spans))
	}

	attrs := map[string]string{}
	for _, kv := range span.Attributes {
		attrs[string(kv.Key)] = kv.Value.String()
	}
	if attrs["fathom.adapter"] != Name {
		t.Errorf("span fathom.adapter = %q, want %q", attrs["fathom.adapter"], Name)
	}
	if _, ok := attrs["fathom.adapter.check_count"]; !ok {
		t.Error("span missing fathom.adapter.check_count attribute")
	}
	// Only control_plane_health is enabled, so its per-family outcome must be
	// present; agent_health must not be.
	// The healthy operator Deployment + pod roll control_plane_health up to Pass.
	if got := attrs["fathom.outcome."+string(FamilyControlPlaneHealth)]; got != string(adapter.OutcomePass) {
		t.Errorf("span fathom.outcome.control_plane_health = %q, want %q", got, adapter.OutcomePass)
	}
	if _, ok := attrs["fathom.outcome."+string(FamilyAgentHealth)]; ok {
		t.Error("span should not tag a disabled family (agent_health)")
	}
}
