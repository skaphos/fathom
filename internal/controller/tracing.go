/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracerScope is the OpenTelemetry instrumentation scope shared by the
// reconcilers in this package (SKA-293). Exported so the manager wiring in
// internal/app can hand each reconciler a tracer from the same scope.
const TracerScope = "github.com/skaphos/fathom/internal/controller"

// reconcilerTracer resolves the tracer a reconciler should use. A nil field
// (reconciler constructed without an injected tracer, e.g. in envtest specs)
// falls back to the global provider, which is a no-op unless the operator
// enabled tracing — so the reconcile hot path stays allocation-light when off.
func reconcilerTracer(t trace.Tracer) trace.Tracer {
	if t != nil {
		return t
	}
	return otel.Tracer(TracerScope)
}

// endReconcileSpan records err on span (mapping it to the Error status) and ends
// the span. Centralizing it keeps each Reconcile's deferred cleanup to one line.
func endReconcileSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
