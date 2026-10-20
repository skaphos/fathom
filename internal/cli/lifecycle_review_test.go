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

	corev1 "k8s.io/api/core/v1"
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

// A known kind is enumerated at whatever version its live CRD serves, not
// the version this binary was compiled against: the storage version when it
// is served, else the first served one. Only a CRD serving nothing refuses.
func TestRefuseUnknownKindsReportsTheServedVersion(t *testing.T) {
	crds := liveCRDs(t, "0.7.0", func(c *apiextensionsv1.CustomResourceDefinition) {
		if c.Name != dnsCRD {
			return
		}
		// A future release that moved DNSCheck on to v2: the compiled-in
		// version is unserved, the live one is what gets enumerated.
		next := *c.Spec.Versions[0].DeepCopy()
		next.Name = "v2"
		c.Spec.Versions[0].Served, c.Spec.Versions[0].Storage = false, false
		c.Spec.Versions = append(c.Spec.Versions, next)
	})
	env := newLifecycleEnv(t, crds...)
	live, err := refuseUnknownKinds(context.Background(), env.c)
	if err != nil {
		t.Fatalf("refuseUnknownKinds: %v", err)
	}
	if live["DNSCheck"] != "v2" {
		t.Errorf("DNSCheck served version = %q, want v2", live["DNSCheck"])
	}
	if live["AddonCheck"] != "v1" {
		t.Errorf("AddonCheck served version = %q, want v1", live["AddonCheck"])
	}
	if live["AddonDefinition"] != "v1alpha1" {
		t.Errorf("AddonDefinition served version = %q, want v1alpha1", live["AddonDefinition"])
	}
	if _, ok := live["Widget"]; ok {
		t.Errorf("a kind with no CRD must not be reported live: %v", live)
	}
}

// The storage version wins when it is served; otherwise the first served
// version is used; a CRD serving nothing is unknown.
func TestServedVersionPrefersStorageThenFirstServed(t *testing.T) {
	crd := func(versions ...apiextensionsv1.CustomResourceDefinitionVersion) *apiextensionsv1.CustomResourceDefinition {
		return &apiextensionsv1.CustomResourceDefinition{Spec: apiextensionsv1.CustomResourceDefinitionSpec{Versions: versions}}
	}
	cases := []struct {
		name     string
		crd      *apiextensionsv1.CustomResourceDefinition
		want     string
		wantSome bool
	}{
		{"storage served", crd(
			apiextensionsv1.CustomResourceDefinitionVersion{Name: "v1alpha1", Served: true},
			apiextensionsv1.CustomResourceDefinitionVersion{Name: "v1", Served: true, Storage: true},
		), "v1", true},
		{"storage unserved falls back to first served", crd(
			apiextensionsv1.CustomResourceDefinitionVersion{Name: "v1alpha1", Served: false, Storage: true},
			apiextensionsv1.CustomResourceDefinitionVersion{Name: "v1", Served: true},
		), "v1", true},
		{"nothing served", crd(
			apiextensionsv1.CustomResourceDefinitionVersion{Name: "v1alpha1", Served: false, Storage: true},
		), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := servedVersion(tc.crd)
			if got != tc.want || ok != tc.wantSome {
				t.Fatalf("servedVersion = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.wantSome)
			}
		})
	}
}

// A CRD in the group that serves no version at all cannot be enumerated and
// refuses the purge before anything changes.
func TestPurgeRefusesAKnownKindServingNoVersion(t *testing.T) {
	crds := liveCRDs(t, "0.6.0", func(c *apiextensionsv1.CustomResourceDefinition) {
		if c.Name == dnsCRD {
			c.Spec.Versions[0].Served = false
		}
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

// "No such kind" from a stale mapping is not believed while the kind's CRD
// is live: the purge stops rather than delete resources it never counted.
func TestPurgeFailsClosedWhenALiveKindCannotBeListed(t *testing.T) {
	env := newLifecycleEnv(t, append(liveCRDs(t, "0.6.0", nil), addonCheck("team-a", "coredns"))...)
	env.listNoMatch["AddonCheckList"] = true
	before := len(env.log.all())
	_, _, err := execVerb(env.f, "uninstall", "--purge-crds", "--yes")
	if err == nil || !strings.Contains(err.Error(), "list AddonCheck") {
		t.Fatalf("err = %v, want the AddonCheck list failure", err)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("purge went ahead without counting AddonChecks: %v", w)
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

// A dependent is reported through its owner only when that owner is in the
// namespace; one whose owner is gone is listed, so it is not deleted unseen.
func TestDeleteNamespaceListsDependentsWithoutAListedOwner(t *testing.T) {
	root := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "team-root", Namespace: "fathom-system", UID: "root-uid"}}
	child := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "team-child", Namespace: "fathom-system", UID: "child-uid",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: "team-root", UID: "root-uid"}}}}
	orphan := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "team-orphan", Namespace: "fathom-system", UID: "orphan-uid",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: "gone", UID: "gone-uid"}}}}
	env := nsEnv(t, root, child, orphan)
	_, _, err := execVerb(env.f, "uninstall", "--delete-namespace")
	if err == nil {
		t.Fatal("uninstall --delete-namespace succeeded without confirming foreign objects")
	}
	msg := err.Error()
	for _, want := range []string{"ConfigMap team-root", "ConfigMap team-orphan"} {
		if !strings.Contains(msg, want) {
			t.Errorf("confirmation does not list %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "team-child") {
		t.Errorf("a dependent of a listed owner was listed separately:\n%s", msg)
	}
}

// The documented retry after a timed-out rollout reaches the same-version
// branch; it must not report success while the operator is unavailable.
func TestUpgradeSameVersionWaitsForTheRollout(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := newLifecycleEnv(t, append(liveCRDs(t, "0.6.0", nil), namespaceObj("fathom-system"))...)
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0"}, false)
	out, _, err := execVerb(env.f, "upgrade", "--timeout", "50ms")
	if err == nil || !strings.Contains(err.Error(), "not Available") {
		t.Fatalf("err = %v, want the unavailable operator reported\n%s", err, out)
	}
	if strings.Contains(out, "nothing to do") {
		t.Errorf("reported success while the operator is unavailable:\n%s", out)
	}
}

func TestUpgradeSameVersionStructuredOutput(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := newLifecycleEnv(t, append(liveCRDs(t, "0.6.0", nil), namespaceObj("fathom-system"))...)
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0"}, true)
	out, _, err := execVerb(env.f, "upgrade", "-o", "json")
	if err != nil {
		t.Fatalf("upgrade -o json: %v", err)
	}
	var plan map[string]any
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("no-op upgrade -o json is not JSON: %v\n%s", err, out)
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
