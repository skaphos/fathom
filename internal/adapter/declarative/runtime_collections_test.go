/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/declarative"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

type collectionPageClient struct {
	client.Client
	calls int
	page  func(int) kruntime.Object
}

func (c *collectionPageClient) List(_ context.Context, list client.ObjectList, opts ...client.ListOption) error {
	options := new(client.ListOptions).ApplyOptions(opts)
	if options.Limit != 100 {
		return fmt.Errorf("missing page limit")
	}
	c.calls++
	if c.calls > 2 {
		return fmt.Errorf("unexpected third page")
	}
	if c.calls == 2 && options.Continue != "next" {
		return fmt.Errorf("missing continuation")
	}
	if err := meta.SetList(list, []kruntime.Object{c.page(c.calls)}); err != nil {
		return err
	}
	if c.calls == 1 {
		list.SetContinue("next")
	}
	return nil
}

func TestRuntimeCollectionPayloadsFollowContinuation(t *testing.T) {
	for _, sample := range []string{"condition", "annotationstaleness", "podprojection", "workload", "webhook"} {
		t.Run(sample, func(t *testing.T) {
			data, err := os.ReadFile("../../../config/samples/addondefinition/" + sample + ".yaml")
			if err != nil {
				t.Fatal(err)
			}
			var d api.AddonDefinition
			if err := yaml.UnmarshalStrict(data, &d); err != nil {
				t.Fatal(err)
			}
			d.Spec.Families[0].DefaultEnabled = true
			check := &d.Spec.Families[0].Checks[0]
			scheme := kruntime.NewScheme()
			if err := clientgoscheme.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			var objects []client.Object
			var page func(int) kruntime.Object
			switch sample {
			case "condition":
				check.Condition.Names = nil
				check.Condition.ListKind = "APIServiceList"
				page = func(i int) kruntime.Object {
					status := "True"
					if i == 2 {
						status = "False"
					}
					return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService", "metadata": map[string]any{"name": fmt.Sprint(i)}, "status": map[string]any{"conditions": []any{map[string]any{"type": "Available", "status": status}}}}}
				}
			case "annotationstaleness":
				check.AnnotationStaleness.DefaultName = ""
				check.AnnotationStaleness.ListKind = "ConfigMapList"
				page = func(i int) kruntime.Object {
					timestamp := time.Now().UTC().Format(time.RFC3339)
					if i == 2 {
						timestamp = "2000-01-01T00:00:00Z"
					}
					return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": fmt.Sprint(i), "annotations": map[string]any{check.AnnotationStaleness.AnnotationKey: timestamp}}}}
				}
			case "podprojection":
				page = func(i int) kruntime.Object {
					return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprint(i), Namespace: "default"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}
				}
			case "workload":
				check.Workload.CheckPods = true
				objects = append(objects, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: string(check.Workload.DefaultName), Namespace: "default"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}}}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1, Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}}})
				page = func(i int) kruntime.Object {
					ready := corev1.ConditionTrue
					if i == 2 {
						ready = corev1.ConditionFalse
					}
					return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprint(i), Namespace: "default"}, Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: ready}}}}
				}
			case "webhook":
				check.Webhook.VerifyEndpoints = true
				check.Webhook.ExpectedService = "hook"
				check.Webhook.ServiceNamespace = "default"
				objects = append(objects, &admissionv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: string(check.Webhook.Name)}, Webhooks: []admissionv1.ValidatingWebhook{{Name: "hook.example.org", ClientConfig: admissionv1.WebhookClientConfig{Service: &admissionv1.ServiceReference{Name: "hook", Namespace: "default"}}}}})
				page = func(i int) kruntime.Object {
					ready := i == 2
					return &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprint(i)}, Endpoints: []discoveryv1.Endpoint{{Conditions: discoveryv1.EndpointConditions{Ready: &ready}}}}
				}
			}
			c := &collectionPageClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(), page: page}
			engine, err := declarative.CompileRuntime(context.Background(), &d)
			if err != nil {
				t.Fatal(err)
			}
			b, done := execution.NewBudget(context.Background(), 0)
			defer done()
			result, err := engine.Run(declarative.WithExecutionBudget(b.Context(), b), adapter.Request{Client: c})
			if err != nil {
				t.Fatal(err)
			}
			if c.calls != 2 {
				t.Fatalf("calls=%d results=%+v", c.calls, result.Checks)
			}
			last := result.Checks[len(result.Checks)-1]
			switch sample {
			case "webhook":
				if last.Details["readyEndpoints"] != "1" {
					t.Fatalf("last page ignored: %+v", last)
				}
			case "podprojection":
				if last.Details["matchedPods"] != "2" {
					t.Fatalf("last page ignored: %+v", last)
				}
			default:
				if last.Outcome == adapter.OutcomePass {
					t.Fatalf("unhealthy last page ignored: %+v", last)
				}
			}
		})
	}
}
