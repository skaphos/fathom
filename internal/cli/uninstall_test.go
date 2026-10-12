/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/internal/nodecert"
)

var managedLabels = map[string]string{nodecert.LabelManagedBy: nodecert.ManagedByValue}

// operatorCreated is one of every kind the operator creates at run time.
func operatorCreated() []client.Object {
	om := func(ns, name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: ns, Labels: managedLabels}
	}
	return []client.Object{
		&appsv1.DaemonSet{ObjectMeta: om("team-a", "fathom-node-agent-certs")},
		&corev1.ServiceAccount{ObjectMeta: om("team-a", "fathom-node-agent-certs")},
		&networkingv1.NetworkPolicy{ObjectMeta: om("team-a", "fathom-node-agent-certs")},
		&rbacv1.Role{ObjectMeta: om("team-a", "fathom-node-report-certs")},
		&rbacv1.RoleBinding{ObjectMeta: om("team-a", "fathom-node-report-certs")},
		&corev1.ConfigMap{ObjectMeta: om("team-a", "fathom-node-report-node-1")},
		&corev1.Pod{ObjectMeta: om("team-a", "fathom-dns-probe-abc")},
		// Addon probes live in kube-system with no owner.
		&corev1.Pod{ObjectMeta: om("kube-system", "fathom-probe-coredns")},
		&admissionregistrationv1.ValidatingAdmissionPolicy{ObjectMeta: metav1.ObjectMeta{Name: reportAuthenticityPolicy, Labels: managedLabels}},
		&admissionregistrationv1.ValidatingAdmissionPolicyBinding{ObjectMeta: metav1.ObjectMeta{Name: reportAuthenticityPolicy, Labels: managedLabels}},
		&coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: defaultLeaderElectionID, Namespace: "fathom-system"}},
	}
}

// installedEnv is a Helm install at 0.6.0 with CRDs, one check, a report and
// every operator-created object.
func installedEnv(t *testing.T, extra ...client.Object) *lifecycleEnv {
	t.Helper()
	objs := append(liveCRDs(t, "0.6.0", nil), namespaceObj("fathom-system"), namespaceObj("team-a"), namespaceObj("kube-system"))
	objs = append(objs, operatorCreated()...)
	objs = append(objs, addonCheck("team-a", "coredns"))
	objs = append(objs, extra...)
	env := newLifecycleEnv(t, objs...)
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0"}, true)
	return env
}

func exists(t *testing.T, c client.Client, obj client.Object) bool {
	t.Helper()
	err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)
	if client.IgnoreNotFound(err) != nil {
		t.Fatal(err)
	}
	return err == nil
}

func TestUninstallRemovesOperatorCreatedObjectsAndKeepsData(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := installedEnv(t)
	before := len(env.log.all())
	out, _, err := execVerb(env.f, "uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	events := env.log.all()[before:]
	if !strings.HasPrefix(events[0], "release:uninstall fathom-system/fathom") {
		t.Errorf("first change = %q, want the release uninstall before any cleanup", events[0])
	}
	for _, obj := range operatorCreated() {
		if exists(t, env.c, obj) {
			t.Errorf("%s survived uninstall", describeObject(obj))
		}
	}
	if !exists(t, env.c, addonCheck("team-a", "coredns")) {
		t.Error("a check resource was deleted")
	}
	if !exists(t, env.c, &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: dnsCRD}}) {
		t.Error("a CRD was deleted")
	}
	if !exists(t, env.c, namespaceObj("fathom-system")) {
		t.Error("the namespace was deleted")
	}
	if !strings.Contains(out, "Kept namespace fathom-system") {
		t.Errorf("output does not report the kept namespace:\n%s", out)
	}
}

func TestUninstallNothingInstalled(t *testing.T) {
	env := newLifecycleEnv(t)
	out, _, err := execVerb(env.f, "uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out, "nothing to remove") {
		t.Errorf("output = %q", out)
	}
}

func TestUninstallRefusals(t *testing.T) {
	cases := map[string]struct {
		env      func(t *testing.T) *lifecycleEnv
		reason   string
		mentions []string
	}{
		"kustomize install": {
			env: func(t *testing.T) *lifecycleEnv {
				return newLifecycleEnv(t, managerDeployment("fathom-system", map[string]string{managedByKey: "kustomize"}, true))
			},
			reason: reasonNotHelm, mentions: []string{"kustomize"},
		},
		"missing delete permissions": {
			env: func(t *testing.T) *lifecycleEnv {
				env := installedEnv(t)
				env.denied["delete daemonsets"] = true
				return env
			},
			reason: reasonMissingPermissions, mentions: []string{"delete daemonsets"},
		},
		"two releases": {
			env: func(t *testing.T) *lifecycleEnv {
				env := installedEnv(t)
				env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-b", ChartVersion: "0.6.0"}, true)
				return env
			},
			reason: reasonMultipleInstalls,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := tc.env(t)
			before := len(env.log.all())
			_, _, err := execVerb(env.f, "uninstall")
			if err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("err = %v, want %s", err, tc.reason)
			}
			for _, m := range tc.mentions {
				if !strings.Contains(err.Error(), m) {
					t.Errorf("error does not mention %q: %v", m, err)
				}
			}
			if w := env.log.all()[before:]; len(w) != 0 {
				t.Errorf("refused uninstall wrote %v", w)
			}
		})
	}
}

// uninstall needs no chart, so a CLI older than the install still runs it.
func TestUninstallWorksWithAnOlderCLI(t *testing.T) {
	withVersion(t, "v0.5.1", true)
	env := installedEnv(t)
	if _, _, err := execVerb(env.f, "uninstall"); err != nil {
		t.Fatalf("uninstall with an older CLI: %v", err)
	}
	withVersion(t, "devel+abc123", false)
	if _, _, err := execVerb(env.f, "uninstall"); err != nil {
		t.Fatalf("uninstall with a development build: %v", err)
	}
}

func TestUninstallProceedsOnAPendingRelease(t *testing.T) {
	env := newLifecycleEnv(t, namespaceObj("fathom-system"))
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0", Status: "pending-upgrade"}, true)
	out, _, err := execVerb(env.f, "uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out, "is pending-upgrade; uninstalling it anyway") {
		t.Errorf("output does not report the state:\n%s", out)
	}
	if _, ok := env.rc.releases["fathom-system/fathom"]; ok {
		t.Error("the pending release was not removed")
	}
}

// An interrupted `helm uninstall` is still a Helm install that uninstall
// can finish, not an unmanaged operator (adversarial review F8).
func TestUninstallFinishesAnInterruptedHelmUninstall(t *testing.T) {
	env := newLifecycleEnv(t, namespaceObj("fathom-system"))
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0", Status: "uninstalling"}, true)
	out, _, err := execVerb(env.f, "uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out, "is uninstalling; uninstalling it anyway") {
		t.Errorf("output = %s", out)
	}
}

// The Lease name follows the release's leaderElectionID value.
func TestUninstallRemovesTheConfiguredLease(t *testing.T) {
	lease := &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: "custom-lease", Namespace: "fathom-system"}}
	env := newLifecycleEnv(t, namespaceObj("fathom-system"), lease)
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0", Config: map[string]any{"leaderElectionID": "custom-lease"}}, true)
	if _, _, err := execVerb(env.f, "uninstall"); err != nil {
		t.Fatal(err)
	}
	if exists(t, env.c, lease) {
		t.Error("the configured Lease survived")
	}
}

// Cleanup waits for the old manager to stop; a manager pod that lingers is
// reported, and nothing else is removed (adversarial review F3).
func TestWaitManagerGone(t *testing.T) {
	lingering := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "fathom-fathom-operator-abc", Namespace: "fathom-system",
		Labels: map[string]string{instanceLabel: "fathom"}}}
	env := newLifecycleEnv(t, lingering)
	err := waitManagerGone(context.Background(), env.f, env.c, "fathom-system", "fathom", 20*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "Pod fathom-system/fathom-fathom-operator-abc") {
		t.Errorf("err = %v, want the lingering pod named", err)
	}
	if err := env.c.Delete(context.Background(), lingering); err != nil {
		t.Fatal(err)
	}
	if err := waitManagerGone(context.Background(), env.f, env.c, "fathom-system", "fathom", time.Second); err != nil {
		t.Errorf("waitManagerGone with no manager: %v", err)
	}
}

func TestUninstallStopsBeforeCleanupWhileTheManagerRuns(t *testing.T) {
	lingering := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "fathom-fathom-operator-abc", Namespace: "fathom-system",
		Labels: map[string]string{instanceLabel: "fathom"}}}
	env := installedEnv(t, lingering)
	_, _, err := execVerb(env.f, "uninstall", "--timeout", "30ms")
	if err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("err = %v, want the running manager reported", err)
	}
	for _, obj := range operatorCreated() {
		if !exists(t, env.c, obj) {
			t.Errorf("%s was removed while the manager still ran", describeObject(obj))
		}
	}
}

// A rerun after an uninstall that stopped part-way finishes the cleanup
// instead of reporting nothing to remove (adversarial re-review N1).
func TestUninstallRerunCleansUpLeftovers(t *testing.T) {
	leftovers := operatorCreated()
	env := newLifecycleEnv(t, append(leftovers, namespaceObj("team-a"), namespaceObj("kube-system"))...)
	out, _, err := execVerb(env.f, "uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out, "remain from an earlier uninstall") {
		t.Errorf("output = %s", out)
	}
	for _, obj := range operatorCreated() {
		// The Lease lives in the release namespace, unknown without a release.
		if _, isLease := obj.(*coordinationv1.Lease); isLease {
			continue
		}
		if exists(t, env.c, obj) {
			t.Errorf("%s survived the rerun", describeObject(obj))
		}
	}
}

func TestUninstallExportRequiresPurge(t *testing.T) {
	env := newLifecycleEnv(t)
	_, _, err := execVerb(env.f, "uninstall", "--export", "x.yaml")
	if err == nil || !strings.Contains(err.Error(), "--export only applies with --purge-crds") {
		t.Errorf("err = %v", err)
	}
}

// namespacedKindsFor returns a fixed discovery result for inventory tests.
func namespacedKindsFor(kinds ...schema.GroupVersionKind) func(*factory) ([]schema.GroupVersionKind, error) {
	return func(*factory) ([]schema.GroupVersionKind, error) { return kinds, nil }
}
