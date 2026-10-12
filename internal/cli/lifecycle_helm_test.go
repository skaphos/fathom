/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"

	helmchart "github.com/skaphos/fathom/deploy/helm"
)

// memoryHelm is the production helmReleaseClient over Helm's in-memory
// release store and a kube client that only prints: the real action wiring,
// no cluster.
func memoryHelm(t *testing.T) *helmReleaseClient {
	t.Helper()
	mem := driver.NewMemory()
	return &helmReleaseClient{
		newConfig: func(namespace string) (*action.Configuration, error) {
			mem.SetNamespace(namespace)
			cfg := action.NewConfiguration(action.ConfigurationSetLogger(slog.NewTextHandler(io.Discard, nil)))
			cfg.Releases = storage.Init(mem)
			cfg.KubeClient = &kubefake.PrintingKubeClient{Out: io.Discard}
			cfg.Capabilities = offlineConfig().Capabilities
			return cfg, nil
		},
		renderConfig: offlineConfig,
	}
}

func TestHelmReleaseClientRoundTrip(t *testing.T) {
	ctx := context.Background()
	h := memoryHelm(t)
	req := releaseRequest{Name: "fathom", Namespace: "fathom-system", Version: "0.6.0", Values: imageValues("0.6.0"), Timeout: time.Second}

	installed, err := h.Install(ctx, req)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if installed.Revision != 1 || installed.Status != releaseDeployed || installed.AppVersion != "0.6.0" || installed.ChartName != chartName {
		t.Errorf("installed = %+v", installed)
	}
	if installed.Labels[installedByLabel] != installedByCLI {
		t.Errorf("labels = %v, want %s=%s", installed.Labels, installedByLabel, installedByCLI)
	}

	list, err := h.List(ctx, []string{"fathom-system", "elsewhere"})
	if err != nil || len(list) != 1 || list[0].Name != "fathom" {
		t.Fatalf("List = %+v, %v", list, err)
	}

	req.Version, req.ReuseValues = "0.7.0", true
	req.Values = mergeValues(map[string]any{"leaderElect": true}, imageValues("0.7.0"))
	upgraded, err := h.Upgrade(ctx, req)
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if upgraded.Revision != 2 || upgraded.AppVersion != "0.7.0" {
		t.Errorf("upgraded = %+v", upgraded)
	}

	if err := h.Rollback(ctx, "fathom", "fathom-system", time.Second); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	list, _ = h.List(ctx, []string{"fathom-system"})
	if len(list) != 1 || list[0].Revision != 3 || list[0].AppVersion != "0.6.0" {
		t.Errorf("after rollback = %+v, want revision 3 at 0.6.0", list)
	}

	if err := h.Uninstall(ctx, "fathom", "fathom-system", time.Second); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if list, _ = h.List(ctx, []string{"fathom-system"}); len(list) != 0 {
		t.Errorf("after uninstall List = %+v, want none", list)
	}
	// Uninstalling what is already gone is not an error.
	if err := h.Uninstall(ctx, "fathom", "fathom-system", time.Second); err != nil {
		t.Errorf("second Uninstall: %v", err)
	}
}

// A newer chart that adds a default its templates read must upgrade a release
// made from the older chart: the new defaults are kept and the release's own
// values laid over them (adversarial review F1; Helm's ReuseValues would
// substitute the old chart's defaults and fail the render).
func TestHelmReleaseClientUpgradeKeepsTheNewChartsDefaults(t *testing.T) {
	ctx := context.Background()
	h := memoryHelm(t)
	if _, err := h.Install(ctx, releaseRequest{Name: "fathom", Namespace: "fathom-system", Version: "0.5.1",
		Values: mergeValues(map[string]any{"leaderElect": false}, imageValues("0.5.1"))}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	h.loadChart = func(version string) (*chart.Chart, error) {
		c, err := helmchart.Load(version)
		if err != nil {
			return nil, err
		}
		c.Values["newFeature"] = map[string]any{"enabled": false}
		c.Templates = append(c.Templates, &common.File{
			Name: "templates/new-feature.yaml",
			Data: []byte("{{- if .Values.newFeature.enabled }}\nkind: ConfigMap\n{{- end }}\n"),
		})
		return c, nil
	}
	rel, err := h.Upgrade(ctx, releaseRequest{Name: "fathom", Namespace: "fathom-system", Version: "0.6.0",
		Values: imageValues("0.6.0"), ReuseValues: true})
	if err != nil {
		t.Fatalf("upgrade to a chart with a new default: %v", err)
	}
	if rel.Config["leaderElect"] != false {
		t.Errorf("the release's own value was lost: leaderElect = %v", rel.Config["leaderElect"])
	}
}

// Other charts' releases in the same namespace are not Fathom installs.
func TestHelmReleaseClientListFiltersByChart(t *testing.T) {
	ctx := context.Background()
	h := memoryHelm(t)
	if _, err := h.Install(ctx, releaseRequest{Name: "fathom", Namespace: "shared", Version: "0.6.0", Values: imageValues("0.6.0")}); err != nil {
		t.Fatal(err)
	}
	list, err := h.List(ctx, []string{"shared"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range list {
		if r.ChartName != chartName {
			t.Errorf("listed a %s release", r.ChartName)
		}
	}
}

func TestHelmReleaseClientRenderValidatesTheSchema(t *testing.T) {
	h := memoryHelm(t)
	_, err := h.Render(context.Background(), releaseRequest{
		Name: "fathom", Namespace: "fathom-system", Version: "0.6.0",
		Values: map[string]any{"leaderElect": "sometimes"},
	})
	if err == nil || !strings.Contains(err.Error(), "leaderElect") {
		t.Errorf("Render err = %v, want a schema violation naming leaderElect", err)
	}
	manifest, err := h.Render(context.Background(), releaseRequest{Name: "fathom", Namespace: "fathom-system", Version: "0.6.0", Values: imageValues("0.6.0")})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(manifest, "kind: Deployment") {
		t.Error("rendered manifest has no Deployment")
	}
	if strings.Contains(manifest, "kind: CustomResourceDefinition") {
		t.Error("rendered manifest includes CRDs; the CLI applies them itself")
	}
}

func TestHelmReleaseClientMergeValuesUsesHelmSemantics(t *testing.T) {
	h := memoryHelm(t)
	vals, err := h.MergeValues(valueFlags{
		sets:       []string{"replicaCount=2", "metrics.prometheusRule.enabled=true"},
		setStrings: []string{"leaderElectionID=12345"},
		setJSON:    []string{`resources={"limits":{"cpu":"1"}}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if vals["replicaCount"] != int64(2) {
		t.Errorf("replicaCount = %#v, want int64 2", vals["replicaCount"])
	}
	if vals["leaderElectionID"] != "12345" {
		t.Errorf("--set-string kept the string: %#v", vals["leaderElectionID"])
	}
	if _, err := h.MergeValues(valueFlags{files: []string{"/nonexistent/values.yaml"}}); err == nil {
		t.Error("a missing values file was accepted")
	}
}
