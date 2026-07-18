/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"

	"github.com/go-logr/logr"

	"github.com/skaphos/fathom/pkg/adapter"
)

// extdnsHealthyObjects returns a fully-healthy external-dns install: the
// controller Deployment (with a ready pod) in the external-dns namespace and
// the established v1alpha1 DNSEndpoint CRD. Index order: [0]=Deployment,
// [1]=pod, [2]=CRD.
func extdnsHealthyObjects() []clientObject {
	return []clientObject{
		deploymentInNamespace("external-dns", "external-dns"),
		podInNamespace("external-dns-abc", "external-dns", "external-dns"),
		establishedCRD("dnsendpoints.externaldns.k8s.io", "v1alpha1", true, true),
	}
}

func TestExternalDNS_HealthyPassesAllFamilies(t *testing.T) {
	result, err := NewExternalDNSEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, extdnsHealthyObjects()...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "external-dns"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertHasOutcome(t, result.Checks, "Deployment", "external-dns", adapter.OutcomePass, "available")
	assertFamily(t, result.Checks, "Deployment", "external-dns", adapter.Family("system_health"))
	assertHasOutcome(t, result.Checks, "Pod", "external-dns-abc", adapter.OutcomePass, "ready")
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "dnsendpoints.externaldns.k8s.io", adapter.OutcomePass, "established")
	assertFamily(t, result.Checks, "CustomResourceDefinition", "dnsendpoints.externaldns.k8s.io", adapter.Family("crd_health"))
}

func TestExternalDNS_MissingDeploymentFails(t *testing.T) {
	// Required posture: an absent external-dns Deployment is Fail. Keep only the
	// CRD so system_health sees an empty cluster.
	result, err := NewExternalDNSEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, extdnsHealthyObjects()[2:]...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{"system_health": {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "external-dns", adapter.OutcomeFail, "not found")
	assertHasDetail(t, result.Checks, "Deployment", "external-dns", adapter.DetailAbsent, "true")
}

func TestExternalDNS_MissingCRDSkippedOptional(t *testing.T) {
	// The DNSEndpoint CRD is an opt-in install shape: its component-level
	// Optional posture overrides the addon's required-by-default, so absence is
	// Skipped (with the absent marker), not Fail.
	result, err := NewExternalDNSEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, extdnsHealthyObjects()[:2]...),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "CustomResourceDefinition", "dnsendpoints.externaldns.k8s.io", adapter.OutcomeSkipped, "not found")
	assertHasDetail(t, result.Checks, "CustomResourceDefinition", "dnsendpoints.externaldns.k8s.io", adapter.DetailAbsent, "true")
	// The Deployment stays healthy — the optional CRD must not drag it down.
	assertHasOutcome(t, result.Checks, "Deployment", "external-dns", adapter.OutcomePass, "available")
}

func TestExternalDNS_DeploymentNameThresholdOverride(t *testing.T) {
	// The chart names the Deployment after the Helm release; the deploymentName
	// threshold points the check at a renamed install.
	result, err := NewExternalDNSEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t,
			deploymentInNamespace("dns-controller", "external-dns"),
			podInNamespace("dns-controller-abc", "dns-controller", "external-dns")),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			"system_health": {Enabled: true, Thresholds: map[string]string{"deploymentName": "dns-controller"}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "dns-controller", adapter.OutcomePass, "available")
}

func TestExternalDNS_AdapterMetadata(t *testing.T) {
	eng := NewExternalDNSEngine()
	if eng.Name() != "external-dns" {
		t.Errorf("Name: got %q, want external-dns", eng.Name())
	}
	caps := eng.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "external-dns" {
		t.Errorf("AddonTypes: got %v, want [external-dns]", caps.AddonTypes)
	}
	if len(caps.Families) != 2 {
		t.Errorf("Families: got %v, want [system_health crd_health]", caps.Families)
	}
	if len(eng.RBACRules()) == 0 {
		t.Error("RBACRules: got none, want the declared read-only grants")
	}
}
