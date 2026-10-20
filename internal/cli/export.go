/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	fathomv1 "github.com/skaphos/fathom/api/v1"
)

// exportKinds are the authorable check kinds, in dependency order: targets
// before the wrappers that reference them, so `kubectl apply -f` of the
// output creates each HealthCheck after its source
// (specs/014-api-v1-promotion/contracts/cli-upgrade-export.md).
var exportKinds = []string{"ClusterHealth", "AddonCheck", "DNSCheck", "NodeCertificateCheck", "NodeHealthCheck", "HealthCheck"}

// exportedVersion is the version `export` renders, and alphaVersion the one
// it reads: the verb exists for exactly the 0.6.x -> 0.7.0 replacement.
const (
	exportedVersion = "v1"
	alphaVersion    = "v1alpha1"
)

const reasonNothingToExport = "nothing-to-export"

type exportOptions struct {
	output string
}

func newExportCommand(f *factory) *cobra.Command {
	opts := &exportOptions{}
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Render the alpha check resources as v1 manifests; never apply them",
		Long: `export renders every check resource still served at fathom.skaphos.io/v1alpha1
as a fathom.skaphos.io/v1 manifest, so they can be committed and re-applied
after the 0.7.0 upgrade replaces the CRDs (which deletes the alpha objects).

It rewrites apiVersion, drops spec.paused, rewrites spec.checkRef.apiVersion,
and strips status and server-populated metadata. HealthReports are history,
not intent, and are not exported; the runtime definition kinds stay alpha and
are not exported either. Nothing is written to the cluster.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExport(commandContext(cmd), cmd, f, opts)
		},
	}
	cmd.Flags().StringVar(&opts.output, "output", "", "Write the manifests to this file (atomically) instead of stdout.")
	return cmd
}

func runExport(ctx context.Context, cmd *cobra.Command, f *factory, opts *exportOptions) error {
	c, err := f.client()
	if err != nil {
		return err
	}
	alpha, err := alphaServedKinds(ctx, c)
	if err != nil {
		return err
	}
	if len(alpha) == 0 {
		return refusal{
			Reason: reasonNothingToExport,
			Detail: "nothing to export: no Fathom CRD serves " + alphaVersion + " (already upgraded?)",
			Next:   "fathomctl status",
		}
	}

	var buf bytes.Buffer
	_, _ = fmt.Fprintf(&buf, "# Fathom check resources exported by fathomctl export at %s, rendered at %s.\n# Review, commit, then: kubectl apply -f <this file>\n",
		time.Now().UTC().Format(time.RFC3339), fathomv1.GroupVersion.String())
	var failures []string
	rendered := 0
	for _, kind := range exportKinds {
		scope, ok := alpha[kind]
		if !ok {
			continue
		}
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersion{Group: fathomv1.GroupVersion.Group, Version: alphaVersion}.WithKind(kind + "List"))
		var listOpts []client.ListOption
		if f.opts.namespace != "" && scope == apiextensionsv1.NamespaceScoped {
			listOpts = append(listOpts, client.InNamespace(f.opts.namespace))
		}
		if err := c.List(ctx, list, listOpts...); err != nil {
			failures = append(failures, fmt.Sprintf("cannot export %s: %v", kind, err))
			continue
		}
		items := list.Items
		sort.Slice(items, func(i, j int) bool {
			if items[i].GetNamespace() != items[j].GetNamespace() {
				return items[i].GetNamespace() < items[j].GetNamespace()
			}
			return items[i].GetName() < items[j].GetName()
		})
		for i := range items {
			obj := &items[i]
			raw, err := renderExported(obj)
			if err != nil {
				failures = append(failures, fmt.Sprintf("cannot export %s/%s/%s: %v", kind, obj.GetNamespace(), obj.GetName(), err))
				continue
			}
			buf.WriteString("---\n")
			buf.Write(raw)
			rendered++
		}
	}

	errOut := cmd.ErrOrStderr()
	if opts.output != "" {
		if err := writeFileAtomically(opts.output, buf.Bytes()); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(errOut, "exported %d resource(s) to %s\n", rendered, opts.output)
	} else if _, err := io.Copy(cmd.OutOrStdout(), &buf); err != nil {
		return err
	}
	for _, msg := range failures {
		_, _ = fmt.Fprintln(errOut, msg)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d resource(s) could not be exported", len(failures))
	}
	return nil
}

// alphaServedKinds maps each exportable kind whose live CRD still serves the
// alpha version to its scope. A kind whose CRD is gone or already at v1 is
// absent.
func alphaServedKinds(ctx context.Context, c client.Client) (map[string]apiextensionsv1.ResourceScope, error) {
	crds, err := fathomCRDs(ctx, c)
	if err != nil {
		return nil, err
	}
	exportable := map[string]bool{}
	for _, k := range exportKinds {
		exportable[k] = true
	}
	out := map[string]apiextensionsv1.ResourceScope{}
	for _, obj := range crds {
		crd := obj.(*apiextensionsv1.CustomResourceDefinition)
		if !exportable[crd.Spec.Names.Kind] {
			continue
		}
		for _, v := range crd.Spec.Versions {
			if v.Name == alphaVersion && v.Served {
				out[crd.Spec.Names.Kind] = crd.Spec.Scope
			}
		}
	}
	return out, nil
}

// renderExported turns one alpha object into its v1 manifest: version
// rewritten, the retired pause field dropped, the target reference moved to
// v1, status and server metadata removed. The object is copied first.
func renderExported(in *unstructured.Unstructured) ([]byte, error) {
	obj := in.DeepCopy()
	sanitizeForExport(obj)
	unstructured.RemoveNestedField(obj.Object, "metadata", "generateName")
	if ann := obj.GetAnnotations(); ann != nil {
		delete(ann, lastAppliedAnnotation)
		if len(ann) == 0 {
			unstructured.RemoveNestedField(obj.Object, "metadata", "annotations")
		} else {
			obj.SetAnnotations(ann)
		}
	}
	obj.SetAPIVersion(fathomv1.GroupVersion.String())
	unstructured.RemoveNestedField(obj.Object, "spec", "paused")
	if _, found, _ := unstructured.NestedString(obj.Object, "spec", "checkRef", "apiVersion"); found {
		if err := unstructured.SetNestedField(obj.Object, fathomv1.GroupVersion.String(), "spec", "checkRef", "apiVersion"); err != nil {
			return nil, err
		}
	}
	return yaml.Marshal(obj.Object)
}

// writeFileAtomically writes data to path through a temporary file in the
// same directory and a rename, so a reader never sees a partial file.
func writeFileAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".fathom-export-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
