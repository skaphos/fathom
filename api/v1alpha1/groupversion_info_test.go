/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
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
		&AddonCheck{}, &AddonCheckList{},
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
	t.Run("AddonCheck", func(t *testing.T) {
		orig := &AddonCheck{Spec: AddonCheckSpec{
			AddonType: "cert-manager",
			Policy: map[string]AddonCheckFamilyPolicy{
				"system_health": {Enabled: ptr.To(true), Thresholds: map[string]string{"warnDays": "14"}},
			},
		}}
		clone, ok := orig.DeepCopyObject().(*AddonCheck)
		if !ok || clone == nil {
			t.Fatalf("DeepCopyObject did not return *AddonCheck: %T", clone)
		}
		if clone == orig {
			t.Fatalf("DeepCopy returned the same pointer")
		}
		clone.Spec.Policy["system_health"].Thresholds["warnDays"] = "7"
		if orig.Spec.Policy["system_health"].Thresholds["warnDays"] != "14" {
			t.Errorf("DeepCopy aliased nested policy thresholds")
		}
		_ = orig.DeepCopy()
		_ = orig.Spec.DeepCopy()
		_ = orig.Status.DeepCopy()
	})

	t.Run("AddonCheckList", func(t *testing.T) {
		orig := &AddonCheckList{Items: []AddonCheck{{Spec: AddonCheckSpec{AddonType: "cert-manager"}}}}
		clone, ok := orig.DeepCopyObject().(*AddonCheckList)
		if !ok || clone == nil {
			t.Fatalf("DeepCopyObject did not return *AddonCheckList: %T", clone)
		}
		if len(clone.Items) != 1 || clone.Items[0].Spec.AddonType != "cert-manager" {
			t.Errorf("unexpected clone: %+v", clone)
		}
		_ = orig.DeepCopy()
	})

	t.Run("HealthCheck", func(t *testing.T) {
		orig := &HealthCheck{Spec: HealthCheckSpec{
			CheckRef: CheckTargetRef{Kind: "AddonCheck", Name: "cert-manager"},
		}}
		clone, ok := orig.DeepCopyObject().(*HealthCheck)
		if !ok || clone == nil {
			t.Fatalf("DeepCopyObject did not return *HealthCheck: %T", clone)
		}
		if clone == orig {
			t.Fatalf("DeepCopy returned the same pointer")
		}
		if clone.Spec.CheckRef.Name != "cert-manager" {
			t.Errorf("Spec.CheckRef.Name = %q, want %q", clone.Spec.CheckRef.Name, "cert-manager")
		}
		_ = orig.DeepCopy()
		_ = orig.Spec.DeepCopy()
		_ = orig.Status.DeepCopy()
	})

	t.Run("HealthCheckList", func(t *testing.T) {
		orig := &HealthCheckList{Items: []HealthCheck{{Spec: HealthCheckSpec{
			CheckRef: CheckTargetRef{Kind: "AddonCheck", Name: "a"},
		}}}}
		clone, ok := orig.DeepCopyObject().(*HealthCheckList)
		if !ok || clone == nil {
			t.Fatalf("DeepCopyObject did not return *HealthCheckList: %T", clone)
		}
		if len(clone.Items) != 1 || clone.Items[0].Spec.CheckRef.Name != "a" {
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
		orig := &HealthReport{Spec: HealthReportSpec{
			SourceRef: HealthReportTargetRef{Kind: "AddonCheck", Name: "cert-manager"},
			Result:    HealthReportResultPass,
			Checks: []HealthReportCheck{{
				Family:    "system_health",
				Result:    HealthReportResultPass,
				TargetRef: HealthReportTargetRef{Kind: "Deployment", Name: "cert-manager"},
				Details:   map[string]string{"available": "true"},
			}},
		}}
		clone, ok := orig.DeepCopyObject().(*HealthReport)
		if !ok || clone == nil || clone == orig {
			t.Fatalf("unexpected DeepCopyObject result: %T %p", clone, clone)
		}
		clone.Spec.Checks[0].Details["available"] = "false"
		if orig.Spec.Checks[0].Details["available"] != "true" {
			t.Errorf("DeepCopy aliased nested check details")
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
