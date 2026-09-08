/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The operator Deployment is found by the label both supported installs
// apply (kustomize config/manager and the Helm chart), then confirmed to be
// Fathom's by name label or image, since control-plane=controller-manager is
// the kubebuilder default any operator may carry.
const (
	operatorLabelKey      = "control-plane"
	operatorLabelValue    = "controller-manager"
	nameLabel             = "app.kubernetes.io/name"
	versionLabel          = "app.kubernetes.io/version"
	operatorImageMarker   = "fathom-operator"
	managerContainerName  = "manager"
	digestDisplayLength   = 19 // "sha256:" plus twelve hex characters
	operatorNotFoundError = "no Fathom operator deployment found (label " + operatorLabelKey + "=" + operatorLabelValue + ")"
)

// versionInfo is the `version -o json` shape.
type versionInfo struct {
	Client   string           `json:"client"`
	Operator *operatorVersion `json:"operator,omitempty"`
}

// operatorVersion describes the operator the CLI found, or why it could not.
// Error set means "unavailable"; the command still exits 0.
type operatorVersion struct {
	Version    string `json:"version,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Deployment string `json:"deployment,omitempty"`
	Image      string `json:"image,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (o *operatorVersion) describe() string {
	if o.Error != "" {
		return "unavailable (" + o.Error + ")"
	}
	return fmt.Sprintf("%s (%s/%s)", o.Version, o.Namespace, o.Deployment)
}

func newVersionCommand(f *factory) *cobra.Command {
	var clientOnly bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the fathomctl version and, when reachable, the operator's",
		Long: `version prints the fathomctl version and, when a cluster is reachable and
Fathom is installed, the operator's version with the namespace and Deployment
it was read from. The operator line degrades to "unavailable (<reason>)"
rather than failing, so version works offline; --client skips the cluster
entirely.

The operator is located by the control-plane=controller-manager label on its
Deployment, in --namespace when given or across all namespaces otherwise, so
fathomctl needs list access to Deployments for this one verb.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := versionInfo{Client: clientVersion()}
			if !clientOnly {
				info.Operator = lookupOperator(commandContext(cmd), f)
			}
			out := cmd.OutOrStdout()
			if f.opts.output.structured() {
				return encode(out, f.opts.output, info)
			}
			_, _ = fmt.Fprintf(out, "Client:   %s\n", info.Client)
			if info.Operator != nil {
				_, _ = fmt.Fprintf(out, "Operator: %s\n", info.Operator.describe())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&clientOnly, "client", false, "Print only the fathomctl version; do not contact the cluster.")
	return cmd
}

// lookupOperator finds the operator Deployment and reads its version from
// the app.kubernetes.io/version label (Helm sets it), else the manager image
// tag, else the image digest. Every failure becomes an "unavailable" reason.
func lookupOperator(ctx context.Context, f *factory) *operatorVersion {
	c, err := f.client()
	if err != nil {
		return &operatorVersion{Error: err.Error()}
	}
	// Only an explicit --namespace narrows the search: the operator normally
	// lives in its own namespace, not the kubeconfig context's.
	var list appsv1.DeploymentList
	if err := c.List(ctx, &list, client.InNamespace(f.opts.namespace), client.MatchingLabels{operatorLabelKey: operatorLabelValue}); err != nil {
		return &operatorVersion{Error: "list operator deployments: " + err.Error()}
	}
	var candidates []*appsv1.Deployment
	for i := range list.Items {
		if isFathomOperator(&list.Items[i]) {
			candidates = append(candidates, &list.Items[i])
		}
	}
	if len(candidates) == 0 {
		scope := "in any namespace"
		if f.opts.namespace != "" {
			scope = "in namespace " + f.opts.namespace
		}
		return &operatorVersion{Error: operatorNotFoundError + " " + scope}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Namespace != candidates[j].Namespace {
			return candidates[i].Namespace < candidates[j].Namespace
		}
		return candidates[i].Name < candidates[j].Name
	})
	d := candidates[0]
	image := managerImage(d)
	version := d.Labels[versionLabel]
	if version == "" {
		version = versionFromImage(image)
	}
	return &operatorVersion{Version: version, Namespace: d.Namespace, Deployment: d.Name, Image: image}
}

func isFathomOperator(d *appsv1.Deployment) bool {
	if strings.HasPrefix(d.Labels[nameLabel], "fathom") {
		return true
	}
	for _, c := range d.Spec.Template.Spec.Containers {
		if strings.Contains(c.Image, operatorImageMarker) {
			return true
		}
	}
	return false
}

func managerImage(d *appsv1.Deployment) string {
	containers := d.Spec.Template.Spec.Containers
	for _, c := range containers {
		if c.Name == managerContainerName {
			return c.Image
		}
	}
	if len(containers) > 0 {
		return containers[0].Image
	}
	return ""
}

// versionFromImage extracts the tag from an image reference, falling back to
// a shortened digest for digest-pinned deployments.
func versionFromImage(image string) string {
	ref, digest := image, ""
	if i := strings.Index(ref, "@"); i >= 0 {
		ref, digest = ref[:i], ref[i+1:]
	}
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		return ref[i+1:]
	}
	if digest != "" {
		if len(digest) > digestDisplayLength {
			digest = digest[:digestDisplayLength]
		}
		return digest
	}
	return "unknown (untagged image)"
}

// Version is the fathomctl release version, injected at build time:
//
//	-ldflags "-X github.com/skaphos/fathom/internal/cli.Version=v0.6.0"
//
// The fathomctl-build and fathomctl-dist tasks set it; a plain `go build`
// leaves it empty and clientVersion falls back to what the Go toolchain
// recorded, so a development binary identifies itself as such.
var Version string

// devVersion is reported when neither the ldflag nor the module build info
// carries a usable version.
const devVersion = "devel"

// clientVersion returns the version fathomctl reports for itself. Order of
// preference: the ldflag, the main module version stamped by `go install
// pkg@version`, then "devel" plus a short VCS revision when the toolchain
// recorded one.
func clientVersion() string {
	if Version != "" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return devVersion
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		v = devVersion
	}
	if rev := buildSetting(info, "vcs.revision"); rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		v += "+" + rev
		if buildSetting(info, "vcs.modified") == "true" {
			v += ".dirty"
		}
	}
	return v
}

func buildSetting(info *debug.BuildInfo, key string) string {
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}
