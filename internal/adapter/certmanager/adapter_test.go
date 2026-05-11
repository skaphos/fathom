/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package certmanager

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/skaphos/fathom/pkg/adapter"
)

func TestAdapterMetadata(t *testing.T) {
	a := New()
	if a.Name() != "cert-manager" {
		t.Fatalf("Name: got %q, want cert-manager", a.Name())
	}
	if a.ContractVersion() != adapter.ContractVersion {
		t.Fatalf("ContractVersion: got %q, want %q", a.ContractVersion(), adapter.ContractVersion)
	}
	caps := a.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "cert-manager" {
		t.Fatalf("AddonTypes: got %#v, want [cert-manager]", caps.AddonTypes)
	}
	if len(caps.Families) != 1 || caps.Families[0] != FamilySystemHealth {
		t.Fatalf("Families: got %#v, want [system_health]", caps.Families)
	}
}

func TestRun_SystemHealthPassesByDefault(t *testing.T) {
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyObjects(false)...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{APIVersion: "fathom.skaphos.io/v1alpha1", Kind: "AddonCheck", Namespace: "default", Name: "cert-manager"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Checks) != 12 {
		t.Fatalf("checks: got %d, want 12", len(result.Checks))
	}
	for _, check := range result.Checks {
		if check.Outcome != adapter.OutcomePass {
			t.Fatalf("check %s/%s outcome: got %s, want Pass: %s", check.TargetRef.Kind, check.TargetRef.Name, check.Outcome, check.Summary)
		}
	}
}

func TestRun_WebhookProbeEnabledChecksService(t *testing.T) {
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyObjects(true)...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilySystemHealth: {Enabled: true, Thresholds: map[string]string{thresholdWebhookProbe: "true"}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Checks) != 13 {
		t.Fatalf("checks: got %d, want 13", len(result.Checks))
	}
	last := result.Checks[len(result.Checks)-1]
	if last.TargetRef.Kind != "Service" || last.TargetRef.Name != "cert-manager-webhook" || last.Outcome != adapter.OutcomePass {
		t.Fatalf("webhook check: got %#v, want pass for webhook Service", last)
	}
}

func TestRun_SystemHealthDisabledSkips(t *testing.T) {
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{APIVersion: "fathom.skaphos.io/v1alpha1", Kind: "AddonCheck", Namespace: "default", Name: "cert-manager"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilySystemHealth: {Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Checks) != 1 {
		t.Fatalf("checks: got %d, want 1", len(result.Checks))
	}
	if result.Checks[0].Outcome != adapter.OutcomeSkipped {
		t.Fatalf("outcome: got %s, want Skipped", result.Checks[0].Outcome)
	}
}

func TestRun_MissingDeploymentFails(t *testing.T) {
	objects := healthyObjects(false)
	objects = objects[1:]
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "cert-manager", adapter.OutcomeFail, "missing")
}

func TestRun_UnreadyPodFails(t *testing.T) {
	objects := healthyObjects(false)
	pod := objects[1].(*corev1.Pod)
	pod.Status.Conditions[0].Status = corev1.ConditionFalse
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", "cert-manager", adapter.OutcomeFail, "not ready")
}

func TestRun_RestartAnomalyWarns(t *testing.T) {
	objects := healthyObjects(false)
	pod := objects[1].(*corev1.Pod)
	pod.Status.ContainerStatuses[0].RestartCount = 4
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", "cert-manager", adapter.OutcomeWarn, "restart count")
}

func TestRun_MissingCRDFails(t *testing.T) {
	objects := healthyObjects(false)
	objects = objects[:len(objects)-1]
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "orders.acme.cert-manager.io", adapter.OutcomeFail, "missing")
}

func assertHasOutcome(t *testing.T, checks []adapter.CheckResult, kind, name string, outcome adapter.Outcome, summaryContains string) {
	t.Helper()
	for _, check := range checks {
		if check.TargetRef.Kind == kind && check.TargetRef.Name == name && check.Outcome == outcome {
			if summaryContains == "" || strings.Contains(check.Summary, summaryContains) {
				return
			}
		}
	}
	t.Fatalf("missing %s/%s outcome %s containing %q in %#v", kind, name, outcome, summaryContains, checks)
}

func newFakeClient(t *testing.T, objects ...clientObject) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apps scheme: %v", err)
	}
	if err := apixv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apiextensions scheme: %v", err)
	}
	clientObjects := make([]runtime.Object, 0, len(objects))
	for _, obj := range objects {
		clientObjects = append(clientObjects, obj)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(clientObjects...).Build()
}

type clientObject interface {
	runtime.Object
	client.Object
}

func healthyObjects(includeWebhookService bool) []clientObject {
	objects := make([]clientObject, 0, len(components)*2+len(crds)+1)
	for _, component := range components {
		objects = append(objects, healthyDeployment(component), readyPod(component))
	}
	for _, crdName := range crds {
		objects = append(objects, establishedCRD(crdName))
	}
	if includeWebhookService {
		objects = append(objects, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "cert-manager-webhook", Namespace: defaultNamespace},
			Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.10"},
		})
	}
	return objects
}

func healthyDeployment(name string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNamespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": name}},
		},
		Status: appsv1.DeploymentStatus{
			AvailableReplicas: 1,
			Conditions: []appsv1.DeploymentCondition{{
				Type:   appsv1.DeploymentAvailable,
				Status: corev1.ConditionTrue,
			}},
		},
	}
}

func readyPod(component string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: component, Namespace: defaultNamespace, Labels: map[string]string{"app.kubernetes.io/name": component}},
		Status: corev1.PodStatus{
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: component, RestartCount: 0}},
		},
	}
}

func establishedCRD(name string) *apixv1.CustomResourceDefinition {
	return &apixv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: apixv1.CustomResourceDefinitionSpec{
			Group: "cert-manager.io",
			Names: apixv1.CustomResourceDefinitionNames{Plural: "tests", Kind: "Test"},
			Versions: []apixv1.CustomResourceDefinitionVersion{{
				Name:    "v1",
				Served:  true,
				Storage: true,
				Schema:  &apixv1.CustomResourceValidation{OpenAPIV3Schema: &apixv1.JSONSchemaProps{Type: "object"}},
			}},
			Scope: apixv1.NamespaceScoped,
		},
		Status: apixv1.CustomResourceDefinitionStatus{Conditions: []apixv1.CustomResourceDefinitionCondition{{
			Type:   apixv1.Established,
			Status: apixv1.ConditionTrue,
		}}},
	}
}
