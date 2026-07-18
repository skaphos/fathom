/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/skaphos/fathom/pkg/adapter"
)

var egCRDs = []string{
	"gatewayclasses.gateway.networking.k8s.io",
	"gateways.gateway.networking.k8s.io",
	"httproutes.gateway.networking.k8s.io",
	"envoyproxies.gateway.envoyproxy.io",
}

// egHealthyObjects returns a fully-healthy Envoy Gateway install: the
// controller Deployment (with a ready pod) in envoy-gateway-system plus the
// Gateway API core CRDs (v1) and the EnvoyProxy CRD (v1alpha1). Index order:
// [0]=Deployment, [1]=pod, [2..]=CRDs.
func egHealthyObjects() []clientObject {
	objs := []clientObject{
		deploymentInNamespace("envoy-gateway", "envoy-gateway-system"),
		podInNamespace("envoy-gateway-abc", "envoy-gateway", "envoy-gateway-system"),
	}
	for _, c := range egCRDs[:3] {
		objs = append(objs, establishedCRD(c, "v1", true, true))
	}
	objs = append(objs, establishedCRD(egCRDs[3], "v1alpha1", true, true))
	return objs
}

// gatewayObject builds a gateway.networking.k8s.io/v1 Gateway carrying the
// given condition types/statuses in status.conditions.
func gatewayObject(namespace, name string, conds map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "Gateway",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
	}}
	entries := make([]any, 0, len(conds))
	for ctype, status := range conds {
		entries = append(entries, map[string]any{"type": ctype, "status": status})
	}
	_ = unstructured.SetNestedSlice(obj.Object, entries, "status", "conditions")
	return obj
}

func TestEnvoyGateway_HealthyAndNoGatewaysSkipped(t *testing.T) {
	result, err := NewEnvoyGatewayEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, egHealthyObjects()...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "envoy-gateway"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertHasOutcome(t, result.Checks, "Deployment", "envoy-gateway", adapter.OutcomePass, "available")
	assertFamily(t, result.Checks, "Deployment", "envoy-gateway", adapter.Family("system_health"))
	for _, c := range egCRDs {
		assertHasOutcome(t, result.Checks, "CustomResourceDefinition", c, adapter.OutcomePass, "established")
		assertFamily(t, result.Checks, "CustomResourceDefinition", c, adapter.Family("crd_health"))
	}

	// No Gateway objects exist -> gateway_status is Skipped (the e2e's
	// empty-cluster contract), once per ConditionCheck — and the two rows must
	// be distinguishable by their conditionType detail, not identical twins.
	assertHasOutcome(t, result.Checks, "Gateway", "gateways", adapter.OutcomeSkipped, "no Gateway objects matched")
	assertFamily(t, result.Checks, "Gateway", "gateways", adapter.Family("gateway_status"))
	seen := map[string]int{}
	for _, c := range result.Checks {
		if c.Family == adapter.Family("gateway_status") && c.Outcome == adapter.OutcomeSkipped {
			seen[c.Details["conditionType"]]++
		}
	}
	if seen["Accepted"] != 1 || seen["Programmed"] != 1 {
		t.Errorf("gateway_status Skipped rows by conditionType: got %v, want one Accepted and one Programmed", seen)
	}
}

func TestEnvoyGateway_MissingDeploymentFails(t *testing.T) {
	// Required posture: an absent envoy-gateway Deployment is Fail.
	result, err := NewEnvoyGatewayEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, egHealthyObjects()[2:]...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{"system_health": {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "envoy-gateway", adapter.OutcomeFail, "not found")
	assertHasDetail(t, result.Checks, "Deployment", "envoy-gateway", adapter.DetailAbsent, "true")
}

func TestEnvoyGateway_GatewayConditionScoring(t *testing.T) {
	// Per-object scoring is tested directly against scoreObject (the fake
	// client's unstructured List is unreliable for dynamic CRs — see
	// condition_test.go). The definition carries one ConditionCheck per
	// condition type; a Gateway that is Accepted but not Programmed must Fail
	// the Programmed check and Pass the Accepted one.
	fam := EnvoyGatewayDefinition.Families[2]
	if got := string(fam.Name); got != "gateway_status" {
		t.Fatalf("Families[2]: got %q, want gateway_status", got)
	}
	accepted, programmed := fam.ManagedResources[0], fam.ManagedResources[1]
	ec := EvalContext{Family: fam.Name}
	gw := gatewayObject("apps", "public", map[string]string{"Accepted": "True", "Programmed": "False"})
	one := func(c adapter.CheckResult) []adapter.CheckResult { return []adapter.CheckResult{c} }

	pass := accepted.scoreObject(ec, gw, adapter.OutcomeFail, adapter.OutcomeFail, time.Now())
	assertHasOutcome(t, one(pass), "Gateway", "public", adapter.OutcomePass, "Accepted")

	fail := programmed.scoreObject(ec, gw, adapter.OutcomeFail, adapter.OutcomeFail, time.Now())
	assertHasOutcome(t, one(fail), "Gateway", "public", adapter.OutcomeFail, "want")
	assertHasDetail(t, one(fail), "Gateway", "public", "conditionType", "Programmed")
}

func TestEnvoyGateway_AdapterMetadata(t *testing.T) {
	eng := NewEnvoyGatewayEngine()
	if eng.Name() != "envoy-gateway" {
		t.Errorf("Name: got %q, want envoy-gateway", eng.Name())
	}
	caps := eng.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "envoy-gateway" {
		t.Errorf("AddonTypes: got %v, want [envoy-gateway]", caps.AddonTypes)
	}
	if len(caps.Families) != 3 {
		t.Errorf("Families: got %v, want [system_health crd_health gateway_status]", caps.Families)
	}
	if len(eng.RBACRules()) == 0 {
		t.Error("RBACRules: got none, want the declared read-only grants")
	}
}
