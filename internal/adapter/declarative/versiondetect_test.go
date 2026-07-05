/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/skaphos/fathom/pkg/adapter"
)

func TestImageTag(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"quay.io/cilium/cilium:v1.15.6", "v1.15.6"},
		{"quay.io/cilium/cilium:v1.15.6@sha256:deadbeef", "v1.15.6"},
		{"registry:5000/team/app:2.1.0", "2.1.0"}, // registry port + tag
		{"registry:5000/team/app", ""},            // registry port, no tag
		{"nginx:1.27", "1.27"},
		{"nginx", ""},            // no tag
		{"nginx:latest", ""},     // latest is not a usable version
		{"nginx@sha256:abc", ""}, // digest only, no tag
		{"repo:1.0.0@sha256:x", "1.0.0"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := imageTag(tc.ref); got != tc.want {
			t.Errorf("imageTag(%q) = %q, want %q", tc.ref, got, tc.want)
		}
	}
}

// deploymentWithTemplateVersion / MetaVersion / Image build a healthy Deployment
// (via deploymentInNamespace) carrying, respectively, the version label on the
// pod template, the version label on the workload metadata, or a container image.
func deploymentWithTemplateVersion(name, ns, version string) *appsv1.Deployment {
	d := deploymentInNamespace(name, ns)
	d.Spec.Template.Labels = map[string]string{versionLabel: version}
	return d
}

func deploymentWithMetaVersion(name, ns, version string) *appsv1.Deployment {
	d := deploymentInNamespace(name, ns)
	d.Labels = map[string]string{versionLabel: version}
	return d
}

func deploymentWithImage(name, ns, image string) *appsv1.Deployment {
	d := deploymentInNamespace(name, ns)
	d.Spec.Template.Spec.Containers = []corev1.Container{{Name: name, Image: image}}
	return d
}

func TestDetectAddonVersion(t *testing.T) {
	vs := &VersionSource{Kind: KindDeployment, Namespace: "ns", Name: "app"}
	ctx := context.Background()

	tests := []struct {
		name string
		obj  clientObject
		want string
	}{
		{"pod-template label", deploymentWithTemplateVersion("app", "ns", "1.2.3"), "1.2.3"},
		{"workload meta label", deploymentWithMetaVersion("app", "ns", "2.0.0"), "2.0.0"},
		{"image tag fallback", deploymentWithImage("app", "ns", "repo/app:v1.5.0"), "v1.5.0"},
		{"no version signal", deploymentInNamespace("app", "ns"), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newFakeClient(t, tc.obj)
			if got := detectAddonVersion(ctx, c, vs); got != tc.want {
				t.Errorf("detectAddonVersion = %q, want %q", got, tc.want)
			}
		})
	}

	// Pod-template label wins over both meta label and image tag.
	both := deploymentWithTemplateVersion("app", "ns", "3.3.3")
	both.Labels = map[string]string{versionLabel: "9.9.9"}
	both.Spec.Template.Spec.Containers = []corev1.Container{{Name: "app", Image: "repo/app:v8.8.8"}}
	if got := detectAddonVersion(ctx, newFakeClient(t, both), vs); got != "3.3.3" {
		t.Errorf("precedence: got %q, want 3.3.3 (pod-template label)", got)
	}

	// Absent workload -> "" (best-effort, no error).
	if got := detectAddonVersion(ctx, newFakeClient(t), vs); got != "" {
		t.Errorf("absent workload: got %q, want \"\"", got)
	}

	// DaemonSet source, image-tag fallback.
	ds := daemonSetInNamespace("agent", "kube-system", 1)
	ds.Spec.Template.Spec.Containers = []corev1.Container{{Name: "agent", Image: "quay.io/cilium/cilium:v1.16.1"}}
	dsVS := &VersionSource{Kind: KindDaemonSet, Namespace: "kube-system", Name: "agent"}
	if got := detectAddonVersion(ctx, newFakeClient(t, ds), dsVS); got != "v1.16.1" {
		t.Errorf("daemonset image: got %q, want v1.16.1", got)
	}
}

// versionEngine builds a minimal valid engine with the given VersionSource and
// supported range, so detectAndGateVersion can be exercised directly.
func versionEngine(t *testing.T, vs *VersionSource, supported string) *Engine {
	t.Helper()
	return MustEngine(AddonDefinition{
		AddonType:         "app",
		AdapterVersion:    "0.0.1",
		VersionSource:     vs,
		SupportedVersions: supported,
		Families: []FamilyDefinition{{
			Name:           "system_health",
			DefaultEnabled: true,
			Workloads:      []WorkloadCheck{{Kind: KindDeployment, DefaultNamespace: "ns", DefaultName: "app", Component: "app"}},
		}},
	})
}

func TestDetectAndGateVersion(t *testing.T) {
	ctx := context.Background()
	vs := &VersionSource{Kind: KindDeployment, Namespace: "ns", Name: "app"}

	t.Run("in range -> no gate", func(t *testing.T) {
		eng := versionEngine(t, vs, ">=1.0.0 <2.0.0")
		detected, gate := eng.detectAndGateVersion(ctx, newFakeClient(t, deploymentWithTemplateVersion("app", "ns", "1.5.0")))
		if detected != "1.5.0" || len(gate) != 0 {
			t.Fatalf("in-range: detected=%q gate=%v, want 1.5.0 / none", detected, gate)
		}
	})

	t.Run("out of range -> Warn UnsupportedAddonVersion", func(t *testing.T) {
		eng := versionEngine(t, vs, ">=1.0.0 <2.0.0")
		detected, gate := eng.detectAndGateVersion(ctx, newFakeClient(t, deploymentWithTemplateVersion("app", "ns", "2.5.0")))
		if detected != "2.5.0" || len(gate) != 1 {
			t.Fatalf("out-of-range: detected=%q gate=%v", detected, gate)
		}
		assertHasOutcome(t, gate, "Deployment", "app", adapter.OutcomeWarn, "outside the supported range")
		assertHasDetail(t, gate, "Deployment", "app", adapter.DetailVersionGate, adapter.ReasonUnsupportedAddonVersion)
		assertHasDetail(t, gate, "Deployment", "app", adapter.DetailDetectedVersion, "2.5.0")
		assertHasDetail(t, gate, "Deployment", "app", adapter.DetailSupportedVersions, ">=1.0.0 <2.0.0")
		if gate[0].Family != versionFamily {
			t.Errorf("gate family = %q, want %q", gate[0].Family, versionFamily)
		}
	})

	t.Run("undetectable with a range -> Warn VersionUnknown", func(t *testing.T) {
		eng := versionEngine(t, vs, ">=1.0.0")
		detected, gate := eng.detectAndGateVersion(ctx, newFakeClient(t, deploymentInNamespace("app", "ns")))
		if detected != "" || len(gate) != 1 {
			t.Fatalf("undetectable: detected=%q gate=%v", detected, gate)
		}
		assertHasOutcome(t, gate, "Deployment", "app", adapter.OutcomeWarn, "could not be detected")
		assertHasDetail(t, gate, "Deployment", "app", adapter.DetailVersionGate, adapter.ReasonVersionUnknown)
	})

	t.Run("unparseable version with a range -> Warn VersionUnknown, raw surfaced", func(t *testing.T) {
		eng := versionEngine(t, vs, ">=1.0.0")
		detected, gate := eng.detectAndGateVersion(ctx, newFakeClient(t, deploymentWithTemplateVersion("app", "ns", "nightly")))
		if detected != "nightly" || len(gate) != 1 {
			t.Fatalf("unparseable: detected=%q gate=%v", detected, gate)
		}
		assertHasDetail(t, gate, "Deployment", "app", adapter.DetailVersionGate, adapter.ReasonVersionUnknown)
		assertHasDetail(t, gate, "Deployment", "app", adapter.DetailDetectedVersion, "nightly")
	})

	t.Run("prerelease inside the range -> no gate (gated on base release)", func(t *testing.T) {
		eng := versionEngine(t, vs, ">=1.14.0 <2.0.0")
		detected, gate := eng.detectAndGateVersion(ctx, newFakeClient(t, deploymentWithTemplateVersion("app", "ns", "1.15.0-rc.1")))
		if detected != "1.15.0-rc.1" || len(gate) != 0 {
			t.Fatalf("prerelease in-range: detected=%q gate=%v, want 1.15.0-rc.1 / none", detected, gate)
		}
	})

	t.Run("prerelease whose base is out of range -> Warn", func(t *testing.T) {
		eng := versionEngine(t, vs, ">=1.0.0 <2.0.0")
		detected, gate := eng.detectAndGateVersion(ctx, newFakeClient(t, deploymentWithTemplateVersion("app", "ns", "2.0.0-rc.1")))
		if detected != "2.0.0-rc.1" || len(gate) != 1 {
			t.Fatalf("prerelease out-of-range: detected=%q gate=%v", detected, gate)
		}
		assertHasDetail(t, gate, "Deployment", "app", adapter.DetailVersionGate, adapter.ReasonUnsupportedAddonVersion)
	})

	t.Run("detection-only (empty range) -> detected, no gate", func(t *testing.T) {
		eng := versionEngine(t, vs, "")
		detected, gate := eng.detectAndGateVersion(ctx, newFakeClient(t, deploymentWithTemplateVersion("app", "ns", "9.9.9")))
		if detected != "9.9.9" || len(gate) != 0 {
			t.Fatalf("detect-only: detected=%q gate=%v, want 9.9.9 / none", detected, gate)
		}
	})

	t.Run("no VersionSource -> nothing", func(t *testing.T) {
		eng := versionEngine(t, nil, "")
		detected, gate := eng.detectAndGateVersion(ctx, newFakeClient(t, deploymentWithTemplateVersion("app", "ns", "1.0.0")))
		if detected != "" || len(gate) != 0 {
			t.Fatalf("no source: detected=%q gate=%v", detected, gate)
		}
	})
}

// TestEngine_SurfacesDetectedVersion checks the Run-level plumbing: a
// detection-only engine populates Result.DetectedVersion and emits no version
// gate.
func TestEngine_SurfacesDetectedVersion(t *testing.T) {
	eng := versionEngine(t, &VersionSource{Kind: KindDeployment, Namespace: "ns", Name: "app"}, "")
	res, err := eng.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, deploymentWithTemplateVersion("app", "ns", "3.1.4")),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.DetectedVersion != "3.1.4" {
		t.Errorf("Result.DetectedVersion = %q, want 3.1.4", res.DetectedVersion)
	}
	// Detection-only: no version-gate Warn under the synthetic family.
	for _, c := range res.Checks {
		if c.Family == versionFamily {
			t.Errorf("unexpected version-gate check in detection-only run: %+v", c)
		}
	}
}

func TestNewEngine_VersionValidation(t *testing.T) {
	base := func() AddonDefinition {
		return AddonDefinition{
			AddonType:      "app",
			AdapterVersion: "0.0.1",
			Families:       []FamilyDefinition{{Name: "system_health", DefaultEnabled: true}},
		}
	}

	t.Run("malformed range rejected", func(t *testing.T) {
		d := base()
		d.SupportedVersions = ">=not-a-version"
		if _, err := NewEngine(d); err == nil {
			t.Fatal("NewEngine accepted a malformed supportedVersions range")
		}
	})

	t.Run("valid range accepted", func(t *testing.T) {
		d := base()
		d.SupportedVersions = ">=1.0.0 <2.0.0"
		if _, err := NewEngine(d); err != nil {
			t.Fatalf("NewEngine rejected a valid range: %v", err)
		}
	})

	t.Run("VersionSource unknown kind rejected", func(t *testing.T) {
		d := base()
		d.VersionSource = &VersionSource{Kind: "Pod", Namespace: "ns", Name: "app"}
		if _, err := NewEngine(d); err == nil {
			t.Fatal("NewEngine accepted a VersionSource with an unknown kind")
		}
	})

	t.Run("VersionSource missing name rejected", func(t *testing.T) {
		d := base()
		d.VersionSource = &VersionSource{Kind: KindDeployment, Namespace: "ns"}
		if _, err := NewEngine(d); err == nil {
			t.Fatal("NewEngine accepted a VersionSource with no Name")
		}
	})
}
