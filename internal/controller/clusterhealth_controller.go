/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
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
	clusterHealthConditionAccepted = "Accepted"
	clusterHealthConditionReady    = "Ready"
)

// ClusterHealthReconciler reconciles a ClusterHealth object. It aggregates
// the Status of selected HealthCheck resources into a single worst-case
// Result. Per the AGENTS.md invariant and ADR-0004, this controller
// deliberately never imports or reads HealthReport — its only input is
// HealthCheck.Status, which the HealthCheckReconciler maintains.
type ClusterHealthReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Tracer creates the per-Reconcile span. Optional: a nil Tracer falls back
	// to the global provider (a no-op unless tracing is enabled).
	Tracer trace.Tracer
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=clusterhealths,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=clusterhealths/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=clusterhealths/finalizers,verbs=update

func (r *ClusterHealthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	ctx, span := reconcilerTracer(r.Tracer).Start(ctx, "clusterhealth.reconcile", trace.WithAttributes(
		attribute.String("fathom.kind", "ClusterHealth"),
		attribute.String("fathom.namespace", req.Namespace),
		attribute.String("fathom.name", req.Name),
	))
	defer func() { endReconcileSpan(span, err) }()

	start := time.Now()
	defer func() {
		outcome := "success"
		if err != nil {
			outcome = "error"
		}
		metrics.RecordReconcile("ClusterHealth", outcome, time.Since(start))
	}()

	log := logf.FromContext(ctx).WithValues("namespacedName", req.NamespacedName)

	var ch fathomv1alpha1.ClusterHealth
	if err := r.Get(ctx, req.NamespacedName, &ch); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	before := ch.Status.DeepCopy()
	ch.Status.ObservedGeneration = ch.Generation

	apiMeta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
		Type:               clusterHealthConditionAccepted,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: ch.Generation,
		Reason:             "SpecAccepted",
		Message:            "ClusterHealth specification has been accepted for aggregation.",
	})

	selector, err := selectorFromSpec(ch.Spec.Selector)
	if err != nil {
		clearClusterHealthAggregateStatus(&ch)
		apiMeta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
			Type:               clusterHealthConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: ch.Generation,
			Reason:             "InvalidSelector",
			Message:            err.Error(),
		})
		// Fall through to the diff/Update so the failure is visible.
	} else {
		if hcs, listErr := r.listSelectedHealthChecks(ctx, &ch, selector); listErr != nil {
			clearClusterHealthAggregateStatus(&ch)
			apiMeta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
				Type:               clusterHealthConditionReady,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: ch.Generation,
				Reason:             "ListFailed",
				Message:            listErr.Error(),
			})
		} else {
			r.aggregate(&ch, hcs)
		}
	}

	if equality.Semantic.DeepEqual(before, &ch.Status) {
		return ctrl.Result{}, nil
	}
	if err := r.Status().Update(ctx, &ch); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("updated ClusterHealth status", "result", ch.Status.Result, "matched", ch.Status.MatchedCount)
	return ctrl.Result{}, nil
}

// listSelectedHealthChecks lists the HealthChecks the aggregate rolls up under
// the namespace-scope precedence on ClusterHealthSpec (allowlist → denylist →
// open). When Namespaces is set the list is one List-scope per allowlisted
// namespace (schema-capped MaxItems). When only ExcludedNamespaces is set (or
// neither is), the list is cluster-wide and the denylist is applied in memory.
func (r *ClusterHealthReconciler) listSelectedHealthChecks(ctx context.Context, ch *fathomv1alpha1.ClusterHealth, selector labels.Selector) ([]fathomv1alpha1.HealthCheck, error) {
	// Allowlist path: list only the named namespaces. ExcludedNamespaces is
	// ignored while an allowlist is present (allow is definitive).
	if len(ch.Spec.Namespaces) > 0 {
		var out []fathomv1alpha1.HealthCheck
		for _, ns := range ch.Spec.Namespaces {
			var hcs fathomv1alpha1.HealthCheckList
			if err := r.List(ctx, &hcs,
				client.InNamespace(ns),
				client.MatchingLabelsSelector{Selector: selector},
			); err != nil {
				// The scope lands in the Ready/ListFailed condition message; without
				// it an RBAC or transient failure is undiagnosable when
				// spec.namespaces lists several namespaces.
				return nil, fmt.Errorf("listing HealthChecks in %s: %w", namespaceScope(ns), err)
			}
			out = append(out, hcs.Items...)
		}
		return out, nil
	}

	// Open or denylist: list all namespaces, then drop excluded ones.
	var hcs fathomv1alpha1.HealthCheckList
	if err := r.List(ctx, &hcs,
		client.MatchingLabelsSelector{Selector: selector},
	); err != nil {
		return nil, fmt.Errorf("listing HealthChecks in %s: %w", namespaceScope(""), err)
	}
	if len(ch.Spec.ExcludedNamespaces) == 0 {
		return hcs.Items, nil
	}
	out := make([]fathomv1alpha1.HealthCheck, 0, len(hcs.Items))
	for i := range hcs.Items {
		if clusterHealthCoversNamespace(ch, hcs.Items[i].Namespace) {
			out = append(out, hcs.Items[i])
		}
	}
	return out, nil
}

// namespaceScope names a list scope for error messages: all namespaces for
// the empty namespace, the quoted namespace otherwise.
func namespaceScope(namespace string) string {
	if namespace == "" {
		return "all namespaces"
	}
	return fmt.Sprintf("namespace %q", namespace)
}

// aggregate populates ch.Status from the selected HealthChecks. It computes
// the worst-case Result via fathomv1alpha1.WorstResult, builds a deterministic
// Children summary (sorted by namespace, then name), and sets ObservedAt to
// the latest input freshness across children (not wall-clock — that would
// defeat no-op idempotency, and a "when did inputs last move" timestamp is
// what dashboards actually want).
func (r *ClusterHealthReconciler) aggregate(ch *fathomv1alpha1.ClusterHealth, hcs []fathomv1alpha1.HealthCheck) {
	ch.Status.MatchedCount = int32(len(hcs))

	// A selector matching nothing is not a healthy verdict. Report Unknown with
	// Ready=False/NoMatches — consistent with the InvalidSelector/ListFailed
	// error paths — so a dashboard or deploy-gate keyed on the roll-up sees "no
	// signal" instead of a green empty result (#161).
	if len(hcs) == 0 {
		ch.Status.Result = fathomv1alpha1.HealthReportResultUnknown
		ch.Status.Children = nil
		ch.Status.ObservedAt = nil
		apiMeta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
			Type:               clusterHealthConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: ch.Generation,
			Reason:             "NoMatches",
			Message:            "No HealthChecks match the selector.",
		})
		return
	}

	sort.Slice(hcs, func(i, j int) bool {
		if hcs[i].Namespace != hcs[j].Namespace {
			return hcs[i].Namespace < hcs[j].Namespace
		}
		return hcs[i].Name < hcs[j].Name
	})

	ch.Status.Children = make([]fathomv1alpha1.ClusterHealthChildSummary, 0, len(hcs))
	results := make([]fathomv1alpha1.HealthReportResult, 0, len(hcs))
	var latest *metav1.Time
	for i := range hcs {
		hc := &hcs[i]
		// The child summary keeps the raw child verdict, including empty; only
		// the roll-up coerces empty -> Unknown.
		ch.Status.Children = append(ch.Status.Children, fathomv1alpha1.ClusterHealthChildSummary{
			Namespace:  hc.Namespace,
			Name:       hc.Name,
			Result:     hc.Status.Result,
			Summary:    hc.Status.Summary,
			ObservedAt: hc.Status.SourceObservedAt,
		})
		results = append(results, hc.Status.Result)
		if hc.Status.SourceObservedAt != nil && (latest == nil || hc.Status.SourceObservedAt.After(latest.Time)) {
			latest = hc.Status.SourceObservedAt
		}
	}

	// coerceEmptyToUnknown: a selected child with no verdict yet — never
	// reconciled, or its mirrored result cleared when the source was deleted —
	// degrades the roll-up to Unknown instead of vanishing. A live Fail sibling
	// still wins, because Fail outranks Unknown (#161).
	ch.Status.Result = fathomv1alpha1.WorstResult(results, true)
	ch.Status.ObservedAt = latest

	apiMeta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
		Type:               clusterHealthConditionReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: ch.Generation,
		Reason:             "Aggregated",
		Message:            "ClusterHealth aggregated the selected HealthChecks.",
	})
}

func clearClusterHealthAggregateStatus(ch *fathomv1alpha1.ClusterHealth) {
	ch.Status.Result = ""
	ch.Status.MatchedCount = 0
	ch.Status.Children = nil
	ch.Status.ObservedAt = nil
}

func selectorFromSpec(sel *metav1.LabelSelector) (labels.Selector, error) {
	if sel == nil {
		return labels.Everything(), nil
	}
	return metav1.LabelSelectorAsSelector(sel)
}

// healthCheckEventHandler enqueues the ClusterHealths affected by a HealthCheck
// event. Update is the interesting verb: it maps ObjectOld *and* ObjectNew so a
// HealthCheck edited out of a selector re-enqueues the ClusterHealth that must
// now drop it (#148).
//
// All four callbacks are required. handler.Funcs treats an unset callback as a
// silent no-op, so deleting one here does not fail to compile — it stops the
// controller reacting to that verb entirely. Dropping DeleteFunc, for example,
// would strand deleted children in status.children forever: a strictly worse
// bug than the one this fixes.
func (r *ClusterHealthReconciler) healthCheckEventHandler() handler.EventHandler {
	enqueue := func(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request], objs ...client.Object) {
		for _, req := range r.clusterHealthsForHealthChecks(ctx, objs...) {
			q.Add(req)
		}
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(ctx, q, e.Object)
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(ctx, q, e.ObjectOld, e.ObjectNew)
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(ctx, q, e.Object)
		},
		GenericFunc: func(ctx context.Context, e event.GenericEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(ctx, q, e.Object)
		},
	}
}

// SetupWithManager sets up the controller with the Manager. It owns
// ClusterHealth and watches HealthCheck so a member's Status change
// re-enqueues every ClusterHealth whose selector matches it — and so a label
// edit that moves a HealthCheck *out* of a selector re-enqueues the
// ClusterHealth that must drop it (#148).
//
// ResourceVersionChangedPredicate filters inside the source, before handler
// dispatch, and passes the event through unmodified, so ObjectOld survives. A
// label-only predicate would be wrong here: status-only changes are the primary
// reason this watch exists.
func (r *ClusterHealthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.ClusterHealth{}).
		Named("clusterhealth").
		Watches(
			&fathomv1alpha1.HealthCheck{},
			r.healthCheckEventHandler(),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

// clusterHealthsForHealthChecks returns one request per ClusterHealth whose
// scope covers at least one of objs. ClusterHealth is cluster-scoped, so the
// requests carry no namespace.
//
// The variadic shape exists for update events (#148): a HealthCheck whose
// labels are edited out of a ClusterHealth's selector must still re-enqueue
// that ClusterHealth, or status.children keeps the now-unselected entry — and
// its worst-case contribution to the rollup verdict — until some unrelated
// event happens to trigger a reconcile. Evaluating old and new against a
// single List makes the result their union, deduped by construction.
func (r *ClusterHealthReconciler) clusterHealthsForHealthChecks(ctx context.Context, objs ...client.Object) []reconcile.Request {
	hcs := make([]*fathomv1alpha1.HealthCheck, 0, len(objs))
	for _, obj := range objs {
		// A typed-nil (*HealthCheck)(nil) in a non-nil client.Object satisfies
		// the assertion with ok==true, so the nil check is load-bearing.
		if hc, ok := obj.(*fathomv1alpha1.HealthCheck); ok && hc != nil {
			hcs = append(hcs, hc)
		}
	}
	if len(hcs) == 0 {
		return nil
	}

	var list fathomv1alpha1.ClusterHealthList
	if err := r.List(ctx, &list); err != nil {
		// Dropping the event here leaves a stale rollup published until some
		// unrelated event reconciles it, so make the loss visible.
		logf.FromContext(ctx).Error(err, "listing ClusterHealths to map a HealthCheck event; skipping enqueue")
		return nil
	}

	var out []reconcile.Request
	for i := range list.Items {
		ch := &list.Items[i]
		for _, hc := range hcs {
			if clusterHealthSelectsHealthCheck(ch, hc) {
				out = append(out, reconcile.Request{NamespacedName: types.NamespacedName{Name: ch.Name}})
				break // union: one request per ClusterHealth, however many sides matched
			}
		}
	}
	return out
}

// clusterHealthSelectsHealthCheck reports whether ch's aggregation scope covers
// hc: the namespace-scope filter admits hc's namespace and ch's selector
// matches hc's labels. An unparseable selector matches nothing here; Reconcile
// is what surfaces that to the user as Ready=False/InvalidSelector.
func clusterHealthSelectsHealthCheck(ch *fathomv1alpha1.ClusterHealth, hc *fathomv1alpha1.HealthCheck) bool {
	if !clusterHealthCoversNamespace(ch, hc.Namespace) {
		return false
	}
	sel, err := selectorFromSpec(ch.Spec.Selector)
	if err != nil {
		return false
	}
	return sel.Matches(labels.Set(hc.Labels))
}

// clusterHealthCoversNamespace reports whether ch aggregates HealthChecks in
// namespace under the ClusterHealthSpec precedence:
//
//  1. Namespaces non-empty → allowlist only (ExcludedNamespaces ignored).
//  2. Else ExcludedNamespaces non-empty → denylist.
//  3. Else open (every namespace).
func clusterHealthCoversNamespace(ch *fathomv1alpha1.ClusterHealth, namespace string) bool {
	if len(ch.Spec.Namespaces) > 0 {
		return slices.Contains(ch.Spec.Namespaces, namespace)
	}
	if len(ch.Spec.ExcludedNamespaces) > 0 {
		return !slices.Contains(ch.Spec.ExcludedNamespaces, namespace)
	}
	return true
}
