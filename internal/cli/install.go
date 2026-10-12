/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// lifecycleOptions are the flags install and upgrade share
// (contracts/cli-lifecycle.md "Shared flags").
type lifecycleOptions struct {
	values             valueFlags
	timeout            time.Duration
	release            string
	createNamespace    bool
	acceptIncompatible bool
}

func (o *lifecycleOptions) register(cmd *cobra.Command, install bool) {
	fs := cmd.Flags()
	fs.StringSliceVarP(&o.values.files, "values", "f", nil, "Values file(s), with Helm semantics. Repeatable.")
	fs.StringArrayVar(&o.values.sets, "set", nil, "Set a value (key=value), with Helm semantics. Repeatable.")
	fs.StringArrayVar(&o.values.setStrings, "set-string", nil, "Set a STRING value (key=value), with Helm semantics. Repeatable.")
	fs.StringArrayVar(&o.values.setJSON, "set-json", nil, "Set a JSON value (key=jsonval), with Helm semantics. Repeatable.")
	fs.DurationVar(&o.timeout, "timeout", defaultLifecycleTimeout, "Bound on every wait (CRDs Established, operator Available).")
	fs.BoolVar(&o.acceptIncompatible, "accept-incompatible-crds", false,
		"Apply incompatible CRD changes the release sanctions. --yes does not imply this.")
	if install {
		fs.StringVar(&o.release, "release", defaultReleaseName, "Helm release name.")
		fs.BoolVar(&o.createNamespace, "create-namespace", false, "Create the namespace if it does not exist.")
	}
}

// lifecycleClients is what a mutating lifecycle verb works with.
type lifecycleClients struct {
	c          client.Client
	rc         releaseClient
	userValues map[string]any
}

// lifecycleEnv builds the clients and parses the user's values, refusing an
// image override before anything else (FR-007).
func (o *lifecycleOptions) lifecycleEnv(f *factory) (*lifecycleClients, error) {
	if o.timeout <= 0 {
		return nil, usageError{fmt.Errorf("--timeout must be positive (got %s)", o.timeout)}
	}
	rc, err := f.newReleaseClient(f)
	if err != nil {
		return nil, err
	}
	vals, err := rc.MergeValues(o.values)
	if err != nil {
		return nil, err
	}
	if err := rejectImageOverrides(vals); err != nil {
		return nil, err
	}
	c, err := f.client()
	if err != nil {
		return nil, err
	}
	return &lifecycleClients{c: c, rc: rc, userValues: vals}, nil
}

func newInstallCommand(f *factory) *cobra.Command {
	opts := &lifecycleOptions{}
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Fathom at this fathomctl's version",
		Long: `install installs Fathom from the chart embedded in this fathomctl, at exactly
this fathomctl's version, as a Helm release that helm itself can read. It
applies every Fathom CRD first, then the operator, with images pinned by the
release's digests, and waits until the operator is Available.

Without -n it installs into fathom-system, not the kubeconfig context's
namespace; the namespace must exist unless --create-namespace is given. On an
install at this version it changes nothing; on an older one it points at
upgrade; it never takes over an install made by kustomize or OLM. When CRDs
remain from an earlier uninstall, install checks them exactly as upgrade does.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInstall(commandContext(cmd), cmd, f, opts)
		},
	}
	opts.register(cmd, true)
	return cmd
}

func runInstall(ctx context.Context, cmd *cobra.Command, f *factory, opts *lifecycleOptions) error {
	version, err := cliChartVersion()
	if err != nil {
		return err
	}
	env, err := opts.lifecycleEnv(f)
	if err != nil {
		return err
	}
	ns := f.opts.namespace
	if ns == "" {
		ns = defaultInstallNamespace
	}
	in, err := detectInstall(ctx, env.c, env.rc, ns)
	if err != nil {
		return err
	}
	switch in.ManagedBy {
	case managedByNone:
	case managedByHelm:
		return installOverExisting(cmd.OutOrStdout(), in, version)
	default:
		return notHelmRefusal(in)
	}
	if err := preflightPermissions(ctx, f, env.c, lifecyclePermissions("install", ns)); err != nil {
		return err
	}

	command := "fathomctl install -n " + ns
	plan, err := buildUpgradePlan(ctx, env.c, planRequest{
		from: oldestRecordedVersion(ctx, env.c), to: version, perCRDFrom: true,
		accept: opts.acceptIncompatible, command: command,
	})
	if err != nil {
		return err
	}
	if len(plan.Refusals) > 0 {
		return refusalList(plan.Refusals)
	}

	vals := mergeValues(env.userValues, imageValues(version))
	req := releaseRequest{Name: opts.release, Namespace: ns, Version: version, Values: vals, CreateNamespace: opts.createNamespace, Timeout: opts.timeout}
	if _, err := env.rc.Render(ctx, req); err != nil {
		return err
	}
	exists, err := namespaceExists(ctx, env.c, ns)
	if err != nil {
		return err
	}
	if !exists && !opts.createNamespace {
		return refusal{
			Reason: reasonNamespaceMissing,
			Detail: "namespace " + ns + " does not exist",
			Next:   command + " --create-namespace",
		}
	}

	if err := applyCRDs(ctx, f, env.c, plan.CRDs, version, opts.timeout); err != nil {
		return err
	}
	rel, err := env.rc.Install(ctx, req)
	if err != nil {
		return err
	}
	if err := waitManagerAvailable(ctx, f, env.c, ns, opts.release, opts.timeout); err != nil {
		deploy := managerDeploymentName(ctx, env.c, ns, opts.release)
		return fmt.Errorf("%w\nThe release was installed but the operator is not Available. To recover:\n"+
			"  inspect: kubectl -n %s describe deploy/%s\n"+
			"  check:   fathomctl status -n %s\n"+
			"  remove:  fathomctl uninstall -n %s", err, ns, deploy, ns, ns)
	}
	return printInstalled(cmd.OutOrStdout(), f.opts.output, rel, version)
}

// installOverExisting handles install when a Helm release exists: the same
// version is a no-op; anything else points at the right command (FR-012).
func installOverExisting(out io.Writer, in *install, version string) error {
	if in.pending() {
		return pendingRefusal(in)
	}
	installed, err := semver.NewVersion(in.Version)
	if err != nil {
		return fmt.Errorf("installed version %q is not SemVer: %w", in.Version, err)
	}
	cli := semver.MustParse(version)
	switch {
	case installed.Equal(cli):
		_, _ = fmt.Fprintf(out, "Fathom is already installed at v%s; nothing to do (release %s in %s).\n", version, in.Release, in.Namespace)
		return nil
	case installed.LessThan(cli):
		return refusal{
			Reason: reasonUseUpgrade,
			Detail: fmt.Sprintf("Fathom v%s is installed (release %s in %s); install never upgrades as a side effect", installed, in.Release, in.Namespace),
			Next:   "fathomctl upgrade -n " + in.Namespace,
		}
	default:
		return refusal{
			Reason: reasonDowngrade,
			Detail: fmt.Sprintf("Fathom v%s is installed, newer than this fathomctl (v%s)", installed, cli),
			Next:   fmt.Sprintf("use fathomctl v%s", installed),
		}
	}
}

// oldestRecordedVersion is the oldest release that applied any live Fathom
// CRD, or "" when none records one. It is the "installed version" for an
// install over CRDs a plain uninstall left behind.
func oldestRecordedVersion(ctx context.Context, c client.Client) string {
	targets, err := targetCRDs()
	if err != nil {
		return ""
	}
	plans, err := planCRDs(ctx, c, targets)
	if err != nil {
		return ""
	}
	var oldest *semver.Version
	for _, p := range plans {
		v, err := semver.NewVersion(p.RecordedVersion)
		if err != nil {
			continue
		}
		if oldest == nil || v.LessThan(oldest) {
			oldest = v
		}
	}
	if oldest == nil {
		return ""
	}
	return oldest.String()
}

// managerDeploymentName names the release's manager Deployment for recovery
// commands, falling back to a label selector.
func managerDeploymentName(ctx context.Context, c client.Client, ns, release string) string {
	var list appsv1.DeploymentList
	if err := c.List(ctx, &list, client.InNamespace(ns), client.MatchingLabels{operatorLabelKey: operatorLabelValue, instanceLabel: release}); err == nil && len(list.Items) > 0 {
		return list.Items[0].Name
	}
	return "-l " + instanceLabel + "=" + release
}

// installResult is install's -o json|yaml shape.
type installResult struct {
	Release   string            `json:"release"`
	Namespace string            `json:"namespace"`
	Revision  int               `json:"revision"`
	Version   string            `json:"version"`
	Images    map[string]string `json:"images"`
	Pinning   imagePinning      `json:"pinning"`
}

func printInstalled(w io.Writer, format outputFormat, rel *helmRelease, version string) error {
	res := installResult{
		Release: rel.Name, Namespace: rel.Namespace, Revision: rel.Revision,
		Version: version, Images: imageReferences(version), Pinning: pinning(),
	}
	if format.structured() {
		return encode(w, format, res)
	}
	_, _ = fmt.Fprintf(w, "Installed Fathom v%s (release %s in %s, revision %d); the operator is Available.\n", version, rel.Name, rel.Namespace, rel.Revision)
	_, _ = fmt.Fprintf(w, "Images:    %s\n", describePinning(res.Pinning))
	names := make([]string, 0, len(res.Images))
	for n := range res.Images {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		_, _ = fmt.Fprintf(w, "  %-11s%s\n", n, res.Images[n])
	}
	return nil
}

func describePinning(p imagePinning) string {
	if p == pinnedByDigest {
		return "pinned by digest"
	}
	return "tag-pinned (development build)"
}
