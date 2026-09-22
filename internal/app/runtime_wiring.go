/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/impersonation"
	"github.com/skaphos/fathom/internal/adapter/registry"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/internal/controller"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
)

// This file is T047: the wiring that connects the single DECIDER
// ([RuntimeLeadership]) to the single ENFORCEMENT POINT (the registry's runtime
// dispatch gate), and hangs the runtime reconcilers, the runtime worker pool
// and the per-run uncached fences off the manager's own lifecycle.
//
// Everything here is behind default-off configuration. [newRuntimeWiring]
// returns a nil wiring and a diagnostic reason unless runtime loading is
// enabled AND leader election is on AND an explicit operator namespace is
// configured AND this process can name its own service account. A nil wiring
// adds no controller, no watch, no index, no runnable and no API read: the
// operator that existed before this file is exactly the operator that runs.
//
// The gate is opened from ONE place — [runtimeDispatchGate.Start], with the
// elected session's holder identity — and it is closed by that runnable's
// deferred close on every exit path. The registry deliberately knows nothing
// about Leases, drain or leadership; keeping the decision in one type and the
// enforcement in another is what stops two gates from disagreeing.
//
// # Leadership loss
//
// contracts/leadership.md requires terminating on leadership loss with no
// reacquisition. controller-runtime v0.25 does not expose its OnStoppedLeading
// callback through manager.Options (the field is unexported and documented as
// test-only), so [RuntimeLeadership.OnStoppedLeading] is deliberately left
// unwired and the property is carried by the manager instead: losing the Lease
// makes Start return "leader election lost", which [Run] returns and the
// process exits on. The wiring here must not defeat that — every runnable below
// returns on context cancellation rather than swallowing it or restarting, and
// the session's own graceful stop closes admission and cancels in-flight work
// on the way out.

// runtimeLeaderRenewDeadline mirrors controller-runtime's default renew
// deadline. The resource lock's client timeout is derived from it, and
// client-go's own advice is that the timeout stay below the renew deadline so a
// hung request cannot cost the Lease.
const runtimeLeaderRenewDeadline = 10 * time.Second

// runtimeLeaseAdoptRetry paces re-reads of the leader Lease while the elected
// session has no epoch yet. The epoch is read once per session; a failure means
// the control plane is unavailable, and the gate stays closed until it is not.
const runtimeLeaseAdoptRetry = 5 * time.Second

// Diagnostics reported when runtime loading is configured but unavailable.
// contracts/leadership.md names the first two verbatim; the reason strings
// [Options.RuntimeLoadingDisabledReason] already returns are used unchanged
// rather than re-spelled here.
const reasonAuthorizationUnavailable = "AuthorizationUnavailable"

// managerTokenPath is the projected ServiceAccount token every in-cluster pod
// receives. It is a variable so tests can supply their own token.
var managerTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// runtimeWiring owns the process-wide runtime singletons: ONE leadership
// session and ONE worker pool, shared by both runtime reconcilers and the
// AddonCheck runtime runner. The adapter registry is shared too, but it is
// built by DefaultControllers because built-ins need it whether or not runtime
// loading is on.
type runtimeWiring struct {
	opts    Options
	session *RuntimeLeadership
	// scheduler is the runtime worker pool's queue and admission. Built-in
	// reconcilers keep their own workers and never touch it.
	scheduler *execution.Scheduler
	// managerServiceAccount is this process's own identity, which can never be
	// a dedicated runtime reader.
	managerServiceAccount string
	// operatorBuild attributes every published snapshot to the build that
	// compiled it.
	operatorBuild string

	// The runtime reconcilers, kept so the wiring that built them can be
	// inspected as a whole rather than only through the Setuppers it hands
	// back. They are nil until attach runs.
	definitions *controller.AddonDefinitionReconciler
	bindings    *controller.AddonDefinitionBindingReconciler
}

// newRuntimeWiring builds the runtime singletons, or returns (nil, reason) when
// runtime loading is unavailable. The reason is a diagnostic, never an error:
// "do not fail manager startup or disable built-ins".
func newRuntimeWiring(opts Options) (*runtimeWiring, string) {
	if reason := opts.RuntimeLoadingDisabledReason(); reason != "" {
		return nil, reason
	}
	// NewRuntimeLeadership re-checks the same three static prerequisites, so
	// this is the one construction that can still fail: a process that cannot
	// mint a unique holder identity cannot own a Lease.
	session, err := NewRuntimeLeadership(opts)
	if err != nil {
		return nil, fmt.Sprintf("%s: %v", reasonAuthorizationUnavailable, err)
	}
	account, err := managerServiceAccountName()
	if err != nil {
		return nil, fmt.Sprintf("%s: %v", reasonAuthorizationUnavailable, err)
	}
	return &runtimeWiring{
		opts:                  opts,
		session:               session,
		scheduler:             execution.NewScheduler(nil),
		managerServiceAccount: account,
		operatorBuild:         operatorVersion(),
	}, ""
}

// managerServiceAccountName resolves the ServiceAccount this process actually
// authenticates as, from the identity in its own projected token.
//
// It is read rather than configured because it must be the TRUTH: the name is
// used to refuse a binding that points a definition at the operator's own
// identity ("the manager service account is not dedicated"), and a configured
// value that drifted from reality would silently disable that refusal. The
// token is this process's own credential and is not trusted as an
// authorization — only as a statement of who the API server will see — so it is
// parsed, never verified.
func managerServiceAccountName() (string, error) {
	raw, err := os.ReadFile(managerTokenPath)
	if err != nil {
		return "", fmt.Errorf("read the operator's own service account token at %s: %w", managerTokenPath, err)
	}
	name, err := serviceAccountFromToken(string(raw))
	if err != nil {
		return "", err
	}
	return name, nil
}

// serviceAccountFromToken extracts the ServiceAccount name from a bound token's
// subject claim (system:serviceaccount:<namespace>:<name>).
func serviceAccountFromToken(token string) (string, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return "", errors.New("the operator's service account token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode the operator's service account token: %w", err)
	}
	var claims struct {
		Subject string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse the operator's service account token: %w", err)
	}
	const prefix = "system:serviceaccount:"
	if !strings.HasPrefix(claims.Subject, prefix) {
		return "", fmt.Errorf("the operator's token subject %q does not name a service account", claims.Subject)
	}
	fields := strings.Split(strings.TrimPrefix(claims.Subject, prefix), ":")
	if len(fields) != 2 || fields[1] == "" {
		return "", fmt.Errorf("the operator's token subject %q does not name a service account", claims.Subject)
	}
	return fields[1], nil
}

// leaderElectionLock builds the manager's Lease lock with THIS session's holder
// identity, explicitly in the configured operator namespace and under the
// configured election ID (contracts/leadership.md, "Activation and identity").
//
// Identity is the whole point. RuntimeLeadership.AdoptLease refuses any Lease
// not held by exactly its own identity, so without this the elected manager and
// the session that gates runtime admission would be two different holders and
// no epoch would ever be adopted — runtime would stay permanently closed.
func (w *runtimeWiring) leaderElectionLock(base *rest.Config) (resourcelock.Interface, error) {
	cfg := rest.CopyConfig(base)
	// Below the renew deadline, so one hung request cannot cost the Lease.
	cfg.Timeout = runtimeLeaderRenewDeadline / 2
	clients, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "leader-election"))
	if err != nil {
		return nil, fmt.Errorf("build the leader election client: %w", err)
	}
	lease := w.session.LeaseRef()
	return resourcelock.New(resourcelock.LeasesResourceLock, lease.Namespace, lease.Name,
		clients.CoreV1(), clients.CoordinationV1(),
		resourcelock.ResourceLockConfig{Identity: w.session.Identity()})
}

// attach wires the runtime path into mgr and returns the extra Setuppers the
// caller registers. It is the only place any of it is connected.
//
// adapterRegistry is the SHARED registry the built-in adapters are already
// registered in: one registry, one dispatch decision, for built-ins and runtime
// definitions alike. addonCheck is the existing reconciler, which gains the
// runner and the pool queue and is otherwise untouched.
func (w *runtimeWiring) attach(mgr ctrl.Manager, adapterRegistry *registry.Registry, addonCheck *controller.AddonCheckReconciler) ([]Setupper, error) {
	if mgr == nil || adapterRegistry == nil || addonCheck == nil {
		return nil, errors.New("runtime wiring requires a manager, the shared adapter registry and the AddonCheck reconciler")
	}

	// The informer sync gate. Without it the session refuses to start with
	// ErrCacheSyncUnwired rather than assuming caches it cannot observe, which
	// is deliberate: "No runtime execution until synchronized and directly
	// validated" must not be satisfiable by a no-op default.
	cacheSynced := mgr.GetCache().WaitForCacheSync
	w.session.WithCacheSync(cacheSynced)

	// The per-run clients. Control is the UNCACHED control-plane reader both
	// fences read through, built from the manager's own rest.Config and bound
	// to this run's budget; the evaluator factory refuses any other reader.
	runner := &controller.AddonCheckRuntimeRunner{
		Client:                mgr.GetClient(),
		Registry:              adapterRegistry,
		Session:               w.session,
		Clients:               w.runClients(mgr),
		OperatorNamespace:     w.opts.Namespace,
		ManagerServiceAccount: w.managerServiceAccount,
		ProbeImage:            w.opts.ProbeImage,
	}
	addonCheck.Runtime = runner
	addonCheck.RuntimeQueue = w.scheduler

	w.definitions = &controller.AddonDefinitionReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		Registry:              adapterRegistry,
		OperatorNamespace:     w.opts.Namespace,
		ManagerServiceAccount: w.managerServiceAccount,
		OperatorBuild:         w.operatorBuild,
		CacheSynced:           cacheSynced,
	}
	w.bindings = &controller.AddonDefinitionBindingReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		// Drain acknowledgement reads the binding and the Lease directly. The
		// manager's APIReader bypasses the informer cache by construction, so
		// the cache cannot answer for state a drain claims to have observed.
		APIReader:             mgr.GetAPIReader(),
		Leadership:            drainSession{w.session},
		Registry:              adapterRegistry,
		OperatorNamespace:     w.opts.Namespace,
		ManagerServiceAccount: w.managerServiceAccount,
		CacheSynced:           cacheSynced,
	}

	// Three leader-elected runnables, started by the manager's own lifecycle
	// rather than by a second election: the session itself, the gate it drives,
	// and the worker pool that may only run inside it.
	for _, runnable := range []manager.Runnable{
		w.session,
		&runtimeDispatchGate{session: w.session, registry: adapterRegistry, reader: mgr.GetAPIReader(), log: mgr.GetLogger().WithName("runtime-dispatch")},
		&runtimeWorkerPool{session: w.session, scheduler: w.scheduler, handle: addonCheck.RunRuntimeWork},
	} {
		if err := mgr.Add(runnable); err != nil {
			return nil, fmt.Errorf("add runtime runnable to manager: %w", err)
		}
	}

	return []Setupper{
		setupWithContext(w.definitions.SetupWithManager),
		setupWithContext(w.bindings.SetupWithManager),
	}, nil
}

// runClients builds one run's control reader and evaluator factory from the
// single outer budget, so the manager-identity fence reads and the delegated
// evaluator's reads share one deadline and one set of counters.
func (w *runtimeWiring) runClients(mgr ctrl.Manager) controller.RuntimeRunFactory {
	cfg, scheme := mgr.GetConfig(), mgr.GetScheme()
	return func(budget *execution.Budget, targets execution.ControlTargets) (controller.RuntimeRunClients, error) {
		reader, err := impersonation.NewRuntimeControlReader(cfg, budget, targets)
		if err != nil {
			return controller.RuntimeRunClients{}, err
		}
		// NewRuntimeControlReader returns the concrete uncached reader; the
		// assertion is what carries that fact into the typed seam, and the
		// factory below independently refuses anything else.
		control, ok := reader.(impersonation.RuntimeControlReader)
		if !ok {
			return controller.RuntimeRunClients{}, fmt.Errorf(
				"%s: the runtime control reader is not the uncached control-plane reader", reasonAuthorizationUnavailable)
		}
		evaluator, err := impersonation.NewRuntimeFactory(cfg, scheme, control, w.opts.Namespace, w.managerServiceAccount)
		if err != nil {
			return controller.RuntimeRunClients{}, err
		}
		return controller.RuntimeRunClients{Control: control, Evaluator: evaluator}, nil
	}
}

// drainSession adapts [RuntimeLeadership] to the reconciler's narrow view. Only
// AcknowledgeDrain needs adapting: its result type lives in this package, and
// internal/controller cannot import internal/app.
type drainSession struct{ *RuntimeLeadership }

func (s drainSession) AcknowledgeDrain(key string) (controller.DrainEvidence, error) {
	ack, err := s.RuntimeLeadership.AcknowledgeDrain(key)
	return controller.DrainEvidence{Epoch: ack.Epoch, Limitation: ack.Limitation}, err
}

var _ controller.RuntimeLeadershipSession = drainSession{}

// setupperFunc adapts the runtime reconcilers' SetupWithManager(ctx, mgr) to
// the package's Setupper interface.
type setupperFunc func(ctrl.Manager) error

func (f setupperFunc) SetupWithManager(mgr ctrl.Manager) error { return f(mgr) }

// setupWithContext supplies the setup context for index registration. It is
// separated so the ctx a caller wants is obvious at the call site.
func setupWithContext(setup func(context.Context, ctrl.Manager) error) Setupper {
	return setupperFunc(func(mgr ctrl.Manager) error { return setup(context.Background(), mgr) })
}

// runtimeDispatchGate is the ONE connection between the elected session and the
// registry's dispatch gate, and the only thing in the process that opens it.
//
// It starts only as a leader-elected runnable, waits for the session to become
// admissible (elected AND caches synchronized AND the monotonic takeover grace
// elapsed), adopts the live Lease so completed runs can be attributed to an
// epoch, and only then admits dispatch. Every exit — leadership loss, manager
// shutdown, a cancelled context — bars dispatch again, because closing needs no
// driver and is the safe direction.
type runtimeDispatchGate struct {
	session  *RuntimeLeadership
	registry *registry.Registry
	// reader is the manager's uncached APIReader. The Lease is read directly:
	// there is no Lease informer, and a cached read could not witness the
	// epoch this session is about to claim.
	reader client.Reader
	log    logr.Logger
	// retry paces Lease re-reads; tests shorten it.
	retry time.Duration
}

var (
	_ manager.Runnable               = (*runtimeDispatchGate)(nil)
	_ manager.LeaderElectionRunnable = (*runtimeDispatchGate)(nil)
)

// NeedLeaderElection keeps the gate out of the non-elected runnable group.
func (g *runtimeDispatchGate) NeedLeaderElection() bool { return true }

func (g *runtimeDispatchGate) Start(ctx context.Context) error {
	// Deferred first: nothing below may leave the gate admitted.
	defer g.registry.CloseRuntimeDispatch("the elected runtime leadership session ended")

	select {
	case <-ctx.Done():
		return nil
	case <-g.session.Ready():
	}

	epoch, err := g.adoptLease(ctx)
	if err != nil {
		// Fail closed and stay closed: an unattributable session must not
		// dispatch, but it must not take the operator's built-ins down with it.
		g.log.Error(err, "runtime dispatch stays closed: no live leader epoch was observed")
		return nil
	}
	if err := g.registry.OpenRuntimeDispatch(g.session.Identity()); err != nil {
		return err
	}
	g.log.Info("runtime dispatch admitted",
		"holderIdentity", epoch.HolderIdentity, "leaseUID", epoch.LeaseUID, "leaseTransitions", epoch.LeaseTransitions)

	<-ctx.Done()
	return nil
}

// adoptLease reads the configured Lease directly until the session adopts it or
// the session ends. AdoptLease refuses a Lease this process does not hold, so a
// stale or foreign Lease can never become this session's epoch.
func (g *runtimeDispatchGate) adoptLease(ctx context.Context) (*fathomv1alpha1.DefinitionLeaderEpoch, error) {
	retry := g.retry
	if retry <= 0 {
		retry = runtimeLeaseAdoptRetry
	}
	ref := g.session.LeaseRef()
	for {
		var lease coordinationv1.Lease
		read, cancel := context.WithTimeout(ctx, limits.MaxRequestDuration)
		err := g.reader.Get(read, ref, &lease)
		cancel()
		if err == nil {
			epoch, adoptErr := g.session.AdoptLease(&lease)
			if adoptErr == nil {
				return epoch, nil
			}
			err = adoptErr
			if errors.Is(adoptErr, ErrLeadershipEnded) || errors.Is(adoptErr, ErrLeadershipNotHeld) {
				return nil, adoptErr
			}
		}
		g.log.V(1).Info("the leader election Lease is not adoptable yet", "lease", ref.String(), "reason", err.Error())
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("the leadership session ended before its Lease was adopted: %w", err)
		case <-time.After(retry):
		}
	}
}

// runtimeWorkerPool starts the manager's single runtime worker pool, and only
// inside an admissible leadership session. Built-in controllers keep their own
// workers; this pool serves runtime definitions alone.
type runtimeWorkerPool struct {
	session   *RuntimeLeadership
	scheduler *execution.Scheduler
	handle    func(context.Context, execution.Work) execution.Disposition
}

var (
	_ manager.Runnable               = (*runtimeWorkerPool)(nil)
	_ manager.LeaderElectionRunnable = (*runtimeWorkerPool)(nil)
)

func (p *runtimeWorkerPool) NeedLeaderElection() bool { return true }

func (p *runtimeWorkerPool) Start(ctx context.Context) error {
	// Ready closes only after election, cache sync and the takeover grace. No
	// worker exists before it, so no runtime run can be admitted before it.
	select {
	case <-ctx.Done():
		return nil
	case <-p.session.Ready():
	}
	return p.scheduler.Run(ctx, p.handle)
}
