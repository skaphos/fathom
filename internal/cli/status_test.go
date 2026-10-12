/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func statusEnv(t *testing.T, version string, crds []client.Object, ready bool, status string) *lifecycleEnv {
	t.Helper()
	env := newLifecycleEnv(t, append(crds, namespaceObj("fathom-system"))...)
	env.rc.seed(helmRelease{
		Name: "fathom", Namespace: "fathom-system", ChartVersion: version, Revision: 3, Status: status,
		Labels: map[string]string{installedByLabel: installedByCLI},
		Config: map[string]any{"image": map[string]any{"repository": operatorRepository, "digest": fakeOperatorDigest}},
	}, ready)
	return env
}

func TestStatusHealthyInstall(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := statusEnv(t, "0.6.0", liveCRDs(t, "0.6.0", nil), true, "")
	before := len(env.log.all())
	out, _, err := execVerb(env.f, "status")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	for _, want := range []string{
		"installed (helm release fathom in fathom-system, revision 3, by fathomctl)",
		"Version:    v0.6.0 (images pinned by digest)",
		"Operator:   Available",
		"CRDs:       9/9 current",
		"Upgrade to this fathomctl (v0.6.0): none needed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("status wrote %v; it must be read-only", w)
	}
}

func TestStatusNotInstalled(t *testing.T) {
	env := newLifecycleEnv(t)
	out, _, err := execVerb(env.f, "status")
	if err != nil || !strings.Contains(out, "not installed") {
		t.Errorf("status = %q, %v; want not installed and exit 0", out, err)
	}
}

func TestStatusFailures(t *testing.T) {
	cases := map[string]struct {
		env  func(t *testing.T) *lifecycleEnv
		want []string
	}{
		"outdated CRDs": {
			env: func(t *testing.T) *lifecycleEnv {
				return statusEnv(t, "0.6.0", liveCRDs(t, "0.5.1", nil), true, "")
			},
			want: []string{"CRDs:       0/9 current", dnsCRD, "outdated"},
		},
		"newer CRDs": {
			env: func(t *testing.T) *lifecycleEnv {
				return statusEnv(t, "0.6.0", liveCRDs(t, "0.7.0", nil), true, "")
			},
			want: []string{"newer"},
		},
		"missing CRDs": {
			env:  func(t *testing.T) *lifecycleEnv { return statusEnv(t, "0.6.0", nil, true, "") },
			want: []string{"missing"},
		},
		"not available": {
			env: func(t *testing.T) *lifecycleEnv {
				return statusEnv(t, "0.6.0", liveCRDs(t, "0.6.0", nil), false, "")
			},
			want: []string{"Operator:   not Available", "Available=False"},
		},
		"pending release": {
			env: func(t *testing.T) *lifecycleEnv {
				return statusEnv(t, "0.6.0", liveCRDs(t, "0.6.0", nil), true, "pending-upgrade")
			},
			want: []string{"pending-upgrade"},
		},
		"failed release": {
			env: func(t *testing.T) *lifecycleEnv {
				return statusEnv(t, "0.6.0", liveCRDs(t, "0.6.0", nil), true, "failed")
			},
			want: []string{"failed"},
		},
		"two releases": {
			env: func(t *testing.T) *lifecycleEnv {
				env := statusEnv(t, "0.6.0", liveCRDs(t, "0.6.0", nil), true, "")
				env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-b", ChartVersion: "0.6.0"}, true)
				return env
			},
			want: []string{"fathom-b/fathom", "fathom-system/fathom"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			withVersion(t, "v0.6.0", true)
			out, _, err := execVerb(tc.env(t).f, "status")
			if err == nil {
				t.Fatalf("status exited 0:\n%s", out)
			}
			all := out + err.Error()
			for _, w := range tc.want {
				if !strings.Contains(all, w) {
					t.Errorf("status missing %q:\n%s", w, all)
				}
			}
		})
	}
}

// CRD currency comes from the recorded version, so a CLI older than the
// install still judges it correctly (spec Edge "Version skew").
func TestStatusWithAnOlderCLI(t *testing.T) {
	withVersion(t, "v0.5.1", true)
	env := statusEnv(t, "0.6.0", liveCRDs(t, "0.6.0", nil), true, "")
	out, _, err := execVerb(env.f, "status")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "9/9 current") || !strings.Contains(out, "use fathomctl v0.6.0") {
		t.Errorf("status = %s", out)
	}
}

// CRDs helm applied carry no version: comparable only when this CLI is the
// operator's release; otherwise unknown, reported but not a failure.
func TestStatusUnrecordedCRDs(t *testing.T) {
	t.Run("same release compares the schema", func(t *testing.T) {
		withVersion(t, "v0.6.0", true)
		env := statusEnv(t, "0.6.0", liveCRDs(t, "", nil), true, "")
		out, _, err := execVerb(env.f, "status")
		if err != nil || !strings.Contains(out, "9/9 current") {
			t.Errorf("status = %s, %v", out, err)
		}
	})
	t.Run("other release is unknown", func(t *testing.T) {
		withVersion(t, "v0.7.0", true)
		env := statusEnv(t, "0.6.0", liveCRDs(t, "", nil), true, "")
		out, _, err := execVerb(env.f, "status")
		if err != nil {
			t.Fatalf("unknown CRDs failed status: %v\n%s", err, out)
		}
		if !strings.Contains(out, "unknown") || !strings.Contains(out, "0/9 current") {
			t.Errorf("status = %s", out)
		}
	})
}

func TestStatusKustomizeInstall(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := newLifecycleEnv(t, append(liveCRDs(t, "", nil), managerDeployment("fathom-system", map[string]string{managedByKey: "kustomize", versionLabel: "0.5.1"}, true))...)
	out, _, _ := execVerb(env.f, "status")
	if !strings.Contains(out, "not managed by fathomctl") || !strings.Contains(out, "v0.5.1") || !strings.Contains(out, "kustomize") {
		t.Errorf("status = %s", out)
	}
}

func TestStatusDevelopmentBuild(t *testing.T) {
	withVersion(t, "v0.6.0", false)
	env := newLifecycleEnv(t, append(liveCRDs(t, "0.6.0", nil), namespaceObj("fathom-system"))...)
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0"}, true)
	out, _, err := execVerb(env.f, "status")
	if err != nil || !strings.Contains(out, "tag-pinned (development build)") {
		t.Errorf("status = %s, %v", out, err)
	}
}

// Multiple installs are reported in the requested format, still exiting 1.
func TestStatusStructuredOutputWithTwoReleases(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			withVersion(t, "v0.6.0", true)
			env := statusEnv(t, "0.6.0", liveCRDs(t, "0.6.0", nil), true, "")
			env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-b", ChartVersion: "0.6.0"}, true)
			out, _, err := execVerb(env.f, "status", "-o", format)
			if err == nil {
				t.Fatalf("status exited 0:\n%s", out)
			}
			raw := []byte(out)
			if format == "yaml" {
				if raw, err = yaml.YAMLToJSON(raw); err != nil {
					t.Fatalf("%v\n%s", err, out)
				}
			}
			var report struct {
				Installed bool     `json:"installed"`
				Problems  []string `json:"problems"`
			}
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatalf("decode: %v\n%s", err, out)
			}
			if !report.Installed || len(report.Problems) != 1 || !strings.Contains(report.Problems[0], "fathom-b/fathom") {
				t.Errorf("report = %+v", report)
			}
		})
	}
}

func TestStatusStructuredOutput(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			withVersion(t, "v0.7.0", true)
			env := statusEnv(t, "0.6.0", liveCRDs(t, "0.6.0", func(c *apiextensionsv1.CustomResourceDefinition) {
				if c.Name == dnsCRD {
					c.Annotations[crdVersionAnnotation] = "0.5.1"
				}
			}), true, "")
			out, _, _ := execVerb(env.f, "status", "-o", format)
			raw := []byte(out)
			if format == "yaml" {
				var err error
				if raw, err = yaml.YAMLToJSON(raw); err != nil {
					t.Fatalf("%v\n%s", err, out)
				}
			}
			var report struct {
				ManagedBy   string `json:"managedBy"`
				Release     string `json:"release"`
				Version     string `json:"version"`
				Pinning     string `json:"pinning"`
				Ready       bool   `json:"ready"`
				Installed   bool   `json:"installed"`
				UpgradePath string `json:"upgradePath"`
				CRDs        []struct {
					Name, Scope, RecordedVersion, State string
				} `json:"crds"`
			}
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatalf("decode: %v\n%s", err, out)
			}
			if report.ManagedBy != "helm" || report.Release != "fathom" || report.Version != "0.6.0" || !report.Installed || !report.Ready || report.Pinning != "digest" {
				t.Errorf("report = %+v", report)
			}
			if report.UpgradePath != "direct" {
				t.Errorf("upgradePath = %q, want direct", report.UpgradePath)
			}
			states := map[string]string{}
			for _, c := range report.CRDs {
				states[c.Name] = c.State
			}
			if states[dnsCRD] != crdOutdated || states["healthchecks.fathom.skaphos.io"] != crdCurrent || len(report.CRDs) != 9 {
				t.Errorf("crds = %+v", report.CRDs)
			}
		})
	}
}
