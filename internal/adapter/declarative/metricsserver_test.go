/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/skaphos/fathom/pkg/adapter"
)

const metricsAPIService = "v1beta1.metrics.k8s.io"

// apiService builds an apiregistration.k8s.io/v1 APIService with an optional
// status.conditions[condType]=status entry (condType == "" leaves the object
// with no conditions).
func apiService(name, condType, status string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1",
		"kind":       "APIService",
		"metadata":   map[string]any{"name": name},
	}}
	if condType != "" {
		_ = unstructured.SetNestedSlice(obj.Object,
			[]any{map[string]any{"type": condType, "status": status}},
			"status", "conditions")
	}
	return obj
}

// msHealthyObjects returns a fully-healthy metrics-server install: the
// Deployment (with a ready pod) in kube-system and the Available aggregated
// APIService. Index order: [0]=Deployment, [1]=pod, [2]=APIService.
func msHealthyObjects() []clientObject {
	return []clientObject{
		deploymentInNamespace("metrics-server", "kube-system"),
		podInNamespace("metrics-server-abc", "metrics-server", "kube-system"),
		apiService(metricsAPIService, "Available", "True"),
	}
}

func TestMetricsServer_HealthyPassesAllFamilies(t *testing.T) {
	result, err := NewMetricsServerEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, msHealthyObjects()...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "metrics-server"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertHasOutcome(t, result.Checks, "Deployment", "metrics-server", adapter.OutcomePass, "available")
	assertFamily(t, result.Checks, "Deployment", "metrics-server", adapter.Family("system_health"))
	assertHasOutcome(t, result.Checks, "APIService", metricsAPIService, adapter.OutcomePass, "Available")
	assertFamily(t, result.Checks, "APIService", metricsAPIService, adapter.Family("api_availability"))
}

func TestMetricsServer_UnavailableAPIServiceFails(t *testing.T) {
	// The load-bearing case: pods Ready but aggregation broken. The APIService
	// exists with Available=False and must Fail while system_health stays green.
	objs := msHealthyObjects()[:2]
	objs = append(objs, apiService(metricsAPIService, "Available", "False"))
	result, err := NewMetricsServerEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "APIService", metricsAPIService, adapter.OutcomeFail, "want")
	assertHasOutcome(t, result.Checks, "Deployment", "metrics-server", adapter.OutcomePass, "available")
}

func TestMetricsServer_MissingAPIServiceFails(t *testing.T) {
	// Required posture in named mode: a missing APIService object is Fail with
	// the absent marker, not a NoMatchingObjects skip.
	result, err := NewMetricsServerEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, msHealthyObjects()[:2]...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{"api_availability": {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "APIService", metricsAPIService, adapter.OutcomeFail, "not found")
	assertHasDetail(t, result.Checks, "APIService", metricsAPIService, adapter.DetailAbsent, "true")
}

func TestMetricsServer_MissingDeploymentFails(t *testing.T) {
	result, err := NewMetricsServerEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, msHealthyObjects()[2:]...),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{"system_health": {Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "metrics-server", adapter.OutcomeFail, "not found")
	assertHasDetail(t, result.Checks, "Deployment", "metrics-server", adapter.DetailAbsent, "true")
}

func TestMetricsServer_AdapterMetadata(t *testing.T) {
	eng := NewMetricsServerEngine()
	if eng.Name() != "metrics-server" {
		t.Errorf("Name: got %q, want metrics-server", eng.Name())
	}
	caps := eng.Capabilities()
	if len(caps.AddonTypes) != 1 || caps.AddonTypes[0] != "metrics-server" {
		t.Errorf("AddonTypes: got %v, want [metrics-server]", caps.AddonTypes)
	}
	if len(caps.Families) != 2 {
		t.Errorf("Families: got %v, want [system_health api_availability]", caps.Families)
	}
	if len(eng.RBACRules()) == 0 {
		t.Error("RBACRules: got none, want the declared read-only grants")
	}
}
