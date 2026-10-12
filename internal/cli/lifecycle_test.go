/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func managerDeployment(ns string, labels map[string]string, ready bool) *appsv1.Deployment {
	l := map[string]string{operatorLabelKey: operatorLabelValue, nameLabel: "fathom"}
	for k, v := range labels {
		l[k] = v
	}
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "fathom-controller-manager", Namespace: ns, Labels: l},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: managerContainerName, Image: operatorRepository + ":v0.5.1",
		}}}}},
		Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: status}}},
	}
}

func TestDetectInstall(t *testing.T) {
	cases := []struct {
		name   string
		objs   []client.Object
		seed   []helmRelease
		want   managedBy
		reason string
	}{
		{name: "nothing installed", want: managedByNone},
		{
			name: "helm release",
			seed: []helmRelease{{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.5.1"}},
			want: managedByHelm,
		},
		{
			name: "olm",
			objs: []client.Object{managerDeployment("operators", map[string]string{"olm.owner": "fathom.v0.5.1", "olm.owner.kind": "ClusterServiceVersion"}, true)},
			want: managedByOLM,
		},
		{
			name: "kustomize",
			objs: []client.Object{managerDeployment("fathom-system", map[string]string{managedByKey: "kustomize"}, true)},
			want: managedByKustomize,
		},
		{
			name: "unmanaged",
			objs: []client.Object{managerDeployment("fathom-system", nil, true)},
			want: managedByUnmanaged,
		},
		{
			name: "two releases",
			seed: []helmRelease{
				{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.5.1"},
				{Name: "fathom", Namespace: "fathom-b", ChartVersion: "0.5.1"},
			},
			reason: reasonMultipleInstalls,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newLifecycleEnv(t, tc.objs...)
			for _, r := range tc.seed {
				env.rc.seed(r, true)
			}
			in, err := detectInstall(context.Background(), env.c, env.rc)
			if tc.reason != "" {
				var r refusal
				if !errors.As(err, &r) || r.Reason != tc.reason {
					t.Fatalf("err = %v, want refusal %s", err, tc.reason)
				}
				if !strings.Contains(r.Detail, "fathom-b/fathom") || !strings.Contains(r.Detail, "fathom-system/fathom") {
					t.Errorf("refusal does not name each release: %s", r.Detail)
				}
				return
			}
			if err != nil {
				t.Fatalf("detectInstall: %v", err)
			}
			if in.ManagedBy != tc.want {
				t.Errorf("managedBy = %s, want %s", in.ManagedBy, tc.want)
			}
		})
	}
}

func TestDetectInstallReadsTheRelease(t *testing.T) {
	env := newLifecycleEnv(t)
	env.rc.seed(helmRelease{
		Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.5.1", Revision: 3,
		Labels: map[string]string{installedByLabel: installedByCLI},
		Config: map[string]any{"image": map[string]any{"repository": operatorRepository, "digest": fakeOperatorDigest}},
	}, false)
	in, err := detectInstall(context.Background(), env.c, env.rc)
	if err != nil {
		t.Fatal(err)
	}
	if in.Release != "fathom" || in.Namespace != "fathom-system" || in.Revision != 3 || in.Version != "0.5.1" {
		t.Errorf("install = %+v", in)
	}
	if in.InstalledBy != installedByCLI {
		t.Errorf("installedBy = %q, want fathomctl", in.InstalledBy)
	}
	if in.Pinning != pinnedByDigest || !strings.Contains(in.Images["operator"], "@"+fakeOperatorDigest) {
		t.Errorf("images = %v pinning = %s", in.Images, in.Pinning)
	}
	if in.Ready || !strings.Contains(in.ReadyReason, "Available=False") {
		t.Errorf("ready = %v (%s), want not Available with the condition named", in.Ready, in.ReadyReason)
	}
}

func TestDetectInstallDefaultsInstalledByToHelm(t *testing.T) {
	env := newLifecycleEnv(t)
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.5.1"}, true)
	in, err := detectInstall(context.Background(), env.c, env.rc)
	if err != nil {
		t.Fatal(err)
	}
	if in.InstalledBy != "helm" {
		t.Errorf("installedBy = %q, want helm", in.InstalledBy)
	}
}

// Release records are read only in namespaces that hold a manager or were
// named: status must not need cluster-wide Secret access (research R6).
func TestDetectInstallScopesReleaseLookups(t *testing.T) {
	env := newLifecycleEnv(t, managerDeployment("fathom-system", map[string]string{managedByKey: "kustomize"}, true))
	var asked []string
	rc := &recordingLister{fakeReleaseClient: env.rc, asked: &asked}
	if _, err := detectInstall(context.Background(), env.c, rc, "team-a"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, ",") != "fathom-system,team-a" {
		t.Errorf("releases listed in %v, want exactly [fathom-system team-a]", asked)
	}
	for _, ns := range asked {
		if ns == "" {
			t.Error("releases listed across all namespaces")
		}
	}
}

type recordingLister struct {
	*fakeReleaseClient
	asked *[]string
}

func (r *recordingLister) List(ctx context.Context, namespaces []string) ([]helmRelease, error) {
	*r.asked = append(*r.asked, namespaces...)
	return r.fakeReleaseClient.List(ctx, namespaces)
}

func TestPreflightPermissionsNamesEachMissingPermission(t *testing.T) {
	env := newLifecycleEnv(t)
	env.denied["create customresourcedefinitions"] = true
	env.denied["create secrets"] = true
	err := preflightPermissions(context.Background(), env.f, env.c, lifecyclePermissions("install", "fathom-system", permissionScope{}))
	var r refusal
	if !errors.As(err, &r) || r.Reason != reasonMissingPermissions {
		t.Fatalf("err = %v, want missing-permissions", err)
	}
	for _, want := range []string{"create customresourcedefinitions.apiextensions.k8s.io (cluster-wide)", "create secrets in namespace fathom-system"} {
		if !strings.Contains(r.Detail, want) {
			t.Errorf("refusal %q does not name %q", r.Detail, want)
		}
	}
	if len(env.clusterWrites()) != 0 {
		t.Errorf("preflight wrote %v", env.clusterWrites())
	}
}

// The flags in use widen the preflight to what they will do, so a passing
// preflight never fails part-way.
func TestLifecyclePermissionsFollowTheFlags(t *testing.T) {
	has := func(perms []permission, verb, resource string) bool {
		for _, p := range perms {
			if p.Verb == verb && p.Resource == resource {
				return true
			}
		}
		return false
	}
	if has(lifecyclePermissions("install", "ns", permissionScope{}), "create", "namespaces") {
		t.Error("install without --create-namespace checks namespace creation")
	}
	if !has(lifecyclePermissions("install", "ns", permissionScope{createNamespace: true}), "create", "namespaces") {
		t.Error("install --create-namespace does not check namespace creation")
	}
	plain := lifecyclePermissions("uninstall", "ns", permissionScope{})
	for _, r := range []string{"leases", "roles", "rolebindings", "networkpolicies", "configmaps", "serviceaccounts", "validatingadmissionpolicybindings"} {
		if !has(plain, "delete", r) {
			t.Errorf("uninstall does not check delete %s", r)
		}
	}
	if has(plain, "delete", "customresourcedefinitions") || has(plain, "delete", "namespaces") {
		t.Error("plain uninstall checks purge or namespace deletion")
	}
	full := lifecyclePermissions("uninstall", "ns", permissionScope{purgeCRDs: true, deleteNamespace: true})
	for _, r := range []string{"customresourcedefinitions", "healthreports", "namespaces"} {
		if !has(full, "delete", r) {
			t.Errorf("uninstall --purge-crds --delete-namespace does not check delete %s", r)
		}
	}
}

func TestWaitManagerAvailableNamesTheUnmetCondition(t *testing.T) {
	env := newLifecycleEnv(t)
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0"}, false)
	err := waitManagerAvailable(context.Background(), env.f, env.c, "fathom-system", "fathom", 20*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "not Available within 20ms") || !strings.Contains(err.Error(), "Available=False") {
		t.Errorf("err = %v, want a bounded wait naming Available=False", err)
	}
}

// The Available condition is not enough: during a surge the old pod keeps
// the Deployment Available while the new one fails (adversarial review F2).
func TestRolloutComplete(t *testing.T) {
	avail := func(s corev1.ConditionStatus) appsv1.DeploymentCondition {
		return appsv1.DeploymentCondition{Type: appsv1.DeploymentAvailable, Status: s}
	}
	cases := []struct {
		name                         string
		gen, observed                int64
		replicas, updated, available int32
		conds                        []appsv1.DeploymentCondition
		done, failed                 bool
		reason                       string
	}{
		{name: "complete", gen: 2, observed: 2, replicas: 1, updated: 1, available: 1, conds: []appsv1.DeploymentCondition{avail(corev1.ConditionTrue)}, done: true},
		{name: "not observed", gen: 3, observed: 2, replicas: 1, updated: 1, available: 1, conds: []appsv1.DeploymentCondition{avail(corev1.ConditionTrue)}, reason: "not yet observed"},
		{name: "surge, new pod not ready", gen: 2, observed: 2, replicas: 2, updated: 1, available: 1, conds: []appsv1.DeploymentCondition{avail(corev1.ConditionTrue)}, reason: "old replica"},
		{name: "updated but unavailable", gen: 2, observed: 2, replicas: 1, updated: 1, available: 0, conds: []appsv1.DeploymentCondition{avail(corev1.ConditionTrue)}, reason: "updated replicas available"},
		{name: "not updated", gen: 2, observed: 2, replicas: 1, updated: 0, available: 1, conds: []appsv1.DeploymentCondition{avail(corev1.ConditionTrue)}, reason: "replicas updated"},
		{name: "not available", gen: 2, observed: 2, replicas: 1, updated: 1, available: 0, conds: []appsv1.DeploymentCondition{avail(corev1.ConditionFalse)}, reason: "Available=False"},
		{
			name: "progress deadline exceeded", gen: 2, observed: 2, replicas: 2, updated: 1, available: 1,
			conds: []appsv1.DeploymentCondition{avail(corev1.ConditionTrue), {
				Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded", Message: "ReplicaSet has timed out progressing.",
			}},
			failed: true, reason: "progress deadline",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := managerDeployment("fathom-system", nil, true)
			d.Generation = tc.gen
			d.Status = appsv1.DeploymentStatus{ObservedGeneration: tc.observed, Replicas: tc.replicas,
				UpdatedReplicas: tc.updated, AvailableReplicas: tc.available, Conditions: tc.conds}
			done, reason, failed := rolloutComplete(d)
			if done != tc.done || failed != tc.failed || !strings.Contains(reason, tc.reason) {
				t.Errorf("rolloutComplete = (%v, %q, %v), want (%v, ~%q, %v)", done, reason, failed, tc.done, tc.reason, tc.failed)
			}
		})
	}
}

func TestWaitManagerAvailableSucceeds(t *testing.T) {
	env := newLifecycleEnv(t)
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0"}, true)
	if err := waitManagerAvailable(context.Background(), env.f, env.c, "fathom-system", "fathom", time.Second); err != nil {
		t.Errorf("waitManagerAvailable: %v", err)
	}
}

func TestCLIChartVersionRefusesDevBuilds(t *testing.T) {
	withVersion(t, "devel+abc123", false)
	_, err := cliChartVersion()
	var r refusal
	if !errors.As(err, &r) || r.Reason != reasonDevVersion || !strings.Contains(r.Next, "FATHOMCTL_VERSION=") {
		t.Errorf("err = %v, want dev-version naming FATHOMCTL_VERSION", err)
	}
	withVersion(t, "v0.6.0", false)
	if v, err := cliChartVersion(); err != nil || v != "0.6.0" {
		t.Errorf("cliChartVersion = %q, %v", v, err)
	}
}
