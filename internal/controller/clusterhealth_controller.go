/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
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

	"github.com/skaphos/fathom/internal/metrics"
	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
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
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=clusterhealths,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=clusterhealths/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=clusterhealths/finalizers,verbs=update

func (r *ClusterHealthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	defer func() {
		outcome := "success"
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
		apiMeta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
			Type:               clusterHealthConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: ch.Generation,
			Reason:             "InvalidSelector",
			Message:            err.Error(),
		})
		// Fall through to the diff/Update so the failure is visible.
	} else {
		var hcs fathomv1alpha1.HealthCheckList
		if listErr := r.List(ctx, &hcs,
			client.InNamespace(ch.Namespace),
			client.MatchingLabelsSelector{Selector: selector},
		); listErr != nil {
			apiMeta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
				Type:               clusterHealthConditionReady,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: ch.Generation,
				Reason:             "ListFailed",
				Message:            listErr.Error(),
			})
		} else {
			r.aggregate(&ch, hcs.Items)
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

// aggregate populates ch.Status from the selected HealthChecks. It computes
// the worst-case Result over those with a non-empty Status.Result, builds a
// deterministic Children summary (sorted by name), and sets ObservedAt to the
// latest input freshness across children (not wall-clock — that would defeat
// no-op idempotency, and a "when did inputs last move" timestamp is what
// dashboards actually want).
func (r *ClusterHealthReconciler) aggregate(ch *fathomv1alpha1.ClusterHealth, hcs []fathomv1alpha1.HealthCheck) {
	sort.Slice(hcs, func(i, j int) bool { return hcs[i].Name < hcs[j].Name })

	ch.Status.MatchedCount = int32(len(hcs))
	ch.Status.Children = make([]fathomv1alpha1.ClusterHealthChildSummary, 0, len(hcs))

	var worst fathomv1alpha1.HealthReportResult
	worstRank := 0
	var latest *metav1.Time
	for i := range hcs {
		hc := &hcs[i]
		ch.Status.Children = append(ch.Status.Children, fathomv1alpha1.ClusterHealthChildSummary{
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

// clusterHealthsForHealthCheck returns the namespaced names of every
// ClusterHealth in hc's namespace whose Selector matches hc's labels.
// Cross-namespace aggregation is intentionally not supported in v0.1; the
// schema documents that an empty/nil Selector means "same namespace".
func (r *ClusterHealthReconciler) clusterHealthsForHealthCheck(ctx context.Context, obj client.Object) []reconcile.Request {
	hc, ok := obj.(*fathomv1alpha1.HealthCheck)
	if !ok {
		return nil
	}
	var list fathomv1alpha1.ClusterHealthList
	if err := r.List(ctx, &list, client.InNamespace(hc.Namespace)); err != nil {
		return nil
	}
	hcLabels := labels.Set(hc.Labels)
	var out []reconcile.Request
	for _, ch := range list.Items {
		sel, err := selectorFromSpec(ch.Spec.Selector)
		if err != nil {
			continue
		}
		if !sel.Matches(hcLabels) {
			continue
		}
		out = append(out, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ch.Namespace, Name: ch.Name}})
	}
	return out
}
