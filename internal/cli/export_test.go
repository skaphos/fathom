/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/yaml"
)

const alphaGroupVersion = "fathom.skaphos.io/" + alphaVersion

// alphaCRD is a live 0.6.x CRD: it serves v1alpha1 only.
func alphaCRD(kind, plural string, scope apiextensionsv1.ResourceScope) *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: plural + ".fathom.skaphos.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "fathom.skaphos.io", Scope: scope,
			Names:    apiextensionsv1.CustomResourceDefinitionNames{Kind: kind, ListKind: kind + "List", Plural: plural, Singular: strings.ToLower(kind)},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: alphaVersion, Served: true, Storage: true}},
		},
	}
}

func alphaObject(kind, ns, name string, spec map[string]any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(alphaGroupVersion)
	u.SetKind(kind)
	u.SetName(name)
	if ns != "" {
		u.SetNamespace(ns)
	}
	u.SetUID(types.UID("uid-" + name))
	u.SetResourceVersion("7")
	u.SetAnnotations(map[string]string{lastAppliedAnnotation: "{}", "team": "a"})
	u.Object["spec"] = spec
	u.Object["status"] = map[string]any{"result": "Pass"}
	return u
}

// exportFactory wires a fake client that knows the alpha kinds only through a
// REST mapper, the way a 0.6.x cluster looks to this CLI's scheme.
func exportFactory(t *testing.T, objs ...client.Object) (*factory, client.WithWatch) {
	t.Helper()
	scheme, err := newScheme()
	if err != nil {
		t.Fatal(err)
	}
	alphaGV := schema.GroupVersion{Group: "fathom.skaphos.io", Version: alphaVersion}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{alphaGV})
	for _, m := range []struct {
		kind, plural string
		scope        meta.RESTScope
	}{
		{"ClusterHealth", "clusterhealths", meta.RESTScopeRoot},
		{"AddonCheck", "addonchecks", meta.RESTScopeNamespace},
		{"DNSCheck", "dnschecks", meta.RESTScopeNamespace},
		{"NodeCertificateCheck", "nodecertificatechecks", meta.RESTScopeNamespace},
		{"NodeHealthCheck", "nodehealthchecks", meta.RESTScopeNamespace},
		{"HealthCheck", "healthchecks", meta.RESTScopeNamespace},
		{"HealthReport", "healthreports", meta.RESTScopeNamespace},
	} {
		mapper.AddSpecific(alphaGV.WithKind(m.kind), alphaGV.WithResource(m.plural), alphaGV.WithResource(strings.ToLower(m.kind)), m.scope)
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(mapper).WithObjects(objs...).Build()
	f := newFactory()
	f.clientConfig = func(*globalOptions) clientcmd.ClientConfig {
		return stubClientConfig{cfg: &rest.Config{}, namespace: "team-a"}
	}
	f.newClient = func(*rest.Config, client.Options) (client.Client, error) { return fc, nil }
	return f, fc
}

func execExport(f *factory, args ...string) (string, string, error) {
	root := newRootCommand(f)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"export"}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func documents(t *testing.T, out string) []map[string]any {
	t.Helper()
	var docs []map[string]any
	for _, raw := range strings.Split(out, "\n---\n")[1:] {
		var m map[string]any
		if err := yaml.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("decode document: %v\n%s", err, raw)
		}
		docs = append(docs, m)
	}
	return docs
}

func TestExportRendersAlphaChecksAsV1(t *testing.T) {
	f, _ := exportFactory(t,
		alphaCRD("AddonCheck", "addonchecks", apiextensionsv1.NamespaceScoped),
		alphaCRD("HealthCheck", "healthchecks", apiextensionsv1.NamespaceScoped),
		alphaCRD("ClusterHealth", "clusterhealths", apiextensionsv1.ClusterScoped),
		alphaCRD("HealthReport", "healthreports", apiextensionsv1.NamespaceScoped),
		alphaObject("HealthCheck", "team-b", "wrap", map[string]any{"checkRef": map[string]any{"apiVersion": alphaGroupVersion, "kind": "AddonCheck", "name": "zeta"}, "paused": true}),
		alphaObject("AddonCheck", "team-b", "zeta", map[string]any{"addonType": "coredns", "paused": true}),
		alphaObject("AddonCheck", "team-a", "alpha", map[string]any{"addonType": "cert-manager"}),
		alphaObject("ClusterHealth", "", "all", map[string]any{"namespaces": []any{"team-a"}}),
		alphaObject("HealthReport", "team-a", "alpha-x", map[string]any{"result": "Pass"}),
	)
	out, errOut, err := execExport(f)
	if err != nil {
		t.Fatalf("export: %v\n%s", err, errOut)
	}
	docs := documents(t, out)
	var order []string
	for _, d := range docs {
		meta := d["metadata"].(map[string]any)
		order = append(order, d["kind"].(string)+"/"+meta["name"].(string))
		if d["apiVersion"] != "fathom.skaphos.io/v1" {
			t.Errorf("%v: apiVersion = %v", order[len(order)-1], d["apiVersion"])
		}
		if _, ok := d["status"]; ok {
			t.Errorf("%v: status was exported", order[len(order)-1])
		}
		for _, k := range []string{"uid", "resourceVersion"} {
			if _, ok := meta[k]; ok {
				t.Errorf("%v: metadata.%s was exported", order[len(order)-1], k)
			}
		}
		if ann, _ := meta["annotations"].(map[string]any); ann[lastAppliedAnnotation] != nil || ann["team"] != "a" {
			t.Errorf("%v: annotations = %v", order[len(order)-1], ann)
		}
		spec := d["spec"].(map[string]any)
		if _, ok := spec["paused"]; ok {
			t.Errorf("%v: spec.paused was exported", order[len(order)-1])
		}
		if ref, ok := spec["checkRef"].(map[string]any); ok && ref["apiVersion"] != "fathom.skaphos.io/v1" {
			t.Errorf("%v: checkRef.apiVersion = %v", order[len(order)-1], ref["apiVersion"])
		}
	}
	want := []string{"ClusterHealth/all", "AddonCheck/alpha", "AddonCheck/zeta", "HealthCheck/wrap"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("documents = %v, want %v (targets first, then namespace, then name; no reports)", order, want)
	}
	if !strings.HasPrefix(out, "# Fathom check resources exported by fathomctl export") {
		t.Errorf("missing header:\n%s", out[:min(len(out), 120)])
	}
}

func TestExportNamespaceFilterKeepsClusterScopedKinds(t *testing.T) {
	f, _ := exportFactory(t,
		alphaCRD("AddonCheck", "addonchecks", apiextensionsv1.NamespaceScoped),
		alphaCRD("ClusterHealth", "clusterhealths", apiextensionsv1.ClusterScoped),
		alphaObject("AddonCheck", "team-b", "zeta", map[string]any{"addonType": "coredns"}),
		alphaObject("AddonCheck", "team-a", "alpha", map[string]any{"addonType": "cert-manager"}),
		alphaObject("ClusterHealth", "", "all", map[string]any{}),
	)
	out, _, err := execExport(f, "-n", "team-a")
	if err != nil {
		t.Fatal(err)
	}
	docs := documents(t, out)
	if len(docs) != 2 || docs[0]["kind"] != "ClusterHealth" || docs[1]["metadata"].(map[string]any)["name"] != "alpha" {
		t.Fatalf("documents = %v, want the ClusterHealth and team-a's AddonCheck only", docs)
	}
}

func TestExportWritesTheFileAtomicallyAndReportsCount(t *testing.T) {
	f, _ := exportFactory(t,
		alphaCRD("AddonCheck", "addonchecks", apiextensionsv1.NamespaceScoped),
		alphaObject("AddonCheck", "team-a", "alpha", map[string]any{"addonType": "cert-manager"}),
	)
	path := filepath.Join(t.TempDir(), "v1.yaml")
	out, errOut, err := execExport(f, "--output", path)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("stdout should be empty with --output, got:\n%s", out)
	}
	if !strings.Contains(errOut, "exported 1 resource(s) to "+path) {
		t.Errorf("stderr = %q", errOut)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "kind: AddonCheck") {
		t.Errorf("file content:\n%s", raw)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".fathom-export-*")); len(leftovers) != 0 {
		t.Errorf("temporary files left behind: %v", leftovers)
	}
}

func TestExportRefusesWhenNothingServesAlpha(t *testing.T) {
	v1 := alphaCRD("AddonCheck", "addonchecks", apiextensionsv1.NamespaceScoped)
	v1.Spec.Versions[0].Name = "v1"
	f, _ := exportFactory(t, v1)
	out, _, err := execExport(f)
	var r refusal
	if !errors.As(err, &r) || r.Reason != reasonNothingToExport {
		t.Fatalf("err = %v, want a %s refusal", err, reasonNothingToExport)
	}
	if out != "" {
		t.Errorf("nothing should be written, got:\n%s", out)
	}
}

func TestExportReportsUnrenderableObjectsAndStillWritesTheRest(t *testing.T) {
	f, fc := exportFactory(t,
		alphaCRD("AddonCheck", "addonchecks", apiextensionsv1.NamespaceScoped),
		alphaCRD("DNSCheck", "dnschecks", apiextensionsv1.NamespaceScoped),
		alphaObject("AddonCheck", "team-a", "alpha", map[string]any{"addonType": "cert-manager"}),
	)
	failing := interceptor.NewClient(fc, interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if list.GetObjectKind().GroupVersionKind().Kind == "DNSCheckList" {
				return errors.New("boom")
			}
			return c.List(ctx, list, opts...)
		},
	})
	f.newClient = func(*rest.Config, client.Options) (client.Client, error) { return failing, nil }
	out, errOut, err := execExport(f)
	if err == nil {
		t.Fatal("a failed kind must make export exit non-zero")
	}
	if !strings.Contains(errOut, "cannot export DNSCheck: boom") {
		t.Errorf("stderr = %q", errOut)
	}
	if !strings.Contains(out, "kind: AddonCheck") {
		t.Errorf("the renderable objects must still be written:\n%s", out)
	}
}
