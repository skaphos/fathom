/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package cilium

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/skaphos/fathom/pkg/adapter"
)

var errInjected = errors.New("injected client error")

// newFakeClientWithErrors wraps the standard fake client with interceptors that
// surface non-NotFound failures: a Get on any object whose name is in
// getFailNames, and (when failList is true) every List. This exercises the
// OutcomeError branches — the adapter must distinguish "adapter could not
// determine state" (Error) from "target absent" (Skipped) and "target
// unhealthy" (Fail).
func newFakeClientWithErrors(t *testing.T, getFailNames map[string]bool, failList bool, objects ...clientObject) client.Client {
	t.Helper()
	base := newFakeClient(t, objects...).(client.WithWatch)
	return interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if getFailNames[key.Name] {
				return apierrors.NewInternalError(errInjected)
			}
			return c.Get(ctx, key, obj, opts...)
		},
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if failList {
				return apierrors.NewInternalError(errInjected)
			}
			return c.List(ctx, list, opts...)
		},
	})
}

// --- Error-path coverage (finding #1) ---

func TestRun_DeploymentGetErrorReportsError(t *testing.T) {
	c := newFakeClientWithErrors(t, map[string]bool{defaultOperatorName: true}, false, healthyObjects()...)
	result, err := New().Run(context.Background(), adapter.Request{Client: c, Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", defaultOperatorName, adapter.OutcomeError, "failed to read")
}

func TestRun_DaemonSetGetErrorReportsError(t *testing.T) {
	c := newFakeClientWithErrors(t, map[string]bool{defaultAgentName: true}, false, healthyObjects()...)
	result, err := New().Run(context.Background(), adapter.Request{Client: c, Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultAgentName, adapter.OutcomeError, "failed to read")
}

func TestRun_CRDGetErrorReportsError(t *testing.T) {
	c := newFakeClientWithErrors(t, map[string]bool{"ciliumnodes.cilium.io": true}, false, healthyObjects()...)
	result, err := New().Run(context.Background(), adapter.Request{Client: c, Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "ciliumnodes.cilium.io", adapter.OutcomeError, "failed to read")
}

func TestRun_PodListErrorReportsError(t *testing.T) {
	c := newFakeClientWithErrors(t, nil, true, healthyObjects()...)
	result, err := New().Run(context.Background(), adapter.Request{
		Client: c,
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilyControlPlaneHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Deployment Get succeeds (present, desired>0) so checkPods runs and its List fails.
	assertHasOutcome(t, result.Checks, "Pod", componentOperator, adapter.OutcomeError, "failed to list")
}

// --- restartWarnCount / int32Threshold coverage (finding #2) ---

func TestRun_CustomRestartWarnCountHonored(t *testing.T) {
	objects := healthyObjects()
	objects[1].(*corev1.Pod).Status.ContainerStatuses[0].RestartCount = 4
	// Default threshold is 3 (4 > 3 => Warn); raising it to 5 makes 4 acceptable.
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilyControlPlaneHealth: {Enabled: true, Thresholds: map[string]string{thresholdRestartWarnCount: "5"}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", objects[1].GetName(), adapter.OutcomePass, "ready")
}

func TestRun_InvalidRestartWarnCountFallsBackToDefault(t *testing.T) {
	objects := healthyObjects()
	objects[1].(*corev1.Pod).Status.ContainerStatuses[0].RestartCount = 4
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			FamilyControlPlaneHealth: {Enabled: true, Thresholds: map[string]string{thresholdRestartWarnCount: "not-a-number"}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", objects[1].GetName(), adapter.OutcomeWarn, "restart count")
}

func TestInt32Threshold(t *testing.T) {
	cases := []struct {
		name       string
		thresholds map[string]string
		def        int32
		want       int32
	}{
		{"nil map", nil, 3, 3},
		{"missing key", map[string]string{"other": "5"}, 3, 3},
		{"valid", map[string]string{"k": "7"}, 3, 7},
		{"zero", map[string]string{"k": "0"}, 3, 0},
		{"negative falls back", map[string]string{"k": "-1"}, 3, 3},
		{"non-numeric falls back", map[string]string{"k": "abc"}, 3, 3},
		{"overflow falls back", map[string]string{"k": "99999999999"}, 3, 3},
		{"empty falls back", map[string]string{"k": ""}, 3, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := int32Threshold(adapter.FamilyPolicy{Thresholds: tc.thresholds}, "k", tc.def); got != tc.want {
				t.Fatalf("int32Threshold: got %d, want %d", got, tc.want)
			}
		})
	}
}

// --- Present workload, zero matching pods (finding #3) ---

func TestRun_PresentWorkloadsWithNoPodsFail(t *testing.T) {
	objects := healthyObjects()
	kept := []clientObject{objects[0], objects[2]} // operator Deployment + agent DaemonSet, no pods
	kept = append(kept, objects[5:]...)            // CRDs
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, kept...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The no-pods CheckResult targets the component constant, not the workload name.
	assertHasOutcome(t, result.Checks, "Pod", componentOperator, adapter.OutcomeFail, "no matching pods")
	assertHasOutcome(t, result.Checks, "Pod", componentAgent, adapter.OutcomeFail, "no matching pods")
	if got := adapter.FamilyOutcome(result.Checks, FamilyControlPlaneHealth); got != adapter.OutcomeFail {
		t.Fatalf("control_plane_health roll-up: got %s, want Fail", got)
	}
	if got := adapter.FamilyOutcome(result.Checks, FamilyAgentHealth); got != adapter.OutcomeFail {
		t.Fatalf("agent_health roll-up: got %s, want Fail", got)
	}
}

// --- Isolated Fail disjuncts (finding #4) ---

func TestRun_OperatorReplicaShortfallFails(t *testing.T) {
	objects := healthyObjects()
	deployment := objects[0].(*appsv1.Deployment)
	two := int32(2)
	deployment.Spec.Replicas = &two // Available=True condition present, but AvailableReplicas(1) < desired(2)
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", defaultOperatorName, adapter.OutcomeFail, "not fully available")
}

func TestRun_OperatorMissingAvailableConditionFails(t *testing.T) {
	objects := healthyObjects()
	deployment := objects[0].(*appsv1.Deployment)
	deployment.Status.Conditions = nil // AvailableReplicas(1) >= desired(1), but no Available=True condition
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", defaultOperatorName, adapter.OutcomeFail, "not fully available")
}

func TestRun_AgentUnavailableAloneFails(t *testing.T) {
	objects := healthyObjects()
	daemonset := objects[2].(*appsv1.DaemonSet)
	daemonset.Status.NumberUnavailable = 1 // NumberReady(2) >= desired(2) but Unavailable > 0
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultAgentName, adapter.OutcomeFail, "not fully ready")
}

func TestRun_AgentNotEnoughReadyAloneFails(t *testing.T) {
	objects := healthyObjects()
	daemonset := objects[2].(*appsv1.DaemonSet)
	daemonset.Status.NumberReady = 1 // NumberReady(1) < desired(2)
	daemonset.Status.NumberUnavailable = 0
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", defaultAgentName, adapter.OutcomeFail, "not fully ready")
}

// --- desiredReplicas nil default (finding #5) ---

func TestRun_OperatorNilReplicasDefaultsToOne(t *testing.T) {
	objects := healthyObjects()
	deployment := objects[0].(*appsv1.Deployment)
	deployment.Spec.Replicas = nil // desiredReplicas should default to 1
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objects...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{FamilyControlPlaneHealth: {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", defaultOperatorName, adapter.OutcomePass, "available")
	// desired==1 (>0) means the pod check runs: a passing pod proves the gate fired.
	assertHasOutcome(t, result.Checks, "Pod", objects[1].GetName(), adapter.OutcomePass, "ready")
}

// --- Pod family attribution (finding #6) ---

func TestRun_PodChecksCarryGatingFamily(t *testing.T) {
	result, err := New().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyObjects()...),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertFamily(t, result.Checks, "Pod", defaultOperatorName+"-7d9c", FamilyControlPlaneHealth)
	assertFamily(t, result.Checks, "Pod", defaultAgentName+"-node1", FamilyAgentHealth)
	assertFamily(t, result.Checks, "Pod", defaultAgentName+"-node2", FamilyAgentHealth)
}

// --- Pod helper edge cases (finding #7) ---

func TestRun_PodRestartAtThresholdPasses(t *testing.T) {
	objects := healthyObjects()
	objects[1].(*corev1.Pod).Status.ContainerStatuses[0].RestartCount = defaultRestartWarnCount // == 3, not > 3
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", objects[1].GetName(), adapter.OutcomePass, "ready")
}

func TestRun_PodWithoutReadyConditionFails(t *testing.T) {
	objects := healthyObjects()
	objects[1].(*corev1.Pod).Status.Conditions = nil // podReady falls through to false
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", objects[1].GetName(), adapter.OutcomeFail, "not ready")
}

func TestRun_PodMaxRestartAcrossContainersWarns(t *testing.T) {
	objects := healthyObjects()
	objects[1].(*corev1.Pod).Status.ContainerStatuses = []corev1.ContainerStatus{
		{Name: "config", RestartCount: 0},
		{Name: "cilium-operator", RestartCount: 5}, // maxRestartCount must pick the larger
	}
	result, err := New().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objects...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", objects[1].GetName(), adapter.OutcomeWarn, "restart count")
}
