/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
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
					ObjectMeta: metav1.ObjectMeta{Name: "nc-auth-unavailable", Namespace: "default", Generation: 2},
				}
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
				cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check, cm).
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
