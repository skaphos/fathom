/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/metrics"
)

// eventReasonResultChanged is the stable reason for result-transition events;
// operational-failure events reuse the Ready condition's own reason
// (AdapterRunFailed, ProbeLaunchFailed, RBACProvisioningFailed, ...) so the
// event and the condition can never disagree about why a check is degraded.
const (
	eventReasonResultChanged  = "ResultChanged"
	eventReasonReconcileError = "ReconcileError"

	// Event actions for the events.k8s.io API: what the controller was doing
	// when the event occurred.
	eventActionEvaluate  = "Evaluate"
	eventActionReconcile = "Reconcile"

	// checkConditionReady is the Ready condition type every check kind uses
	// (the per-kind *ConditionReady constants all equal it).
	checkConditionReady = "Ready"
)

// observeCheck mirrors a reconciled check's final in-memory status into the
// check gauges and records the Events contract (result transitions, newly
// failing Ready conditions, reconcile errors). Each reconciler defers one
// call right after a successful Get, so every exit path is covered and the
// previous result/conditions always come from the status as it was fetched —
// never from process memory — which is what makes an operator restart unable
// to fire a false transition. A nil recorder disables events only; the gauge
// mirror always runs. Recording is fire-and-forget: it can neither fail nor
// block the reconcile.
func observeCheck(recorder events.EventRecorder, obj client.Object, kind string,
	previousResult, result fathomv1alpha1.HealthReportResult,
	previousConditions, conditions []metav1.Condition,
	lastRun *metav1.Time, reconcileErr error,
) {
	var freshness time.Time
	if lastRun != nil {
		freshness = lastRun.Time
	}
	metrics.ObserveCheck(kind, obj.GetNamespace(), obj.GetName(), string(result), freshness)

	if recorder == nil {
		return
	}

	// A check with no evaluated result yet reads as Unknown, so the first
	// completed evaluation records a transition from Unknown (spec 001,
	// clarification Q3) and a source-cleared result reads as a transition to
	// Unknown rather than silence.
	previous, next := coerceResult(previousResult), coerceResult(result)
	if previous != next {
		eventType := corev1.EventTypeNormal
		if next.Severity() >= fathomv1alpha1.HealthReportResultWarn.Severity() {
			eventType = corev1.EventTypeWarning
		}
		recorder.Eventf(obj, nil, eventType, eventReasonResultChanged, eventActionEvaluate, "check result changed from %s to %s", previous, next)
	}

	// A Ready condition that just turned False — or changed its failure reason
	// — surfaces once as a Warning carrying the condition's reason and message.
	// Comparing against the pre-reconcile conditions keeps a persistently
	// failing check to a single event per failure episode (the recorder's
	// aggregation is only the backstop).
	emittedFailure := false
	if ready := apiMeta.FindStatusCondition(conditions, checkConditionReady); ready != nil && ready.Status == metav1.ConditionFalse {
		beforeReady := apiMeta.FindStatusCondition(previousConditions, checkConditionReady)
		if beforeReady == nil || beforeReady.Status != metav1.ConditionFalse || beforeReady.Reason != ready.Reason {
			recorder.Eventf(obj, nil, corev1.EventTypeWarning, ready.Reason, eventActionReconcile, "%s", ready.Message)
			emittedFailure = true
		}
	}

	// A terminal reconcile error that did not already surface through a Ready
	// failure above still leaves an event trail (FR-006) — but never two
	// events for one cause.
	if reconcileErr != nil && !emittedFailure {
		recorder.Eventf(obj, nil, corev1.EventTypeWarning, eventReasonReconcileError, eventActionReconcile, "reconcile failed: %v", reconcileErr)
	}
}

func coerceResult(r fathomv1alpha1.HealthReportResult) fathomv1alpha1.HealthReportResult {
	if r == "" {
		return fathomv1alpha1.HealthReportResultUnknown
	}
	return r
}
