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

const (
	awiNamespace  = "azure-workload-identity-system"
	awiDeployment = "azure-wi-webhook-controller-manager"
	awiWebhook    = "azure-wi-webhook-mutating-webhook-configuration"
	awiService    = "azure-wi-webhook-webhook-service"
)

// healthyAzureWIObjects is a fully-healthy webhook install plus one correctly
// injected opted-in workload pod.
func healthyAzureWIObjects() []clientObject {
	return []clientObject{
		deploymentInNamespace(awiDeployment, awiNamespace),
		podInNamespace(awiDeployment+"-7d9c", awiDeployment, awiNamespace),
		mutatingConfig(awiWebhook, wiredEntry("mutation.azure-workload-identity.io", awiNamespace, awiService, []byte("ca"))),
		endpointSlice(awiService+"-abc", awiNamespace, awiService, boolPtr(true)),
		optedInPod("worker-1", "team-a", true, true),
	}
}

func TestAzureWorkloadIdentity_HealthyClusterAllPass(t *testing.T) {
	result, err := NewAzureWorkloadIdentityEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, healthyAzureWIObjects()...),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", awiDeployment, adapter.OutcomePass, "available")
	assertFamily(t, result.Checks, "Deployment", awiDeployment, adapter.Family("system_health"))
	assertHasOutcome(t, result.Checks, KindMutatingWebhookConfiguration, awiWebhook, adapter.OutcomePass, "wired")
	assertFamily(t, result.Checks, KindMutatingWebhookConfiguration, awiWebhook, adapter.Family("webhook_wiring"))
	assertHasOutcome(t, result.Checks, "EndpointSlice", awiService, adapter.OutcomePass, "ready endpoints")
	assertHasOutcome(t, result.Checks, "Pod", "workload-pods", adapter.OutcomePass, "carry the injected azure-identity-token projection")
	assertFamily(t, result.Checks, "Pod", "workload-pods", adapter.Family("projection_sanity"))
	for _, c := range result.Checks {
		if c.Outcome == adapter.OutcomeFail || c.Outcome == adapter.OutcomeError {
			t.Fatalf("no check should fail on a healthy cluster: %#v", c)
		}
	}
}

func TestAzureWorkloadIdentity_AbsentWebhookFails(t *testing.T) {
	// The webhook Deployment and configuration are gone but an opted-in pod
	// remains from before: the exact silent-failure mode of #185. Every family
	// must flag it — nothing may quietly skip.
	result, err := NewAzureWorkloadIdentityEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, optedInPod("worker-1", "team-a", false, false)),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", awiDeployment, adapter.OutcomeFail, "not found")
	assertHasDetail(t, result.Checks, "Deployment", awiDeployment, adapter.DetailAbsent, "true")
	assertHasOutcome(t, result.Checks, KindMutatingWebhookConfiguration, awiWebhook, adapter.OutcomeFail, "not found")
	assertHasOutcome(t, result.Checks, "Pod", "workload-pods", adapter.OutcomeFail, "missing the injected azure-identity-token projection")
}

func TestAzureWorkloadIdentity_NoOptedInPodsProjectionSkipped(t *testing.T) {
	objs := healthyAzureWIObjects()[:4] // drop the opted-in workload pod
	result, err := NewAzureWorkloadIdentityEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Pod", "workload-pods", adapter.OutcomeSkipped, "no pods carry the")
	assertHasDetail(t, result.Checks, "Pod", "workload-pods", "skipReason", "NoMatchingObjects")
}

func TestAzureWorkloadIdentity_UnpopulatedCABundleFails(t *testing.T) {
	objs := []clientObject{
		deploymentInNamespace(awiDeployment, awiNamespace),
		podInNamespace(awiDeployment+"-7d9c", awiDeployment, awiNamespace),
		mutatingConfig(awiWebhook, wiredEntry("mutation.azure-workload-identity.io", awiNamespace, awiService, nil)),
		endpointSlice(awiService+"-abc", awiNamespace, awiService, boolPtr(true)),
	}
	result, err := NewAzureWorkloadIdentityEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, KindMutatingWebhookConfiguration, awiWebhook, adapter.OutcomeFail, "caBundle")
}

func TestAzureWorkloadIdentity_Capabilities(t *testing.T) {
	caps := NewAzureWorkloadIdentityEngine().Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "azure-workload-identity" {
		t.Fatalf("AddonTypes: %#v", caps.AddonTypes)
	}
	want := []adapter.Family{"system_health", "webhook_wiring", "projection_sanity"}
	if len(caps.Families) != len(want) {
		t.Fatalf("Families: %#v", caps.Families)
	}
	for i, f := range want {
		if caps.Families[i] != f {
			t.Fatalf("Families[%d]: got %s, want %s", i, caps.Families[i], f)
		}
	}
}
