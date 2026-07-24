/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
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
// WorkloadCheck.Kind known, each CRDCheck carrying at least one name, and each
// WebhookCheck naming a known configuration kind with a complete (or absent)
// backing-service reference.
func NewEngine(def AddonDefinition) (*Engine, error) {
	if def.AddonType == "" {
		return nil, fmt.Errorf("declarative: AddonType must not be empty")
	}
	if len(def.Families) == 0 {
		return nil, fmt.Errorf("declarative: adapter %q declares no families", def.AddonType)
	}
	if err := validSupportedVersions(def.SupportedVersions); err != nil {
		return nil, fmt.Errorf("declarative: adapter %q has %w", def.AddonType, err)
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
			if !validWorkloadKind(w.Kind) {
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
		for _, m := range append(append([]ConditionCheck{}, f.ManagedResources...), f.APIServices...) {
			for _, n := range m.Names {
				if n == "" {
					// An empty name would Get "" at run time and surface as a
					// per-run Error — catch the definition bug at construction.
					return nil, fmt.Errorf("declarative: adapter %q family %q has a %s ConditionCheck with an empty name", def.AddonType, f.Name, m.Kind)
				}
			}
		}
		for _, fc := range f.Fields {
			if fc.APIVersion == "" || fc.Kind == "" || fc.ListKind == "" {
				return nil, fmt.Errorf("declarative: adapter %q family %q has a FieldCheck with no APIVersion/Kind/ListKind", def.AddonType, f.Name)
			}
			if len(fc.FieldPath) == 0 {
				// An empty path would score every object as field-absent — a
				// silent definition bug.
				return nil, fmt.Errorf("declarative: adapter %q family %q has a %s FieldCheck with no FieldPath", def.AddonType, f.Name, fc.Kind)
			}
			if fc.ExpectedValue == "" {
				return nil, fmt.Errorf("declarative: adapter %q family %q has a %s FieldCheck with no ExpectedValue", def.AddonType, f.Name, fc.Kind)
			}
		}
		for _, w := range f.Webhooks {
			if w.Kind != KindMutatingWebhookConfiguration && w.Kind != KindValidatingWebhookConfiguration {
				return nil, fmt.Errorf("declarative: adapter %q family %q has a WebhookCheck with unknown kind %q", def.AddonType, f.Name, w.Kind)
			}
			if w.Name == "" {
				return nil, fmt.Errorf("declarative: adapter %q family %q has a WebhookCheck with an empty name", def.AddonType, f.Name)
			}
			if (w.ExpectedService == "") != (w.ServiceNamespace == "") {
				// Half a service reference can never match any entry at run
				// time — catch the definition bug at construction.
				return nil, fmt.Errorf("declarative: adapter %q family %q WebhookCheck %q must set ExpectedService and ServiceNamespace together", def.AddonType, f.Name, w.Name)
			}
		}
		for _, c := range f.CronJobs {
			if c.DefaultName == "" && c.NameThresholdKey == "" {
				return nil, fmt.Errorf("declarative: adapter %q family %q has a CronJobCheck with no name", def.AddonType, f.Name)
			}
			if c.DefaultNamespace == "" {
				// A CronJob is always namespaced; an empty default would Get with an
				// empty namespace when policy names none — a run-time error from a
				// definition bug. Require the fallback namespace at construction.
				return nil, fmt.Errorf("declarative: adapter %q family %q CronJobCheck %q has no DefaultNamespace", def.AddonType, f.Name, c.DefaultName)
			}
		}
		for _, c := range f.ConfigMaps {
			if c.DefaultName == "" && c.NameThresholdKey == "" {
				return nil, fmt.Errorf("declarative: adapter %q family %q has a ConfigMapCheck with no name", def.AddonType, f.Name)
			}
			if c.DefaultNamespace == "" {
				return nil, fmt.Errorf("declarative: adapter %q family %q ConfigMapCheck %q has no DefaultNamespace", def.AddonType, f.Name, c.DefaultName)
			}
			if c.Key == "" {
				// A check with no key would score every ConfigMap as missing its
				// (empty) key — a silent definition bug.
				return nil, fmt.Errorf("declarative: adapter %q family %q ConfigMapCheck %q has no Key", def.AddonType, f.Name, c.DefaultName)
			}
		}
		for _, a := range f.Annotations {
			if a.APIVersion == "" || a.Kind == "" {
				return nil, fmt.Errorf("declarative: adapter %q family %q has an AnnotationStalenessCheck with no APIVersion/Kind", def.AddonType, f.Name)
			}
			if a.AnnotationKey == "" {
				return nil, fmt.Errorf("declarative: adapter %q family %q has an AnnotationStalenessCheck with no AnnotationKey", def.AddonType, f.Name)
			}
			if a.ListKind == "" && a.DefaultName == "" && a.NameThresholdKey == "" {
				return nil, fmt.Errorf("declarative: adapter %q family %q has a named AnnotationStalenessCheck with no name", def.AddonType, f.Name)
			}
			if !a.ClusterScoped && a.DefaultNamespace == "" {
				// A namespaced check (named Get or per-namespace List) would resolve
				// to an empty namespace when policy names none. Cluster-scoped checks
				// (e.g. Nodes) legitimately carry no namespace.
				return nil, fmt.Errorf("declarative: adapter %q family %q has a namespaced AnnotationStalenessCheck with no DefaultNamespace", def.AddonType, f.Name)
			}
		}
	}
	// Validated after the family loop so the reference resolves against families
	// already known to have unique names and valid workload kinds.
	if err := validVersionSource(def); err != nil {
		return nil, err
	}
	return &Engine{def: def}, nil
}

// validVersionSource rejects a VersionSource whose family/component reference
// does not resolve to exactly one WorkloadCheck. Catching it at construction
// keeps a bad definition a build/startup failure rather than an addon that
// silently reports no version (#172).
func validVersionSource(def AddonDefinition) error {
	vs := def.VersionSource
	if vs == nil {
		return nil
	}
	if vs.FromFamily == "" {
		return fmt.Errorf("declarative: adapter %q VersionSource needs a FromFamily", def.AddonType)
	}
	fam, found := def.family(vs.FromFamily)
	if !found {
		return fmt.Errorf("declarative: adapter %q VersionSource references unknown family %q", def.AddonType, vs.FromFamily)
	}
	if len(fam.Workloads) == 0 {
		return fmt.Errorf("declarative: adapter %q VersionSource family %q declares no workloads", def.AddonType, vs.FromFamily)
	}
	if vs.FromComponent == "" && len(fam.Workloads) > 1 {
		return fmt.Errorf("declarative: adapter %q VersionSource family %q declares %d workloads, so FromComponent must select one",
			def.AddonType, vs.FromFamily, len(fam.Workloads))
	}
	if _, _, ok := def.versionWorkload(); !ok {
		return fmt.Errorf("declarative: adapter %q VersionSource component %q is not a workload of family %q",
			def.AddonType, vs.FromComponent, vs.FromFamily)
	}
	return nil
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

// validWorkloadKind reports whether k is one of the controller kinds the engine
// can read (used by both family-workload and VersionSource validation).
func validWorkloadKind(k WorkloadKind) bool {
	switch k {
	case KindDeployment, KindDaemonSet, KindStatefulSet:
		return true
	}
	return false
}

// Name returns the adapter identity (the AddonType).
func (e *Engine) Name() string { return e.def.AddonType }

// Version returns the adapter's own SemVer.
func (e *Engine) Version() string { return e.def.AdapterVersion }

// ContractVersion returns the contract version this engine builds against.
func (e *Engine) ContractVersion() string { return adapter.ContractVersion }

// RBACRules returns the definition's declared least-privilege grants, satisfying
// adapter.RBACDeclarer so the RBAC generator can emit this addon's scoped
// read-only ClusterRole and the reconciler can impersonate its ServiceAccount
// (SKA-58).
func (e *Engine) RBACRules() []adapter.PolicyRule { return e.def.RBAC }

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
	// (Families[0]) targeting the driving AddonCheck. Version detection is
	// skipped too — a fully-disabled check performs no reads.
	if !anyEnabled {
		c := skippedResult(e.def.Families[0].Name, req.Target,
			"all check families are disabled by policy", "FamilyDisabled")
		return adapter.Result{Checks: []adapter.CheckResult{c}, Duration: time.Since(started)}, nil
	}

	// Detect the installed addon version once per run and gate it against the
	// definition's supported range; the Warn gate (if any) leads the checks
	// (SKA-527). req.Policy is threaded in so the version-source address resolves
	// through the referenced family's overrides, exactly as its workload check
	// does (#172).
	detectedVersion, versionGate := e.detectAndGateVersion(ctx, req.Client, req.Policy)
	checks := append([]adapter.CheckResult{}, versionGate...)
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
			Ctx:            ctx,
			Client:         req.Client,
			Logger:         req.Logger.WithValues("family", fr.def.Name),
			Family:         fr.def.Name,
			Policy:         fr.policy,
			DefaultPosture: e.def.defaultPosture(),
		}
		for _, ev := range fr.def.evaluators() {
			out, evErr := ev.Evaluate(ec)
			if evErr != nil {
				// Adapter-level failure: abort the whole Run. The shipped
				// read-and-compare evaluators never take this path.
				return adapter.Result{Checks: checks, Duration: time.Since(started), DetectedVersion: detectedVersion}, evErr
			}
			checks = append(checks, out...)
		}
		// Per-family metric, timed independently and rolled up over only this
		// family's checks (SKA-290).
		metrics.RecordAdapterRun(e.def.AddonType, string(fr.def.Name),
			string(adapter.FamilyOutcome(checks, fr.def.Name)), time.Since(famStart))
	}

	return adapter.Result{Checks: checks, Duration: time.Since(started), DetectedVersion: detectedVersion}, nil
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
