/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package controller implements reconcilers for the fathom.skaphos.io CRDs.
package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/metrics"
)

const (
	healthCheckConditionAccepted = "Accepted"
	healthCheckConditionPaused   = "Paused"
	healthCheckConditionReady    = "Ready"

	healthCheckTargetKindAddonCheck = "AddonCheck"
)

// HealthCheckReconciler reconciles a HealthCheck object. It is a wrapper that
// mirrors a referenced specialized check's status into a uniform shape per
// docs/adr/0004-healthcheck-as-wrapper.md. It does not execute checks itself.
type HealthCheckReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Tracer creates the per-Reconcile span. Optional: a nil Tracer falls back
	// to the global provider (a no-op unless tracing is enabled).
	Tracer trace.Tracer

	// Recorder emits the Kubernetes Events contract (result transitions and
	// operational failures) on HealthCheck resources. Optional: nil disables
	// event recording; the check gauges are unaffected.
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthchecks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthchecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthchecks/finalizers,verbs=update
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks,verbs=get;list;watch

func (r *HealthCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	ctx, span := reconcilerTracer(r.Tracer).Start(ctx, "healthcheck.reconcile", trace.WithAttributes(
		attribute.String("fathom.kind", "HealthCheck"),
		attribute.String("fathom.namespace", req.Namespace),
		attribute.String("fathom.name", req.Name),
	))
	defer func() { endReconcileSpan(span, err) }()

	start := time.Now()
	defer func() {
		// Record at the very end so we capture the full duration of the reconcile,
		// including any status updates or error paths. outcome distinguishes a
		// returned error from a clean reconcile (requeue/no-op refinements can
		// come later).
		outcome := "success"
		if err != nil {
			outcome = "error"
		}
		metrics.RecordReconcile("HealthCheck", outcome, time.Since(start))
	}()

	log := logf.FromContext(ctx).WithValues("namespacedName", req.NamespacedName)

	var hc fathomv1alpha1.HealthCheck
	if err := r.Get(ctx, req.NamespacedName, &hc); err != nil {
		if apierrors.IsNotFound(err) {
			metrics.DeleteCheckSeries("HealthCheck", req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	before := hc.Status.DeepCopy()
	// SourceObservedAt carries the mirrored target's own run time, so the
	// staleness gauge tracks the freshness of the evidence behind this wrapper:
	// a paused or wedged target freezes it, which is exactly when the wrapper's
	// verdict stops meaning anything.
	defer func() {
		observeCheck(r.Recorder, &hc, "HealthCheck",
			before.Result, hc.Status.Result,
			before.Conditions, hc.Status.Conditions,
			hc.Status.SourceObservedAt, err)
	}()
	hc.Status.ObservedGeneration = hc.Generation

	apiMeta.SetStatusCondition(&hc.Status.Conditions, metav1.Condition{
		Type:               healthCheckConditionAccepted,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: hc.Generation,
		Reason:             "SpecAccepted",
		Message:            "HealthCheck specification has been accepted for reconciliation.",
	})

	pausedStatus := metav1.ConditionFalse
	pausedReason := "RunEnabled"
	pausedMessage := "HealthCheck is mirroring its referenced check."
	if hc.Spec.Paused {
		pausedStatus = metav1.ConditionTrue
		pausedReason = "Paused"
		pausedMessage = "HealthCheck is paused; the last mirrored Status snapshot is preserved."
	}
	apiMeta.SetStatusCondition(&hc.Status.Conditions, metav1.Condition{
		Type:               healthCheckConditionPaused,
		Status:             pausedStatus,
		ObservedGeneration: hc.Generation,
		Reason:             pausedReason,
		Message:            pausedMessage,
	})

	if hc.Spec.Paused {
		apiMeta.SetStatusCondition(&hc.Status.Conditions, metav1.Condition{
			Type:               healthCheckConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: hc.Generation,
			Reason:             "Paused",
			Message:            "HealthCheck is paused; status mirroring is suspended.",
		})
	} else {
		r.mirrorTarget(ctx, &hc)
	}

	if equality.Semantic.DeepEqual(before, &hc.Status) {
		return ctrl.Result{}, nil
	}
	if err := r.Status().Update(ctx, &hc); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("updated HealthCheck status", "result", hc.Status.Result)
	return ctrl.Result{}, nil
}

// mirrorTarget projects the referenced specialized check's status into hc.Status.
// It sets the Ready condition and (on success) Result, Summary,
// SourceObservedAt, and LastReportName. It does not return an error: any failure
// is recorded as a Ready=False condition so the caller's Update call remains
// idempotent. The only supported target kind in v0.1 is AddonCheck.
func (r *HealthCheckReconciler) mirrorTarget(ctx context.Context, hc *fathomv1alpha1.HealthCheck) {
	if hc.Spec.CheckRef.Kind != healthCheckTargetKindAddonCheck {
		clearMirroredHealthCheckStatus(hc)
		apiMeta.SetStatusCondition(&hc.Status.Conditions, metav1.Condition{
			Type:               healthCheckConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: hc.Generation,
			Reason:             "UnsupportedKind",
			Message:            "checkRef.kind " + hc.Spec.CheckRef.Kind + " is not supported by this build of Fathom.",
		})
		return
	}

	namespace := hc.Spec.CheckRef.Namespace
	if namespace == "" {
		namespace = hc.Namespace
	}
	var target fathomv1alpha1.AddonCheck
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: hc.Spec.CheckRef.Name}, &target)
	if err != nil {
		clearMirroredHealthCheckStatus(hc)
		reason := "TargetLookupFailed"
		if apierrors.IsNotFound(err) {
			reason = "TargetNotFound"
		}
		apiMeta.SetStatusCondition(&hc.Status.Conditions, metav1.Condition{
			Type:               healthCheckConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: hc.Generation,
			Reason:             reason,
			Message:            err.Error(),
		})
		return
	}

	hc.Status.Result = fathomv1alpha1.HealthReportResult(target.Status.LastResult)
	hc.Status.SourceObservedAt = target.Status.LastRunTime
	hc.Status.LastReportName = target.Status.LastReportName
	hc.Status.Summary = summarizeFromConditions(target.Status.Conditions)

	apiMeta.SetStatusCondition(&hc.Status.Conditions, metav1.Condition{
		Type:               healthCheckConditionReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: hc.Generation,
		Reason:             "TargetMirrored",
		Message:            "HealthCheck mirrored the referenced check's status.",
	})
}

func clearMirroredHealthCheckStatus(hc *fathomv1alpha1.HealthCheck) {
	hc.Status.Result = ""
	hc.Status.SourceObservedAt = nil
	hc.Status.LastReportName = ""
	hc.Status.Summary = ""
}

// summarizeFromConditions extracts a human-readable one-liner from the source
// check's conditions. Prefers the Ready condition's message when present.
func summarizeFromConditions(conds []metav1.Condition) string {
	for _, c := range conds {
		if c.Type == healthCheckConditionReady {
			return c.Message
		}
	}
	return ""
}

// SetupWithManager sets up the controller with the Manager. It owns
// HealthCheck and watches AddonCheck so a target's status change re-enqueues
// every HealthCheck that wraps it.
func (r *HealthCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.HealthCheck{}).
		Named("healthcheck").
		Watches(
			&fathomv1alpha1.AddonCheck{},
			handler.EnqueueRequestsFromMapFunc(r.healthChecksForAddonCheck),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

// healthChecksForAddonCheck returns the namespaced names of every HealthCheck
// that references the given AddonCheck. Called from the watch map function.
func (r *HealthCheckReconciler) healthChecksForAddonCheck(ctx context.Context, obj client.Object) []reconcile.Request {
	addonCheck, ok := obj.(*fathomv1alpha1.AddonCheck)
	if !ok {
		return nil
	}
	var list fathomv1alpha1.HealthCheckList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}
	var out []reconcile.Request
	for _, hc := range list.Items {
		if hc.Spec.CheckRef.Kind != healthCheckTargetKindAddonCheck {
			continue
		}
		if hc.Spec.CheckRef.Name != addonCheck.Name {
			continue
		}
		ns := hc.Spec.CheckRef.Namespace
		if ns == "" {
			ns = hc.Namespace
		}
		if ns != addonCheck.Namespace {
			continue
		}
		out = append(out, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: hc.Namespace, Name: hc.Name}})
	}
	return out
}
