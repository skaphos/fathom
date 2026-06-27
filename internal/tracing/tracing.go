/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package tracing wires OpenTelemetry tracing for the Fathom operator (SKA-293).
//
// Init installs a process-global TracerProvider; reconcilers and adapters then
// obtain tracers from that global provider (via otel.Tracer) so instrumentation
// stays decoupled from this package's construction logic. When tracing is
// disabled, Init installs a no-op provider so span creation on the reconcile and
// adapter-run hot paths costs effectively nothing.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// ServiceName is the value reported as the OpenTelemetry service.name resource
// attribute for every span the operator emits.
const ServiceName = "fathom"

// Config is the resolved tracing configuration. It mirrors app.TracingOptions
// so this package does not import internal/app (which would be a layering
// inversion); the caller translates one into the other.
type Config struct {
	// Enabled gates exporter construction and span recording.
	Enabled bool
	// OTLPEndpoint is the gRPC collector endpoint (host:port). Empty defers to
	// the OTel SDK default and the standard OTEL_EXPORTER_OTLP_* env vars.
	OTLPEndpoint string
	// SamplingRatio is the head-based sampling probability in [0,1].
	SamplingRatio float64
	// Insecure disables transport security to the collector (plaintext gRPC).
	Insecure bool
	// ServiceVersion is reported as the service.version resource attribute.
	ServiceVersion string
}

// ShutdownFunc flushes any buffered spans and releases exporter resources. It is
// safe to call exactly once; callers should invoke it with a bounded context on
// operator shutdown. The disabled path returns a no-op ShutdownFunc so callers
// can defer it unconditionally.
type ShutdownFunc func(context.Context) error

// Init installs a global TracerProvider according to cfg and returns its
// shutdown hook.
//
// When cfg.Enabled is false it installs a no-op provider and returns a no-op
// shutdown — no exporter is created, no collector connection is attempted. When
// enabled it builds an OTLP/gRPC exporter, a resource describing the service, a
// batch span processor, and a parent-based ratio sampler.
//
// The exporter dials the collector lazily, so Init does not block startup or
// fail when no collector is reachable; spans are buffered and dropped if export
// keeps failing.
func Init(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	if !cfg.Enabled {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	opts := []otlptracegrpc.Option{}
	if cfg.OTLPEndpoint != "" {
		opts = append(opts, otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint))
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	// NewSchemaless avoids schema-URL merge conflicts and never errors, so
	// resource construction cannot disable tracing on its own.
	res := resource.NewSchemaless(
		attribute.String("service.name", ServiceName),
		attribute.String("service.version", cfg.ServiceVersion),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SamplingRatio))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
