/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package cilium

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
	if a.Name() != "cilium" {
		t.Fatalf("Name: got %q, want cilium", a.Name())
	}
	if a.Version() != Version {
		t.Fatalf("Version: got %q, want %q", a.Version(), Version)
	}
	if a.ContractVersion() != adapter.ContractVersion {
		t.Fatalf("ContractVersion: got %q, want %q", a.ContractVersion(), adapter.ContractVersion)
	}
	caps := a.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "cilium" {
		t.Fatalf("AddonTypes: got %#v, want [cilium]", caps.AddonTypes)
	}
	if len(caps.Families) != 3 {
		t.Fatalf("Families: got %#v, want 3 families", caps.Families)
	}
}

func TestRun_AllFamiliesPassByDefault(t *testing.T) {
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyObjects()...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{APIVersion: "fathom.skaphos.io/v1alpha1", Kind: "AddonCheck", Namespace: "default", Name: "cilium"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// 2 control-plane (deployment + 1 pod) + 3 agent (daemonset + 2 pods) + 5 CRDs.
	if len(result.Checks) != 10 {
		t.Fatalf("checks: got %d, want 10: %#v", len(result.Checks), result.Checks)
	}
	for _, check := range result.Checks {
		if check.Outcome != adapter.OutcomePass {
			t.Fatalf("check %s/%s outcome: got %s, want Pass: %s", check.TargetRef.Kind, check.TargetRef.Name, check.Outcome, check.Summary)
		}
	}
	assertHasOutcome(t, result.Checks, "Deployment", defaultOperatorName, adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultAgentName, adapter.OutcomePass, "fully ready")
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "ciliumnodes.cilium.io", adapter.OutcomePass, "established")
}

func TestRun_AllFamiliesDisabledSkips(t *testing.T) {
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyObjects()...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{APIVersion: "fathom.skaphos.io/v1alpha1", Kind: "AddonCheck", Namespace: "default", Name: "cilium"},
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilyControlPlaneHealth: {Enabled: false},
			FamilyAgentHealth:        {Enabled: false},
			FamilyCRDHealth:          {Enabled: false},
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
	if result.Checks[0].Family != FamilyControlPlaneHealth {
		t.Fatalf("family: got %s, want %s", result.Checks[0].Family, FamilyControlPlaneHealth)
	}
}

// TestRun_NotInstalledRollsUpGreen is the core "Cilium absent => Skipped"
// contract: with every family enabled by default and an empty cluster, all
// emissions are Skipped and each family rolls up to Pass (FamilyOutcome treats
// Skipped as Pass), so a cilium AddonCheck on a non-Cilium cluster stays quiet
// instead of failing.
func TestRun_NotInstalledRollsUpGreen(t *testing.T) {
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// control-plane deployment (no pods) + agent daemonset (no pods) + 5 CRDs.
	if len(result.Checks) != 7 {
		t.Fatalf("checks: got %d, want 7: %#v", len(result.Checks), result.Checks)
	}
	for _, check := range result.Checks {
		if check.Outcome != adapter.OutcomeSkipped {
			t.Fatalf("check %s/%s outcome: got %s, want Skipped: %s", check.TargetRef.Kind, check.TargetRef.Name, check.Outcome, check.Summary)
		}
	}
	for _, family := range []adapter.Family{FamilyControlPlaneHealth, FamilyAgentHealth, FamilyCRDHealth} {
		if got := adapter.FamilyOutcome(result.Checks, family); got != adapter.OutcomePass {
			t.Fatalf("FamilyOutcome(%s): got %s, want Pass", family, got)
		}
	}
}

func TestRun_MissingOperatorDeploymentSkipped(t *testing.T) {
	objects := healthyObjects()
	objects = objects[1:] // drop the operator Deployment
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", defaultOperatorName, adapter.OutcomeSkipped, "may not be installed")
}

func TestRun_OperatorNotAvailableFails(t *testing.T) {
	objects := healthyObjects()
	deployment := objects[0].(*appsv1.Deployment)
	deployment.Status.AvailableReplicas = 0
	deployment.Status.Conditions = nil
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", defaultOperatorName, adapter.OutcomeFail, "not fully available")
}

func TestRun_OperatorScaledToZeroWarnsAndSkipsPods(t *testing.T) {
	objects := healthyObjects()
	deployment := objects[0].(*appsv1.Deployment)
	zero := int32(0)
	deployment.Spec.Replicas = &zero
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", defaultOperatorName, adapter.OutcomeWarn, "scaled to zero")
	// A scaled-to-zero deployment must not also produce a "no matching pods" Fail.
	assertNoOutcome(t, result.Checks, "Pod", defaultOperatorName, adapter.OutcomeFail)
}

func TestRun_OperatorPodNotReadyFails(t *testing.T) {
	objects := healthyObjects()
	pod := objects[1].(*corev1.Pod)
	pod.Status.Conditions[0].Status = corev1.ConditionFalse
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", pod.Name, adapter.OutcomeFail, "not ready")
}

func TestRun_OperatorPodRestartWarns(t *testing.T) {
	objects := healthyObjects()
	pod := objects[1].(*corev1.Pod)
	pod.Status.ContainerStatuses[0].RestartCount = 4
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", pod.Name, adapter.OutcomeWarn, "restart count")
}

func TestRun_MissingAgentDaemonSetSkipped(t *testing.T) {
	objects := healthyObjects()
	objects = append(objects[:2], objects[5:]...) // drop the agent DaemonSet and its two pods
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultAgentName, adapter.OutcomeSkipped, "may not be installed")
}

func TestRun_AgentDaemonSetNotReadyFails(t *testing.T) {
	objects := healthyObjects()
	daemonset := objects[2].(*appsv1.DaemonSet)
	daemonset.Status.NumberReady = 1
	daemonset.Status.NumberUnavailable = 1
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultAgentName, adapter.OutcomeFail, "not fully ready")
	assertHasDetail(t, result.Checks, "DaemonSet", defaultAgentName, "numberUnavailable", "1")
}

func TestRun_AgentDaemonSetZeroScheduledWarnsAndSkipsPods(t *testing.T) {
	objects := healthyObjects()
	daemonset := objects[2].(*appsv1.DaemonSet)
	daemonset.Status.DesiredNumberScheduled = 0
	daemonset.Status.NumberReady = 0
	daemonset.Status.NumberAvailable = 0
	daemonset.Status.UpdatedNumberScheduled = 0
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultAgentName, adapter.OutcomeWarn, "schedules zero pods")
	assertNoOutcome(t, result.Checks, "Pod", defaultAgentName, adapter.OutcomeFail)
}

func TestRun_AgentDaemonSetRolloutWarns(t *testing.T) {
	objects := healthyObjects()
	daemonset := objects[2].(*appsv1.DaemonSet)
	daemonset.Status.UpdatedNumberScheduled = 1 // mid-rollout: desired=2, ready=2, updated=1
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultAgentName, adapter.OutcomeWarn, "rollout is in progress")
}

func TestRun_AgentPodNotReadyFails(t *testing.T) {
	objects := healthyObjects()
	pod := objects[3].(*corev1.Pod)
	pod.Status.Conditions[0].Status = corev1.ConditionFalse
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", pod.Name, adapter.OutcomeFail, "not ready")
}

func TestRun_CRDNotEstablishedFails(t *testing.T) {
	objects := healthyObjects()
	// Replace the last CRD (ciliumnodes) with a not-established variant.
	objects[len(objects)-1] = ciliumCRD("ciliumnodes.cilium.io", "v2", true, false)
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "ciliumnodes.cilium.io", adapter.OutcomeFail, "not established")
}

func TestRun_CRDUnsupportedVersionWarns(t *testing.T) {
	objects := healthyObjects()
	// Established, but only serves an unrecognized version.
	objects[len(objects)-1] = ciliumCRD("ciliumnodes.cilium.io", "v1", true, true)
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "ciliumnodes.cilium.io", adapter.OutcomeWarn, "no version Fathom recognizes")
}

func TestRun_SupportsCustomNamesAndNamespace(t *testing.T) {
	const ns = "cilium-system"
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t,
			deploymentInNamespace("rke2-cilium-operator", ns),
			podInNamespace("rke2-cilium-operator-abc", "rke2-cilium-operator", ns),
			daemonSetInNamespace("rke2-cilium", ns, 1),
			podInNamespace("rke2-cilium-node1", "rke2-cilium", ns),
		),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilyControlPlaneHealth: {Enabled: true, Namespaces: []string{ns}, Thresholds: map[string]string{thresholdOperatorName: "rke2-cilium-operator"}},
			FamilyAgentHealth:        {Enabled: true, Namespaces: []string{ns}, Thresholds: map[string]string{thresholdAgentName: "rke2-cilium"}},
			FamilyCRDHealth:          {Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "rke2-cilium-operator", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "DaemonSet", "rke2-cilium", adapter.OutcomePass, "fully ready")
}

// TestRun_FamilyAttribution locks the contract that every CheckResult is tagged
// with the policy family that gated its execution (SKA-428).
func TestRun_FamilyAttribution(t *testing.T) {
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyObjects()...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilyControlPlaneHealth: {Enabled: true},
			FamilyAgentHealth:        {Enabled: true},
			FamilyCRDHealth:          {Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertFamily(t, result.Checks, "Deployment", defaultOperatorName, FamilyControlPlaneHealth)
	assertFamily(t, result.Checks, "DaemonSet", defaultAgentName, FamilyAgentHealth)
	assertFamily(t, result.Checks, "CustomResourceDefinition", "ciliumnodes.cilium.io", FamilyCRDHealth)
}

// --- helpers ---

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

func assertNoOutcome(t *testing.T, checks []adapter.CheckResult, kind, name string, outcome adapter.Outcome) {
	t.Helper()
	for _, check := range checks {
		if check.TargetRef.Kind == kind && check.TargetRef.Name == name && check.Outcome == outcome {
			t.Fatalf("unexpected %s/%s outcome %s: %#v", kind, name, outcome, check)
		}
	}
}

func assertFamily(t *testing.T, checks []adapter.CheckResult, kind, name string, want adapter.Family) {
	t.Helper()
	matched := 0
	for _, check := range checks {
		if check.TargetRef.Kind != kind || check.TargetRef.Name != name {
			continue
		}
		matched++
		if check.Family != want {
			t.Fatalf("family for %s/%s: got %q, want %q (outcome=%s summary=%q)", kind, name, check.Family, want, check.Outcome, check.Summary)
		}
	}
	if matched == 0 {
		t.Fatalf("no checks matched %s/%s in %#v", kind, name, checks)
	}
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

const testLabelKey = "app.kubernetes.io/name"

// healthyObjects returns a fully-healthy Cilium install. Index order is relied
// on by mutation tests: [0]=operator Deployment, [1]=operator Pod,
// [2]=agent DaemonSet, [3..4]=agent Pods, [5..]=CRDs.
func healthyObjects() []clientObject {
	objects := []clientObject{
		readyDeployment(defaultOperatorName),
		readyPod(defaultOperatorName+"-7d9c", defaultOperatorName),
		readyDaemonSet(defaultAgentName, 2),
		readyPod(defaultAgentName+"-node1", defaultAgentName),
		readyPod(defaultAgentName+"-node2", defaultAgentName),
	}
	for _, name := range ciliumCRDs {
		objects = append(objects, establishedCiliumCRD(name))
	}
	return objects
}

func readyDeployment(name string) *appsv1.Deployment {
	return deploymentInNamespace(name, defaultNamespace)
}

func deploymentInNamespace(name, namespace string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKey: name}},
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

func readyDaemonSet(name string, desired int32) *appsv1.DaemonSet {
	return daemonSetInNamespace(name, defaultNamespace, desired)
}

func daemonSetInNamespace(name, namespace string, desired int32) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKey: name}},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: desired,
			NumberReady:            desired,
			NumberAvailable:        desired,
			NumberUnavailable:      0,
			UpdatedNumberScheduled: desired,
		},
	}
}

func readyPod(name, component string) *corev1.Pod {
	return podInNamespace(name, component, defaultNamespace)
}

func podInNamespace(name, component, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{testLabelKey: component}},
		Status: corev1.PodStatus{
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: component, RestartCount: 0}},
		},
	}
}

func establishedCiliumCRD(name string) *apixv1.CustomResourceDefinition {
	return ciliumCRD(name, "v2", true, true)
}

func ciliumCRD(name, version string, served, established bool) *apixv1.CustomResourceDefinition {
	condStatus := apixv1.ConditionTrue
	if !established {
		condStatus = apixv1.ConditionFalse
	}
	return &apixv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: apixv1.CustomResourceDefinitionSpec{
			Group: "cilium.io",
			Names: apixv1.CustomResourceDefinitionNames{Plural: "tests", Kind: "Test"},
			Versions: []apixv1.CustomResourceDefinitionVersion{{
				Name:    version,
				Served:  served,
				Storage: true,
				Schema:  &apixv1.CustomResourceValidation{OpenAPIV3Schema: &apixv1.JSONSchemaProps{Type: "object"}},
			}},
			Scope: apixv1.ClusterScoped,
		},
		Status: apixv1.CustomResourceDefinitionStatus{Conditions: []apixv1.CustomResourceDefinitionCondition{{
			Type:   apixv1.Established,
			Status: condStatus,
		}}},
	}
}
