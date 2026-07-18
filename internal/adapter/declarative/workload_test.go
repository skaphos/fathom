/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"

	"github.com/skaphos/fathom/pkg/adapter"
)

func statefulSet(name, namespace string, replicas, ready int32) *appsv1.StatefulSet {
	r := replicas
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &r,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKey: name}},
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: ready},
	}
}

func podWithRestarts(name, component, namespace string, restarts int32) *corev1.Pod {
	p := podInNamespace(name, component, namespace)
	p.Status.ContainerStatuses[0].RestartCount = restarts
	return p
}

func notReadyPod(name, component, namespace string) *corev1.Pod {
	p := podInNamespace(name, component, namespace)
	p.Status.Conditions[0].Status = corev1.ConditionFalse
	return p
}

func runEngine(t *testing.T, eng *Engine, policy map[adapter.Family]adapter.FamilyPolicy, objs ...clientObject) []adapter.CheckResult {
	t.Helper()
	res, err := eng.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Policy: policy,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res.Checks
}

// stsEngine builds a single-family engine with one StatefulSet workload, pods on.
func stsEngine(component, namespace string) *Engine {
	return MustEngine(AddonDefinition{
		AddonType:      "sts",
		AdapterVersion: "0.0.1",
		Families: []FamilyDefinition{{
			Name:           "system_health",
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:               KindStatefulSet,
				DefaultNamespace:   namespace,
				DefaultName:        component,
				Component:          component,
				Absence:            Required,
				CheckPods:          true,
				DefaultRestartWarn: 3,
			}},
		}},
	})
}

func TestWorkload_StatefulSetHealthy(t *testing.T) {
	checks := runEngine(t, stsEngine("db", "prod"), nil,
		statefulSet("db", "prod", 2, 2),
		podInNamespace("db-0", "db", "prod"),
		podInNamespace("db-1", "db", "prod"),
	)
	assertHasOutcome(t, checks, "StatefulSet", "db", adapter.OutcomePass, "fully ready")
	assertHasOutcome(t, checks, "Pod", "db-0", adapter.OutcomePass, "ready")
}

func TestWorkload_StatefulSetNotReadyFails(t *testing.T) {
	checks := runEngine(t, stsEngine("db", "prod"), nil, statefulSet("db", "prod", 3, 1))
	assertHasOutcome(t, checks, "StatefulSet", "db", adapter.OutcomeFail, "not fully ready")
}

func TestWorkload_StatefulSetScaledToZeroWarnsAndSkipsPods(t *testing.T) {
	checks := runEngine(t, stsEngine("db", "prod"), nil, statefulSet("db", "prod", 0, 0))
	assertHasOutcome(t, checks, "StatefulSet", "db", adapter.OutcomeWarn, "scaled to zero")
	assertNoKind(t, checks, "system_health", "Pod") // pod sub-check gated off
}

func TestWorkload_StatefulSetAbsentRequiredFails(t *testing.T) {
	checks := runEngine(t, stsEngine("db", "prod"), nil) // no objects
	assertHasOutcome(t, checks, "StatefulSet", "db", adapter.OutcomeFail, "not found")
	// Required-absent is a Fail that still carries the absent marker (SKA-526),
	// so "not installed" is distinguishable from an unhealthy-but-present target.
	assertHasDetail(t, checks, "StatefulSet", "db", adapter.DetailAbsent, "true")
}

// absenceEngine builds a single Deployment workload with the given component
// Posture (blank inherits the addon default) and addon-level Optional, to
// exercise required-by-default resolution and per-component override (SKA-526).
func absenceEngine(component Posture, addonOptional bool) *Engine {
	return MustEngine(AddonDefinition{
		AddonType:      "app",
		AdapterVersion: "0.0.1",
		Optional:       addonOptional,
		Families: []FamilyDefinition{{
			Name:           "system_health",
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:             KindDeployment,
				DefaultNamespace: "prod",
				DefaultName:      "app",
				Component:        "app",
				Absence:          component,
			}},
		}},
	})
}

func TestWorkload_AbsenceResolution(t *testing.T) {
	tests := []struct {
		name          string
		component     Posture
		addonOptional bool
		want          adapter.Outcome
	}{
		{"unset posture defaults to Required -> Fail", "", false, adapter.OutcomeFail},
		{"addon Optional makes unset posture Skipped", "", true, adapter.OutcomeSkipped},
		{"component Required overrides addon Optional", Required, true, adapter.OutcomeFail},
		{"component Optional overrides required default", Optional, false, adapter.OutcomeSkipped},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checks := runEngine(t, absenceEngine(tc.component, tc.addonOptional), nil) // no objects
			assertHasOutcome(t, checks, "Deployment", "app", tc.want, "not found")
			// Every absence path, Fail or Skipped, carries the absent marker.
			assertHasDetail(t, checks, "Deployment", "app", adapter.DetailAbsent, "true")
		})
	}
}

// deployEngine builds a Deployment workload with a policy-overridable restart
// threshold, to exercise checkPods + int32Threshold.
func deployEngine() *Engine {
	return MustEngine(AddonDefinition{
		AddonType:      "app",
		AdapterVersion: "0.0.1",
		Families: []FamilyDefinition{{
			Name:           "system_health",
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDeployment,
				DefaultNamespace:        "prod",
				DefaultName:             "app",
				Component:               "app",
				Absence:                 Required,
				CheckPods:               true,
				RestartWarnThresholdKey: "restartWarnCount",
				DefaultRestartWarn:      3,
			}},
		}},
	})
}

func TestWorkload_PodRestartWarn_ThresholdOverride(t *testing.T) {
	// Override the restart-warn threshold to 1 via policy; a pod with 2 restarts warns.
	policy := map[adapter.Family]adapter.FamilyPolicy{
		"system_health": {Enabled: true, Thresholds: map[string]string{"restartWarnCount": "1"}},
	}
	checks := runEngine(t, deployEngine(), policy,
		deploymentInNamespace("app", "prod"),
		podWithRestarts("app-x", "app", "prod", 2),
	)
	assertHasOutcome(t, checks, "Pod", "app-x", adapter.OutcomeWarn, "restart count")
	assertHasDetail(t, checks, "Pod", "app-x", "restartWarnCount", "1")
}

func TestWorkload_PodRestartWarn_InvalidThresholdFallsBack(t *testing.T) {
	// A non-numeric override falls back to DefaultRestartWarn (3); 2 restarts pass.
	policy := map[adapter.Family]adapter.FamilyPolicy{
		"system_health": {Enabled: true, Thresholds: map[string]string{"restartWarnCount": "not-a-number"}},
	}
	checks := runEngine(t, deployEngine(), policy,
		deploymentInNamespace("app", "prod"),
		podWithRestarts("app-x", "app", "prod", 2),
	)
	assertHasOutcome(t, checks, "Pod", "app-x", adapter.OutcomePass, "ready")
}

func TestWorkload_PodNotReadyFails(t *testing.T) {
	checks := runEngine(t, deployEngine(), nil,
		deploymentInNamespace("app", "prod"),
		notReadyPod("app-y", "app", "prod"),
	)
	assertHasOutcome(t, checks, "Pod", "app-y", adapter.OutcomeFail, "not ready")
}

func TestWorkload_NoMatchingPodsFails(t *testing.T) {
	// Deployment present and available, but no pods match its selector.
	checks := runEngine(t, deployEngine(), nil, deploymentInNamespace("app", "prod"))
	assertHasOutcome(t, checks, "Pod", "app", adapter.OutcomeFail, "no matching pods")
}

// TestWorkload_UnknownKindErrors bypasses NewEngine validation (direct struct
// literal) to reach readWorkload's defensive default branch.
func TestWorkload_UnknownKindErrors(t *testing.T) {
	eng := &Engine{def: AddonDefinition{
		AddonType:      "x",
		AdapterVersion: "0",
		Families: []FamilyDefinition{{
			Name:           "system_health",
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind: WorkloadKind("Bogus"), DefaultName: "n", DefaultNamespace: "ns", Component: "c",
			}},
		}},
	}}
	checks := runEngine(t, eng, nil)
	assertHasOutcome(t, checks, "Bogus", "n", adapter.OutcomeError, "unknown workload kind")
}
