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

var esoDeployments = []string{"external-secrets", "external-secrets-webhook", "external-secrets-cert-controller"}

var esoCRDs = []string{
	"externalsecrets.external-secrets.io",
	"secretstores.external-secrets.io",
	"clustersecretstores.external-secrets.io",
	"clusterexternalsecrets.external-secrets.io",
}

// esoHealthyObjects returns a fully-healthy ESO install: the three controller
// Deployments (each with a ready pod) in the external-secrets namespace and the
// four established, v1-serving CRDs. Index order: [0,1]=external-secrets dep+pod.
func esoHealthyObjects() []clientObject {
	objs := []clientObject{}
	for _, d := range esoDeployments {
		objs = append(objs, deploymentInNamespace(d, "external-secrets"), podInNamespace(d+"-abc", d, "external-secrets"))
	}
	for _, c := range esoCRDs {
		objs = append(objs, establishedCRD(c, "v1", true, true))
	}
	return objs
}

func TestExternalSecrets_HealthyAndEmptySyncSkipped(t *testing.T) {
	result, err := NewExternalSecretsEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, esoHealthyObjects()...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "external-secrets"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, d := range esoDeployments {
		assertHasOutcome(t, result.Checks, "Deployment", d, adapter.OutcomePass, "available")
		assertFamily(t, result.Checks, "Deployment", d, adapter.Family("system_health"))
	}
	for _, c := range esoCRDs {
		assertHasOutcome(t, result.Checks, "CustomResourceDefinition", c, adapter.OutcomePass, "established")
	}

	// No ExternalSecret resources exist -> secret_sync is Skipped (the e2e's
	// empty-cluster contract).
	assertHasOutcome(t, result.Checks, "ExternalSecret", "externalsecrets", adapter.OutcomeSkipped, "no ExternalSecret objects matched")
	assertFamily(t, result.Checks, "ExternalSecret", "externalsecrets", adapter.Family("secret_sync"))
}

func TestExternalSecrets_MissingDeploymentFails(t *testing.T) {
	// Required posture: an absent ESO Deployment is Fail (not Skipped), unlike
	// Cilium's Optional workloads. Drop the external-secrets Deployment + its pod.
	objs := esoHealthyObjects()[2:]
	result, err := NewExternalSecretsEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{"system_health": {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "external-secrets", adapter.OutcomeFail, "not found")
	assertHasDetail(t, result.Checks, "Deployment", "external-secrets", adapter.DetailAbsent, "true")
}
