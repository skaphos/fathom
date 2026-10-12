/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The lifecycle verbs (install, upgrade, uninstall, status) drive the chart
// embedded in this fathomctl as a Helm release (spec 013). Everything here is
// Helm-free: Helm itself sits behind releaseClient in lifecycle_helm.go.

const (
	// chartName is the chart every Fathom Helm release is made from.
	chartName = "fathom-operator"
	// defaultReleaseName and defaultInstallNamespace are where `install`
	// puts Fathom without flags; the namespace is the documented one, not the
	// kubeconfig context's (FR-010).
	defaultReleaseName      = "fathom"
	defaultInstallNamespace = "fathom-system"
	// defaultLifecycleTimeout bounds every wait a lifecycle verb performs.
	defaultLifecycleTimeout = 5 * time.Minute
	// installedByLabel is recorded on releases fathomctl makes.
	installedByLabel = "fathom.skaphos.io/installed-by"
	installedByCLI   = "fathomctl"
	// instanceLabel ties the manager Deployment to its Helm release.
	instanceLabel = "app.kubernetes.io/instance"
	managedByKey  = "app.kubernetes.io/managed-by"
)

// Refusal reasons. Every refusal happens before the cluster is changed and
// carries the exact next command (contracts/cli-lifecycle.md).
const (
	reasonBelowFloor         = "below-floor"
	reasonStopSkipped        = "stop-skipped"
	reasonCRDDowngrade       = "crd-downgrade"
	reasonDevVersion         = "dev-version"
	reasonScopeChange        = "scope-change"
	reasonUnsanctioned       = "unsanctioned-incompatible"
	reasonAckRequired        = "acknowledgement-required"
	reasonDowngrade          = "downgrade"
	reasonUseUpgrade         = "use-upgrade"
	reasonNotInstalled       = "not-installed"
	reasonNotHelm            = "not-helm"
	reasonMultipleInstalls   = "multiple-installs"
	reasonReleasePending     = "release-pending"
	reasonMissingPermissions = "missing-permissions"
	reasonNamespaceMissing   = "namespace-missing"
	reasonRollbackRefused    = "rollback-refused"
)

// refusal is a verb declining to act, with why and what to do instead.
type refusal struct {
	Reason string `json:"reason"`
	Detail string `json:"detail"`
	Next   string `json:"next,omitempty"`
}

func (r refusal) Error() string {
	s := r.Reason + ": " + r.Detail
	if r.Next != "" {
		s += "\nnext: " + r.Next
	}
	return s
}

// refusalList is the error for a plan with one or more refusals.
type refusalList []refusal

func (rs refusalList) Error() string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = r.Error()
	}
	return strings.Join(parts, "\n")
}

// managedBy is the tool that appears to own the operator.
type managedBy string

const (
	managedByHelm      managedBy = "helm"
	managedByKustomize managedBy = "kustomize"
	managedByOLM       managedBy = "olm"
	managedByUnmanaged managedBy = "unmanaged"
	managedByNone      managedBy = "none"
)

// install is what fathomctl finds in the cluster (data-model "Install").
type install struct {
	ManagedBy     managedBy         `json:"managedBy"`
	Release       string            `json:"release,omitempty"`
	Namespace     string            `json:"namespace,omitempty"`
	Revision      int               `json:"revision,omitempty"`
	ReleaseStatus string            `json:"releaseStatus,omitempty"`
	InstalledBy   string            `json:"installedBy,omitempty"`
	Version       string            `json:"version,omitempty"`
	Images        map[string]string `json:"images,omitempty"`
	Pinning       imagePinning      `json:"pinning,omitempty"`
	Ready         bool              `json:"ready"`
	ReadyReason   string            `json:"readyReason,omitempty"`

	// values are the release's user-supplied values; deployment is the
	// manager Deployment's name. Neither is part of the output contract.
	values     map[string]any
	deployment string
}

// pending reports a release left mid-change, which install and upgrade must
// not stack another change on (spec Edge "Interrupted install or upgrade").
func (i *install) pending() bool {
	return i.ReleaseStatus != "" && i.ReleaseStatus != releaseDeployed
}

// Helm release statuses the verbs care about.
const (
	releaseDeployed = "deployed"
)

// helmRelease is a Helm release reduced to what the verbs need.
type helmRelease struct {
	Name         string
	Namespace    string
	Revision     int
	Status       string
	ChartName    string
	ChartVersion string
	AppVersion   string
	Labels       map[string]string
	// Config is the user-supplied values the release was made with.
	Config map[string]any
}

// releaseRequest asks for one install, upgrade or render of the embedded
// chart. Values are the user's values with the CLI's image values merged on
// top; ReuseValues merges them over the previous revision's.
type releaseRequest struct {
	Name            string
	Namespace       string
	Version         string
	Values          map[string]any
	ReuseValues     bool
	CreateNamespace bool
	Upgrade         bool
	Timeout         time.Duration
}

// valueFlags are Helm's value flags, parsed by the Helm library so the
// semantics match `helm` exactly (FR-007).
type valueFlags struct {
	files      []string
	sets       []string
	setStrings []string
	setJSON    []string
}

// releaseClient is the seam between the verbs and Helm. The production
// implementation is in lifecycle_helm.go; tests supply a fake.
type releaseClient interface {
	// List returns the fathom-operator releases in the given namespaces, in
	// every state except uninstalled.
	List(ctx context.Context, namespaces []string) ([]helmRelease, error)
	Install(ctx context.Context, req releaseRequest) (*helmRelease, error)
	Upgrade(ctx context.Context, req releaseRequest) (*helmRelease, error)
	Uninstall(ctx context.Context, name, namespace string, timeout time.Duration) error
	Rollback(ctx context.Context, name, namespace string, timeout time.Duration) error
	// Render validates req against the chart (values schema included) with a
	// client-side dry run and returns the manifest. Nothing is changed.
	Render(ctx context.Context, req releaseRequest) (string, error)
	// MergeValues parses Helm value flags into one values map.
	MergeValues(flags valueFlags) (map[string]any, error)
}

// detectInstall finds the Fathom install. It lists manager Deployments
// across namespaces, then reads Helm releases only in those namespaces plus
// extraNamespaces (the -n namespace, catching a release whose Deployment was
// never created) — never Secrets cluster-wide (research R6).
func detectInstall(ctx context.Context, c client.Client, rc releaseClient, extraNamespaces ...string) (*install, error) {
	var list appsv1.DeploymentList
	if err := c.List(ctx, &list, client.MatchingLabels{operatorLabelKey: operatorLabelValue}); err != nil {
		return nil, fmt.Errorf("list operator deployments: %w", err)
	}
	var managers []*appsv1.Deployment
	nsSet := map[string]bool{}
	for i := range list.Items {
		d := &list.Items[i]
		if isFathomOperator(d) {
			managers = append(managers, d)
			nsSet[d.Namespace] = true
		}
	}
	for _, ns := range extraNamespaces {
		if ns != "" {
			nsSet[ns] = true
		}
	}
	namespaces := sortedKeys(nsSet)
	sort.Slice(managers, func(i, j int) bool {
		if managers[i].Namespace != managers[j].Namespace {
			return managers[i].Namespace < managers[j].Namespace
		}
		return managers[i].Name < managers[j].Name
	})

	var releases []helmRelease
	if len(namespaces) > 0 {
		var err error
		if releases, err = rc.List(ctx, namespaces); err != nil {
			return nil, fmt.Errorf("list Helm releases: %w", err)
		}
	}
	switch {
	case len(releases) > 1:
		names := make([]string, len(releases))
		for i, r := range releases {
			names[i] = r.Namespace + "/" + r.Name
		}
		sort.Strings(names)
		return nil, refusal{
			Reason: reasonMultipleInstalls,
			Detail: "more than one Fathom release exists (" + strings.Join(names, ", ") + "); one install per cluster is supported because the CRDs are shared",
			Next:   "helm uninstall <release> -n <namespace>  # keep exactly one",
		}
	case len(releases) == 1:
		// An operator the release does not own is a second install (kustomize,
		// OLM, another tool); treating the cluster as one Helm install would
		// let uninstall sweep that install's objects too.
		if stray := strayManagers(releases[0], managers); len(stray) > 0 {
			return nil, refusal{
				Reason: reasonMultipleInstalls,
				Detail: fmt.Sprintf("Helm release %s/%s and Fathom operator deployment(s) it does not own (%s); one install per cluster is supported because the CRDs are shared",
					releases[0].Namespace, releases[0].Name, strings.Join(stray, ", ")),
				Next: "kubectl get deploy -A -l " + operatorLabelKey + "=" + operatorLabelValue + "  # keep exactly one",
			}
		}
		return helmInstall(releases[0], managers), nil
	}
	if len(managers) == 0 {
		return &install{ManagedBy: managedByNone}, nil
	}
	if len(managers) > 1 {
		return nil, refusal{
			Reason: reasonMultipleInstalls,
			Detail: fmt.Sprintf("found %d Fathom operator deployments and no Helm release", len(managers)),
			Next:   "kubectl get deploy -A -l " + operatorLabelKey + "=" + operatorLabelValue,
		}
	}
	return foreignInstall(managers[0]), nil
}

// helmInstall describes a Helm-managed install from its release and, when it
// exists, the release's manager Deployment.
func helmInstall(r helmRelease, managers []*appsv1.Deployment) *install {
	in := &install{
		ManagedBy:     managedByHelm,
		Release:       r.Name,
		Namespace:     r.Namespace,
		Revision:      r.Revision,
		ReleaseStatus: r.Status,
		InstalledBy:   "helm",
		Version:       r.AppVersion,
		values:        r.Config,
	}
	if r.Labels[installedByLabel] == installedByCLI {
		in.InstalledBy = installedByCLI
	}
	for _, d := range managers {
		if releaseOwns(r, d) {
			describeManager(in, d)
			break
		}
	}
	if in.deployment == "" {
		in.ReadyReason = "operator Deployment not found in namespace " + r.Namespace
	}
	return in
}

// releaseOwns reports whether d is the release's manager Deployment.
func releaseOwns(r helmRelease, d *appsv1.Deployment) bool {
	return d.Namespace == r.Namespace && (d.Labels[instanceLabel] == r.Name || d.Labels[instanceLabel] == "")
}

// strayManagers names every manager Deployment beyond the one the release
// owns; a release has at most one.
func strayManagers(r helmRelease, managers []*appsv1.Deployment) []string {
	var stray []string
	owned := false
	for _, d := range managers {
		if !owned && releaseOwns(r, d) {
			owned = true
			continue
		}
		stray = append(stray, d.Namespace+"/"+d.Name)
	}
	return stray
}

// foreignInstall classifies an operator not managed by Helm, so the refusal
// can name the tool that owns it (FR-006).
func foreignInstall(d *appsv1.Deployment) *install {
	in := &install{ManagedBy: managedByUnmanaged, Namespace: d.Namespace}
	for k := range d.Labels {
		if strings.HasPrefix(k, "olm.owner") {
			in.ManagedBy = managedByOLM
		}
	}
	if in.ManagedBy != managedByOLM && d.Labels[managedByKey] == "kustomize" {
		in.ManagedBy = managedByKustomize
	}
	describeManager(in, d)
	in.Version = d.Labels[versionLabel]
	if in.Version == "" {
		in.Version = versionFromImage(in.Images["operator"])
	}
	return in
}

// describeManager fills images, pinning and readiness from the Deployment.
func describeManager(in *install, d *appsv1.Deployment) {
	in.deployment = d.Name
	in.Images = map[string]string{"operator": managerImage(d)}
	for _, c := range d.Spec.Template.Spec.Containers {
		if c.Name != managerContainerName {
			continue
		}
		for _, a := range c.Args {
			if v, ok := strings.CutPrefix(a, "--probe-image="); ok {
				in.Images["probe"] = v
			}
			if v, ok := strings.CutPrefix(a, "--node-agent-image="); ok {
				in.Images["node-agent"] = v
			}
		}
	}
	in.Pinning = pinnedByTag
	if strings.Contains(in.Images["operator"], "@") {
		in.Pinning = pinnedByDigest
	}
	in.Ready, in.ReadyReason = deploymentAvailable(d)
}

// deploymentAvailable reports the Available condition and, when false, the
// unmet condition in words.
func deploymentAvailable(d *appsv1.Deployment) (bool, string) {
	for _, c := range d.Status.Conditions {
		if c.Type != appsv1.DeploymentAvailable {
			continue
		}
		if c.Status == corev1.ConditionTrue {
			return true, ""
		}
		return false, fmt.Sprintf("Deployment %s/%s condition Available=%s (%s: %s)", d.Namespace, d.Name, c.Status, c.Reason, c.Message)
	}
	return false, fmt.Sprintf("Deployment %s/%s has no Available condition yet", d.Namespace, d.Name)
}

// notHelmRefusal names the tool that owns a non-Helm install (FR-006).
func notHelmRefusal(in *install) refusal {
	owner := map[managedBy]string{
		managedByKustomize: "kustomize (dist/install.yaml or config/default)",
		managedByOLM:       "OLM",
		managedByUnmanaged: "an unknown tool",
	}[in.ManagedBy]
	next := "remove it with the tool that installed it, then run fathomctl install"
	if in.ManagedBy == managedByOLM {
		next = "manage it through OLM (Subscription/ClusterServiceVersion); fathomctl does not support OLM installs"
	}
	return refusal{
		Reason: reasonNotHelm,
		Detail: fmt.Sprintf("the Fathom operator in namespace %s is not managed by Helm; it appears to be installed by %s, and fathomctl never takes over another tool's install", in.Namespace, owner),
		Next:   next,
	}
}

// pendingRefusal stops install/upgrade from stacking a change on a release
// left mid-change.
//
// A first install that never completed (revision 1, or pending-install) has
// no earlier revision to roll back to, so the way out is to remove it
// (adversarial review F6); so is a release caught mid-uninstall.
func pendingRefusal(in *install) refusal {
	r := refusal{
		Reason: reasonReleasePending,
		Detail: fmt.Sprintf("release %s in %s is %s; finish or roll back that change first", in.Release, in.Namespace, in.ReleaseStatus),
		Next:   fmt.Sprintf("helm history %s -n %s && helm rollback %s -n %s", in.Release, in.Namespace, in.Release, in.Namespace),
	}
	if in.Revision <= 1 || in.ReleaseStatus == "pending-install" || in.ReleaseStatus == "uninstalling" {
		r.Detail = fmt.Sprintf("release %s in %s is %s and has no earlier revision to roll back to", in.Release, in.Namespace, in.ReleaseStatus)
		r.Next = fmt.Sprintf("fathomctl uninstall -n %s, then rerun fathomctl install -n %s", in.Namespace, in.Namespace)
	}
	return r
}

// waitManagerAvailable polls the release's manager Deployment until it is
// Available, bounded by timeout (FR-004, FR-013, FR-026). The error names the
// unmet condition.
func waitManagerAvailable(ctx context.Context, f *factory, c client.Client, namespace, release string, timeout time.Duration) error {
	var last string
	err := waitBounded(ctx, f, timeout, func(ctx context.Context) (bool, error) {
		var list appsv1.DeploymentList
		if err := c.List(ctx, &list, client.InNamespace(namespace), client.MatchingLabels{operatorLabelKey: operatorLabelValue, instanceLabel: release}); err != nil {
			return false, err
		}
		if len(list.Items) == 0 {
			last = fmt.Sprintf("no operator Deployment for release %s in namespace %s", release, namespace)
			return false, nil
		}
		done, reason, failed := rolloutComplete(&list.Items[0])
		last = reason
		if failed {
			return false, errRolloutFailed
		}
		return done, nil
	})
	switch {
	case errors.Is(err, errWaitTimeout):
		return fmt.Errorf("operator not Available within %s: %s", timeout, last)
	case errors.Is(err, errRolloutFailed):
		return fmt.Errorf("operator rollout failed: %s", last)
	}
	return err
}

// errRolloutFailed marks a rollout the Deployment controller gave up on.
var errRolloutFailed = errors.New("rollout failed")

// rolloutComplete applies `kubectl rollout status` semantics: the new
// revision is observed, every replica is updated, no old replica remains,
// and every updated replica is available. The Available condition alone is
// not enough — during a surge the old pod keeps the Deployment Available
// while the new one crash-loops (adversarial review F2). failed reports a
// rollout past its progress deadline.
func rolloutComplete(d *appsv1.Deployment) (done bool, reason string, failed bool) {
	name := d.Namespace + "/" + d.Name
	if d.Status.ObservedGeneration < d.Generation {
		return false, fmt.Sprintf("Deployment %s rollout not yet observed (generation %d, observed %d)", name, d.Generation, d.Status.ObservedGeneration), false
	}
	for _, c := range d.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Reason == "ProgressDeadlineExceeded" {
			return false, fmt.Sprintf("Deployment %s exceeded its progress deadline: %s", name, c.Message), true
		}
	}
	if ok, reason := deploymentAvailable(d); !ok {
		return false, reason, false
	}
	want := replicasOf(d)
	switch {
	case d.Status.UpdatedReplicas < want:
		return false, fmt.Sprintf("Deployment %s has %d of %d replicas updated", name, d.Status.UpdatedReplicas, want), false
	case d.Status.Replicas > d.Status.UpdatedReplicas:
		return false, fmt.Sprintf("Deployment %s has %d old replica(s) pending termination", name, d.Status.Replicas-d.Status.UpdatedReplicas), false
	case d.Status.AvailableReplicas < d.Status.UpdatedReplicas:
		return false, fmt.Sprintf("Deployment %s has %d of %d updated replicas available", name, d.Status.AvailableReplicas, d.Status.UpdatedReplicas), false
	}
	return true, "", false
}

func replicasOf(d *appsv1.Deployment) int32 {
	if d.Spec.Replicas == nil {
		return 1
	}
	return *d.Spec.Replicas
}

// errWaitTimeout is returned by waitBounded when the bound is reached.
var errWaitTimeout = errors.New("timed out")

// waitBounded polls cond every f.pollInterval until it reports done, it
// fails, or timeout elapses. It never waits without bound (FR-004).
func waitBounded(ctx context.Context, f *factory, timeout time.Duration, cond func(context.Context) (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		done, err := cond(ctx)
		if err != nil && ctx.Err() == nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return errWaitTimeout
		case <-time.After(f.pollInterval):
		}
	}
}

// permission is one access the preflight checks.
type permission struct {
	Verb, Group, Resource, Namespace, Name string
}

func (p permission) String() string {
	res := p.Resource
	if p.Group != "" {
		res += "." + p.Group
	}
	s := p.Verb + " " + res
	if p.Name != "" {
		s += "/" + p.Name
	}
	if p.Namespace != "" {
		s += " in namespace " + p.Namespace
	} else {
		s += " (cluster-wide)"
	}
	return s
}

// lifecyclePermissions is what each mutating verb needs. Cluster RBAC is
// checked because the chart creates ClusterRoles and bindings; escalate and
// bind are implied for cluster-admin-level users (FR-050).
//
// The list covers every kind each verb writes or deletes, so an identity that
// passes it does not fail part-way through; scope adds what the flags in use
// need.
func lifecyclePermissions(verb, namespace string, scope permissionScope) []permission {
	const rbac, admission = "rbac.authorization.k8s.io", "admissionregistration.k8s.io"
	crd := func(v string) permission {
		return permission{Verb: v, Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"}
	}
	var perms []permission
	switch verb {
	case "install", "upgrade":
		perms = []permission{
			crd("create"), crd("update"), crd("get"),
			{Verb: "create", Group: rbac, Resource: "clusterroles"},
			{Verb: "create", Group: rbac, Resource: "clusterrolebindings"},
			{Verb: "create", Group: rbac, Resource: "roles", Namespace: namespace},
			{Verb: "create", Group: rbac, Resource: "rolebindings", Namespace: namespace},
			{Verb: "create", Resource: "serviceaccounts", Namespace: namespace},
			{Verb: "create", Resource: "configmaps", Namespace: namespace},
			{Verb: "create", Resource: "services", Namespace: namespace},
			{Verb: "create", Group: "apps", Resource: "deployments", Namespace: namespace},
			{Verb: "update", Group: "apps", Resource: "deployments", Namespace: namespace},
			{Verb: "create", Resource: "secrets", Namespace: namespace},
			{Verb: "update", Resource: "secrets", Namespace: namespace},
			{Verb: "get", Resource: "secrets", Namespace: namespace},
			{Verb: "list", Resource: "secrets", Namespace: namespace},
		}
		// Helm applies the chart server-side, which patches every object.
		for _, p := range []permission{
			{Group: rbac, Resource: "clusterroles"}, {Group: rbac, Resource: "clusterrolebindings"},
			{Group: rbac, Resource: "roles", Namespace: namespace}, {Group: rbac, Resource: "rolebindings", Namespace: namespace},
			{Resource: "serviceaccounts", Namespace: namespace}, {Resource: "configmaps", Namespace: namespace},
			{Resource: "services", Namespace: namespace}, {Group: "apps", Resource: "deployments", Namespace: namespace},
		} {
			p.Verb = "patch"
			perms = append(perms, p)
		}
		if scope.createNamespace {
			perms = append(perms, permission{Verb: "create", Resource: "namespaces"})
		}
	case "uninstall":
		// The release's own objects only exist when its namespace is known;
		// a rerun after the release was removed has none to delete.
		if namespace != "" {
			perms = []permission{
				{Verb: "delete", Group: "apps", Resource: "deployments", Namespace: namespace},
				{Verb: "delete", Resource: "secrets", Namespace: namespace},
				{Verb: "delete", Resource: "services", Namespace: namespace},
				{Verb: "delete", Group: "coordination.k8s.io", Resource: "leases", Namespace: namespace},
			}
		}
		perms = append(perms, []permission{
			{Verb: "delete", Group: rbac, Resource: "clusterroles"},
			{Verb: "delete", Group: rbac, Resource: "clusterrolebindings"},
			// Operator-created objects, in any namespace.
			{Verb: "delete", Group: "apps", Resource: "daemonsets"},
			{Verb: "delete", Resource: "pods"},
			{Verb: "delete", Resource: "serviceaccounts"},
			{Verb: "delete", Resource: "configmaps"},
			{Verb: "delete", Group: rbac, Resource: "roles"},
			{Verb: "delete", Group: rbac, Resource: "rolebindings"},
			{Verb: "delete", Group: "networking.k8s.io", Resource: "networkpolicies"},
			{Verb: "delete", Group: admission, Resource: "validatingadmissionpolicies"},
			{Verb: "delete", Group: admission, Resource: "validatingadmissionpolicybindings"},
		}...)
		if scope.purgeCRDs {
			perms = append(perms, crd("delete"))
			for _, r := range []string{"addondefinitions", "addondefinitionbindings", "addonchecks", "dnschecks",
				"nodecertificatechecks", "nodehealthchecks", "healthchecks", "clusterhealths", "healthreports"} {
				perms = append(perms, permission{Verb: "delete", Group: "fathom.skaphos.io", Resource: r})
			}
		}
		if scope.deleteNamespace {
			perms = append(perms, permission{Verb: "delete", Resource: "namespaces", Name: namespace})
		}
	}
	return perms
}

// permissionScope is what the verb's flags add to its permission needs.
type permissionScope struct {
	createNamespace, purgeCRDs, deleteNamespace bool
}

// preflightPermissions refuses before any change when the user lacks a
// permission the verb needs, naming each one (spec Edge "Insufficient
// permissions").
func preflightPermissions(ctx context.Context, f *factory, c client.Client, perms []permission) error {
	var missing []string
	for _, p := range perms {
		allowed, err := f.accessReview(ctx, c, p)
		if err != nil {
			return fmt.Errorf("check permission %s: %w", p, err)
		}
		if !allowed {
			missing = append(missing, p.String())
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return refusal{
		Reason: reasonMissingPermissions,
		Detail: "the kubeconfig identity lacks: " + strings.Join(missing, "; "),
		Next:   "rerun with a cluster-admin-level identity (see docs/reference/fathomctl.md, Permissions)",
	}
}

// selfAccessReview asks the API server whether the caller may do p.
func selfAccessReview(ctx context.Context, c client.Client, p permission) (bool, error) {
	review := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authorizationv1.ResourceAttributes{
			Verb: p.Verb, Group: p.Group, Resource: p.Resource, Namespace: p.Namespace, Name: p.Name,
		}},
	}
	if err := c.Create(ctx, review); err != nil {
		return false, err
	}
	return review.Status.Allowed, nil
}

// namespaceExists reports whether ns exists.
func namespaceExists(ctx context.Context, c client.Client, ns string) (bool, error) {
	err := c.Get(ctx, client.ObjectKey{Name: ns}, &corev1.Namespace{})
	switch {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err):
		return false, nil
	default:
		return false, fmt.Errorf("get namespace %s: %w", ns, err)
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// cliChartVersion is this fathomctl's version as a chart version, or a
// dev-version refusal when it is not SemVer (spec Assumptions).
func cliChartVersion() (string, error) {
	v, err := normalizeChartVersion(clientVersion())
	if err != nil {
		return "", refusal{
			Reason: reasonDevVersion,
			Detail: fmt.Sprintf("this fathomctl's version %q is not SemVer, so it cannot install or upgrade anything", clientVersion()),
			Next:   "FATHOMCTL_VERSION=<semver> go -C tools tool task fathomctl-build",
		}
	}
	return v, nil
}
