/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	fathomv1 "github.com/skaphos/fathom/api/v1"
	"github.com/skaphos/fathom/internal/nodecert"
)

// Objects the operator creates at run time that `helm uninstall` leaves
// behind (research R7, data-model "Operator-created objects").
const (
	reportAuthenticityPolicy = "fathom-node-report-authenticity"
	defaultLeaderElectionID  = "2d3dbc4f.skaphos.io"
)

// purgeKinds are the Fathom kinds, in export (dependency) order: definitions
// before the checks that use them, checks before the reports they own.
var purgeKinds = []string{
	"AddonDefinition", "AddonDefinitionBinding", "AddonCheck", "DNSCheck",
	"NodeCertificateCheck", "NodeHealthCheck", "HealthCheck", "ClusterHealth", "HealthReport",
}

// operatorCreatedLists are the kinds carrying the managed-by label.
func operatorCreatedLists() []client.ObjectList {
	return []client.ObjectList{
		&appsv1.DaemonSetList{}, &corev1.PodList{}, &corev1.ServiceAccountList{},
		&rbacv1.RoleList{}, &rbacv1.RoleBindingList{}, &networkingv1.NetworkPolicyList{},
		&corev1.ConfigMapList{},
	}
}

// operatorCreatedObjects lists every object the operator created: labelled
// objects in any namespace (check namespaces and kube-system probe Pods),
// the shared admission policy and binding, and the leader-election Lease.
func operatorCreatedObjects(ctx context.Context, c client.Client, in *install) ([]client.Object, error) {
	var objs []client.Object
	for _, list := range operatorCreatedLists() {
		if err := c.List(ctx, list, client.MatchingLabels{nodecert.LabelManagedBy: nodecert.ManagedByValue}); err != nil {
			return nil, fmt.Errorf("list %T: %w", list, err)
		}
		items, err := meta.ExtractList(list)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			objs = append(objs, item.(client.Object))
		}
	}
	named := []client.Object{
		&admissionregistrationv1.ValidatingAdmissionPolicyBinding{ObjectMeta: metav1.ObjectMeta{Name: reportAuthenticityPolicy}},
		&admissionregistrationv1.ValidatingAdmissionPolicy{ObjectMeta: metav1.ObjectMeta{Name: reportAuthenticityPolicy}},
	}
	if in != nil && in.Namespace != "" {
		named = append(named, &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: leaderElectionID(in), Namespace: in.Namespace}})
	}
	for _, obj := range named {
		err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		switch {
		case err == nil:
			objs = append(objs, obj)
		case apierrors.IsNotFound(err):
		default:
			return nil, fmt.Errorf("get %T %s: %w", obj, obj.GetName(), err)
		}
	}
	return objs, nil
}

// leaderElectionID is the release's configured Lease name.
func leaderElectionID(in *install) string {
	if v, ok := lookupValue(in.values, []string{"leaderElectionID"}); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return defaultLeaderElectionID
}

// deleteOperatorCreated removes every operator-created object and waits,
// bounded by timeout, until they are gone (FR-030). It runs after the
// release is uninstalled and its manager has stopped (waitManagerGone), so
// nothing recreates them.
func deleteOperatorCreated(ctx context.Context, f *factory, c client.Client, in *install, timeout time.Duration) (int, error) {
	objs, err := operatorCreatedObjects(ctx, c, in)
	if err != nil {
		return 0, err
	}
	return len(objs), deleteAndWait(ctx, f, c, objs, timeout)
}

// waitManagerGone waits, bounded by timeout, until the release's manager
// Deployment and every pod of it are gone, so no operator is left to recreate
// what uninstall removes next (adversarial review F3).
func waitManagerGone(ctx context.Context, f *factory, c client.Client, ns, release string, timeout time.Duration) error {
	var last string
	err := waitBounded(ctx, f, timeout, func(ctx context.Context) (bool, error) {
		var deploys appsv1.DeploymentList
		if err := c.List(ctx, &deploys, client.InNamespace(ns), client.MatchingLabels{instanceLabel: release, operatorLabelKey: operatorLabelValue}); err != nil {
			return false, err
		}
		var pods corev1.PodList
		if err := c.List(ctx, &pods, client.InNamespace(ns), client.MatchingLabels{instanceLabel: release}); err != nil {
			return false, err
		}
		switch {
		case len(deploys.Items) > 0:
			last = "Deployment " + ns + "/" + deploys.Items[0].Name
		case len(pods.Items) > 0:
			last = "Pod " + ns + "/" + pods.Items[0].Name
		default:
			return true, nil
		}
		return false, nil
	})
	if errors.Is(err, errWaitTimeout) {
		return fmt.Errorf("the release was removed but the operator is still running after %s (%s); its objects were not cleaned up yet\n"+
			"next: fathomctl uninstall -n %s --timeout %s  # finishes the cleanup; if a pod is stuck on an unreachable node, `kubectl -n %s delete pod <pod> --force --grace-period=0` first",
			timeout, last, ns, timeout*2, ns)
	}
	return err
}

// deleteAndWait deletes objs with foreground propagation and waits until
// every one is gone.
func deleteAndWait(ctx context.Context, f *factory, c client.Client, objs []client.Object, timeout time.Duration) error {
	for _, obj := range objs {
		if err := c.Delete(ctx, obj, client.PropagationPolicy(metav1.DeletePropagationForeground)); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("delete %s: %w", describeObject(obj), err)
		}
	}
	remaining := objs
	err := waitBounded(ctx, f, timeout, func(ctx context.Context) (bool, error) {
		var still []client.Object
		for _, obj := range remaining {
			probe := obj.DeepCopyObject().(client.Object)
			err := c.Get(ctx, client.ObjectKeyFromObject(obj), probe)
			switch {
			case apierrors.IsNotFound(err):
			case err != nil:
				return false, err
			default:
				still = append(still, obj)
			}
		}
		remaining = still
		return len(still) == 0, nil
	})
	if errors.Is(err, errWaitTimeout) {
		names := make([]string, len(remaining))
		for i, obj := range remaining {
			names[i] = describeObject(obj)
		}
		return fmt.Errorf("still present after %s: %s", timeout, strings.Join(names, ", "))
	}
	return err
}

func describeObject(obj client.Object) string {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if kind == "" {
		kind = strings.TrimPrefix(fmt.Sprintf("%T", obj), "*")
		kind = kind[strings.LastIndex(kind, ".")+1:]
	}
	if obj.GetNamespace() == "" {
		return kind + " " + obj.GetName()
	}
	return kind + " " + obj.GetNamespace() + "/" + obj.GetName()
}

// fathomResources lists every Fathom custom resource, in purgeKinds order,
// each kind at the version its live CRD serves. A kind whose CRD is already
// gone has no resources.
func fathomResources(ctx context.Context, c client.Client) (map[string][]unstructured.Unstructured, error) {
	// The purge deletes every CRD in the group, but can only count and
	// export the kinds this binary knows; a newer release's kind would lose
	// its resources unseen.
	live, err := refuseUnknownKinds(ctx, c)
	if err != nil {
		return nil, err
	}
	out := map[string][]unstructured.Unstructured{}
	for _, kind := range purgeKinds {
		gv := fathomv1.GroupVersion
		if version, ok := live[kind]; ok {
			gv.Version = version
		}
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gv.WithKind(kind + "List"))
		if err := c.List(ctx, list); err != nil {
			// "No such kind" is believed only when its CRD is really gone; a
			// stale mapping for a live CRD would purge its resources unseen.
			if _, isLive := live[kind]; (apierrors.IsNotFound(err) || meta.IsNoMatchError(err)) && !isLive {
				continue
			}
			return nil, fmt.Errorf("list %s: %w", kind, err)
		}
		items := list.Items
		sort.Slice(items, func(i, j int) bool {
			if items[i].GetNamespace() != items[j].GetNamespace() {
				return items[i].GetNamespace() < items[j].GetNamespace()
			}
			return items[i].GetName() < items[j].GetName()
		})
		out[kind] = items
	}
	return out, nil
}

// exportResources writes every resource to path as one multi-document YAML,
// atomically (temp file, then rename), stripped of server-populated fields,
// status and every ownerReference, so `kubectl apply -f` recreates them
// after a reinstall (FR-032, research R10). A restored reference to a purged
// owner's UID would have the garbage collector delete the restored object —
// HealthReports are owned by their checks.
func exportResources(path string, resources map[string][]unstructured.Unstructured) (int, error) {
	var buf bytes.Buffer
	_, _ = fmt.Fprintf(&buf, "# Fathom resources exported by fathomctl uninstall --purge-crds.\n# Re-apply after a reinstall: kubectl apply -f %s\n# Owner references and status were removed; owners are recreated with new identities.\n", filepath.Base(path))
	n := 0
	for _, kind := range purgeKinds {
		for _, item := range resources[kind] {
			obj := item.DeepCopy()
			sanitizeForExport(obj)
			raw, err := yaml.Marshal(obj.Object)
			if err != nil {
				return 0, fmt.Errorf("encode %s %s: %w", kind, obj.GetName(), err)
			}
			buf.WriteString("---\n")
			buf.Write(raw)
			n++
		}
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".fathom-export-*")
	if err != nil {
		return 0, fmt.Errorf("write export %s: %w", path, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return 0, fmt.Errorf("write export %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("write export %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return 0, fmt.Errorf("write export %s: %w", path, err)
	}
	return n, nil
}

func sanitizeForExport(obj *unstructured.Unstructured) {
	for _, field := range []string{"uid", "resourceVersion", "managedFields", "creationTimestamp", "generation", "ownerReferences", "selfLink", "deletionTimestamp", "deletionGracePeriodSeconds", "finalizers"} {
		unstructured.RemoveNestedField(obj.Object, "metadata", field)
	}
	unstructured.RemoveNestedField(obj.Object, "status")
}

// purgeResources deletes every Fathom custom resource, then every
// fathom.skaphos.io CRD, awaiting each step within timeout (FR-033). The
// operator adds no finalizers, so neither step needs it running.
func purgeResources(ctx context.Context, f *factory, c client.Client, resources map[string][]unstructured.Unstructured, timeout time.Duration) error {
	var objs []client.Object
	for i := len(purgeKinds) - 1; i >= 0; i-- {
		for _, item := range resources[purgeKinds[i]] {
			objs = append(objs, item.DeepCopy())
		}
	}
	if err := deleteAndWait(ctx, f, c, objs, timeout); err != nil {
		return fmt.Errorf("delete Fathom resources: %w", err)
	}
	crds, err := fathomCRDs(ctx, c)
	if err != nil {
		return err
	}
	if err := deleteAndWait(ctx, f, c, crds, timeout); err != nil {
		return fmt.Errorf("delete Fathom CRDs: %w", err)
	}
	return nil
}

// refuseUnknownKinds refuses when the group holds a CRD this binary cannot
// enumerate — a kind not in purgeKinds, or one that serves no version at all
// — naming each one. Otherwise it returns, for every live kind, the version
// its CRD serves, so callers list at whatever version the cluster really has
// rather than the one this binary was compiled against.
func refuseUnknownKinds(ctx context.Context, c client.Client) (map[string]string, error) {
	crds, err := fathomCRDs(ctx, c)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, k := range purgeKinds {
		known[k] = true
	}
	live := map[string]string{}
	var unknown []string
	for _, obj := range crds {
		crd := obj.(*apiextensionsv1.CustomResourceDefinition)
		version, served := servedVersion(crd)
		if !known[crd.Spec.Names.Kind] || !served {
			unknown = append(unknown, crd.Name)
			continue
		}
		live[crd.Spec.Names.Kind] = version
	}
	if len(unknown) == 0 {
		return live, nil
	}
	sort.Strings(unknown)
	return nil, refusal{
		Reason: reasonUnknownKinds,
		Detail: "--purge-crds would delete CRDs this fathomctl cannot enumerate (" + strings.Join(unknown, ", ") + ") without counting or exporting their resources; they come from a newer Fathom release",
		Next:   "run the uninstall with the fathomctl from the installed release",
	}
}

// servedVersion is the version a CRD's resources are listed at: its storage
// version when that is served, else the first served version. The second
// result is false when the CRD serves nothing.
func servedVersion(crd *apiextensionsv1.CustomResourceDefinition) (string, bool) {
	first := ""
	for _, v := range crd.Spec.Versions {
		if !v.Served {
			continue
		}
		if v.Storage {
			return v.Name, true
		}
		if first == "" {
			first = v.Name
		}
	}
	return first, first != ""
}

// fathomCRDs lists every CRD in the fathom.skaphos.io group, shipped by this
// release or not.
func fathomCRDs(ctx context.Context, c client.Client) ([]client.Object, error) {
	var list apiextensionsv1.CustomResourceDefinitionList
	if err := c.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("list CRDs: %w", err)
	}
	var out []client.Object
	for i := range list.Items {
		if list.Items[i].Spec.Group == fathomv1.GroupVersion.Group {
			out = append(out, &list.Items[i])
		}
	}
	return out, nil
}

// namespaceEntry is one object in the release namespace.
type namespaceEntry struct {
	Kind, Name string
}

func (e namespaceEntry) String() string { return e.Kind + " " + e.Name }

// namespaceInventory is what --delete-namespace would delete, split into
// objects Fathom or Kubernetes put there and objects someone else did
// (research R11).
type namespaceInventory struct {
	Foreign   []namespaceEntry
	Automatic []namespaceEntry
}

// automaticObjects are created in every namespace by Kubernetes or common
// add-ons; they are reported, not treated as someone else's data.
var automaticObjects = map[namespaceEntry]bool{
	{"ServiceAccount", "default"}:             true,
	{"ConfigMap", "kube-root-ca.crt"}:         true,
	{"ConfigMap", "istio-ca-root-cert"}:       true,
	{"ConfigMap", "openshift-service-ca.crt"}: true,
}

// derivedKinds are records of other objects, never data in their own right.
var derivedKinds = map[string]bool{
	"Event": true, "Endpoints": true, "EndpointSlice": true, "ControllerRevision": true,
}

// inventoryNamespace lists every object in ns through discovery and splits
// out the ones Fathom did not create. It runs before anything is removed.
func inventoryNamespace(ctx context.Context, f *factory, c client.Client, ns string, in *install) (*namespaceInventory, error) {
	gvks, err := f.namespacedKinds(f)
	if err != nil {
		return nil, fmt.Errorf("discover namespaced resource types: %w", err)
	}
	inv := &namespaceInventory{}
	seen := map[namespaceEntry]bool{}
	var unlistable []string
	// Every object is gathered before any is classified, so a dependent can
	// be matched against the owners actually in the namespace.
	type listed struct {
		kind string
		obj  *unstructured.Unstructured
	}
	var items []listed
	uids := map[types.UID]bool{}
	for _, gvk := range gvks {
		if derivedKinds[gvk.Kind] {
			continue
		}
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk.GroupVersion().WithKind(gvk.Kind + "List"))
		if err := c.List(ctx, list, client.InNamespace(ns)); err != nil {
			// A kind the user cannot list hides objects the namespace
			// deletion would still remove, so it stops the uninstall.
			if apierrors.IsForbidden(err) {
				unlistable = append(unlistable, gvk.Kind)
				continue
			}
			if apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err) || meta.IsNoMatchError(err) {
				continue
			}
			return nil, fmt.Errorf("list %s in %s: %w", gvk.Kind, ns, err)
		}
		for i := range list.Items {
			items = append(items, listed{kind: gvk.Kind, obj: &list.Items[i]})
			uids[list.Items[i].GetUID()] = true
		}
	}
	for _, it := range items {
		entry := namespaceEntry{Kind: it.kind, Name: it.obj.GetName()}
		if seen[entry] || fathomOwned(it.obj, in) || ownerListed(it.obj, uids) {
			continue
		}
		seen[entry] = true
		if automaticObjects[entry] {
			inv.Automatic = append(inv.Automatic, entry)
		} else {
			inv.Foreign = append(inv.Foreign, entry)
		}
	}
	if len(unlistable) > 0 {
		sort.Strings(unlistable)
		return nil, refusal{
			Reason: reasonMissingPermissions,
			Detail: fmt.Sprintf("--delete-namespace cannot list %s in %s, so it cannot show what deleting the namespace would remove", strings.Join(unlistable, ", "), ns),
			Next:   "grant list on those kinds in " + ns + ", or rerun without --delete-namespace",
		}
	}
	sortEntries(inv.Foreign)
	sortEntries(inv.Automatic)
	return inv, nil
}

// ownerListed reports a dependent whose owner is itself in the inventory: it
// is reported through that owner (Fathom's, or listed as foreign) and
// garbage-collected with it. A dependent whose owner is elsewhere or gone is
// listed in its own right, so nothing is deleted unseen.
func ownerListed(obj *unstructured.Unstructured, uids map[types.UID]bool) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if uids[ref.UID] {
			return true
		}
	}
	return false
}

// fathomOwned reports objects Fathom put in its namespace: the release's
// own objects and records, operator-created objects and the leader Lease.
func fathomOwned(obj *unstructured.Unstructured, in *install) bool {
	labels, annotations := obj.GetLabels(), obj.GetAnnotations()
	if labels[nodecert.LabelManagedBy] == nodecert.ManagedByValue {
		return true
	}
	if in == nil || in.Release == "" {
		return false
	}
	if annotations["meta.helm.sh/release-name"] == in.Release {
		return true
	}
	if labels["owner"] == "helm" && labels["name"] == in.Release {
		return true // Helm's release record Secrets
	}
	return obj.GetKind() == "Lease" && obj.GetName() == leaderElectionID(in)
}

func sortEntries(es []namespaceEntry) {
	sort.Slice(es, func(i, j int) bool {
		if es[i].Kind != es[j].Kind {
			return es[i].Kind < es[j].Kind
		}
		return es[i].Name < es[j].Name
	})
}

// discoverNamespacedKinds returns every namespaced, listable, deletable
// resource type the API server serves, at its preferred version.
func discoverNamespacedKinds(f *factory) ([]schema.GroupVersionKind, error) {
	cfg, err := f.restConfig()
	if err != nil {
		return nil, err
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	// Partial results (an unavailable aggregated API) would hide that
	// group's objects from the inventory while the namespace deletion still
	// removes them, so any discovery error fails closed.
	lists, err := dc.ServerPreferredNamespacedResources()
	if err != nil {
		return nil, err
	}
	var out []schema.GroupVersionKind
	for _, l := range lists {
		gv, err := schema.ParseGroupVersion(l.GroupVersion)
		if err != nil {
			continue
		}
		for _, r := range l.APIResources {
			if strings.Contains(r.Name, "/") || !hasVerbs(r.Verbs, "list", "delete") {
				continue
			}
			out = append(out, gv.WithKind(r.Kind))
		}
	}
	return out, nil
}

func hasVerbs(have metav1.Verbs, want ...string) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if h == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
