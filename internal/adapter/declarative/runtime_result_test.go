/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative_test

import (
	"context"
	"os"
	"strings"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/declarative"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type oversizedEvidenceClient struct {
	client.Client
	calls int
}

func (c *oversizedEvidenceClient) Get(_ context.Context, key client.ObjectKey, out client.Object, _ ...client.GetOption) error {
	c.calls++
	cm := out.(*corev1.ConfigMap)
	cm.Name = key.Name
	cm.Namespace = key.Namespace
	cm.Data = map[string]string{"config": "apiVersion: " + strings.Repeat("a", 1024)}
	return nil
}
func TestRuntimeResultLimitStopsLaterChecks(t *testing.T) {
	data, err := os.ReadFile("../../../config/samples/addondefinition/configmap.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var d api.AddonDefinition
	if err := yaml.UnmarshalStrict(data, &d); err != nil {
		t.Fatal(err)
	}
	family := &d.Spec.Families[0]
	family.DefaultEnabled = true
	family.Checks[0].ConfigMap.Key = "config"
	family.Checks[0].ConfigMap.RecognizedAPIVersions = []string{"v1"}
	second := family.Checks[0].DeepCopy()
	second.Name = "second"
	family.Checks = append(family.Checks, *second)
	engine, err := declarative.CompileRuntime(context.Background(), &d)
	if err != nil {
		t.Fatal(err)
	}
	b, done := execution.NewBudget(context.Background(), 0)
	defer done()
	c := &oversizedEvidenceClient{}
	result, err := engine.Run(declarative.WithExecutionBudget(b.Context(), b), adapter.Request{Client: c})
	if err == nil || !strings.Contains(err.Error(), "ResultLimitExceeded") || c.calls != 1 || len(result.Checks) != 0 {
		t.Fatalf("calls=%d result=%+v err=%v", c.calls, result, err)
	}
}
