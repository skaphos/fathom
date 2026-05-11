/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package coredns

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/skaphos/fathom/pkg/adapter"
)

func TestAdapterMetadata(t *testing.T) {
	a := New()
	if a.Name() != "coredns" {
		t.Fatalf("Name: got %q, want coredns", a.Name())
	}
	if a.ContractVersion() != adapter.ContractVersion {
		t.Fatalf("ContractVersion: got %q, want %q", a.ContractVersion(), adapter.ContractVersion)
	}
	caps := a.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "coredns" {
		t.Fatalf("AddonTypes: got %#v, want [coredns]", caps.AddonTypes)
	}
	if len(caps.Families) != 2 {
		t.Fatalf("Families: got %#v, want 2 families", caps.Families)
	}
}

func TestRun_SystemHealthAndDNSResolutionPass(t *testing.T) {
	a := Adapter{lookupHost: func(_ context.Context, name string) ([]string, error) {
		if name != defaultDNSTargets {
			t.Fatalf("lookup target: got %q, want %q", name, defaultDNSTargets)
		}
		return []string{"10.96.0.1"}, nil
	}}
	result, err := a.Run(context.Background(), adapter.Request{Client: newFakeClient(t, healthyObjects()...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "coredns", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "Pod", "coredns", adapter.OutcomePass, "ready")
	assertHasOutcome(t, result.Checks, "Service", "kube-dns", adapter.OutcomePass, "routable")
	assertHasOutcome(t, result.Checks, "EndpointSlice", "kube-dns", adapter.OutcomePass, "ready endpoints")
	assertHasOutcome(t, result.Checks, "DNSName", defaultDNSTargets, adapter.OutcomePass, "succeeded")
}

func TestRun_DNSResolutionFailureIncludesError(t *testing.T) {
	a := Adapter{lookupHost: func(context.Context, string) ([]string, error) { return nil, errors.New("no such host") }}
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilyDNSResolution: {Enabled: true, Thresholds: map[string]string{thresholdTargets: "svc-a,svc-b"}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DNSName", "svc-a", adapter.OutcomeError, "failed")
	assertHasOutcome(t, result.Checks, "DNSName", "svc-b", adapter.OutcomeError, "failed")
	assertHasDetail(t, result.Checks, "DNSName", "svc-a", "error", "no such host")
}

func TestRun_MissingDeploymentFails(t *testing.T) {
	objects := healthyObjects()
	objects = objects[1:]
	a := Adapter{lookupHost: func(context.Context, string) ([]string, error) { return []string{"10.96.0.1"}, nil }}
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "coredns", adapter.OutcomeFail, "missing")
}

func TestRun_UnreadyPodFails(t *testing.T) {
	objects := healthyObjects()
	pod := objects[1].(*corev1.Pod)
	pod.Status.Conditions[0].Status = corev1.ConditionFalse
	a := Adapter{lookupHost: func(context.Context, string) ([]string, error) { return []string{"10.96.0.1"}, nil }}
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", "coredns", adapter.OutcomeFail, "not ready")
}

func TestRun_NoReadyEndpointSlicesFails(t *testing.T) {
	objects := healthyObjects()
	objects = objects[:len(objects)-1]
	a := Adapter{lookupHost: func(context.Context, string) ([]string, error) { return []string{"10.96.0.1"}, nil }}
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "EndpointSlice", "kube-dns", adapter.OutcomeFail, "no ready endpoints")
}

func TestRun_SystemHealthSupportsDistributionNamesAndAutoscaler(t *testing.T) {
	a := Adapter{lookupHost: func(context.Context, string) ([]string, error) { return []string{"10.43.0.1"}, nil }}
	result, err := a.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t,
			healthyDeploymentNamed("rke2-coredns", map[string]string{"k8s-app": "kube-dns"}),
			readyPodNamed("rke2-coredns", map[string]string{"k8s-app": "kube-dns"}),
			healthyDeploymentNamed("rke2-coredns-autoscaler", map[string]string{"k8s-app": "kube-dns-autoscaler"}),
			readyPodNamed("rke2-coredns-autoscaler", map[string]string{"k8s-app": "kube-dns-autoscaler"}),
			dnsServiceNamed("rke2-coredns-rke2-coredns"),
			dnsEndpointSliceNamed("rke2-coredns-rke2-coredns"),
		),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true, Thresholds: map[string]string{
			thresholdDeploymentName: "rke2-coredns",
			thresholdAutoscalerName: "rke2-coredns-autoscaler",
			thresholdServiceName:    "rke2-coredns-rke2-coredns",
		}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "rke2-coredns", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "Deployment", "rke2-coredns-autoscaler", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "Service", "rke2-coredns-rke2-coredns", adapter.OutcomePass, "routable")
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

func assertHasDetail(t *testing.T, checks []adapter.CheckResult, kind, name, key, want string) {
	t.Helper()
	for _, check := range checks {
		if check.TargetRef.Kind == kind && check.TargetRef.Name == name {
			if got := check.Details[key]; got != want {
				t.Fatalf("detail %s for %s/%s: got %q, want %q", key, kind, name, got, want)
			}
			return
		}
	}
	t.Fatalf("missing %s/%s in %#v", kind, name, checks)
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
	if err := discoveryv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add discovery scheme: %v", err)
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

func healthyObjects() []clientObject {
	return []clientObject{healthyDeployment(), readyPod(), dnsService(), dnsEndpointSlice()}
}

func healthyDeployment() *appsv1.Deployment {
	return healthyDeploymentNamed("coredns", map[string]string{"k8s-app": "kube-dns"})
}

func healthyDeploymentNamed(name string, labels map[string]string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNamespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 1, Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}},
	}
}

func readyPod() *corev1.Pod {
	return readyPodNamed("coredns", map[string]string{"k8s-app": "kube-dns"})
}

func readyPodNamed(name string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNamespace, Labels: labels},
		Status: corev1.PodStatus{
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: name, RestartCount: 0}},
		},
	}
}

func dnsService() *corev1.Service {
	return dnsServiceNamed(defaultDNSServiceName)
}

func dnsServiceNamed(name string) *corev1.Service {
	return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNamespace}, Spec: corev1.ServiceSpec{ClusterIP: "10.96.0.10"}}
}

func dnsEndpointSlice() *discoveryv1.EndpointSlice {
	return dnsEndpointSliceNamed(defaultDNSServiceName)
}

func dnsEndpointSliceNamed(serviceName string) *discoveryv1.EndpointSlice {
	ready := true
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Name: serviceName + "-abc", Namespace: defaultNamespace, Labels: map[string]string{"kubernetes.io/service-name": serviceName}},
		Endpoints:  []discoveryv1.Endpoint{{Conditions: discoveryv1.EndpointConditions{Ready: &ready}}},
	}
}
