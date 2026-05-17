/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package crdutil_test

import (
	"testing"

	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/skaphos/fathom/internal/adapter/crdutil"
)

func TestEstablished(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		crd  *apixv1.CustomResourceDefinition
		want bool
	}{
		{
			name: "established true",
			crd:  crd(apixv1.Established, apixv1.ConditionTrue),
			want: true,
		},
		{
			name: "established false",
			crd:  crd(apixv1.Established, apixv1.ConditionFalse),
			want: false,
		},
		{
			name: "no established condition",
			crd:  crd(apixv1.NamesAccepted, apixv1.ConditionTrue),
			want: false,
		},
		{
			name: "no conditions at all",
			crd:  &apixv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "any.example.io"}},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := crdutil.Established(tc.crd); got != tc.want {
				t.Errorf("Established() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPreferredServedVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		served     []string
		preference []string
		want       string
		ok         bool
	}{
		{
			name:       "single match",
			served:     []string{"v1"},
			preference: []string{"v1"},
			want:       "v1",
			ok:         true,
		},
		{
			name:       "prefers earlier preference",
			served:     []string{"v1beta1", "v1"},
			preference: []string{"v1", "v1beta1"},
			want:       "v1",
			ok:         true,
		},
		{
			name:       "falls through to later preference",
			served:     []string{"v1beta1"},
			preference: []string{"v1", "v1beta1"},
			want:       "v1beta1",
			ok:         true,
		},
		{
			name:       "served version not in preference",
			served:     []string{"v1alpha1"},
			preference: []string{"v1", "v1beta1"},
			want:       "",
			ok:         false,
		},
		{
			name:       "non-served preference is ignored",
			served:     []string{"v1beta1"},
			preference: []string{"v2", "v1beta1"},
			want:       "v1beta1",
			ok:         true,
		},
		{
			name:       "empty served list",
			served:     nil,
			preference: []string{"v1"},
			want:       "",
			ok:         false,
		},
		{
			name:       "empty preference",
			served:     []string{"v1"},
			preference: nil,
			want:       "",
			ok:         false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := crdWithServed(tc.served...)
			got, ok := crdutil.PreferredServedVersion(c, tc.preference)
			if got != tc.want || ok != tc.ok {
				t.Errorf("PreferredServedVersion(%v, %v) = (%q, %v), want (%q, %v)",
					tc.served, tc.preference, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// PreferredServedVersion must ignore CRD versions that are present in
// Spec.Versions but not marked Served — they represent migration leftovers
// or deprecated paths the apiserver no longer accepts requests for.
func TestPreferredServedVersion_IgnoresUnservedEntries(t *testing.T) {
	t.Parallel()

	c := &apixv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "any.example.io"},
		Spec: apixv1.CustomResourceDefinitionSpec{
			Versions: []apixv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: false},
				{Name: "v1beta1", Served: true, Storage: true},
			},
		},
	}
	got, ok := crdutil.PreferredServedVersion(c, []string{"v1", "v1beta1"})
	if got != "v1beta1" || !ok {
		t.Fatalf("PreferredServedVersion = (%q, %v), want (v1beta1, true) — must skip Served=false entries", got, ok)
	}
}

func crd(condType apixv1.CustomResourceDefinitionConditionType, status apixv1.ConditionStatus) *apixv1.CustomResourceDefinition {
	return &apixv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "any.example.io"},
		Status: apixv1.CustomResourceDefinitionStatus{
			Conditions: []apixv1.CustomResourceDefinitionCondition{{Type: condType, Status: status}},
		},
	}
}

func crdWithServed(versions ...string) *apixv1.CustomResourceDefinition {
	out := &apixv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "any.example.io"},
	}
	for i, v := range versions {
		out.Spec.Versions = append(out.Spec.Versions, apixv1.CustomResourceDefinitionVersion{
			Name:    v,
			Served:  true,
			Storage: i == 0,
		})
	}
	return out
}
