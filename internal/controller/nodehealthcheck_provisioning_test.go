/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// TestNodeHealthProvisioningFailurePersistsStatus is the COR-2 property for
// this kind, written against the corrected shape rather than inherited: the
// check starts healthy (Ready=True, LastResult=Pass), provisioning then fails
// permanently, and the failure must reach the API server as Ready=False while
// the last complete verdict is retained — never a stale Ready=True/Pass, never
// a wiped verdict.
func TestNodeHealthProvisioningFailurePersistsStatus(t *testing.T) {
	t.Parallel()

	// Second precision: the fake client round-trips status through JSON, and
	// metav1.Time serialises without sub-second digits.
	lastRun := metav1.NewTime(time.Now().Add(-time.Minute).Truncate(time.Second))
	check := &fathomv1alpha1.NodeHealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "nh-provisioning", Namespace: "default", Generation: 1},
		Spec: fathomv1alpha1.NodeHealthCheckSpec{
			Checks: []fathomv1alpha1.NodeHealthCheckItem{{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet"}},
		},
		Status: fathomv1alpha1.NodeHealthCheckStatus{
			LastRunTime:    &lastRun,
			LastResult:     string(fathomv1alpha1.HealthReportResultPass),
			LastReportName: "nh-provisioning-report",
			Summary:        "2 of 2 node(s) passed",
			NodeResults:    []fathomv1alpha1.NodeHealthNodeResult{{Node: "node-a", Result: "Pass"}, {Node: "node-b", Result: "Pass"}},
			Conditions: []metav1.Condition{{
				Type:               nodeHealthConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             "Reporting",
				Message:            "Node-agents are reporting and a HealthReport was rolled up.",
				LastTransitionTime: metav1.NewTime(time.Now().Add(-time.Hour)),
			}},
		},
	}

	scheme := newProvisioningScheme(t)
	denied := apierrors.NewForbidden(schema.GroupResource{Group: "rbac.authorization.k8s.io", Resource: "clusterroles"}, defaultNodeAgentRoleName, context.DeadlineExceeded)
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(check).
		WithStatusSubresource(&fathomv1alpha1.NodeHealthCheck{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*rbacv1.ClusterRole); ok {
					return denied
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	r := &NodeHealthCheckReconciler{Client: cl, Scheme: scheme, NodeAgentImage: "img:test"}
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(check)})
	if err == nil {
		t.Fatal("expected the provisioning error to be returned for retry")
	}

	persisted := &fathomv1alpha1.NodeHealthCheck{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(check), persisted); err != nil {
		t.Fatal(err)
	}
	ready := apiMeta.FindStatusCondition(persisted.Status.Conditions, nodeHealthConditionReady)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != "RBACProvisioningFailed" {
		t.Fatalf("Ready must be persisted as False/RBACProvisioningFailed (COR-2), got %+v", ready)
	}
	if persisted.Status.LastResult != string(fathomv1alpha1.HealthReportResultPass) || persisted.Status.LastReportName != "nh-provisioning-report" {
		t.Fatalf("a provisioning failure must not clear the last complete verdict: %+v", persisted.Status)
	}
	if persisted.Status.LastRunTime == nil || !persisted.Status.LastRunTime.Time.Equal(lastRun.Time) {
		t.Fatalf("lastRunTime moved: %v", persisted.Status.LastRunTime)
	}
	if len(persisted.Status.NodeResults) != 2 || persisted.Status.Summary == "" {
		t.Fatalf("per-node results/summary must survive a provisioning failure: %+v", persisted.Status)
	}
	if persisted.Status.ObservedGeneration != 1 {
		t.Fatalf("observedGeneration = %d", persisted.Status.ObservedGeneration)
	}
}
