/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// ClusterHealthReconciler reconciles a ClusterHealth object
type ClusterHealthReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=clusterhealths,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=clusterhealths/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=clusterhealths/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ClusterHealth object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *ClusterHealthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("namespacedName", req.NamespacedName)
	log.V(1).Info("reconciliation requested")

	// TODO(SKA-310): implement ClusterHealth reconciliation body.
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterHealthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.ClusterHealth{}).
		Named("clusterhealth").
		Complete(r)
}
