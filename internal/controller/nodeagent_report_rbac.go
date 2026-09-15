/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const scopedReportAccessSuffix = "-report-access"

func scopedReportAccessName(serviceAccount string) string {
	if len(serviceAccount)+len(scopedReportAccessSuffix) <= 253 {
		return serviceAccount + scopedReportAccessSuffix
	}
	sum := sha256.Sum256([]byte(serviceAccount))
	return "fathom-report-access-" + hex.EncodeToString(sum[:])[:32]
}

// clearScopedReportAccess revokes an existing check-owned Role without
// creating RBAC for a check that never provisioned an agent.
func clearScopedReportAccess(ctx context.Context, c client.Client, owner client.Object, serviceAccount string) error {
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: scopedReportAccessName(serviceAccount), Namespace: owner.GetNamespace()}}
	if err := c.Get(ctx, client.ObjectKeyFromObject(role), role); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if !metav1.IsControlledBy(role, owner) {
		return fmt.Errorf("refusing to modify report-access role %s/%s not controlled by %s", role.Namespace, role.Name, owner.GetName())
	}
	if len(role.Rules) == 0 {
		return nil
	}
	role.Rules = nil
	return c.Update(ctx, role)
}

// clearSharedAgentBindingAccess revokes the agent's create capability without
// deleting its owner-referenced RoleBinding. Reconciliation restores the sole
// expected subject when the check can safely run again.
func clearSharedAgentBindingAccess(ctx context.Context, c client.Client, owner client.Object, serviceAccount, expectedClusterRole string) error {
	binding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: serviceAccount, Namespace: owner.GetNamespace()}}
	if err := c.Get(ctx, client.ObjectKeyFromObject(binding), binding); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if !metav1.IsControlledBy(binding, owner) {
		return fmt.Errorf("refusing to modify node-agent rolebinding %s/%s not controlled by %s", binding.Namespace, binding.Name, owner.GetName())
	}
	expected := rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: expectedClusterRole}
	if binding.RoleRef != expected {
		return fmt.Errorf("rolebinding %s/%s has immutable roleRef %s/%s, want ClusterRole/%s", binding.Namespace, binding.Name, binding.RoleRef.Kind, binding.RoleRef.Name, expected.Name)
	}
	if len(binding.Subjects) == 0 {
		return nil
	}
	binding.Subjects = nil
	return c.Update(ctx, binding)
}

// clearNodeAgentAccess independently revokes the agent's named get/update and
// shared create grants, returning every failure so callers can report partial
// cleanup honestly.
func clearNodeAgentAccess(ctx context.Context, c client.Client, owner client.Object, serviceAccount, expectedClusterRole string) error {
	return errors.Join(
		clearScopedReportAccess(ctx, c, owner, serviceAccount),
		clearSharedAgentBindingAccess(ctx, c, owner, serviceAccount, expectedClusterRole),
	)
}

func activeAgentReportNames(ctx context.Context, reader client.Reader, namespace string, labels map[string]string, ds *appsv1.DaemonSet, reportName func(string) string) ([]string, error) {
	var pods corev1.PodList
	if err := reader.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	nameSet := make(map[string]struct{}, len(pods.Items))
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.DeletionTimestamp != nil || !metav1.IsControlledBy(pod, ds) || pod.Spec.NodeName == "" {
			continue
		}
		nameSet[reportName(pod.Spec.NodeName)] = struct{}{}
	}
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ensureScopedReportRBAC grants a check's agent get/update only on reports for
// its current pods. An empty fleet produces no rules: an empty resourceNames
// list would otherwise match every ConfigMap.
func ensureScopedReportRBAC(ctx context.Context, c client.Client, scheme *runtime.Scheme, owner client.Object, labels map[string]string, serviceAccount string, reportNames []string) error {
	name := scopedReportAccessName(serviceAccount)
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: owner.GetNamespace()}}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, role, func() error {
		role.Labels = mergeLabels(role.Labels, labels)
		role.Rules = nil
		if len(reportNames) > 0 {
			role.Rules = []rbacv1.PolicyRule{{
				APIGroups:     []string{""},
				Resources:     []string{"configmaps"},
				ResourceNames: append([]string(nil), reportNames...),
				Verbs:         []string{"get", "update"},
			}}
		}
		return controllerutil.SetControllerReference(owner, role, scheme)
	}); err != nil {
		return err
	}

	binding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: owner.GetNamespace()}}
	_, err := controllerutil.CreateOrUpdate(ctx, c, binding, func() error {
		binding.Labels = mergeLabels(binding.Labels, labels)
		if binding.CreationTimestamp.IsZero() {
			binding.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name}
		} else if binding.RoleRef.APIGroup != rbacv1.GroupName || binding.RoleRef.Kind != "Role" || binding.RoleRef.Name != name {
			return fmt.Errorf("rolebinding %s/%s has immutable roleRef %s/%s, want Role/%s", binding.Namespace, binding.Name, binding.RoleRef.Kind, binding.RoleRef.Name, name)
		}
		binding.Subjects = []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: serviceAccount, Namespace: owner.GetNamespace()}}
		return controllerutil.SetControllerReference(owner, binding, scheme)
	})
	return err
}
