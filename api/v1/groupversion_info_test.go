/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestGroupVersion(t *testing.T) {
	if GroupVersion.Group != "fathom.skaphos.io" {
		t.Errorf("GroupVersion.Group = %q, want %q", GroupVersion.Group, "fathom.skaphos.io")
	}
	if GroupVersion.Version != "v1" {
		t.Errorf("GroupVersion.Version = %q, want %q", GroupVersion.Version, "v1")
	}
}

// TestAddToScheme verifies that every Kind registered via init() is wired
// into a fresh scheme by AddToScheme: exactly the seven stable check kinds.
func TestAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	kinds := []runtime.Object{
		&AddonCheck{}, &AddonCheckList{},
		&HealthCheck{}, &HealthCheckList{},
		&ClusterHealth{}, &ClusterHealthList{},
		&HealthReport{}, &HealthReportList{},
		&NodeCertificateCheck{}, &NodeCertificateCheckList{},
		&DNSCheck{}, &DNSCheckList{},
		&NodeHealthCheck{}, &NodeHealthCheckList{},
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

	// The definition kinds stay alpha: nothing in this group version may
	// register under their names.
	for _, kind := range []string{"AddonDefinition", "AddonDefinitionBinding"} {
		if scheme.Recognizes(GroupVersion.WithKind(kind)) {
			t.Errorf("%s must not be registered under %s", kind, GroupVersion)
		}
	}
}

// TestSchemeBuilderRegisterReturnsSelf guards against accidental nil-receiver
// regressions in the local builder type.
func TestSchemeBuilderRegisterReturnsSelf(t *testing.T) {
	b := &builder{groupVersion: GroupVersion}
	got := b.Register(&HealthCheck{})
	if got != b {
		t.Errorf("Register returned %p, want %p", got, b)
	}
	scheme := runtime.NewScheme()
	if err := b.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	if _, _, err := scheme.ObjectKinds(&HealthCheck{}); err != nil {
		t.Fatalf("ObjectKinds: %v", err)
	}
	// metav1 list/watch types should also have been pulled in via
	// metav1.AddToGroupVersion inside Register.
	if _, _, err := scheme.ObjectKinds(&metav1.ListOptions{}); err != nil {
		t.Fatalf("ListOptions not registered: %v", err)
	}
}

// TestDeepCopyIntoExercise asserts the generated DeepCopyInto helpers copy
// nested fields (the shape of the call site counts toward coverage).
func TestDeepCopyIntoExercise(t *testing.T) {
	src := &HealthCheck{Spec: HealthCheckSpec{
		CheckRef: CheckTargetRef{Kind: "AddonCheck", Name: "x"},
	}}
	dst := &HealthCheck{}
	src.DeepCopyInto(dst)
	if dst.Spec.CheckRef.Name != "x" {
		t.Errorf("DeepCopyInto did not copy Spec.CheckRef.Name")
	}

	srcList := &HealthCheckList{Items: []HealthCheck{{Spec: HealthCheckSpec{
		CheckRef: CheckTargetRef{Kind: "AddonCheck", Name: "y"},
	}}}}
	dstList := &HealthCheckList{}
	srcList.DeepCopyInto(dstList)
	if len(dstList.Items) != 1 || dstList.Items[0].Spec.CheckRef.Name != "y" {
		t.Errorf("DeepCopyInto did not copy Items")
	}
}
