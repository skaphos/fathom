/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// A Helm release's manager always carries the chart's instance and
// managed-by labels; an unlabelled operator in the release namespace is
// another install, not the release's.
func TestDetectInstallTreatsAnUnlabelledManagerAsAnotherInstall(t *testing.T) {
	env := newLifecycleEnv(t, managerDeployment("fathom-system", nil, true))
	env.rc.releases["fathom-system/fathom"] = &helmRelease{
		Name: "fathom", Namespace: "fathom-system", ChartName: chartName, ChartVersion: "0.5.1",
		AppVersion: "0.5.1", Status: "failed", Revision: 1,
	}
	_, err := detectInstall(context.Background(), env.c, env.rc)
	var r refusal
	if !errors.As(err, &r) || r.Reason != reasonMultipleInstalls {
		t.Fatalf("err = %v, want refusal %s", err, reasonMultipleInstalls)
	}
}

// A purge deletes every CRD in the group; one this binary cannot count or
// export (a newer release's kind) refuses before anything changes.
func TestPurgeRefusesUnknownKinds(t *testing.T) {
	future := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "widgets.fathom.skaphos.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "fathom.skaphos.io", Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Widget", ListKind: "WidgetList", Plural: "widgets", Singular: "widget"},
		},
	}
	env := newLifecycleEnv(t, append(liveCRDs(t, "0.6.0", nil), future)...)
	before := len(env.log.all())
	_, _, err := execVerb(env.f, "uninstall", "--purge-crds", "--yes")
	var r refusal
	if !errors.As(err, &r) || r.Reason != reasonUnknownKinds || !strings.Contains(r.Detail, "widgets.fathom.skaphos.io") {
		t.Fatalf("err = %v, want a %s refusal naming the unknown CRD", err, reasonUnknownKinds)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("refused purge changed the cluster: %v", w)
	}
}

// A known kind no longer served at the version this binary lists would list
// as absent and be purged unseen, so it refuses too.
func TestPurgeRefusesAKnownKindNoLongerServedAtV1alpha1(t *testing.T) {
	crds := liveCRDs(t, "0.6.0", func(c *apiextensionsv1.CustomResourceDefinition) {
		if c.Name != dnsCRD {
			return
		}
		next := *c.Spec.Versions[0].DeepCopy()
		next.Name = "v1"
		c.Spec.Versions[0].Served, c.Spec.Versions[0].Storage = false, false
		c.Spec.Versions = append(c.Spec.Versions, next)
	})
	env := newLifecycleEnv(t, crds...)
	before := len(env.log.all())
	_, _, err := execVerb(env.f, "uninstall", "--purge-crds", "--yes")
	var r refusal
	if !errors.As(err, &r) || r.Reason != reasonUnknownKinds || !strings.Contains(r.Detail, dnsCRD) {
		t.Fatalf("err = %v, want a %s refusal naming %s", err, reasonUnknownKinds, dnsCRD)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("refused purge changed the cluster: %v", w)
	}
}

// Any discovery error fails closed: partial results would hide an
// unavailable group's objects from the --delete-namespace inventory.
func TestDiscoverNamespacedKindsFailsClosedOnPartialDiscovery(t *testing.T) {
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api":
			write(w, metav1.APIVersions{Versions: []string{"v1"}})
		case "/api/v1":
			write(w, metav1.APIResourceList{GroupVersion: "v1", APIResources: []metav1.APIResource{
				{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"list", "delete"}},
			}})
		case "/apis":
			write(w, metav1.APIGroupList{Groups: []metav1.APIGroup{{
				Name:             "metrics.k8s.io",
				Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: "metrics.k8s.io/v1beta1", Version: "v1beta1"}},
				PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "metrics.k8s.io/v1beta1", Version: "v1beta1"},
			}}})
		default:
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()

	f := newFactory()
	f.clientConfig = func(*globalOptions) clientcmd.ClientConfig {
		return stubClientConfig{cfg: &rest.Config{Host: srv.URL}}
	}
	if kinds, err := discoverNamespacedKinds(f); err == nil {
		t.Fatalf("discovery with an unavailable group returned %v and no error", kinds)
	}
}

// A secret-looking key is redacted whatever its value's shape.
func TestRedactValuesRedactsStructuredSecrets(t *testing.T) {
	in := map[string]any{
		"image":    map[string]any{"repository": "ghcr.io/skaphos/fathom-operator"},
		"secret":   map[string]any{"data": map[string]any{"key": "s3cr3t"}},
		"apiToken": map[string]any{"value": "t0ken"},
		"nested":   map[string]any{"password": "hunter2", "replicas": 2},
	}
	out := redactValues(in)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"s3cr3t", "t0ken", "hunter2"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("redacted values still contain %q: %s", leaked, raw)
		}
	}
	if !strings.Contains(string(raw), "fathom-operator") || !strings.Contains(string(raw), `"replicas":2`) {
		t.Errorf("non-secret values were lost: %s", raw)
	}
}

var _ client.Object = (*apiextensionsv1.CustomResourceDefinition)(nil)
