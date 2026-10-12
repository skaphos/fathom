/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var inventoryKinds = namespacedKindsFor(
	corev1.SchemeGroupVersion.WithKind("ConfigMap"),
	corev1.SchemeGroupVersion.WithKind("ServiceAccount"),
	corev1.SchemeGroupVersion.WithKind("Secret"),
	corev1.SchemeGroupVersion.WithKind("Event"),
)

// helmObject is an object the release itself installed.
func helmObject(obj client.Object) client.Object {
	obj.SetAnnotations(map[string]string{"meta.helm.sh/release-name": "fathom"})
	return obj
}

func nsEnv(t *testing.T, extra ...client.Object) *lifecycleEnv {
	t.Helper()
	objs := append([]client.Object{
		namespaceObj("fathom-system"),
		// Automatic objects Kubernetes puts in every namespace.
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "fathom-system"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "kube-root-ca.crt", Namespace: "fathom-system"}},
		// The release's own objects and record.
		helmObject(&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "fathom-fathom-operator", Namespace: "fathom-system"}}),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sh.helm.release.v1.fathom.v1", Namespace: "fathom-system", Labels: map[string]string{"owner": "helm", "name": "fathom"}}},
	}, extra...)
	env := newLifecycleEnv(t, objs...)
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0"}, true)
	env.f.namespacedKinds = inventoryKinds
	return env
}

func TestDeleteNamespaceWithOnlyFathomObjects(t *testing.T) {
	env := nsEnv(t)
	out, _, err := execVerb(env.f, "uninstall", "--delete-namespace")
	if err != nil {
		t.Fatalf("uninstall --delete-namespace: %v", err)
	}
	if !strings.Contains(out, "Deleted namespace fathom-system") {
		t.Errorf("output = %q", out)
	}
	if exists(t, env.c, namespaceObj("fathom-system")) {
		t.Error("namespace survived")
	}
}

func TestDeleteNamespaceWithForeignObjectsNeedsConfirmation(t *testing.T) {
	foreign := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "team-settings", Namespace: "fathom-system"}}

	t.Run("no terminal and no --yes", func(t *testing.T) {
		env := nsEnv(t, foreign)
		before := len(env.log.all())
		_, _, err := execVerb(env.f, "uninstall", "--delete-namespace")
		if err == nil || !strings.Contains(err.Error(), "ConfigMap team-settings") || !strings.Contains(err.Error(), "--yes") {
			t.Fatalf("err = %v, want the foreign object listed and --yes required", err)
		}
		// Automatic objects are reported, not treated as foreign.
		if !strings.Contains(err.Error(), "ServiceAccount default") || strings.Contains(err.Error(), "  ServiceAccount default\n") {
			t.Errorf("automatic objects mishandled: %v", err)
		}
		if w := env.log.all()[before:]; len(w) != 0 {
			t.Errorf("unconfirmed uninstall changed the cluster, release included: %v", w)
		}
	})

	t.Run("declined", func(t *testing.T) {
		env := nsEnv(t, foreign)
		env.f.isTerminal = func() bool { return true }
		env.f.stdin = strings.NewReader("no\n")
		before := len(env.log.all())
		if _, _, err := execVerb(env.f, "uninstall", "--delete-namespace"); err == nil {
			t.Fatal("declined uninstall succeeded")
		}
		if w := env.log.all()[before:]; len(w) != 0 {
			t.Errorf("declined uninstall changed the cluster: %v", w)
		}
		if _, ok := env.rc.releases["fathom-system/fathom"]; !ok {
			t.Error("declined uninstall removed the release")
		}
	})

	t.Run("--yes", func(t *testing.T) {
		env := nsEnv(t, foreign)
		if _, _, err := execVerb(env.f, "uninstall", "--delete-namespace", "--yes"); err != nil {
			t.Fatalf("uninstall --delete-namespace --yes: %v", err)
		}
		if exists(t, env.c, namespaceObj("fathom-system")) {
			t.Error("namespace survived")
		}
	})
}

// --purge-crds and --delete-namespace together ask exactly once.
func TestPurgeAndDeleteNamespaceAskOnce(t *testing.T) {
	foreign := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "team-settings", Namespace: "fathom-system"}}
	env := nsEnv(t, append(liveCRDs(t, "0.6.0", nil), foreign)...)
	env.f.isTerminal = func() bool { return true }
	env.f.stdin = strings.NewReader("y\n")
	_, errOut, err := execVerb(env.f, "uninstall", "--purge-crds", "--delete-namespace")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if n := strings.Count(errOut, "Continue? [y/N]"); n != 1 {
		t.Errorf("asked %d times, want once:\n%s", n, errOut)
	}
	if !strings.Contains(errOut, "--purge-crds deletes") || !strings.Contains(errOut, "ConfigMap team-settings") {
		t.Errorf("the single prompt does not cover both:\n%s", errOut)
	}
}
