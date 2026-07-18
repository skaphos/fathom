/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/skaphos/fathom/pkg/adapter"
)

func configMap(name, namespace string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data:       data,
	}
}

func runConfigMap(t *testing.T, cc ConfigMapCheck, objs ...clientObject) []adapter.CheckResult {
	t.Helper()
	eng := MustEngine(AddonDefinition{
		AddonType:      "configmaptest",
		AdapterVersion: "0.0.1",
		Families:       []FamilyDefinition{{Name: "policy", DefaultEnabled: true, ConfigMaps: []ConfigMapCheck{cc}}},
	})
	res, err := eng.Run(context.Background(), adapter.Request{Client: newFakeClient(t, objs...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res.Checks
}

func TestConfigMapCheck(t *testing.T) {
	check := ConfigMapCheck{
		DefaultNamespace:      "kube-system",
		DefaultName:           "descheduler",
		Component:             "descheduler",
		Key:                   "policy.yaml",
		RecognizedAPIVersions: []string{"descheduler/v1alpha1", "descheduler/v1alpha2"},
		UnrecognizedOutcome:   adapter.OutcomeWarn,
		InvalidOutcome:        adapter.OutcomeFail,
	}
	validPolicy := "apiVersion: descheduler/v1alpha2\nkind: DeschedulerPolicy\nprofiles: []\n"
	unknownPolicy := "apiVersion: descheduler/v1beta9\nkind: DeschedulerPolicy\n"

	tests := []struct {
		name    string
		data    map[string]string
		outcome adapter.Outcome
		summary string
	}{
		{"valid recognized policy passes", map[string]string{"policy.yaml": validPolicy}, adapter.OutcomePass, "well-formed"},
		{"missing key fails", map[string]string{"other.yaml": validPolicy}, adapter.OutcomeFail, "no \"policy.yaml\" key"},
		{"unparseable yaml fails", map[string]string{"policy.yaml": "\tnot: [valid"}, adapter.OutcomeFail, "not valid YAML"},
		{"unrecognized apiVersion warns", map[string]string{"policy.yaml": unknownPolicy}, adapter.OutcomeWarn, "not recognized"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checks := runConfigMap(t, check, configMap("descheduler", "kube-system", tc.data))
			assertHasOutcome(t, checks, "ConfigMap", "descheduler", tc.outcome, tc.summary)
		})
	}
}

func TestConfigMapCheck_AbsentInheritsOptional(t *testing.T) {
	eng := MustEngine(AddonDefinition{
		AddonType:      "configmaptest",
		AdapterVersion: "0.0.1",
		Optional:       true,
		Families:       []FamilyDefinition{{Name: "policy", DefaultEnabled: true, ConfigMaps: []ConfigMapCheck{{DefaultNamespace: "kube-system", DefaultName: "descheduler", Component: "descheduler", Key: "policy.yaml"}}}},
	})
	res, err := eng.Run(context.Background(), adapter.Request{Client: newFakeClient(t), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, res.Checks, "ConfigMap", "descheduler", adapter.OutcomeSkipped, "not found")
	assertHasDetail(t, res.Checks, "ConfigMap", "descheduler", adapter.DetailAbsent, "true")
}

func TestConfigMapCheck_NoAPIVersionAssertionPassesAnyYAML(t *testing.T) {
	// With RecognizedAPIVersions unset, any parseable YAML passes — the apiVersion
	// assertion is opt-in.
	check := ConfigMapCheck{DefaultNamespace: "kube-system", DefaultName: "cm", Component: "cm", Key: "policy.yaml"}
	checks := runConfigMap(t, check, configMap("cm", "kube-system", map[string]string{"policy.yaml": "any: value\n"}))
	assertHasOutcome(t, checks, "ConfigMap", "cm", adapter.OutcomePass, "well-formed")
}
