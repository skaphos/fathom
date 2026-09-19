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
// creating RBAC for a check that never provisioned an agent. reader must be
// uncached: absence from a label-filtered cache is not proof that access has
// already been revoked. Writes remain on c.
func clearScopedReportAccess(ctx context.Context, reader client.Reader, c client.Client, owner client.Object, serviceAccount string) error {
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: scopedReportAccessName(serviceAccount), Namespace: owner.GetNamespace()}}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(role), role); err != nil {
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

// clearSharedAgentBindingAccess drains a legacy binding to the shared
// ClusterRole without deleting or repurposing it. Current reconciliation uses
// the per-check report-access RoleBinding for all ConfigMap access. reader must
// be uncached because legacy bindings can predate the manager cache's label.
func clearSharedAgentBindingAccess(ctx context.Context, reader client.Reader, c client.Client, owner client.Object, serviceAccount, expectedClusterRole string) error {
	binding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: serviceAccount, Namespace: owner.GetNamespace()}}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(binding), binding); err != nil {
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

// clearNodeAgentAccess independently revokes the agent's per-check
// create/get/update access and legacy shared grant, returning every failure so
// callers can report partial cleanup honestly. reader must be uncached; a
// filtered-cache miss cannot establish that either grant is absent.
func clearNodeAgentAccess(ctx context.Context, reader client.Reader, c client.Client, owner client.Object, serviceAccount, expectedClusterRole string) error {
	return errors.Join(
		clearScopedReportAccess(ctx, reader, c, owner, serviceAccount),
		clearSharedAgentBindingAccess(ctx, reader, c, owner, serviceAccount, expectedClusterRole),
	)
}

// deleteOwnedNodeAgentDaemonSet removes only the DaemonSet controlled by the
// check. Preconditions bind the delete to the object that passed the ownership
// check, so a same-name replacement cannot be deleted between Get and Delete.
// reader must be uncached because an old or relabeled DaemonSet may be hidden
// from the manager cache while it is still running.
func deleteOwnedNodeAgentDaemonSet(ctx context.Context, reader client.Reader, c client.Client, owner client.Object, name string) error {
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: owner.GetNamespace()}}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(ds), ds); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if !metav1.IsControlledBy(ds, owner) {
		return fmt.Errorf("refusing to delete node-agent DaemonSet %s/%s not controlled by %s", ds.Namespace, ds.Name, owner.GetName())
	}
	return client.IgnoreNotFound(c.Delete(ctx, ds, client.Preconditions{
		UID:             &ds.UID,
		ResourceVersion: &ds.ResourceVersion,
	}))
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

// ensureScopedReportRBAC grants a check's agent ConfigMap create access in its
// namespace and get/update only on reports for its current pods. The create
// rule has no resourceNames because Kubernetes RBAC cannot restrict create by
// resourceNames. An empty fleet omits the named read/update rule: an empty
// resourceNames list would otherwise match every ConfigMap.
func ensureScopedReportRBAC(ctx context.Context, c client.Client, scheme *runtime.Scheme, owner client.Object, labels map[string]string, serviceAccount string, reportNames []string) error {
	name := scopedReportAccessName(serviceAccount)
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: owner.GetNamespace()}}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, role, func() error {
		role.Labels = mergeLabels(role.Labels, labels)
		role.Rules = []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"create"},
		}}
		if len(reportNames) > 0 {
			role.Rules = append(role.Rules, rbacv1.PolicyRule{
				APIGroups:     []string{""},
				Resources:     []string{"configmaps"},
				ResourceNames: append([]string(nil), reportNames...),
				Verbs:         []string{"get", "update"},
			})
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
