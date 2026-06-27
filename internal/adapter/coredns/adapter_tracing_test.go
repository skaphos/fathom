/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package coredns

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

// TestRun_EmitsSpan asserts an adapter Run produces a span named "<adapter>.run"
// carrying the adapter, aggregate outcome, and check-count attributes. Only the
// system_health family is enabled so the run stays offline (no probe pods), and
// a nil-injected global provider with an in-memory exporter captures the span
// without envtest.
func TestRun_EmitsSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	req := adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "kube-system", Name: "coredns"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilySystemHealth: {Enabled: true},
		},
		Timeout: 5 * time.Second,
	}

	result, err := New().Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Checks) == 0 {
		t.Fatal("expected system_health to emit at least one check")
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
	// Only system_health is enabled, so its per-family outcome (via
	// adapter.FamilyOutcome) must be present; dns_resolution must not be.
	if _, ok := attrs["fathom.outcome."+string(FamilySystemHealth)]; !ok {
		t.Error("span missing fathom.outcome.system_health attribute")
	}
	if _, ok := attrs["fathom.outcome."+string(FamilyDNSResolution)]; ok {
		t.Error("span should not tag a disabled family (dns_resolution)")
	}
}
