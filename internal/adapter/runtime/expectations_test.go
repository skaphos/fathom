/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"os"
	"reflect"
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
			data, err := os.ReadFile("../../../config/samples/addondefinition/" + tc.sample + ".yaml")
			if err != nil {
				t.Fatal(err)
			}
			var d api.AddonDefinition
			if err := yaml.UnmarshalStrict(data, &d); err != nil {
				t.Fatal(err)
			}
			want := map[schema.GroupVersionKind]bool{schema.FromAPIVersionAndKind(tc.version, tc.kind): tc.namespaced}
			check := &d.Spec.Families[0].Checks[0]
			switch tc.sample {
			case "workload":
				check.Workload.CheckPods = true
				want[schema.FromAPIVersionAndKind("v1", "Pod")] = true
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
