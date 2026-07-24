/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/skaphos/fathom/pkg/adapter"
)

// testProjection is the azure-workload-identity-shaped check the tests run.
func testProjection() PodProjectionCheck {
	return PodProjectionCheck{
		Selector:   map[string]string{"azure.workload.identity/use": "true"},
		ListName:   "workload-pods",
		Component:  "azure-wi-webhook",
		VolumeName: "azure-identity-token",
		EnvVar:     "AZURE_FEDERATED_TOKEN_FILE",
	}
}

// optedInPod builds a Ready pod carrying the opt-in label. injectVolume /
// injectEnv toggle the webhook's two injection artifacts independently.
func optedInPod(name, namespace string, injectVolume, injectEnv bool) *corev1.Pod {
	container := corev1.Container{Name: "app", Image: "example.com/app:1"}
	if injectEnv {
		container.Env = []corev1.EnvVar{{Name: "AZURE_FEDERATED_TOKEN_FILE", Value: "/var/run/secrets/azure/tokens/azure-identity-token"}}
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"azure.workload.identity/use": "true"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{container}},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
	if injectVolume {
		pod.Spec.Volumes = []corev1.Volume{{
			Name: "azure-identity-token",
			VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{
				Sources: []corev1.VolumeProjection{{
					ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
						Audience: "api://AzureADTokenExchange",
						Path:     "azure-identity-token",
					},
				}},
			}},
		}}
	}
	return pod
}

// runProjection runs an engine whose single enabled family carries pc as its
// only PodProjectionCheck, against objs.
func runProjection(t *testing.T, pc PodProjectionCheck, policy map[adapter.Family]adapter.FamilyPolicy, objs ...clientObject) []adapter.CheckResult {
	t.Helper()
	eng := MustEngine(AddonDefinition{
		AddonType:      "projectiontest",
		AdapterVersion: "0.0.1",
		Families: []FamilyDefinition{{
			Name:           "projection_sanity",
			DefaultEnabled: true,
			PodProjections: []PodProjectionCheck{pc},
		}},
	})
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

func TestPodProjection_AllInjectedPasses(t *testing.T) {
	checks := runProjection(t, testProjection(), nil,
		optedInPod("app-1", "team-a", true, true),
		optedInPod("app-2", "team-b", true, true),
	)
	assertHasOutcome(t, checks, "Pod", "workload-pods", adapter.OutcomePass, "carry the injected azure-identity-token projection")
	assertHasDetail(t, checks, "Pod", "workload-pods", "livePods", "2")
	assertHasDetail(t, checks, "Pod", "workload-pods", "selector", "azure.workload.identity/use=true")
	assertFamily(t, checks, "Pod", "workload-pods", adapter.Family("projection_sanity"))
}

func TestPodProjection_MissingVolumeFails(t *testing.T) {
	checks := runProjection(t, testProjection(), nil,
		optedInPod("injected", "team-a", true, true),
		optedInPod("uninjected", "team-a", false, false),
	)
	assertHasOutcome(t, checks, "Pod", "workload-pods", adapter.OutcomeFail, "missing the injected azure-identity-token projection")
	assertHasDetail(t, checks, "Pod", "workload-pods", "uninjectedCount", "1")
	assertHasDetail(t, checks, "Pod", "workload-pods", "uninjectedPods", "team-a/uninjected")
}

func TestPodProjection_MissingEnvOnlyFails(t *testing.T) {
	// The volume is present but a container lacks the env var: still not the
	// webhook's full injection, so the pod counts as uninjected.
	checks := runProjection(t, testProjection(), nil, optedInPod("half", "team-a", true, false))
	assertHasOutcome(t, checks, "Pod", "workload-pods", adapter.OutcomeFail, "1 of 1 opted-in pods")
}

func TestPodProjection_NoOptedInPodsSkipped(t *testing.T) {
	// A pod without the opt-in label is invisible to the check.
	unlabeled := podInNamespace("bystander", "bystander", "team-a")
	checks := runProjection(t, testProjection(), nil, unlabeled)
	assertHasOutcome(t, checks, "Pod", "workload-pods", adapter.OutcomeSkipped, "no pods carry the")
	assertHasDetail(t, checks, "Pod", "workload-pods", "skipReason", "NoMatchingObjects")
}

func TestPodProjection_InactivePodsSkipped(t *testing.T) {
	done := optedInPod("finished", "team-a", false, false)
	done.Status.Phase = corev1.PodSucceeded
	checks := runProjection(t, testProjection(), nil, done)
	assertHasOutcome(t, checks, "Pod", "workload-pods", adapter.OutcomeSkipped, "terminating, failed, or completed")
}

func TestPodProjection_PolicyNamespacesScopeTheScan(t *testing.T) {
	policy := map[adapter.Family]adapter.FamilyPolicy{
		"projection_sanity": {Enabled: true, Namespaces: []string{"team-a"}},
	}
	checks := runProjection(t, testProjection(), policy,
		optedInPod("in-scope", "team-a", true, true),
		optedInPod("out-of-scope-uninjected", "team-b", false, false),
	)
	// The uninjected pod lives outside the scanned namespace, so the scan passes.
	assertHasOutcome(t, checks, "Pod", "workload-pods", adapter.OutcomePass, "all 1 opted-in pods")
}

func TestPodProjection_CapNames(t *testing.T) {
	if got := capNames([]string{"a", "b"}, 5); got != "a,b" {
		t.Fatalf("capNames under limit: got %q", got)
	}
	if got := capNames([]string{"a", "b", "c"}, 2); got != "a,b,+1 more" {
		t.Fatalf("capNames over limit: got %q", got)
	}
}

func TestNewEngine_PodProjectionValidation(t *testing.T) {
	base := func(pc PodProjectionCheck) AddonDefinition {
		return AddonDefinition{
			AddonType:      "projectiontest",
			AdapterVersion: "0.0.1",
			Families: []FamilyDefinition{{
				Name:           "projection_sanity",
				DefaultEnabled: true,
				PodProjections: []PodProjectionCheck{pc},
			}},
		}
	}
	cases := []struct {
		name    string
		check   PodProjectionCheck
		wantErr string
	}{
		{
			name:    "empty selector",
			check:   PodProjectionCheck{VolumeName: "azure-identity-token"},
			wantErr: "empty Selector",
		},
		{
			name:    "no volume name",
			check:   PodProjectionCheck{Selector: map[string]string{"use": "true"}},
			wantErr: "no VolumeName",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEngine(base(tc.check))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("NewEngine: got %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}
