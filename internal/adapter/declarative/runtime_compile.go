/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"fmt"
	"time"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/pkg/adapter"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
)

// runtimeAdapter keeps a private wire snapshot. Run validates and resolves every
// policy before version discovery or any evaluator can make a read. The caller
// must still provide the scoped, budgeted runtime client and publication fences.
type runtimeAdapter struct {
	source    *api.AddonDefinition
	prototype *Engine
	scope     *api.DefinitionBindingScope
}

// CompileRuntime converts an admitted or offline definition without cluster I/O.
// It snapshots all caller-owned collections and preserves declared check order.
func CompileRuntime(ctx context.Context, source *api.AddonDefinition) (adapter.Adapter, error) {
	return compileRuntime(ctx, source, nil)
}

// CompileRuntimeScoped enforces the binding allowlist after policy resolution,
// before version detection or evaluator reads. Transport still enforces actual
// discovered scope and every helper request independently.
func CompileRuntimeScoped(ctx context.Context, source *api.AddonDefinition, scope api.DefinitionBindingScope) (adapter.Adapter, error) {
	if err := definitions.ValidateBindingScope(scope); err != nil {
		return nil, err
	}
	return compileRuntime(ctx, source, scope.DeepCopy())
}

func compileRuntime(ctx context.Context, source *api.AddonDefinition, scope *api.DefinitionBindingScope) (*runtimeAdapter, error) {
	ctx, cancel := context.WithTimeout(ctx, definitions.MaxCompileDuration)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := definitions.Validate(source); err != nil {
		return nil, err
	}
	snapshot := source.DeepCopy()
	engine, err := lowerRuntime(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	return &runtimeAdapter{source: snapshot, prototype: engine, scope: scope}, nil
}
func (r *runtimeAdapter) ContractVersion() string            { return adapter.ContractVersion }
func (r *runtimeAdapter) Name() string                       { return r.prototype.Name() }
func (r *runtimeAdapter) Version() string                    { return r.prototype.Version() }
func (r *runtimeAdapter) Capabilities() adapter.Capabilities { return r.prototype.Capabilities() }
func (r *runtimeAdapter) Run(ctx context.Context, req adapter.Request) (adapter.Result, error) {
	timeout := definitions.MaxRunDuration
	if req.Timeout > 0 && req.Timeout < timeout {
		timeout = req.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resolved := r.source.DeepCopy()
	if err := resolveRuntimePolicy(ctx, resolved, req.Policy); err != nil {
		return adapter.Result{}, err
	}
	if r.scope != nil {
		if err := definitions.ValidateScope(resolved, *r.scope); err != nil {
			return adapter.Result{}, err
		}
	}
	engine, err := lowerRuntime(ctx, resolved)
	if err != nil {
		return adapter.Result{}, err
	}
	b, _ := ctx.Value(executionBudgetKey{}).(ExecutionBudget)
	if r.scope != nil && b == nil {
		return adapter.Result{}, fmt.Errorf("AuthorizationUnavailable: scoped runtime execution requires a shared budget")
	}
	if b != nil {
		if err := b.Err(); err != nil {
			return adapter.Result{}, err
		}
		req.Client = workClient{Client: req.Client, work: b}
	}
	result, err := engine.Run(ctx, req)
	if b != nil && b.Err() != nil {
		return adapter.Result{}, b.Err()
	}
	if ctx.Err() != nil {
		return adapter.Result{}, ctx.Err()
	}
	if err != nil {
		return adapter.Result{}, err
	}
	if b != nil {
		if err := b.ValidateResult(result); err != nil {
			return adapter.Result{}, err
		}
	}
	return result, nil
}

// runtimeStep supplies explicit namespaces even when the family has no override;
// the existing built-in collection evaluators otherwise interpret empty as all.
type runtimeStep struct {
	evaluator  Evaluator
	namespaces []string
}

func (s runtimeStep) Evaluate(ec EvalContext) ([]adapter.CheckResult, error) {
	ec.runtime = true
	ec.Policy.Namespaces = append([]string(nil), s.namespaces...)
	// All named threshold overrides have already been validated and resolved.
	ec.Policy.Thresholds = nil
	if err := ec.Ctx.Err(); err != nil {
		return nil, err
	}
	out, err := s.evaluator.Evaluate(ec)
	if b := ec.budget(); b != nil && b.Err() != nil {
		return nil, b.Err()
	}
	if err != nil {
		return nil, err
	}
	for _, result := range out {
		if result.Outcome == adapter.OutcomeError {
			return nil, fmt.Errorf("runtime evaluator failed: %s", result.Summary)
		}
	}
	return out, nil
}

func lowerRuntime(ctx context.Context, source *api.AddonDefinition) (*Engine, error) {
	def := AddonDefinition{AddonType: string(source.Spec.AddonType), AdapterVersion: source.Spec.AdapterVersion, Optional: source.Spec.Optional, SupportedVersions: source.Spec.SupportedVersions}
	if vs := source.Spec.VersionSource; vs != nil {
		def.VersionSource = &VersionSource{FromFamily: adapter.Family(vs.FromFamily), FromComponent: string(vs.FromComponent), Container: string(vs.Container)}
	}
	for _, f := range source.Spec.Families {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		family := FamilyDefinition{Name: adapter.Family(f.Name), DefaultEnabled: f.DefaultEnabled}
		for _, check := range f.Checks {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			ev, scope := lowerCheck(check)
			family.ordered = append(family.ordered, runtimeStep{evaluator: ev, namespaces: scope})
			// Existing constructor validation and versionSource lookup use the buckets;
			// only the explicit ordered sequence is dispatched for runtime definitions.
			switch v := ev.(type) {
			case WorkloadCheck:
				family.Workloads = append(family.Workloads, v)
			case CRDCheck:
				family.CRDs = append(family.CRDs, v)
			case ConditionCheck:
				family.ManagedResources = append(family.ManagedResources, v)
			case FieldCheck:
				family.Fields = append(family.Fields, v)
			case WebhookCheck:
				family.Webhooks = append(family.Webhooks, v)
			case CronJobCheck:
				family.CronJobs = append(family.CronJobs, v)
			case ConfigMapCheck:
				family.ConfigMaps = append(family.ConfigMaps, v)
			case AnnotationStalenessCheck:
				family.Annotations = append(family.Annotations, v)
			case PodProjectionCheck:
				family.PodProjections = append(family.PodProjections, v)
			}
		}
		def.Families = append(def.Families, family)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	engine, err := NewEngine(def)
	if err != nil {
		return nil, err
	}
	engine.runtime = true
	return engine, nil
}

func lowerCheck(c api.DefinitionCheck) (Evaluator, []string) {
	switch c.Kind {
	case "Workload":
		p := c.Workload
		ns := runtimeNamespaces(p.Target)
		return WorkloadCheck{
			Kind:                    WorkloadKind(p.Kind),
			DefaultNamespace:        firstRuntimeNamespace(ns),
			NameThresholdKey:        "",
			DefaultName:             string(p.DefaultName),
			Component:               runtimeLabel(string(p.Component), string(c.Name)),
			Absence:                 Posture(p.Absence),
			CheckPods:               p.CheckPods,
			RestartWarnThresholdKey: "",
			DefaultRestartWarn:      p.DefaultRestartWarn,
		}, ns
	case "CRD":
		p := c.CRD
		ns := runtimeNamespaces(p.Target)
		return CRDCheck{
			Names:                     runtimeStrings(p.Names),
			SupportedVersions:         runtimeStrings(p.SupportedVersions),
			Absence:                   Posture(p.Absence),
			UnsupportedVersionOutcome: adapter.Outcome(p.UnsupportedVersionOutcome),
		}, ns
	case "Condition":
		p := c.Condition
		ns := runtimeNamespaces(p.Target)
		return ConditionCheck{
			APIVersion:        p.APIVersion,
			VersionCRD:        string(p.VersionCRD),
			SupportedVersions: runtimeStrings(p.SupportedVersions),
			Kind:              string(p.Kind),
			ListKind:          string(p.ListKind),
			ListName:          runtimeLabel(string(p.ListName), string(c.Name)),
			Names:             runtimeStrings(p.Names),
			Absence:           Posture(p.Absence),
			ClusterScoped:     p.Target.Scope == "Cluster",
			DefaultNamespace:  firstRuntimeNamespace(ns),
			ConditionType:     p.ConditionType,
			ExpectedStatus:    p.ExpectedStatus,
			AbsentCondition:   adapter.Outcome(p.AbsentCondition),
			Mismatch:          adapter.Outcome(p.Mismatch),
		}, ns
	case "Field":
		p := c.Field
		ns := runtimeNamespaces(p.Target)
		return FieldCheck{
			APIVersion:    p.APIVersion,
			Kind:          string(p.Kind),
			ListKind:      string(p.ListKind),
			ListName:      runtimeLabel(string(p.ListName), string(c.Name)),
			ClusterScoped: p.Target.Scope == "Cluster",
			Absence:       Posture(p.Absence),
			FieldPath:     runtimeStrings(p.FieldPath),
			ExpectedValue: string(p.ExpectedValue),
			ValueOutcomes: runtimeOutcomes(p.ValueOutcomes),
			AbsentOutcome: adapter.Outcome(p.AbsentOutcome),
			OtherOutcome:  adapter.Outcome(p.OtherOutcome),
		}, ns
	case "Webhook":
		p := c.Webhook
		ns := runtimeNamespaces(p.Target)
		return WebhookCheck{
			Kind:             p.Kind,
			Name:             string(p.Name),
			NameThresholdKey: "",
			ExpectedService:  string(p.ExpectedService),
			ServiceNamespace: string(p.ServiceNamespace),
			Absence:          Posture(p.Absence),
			VerifyEndpoints:  p.VerifyEndpoints,
		}, ns
	case "CronJob":
		p := c.CronJob
		ns := runtimeNamespaces(p.Target)
		return CronJobCheck{
			DefaultNamespace:          firstRuntimeNamespace(ns),
			NameThresholdKey:          "",
			DefaultName:               string(p.DefaultName),
			Component:                 runtimeLabel(string(p.Component), string(c.Name)),
			Absence:                   Posture(p.Absence),
			SuccessMaxAgeThresholdKey: "",
			DefaultSuccessMaxAge:      runtimeDuration(p.DefaultSuccessMaxAge),
			StaleOutcome:              adapter.Outcome(p.StaleOutcome),
		}, ns
	case "ConfigMap":
		p := c.ConfigMap
		ns := runtimeNamespaces(p.Target)
		return ConfigMapCheck{
			DefaultNamespace:      firstRuntimeNamespace(ns),
			NameThresholdKey:      "",
			DefaultName:           string(p.DefaultName),
			Component:             runtimeLabel(string(p.Component), string(c.Name)),
			Absence:               Posture(p.Absence),
			Key:                   p.Key,
			RecognizedAPIVersions: runtimeStrings(p.RecognizedAPIVersions),
			UnrecognizedOutcome:   adapter.Outcome(p.UnrecognizedOutcome),
			InvalidOutcome:        adapter.Outcome(p.InvalidOutcome),
		}, ns
	case "AnnotationStaleness":
		p := c.AnnotationStaleness
		ns := runtimeNamespaces(p.Target)
		return AnnotationStalenessCheck{
			APIVersion:         p.APIVersion,
			Kind:               string(p.Kind),
			ListKind:           string(p.ListKind),
			ListName:           runtimeLabel(string(p.ListName), string(c.Name)),
			ClusterScoped:      p.Target.Scope == "Cluster",
			DefaultNamespace:   firstRuntimeNamespace(ns),
			NameThresholdKey:   "",
			DefaultName:        string(p.DefaultName),
			Component:          runtimeLabel(string(p.Component), string(c.Name)),
			Absence:            Posture(p.Absence),
			AnnotationKey:      p.AnnotationKey,
			TimestampJSONField: p.TimestampJSONField,
			MaxAgeThresholdKey: "",
			DefaultMaxAge:      runtimeDuration(p.DefaultMaxAge),
			StaleOutcome:       adapter.Outcome(p.StaleOutcome),
		}, ns
	case "PodProjection":
		p := c.PodProjection
		ns := runtimeNamespaces(p.Target)
		return PodProjectionCheck{
			Selector:       runtimeSelector(p.Selector),
			ListName:       runtimeLabel(string(p.ListName), string(c.Name)),
			Component:      runtimeLabel(string(p.Component), string(c.Name)),
			VolumeName:     string(p.VolumeName),
			EnvVar:         p.EnvVar,
			MissingOutcome: adapter.Outcome(p.MissingOutcome),
		}, ns
	}
	panic("validated runtime kind missing converter")
}
func runtimeNamespaces(t api.DefinitionTarget) []string { return runtimeStrings(t.Namespaces) }
func firstRuntimeNamespace(ns []string) string {
	if len(ns) == 0 {
		return ""
	}
	return ns[0]
}
func runtimeLabel(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func runtimeDuration(d api.DefinitionDuration) time.Duration {
	if d == "" {
		return 0
	}
	v, _ := time.ParseDuration(string(d))
	return v
}
func runtimeStrings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}
func runtimeOutcomes(values map[string]api.DefinitionOutcome) map[string]adapter.Outcome {
	out := make(map[string]adapter.Outcome, len(values))
	for k, v := range values {
		out[k] = adapter.Outcome(v)
	}
	return out
}
func runtimeSelector(values map[string]api.DefinitionSelectorValue) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = string(v)
	}
	return out
}
