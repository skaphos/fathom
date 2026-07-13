/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"

	"github.com/go-logr/logr"

	"github.com/skaphos/fathom/pkg/adapter"
)

var vpaDeployments = []string{"vpa-recommender", "vpa-updater", "vpa-admission-controller"}

var vpaCRDs = []string{
	"verticalpodautoscalers.autoscaling.k8s.io",
	"verticalpodautoscalercheckpoints.autoscaling.k8s.io",
}

func vpaHealthyObjects() []clientObject {
	objs := []clientObject{}
	for _, d := range vpaDeployments {
		objs = append(objs, deploymentInNamespace(d, "kube-system"), podInNamespace(d+"-abc", d, "kube-system"))
	}
	for _, c := range vpaCRDs {
		objs = append(objs, establishedCRD(c, "v1", true, true))
	}
	objs = append(objs, mutatingConfig("vpa-webhook-config", wiredEntry("vpa.k8s.io", "kube-system", "vpa-webhook", []byte("ca"))))
	return objs
}

func TestVpa_HealthyWithRecommendation(t *testing.T) {
	objs := append(vpaHealthyObjects(),
		conditionCR("autoscaling.k8s.io/v1", "VerticalPodAutoscaler", "team-a", "web", map[string]string{"RecommendationProvided": "True"}))
	result, err := NewVpaEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "vpa"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, d := range vpaDeployments {
		assertHasOutcome(t, result.Checks, "Deployment", d, adapter.OutcomePass, "available")
		assertFamily(t, result.Checks, "Deployment", d, adapter.Family("system_health"))
	}
	for _, c := range vpaCRDs {
		assertHasOutcome(t, result.Checks, "CustomResourceDefinition", c, adapter.OutcomePass, "established")
	}
	assertHasOutcome(t, result.Checks, "MutatingWebhookConfiguration", "vpa-webhook-config", adapter.OutcomePass, "")
	assertHasOutcome(t, result.Checks, "VerticalPodAutoscaler", "web", adapter.OutcomePass, "RecommendationProvided")
	assertFamily(t, result.Checks, "VerticalPodAutoscaler", "web", adapter.Family("recommendation_health"))
}

func TestVpa_NoRecommendationWarns(t *testing.T) {
	// A VPA that has not produced a recommendation yet (condition absent) is a
	// Warn, not a Fail — it is often young or has no matching pods.
	objs := append(vpaHealthyObjects(),
		conditionCR("autoscaling.k8s.io/v1", "VerticalPodAutoscaler", "team-a", "cold", nil))
	result, err := NewVpaEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objs...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "VerticalPodAutoscaler", "cold", adapter.OutcomeWarn, "RecommendationProvided")
}

func TestVpa_AbsentClusterAllSkipped(t *testing.T) {
	result, err := NewVpaEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, d := range vpaDeployments {
		assertHasOutcome(t, result.Checks, "Deployment", d, adapter.OutcomeSkipped, "not found")
	}
	for _, c := range vpaCRDs {
		assertHasOutcome(t, result.Checks, "CustomResourceDefinition", c, adapter.OutcomeSkipped, "not found")
	}
	assertHasOutcome(t, result.Checks, "VerticalPodAutoscaler", "verticalpodautoscalers", adapter.OutcomeSkipped, "no VerticalPodAutoscaler objects matched")
	for _, c := range result.Checks {
		if c.Outcome == adapter.OutcomeFail {
			t.Fatalf("no check should Fail on an absent Optional addon: %#v", c)
		}
	}
}
