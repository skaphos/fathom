/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

// TestNodeHealthRejectedItemIsObservable pins that an item the allowlist
// refuses — reachable only for an object stored under an older CRD, since
// admission rejects it today — fails the check visibly instead of being
// filtered out: Accepted and Ready go False naming the item, and no agent is
// provisioned for the remainder. A spec that cannot measure what it declares
// must never report a vacuous verdict.
func TestNodeHealthRejectedItemIsObservable(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{fathomv1alpha1.AddToScheme, corev1.AddToScheme, appsv1.AddToScheme, rbacv1.AddToScheme, networkingv1.AddToScheme, admissionregistrationv1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	check := &fathomv1alpha1.NodeHealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "nh-bad", Namespace: "default", Generation: 3},
		Spec: fathomv1alpha1.NodeHealthCheckSpec{Checks: []fathomv1alpha1.NodeHealthCheckItem{
			{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/etc/shadow"},
			{Type: fathomv1alpha1.NodeHealthCheckInodeHeadroom, Path: "/var/lib/kubelet"},
		}},
	}
	// The previous, accepted generation left a host-network agent running.
	check.Generation = 4
	check.Status = fathomv1alpha1.NodeHealthCheckStatus{
		ObservedGeneration: 3, LastResult: "Pass", DesiredNodes: 3, ReportingNodes: 3,
		Conditions: []metav1.Condition{
			{Type: nodeHealthConditionPrivileged, Status: metav1.ConditionTrue, Reason: "HostNetwork", ObservedGeneration: 3, LastTransitionTime: metav1.Now()},
			{Type: nodeHealthConditionAgentReady, Status: metav1.ConditionTrue, Reason: "RolledOut", ObservedGeneration: 3, LastTransitionTime: metav1.Now()},
		},
	}
	oldAgent := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: nodeHealthAgentResourceName(check), Namespace: "default"},
		Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{HostNetwork: true}}}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check, oldAgent).WithStatusSubresource(&fathomv1alpha1.NodeHealthCheck{}).Build()
	r := &NodeHealthCheckReconciler{Client: cl, Scheme: scheme, NodeAgentImage: "img", APIReader: cl}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "nh-bad", Namespace: "default"}}); err != nil {
		t.Fatalf("a rejected item is a status outcome, not a reconcile error: %v", err)
	}
	got := &fathomv1alpha1.NodeHealthCheck{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "nh-bad", Namespace: "default"}, got); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{nodeHealthConditionAccepted, nodeHealthConditionReady} {
		c := apiMeta.FindStatusCondition(got.Status.Conditions, typ)
		if c == nil || c.Status != metav1.ConditionFalse || c.Reason != conditionReasonItemsRejected || !strings.Contains(c.Message, "DiskHeadroom path /etc/shadow") {
			t.Fatalf("%s = %+v, want False/%s naming the item", typ, c, conditionReasonItemsRejected)
		}
	}
	var ds appsv1.DaemonSetList
	if err := cl.List(context.Background(), &ds); err != nil || len(ds.Items) != 0 {
		t.Fatalf("the previous generation's agent must be revoked, not left running under a refused spec: %d DaemonSet(s), err=%v", len(ds.Items), err)
	}
	priv := apiMeta.FindStatusCondition(got.Status.Conditions, nodeHealthConditionPrivileged)
	if priv == nil || priv.Status != metav1.ConditionFalse || priv.Reason != conditionReasonItemsRejected || priv.ObservedGeneration != 4 {
		t.Fatalf("AgentPrivileged = %+v, want False/ItemsRejected at the current generation (nothing is running)", priv)
	}
	if got.Status.LastResult != "Pass" || got.Status.DesiredNodes != 0 {
		t.Fatalf("the last verdict is retained frozen and desiredNodes is 0: %+v", got.Status)
	}
}

// TestNodeHealthProvisioningFailureInvalidatesAgentConditions pins that a
// provisioning failure on a new generation does not leave the previous
// generation's AgentReady=True / CoverageComplete=True advertised next to
// Ready=False, and that the privilege posture reflects the new spec.
func TestNodeHealthProvisioningFailureInvalidatesAgentConditions(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{fathomv1alpha1.AddToScheme, corev1.AddToScheme, appsv1.AddToScheme, rbacv1.AddToScheme, networkingv1.AddToScheme, admissionregistrationv1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	check := &fathomv1alpha1.NodeHealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "nh-stale", Namespace: "default", Generation: 2},
		Spec: fathomv1alpha1.NodeHealthCheckSpec{Checks: []fathomv1alpha1.NodeHealthCheckItem{
			{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet"},
			{Type: fathomv1alpha1.NodeHealthCheckKubeletHealthz},
		}},
		Status: fathomv1alpha1.NodeHealthCheckStatus{
			ObservedGeneration: 1, LastResult: "Pass",
			Conditions: []metav1.Condition{
				{Type: nodeHealthConditionReady, Status: metav1.ConditionTrue, Reason: "Reporting", ObservedGeneration: 1, LastTransitionTime: metav1.Now()},
				{Type: nodeHealthConditionAgentReady, Status: metav1.ConditionTrue, Reason: "RolledOut", ObservedGeneration: 1, LastTransitionTime: metav1.Now()},
				{Type: nodeHealthConditionCoverage, Status: metav1.ConditionTrue, Reason: "AllNodesReporting", ObservedGeneration: 1, LastTransitionTime: metav1.Now()},
				{Type: nodeHealthConditionPrivileged, Status: metav1.ConditionFalse, Reason: "Hardened", ObservedGeneration: 1, LastTransitionTime: metav1.Now()},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check).WithStatusSubresource(&fathomv1alpha1.NodeHealthCheck{}).
		WithInterceptorFuncs(interceptor.Funcs{Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*appsv1.DaemonSet); ok {
				return apierrors.NewInternalError(errors.New("daemonset create refused"))
			}
			return c.Create(ctx, obj, opts...)
		}}).Build()
	r := &NodeHealthCheckReconciler{Client: cl, Scheme: scheme, NodeAgentImage: "img", APIReader: cl}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "nh-stale", Namespace: "default"}}); err == nil {
		t.Fatal("the provisioning error must be returned for retry")
	}
	got := &fathomv1alpha1.NodeHealthCheck{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "nh-stale", Namespace: "default"}, got); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{nodeHealthConditionReady, nodeHealthConditionAgentReady, nodeHealthConditionCoverage} {
		c := apiMeta.FindStatusCondition(got.Status.Conditions, typ)
		if c == nil || c.Status != metav1.ConditionFalse || c.ObservedGeneration != 2 {
			t.Fatalf("%s = %+v, want False at generation 2", typ, c)
		}
	}
	priv := apiMeta.FindStatusCondition(got.Status.Conditions, nodeHealthConditionPrivileged)
	if priv == nil || priv.Status != metav1.ConditionTrue || priv.ObservedGeneration != 2 {
		t.Fatalf("AgentPrivileged = %+v, want True at generation 2 (KubeletHealthz needs the host network)", priv)
	}
	if got.Status.LastResult != "Pass" {
		t.Fatal("the last complete verdict must be retained (COR-2)")
	}
}
