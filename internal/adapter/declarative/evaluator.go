/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/pkg/adapter"
)

// EvalContext is the per-Run, per-family dynamic input handed to each
// Evaluator. All fields are read-only; the engine owns construction. Ctx
// already carries the AddonCheck timeout deadline (bounded by the controller's
// WithTimeout).
type EvalContext struct {
	// Ctx carries the deadline and cancellation for this Run.
	Ctx context.Context
	// Client is the least-privilege controller-runtime client.
	Client client.Client
	// Logger is the family-scoped contextual logger.
	Logger logr.Logger
	// Family is the family whose components are running.
	Family adapter.Family
	// Policy is the resolved policy for this family (Enabled==true).
	Policy adapter.FamilyPolicy
	// DefaultPosture is the addon-level absence default (from
	// AddonDefinition.defaultPosture) a component inherits when it declares no
	// Posture of its own. The engine sets it; evaluators resolve it via
	// effectiveAbsence.
	DefaultPosture Posture
}

// Evaluator reads live cluster state and returns zero or more CheckResults, all
// tagged with ec.Family. It must honor ec.Ctx cancellation.
//
// The returned error is reserved for adapter-level failures that should abort
// the whole Run (it becomes adapter.Run's error return, forcing the
// HealthReport to Error). Per-target problems are NEVER errors — they are
// CheckResults with OutcomeFail (unhealthy) or OutcomeError (indeterminate).
// The shipped read-and-compare evaluators never return a non-nil error; the
// seam exists for future non-declarative evaluators (probe pods, admission
// dry-run).
type Evaluator interface {
	Evaluate(ec EvalContext) ([]adapter.CheckResult, error)
}

// resolveFamily is the generalized cilium.familyPolicy: a nil Policy enables
// every DefaultEnabled family; a non-nil Policy with the family key absent
// disables it; a present entry is gated by its Enabled flag.
func resolveFamily(policy map[adapter.Family]adapter.FamilyPolicy, f adapter.Family, dflt bool) (adapter.FamilyPolicy, bool) {
	if policy == nil {
		return adapter.FamilyPolicy{Enabled: dflt}, dflt
	}
	fp, ok := policy[f]
	if !ok {
		return adapter.FamilyPolicy{}, false
	}
	return fp, fp.Enabled
}

// firstNamespace resolves the single namespace a named singleton lives in: the
// first policy namespace, or def when the policy list is empty.
func firstNamespace(policy adapter.FamilyPolicy, def string) string {
	if len(policy.Namespaces) > 0 {
		return policy.Namespaces[0]
	}
	return def
}

// policyNamespaces resolves the namespace set a collection is listed across.
// An empty policy means all namespaces, represented by client.InNamespace("").
func policyNamespaces(policy adapter.FamilyPolicy, _ string) []string {
	if len(policy.Namespaces) > 0 {
		return policy.Namespaces
	}
	return []string{""}
}

// stringThreshold returns the trimmed threshold value for key, or dflt when the
// key is absent or blank.
func stringThreshold(policy adapter.FamilyPolicy, key, dflt string) string {
	if policy.Thresholds == nil {
		return dflt
	}
	value := strings.TrimSpace(policy.Thresholds[key])
	if value == "" {
		return dflt
	}
	return value
}

// int32Threshold returns the parsed int32 threshold for key, or dflt when the
// key is absent, negative, or unparseable.
func int32Threshold(policy adapter.FamilyPolicy, key string, dflt int32) int32 {
	if policy.Thresholds == nil {
		return dflt
	}
	value, ok := policy.Thresholds[key]
	if !ok {
		return dflt
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 {
		return dflt
	}
	return int32(parsed)
}

// clockSkewGrace is how far a status timestamp may sit in the future before a
// recency check treats it as anomalous rather than fresh. It absorbs benign
// clock skew between the operator and the API server (whose clock stamps object
// status times) while still surfacing a clearly-future timestamp — from a
// malformed payload or a badly-skewed node — instead of scoring it a healthy
// Pass (a naive now-minus-timestamp age goes negative for a future stamp and
// would otherwise slip under any freshness window).
const clockSkewGrace = 5 * time.Minute

// isFutureTimestamp reports whether ts sits more than clockSkewGrace in the
// future relative to now.
func isFutureTimestamp(ts time.Time) bool {
	return time.Until(ts) > clockSkewGrace
}

// durationThreshold returns the parsed Go-duration threshold for key, or dflt
// when the key is absent, negative, or unparseable.
func durationThreshold(policy adapter.FamilyPolicy, key string, dflt time.Duration) time.Duration {
	if policy.Thresholds == nil {
		return dflt
	}
	value, ok := policy.Thresholds[key]
	if !ok {
		return dflt
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return dflt
	}
	return parsed
}

// deploymentAvailable reports whether the Deployment carries Available=True.
func deploymentAvailable(d *appsv1.Deployment) bool {
	for _, condition := range d.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// derefReplicas returns *spec.replicas, defaulting a nil pointer to 1 (the
// Kubernetes default for Deployment and StatefulSet).
func derefReplicas(replicas *int32) int32 {
	if replicas != nil {
		return *replicas
	}
	return 1
}

// podReady reports whether the Pod carries Ready=True.
func podReady(p *corev1.Pod) bool {
	for _, condition := range p.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// maxRestartCount returns the highest container restart count in the Pod.
func maxRestartCount(p *corev1.Pod) int32 {
	var max int32
	for _, status := range p.Status.ContainerStatuses {
		if status.RestartCount > max {
			max = status.RestartCount
		}
	}
	return max
}

// podTarget returns the TargetRef for a concrete Pod.
func podTarget(p *corev1.Pod) adapter.TargetRef {
	return adapter.TargetRef{APIVersion: "v1", Kind: "Pod", Namespace: p.Namespace, Name: p.Name}
}

// effectiveAbsence resolves the Posture for a NotFound target: the component's
// own Posture when set, otherwise the addon-level DefaultPosture, otherwise the
// required-by-default Required. It never returns "".
func effectiveAbsence(component, dflt Posture) Posture {
	if component != "" {
		return component
	}
	if dflt != "" {
		return dflt
	}
	return Required
}

// absenceOutcome maps the effective Posture to the Outcome used when a named
// singleton target is NotFound: the explicit Optional opt-out -> Skipped;
// Required (the default) -> Fail. Callers additionally tag the result with the
// adapter.DetailAbsent marker via adapter.MarkAbsent so "not installed" stays
// queryable regardless of the verdict (SKA-526).
func absenceOutcome(p Posture) adapter.Outcome {
	if p == Optional {
		return adapter.OutcomeSkipped
	}
	return adapter.OutcomeFail
}

// result builds a CheckResult tagged with family, timed from started, and
// carrying details. details may be nil.
func result(family adapter.Family, ref adapter.TargetRef, o adapter.Outcome,
	summary string, details map[string]string, started time.Time) adapter.CheckResult {
	return adapter.CheckResult{
		Family:     family,
		Outcome:    o,
		TargetRef:  ref,
		Summary:    summary,
		Details:    details,
		ObservedAt: time.Now(),
		Duration:   time.Since(started),
	}
}

// skippedResult builds an OutcomeSkipped CheckResult carrying the
// machine-readable Details["skipReason"] (one of FamilyDisabled,
// NoMatchingObjects). A NotFound singleton is not a skipReason category — it is
// tagged with the cross-outcome adapter.DetailAbsent marker instead (SKA-526).
func skippedResult(family adapter.Family, ref adapter.TargetRef, summary, skipReason string) adapter.CheckResult {
	return adapter.CheckResult{
		Family:     family,
		Outcome:    adapter.OutcomeSkipped,
		TargetRef:  ref,
		Summary:    summary,
		Details:    map[string]string{"skipReason": skipReason},
		ObservedAt: time.Now(),
	}
}
