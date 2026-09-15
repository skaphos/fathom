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

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"

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
	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/internal/nodehealth"
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

// TestNodeHealthRejectedSpecWithFailedRevocation pins that a failed
// revocation is still a rejected generation: Accepted=False is persisted
// (never Accepted=True for a refused spec), AgentPrivileged says the previous
// agent may still be running, Ready carries the error, the verdict is kept,
// and the error is returned for retry.
func TestNodeHealthRejectedSpecWithFailedRevocation(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{fathomv1alpha1.AddToScheme, corev1.AddToScheme, appsv1.AddToScheme, rbacv1.AddToScheme, networkingv1.AddToScheme, admissionregistrationv1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	check := &fathomv1alpha1.NodeHealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "nh-stuck", Namespace: "default", Generation: 2},
		Spec:       fathomv1alpha1.NodeHealthCheckSpec{Checks: []fathomv1alpha1.NodeHealthCheckItem{{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/etc/shadow"}}},
		Status: fathomv1alpha1.NodeHealthCheckStatus{ObservedGeneration: 1, LastResult: "Pass", Conditions: []metav1.Condition{
			{Type: nodeHealthConditionAccepted, Status: metav1.ConditionTrue, Reason: "SpecAccepted", ObservedGeneration: 1, LastTransitionTime: metav1.Now()},
			{Type: nodeHealthConditionPrivileged, Status: metav1.ConditionTrue, Reason: "HostNetwork", ObservedGeneration: 1, LastTransitionTime: metav1.Now()},
		}},
	}
	agent := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: nodeHealthAgentResourceName(check), Namespace: "default"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check, agent).WithStatusSubresource(&fathomv1alpha1.NodeHealthCheck{}).
		WithInterceptorFuncs(interceptor.Funcs{Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if _, ok := obj.(*appsv1.DaemonSet); ok {
				return apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, obj.GetName(), errors.New("delete denied"))
			}
			return c.Delete(ctx, obj, opts...)
		}}).Build()
	r := &NodeHealthCheckReconciler{Client: cl, Scheme: scheme, NodeAgentImage: "img", APIReader: cl}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "nh-stuck", Namespace: "default"}}); err == nil {
		t.Fatal("a failed revocation must be returned for retry")
	}
	got := &fathomv1alpha1.NodeHealthCheck{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "nh-stuck", Namespace: "default"}, got); err != nil {
		t.Fatal(err)
	}
	acc := apiMeta.FindStatusCondition(got.Status.Conditions, nodeHealthConditionAccepted)
	if acc == nil || acc.Status != metav1.ConditionFalse || acc.Reason != conditionReasonItemsRejected || acc.ObservedGeneration != 2 {
		t.Fatalf("Accepted = %+v, want False/ItemsRejected at generation 2 even though revocation failed", acc)
	}
	priv := apiMeta.FindStatusCondition(got.Status.Conditions, nodeHealthConditionPrivileged)
	if priv == nil || priv.Status != metav1.ConditionUnknown || priv.Reason != "AgentRevocationFailed" || priv.ObservedGeneration != 2 {
		t.Fatalf("AgentPrivileged = %+v, want Unknown/AgentRevocationFailed at generation 2", priv)
	}
	ready := apiMeta.FindStatusCondition(got.Status.Conditions, nodeHealthConditionReady)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != "AgentRevocationFailed" {
		t.Fatalf("Ready = %+v, want False/AgentRevocationFailed", ready)
	}
	if got.Status.LastResult != "Pass" {
		t.Fatal("the last complete verdict must be retained")
	}
}

// TestNodeHealthInvertedThresholdsAreRejected pins that an older-CRD object
// whose critical threshold sits above the effective warning threshold is a
// rejected generation, not a silently rewritten policy.
func TestNodeHealthInvertedThresholdsAreRejected(t *testing.T) {
	t.Parallel()
	check := &fathomv1alpha1.NodeHealthCheck{Spec: fathomv1alpha1.NodeHealthCheckSpec{Checks: []fathomv1alpha1.NodeHealthCheckItem{
		{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet", CriticalPercentFree: ptr.To[int32](30)}, // warn defaults to 20
		{Type: fathomv1alpha1.NodeHealthCheckInodeHeadroom, Path: "/var/log", WarnPercentFree: ptr.To[int32](50), CriticalPercentFree: ptr.To[int32](10)},
	}}}
	rejected := rejectedNodeHealthItems(check)
	if len(rejected) != 1 || !strings.Contains(rejected[0], "criticalPercentFree 30 above warnPercentFree 20") {
		t.Fatalf("rejected = %v, want exactly the inverted DiskHeadroom item", rejected)
	}
}

// TestNodeHealthAuthenticityUnenforcedWithoutPolicyAPI pins that on a cluster
// without ValidatingAdmissionPolicy the controller fails closed before it
// provisions an agent: collect-time bindings are forgeable by another
// ConfigMap writer and cannot substitute for admission enforcement.
func TestNodeHealthAuthenticityUnenforcedWithoutPolicyAPI(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{fathomv1alpha1.AddToScheme, corev1.AddToScheme, appsv1.AddToScheme, rbacv1.AddToScheme, networkingv1.AddToScheme, admissionregistrationv1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	check := &fathomv1alpha1.NodeHealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "nh-novap", Namespace: "default", Generation: 1},
		Spec:       fathomv1alpha1.NodeHealthCheckSpec{Checks: []fathomv1alpha1.NodeHealthCheckItem{{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet"}}},
	}
	noMatch := &apiMeta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "admissionregistration.k8s.io", Kind: "ValidatingAdmissionPolicy"}, SearchedVersions: []string{"v1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check).WithStatusSubresource(&fathomv1alpha1.NodeHealthCheck{}).
		WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			switch obj.(type) {
			case *admissionregistrationv1.ValidatingAdmissionPolicy, *admissionregistrationv1.ValidatingAdmissionPolicyBinding:
				return noMatch
			}
			return c.Get(ctx, key, obj, opts...)
		}}).Build()
	r := &NodeHealthCheckReconciler{Client: cl, Scheme: scheme, NodeAgentImage: "img", APIReader: cl}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "nh-novap", Namespace: "default"}}); err == nil {
		t.Fatal("an unsupported policy API must fail closed and retry")
	}
	got := &fathomv1alpha1.NodeHealthCheck{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "nh-novap", Namespace: "default"}, got); err != nil {
		t.Fatal(err)
	}
	c := apiMeta.FindStatusCondition(got.Status.Conditions, nodeHealthConditionAuthentic)
	if c == nil || c.Status != metav1.ConditionUnknown || c.Reason != reasonAuthenticityUnavailable || c.ObservedGeneration != got.Generation {
		t.Fatalf("ReportsAuthentic = %+v, want Unknown/%s at generation %d", c, reasonAuthenticityUnavailable, got.Generation)
	}
	ready := apiMeta.FindStatusCondition(got.Status.Conditions, nodeHealthConditionReady)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != "AdmissionPolicyProvisioningFailed" {
		t.Fatalf("Ready = %+v, want False/AdmissionPolicyProvisioningFailed", ready)
	}
	var daemonSets appsv1.DaemonSetList
	if err := cl.List(context.Background(), &daemonSets, client.InNamespace(check.Namespace)); err != nil {
		t.Fatal(err)
	}
	if len(daemonSets.Items) != 0 {
		t.Fatalf("unsupported authenticity enforcement provisioned %d agent DaemonSet(s)", len(daemonSets.Items))
	}
}

// TestNodeHealthEvaluationIgnoresDepartedNodes pins that a surplus fresh
// report cannot make the current fleet unevaluable. Node reads are needed only
// for reports that still belong to nodes in scope; a transient read failure
// for a departed node must not abort the roll-up for the live fleet.
func TestNodeHealthEvaluationIgnoresDepartedNodes(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	live := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-live"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type: corev1.NodeReady, Status: corev1.ConditionTrue,
		}}},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(live).
		WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Node); ok && key.Name == "node-departed" {
				return apierrors.NewInternalError(errors.New("departed node read failed"))
			}
			return c.Get(ctx, key, obj, opts...)
		}}).Build()
	check := &fathomv1alpha1.NodeHealthCheck{Spec: fathomv1alpha1.NodeHealthCheckSpec{Checks: []fathomv1alpha1.NodeHealthCheckItem{{
		Type: fathomv1alpha1.NodeHealthCheckNodeCondition, Conditions: []string{"Ready"},
	}}}}
	reports := []nodehealth.NodeReport{{Node: "node-departed"}, {Node: "node-live"}}

	evals, err := (&NodeHealthCheckReconciler{Client: cl, APIReader: cl}).evaluateNodes(
		context.Background(), logr.Discard(), check, reports, map[string]struct{}{"node-live": {}},
	)
	if err != nil {
		t.Fatalf("a departed node's read failure blocked the live fleet: %v", err)
	}
	if len(evals) != 1 || evals[0].Node != "node-live" || evals[0].Outcome != nodehealth.OutcomePass {
		t.Fatalf("evaluations = %+v, want one passing live node", evals)
	}
}

// TestNodeHealthEvaluationFailuresPersistStatus covers every API boundary
// after the agent has been provisioned. Each failure must be observable on the
// current generation without rewriting the last complete roll-up (COR-3).
func TestNodeHealthEvaluationFailuresPersistStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		stage string
	}{
		{name: "report ConfigMap list", stage: "reports"},
		{name: "agent pod list", stage: "pods"},
		{name: "Node read", stage: "node"},
		{name: "HealthReport create", stage: "healthreport"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			scheme := newProvisioningScheme(t)
			lastRun := metav1.NewTime(time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC))
			resultObservedAt := metav1.NewTime(lastRun.Add(-time.Minute))
			check := &fathomv1alpha1.NodeHealthCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "nh-evaluation-error", Namespace: "default", UID: "check-uid", Generation: 2},
				Spec: fathomv1alpha1.NodeHealthCheckSpec{Checks: []fathomv1alpha1.NodeHealthCheckItem{
					{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet"},
					{Type: fathomv1alpha1.NodeHealthCheckNodeCondition, Conditions: []string{"Ready"}},
				}},
				Status: fathomv1alpha1.NodeHealthCheckStatus{
					ObservedGeneration: 1,
					LastRunTime:        &lastRun,
					LastResult:         string(fathomv1alpha1.HealthReportResultPass),
					Summary:            "1 of 1 node(s) passed",
					LastReportName:     "nh-evaluation-error-previous",
					NodeResults: []fathomv1alpha1.NodeHealthNodeResult{{
						Node: "node-live", Result: string(fathomv1alpha1.HealthReportResultPass), ObservedAt: &resultObservedAt,
					}},
					DesiredNodes: 1, ReportingNodes: 1,
					Conditions: []metav1.Condition{
						{Type: nodeHealthConditionReady, Status: metav1.ConditionTrue, Reason: "Reporting", ObservedGeneration: 1, LastTransitionTime: lastRun},
						{Type: nodeHealthConditionCoverage, Status: metav1.ConditionTrue, Reason: "AllNodesReporting", ObservedGeneration: 1, LastTransitionTime: lastRun},
						{Type: nodeHealthConditionAgentReady, Status: metav1.ConditionTrue, Reason: "RolledOut", ObservedGeneration: 1, LastTransitionTime: lastRun},
					},
				},
			}

			items := resolveNodeHealthItems(check)
			renderer := &NodeHealthCheckReconciler{NodeAgentImage: "img:test"}
			ds := renderer.desiredDaemonSet(check, nodeHealthAgentResourceName(check), items)
			ds.UID = "daemonset-uid"
			ds.Generation = 1
			ds.CreationTimestamp = lastRun
			ds.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: nodeHealthKind,
				Name: check.Name, UID: check.UID, Controller: ptr.To(true),
			}}
			ds.Annotations = map[string]string{nodeAgentSpecHashAnnotation: nodeAgentSpecHash(ds)}
			ds.Status = appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 1, CurrentNumberScheduled: 1, UpdatedNumberScheduled: 1,
				NumberAvailable: 1, NumberReady: 1, ObservedGeneration: ds.Generation,
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nh-evaluation-error-node-live", Namespace: check.Namespace,
					Labels: nodeHealthAgentSelectorLabels(check),
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: ptr.To(true),
					}},
				},
				Spec: corev1.PodSpec{NodeName: "node-live", Containers: []corev1.Container{{Name: "node-agent", Image: "img:test"}}},
			}
			failedCheck := nodehealth.CheckResult{
				Type: nodehealth.TypeDiskHeadroom, Path: "/var/lib/kubelet", Outcome: nodehealth.OutcomeFail, Summary: "disk full",
			}
			report := nodehealth.NodeReport{
				Node: "node-live", CheckName: check.Name, ObservedAt: lastRun.Add(time.Minute),
				Aggregate: nodehealth.OutcomeFail, Checks: []nodehealth.CheckResult{failedCheck},
				ItemsDigest: nodehealth.ItemsDigest(nodeHealthAgentItems(items), nodeHealthAgentTimeout(check)),
			}
			encoded, err := nodehealth.EncodeReport(report)
			if err != nil {
				t.Fatal(err)
			}
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: nodehealth.ReportConfigMapName(check.Name, report.Node), Namespace: check.Namespace,
				Labels: map[string]string{
					nodecert.LabelManagedBy: nodecert.ManagedByValue, nodecert.LabelSourceKind: nodeHealthKind,
					nodecert.LabelSourceName: check.Name,
				},
				Annotations: map[string]string{nodecert.AnnotationNodeName: report.Node},
			}, Data: map[string]string{nodecert.ConfigMapReportKey: encoded}}
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: report.Node}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
				Type: corev1.NodeReady, Status: corev1.ConditionTrue,
			}}}}

			injected := errors.New("injected " + tt.stage + " failure")
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check, ds, pod, cm, node).
				WithStatusSubresource(&fathomv1alpha1.NodeHealthCheck{}).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						switch list.(type) {
						case *corev1.ConfigMapList:
							if tt.stage == "reports" {
								return injected
							}
						case *corev1.PodList:
							if tt.stage == "pods" {
								return injected
							}
						}
						return c.List(ctx, list, opts...)
					},
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if tt.stage == "node" {
							if _, ok := obj.(*corev1.Node); ok {
								return injected
							}
						}
						return c.Get(ctx, key, obj, opts...)
					},
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if tt.stage == "healthreport" {
							if _, ok := obj.(*fathomv1alpha1.HealthReport); ok {
								return injected
							}
						}
						return c.Create(ctx, obj, opts...)
					},
				}).Build()
			r := &NodeHealthCheckReconciler{
				Client: cl, APIReader: cl, Scheme: scheme, NodeAgentImage: "img:test",
				Clock: func() time.Time { return lastRun.Add(time.Minute) },
			}
			_, err = r.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(check)})
			if !errors.Is(err, injected) {
				t.Fatalf("Reconcile error = %v, want original %v", err, injected)
			}

			persisted := &fathomv1alpha1.NodeHealthCheck{}
			if err := cl.Get(context.Background(), client.ObjectKeyFromObject(check), persisted); err != nil {
				t.Fatal(err)
			}
			if persisted.Status.ObservedGeneration != check.Generation {
				t.Fatalf("observedGeneration = %d, want %d", persisted.Status.ObservedGeneration, check.Generation)
			}
			for _, typ := range []string{nodeHealthConditionReady, nodeHealthConditionCoverage} {
				condition := apiMeta.FindStatusCondition(persisted.Status.Conditions, typ)
				if condition == nil || condition.Reason != "EvaluationFailed" || condition.ObservedGeneration != check.Generation {
					t.Fatalf("%s = %+v, want EvaluationFailed at generation %d", typ, condition, check.Generation)
				}
				if typ == nodeHealthConditionReady && condition.Status != metav1.ConditionFalse {
					t.Fatalf("Ready = %+v, want False", condition)
				}
				if typ == nodeHealthConditionCoverage && condition.Status != metav1.ConditionUnknown {
					t.Fatalf("CoverageComplete = %+v, want Unknown", condition)
				}
			}
			agentReady := apiMeta.FindStatusCondition(persisted.Status.Conditions, nodeHealthConditionAgentReady)
			if agentReady == nil || agentReady.Status != metav1.ConditionTrue || agentReady.ObservedGeneration != check.Generation {
				t.Fatalf("AgentReady = %+v, want the known rolled-out state at generation %d", agentReady, check.Generation)
			}
			if persisted.Status.LastResult != check.Status.LastResult || persisted.Status.Summary != check.Status.Summary ||
				persisted.Status.LastReportName != check.Status.LastReportName || persisted.Status.LastRunTime == nil ||
				!persisted.Status.LastRunTime.Equal(check.Status.LastRunTime) || len(persisted.Status.NodeResults) != 1 ||
				persisted.Status.NodeResults[0].Result != check.Status.NodeResults[0].Result ||
				!persisted.Status.NodeResults[0].ObservedAt.Equal(check.Status.NodeResults[0].ObservedAt) {
				t.Fatalf("last complete roll-up changed: got %+v, want historical %+v", persisted.Status, check.Status)
			}
		})
	}
}
