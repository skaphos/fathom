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
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodecert"
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

// TestNodeCertAuthenticityUnavailableFailsClosed verifies that neither an
// unsupported policy API nor an unsupported binding API permits agents or
// report consumption. Existing verdicts remain frozen; a new check remains
// unknown until the cluster can enforce report-writer identity.
func TestNodeCertAuthenticityUnavailableFailsClosed(t *testing.T) {
	t.Parallel()

	for _, unavailableKind := range []string{"ValidatingAdmissionPolicy", "ValidatingAdmissionPolicyBinding"} {
		unavailableKind := unavailableKind
		for _, previouslyHealthy := range []bool{false, true} {
			previouslyHealthy := previouslyHealthy
			t.Run(unavailableKind+map[bool]string{false: "/new", true: "/previously-healthy"}[previouslyHealthy], func(t *testing.T) {
				t.Parallel()

				now := time.Now().Truncate(time.Second)
				check := &fathomv1alpha1.NodeCertificateCheck{
					ObjectMeta: metav1.ObjectMeta{Name: "nc-auth-unavailable", Namespace: "default", UID: "check-uid", Generation: 2},
				}
				objects := []client.Object{check}
				if previouslyHealthy {
					lastRun := metav1.NewTime(now.Add(-time.Minute))
					check.Status = fathomv1alpha1.NodeCertificateCheckStatus{
						LastRunTime: &lastRun, LastResult: string(fathomv1alpha1.HealthReportResultPass),
						LastReportName: "historical-report", DesiredNodes: 1, ReportingNodes: 1,
						Conditions: []metav1.Condition{
							{Type: nodeCertConditionReady, Status: metav1.ConditionTrue, Reason: "Reporting", ObservedGeneration: 1},
							{Type: nodeCertConditionAgentReady, Status: metav1.ConditionTrue, Reason: "RolledOut", ObservedGeneration: 1},
							{Type: nodeCertConditionCoverage, Status: metav1.ConditionTrue, Reason: "Complete", ObservedGeneration: 1},
							{Type: nodeCertConditionAuthentic, Status: metav1.ConditionTrue, Reason: "AllReportsBound", ObservedGeneration: 1},
						},
					}
					controller := true
					objects = append(objects,
						&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
							Name: agentResourceName(check), Namespace: check.Namespace,
							OwnerReferences: []metav1.OwnerReference{{APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: nodecert.KindNodeCertificateCheck, Name: check.Name, UID: check.UID, Controller: &controller}},
						}},
						&rbacv1.Role{
							ObjectMeta: metav1.ObjectMeta{
								Name: scopedReportAccessName(agentResourceName(check)), Namespace: check.Namespace,
								OwnerReferences: []metav1.OwnerReference{{APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: "NodeCertificateCheck", Name: check.Name, UID: check.UID, Controller: &controller}},
							},
							Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"configmaps"}, ResourceNames: []string{nodecert.NodeReportConfigMapName(check.Name, "node-a")}, Verbs: []string{"get", "update"}}},
						},
					)
				}

				report := nodecert.NodeReport{
					Node: "node-a", CheckName: check.Name, ObservedAt: now,
					Aggregate: nodecert.OutcomeFail,
					Certs:     []nodecert.CertResult{{Path: "/etc/kubernetes/pki/apiserver.crt", Outcome: nodecert.OutcomeFail}},
				}
				encoded, err := nodecert.EncodeReport(report)
				if err != nil {
					t.Fatal(err)
				}
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name: nodecert.NodeReportConfigMapName(check.Name, report.Node), Namespace: check.Namespace,
						Labels: map[string]string{
							nodecert.LabelManagedBy: nodecert.ManagedByValue, nodecert.LabelSourceKind: nodecert.KindNodeCertificateCheck,
							nodecert.LabelSourceName: check.Name, nodecert.LabelNode: report.Node,
						},
						Annotations: map[string]string{nodecert.AnnotationNodeName: report.Node},
					},
					Data: map[string]string{nodecert.ConfigMapReportKey: encoded},
				}

				scheme := newProvisioningScheme(t)
				noMatch := &apiMeta.NoKindMatchError{
					GroupKind:        schema.GroupKind{Group: admissionregistrationv1.GroupName, Kind: unavailableKind},
					SearchedVersions: []string{"v1"},
				}
				objects = append(objects, cm)
				cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).
					WithStatusSubresource(&fathomv1alpha1.NodeCertificateCheck{}).
					WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						policyUnavailable := false
						switch obj.(type) {
						case *admissionregistrationv1.ValidatingAdmissionPolicy:
							policyUnavailable = unavailableKind == "ValidatingAdmissionPolicy"
						case *admissionregistrationv1.ValidatingAdmissionPolicyBinding:
							policyUnavailable = unavailableKind == "ValidatingAdmissionPolicyBinding"
						}
						if policyUnavailable {
							return noMatch
						}
						return c.Get(ctx, key, obj, opts...)
					}}).Build()

				r := &NodeCertificateCheckReconciler{Client: cl, Scheme: scheme, NodeAgentImage: "img"}
				key := client.ObjectKeyFromObject(check)
				if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); !errors.Is(err, errReportAuthenticityUnavailable) {
					t.Fatalf("Reconcile error = %v, want errReportAuthenticityUnavailable", err)
				}

				got := &fathomv1alpha1.NodeCertificateCheck{}
				if err := cl.Get(context.Background(), key, got); err != nil {
					t.Fatal(err)
				}
				for _, conditionType := range []string{nodeCertConditionReady, nodeCertConditionAgentReady, nodeCertConditionCoverage} {
					condition := apiMeta.FindStatusCondition(got.Status.Conditions, conditionType)
					if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "AdmissionPolicyProvisioningFailed" {
						t.Errorf("%s = %+v, want False/AdmissionPolicyProvisioningFailed", conditionType, condition)
					}
				}
				authentic := apiMeta.FindStatusCondition(got.Status.Conditions, nodeCertConditionAuthentic)
				if authentic == nil || authentic.Status != metav1.ConditionUnknown || authentic.Reason != reasonAuthenticityUnavailable {
					t.Errorf("ReportsAuthentic = %+v, want Unknown/%s", authentic, reasonAuthenticityUnavailable)
				}
				if previouslyHealthy {
					if got.Status.LastResult != string(fathomv1alpha1.HealthReportResultPass) || got.Status.LastReportName != "historical-report" || got.Status.LastRunTime == nil || !got.Status.LastRunTime.Equal(&metav1.Time{Time: now.Add(-time.Minute)}) {
						t.Errorf("historical verdict changed: %+v", got.Status)
					}
				} else if got.Status.LastResult != "" || got.Status.LastReportName != "" || got.Status.LastRunTime != nil {
					t.Errorf("new check acquired a verdict without authenticated reports: %+v", got.Status)
				}

				for _, obj := range []client.Object{
					&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: agentResourceName(check), Namespace: check.Namespace}},
					&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: agentResourceName(check), Namespace: check.Namespace}},
				} {
					if err := cl.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); !apierrors.IsNotFound(err) {
						t.Errorf("agent object %T exists or lookup failed: %v", obj, err)
					}
				}
				role := &rbacv1.Role{}
				roleErr := cl.Get(context.Background(), client.ObjectKey{Namespace: check.Namespace, Name: scopedReportAccessName(agentResourceName(check))}, role)
				if previouslyHealthy {
					if roleErr != nil {
						t.Fatalf("get scoped report Role: %v", roleErr)
					}
					if len(role.Rules) != 0 {
						t.Errorf("scoped report Role retained access after authenticity failure: %+v", role.Rules)
					}
				} else if !apierrors.IsNotFound(roleErr) {
					t.Errorf("new check unexpectedly provisioned scoped report Role: %v", roleErr)
				}
				var reports fathomv1alpha1.HealthReportList
				if err := cl.List(context.Background(), &reports, client.InNamespace(check.Namespace)); err != nil {
					t.Fatal(err)
				}
				if len(reports.Items) != 0 || got.Status.ReportingNodes != map[bool]int32{false: 0, true: 1}[previouslyHealthy] {
					t.Errorf("fresh unauthenticated report was consumed: healthReports=%d reportingNodes=%d", len(reports.Items), got.Status.ReportingNodes)
				}
			})
		}
	}
}

func TestNodeCertAuthenticityUnavailablePersistsRevocationFailure(t *testing.T) {
	t.Parallel()
	lastRun := metav1.NewTime(time.Now().Add(-time.Minute).Truncate(time.Second))
	check := &fathomv1alpha1.NodeCertificateCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "nc-auth-revoke-fails", Namespace: "default", UID: "check-uid", Generation: 2},
		Status: fathomv1alpha1.NodeCertificateCheckStatus{
			LastRunTime: &lastRun, LastResult: string(fathomv1alpha1.HealthReportResultPass), LastReportName: "historical-report",
			Conditions: []metav1.Condition{
				{Type: nodeCertConditionReady, Status: metav1.ConditionTrue, Reason: "Reporting", ObservedGeneration: 1},
				{Type: nodeCertConditionAgentReady, Status: metav1.ConditionTrue, Reason: "RolledOut", ObservedGeneration: 1},
				{Type: nodeCertConditionCoverage, Status: metav1.ConditionTrue, Reason: "Complete", ObservedGeneration: 1},
				{Type: nodeCertConditionAuthentic, Status: metav1.ConditionTrue, Reason: "AllReportsBound", ObservedGeneration: 1},
			},
		},
	}
	controller := true
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name: scopedReportAccessName(agentResourceName(check)), Namespace: check.Namespace,
			OwnerReferences: []metav1.OwnerReference{{APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: "NodeCertificateCheck", Name: check.Name, UID: check.UID, Controller: &controller}},
		},
		Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"configmaps"}, ResourceNames: []string{"report"}, Verbs: []string{"get", "update"}}},
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: agentResourceName(check), Namespace: check.Namespace, OwnerReferences: role.OwnerReferences},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: defaultNodeAgentRoleName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: agentResourceName(check), Namespace: check.Namespace}},
	}
	agent := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name: agentResourceName(check), Namespace: check.Namespace, OwnerReferences: role.OwnerReferences,
	}}
	scheme := newProvisioningScheme(t)
	noMatch := &apiMeta.NoKindMatchError{GroupKind: schema.GroupKind{Group: admissionregistrationv1.GroupName, Kind: "ValidatingAdmissionPolicy"}, SearchedVersions: []string{"v1"}}
	deleteDenied := apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, agent.Name, errors.New("delete denied"))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check, role, binding, agent).
		WithStatusSubresource(&fathomv1alpha1.NodeCertificateCheck{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*admissionregistrationv1.ValidatingAdmissionPolicy); ok {
					return noMatch
				}
				return c.Get(ctx, key, obj, opts...)
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*appsv1.DaemonSet); ok {
					return deleteDenied
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).Build()
	r := &NodeCertificateCheckReconciler{Client: cl, Scheme: scheme, NodeAgentImage: "img"}
	key := client.ObjectKeyFromObject(check)
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if !errors.Is(err, errReportAuthenticityUnavailable) || !errors.Is(err, deleteDenied) {
		t.Fatalf("Reconcile error = %v, want joined authenticity and revocation causes", err)
	}
	got := &fathomv1alpha1.NodeCertificateCheck{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{nodeCertConditionReady, nodeCertConditionAgentReady, nodeCertConditionCoverage} {
		condition := apiMeta.FindStatusCondition(got.Status.Conditions, typ)
		if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "AgentRevocationFailed" {
			t.Errorf("%s = %+v, want False/AgentRevocationFailed", typ, condition)
		}
	}
	authentic := apiMeta.FindStatusCondition(got.Status.Conditions, nodeCertConditionAuthentic)
	if authentic == nil || authentic.Status != metav1.ConditionUnknown || authentic.Reason != reasonAuthenticityUnavailable {
		t.Errorf("ReportsAuthentic = %+v, want Unknown/%s", authentic, reasonAuthenticityUnavailable)
	}
	if got.Status.LastResult != string(fathomv1alpha1.HealthReportResultPass) || got.Status.LastReportName != "historical-report" || got.Status.LastRunTime == nil || !got.Status.LastRunTime.Equal(&lastRun) {
		t.Errorf("historical verdict changed: %+v", got.Status)
	}
	cleared := &rbacv1.Role{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(role), cleared); err != nil {
		t.Fatal(err)
	}
	if len(cleared.Rules) != 0 {
		t.Errorf("scoped Role retained rules after delete failure: %+v", cleared.Rules)
	}
	clearedBinding := &rbacv1.RoleBinding{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(binding), clearedBinding); err != nil {
		t.Fatal(err)
	}
	if len(clearedBinding.Subjects) != 0 {
		t.Errorf("legacy shared RoleBinding retained subjects after delete failure: %+v", clearedBinding.Subjects)
	}
}

func TestNodeCertAuthenticityUnavailableStillDeletesAgentWhenRoleClearFails(t *testing.T) {
	t.Parallel()
	check := &fathomv1alpha1.NodeCertificateCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "nc-auth-role-clear-fails", Namespace: "default", UID: "check-uid", Generation: 2},
		Status:     fathomv1alpha1.NodeCertificateCheckStatus{LastResult: string(fathomv1alpha1.HealthReportResultPass)},
	}
	controller := true
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name: scopedReportAccessName(agentResourceName(check)), Namespace: check.Namespace,
			OwnerReferences: []metav1.OwnerReference{{APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: "NodeCertificateCheck", Name: check.Name, UID: check.UID, Controller: &controller}},
		},
		Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"configmaps"}, ResourceNames: []string{"report"}, Verbs: []string{"get", "update"}}},
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: agentResourceName(check), Namespace: check.Namespace, OwnerReferences: role.OwnerReferences},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: defaultNodeAgentRoleName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: agentResourceName(check), Namespace: check.Namespace}},
	}
	agent := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name: agentResourceName(check), Namespace: check.Namespace, OwnerReferences: role.OwnerReferences,
	}}
	scheme := newProvisioningScheme(t)
	noMatch := &apiMeta.NoKindMatchError{GroupKind: schema.GroupKind{Group: admissionregistrationv1.GroupName, Kind: "ValidatingAdmissionPolicy"}, SearchedVersions: []string{"v1"}}
	updateDenied := apierrors.NewForbidden(schema.GroupResource{Group: rbacv1.GroupName, Resource: "roles"}, role.Name, errors.New("update denied"))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check, role, binding, agent).
		WithStatusSubresource(&fathomv1alpha1.NodeCertificateCheck{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*admissionregistrationv1.ValidatingAdmissionPolicy); ok {
					return noMatch
				}
				return c.Get(ctx, key, obj, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*rbacv1.Role); ok {
					return updateDenied
				}
				return c.Update(ctx, obj, opts...)
			},
		}).Build()
	r := &NodeCertificateCheckReconciler{Client: cl, Scheme: scheme, NodeAgentImage: "img"}
	key := client.ObjectKeyFromObject(check)
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if !errors.Is(err, errReportAuthenticityUnavailable) || !errors.Is(err, updateDenied) {
		t.Fatalf("Reconcile error = %v, want joined authenticity and Role-update causes", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(agent), &appsv1.DaemonSet{}); !apierrors.IsNotFound(err) {
		t.Fatalf("DaemonSet deletion was not attempted after Role update failure: %v", err)
	}
	clearedBinding := &rbacv1.RoleBinding{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(binding), clearedBinding); err != nil {
		t.Fatal(err)
	}
	if len(clearedBinding.Subjects) != 0 {
		t.Errorf("legacy shared RoleBinding clear was not attempted after Role update failure: %+v", clearedBinding.Subjects)
	}
	got := &fathomv1alpha1.NodeCertificateCheck{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatal(err)
	}
	ready := apiMeta.FindStatusCondition(got.Status.Conditions, nodeCertConditionReady)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != "AgentRevocationFailed" {
		t.Errorf("Ready = %+v, want False/AgentRevocationFailed", ready)
	}
	authentic := apiMeta.FindStatusCondition(got.Status.Conditions, nodeCertConditionAuthentic)
	if authentic == nil || authentic.Status != metav1.ConditionUnknown || authentic.Reason != reasonAuthenticityUnavailable {
		t.Errorf("ReportsAuthentic = %+v, want Unknown/%s", authentic, reasonAuthenticityUnavailable)
	}
}

func TestNodeCertLegacyAgentRoleBindingMustReferenceExpectedClusterRole(t *testing.T) {
	t.Parallel()
	check := &fathomv1alpha1.NodeCertificateCheck{ObjectMeta: metav1.ObjectMeta{Name: "nc-wrong-role-ref", Namespace: "default", UID: "check-uid", Generation: 1}}
	controller := true
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: agentResourceName(check), Namespace: check.Namespace, CreationTimestamp: metav1.Now(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: nodecert.KindNodeCertificateCheck, Name: check.Name, UID: check.UID, Controller: &controller,
			}},
		},
		RoleRef:  rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "foreign-role"},
		Subjects: []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "victim", Namespace: check.Namespace}},
	}
	foreignDaemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: agentResourceName(check), Namespace: check.Namespace}}
	scheme := newProvisioningScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check, binding, foreignDaemonSet).WithStatusSubresource(&fathomv1alpha1.NodeCertificateCheck{}).Build()
	r := &NodeCertificateCheckReconciler{Client: cl, Scheme: scheme, NodeAgentImage: "img"}
	key := client.ObjectKeyFromObject(check)
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err == nil || !strings.Contains(err.Error(), "immutable roleRef") {
		t.Fatalf("Reconcile error = %v, want immutable roleRef failure", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(binding), binding); err != nil {
		t.Fatal(err)
	}
	if binding.RoleRef.Name != "foreign-role" || len(binding.Subjects) != 1 || binding.Subjects[0].Name != "victim" {
		t.Errorf("foreign RoleBinding was adopted or mutated: %+v", binding)
	}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(foreignDaemonSet), &appsv1.DaemonSet{}); err != nil {
		t.Fatalf("foreign same-name DaemonSet was deleted after RoleBinding validation failure: %v", err)
	}
	got := &fathomv1alpha1.NodeCertificateCheck{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatal(err)
	}
	ready := apiMeta.FindStatusCondition(got.Status.Conditions, nodeCertConditionReady)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != "AgentRevocationFailed" {
		t.Errorf("Ready = %+v, want False/AgentRevocationFailed", ready)
	}
}

func TestNodeCertRBACMigrationFailureRevokesExistingAgent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		deleteFails     bool
		daemonSetExists bool
	}{
		{name: "legacy binding update fails but DaemonSet is deleted"},
		{name: "legacy binding update and DaemonSet deletion fail", deleteFails: true, daemonSetExists: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scheme := newProvisioningScheme(t)
			lastRun := metav1.NewTime(time.Now().Add(-time.Minute).Truncate(time.Second))
			check := &fathomv1alpha1.NodeCertificateCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "nc-rbac-migration", Namespace: "default", UID: "check-uid", Generation: 2},
				Status: fathomv1alpha1.NodeCertificateCheckStatus{
					ObservedGeneration: 1, LastRunTime: &lastRun, LastResult: "Pass", LastReportName: "previous-report",
					Conditions: []metav1.Condition{
						{Type: nodeCertConditionReady, Status: metav1.ConditionTrue, Reason: "Reporting", ObservedGeneration: 1, LastTransitionTime: lastRun},
						{Type: nodeCertConditionAgentReady, Status: metav1.ConditionTrue, Reason: "RolledOut", ObservedGeneration: 1, LastTransitionTime: lastRun},
						{Type: nodeCertConditionCoverage, Status: metav1.ConditionTrue, Reason: "Complete", ObservedGeneration: 1, LastTransitionTime: lastRun},
					},
				},
			}
			agentName := agentResourceName(check)
			owner := metav1.OwnerReference{
				APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: nodecert.KindNodeCertificateCheck,
				Name: check.Name, UID: check.UID, Controller: ptr.To(true),
			}
			ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: check.Namespace, UID: "ds-uid", OwnerReferences: []metav1.OwnerReference{owner}}}
			role := &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{Name: scopedReportAccessName(agentName), Namespace: check.Namespace, OwnerReferences: []metav1.OwnerReference{owner}},
				Rules: []rbacv1.PolicyRule{
					{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"create"}},
					{APIGroups: []string{""}, Resources: []string{"configmaps"}, ResourceNames: []string{"report"}, Verbs: []string{"get", "update"}},
				},
			}
			binding := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: check.Namespace, OwnerReferences: []metav1.OwnerReference{owner}},
				RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: defaultNodeAgentRoleName},
				Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: agentName, Namespace: check.Namespace}},
			}
			bindingErr := errors.New("legacy RoleBinding update denied")
			deleteErr := errors.New("DaemonSet delete denied")
			var bindingUpdates, deleteAttempts int
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check, ds, role, binding).
				WithStatusSubresource(&fathomv1alpha1.NodeCertificateCheck{}).
				WithInterceptorFuncs(interceptor.Funcs{
					Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
						if _, ok := obj.(*rbacv1.RoleBinding); ok {
							bindingUpdates++
							return bindingErr
						}
						return c.Update(ctx, obj, opts...)
					},
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						if _, ok := obj.(*appsv1.DaemonSet); ok {
							deleteAttempts++
							if tc.deleteFails {
								return deleteErr
							}
						}
						return c.Delete(ctx, obj, opts...)
					},
				}).Build()
			r := &NodeCertificateCheckReconciler{Client: cl, Scheme: scheme, NodeAgentImage: "img:test"}
			_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(check)})
			if !errors.Is(err, bindingErr) || !strings.Contains(err.Error(), "revoke node-agent after RBAC provisioning failed") {
				t.Fatalf("Reconcile error = %v, want original binding failure and contextual revocation failure", err)
			}
			if tc.deleteFails && !errors.Is(err, deleteErr) {
				t.Fatalf("Reconcile error = %v, want joined delete failure %v", err, deleteErr)
			}
			if bindingUpdates != 2 || deleteAttempts != 1 {
				t.Fatalf("cleanup attempts = binding updates %d, DaemonSet deletes %d; want 2 and 1", bindingUpdates, deleteAttempts)
			}
			clearedRole := &rbacv1.Role{}
			if err := cl.Get(context.Background(), client.ObjectKeyFromObject(role), clearedRole); err != nil || len(clearedRole.Rules) != 0 {
				t.Fatalf("per-check permissions were not cleared: role=%+v err=%v", clearedRole.Rules, err)
			}
			dsErr := cl.Get(context.Background(), client.ObjectKeyFromObject(ds), &appsv1.DaemonSet{})
			if tc.daemonSetExists && dsErr != nil {
				t.Fatalf("failed delete unexpectedly removed DaemonSet: %v", dsErr)
			}
			if !tc.daemonSetExists && !apierrors.IsNotFound(dsErr) {
				t.Fatalf("successful delete left DaemonSet behind: %v", dsErr)
			}
			persisted := &fathomv1alpha1.NodeCertificateCheck{}
			if err := cl.Get(context.Background(), client.ObjectKeyFromObject(check), persisted); err != nil {
				t.Fatal(err)
			}
			for _, typ := range []string{nodeCertConditionReady, nodeCertConditionAgentReady, nodeCertConditionCoverage} {
				condition := apiMeta.FindStatusCondition(persisted.Status.Conditions, typ)
				if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "AgentRevocationFailed" || condition.ObservedGeneration != check.Generation {
					t.Errorf("%s = %+v, want False/AgentRevocationFailed at generation %d", typ, condition, check.Generation)
				}
			}
			if persisted.Status.LastResult != check.Status.LastResult || persisted.Status.LastReportName != check.Status.LastReportName || persisted.Status.LastRunTime == nil || !persisted.Status.LastRunTime.Equal(check.Status.LastRunTime) {
				t.Errorf("RBAC migration failure changed historical verdict: got %+v, want %+v", persisted.Status, check.Status)
			}
		})
	}
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
	denied := apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, agentResourceName(check), context.DeadlineExceeded)
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(check).
		WithStatusSubresource(&fathomv1alpha1.NodeCertificateCheck{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*corev1.ServiceAccount); ok {
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
