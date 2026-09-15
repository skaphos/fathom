/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
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

func TestClearScopedReportAccessDoesNotCreateMissingRole(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: types.UID("owner-uid")}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).Build()
	if err := clearScopedReportAccess(context.Background(), c, owner, "never-provisioned"); err != nil {
		t.Fatalf("clearScopedReportAccess() error = %v", err)
	}
	var roles rbacv1.RoleList
	if err := c.List(context.Background(), &roles, client.InNamespace("ns")); err != nil {
		t.Fatal(err)
	}
	if len(roles.Items) != 0 {
		t.Fatalf("clearScopedReportAccess() created roles: %v", roles.Items)
	}
}
