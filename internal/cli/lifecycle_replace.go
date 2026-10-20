/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The replace action (specs/014-api-v1-promotion/contracts/cli-upgrade-export.md).
//
// The API server refuses a CRD update that drops a version the CRD still
// stores, so a release that stops serving a version (0.7.0: v1alpha1 -> v1)
// cannot be applied in place. The CLI deletes the CRD, which deletes every
// object of that kind, and creates the release's CRD in its place. Because
// that is destructive, the plan lists the objects first and the upgrade
// refuses until the administrator acknowledges with --accept-incompatible-crds
// (unless nothing would be removed).

// objectRef names one custom resource a replace would remove.
type objectRef struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

func (r objectRef) String() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}

// storedVersionsDropped lists the versions the live CRD stores that the
// target no longer declares at all; one of them makes an in-place update
// impossible.
func storedVersionsDropped(live, target *apiextensionsv1.CustomResourceDefinition) []string {
	declared := map[string]bool{}
	for _, v := range target.Spec.Versions {
		declared[v.Name] = true
	}
	var dropped []string
	for _, v := range live.Status.StoredVersions {
		if !declared[v] {
			dropped = append(dropped, v)
		}
	}
	return dropped
}

// inventoryOf lists the objects of a live CRD at the version it serves, so
// the plan can name what a replace removes.
func inventoryOf(ctx context.Context, c client.Client, live *apiextensionsv1.CustomResourceDefinition) ([]objectRef, error) {
	version, ok := servedVersion(live)
	if !ok {
		return nil, fmt.Errorf("%s serves no version, so its objects cannot be listed", live.Name)
	}
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersion{Group: live.Spec.Group, Version: version}.WithKind(live.Spec.Names.Kind + "List"))
	if err := c.List(ctx, list); err != nil {
		return nil, err
	}
	refs := make([]objectRef, 0, len(list.Items))
	for _, item := range list.Items {
		refs = append(refs, objectRef{Namespace: item.GetNamespace(), Name: item.GetName()})
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Namespace != refs[j].Namespace {
			return refs[i].Namespace < refs[j].Namespace
		}
		return refs[i].Name < refs[j].Name
	})
	return refs, nil
}

// needsAcknowledgement reports whether a replace must be acknowledged with
// --accept-incompatible-crds: when it would remove objects, or when the
// inventory could not be taken and so might.
func (p *crdPlan) needsAcknowledgement() bool {
	return p.Action == crdReplace && (len(p.Inventory) > 0 || p.InventoryErr != "")
}

// renderInventory prints the block the contract specifies, once per CRD to
// be replaced, after the CRD table of a plan.
func renderInventory(w io.Writer, plans []crdPlan) {
	for _, p := range plans {
		if p.Action != crdReplace {
			continue
		}
		switch {
		case p.InventoryErr != "":
			_, _ = fmt.Fprintf(w, "replace crd/%s (%s -> %s): objects could not be listed (%s); the replace removes every object of this kind\n",
				p.Name, p.FromVersion, p.ToVersion, p.InventoryErr)
		default:
			_, _ = fmt.Fprintf(w, "replace crd/%s (%s -> %s): %d objects will be removed\n", p.Name, p.FromVersion, p.ToVersion, len(p.Inventory))
			for _, ref := range p.Inventory {
				_, _ = fmt.Fprintf(w, "  %s\n", ref)
			}
			if len(p.Inventory) > 0 {
				_, _ = fmt.Fprintln(w, "  and their HealthReport history")
			}
		}
	}
}

// inventoryAckLine is the acknowledgement-required detail for one replace.
func inventoryAckLine(p *crdPlan, s *sanctionedChange, release string) string {
	what := fmt.Sprintf("%d objects will be removed", len(p.Inventory))
	if p.InventoryErr != "" {
		what = "its objects could not be listed and every one will be removed"
	}
	return fmt.Sprintf("%s: replace %s -> %s, %s — %s (sanctioned by v%s)", p.Name, p.FromVersion, p.ToVersion, what, s.Issue, release)
}

// replaceCRD deletes the live CRD, waits until it is gone (its objects go
// with it), creates the target and leaves the Established wait to the
// caller. A CRD already gone at delete time counts as replaced.
func replaceCRD(ctx context.Context, f *factory, c client.Client, p *crdPlan, version string, timeout time.Duration) error {
	live := &apiextensionsv1.CustomResourceDefinition{}
	live.Name = p.Name
	if err := c.Delete(ctx, live); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete CRD %s for replacement: %w", p.Name, err)
	}
	err := waitBounded(ctx, f, timeout, func(ctx context.Context) (bool, error) {
		err := c.Get(ctx, client.ObjectKey{Name: p.Name}, &apiextensionsv1.CustomResourceDefinition{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
	if errors.Is(err, errWaitTimeout) {
		return fmt.Errorf("CRD %s was not removed within %s; its objects may still be finalizing", p.Name, timeout)
	}
	if err != nil {
		return err
	}
	desired := p.target.DeepCopy()
	desired.ResourceVersion = ""
	desired.Annotations = withAnnotation(desired.Annotations, crdVersionAnnotation, version)
	if err := c.Create(ctx, desired, client.FieldOwner(fieldManager)); err != nil {
		return fmt.Errorf("create CRD %s after removing its %s version: %w", p.Name, strings.Join(storedVersionsDropped(p.live, p.target), ","), err)
	}
	p.RemovedObjects = len(p.Inventory)
	return nil
}

// renderReplaced reports what each replace removed, after the CRDs are applied.
func renderReplaced(w io.Writer, plans []crdPlan) {
	for _, p := range plans {
		if p.Action == crdReplace {
			_, _ = fmt.Fprintf(w, "replaced crd/%s: removed %d objects\n", p.Name, p.RemovedObjects)
		}
	}
}
