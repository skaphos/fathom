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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestInstallFreshCluster(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := newLifecycleEnv(t)
	out, _, err := execVerb(env.f, "install", "--create-namespace")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	// CRDs first, then the release (FR-010).
	crd, rel := env.log.index("create CustomResourceDefinition"), env.log.index("release:install")
	if crd < 0 || rel < 0 || crd > rel {
		t.Errorf("order = %v, want every CRD before the release", env.log.all())
	}
	if !strings.Contains(strings.Join(env.log.all(), "\n"), "release:install fathom-system/fathom 0.6.0") {
		t.Errorf("release not installed into fathom-system (default namespace): %v", env.log.all())
	}
	rel0 := env.rc.releases["fathom-system/fathom"]
	if rel0 == nil {
		t.Fatal("no release recorded")
	}
	if got, _ := lookupValue(rel0.Config, []string{"image", "digest"}); got != fakeOperatorDigest {
		t.Errorf("release values carry operator digest %v, want %s (FR-011)", got, fakeOperatorDigest)
	}
	for _, want := range []string{"v0.6.0", "fathom-system", "@" + fakeOperatorDigest, "digest"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
	crds := &apiextensionsv1.CustomResourceDefinitionList{}
	_ = env.c.List(context.Background(), crds)
	if len(crds.Items) != 9 {
		t.Errorf("CRDs = %d, want 9", len(crds.Items))
	}
	for _, c := range crds.Items {
		if c.Annotations[crdVersionAnnotation] != "0.6.0" {
			t.Errorf("%s not stamped with the version (FR-015)", c.Name)
		}
	}
}

func TestInstallSameVersionChangesNothing(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := newLifecycleEnv(t, append(liveCRDs(t, "0.6.0", nil), namespaceObj("fathom-system"))...)
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0"}, true)
	before := len(env.log.all())
	out, _, err := execVerb(env.f, "install")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out, "already installed at v0.6.0; nothing to do") {
		t.Errorf("output = %q", out)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("idempotent install wrote %v", w)
	}
}

func TestInstallRefusals(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(t *testing.T, env *lifecycleEnv)
		objs     func(t *testing.T) []client.Object
		args     []string
		version  string
		reason   string
		mentions []string
	}{
		{
			name:     "older install",
			setup:    func(_ *testing.T, env *lifecycleEnv) { seedRelease(env, "0.5.1", releaseDeployed) },
			reason:   reasonUseUpgrade,
			mentions: []string{"next: fathomctl upgrade -n fathom-system"},
		},
		{
			name:     "newer install",
			setup:    func(_ *testing.T, env *lifecycleEnv) { seedRelease(env, "0.7.0", releaseDeployed) },
			reason:   reasonDowngrade,
			mentions: []string{"fathomctl v0.7.0"},
		},
		{
			name: "kustomize install",
			objs: func(*testing.T) []client.Object {
				return []client.Object{managerDeployment("fathom-system", map[string]string{managedByKey: "kustomize"}, true)}
			},
			reason:   reasonNotHelm,
			mentions: []string{"kustomize"},
		},
		{
			name: "olm install",
			objs: func(*testing.T) []client.Object {
				return []client.Object{managerDeployment("operators", map[string]string{"olm.owner": "x"}, true)}
			},
			reason:   reasonNotHelm,
			mentions: []string{"OLM"},
		},
		{
			name:     "namespace missing",
			reason:   reasonNamespaceMissing,
			mentions: []string{"--create-namespace"},
		},
		{
			name:     "invalid values",
			objs:     func(*testing.T) []client.Object { return []client.Object{namespaceObj("fathom-system")} },
			args:     []string{"--set", "leaderElect=sometimes"},
			reason:   "leaderElect",
			mentions: nil,
		},
		{
			name:     "image override",
			objs:     func(*testing.T) []client.Object { return []client.Object{namespaceObj("fathom-system")} },
			args:     []string{"--set", "image.tag=v9.9.9"},
			reason:   "image values cannot be overridden",
			mentions: []string{"image.tag", "its own release's images"},
		},
		{
			name: "pending release",
			setup: func(_ *testing.T, env *lifecycleEnv) {
				seedRelease(env, "0.6.0", "pending-install")
			},
			reason: reasonReleasePending,
			// A first install has nothing to roll back to (F6).
			mentions: []string{"next: fathomctl uninstall -n fathom-system"},
		},
		{
			name:     "missing permissions",
			objs:     func(*testing.T) []client.Object { return []client.Object{namespaceObj("fathom-system")} },
			setup:    func(_ *testing.T, env *lifecycleEnv) { env.denied["create customresourcedefinitions"] = true },
			reason:   reasonMissingPermissions,
			mentions: []string{"create customresourcedefinitions"},
		},
		{
			name: "two releases",
			setup: func(_ *testing.T, env *lifecycleEnv) {
				env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: "0.6.0"}, true)
				env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-b", ChartVersion: "0.6.0"}, true)
			},
			reason:   reasonMultipleInstalls,
			mentions: []string{"fathom-b/fathom", "fathom-system/fathom"},
		},
		{
			name:     "dev build",
			version:  "devel+abc123",
			reason:   reasonDevVersion,
			mentions: []string{"FATHOMCTL_VERSION"},
		},
		// Leftover CRDs from an earlier plain uninstall (FR-014).
		{
			name: "leftover CRDs from a newer release",
			objs: func(t *testing.T) []client.Object {
				return append(liveCRDs(t, "0.7.0", nil), namespaceObj("fathom-system"))
			},
			reason:   reasonCRDDowngrade,
			mentions: []string{"0.7.0"},
		},
		{
			name: "leftover CRDs below the floor",
			objs: func(t *testing.T) []client.Object {
				return append(liveCRDs(t, "0.4.1", nil), namespaceObj("fathom-system"))
			},
			reason: reasonBelowFloor,
		},
		{
			name: "leftover CRDs without a recorded version and an incompatible change",
			objs: func(t *testing.T) []client.Object {
				return append(liveCRDs(t, "", addLegacyProperty), namespaceObj("fathom-system"))
			},
			reason:   reasonUnsanctioned,
			mentions: []string{dnsCRD},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			version := tc.version
			if version == "" {
				version = "v0.6.0"
			}
			withVersion(t, version, true)
			var objs []client.Object
			if tc.objs != nil {
				objs = tc.objs(t)
			}
			env := newLifecycleEnv(t, objs...)
			if tc.setup != nil {
				tc.setup(t, env)
			}
			// Seeding writes; anything logged after this point is the verb's.
			before := len(env.log.all())
			_, _, err := execVerb(env.f, "install", tc.args...)
			if writes := env.log.all()[before:]; len(writes) != 0 {
				t.Errorf("refusal changed the cluster: %v", writes)
			}
			if err == nil {
				t.Fatalf("install succeeded; want %s", tc.reason)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("err = %v, want %s", err, tc.reason)
			}
			for _, m := range tc.mentions {
				if !strings.Contains(err.Error(), m) {
					t.Errorf("error does not mention %q:\n%v", m, err)
				}
			}
		})
	}
}

// No refusal case writes anything: every check happens before the first
// change (contract: "Every refusal happens before the cluster is changed").
func TestInstallRefusalsWriteNothing(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	for name, objs := range map[string][]client.Object{
		"downgrade CRDs": append(liveCRDs(t, "0.7.0", nil), namespaceObj("fathom-system")),
		"no namespace":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			env := newLifecycleEnv(t, objs...)
			_, _, err := execVerb(env.f, "install")
			if err == nil {
				t.Fatal("install succeeded")
			}
			if w := env.clusterWrites(); len(w) != 0 {
				t.Errorf("refused install wrote %v", w)
			}
		})
	}
}

func TestInstallOntoCompatibleLeftoverCRDsKeepsResources(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	check := addonCheck("team-a", "coredns")
	env := newLifecycleEnv(t, append(liveCRDs(t, "0.5.1", nil), namespaceObj("fathom-system"), check)...)
	if _, _, err := execVerb(env.f, "install"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := env.c.Get(context.Background(), client.ObjectKeyFromObject(check), check); err != nil {
		t.Errorf("existing check resource lost: %v", err)
	}
}

func TestInstallWithAcceptedSanctionedChange(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	objs := append(liveCRDs(t, "0.5.1", unboundClusterHealthChildren), namespaceObj("fathom-system"))
	env := newLifecycleEnv(t, objs...)
	_, _, err := execVerb(env.f, "install")
	if err == nil || !strings.Contains(err.Error(), reasonAckRequired) {
		t.Fatalf("err = %v, want acknowledgement-required", err)
	}
	if _, _, err := execVerb(env.f, "install", "--accept-incompatible-crds"); err != nil {
		t.Fatalf("install --accept-incompatible-crds: %v", err)
	}
}

func TestInstallRolloutTimeout(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := newLifecycleEnv(t, namespaceObj("fathom-system"))
	env.rc.operatorReady = false
	_, _, err := execVerb(env.f, "install", "--timeout", "30ms")
	if err == nil || !strings.Contains(err.Error(), "not Available within 30ms") || !strings.Contains(err.Error(), "Available=False") {
		t.Errorf("err = %v, want a timeout naming the unmet condition", err)
	}
	if err != nil && !strings.Contains(err.Error(), "kubectl -n fathom-system describe deploy/") {
		t.Errorf("no recovery commands in %v", err)
	}
}

func TestInstallDevelopmentBuildPinsByTag(t *testing.T) {
	withVersion(t, "v0.6.0-rc.1", false)
	env := newLifecycleEnv(t, namespaceObj("fathom-system"))
	out, _, err := execVerb(env.f, "install")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out, "tag-pinned (development build)") || !strings.Contains(out, operatorRepository+":v0.6.0-rc.1") {
		t.Errorf("summary = %q, want a tag-pinned development build", out)
	}
}

func TestInstallValuesFile(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := newLifecycleEnv(t, namespaceObj("fathom-system"))
	path := filepath.Join(t.TempDir(), "values.yaml")
	if err := os.WriteFile(path, []byte("leaderElect: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := execVerb(env.f, "install", "-f", path); err != nil {
		t.Fatalf("install -f: %v", err)
	}
	if got := env.rc.releases["fathom-system/fathom"].Config["leaderElect"]; got != false {
		t.Errorf("leaderElect = %v, want the values file's false", got)
	}
}

// The image repositories the CLI pins are the chart's own defaults.
func TestImageRepositoriesMatchTheChart(t *testing.T) {
	targets := map[string]string{"image": operatorRepository, "probeImage": probeRepository}
	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "helm", "fathom-operator", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for key, repo := range targets {
		if !strings.Contains(string(raw), key+":\n  repository: "+repo) {
			t.Errorf("values.yaml %s.repository is not %s", key, repo)
		}
	}
	if !strings.Contains(string(raw), "repository: "+nodeAgentRepository) {
		t.Errorf("values.yaml nodeAgent.image.repository is not %s", nodeAgentRepository)
	}
}

func seedRelease(env *lifecycleEnv, version, status string) {
	env.rc.seed(helmRelease{Name: "fathom", Namespace: "fathom-system", ChartVersion: version, Status: status}, true)
}

// addLegacyProperty gives dnschecks a property the target removed.
func addLegacyProperty(c *apiextensionsv1.CustomResourceDefinition) {
	if c.Name != dnsCRD {
		return
	}
	spec := crdSchema(c).Properties["spec"]
	spec.Properties["legacyField"] = apiextensionsv1.JSONSchemaProps{Type: "string"}
	crdSchema(c).Properties["spec"] = spec
}

// unboundClusterHealthChildren is the v0.5.x schema: status.children had no
// maxItems, which 0.6.0 sanctions adding.
func unboundClusterHealthChildren(c *apiextensionsv1.CustomResourceDefinition) {
	if c.Name != "clusterhealths.fathom.skaphos.io" {
		return
	}
	status := crdSchema(c).Properties["status"]
	children := status.Properties["children"]
	children.MaxItems = nil
	status.Properties["children"] = children
	crdSchema(c).Properties["status"] = status
}
