/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package addondefinition

import (
	"bytes"
	"fmt"

	api "github.com/skaphos/fathom/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"
)

// RenderOptions identifies the dedicated reader and the actual operator identity.
// OperatorServiceAccount must be supplied; deployment naming is not inferred.
type RenderOptions struct {
	ServiceAccount         string
	OperatorNamespace      string
	OperatorServiceAccount string
	BuiltinNames           []string
}

// Render produces staged YAML without clients, cluster reads, or writes. Its last
// document is deliberately inadmissible until bind resolves both live UIDs.
func Render(d *api.AddonDefinition, opts RenderOptions) ([]byte, []string, error) {
	plan, err := PlanGrants(d)
	if err != nil {
		return nil, nil, err
	}
	if len(validation.IsDNS1123Label(opts.OperatorNamespace)) != 0 || opts.OperatorNamespace == "" {
		return nil, nil, fmt.Errorf("operator namespace must be an explicit DNS label")
	}
	for _, name := range []string{opts.ServiceAccount, opts.OperatorServiceAccount} {
		if name == "" || len(validation.IsDNS1123Subdomain(name)) != 0 {
			return nil, nil, fmt.Errorf("service account names must be explicit DNS subdomains")
		}
	}
	if opts.ServiceAccount == opts.OperatorServiceAccount {
		return nil, nil, fmt.Errorf("reader service account must differ from operator service account")
	}
	if opts.BuiltinNames == nil {
		return nil, nil, fmt.Errorf("built-in inventory is required for collision checking")
	}
	for _, name := range opts.BuiltinNames {
		if name == d.Name {
			return nil, nil, fmt.Errorf("definition %q collides with a built-in", name)
		}
	}
	definition := &api.AddonDefinition{TypeMeta: metav1.TypeMeta{APIVersion: api.GroupVersion.String(), Kind: "AddonDefinition"}, ObjectMeta: metav1.ObjectMeta{Name: d.Name}, Spec: d.DeepCopy().Spec}
	account := &corev1.ServiceAccount{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceAccount"}, ObjectMeta: metav1.ObjectMeta{Name: opts.ServiceAccount, Namespace: opts.OperatorNamespace}, AutomountServiceAccountToken: new(bool)}
	objects := []any{definition, account}
	subject := []rbacv1.Subject{{Kind: "ServiceAccount", Name: opts.ServiceAccount, Namespace: opts.OperatorNamespace}}
	for i, grant := range plan.Grants {
		name := fmt.Sprintf("fathom-definition-%s-%d", d.Name, i)
		meta := metav1.ObjectMeta{Name: name, Namespace: grant.Namespace}
		ref := rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Name: name}
		if grant.Namespace == "" {
			ref.Kind = "ClusterRole"
			objects = append(objects, &rbacv1.ClusterRole{TypeMeta: metav1.TypeMeta{APIVersion: rbacv1.SchemeGroupVersion.String(), Kind: "ClusterRole"}, ObjectMeta: meta, Rules: []rbacv1.PolicyRule{grant.Rule}}, &rbacv1.ClusterRoleBinding{TypeMeta: metav1.TypeMeta{APIVersion: rbacv1.SchemeGroupVersion.String(), Kind: "ClusterRoleBinding"}, ObjectMeta: meta, RoleRef: ref, Subjects: subject})
		} else {
			ref.Kind = "Role"
			objects = append(objects, &rbacv1.Role{TypeMeta: metav1.TypeMeta{APIVersion: rbacv1.SchemeGroupVersion.String(), Kind: "Role"}, ObjectMeta: meta, Rules: []rbacv1.PolicyRule{grant.Rule}}, &rbacv1.RoleBinding{TypeMeta: metav1.TypeMeta{APIVersion: rbacv1.SchemeGroupVersion.String(), Kind: "RoleBinding"}, ObjectMeta: meta, RoleRef: ref, Subjects: subject})
		}
	}
	// ServiceAccount impersonation is namespace-scoped; do not grant users/groups.
	name := "fathom-definition-" + d.Name + "-impersonate"
	meta := metav1.ObjectMeta{Name: name, Namespace: opts.OperatorNamespace}
	objects = append(objects, &rbacv1.Role{TypeMeta: metav1.TypeMeta{APIVersion: rbacv1.SchemeGroupVersion.String(), Kind: "Role"}, ObjectMeta: meta, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"serviceaccounts"}, Verbs: []string{"impersonate"}, ResourceNames: []string{opts.ServiceAccount}}}}, &rbacv1.RoleBinding{TypeMeta: metav1.TypeMeta{APIVersion: rbacv1.SchemeGroupVersion.String(), Kind: "RoleBinding"}, ObjectMeta: meta, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: opts.OperatorServiceAccount, Namespace: opts.OperatorNamespace}}})
	// Keep the template structurally separate: no fake UID can be mistaken for a
	// captured authorization reference, and it cannot accidentally be enabled.
	objects = append(objects, map[string]any{"apiVersion": api.GroupVersion.String(), "kind": "AddonDefinitionBinding", "metadata": map[string]any{"name": d.Name, "namespace": opts.OperatorNamespace}, "spec": map[string]any{"enabled": false, "definitionRef": map[string]any{"name": d.Name, "uid": ""}, "serviceAccountRef": map[string]any{"name": opts.ServiceAccount, "uid": ""}, "targetScope": map[string]any{"allowClusterScoped": false}}})
	var out bytes.Buffer
	out.WriteString("# Staged output: review grants, apply prerequisites, then use definition bind.\n# The final binding template is incomplete and must not be applied.\n")
	for _, diagnostic := range plan.Diagnostics {
		fmt.Fprintf(&out, "# MANUAL COMPLETION: %s\n", diagnostic)
	}
	for _, object := range objects {
		data, marshalErr := yaml.Marshal(object)
		if marshalErr != nil {
			return nil, nil, marshalErr
		}
		out.WriteString("---\n")
		out.Write(data)
	}
	return out.Bytes(), plan.Diagnostics, nil
}
