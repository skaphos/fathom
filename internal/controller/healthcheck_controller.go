/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package controller implements reconcilers for the fathom.skaphos.io CRDs.
package controller

import (
	"context"
	"reflect"
	"time"
	"unicode/utf8"

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

	healthCheckTargetKindAddonCheck           = "AddonCheck"
	healthCheckTargetKindDNSCheck             = "DNSCheck"
	healthCheckTargetKindNodeCertificateCheck = "NodeCertificateCheck"

	healthCheckConditionMessageMaxLen = 1024
)

type healthCheckTargetIdentity struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

func normalizeHealthCheckTarget(hc *fathomv1alpha1.HealthCheck) healthCheckTargetIdentity {
	apiVersion := hc.Spec.CheckRef.APIVersion
	if apiVersion == "" {
		apiVersion = fathomv1alpha1.GroupVersion.String()
	}
	namespace := hc.Spec.CheckRef.Namespace
	if namespace == "" {
		namespace = hc.Namespace
	}
	return healthCheckTargetIdentity{
		APIVersion: apiVersion,
		Kind:       hc.Spec.CheckRef.Kind,
		Namespace:  namespace,
		Name:       hc.Spec.CheckRef.Name,
	}
}

type healthCheckTargetSnapshot struct {
	Result           fathomv1alpha1.HealthReportResult
	Summary          string
	SourceObservedAt *metav1.Time
	LastReportName   string
	Interval         time.Duration
}

type healthCheckTargetReader func(
	context.Context,
	client.Client,
	types.NamespacedName,
) (healthCheckTargetSnapshot, error)

type healthCheckTargetHandler struct {
	APIVersion string
	Kind       string
	Object     client.Object
	read       healthCheckTargetReader
}

type healthCheckTargetRegistry struct {
	handlers []healthCheckTargetHandler
}

func newHealthCheckTargetRegistry() healthCheckTargetRegistry {
	return healthCheckTargetRegistry{handlers: []healthCheckTargetHandler{
		{
			APIVersion: fathomv1alpha1.GroupVersion.String(),
			Kind:       healthCheckTargetKindAddonCheck,
			Object:     &fathomv1alpha1.AddonCheck{},
			read:       readAddonCheckTarget,
		},
		{
			APIVersion: fathomv1alpha1.GroupVersion.String(),
			Kind:       healthCheckTargetKindDNSCheck,
			Object:     &fathomv1alpha1.DNSCheck{},
			read:       readDNSCheckTarget,
		},
		{
			APIVersion: fathomv1alpha1.GroupVersion.String(),
			Kind:       healthCheckTargetKindNodeCertificateCheck,
			Object:     &fathomv1alpha1.NodeCertificateCheck{},
			read:       readNodeCertificateCheckTarget,
		},
	}}
}

func (r healthCheckTargetRegistry) lookup(apiVersion, kind string) (healthCheckTargetHandler, bool) {
	for _, targetHandler := range r.handlers {
		if targetHandler.APIVersion == apiVersion && targetHandler.Kind == kind {
			return targetHandler, true
		}
	}
	return healthCheckTargetHandler{}, false
}

func readAddonCheckTarget(
	ctx context.Context,
	cl client.Client,
	key types.NamespacedName,
) (healthCheckTargetSnapshot, error) {
	var target fathomv1alpha1.AddonCheck
	if err := cl.Get(ctx, key, &target); err != nil {
		return healthCheckTargetSnapshot{}, err
	}
	return healthCheckTargetSnapshot{
		Result:           fathomv1alpha1.HealthReportResult(target.Status.LastResult),
		Summary:          summarizeFromConditions(target.Status.Conditions),
		SourceObservedAt: target.Status.LastRunTime,
		LastReportName:   target.Status.LastReportName,
		Interval:         addonCheckInterval(&target),
	}, nil
}

func readDNSCheckTarget(
	ctx context.Context,
	cl client.Client,
	key types.NamespacedName,
) (healthCheckTargetSnapshot, error) {
	var target fathomv1alpha1.DNSCheck
	if err := cl.Get(ctx, key, &target); err != nil {
		return healthCheckTargetSnapshot{}, err
	}
	return healthCheckTargetSnapshot{
		Result:           fathomv1alpha1.HealthReportResult(target.Status.LastResult),
		Summary:          summarizeFromConditions(target.Status.Conditions),
		SourceObservedAt: target.Status.LastRunTime,
		LastReportName:   target.Status.LastReportName,
		Interval:         dnsCheckInterval(&target),
	}, nil
}

func readNodeCertificateCheckTarget(
	ctx context.Context,
	cl client.Client,
	key types.NamespacedName,
) (healthCheckTargetSnapshot, error) {
	var target fathomv1alpha1.NodeCertificateCheck
	if err := cl.Get(ctx, key, &target); err != nil {
		return healthCheckTargetSnapshot{}, err
	}
	return healthCheckTargetSnapshot{
		Result:           fathomv1alpha1.HealthReportResult(target.Status.LastResult),
		Summary:          summarizeFromConditions(target.Status.Conditions),
		SourceObservedAt: target.Status.LastRunTime,
		LastReportName:   target.Status.LastReportName,
		Interval:         nodeCertInterval(&target),
	}, nil
}

func applyHealthCheckTargetSnapshot(hc *fathomv1alpha1.HealthCheck, snapshot healthCheckTargetSnapshot) {
	hc.Status.Result = snapshot.Result
	hc.Status.Summary = snapshot.Summary
	hc.Status.SourceObservedAt = snapshot.SourceObservedAt
	hc.Status.LastReportName = snapshot.LastReportName
	hc.Status.SourceInterval = &metav1.Duration{Duration: snapshot.Interval}
}

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

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthchecks,verbs=get;list;watch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthchecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthchecks/finalizers,verbs=update
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks,verbs=get;list;watch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=dnschecks,verbs=get;list;watch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=nodecertificatechecks,verbs=get;list;watch

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
	// staleness gauge tracks the staleness of the evidence behind this wrapper:
	// a paused or wedged target freezes it, which is exactly when the wrapper's
	// verdict stops meaning anything.
	//
	// targetInterval is the wrapped check's cadence, resolved during mirroring
	// so it costs no extra API read. A HealthCheck has no cadence of its own,
	// but neither is it an opaque mirror: checkRef always names a Fathom-native
	// check whose cadence the operator owns. Publishing it is what lets a
	// ClusterHealth be cadence-aware at all, because aggregates select
	// HealthChecks rather than the underlying checks (#277). It stays zero for a
	// kind this build cannot resolve, which leaves the series unset.
	// Start from the last-good mirrored cadence for paths that deliberately
	// preserve the mirrored snapshot (paused and transient lookup failures).
	// Dropping only the interval metric while retaining a non-zero timestamp
	// removes the wrapper from the cadence-relative staleness rule.
	var targetInterval time.Duration
	if hc.Status.SourceInterval != nil {
		targetInterval = hc.Status.SourceInterval.Duration
	}
	defer func() {
		observeCheck(r.Recorder, &hc, "HealthCheck",
			before.Result, hc.Status.Result,
			before.Conditions, hc.Status.Conditions,
			hc.Status.SourceObservedAt, targetInterval, err)
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

	var mirrorErr error
	if hc.Spec.Paused {
		apiMeta.SetStatusCondition(&hc.Status.Conditions, metav1.Condition{
			Type:               healthCheckConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: hc.Generation,
			Reason:             "Paused",
			Message:            "HealthCheck is paused; status mirroring is suspended.",
		})
	} else {
		var resolvedInterval time.Duration
		resolvedInterval, mirrorErr = r.mirrorTarget(ctx, &hc)
		if mirrorErr == nil || resolvedInterval > 0 {
			targetInterval = resolvedInterval
		}
	}

	if !equality.Semantic.DeepEqual(before, &hc.Status) {
		if err := r.Status().Update(ctx, &hc); err != nil {
			return ctrl.Result{}, err
		}
		log.V(1).Info("updated HealthCheck status", "result", hc.Status.Result)
	}
	return ctrl.Result{}, mirrorErr
}

// mirrorTarget projects the referenced specialized check's status into hc.Status.
// It sets the Ready condition and (on success) Result, Summary,
// SourceObservedAt, and LastReportName. Terminal failures (unsupported kind,
// target NotFound) clear the mirrored fields and are recorded only as a
// Ready=False condition. A transient target lookup failure instead preserves
// the last-good mirrored fields — so a blip does not ripple into the
// ClusterHealth roll-up as Unknown — and is returned as an error so the caller
// requeues and re-attempts the mirror.
//
// It also returns the target's effective cadence, resolved from the object this
// function already fetched so it costs no extra API read. Zero means no cadence
// could be resolved — an unsupported kind, a missing target, or a lookup
// failure — which leaves the cadence series unset rather than asserting zero.
func (r *HealthCheckReconciler) mirrorTarget(ctx context.Context, hc *fathomv1alpha1.HealthCheck) (time.Duration, error) {
	identity := normalizeHealthCheckTarget(hc)
	if identity.APIVersion != fathomv1alpha1.GroupVersion.String() {
		clearMirroredHealthCheckStatus(hc)
		setHealthCheckTargetFailure(
			hc,
			"UnsupportedAPIVersion",
			"checkRef.apiVersion "+identity.APIVersion+" is not supported; use "+fathomv1alpha1.GroupVersion.String()+" or omit apiVersion to select the current version.",
		)
		return 0, nil
	}
	targetHandler, ok := newHealthCheckTargetRegistry().lookup(identity.APIVersion, identity.Kind)
	if !ok {
		clearMirroredHealthCheckStatus(hc)
		setHealthCheckTargetFailure(
			hc,
			"UnsupportedKind",
			"checkRef.kind "+identity.Kind+" is not supported; supported kinds are AddonCheck, DNSCheck, and NodeCertificateCheck.",
		)
		// No cadence is knowable for a kind this build cannot resolve. Returning
		// zero leaves the series unset, so the check simply drops out of any
		// cadence-relative rule instead of reading as permanently overdue.
		return 0, nil
	}

	snapshot, err := targetHandler.read(ctx, r.Client, types.NamespacedName{
		Namespace: identity.Namespace,
		Name:      identity.Name,
	})
	if apierrors.IsNotFound(err) {
		clearMirroredHealthCheckStatus(hc)
		setHealthCheckTargetFailure(hc, "TargetNotFound", err.Error())
		return 0, nil
	}
	if err != nil {
		setHealthCheckTargetFailure(hc, "TargetLookupFailed", err.Error())
		return 0, err
	}

	applyHealthCheckTargetSnapshot(hc, snapshot)

	apiMeta.SetStatusCondition(&hc.Status.Conditions, metav1.Condition{
		Type:               healthCheckConditionReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: hc.Generation,
		Reason:             "TargetMirrored",
		Message:            "HealthCheck mirrored the referenced check's status.",
	})
	return snapshot.Interval, nil
}

func setHealthCheckTargetFailure(hc *fathomv1alpha1.HealthCheck, reason, message string) {
	apiMeta.SetStatusCondition(&hc.Status.Conditions, metav1.Condition{
		Type:               healthCheckConditionReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: hc.Generation,
		Reason:             reason,
		Message:            truncateToRuneLimit(message, healthCheckConditionMessageMaxLen),
	})
}

func clearMirroredHealthCheckStatus(hc *fathomv1alpha1.HealthCheck) {
	hc.Status.Result = ""
	hc.Status.SourceInterval = nil
	hc.Status.SourceObservedAt = nil
	hc.Status.LastReportName = ""
	hc.Status.Summary = ""
}

// summarizeFromConditions extracts a human-readable one-liner from the source
// check's conditions. Prefers the Ready condition's message when present.
//
// The result is truncated to healthCheckSummaryMaxLen. A source condition
// Message admits up to 32768 chars (the metav1.Condition bound), while
// HealthCheckStatus.Summary carries a kubebuilder MaxLength of 1024. Mirroring
// an over-long message verbatim makes the API server reject the whole status
// update, wedging the mirror on that value forever (the retry replays the same
// payload) and freezing the child's contribution to the ClusterHealth roll-up
// at a stale verdict. Truncating at the mirror boundary keeps the summary a
// valid, admissible value.
func summarizeFromConditions(conds []metav1.Condition) string {
	for _, c := range conds {
		if c.Type == healthCheckConditionReady {
			return truncateSummary(c.Message)
		}
	}
	return ""
}

// healthCheckSummaryMaxLen mirrors the +kubebuilder:validation:MaxLength on
// HealthCheckStatus.Summary. OpenAPI maxLength counts Unicode code points, so
// truncation is rune-based to stay within the schema bound for multi-byte text.
const healthCheckSummaryMaxLen = 1024

func truncateSummary(s string) string {
	return truncateToRuneLimit(s, healthCheckSummaryMaxLen)
}

func truncateToRuneLimit(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	// Reserve one rune for the ellipsis marker so the total stays within bound.
	const ellipsis = "…"
	runes := []rune(s)
	return string(runes[:limit-1]) + ellipsis
}

// SetupWithManager sets up the controller with the Manager. It owns
// HealthCheck and watches every registered target type so source status changes
// re-enqueue the HealthChecks that wrap the exact source identity.
func (r *HealthCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.HealthCheck{}).
		Named("healthcheck")
	for _, targetHandler := range newHealthCheckTargetRegistry().handlers {
		controllerBuilder = controllerBuilder.Watches(
			targetHandler.Object,
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return r.healthChecksForTarget(ctx, obj, targetHandler)
			}),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		)
	}
	return controllerBuilder.Complete(r)
}

// healthChecksForAddonCheck returns the namespaced names of every HealthCheck
// that references the given AddonCheck. Called from the watch map function.
func (r *HealthCheckReconciler) healthChecksForAddonCheck(ctx context.Context, obj client.Object) []reconcile.Request {
	targetHandler, ok := newHealthCheckTargetRegistry().lookup(
		fathomv1alpha1.GroupVersion.String(),
		healthCheckTargetKindAddonCheck,
	)
	if !ok {
		return nil
	}
	return r.healthChecksForTarget(ctx, obj, targetHandler)
}

func (r *HealthCheckReconciler) healthChecksForTarget(
	ctx context.Context,
	obj client.Object,
	targetHandler healthCheckTargetHandler,
) []reconcile.Request {
	if reflect.TypeOf(obj) != reflect.TypeOf(targetHandler.Object) {
		return nil
	}
	var list fathomv1alpha1.HealthCheckList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}
	var out []reconcile.Request
	for _, hc := range list.Items {
		identity := normalizeHealthCheckTarget(&hc)
		if identity.APIVersion != targetHandler.APIVersion ||
			identity.Kind != targetHandler.Kind ||
			identity.Namespace != obj.GetNamespace() ||
			identity.Name != obj.GetName() {
			continue
		}
		out = append(out, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: hc.Namespace, Name: hc.Name}})
	}
	return out
}
