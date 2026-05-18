/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package externalsecrets

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/skaphos/fathom/pkg/adapter"
)

func TestAdapterMetadata(t *testing.T) {
	a := New()
	if a.Name() != "external-secrets" {
		t.Fatalf("Name: got %q, want external-secrets", a.Name())
	}
	if a.ContractVersion() != adapter.ContractVersion {
		t.Fatalf("ContractVersion: got %q, want %q", a.ContractVersion(), adapter.ContractVersion)
	}
	caps := a.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "external-secrets" {
		t.Fatalf("AddonTypes: got %#v, want [external-secrets]", caps.AddonTypes)
	}
	if len(caps.Families) != 2 {
		t.Fatalf("Families: got %#v, want 2 families", caps.Families)
	}
}

func TestRun_SystemHealthAndSecretSyncPass(t *testing.T) {
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, append(healthySystemObjects(), readyExternalSecret("app-secret", time.Now().Add(-5*time.Minute)))...),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "external-secrets", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "Pod", "external-secrets", adapter.OutcomePass, "ready")
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "externalsecrets.external-secrets.io", adapter.OutcomePass, "established")
	assertHasOutcome(t, result.Checks, "ExternalSecret", "app-secret", adapter.OutcomePass, "ready")
}

func TestRun_MissingDeploymentFails(t *testing.T) {
	objects := healthySystemObjects()
	objects = objects[1:]
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "external-secrets", adapter.OutcomeFail, "missing")
}

func TestRun_UnreadyPodFails(t *testing.T) {
	objects := healthySystemObjects()
	pod := objects[1].(*corev1.Pod)
	pod.Status.Conditions[0].Status = corev1.ConditionFalse
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", "external-secrets", adapter.OutcomeFail, "not ready")
}

func TestRun_ExternalSecretFailureAndStaleness(t *testing.T) {
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t,
			notReadyExternalSecret("broken-secret"),
			readyExternalSecret("stale-secret", time.Now().Add(-2*time.Hour)),
		),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilySecretSync: {Enabled: true, Thresholds: map[string]string{thresholdStaleMinutes: "30"}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "ExternalSecret", "broken-secret", adapter.OutcomeFail, "not ready")
	assertHasOutcome(t, result.Checks, "ExternalSecret", "stale-secret", adapter.OutcomeWarn, "stale")
	assertHasDetail(t, result.Checks, "ExternalSecret", "broken-secret", "reason", "SecretSyncedError")
	assertHasDetail(t, result.Checks, "ExternalSecret", "stale-secret", "targetName", "stale-secret")
}

func TestRun_FamiliesCanBeDisabled(t *testing.T) {
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{APIVersion: "fathom.skaphos.io/v1alpha1", Kind: "AddonCheck", Namespace: "default", Name: "eso"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilySystemHealth: {Enabled: false},
			FamilySecretSync:   {Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Checks) != 1 || result.Checks[0].Outcome != adapter.OutcomeSkipped {
		t.Fatalf("checks: got %#v, want one skipped result", result.Checks)
	}
}

// TestRun_FallsBackToV1Beta1 covers ESO ≤ 0.10 clusters where the
// CRDs only serve v1beta1. The adapter must Pass the CRD check (v1beta1
// is still in supportedAPIVersions) and target ExternalSecret listings
// at v1beta1 too.
func TestRun_FallsBackToV1Beta1(t *testing.T) {
	objects := readyDeploymentsAndPods()
	for _, name := range crds {
		objects = append(objects, establishedCRDWithVersions(name, "v1beta1"))
	}
	objects = append(objects, externalSecretWithVersion("legacy-secret", "v1beta1", map[string]any{
		"conditions":  []any{map[string]any{"type": "Ready", "status": "True"}},
		"refreshTime": time.Now().UTC().Format(time.RFC3339),
	}))

	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "externalsecrets.external-secrets.io", adapter.OutcomePass, "established")
	assertHasDetail(t, result.Checks, "CustomResourceDefinition", "externalsecrets.external-secrets.io", "version", "v1beta1")
	assertHasOutcome(t, result.Checks, "ExternalSecret", "legacy-secret", adapter.OutcomePass, "ready")
}

// TestRun_WarnsWhenNoSupportedVersion covers the case where a CRD is
// installed but only serves a version Fathom doesn't recognize (e.g. a
// fresh ESO version that has dropped both v1 and v1beta1). The check
// surfaces a Warn so the operator notices the regression without going
// hard-fail on a still-running install.
func TestRun_WarnsWhenNoSupportedVersion(t *testing.T) {
	objects := readyDeploymentsAndPods()
	objects = append(objects, establishedCRDWithVersions(externalSecretCRD, "v1alpha1"))
	for _, name := range crds[1:] {
		objects = append(objects, establishedCRDWithVersions(name, "v1"))
	}

	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilySystemHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", externalSecretCRD, adapter.OutcomeWarn, "no supported version")
	assertHasDetail(t, result.Checks, "CustomResourceDefinition", externalSecretCRD, "expectedVersions", strings.Join(supportedAPIVersions, ","))
}

// externalSecretWithVersion builds an ExternalSecret unstructured at a
// caller-chosen apiVersion. The default `externalSecret` helper emits v1.
func externalSecretWithVersion(name, version string, status map[string]any) *unstructured.Unstructured {
	es := externalSecret(name, status)
	es.SetAPIVersion("external-secrets.io/" + version)
	return es
}

// readyDeploymentsAndPods returns just the workload objects (no CRDs),
// so individual tests can attach their own CRD fixtures with custom
// served-version sets.
func readyDeploymentsAndPods() []clientObject {
	objects := []clientObject{}
	for _, c := range components {
		objects = append(objects, healthyDeployment(c), readyPod(c))
	}
	return objects
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

func healthySystemObjects() []clientObject {
	objects := make([]clientObject, 0, len(components)*2+len(crds))
	for _, component := range components {
		objects = append(objects, healthyDeployment(component), readyPod(component))
	}
	for _, crdName := range crds {
		objects = append(objects, establishedCRD(crdName))
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
			Conditions:        []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}},
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
	return establishedCRDWithVersions(name, "v1")
}

// establishedCRDWithVersions builds a CRD whose Spec.Versions reflect
// the named served versions (the first is also marked Storage). Used to
// exercise the adapter's preferredServedVersion priority logic.
func establishedCRDWithVersions(name string, served ...string) *apixv1.CustomResourceDefinition {
	versions := make([]apixv1.CustomResourceDefinitionVersion, 0, len(served))
	for i, v := range served {
		versions = append(versions, apixv1.CustomResourceDefinitionVersion{
			Name:    v,
			Served:  true,
			Storage: i == 0,
			Schema:  &apixv1.CustomResourceValidation{OpenAPIV3Schema: &apixv1.JSONSchemaProps{Type: "object"}},
		})
	}
	return &apixv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: apixv1.CustomResourceDefinitionSpec{
			Group:    "external-secrets.io",
			Names:    apixv1.CustomResourceDefinitionNames{Plural: "tests", Kind: "Test"},
			Versions: versions,
			Scope:    apixv1.NamespaceScoped,
		},
		Status: apixv1.CustomResourceDefinitionStatus{Conditions: []apixv1.CustomResourceDefinitionCondition{{Type: apixv1.Established, Status: apixv1.ConditionTrue}}},
	}
}

func readyExternalSecret(name string, refreshTime time.Time) *unstructured.Unstructured {
	return externalSecret(name, map[string]any{
		"conditions":            []any{map[string]any{"type": "Ready", "status": "True", "reason": "SecretSynced", "message": "secret synced"}},
		"refreshTime":           refreshTime.UTC().Format(time.RFC3339),
		"syncedResourceVersion": "123",
		"binding":               map[string]any{"name": name, "resourceVersion": "456"},
	})
}

func notReadyExternalSecret(name string) *unstructured.Unstructured {
	return externalSecret(name, map[string]any{
		"conditions": []any{map[string]any{"type": "Ready", "status": "False", "reason": "SecretSyncedError", "message": "provider error"}},
	})
}

// TestInt32Threshold_RejectsOverflowingValue locks the post-CodeQL contract
// that int32Threshold falls back to the default when the policy threshold
// would overflow int32. Before the fix, intThreshold returned a host-sized
// int and int32() silently truncated the high bits.
func TestInt32Threshold_RejectsOverflowingValue(t *testing.T) {
	tests := []struct {
		name      string
		threshold string
		want      int32
	}{
		{"in-range value preserved", "42", 42},
		{"int32 boundary preserved", "2147483647", 2147483647},
		{"overflow past int32 falls back", "2147483648", 7}, // 2^31, just over MaxInt32
		{"far overflow falls back", "9999999999", 7},        // 10x int32
		{"negative falls back", "-1", 7},
		{"non-numeric falls back", "not-a-number", 7},
		{"empty threshold falls back", "", 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			policy := adapter.FamilyPolicy{
				Thresholds: map[string]string{"restartWarnCount": tc.threshold},
			}
			got := int32Threshold(policy, "restartWarnCount", 7)
			if got != tc.want {
				t.Errorf("int32Threshold(%q) = %d, want %d", tc.threshold, got, tc.want)
			}
		})
	}
}

func TestInt32Threshold_NilThresholdsReturnsDefault(t *testing.T) {
	got := int32Threshold(adapter.FamilyPolicy{}, "restartWarnCount", 7)
	if got != 7 {
		t.Errorf("int32Threshold on nil Thresholds = %d, want 7", got)
	}
}

func TestInt32Threshold_MissingKeyReturnsDefault(t *testing.T) {
	policy := adapter.FamilyPolicy{Thresholds: map[string]string{"other": "1"}}
	got := int32Threshold(policy, "restartWarnCount", 7)
	if got != 7 {
		t.Errorf("int32Threshold on missing key = %d, want 7", got)
	}
}

func externalSecret(name string, status map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "external-secrets.io/v1",
		"kind":       "ExternalSecret",
		"metadata":   map[string]any{"name": name, "namespace": defaultNamespace},
		"spec": map[string]any{
			"secretStoreRef": map[string]any{"name": "store", "kind": "SecretStore"},
			"target":         map[string]any{"name": name},
		},
		"status": status,
	}}
}
