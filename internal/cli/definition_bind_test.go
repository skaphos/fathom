/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func TestDefinitionBindReviewedUIDs(t *testing.T) {
	d := &api.AddonDefinition{TypeMeta: metav1.TypeMeta{APIVersion: api.GroupVersion.String(), Kind: "AddonDefinition"}, ObjectMeta: metav1.ObjectMeta{Name: "custom", UID: "definition-uid"}, Spec: api.AddonDefinitionSpec{AddonType: "custom", AdapterVersion: "1.0.0", SemanticsVersion: 1, Families: []api.DefinitionFamily{{Name: "health", Checks: []api.DefinitionCheck{{Name: "hook", Kind: "Webhook", Webhook: &api.DefinitionWebhook{Target: api.DefinitionTarget{Scope: "Cluster"}, Kind: "ValidatingWebhookConfiguration", Name: "hook", ExpectedService: "hook", ServiceNamespace: "helpers", VerifyEndpoints: true}}}}}}}
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: "fathom", UID: "sa-uid"}}
	file := filepath.Join(t.TempDir(), "reviewed.yaml")
	data, err := yaml.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	f, c := fakeFactory(t, d, sa)
	args := []string{"bind", "--file", file, "--name", "custom", "--service-account", "reader", "--operator-namespace", "fathom"}
	out, _, err := execVerb(f, "definition", args...)
	if err != nil {
		t.Fatal(err)
	}
	var binding api.AddonDefinitionBinding
	if err := yaml.UnmarshalStrict([]byte(out), &binding); err != nil {
		t.Fatal(err)
	}
	if binding.Spec.Enabled || binding.Spec.DefinitionRef.UID != "definition-uid" || binding.Spec.ServiceAccountRef.UID != "sa-uid" {
		t.Fatalf("incorrect authority: %+v", binding.Spec)
	}
	if !binding.Spec.TargetScope.AllowClusterScoped || len(binding.Spec.TargetScope.Namespaces) != 1 || binding.Spec.TargetScope.Namespaces[0] != "helpers" {
		t.Fatal("helper scope omitted")
	}
	var bindings api.AddonDefinitionBindingList
	if err := c.List(context.Background(), &bindings); err != nil || len(bindings.Items) != 0 {
		t.Fatal("bind wrote resources")
	}
	d.Spec.AdapterVersion = "2.0.0"
	if err := c.Update(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if _, _, err := execVerb(f, "definition", args...); err == nil {
		t.Fatal("changed live definition accepted")
	}
}
