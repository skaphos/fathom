/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"helm.sh/helm/v4/pkg/action"
	chartcommon "helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/kube"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	ri "helm.sh/helm/v4/pkg/release"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	helmchart "github.com/skaphos/fathom/deploy/helm"
)

// This is the only file that imports Helm (spec 013 plan). Everything else
// reaches Helm through releaseClient, which tests replace.

// helmDriver stores releases as Secrets in the release namespace, exactly as
// the helm CLI does, so `helm ls`/`history`/`rollback` see every revision.
const helmDriver = "secret"

// helmReleaseClient drives the embedded chart through the Helm v4 library.
type helmReleaseClient struct {
	// newConfig builds a Helm configuration scoped to one namespace. The
	// production one talks to the cluster; tests use the in-memory driver.
	newConfig func(namespace string) (*action.Configuration, error)
	// renderConfig backs client-side dry runs: no cluster, no storage.
	renderConfig func() *action.Configuration
	// loadChart loads the chart a request installs; nil means the embedded
	// chart. Tests substitute an older chart to exercise real upgrades.
	loadChart func(version string) (*chart.Chart, error)
}

func (h *helmReleaseClient) chart(version string) (*chart.Chart, error) {
	if h.loadChart != nil {
		return h.loadChart(version)
	}
	return helmchart.Load(version)
}

// helmFieldManager is the server-side-apply field manager for everything
// fathomctl applies through Helm. Helm otherwise derives it from the binary
// name (os.Args[0]), so a renamed or versioned fathomctl — or plain `helm`
// against a fathomctl-made release — would conflict on every field. Using
// Helm's own name lets the two tools take turns on one release (SC-007);
// which tool made it is recorded by the installed-by release label. CRDs are
// written by the CLI itself, under fieldManager "fathomctl".
const helmFieldManager = "helm"

func newHelmReleaseClient(f *factory) (releaseClient, error) {
	kube.ManagedFieldsManager = helmFieldManager
	flags := genericclioptions.NewConfigFlags(false)
	flags.KubeConfig = &f.opts.kubeconfig
	flags.Context = &f.opts.context
	timeout := f.opts.requestTimeout.String()
	flags.Timeout = &timeout
	return &helmReleaseClient{
		newConfig: func(namespace string) (*action.Configuration, error) {
			cfg := action.NewConfiguration(action.ConfigurationSetLogger(slog.NewTextHandler(io.Discard, nil)))
			if err := cfg.Init(flags, namespace, helmDriver); err != nil {
				return nil, fmt.Errorf("initialise Helm for namespace %s: %w", namespace, err)
			}
			return cfg, nil
		},
		renderConfig: offlineConfig,
	}, nil
}

// offlineKubeVersion is the Kubernetes version an offline render assumes.
// Helm's default (v1.20) fails the chart's kubeVersion floor, so this tracks
// the k8s.io/* minor in go.mod, bumped in lockstep with ENVTEST_K8S_VERSION.
const offlineKubeVersion = "v1.37.0"

// offlineConfig is a Helm configuration that never contacts a cluster: an
// in-memory release store and a kube client that only prints.
func offlineConfig() *action.Configuration {
	cfg := action.NewConfiguration(action.ConfigurationSetLogger(slog.NewTextHandler(io.Discard, nil)))
	cfg.Releases = storage.Init(driver.NewMemory())
	cfg.KubeClient = &kubefake.PrintingKubeClient{Out: io.Discard}
	caps := chartcommon.DefaultCapabilities.Copy()
	if kv, err := chartcommon.ParseKubeVersion(offlineKubeVersion); err == nil {
		caps.KubeVersion = *kv
	}
	cfg.Capabilities = caps
	return cfg
}

func (h *helmReleaseClient) List(_ context.Context, namespaces []string) ([]helmRelease, error) {
	var out []helmRelease
	for _, ns := range namespaces {
		cfg, err := h.newConfig(ns)
		if err != nil {
			return nil, err
		}
		list := action.NewList(cfg)
		// The latest revision in any live state: a pending or failed release
		// is still an install the verbs must see (spec Edge "Interrupted
		// install"); uninstalled ones are not.
		// An interrupted `helm uninstall` (uninstalling) is listed too, so
		// `fathomctl uninstall` can finish it rather than calling the
		// leftover operator a non-Helm install (FR-036).
		list.StateMask = action.ListDeployed | action.ListFailed | action.ListUnknown |
			action.ListPendingInstall | action.ListPendingUpgrade | action.ListPendingRollback |
			action.ListUninstalling
		releasers, err := list.Run()
		if err != nil {
			return nil, fmt.Errorf("list releases in %s: %w", ns, err)
		}
		for _, r := range releasers {
			rel, err := asRelease(r)
			if err != nil {
				return nil, err
			}
			if rel.Chart != nil && rel.Chart.Metadata != nil && rel.Chart.Metadata.Name == chartName {
				out = append(out, toHelmRelease(rel))
			}
		}
	}
	return out, nil
}

func (h *helmReleaseClient) Install(ctx context.Context, req releaseRequest) (*helmRelease, error) {
	ch, err := h.chart(req.Version)
	if err != nil {
		return nil, err
	}
	cfg, err := h.newConfig(req.Namespace)
	if err != nil {
		return nil, err
	}
	in := action.NewInstall(cfg)
	in.ReleaseName = req.Name
	in.Namespace = req.Namespace
	in.CreateNamespace = req.CreateNamespace
	// The CLI applies CRDs itself, before the rollout (FR-010, FR-020).
	in.SkipCRDs = true
	in.ServerSideApply = true
	// The CLI waits for the manager itself, so it can name the unmet
	// condition and decide about rollback (FR-004, FR-026).
	in.WaitStrategy = kube.HookOnlyStrategy
	in.Timeout = req.Timeout
	in.Labels = map[string]string{installedByLabel: installedByCLI}
	r, err := in.RunWithContext(ctx, ch, req.Values)
	if err != nil {
		return nil, fmt.Errorf("helm install: %w", err)
	}
	return releaseResult(r)
}

func (h *helmReleaseClient) Upgrade(ctx context.Context, req releaseRequest) (*helmRelease, error) {
	ch, err := h.chart(req.Version)
	if err != nil {
		return nil, err
	}
	cfg, err := h.newConfig(req.Namespace)
	if err != nil {
		return nil, err
	}
	up := action.NewUpgrade(cfg)
	up.Namespace = req.Namespace
	up.SkipCRDs = true
	// ResetThenReuseValues, not ReuseValues: Helm's ReuseValues replaces the
	// new chart's defaults with the previous chart's, so values a newer chart
	// added (and its templates read) would be missing. This keeps the new
	// defaults and lays the release's own values over them — exactly what
	// Render validated before anything changed.
	up.ResetThenReuseValues = req.ReuseValues
	// Keep whichever apply method the release already uses (Helm v3
	// installs are client-side apply).
	up.ServerSideApply = "auto"
	up.WaitStrategy = kube.HookOnlyStrategy
	up.Timeout = req.Timeout
	r, err := up.RunWithContext(ctx, req.Name, ch, req.Values)
	if err != nil {
		return nil, fmt.Errorf("helm upgrade: %w", err)
	}
	return releaseResult(r)
}

func (h *helmReleaseClient) Uninstall(_ context.Context, name, namespace string, timeout time.Duration) error {
	cfg, err := h.newConfig(namespace)
	if err != nil {
		return err
	}
	un := action.NewUninstall(cfg)
	un.WaitStrategy = kube.HookOnlyStrategy
	un.Timeout = timeout
	un.IgnoreNotFound = true
	if _, err := un.Run(name); err != nil {
		return fmt.Errorf("helm uninstall: %w", err)
	}
	return nil
}

func (h *helmReleaseClient) Rollback(_ context.Context, name, namespace string, timeout time.Duration) error {
	cfg, err := h.newConfig(namespace)
	if err != nil {
		return err
	}
	rb := action.NewRollback(cfg)
	rb.Version = 0 // the previous revision
	rb.WaitStrategy = kube.HookOnlyStrategy
	rb.ServerSideApply = "auto"
	rb.Timeout = timeout
	if err := rb.Run(name); err != nil {
		return fmt.Errorf("helm rollback: %w", err)
	}
	return nil
}

// Render runs a client-side install dry run: it renders every template and
// validates the values against the chart's schema without contacting the
// cluster (FR-007, research R9 step 3).
func (h *helmReleaseClient) Render(ctx context.Context, req releaseRequest) (string, error) {
	ch, err := h.chart(req.Version)
	if err != nil {
		return "", err
	}
	cfg := h.renderConfig()
	in := action.NewInstall(cfg)
	in.DryRunStrategy = action.DryRunClient
	in.KubeVersion = &cfg.Capabilities.KubeVersion
	in.ReleaseName = req.Name
	in.Namespace = req.Namespace
	in.Replace = true
	in.IsUpgrade = req.Upgrade
	in.SkipCRDs = true
	r, err := in.RunWithContext(ctx, ch, req.Values)
	if err != nil {
		return "", fmt.Errorf("render chart: %w", err)
	}
	rel, err := asRelease(r)
	if err != nil {
		return "", err
	}
	return rel.Manifest, nil
}

// MergeValues parses Helm's value flags with Helm's own code, so -f, --set,
// --set-string and --set-json mean exactly what they mean to helm.
func (h *helmReleaseClient) MergeValues(flags valueFlags) (map[string]any, error) {
	opts := values.Options{
		ValueFiles:   flags.files,
		Values:       flags.sets,
		StringValues: flags.setStrings,
		JSONValues:   flags.setJSON,
	}
	vals, err := opts.MergeValues(getter.Getters())
	if err != nil {
		return nil, usageError{fmt.Errorf("parse values: %w", err)}
	}
	return vals, nil
}

// mergeValues overlays over onto base the way Helm merges values: maps merge
// key by key, anything else is replaced. Neither input is changed.
func mergeValues(base, over map[string]any) map[string]any {
	return loader.MergeMaps(base, over)
}

func asRelease(r ri.Releaser) (*releasev1.Release, error) {
	rel, ok := r.(*releasev1.Release)
	if !ok {
		return nil, fmt.Errorf("unexpected Helm release type %T", r)
	}
	return rel, nil
}

func releaseResult(r ri.Releaser) (*helmRelease, error) {
	rel, err := asRelease(r)
	if err != nil {
		return nil, err
	}
	out := toHelmRelease(rel)
	return &out, nil
}

func toHelmRelease(rel *releasev1.Release) helmRelease {
	out := helmRelease{
		Name:      rel.Name,
		Namespace: rel.Namespace,
		Revision:  rel.Version,
		Labels:    rel.Labels,
		Config:    rel.Config,
	}
	if rel.Info != nil {
		out.Status = rel.Info.Status.String()
	}
	if md := chartMetadata(rel.Chart); md != nil {
		out.ChartName, out.ChartVersion, out.AppVersion = md.Name, md.Version, md.AppVersion
	}
	return out
}

func chartMetadata(c *chart.Chart) *chart.Metadata {
	if c == nil {
		return nil
	}
	return c.Metadata
}
