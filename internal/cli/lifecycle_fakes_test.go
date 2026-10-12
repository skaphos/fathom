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
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// Test digests for a "release build" of fathomctl.
const (
	fakeOperatorDigest  = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fakeProbeDigest     = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	fakeNodeAgentDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

// eventLog records cluster-changing calls in order, so tests can assert
// "CRDs before the release" and "nothing written before a refusal".
type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, fmt.Sprintf(format, args...))
}

func (l *eventLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

// writes returns the logged mutating events (everything is a write today).
func (l *eventLog) writes() []string { return l.all() }

func (l *eventLog) index(prefix string) int {
	for i, e := range l.all() {
		if strings.HasPrefix(e, prefix) {
			return i
		}
	}
	return -1
}

// withVersion sets the CLI version and, when digests is true, the release
// digests for one test.
func withVersion(t *testing.T, version string, digests bool) {
	t.Helper()
	oldV, oldO, oldP, oldN := Version, operatorDigest, probeDigest, nodeAgentDigest
	Version = version
	operatorDigest, probeDigest, nodeAgentDigest = "", "", ""
	if digests {
		operatorDigest, probeDigest, nodeAgentDigest = fakeOperatorDigest, fakeProbeDigest, fakeNodeAgentDigest
	}
	t.Cleanup(func() { Version, operatorDigest, probeDigest, nodeAgentDigest = oldV, oldO, oldP, oldN })
}

// fakeReleaseClient is an in-memory Helm. It also plays the cluster's part
// of a release: installing or upgrading writes the manager Deployment, ready
// or not per operatorReady.
type fakeReleaseClient struct {
	t        *testing.T
	c        client.Client
	log      *eventLog
	real     *helmReleaseClient
	releases map[string]*helmRelease

	operatorReady bool
	// surgeStuck models a rolling update whose new pod never becomes ready
	// while the old one keeps the Deployment Available.
	surgeStuck bool
	listErr    error
	// onUninstall models the operator writing while it shuts down.
	onUninstall func(ctx context.Context)
}

func newFakeReleaseClient(t *testing.T, c client.Client, log *eventLog) *fakeReleaseClient {
	return &fakeReleaseClient{
		t: t, c: c, log: log,
		real:          &helmReleaseClient{renderConfig: offlineConfig},
		releases:      map[string]*helmRelease{},
		operatorReady: true,
	}
}

// seed adds an existing release (and its manager Deployment).
func (r *fakeReleaseClient) seed(rel helmRelease, ready bool) {
	r.t.Helper()
	if rel.ChartName == "" {
		rel.ChartName = chartName
	}
	if rel.Status == "" {
		rel.Status = releaseDeployed
	}
	if rel.Revision == 0 {
		rel.Revision = 1
	}
	if rel.AppVersion == "" {
		rel.AppVersion = rel.ChartVersion
	}
	cp := rel
	r.releases[rel.Namespace+"/"+rel.Name] = &cp
	if err := r.writeManager(context.Background(), &cp, ready); err != nil {
		r.t.Fatal(err)
	}
}

func (r *fakeReleaseClient) List(_ context.Context, namespaces []string) ([]helmRelease, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	want := map[string]bool{}
	for _, ns := range namespaces {
		want[ns] = true
	}
	var out []helmRelease
	for _, rel := range r.releases {
		if want[rel.Namespace] {
			out = append(out, *rel)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Namespace+out[i].Name < out[j].Namespace+out[j].Name })
	return out, nil
}

func (r *fakeReleaseClient) Install(ctx context.Context, req releaseRequest) (*helmRelease, error) {
	r.log.add("release:install %s/%s %s", req.Namespace, req.Name, req.Version)
	if _, err := r.Render(ctx, req); err != nil {
		return nil, err
	}
	if req.CreateNamespace {
		_ = r.c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: req.Namespace}})
	}
	rel := &helmRelease{
		Name: req.Name, Namespace: req.Namespace, Revision: 1, Status: releaseDeployed,
		ChartName: chartName, ChartVersion: req.Version, AppVersion: req.Version,
		Labels: map[string]string{installedByLabel: installedByCLI}, Config: req.Values,
	}
	r.releases[req.Namespace+"/"+req.Name] = rel
	return rel, r.writeManager(ctx, rel, r.operatorReady)
}

func (r *fakeReleaseClient) Upgrade(ctx context.Context, req releaseRequest) (*helmRelease, error) {
	r.log.add("release:upgrade %s/%s %s", req.Namespace, req.Name, req.Version)
	rel, ok := r.releases[req.Namespace+"/"+req.Name]
	if !ok {
		return nil, errors.New("release not found")
	}
	cfg := req.Values
	if req.ReuseValues {
		cfg = mergeValues(rel.Config, req.Values)
	}
	rel.Revision++
	rel.ChartVersion, rel.AppVersion, rel.Config = req.Version, req.Version, cfg
	return rel, r.writeManager(ctx, rel, r.operatorReady)
}

func (r *fakeReleaseClient) Uninstall(ctx context.Context, name, namespace string, _ time.Duration) error {
	r.log.add("release:uninstall %s/%s", namespace, name)
	if r.onUninstall != nil {
		r.onUninstall(ctx)
	}
	delete(r.releases, namespace+"/"+name)
	return client.IgnoreNotFound(r.c.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name + "-fathom-operator", Namespace: namespace}}))
}

func (r *fakeReleaseClient) Rollback(_ context.Context, name, namespace string, _ time.Duration) error {
	r.log.add("release:rollback %s/%s", namespace, name)
	if rel, ok := r.releases[namespace+"/"+name]; ok {
		rel.Revision++
	}
	return nil
}

// Render uses the real, offline Helm render so schema validation is genuine.
func (r *fakeReleaseClient) Render(ctx context.Context, req releaseRequest) (string, error) {
	return r.real.Render(ctx, req)
}

func (r *fakeReleaseClient) MergeValues(flags valueFlags) (map[string]any, error) {
	return r.real.MergeValues(flags)
}

// writeManager makes the release's manager Deployment reflect its values.
func (r *fakeReleaseClient) writeManager(ctx context.Context, rel *helmRelease, ready bool) error {
	image := func(block map[string]any, def string) string {
		if block == nil {
			return def
		}
		repo, _ := block["repository"].(string)
		if d, _ := block["digest"].(string); d != "" {
			return repo + "@" + d
		}
		if tag, _ := block["tag"].(string); tag != "" {
			return repo + ":" + tag
		}
		return def
	}
	vals := rel.Config
	sub := func(k string) map[string]any { m, _ := vals[k].(map[string]any); return m }
	tagRef := func(repo string) string { return repo + ":v" + rel.AppVersion }
	var agent map[string]any
	if na := sub("nodeAgent"); na != nil {
		agent, _ = na["image"].(map[string]any)
	}
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: rel.Name + "-fathom-operator", Namespace: rel.Namespace}}
	_, err := createOrUpdate(ctx, r.c, d, func() {
		d.Labels = map[string]string{
			operatorLabelKey: operatorLabelValue, nameLabel: chartName, instanceLabel: rel.Name,
			versionLabel: rel.AppVersion, managedByKey: "Helm",
		}
		d.Generation++
		d.Spec.Replicas = ptr.To[int32](1)
		d.Spec.Template.Spec.Containers = []corev1.Container{{
			Name:  managerContainerName,
			Image: image(sub("image"), tagRef(operatorRepository)),
			Args: []string{
				"--probe-image=" + image(sub("probeImage"), tagRef(probeRepository)),
				"--node-agent-image=" + image(agent, tagRef(nodeAgentRepository)),
			},
		}}
	})
	if err != nil {
		return err
	}
	// Play the Deployment controller: the API server owns
	// metadata.generation, and status is a subresource the fake client only
	// writes through Status().
	if err := r.c.Get(ctx, client.ObjectKeyFromObject(d), d); err != nil {
		return err
	}
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	available, replicas := int32(0), int32(1)
	if ready {
		available = 1
	}
	if r.surgeStuck {
		// Old pod still serving (Available), new pod not ready.
		available, replicas = 1, 2
		status = corev1.ConditionTrue
	}
	d.Status = appsv1.DeploymentStatus{
		ObservedGeneration: d.Generation,
		Replicas:           replicas,
		UpdatedReplicas:    1,
		AvailableReplicas:  available,
		Conditions: []appsv1.DeploymentCondition{{
			Type: appsv1.DeploymentAvailable, Status: status,
			Reason: "MinimumReplicasUnavailable", Message: "Deployment does not have minimum availability.",
		}},
	}
	return r.c.Status().Update(ctx, d)
}

func createOrUpdate(ctx context.Context, c client.Client, obj client.Object, mutate func()) (bool, error) {
	key := client.ObjectKeyFromObject(obj)
	if err := c.Get(ctx, key, obj); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return false, err
		}
		mutate()
		return true, c.Create(ctx, obj)
	}
	mutate()
	return false, c.Update(ctx, obj)
}

// lifecycleEnv is one test's cluster: a fake client that logs writes and
// marks applied CRDs Established, plus the fake Helm.
type lifecycleEnv struct {
	f   *factory
	c   client.Client
	rc  *fakeReleaseClient
	log *eventLog
	// denied lists permissions the access review refuses.
	denied map[string]bool
	// neverEstablish stops the fake API server marking CRDs Established.
	neverEstablish bool
	// listForbidden lists list kinds (e.g. "SecretList") the API refuses.
	listForbidden map[string]bool
}

func newLifecycleEnv(t *testing.T, objs ...client.Object) *lifecycleEnv {
	t.Helper()
	scheme, err := newScheme()
	if err != nil {
		t.Fatal(err)
	}
	env := &lifecycleEnv{log: &eventLog{}, denied: map[string]bool{}, listForbidden: map[string]bool{}}
	logWrite := func(op string, obj client.Object) {
		kind := fmt.Sprintf("%T", obj)
		kind = kind[strings.LastIndex(kind, ".")+1:]
		env.log.add("%s %s %s/%s", op, kind, obj.GetNamespace(), obj.GetName())
	}
	establish := func(obj client.Object) {
		if crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition); ok && !env.neverEstablish {
			crd.Status.Conditions = []apiextensionsv1.CustomResourceDefinitionCondition{{Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue}}
		}
	}
	env.c = fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
		WithStatusSubresource(&fathomv1alpha1.HealthCheck{}).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if gvk := list.GetObjectKind().GroupVersionKind(); env.listForbidden[gvk.Kind] {
					return apierrors.NewForbidden(gvk.GroupVersion().WithResource(strings.ToLower(strings.TrimSuffix(gvk.Kind, "List"))+"s").GroupResource(), "", errors.New("denied by test"))
				}
				return c.List(ctx, list, opts...)
			},
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				establish(obj)
				logWrite("create", obj)
				return c.Create(ctx, obj, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				establish(obj)
				logWrite("update", obj)
				return c.Update(ctx, obj, opts...)
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				logWrite("delete", obj)
				return c.Delete(ctx, obj, opts...)
			},
			DeleteAllOf: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteAllOfOption) error {
				logWrite("deleteall", obj)
				return c.DeleteAllOf(ctx, obj, opts...)
			},
		}).Build()
	env.rc = newFakeReleaseClient(t, env.c, env.log)
	f := newFactory()
	f.clientConfig = func(*globalOptions) clientcmd.ClientConfig {
		return stubClientConfig{cfg: &rest.Config{}, namespace: "team-a"}
	}
	f.newClient = func(*rest.Config, client.Options) (client.Client, error) { return env.c, nil }
	f.newReleaseClient = func(*factory) (releaseClient, error) { return env.rc, nil }
	f.accessReview = func(_ context.Context, _ client.Client, p permission) (bool, error) {
		return !env.denied[p.Verb+" "+p.Resource], nil
	}
	f.pollInterval = time.Millisecond
	f.isTerminal = func() bool { return false }
	f.stdin = strings.NewReader("")
	env.f = f
	return env
}

// clusterWrites is the log without the fake Helm's own entries for seeded
// state; tests compare it to assert "nothing changed".
func (e *lifecycleEnv) clusterWrites() []string { return e.log.writes() }

func namespaceObj(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

// liveCRDs returns the embedded CRDs as if a release recorded at version
// applied them (version "" leaves them unannotated, as helm would).
func liveCRDs(t *testing.T, version string, mutate func(*apiextensionsv1.CustomResourceDefinition)) []client.Object {
	t.Helper()
	targets, err := targetCRDs()
	if err != nil {
		t.Fatal(err)
	}
	var out []client.Object
	for _, crd := range targets {
		live := crd.DeepCopy()
		if version != "" {
			live.Annotations = withAnnotation(live.Annotations, crdVersionAnnotation, version)
		}
		live.Status.StoredVersions = []string{"v1alpha1"}
		live.Status.Conditions = []apiextensionsv1.CustomResourceDefinitionCondition{{Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue}}
		if mutate != nil {
			mutate(live)
		}
		out = append(out, live)
	}
	return out
}

// crdSchema returns the openAPI schema of the CRD's first version.
func crdSchema(crd *apiextensionsv1.CustomResourceDefinition) *apiextensionsv1.JSONSchemaProps {
	return crd.Spec.Versions[0].Schema.OpenAPIV3Schema
}
