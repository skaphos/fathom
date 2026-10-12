/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func crdByName(t *testing.T, name string) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	targets, err := targetCRDs()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range targets {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no embedded CRD %s", name)
	return nil
}

const dnsCRD = "dnschecks.fathom.skaphos.io"

func TestTargetCRDsAreTheNineShipped(t *testing.T) {
	targets, err := targetCRDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 9 {
		t.Errorf("embedded CRDs = %d, want 9", len(targets))
	}
}

func TestCompareCRD(t *testing.T) {
	target := crdByName(t, dnsCRD)
	cases := []struct {
		name      string
		mutate    func(live *apiextensionsv1.CustomResourceDefinition)
		wantCheck string
	}{
		{name: "identical", mutate: func(*apiextensionsv1.CustomResourceDefinition) {}},
		{
			// The live CRD has a property the target removed.
			name: "removed property",
			mutate: func(live *apiextensionsv1.CustomResourceDefinition) {
				spec := crdSchema(live).Properties["spec"]
				spec.Properties["legacyField"] = apiextensionsv1.JSONSchemaProps{Type: "string"}
				crdSchema(live).Properties["spec"] = spec
			},
			wantCheck: "existingFieldRemoval",
		},
		{
			name: "type change",
			mutate: func(live *apiextensionsv1.CustomResourceDefinition) {
				spec := crdSchema(live).Properties["spec"]
				p := spec.Properties["interval"]
				p.Type = "integer"
				p.Format, p.Pattern = "", ""
				spec.Properties["interval"] = p
				crdSchema(live).Properties["spec"] = spec
			},
			wantCheck: "type",
		},
		{
			name: "scope change",
			mutate: func(live *apiextensionsv1.CustomResourceDefinition) {
				live.Spec.Scope = apiextensionsv1.ClusterScoped
			},
			wantCheck: scopeCheck,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			live := target.DeepCopy()
			tc.mutate(live)
			findings, err := compareCRD(live, target)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantCheck == "" {
				if len(findings) != 0 {
					t.Errorf("findings = %+v, want none", findings)
				}
				return
			}
			found := false
			for _, f := range findings {
				if f.Check == tc.wantCheck {
					found = true
				}
			}
			if !found {
				t.Errorf("findings = %+v, want a %s finding", findings, tc.wantCheck)
			}
		})
	}
}

// A newly required field is incompatible: existing resources lack it.
func TestCompareCRDFlagsNewRequiredField(t *testing.T) {
	live := crdByName(t, dnsCRD)
	target := live.DeepCopy()
	spec := crdSchema(target).Properties["spec"]
	spec.Properties["newField"] = apiextensionsv1.JSONSchemaProps{Type: "string"}
	spec.Required = append(spec.Required, "newField")
	crdSchema(target).Properties["spec"] = spec
	findings, err := compareCRD(live, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Error("a newly required field produced no finding")
	}
}

// An added optional field is compatible: the plan updates, with no finding.
func TestPlanCRDsAddedOptionalFieldIsAnUpdate(t *testing.T) {
	older := liveCRDs(t, "0.5.1", func(c *apiextensionsv1.CustomResourceDefinition) {
		if c.Name != dnsCRD {
			return
		}
		spec := crdSchema(c).Properties["spec"]
		delete(spec.Properties, "timeout")
		crdSchema(c).Properties["spec"] = spec
	})
	env := newLifecycleEnv(t, older...)
	targets, _ := targetCRDs()
	plans, err := planCRDs(context.Background(), env.c, targets)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plans {
		want := crdUnchanged
		if p.Name == dnsCRD {
			want = crdUpdate
		}
		if p.Action != want || len(p.Findings) != 0 {
			t.Errorf("%s: action %s findings %+v, want %s and none", p.Name, p.Action, p.Findings, want)
		}
		if p.RecordedVersion != "0.5.1" {
			t.Errorf("%s: recordedVersion %q, want 0.5.1", p.Name, p.RecordedVersion)
		}
	}
}

func TestPlanCRDsMissingIsAdd(t *testing.T) {
	env := newLifecycleEnv(t)
	targets, _ := targetCRDs()
	plans, err := planCRDs(context.Background(), env.c, targets)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plans {
		if p.Action != crdAdd || p.RecordedVersion != "" {
			t.Errorf("%s: action %s recorded %q, want add and none", p.Name, p.Action, p.RecordedVersion)
		}
	}
}

// A CRD first created by another field manager still ends up exactly the
// target, stamped, with kubectl's last-applied record dropped (FR-015, FR-020).
func TestApplyCRDsReplacesForeignOwnedCRDsExactly(t *testing.T) {
	live := liveCRDs(t, "", func(c *apiextensionsv1.CustomResourceDefinition) {
		if c.Name != dnsCRD {
			return
		}
		c.Annotations = map[string]string{lastAppliedAnnotation: "{}", "team.example/owner": "platform"}
		c.ManagedFields = nil
		spec := crdSchema(c).Properties["spec"]
		spec.Properties["legacyField"] = apiextensionsv1.JSONSchemaProps{Type: "string"}
		crdSchema(c).Properties["spec"] = spec
		// A served version the target removed.
		old := c.Spec.Versions[0].DeepCopy()
		old.Name, old.Storage = "v1alpha0", false
		c.Spec.Versions = append(c.Spec.Versions, *old)
	})
	env := newLifecycleEnv(t, live...)
	targets, _ := targetCRDs()
	plans, err := planCRDs(context.Background(), env.c, targets)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyCRDs(context.Background(), env.f, env.c, plans, "0.6.0", time.Second); err != nil {
		t.Fatalf("applyCRDs: %v", err)
	}
	got := &apiextensionsv1.CustomResourceDefinition{}
	if err := env.c.Get(context.Background(), client.ObjectKey{Name: dnsCRD}, got); err != nil {
		t.Fatal(err)
	}
	if !crdSpecEqual(got, crdByName(t, dnsCRD)) {
		t.Error("live spec differs from the target after apply")
	}
	if _, ok := crdSchema(got).Properties["spec"].Properties["legacyField"]; ok {
		t.Error("removed property survived the apply")
	}
	if len(got.Spec.Versions) != 1 {
		t.Errorf("versions = %d, want the removed version gone", len(got.Spec.Versions))
	}
	if got.Annotations[crdVersionAnnotation] != "0.6.0" {
		t.Errorf("version annotation = %q", got.Annotations[crdVersionAnnotation])
	}
	if _, ok := got.Annotations[lastAppliedAnnotation]; ok {
		t.Error("kubectl last-applied annotation kept")
	}
	if got.Annotations["team.example/owner"] != "platform" {
		t.Error("another tool's annotation was dropped")
	}
}

func TestApplyCRDsStampsUnchangedCRDsOnlyWhenTheVersionMoves(t *testing.T) {
	env := newLifecycleEnv(t, liveCRDs(t, "0.6.0", nil)...)
	targets, _ := targetCRDs()
	plans, _ := planCRDs(context.Background(), env.c, targets)
	if err := applyCRDs(context.Background(), env.f, env.c, plans, "0.6.0", time.Second); err != nil {
		t.Fatal(err)
	}
	if n := len(env.clusterWrites()); n != 0 {
		t.Errorf("unchanged CRDs at the same version were rewritten (%d writes)", n)
	}
}

func TestApplyCRDsTimesOutWaitingForEstablished(t *testing.T) {
	env := newLifecycleEnv(t)
	env.neverEstablish = true
	targets, _ := targetCRDs()
	plans, err := planCRDs(context.Background(), env.c, targets)
	if err != nil {
		t.Fatal(err)
	}
	err = applyCRDs(context.Background(), env.f, env.c, plans, "0.6.0", 20*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "not Established within 20ms") {
		t.Errorf("err = %v, want a bounded Established wait", err)
	}
}

// The CLI's crdify configuration is the CI gate's (.crdify.yaml): the same
// rules decide what a user's upgrade refuses and what CI refuses.
func TestCrdifyConfigMatchesTheRepositoryGate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".crdify.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var repo struct {
		Validations []struct {
			Name        string `json:"name"`
			Enforcement string `json:"enforcement"`
		} `json:"validations"`
		UnhandledEnforcement string `json:"unhandledEnforcement"`
	}
	if err := yaml.Unmarshal(raw, &repo); err != nil {
		t.Fatal(err)
	}
	cli := crdifyConfig()
	if string(cli.UnhandledEnforcement) != repo.UnhandledEnforcement {
		t.Errorf("unhandledEnforcement: cli %s, repo %s", cli.UnhandledEnforcement, repo.UnhandledEnforcement)
	}
	if len(cli.Validations) != len(repo.Validations) {
		t.Fatalf("validations: cli %+v, repo %+v", cli.Validations, repo.Validations)
	}
	for i, v := range repo.Validations {
		if cli.Validations[i].Name != v.Name || string(cli.Validations[i].Enforcement) != v.Enforcement {
			t.Errorf("validation %d: cli %+v, repo %+v", i, cli.Validations[i], v)
		}
	}
}
