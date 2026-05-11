/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

const (
	addonCheckConditionAccepted = "Accepted"
	addonCheckConditionPaused   = "Paused"
)

// AddonCheckReconciler reconciles an AddonCheck object.
type AddonCheckReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks/finalizers,verbs=update

// Reconcile records that the AddonCheck spec has been observed. Adapter
// dispatch and HealthReport creation are wired in follow-up SKA-46 work once
// the registry is available to the reconciler.
func (r *AddonCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("namespacedName", req.NamespacedName)

	var check fathomv1alpha1.AddonCheck
	if err := r.Get(ctx, req.NamespacedName, &check); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	before := check.Status.DeepCopy()
	check.Status.ObservedGeneration = check.Generation
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               addonCheckConditionAccepted,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "SpecAccepted",
		Message:            "AddonCheck specification has been accepted for reconciliation.",
	})

	pausedStatus := metav1.ConditionFalse
	pausedReason := "RunEnabled"
	pausedMessage := "AddonCheck is eligible for adapter execution."
	if check.Spec.Paused {
		pausedStatus = metav1.ConditionTrue
		pausedReason = "Paused"
		pausedMessage = "AddonCheck is paused; adapter execution is disabled."
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               addonCheckConditionPaused,
		Status:             pausedStatus,
		ObservedGeneration: check.Generation,
		Reason:             pausedReason,
		Message:            pausedMessage,
	})

	if equality.Semantic.DeepEqual(before, &check.Status) {
		return ctrl.Result{}, nil
	}
	if err := r.Status().Update(ctx, &check); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("updated AddonCheck status")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AddonCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.AddonCheck{}).
		Named("addoncheck").
		Complete(r)
}
