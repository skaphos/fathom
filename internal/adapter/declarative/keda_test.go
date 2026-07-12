/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/skaphos/fathom/pkg/adapter"
)

// allFamiliesSelector enables the named families with an empty (match-all)
// LabelSelector. The engine's ManagedResources list uses the policy's
// LabelSelector; a nil selector resolves to labels.Nothing() (matching no
// objects), so tests that expect a managed-resource CR to be scored must supply
// a non-nil selector — mirroring the reconciler, which always populates one.
func allFamiliesSelector(families ...adapter.Family) map[adapter.Family]adapter.FamilyPolicy {
	policy := make(map[adapter.Family]adapter.FamilyPolicy, len(families))
	for _, f := range families {
		policy[f] = adapter.FamilyPolicy{Enabled: true, LabelSelector: &metav1.LabelSelector{}}
	}
	return policy
}

var kedaDeployments = []string{"keda-operator", "keda-operator-metrics-apiserver", "keda-admission-webhooks"}

var kedaCRDs = []string{
	"scaledobjects.keda.sh",
	"scaledjobs.keda.sh",
	"triggerauthentications.keda.sh",
	"clustertriggerauthentications.keda.sh",
}

// conditionCR builds a namespaced custom resource carrying the given
// status.conditions[type]=status entries.
func conditionCR(apiVersion, kind, namespace, name string, conds map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name, "namespace": namespace},
	}}
	entries := make([]any, 0, len(conds))
	for condType, status := range conds {
		entries = append(entries, map[string]any{"type": condType, "status": status})
	}
	if len(entries) > 0 {
		_ = unstructured.SetNestedSlice(obj.Object, entries, "status", "conditions")
	}
	return obj
}

func kedaHealthyObjects() []clientObject {
	objs := []clientObject{}
	for _, d := range kedaDeployments {
		objs = append(objs, deploymentInNamespace(d, "keda"), podInNamespace(d+"-abc", d, "keda"))
	}
	for _, c := range kedaCRDs {
		objs = append(objs, establishedCRD(c, "v1alpha1", true, true))
	}
	objs = append(objs, validatingConfig("keda-admission", wiredEntry("validate.keda.sh", "keda", "keda-admission-webhooks", []byte("ca"))))
	return objs
}

func TestKeda_HealthyWithReadyScaledObject(t *testing.T) {
	objs := append(kedaHealthyObjects(),
		conditionCR("keda.sh/v1alpha1", "ScaledObject", "team-a", "web", map[string]string{"Ready": "True", "Active": "True"}))
	result, err := NewKedaEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "keda"},
		Policy: allFamiliesSelector("system_health", "crd_health", "scaling_health"),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, d := range kedaDeployments {
		assertHasOutcome(t, result.Checks, "Deployment", d, adapter.OutcomePass, "available")
		assertFamily(t, result.Checks, "Deployment", d, adapter.Family("system_health"))
	}
	for _, c := range kedaCRDs {
		assertHasOutcome(t, result.Checks, "CustomResourceDefinition", c, adapter.OutcomePass, "established")
	}
	assertHasOutcome(t, result.Checks, "ValidatingWebhookConfiguration", "keda-admission", adapter.OutcomePass, "")
	assertHasOutcome(t, result.Checks, "ScaledObject", "web", adapter.OutcomePass, "Ready")
	assertFamily(t, result.Checks, "ScaledObject", "web", adapter.Family("scaling_health"))
}

func TestKeda_UnreadyScaledObjectFails(t *testing.T) {
	objs := append(kedaHealthyObjects(),
		conditionCR("keda.sh/v1alpha1", "ScaledObject", "team-a", "broken", map[string]string{"Ready": "False"}))
	result, err := NewKedaEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objs...), Logger: logr.Discard(), Policy: allFamiliesSelector("scaling_health")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "ScaledObject", "broken", adapter.OutcomeFail, "Ready")
}

func TestKeda_PausedScaledObjectWarns(t *testing.T) {
	objs := append(kedaHealthyObjects(),
		conditionCR("keda.sh/v1alpha1", "ScaledObject", "team-a", "held", map[string]string{"Ready": "True", "Paused": "True"}))
	result, err := NewKedaEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objs...), Logger: logr.Discard(), Policy: allFamiliesSelector("scaling_health")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Ready=True still passes; Paused=True is surfaced as a Warn on the same object.
	assertHasOutcome(t, result.Checks, "ScaledObject", "held", adapter.OutcomePass, "Ready")
	assertHasOutcome(t, result.Checks, "ScaledObject", "held", adapter.OutcomeWarn, "Paused")
}

func TestKeda_AbsentClusterAllSkipped(t *testing.T) {
	// Optional posture: on a cluster without KEDA every target is Skipped, and no
	// check is a hard Fail.
	result, err := NewKedaEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, d := range kedaDeployments {
		assertHasOutcome(t, result.Checks, "Deployment", d, adapter.OutcomeSkipped, "not found")
		assertHasDetail(t, result.Checks, "Deployment", d, adapter.DetailAbsent, "true")
	}
	for _, c := range kedaCRDs {
		assertHasOutcome(t, result.Checks, "CustomResourceDefinition", c, adapter.OutcomeSkipped, "not found")
	}
	assertHasOutcome(t, result.Checks, "ScaledObject", "scaledobjects", adapter.OutcomeSkipped, "no ScaledObject objects matched")
	for _, c := range result.Checks {
		if c.Outcome == adapter.OutcomeFail {
			t.Fatalf("no check should Fail on an absent Optional addon: %#v", c)
		}
	}
}
