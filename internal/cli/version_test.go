/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func deployment(ns, name, image string, labels map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "kube-rbac-proxy", Image: "quay.io/brancz/kube-rbac-proxy:v0.18"}, {Name: "manager", Image: image}},
		}}},
	}
}

func TestClientVersion(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "v1.2.3"
	if got := clientVersion(); got != "v1.2.3" {
		t.Fatalf("ldflag version = %q", got)
	}
	Version = ""
	if got := clientVersion(); got == "" {
		t.Fatal("fallback version must not be empty")
	}
}

func TestVersionFromImage(t *testing.T) {
	tests := map[string]string{
		"ghcr.io/skaphos/fathom-operator:v0.6.0":                             "v0.6.0",
		"localhost:5000/fathom-operator:e2e":                                 "e2e",
		"ghcr.io/skaphos/fathom-operator:v0.6.0@sha256:0123456789abcdef0123": "v0.6.0",
		"ghcr.io/skaphos/fathom-operator@sha256:0123456789abcdef0123456789":  "sha256:0123456789ab",
		"localhost:5000/fathom-operator":                                     "unknown (untagged image)",
	}
	for image, want := range tests {
		if got := versionFromImage(image); got != want {
			t.Errorf("versionFromImage(%q) = %q, want %q", image, got, want)
		}
	}
}

func TestVersion_ClientOnlyNeverContactsCluster(t *testing.T) {
	f, _ := fakeFactory(t)
	called := false
	f.newClient = func(*rest.Config, client.Options) (client.Client, error) {
		called = true
		return nil, errors.New("boom")
	}
	f.clientConfig = func(*globalOptions) clientcmd.ClientConfig {
		called = true
		return stubClientConfig{cfgErr: errors.New("boom")}
	}
	out, _, err := execVerb(f, "version", "--client")
	if err != nil || called {
		t.Fatalf("--client must not touch the cluster: err=%v called=%v", err, called)
	}
	if !strings.HasPrefix(out, "Client:") || strings.Contains(out, "Operator:") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	out, _, _ = execVerb(f, "version", "--client", "-o", "json")
	var info map[string]any
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatal(err)
	}
	if _, has := info["operator"]; has || info["client"] == "" {
		t.Fatalf("json = %v", info)
	}
}

func TestVersion_FindsOperator(t *testing.T) {
	helm := deployment("fathom-system", "fathom-operator", "ghcr.io/skaphos/fathom-operator:v0.6.0",
		map[string]string{"control-plane": "controller-manager", "app.kubernetes.io/name": "fathom-operator", "app.kubernetes.io/version": "0.6.0"})
	kustomize := deployment("fathom-kustomize", "fathom-controller-manager", "ghcr.io/skaphos/fathom-operator:v0.5.1",
		map[string]string{"control-plane": "controller-manager", "app.kubernetes.io/name": "fathom"})
	digest := deployment("pinned", "fathom-controller-manager", "ghcr.io/skaphos/fathom-operator@sha256:abcdef0123456789abcdef",
		map[string]string{"control-plane": "controller-manager"})
	other := deployment("other", "other-controller-manager", "example.com/other-operator:v9",
		map[string]string{"control-plane": "controller-manager", "app.kubernetes.io/name": "other"})

	t.Run("helm version label wins and namespaces sort first", func(t *testing.T) {
		f, _ := fakeFactory(t, helm, kustomize, other)
		out, _, err := execVerb(f, "version")
		if err != nil {
			t.Fatal(err)
		}
		// fathom-kustomize sorts before fathom-system, so the kustomize one is
		// reported first: no label, so the image tag.
		if !strings.Contains(out, "Operator: v0.5.1 (fathom-kustomize/fathom-controller-manager)") {
			t.Fatalf("unexpected output:\n%s", out)
		}
		out, _, _ = execVerb(f, "version", "-n", "fathom-system", "-o", "json")
		var info versionInfo
		if err := json.Unmarshal([]byte(out), &info); err != nil {
			t.Fatal(err)
		}
		if info.Operator == nil || info.Operator.Version != "0.6.0" || info.Operator.Deployment != "fathom-operator" || info.Operator.Image != "ghcr.io/skaphos/fathom-operator:v0.6.0" {
			t.Fatalf("json operator = %+v", info.Operator)
		}
	})
	t.Run("digest-pinned image", func(t *testing.T) {
		f, _ := fakeFactory(t, digest)
		out, _, _ := execVerb(f, "version")
		if !strings.Contains(out, "Operator: sha256:abcdef012345 (pinned/fathom-controller-manager)") {
			t.Fatalf("unexpected output:\n%s", out)
		}
	})
	t.Run("another operator with the label is not Fathom", func(t *testing.T) {
		f, _ := fakeFactory(t, other)
		out, _, err := execVerb(f, "version")
		if err != nil || !strings.Contains(out, "Operator: unavailable (no Fathom operator deployment found") || !strings.Contains(out, "in any namespace") {
			t.Fatalf("err=%v\n%s", err, out)
		}
	})
	t.Run("namespace scoping", func(t *testing.T) {
		f, _ := fakeFactory(t, helm)
		out, _, _ := execVerb(f, "version", "-n", "elsewhere")
		if !strings.Contains(out, "unavailable") || !strings.Contains(out, "in namespace elsewhere") {
			t.Fatalf("unexpected output:\n%s", out)
		}
	})
}

func TestVersion_UnreachableIsNotAnError(t *testing.T) {
	f, _ := fakeFactory(t)
	f.clientConfig = func(*globalOptions) clientcmd.ClientConfig {
		return stubClientConfig{cfgErr: errors.New("no such kubeconfig")}
	}
	out, _, err := execVerb(f, "version")
	if err != nil {
		t.Fatalf("version must succeed offline: %v", err)
	}
	if !strings.Contains(out, "Client:") || !strings.Contains(out, "Operator: unavailable (load kubeconfig: no such kubeconfig)") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	out, _, _ = execVerb(f, "version", "-o", "yaml")
	if !strings.Contains(out, "error: 'load kubeconfig") && !strings.Contains(out, "error: load kubeconfig") {
		t.Fatalf("yaml should carry the error:\n%s", out)
	}
}
