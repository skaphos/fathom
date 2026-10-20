/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestGroupVersion(t *testing.T) {
	if GroupVersion.Group != "fathom.skaphos.io" {
		t.Errorf("GroupVersion.Group = %q, want %q", GroupVersion.Group, "fathom.skaphos.io")
	}
	if GroupVersion.Version != "v1alpha1" {
		t.Errorf("GroupVersion.Version = %q, want %q", GroupVersion.Version, "v1alpha1")
	}
}

// TestAddToScheme verifies that every Kind registered via init() is wired
// into a fresh scheme by AddToScheme. Since the seven check kinds moved to
// the stable v1 package, this group version carries exactly the two runtime
// definition kinds.
func TestAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	kinds := []runtime.Object{
		&AddonDefinition{}, &AddonDefinitionList{},
		&AddonDefinitionBinding{}, &AddonDefinitionBindingList{},
	}
	for _, obj := range kinds {
		gvks, _, err := scheme.ObjectKinds(obj)
		if err != nil {
			t.Errorf("ObjectKinds(%T): %v", obj, err)
			continue
		}
		found := false
		for _, gvk := range gvks {
			if gvk.GroupVersion() == GroupVersion {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%T not registered under %s; got %v", obj, GroupVersion, gvks)
		}
	}

	for _, kind := range []string{"AddonCheck", "HealthCheck", "ClusterHealth", "HealthReport", "NodeCertificateCheck", "DNSCheck", "NodeHealthCheck"} {
		if scheme.Recognizes(GroupVersion.WithKind(kind)) {
			t.Errorf("%s is a stable v1 kind and must not be registered under %s", kind, GroupVersion)
		}
	}
}

// TestSchemeBuilderRegisterReturnsSelf guards against accidental nil-receiver
// regressions in the local builder type.
func TestSchemeBuilderRegisterReturnsSelf(t *testing.T) {
	b := &builder{groupVersion: GroupVersion}
	got := b.Register(&AddonDefinition{})
	if got != b {
		t.Errorf("Register returned %p, want %p", got, b)
	}
	scheme := runtime.NewScheme()
	if err := b.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	if _, _, err := scheme.ObjectKinds(&AddonDefinition{}); err != nil {
		t.Fatalf("ObjectKinds: %v", err)
	}
	// metav1 list/watch types should also have been pulled in via
	// metav1.AddToGroupVersion inside Register.
	if _, _, err := scheme.ObjectKinds(&metav1.ListOptions{}); err != nil {
		t.Fatalf("ListOptions not registered: %v", err)
	}
}
