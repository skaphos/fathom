/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/impersonation"
	"github.com/skaphos/fathom/internal/adapter/registry"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
)

// This file is the runtime AddonCheck execution path: the production caller of
// internal/adapter/runtime.Execute (T039/T040 of
// specs/012-addon-definition-runtime).
//
// It is deliberately not a reconciler. Runtime loading is default-off and the
// manager wiring is T047's, so this type is constructed and driven by the
// AddonCheck reconciler once that wiring exists; until then nothing in
// production calls it and built-in AddonCheck behaviour is untouched.
//
// The shape of a run is fixed:
//
//	resolve -> admit -> budget -> pre-run fence -> execute -> final fence -> publish
//
// Everything between the two fences runs under exactly one [execution.Budget],
// so the manager's control-plane metadata reads and the delegated evaluator's
// reads share one deadline and one set of counters even though their identities
// are separate and the manager's client never reaches an evaluator.

// Additional condition reasons for the runtime check path. The matrix in
// contracts/runtime.md spells the failures; the rest name what happened.
const (
	// reasonUnknownAddonType marks an addon identity that neither a built-in
	// adapter nor a published runtime snapshot claims. It lives here rather
	// than on the definition reconciler because only an AddonCheck can name an
	// identity no definition ever stored.
	reasonUnknownAddonType = "UnknownAddonType"
	// reasonSuperseded marks a completed run whose captured context no longer
	// matches the stored one: "Final revision/context mismatch discards
	// completion ... enqueue current revision".
	reasonSuperseded = "Superseded"
	// reasonInvalidBinding marks a stored binding spec that fails validation.
	reasonInvalidBinding = "InvalidBinding"
	// reasonRuntimeTimeout is the contract's Time row: the one run deadline
	// covering discovery, reads, evaluation and final validation elapsed.
	reasonRuntimeTimeout = "Timeout"
	// reasonRunCanceled marks a run cancelled by something other than an
	// observed revocation or its own deadline (manager shutdown, for example).
	reasonRunCanceled = "RunCanceled"
	// reasonAuthorizationUnavailable marks authority that cannot be
	// established at all: no leadership session may admit, no epoch has been
	// observed, or the per-run clients cannot be built.
	reasonAuthorizationUnavailable = "AuthorizationUnavailable"
	// reasonPublicationInFlight marks a publication refused because another
	// run is already publishing this check. It is a serialization outcome, not
	// a failure of the run.
	reasonPublicationInFlight = "PublicationInFlight"
	// reasonPublicationConflict marks a lost compare-and-swap: the AddonCheck
	// changed between the final fence reading it and the status write.
	reasonPublicationConflict = "PublicationConflict"
	// reasonRunCompleted is the only reason accompanying Ready=True. It means
	// the run executed to completion with eligible inputs — contracts/
	// runtime.md: "Ready denotes executable/completed ... neither means Pass."
	reasonRunCompleted = "RunCompleted"
	// reasonIncompleteEvaluation marks a run that executed to completion but
	// whose aggregate verdict is Error or Unknown — the evaluator could not
	// determine health. Completed evidence is Pass/Warn/Fail/Skipped only, so
	// this is recorded as an attempt error and the previous observation is
	// preserved ("Attempt Error still preserves previous completed evidence").
	reasonIncompleteEvaluation = "IncompleteEvaluation"
	// reasonInvalidPolicy marks an AddonCheck policy rejected against the exact
	// runtime adapter and check generation captured by the pre-run fence.
	reasonInvalidPolicy = "InvalidPolicy"
)

// Status text bounds. contracts/runtime.md: "Strings ≤1,024 UTF-8 bytes and
// 1,024 code points unless tighter"; data-model.md tightens condition reasons
// to 128 bytes and identifiers to 128. They are enforced here rather than left
// to the API server, because a rejected status write turns a health observation
// into a controller error.
const (
	addonCheckStatusTextLimit       = 1024
	addonCheckStatusReasonLimit     = 128
	addonCheckStatusIdentifierLimit = 128
	addonCheckStatusVersionLimit    = 63
)

// Barrier phases for the runtime check path, mirroring the seam the definition
// reconciler uses: a test holds a run at an exact point and mutates the API
// underneath it, which is the only deterministic way to drive the "edit during
// evaluation", revocation and publication-race rows. Production leaves the seam
// nil and pays nothing for it.
const (
	runtimeBarrierAfterPreFence = "afterPreFence"
	runtimeBarrierAfterExecute  = "afterExecute"
	runtimeBarrierBeforePublish = "beforePublish"
)

// runtimeFenceRequeue is how long the caller waits before re-reading current
// inputs after a fence, a revocation or a lost publication discarded a run. It
// is short because every one of those outcomes is expected to be transient and
// the watch that would also wake the check may have already fired.
const runtimeFenceRequeue = 5 * time.Second

// errNotRuntimeAddonType reports that an addon identity resolves to a built-in
// adapter. The runtime path declines it so the existing AddonCheck reconciler
// keeps serving it unchanged: runtime loading must never alter a built-in.
var errNotRuntimeAddonType = errors.New("controller: addon type is served by a built-in adapter, not a runtime definition")

// errRuntimeRunnerUnwired is returned when the runner was constructed without
// something it has no safe default for. Every field below is mandatory: a
// guessed operator namespace, an absent registry or a missing leadership
// decision would each turn a fail-closed path into a silently permissive one.
var errRuntimeRunnerUnwired = errors.New(
	"controller: runtime AddonCheck execution requires a client, a registry, a leadership session, a per-run client factory, an explicit operator namespace and the manager service account")

// RuntimeExecutionSession is the narrow view of the elected leadership session
// the runtime AddonCheck path needs: it asks before executing, and believes the
// session about the epoch a completed run may be attributed to.
//
// It is declared here rather than imported because internal/app imports
// internal/controller, so the dependency cannot run the other way (verified
// with `go list -deps ./internal/app`, which lists
// github.com/skaphos/fathom/internal/controller, while `go list -deps
// ./internal/controller` lists no internal/app). *app.RuntimeLeadership
// implements all four methods with these exact signatures, so T047 can wire it
// directly with no shadowing adapter.
type RuntimeExecutionSession interface {
	// LeaseRef is the configured namespaced Lease of this session. It is
	// trusted operator configuration, never inferred from a binding's status.
	LeaseRef() types.NamespacedName
	// Admit registers one unit of runtime work for the binding and returns a
	// context cancelled when leadership ends or the binding is revoked, plus an
	// idempotent release.
	Admit(key string) (context.Context, func(), error)
	// Epoch is the leadership epoch observed live by this session, or nil
	// before one has been observed.
	Epoch() *fathomv1alpha1.DefinitionLeaderEpoch
	// EpochValid reports whether an epoch still belongs to this session's
	// observed leadership. Equality uses all four epoch fields.
	EpochValid(*fathomv1alpha1.DefinitionLeaderEpoch) bool
}

// RuntimeDispatchResolver is the registry's dispatch surface. The registry is
// the single enforcement point: this path executes only what it resolves.
type RuntimeDispatchResolver interface {
	Resolve(addonType string) (registry.Resolution, error)
}

// RuntimeEvaluatorFactory builds the isolated, impersonating client an
// evaluator runs under. *impersonation.RuntimeFactory implements it.
type RuntimeEvaluatorFactory interface {
	ClientFor(ctx context.Context, name string,
		buildGuard func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error),
	) (client.Client, *impersonation.RuntimeAuthority, error)
}

// RuntimeRunClients are the two per-run seams. Control is the uncached
// control-plane reader both fences use and no evaluator ever sees; Evaluator
// builds the delegated client. They are returned together because they are
// built together, from one budget.
//
// Control is typed as the concrete [impersonation.RuntimeControlReader] rather
// than as a bare client.Reader on purpose. contracts/runtime.md requires that
// "Control-plane metadata/final fences use uncached APIReader and never enter
// the evaluator", and the manager's cached client satisfies client.Reader — so
// a bare interface would let a wiring mistake hand the fences the informer
// cache and still compile. Only [impersonation.NewRuntimeControlReader] builds
// this type, so the compiler enforces the property rather than review.
type RuntimeRunClients struct {
	Control   impersonation.RuntimeControlReader
	Evaluator RuntimeEvaluatorFactory
}

// RuntimeRunFactory builds the per-run clients from the single outer budget and
// the exact control-plane targets this run may read.
//
// The budget parameter is the whole contract of this seam: contracts/
// runtime.md requires that "Internal retry and pagination restart counters
// share outer budgets" and contracts/leadership.md that "The same outer run
// deadline/counters cover pre/final control-plane validation and runtime reads,
// even though manager and evaluator identities remain separate". Handing both
// readers the same *execution.Budget is how that holds. T047 supplies
// impersonation.NewRuntimeControlReader and impersonation.NewRuntimeFactory.
type RuntimeRunFactory func(*execution.Budget, execution.ControlTargets) (RuntimeRunClients, error)

// RuntimeFence is the publication context a run is attributed to: everything
// T039 requires captured, taken from uncached control-plane reads plus the
// registry snapshot actually dispatched to and the live leadership epoch.
//
// Generations are used, never resource versions. metadata.generation advances
// only on a spec change, so a binding or check status write — which happens on
// every reconcile — cannot invalidate captured authority, while a spec edit
// always does (contracts/leadership.md: "status writes do not change
// authority").
type RuntimeFence struct {
	DefinitionUID        types.UID
	DefinitionGeneration int64
	Revision             registry.RuntimeRevision
	// Provenance is the publication context recorded WITH, but not part of,
	// the revision: "publication additionally records operator build and
	// adapterVersion" (data-model.md). It is deliberately outside the
	// supersession comparison, which is a revision comparison.
	Provenance            registry.RuntimeProvenance
	BindingUID            types.UID
	BindingSpecGeneration int64
	ServiceAccountUID     types.UID
	CheckUID              types.UID
	CheckGeneration       int64
	// CheckPolicy fingerprints spec.policy. The policy selects which families
	// run, so two runs of the same generation under different policies are not
	// interchangeable evidence.
	CheckPolicy string
	// Epoch is the leadership epoch observed when the run was admitted.
	Epoch *fathomv1alpha1.DefinitionLeaderEpoch
}

// RuntimeAttempt is one runtime AddonCheck attempt. Completed means the
// evaluator ran to completion with eligible inputs; it is not a verdict, and
// Evidence may be entirely Fail. Published means a status write landed.
type RuntimeAttempt struct {
	Fence     RuntimeFence
	Evidence  adapter.Result
	Completed bool
	Published bool
	// publication is the exact status object accepted by the API server and
	// the evidence it replaced. The report path must not reconstruct either
	// from the manager cache, which can lag behind this publication.
	publication      *fathomv1alpha1.AddonCheck
	previousEvidence *fathomv1alpha1.AddonCheckEvidence
	// recordBase is the uncached, fenced AddonCheck whose resourceVersion and
	// generation an early refusal describes. Nil uses the caller's check.
	recordBase *fathomv1alpha1.AddonCheck
	// policyValidated asks record to publish policy acceptance through the same
	// epoch-checked status CAS: false with policyErrs for InvalidPolicy, or true
	// with no errors when execution failed after validation succeeded.
	policyValidated bool
	policyErrs      []string
	// Reason and Message are the operator-visible outcome, chosen by the
	// publication precedence order.
	Reason  string
	Message string
	// Requeue asks the caller to re-read current inputs. It is set whenever a
	// run was discarded by something that is expected to resolve itself.
	Requeue time.Duration
}

// AddonCheckRuntimeRunner executes one AddonCheck against a published runtime
// snapshot, under the elected leader's admission and the registry's dispatch
// gate, and publishes the outcome under a per-check compare-and-swap.
type AddonCheckRuntimeRunner struct {
	// Client is the manager's client. It is used for exactly one thing: the
	// status compare-and-swap. No fence ever reads through it.
	Client client.Client

	// Registry owns dispatch. Resolve is the only way this path obtains an
	// adapter, so a barrier or a withdrawn snapshot stops execution.
	Registry RuntimeDispatchResolver

	// Session is the elected leadership session. It decides admission; this
	// path supplies the object-side fences the session deliberately never looks
	// at.
	Session RuntimeExecutionSession

	// Clients builds the per-run control reader and evaluator factory.
	Clients RuntimeRunFactory

	// OperatorNamespace is the only namespace whose bindings carry authority.
	OperatorNamespace string
	// ManagerServiceAccount is the operator's own identity, which can never be
	// a dedicated reader.
	ManagerServiceAccount string
	// ProbeImage is forwarded verbatim into adapter.Request, as for built-ins.
	ProbeImage string

	// Now supplies the wall clock every observation and attempt timestamp is
	// stamped from. Nil means [time.Now]. It exists because evidence ageing is
	// measured in whole check intervals: a test of the freshness window must
	// move the clock, not sleep through it.
	Now func() time.Time

	// publishing serializes publication per check. It holds only the checks
	// currently being published, so it is bounded by the runtime concurrency
	// cap rather than by the number of checks in the cluster.
	publishing sync.Map

	// barrier is the test-only rendezvous seam described above.
	barrier func(ctx context.Context, phase string)
}

// Run executes check against its runtime snapshot, publishes a completed run's
// evidence and records every other outcome as an attempt.
//
// It returns errNotRuntimeAddonType when the identity belongs to a built-in, so
// the caller falls through to the unchanged built-in path. Every other outcome
// — including every contract failure — is reported in the attempt rather than
// as an error: a bounded failure is a health observation, not a controller
// malfunction. An error means a mis-wiring or a status write that failed for a
// reason other than a lost compare-and-swap.
//
// Recording is done HERE rather than handed back to the caller because the
// lifecycle matrix's "Evidence and recovery" column is not optional: a caller
// that forgot it would leave every Ready=False/UnknownAddonType,
// Ready=False/AccessDenied and freshness=Unavailable row silently unwritten,
// and the omission would look exactly like a healthy check that simply has not
// run yet.
func (r *AddonCheckRuntimeRunner) Run(ctx context.Context, check *fathomv1alpha1.AddonCheck) (RuntimeAttempt, error) {
	attempt, err := r.execute(ctx, check)
	if err != nil {
		return attempt, err
	}
	if attempt.Completed {
		// A completed run has already been through publish, which is the only
		// place completed evidence is written.
		return attempt, nil
	}
	recordBase := check
	if attempt.recordBase != nil {
		recordBase = attempt.recordBase
	}
	if err := r.record(ctx, recordBase, attempt); err != nil {
		return attempt, err
	}
	return attempt, nil
}

// execute is the run itself: resolve, admit, fence, evaluate, fence, publish.
func (r *AddonCheckRuntimeRunner) execute(ctx context.Context, check *fathomv1alpha1.AddonCheck) (RuntimeAttempt, error) {
	if r == nil || r.Client == nil || r.Registry == nil || r.Session == nil || r.Clients == nil ||
		r.OperatorNamespace == "" || r.ManagerServiceAccount == "" {
		return RuntimeAttempt{}, errRuntimeRunnerUnwired
	}
	if check == nil || check.Spec.AddonType == "" || check.Name == "" || check.Namespace == "" {
		return RuntimeAttempt{}, errors.New("controller: runtime execution requires a named AddonCheck with an addon type")
	}
	addonType := check.Spec.AddonType

	// The registry decides what may dispatch at all. A barrier or an unknown
	// identity is refused before a leadership slot is taken, so a barred
	// definition cannot keep a run slot away from a healthy one.
	resolution, err := r.Registry.Resolve(addonType)
	switch {
	case err != nil:
		var barrier *registry.DispatchBarrier
		if errors.As(err, &barrier) {
			return refused(barrier.Reason, barrier.Error()), nil
		}
		if errors.Is(err, registry.ErrNotFound) {
			return refused(reasonUnknownAddonType,
				fmt.Sprintf("no built-in adapter and no runtime definition claims addon type %q", addonType)), nil
		}
		return RuntimeAttempt{}, err
	case !resolution.Runtime:
		return RuntimeAttempt{}, fmt.Errorf("%w: %q", errNotRuntimeAddonType, addonType)
	case resolution.Adapter == nil:
		return RuntimeAttempt{}, fmt.Errorf("controller: registry resolved addon type %q to no adapter", addonType)
	}

	// The Lease is trusted operator configuration. A session pointed at another
	// namespace cannot fence anything in this one, so it is refused rather than
	// reinterpreted.
	lease := r.Session.LeaseRef()
	if lease.Name == "" || lease.Namespace != r.OperatorNamespace {
		return RuntimeAttempt{}, fmt.Errorf(
			"controller: leadership session observes Lease %s, which is not in the configured operator namespace %q",
			lease, r.OperatorNamespace)
	}

	// The binding's name equals the definition's name, which equals the addon
	// type: one admission key per binding, which is what drain counts.
	admitted, releaseAdmission, err := r.Session.Admit(addonType)
	if err != nil {
		return refused(reasonAuthorizationUnavailable,
			fmt.Sprintf("the leadership session refused admission for %q: %v", addonType, err)), nil
	}

	// One budget for the whole run. Its parent is the caller's context, not the
	// admission context, so releasing the slot cannot cancel the final fence;
	// an observed revocation reaches the run through the watcher below instead.
	budget, closeBudget := execution.NewBudget(ctx, addonCheckTimeout(check))
	defer closeBudget()
	// An OBSERVED revocation cancels this run. That is all it is: the run
	// notices the cancellation and the final fence re-reads live authority, so
	// nothing is published under authority this process has seen withdrawn.
	// Nothing here fences a run that never observes the revocation — there is no
	// atomic revocation to claim, and contracts/leadership.md is explicit that a
	// suspended prior holder is not fenced. A run that has already issued a read
	// the API server has already answered cannot be un-issued; what the contract
	// guarantees is that such a run cannot publish, not that it cannot exist.
	stopWatchingAdmission := context.AfterFunc(admitted, func() {
		_ = budget.Fail(reasonAuthorizationRevoked,
			"leadership admission for this binding was revoked or the session ended")
	})
	// Releasing the slot detaches the watcher first, so a normal release is
	// never mistaken for a revocation. It is handed to execution.Execute, which
	// releases it the moment evaluation returns — the fences that follow need
	// no slot, and holding one would keep a drain from ever reaching zero.
	release := sync.OnceFunc(func() {
		stopWatchingAdmission()
		releaseAdmission()
	})
	defer release()

	targets := execution.ControlTargets{
		OperatorNamespace: r.OperatorNamespace,
		DefinitionName:    addonType,
		LeaseName:         lease.Name,
		Check:             client.ObjectKeyFromObject(check),
	}
	clients, err := r.Clients(budget, targets)
	if err != nil {
		return r.discarded(budget, err), nil
	}
	// The zero RuntimeControlReader carries no reader at all, which is the only
	// way this seam can still be unusable now that its type is fixed.
	if clients.Control.Reader == nil || clients.Evaluator == nil {
		return refused(reasonAuthorizationUnavailable,
			"the per-run client factory supplied no control reader or no evaluator factory"), nil
	}

	fence, fenced, evaluator, err := r.preRunFence(budget, clients, resolution, addonType, check)
	if err != nil {
		// "failed validation prevents publication": nothing has executed and
		// nothing is written.
		return r.discarded(budget, err), nil
	}
	if policyErrs := validateAddonCheckPolicy(fenced, resolution.Adapter); len(policyErrs) > 0 {
		return RuntimeAttempt{
			Fence:           fence,
			Reason:          reasonInvalidPolicy,
			Message:         "AddonCheck policy is invalid: " + strings.Join(policyErrs, "; ") + ".",
			Requeue:         runtimeFenceRequeue,
			recordBase:      fenced,
			policyValidated: true,
			policyErrs:      append([]string(nil), policyErrs...),
		}, nil
	}
	r.hold(ctx, runtimeBarrierAfterPreFence)

	attempt := execution.Execute(budget,
		func(context.Context) (adapter.Adapter, error) { return resolution.Adapter, nil },
		adapter.Request{
			Client: evaluator,
			Logger: logf.FromContext(ctx).WithValues("adapter", resolution.Adapter.Name(), "addonType", addonType),
			Target: addonCheckTargetRef(fenced),
			Policy: addonCheckPolicy(fenced),
			// The timeout the adapter is told about is the one the budget was
			// actually built with, so a spec edit between the two cannot leave
			// an evaluator pacing itself against a deadline it does not have.
			Timeout:    addonCheckTimeout(check),
			ProbeImage: r.ProbeImage,
		}, release)
	r.hold(ctx, runtimeBarrierAfterExecute)

	result, err := r.finalFence(ctx, budget, clients, fence, addonType, targets.Check, attempt)
	if !result.Completed {
		// A failed evaluation still proved that this exact fenced policy is
		// accepted. Record from the fenced resourceVersion so a later spec edit
		// wins the CAS and never inherits that conclusion.
		result.recordBase = fenced
		result.policyValidated = true
	}
	return result, err
}

// preRunFence resolves live authority and captures the exact context this run
// will be attributed to. Every read is uncached and budget-bound: the informer
// cache cannot be trusted to have seen the revocation that matters.
func (r *AddonCheckRuntimeRunner) preRunFence(
	budget *execution.Budget, clients RuntimeRunClients, resolution registry.Resolution,
	addonType string, check *fathomv1alpha1.AddonCheck,
) (RuntimeFence, *fathomv1alpha1.AddonCheck, client.Client, error) {
	epoch := r.Session.Epoch()
	if epoch == nil || !r.Session.EpochValid(epoch) {
		return RuntimeFence{}, nil, nil, &authorityFailure{
			Reason:  reasonAuthorizationRevoked,
			Message: "this leadership session has observed no live Lease epoch, so no run may be attributed to it",
		}
	}

	// ClientFor resolves definition, binding and dedicated service account
	// through the same uncached reader, applies every identity and scope rule,
	// and only then builds the impersonating client. A failure here is an
	// authority failure, already labelled with its contract reason.
	evaluator, authority, err := clients.Evaluator.ClientFor(budget.Context(), addonType, r.buildGuard(budget))
	if err != nil {
		return RuntimeFence{}, nil, nil, err
	}

	var fenced fathomv1alpha1.AddonCheck
	key := client.ObjectKeyFromObject(check)
	if err := clients.Control.Get(budget.Context(), key, &fenced); err != nil {
		return RuntimeFence{}, nil, nil, readFailure(err, fmt.Sprintf("AddonCheck %s", key))
	}
	if fenced.Spec.AddonType != addonType {
		return RuntimeFence{}, nil, nil, &authorityFailure{
			Reason:  reasonSuperseded,
			Message: fmt.Sprintf("AddonCheck %s now names addon type %q", key, fenced.Spec.AddonType),
		}
	}

	return RuntimeFence{
		DefinitionUID:         authority.Definition.UID,
		DefinitionGeneration:  authority.Definition.Generation,
		Revision:              resolution.Revision,
		Provenance:            resolution.Provenance,
		BindingUID:            authority.Binding.UID,
		BindingSpecGeneration: authority.Binding.Generation,
		ServiceAccountUID:     authority.ServiceAccount.UID,
		CheckUID:              fenced.UID,
		CheckGeneration:       fenced.Generation,
		CheckPolicy:           addonCheckPolicyFingerprint(&fenced),
		Epoch:                 epoch,
	}, &fenced, evaluator, nil
}

// buildGuard supplies the delegated transport's scope and work guard. It closes
// over the same budget the fences use, which is what makes evaluator reads
// visible to the final fence and fence reads visible to the evaluator.
func (r *AddonCheckRuntimeRunner) buildGuard(budget *execution.Budget) func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
	return func(authority *impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
		expected, err := execution.DiscoveryExpectations(authority.Definition)
		if err != nil {
			return nil, err
		}
		guard, err := execution.NewGuard(budget, authority.Binding.Spec.TargetScope, expected)
		if err != nil {
			return nil, err
		}
		return guard.Wrap, nil
	}
}

// publicationRank is the publication precedence of contracts/runtime.md,
// verbatim: "authority revoked/mismatched; definition or check superseded;
// invalid input; recorded execution failure."
//
// It is modelled as an order rather than as four independent branches on
// purpose. Several of these routinely hold at once — a revoked binding whose
// definition was also edited while the run was failing its budget — and the
// operator must always read the earliest one, because it is the one that
// explains why the later ones no longer matter.
type publicationRank int

const (
	rankAuthority publicationRank = iota
	rankSuperseded
	rankInvalidInput
	rankExecution
	rankCompleted
)

// publicationCandidate is one reason a run might not publish completed
// evidence, together with where it sits in the precedence order.
type publicationCandidate struct {
	rank    publicationRank
	reason  string
	message string
}

// publicationRankOf places a contract reason in the precedence order. Reasons
// are grouped by what they say about the run, not by which component raised
// them: an access denial and a replaced service account are both statements
// that authority no longer holds, and a size or parser rejection is a statement
// about input regardless of whether admission or the evaluator caught it.
func publicationRankOf(reason string) publicationRank {
	switch reason {
	case reasonAuthorizationRevoked, reasonBindingMismatch, reasonAccessDenied,
		reasonDefinitionUnavailable, reasonAuthorizationUnavailable:
		return rankAuthority
	case reasonSuperseded:
		return rankSuperseded
	case reasonInvalidDefinition, reasonInvalidBinding, "DefinitionTooLarge",
		"InputLimitExceeded", "InvalidVersionRange", "ScopeDenied":
		return rankInvalidInput
	default:
		return rankExecution
	}
}

// electPublication returns the earliest candidate in the precedence order, or
// the completed outcome when there is none. Ties keep the first candidate
// offered, so the order candidates are gathered in is stable and the elected
// reason does not depend on map iteration or read ordering.
func electPublication(candidates []publicationCandidate) publicationCandidate {
	elected := publicationCandidate{rank: rankCompleted, reason: reasonRunCompleted,
		message: "runtime evaluation completed with eligible inputs"}
	for _, candidate := range candidates {
		if candidate.rank < elected.rank {
			elected = candidate
		}
	}
	return elected
}

// finalFence decides, in precedence order, whether this run may publish.
//
// Two candidate sources need no API read and are therefore always available:
// the run's own recorded cause (which carries an observed revocation when the
// admission context was cancelled), and the registry's current snapshot for
// this identity. The remaining candidates come from uncached re-reads, which
// require a budget that has not already recorded a failure — the control reader
// charges those reads to the shared counters, and a run that has already failed
// has none left to spend. That is the matrix's own answer for that case:
// "Deadline, size, parser or read budget exhausted | Attempt Error with
// specific reason", with the observed revocation still outranking it.
func (r *AddonCheckRuntimeRunner) finalFence(
	ctx context.Context, budget *execution.Budget, clients RuntimeRunClients,
	fence RuntimeFence, addonType string, checkKey types.NamespacedName, attempt execution.Attempt,
) (RuntimeAttempt, error) {
	var candidates []publicationCandidate

	// The run's recorded cause. execution.Execute has already collapsed the
	// budget's cause into the attempt, and the budget records only its FIRST
	// cause, so a deadline the run already observed is never renamed by a size
	// or parser error a late evaluator raised after cancellation — "Deadline
	// wins over later response-limit errors after cancellation."
	if attempt.Err != nil {
		candidates = append(candidates, candidateFor(attempt.Err))
	}

	// Snapshot supersession, decided with no I/O: the registry either still
	// publishes the exact revision this run dispatched to, or it does not.
	if resolution, err := r.Registry.Resolve(addonType); err != nil || !resolution.Runtime || resolution.Revision != fence.Revision {
		candidates = append(candidates, publicationCandidate{
			rank: rankSuperseded, reason: reasonSuperseded,
			message: fmt.Sprintf("the runtime snapshot for %q was replaced or withdrawn while the run was executing", addonType),
		})
	}

	var fenced *fathomv1alpha1.AddonCheck
	if budget.Err() == nil {
		var read []publicationCandidate
		read, fenced = r.rereadFence(budget, clients, fence, addonType, checkKey)
		candidates = append(candidates, read...)
	}

	elected := electPublication(candidates)
	if elected.rank != rankCompleted || fenced == nil {
		return RuntimeAttempt{
			Fence: fence, Reason: elected.reason, Message: elected.message,
			Requeue: runtimeFenceRequeue,
		}, nil
	}

	// Reaching rankCompleted means there were no candidates at all, which means
	// the budget recorded no failure, which means execution.Execute completed —
	// so a publication from here is always a completed run carrying its own
	// evidence. A recorded attempt failure can never reach publication, because
	// Execute aborts the shared budget and the fences that would make the write
	// attributable cannot then be made; the caller records such an attempt from
	// the returned RuntimeAttempt, and the retained evidence stays untouched
	// ("Attempt Error still preserves previous completed evidence").
	result := RuntimeAttempt{
		Fence: fence, Evidence: attempt.Evidence, Completed: attempt.Completed,
		Reason: elected.reason, Message: elected.message,
	}
	published, err := r.publish(ctx, fenced, fence, elected.message, attempt.Evidence)
	if err != nil {
		return result, err
	}
	result.Published = published.Published
	result.publication = published.publication
	result.previousEvidence = published.previousEvidence
	if published.Reason != "" {
		result.Reason, result.Message, result.Requeue = published.Reason, published.Message, published.Requeue
	}
	return result, nil
}

// rereadFence re-reads every captured object through the uncached control
// reader and reports every candidate the comparison produces.
//
// Within one object the checks are ordered so that a single edit contributes
// exactly one candidate: an edit that leaves the spec invalid is InvalidDefinition
// ("Invalid edit stored | Remove eligibility; Accepted=False/InvalidDefinition"),
// and only an edit that leaves it valid is a supersession ("Edited to valid
// revision | ... old run becomes Superseded"). Emitting both would let the
// precedence order report a valid replacement where the matrix names an invalid
// one.
func (r *AddonCheckRuntimeRunner) rereadFence(
	budget *execution.Budget, clients RuntimeRunClients, fence RuntimeFence,
	addonType string, checkKey types.NamespacedName,
) ([]publicationCandidate, *fathomv1alpha1.AddonCheck) {
	var candidates []publicationCandidate
	deny := func(reason, format string, args ...any) {
		candidates = append(candidates, publicationCandidate{
			rank: publicationRankOf(reason), reason: reason, message: fmt.Sprintf(format, args...),
		})
	}

	// The epoch is re-validated against the live session, not against stored
	// status: "Lease loss/unknown epoch rejects publication".
	if !r.Session.EpochValid(fence.Epoch) {
		deny(reasonAuthorizationRevoked, "leadership changed while the run was executing; no evidence may be attributed to the observed epoch")
	}

	var definition fathomv1alpha1.AddonDefinition
	switch err := clients.Control.Get(budget.Context(), types.NamespacedName{Name: addonType}, &definition); {
	case apierrors.IsNotFound(err):
		deny(reasonDefinitionUnavailable, "AddonDefinition %q was deleted while the run was executing", addonType)
	case err != nil:
		failure := readFailure(err, fmt.Sprintf("AddonDefinition %q", addonType))
		deny(failure.Reason, "%s", failure.Error())
	case definition.UID != fence.DefinitionUID:
		deny(reasonBindingMismatch, "AddonDefinition %q was recreated with UID %q; authority is not inherited", addonType, definition.UID)
	default:
		if err := definitions.Validate(&definition); err != nil {
			deny(reasonInvalidDefinition, "AddonDefinition %q no longer validates: %v", addonType, err)
		} else if definition.Generation != fence.DefinitionGeneration {
			deny(reasonSuperseded, "AddonDefinition %q advanced to generation %d", addonType, definition.Generation)
		}
	}

	bindingKey := types.NamespacedName{Namespace: r.OperatorNamespace, Name: addonType}
	var binding fathomv1alpha1.AddonDefinitionBinding
	switch err := clients.Control.Get(budget.Context(), bindingKey, &binding); {
	case apierrors.IsNotFound(err):
		deny(reasonAuthorizationRevoked, "AddonDefinitionBinding %s was deleted while the run was executing", bindingKey)
	case err != nil:
		failure := readFailure(err, fmt.Sprintf("AddonDefinitionBinding %s", bindingKey))
		deny(failure.Reason, "%s", failure.Error())
	case binding.UID != fence.BindingUID, !binding.DeletionTimestamp.IsZero():
		deny(reasonBindingMismatch, "AddonDefinitionBinding %s is not the binding this run was authorized by", bindingKey)
	case !binding.Spec.Enabled:
		deny(reasonAuthorizationRevoked, "AddonDefinitionBinding %s was disabled while the run was executing", bindingKey)
	case binding.Spec.ServiceAccountRef.UID != string(fence.ServiceAccountUID):
		deny(reasonBindingMismatch, "AddonDefinitionBinding %s now delegates to a different service account", bindingKey)
	default:
		if err := definitions.ValidateBinding(&binding); err != nil {
			deny(reasonInvalidBinding, "AddonDefinitionBinding %s no longer validates: %v", bindingKey, err)
		} else if binding.Generation != fence.BindingSpecGeneration {
			// Generation, not resourceVersion: a status write never reaches
			// here, which is exactly what "status writes do not change
			// authority" requires.
			deny(reasonSuperseded, "AddonDefinitionBinding %s advanced to spec generation %d", bindingKey, binding.Generation)
		}
		// The dedicated reader is only worth re-reading once the binding that
		// names it still holds: a revoked or mismatched binding has already
		// produced the earlier candidate, and reading an account it no longer
		// delegates to would spend a request to learn nothing.
		saKey := types.NamespacedName{Namespace: r.OperatorNamespace, Name: string(binding.Spec.ServiceAccountRef.Name)}
		var account corev1.ServiceAccount
		switch err := clients.Control.Get(budget.Context(), saKey, &account); {
		case apierrors.IsNotFound(err):
			deny(reasonBindingMismatch, "the dedicated service account %s was deleted while the run was executing", saKey)
		case err != nil:
			failure := readFailure(err, fmt.Sprintf("ServiceAccount %s", saKey))
			deny(failure.Reason, "%s", failure.Error())
		case account.UID != fence.ServiceAccountUID:
			deny(reasonBindingMismatch, "the dedicated service account %s was recreated with UID %q", saKey, account.UID)
		}
	}

	var check fathomv1alpha1.AddonCheck
	switch err := clients.Control.Get(budget.Context(), checkKey, &check); {
	case apierrors.IsNotFound(err):
		deny(reasonSuperseded, "AddonCheck %s was deleted while the run was executing", checkKey)
	case err != nil:
		failure := readFailure(err, fmt.Sprintf("AddonCheck %s", checkKey))
		deny(failure.Reason, "%s", failure.Error())
	case check.UID != fence.CheckUID:
		deny(reasonSuperseded, "AddonCheck %s was recreated with UID %q", checkKey, check.UID)
	case check.Generation != fence.CheckGeneration:
		deny(reasonSuperseded, "AddonCheck %s advanced to generation %d", checkKey, check.Generation)
	case addonCheckPolicyFingerprint(&check) != fence.CheckPolicy:
		deny(reasonSuperseded, "the policy of AddonCheck %s changed while the run was executing", checkKey)
	default:
		return candidates, &check
	}
	return candidates, nil
}

// publish performs the per-check serialized compare-and-swap.
//
// Serialization is a refusal, not a queue: a reconciler must never block on
// another run, so a second publisher for the same check is turned away and
// requeued. The swap itself is the resource version the final fence read, so a
// status written after the fence read it always wins and this run publishes
// nothing rather than clobbering it.
func (r *AddonCheckRuntimeRunner) publish(
	ctx context.Context, fenced *fathomv1alpha1.AddonCheck, fence RuntimeFence, message string,
	result adapter.Result,
) (RuntimeAttempt, error) {
	key := client.ObjectKeyFromObject(fenced)
	if _, busy := r.publishing.LoadOrStore(key, struct{}{}); busy {
		return RuntimeAttempt{
			Reason:  reasonPublicationInFlight,
			Message: fmt.Sprintf("another runtime run is already publishing AddonCheck %s", key),
			Requeue: runtimeFenceRequeue,
		}, nil
	}
	defer r.publishing.Delete(key)

	r.hold(ctx, runtimeBarrierBeforePublish)

	published := fenced.DeepCopy()
	now := r.now()

	// The most recent run's own result is always recorded, whether or not it
	// becomes evidence: LastResult documents itself as "the aggregate result
	// from the most recent adapter run".
	aggregate, coverage, coverageMessage := addonCheckEvidenceOutcome(fenced, result)
	published.Status.LastRunTime = &metav1.Time{Time: now}
	published.Status.LastResult = string(aggregate)
	published.Status.LatestAttemptAt = &metav1.Time{Time: now}

	if verdict, ok := addonCheckCompletedVerdict(aggregate); ok {
		published.Status.LatestAttemptOutcome = fathomv1alpha1.AddonCheckAttemptCompleted
		published.Status.LatestAttemptReason = reasonRunCompleted
		published.Status.LatestAttemptMessage = boundedText(message, addonCheckStatusTextLimit)
		// A completed run REPLACES current evidence, and its observedAt is its
		// own — including the all-Skipped case, whose "observedAt/revision/
		// context advances after successful publication fences".
		published.Status.LastSuccessfulEvaluation = &fathomv1alpha1.AddonCheckEvidence{
			Verdict:    verdict,
			Coverage:   coverage,
			Message:    boundedText(coverageMessage, addonCheckStatusTextLimit),
			ObservedAt: metav1.Time{Time: now},
			Revision:   addonCheckEvidenceRevision(fence),
			Authority:  addonCheckEvidenceAuthority(fence),
		}
		setAddonCheckFreshness(published, reasonRunCompleted, now)
	} else {
		// The run completed but could not determine health. Completed evidence
		// is Pass/Warn/Fail/Skipped; anything else is an attempt error, and an
		// attempt error preserves — nothing below touches
		// Status.LastSuccessfulEvaluation or its observedAt.
		//
		// This branch is unreachable through execution.Execute today, because
		// runtime.ValidateResult rejects a check-level Error and any invalid
		// outcome before evidence is sealed (asserted by
		// TestACheckLevelErrorNeverBecomesCompletedEvidence). It is kept as a
		// schema guard rather than removed: writing an out-of-enum verdict
		// would be rejected by the API server outright, turning a health
		// observation into a controller error, and the narrowing that prevents
		// it is pinned directly by TestCompletedEvidenceFoldAndNarrowing.
		published.Status.LatestAttemptOutcome = fathomv1alpha1.AddonCheckAttemptError
		published.Status.LatestAttemptReason = reasonIncompleteEvaluation
		published.Status.LatestAttemptMessage = boundedText(fmt.Sprintf(
			"the run completed but aggregated to %s, which is not completed evidence; the previous observation is preserved",
			aggregate), addonCheckStatusTextLimit)
		setAddonCheckFreshness(published, reasonIncompleteEvaluation, now)
	}

	// Ready=True means this run executed to completion with eligible inputs. It
	// is not a verdict: contracts/runtime.md, "Ready denotes executable/
	// completed, freshness denotes recency, and neither means Pass."
	apiMeta.SetStatusCondition(&published.Status.Conditions, metav1.Condition{
		Type:   addonCheckConditionReady,
		Status: metav1.ConditionTrue,
		// The fenced generation, not the live object's: the condition describes
		// the revision this run was actually attributed to.
		ObservedGeneration: fence.CheckGeneration,
		Reason:             reasonRunCompleted,
		Message:            boundedText(message, addonCheckStatusTextLimit),
	})
	setAddonCheckAccepted(published, nil)
	if equality.Semantic.DeepEqual(fenced.Status, published.Status) {
		// Nothing changed, not even a timestamp: rewriting it would churn
		// resourceVersion and re-trigger the very watch that scheduled this run.
		return RuntimeAttempt{}, nil
	}
	if err := r.Client.Status().Update(ctx, published); err != nil {
		if apierrors.IsConflict(err) {
			return RuntimeAttempt{
				Reason:  reasonPublicationConflict,
				Message: fmt.Sprintf("AddonCheck %s changed after the final fence read it; this run publishes nothing", key),
				Requeue: runtimeFenceRequeue,
			}, nil
		}
		return RuntimeAttempt{}, fmt.Errorf("controller: publish runtime AddonCheck %s status: %w", key, err)
	}
	return RuntimeAttempt{
		Published:        true,
		publication:      published.DeepCopy(),
		previousEvidence: fenced.Status.LastSuccessfulEvaluation.DeepCopy(),
	}, nil
}

// record writes the LATEST ATTEMPT for a run that produced no completed
// evidence, and re-derives freshness.
//
// It is the preservation half of T044: the completed evidence and its original
// observedAt, revision and context are never read, let alone written, from
// here — "Attempt Error still preserves previous completed evidence", and
// "Failed attempts cannot renew the observation timestamp."
//
// The compare-and-swap base is the object the caller handed in. A status write
// that loses the swap is dropped rather than retried: something newer has
// already described this check, and an attempt record is worth strictly less
// than whatever won.
func (r *AddonCheckRuntimeRunner) record(
	ctx context.Context, check *fathomv1alpha1.AddonCheck, attempt RuntimeAttempt,
) error {
	// A condition needs a reason: an empty one is rejected by the API server,
	// so an unattributed outcome is not worth a write. The serialization
	// outcomes (PublicationInFlight, PublicationConflict) never arrive here —
	// they accompany a completed run, and [AddonCheckRuntimeRunner.Run] returns
	// before recording for those.
	if attempt.Reason == "" {
		return nil
	}

	// contracts/leadership.md: losing the session "closes admission, cancels
	// workers and prevents further evidence/drain publication". A process that
	// does not hold a live epoch does not write this check's status at all —
	// not even to say why it could not run.
	epoch := r.Session.Epoch()
	if epoch == nil || !r.Session.EpochValid(epoch) {
		return nil
	}

	key := client.ObjectKeyFromObject(check)
	if _, busy := r.publishing.LoadOrStore(key, struct{}{}); busy {
		return nil
	}
	defer r.publishing.Delete(key)

	now := r.now()
	recorded := check.DeepCopy()
	recorded.Status.LatestAttemptAt = &metav1.Time{Time: now}
	recorded.Status.LatestAttemptOutcome = fathomv1alpha1.AddonCheckAttemptError
	recorded.Status.LatestAttemptReason = boundedText(attempt.Reason, addonCheckStatusReasonLimit)
	recorded.Status.LatestAttemptMessage = boundedText(attempt.Message, addonCheckStatusTextLimit)
	setAddonCheckFreshness(recorded, attempt.Reason, now)
	if attempt.policyValidated {
		setAddonCheckAccepted(recorded, attempt.policyErrs)
	}
	apiMeta.SetStatusCondition(&recorded.Status.Conditions, metav1.Condition{
		Type:   addonCheckConditionReady,
		Status: metav1.ConditionFalse,
		// The live generation: no fence attributed this attempt to an older one.
		ObservedGeneration: check.Generation,
		Reason:             boundedText(attempt.Reason, addonCheckStatusReasonLimit),
		Message:            boundedText(attempt.Message, addonCheckStatusTextLimit),
	})
	if equality.Semantic.DeepEqual(check.Status, recorded.Status) {
		return nil
	}
	if err := r.Client.Status().Update(ctx, recorded); err != nil {
		if apierrors.IsConflict(err) {
			return nil
		}
		return fmt.Errorf("controller: record runtime AddonCheck %s attempt: %w", key, err)
	}
	return nil
}

// addonCheckEvidenceOutcome folds a completed run into the aggregate verdict,
// the coverage it represents and the message that explains it.
//
// The all-Skipped test comes FIRST and is not the aggregate's job: the existing
// fold treats Skipped as informational, which is right for a mixed run and
// wrong for a run that evaluated nothing. contracts/runtime.md's clarification
// is explicit that such a run is "Skipped and NoChecksEvaluated coverage" with
// the message "no checks evaluated", and data-model.md that "Zero enabled
// checks follows the engine's explicit Skipped sentinel" — so an empty run and
// an all-Skipped run are the same row.
func addonCheckEvidenceOutcome(check *fathomv1alpha1.AddonCheck, result adapter.Result) (
	fathomv1alpha1.HealthReportResult, fathomv1alpha1.AddonCheckEvidenceCoverage, string,
) {
	if addonCheckEvaluatedNothing(result.Checks) {
		return fathomv1alpha1.HealthReportResultSkipped,
			fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated,
			fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage
	}
	// The existing family-aware aggregate, unchanged: "Mixed results use
	// existing aggregate semantics" and "Ratio aggregation cannot average a
	// failed family into Pass."
	aggregate, _ := aggregateWithRatioRollups(result.Checks, ratioThresholdsByFamily(check))
	return aggregate, fathomv1alpha1.AddonCheckCoverageChecksEvaluated,
		fmt.Sprintf("%d checks evaluated", len(result.Checks))
}

// addonCheckEvaluatedNothing reports a completed run that produced no health
// observation at all: no checks, or every check Skipped.
func addonCheckEvaluatedNothing(checks []adapter.CheckResult) bool {
	for _, check := range checks {
		if check.Outcome != adapter.OutcomeSkipped {
			return false
		}
	}
	return true
}

// addonCheckCompletedVerdict narrows an aggregate to the completed-evidence
// enum. Error and Unknown are not evidence: the first says the run could not
// determine health and the second that nothing was observed, and presenting
// either as a stored verdict is how a partial result becomes a healthy-looking
// one.
func addonCheckCompletedVerdict(aggregate fathomv1alpha1.HealthReportResult) (
	fathomv1alpha1.AddonCheckEvidenceVerdict, bool,
) {
	switch aggregate {
	case fathomv1alpha1.HealthReportResultPass:
		return fathomv1alpha1.AddonCheckEvidenceVerdictPass, true
	case fathomv1alpha1.HealthReportResultWarn:
		return fathomv1alpha1.AddonCheckEvidenceVerdictWarn, true
	case fathomv1alpha1.HealthReportResultFail:
		return fathomv1alpha1.AddonCheckEvidenceVerdictFail, true
	case fathomv1alpha1.HealthReportResultSkipped:
		return fathomv1alpha1.AddonCheckEvidenceVerdictSkipped, true
	default:
		return "", false
	}
}

// addonCheckEvidenceRevision copies the fenced revision and publication
// provenance onto the evidence, bounded to the schema.
func addonCheckEvidenceRevision(fence RuntimeFence) fathomv1alpha1.AddonCheckEvidenceRevision {
	return fathomv1alpha1.AddonCheckEvidenceRevision{
		DefinitionUID:        boundedText(string(fence.Revision.DefinitionUID), addonCheckStatusIdentifierLimit),
		DefinitionGeneration: fence.Revision.Generation,
		SchemaVersion:        boundedText(fence.Revision.SchemaVersion, addonCheckStatusVersionLimit),
		SemanticsVersion:     fence.Revision.SemanticsVersion,
		AdapterVersion:       boundedText(fence.Provenance.AdapterVersion, addonCheckStatusIdentifierLimit),
		OperatorBuild:        boundedText(fence.Provenance.OperatorBuild, addonCheckStatusIdentifierLimit),
	}
}

// addonCheckEvidenceAuthority copies the fenced authority and policy context
// onto the evidence, bounded to the schema.
func addonCheckEvidenceAuthority(fence RuntimeFence) fathomv1alpha1.AddonCheckEvidenceAuthority {
	return fathomv1alpha1.AddonCheckEvidenceAuthority{
		BindingUID:        boundedText(string(fence.BindingUID), addonCheckStatusIdentifierLimit),
		BindingGeneration: fence.BindingSpecGeneration,
		ServiceAccountUID: boundedText(string(fence.ServiceAccountUID), addonCheckStatusIdentifierLimit),
		CheckUID:          boundedText(string(fence.CheckUID), addonCheckStatusIdentifierLimit),
		CheckGeneration:   fence.CheckGeneration,
		PolicyDigest:      boundedText(fence.CheckPolicy, addonCheckStatusIdentifierLimit),
		LeaderEpoch:       fence.Epoch.DeepCopy(),
	}
}

// AddonCheckEvidenceWindow is the age at which completed evidence stops being
// Current: two EFFECTIVE intervals plus one EFFECTIVE timeout (data-model.md,
// "Freshness is Current only for eligible input aged at most two effective
// intervals plus timeout").
//
// Effective, not declared: a stored sub-floor cadence is clamped and a missing
// one defaults, exactly as the run itself is paced, so the window always
// describes the schedule the check actually runs on. Two intervals means one
// missed run is tolerated; the timeout covers a run that is still in flight.
func AddonCheckEvidenceWindow(check *fathomv1alpha1.AddonCheck) time.Duration {
	return 2*addonCheckInterval(check) + addonCheckTimeout(check)
}

// AddonCheckEvidenceAged reports whether the completed evidence stored on check
// has aged past [AddonCheckEvidenceWindow] as of now. The bound is INCLUSIVE:
// evidence exactly at the window is still Current, because the contract says
// "aged at most two effective intervals plus timeout".
//
// Evidence that does not exist is not aged — its absence is Unavailable and an
// Unknown verdict, which is a different statement from "this observation got
// old". It is exported so the HealthCheck mirror can re-derive staleness at
// read time rather than trusting a stored value that only advances when a
// reconcile happens to run.
func AddonCheckEvidenceAged(check *fathomv1alpha1.AddonCheck, now time.Time) bool {
	evidence := check.Status.LastSuccessfulEvaluation
	if evidence == nil {
		return false
	}
	return now.Sub(evidence.ObservedAt.Time) > AddonCheckEvidenceWindow(check)
}

// addonCheckEvidenceEligibility maps an attempt's contract reason onto what it
// says about the INPUTS the stored evidence came from, which is the half of
// freshness that time cannot answer.
//
// Current here means "the inputs are still eligible", not "the evidence is
// fresh" — the caller still applies the age test. An execution failure
// (deadline, budget, parser, transport) deliberately lands here: the matrix row
// "Deadline, size, parser or read budget exhausted" names an attempt error and
// says nothing about the definition or binding, which are both still there. A
// run that cannot complete does not make its inputs ineligible; it just fails
// to refresh the observation, and the age test then tells the truth on its own.
func addonCheckEvidenceEligibility(reason string) fathomv1alpha1.AddonCheckEvidenceFreshness {
	switch reason {
	case reasonUnknownAddonType, registry.ReasonBuiltinCollision,
		registry.ReasonRuntimeCollision, registry.ReasonAdmissionClosed:
		// No dispatchable snapshot claims this identity at all — the matrix's
		// "Missing definition" and "New built-in collides" rows.
		return fathomv1alpha1.AddonCheckEvidenceUnavailable
	}
	switch publicationRankOf(reason) {
	case rankAuthority, rankInvalidInput:
		// "Old evidence retained with original time/revision/context;
		// freshness=Unavailable" — the revoked, denied, deleted and
		// invalid-edit rows.
		return fathomv1alpha1.AddonCheckEvidenceUnavailable
	case rankSuperseded:
		return fathomv1alpha1.AddonCheckEvidenceSuperseded
	default:
		return fathomv1alpha1.AddonCheckEvidenceCurrent
	}
}

// setAddonCheckFreshness derives and stores the freshness of whatever evidence
// check currently carries. It reads the evidence and never writes it.
func setAddonCheckFreshness(check *fathomv1alpha1.AddonCheck, reason string, now time.Time) {
	freshness, explanation := addonCheckFreshness(check, reason, now)
	check.Status.EvidenceFreshness = freshness
	check.Status.EvidenceFreshnessReason = boundedText(explanation, addonCheckStatusTextLimit)
}

// addonCheckFreshness answers two independent questions in one value, in the
// order the contract asks them: are the inputs this evidence came from still
// eligible, and is the observation still recent?
//
// Freshness is derived, never authority, and it never implies health:
// "Freshness denotes recency ... neither means Pass", so a Stale Pass and a
// Current Fail are both ordinary.
func addonCheckFreshness(check *fathomv1alpha1.AddonCheck, reason string, now time.Time) (
	fathomv1alpha1.AddonCheckEvidenceFreshness, string,
) {
	evidence := check.Status.LastSuccessfulEvaluation
	if evidence == nil {
		// "Unknown applies when no evidence exists": there is nothing to be
		// fresh, and no verdict is invented for it.
		return fathomv1alpha1.AddonCheckEvidenceUnavailable,
			"no completed evaluation has been recorded for this check"
	}
	switch eligibility := addonCheckEvidenceEligibility(reason); eligibility {
	case fathomv1alpha1.AddonCheckEvidenceUnavailable:
		return eligibility, fmt.Sprintf(
			"the inputs this evidence was produced from are no longer eligible (%s); it is retained with its original observation time %s",
			reason, evidence.ObservedAt.UTC().Format(time.RFC3339))
	case fathomv1alpha1.AddonCheckEvidenceSuperseded:
		return eligibility, fmt.Sprintf(
			"the revision or context this evidence was produced under has been replaced (%s); it is retained with its original observation time %s",
			reason, evidence.ObservedAt.UTC().Format(time.RFC3339))
	}
	if AddonCheckEvidenceAged(check, now) {
		return fathomv1alpha1.AddonCheckEvidenceStale, fmt.Sprintf(
			"observed at %s, more than two intervals plus one timeout (%s) ago; the stored verdict is not current coverage",
			evidence.ObservedAt.UTC().Format(time.RFC3339), AddonCheckEvidenceWindow(check))
	}
	return fathomv1alpha1.AddonCheckEvidenceCurrent, ""
}

// boundedText bounds a status string to a schema limit in BOTH units the
// contract names: "Strings ≤1,024 UTF-8 bytes and 1,024 code points" and
// "String character bounds do not replace UTF-8 byte bounds."
//
// Cutting at a rune boundary at or before the byte limit satisfies both at
// once, because a code point is at least one byte — and it is the only cut that
// leaves valid UTF-8, which the API server requires.
func boundedText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

// now is the runner's clock.
func (r *AddonCheckRuntimeRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// discarded reports a run that never reached publication. The run's own
// recorded cause wins over the error the caller observed, because a read that
// failed because the budget was already spent describes the symptom, not the
// cause — contracts/runtime.md: "Deadline wins over later response-limit errors
// after cancellation."
func (r *AddonCheckRuntimeRunner) discarded(budget *execution.Budget, err error) RuntimeAttempt {
	cause := budget.Err()
	if cause == nil {
		cause = err
	}
	candidate := candidateFor(cause)
	return RuntimeAttempt{Reason: candidate.reason, Message: candidate.message, Requeue: runtimeFenceRequeue}
}

// refused reports an outcome decided before any run budget existed.
func refused(reason, message string) RuntimeAttempt {
	return RuntimeAttempt{Reason: reason, Message: message, Requeue: runtimeFenceRequeue}
}

// candidateFor classifies an execution or control-plane error into the
// precedence order.
func candidateFor(err error) publicationCandidate {
	reason := runtimeFailureReason(err)
	return publicationCandidate{rank: publicationRankOf(reason), reason: reason, message: err.Error()}
}

// runtimeFailureReason extracts the contract reason an error carries. Every
// failure raised by the runtime packages and by pkg/addondefinition is either a
// typed *execution.Failure, a context cause, or a string prefixed with its
// reason; anything else is an unattributed execution failure.
func runtimeFailureReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return reasonRuntimeTimeout
	}
	var failure *execution.Failure
	if errors.As(err, &failure) {
		return failure.Reason
	}
	var authority *authorityFailure
	if errors.As(err, &authority) {
		return authority.Reason
	}
	if errors.Is(err, context.Canceled) {
		return reasonRunCanceled
	}
	if reason, ok := reasonPrefix(err.Error()); ok {
		return reason
	}
	return "ExecutionFailed"
}

// reasonPrefix reads a "Reason: detail" prefix. pkg/addondefinition and
// internal/adapter/impersonation attribute every failure that way, so relaying
// the reason keeps a single spelling between the packages that raise a
// condition and the status an administrator reads.
func reasonPrefix(message string) (string, bool) {
	for i, r := range message {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
			continue
		case r == ':' && i > 0:
			return message[:i], true
		default:
			return "", false
		}
	}
	return "", false
}

// addonCheckPolicyFingerprint is the captured spec.policy context. The policy
// selects which families execute, so evidence produced under one policy is not
// interchangeable with evidence produced under another even at the same
// generation. A digest is used rather than the policy itself so the fence stays
// a small comparable value whatever the policy's size.
func addonCheckPolicyFingerprint(check *fathomv1alpha1.AddonCheck) string {
	if check == nil || len(check.Spec.Policy) == 0 {
		return ""
	}
	// encoding/json sorts map keys, so the encoding is canonical.
	encoded, err := json.Marshal(check.Spec.Policy)
	if err != nil {
		// An unmarshalable policy cannot be fenced, so it must never compare
		// equal to anything, including itself.
		return fmt.Sprintf("unencodable-policy-%v", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// hold runs the barrier seam if a test installed one.
func (r *AddonCheckRuntimeRunner) hold(ctx context.Context, phase string) {
	if r.barrier != nil {
		r.barrier(ctx, phase)
	}
}
