/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
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

var argocdDeployments = []string{"argocd-repo-server", "argocd-server", "argocd-redis"}

var argocdCRDs = []string{
	"applications.argoproj.io",
	"applicationsets.argoproj.io",
	"appprojects.argoproj.io",
}

// argoApp builds an argoproj.io/v1alpha1 Application whose status.sync.status
// and status.health.status carry the given values ("" leaves a field unset).
func argoApp(namespace, name, syncStatus, healthStatus string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
	}}
	if syncStatus != "" {
		_ = unstructured.SetNestedField(obj.Object, syncStatus, "status", "sync", "status")
	}
	if healthStatus != "" {
		_ = unstructured.SetNestedField(obj.Object, healthStatus, "status", "health", "status")
	}
	return obj
}

func argocdHealthyObjects() []clientObject {
	objs := []clientObject{
		statefulSet("argocd-application-controller", "argocd", 1, 1),
		podInNamespace("argocd-application-controller-0", "argocd-application-controller", "argocd"),
	}
	for _, d := range argocdDeployments {
		objs = append(objs, deploymentInNamespace(d, "argocd"), podInNamespace(d+"-abc", d, "argocd"))
	}
	for _, c := range argocdCRDs {
		objs = append(objs, establishedCRD(c, "v1alpha1", true, true))
	}
	return objs
}

func runArgoCD(t *testing.T, objs ...clientObject) adapter.Result {
	t.Helper()
	result, err := NewArgoCDEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "argocd"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return result
}

func TestArgoCD_HealthyWithSyncedApplication(t *testing.T) {
	result := runArgoCD(t, append(argocdHealthyObjects(),
		argoApp("argocd", "web", "Synced", "Healthy"))...)

	assertHasOutcome(t, result.Checks, "StatefulSet", "argocd-application-controller", adapter.OutcomePass, "fully ready")
	assertFamily(t, result.Checks, "StatefulSet", "argocd-application-controller", adapter.Family("system_health"))
	for _, d := range argocdDeployments {
		assertHasOutcome(t, result.Checks, "Deployment", d, adapter.OutcomePass, "available")
		assertFamily(t, result.Checks, "Deployment", d, adapter.Family("system_health"))
	}
	for _, c := range argocdCRDs {
		assertHasOutcome(t, result.Checks, "CustomResourceDefinition", c, adapter.OutcomePass, "established")
		assertFamily(t, result.Checks, "CustomResourceDefinition", c, adapter.Family("system_health"))
	}
	// One Pass per FieldCheck: the sync score and the health score are distinct
	// results on the same Application.
	assertHasOutcome(t, result.Checks, "Application", "web", adapter.OutcomePass, "status.sync.status is Synced")
	assertHasOutcome(t, result.Checks, "Application", "web", adapter.OutcomePass, "status.health.status is Healthy")
	assertFamily(t, result.Checks, "Application", "web", adapter.Family("sync_health"))
}

// TestArgoCD_ApplicationStateRollup pins the issue-191 rollup: Degraded and
// Missing are Fails, OutOfSync / Unknown / Progressing / Suspended are Warns,
// and an unreconciled Application (no status yet) is surfaced as a Warn.
func TestArgoCD_ApplicationStateRollup(t *testing.T) {
	cases := []struct {
		name        string
		sync        string
		health      string
		wantOutcome adapter.Outcome
		wantSummary string
	}{
		{name: "degraded fails", sync: "Synced", health: "Degraded", wantOutcome: adapter.OutcomeFail, wantSummary: `status.health.status is "Degraded"`},
		{name: "missing fails", sync: "Synced", health: "Missing", wantOutcome: adapter.OutcomeFail, wantSummary: `status.health.status is "Missing"`},
		{name: "out of sync warns", sync: "OutOfSync", health: "Healthy", wantOutcome: adapter.OutcomeWarn, wantSummary: `status.sync.status is "OutOfSync"`},
		{name: "sync unknown warns", sync: "Unknown", health: "Healthy", wantOutcome: adapter.OutcomeWarn, wantSummary: `status.sync.status is "Unknown"`},
		{name: "health unknown warns", sync: "Synced", health: "Unknown", wantOutcome: adapter.OutcomeWarn, wantSummary: `status.health.status is "Unknown"`},
		{name: "progressing warns", sync: "Synced", health: "Progressing", wantOutcome: adapter.OutcomeWarn, wantSummary: `status.health.status is "Progressing"`},
		{name: "suspended warns", sync: "Synced", health: "Suspended", wantOutcome: adapter.OutcomeWarn, wantSummary: `status.health.status is "Suspended"`},
		{name: "future state warns not fails", sync: "Synced", health: "Hovering", wantOutcome: adapter.OutcomeWarn, wantSummary: `status.health.status is "Hovering"`},
		{name: "no status yet warns", sync: "", health: "", wantOutcome: adapter.OutcomeWarn, wantSummary: "is not set"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := runArgoCD(t, append(argocdHealthyObjects(),
				argoApp("argocd", "app", tc.sync, tc.health))...)
			assertHasOutcome(t, result.Checks, "Application", "app", tc.wantOutcome, tc.wantSummary)
		})
	}
}

func TestArgoCD_NoApplicationsSkipped(t *testing.T) {
	// A cluster with Argo CD installed but no Applications is quiet by design:
	// both FieldChecks emit their list-level Skipped under distinct ListNames.
	result := runArgoCD(t, argocdHealthyObjects()...)
	assertHasOutcome(t, result.Checks, "Application", "applications-sync", adapter.OutcomeSkipped, "no Application objects matched")
	assertHasDetail(t, result.Checks, "Application", "applications-sync", "skipReason", "NoMatchingObjects")
	assertHasOutcome(t, result.Checks, "Application", "applications-health", adapter.OutcomeSkipped, "no Application objects matched")
}

func TestArgoCD_AbsentClusterFails(t *testing.T) {
	// Argo CD is a Required addon: pointing an AddonCheck at a cluster that does
	// not run it is a Fail (with the absent marker), never a silent Skip.
	result := runArgoCD(t)
	assertHasOutcome(t, result.Checks, "StatefulSet", "argocd-application-controller", adapter.OutcomeFail, "not found")
	assertHasDetail(t, result.Checks, "StatefulSet", "argocd-application-controller", adapter.DetailAbsent, "true")
	for _, d := range argocdDeployments {
		assertHasOutcome(t, result.Checks, "Deployment", d, adapter.OutcomeFail, "not found")
	}
	for _, c := range argocdCRDs {
		assertHasOutcome(t, result.Checks, "CustomResourceDefinition", c, adapter.OutcomeFail, "not found")
	}
}

func TestArgoCD_UnavailableRepoServerFails(t *testing.T) {
	repo := deploymentInNamespace("argocd-repo-server", "argocd")
	repo.Status.AvailableReplicas = 0
	repo.Status.Conditions = nil
	objs := []clientObject{
		statefulSet("argocd-application-controller", "argocd", 1, 1),
		podInNamespace("argocd-application-controller-0", "argocd-application-controller", "argocd"),
		repo,
		deploymentInNamespace("argocd-server", "argocd"),
		deploymentInNamespace("argocd-redis", "argocd"),
	}
	for _, c := range argocdCRDs {
		objs = append(objs, establishedCRD(c, "v1alpha1", true, true))
	}
	result := runArgoCD(t, objs...)
	assertHasOutcome(t, result.Checks, "Deployment", "argocd-repo-server", adapter.OutcomeFail, "")
	// The healthy siblings still pass alongside the failure.
	assertHasOutcome(t, result.Checks, "Deployment", "argocd-server", adapter.OutcomePass, "available")
}

func TestArgoCD_PolicyOverridesWorkloadNames(t *testing.T) {
	const ns = "gitops"
	result, err := NewArgoCDEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t,
			statefulSet("argo-controller", ns, 1, 1),
			deploymentInNamespace("argo-repo", ns),
			deploymentInNamespace("argo-server", ns),
			deploymentInNamespace("argo-redis", ns),
		),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			"system_health": {Enabled: true, Namespaces: []string{ns}, Thresholds: map[string]string{
				"applicationControllerName": "argo-controller",
				"repoServerName":            "argo-repo",
				"serverName":                "argo-server",
				"redisName":                 "argo-redis",
			}},
			"sync_health": {Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "StatefulSet", "argo-controller", adapter.OutcomePass, "fully ready")
	assertHasOutcome(t, result.Checks, "Deployment", "argo-repo", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "Deployment", "argo-server", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "Deployment", "argo-redis", adapter.OutcomePass, "available")
}
