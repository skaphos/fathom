/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package tracing

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

// restoreGlobalProvider snapshots and restores the process-global tracer
// provider AND text-map propagator (Init sets both) so these tests do not leak
// the no-op/SDK provider or the propagator they install into the rest of the
// suite, which would otherwise make other tests order-dependent.
func restoreGlobalProvider(t *testing.T) {
	t.Helper()
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
}

func TestInit_DisabledInstallsNoopProvider(t *testing.T) {
	restoreGlobalProvider(t)

	shutdown, err := Init(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatalf("Init(disabled): %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init returned a nil shutdown func")
	}

	// A no-op provider yields non-recording spans with an invalid span context;
	// this is the property that keeps the hot path free when tracing is off.
	_, span := otel.Tracer("test").Start(context.Background(), "noop")
	if span.IsRecording() {
		t.Error("span from disabled tracing should not be recording")
	}
	if span.SpanContext().IsValid() {
		t.Error("span from disabled tracing should not have a valid span context")
	}
	span.End()

	if err := shutdown(context.Background()); err != nil {
		t.Errorf("disabled shutdown should be a no-op, got: %v", err)
	}
}

func TestInit_EnabledInstallsRecordingProvider(t *testing.T) {
	restoreGlobalProvider(t)

	// Insecure + an arbitrary endpoint: the gRPC exporter dials lazily, so Init
	// succeeds without a collector and we can exercise the enabled path offline.
	shutdown, err := Init(context.Background(), Config{
		Enabled:        true,
		OTLPEndpoint:   "localhost:4317",
		SamplingRatio:  1.0,
		Insecure:       true,
		ServiceVersion: "test",
	})
	if err != nil {
		t.Fatalf("Init(enabled): %v", err)
	}
	t.Cleanup(func() {
		// Best-effort, bounded shutdown: no collector is reachable in a unit
		// test, so flushing the batched span would otherwise block on export
		// retries. We only need to assert the enabled provider records spans.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = shutdown(shutdownCtx)
	})

	_, span := otel.Tracer("test").Start(context.Background(), "recorded")
	if !span.IsRecording() {
		t.Error("span from enabled tracing should be recording")
	}
	if !span.SpanContext().IsValid() {
		t.Error("span from enabled tracing should have a valid span context")
	}
	span.End()
}
