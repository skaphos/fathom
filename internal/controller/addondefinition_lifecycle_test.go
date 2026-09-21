/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/registry"
	"github.com/skaphos/fathom/pkg/adapter"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
)

// This file is the lifecycle matrix of specs/012-addon-definition-runtime/
// contracts/runtime.md, one fixture per row. The matrix is the acceptance bar
// for the definition/binding reconcilers, so the fixtures are written against
// the observable outcome of a row — what dispatch resolves to and which
// condition reason an operator reads — never against an implementation detail.
//
// Determinism is a requirement, not a preference: every fixture drives
// Reconcile directly and holds an in-flight reconcile with the reconciler's
// barrier seam rather than sleeping, so there is no wall-clock dependency
// anywhere in the file.

const (
	lifecycleNamespace = "fathom-system"
	lifecycleManagerSA = "fathom-controller-manager"
	lifecycleAddon     = "custom-addon"
	lifecycleDefUID    = types.UID("definition-uid-1")
	lifecycleBindUID   = types.UID("binding-uid-1")
	lifecycleSAName    = "custom-addon-reader"
	lifecycleSAUID     = types.UID("service-account-uid-1")
	lifecycleBuild     = "v0.0.0-lifecycle-test"
	lifecycleLeader    = "lifecycle-test-leader"
)

// lifecycleStubAdapter stands in for a compiled-in adapter when a row needs a
// built-in to contest an identity.
type lifecycleStubAdapter struct {
	name       string
	addonTypes []string
}

func (a lifecycleStubAdapter) Name() string            { return a.name }
func (a lifecycleStubAdapter) Version() string         { return "0.1.0" }
func (a lifecycleStubAdapter) ContractVersion() string { return adapter.ContractVersion }
func (a lifecycleStubAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: append([]string(nil), a.addonTypes...), Families: []adapter.Family{"health"}}
}
func (a lifecycleStubAdapter) Run(context.Context, adapter.Request) (adapter.Result, error) {
	return adapter.Result{}, nil
}

func lifecycleDefinitionNamed(name string, uid types.UID) *fathomv1alpha1.AddonDefinition {
	return &fathomv1alpha1.AddonDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: uid, Generation: 1},
		Spec: fathomv1alpha1.AddonDefinitionSpec{
			AddonType: fathomv1alpha1.DefinitionDNSLabel(name), AdapterVersion: "1.0.0", SemanticsVersion: 1,
			Families: []fathomv1alpha1.DefinitionFamily{{Name: "health", DefaultEnabled: true, Checks: []fathomv1alpha1.DefinitionCheck{{
				Name: "controller", Kind: "Workload",
				Workload: &fathomv1alpha1.DefinitionWorkload{
					Target:      fathomv1alpha1.DefinitionTarget{Scope: "Namespaced", Namespaces: []fathomv1alpha1.DefinitionDNSLabel{"default"}},
					Kind:        "Deployment",
					DefaultName: "controller",
				},
			}}}},
		},
	}
}

func lifecycleDefinition() *fathomv1alpha1.AddonDefinition {
	return lifecycleDefinitionNamed(lifecycleAddon, lifecycleDefUID)
}

func lifecycleBindingNamed(name string, uid types.UID, defUID types.UID, saName string, saUID types.UID) *fathomv1alpha1.AddonDefinitionBinding {
	return &fathomv1alpha1.AddonDefinitionBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: lifecycleNamespace, UID: uid, Generation: 1},
		Spec: fathomv1alpha1.AddonDefinitionBindingSpec{
			DefinitionRef:     fathomv1alpha1.DefinitionReference{Name: fathomv1alpha1.DefinitionDNSLabel(name), UID: string(defUID)},
			ServiceAccountRef: fathomv1alpha1.DefinitionObjectReference{Name: fathomv1alpha1.DefinitionResourceName(saName), UID: string(saUID)},
			Enabled:           true,
			TargetScope:       fathomv1alpha1.DefinitionBindingScope{Namespaces: []fathomv1alpha1.DefinitionDNSLabel{"default"}},
		},
	}
}

func lifecycleBinding() *fathomv1alpha1.AddonDefinitionBinding {
	return lifecycleBindingNamed(lifecycleAddon, lifecycleBindUID, lifecycleDefUID, lifecycleSAName, lifecycleSAUID)
}

func lifecycleServiceAccountNamed(name string, uid types.UID) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: lifecycleNamespace, UID: uid}}
}

func lifecycleServiceAccount() *corev1.ServiceAccount {
	return lifecycleServiceAccountNamed(lifecycleSAName, lifecycleSAUID)
}

// lifecycleFixture wires both reconcilers over one fake API and one registry.
type lifecycleFixture struct {
	t            *testing.T
	client       client.WithWatch
	scheme       *runtime.Scheme
	registry     *registry.Registry
	definitions  *AddonDefinitionReconciler
	bindings     *AddonDefinitionBindingReconciler
	lists        []client.ListOptions
	statusWrites int
	compiles     int
	compileErr   error
}

func newLifecycleFixture(t *testing.T, objs ...client.Object) *lifecycleFixture {
	t.Helper()
	f := &lifecycleFixture{t: t, scheme: runtime.NewScheme()}
	if err := corev1.AddToScheme(f.scheme); err != nil {
		t.Fatalf("core scheme: %v", err)
	}
	if err := fathomv1alpha1.AddToScheme(f.scheme); err != nil {
		t.Fatalf("fathom scheme: %v", err)
	}
	f.client = fake.NewClientBuilder().
		WithScheme(f.scheme).
		WithStatusSubresource(&fathomv1alpha1.AddonDefinition{}, &fathomv1alpha1.AddonDefinitionBinding{}).
		WithIndex(&fathomv1alpha1.AddonDefinitionBinding{}, IndexBindingDefinitionName, indexBindingByDefinitionName).
		WithIndex(&fathomv1alpha1.AddonDefinitionBinding{}, IndexBindingServiceAccountName, indexBindingByServiceAccountName).
		WithIndex(&fathomv1alpha1.AddonDefinitionBinding{}, IndexBindingServiceAccountUID, indexBindingByServiceAccountUID).
		WithObjects(objs...).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				var options client.ListOptions
				for _, opt := range opts {
					opt.ApplyToList(&options)
				}
				f.lists = append(f.lists, options)
				return c.List(ctx, list, opts...)
			},
			SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				f.statusWrites++
				return c.SubResource(sub).Update(ctx, obj, opts...)
			},
		}).
		Build()
	f.registry = registry.New(logr.Discard())
	f.definitions = &AddonDefinitionReconciler{
		Client:                f.client,
		Scheme:                f.scheme,
		Registry:              f.registry,
		OperatorNamespace:     lifecycleNamespace,
		ManagerServiceAccount: lifecycleManagerSA,
		OperatorBuild:         lifecycleBuild,
		CacheSynced:           func(context.Context) bool { return true },
		Compile:               f.compile,
	}
	f.bindings = &AddonDefinitionBindingReconciler{
		Client:                f.client,
		Scheme:                f.scheme,
		Registry:              f.registry,
		OperatorNamespace:     lifecycleNamespace,
		ManagerServiceAccount: lifecycleManagerSA,
		CacheSynced:           func(context.Context) bool { return true },
	}
	return f
}

// Every field the reconciler needs is mandatory and has no usable zero value:
// a mis-wired manager must fail loudly at the first reconcile rather than
// publish a snapshot that is unattributable or scoped to the wrong namespace.
// OperatorBuild matters as much as the rest — provenance is what lets an
// operator tell which build compiled a published revision.
func TestDefinitionReconcilerRefusesToRunMisWired(t *testing.T) {
	for _, tc := range []struct {
		name   string
		break_ func(*AddonDefinitionReconciler)
	}{
		{"no registry", func(r *AddonDefinitionReconciler) { r.Registry = nil }},
		{"no operator namespace", func(r *AddonDefinitionReconciler) { r.OperatorNamespace = "" }},
		{"no manager service account", func(r *AddonDefinitionReconciler) { r.ManagerServiceAccount = "" }},
		{"no operator build", func(r *AddonDefinitionReconciler) { r.OperatorBuild = "" }},
		{"cache sync unwired", func(r *AddonDefinitionReconciler) { r.CacheSynced = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
			tc.break_(f.definitions)

			_, err := f.reconcileDefinition(lifecycleAddon)
			if err == nil {
				t.Fatal("a mis-wired reconciler returned no error; the mis-wiring would be invisible")
			}
			if f.compiles != 0 {
				t.Fatalf("a mis-wired reconciler compiled %d times; it must refuse before any bounded work", f.compiles)
			}
			requireNoRuntime(t, f.registry, lifecycleAddon)
		})
	}
}

// compile is the compilation seam. It runs the real declarative compiler unless
// a row injects a failure, so a published snapshot is a genuinely compiled one.
func (f *lifecycleFixture) compile(ctx context.Context, def *fathomv1alpha1.AddonDefinition, scope fathomv1alpha1.DefinitionBindingScope) (adapter.Adapter, error) {
	f.compiles++
	if f.compileErr != nil {
		return nil, f.compileErr
	}
	return compileRuntimeDefinition(ctx, def, scope)
}

// admit opens the registry's dispatch gate the way an elected leadership
// session would, so a row can assert what dispatch resolves to.
func (f *lifecycleFixture) admit() {
	f.t.Helper()
	if err := f.registry.OpenRuntimeDispatch(lifecycleLeader); err != nil {
		f.t.Fatalf("open runtime dispatch: %v", err)
	}
}

func (f *lifecycleFixture) reconcileDefinition(name string) (ctrl.Result, error) {
	f.t.Helper()
	return f.definitions.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: name}})
}

func (f *lifecycleFixture) reconcileDefinitionOK(name string) ctrl.Result {
	f.t.Helper()
	res, err := f.reconcileDefinition(name)
	if err != nil {
		f.t.Fatalf("reconcile definition %q: %v", name, err)
	}
	return res
}

func (f *lifecycleFixture) reconcileBinding(name string) (ctrl.Result, error) {
	f.t.Helper()
	return f.bindings.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: lifecycleNamespace, Name: name}})
}

func (f *lifecycleFixture) definition(name string) *fathomv1alpha1.AddonDefinition {
	f.t.Helper()
	var def fathomv1alpha1.AddonDefinition
	if err := f.client.Get(context.Background(), types.NamespacedName{Name: name}, &def); err != nil {
		f.t.Fatalf("get definition %q: %v", name, err)
	}
	return &def
}

func (f *lifecycleFixture) binding(name string) *fathomv1alpha1.AddonDefinitionBinding {
	f.t.Helper()
	var b fathomv1alpha1.AddonDefinitionBinding
	if err := f.client.Get(context.Background(), types.NamespacedName{Namespace: lifecycleNamespace, Name: name}, &b); err != nil {
		f.t.Fatalf("get binding %q: %v", name, err)
	}
	return &b
}

func (f *lifecycleFixture) update(obj client.Object) {
	f.t.Helper()
	if err := f.client.Update(context.Background(), obj); err != nil {
		f.t.Fatalf("update %T %q: %v", obj, obj.GetName(), err)
	}
}

func (f *lifecycleFixture) create(obj client.Object) {
	f.t.Helper()
	if err := f.client.Create(context.Background(), obj); err != nil {
		f.t.Fatalf("create %T %q: %v", obj, obj.GetName(), err)
	}
}

func (f *lifecycleFixture) delete(obj client.Object) {
	f.t.Helper()
	if err := f.client.Delete(context.Background(), obj); err != nil {
		f.t.Fatalf("delete %T %q: %v", obj, obj.GetName(), err)
	}
}

// requireRuntime asserts addonType dispatches to a runtime snapshot of the
// expected revision.
func requireRuntime(t *testing.T, reg *registry.Registry, addonType string, uid types.UID, generation int64) registry.Resolution {
	t.Helper()
	res, err := reg.Resolve(addonType)
	if err != nil {
		t.Fatalf("resolve %q: want a runtime snapshot, got error %v", addonType, err)
	}
	if !res.Runtime {
		t.Fatalf("resolve %q: want the runtime snapshot, got built-in adapter %q", addonType, res.Adapter.Name())
	}
	if res.Revision.DefinitionUID != uid || res.Revision.Generation != generation {
		t.Fatalf("resolve %q: want revision %s/%d, got %s/%d", addonType, uid, generation, res.Revision.DefinitionUID, res.Revision.Generation)
	}
	return res
}

// requireNoRuntime asserts nothing runtime-owned claims addonType any more.
func requireNoRuntime(t *testing.T, reg *registry.Registry, addonType string) {
	t.Helper()
	res, err := reg.Resolve(addonType)
	if err == nil {
		if res.Runtime {
			t.Fatalf("resolve %q: want no runtime snapshot, got revision %s/%d", addonType, res.Revision.DefinitionUID, res.Revision.Generation)
		}
		return
	}
	if !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("resolve %q: want ErrNotFound, got %v", addonType, err)
	}
	for _, entry := range reg.RuntimeEntries() {
		for _, at := range entry.AddonTypes {
			if at == addonType {
				t.Fatalf("resolve %q reported not-found but definition %q still owns a snapshot", addonType, entry.Revision.DefinitionUID)
			}
		}
	}
}

// requireBarrier asserts addonType is suppressed with the expected reason.
func requireBarrier(t *testing.T, reg *registry.Registry, addonType, reason string) *registry.DispatchBarrier {
	t.Helper()
	res, err := reg.Resolve(addonType)
	var barrier *registry.DispatchBarrier
	if !errors.As(err, &barrier) {
		t.Fatalf("resolve %q: want a %s dispatch barrier, got adapter %v / error %v", addonType, reason, res.Adapter, err)
	}
	if barrier.Reason != reason {
		t.Fatalf("resolve %q: want barrier reason %q, got %q (%v)", addonType, reason, barrier.Reason, barrier)
	}
	return barrier
}

// requireCollisionBarsDispatch asserts the outcome a built-in collision names:
// nothing dispatches for the identity. Whichever half suppresses it — the
// registry's barrier for a built-in this process registered, or the refusal to
// publish an identity the release ships as a built-in's — resolution must hand
// a dispatcher neither a runtime snapshot nor a built-in adapter.
func requireCollisionBarsDispatch(t *testing.T, reg *registry.Registry, addonType string) {
	t.Helper()
	res, err := reg.Resolve(addonType)
	if err != nil {
		return
	}
	if res.Runtime {
		t.Fatalf("addon type %q is reported as contested but still dispatches to runtime revision %s/%d",
			addonType, res.Revision.DefinitionUID, res.Revision.Generation)
	}
	t.Fatalf("addon type %q is reported as contested but still dispatches to built-in adapter %q", addonType, res.Adapter.Name())
}

// requireBuiltinCollisionIsEnforced states the invariant the condition is only
// worth anything under: whenever Ready reports BuiltinCollision, the identity
// does not dispatch. A status that claimed a suppression the registry was not
// providing would be strictly worse than no status at all.
func requireBuiltinCollisionIsEnforced(t *testing.T, f *lifecycleFixture, addonType string) {
	t.Helper()
	requireCondition(t, f.definition(addonType).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonBuiltinCollision)
	requireCollisionBarsDispatch(t, f.registry, addonType)
}

func conditionByType(conds []fathomv1alpha1.DefinitionStatusCondition, condType string) (fathomv1alpha1.DefinitionStatusCondition, bool) {
	for _, c := range conds {
		if c.Type == condType {
			return c, true
		}
	}
	return fathomv1alpha1.DefinitionStatusCondition{}, false
}

func requireCondition(t *testing.T, conds []fathomv1alpha1.DefinitionStatusCondition, condType string, status metav1.ConditionStatus, reason string) fathomv1alpha1.DefinitionStatusCondition {
	t.Helper()
	got, ok := conditionByType(conds, condType)
	if !ok {
		t.Fatalf("condition %q is missing; conditions: %+v", condType, conds)
	}
	if got.Status != status {
		t.Fatalf("condition %q: want status %q, got %q (reason %q, message %q)", condType, status, got.Status, got.Reason, got.Message)
	}
	if reason != "" && got.Reason != reason {
		t.Fatalf("condition %q: want reason %q, got %q (message %q)", condType, reason, got.Reason, got.Message)
	}
	return got
}

// --- lifecycle matrix rows -------------------------------------------------

// Row: "Missing definition | No runtime snapshot; Ready=False/UnknownAddonType".
// The AddonCheck-side condition is published by the AddonCheck runtime path
// (T039/T040); the reconciler half of the row is that a name with no stored
// definition owns no snapshot, and that a snapshot left over from a definition
// that has since disappeared is removed rather than kept dispatchable.
func TestMissingDefinitionLeavesNoRuntimeSnapshot(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	f.delete(lifecycleDefinition())
	f.reconcileDefinitionOK(lifecycleAddon)

	requireNoRuntime(t, f.registry, lifecycleAddon)
	if _, err := f.registry.Resolve("never-declared"); !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("resolve of an undeclared addon type: want ErrNotFound, got %v", err)
	}
}

// Row: "Valid definition added | Activate only after valid binding and
// compilation | Requeue".
func TestValidDefinitionActivatesOnlyAfterValidBindingAndCompilation(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleServiceAccount())
	f.admit()

	res := f.reconcileDefinitionOK(lifecycleAddon)
	requireNoRuntime(t, f.registry, lifecycleAddon)
	if f.compiles != 0 {
		t.Fatalf("definition without a binding compiled %d times; compilation must follow authorization", f.compiles)
	}
	if res.RequeueAfter != definitions.MissingInputPoll {
		t.Fatalf("missing binding: want a bounded %s requeue, got %+v", definitions.MissingInputPoll, res)
	}
	def := f.definition(lifecycleAddon)
	requireCondition(t, def.Status.Conditions, definitionConditionAccepted, metav1.ConditionTrue, "")
	requireCondition(t, def.Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonAuthorizationRevoked)

	f.create(lifecycleBinding())
	f.reconcileDefinitionOK(lifecycleAddon)

	resolution := requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)
	if resolution.Provenance.OperatorBuild != lifecycleBuild || resolution.Provenance.AdapterVersion != "1.0.0" {
		t.Fatalf("publication provenance: want build %q and adapterVersion %q, got %+v", lifecycleBuild, "1.0.0", resolution.Provenance)
	}
	if resolution.Revision.SemanticsVersion != 1 || resolution.Revision.SchemaVersion == "" {
		t.Fatalf("publication revision is incomplete: %+v", resolution.Revision)
	}
	def = f.definition(lifecycleAddon)
	requireCondition(t, def.Status.Conditions, definitionConditionReady, metav1.ConditionTrue, "")
	if def.Status.ObservedGeneration != 1 {
		t.Fatalf("status.observedGeneration: want 1, got %d", def.Status.ObservedGeneration)
	}
	if !strings.Contains(def.Status.Revision, string(lifecycleDefUID)) {
		t.Fatalf("status.revision %q does not attribute the publication to the definition UID", def.Status.Revision)
	}
}

// Row: "Edited to valid revision | Replace snapshot; old run becomes
// Superseded | Preserve old evidence with revision; evaluate new snapshot".
// Marking the in-flight run Superseded belongs to T040; the reconciler half is
// that the published snapshot is replaced by the edited revision.
func TestEditedDefinitionReplacesSnapshotWithTheNewRevision(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	edited := f.definition(lifecycleAddon)
	edited.Generation = 2
	edited.Spec.Families[0].Checks[0].Workload.DefaultName = "controller-v2"
	f.update(edited)
	f.reconcileDefinitionOK(lifecycleAddon)

	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 2)
	if entries := f.registry.RuntimeEntries(); len(entries) != 1 {
		t.Fatalf("an edit must replace the snapshot, not add one: %d entries published", len(entries))
	}
	if got := f.definition(lifecycleAddon).Status.ObservedGeneration; got != 2 {
		t.Fatalf("status.observedGeneration after the edit: want 2, got %d", got)
	}
}

// Row: "Invalid edit stored | Remove eligibility; Accepted=False/
// InvalidDefinition | ... no stale adapter fallback". Evidence retention is
// T044; eligibility removal and the reason are this task's half.
func TestInvalidEditStoredRemovesEligibility(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	// A spec that admission would reject but that legacy storage can hold.
	invalid := f.definition(lifecycleAddon)
	invalid.Generation = 2
	invalid.Spec.SemanticsVersion = 2
	f.update(invalid)
	f.reconcileDefinitionOK(lifecycleAddon)

	requireNoRuntime(t, f.registry, lifecycleAddon)
	def := f.definition(lifecycleAddon)
	requireCondition(t, def.Status.Conditions, definitionConditionAccepted, metav1.ConditionFalse, reasonInvalidDefinition)
	requireCondition(t, def.Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonInvalidDefinition)
}

// Row: "Unknown kind submitted | Admission rejects; stored revision unchanged |
// Existing valid snapshot remains; if legacy invalid storage is seen, previous
// row applies". Admission itself is exercised by the envtest specs at the end
// of this file; here the legacy-storage path is pinned.
func TestUnknownCheckKindInLegacyStorageRemovesEligibility(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	legacy := f.definition(lifecycleAddon)
	legacy.Generation = 2
	legacy.Spec.Families[0].Checks[0].Kind = "Telepathy"
	f.update(legacy)
	f.reconcileDefinitionOK(lifecycleAddon)

	requireNoRuntime(t, f.registry, lifecycleAddon)
	requireCondition(t, f.definition(lifecycleAddon).Status.Conditions, definitionConditionAccepted, metav1.ConditionFalse, reasonInvalidDefinition)
}

// Row: "Definition deleted | Remove matching UID only; Ready=False/
// DefinitionUnavailable | Keep original time and revision".
func TestDeletedDefinitionRemovesOnlyItsOwnSnapshot(t *testing.T) {
	const otherAddon = "other-addon"
	otherUID := types.UID("definition-uid-2")
	f := newLifecycleFixture(t,
		lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount(),
		lifecycleDefinitionNamed(otherAddon, otherUID),
		lifecycleBindingNamed(otherAddon, "binding-uid-2", otherUID, "other-addon-reader", "service-account-uid-2"),
		lifecycleServiceAccountNamed("other-addon-reader", "service-account-uid-2"),
	)
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	f.reconcileDefinitionOK(otherAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)
	requireRuntime(t, f.registry, otherAddon, otherUID, 1)

	f.delete(lifecycleDefinition())
	f.reconcileDefinitionOK(lifecycleAddon)

	requireNoRuntime(t, f.registry, lifecycleAddon)
	requireRuntime(t, f.registry, otherAddon, otherUID, 1)

	// The binding outlives its definition and must say so with the matrix reason.
	if _, err := f.reconcileBinding(lifecycleAddon); err != nil {
		t.Fatalf("reconcile orphaned binding: %v", err)
	}
	requireCondition(t, f.binding(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonDefinitionUnavailable)
}

// Row: "Definition deleted | Remove matching UID only", for the deletion that
// is not a disappearance: an object held open by a finalizer is still readable
// and its binding is still valid, so nothing but its deletionTimestamp says the
// administrator asked for it to go. Waiting for the object to vanish would keep
// dispatching a definition that is already being deleted, for as long as the
// finalizer's owner takes.
func TestDefinitionDeletingUnderAFinalizerRemovesOnlyItsOwnSnapshot(t *testing.T) {
	const otherAddon = "other-addon"
	otherUID := types.UID("definition-uid-2")
	held := lifecycleDefinition()
	held.Finalizers = []string{"fathom.skaphos.io/lifecycle-test-hold"}
	f := newLifecycleFixture(t,
		held, lifecycleBinding(), lifecycleServiceAccount(),
		lifecycleDefinitionNamed(otherAddon, otherUID),
		lifecycleBindingNamed(otherAddon, "binding-uid-2", otherUID, "other-addon-reader", "service-account-uid-2"),
		lifecycleServiceAccountNamed("other-addon-reader", "service-account-uid-2"),
	)
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	f.reconcileDefinitionOK(otherAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)
	requireRuntime(t, f.registry, otherAddon, otherUID, 1)

	f.delete(f.definition(lifecycleAddon))

	deleting := f.definition(lifecycleAddon)
	if deleting.DeletionTimestamp.IsZero() {
		t.Fatal("the finalizer must hold the definition open with a deletionTimestamp set")
	}
	f.reconcileDefinitionOK(lifecycleAddon)

	requireNoRuntime(t, f.registry, lifecycleAddon)
	requireRuntime(t, f.registry, otherAddon, otherUID, 1)

	// Still deleting on the next pass: the retirement is idempotent and the
	// definition does not come back while the finalizer holds it.
	f.reconcileDefinitionOK(lifecycleAddon)
	requireNoRuntime(t, f.registry, lifecycleAddon)
	requireRuntime(t, f.registry, otherAddon, otherUID, 1)
}

// Row: "Same name recreated | New UID has no authority inherited from old
// binding | Ready=False/BindingMismatch until administrator authorizes new UID".
func TestRecreatedDefinitionRequiresReauthorizationOfTheNewUID(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	// Delete and recreate under the same name: a new UID, the same old binding.
	f.delete(lifecycleDefinition())
	recreated := lifecycleDefinitionNamed(lifecycleAddon, "definition-uid-recreated")
	f.create(recreated)
	f.reconcileDefinitionOK(lifecycleAddon)

	requireNoRuntime(t, f.registry, lifecycleAddon)
	requireCondition(t, f.definition(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonBindingMismatch)
	if _, err := f.reconcileBinding(lifecycleAddon); err != nil {
		t.Fatalf("reconcile stale binding: %v", err)
	}
	requireCondition(t, f.binding(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonBindingMismatch)

	// The administrator authorizes the new UID explicitly.
	authorized := f.binding(lifecycleAddon)
	authorized.Spec.DefinitionRef.UID = "definition-uid-recreated"
	authorized.Generation = 2
	f.update(authorized)
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, "definition-uid-recreated", 1)
}

// Row: "Binding disabled/deleted or SA replaced | Revoke eligibility;
// Ready=False/AuthorizationRevoked or BindingMismatch | ... explicit
// reauthorization recovers".
func TestBindingRevocationRemovesEligibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		revoke func(f *lifecycleFixture)
		reason string
	}{
		{"disabled", func(f *lifecycleFixture) {
			b := f.binding(lifecycleAddon)
			b.Spec.Enabled = false
			b.Generation = 2
			f.update(b)
		}, reasonAuthorizationRevoked},
		{"deleted", func(f *lifecycleFixture) { f.delete(lifecycleBinding()) }, reasonAuthorizationRevoked},
		{"service account replaced", func(f *lifecycleFixture) {
			f.delete(lifecycleServiceAccount())
			f.create(lifecycleServiceAccountNamed(lifecycleSAName, "service-account-uid-replaced"))
		}, reasonBindingMismatch},
		{"service account deleted", func(f *lifecycleFixture) { f.delete(lifecycleServiceAccount()) }, reasonBindingMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
			f.admit()
			f.reconcileDefinitionOK(lifecycleAddon)
			requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

			tc.revoke(f)
			f.reconcileDefinitionOK(lifecycleAddon)

			requireNoRuntime(t, f.registry, lifecycleAddon)
			requireCondition(t, f.definition(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, tc.reason)
		})
	}
}

// Row: "RBAC denies an API read | No completed health result; Ready=False/
// AccessDenied | Old evidence retained ...; bounded retries use actual current
// permissions".
func TestDeniedControlPlaneReadReportsAccessDenied(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	denied := apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, lifecycleSAName, errors.New("RBAC: denied"))
	f.definitions.Client = deniedReadClient{Client: f.client, err: denied}
	f.bindings.Client = f.definitions.Client

	if _, err := f.reconcileDefinition(lifecycleAddon); err == nil {
		t.Fatal("a denied control-plane read must be retried, so Reconcile must report the error")
	}
	requireCondition(t, f.definition(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonAccessDenied)
	// Nothing about the stored state is known to have changed, so the published
	// snapshot is retained: an API denial must not turn into a self-inflicted
	// outage, and no run can publish evidence without its own authority fence.
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)
	if _, err := f.reconcileBinding(lifecycleAddon); err == nil {
		t.Fatal("binding reconcile must also report the denied read")
	}
	requireCondition(t, f.binding(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonAccessDenied)
}

// deniedReadClient fails every ServiceAccount read with a canned error.
type deniedReadClient struct {
	client.Client
	err error
}

func (c deniedReadClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*corev1.ServiceAccount); ok {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

// Row: "Revocation during run | Observed revocation cancels/rejects ... |
// Latest attempt explains discard". Cancelling in-flight evaluation and
// discarding its publication is T040/T042; the reconciler half is that an
// observed revocation removes eligibility immediately, even while the binding
// still reports active runs, so no new dispatch can start.
func TestObservedRevocationDuringActiveRunsRemovesEligibility(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	running := f.binding(lifecycleAddon)
	running.Status.ActiveRuns = 1
	if err := f.client.Status().Update(context.Background(), running); err != nil {
		t.Fatalf("record an active run: %v", err)
	}
	revoked := f.binding(lifecycleAddon)
	revoked.Spec.Enabled = false
	revoked.Generation = 2
	f.update(revoked)

	f.reconcileDefinitionOK(lifecycleAddon)

	requireNoRuntime(t, f.registry, lifecycleAddon)
	requireCondition(t, f.definition(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonAuthorizationRevoked)
	if got := f.binding(lifecycleAddon).Status.ActiveRuns; got != 1 {
		t.Fatalf("active-run accounting must be left to the run owner, got %d", got)
	}
}

// Row: "Restart or partial informer sync | No runtime execution until
// synchronized and directly validated | ... no manager fallback".
func TestUnsynchronizedCacheBlocksRuntimePublication(t *testing.T) {
	t.Run("not yet synchronized", func(t *testing.T) {
		f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
		f.admit()
		f.definitions.CacheSynced = func(context.Context) bool { return false }

		res, err := f.reconcileDefinition(lifecycleAddon)
		if err != nil {
			t.Fatalf("an unsynchronized cache is not an error: %v", err)
		}
		if res.RequeueAfter <= 0 {
			t.Fatalf("an unsynchronized reconcile must requeue, got %+v", res)
		}
		requireNoRuntime(t, f.registry, lifecycleAddon)
		if f.compiles != 0 {
			t.Fatalf("compilation ran %d times before informer synchronization", f.compiles)
		}
	})

	t.Run("gate not wired", func(t *testing.T) {
		f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
		f.admit()
		f.definitions.CacheSynced = nil

		if _, err := f.reconcileDefinition(lifecycleAddon); !errors.Is(err, errCacheSyncUnwired) {
			t.Fatalf("an unwired sync gate must fail closed with errCacheSyncUnwired, got %v", err)
		}
		requireNoRuntime(t, f.registry, lifecycleAddon)
	})
}

// Row: "Binding/grants recover | Capture new authority context; Ready remains
// false until evaluation succeeds | New run supplies fresh evidence". The
// AddonCheck's Ready is T044's; recovering eligibility is this task's.
func TestRecoveredBindingRepublishesRuntimeAuthority(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)

	disabled := f.binding(lifecycleAddon)
	disabled.Spec.Enabled = false
	disabled.Generation = 2
	f.update(disabled)
	f.reconcileDefinitionOK(lifecycleAddon)
	requireNoRuntime(t, f.registry, lifecycleAddon)

	// The administrator restores the grant and re-enables the binding.
	restored := f.binding(lifecycleAddon)
	restored.Spec.Enabled = true
	restored.Generation = 3
	f.update(restored)
	f.reconcileDefinitionOK(lifecycleAddon)

	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)
	requireCondition(t, f.definition(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionTrue, "")
	if _, err := f.reconcileBinding(lifecycleAddon); err != nil {
		t.Fatalf("reconcile recovered binding: %v", err)
	}
	b := f.binding(lifecycleAddon)
	requireCondition(t, b.Status.Conditions, definitionConditionAccepted, metav1.ConditionTrue, "")
	if b.Status.ObservedGeneration != 3 {
		t.Fatalf("binding status.observedGeneration: want 3, got %d", b.Status.ObservedGeneration)
	}
}

// Row: "Edit during evaluation | Final revision/context mismatch discards
// completion | Superseded attempt recorded; enqueue current revision". The
// run-completion fence is T039/T040. What this fixture pins is the reconciler's
// half of the same invariant: a reconcile publishes exactly the revision it
// validated, so an edit that lands mid-reconcile cannot be published as if it
// had been validated, and the edit is then published by its own reconcile.
func TestEditDuringReconcilePublishesOnlyTheValidatedRevision(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()

	held := 0
	f.definitions.barrier = func(ctx context.Context, phase string, def *fathomv1alpha1.AddonDefinition) {
		if phase != barrierBeforePublish || held > 0 {
			return
		}
		held++
		if def.Generation != 1 {
			t.Errorf("the held reconcile must still carry the revision it validated, got generation %d", def.Generation)
		}
		edited := f.definition(lifecycleAddon)
		edited.Generation = 2
		edited.Spec.Families[0].Checks[0].Workload.DefaultName = "controller-v2"
		f.update(edited)
	}

	// The edit lands between this reconcile's validation and its status write,
	// so the write loses the race with a conflict — the requeue that follows is
	// what publishes the edited revision, and nothing half-validated escapes.
	if _, err := f.reconcileDefinition(lifecycleAddon); err != nil && !apierrors.IsConflict(err) {
		t.Fatalf("reconcile held at the publication barrier: %v", err)
	}
	if held != 1 {
		t.Fatalf("the publication barrier did not hold the reconcile (held=%d)", held)
	}
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	f.definitions.barrier = nil
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 2)
}

// Row: "Two valid runtime names claim identity | Name equality/uniqueness
// prohibits it | Invalid legacy collision fails closed; no winner by arrival
// order". Cluster name uniqueness is proven by the envtest spec below; here the
// legacy state is constructed directly and must fail closed.
func TestTwoRuntimeOwnersOfOneIdentityFailClosed(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	// A legacy owner of the same identity under a different UID.
	legacy := registry.RuntimeEntry{
		Adapter: lifecycleStubAdapter{name: lifecycleAddon, addonTypes: []string{lifecycleAddon}},
		Revision: registry.RuntimeRevision{
			DefinitionUID: "definition-uid-legacy", Generation: 7, SchemaVersion: "v1alpha1", SemanticsVersion: 1,
		},
		Provenance: registry.RuntimeProvenance{OperatorBuild: lifecycleBuild, AdapterVersion: "1.0.0"},
	}
	var barrier *registry.DispatchBarrier
	if err := f.registry.SetRuntime(legacy); !errors.As(err, &barrier) {
		t.Fatalf("publishing a second owner of one identity must raise a barrier, got %v", err)
	}
	contested := requireBarrier(t, f.registry, lifecycleAddon, registry.ReasonRuntimeCollision)
	if len(contested.Claimants) != 2 {
		t.Fatalf("a contested identity must name every claimant, got %v", contested.Claimants)
	}

	// Reconciling against the live API object resolves the contest on evidence:
	// only one definition object can hold the name, so the other owner is dead.
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	// With no live definition at all, neither claimant survives.
	f.delete(lifecycleDefinition())
	if err := f.registry.SetRuntime(legacy); err != nil && !errors.As(err, &barrier) {
		t.Fatalf("republish legacy owner: %v", err)
	}
	f.reconcileDefinitionOK(lifecycleAddon)
	requireNoRuntime(t, f.registry, lifecycleAddon)
}

// Row: "New built-in collides | Conflict barrier; Ready=False/BuiltinCollision |
// Preserve evidence; explicit migration/deletion resolves barrier".
//
// The definition must still be published so the registry can suppress BOTH
// candidates. Dropping it instead would let the newly shipped built-in silently
// reinterpret every AddonCheck of that identity, which is exactly what the
// barrier exists to prevent.
func TestBuiltinCollisionSuppressesBothCandidates(t *testing.T) {
	// A built-in this process registered: the registry raises the real barrier.
	t.Run("registered built-in", func(t *testing.T) {
		const addon = "vendor-addon"
		const saName = "vendor-addon-reader"
		f := newLifecycleFixture(t,
			lifecycleDefinitionNamed(addon, "definition-uid-vendor"),
			lifecycleBindingNamed(addon, "binding-uid-vendor", "definition-uid-vendor", saName, "service-account-uid-vendor"),
			lifecycleServiceAccountNamed(saName, "service-account-uid-vendor"),
		)
		f.admit()
		builtin := lifecycleStubAdapter{name: "vendor-builtin", addonTypes: []string{addon}}
		if err := f.registry.Register(builtin); err != nil {
			t.Fatalf("register built-in: %v", err)
		}

		f.reconcileDefinitionOK(addon)

		barrier := requireBarrier(t, f.registry, addon, registry.ReasonBuiltinCollision)
		if len(barrier.Claimants) != 2 {
			t.Fatalf("a built-in collision must name both claimants, got %v", barrier.Claimants)
		}
		requireBuiltinCollisionIsEnforced(t, f, addon)
		if _, err := f.reconcileBinding(addon); err != nil {
			t.Fatalf("reconcile binding: %v", err)
		}
		requireCondition(t, f.binding(addon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonBuiltinCollision)
	})

	// An identity this release ships as a built-in, per the generated inventory
	// `fathomctl definition collisions` compares against. Definition and binding
	// must agree with the CLI rather than each other.
	//
	// The registry raises no barrier here, because this process registered no
	// such built-in: the inventory claim is enforced by refusing to publish at
	// all. Publishing and then reporting the collision anyway — which is what
	// this row used to do — left the identity dispatchable while its status
	// said it was barred, so the suppression rested entirely on the registry
	// and the inventory agreeing, which nothing asserts.
	t.Run("shipped inventory", func(t *testing.T) {
		const addon = "coredns"
		const saName = "coredns-runtime-reader"
		f := newLifecycleFixture(t,
			lifecycleDefinitionNamed(addon, "definition-uid-coredns"),
			lifecycleBindingNamed(addon, "binding-uid-coredns", "definition-uid-coredns", saName, "service-account-uid-coredns"),
			lifecycleServiceAccountNamed(saName, "service-account-uid-coredns"),
		)
		f.admit()

		f.reconcileDefinitionOK(addon)
		if _, err := f.reconcileBinding(addon); err != nil {
			t.Fatalf("reconcile binding: %v", err)
		}

		requireBuiltinCollisionIsEnforced(t, f, addon)
		requireNoRuntime(t, f.registry, addon)
		requireCondition(t, f.binding(addon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonBuiltinCollision)
		if f.compiles != 0 {
			t.Fatalf("a contested identity compiled %d times; bounded work must not be spent on a definition that cannot dispatch", f.compiles)
		}
	})

	// A snapshot published before the inventory claim was observable — an
	// operator upgrade that adds the built-in to the release — must be retired,
	// not left dispatching behind a status that says it is barred.
	t.Run("shipped inventory retires a published snapshot", func(t *testing.T) {
		const addon = "coredns"
		const saName = "coredns-runtime-reader"
		const defUID = types.UID("definition-uid-coredns")
		f := newLifecycleFixture(t,
			lifecycleDefinitionNamed(addon, defUID),
			lifecycleBindingNamed(addon, "binding-uid-coredns", defUID, saName, "service-account-uid-coredns"),
			lifecycleServiceAccountNamed(saName, "service-account-uid-coredns"),
		)
		f.admit()
		// The release before the upgrade shipped no built-in for this identity.
		f.definitions.Builtins = func() []string { return nil }
		f.bindings.Builtins = f.definitions.Builtins
		f.reconcileDefinitionOK(addon)
		requireRuntime(t, f.registry, addon, defUID, 1)

		// A dead claimant of the same identity, published by an owner that no
		// longer exists. It is suppressed while both are published, but it must
		// not be what survives the collision.
		legacy := registry.RuntimeEntry{
			Adapter: lifecycleStubAdapter{name: addon, addonTypes: []string{addon}},
			Revision: registry.RuntimeRevision{
				DefinitionUID: "definition-uid-coredns-legacy", Generation: 3, SchemaVersion: "v1alpha1", SemanticsVersion: 1,
			},
			Provenance: registry.RuntimeProvenance{OperatorBuild: lifecycleBuild, AdapterVersion: "1.0.0"},
		}
		var contested *registry.DispatchBarrier
		if err := f.registry.SetRuntime(legacy); !errors.As(err, &contested) {
			t.Fatalf("publishing a second owner of one identity must raise a barrier, got %v", err)
		}

		// The upgrade adds it to the shipped inventory.
		f.definitions.Builtins = nil
		f.bindings.Builtins = nil
		f.reconcileDefinitionOK(addon)

		requireBuiltinCollisionIsEnforced(t, f, addon)
		requireNoRuntime(t, f.registry, addon)
		if entries := f.registry.RuntimeEntries(); len(entries) != 0 {
			t.Fatalf("no claimant of a contested identity may stay published, got %+v", entries)
		}
	})
}

// The registry's barrier and the reported collision must never disagree, which
// is only guaranteed if a publication that raised no barrier for a contested
// identity is withdrawn. The one way to produce that state is a compilation
// that publishes under some identity other than the definition's own, so that
// is what this fixture injects: the definition is contested, its snapshot would
// otherwise stay dispatchable under the identity it advertised, and the status
// would report a suppression nobody was enforcing.
func TestContestedDefinitionLeavesNothingPublishedWhenTheRegistryRaisesNoBarrier(t *testing.T) {
	const addon = "vendor-addon"
	const saName = "vendor-addon-reader"
	f := newLifecycleFixture(t,
		lifecycleDefinitionNamed(addon, "definition-uid-vendor"),
		lifecycleBindingNamed(addon, "binding-uid-vendor", "definition-uid-vendor", saName, "service-account-uid-vendor"),
		lifecycleServiceAccountNamed(saName, "service-account-uid-vendor"),
	)
	f.admit()
	if err := f.registry.Register(lifecycleStubAdapter{name: "vendor-builtin", addonTypes: []string{addon}}); err != nil {
		t.Fatalf("register built-in: %v", err)
	}
	f.definitions.Compile = func(context.Context, *fathomv1alpha1.AddonDefinition, fathomv1alpha1.DefinitionBindingScope) (adapter.Adapter, error) {
		return lifecycleStubAdapter{name: addon, addonTypes: []string{"some-other-identity"}}, nil
	}

	f.reconcileDefinitionOK(addon)

	requireCondition(t, f.definition(addon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonBuiltinCollision)
	if entries := f.registry.RuntimeEntries(); len(entries) != 0 {
		t.Fatalf("a contested definition must leave no snapshot published, got %+v", entries)
	}
	requireNoRuntime(t, f.registry, "some-other-identity")
}

// Row: "Valid definition added | Activate only after valid binding and
// compilation". A contested identity is no exception: without authority
// nothing is compiled and nothing is published, so the bounded compile budget
// is not spent on a revision that cannot be activated and no snapshot is
// published that would have to be barred afterwards.
func TestContestedIdentityWithoutABindingIsNeitherCompiledNorPublished(t *testing.T) {
	const addon = "coredns"
	f := newLifecycleFixture(t, lifecycleDefinitionNamed(addon, "definition-uid-coredns"))
	f.admit()

	res := f.reconcileDefinitionOK(addon)

	if f.compiles != 0 {
		t.Fatalf("an unauthorized definition compiled %d times; compilation must follow authorization", f.compiles)
	}
	requireNoRuntime(t, f.registry, addon)
	requireCondition(t, f.definition(addon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonAuthorizationRevoked)
	if res.RequeueAfter != definitions.MissingInputPoll {
		t.Fatalf("missing binding: want a bounded %s requeue, got %+v", definitions.MissingInputPoll, res)
	}
}

// The binding's namespace allowlist must authorize every target the definition
// declares. A scope that covers only part of it is a mismatch, never a partial
// run over the authorized subset.
func TestBindingScopeMustCoverTheDefinitionTargets(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	narrowed := f.binding(lifecycleAddon)
	narrowed.Spec.TargetScope.Namespaces = []fathomv1alpha1.DefinitionDNSLabel{"somewhere-else"}
	narrowed.Generation = 2
	f.update(narrowed)
	f.reconcileDefinitionOK(lifecycleAddon)

	requireNoRuntime(t, f.registry, lifecycleAddon)
	requireCondition(t, f.definition(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonBindingMismatch)
}

// Row: "Deadline, size, parser or read budget exhausted | Attempt Error with
// specific reason; Ready=False | No partial Pass; bounded retry and other
// definitions continue". Run-time budgets are enforced by internal/adapter/
// runtime; the reconciler's half is compilation, whose failure must neither
// publish a partial snapshot nor stop unrelated definitions.
func TestCompilationBudgetFailureLeavesOtherDefinitionsRunning(t *testing.T) {
	const otherAddon = "other-addon"
	otherUID := types.UID("definition-uid-2")
	f := newLifecycleFixture(t,
		lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount(),
		lifecycleDefinitionNamed(otherAddon, otherUID),
		lifecycleBindingNamed(otherAddon, "binding-uid-2", otherUID, "other-addon-reader", "service-account-uid-2"),
		lifecycleServiceAccountNamed("other-addon-reader", "service-account-uid-2"),
	)
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	// The next revision exhausts the compile budget: eligibility goes away
	// rather than leaving the superseded snapshot dispatchable.
	f.compileErr = context.DeadlineExceeded
	edited := f.definition(lifecycleAddon)
	edited.Generation = 2
	f.update(edited)

	f.reconcileDefinitionOK(lifecycleAddon)
	requireNoRuntime(t, f.registry, lifecycleAddon)
	requireCondition(t, f.definition(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonInvalidDefinition)

	f.compileErr = nil
	f.reconcileDefinitionOK(otherAddon)
	requireRuntime(t, f.registry, otherAddon, otherUID, 1)
}

// --- T038 clauses that are not one matrix row ------------------------------

// The dedicated reader must be dedicated: never the manager's own identity,
// never a built-in's, and never shared with another binding. The checks mirror
// impersonation.ResolveRuntimeAuthority, which fences the same conditions
// immediately before a run.
func TestDedicatedServiceAccountUniquenessAndExclusions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *lifecycleFixture)
	}{
		{"manager identity", func(f *lifecycleFixture) {
			b := f.binding(lifecycleAddon)
			b.Spec.ServiceAccountRef.Name = lifecycleManagerSA
			f.update(b)
			f.create(lifecycleServiceAccountNamed(lifecycleManagerSA, lifecycleSAUID))
		}},
		{"built-in identity", func(f *lifecycleFixture) {
			name := adapter.AddonServiceAccountName("coredns")
			b := f.binding(lifecycleAddon)
			b.Spec.ServiceAccountRef.Name = fathomv1alpha1.DefinitionResourceName(name)
			f.update(b)
			f.create(lifecycleServiceAccountNamed(name, lifecycleSAUID))
		}},
		{"renamed built-in identity", func(f *lifecycleFixture) {
			name := "fathom-" + adapter.AddonServiceAccountName("coredns")
			b := f.binding(lifecycleAddon)
			b.Spec.ServiceAccountRef.Name = fathomv1alpha1.DefinitionResourceName(name)
			f.update(b)
			f.create(lifecycleServiceAccountNamed(name, lifecycleSAUID))
		}},
		{"reserved built-in label", func(f *lifecycleFixture) {
			sa := lifecycleServiceAccount()
			sa.Labels = map[string]string{adapter.AddonLabel: "coredns"}
			f.update(sa)
		}},
		{"shared with another binding", func(f *lifecycleFixture) {
			other := lifecycleBindingNamed("other-addon", "binding-uid-2", "definition-uid-2", lifecycleSAName, lifecycleSAUID)
			f.create(other)
		}},
		{"two bindings claim one definition", func(f *lifecycleFixture) {
			// Only legacy or invalid storage can produce this; it must fail
			// closed rather than let arrival order pick the authority.
			rival := lifecycleBindingNamed("rival-binding", "binding-uid-3", lifecycleDefUID, "rival-reader", "service-account-uid-3")
			rival.Spec.DefinitionRef.Name = lifecycleAddon
			f.create(rival)
			f.create(lifecycleServiceAccountNamed("rival-reader", "service-account-uid-3"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
			f.admit()
			tc.setup(f)

			f.reconcileDefinitionOK(lifecycleAddon)

			requireNoRuntime(t, f.registry, lifecycleAddon)
			requireCondition(t, f.definition(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonBindingMismatch)
		})
	}
}

// Dependency requeues must be indexed: a binding change enqueues exactly its
// own definition, and the reconciler never lists bindings without a selector.
func TestBindingDependencyRequeuesAreIndexed(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)

	if len(f.lists) == 0 {
		t.Fatal("the reconciler resolved authority without listing bindings at all")
	}
	for i, opts := range f.lists {
		if opts.FieldSelector == nil {
			t.Fatalf("list #%d ran without a field selector; dependency lookups must use the registered indexes", i)
		}
		if opts.Namespace != lifecycleNamespace {
			t.Fatalf("list #%d ran outside the operator namespace (%q)", i, opts.Namespace)
		}
	}

	requests := f.definitions.definitionsForBinding(context.Background(), lifecycleBinding())
	if len(requests) != 1 || requests[0].Name != lifecycleAddon || requests[0].Namespace != "" {
		t.Fatalf("a binding change must enqueue exactly its own cluster-scoped definition, got %+v", requests)
	}
	foreign := lifecycleBinding()
	foreign.Namespace = "tenant"
	if got := f.definitions.definitionsForBinding(context.Background(), foreign); len(got) != 0 {
		t.Fatalf("a binding outside the operator namespace carries no authority and must enqueue nothing, got %+v", got)
	}
	if got := f.bindings.bindingsForDefinition(context.Background(), lifecycleDefinition()); len(got) != 1 || got[0].Name != lifecycleAddon {
		t.Fatalf("a definition change must enqueue its binding through the index, got %+v", got)
	}
}

// Only the operator namespace holds authority. A binding elsewhere is not
// reconciled at all: writing a status on it would imply it means something.
func TestBindingsOutsideTheOperatorNamespaceCarryNoAuthority(t *testing.T) {
	foreign := lifecycleBinding()
	foreign.Namespace = "tenant"
	f := newLifecycleFixture(t, lifecycleDefinition(), foreign, lifecycleServiceAccount())
	f.admit()

	res, err := f.bindings.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "tenant", Name: lifecycleAddon},
	})
	if err != nil || res != (ctrl.Result{}) {
		t.Fatalf("a foreign-namespace binding must be ignored outright, got %+v / %v", res, err)
	}
	if f.statusWrites != 0 {
		t.Fatalf("a foreign-namespace binding must not be given a status (%d writes)", f.statusWrites)
	}

	// It cannot authorize its definition either.
	f.reconcileDefinitionOK(lifecycleAddon)
	requireNoRuntime(t, f.registry, lifecycleAddon)
	requireCondition(t, f.definition(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonAuthorizationRevoked)
}

// Publication is not admission: the reconcilers publish snapshots, but only an
// elected leadership session may open the dispatch gate (T047). Runtime loading
// stays default-off until it does.
func TestPublicationNeverAdmitsRuntimeDispatch(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())

	f.reconcileDefinitionOK(lifecycleAddon)

	barrier := requireBarrier(t, f.registry, lifecycleAddon, registry.ReasonAdmissionClosed)
	if !strings.Contains(barrier.Detail, "admitted") {
		t.Fatalf("a closed gate must explain itself, got %q", barrier.Detail)
	}
	requireCondition(t, f.definition(lifecycleAddon).Status.Conditions, definitionConditionReady, metav1.ConditionTrue, "")
}

// Retiring dead claimants of an identity must never take this definition's own
// live snapshot down on the way past. The barrier seam observes the registry at
// the instant just before republication, which is the only moment a gap could
// exist: a dispatcher resolving inside that window would otherwise see the
// identity as unknown and report a spurious UnknownAddonType.
func TestRepublicationNeverExposesADispatchGap(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)

	observed := 0
	f.definitions.barrier = func(_ context.Context, phase string, _ *fathomv1alpha1.AddonDefinition) {
		if phase != barrierBeforePublish {
			return
		}
		observed++
		requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)
	}

	f.reconcileDefinitionOK(lifecycleAddon)

	if observed != 1 {
		t.Fatalf("the publication barrier did not observe the republication (observed=%d)", observed)
	}
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)
}

// Reconciles are idempotent: a second pass over unchanged inputs writes no
// status and republishes nothing new.
func TestReconcileIsIdempotentForUnchangedInputs(t *testing.T) {
	f := newLifecycleFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount())
	f.admit()
	f.reconcileDefinitionOK(lifecycleAddon)
	if _, err := f.reconcileBinding(lifecycleAddon); err != nil {
		t.Fatalf("reconcile binding: %v", err)
	}
	first := f.statusWrites
	if first == 0 {
		t.Fatal("the first pass wrote no status at all")
	}

	f.reconcileDefinitionOK(lifecycleAddon)
	if _, err := f.reconcileBinding(lifecycleAddon); err != nil {
		t.Fatalf("reconcile binding again: %v", err)
	}

	if f.statusWrites != first {
		t.Fatalf("a repeated reconcile wrote status again (%d writes, was %d)", f.statusWrites, first)
	}
	requireRuntime(t, f.registry, lifecycleAddon, lifecycleDefUID, 1)
}

// --- envtest rows: what only a real API server can prove --------------------

var _ = Describe("AddonDefinition lifecycle admission", Ordered, func() {
	const addon = "lifecycle-envtest-addon"

	AfterAll(func() {
		_ = k8sClient.Delete(ctx, &fathomv1alpha1.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: addon}})
	})

	It("rejects an unknown check kind at admission, leaving the stored revision unchanged", func() {
		def := lifecycleDefinitionNamed(addon, "")
		def.Generation = 0
		Expect(k8sClient.Create(ctx, def)).To(Succeed())
		stored := &fathomv1alpha1.AddonDefinition{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: addon}, stored)).To(Succeed())
		originalUID, originalGeneration := stored.UID, stored.Generation

		unknown := stored.DeepCopy()
		unknown.Spec.Families[0].Checks[0].Kind = "Telepathy"
		Expect(k8sClient.Update(ctx, unknown)).NotTo(Succeed())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: addon}, stored)).To(Succeed())
		Expect(stored.UID).To(Equal(originalUID))
		Expect(stored.Generation).To(Equal(originalGeneration))
		Expect(stored.Spec.Families[0].Checks[0].Kind).To(Equal("Workload"))
	})

	It("prohibits a second definition claiming the same identity", func() {
		duplicate := lifecycleDefinitionNamed(addon, "")
		duplicate.Generation = 0
		err := k8sClient.Create(ctx, duplicate)
		Expect(apierrors.IsAlreadyExists(err)).To(BeTrue(), "name uniqueness must prohibit two definitions of one identity: %v", err)
	})

	It("holds addonType immutable and gives a recreated definition a new UID", func() {
		stored := &fathomv1alpha1.AddonDefinition{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: addon}, stored)).To(Succeed())
		originalUID := stored.UID

		renamed := stored.DeepCopy()
		renamed.Spec.AddonType = "something-else"
		Expect(k8sClient.Update(ctx, renamed)).NotTo(Succeed())

		Expect(k8sClient.Delete(ctx, stored)).To(Succeed())
		recreated := lifecycleDefinitionNamed(addon, "")
		recreated.Generation = 0
		Eventually(func() error { return k8sClient.Create(ctx, recreated) }).Should(Succeed())
		Expect(recreated.UID).NotTo(Equal(originalUID))
	})
})
