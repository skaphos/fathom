/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/pkg/adapter"
)

// tracer is the OpenTelemetry instrumentation scope for the declarative engine.
// It draws from the global provider, so it is a no-op unless the operator
// enabled tracing (SKA-293).
var tracer = otel.Tracer("github.com/skaphos/fathom/internal/adapter/declarative")

// Engine implements pkg/adapter.Adapter from an immutable AddonDefinition.
//
// The Engine holds only the definition; every per-Run datum is stack-local, so
// Run is safe for concurrent invocation from many goroutines (the contract's
// concurrency requirement).
type Engine struct {
	def AddonDefinition
}

// NewEngine validates def and returns an Engine implementing pkg/adapter.Adapter.
//
// Validation catches programmer errors at manager startup (before Register):
// non-empty AddonType, non-empty Families, unique family names, each
// WorkloadCheck.Kind known, and each CRDCheck carrying at least one name.
func NewEngine(def AddonDefinition) (*Engine, error) {
	if def.AddonType == "" {
		return nil, fmt.Errorf("declarative: AddonType must not be empty")
	}
	if len(def.Families) == 0 {
		return nil, fmt.Errorf("declarative: adapter %q declares no families", def.AddonType)
	}
	seen := make(map[adapter.Family]struct{}, len(def.Families))
	for _, f := range def.Families {
		if f.Name == "" {
			return nil, fmt.Errorf("declarative: adapter %q has a family with an empty name", def.AddonType)
		}
		if _, dup := seen[f.Name]; dup {
			return nil, fmt.Errorf("declarative: adapter %q declares duplicate family %q", def.AddonType, f.Name)
		}
		seen[f.Name] = struct{}{}
		for _, w := range f.Workloads {
			switch w.Kind {
			case KindDeployment, KindDaemonSet, KindStatefulSet:
			default:
				return nil, fmt.Errorf("declarative: adapter %q family %q has workload with unknown kind %q", def.AddonType, f.Name, w.Kind)
			}
		}
		for _, c := range f.CRDs {
			if len(c.Names) == 0 {
				return nil, fmt.Errorf("declarative: adapter %q family %q has a CRDCheck with no names", def.AddonType, f.Name)
			}
			if len(c.SupportedVersions) == 0 {
				// An empty list makes crdutil.PreferredServedVersion always
				// return !ok, so every established CRD would be scored as
				// "serves no recognized version" — a silent definition bug.
				return nil, fmt.Errorf("declarative: adapter %q family %q CRDCheck %v has no SupportedVersions", def.AddonType, f.Name, c.Names)
			}
		}
	}
	return &Engine{def: def}, nil
}

// MustEngine wraps NewEngine and panics on validation error. It is intended for
// package-level construction from a compiled-in, statically-valid definition.
func MustEngine(def AddonDefinition) *Engine {
	e, err := NewEngine(def)
	if err != nil {
		panic(err)
	}
	return e
}

// Name returns the adapter identity (the AddonType).
func (e *Engine) Name() string { return e.def.AddonType }

// Version returns the adapter's own SemVer.
func (e *Engine) Version() string { return e.def.AdapterVersion }

// ContractVersion returns the contract version this engine builds against.
func (e *Engine) ContractVersion() string { return adapter.ContractVersion }

// Capabilities advertises the addon type and the declared families.
func (e *Engine) Capabilities() adapter.Capabilities {
	fams := make([]adapter.Family, len(e.def.Families))
	for i, f := range e.def.Families {
		fams[i] = f.Name
	}
	return adapter.Capabilities{AddonTypes: []string{e.def.AddonType}, Families: fams}
}

// Run executes the definition's enabled families in order and returns their
// aggregated CheckResults. It mirrors the hand-written cilium adapter's Run,
// generalized over the definition.
func (e *Engine) Run(ctx context.Context, req adapter.Request) (result adapter.Result, err error) {
	ctx, span := tracer.Start(ctx, e.def.AddonType+".run")
	span.SetAttributes(attribute.String("fathom.adapter", e.def.AddonType))
	defer func() { endRunSpan(span, result, err) }()

	started := time.Now()

	type famRun struct {
		def     FamilyDefinition
		policy  adapter.FamilyPolicy
		enabled bool
	}
	runs := make([]famRun, len(e.def.Families))
	anyEnabled := false
	for i, f := range e.def.Families {
		p, on := resolveFamily(req.Policy, f.Name, f.DefaultEnabled)
		runs[i] = famRun{def: f, policy: p, enabled: on}
		anyEnabled = anyEnabled || on
	}

	// All-disabled sentinel: a single Skipped under the primary family
	// (Families[0]) targeting the driving AddonCheck.
	if !anyEnabled {
		c := skippedResult(e.def.Families[0].Name, req.Target,
			"all check families are disabled by policy", "FamilyDisabled")
		return adapter.Result{Checks: []adapter.CheckResult{c}, Duration: time.Since(started)}, nil
	}

	checks := []adapter.CheckResult{}
	for _, fr := range runs {
		if !fr.enabled {
			continue
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			// Honor cancellation between families; already-collected checks are
			// returned as-is, without an adapter-level error.
			break
		}
		famStart := time.Now()
		ec := EvalContext{
			Ctx:    ctx,
			Client: req.Client,
			Logger: req.Logger.WithValues("family", fr.def.Name),
			Family: fr.def.Name,
			Policy: fr.policy,
		}
		for _, ev := range fr.def.evaluators() {
			out, evErr := ev.Evaluate(ec)
			if evErr != nil {
				// Adapter-level failure: abort the whole Run. The shipped
				// read-and-compare evaluators never take this path.
				return adapter.Result{Checks: checks, Duration: time.Since(started)}, evErr
			}
			checks = append(checks, out...)
		}
		// Per-family metric, timed independently and rolled up over only this
		// family's checks (SKA-290).
		metrics.RecordAdapterRun(e.def.AddonType, string(fr.def.Name),
			string(adapter.FamilyOutcome(checks, fr.def.Name)), time.Since(famStart))
	}

	return adapter.Result{Checks: checks, Duration: time.Since(started)}, nil
}

// endRunSpan annotates span with the check count and a per-family outcome
// (the same FamilyOutcome roll-up the per-family metrics use), records err, and
// ends the span. Only families that produced checks are tagged.
func endRunSpan(span trace.Span, result adapter.Result, err error) {
	span.SetAttributes(attribute.Int("fathom.adapter.check_count", len(result.Checks)))
	seen := map[adapter.Family]struct{}{}
	for _, c := range result.Checks {
		if _, ok := seen[c.Family]; ok {
			continue
		}
		seen[c.Family] = struct{}{}
		span.SetAttributes(attribute.String(
			"fathom.outcome."+string(c.Family),
			string(adapter.FamilyOutcome(result.Checks, c.Family)),
		))
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
