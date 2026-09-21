/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/declarative"
	"github.com/skaphos/fathom/internal/adapter/registry"
	"github.com/skaphos/fathom/pkg/adapter"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
)

// runtimeSyncRequeue is how long a reconcile waits when the manager's informers
// are not synchronized yet. It is short because it is a startup condition, not
// a missing input.
const runtimeSyncRequeue = 5 * time.Second

// Barrier phases. The barrier seam exists so a test can hold a reconcile at an
// exact point — most importantly immediately before publication — and mutate
// the API underneath it, which is the only way to make the "edit during
// evaluation" and fence-ordering rows of the lifecycle matrix deterministic
// instead of racy. Production leaves the seam nil and pays nothing for it.
const (
	barrierAfterValidate  = "afterValidate"
	barrierAfterAuthority = "afterAuthority"
	barrierBeforePublish  = "beforePublish"
	barrierAfterPublish   = "afterPublish"
)

// compileRuntimeDefinition is the production compilation seam: the declarative
// compiler, bounded by its own compile deadline, with the binding's namespace
// allowlist applied before any evaluator can exist.
func compileRuntimeDefinition(ctx context.Context, def *fathomv1alpha1.AddonDefinition, scope fathomv1alpha1.DefinitionBindingScope) (adapter.Adapter, error) {
	return declarative.CompileRuntimeScoped(ctx, def, scope)
}

// AddonDefinitionReconciler turns stored AddonDefinition revisions into
// published runtime snapshots, and removes them again the moment the stored
// state stops justifying one.
//
// It publishes; it never admits. Opening the registry's dispatch gate belongs
// to the elected leadership session (T047), so until that wiring exists a
// published snapshot resolves to a RuntimeAdmissionClosed barrier and runtime
// loading stays default-off.
type AddonDefinitionReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Registry owns dispatch. It is the single enforcement point: this
	// reconciler decides eligibility, the registry decides what a dispatcher
	// may see.
	Registry *registry.Registry

	// OperatorNamespace is the only namespace whose bindings carry authority.
	OperatorNamespace string
	// ManagerServiceAccount is the operator's own identity, which can never be
	// a dedicated reader.
	ManagerServiceAccount string
	// OperatorBuild is this build's identifier, recorded as publication
	// provenance so a snapshot is attributable to the compiler that produced
	// it. It is mandatory: an unattributable snapshot is not publishable.
	OperatorBuild string
	// SchemaVersion is the stored API version a revision was compiled from.
	// Empty means the API's current version.
	SchemaVersion string

	// Compile converts a validated definition into an adapter. Optional; nil
	// uses the declarative compiler.
	Compile func(ctx context.Context, def *fathomv1alpha1.AddonDefinition, scope fathomv1alpha1.DefinitionBindingScope) (adapter.Adapter, error)

	// CacheSynced reports whether this manager's informers are synchronized.
	// There is no default: see errCacheSyncUnwired.
	CacheSynced func(context.Context) bool

	// Builtins overrides the shipped collision inventory in tests.
	Builtins func() []string

	// barrier is the test-only rendezvous seam described above.
	barrier func(ctx context.Context, phase string, def *fathomv1alpha1.AddonDefinition)
}

// Definitions are read and watched and their status subresource is written.
// The reconciler never writes a definition or binding spec, never creates a
// grant, never issues a SubjectAccessReview and never reads a Lease: an
// administrator grants authority out of band and this controller only observes
// it. The binding grants are declared on the binding reconciler.
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addondefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addondefinitions/status,verbs=get;update;patch

// Reconcile is bounded and idempotent: one definition read, one service-account
// read, three indexed binding lists, at most one bounded compilation and one
// atomic snapshot publication. Nothing in it waits on evaluation.
func (r *AddonDefinitionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	if r.Registry == nil || r.OperatorNamespace == "" || r.ManagerServiceAccount == "" || r.OperatorBuild == "" {
		return ctrl.Result{}, errors.New(
			"controller: AddonDefinitionReconciler requires a registry, an explicit operator namespace, the manager service account and the operator build")
	}
	if r.CacheSynced == nil {
		return ctrl.Result{}, errCacheSyncUnwired
	}
	if !r.CacheSynced(ctx) {
		// No runtime publication before synchronization: a partial informer
		// sync cannot distinguish "no binding" from "binding not seen yet".
		return ctrl.Result{RequeueAfter: runtimeSyncRequeue}, nil
	}

	var def fathomv1alpha1.AddonDefinition
	if err := r.Get(ctx, req.NamespacedName, &def); err != nil {
		if apierrors.IsNotFound(err) {
			// Owner-aware: only snapshots claiming this identity are retired,
			// and only because the live API says nothing holds the name.
			r.retireIdentity(log, req.Name, "")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if !def.DeletionTimestamp.IsZero() {
		r.retireIdentity(log, def.Name, "")
		return ctrl.Result{}, nil
	}

	accepted, ready, result, retry := r.evaluate(ctx, log, &def)
	if err := r.writeStatus(ctx, &def, accepted, ready); err != nil {
		return ctrl.Result{}, err
	}
	return result, retry
}

// evaluate decides eligibility and performs the registry side effects. It
// returns the two conditions to publish plus the requeue/retry outcome.
func (r *AddonDefinitionReconciler) evaluate(ctx context.Context, log logr.Logger, def *fathomv1alpha1.AddonDefinition) (accepted, ready conditionSpec, result ctrl.Result, retry error) {
	// Semantic validation beyond schema admission. A stored spec that fails it
	// loses eligibility immediately: there is no stale-adapter fallback.
	if err := definitions.Validate(def); err != nil {
		r.retireIdentity(log, def.Name, "")
		return definitionAcceptedCondition(err),
			conditionSpec{Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonInvalidDefinition, Message: err.Error()},
			ctrl.Result{}, nil
	}
	accepted = definitionAcceptedCondition(nil)
	r.hold(ctx, barrierAfterValidate, def)

	cfg := bindingAuthority{Namespace: r.OperatorNamespace, ManagerAccount: r.ManagerServiceAccount, Builtins: r.Builtins}
	binding, authErr := resolveBindingAuthority(ctx, r.Client, cfg, def)
	r.hold(ctx, barrierAfterAuthority, def)

	if authErr != nil {
		var failure *authorityFailure
		if !errors.As(authErr, &failure) {
			return accepted, conditionSpec{Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonBindingMismatch, Message: authErr.Error()}, ctrl.Result{}, authErr
		}
		ready = conditionSpec{Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: failure.Reason, Message: failure.Message}
		if failure.Cause != nil {
			// A control-plane read failed; nothing about the stored state is
			// known to have changed. The published snapshot is left alone —
			// dropping it would turn an API blip into a self-inflicted outage,
			// and no run can publish evidence without the uncached authority
			// fence succeeding anyway — and the read is retried with backoff.
			return accepted, ready, ctrl.Result{}, failure
		}
		// No authority, no compilation and no publication — including for a
		// contested identity. "Activate only after valid binding and
		// compilation" is the matrix row; compiling an unauthorized definition
		// would spend the bounded compile budget on a revision that cannot be
		// activated, and publishing it would put a snapshot behind a barrier
		// nobody authorized.
		r.retireIdentity(log, def.Name, "")
		return accepted, ready, ctrl.Result{RequeueAfter: definitions.MissingInputPoll}, nil
	}

	scope := binding.Spec.TargetScope
	// The binding allowlist must actually cover what the definition reads.
	// A scope that cannot satisfy the definition is a mismatch, never a
	// partial run over the subset that happens to be authorized.
	if err := definitions.ValidateScope(def, scope); err != nil {
		r.retireIdentity(log, def.Name, "")
		return accepted, conditionSpec{
			Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonBindingMismatch,
			Message: fmt.Sprintf("binding scope does not authorize the definition's targets: %v", err),
		}, ctrl.Result{}, nil
	}

	// A contested identity is decided before anything is compiled, and the two
	// sources of the claim are enforced differently because they can enforce
	// different things.
	//
	// The registry bars dispatch for built-ins this process actually
	// registered: publishing there is what makes it suppress *both* candidates,
	// so that path still publishes and reports the registry's own barrier.
	//
	// The shipped inventory answers for the release as a whole, and the
	// registry raises nothing for a built-in it does not hold. Publishing an
	// inventory-claimed identity would therefore leave it dispatchable while
	// the status claimed a suppression nobody was enforcing. It fails closed
	// instead: nothing is compiled, nothing is published, and every snapshot
	// claiming the identity is retired, so a BuiltinCollision condition always
	// means the identity does not dispatch.
	// A nil inventory asks identityClaimedByBuiltin the registry-side half of
	// the question only; the release inventory is the separate check below.
	registryClaims := identityClaimedByBuiltin(r.Registry, nil, def.Name)
	if !registryClaims && identityShippedAsBuiltin(cfg.builtins(), def.Name) {
		r.withdrawContested(log, def)
		return accepted, builtinCollisionCondition(def.Name), ctrl.Result{}, nil
	}

	compile := r.Compile
	if compile == nil {
		compile = compileRuntimeDefinition
	}
	compiled, err := compile(ctx, def, scope)
	if err != nil {
		// Compilation is the last half of semantic validation: a definition
		// that cannot be compiled within its bounded budget is not eligible,
		// and no partial snapshot is published for it. Unrelated definitions
		// are untouched, because nothing here is shared between owners.
		r.retireIdentity(log, def.Name, "")
		return definitionAcceptedCondition(err),
			conditionSpec{Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonInvalidDefinition, Message: err.Error()},
			ctrl.Result{}, nil
	}

	// Retire dead claimants of this identity before publishing. Only one object
	// can hold a cluster-scoped name, so any other owner of it is provably a
	// definition that no longer exists — evidence from the live API, not a
	// winner picked by arrival order.
	r.retireIdentity(log, def.Name, def.UID)
	r.hold(ctx, barrierBeforePublish, def)

	entry := registry.RuntimeEntry{
		Adapter: compiled,
		Revision: registry.RuntimeRevision{
			DefinitionUID:    def.UID,
			Generation:       def.Generation,
			SchemaVersion:    r.schemaVersion(),
			SemanticsVersion: def.Spec.SemanticsVersion,
		},
		Provenance: registry.RuntimeProvenance{
			OperatorBuild:  r.OperatorBuild,
			AdapterVersion: def.Spec.AdapterVersion,
		},
	}
	publishErr := r.Registry.SetRuntime(entry)
	r.hold(ctx, barrierAfterPublish, def)

	var barrier *registry.DispatchBarrier
	switch {
	case publishErr == nil && registryClaims:
		// The registry said a built-in claims this identity yet raised no
		// barrier for the publication — the two cannot both be true, and the
		// only way to reach here is a compilation that published under some
		// other identity than the definition's own. Withdraw it: a contested
		// definition must leave nothing dispatchable behind, whatever it
		// advertised.
		r.withdrawContested(log, def)
		return accepted, builtinCollisionCondition(def.Name), ctrl.Result{}, nil
	case publishErr == nil:
		return accepted, conditionSpec{
			Type: definitionConditionReady, Status: metav1.ConditionTrue, Reason: reasonRuntimePublished,
			Message: fmt.Sprintf("runtime snapshot %s published", revisionString(entry.Revision)),
		}, ctrl.Result{}, nil
	case errors.As(publishErr, &barrier):
		// Published but suppressed. Dropping the snapshot instead would let the
		// surviving claimant — a newly shipped built-in, typically — silently
		// reinterpret every AddonCheck of this identity, which is precisely
		// what the barrier exists to prevent.
		return accepted, conditionSpec{
			Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: barrier.Reason, Message: barrier.Error(),
		}, ctrl.Result{}, nil
	default:
		r.retireIdentity(log, def.Name, "")
		return accepted, conditionSpec{
			Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonDefinitionUnavailable, Message: publishErr.Error(),
		}, ctrl.Result{}, publishErr
	}
}

// identityShippedAsBuiltin reports whether this release's collision inventory
// names addonType as a built-in's, independently of what this process actually
// registered. It is deliberately separate from the registry's answer: the
// registry can only bar dispatch for built-ins it holds, so an inventory-only
// claim has to be enforced by refusing to publish at all.
func identityShippedAsBuiltin(builtins []string, addonType string) bool {
	for _, builtin := range builtins {
		if builtin == addonType {
			return true
		}
	}
	return false
}

// builtinCollisionCondition is the matrix's BuiltinCollision row, published
// only where the identity has been made undispatchable first.
func builtinCollisionCondition(addonType string) conditionSpec {
	return conditionSpec{
		Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonBuiltinCollision,
		Message: fmt.Sprintf(
			"addon type %q is claimed by a built-in adapter in this release; no runtime snapshot is published for it, migrate or delete the definition",
			addonType),
	}
}

// writeStatus mirrors the decision into status, writing only on a real change.
func (r *AddonDefinitionReconciler) writeStatus(ctx context.Context, def *fathomv1alpha1.AddonDefinition, accepted, ready conditionSpec) error {
	status := def.Status.DeepCopy()
	status.ObservedGeneration = def.Generation
	status.Conditions = upsertDefinitionCondition(status.Conditions, accepted.condition(def.Generation))
	status.Conditions = upsertDefinitionCondition(status.Conditions, ready.condition(def.Generation))
	if ready.Status == metav1.ConditionTrue {
		// Only a published snapshot updates the attributed revision; a failed
		// reconcile leaves the last published one readable.
		status.Revision = revisionString(registry.RuntimeRevision{
			DefinitionUID:    def.UID,
			Generation:       def.Generation,
			SchemaVersion:    r.schemaVersion(),
			SemanticsVersion: def.Spec.SemanticsVersion,
		})
	}
	if equality.Semantic.DeepEqual(def.Status, *status) {
		return nil
	}
	def.Status = *status
	return r.Status().Update(ctx, def)
}

func (r *AddonDefinitionReconciler) schemaVersion() string {
	if r.SchemaVersion != "" {
		return r.SchemaVersion
	}
	return fathomv1alpha1.GroupVersion.Version
}

// revisionString renders the attributable revision tuple for status.
func revisionString(rev registry.RuntimeRevision) string {
	return fmt.Sprintf("%s/%d/%s/%d", rev.DefinitionUID, rev.Generation, rev.SchemaVersion, rev.SemanticsVersion)
}

// retireIdentity removes every published snapshot claiming addonType except the
// one owned by keep. Removal is always by UID, so a delete event that arrives
// after a recreated definition published a new UID cannot take the replacement
// down with it.
func (r *AddonDefinitionReconciler) retireIdentity(log logr.Logger, addonType string, keep types.UID) {
	for _, entry := range r.Registry.RuntimeEntries() {
		uid := entry.Revision.DefinitionUID
		if uid == keep {
			continue
		}
		for _, claimed := range entry.AddonTypes {
			if claimed != addonType {
				continue
			}
			if r.Registry.RemoveRuntime(uid) {
				log.Info("retired runtime snapshot", "addonType", addonType, "definitionUID", uid, "generation", entry.Revision.Generation)
			}
			break
		}
	}
}

// withdrawContested makes a contested identity undispatchable: this
// definition's own snapshot goes, whatever identity it was published under, and
// so does every other snapshot claiming the name. It is what lets a
// BuiltinCollision condition mean something — the status never reports a
// suppression the registry is not providing.
func (r *AddonDefinitionReconciler) withdrawContested(log logr.Logger, def *fathomv1alpha1.AddonDefinition) {
	if r.Registry.RemoveRuntime(def.UID) {
		log.Info("withdrew the runtime snapshot of a contested identity", "addonType", def.Name, "definitionUID", def.UID)
	}
	r.retireIdentity(log, def.Name, "")
}

// hold runs the barrier seam if a test installed one.
func (r *AddonDefinitionReconciler) hold(ctx context.Context, phase string, def *fathomv1alpha1.AddonDefinition) {
	if r.barrier != nil {
		r.barrier(ctx, phase, def)
	}
}

// definitionsForBinding maps a binding event to the single cluster-scoped
// definition it references. Bindings outside the operator namespace carry no
// authority and enqueue nothing.
func (r *AddonDefinitionReconciler) definitionsForBinding(_ context.Context, obj client.Object) []reconcile.Request {
	binding, ok := obj.(*fathomv1alpha1.AddonDefinitionBinding)
	if !ok || binding.Namespace != r.OperatorNamespace || binding.Spec.DefinitionRef.Name == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: string(binding.Spec.DefinitionRef.Name)}}}
}

// SetupWithManager registers the definition controller and the shared indexes.
// Wiring it into the manager is T047's; nothing calls this yet.
func (r *AddonDefinitionReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if err := RegisterAddonDefinitionIndexes(ctx, mgr.GetFieldIndexer()); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.AddonDefinition{}).
		Watches(&fathomv1alpha1.AddonDefinitionBinding{}, handler.EnqueueRequestsFromMapFunc(r.definitionsForBinding)).
		Named("addondefinition").
		Complete(r)
}
