/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

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

const (
	defaultNamespace    = "kube-system"
	defaultOperatorName = "cilium-operator"
	defaultAgentName    = "cilium"
	testLabelKey        = "app.kubernetes.io/name"
)

func ciliumCRDNames() []string {
	return CiliumDefinition.Families[2].CRDs[0].Names
}

func TestEngine_Metadata(t *testing.T) {
	e := NewCiliumEngine()
	if e.Name() != "cilium" {
		t.Fatalf("Name: got %q, want cilium", e.Name())
	}
	if e.Version() != "0.2.0" {
		t.Fatalf("Version: got %q, want 0.2.0", e.Version())
	}
	if e.ContractVersion() != adapter.ContractVersion {
		t.Fatalf("ContractVersion: got %q, want %q", e.ContractVersion(), adapter.ContractVersion)
	}
	caps := e.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "cilium" {
		t.Fatalf("AddonTypes: got %#v, want [cilium]", caps.AddonTypes)
	}
	if len(caps.Families) != 3 {
		t.Fatalf("Families: got %#v, want 3 families", caps.Families)
	}
}

func TestEngine_AllFamiliesPassByDefault(t *testing.T) {
	result, err := NewCiliumEngine().Run(context.Background(), adapter.Request{
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
	assertFamily(t, result.Checks, "Deployment", defaultOperatorName, adapter.Family("control_plane_health"))
	assertFamily(t, result.Checks, "DaemonSet", defaultAgentName, adapter.Family("agent_health"))
	assertFamily(t, result.Checks, "CustomResourceDefinition", "ciliumnodes.cilium.io", adapter.Family("crd_health"))
}

func TestEngine_NotInstalledOptionalAbsent(t *testing.T) {
	result, err := NewCiliumEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// deployment (no pods) + daemonset (no pods) + 5 CRDs.
	if len(result.Checks) != 7 {
		t.Fatalf("checks: got %d, want 7: %#v", len(result.Checks), result.Checks)
	}
	for _, check := range result.Checks {
		// Cilium is an Optional addon, so each absent target is Skipped and
		// carries the absent marker (SKA-526) — absence is a fact about the
		// target, not a skipReason category.
		if check.Outcome != adapter.OutcomeSkipped {
			t.Fatalf("check %s/%s outcome: got %s, want Skipped: %s", check.TargetRef.Kind, check.TargetRef.Name, check.Outcome, check.Summary)
		}
		if !adapter.IsAbsent(check.Details) {
			t.Fatalf("check %s/%s: want absent marker, got Details=%v", check.TargetRef.Kind, check.TargetRef.Name, check.Details)
		}
	}
	// The persisted verdict of an all-Skipped run is Skipped; only the metrics/
	// tracing FamilyOutcome roll-up relabels an absent Optional family as Pass.
	for _, family := range []adapter.Family{"control_plane_health", "agent_health", "crd_health"} {
		if got := adapter.FamilyOutcome(result.Checks, family); got != adapter.OutcomePass {
			t.Fatalf("FamilyOutcome(%s): got %s, want Pass", family, got)
		}
	}
}

func TestEngine_MissingOperatorDeploymentSkipped(t *testing.T) {
	objects := healthyObjects()
	objects = objects[1:] // drop the operator Deployment
	result, err := NewCiliumEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", defaultOperatorName, adapter.OutcomeSkipped, "not found")
	// A skipped (absent) deployment must not produce any pod checks.
	assertNoKind(t, result.Checks, "control_plane_health", "Pod")
}

func TestEngine_AgentDaemonSetNotReadyFails(t *testing.T) {
	objects := healthyObjects()
	daemonset := objects[2].(*appsv1.DaemonSet)
	daemonset.Status.NumberReady = 1
	daemonset.Status.NumberUnavailable = 1
	result, err := NewCiliumEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultAgentName, adapter.OutcomeFail, "not fully ready")
	assertHasDetail(t, result.Checks, "DaemonSet", defaultAgentName, "numberUnavailable", "1")
}

func TestEngine_OperatorScaledToZeroWarnsAndSkipsPods(t *testing.T) {
	objects := healthyObjects()
	deployment := objects[0].(*appsv1.Deployment)
	zero := int32(0)
	deployment.Spec.Replicas = &zero
	result, err := NewCiliumEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", defaultOperatorName, adapter.OutcomeWarn, "scaled to zero")
	assertNoOutcome(t, result.Checks, "Pod", defaultOperatorName, adapter.OutcomeFail)
}

func TestEngine_CRDUnsupportedVersionWarns(t *testing.T) {
	objects := healthyObjects()
	// Established, but only serves an unrecognized version.
	objects[len(objects)-1] = establishedCRD("ciliumnodes.cilium.io", "v1", true, true)
	result, err := NewCiliumEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "ciliumnodes.cilium.io", adapter.OutcomeWarn, "no recognized version")
}

func TestEngine_CRDNotEstablishedFails(t *testing.T) {
	objects := healthyObjects()
	objects[len(objects)-1] = establishedCRD("ciliumnodes.cilium.io", "v2", true, false)
	result, err := NewCiliumEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "ciliumnodes.cilium.io", adapter.OutcomeFail, "not established")
}

func TestEngine_AllFamiliesDisabledSkips(t *testing.T) {
	target := adapter.TargetRef{APIVersion: "fathom.skaphos.io/v1alpha1", Kind: "AddonCheck", Namespace: "default", Name: "cilium"}
	result, err := NewCiliumEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyObjects()...),
		Logger: logr.Discard(),
		Target: target,
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			"control_plane_health": {Enabled: false},
			"agent_health":         {Enabled: false},
			"crd_health":           {Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Checks) != 1 {
		t.Fatalf("checks: got %d, want 1: %#v", len(result.Checks), result.Checks)
	}
	c := result.Checks[0]
	if c.Outcome != adapter.OutcomeSkipped {
		t.Fatalf("outcome: got %s, want Skipped", c.Outcome)
	}
	if c.Family != adapter.Family("control_plane_health") {
		t.Fatalf("family: got %s, want control_plane_health", c.Family)
	}
	if c.TargetRef != target {
		t.Fatalf("targetRef: got %#v, want %#v", c.TargetRef, target)
	}
	if c.Details["skipReason"] != "FamilyDisabled" {
		t.Fatalf("skipReason: got %q, want FamilyDisabled", c.Details["skipReason"])
	}
}

func TestEngine_OneFamilyDisabled(t *testing.T) {
	result, err := NewCiliumEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyObjects()...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			"control_plane_health": {Enabled: true},
			"agent_health":         {Enabled: true},
			"crd_health":           {Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertNoKind(t, result.Checks, "crd_health", "CustomResourceDefinition")
	assertHasOutcome(t, result.Checks, "Deployment", defaultOperatorName, adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultAgentName, adapter.OutcomePass, "fully ready")
}

func TestEngine_CustomNamesAndNamespace(t *testing.T) {
	const ns = "cilium-system"
	result, err := NewCiliumEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t,
			deploymentInNamespace("rke2-cilium-operator", ns),
			podInNamespace("rke2-cilium-operator-abc", "rke2-cilium-operator", ns),
			daemonSetInNamespace("rke2-cilium", ns, 1),
			podInNamespace("rke2-cilium-node1", "rke2-cilium", ns),
		),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			"control_plane_health": {Enabled: true, Namespaces: []string{ns}, Thresholds: map[string]string{"operatorDeploymentName": "rke2-cilium-operator"}},
			"agent_health":         {Enabled: true, Namespaces: []string{ns}, Thresholds: map[string]string{"agentDaemonSetName": "rke2-cilium"}},
			"crd_health":           {Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "rke2-cilium-operator", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "DaemonSet", "rke2-cilium", adapter.OutcomePass, "fully ready")
}

func TestNewEngine_Validation(t *testing.T) {
	cases := []struct {
		name string
		def  AddonDefinition
	}{
		{"empty addon type", AddonDefinition{Families: []FamilyDefinition{{Name: "f"}}}},
		{"no families", AddonDefinition{AddonType: "x"}},
		{"duplicate family", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f"}, {Name: "f"}}}},
		{"unknown workload kind", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f", Workloads: []WorkloadCheck{{Kind: "Nope"}}}}}},
		{"empty crd names", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f", CRDs: []CRDCheck{{}}}}}},
		{"crd without supported versions", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f", CRDs: []CRDCheck{{Names: []string{"foos.example.io"}}}}}}},
		{"managed resource with empty name", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f", ManagedResources: []ConditionCheck{{Kind: "Widget", Names: []string{""}}}}}}},
		{"apiservice with empty name", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f", APIServices: []ConditionCheck{{Kind: "APIService", Names: []string{"good", ""}}}}}}},
		{"cronjob without name", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f", CronJobs: []CronJobCheck{{DefaultNamespace: "ns"}}}}}},
		{"cronjob without namespace", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f", CronJobs: []CronJobCheck{{DefaultName: "c"}}}}}},
		{"configmap without namespace", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f", ConfigMaps: []ConfigMapCheck{{DefaultName: "c", Key: "policy.yaml"}}}}}},
		{"configmap without key", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f", ConfigMaps: []ConfigMapCheck{{DefaultName: "c", DefaultNamespace: "ns"}}}}}},
		{"annotation without key", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f", Annotations: []AnnotationStalenessCheck{{APIVersion: "apps/v1", Kind: "DaemonSet", DefaultName: "d", DefaultNamespace: "ns"}}}}}},
		{"namespaced annotation without namespace", AddonDefinition{AddonType: "x", Families: []FamilyDefinition{{Name: "f", Annotations: []AnnotationStalenessCheck{{APIVersion: "apps/v1", Kind: "DaemonSet", DefaultName: "d", AnnotationKey: "k"}}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewEngine(tc.def); err == nil {
				t.Fatalf("NewEngine(%s): got nil error, want validation error", tc.name)
			}
		})
	}
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

func assertNoKind(t *testing.T, checks []adapter.CheckResult, family, kind string) {
	t.Helper()
	for _, check := range checks {
		if string(check.Family) == family && check.TargetRef.Kind == kind {
			t.Fatalf("unexpected %s check in family %s: %#v", kind, family, check)
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
			t.Fatalf("family for %s/%s: got %q, want %q", kind, name, check.Family, want)
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

// healthyObjects returns a fully-healthy Cilium install. Index order is relied
// on by mutation tests: [0]=operator Deployment, [1]=operator Pod,
// [2]=agent DaemonSet, [3..4]=agent Pods, [5..]=CRDs.
func healthyObjects() []clientObject {
	objects := []clientObject{
		deploymentInNamespace(defaultOperatorName, defaultNamespace),
		podInNamespace(defaultOperatorName+"-7d9c", defaultOperatorName, defaultNamespace),
		daemonSetInNamespace(defaultAgentName, defaultNamespace, 2),
		podInNamespace(defaultAgentName+"-node1", defaultAgentName, defaultNamespace),
		podInNamespace(defaultAgentName+"-node2", defaultAgentName, defaultNamespace),
	}
	for _, name := range ciliumCRDNames() {
		objects = append(objects, establishedCRD(name, "v2", true, true))
	}
	return objects
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

func podInNamespace(name, component, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{testLabelKey: component}},
		Status: corev1.PodStatus{
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: component, RestartCount: 0}},
		},
	}
}

// establishedCRD builds a CRD object named "<plural>.<group>" serving the given
// version. Group and plural are derived from name so fixtures are self-consistent
// (e.g. externalsecrets.external-secrets.io -> group external-secrets.io, plural
// externalsecrets), unlike a hard-coded group. served/established toggle the
// served flag and the Established condition.
func establishedCRD(name, version string, served, established bool) *apixv1.CustomResourceDefinition {
	condStatus := apixv1.ConditionTrue
	if !established {
		condStatus = apixv1.ConditionFalse
	}
	plural, group, _ := strings.Cut(name, ".")
	return &apixv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: apixv1.CustomResourceDefinitionSpec{
			Group: group,
			Names: apixv1.CustomResourceDefinitionNames{Plural: plural, Kind: "Test"},
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
