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
// open). When Namespaces is set the list is one Get-scope per allowlisted
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
// the worst-case Result over those with a non-empty Status.Result, builds a
// deterministic Children summary (sorted by namespace, then name), and sets
// ObservedAt to the latest input freshness across children (not wall-clock —
// that would defeat no-op idempotency, and a "when did inputs last move"
// timestamp is what dashboards actually want).
func (r *ClusterHealthReconciler) aggregate(ch *fathomv1alpha1.ClusterHealth, hcs []fathomv1alpha1.HealthCheck) {
	sort.Slice(hcs, func(i, j int) bool {
		if hcs[i].Namespace != hcs[j].Namespace {
			return hcs[i].Namespace < hcs[j].Namespace
		}
		return hcs[i].Name < hcs[j].Name
	})

	ch.Status.MatchedCount = int32(len(hcs))
	ch.Status.Children = make([]fathomv1alpha1.ClusterHealthChildSummary, 0, len(hcs))

	var worst fathomv1alpha1.HealthReportResult
	worstRank := 0
	var latest *metav1.Time
	for i := range hcs {
		hc := &hcs[i]
		ch.Status.Children = append(ch.Status.Children, fathomv1alpha1.ClusterHealthChildSummary{
			Namespace:  hc.Namespace,
			Name:       hc.Name,
			Result:     hc.Status.Result,
			Summary:    hc.Status.Summary,
			ObservedAt: hc.Status.SourceObservedAt,
		})
		if rank := hc.Status.Result.Severity(); rank > 0 && rank > worstRank {
			worst = hc.Status.Result
			worstRank = rank
		}
		if hc.Status.SourceObservedAt != nil && (latest == nil || hc.Status.SourceObservedAt.After(latest.Time)) {
			latest = hc.Status.SourceObservedAt
		}
	}
	ch.Status.Result = worst
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

// SetupWithManager sets up the controller with the Manager. It owns
// ClusterHealth and watches HealthCheck so a member's Status change
// re-enqueues every ClusterHealth whose selector matches it.
func (r *ClusterHealthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.ClusterHealth{}).
		Named("clusterhealth").
		Watches(
			&fathomv1alpha1.HealthCheck{},
			handler.EnqueueRequestsFromMapFunc(r.clusterHealthsForHealthCheck),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

// clusterHealthsForHealthCheck returns the names of every ClusterHealth whose
// scope covers hc: the selector matches hc's labels and the namespace-scope
// filter (allowlist / denylist / open) includes hc's namespace. ClusterHealth
// is cluster-scoped, so the requests carry no namespace.
func (r *ClusterHealthReconciler) clusterHealthsForHealthCheck(ctx context.Context, obj client.Object) []reconcile.Request {
	hc, ok := obj.(*fathomv1alpha1.HealthCheck)
	if !ok {
		return nil
	}
	var list fathomv1alpha1.ClusterHealthList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}
	hcLabels := labels.Set(hc.Labels)
	var out []reconcile.Request
	for _, ch := range list.Items {
		if !clusterHealthCoversNamespace(&ch, hc.Namespace) {
			continue
		}
		sel, err := selectorFromSpec(ch.Spec.Selector)
		if err != nil {
			continue
		}
		if !sel.Matches(hcLabels) {
			continue
		}
		out = append(out, reconcile.Request{NamespacedName: types.NamespacedName{Name: ch.Name}})
	}
	return out
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
