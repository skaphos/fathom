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

var istioCRDs = []string{
	"virtualservices.networking.istio.io",
	"destinationrules.networking.istio.io",
	"gateways.networking.istio.io",
	"serviceentries.networking.istio.io",
	"peerauthentications.security.istio.io",
	"authorizationpolicies.security.istio.io",
}

// istiodControlPlane returns the istiod Deployment with a ready pod.
func istiodControlPlane() []clientObject {
	return []clientObject{
		deploymentInNamespace("istiod", "istio-system"),
		podInNamespace("istiod-abc", "istiod", "istio-system"),
	}
}

// istioInjectorConfig returns the sidecar-injector configuration with every
// entry wired to istio-system/istiod and carrying caBundle.
func istioInjectorConfig(caBundle []byte) clientObject {
	return mutatingConfig("istio-sidecar-injector",
		wiredEntry("rev.namespace.sidecar-injector.istio.io", "istio-system", "istiod", caBundle),
		wiredEntry("rev.object.sidecar-injector.istio.io", "istio-system", "istiod", caBundle),
	)
}

// istioValidatorConfig returns the chart-owned validator configuration wired
// to istio-system/istiod.
func istioValidatorConfig() clientObject {
	return validatingConfig("istio-validator-istio-system",
		wiredEntry("rev.validation.istio.io", "istio-system", "istiod", []byte("ca")),
	)
}

// istioCRDObjects returns the core CRDs, established and serving v1.
func istioCRDObjects() []clientObject {
	objs := make([]clientObject, 0, len(istioCRDs))
	for _, name := range istioCRDs {
		objs = append(objs, establishedCRD(name, "v1", true, true))
	}
	return objs
}

// istioHealthyObjects returns a healthy sidecar-mode istio install: the istiod
// Deployment (with a ready pod), both wired webhook configurations, and the
// core CRDs — deliberately NO ztunnel or istio-cni DaemonSets, so the ambient
// families exercise the Optional-absence contract.
func istioHealthyObjects() []clientObject {
	objs := append(istiodControlPlane(), istioInjectorConfig([]byte("ca")), istioValidatorConfig())
	return append(objs, istioCRDObjects()...)
}

// istioAmbientObjects extends the healthy install with the ambient data plane:
// the ztunnel and istio-cni-node DaemonSets, each with a ready pod.
func istioAmbientObjects() []clientObject {
	return append(istioHealthyObjects(),
		daemonSetInNamespace("ztunnel", "istio-system", 1),
		podInNamespace("ztunnel-node1", "ztunnel", "istio-system"),
		daemonSetInNamespace("istio-cni-node", "istio-system", 1),
		podInNamespace("istio-cni-node-node1", "istio-cni-node", "istio-system"),
	)
}

func TestIstio_SidecarModeHealthy(t *testing.T) {
	result, err := NewIstioEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, istioHealthyObjects()...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "istio"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertHasOutcome(t, result.Checks, "Deployment", "istiod", adapter.OutcomePass, "available")
	assertFamily(t, result.Checks, "Deployment", "istiod", adapter.Family("system_health"))
	assertHasOutcome(t, result.Checks, KindMutatingWebhookConfiguration, "istio-sidecar-injector", adapter.OutcomePass, "wired")
	assertFamily(t, result.Checks, KindMutatingWebhookConfiguration, "istio-sidecar-injector", adapter.Family("system_health"))
	assertHasOutcome(t, result.Checks, KindValidatingWebhookConfiguration, "istio-validator-istio-system", adapter.OutcomePass, "wired")
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "virtualservices.networking.istio.io", adapter.OutcomePass, "established")
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "peerauthentications.security.istio.io", adapter.OutcomePass, "established")

	// The Optional-absence contract: a sidecar-mode mesh has no ambient data
	// plane, so both ambient families are Skipped with the absent marker.
	assertHasOutcome(t, result.Checks, "DaemonSet", "ztunnel", adapter.OutcomeSkipped, "not found")
	assertHasDetail(t, result.Checks, "DaemonSet", "ztunnel", adapter.DetailAbsent, "true")
	assertHasOutcome(t, result.Checks, "DaemonSet", "istio-cni-node", adapter.OutcomeSkipped, "not found")
	assertHasDetail(t, result.Checks, "DaemonSet", "istio-cni-node", adapter.DetailAbsent, "true")
}

func TestIstio_AmbientDataPlanePasses(t *testing.T) {
	result, err := NewIstioEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, istioAmbientObjects()...),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertHasOutcome(t, result.Checks, "DaemonSet", "ztunnel", adapter.OutcomePass, "ready")
	assertFamily(t, result.Checks, "DaemonSet", "ztunnel", adapter.Family("ztunnel_health"))
	assertHasOutcome(t, result.Checks, "DaemonSet", "istio-cni-node", adapter.OutcomePass, "ready")
	assertFamily(t, result.Checks, "DaemonSet", "istio-cni-node", adapter.Family("istio_cni_health"))
}

func TestIstio_UnpopulatedCABundleFails(t *testing.T) {
	// The istio failure mode the webhook check exists for: istiod pods Ready
	// but the injector's caBundle never patched — system_health must Fail on
	// the webhook while the Deployment stays green.
	objs := append(istiodControlPlane(), istioInjectorConfig(nil), istioValidatorConfig())
	result, err := NewIstioEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{"system_health": {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertHasOutcome(t, result.Checks, KindMutatingWebhookConfiguration, "istio-sidecar-injector", adapter.OutcomeFail, "caBundle")
	assertHasOutcome(t, result.Checks, "Deployment", "istiod", adapter.OutcomePass, "available")
}

func TestIstio_MissingDeploymentFails(t *testing.T) {
	// istiod is Required: a mesh AddonCheck against a cluster without the
	// control plane is Fail-with-absent-marker, not a quiet skip.
	objs := []clientObject{istioInjectorConfig([]byte("ca")), istioValidatorConfig()}
	result, err := NewIstioEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{"system_health": {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "istiod", adapter.OutcomeFail, "not found")
	assertHasDetail(t, result.Checks, "Deployment", "istiod", adapter.DetailAbsent, "true")
}

func TestIstio_RelocatedControlPlanePasses(t *testing.T) {
	// Regression (adversarial review of #124): a mesh installed outside
	// istio-system must be reachable entirely through policy — namespaces
	// redirect the workload AND the expected backing service, and the
	// validator's namespace-suffixed name is overridden by threshold. Without
	// this, a healthy relocated mesh reported system_health Fail permanently.
	objs := []clientObject{
		deploymentInNamespace("istiod", "istio-mesh"),
		podInNamespace("istiod-abc", "istiod", "istio-mesh"),
		mutatingConfig("istio-sidecar-injector",
			wiredEntry("rev.namespace.sidecar-injector.istio.io", "istio-mesh", "istiod", []byte("ca"))),
		validatingConfig("istio-validator-istio-mesh",
			wiredEntry("rev.validation.istio.io", "istio-mesh", "istiod", []byte("ca"))),
	}
	result, err := NewIstioEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			"system_health": {
				Enabled:    true,
				Namespaces: []string{"istio-mesh"},
				Thresholds: map[string]string{"validatorWebhookName": "istio-validator-istio-mesh"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertHasOutcome(t, result.Checks, "Deployment", "istiod", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, KindMutatingWebhookConfiguration, "istio-sidecar-injector", adapter.OutcomePass, "wired")
	assertHasOutcome(t, result.Checks, KindValidatingWebhookConfiguration, "istio-validator-istio-mesh", adapter.OutcomePass, "wired")
}

func TestIstio_DeploymentNameThresholdOverride(t *testing.T) {
	// A revisioned control plane names its Deployment istiod-<rev>; the
	// deploymentName threshold points the check at it.
	objs := []clientObject{
		deploymentInNamespace("istiod-1-30", "istio-system"),
		podInNamespace("istiod-1-30-abc", "istiod-1-30", "istio-system"),
	}
	result, err := NewIstioEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			"system_health": {Enabled: true, Thresholds: map[string]string{"deploymentName": "istiod-1-30"}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "istiod-1-30", adapter.OutcomePass, "available")
}

func TestIstio_AdapterMetadata(t *testing.T) {
	eng := NewIstioEngine()
	if eng.Name() != "istio" {
		t.Errorf("Name: got %q, want istio", eng.Name())
	}
	caps := eng.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "istio" {
		t.Errorf("AddonTypes: got %v, want [istio]", caps.AddonTypes)
	}
	want := []adapter.Family{"system_health", "ztunnel_health", "istio_cni_health", "crd_health"}
	if len(caps.Families) != len(want) {
		t.Fatalf("Families: got %v, want %v", caps.Families, want)
	}
	for i, f := range want {
		if caps.Families[i] != f {
			t.Errorf("Families[%d]: got %q, want %q", i, caps.Families[i], f)
		}
	}
	if len(eng.RBACRules()) == 0 {
		t.Error("RBACRules: got none, want the declared read-only grants")
	}
}
