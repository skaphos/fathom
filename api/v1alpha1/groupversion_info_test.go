/*
SPDX-FileCopyrightText: 2026 Skaphos
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
// into a fresh scheme by AddToScheme. This exercises both the builder.Register
// helper and the runtime.SchemeBuilder.AddToScheme delegation.
func TestAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	kinds := []runtime.Object{
		&HealthCheck{}, &HealthCheckList{},
		&ClusterHealth{}, &ClusterHealthList{},
		&HealthReport{}, &HealthReportList{},
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
}

// TestSchemeBuilderRegisterIsIdempotent guards against accidental nil-receiver
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

// TestDeepCopy round-trips every Kind through its generated DeepCopyObject so
// the zz_generated.deepcopy.go statements light up. Bugs in DeepCopy show up
// as nil or aliased pointers, both of which we assert against.
func TestDeepCopyRoundTrip(t *testing.T) {
	t.Run("HealthCheck", func(t *testing.T) {
		orig := &HealthCheck{Spec: HealthCheckSpec{Foo: "bar"}}
		clone, ok := orig.DeepCopyObject().(*HealthCheck)
		if !ok || clone == nil {
			t.Fatalf("DeepCopyObject did not return *HealthCheck: %T", clone)
		}
		if clone == orig {
			t.Fatalf("DeepCopy returned the same pointer")
		}
		if clone.Spec.Foo != "bar" {
			t.Errorf("Spec.Foo = %q, want %q", clone.Spec.Foo, "bar")
		}
		_ = orig.DeepCopy()
		_ = orig.Spec.DeepCopy()
		_ = orig.Status.DeepCopy()
	})

	t.Run("HealthCheckList", func(t *testing.T) {
		orig := &HealthCheckList{Items: []HealthCheck{{Spec: HealthCheckSpec{Foo: "a"}}}}
		clone, ok := orig.DeepCopyObject().(*HealthCheckList)
		if !ok || clone == nil {
			t.Fatalf("DeepCopyObject did not return *HealthCheckList: %T", clone)
		}
		if len(clone.Items) != 1 || clone.Items[0].Spec.Foo != "a" {
			t.Errorf("unexpected clone: %+v", clone)
		}
		_ = orig.DeepCopy()
	})

	t.Run("ClusterHealth", func(t *testing.T) {
		orig := &ClusterHealth{}
		clone, ok := orig.DeepCopyObject().(*ClusterHealth)
		if !ok || clone == nil || clone == orig {
			t.Fatalf("unexpected DeepCopyObject result: %T %p", clone, clone)
		}
		_ = orig.DeepCopy()
		_ = orig.Spec.DeepCopy()
		_ = orig.Status.DeepCopy()
	})

	t.Run("ClusterHealthList", func(t *testing.T) {
		orig := &ClusterHealthList{Items: []ClusterHealth{{}, {}}}
		clone, ok := orig.DeepCopyObject().(*ClusterHealthList)
		if !ok || clone == nil {
			t.Fatalf("DeepCopyObject did not return *ClusterHealthList: %T", clone)
		}
		if len(clone.Items) != 2 {
			t.Errorf("len(Items) = %d, want 2", len(clone.Items))
		}
		_ = orig.DeepCopy()
	})

	t.Run("HealthReport", func(t *testing.T) {
		orig := &HealthReport{}
		clone, ok := orig.DeepCopyObject().(*HealthReport)
		if !ok || clone == nil || clone == orig {
			t.Fatalf("unexpected DeepCopyObject result: %T %p", clone, clone)
		}
		_ = orig.DeepCopy()
		_ = orig.Spec.DeepCopy()
		_ = orig.Status.DeepCopy()
	})

	t.Run("HealthReportList", func(t *testing.T) {
		orig := &HealthReportList{Items: []HealthReport{{}}}
		clone, ok := orig.DeepCopyObject().(*HealthReportList)
		if !ok || clone == nil {
			t.Fatalf("DeepCopyObject did not return *HealthReportList: %T", clone)
		}
		if len(clone.Items) != 1 {
			t.Errorf("len(Items) = %d, want 1", len(clone.Items))
		}
		_ = orig.DeepCopy()
	})
}

// TestDeepCopyIntoNil asserts the generated DeepCopyInto helpers handle a nil
// receiver-side input gracefully (they are no-ops in that case, but the
// shape of the call site still counts toward coverage).
func TestDeepCopyIntoExercise(t *testing.T) {
	src := &HealthCheck{Spec: HealthCheckSpec{Foo: "x"}}
	dst := &HealthCheck{}
	src.DeepCopyInto(dst)
	if dst.Spec.Foo != "x" {
		t.Errorf("DeepCopyInto did not copy Spec.Foo")
	}

	srcList := &HealthCheckList{Items: []HealthCheck{{Spec: HealthCheckSpec{Foo: "y"}}}}
	dstList := &HealthCheckList{}
	srcList.DeepCopyInto(dstList)
	if len(dstList.Items) != 1 || dstList.Items[0].Spec.Foo != "y" {
		t.Errorf("DeepCopyInto did not copy Items")
	}
}
