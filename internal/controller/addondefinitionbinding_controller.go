/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/registry"
	"github.com/skaphos/fathom/pkg/adapter"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
)

// Condition types and reasons for the runtime definition/binding pair.
//
// The reasons are spelled exactly as the lifecycle matrix in
// specs/012-addon-definition-runtime/contracts/runtime.md spells them: they are
// the operator-visible contract, and an administrator triaging a stuck
// definition matches them against that table verbatim.
const (
	definitionConditionAccepted = "Accepted"
	definitionConditionReady    = "Ready"
	// definitionConditionDrained is the leader's acknowledgement that a
	// disabled binding has no runtime work left in this leadership session.
	// `fathomctl definition drain` reads this exact spelling.
	definitionConditionDrained = "Drained"

	// The matrix's remaining reason, UnknownAddonType, belongs to the AddonCheck
	// runtime path (T039/T040): it describes an addon identity no definition and
	// no built-in claims, which a definition reconciler — running only for
	// stored definitions — can never observe. It is deliberately not declared
	// here, because an unused constant is a claim this file does not honour.
	//
	// reasonInvalidDefinition marks a stored spec that fails semantic
	// validation or compilation. Eligibility is removed; evidence is retained.
	reasonInvalidDefinition = "InvalidDefinition"
	// reasonDefinitionUnavailable marks a binding whose definition is gone.
	reasonDefinitionUnavailable = "DefinitionUnavailable"
	// reasonBindingMismatch marks authority that does not match the live
	// objects: a recreated definition or service account, an ambiguous or
	// shared identity, or a reader that is not dedicated.
	reasonBindingMismatch = "BindingMismatch"
	// reasonAuthorizationRevoked marks a binding that is missing, deleting or
	// disabled. It is the deliberate administrative off switch.
	reasonAuthorizationRevoked = "AuthorizationRevoked"
	// reasonAccessDenied marks a control-plane read the operator's own identity
	// is not permitted to make. It is transient by nature, so it is retried.
	reasonAccessDenied = "AccessDenied"
	// reasonBuiltinCollision and reasonRuntimeCollision are the registry's
	// barrier reasons, re-exported so a condition never drifts from a barrier.
	reasonBuiltinCollision = registry.ReasonBuiltinCollision
	reasonRuntimeCollision = registry.ReasonRuntimeCollision

	// Success reasons. They are not in the matrix (which enumerates failures),
	// so they simply name what happened.
	reasonDefinitionAccepted = "DefinitionAccepted"
	reasonRuntimePublished   = "RuntimeSnapshotPublished"
	reasonBindingAuthorized  = "BindingAuthorized"

	// Drain reasons. contracts/leadership.md names only the outcome ("Drained=True"
	// and "unverifiable, not drained"), so these name the three states a leader
	// can be in about a binding.
	//
	// reasonDrainAcknowledged is the only reason that accompanies Drained=True.
	reasonDrainAcknowledged = "DrainAcknowledged"
	// reasonDrainUnverifiable covers every failure to prove a drain: a failed
	// direct read, an epoch that no longer matches, work still unwinding, or a
	// session that may not acknowledge yet. Unverifiable is not drained.
	reasonDrainUnverifiable = "DrainUnverifiable"
	// reasonBindingEnabled means the binding is not disabled, so there is
	// nothing to acknowledge. Re-enabling removes acknowledgement eligibility.
	reasonBindingEnabled = "BindingEnabled"
)

// Field indexes backing dependency requeues. Without them a binding event would
// have to list every binding in the cluster to find the one definition it
// belongs to, and the service-account uniqueness check would list every binding
// on every reconcile. Every List in this file therefore carries one of these
// selectors plus the operator namespace.
const (
	IndexBindingDefinitionName     = "spec.definitionRef.name"
	IndexBindingServiceAccountName = "spec.serviceAccountRef.name"
	IndexBindingServiceAccountUID  = "spec.serviceAccountRef.uid"
)

// errCacheSyncUnwired is returned when a reconciler was constructed without a
// cache-synchronization gate. The zero value must not be usable: the lifecycle
// matrix forbids runtime execution "until synchronized and directly validated",
// and a gate that defaults to "synchronized" would satisfy that requirement
// with a no-op nobody could see.
var errCacheSyncUnwired = errors.New(
	"controller: runtime definition reconcilers require an explicit cache-synchronization gate")

// errDrainReaderUnwired is returned when a leadership session was wired without
// the manager's uncached APIReader. A drain acknowledgement is worth exactly the
// reads that back it, and contracts/leadership.md requires those reads to be
// direct, so falling back to the informer cache is not an option: the
// reconciler refuses to claim anything instead.
var errDrainReaderUnwired = errors.New(
	"controller: drain acknowledgement requires the manager's uncached APIReader")

// DrainEvidence is what an elected leadership session hands the reconciler when
// it is willing to have a drain published: the epoch it observed live, and the
// caveat that epoch carries.
//
// It mirrors internal/app.DrainAcknowledgement field for field. It is redeclared
// here because internal/app imports internal/controller, so the dependency
// cannot run the other way (verified with `go list -deps ./internal/app`).
type DrainEvidence struct {
	// Epoch is the leadership epoch the acknowledgement describes.
	Epoch fathomv1alpha1.DefinitionLeaderEpoch
	// Limitation is the observation caveat published with the acknowledgement:
	// a suspended prior holder is not fenced, so this is an observation rather
	// than proof of exclusive execution.
	Limitation string
}

// RuntimeLeadershipSession is the narrow view of the elected leadership session
// this reconciler needs. The session is the single decider about admission and
// drain; the reconciler only supplies the binding-side fences the session
// deliberately does not look at (enabled=false, the generation, and the uncached
// re-reads).
//
// internal/app.RuntimeLeadership implements every method with these exact
// signatures except AcknowledgeDrain, whose result type lives in that package.
// T047 therefore wires it through a shadowing adapter:
//
//	type drainSession struct{ *app.RuntimeLeadership }
//	func (s drainSession) AcknowledgeDrain(key string) (controller.DrainEvidence, error) {
//	    ack, err := s.RuntimeLeadership.AcknowledgeDrain(key)
//	    return controller.DrainEvidence{Epoch: ack.Epoch, Limitation: ack.Limitation}, err
//	}
type RuntimeLeadershipSession interface {
	// LeaseRef is the configured namespaced Lease of this session. It is
	// trusted operator configuration, never inferred from a binding's status.
	LeaseRef() types.NamespacedName
	// EpochValid reports whether an epoch belongs to this session's observed
	// leadership. Equality uses all four epoch fields; resourceVersion is
	// excluded because ordinary renewal changes it.
	EpochValid(*fathomv1alpha1.DefinitionLeaderEpoch) bool
	// Revoke closes admission for the binding and cancels its in-flight work.
	Revoke(key string)
	// Restore reopens admission after explicit reauthorization. It grants
	// nothing on its own.
	Restore(key string)
	// ActiveRuns counts this session's unreleased work for the binding.
	ActiveRuns(key string) int
	// AcknowledgeDrain returns the evidence a drain may be published with, or
	// the reason this session may not acknowledge one.
	AcknowledgeDrain(key string) (DrainEvidence, error)
}

func indexBindingByDefinitionName(obj client.Object) []string {
	b, ok := obj.(*fathomv1alpha1.AddonDefinitionBinding)
	if !ok || b.Spec.DefinitionRef.Name == "" {
		return nil
	}
	return []string{string(b.Spec.DefinitionRef.Name)}
}

func indexBindingByServiceAccountName(obj client.Object) []string {
	b, ok := obj.(*fathomv1alpha1.AddonDefinitionBinding)
	if !ok || b.Spec.ServiceAccountRef.Name == "" {
		return nil
	}
	return []string{string(b.Spec.ServiceAccountRef.Name)}
}

func indexBindingByServiceAccountUID(obj client.Object) []string {
	b, ok := obj.(*fathomv1alpha1.AddonDefinitionBinding)
	if !ok || b.Spec.ServiceAccountRef.UID == "" {
		return nil
	}
	return []string{b.Spec.ServiceAccountRef.UID}
}

// RegisterAddonDefinitionIndexes registers the binding indexes both runtime
// reconcilers depend on. It is safe to call once per manager from either
// SetupWithManager; a repeated registration of the same field is reported by
// the informer cache as a conflict and ignored here, because the index it
// conflicts with is this same one.
func RegisterAddonDefinitionIndexes(ctx context.Context, indexer client.FieldIndexer) error {
	for _, idx := range []struct {
		field   string
		extract client.IndexerFunc
	}{
		{IndexBindingDefinitionName, indexBindingByDefinitionName},
		{IndexBindingServiceAccountName, indexBindingByServiceAccountName},
		{IndexBindingServiceAccountUID, indexBindingByServiceAccountUID},
	} {
		err := indexer.IndexField(ctx, &fathomv1alpha1.AddonDefinitionBinding{}, idx.field, idx.extract)
		if err != nil && !strings.Contains(err.Error(), "indexer conflict") {
			return fmt.Errorf("controller: index AddonDefinitionBinding by %s: %w", idx.field, err)
		}
	}
	return nil
}

// authorityFailure is one lifecycle-matrix outcome: the reason an administrator
// reads, the detail that makes it actionable, and — when the failure is a
// transient control-plane error rather than a stored-state problem — the cause
// that must be retried with backoff.
type authorityFailure struct {
	Reason  string
	Message string
	Cause   error
}

func (f *authorityFailure) Error() string {
	if f.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", f.Reason, f.Message, f.Cause)
	}
	return fmt.Sprintf("%s: %s", f.Reason, f.Message)
}

func (f *authorityFailure) Unwrap() error { return f.Cause }

func revoked(format string, args ...any) *authorityFailure {
	return &authorityFailure{Reason: reasonAuthorizationRevoked, Message: fmt.Sprintf(format, args...)}
}

func mismatched(format string, args ...any) *authorityFailure {
	return &authorityFailure{Reason: reasonBindingMismatch, Message: fmt.Sprintf(format, args...)}
}

// readFailure classifies a control-plane read error. A denial is its own
// lifecycle row — "RBAC denies an API read ... bounded retries use actual
// current permissions" — so it keeps its own reason while still being retried.
func readFailure(err error, what string) *authorityFailure {
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return &authorityFailure{Reason: reasonAccessDenied, Message: "cannot read " + what, Cause: err}
	}
	return &authorityFailure{Reason: reasonDefinitionUnavailable, Message: "cannot read " + what, Cause: err}
}

// bindingAuthority is the configuration every authority decision needs. It is
// deliberately explicit: an empty operator namespace or manager identity fails
// closed rather than being guessed from the object under reconciliation.
type bindingAuthority struct {
	Namespace      string
	ManagerAccount string
	Builtins       func() []string
}

func (a bindingAuthority) builtins() []string {
	if a.Builtins != nil {
		return a.Builtins()
	}
	return definitions.BuiltinNames()
}

// resolveBindingAuthority returns the binding that authorizes def, or an
// *authorityFailure carrying the lifecycle-matrix reason.
//
// The order and the substance of the checks mirror
// impersonation.ResolveRuntimeAuthority, which fences the same conditions
// against an uncached reader immediately before a run: this is the eligibility
// decision, that one is the execution fence, and the two must never disagree
// about what "dedicated identity" means.
func resolveBindingAuthority(ctx context.Context, reader client.Reader, cfg bindingAuthority, def *fathomv1alpha1.AddonDefinition) (*fathomv1alpha1.AddonDefinitionBinding, error) {
	if cfg.Namespace == "" || cfg.ManagerAccount == "" {
		return nil, mismatched("runtime authority requires an explicit operator namespace and manager identity")
	}
	var bindings fathomv1alpha1.AddonDefinitionBindingList
	// Limit is safe with an exact-match index: a page that is full already
	// contains more than the one binding this identity may have, which is a
	// failure either way, so truncation cannot hide a violation.
	if err := reader.List(ctx, &bindings,
		client.InNamespace(cfg.Namespace),
		client.MatchingFields{IndexBindingDefinitionName: def.Name},
		client.Limit(definitions.MaxPageObjects),
	); err != nil {
		return nil, readFailure(err, "bindings for definition "+def.Name)
	}
	switch {
	case len(bindings.Items) == 0:
		return nil, revoked("no binding in %s authorizes definition %q", cfg.Namespace, def.Name)
	case len(bindings.Items) > 1:
		return nil, mismatched("%d bindings claim definition %q; authority must be unambiguous", len(bindings.Items), def.Name)
	}
	binding := bindings.Items[0].DeepCopy()

	if err := definitions.ValidateBinding(binding); err != nil {
		return nil, mismatched("binding %q is invalid: %v", binding.Name, err)
	}
	if !binding.DeletionTimestamp.IsZero() {
		return nil, revoked("binding %q is being deleted", binding.Name)
	}
	if def.UID == "" || binding.UID == "" {
		return nil, mismatched("definition and binding must both have a stored UID")
	}
	if binding.Spec.DefinitionRef.UID != string(def.UID) ||
		binding.Spec.DefinitionRef.Name != fathomv1alpha1.DefinitionDNSLabel(def.Name) {
		return nil, mismatched("binding %q authorizes definition UID %q, but the live definition is %q",
			binding.Name, binding.Spec.DefinitionRef.UID, def.UID)
	}
	if !binding.Spec.Enabled {
		return nil, revoked("binding %q is disabled", binding.Name)
	}

	saName := string(binding.Spec.ServiceAccountRef.Name)
	if saName == cfg.ManagerAccount {
		return nil, mismatched("the manager service account %q is not a dedicated reader", saName)
	}
	for _, builtin := range cfg.builtins() {
		base := adapter.AddonServiceAccountName(builtin)
		if saName == base || saName == "fathom-"+base {
			return nil, mismatched("built-in service account %q is not a dedicated reader", saName)
		}
	}
	var sa corev1.ServiceAccount
	if err := reader.Get(ctx, types.NamespacedName{Namespace: cfg.Namespace, Name: saName}, &sa); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, mismatched("service account %s/%s does not exist", cfg.Namespace, saName)
		}
		return nil, readFailure(err, "service account "+cfg.Namespace+"/"+saName)
	}
	switch {
	case sa.UID == "" || string(sa.UID) != binding.Spec.ServiceAccountRef.UID:
		return nil, mismatched("service account %q has UID %q, but the binding authorizes %q",
			saName, sa.UID, binding.Spec.ServiceAccountRef.UID)
	case !sa.DeletionTimestamp.IsZero():
		return nil, mismatched("service account %q is being deleted", saName)
	case sa.Labels[adapter.AddonLabel] != "":
		return nil, mismatched("service account %q is reserved for built-in adapter %q", saName, sa.Labels[adapter.AddonLabel])
	}

	// Dedicated means dedicated: no other binding may name or reference this
	// identity, by name or by UID. Both indexes are consulted because a stale
	// name and a stale UID are independently able to share an identity.
	for _, lookup := range []struct{ field, value string }{
		{IndexBindingServiceAccountUID, string(sa.UID)},
		{IndexBindingServiceAccountName, saName},
	} {
		var sharing fathomv1alpha1.AddonDefinitionBindingList
		if err := reader.List(ctx, &sharing,
			client.InNamespace(cfg.Namespace),
			client.MatchingFields{lookup.field: lookup.value},
			client.Limit(definitions.MaxPageObjects),
		); err != nil {
			return nil, readFailure(err, "bindings sharing service account "+saName)
		}
		for i := range sharing.Items {
			other := &sharing.Items[i]
			if other.UID == binding.UID && other.Name == binding.Name {
				continue
			}
			return nil, mismatched("service account %q is already bound by %q; each binding requires its own reader",
				saName, other.Name)
		}
	}
	return binding, nil
}

// identityClaimedByBuiltin reports whether a built-in adapter claims addonType.
//
// Both sources matter. The registry answers for adapters this process actually
// registered; the shipped inventory answers for the release as a whole, which
// is what `fathomctl definition collisions` compares against, so an upgrade
// that adds a built-in is reported the same way by the CLI and by the operator.
func identityClaimedByBuiltin(reg *registry.Registry, builtins []string, addonType string) bool {
	for _, builtin := range builtins {
		if builtin == addonType {
			return true
		}
	}
	if reg == nil {
		return false
	}
	res, err := reg.Resolve(addonType)
	var barrier *registry.DispatchBarrier
	switch {
	case err == nil:
		return !res.Runtime
	case errors.As(err, &barrier):
		return barrier.Reason == registry.ReasonBuiltinCollision
	default:
		return false
	}
}

// upsertDefinitionCondition applies next, preserving the transition time of an
// existing entry whose status has not changed. Keeping that timestamp stable is
// what makes a repeated reconcile a no-op instead of a status write.
func upsertDefinitionCondition(conds []fathomv1alpha1.DefinitionStatusCondition, next fathomv1alpha1.DefinitionStatusCondition) []fathomv1alpha1.DefinitionStatusCondition {
	if next.LastTransitionTime.IsZero() {
		next.LastTransitionTime = metav1.Now()
	}
	for i, existing := range conds {
		if existing.Type != next.Type {
			continue
		}
		if existing.Status == next.Status {
			next.LastTransitionTime = existing.LastTransitionTime
		}
		conds[i] = next
		return conds
	}
	if len(conds) >= definitions.MaxConditions {
		return conds
	}
	conds = append(conds, next)
	sort.Slice(conds, func(i, j int) bool { return conds[i].Type < conds[j].Type })
	return conds
}

// conditionSpec is the intent of one condition before it is stamped with the
// observed generation and a transition time.
type conditionSpec struct {
	Type    string
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

func (c conditionSpec) condition(generation int64) fathomv1alpha1.DefinitionStatusCondition {
	message := c.Message
	if len(message) > definitions.MaxStringBytes {
		message = message[:definitions.MaxStringBytes]
	}
	reason := c.Reason
	if len(reason) > definitions.MaxConditionReasonBytes {
		reason = reason[:definitions.MaxConditionReasonBytes]
	}
	return fathomv1alpha1.DefinitionStatusCondition{
		Type:               c.Type,
		Status:             c.Status,
		ObservedGeneration: generation,
		Reason:             reason,
		Message:            message,
	}
}

func definitionAcceptedCondition(err error) conditionSpec {
	if err == nil {
		return conditionSpec{Type: definitionConditionAccepted, Status: metav1.ConditionTrue, Reason: reasonDefinitionAccepted, Message: "stored spec is valid"}
	}
	return conditionSpec{Type: definitionConditionAccepted, Status: metav1.ConditionFalse, Reason: reasonInvalidDefinition, Message: err.Error()}
}

// AddonDefinitionBindingReconciler reports whether a binding currently
// authorizes its definition. It writes only its own status: authority comes
// from the two specs and the live ServiceAccount, never from any status, so a
// status write can neither grant nor extend permission.
//
// It also publishes the drain acknowledgement of contracts/leadership.md, but
// only while this process holds the elected session: an acknowledgement is a
// claim about one leadership epoch's work, and no other process can make it.
type AddonDefinitionBindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// APIReader is the manager's uncached reader. Drain acknowledgement reads
	// the binding and the leader Lease through it, never through the informer
	// cache. Required whenever Leadership is set.
	APIReader client.Reader

	// Leadership is the elected session, or nil when this process holds none.
	// Nil is the default-off case: the reconciler reports authority and claims
	// nothing about draining.
	Leadership RuntimeLeadershipSession

	// Registry is consulted only to detect built-in collisions. It is optional:
	// a nil registry falls back to the shipped inventory.
	Registry *registry.Registry

	// OperatorNamespace is the only namespace whose bindings carry authority.
	OperatorNamespace string
	// ManagerServiceAccount is the operator's own identity, which can never be
	// a dedicated reader.
	ManagerServiceAccount string

	// CacheSynced reports whether this manager's informers are synchronized.
	// There is no default: see errCacheSyncUnwired.
	CacheSynced func(context.Context) bool

	// Builtins overrides the shipped collision inventory in tests.
	Builtins func() []string
}

// The definition and binding kinds are read and watched, and their status
// subresources are written. No binding-spec write, no grant write, no
// SubjectAccessReview and no cluster-wide Lease read appears anywhere in this
// feature: authority is administrator-granted out of band and only observed
// here. The ServiceAccount read the authority check makes needs no new grant —
// the operator already reads service accounts for built-in impersonation.
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addondefinitionbindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addondefinitionbindings/status,verbs=get;update;patch

// Reconcile is bounded: at most one definition read, one service-account read
// and three indexed binding lists, each with an exact-match selector.
func (r *AddonDefinitionBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.OperatorNamespace == "" || r.ManagerServiceAccount == "" {
		return ctrl.Result{}, errors.New("controller: AddonDefinitionBindingReconciler requires an explicit operator namespace and manager service account")
	}
	if r.CacheSynced == nil {
		return ctrl.Result{}, errCacheSyncUnwired
	}
	if !r.CacheSynced(ctx) {
		return ctrl.Result{RequeueAfter: runtimeSyncRequeue}, nil
	}
	// A binding outside the operator namespace carries no authority at all, so
	// it is not reconciled: reporting on it would suggest otherwise.
	if req.Namespace != r.OperatorNamespace {
		return ctrl.Result{}, nil
	}

	var binding fathomv1alpha1.AddonDefinitionBinding
	if err := r.Get(ctx, req.NamespacedName, &binding); err != nil {
		if apierrors.IsNotFound(err) {
			// "Binding disabled/deleted or SA replaced | ... | Cancel active
			// work". Withdrawing the runtime snapshot is the definition
			// reconciler's half; this half cancels the work already running
			// under the deleted binding's dedicated identity, which would
			// otherwise run to completion under authority nobody holds. There
			// is no object left to report on, so nothing is published.
			r.cancelRuntimeWork(req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if !binding.DeletionTimestamp.IsZero() {
		// A finalizer-held delete is still a delete: authority ends when the
		// deletion timestamp is set, not when the object finally disappears.
		// Cancel this session's work and write nothing — a status published on
		// a dying object vanishes with it, and evaluating one would spend the
		// reconcile's read budget deciding about an object that is leaving.
		r.cancelRuntimeWork(binding.Name)
		return ctrl.Result{}, nil
	}

	accepted, ready, result, retry := r.evaluate(ctx, &binding)
	status := binding.Status.DeepCopy()
	status.ObservedGeneration = binding.Generation

	var drained conditionSpec
	if r.Leadership != nil {
		var drainResult ctrl.Result
		var drainErr error
		// acknowledgeDrain may replace the Ready condition: a denied direct
		// read on the drain path is the matrix's "RBAC denies an API read"
		// row, and it is only visible if it reaches the published condition.
		drained, ready, drainResult, drainErr = r.acknowledgeDrain(ctx, &binding, ready, status)
		if drained.Status != metav1.ConditionTrue {
			// The epoch is the acknowledgement's evidence and exists only while
			// the acknowledgement does. Clearing it here — rather than trusting
			// what a previous write left behind — is what makes a persisted
			// acknowledgement worthless to a session that cannot re-prove it.
			status.LeaderEpoch, status.LeaderIdentity = nil, ""
		}
		result = soonest(result, drainResult)
		if retry == nil {
			retry = drainErr
		}
	}

	status.Conditions = upsertDefinitionCondition(status.Conditions, accepted.condition(binding.Generation))
	status.Conditions = upsertDefinitionCondition(status.Conditions, ready.condition(binding.Generation))
	if r.Leadership != nil {
		status.Conditions = upsertDefinitionCondition(status.Conditions, drained.condition(binding.Generation))
	}

	if !equality.Semantic.DeepEqual(binding.Status, *status) {
		binding.Status = *status
		if err := r.Status().Update(ctx, &binding); err != nil {
			// A binding deleted under the reconcile needs no status; deletion
			// is itself the revocation.
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}
	return result, retry
}

// cancelRuntimeWork closes admission for a binding that no longer authorizes
// anything and cancels whatever this leadership session is still running under
// it. It is a no-op in the default-off case, where no session exists and no
// runtime work can be in flight.
func (r *AddonDefinitionBindingReconciler) cancelRuntimeWork(key string) {
	if r.Leadership != nil {
		r.Leadership.Revoke(key)
	}
}

// soonest keeps the tighter of two requeues; a zero RequeueAfter means "no
// requeue of my own", never "immediately".
func soonest(a, b ctrl.Result) ctrl.Result {
	switch {
	case a.RequeueAfter == 0:
		return b
	case b.RequeueAfter == 0:
		return a
	case b.RequeueAfter < a.RequeueAfter:
		return b
	default:
		return a
	}
}

// evaluate resolves the binding's acceptance and eligibility without writing.
func (r *AddonDefinitionBindingReconciler) evaluate(ctx context.Context, binding *fathomv1alpha1.AddonDefinitionBinding) (accepted, ready conditionSpec, result ctrl.Result, retry error) {
	if err := definitions.ValidateBinding(binding); err != nil {
		return definitionAcceptedCondition(err),
			conditionSpec{Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonInvalidDefinition, Message: err.Error()},
			ctrl.Result{}, nil
	}
	accepted = definitionAcceptedCondition(nil)

	if !binding.Spec.Enabled {
		// The administrative off switch dominates every other authority
		// question: a disabled binding is revoked whether or not its definition
		// and service account still exist, and AuthorizationRevoked is exactly
		// the reason a drain acknowledgement must carry.
		return accepted, conditionSpec{
			Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonAuthorizationRevoked,
			Message: fmt.Sprintf("binding %q is disabled", binding.Name),
		}, ctrl.Result{}, nil
	}

	name := string(binding.Spec.DefinitionRef.Name)
	var def fathomv1alpha1.AddonDefinition
	if err := r.Get(ctx, types.NamespacedName{Name: name}, &def); err != nil {
		if apierrors.IsNotFound(err) {
			return accepted, conditionSpec{
				Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonDefinitionUnavailable,
				Message: fmt.Sprintf("definition %q does not exist", name),
			}, ctrl.Result{RequeueAfter: definitions.MissingInputPoll}, nil
		}
		failure := readFailure(err, "definition "+name)
		return accepted, conditionSpec{
			Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: failure.Reason, Message: failure.Message,
		}, ctrl.Result{}, failure
	}
	if err := definitions.Validate(&def); err != nil {
		return accepted, conditionSpec{
			Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonInvalidDefinition, Message: err.Error(),
		}, ctrl.Result{}, nil
	}

	cfg := bindingAuthority{Namespace: r.OperatorNamespace, ManagerAccount: r.ManagerServiceAccount, Builtins: r.Builtins}
	if _, err := resolveBindingAuthority(ctx, r.Client, cfg, &def); err != nil {
		var failure *authorityFailure
		if !errors.As(err, &failure) {
			return accepted, conditionSpec{Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonBindingMismatch, Message: err.Error()}, ctrl.Result{}, err
		}
		ready = conditionSpec{Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: failure.Reason, Message: failure.Message}
		if failure.Cause != nil {
			return accepted, ready, ctrl.Result{}, failure
		}
		return accepted, ready, ctrl.Result{RequeueAfter: definitions.MissingInputPoll}, nil
	}

	if identityClaimedByBuiltin(r.Registry, cfg.builtins(), name) {
		return accepted, conditionSpec{
			Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonBuiltinCollision,
			Message: fmt.Sprintf("addon type %q is also claimed by a built-in adapter; migrate or delete the definition", name),
		}, ctrl.Result{}, nil
	}

	return accepted, conditionSpec{
		Type: definitionConditionReady, Status: metav1.ConditionTrue, Reason: reasonBindingAuthorized,
		Message: fmt.Sprintf("definition %q is authorized to run as %s/%s", name, r.OperatorNamespace, binding.Spec.ServiceAccountRef.Name),
	}, ctrl.Result{}, nil
}

// acknowledgeDrain implements the drain acknowledgement of
// contracts/leadership.md for one binding:
//
//	"For disabled bindings, acknowledge only after observing enabled=false
//	 directly, cancelling/awaiting this session's work, and observing
//	 activeRuns=0. Re-read the Lease and binding through uncached control-plane
//	 reads before publishing the matching generation/epoch, Drained=True and
//	 Ready=False/AuthorizationRevoked. Treat read failure or epoch mismatch as
//	 unverifiable, not drained."
//
// Nothing in binding.Status is an input. The decision is rebuilt from the live
// objects and this session's own accounting on every reconcile, so a status
// write can neither grant an acknowledgement nor extend one, and a persisted
// acknowledgement from an earlier leadership lifetime is worth nothing.
//
// The "await" is bounded and requeued, never blocking: a Reconcile that slept
// until a run unwound would hold a worker for up to a full run deadline. Revoke
// cancels the work, and the requeue is when this session looks again.
//
// It mutates status's leader/active-run fields but never writes; the caller
// publishes, and clears the epoch whenever the outcome is not Drained=True.
//
// It returns the Ready condition to publish alongside the Drained one. That is
// almost always the one evaluate produced, but a control-plane read the drain
// path itself makes can be denied, and the lifecycle matrix's "RBAC denies an
// API read | Ready=False/AccessDenied" row is only visible if that denial
// reaches the condition an administrator reads.
func (r *AddonDefinitionBindingReconciler) acknowledgeDrain(ctx context.Context, binding *fathomv1alpha1.AddonDefinitionBinding, ready conditionSpec, status *fathomv1alpha1.AddonDefinitionBindingStatus) (conditionSpec, conditionSpec, ctrl.Result, error) {
	key := binding.Name

	if binding.Spec.Enabled {
		// Not disabled: there is nothing to acknowledge, and re-enabling is
		// exactly what removes acknowledgement eligibility. Admission follows
		// the authority decision — reopened only by an authorized binding.
		if ready.Status == metav1.ConditionTrue {
			r.Leadership.Restore(key)
		} else {
			r.Leadership.Revoke(key)
		}
		return drainedCondition(metav1.ConditionFalse, reasonBindingEnabled,
			fmt.Sprintf("binding %q is enabled; no drain is acknowledged", key)), ready, ctrl.Result{}, nil
	}

	// Cancel first. An acknowledgement describes work that was already stopped,
	// so revocation precedes every observation it is built from — including
	// the refusals below, each of which leaves a disabled binding with its
	// runtime work cancelled and no acknowledgement to show for it.
	r.Leadership.Revoke(key)

	// contracts/leadership.md acknowledges a drain by "publishing the matching
	// generation/epoch, Drained=True and Ready=False/AuthorizationRevoked",
	// and `fathomctl definition drain` — the contract's independent verifier —
	// requires that exact pair before it reports a verified drain. A disabled
	// binding whose stored spec is invalid reports Ready=False/InvalidDefinition
	// instead, so a Drained=True beside it would be an acknowledgement the
	// verifier rejects: the operator would claim drained while the CLI exits 1.
	// The operator says what the CLI says.
	if ready.Status != metav1.ConditionFalse || ready.Reason != reasonAuthorizationRevoked {
		return drainUnverifiable(
			"binding %q reports Ready=%s/%s; a drain is acknowledged only beside Ready=False/%s",
			key, ready.Status, ready.Reason, reasonAuthorizationRevoked), ready, ctrl.Result{}, nil
	}

	if r.APIReader == nil {
		return drainUnverifiable("no uncached reader is wired; a drain cannot be verified"), ready, ctrl.Result{}, errDrainReaderUnwired
	}

	live, err := r.directBinding(ctx, key)
	if err != nil {
		return r.unreadableBinding(err, ready, "binding "+key)
	}
	if reason, eligible := acknowledgementEligible(binding, live); !eligible {
		return reason, ready, ctrl.Result{}, nil
	}

	if runs := r.Leadership.ActiveRuns(key); runs > 0 {
		status.ActiveRuns = clampActiveRuns(runs)
		return drainUnverifiable("%d run(s) of this leadership session are still unwinding", runs), ready,
			ctrl.Result{RequeueAfter: definitions.InitialRetryBackoff}, nil
	}

	evidence, err := r.Leadership.AcknowledgeDrain(key)
	if err != nil {
		// The session refused: it is not admissible, has observed no live
		// epoch, or still holds work. None of those is a stored-state problem,
		// so it is retried rather than reported as an error.
		return drainUnverifiable("this leadership session cannot acknowledge a drain: %v", err), ready,
			ctrl.Result{RequeueAfter: definitions.InitialRetryBackoff}, nil
	}

	// The final fence: both objects read directly again, immediately before
	// publication, so an edit or a leadership change that landed while the work
	// unwound cannot be published as if it had been verified.
	final, err := r.directBinding(ctx, key)
	if err != nil {
		return r.unreadableBinding(err, ready, "binding "+key)
	}
	if reason, eligible := acknowledgementEligible(binding, final); !eligible {
		return reason, ready, ctrl.Result{}, nil
	}
	observed, err := r.directLeaderEpoch(ctx)
	if err != nil {
		return drainUnverifiable("cannot verify the leader epoch: %v", err),
			readyAfterDeniedRead(ready, err, "the leader Lease"), ctrl.Result{}, err
	}
	if !r.Leadership.EpochValid(observed) || !sameLeaderEpoch(observed, &evidence.Epoch) {
		return drainUnverifiable("the live leader epoch is no longer this session's; the acknowledgement would describe another leader"), ready,
			ctrl.Result{RequeueAfter: definitions.InitialRetryBackoff}, nil
	}

	status.ActiveRuns = 0
	status.LeaderIdentity = evidence.Epoch.HolderIdentity
	status.LeaderEpoch = evidence.Epoch.DeepCopy()
	return drainedCondition(metav1.ConditionTrue, reasonDrainAcknowledged,
			fmt.Sprintf("runtime admission is revoked and this leadership session has no active runs for %q; %s", key, evidence.Limitation)),
		ready, ctrl.Result{}, nil
}

// readyAfterDeniedRead applies the lifecycle matrix's "RBAC denies an API read
// | Ready=False/AccessDenied" row to the drain path. The drain path's direct
// reads are API reads like any other, and an administrator who cannot tell a
// revoked grant from a missing one on the operator's own identity has no way to
// act on either. Any other read failure leaves the authority reason alone: it
// says nothing about permissions.
func readyAfterDeniedRead(ready conditionSpec, err error, what string) conditionSpec {
	if !apierrors.IsForbidden(err) && !apierrors.IsUnauthorized(err) {
		return ready
	}
	return conditionSpec{
		Type: definitionConditionReady, Status: metav1.ConditionFalse, Reason: reasonAccessDenied,
		Message: fmt.Sprintf("cannot read %s: %v", what, err),
	}
}

// acknowledgementEligible re-checks, against a directly read object, everything
// that makes an acknowledgement meaningful. observed is the revision the
// reconcile is deciding about; live is what the control plane holds now.
func acknowledgementEligible(observed, live *fathomv1alpha1.AddonDefinitionBinding) (conditionSpec, bool) {
	switch {
	case !live.DeletionTimestamp.IsZero():
		return drainUnverifiable("binding %q is being deleted; there is no durable object to acknowledge", live.Name), false
	case live.Spec.Enabled:
		// Re-enabled under the reconcile. Admission stays closed: reopening it
		// belongs to the reconcile that observes the enabled binding and
		// revalidates its authority, not to this one.
		return drainUnverifiable("the control plane reports binding %q enabled; acknowledgement eligibility is removed", live.Name), false
	case live.Generation != observed.Generation:
		return drainUnverifiable("binding %q was edited to generation %d while generation %d was being drained",
			live.Name, live.Generation, observed.Generation), false
	}
	return conditionSpec{}, true
}

// unreadableBinding classifies a failed direct binding read. A missing object
// is settled — deletion revokes authority outright — while any other failure is
// unverifiable and retried with the caller's backoff.
func (r *AddonDefinitionBindingReconciler) unreadableBinding(err error, ready conditionSpec, what string) (conditionSpec, conditionSpec, ctrl.Result, error) {
	if apierrors.IsNotFound(err) {
		return drainUnverifiable("binding no longer exists; deletion revokes authority outright"), ready, ctrl.Result{}, nil
	}
	return drainUnverifiable("cannot read the binding directly: %v", err),
		readyAfterDeniedRead(ready, err, what), ctrl.Result{}, err
}

// directBinding reads the binding through the uncached reader, bounded by the
// contract's per-request limit so a hung control plane cannot hold a worker.
func (r *AddonDefinitionBindingReconciler) directBinding(ctx context.Context, name string) (*fathomv1alpha1.AddonDefinitionBinding, error) {
	ctx, cancel := context.WithTimeout(ctx, definitions.MaxRequestDuration)
	defer cancel()
	var live fathomv1alpha1.AddonDefinitionBinding
	if err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: r.OperatorNamespace, Name: name}, &live); err != nil {
		return nil, err
	}
	return &live, nil
}

// directLeaderEpoch reads the configured leader-election Lease through the
// uncached reader and derives its epoch.
//
// The Lease is the manager's own, in the operator namespace, and this read uses
// the namespaced leader-election Role the operator already holds. A configured
// Lease outside that namespace is a misconfiguration and fails closed rather
// than reaching for a permission this feature refuses to add: no cluster-wide
// Lease read is introduced anywhere.
func (r *AddonDefinitionBindingReconciler) directLeaderEpoch(ctx context.Context) (*fathomv1alpha1.DefinitionLeaderEpoch, error) {
	ref := r.Leadership.LeaseRef()
	if ref.Name == "" || ref.Namespace != r.OperatorNamespace {
		return nil, fmt.Errorf("configured leader Lease %s is not in the operator namespace %q", ref, r.OperatorNamespace)
	}
	ctx, cancel := context.WithTimeout(ctx, definitions.MaxRequestDuration)
	defer cancel()
	var lease coordinationv1.Lease
	if err := r.APIReader.Get(ctx, ref, &lease); err != nil {
		return nil, err
	}
	return leaderEpochFromLease(&lease)
}

// leaderEpochFromLease derives the four-field epoch from a Lease that carries
// live leadership evidence. renewTime and leaseDurationSeconds are required as
// evidence of liveness but are not part of the epoch, and neither is
// resourceVersion: ordinary renewal changes both, and renewal is not a new
// leadership epoch. It mirrors the CLI verifier's derivation exactly, so the
// operator and `fathomctl definition drain` cannot disagree about what epoch a
// Lease represents.
func leaderEpochFromLease(lease *coordinationv1.Lease) (*fathomv1alpha1.DefinitionLeaderEpoch, error) {
	spec := lease.Spec
	if lease.UID == "" || !lease.DeletionTimestamp.IsZero() ||
		spec.HolderIdentity == nil || *spec.HolderIdentity == "" ||
		spec.AcquireTime == nil || spec.AcquireTime.IsZero() ||
		spec.RenewTime == nil || spec.RenewTime.IsZero() ||
		spec.LeaseDurationSeconds == nil || *spec.LeaseDurationSeconds <= 0 ||
		spec.LeaseTransitions == nil || *spec.LeaseTransitions < 0 {
		return nil, fmt.Errorf("lease %s/%s lacks live leadership evidence", lease.Namespace, lease.Name)
	}
	return &fathomv1alpha1.DefinitionLeaderEpoch{
		LeaseUID:         string(lease.UID),
		HolderIdentity:   *spec.HolderIdentity,
		AcquireTime:      *spec.AcquireTime,
		LeaseTransitions: *spec.LeaseTransitions,
	}, nil
}

// sameLeaderEpoch compares all four epoch fields.
func sameLeaderEpoch(a, b *fathomv1alpha1.DefinitionLeaderEpoch) bool {
	return a != nil && b != nil &&
		a.LeaseUID == b.LeaseUID &&
		a.HolderIdentity == b.HolderIdentity &&
		a.AcquireTime.Equal(&b.AcquireTime) &&
		a.LeaseTransitions == b.LeaseTransitions
}

func drainedCondition(status metav1.ConditionStatus, reason, message string) conditionSpec {
	return conditionSpec{Type: definitionConditionDrained, Status: status, Reason: reason, Message: message}
}

func drainUnverifiable(format string, args ...any) conditionSpec {
	return drainedCondition(metav1.ConditionFalse, reasonDrainUnverifiable, fmt.Sprintf(format, args...))
}

// clampActiveRuns keeps the reported count inside the schema's bound; the
// scheduler cannot exceed it, so a larger value would be a lie the API rejects.
func clampActiveRuns(runs int) int32 {
	switch {
	case runs < 0:
		return 0
	case runs > definitions.MaxConcurrentRuns:
		return definitions.MaxConcurrentRuns
	}
	return int32(runs)
}

// bindingsForDefinition maps a definition event to its binding through the
// definition-name index, so a definition change never lists the namespace.
func (r *AddonDefinitionBindingReconciler) bindingsForDefinition(ctx context.Context, obj client.Object) []reconcile.Request {
	def, ok := obj.(*fathomv1alpha1.AddonDefinition)
	if !ok || r.OperatorNamespace == "" {
		return nil
	}
	var bindings fathomv1alpha1.AddonDefinitionBindingList
	if err := r.List(ctx, &bindings,
		client.InNamespace(r.OperatorNamespace),
		client.MatchingFields{IndexBindingDefinitionName: def.Name},
		client.Limit(definitions.MaxPageObjects),
	); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(bindings.Items))
	for i := range bindings.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: bindings.Items[i].Namespace, Name: bindings.Items[i].Name,
		}})
	}
	return requests
}

// SetupWithManager registers the binding controller and the shared indexes.
// Wiring it into the manager is T047's; nothing calls this yet.
func (r *AddonDefinitionBindingReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if err := RegisterAddonDefinitionIndexes(ctx, mgr.GetFieldIndexer()); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.AddonDefinitionBinding{}).
		Watches(&fathomv1alpha1.AddonDefinition{}, handler.EnqueueRequestsFromMapFunc(r.bindingsForDefinition)).
		Named("addondefinitionbinding").
		Complete(r)
}
