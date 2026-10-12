/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	helmchart "github.com/skaphos/fathom/deploy/helm"
)

// upgradeEnv is a cluster with Fathom at version installed by Helm, its CRDs
// recorded at crdVersion and shaped by mutate.
func upgradeEnv(t *testing.T, installed, crdVersion string, mutate func(*apiextensionsv1.CustomResourceDefinition), config map[string]any) *lifecycleEnv {
	t.Helper()
	env := newLifecycleEnv(t, append(liveCRDs(t, crdVersion, mutate), namespaceObj("fathom-system"))...)
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: installed, Config: config}, true)
	return env
}

// withUpgradePath swaps the embedded declaration for one test.
func withUpgradePath(t *testing.T, raw string) {
	t.Helper()
	old := readUpgradePathFile
	readUpgradePathFile = func() ([]byte, error) { return []byte(raw), nil }
	t.Cleanup(func() { readUpgradePathFile = old })
}

func TestUpgradeCompatibleAppliesCRDsBeforeTheRollout(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := upgradeEnv(t, "0.5.1", "0.5.1", func(c *apiextensionsv1.CustomResourceDefinition) {
		if c.Name == dnsCRD {
			spec := crdSchema(c).Properties["spec"]
			delete(spec.Properties, "timeout")
			crdSchema(c).Properties["spec"] = spec
		}
	}, nil)
	before := len(env.log.all())
	out, _, err := execVerb(env.f, "upgrade")
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	events := env.log.all()[before:]
	crd, rel := -1, -1
	for i, e := range events {
		if strings.HasPrefix(e, "update CustomResourceDefinition") && crd < 0 {
			crd = i
		}
		if strings.HasPrefix(e, "release:upgrade") {
			rel = i
		}
	}
	if crd < 0 || rel < 0 || crd > rel {
		t.Errorf("events = %v, want CRD updates before the release upgrade (FR-020)", events)
	}
	for _, want := range []string{"Fathom upgrade plan: v0.5.1 -> v0.6.0", dnsCRD, "update", "Upgraded Fathom to v0.6.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestUpgradeRefusals(t *testing.T) {
	const stops = `
floor: 0.5.0
releases:
  - version: 0.6.0
    stop: true
`
	cases := []struct {
		name     string
		cli      string
		decl     string
		env      func(t *testing.T) *lifecycleEnv
		args     []string
		reason   string
		mentions []string
		absent   []string
	}{
		{
			name: "sanctioned incompatible without the flag",
			env: func(t *testing.T) *lifecycleEnv {
				return upgradeEnv(t, "0.5.1", "0.5.1", unboundClusterHealthChildren, nil)
			},
			reason:   reasonAckRequired,
			mentions: []string{"clusterhealths.fathom.skaphos.io ^.status.children", "issues/277", "next: fathomctl upgrade -n fathom-system --accept-incompatible-crds"},
		},
		{
			name: "--yes does not acknowledge",
			env: func(t *testing.T) *lifecycleEnv {
				return upgradeEnv(t, "0.5.1", "0.5.1", unboundClusterHealthChildren, nil)
			},
			args:   []string{"--set", "leaderElect=true"},
			reason: reasonAckRequired,
		},
		{
			name: "unsanctioned incompatible",
			env: func(t *testing.T) *lifecycleEnv {
				return upgradeEnv(t, "0.5.1", "0.5.1", addLegacyProperty, nil)
			},
			reason:   reasonUnsanctioned,
			mentions: []string{dnsCRD, "^.spec", "existingFieldRemoval"},
		},
		{
			name: "scope change",
			env: func(t *testing.T) *lifecycleEnv {
				return upgradeEnv(t, "0.5.1", "0.5.1", func(c *apiextensionsv1.CustomResourceDefinition) {
					if c.Name == dnsCRD {
						c.Spec.Scope = apiextensionsv1.ClusterScoped
					}
				}, nil)
			},
			args:     []string{"--accept-incompatible-crds"},
			reason:   reasonScopeChange,
			mentions: []string{"uninstall --purge-crds --export"},
		},
		{
			name:     "skipped stop",
			cli:      "v0.7.0",
			decl:     stops,
			env:      func(t *testing.T) *lifecycleEnv { return upgradeEnv(t, "0.5.1", "0.5.1", nil, nil) },
			args:     []string{"--accept-incompatible-crds"},
			reason:   reasonStopSkipped,
			mentions: []string{"Upgrade path: v0.5.1 -> v0.6.0 -> v0.7.0", `next: download fathomctl v0.6.0 and run "fathomctl upgrade -n fathom-system"`},
		},
		{
			name:     "below the floor",
			env:      func(t *testing.T) *lifecycleEnv { return upgradeEnv(t, "0.4.1", "", nil, nil) },
			reason:   reasonBelowFloor,
			mentions: []string{"v0.5.x", publishedChart},
		},
		{
			name:     "CLI older than the install",
			env:      func(t *testing.T) *lifecycleEnv { return upgradeEnv(t, "0.7.0", "0.7.0", nil, nil) },
			reason:   reasonDowngrade,
			mentions: []string{"fathomctl v0.7.0"},
		},
		{
			name:     "not installed",
			env:      func(t *testing.T) *lifecycleEnv { return newLifecycleEnv(t) },
			reason:   reasonNotInstalled,
			mentions: []string{"next: fathomctl install"},
		},
		{
			name: "pending release",
			env: func(t *testing.T) *lifecycleEnv {
				env := newLifecycleEnv(t)
				env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.5.1", Status: "pending-upgrade"}, true)
				return env
			},
			reason:   reasonReleasePending,
			mentions: []string{"helm rollback"},
		},
		{
			name: "kustomize install",
			env: func(t *testing.T) *lifecycleEnv {
				return newLifecycleEnv(t, managerDeployment("fathom-system", map[string]string{managedByKey: "kustomize"}, true))
			},
			reason:   reasonNotHelm,
			mentions: []string{"kustomize"},
		},
		{
			name: "missing permissions",
			env: func(t *testing.T) *lifecycleEnv {
				env := upgradeEnv(t, "0.5.1", "0.5.1", nil, nil)
				env.denied["update customresourcedefinitions"] = true
				return env
			},
			reason:   reasonMissingPermissions,
			mentions: []string{"update customresourcedefinitions"},
		},
		{
			name: "two releases",
			env: func(t *testing.T) *lifecycleEnv {
				env := upgradeEnv(t, "0.5.1", "0.5.1", nil, nil)
				env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-b", ChartVersion: "0.5.1"}, true)
				return env
			},
			reason: reasonMultipleInstalls,
		},
		{
			name:     "image override",
			env:      func(t *testing.T) *lifecycleEnv { return upgradeEnv(t, "0.5.1", "0.5.1", nil, nil) },
			args:     []string{"--set", "nodeAgent.image.digest=" + fakeOperatorDigest},
			reason:   "image values cannot be overridden",
			mentions: []string{"nodeAgent.image.digest"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cli := tc.cli
			if cli == "" {
				cli = "v0.6.0"
			}
			withVersion(t, cli, true)
			if tc.decl != "" {
				withUpgradePath(t, tc.decl)
			}
			env := tc.env(t)
			before := len(env.log.all())
			_, _, err := execVerb(env.f, "upgrade", tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("err = %v, want %s", err, tc.reason)
			}
			for _, m := range tc.mentions {
				if !strings.Contains(err.Error(), m) {
					t.Errorf("error does not mention %q:\n%v", m, err)
				}
			}
			if w := env.log.all()[before:]; len(w) != 0 {
				t.Errorf("refused upgrade changed the cluster: %v", w)
			}
		})
	}
}

func TestUpgradeAcceptsSanctionedChangesWithTheFlag(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := upgradeEnv(t, "0.5.1", "0.5.1", unboundClusterHealthChildren, nil)
	if _, _, err := execVerb(env.f, "upgrade", "--accept-incompatible-crds"); err != nil {
		t.Fatalf("upgrade --accept-incompatible-crds: %v", err)
	}
	got := &apiextensionsv1.CustomResourceDefinition{}
	if err := env.c.Get(context.Background(), client.ObjectKey{Name: "clusterhealths.fathom.skaphos.io"}, got); err != nil {
		t.Fatal(err)
	}
	if crdSchema(got).Properties["status"].Properties["children"].MaxItems == nil {
		t.Error("the sanctioned change was not applied")
	}
}

// Values the release was installed with survive an upgrade; --set wins over
// them; image values a previous revision recorded never survive (FR-007,
// FR-025).
func TestUpgradeValues(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	installed := map[string]any{
		"metrics":    map[string]any{"prometheusRule": map[string]any{"enabled": true}},
		"leaderElect": true,
		"image":      map[string]any{"repository": operatorRepository, "tag": "v0.5.1-custom"},
		"nodeAgent":  map[string]any{"image": map[string]any{"repository": nodeAgentRepository, "digest": fakeProbeDigest}},
	}
	env := upgradeEnv(t, "0.5.1", "0.5.1", nil, installed)
	if _, _, err := execVerb(env.f, "upgrade", "--set", "leaderElect=false"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	cfg := env.rc.releases["fathom-system/fathom"].Config
	if v, _ := lookupValue(cfg, []string{"metrics", "prometheusRule", "enabled"}); v != true {
		t.Errorf("installed value metrics.prometheusRule.enabled lost: %v", v)
	}
	if v, _ := lookupValue(cfg, []string{"leaderElect"}); v != false {
		t.Errorf("--set leaderElect=false did not win: %v", v)
	}
	if v, _ := lookupValue(cfg, []string{"image", "tag"}); v != "v0.6.0" {
		t.Errorf("image.tag = %v, want the CLI's v0.6.0", v)
	}
	if v, _ := lookupValue(cfg, []string{"image", "digest"}); v != fakeOperatorDigest {
		t.Errorf("image.digest = %v, want the CLI's", v)
	}
	if v, _ := lookupValue(cfg, []string{"nodeAgent", "image", "digest"}); v != fakeNodeAgentDigest {
		t.Errorf("nodeAgent.image.digest = %v, want the CLI's, not the previous release's", v)
	}
}

func TestUpgradeSameVersionIsANoOp(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := upgradeEnv(t, "0.6.0", "0.6.0", nil, nil)
	before := len(env.log.all())
	out, _, err := execVerb(env.f, "upgrade")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "nothing to do") {
		t.Errorf("output = %q", out)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("no-op upgrade wrote %v", w)
	}
}

func TestEmbeddedDeclarationIsTheDefault(t *testing.T) {
	raw, err := readUpgradePathFile()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := helmchart.ReadFile(helmchart.UpgradePathFile)
	if string(raw) != string(want) {
		t.Error("readUpgradePathFile is not the embedded declaration")
	}
}
