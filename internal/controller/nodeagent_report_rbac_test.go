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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestScopedReportAccessNameBoundsMaximumServiceAccount(t *testing.T) {
	serviceAccount := strings.Repeat("a", 253)
	got := scopedReportAccessName(serviceAccount)
	if len(got) != 53 || !strings.HasPrefix(got, "fathom-report-access-") {
		t.Fatalf("scopedReportAccessName(maximum ServiceAccount) = %q (len %d)", got, len(got))
	}
	if got != scopedReportAccessName(serviceAccount) {
		t.Fatal("scopedReportAccessName is not deterministic")
	}
	if got == scopedReportAccessName(strings.Repeat("b", 253)) {
		t.Fatal("distinct maximum-length ServiceAccounts produced the same scoped name")
	}
}

func TestActiveAgentReportNamesFiltersAndDeduplicatesPods(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	controller := true
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns", UID: types.UID("agent-uid")}}
	owned := []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: &controller}}
	deletedAt := metav1.NewTime(time.Now())
	labels := map[string]string{"agent": "one"}
	pods := []client.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "active-a", Namespace: "ns", Labels: labels, OwnerReferences: owned}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "active-a-rollout", Namespace: "ns", Labels: labels, OwnerReferences: owned}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: "ns", Labels: labels}, Spec: corev1.PodSpec{NodeName: "node-b"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "unscheduled", Namespace: "ns", Labels: labels, OwnerReferences: owned}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "terminating", Namespace: "ns", Labels: labels, OwnerReferences: owned, DeletionTimestamp: &deletedAt, Finalizers: []string{"test"}}, Spec: corev1.PodSpec{NodeName: "node-c"}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pods...).Build()
	names, err := activeAgentReportNames(context.Background(), c, "ns", labels, ds, func(node string) string { return "report-" + node })
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "report-node-a" {
		t.Fatalf("activeAgentReportNames() = %v, want [report-node-a]", names)
	}
}

func TestEnsureScopedReportRBACRejectsUnexpectedImmutableRoleRef(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: types.UID("owner-uid")}}
	name := scopedReportAccessName("agent")
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns", CreationTimestamp: metav1.Now()},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "foreign"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, binding).Build()
	err := ensureScopedReportRBAC(context.Background(), c, scheme, owner, nil, "agent", []string{"report-node-a"})
	if err == nil || !strings.Contains(err.Error(), "immutable roleRef") {
		t.Fatalf("ensureScopedReportRBAC() error = %v, want immutable roleRef error", err)
	}
}

func TestClearNodeAgentAccessDoesNotCreateMissingRBAC(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: types.UID("owner-uid")}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).Build()
	if err := clearNodeAgentAccess(context.Background(), c, owner, "never-provisioned", defaultNodeAgentRoleName); err != nil {
		t.Fatalf("clearNodeAgentAccess() error = %v", err)
	}
	var roles rbacv1.RoleList
	if err := c.List(context.Background(), &roles, client.InNamespace("ns")); err != nil {
		t.Fatal(err)
	}
	if len(roles.Items) != 0 {
		t.Fatalf("clearNodeAgentAccess() created roles: %v", roles.Items)
	}
	var bindings rbacv1.RoleBindingList
	if err := c.List(context.Background(), &bindings, client.InNamespace("ns")); err != nil {
		t.Fatal(err)
	}
	if len(bindings.Items) != 0 {
		t.Fatalf("clearNodeAgentAccess() created bindings: %v", bindings.Items)
	}
}

func TestClearNodeAgentAccessAttemptsBothGrantsWhenUpdatesFail(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: types.UID("owner-uid")}}
	controller := true
	owned := []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: owner.Name, UID: owner.UID, Controller: &controller}}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: scopedReportAccessName("agent"), Namespace: "ns", OwnerReferences: owned},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}}},
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns", OwnerReferences: owned},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: defaultNodeAgentRoleName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "agent", Namespace: "ns"}},
	}
	roleDenied := errors.New("role update denied")
	bindingDenied := errors.New("binding update denied")
	var roleUpdates, bindingUpdates int
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, role, binding).
		WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			switch obj.(type) {
			case *rbacv1.Role:
				roleUpdates++
				return roleDenied
			case *rbacv1.RoleBinding:
				bindingUpdates++
				return bindingDenied
			default:
				return c.Update(ctx, obj, opts...)
			}
		}}).Build()
	err := clearNodeAgentAccess(context.Background(), c, owner, "agent", defaultNodeAgentRoleName)
	if !errors.Is(err, roleDenied) || !errors.Is(err, bindingDenied) {
		t.Fatalf("clearNodeAgentAccess() error = %v, want both update failures", err)
	}
	if roleUpdates != 1 || bindingUpdates != 1 {
		t.Fatalf("update attempts = Role %d, RoleBinding %d; want one each", roleUpdates, bindingUpdates)
	}
}

func TestClearSharedAgentBindingAccessRefusesForeignBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: types.UID("owner-uid")}}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: defaultNodeAgentRoleName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "victim", Namespace: "ns"}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, binding).Build()
	if err := clearSharedAgentBindingAccess(context.Background(), c, owner, "agent", defaultNodeAgentRoleName); err == nil || !strings.Contains(err.Error(), "not controlled") {
		t.Fatalf("clearSharedAgentBindingAccess() error = %v, want ownership refusal", err)
	}
	got := &rbacv1.RoleBinding{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(binding), got); err != nil {
		t.Fatal(err)
	}
	if len(got.Subjects) != 1 || got.Subjects[0].Name != "victim" {
		t.Fatalf("foreign RoleBinding subjects changed: %+v", got.Subjects)
	}
}
