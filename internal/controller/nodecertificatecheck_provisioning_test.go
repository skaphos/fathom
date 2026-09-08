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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func newProvisioningScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add fathom scheme: %v", err)
	}
	return scheme
}

// TestNodeCertProvisioningFailurePersistsStatus is the COR-2 regression.
//
// The check starts from a healthy persisted status (Ready=True, LastResult=Pass)
// — the state a check reaches after provisioning has succeeded at least once.
// Provisioning then starts failing permanently. Before the fix the reconciler
// set Ready=False in memory and returned the error without ever calling
// finish(), so nothing reached the API server and the check kept advertising
// Ready=True/Pass indefinitely.
func TestNodeCertProvisioningFailurePersistsStatus(t *testing.T) {
	t.Parallel()

	lastRun := metav1.NewTime(time.Now().Add(-time.Minute))
	check := &fathomv1alpha1.NodeCertificateCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "nc-provisioning", Namespace: "default", Generation: 1},
		Status: fathomv1alpha1.NodeCertificateCheckStatus{
			LastRunTime:    &lastRun,
			LastResult:     string(fathomv1alpha1.HealthReportResultPass),
			LastReportName: "nc-provisioning-report",
			Conditions: []metav1.Condition{{
				Type:               nodeCertConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             "Reporting",
				Message:            "Node-agents are reporting and a HealthReport was rolled up.",
				LastTransitionTime: metav1.NewTime(time.Now().Add(-time.Hour)),
			}},
		},
	}

	scheme := newProvisioningScheme(t)
	denied := apierrors.NewForbidden(schema.GroupResource{Group: "rbac.authorization.k8s.io", Resource: "clusterroles"}, "fathom-node-agent-role", context.DeadlineExceeded)
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(check).
		WithStatusSubresource(&fathomv1alpha1.NodeCertificateCheck{}).
		WithInterceptorFuncs(interceptor.Funcs{
			// The node-agent ClusterRole is the first thing Reconcile ensures, so
			// failing it exercises the earliest ensure-failure return.
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*rbacv1.ClusterRole); ok {
					return denied
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	r := &NodeCertificateCheckReconciler{
		Client:            cl,
		Scheme:            scheme,
		NodeAgentImage:    "ghcr.io/skaphos/fathom-node-agent:test",
		NodeAgentRoleName: defaultNodeAgentRoleName,
	}

	key := client.ObjectKeyFromObject(check)
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err == nil {
		t.Fatal("expected the provisioning error to be returned so the controller retries")
	}

	var persisted fathomv1alpha1.NodeCertificateCheck
	if err := cl.Get(context.Background(), key, &persisted); err != nil {
		t.Fatalf("get check: %v", err)
	}

	ready := apiMeta.FindStatusCondition(persisted.Status.Conditions, nodeCertConditionReady)
	if ready == nil {
		t.Fatal("Ready condition missing from persisted status")
	}
	if ready.Status != metav1.ConditionFalse {
		t.Errorf("persisted Ready = %s, want False: a check whose provisioning is failing must not keep advertising healthy", ready.Status)
	}
	if ready.Reason != "RBACProvisioningFailed" {
		t.Errorf("persisted Ready reason = %q, want %q", ready.Reason, "RBACProvisioningFailed")
	}

	// The verdict itself is frozen, not wiped: provisioning failing says nothing
	// about what the last complete scan found (COR-3).
	if got := persisted.Status.LastResult; got != string(fathomv1alpha1.HealthReportResultPass) {
		t.Errorf("persisted LastResult = %q, want the frozen prior verdict %q", got, fathomv1alpha1.HealthReportResultPass)
	}
}
