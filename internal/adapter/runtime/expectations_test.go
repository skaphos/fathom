/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func TestDiscoveryExpectationsAllPayloadsAndHelpers(t *testing.T) {
	for _, tc := range []struct {
		sample, version, kind string
		namespaced            bool
	}{
		{"workload", "apps/v1", "Deployment", true},
		{"crd", "apiextensions.k8s.io/v1", "CustomResourceDefinition", false},
		{"condition", "apiregistration.k8s.io/v1", "APIService", false},
		{"field", "example.org/v1", "Widget", true},
		{"webhook", "admissionregistration.k8s.io/v1", "ValidatingWebhookConfiguration", false},
		{"cronjob", "batch/v1", "CronJob", true},
		{"configmap", "v1", "ConfigMap", true},
		{"annotationstaleness", "v1", "ConfigMap", true},
		{"podprojection", "v1", "Pod", true},
	} {
		t.Run(tc.sample, func(t *testing.T) {
			d := helperFLoadSample(t, tc.sample)
			want := map[schema.GroupVersionKind]bool{schema.FromAPIVersionAndKind(tc.version, tc.kind): tc.namespaced}
			check := &d.Spec.Families[0].Checks[0]
			switch tc.sample {
			case "workload":
				check.Workload.CheckPods = true
				want[schema.FromAPIVersionAndKind("v1", "Pod")] = true
			case "condition":
				// A declared version CRD adds the cluster-scoped CRD helper and one
				// expectation per supported version of the primary kind.
				check.Condition.VersionCRD = "apiservices.apiregistration.k8s.io"
				check.Condition.SupportedVersions = []api.DefinitionToken{"v1", "v2"}
				want[schema.FromAPIVersionAndKind("apiextensions.k8s.io/v1", "CustomResourceDefinition")] = false
				want[schema.FromAPIVersionAndKind("apiregistration.k8s.io/v2", tc.kind)] = tc.namespaced
			case "webhook":
				check.Webhook.VerifyEndpoints = true
				check.Webhook.ServiceNamespace = "default"
				check.Webhook.ExpectedService = "hook"
				want[schema.FromAPIVersionAndKind("discovery.k8s.io/v1", "EndpointSlice")] = true
			}
			got, err := execution.DiscoveryExpectations(&d)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}

func helperFLoadSample(t *testing.T, sample string) api.AddonDefinition {
	t.Helper()
	data, err := os.ReadFile("../../../config/samples/addondefinition/" + sample + ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	var d api.AddonDefinition
	if err := yaml.UnmarshalStrict(data, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// One GVK cannot be both namespaced and cluster-scoped, so a definition that
// declares both must be rejected rather than resolved to whichever check the
// map happened to visit last — the mapper would then read the wrong scope.
func TestDiscoveryExpectationsRejectContradictoryScope(t *testing.T) {
	d := helperFLoadSample(t, "field")
	contradiction := *d.Spec.Families[0].Checks[0].DeepCopy()
	contradiction.Name = "contradiction"
	contradiction.Field.Target = api.DefinitionTarget{Scope: "Cluster"}
	d.Spec.Families[0].Checks = append(d.Spec.Families[0].Checks, contradiction)
	got, err := execution.DiscoveryExpectations(&d)
	if got != nil || err == nil || !strings.Contains(err.Error(), "contradictory scope") {
		t.Fatalf("contradictory scope accepted: %v %v", got, err)
	}
}
