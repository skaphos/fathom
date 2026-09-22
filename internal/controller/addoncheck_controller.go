/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/go-logr/logr"
	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/impersonation"
	"github.com/skaphos/fathom/internal/adapter/registry"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/internal/probe"
	"github.com/skaphos/fathom/pkg/adapter"
)

const (
	addonCheckConditionAccepted = "Accepted"
	addonCheckConditionPaused   = "Paused"
	addonCheckConditionReady    = "Ready"

	// addonCheckMaxConcurrentReconciles bounds how many AddonChecks reconcile in
	// parallel. Adapter Run is synchronous and may block up to spec.timeout
	// (probe pods, admission dry-runs, network I/O), so the default single
	// worker would serialize every check and let periodic runs slip past their
	// interval under load. A small pool keeps per-check cadence honest without
	// hammering the API server.
	addonCheckMaxConcurrentReconciles = 4

	// defaultHealthReportHistoryLimit matches the +kubebuilder:default=10 on
	// AddonCheckSpec.HistoryLimit. It is duplicated here so the reconciler can
	// fall back when an in-memory AddonCheck has not been round-tripped through
	// the API server (envtest fixtures, etc.).
	defaultHealthReportHistoryLimit = 10
)

type addonAdapterLookup interface {
	Lookup(addonType string) (adapter.Adapter, error)
}

// AddonCheckReconciler reconciles an AddonCheck object.
type AddonCheckReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Adapters addonAdapterLookup

	// ProbeImage is the operator-level default container image surfaced to
	// adapters that launch probe pods. Forwarded verbatim into adapter.Request.
	// Empty when no operator default is configured; adapters then fall back to
	// per-AddonCheck thresholds or their own hardcoded default.
	ProbeImage string

	// Tracer creates the per-Reconcile span. Optional: a nil Tracer falls back
	// to the global provider (a no-op unless tracing is enabled). The adapter
	// Run span nests under this reconcile span via the context.
	Tracer trace.Tracer

	// AddonClients builds the per-addon impersonating client handed to an adapter
	// as adapter.Request.Client, so the adapter reads under its own least-privilege
	// ServiceAccount rather than the operator's (SKA-58). Nil disables
	// impersonation — the operator client is used instead (unit tests, and local
	// out-of-cluster runs where the manager already uses a privileged kubeconfig).
	AddonClients impersonation.ClientFactory

	// Namespace is the operator's own namespace, where the per-addon
	// ServiceAccounts live. Populated from FATHOM_NAMESPACE (downward API) in
	// cluster. Empty is only valid out of cluster (or when AddonClients is nil):
	// in-cluster with an empty Namespace fails closed so adapters never run as
	// the operator SA (SKA-162).
	Namespace string

	// Recorder emits the Kubernetes Events contract (result transitions and
	// operational failures) on AddonCheck resources. Optional: nil disables
	// event recording; the check gauges are unaffected.
	Recorder events.EventRecorder

	// Runtime executes AddonChecks whose addon type is served by a published
	// runtime snapshot rather than by a built-in adapter (T047 of
	// specs/012-addon-definition-runtime).
	//
	// Nil is the DEFAULT-OFF case and it is load-bearing: with either Runtime
	// or RuntimeQueue nil this reconciler behaves exactly as it did before
	// runtime loading existed — no extra read, no extra branch taken, no
	// resolution beyond the built-in Lookup it has always performed.
	Runtime *AddonCheckRuntimeRunner

	// RuntimeQueue is the manager's single runtime worker pool queue.
	// *runtime.Scheduler implements it. It is separate from the built-in
	// reconcile workers on purpose: contracts/runtime.md's scheduling row
	// requires a "separate runtime worker pool/limiter ... built-ins retain
	// their workers", so a runtime-backed check is ENQUEUED here and executed
	// by RunRuntimeWork under that pool's ≤4-run, ≤1-per-definition,
	// ≤1-per-check admission rather than inline on a reconcile worker.
	RuntimeQueue RuntimeWorkQueue
}

// RuntimeWorkQueue is the runtime worker pool's queue surface, as much of
// *runtime.Scheduler as this reconciler may touch: it may ask for a check to be
// run and it may withdraw a queued wake, and it may do nothing else. Notably it
// cannot start workers — only the elected leadership session does that.
type RuntimeWorkQueue interface {
	// Enqueue asks for one run of work, deduplicated per check.
	Enqueue(work execution.Work, now time.Time) error
	// Forget withdraws a queued wake. It never cancels an active run: an
	// observed revocation cancels active work through the leadership session.
	Forget(check types.NamespacedName, now time.Time)
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks,verbs=get;list;watch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks/finalizers,verbs=update
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthreports,verbs=create;get;list;watch;delete
// All four check reconcilers record Kubernetes Events (result transitions and
// operational failures, skaphos/fathom#154) through the manager's shared
// EventRecorder; the grant lives once, here, because the markers aggregate
// into the single manager role. The manager's GetEventRecorder writes through
// the events.k8s.io/v1 API (k8s.io/client-go/tools/events), so the grant must
// cover that group — the core-group grant alone leaves every event rejected
// with "events.events.k8s.io is forbidden". Core stays granted for the legacy
// recorder paths (e.g. leader election's cluster-scope fallback).
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile resolves the AddonCheck's adapter and, when the check is due (first
// sight, a spec change, an elapsed interval, or a new run-now trigger), runs the
// adapter and records a HealthReport plus status. It requeues one interval out so
// the result tracks the addon's live state.
func (r *AddonCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	ctx, span := reconcilerTracer(r.Tracer).Start(ctx, "addoncheck.reconcile", trace.WithAttributes(
		attribute.String("fathom.kind", "AddonCheck"),
		attribute.String("fathom.namespace", req.Namespace),
		attribute.String("fathom.name", req.Name),
	))
	defer func() { endReconcileSpan(span, err) }()

	start := time.Now()
	defer func() {
		outcome := "success"
		if err != nil {
			outcome = "error"
		}
		metrics.RecordReconcile("AddonCheck", outcome, time.Since(start))
	}()

	log := logf.FromContext(ctx).WithValues("namespacedName", req.NamespacedName)

	var check fathomv1alpha1.AddonCheck
	if err := r.Get(ctx, req.NamespacedName, &check); err != nil {
		if apierrors.IsNotFound(err) {
			metrics.DeleteCheckSeries("AddonCheck", req.Namespace, req.Name)
			// A deleted check must not keep a queued runtime wake alive. Forget
			// withdraws the wake only; an active run keeps its slot and is
			// cancelled by the leadership session, never by this reconciler.
			r.forgetRuntimeWork(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	before := check.Status.DeepCopy()
	defer func() {
		observeCheck(r.Recorder, &check, "AddonCheck",
			fathomv1alpha1.HealthReportResult(before.LastResult), fathomv1alpha1.HealthReportResult(check.Status.LastResult),
			before.Conditions, check.Status.Conditions,
			check.Status.LastRunTime, addonCheckInterval(&check), err)
	}()
	previousObservedGeneration := check.Status.ObservedGeneration
	check.Status.ObservedGeneration = check.Generation

	pausedStatus := metav1.ConditionFalse
	pausedReason := "RunEnabled"
	pausedMessage := "AddonCheck is eligible for adapter execution."
	if check.Spec.Paused {
		pausedStatus = metav1.ConditionTrue
		pausedReason = "Paused"
		pausedMessage = "AddonCheck is paused; adapter execution is disabled."
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               addonCheckConditionPaused,
		Status:             pausedStatus,
		ObservedGeneration: check.Generation,
		Reason:             pausedReason,
		Message:            pausedMessage,
	})
	// Pausing a check withdraws any queued runtime wake: the pool would
	// otherwise still admit a run for a check that has been switched off.
	// Forget for a check the pool never heard of is a no-op, and the whole call
	// is a no-op while runtime loading is off.
	if check.Spec.Paused {
		r.forgetRuntimeWork(req.NamespacedName)
	}
	// A runtime-backed identity never goes through resolveAddonAdapter: that
	// path reports MissingAdapter for an identity no BUILT-IN claims, and it
	// would run a compiled runtime adapter through the built-in impersonation
	// path, under the per-addon ServiceAccount convention instead of the
	// binding's dedicated identity. runtimeBacked is false for every built-in
	// and for the whole default-off configuration, so the branch below is
	// exactly what this reconcile did before runtime loading existed.
	runtimeBacked := r.runtimeBacked(&check)
	var (
		selectedAdapter adapter.Adapter
		adapterReady    bool
	)
	if !runtimeBacked {
		selectedAdapter, adapterReady = resolveAddonAdapter(&check, r.Adapters)
	}

	// Validate spec.policy against the adapter that will run it, so a misconfig
	// is loud (Accepted=False) and gates the run, instead of being silently
	// ignored (unknown family) or only surfacing as a post-run Error (invalid
	// selector) -- SKA-54. Policy is validated only once an adapter is resolved,
	// since the valid family set is the adapter's; a paused or adapterless check
	// defers validation and is accepted structurally.
	//
	// Ready is given its final value here in a single write, rather than being
	// set True in resolveAddonAdapter and then flipped False for an invalid
	// policy: flipping the condition within one reconcile bumps its
	// LastTransitionTime every pass, which -- with no watch predicate -- would
	// re-trigger reconciliation forever for an invalid check.
	policyValid := true
	if adapterReady {
		policyErrs := validateAddonCheckPolicy(&check, selectedAdapter)
		policyValid = len(policyErrs) == 0
		setAddonCheckAccepted(&check, policyErrs)

		ready := metav1.Condition{
			Type:               addonCheckConditionReady,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: check.Generation,
			Reason:             "AdapterResolved",
			Message:            "AddonCheck has a registered adapter and is ready for execution.",
		}
		if !policyValid {
			ready.Status = metav1.ConditionFalse
			ready.Reason = "InvalidPolicy"
			ready.Message = "AddonCheck policy is invalid; adapter execution is skipped until it is corrected."
		}
		apiMeta.SetStatusCondition(&check.Status.Conditions, ready)
	} else if !runtimeBacked {
		// Runtime-backed checks publish Accepted from the worker after validating
		// the uncached, fenced check against the exact snapshot that would run.
		// Rewriting it from this cached reconcile would both misattribute policy
		// validation and churn an InvalidPolicy check between true and false.
		setAddonCheckAccepted(&check, nil)
	}

	interval := addonCheckInterval(&check)
	runNow, runNowDue := runTriggerDue(check.Annotations, check.Status.LastRunTrigger)
	switch {
	case runtimeBacked:
		// The same due-ness rule as a built-in — first sight, spec change,
		// run-now trigger, elapsed interval — but the run itself belongs to the
		// runtime pool, so this reconcile only asks for one. The pool admits it
		// only while the elected session is admissible, which is what keeps
		// runtime execution behind election, cache sync and the takeover grace.
		if addonCheckDueForRun(&check, previousObservedGeneration, runNowDue, interval) {
			r.enqueueRuntimeWork(log, &check)
			if runNow != "" {
				check.Status.LastRunTrigger = runNow
			}
		}
	case adapterReady && policyValid && addonCheckDueForRun(&check, previousObservedGeneration, runNowDue, interval):
		if err := r.runAddonCheck(ctx, log, &check, selectedAdapter); err != nil {
			return ctrl.Result{}, err
		}
		// Record the consumed trigger so the same annotation value does not
		// re-run the adapter on every subsequent reconcile. Only overwrite on a
		// non-empty value: a periodic/generation run with no run-now annotation
		// must not clear a previously consumed token (which would let that same
		// token fire again once re-applied).
		if runNow != "" {
			check.Status.LastRunTrigger = runNow
		}
	}

	// A ready AddonCheck is requeued one interval out so its HealthReport tracks
	// the addon's live state instead of freezing at first sight. This requeue
	// must survive the no-status-change fast path below, or periodic execution
	// stalls after the first run. Paused / adapterless checks are left to a
	// spec change (generation bump) to wake them.
	result = ctrl.Result{}
	if runtimeBacked || (adapterReady && policyValid) {
		result.RequeueAfter = interval
	}

	if equality.Semantic.DeepEqual(before, &check.Status) {
		return result, nil
	}
	if err := r.Status().Update(ctx, &check); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("updated AddonCheck status")

	return result, nil
}

// addonCheckDueForRun reports whether the adapter should run this reconcile: on
// first sight, on a spec (generation) change, when the on-demand run-now trigger
// is due (runTriggerDue), or when Interval has elapsed since the last run.
func addonCheckDueForRun(check *fathomv1alpha1.AddonCheck, previousObservedGeneration int64, triggerDue bool, interval time.Duration) bool {
	switch {
	case check.Status.LastRunTime == nil:
		return true
	case previousObservedGeneration != check.Generation:
		return true
	case triggerDue:
		return true
	default:
		return time.Since(check.Status.LastRunTime.Time) >= interval
	}
}

// builtinAdapterFor resolves addonType to a BUILT-IN adapter, and to nothing
// else. [registry.Registry.Lookup] resolves runtime snapshots too once dispatch
// is admitted, and this path — the built-in path — must never execute one: a
// compiled definition runs under its binding's dedicated identity and its own
// fences, not under the per-addon ServiceAccount convention built-ins use.
//
// The wired reconciler never reaches here for a runtime identity (runtimeBacked
// routes it to the pool first), so this is the belt to that suspenders: the two
// paths cannot silently disagree about who owns an identity. A registry that
// does not expose Resolve keeps the plain lookup it always had.
func builtinAdapterFor(adapters addonAdapterLookup, addonType string) (adapter.Adapter, error) {
	resolver, ok := adapters.(interface {
		Resolve(string) (registry.Resolution, error)
	})
	if !ok {
		return adapters.Lookup(addonType)
	}
	resolution, err := resolver.Resolve(addonType)
	switch {
	case err != nil:
		return nil, err
	case resolution.Runtime:
		return nil, fmt.Errorf("%w: %q is served by a runtime definition, not by a built-in adapter",
			registry.ErrNotFound, addonType)
	}
	return resolution.Adapter, nil
}

func resolveAddonAdapter(check *fathomv1alpha1.AddonCheck, adapters addonAdapterLookup) (adapter.Adapter, bool) {
	if check.Spec.Paused {
		apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
			Type:               addonCheckConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: check.Generation,
			Reason:             "Paused",
			Message:            "AddonCheck is paused; adapter execution is disabled.",
		})
		return nil, false
	}
	if adapters == nil {
		apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
			Type:               addonCheckConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: check.Generation,
			Reason:             "MissingAdapter",
			Message:            "No adapter registry is configured for AddonCheck reconciliation.",
		})
		return nil, false
	}
	selectedAdapter, err := builtinAdapterFor(adapters, check.Spec.AddonType)
	if err != nil {
		if errors.Is(err, registry.ErrNotFound) {
			apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
				Type:               addonCheckConditionReady,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: check.Generation,
				Reason:             "MissingAdapter",
				Message:            "No adapter is registered for addonType " + check.Spec.AddonType + ".",
			})
			return nil, false
		}
		apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
			Type:               addonCheckConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: check.Generation,
			Reason:             "AdapterLookupFailed",
			Message:            err.Error(),
		})
		return nil, false
	}
	// The resolved-and-ready Ready condition (True/AdapterResolved, or
	// False/InvalidPolicy) is set by the caller after policy validation, in one
	// write, so an invalid policy does not flip Ready True->False within a single
	// reconcile (which would churn LastTransitionTime and self-re-trigger).
	return selectedAdapter, true
}

func (r *AddonCheckReconciler) runAddonCheck(ctx context.Context, log logr.Logger, check *fathomv1alpha1.AddonCheck, selectedAdapter adapter.Adapter) error {
	timeout := addonCheckTimeout(check)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	started := time.Now()
	var (
		result adapter.Result
		runErr error
	)
	// Hand the adapter a client scoped to its own ServiceAccount via
	// impersonation, so it reads under least privilege (SKA-58). If the scoped
	// client cannot be built (e.g. the addon RBAC is not installed), surface that
	// as an adapter-level error rather than silently running with broader access.
	runClient, clientErr := r.adapterClient(runCtx, selectedAdapter.Name())
	if clientErr != nil {
		runErr = clientErr
	} else {
		result, runErr = selectedAdapter.Run(runCtx, adapter.Request{
			Client:     runClient,
			Logger:     log.WithValues("adapter", selectedAdapter.Name(), "addonType", check.Spec.AddonType),
			Target:     addonCheckTargetRef(check),
			Policy:     addonCheckPolicy(check),
			Timeout:    timeout,
			ProbeImage: r.ProbeImage,
		})
	}
	observedAt := metav1.NewTime(time.Now())
	if result.Duration == 0 {
		result.Duration = time.Since(started)
	}

	// fathom_adapter_run_duration_seconds is recorded by each adapter's Run()
	// per executed family — the adapter owns the accurate per-family outcome
	// and duration. The controller deliberately does not record it here;
	// doing so double-counts the histogram. See SKA-290 / SKA-504.

	report := healthReportForAddonCheck(check, selectedAdapter, result, observedAt, runErr)
	newResult := string(report.Spec.Result)

	// Persist a HealthReport only when the aggregate result changes (or on the
	// first run). Periodic re-runs that observe the same result refresh liveness
	// via LastRunTime without flooding history with identical reports: this keeps
	// HealthReport a record of state transitions (ADR-0002) and bounds etcd churn
	// now that execution is periodic. LastReportName therefore points at the
	// report capturing the current result; LastRunTime tracks the latest poll.
	resultChanged := check.Status.LastReportName == "" || newResult != check.Status.LastResult
	if resultChanged {
		useDeterministicHealthReportName(report, check.Name,
			"AddonCheck",
			string(check.UID),
			strconv.FormatInt(check.Generation, 10),
			check.Status.LastReportName,
			check.Status.LastResult,
			newResult,
		)
		if r.Scheme != nil {
			if err := controllerutil.SetControllerReference(check, report, r.Scheme); err != nil {
				return err
			}
		}
		persistedReport, created, err := createOrReuseHealthReport(ctx, r.Client, report)
		if err != nil {
			return err
		}
		if created {
			r.pruneHealthReportHistory(ctx, log, check)
		}
		observedAt = persistedReport.Spec.ObservedAt
		newResult = string(persistedReport.Spec.Result)
		check.Status.LastReportName = persistedReport.Name
	}

	check.Status.LastRunTime = &observedAt
	check.Status.LastResult = newResult
	check.Status.Absent = countAbsent(result.Checks)
	check.Status.DetectedVersion = result.DetectedVersion

	// An adapter Run error reports a genuine health condition (the adapter could
	// not determine state — e.g. the API server was unreachable), not a
	// controller malfunction. It is surfaced as an Error result/condition and
	// retried on the normal interval, never propagated as a reconcile error:
	// the interval already bounds retry frequency, and treating it as a reconcile
	// failure would spam error logs/metrics and delay recovery detection.
	readyStatus := metav1.ConditionTrue
	readyReason := "RunCompleted"
	readyMessage := "AddonCheck adapter run completed."
	if runErr != nil {
		readyStatus = metav1.ConditionFalse
		readyReason = "AdapterRunFailed"
		// A probe pod that never got running (image, RBAC, quota) is an
		// infrastructure fault with different remediation than the addon
		// failing its checks; the typed error keeps the distinction a contract
		// instead of string matching. The reason also drives the Warning event
		// observeCheck records for this condition.
		var launchErr *probe.LaunchError
		if errors.As(runErr, &launchErr) {
			readyReason = "ProbeLaunchFailed"
		}
		readyMessage = runErr.Error()
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               addonCheckConditionReady,
		Status:             readyStatus,
		ObservedGeneration: check.Generation,
		Reason:             readyReason,
		Message:            readyMessage,
	})
	return nil
}

// adapterClient returns the client an adapter Run should use. With impersonation
// configured (AddonClients set and Namespace known), it resolves the addon's
// ServiceAccount by the adapter.AddonLabel — prefix-agnostic, so it works under
// either the kustomize or Helm naming — and returns a client impersonating that
// ServiceAccount (SKA-58). It fails if the addon RBAC is not installed (no
// matching ServiceAccount) rather than falling back to broader access.
//
// Without impersonation configured (AddonClients nil), it returns the operator
// client: unit tests run unscoped.
//
// When AddonClients is set but Namespace is empty: out-of-cluster runs fall back
// to the operator client (privileged kubeconfig); in-cluster fails closed so a
// missing FATHOM_NAMESPACE cannot silently grant every adapter the operator SA's
// cluster-wide powers (SKA-162).
func (r *AddonCheckReconciler) adapterClient(ctx context.Context, addon string) (client.Client, error) {
	if r.AddonClients == nil {
		return r.Client, nil
	}
	if r.Namespace == "" {
		if impersonation.RunningInCluster() {
			return nil, impersonation.ErrNamespaceRequiredInCluster
		}
		// Out-of-cluster: manager already uses a privileged kubeconfig.
		return r.Client, nil
	}
	var sas corev1.ServiceAccountList
	if err := r.List(ctx, &sas,
		client.InNamespace(r.Namespace),
		client.MatchingLabels{adapter.AddonLabel: addon},
	); err != nil {
		return nil, fmt.Errorf("list scoped ServiceAccount for addon %q: %w", addon, err)
	}
	// Distinguish the two failure modes so the message points at the real cause:
	// 0 matches means the addon RBAC was never installed; >1 means several
	// installs (e.g. multiple Fathom releases) share the addon label in this
	// namespace. Both fail closed — never fall back to broader access.
	switch n := len(sas.Items); {
	case n == 0:
		return nil, fmt.Errorf(
			"no ServiceAccount labeled %s=%s in namespace %q; is the addon RBAC installed?",
			adapter.AddonLabel, addon, r.Namespace,
		)
	case n > 1:
		return nil, fmt.Errorf(
			"found %d ServiceAccounts labeled %s=%s in namespace %q, expected exactly one; multiple installs share the addon label",
			n, adapter.AddonLabel, addon, r.Namespace,
		)
	}
	return r.AddonClients.ClientFor(impersonation.SAUsername(r.Namespace, sas.Items[0].Name))
}

// runtimeBacked reports whether this check's addon type is served by a runtime
// definition rather than by a built-in adapter, and is therefore executed by
// the runtime pool instead of inline by this reconciler.
//
// The three ways it answers false are the three halves of the feature's safety
// argument:
//
//   - Runtime or RuntimeQueue nil — runtime loading is off (the default, and
//     every unit test that predates it). Nothing below this line is reached,
//     so a disabled runtime cannot change one built-in outcome.
//   - The registry resolves the identity to a BUILT-IN adapter. "Preserve
//     unrelated built-ins": a built-in is never routed through the runtime
//     pool, never gated by the runtime dispatch barrier, and keeps its own
//     reconcile workers.
//   - The addon type cannot name a definition at all. The scheduler keys work
//     by definition name, so an identity that is not a DNS-1123 label could
//     never be enqueued; it keeps the existing MissingAdapter answer rather
//     than disappearing into a queue that would refuse it.
//
// A paused check is likewise left to the built-in path, which is where the
// Paused condition is written — and its queued wake is withdrawn, because a
// paused check must not run.
//
// A barrier (BuiltinCollision, RuntimeCollision, RuntimeAdmissionClosed) and an
// entirely unknown identity both answer TRUE: those outcomes belong to the
// runtime path, which reports them with the lifecycle matrix's own reasons
// (contracts/runtime.md), and the reconciler has nothing truer to say.
func (r *AddonCheckReconciler) runtimeBacked(check *fathomv1alpha1.AddonCheck) bool {
	if r.Runtime == nil || r.RuntimeQueue == nil || r.Runtime.Registry == nil {
		return false
	}
	if check.Spec.Paused {
		return false
	}
	if len(validation.IsDNS1123Label(check.Spec.AddonType)) != 0 {
		return false
	}
	resolution, err := r.Runtime.Registry.Resolve(check.Spec.AddonType)
	return err != nil || resolution.Runtime
}

// enqueueRuntimeWork asks the shared runtime pool for one run of check. The
// queue deduplicates per check, so an event storm cannot multiply runs.
func (r *AddonCheckReconciler) enqueueRuntimeWork(log logr.Logger, check *fathomv1alpha1.AddonCheck) {
	work := execution.Work{Definition: check.Spec.AddonType, Check: client.ObjectKeyFromObject(check)}
	if err := r.RuntimeQueue.Enqueue(work, time.Now()); err != nil {
		// The queue validates the names it will key work by. A rejected enqueue
		// is a stored-input problem, not a controller malfunction, so it is
		// logged and retried on the next interval rather than raised as a
		// reconcile error that would back off this check's whole reconcile.
		log.V(1).Info("runtime AddonCheck was not enqueued", "addonType", check.Spec.AddonType, "reason", err.Error())
	}
}

// forgetRuntimeWork withdraws a queued runtime wake for key. It is a no-op
// unless the runtime pool is wired.
func (r *AddonCheckReconciler) forgetRuntimeWork(key types.NamespacedName) {
	if r.RuntimeQueue == nil {
		return
	}
	r.RuntimeQueue.Forget(key, time.Now())
}

// RunRuntimeWork is the runtime worker pool's handler: the production caller of
// AddonCheckRuntimeRunner.Run and of the transition/backfill path next door.
//
// It runs on a runtime pool worker, never on a reconcile worker, and one
// admission at a time per check. The order is fixed and each step is load
// bearing:
//
//  1. read the check to verify the queued work still applies;
//  2. run — the runner owns resolution, admission, both fences and publication;
//  3. use the exact PUBLISHED check and its previous evidence returned by the
//     runner; the manager cache may still hold an older status;
//  4. record the transition and persist the report name it chose.
//
// The disposition it returns is what paces the next attempt: Completed clears
// backoff, MissingInput polls at the contract's 60s, and everything else is the
// bounded exponential retry. Nothing here loops or waits on its own.
func (r *AddonCheckReconciler) RunRuntimeWork(ctx context.Context, work execution.Work) execution.Disposition {
	log := logf.FromContext(ctx).WithValues("namespacedName", work.Check, "addonType", work.Definition)
	if r.Runtime == nil {
		// Unreachable in production: the pool is started only by the wiring
		// that also sets this field. Refusing beats running an unwired path.
		log.Error(errRuntimeRunnerUnwired, "runtime work dispatched without a runner")
		return execution.MissingInput
	}

	var check fathomv1alpha1.AddonCheck
	if err := r.Get(ctx, work.Check, &check); err != nil {
		if apierrors.IsNotFound(err) {
			return execution.Completed
		}
		log.Error(err, "read AddonCheck for a runtime run")
		return execution.Retry
	}
	// The queue entry is keyed by the check, not by its spec, so an edit
	// between the enqueue and the admission can retarget or pause it. Neither
	// is this run's to execute.
	if check.Spec.Paused || check.Spec.AddonType != work.Definition {
		return execution.Completed
	}

	attempt, err := r.Runtime.Run(ctx, &check)
	if err != nil {
		if errors.Is(err, errNotRuntimeAddonType) {
			// The identity became a built-in (an upgrade registered it). The
			// built-in path owns it from the next reconcile on.
			return execution.Completed
		}
		log.Error(err, "runtime AddonCheck execution failed")
		return execution.Retry
	}

	if attempt.Completed && attempt.Published {
		if attempt.publication == nil {
			log.Error(errors.New("published runtime attempt has no status snapshot"), "record the runtime AddonCheck transition")
			return execution.Retry
		}
		published := attempt.publication
		name, err := r.recordRuntimeTransition(ctx, log, published, attempt.previousEvidence, attempt)
		if err != nil {
			log.Error(err, "record the runtime AddonCheck transition")
			return execution.Retry
		}
		if name != "" {
			// Second status write, by design: the report cannot be named
			// before it exists. See recordRuntimeTransition's "two status
			// writes" note — a lost write here is repaired by the backfill.
			if err := r.Status().Update(ctx, published); err != nil {
				log.Error(err, "persist the runtime AddonCheck report name")
				return execution.Retry
			}
		}
		return execution.Completed
	}

	return runtimeDisposition(attempt)
}

// runtimeDisposition maps a run that published no completed evidence onto the
// pool's pacing. The MissingInput reasons are the lifecycle rows whose recovery
// is an administrator action or another controller's publication — a definition
// that does not exist, authority that has not been granted, a contested
// identity, a gate no leader has opened. Polling those at the contract's
// missing-input interval is right; retrying them on an exponential ramp would
// hammer the API server over state that cannot change on its own.
func runtimeDisposition(attempt RuntimeAttempt) execution.Disposition {
	switch attempt.Reason {
	case reasonUnknownAddonType,
		reasonInvalidPolicy,
		reasonAuthorizationUnavailable,
		reasonAuthorizationRevoked,
		reasonDefinitionUnavailable,
		reasonBindingMismatch,
		reasonInvalidDefinition,
		reasonInvalidBinding,
		registry.ReasonBuiltinCollision,
		registry.ReasonRuntimeCollision,
		registry.ReasonAdmissionClosed:
		return execution.MissingInput
	default:
		return execution.Retry
	}
}

// recordRuntimeTransition adds the HealthReport for a COMPLETED runtime
// evaluation, and only when the completed verdict changed (T045 of
// specs/012-addon-definition-runtime).
//
// It is the runtime sibling of the report half of runAddonCheck, and it keeps
// that reconciler's rule verbatim: "Persist a HealthReport only when the
// aggregate result changes (or on the first run)" — contracts/runtime.md,
// "No-change verdicts do not create reports". A run that publishes new evidence
// under a NEW revision but the SAME verdict therefore updates status and adds
// nothing to history, which is spec.md acceptance 2.
//
// Three inputs are needed and none of them can be recovered from the others:
//
//   - check is the AddonCheck as the runner PUBLISHED it, carrying the new
//     evidence;
//   - previous is the evidence stored BEFORE that publication, which is the
//     only thing that can say whether the verdict moved (the published object
//     already shows the new verdict in both places);
//   - attempt says whether a completed run actually landed. A refused or lost
//     publication wrote nothing, so it may not be reported as a transition.
//
// The decision reads the evidence verdict rather than Status.LastResult
// because only completed evidence has a revision and an authority context to
// attribute a report to. A failed attempt preserves evidence by construction
// and therefore reports nothing: "Attempt Error still preserves previous
// completed evidence."
//
// It returns the name of the report now capturing the current verdict, or ""
// when the run was not a transition. The name is also written to
// check.Status.LastReportName in memory; persisting that status is the
// caller's.
//
// # Why a runtime transition costs TWO status writes
//
// The built-in path in runAddonCheck is ordered report-first / status-after, so
// one reconcile performs one status write. The runtime path cannot be: the
// runner publishes evidence under its own compare-and-swap before this function
// is reached, and a report cannot be named before it exists — so the caller
// writes status once at publication and once more to persist LastReportName.
// Between the two, status shows the new verdict beside a stale or empty
// lastReportName, and the process can simply stop there.
// A repeated same-verdict completion under that stale pointer reuses the
// existing report. If every pointer write fails across opposite verdict
// flaps, intermediate transitions can coalesce; history cannot claim a
// durable ordering that status never recorded.
//
// Creating the report BEFORE publication would collapse that to one write, and
// was rejected deliberately: the publication fence is what decides whether this
// run may speak for the check at all, and a report written ahead of it would
// attribute history to a run that authority, supersession or a lost lease may
// still refuse. The ordering stays; the intermediate state is made RECOVERABLE
// instead, by backfillRuntimeTransition.
func (r *AddonCheckReconciler) recordRuntimeTransition(
	ctx context.Context, log logr.Logger, check *fathomv1alpha1.AddonCheck,
	previous *fathomv1alpha1.AddonCheckEvidence, attempt RuntimeAttempt,
) (string, error) {
	if check == nil || !attempt.Completed || !attempt.Published {
		return "", nil
	}
	evidence := check.Status.LastSuccessfulEvaluation
	if evidence == nil {
		return "", nil
	}
	previousVerdict := ""
	if previous != nil {
		previousVerdict = string(previous.Verdict)
		if previous.Verdict == evidence.Verdict {
			// The verdict did not move against the evidence this run replaced.
			// That is normally the whole answer — "No-change verdicts do not
			// create reports" — but only while history already holds this
			// verdict. Ask what history actually has before staying silent.
			recorded, backfill, err := r.backfillRuntimeTransition(ctx, check, evidence.Verdict)
			if err != nil {
				return "", err
			}
			if !backfill {
				return "", nil
			}
			previousVerdict = recorded
		}
	}

	report := runtimeHealthReportForAddonCheck(check, evidence, attempt)
	// The durable history predecessor identifies the transition. If creating
	// the report succeeds but persisting LastReportName conflicts, a later
	// same-verdict run must reuse that report rather than create a duplicate
	// with its newer observation time. Once the pointer advances, a later
	// verdict flap receives a different predecessor and a distinct name.
	useDeterministicHealthReportName(report, check.Name,
		"AddonCheck",
		string(check.UID),
		check.Status.LastReportName,
		previousVerdict,
		string(evidence.Verdict),
	)
	if r.Scheme != nil {
		if err := controllerutil.SetControllerReference(check, report, r.Scheme); err != nil {
			return "", err
		}
	}
	persisted, created, err := createOrReuseHealthReport(ctx, r.Client, report)
	if err != nil {
		return "", err
	}
	if created {
		r.pruneHealthReportHistory(ctx, log, check)
	}
	check.Status.LastReportName = persisted.Name
	return persisted.Name, nil
}

// backfillRuntimeTransition decides whether a run whose verdict did NOT move
// must nevertheless write history, and which predecessor that entry records.
//
// Comparing the published verdict against the previously stored evidence
// silently assumes history holds whatever the last publication concluded. It
// does not. Evidence is published BEFORE the report is created, so a create
// that fails — a transient API error, a lost lease, a restarted process —
// leaves status showing the new verdict and history showing nothing. Every
// later run then compares the published verdict against itself, finds no
// change, and the transition is lost permanently: contracts/runtime.md requires
// a report on a verdict change, and that change would never be recorded.
//
// Status.LastReportName is the only durable record of what history holds, so
// the no-change decision is made against it rather than against evidence alone.
// This is the same backfill clause the built-in path has always carried
// (`check.Status.LastReportName == "" || newResult != check.Status.LastResult`
// in runAddonCheck); the runtime path needs it MORE, because it writes status
// first rather than last.
//
// It returns the verdict history currently records — the empty string when it
// records nothing — and whether a report must be written. A genuine no-change,
// where lastReportName names a report whose result already IS this verdict,
// writes nothing: that is the contract clause this repair must not break.
func (r *AddonCheckReconciler) backfillRuntimeTransition(
	ctx context.Context, check *fathomv1alpha1.AddonCheck,
	verdict fathomv1alpha1.AddonCheckEvidenceVerdict,
) (string, bool, error) {
	if check.Status.LastReportName == "" {
		// Nothing was ever recorded for this check, so the current verdict is
		// not in history at all — the transition that produced it was lost.
		return "", true, nil
	}
	var recorded fathomv1alpha1.HealthReport
	key := client.ObjectKey{Namespace: check.Namespace, Name: check.Status.LastReportName}
	if err := r.Get(ctx, key, &recorded); err != nil {
		if apierrors.IsNotFound(err) {
			// The named report is gone. Retention pruning deletes history on
			// purpose, so this is a deliberately bounded history rather than a
			// lost write: re-creating the entry would fight spec.historyLimit
			// on every poll.
			return "", false, nil
		}
		return "", false, err
	}
	if string(recorded.Spec.Result) == string(verdict) {
		return string(recorded.Spec.Result), false, nil
	}
	// The pointer names a report whose result contradicts the published
	// verdict, which is the same lost write seen one transition later: status
	// moved on, history did not. Record the move history can actually show,
	// from the entry it still names.
	return string(recorded.Spec.Result), true, nil
}

// runtimeHealthReportForAddonCheck renders the history entry for one completed
// runtime evaluation.
//
// The report's verdict is the PUBLISHED evidence verdict rather than a second,
// independently recomputed aggregate: status and history must not be able to
// disagree about what a run concluded. The per-check entries and the ratio
// rollups are produced by the existing shared helpers, unchanged, so a runtime
// report reads exactly like a built-in one — "Mixed results use existing
// aggregate semantics", including the reserved warnRatio/failRatio rollups.
//
// Every timestamp is the evidence's own observation, never the wall clock: a
// report is a record of when the observation happened, and re-dating it is the
// same mistake as renewing evidence on a failed attempt.
func runtimeHealthReportForAddonCheck(
	check *fathomv1alpha1.AddonCheck, evidence *fathomv1alpha1.AddonCheckEvidence, attempt RuntimeAttempt,
) *fathomv1alpha1.HealthReport {
	sourceRef := fathomv1alpha1.HealthReportTargetRef{
		APIVersion: fathomv1alpha1.GroupVersion.String(),
		Kind:       "AddonCheck",
		Namespace:  check.Namespace,
		Name:       check.Name,
	}
	observedAt := evidence.ObservedAt
	_, rollups := aggregateWithRatioRollups(attempt.Evidence.Checks, ratioThresholdsByFamily(check))

	var duration *metav1.Duration
	if attempt.Evidence.Duration > 0 {
		duration = &metav1.Duration{Duration: attempt.Evidence.Duration}
	}
	return &fathomv1alpha1.HealthReport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    check.Namespace,
			GenerateName: check.Name + "-",
			Labels: map[string]string{
				fathomv1alpha1.LabelHealthReportSourceKind: "AddonCheck",
				fathomv1alpha1.LabelHealthReportSourceName: check.Name,
			},
		},
		Spec: fathomv1alpha1.HealthReportSpec{
			SourceRef: sourceRef,
			AddonType: check.Spec.AddonType,
			// A compiled runtime adapter's identity IS the addon type, and its
			// version is the publication provenance the fence captured — taken
			// from the evidence rather than from a live registry lookup, which
			// could by now describe a different revision entirely.
			AdapterName:     check.Spec.AddonType,
			AdapterVersion:  evidence.Revision.AdapterVersion,
			DetectedVersion: attempt.Evidence.DetectedVersion,
			// Every runtime adapter is compiled by this operator build against
			// this binary's adapter contract, so this is the exact contract the
			// run used, not an assumption about someone else's adapter.
			ContractVersion: adapter.ContractVersion,
			Result:          fathomv1alpha1.HealthReportResult(evidence.Verdict),
			Checks: append(healthReportChecks(attempt.Evidence.Checks, observedAt),
				ratioRollupReportChecks(rollups, sourceRef, observedAt)...),
			ObservedAt: observedAt,
			Duration:   duration,
			Attribution: &fathomv1alpha1.HealthReportAttribution{
				Revision:  evidence.Revision,
				Authority: *evidence.Authority.DeepCopy(),
				Coverage:  evidence.Coverage,
				Message:   evidence.Message,
			},
		},
	}
}

// pruneHealthReportHistory enforces Spec.HistoryLimit by deleting the oldest
// HealthReports owned by check beyond the cap. Failures are logged but not
// returned: the user-facing write (the new HealthReport) already succeeded,
// and the next reconcile will retry the prune. The list query is indexed by
// the source-kind/name labels written in healthReportForAddonCheck.
func (r *AddonCheckReconciler) pruneHealthReportHistory(ctx context.Context, log logr.Logger, check *fathomv1alpha1.AddonCheck) {
	limit := defaultHealthReportHistoryLimit
	if check.Spec.HistoryLimit != nil {
		limit = int(*check.Spec.HistoryLimit)
	}
	if limit < 1 {
		// CRD validation rejects this, but defend in depth: a Minimum=1 below
		// would prune the just-created report and strand Status.LastReportName.
		return
	}

	var reports fathomv1alpha1.HealthReportList
	if err := r.List(ctx, &reports,
		client.InNamespace(check.Namespace),
		client.MatchingLabels{
			fathomv1alpha1.LabelHealthReportSourceKind: "AddonCheck",
			fathomv1alpha1.LabelHealthReportSourceName: check.Name,
		},
	); err != nil {
		log.Error(err, "list HealthReports for retention pruning failed; will retry on next reconcile")
		return
	}
	if len(reports.Items) <= limit {
		return
	}

	sort.Slice(reports.Items, func(i, j int) bool {
		return reports.Items[i].CreationTimestamp.Before(&reports.Items[j].CreationTimestamp)
	})
	excess := len(reports.Items) - limit
	for i := 0; i < excess; i++ {
		victim := &reports.Items[i]
		if err := r.Delete(ctx, victim); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "delete old HealthReport failed", "name", victim.Name)
			// Continue: a transient delete error on one report should not block pruning the rest.
		}
	}
	log.V(1).Info("pruned HealthReport history", "deleted", excess, "limit", limit)
}

func addonCheckTimeout(check *fathomv1alpha1.AddonCheck) time.Duration {
	if check.Spec.Timeout != nil && check.Spec.Timeout.Duration > 0 {
		return clampCadence(check.Spec.Timeout.Duration, fathomv1alpha1.MinCheckTimeout)
	}
	return fathomv1alpha1.DefaultAddonCheckTimeout
}

func addonCheckInterval(check *fathomv1alpha1.AddonCheck) time.Duration {
	if check.Spec.Interval != nil && check.Spec.Interval.Duration > 0 {
		return clampCadence(check.Spec.Interval.Duration, fathomv1alpha1.MinCheckInterval)
	}
	return fathomv1alpha1.DefaultAddonCheckInterval
}

// setAddonCheckAccepted records the Accepted condition from policy validation:
// True/SpecAccepted when policyErrs is empty, otherwise False/InvalidPolicy
// carrying the (deterministically ordered) list of problems. A stored
// sub-floor cadence (pre-floor object) downgrades a clean acceptance to
// True/SpecClamped naming the clamped fields — an invalid policy outranks the
// clamp notice because it stops the check entirely.
func setAddonCheckAccepted(check *fathomv1alpha1.AddonCheck, policyErrs []string) {
	cond := metav1.Condition{
		Type:               addonCheckConditionAccepted,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "SpecAccepted",
		Message:            "AddonCheck specification has been accepted for reconciliation.",
	}
	if len(policyErrs) > 0 {
		cond.Status = metav1.ConditionFalse
		cond.Reason = "InvalidPolicy"
		cond.Message = boundedText("AddonCheck policy is invalid: "+strings.Join(policyErrs, "; ")+".", addonCheckStatusTextLimit)
	} else if msgs := cadenceClampMessages(check.Spec.Interval, check.Spec.Timeout); len(msgs) > 0 {
		cond.Reason = conditionReasonSpecClamped
		cond.Message = strings.Join(msgs, "; ") + "."
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, cond)
}

// validateAddonCheckPolicy reports spec.policy misconfiguration the controller
// can detect without running the adapter: family keys the selected adapter does
// not advertise, and structurally-invalid label selectors. The returned
// problems are deterministically ordered (stable Accepted-condition message) and
// empty when the policy is valid.
//
// Family validation is skipped when selectedAdapter is nil (a paused or
// unregistered addonType), since the valid family set is not yet known; selector
// validation is adapter-independent and always runs. Threshold keys are
// validated only when the adapter implements [adapter.ThresholdAdvertiser]
// and advertises keys for the family.
//
// The reserved engine keys warnRatio and failRatio are the one exception to
// threshold values being adapter-private: they are parsed here, and rejected
// outright when the selected adapter predates contract
// [adapter.RatioThresholdsContractVersion]. Every other threshold value remains
// adapter-private and is never validated here.
func validateAddonCheckPolicy(check *fathomv1alpha1.AddonCheck, selectedAdapter adapter.Adapter) []string {
	if len(check.Spec.Policy) == 0 {
		return nil
	}
	var known map[adapter.Family]struct{}
	if selectedAdapter != nil {
		families := selectedAdapter.Capabilities().Families
		known = make(map[adapter.Family]struct{}, len(families))
		for _, f := range families {
			known[f] = struct{}{}
		}
	}

	var advertised map[adapter.Family][]string
	if ta, ok := selectedAdapter.(adapter.ThresholdAdvertiser); ok {
		advertised = ta.ThresholdKeys()
	}

	names := make([]string, 0, len(check.Spec.Policy))
	for family := range check.Spec.Policy {
		names = append(names, family)
	}
	sort.Strings(names)

	var problems []string
	for _, family := range names {
		if known != nil {
			if _, ok := known[adapter.Family(family)]; !ok {
				problems = append(problems, fmt.Sprintf("unknown family %q", family))
			}
		}
		if sel := check.Spec.Policy[family].LabelSelector; sel != nil {
			if _, err := metav1.LabelSelectorAsSelector(sel); err != nil {
				problems = append(problems, fmt.Sprintf("family %q has an invalid labelSelector: %v", family, err))
			}
		}
		thresholds := thresholdStringMap(check.Spec.Policy[family].Thresholds)
		_, warnRatioConfigured := thresholds[adapter.ThresholdKeyWarnRatio]
		_, failRatioConfigured := thresholds[adapter.ThresholdKeyFailRatio]
		ratioConfigured := warnRatioConfigured || failRatioConfigured
		if ratioConfigured && selectedAdapter != nil && !adapter.SupportsRatioThresholds(selectedAdapter.ContractVersion()) {
			problems = append(problems, fmt.Sprintf(
				"family %q configures engine ratio thresholds, but adapter %q uses contract version %s; warnRatio and failRatio require contract version %s or newer (rename legacy private keys and rebuild the adapter before using engine ratio thresholds)",
				family, selectedAdapter.Name(), selectedAdapter.ContractVersion(), adapter.RatioThresholdsContractVersion,
			))
		} else if _, err := adapter.ParseRatioThresholds(thresholds); err != nil {
			problems = append(problems, fmt.Sprintf("family %q has an invalid ratio threshold: %v", family, err))
		}
		problems = append(problems, unknownThresholdKeys(family, thresholds, advertised)...)
	}
	return problems
}

// unknownThresholdKeys reports policy threshold keys the adapter does not
// advertise for family. Validation is opt-in twice over: it runs only when the
// adapter implements ThresholdAdvertiser (advertised non-nil) and only for
// families it advertises keys for — an unadvertised family stays unvalidated
// rather than rejecting every key.
func unknownThresholdKeys(family string, thresholds map[string]string, advertised map[adapter.Family][]string) []string {
	if len(thresholds) == 0 || advertised == nil {
		return nil
	}
	keys, ok := advertised[adapter.Family(family)]
	if !ok {
		return nil
	}
	known := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		known[key] = struct{}{}
	}
	var problems []string
	for _, key := range slices.Sorted(maps.Keys(thresholds)) {
		// The reserved ratio keys are engine-level (#159): the controller's
		// rollup aggregation consumes them, so they are valid on every family
		// regardless of what the adapter advertises.
		if key == adapter.ThresholdKeyWarnRatio || key == adapter.ThresholdKeyFailRatio {
			continue
		}
		if _, ok := known[key]; !ok {
			problems = append(problems, fmt.Sprintf("family %q has an unknown threshold key %q", family, key))
		}
	}
	return problems
}

func addonCheckPolicy(check *fathomv1alpha1.AddonCheck) map[adapter.Family]adapter.FamilyPolicy {
	if len(check.Spec.Policy) == 0 {
		return nil
	}
	policy := make(map[adapter.Family]adapter.FamilyPolicy, len(check.Spec.Policy))
	for family, familyPolicy := range check.Spec.Policy {
		enabled := true
		if familyPolicy.Enabled != nil {
			enabled = *familyPolicy.Enabled
		}
		policy[adapter.Family(family)] = adapter.FamilyPolicy{
			Enabled:       enabled,
			Namespaces:    append([]string(nil), familyPolicy.Namespaces...),
			LabelSelector: familyPolicy.LabelSelector.DeepCopy(),
			Thresholds:    thresholdStringMap(familyPolicy.Thresholds),
		}
	}
	return policy
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// thresholdStringMap converts the API's bounded ThresholdValue map to the
// plain string map the adapter contract uses (pkg/adapter deliberately knows
// nothing about API schema bounds).
func thresholdStringMap(in map[string]fathomv1alpha1.ThresholdValue) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = string(v)
	}
	return out
}

func addonCheckTargetRef(check *fathomv1alpha1.AddonCheck) adapter.TargetRef {
	return adapter.TargetRef{
		APIVersion: fathomv1alpha1.GroupVersion.String(),
		Kind:       "AddonCheck",
		Namespace:  check.Namespace,
		Name:       check.Name,
	}
}

func healthReportForAddonCheck(check *fathomv1alpha1.AddonCheck, selectedAdapter adapter.Adapter, result adapter.Result, observedAt metav1.Time, runErr error) *fathomv1alpha1.HealthReport {
	sourceRef := fathomv1alpha1.HealthReportTargetRef{
		APIVersion: fathomv1alpha1.GroupVersion.String(),
		Kind:       "AddonCheck",
		Namespace:  check.Namespace,
		Name:       check.Name,
	}
	aggregate, rollups := aggregateWithRatioRollups(result.Checks, ratioThresholdsByFamily(check))
	if runErr != nil {
		aggregate = fathomv1alpha1.HealthReportResultError
	}
	duration := metav1.Duration{Duration: result.Duration}
	report := &fathomv1alpha1.HealthReport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    check.Namespace,
			GenerateName: check.Name + "-",
			Labels: map[string]string{
				fathomv1alpha1.LabelHealthReportSourceKind: "AddonCheck",
				fathomv1alpha1.LabelHealthReportSourceName: check.Name,
			},
		},
		Spec: fathomv1alpha1.HealthReportSpec{
			SourceRef:       sourceRef,
			AddonType:       check.Spec.AddonType,
			AdapterName:     selectedAdapter.Name(),
			AdapterVersion:  selectedAdapter.Version(),
			DetectedVersion: result.DetectedVersion,
			ContractVersion: selectedAdapter.ContractVersion(),
			Result:          aggregate,
			Checks:          append(healthReportChecks(result.Checks, observedAt), ratioRollupReportChecks(rollups, sourceRef, observedAt)...),
			ObservedAt:      observedAt,
			Duration:        &duration,
		},
	}
	if runErr != nil {
		report.Spec.Checks = append(report.Spec.Checks, fathomv1alpha1.HealthReportCheck{
			Family:     "adapter",
			Result:     fathomv1alpha1.HealthReportResultError,
			TargetRef:  report.Spec.SourceRef,
			Summary:    runErr.Error(),
			ObservedAt: observedAt,
		})
	}
	return report
}

func healthReportChecks(checks []adapter.CheckResult, fallbackObservedAt metav1.Time) []fathomv1alpha1.HealthReportCheck {
	if len(checks) == 0 {
		return nil
	}
	out := make([]fathomv1alpha1.HealthReportCheck, 0, len(checks))
	for _, check := range checks {
		observedAt := fallbackObservedAt
		if !check.ObservedAt.IsZero() {
			observedAt = metav1.NewTime(check.ObservedAt)
		}
		var duration *metav1.Duration
		if check.Duration > 0 {
			duration = &metav1.Duration{Duration: check.Duration}
		}
		out = append(out, fathomv1alpha1.HealthReportCheck{
			Family: string(check.Family),
			Result: healthReportResult(check.Outcome),
			TargetRef: fathomv1alpha1.HealthReportTargetRef{
				APIVersion: check.TargetRef.APIVersion,
				Kind:       check.TargetRef.Kind,
				Namespace:  check.TargetRef.Namespace,
				Name:       check.TargetRef.Name,
			},
			Summary:    check.Summary,
			Details:    copyStringMap(check.Details),
			ObservedAt: observedAt,
			Duration:   duration,
		})
	}
	return out
}

// aggregateHealthReportResult returns the worst-case Result across an
// adapter's per-check outcomes via the shared WorstResult fold. Skipped is
// informational there: a healthy adapter whose only non-Pass outcome is a
// NoMatchingObjects Skipped folds to Pass, not Skipped, so the addon's verdict
// no longer flaps as the first managed CR appears or disappears (#160). An
// empty checks slice (the adapter ran but produced no outcomes) folds to
// Skipped so the report still carries a signal.
func aggregateHealthReportResult(checks []adapter.CheckResult) fathomv1alpha1.HealthReportResult {
	results := make([]fathomv1alpha1.HealthReportResult, 0, len(checks))
	for _, check := range checks {
		results = append(results, healthReportResult(check.Outcome))
	}
	return fathomv1alpha1.WorstResult(results, false)
}

// familyRatioRollup pairs a ratio-evaluated family with its thresholds and
// computed rollup, for the synthetic report entry and the aggregate fold.
type familyRatioRollup struct {
	family     adapter.Family
	thresholds adapter.RatioThresholds
	rollup     adapter.RatioRollup
}

// ratioThresholdsByFamily extracts the families whose policy configures the
// reserved ratio keys (#159). Values that fail to parse are skipped here —
// they were already rejected loudly via the Accepted condition before the
// run, so this is only a defensive guard against racing spec updates.
func ratioThresholdsByFamily(check *fathomv1alpha1.AddonCheck) map[adapter.Family]adapter.RatioThresholds {
	var out map[adapter.Family]adapter.RatioThresholds
	for family, familyPolicy := range check.Spec.Policy {
		rt, err := adapter.ParseRatioThresholds(thresholdStringMap(familyPolicy.Thresholds))
		if err != nil || !rt.Configured() {
			continue
		}
		if out == nil {
			out = make(map[adapter.Family]adapter.RatioThresholds)
		}
		out[adapter.Family(family)] = rt
	}
	return out
}

// aggregateWithRatioRollups is the family-aware sibling of
// aggregateHealthReportResult (#159): families with configured ratio
// thresholds contribute a single ratio verdict to the worst-of fold instead
// of their raw per-check outcomes; every other check participates exactly as
// before. A ratio family whose run produced no evaluable population (only
// Skipped, and no Error) falls back to its raw outcomes so Skipped stays
// informational and an all-Skipped report still folds to Skipped, exactly as
// without thresholds. Rollups are returned in first-appearance order, which
// follows the adapter's deterministic emit order.
func aggregateWithRatioRollups(checks []adapter.CheckResult, ratios map[adapter.Family]adapter.RatioThresholds) (fathomv1alpha1.HealthReportResult, []familyRatioRollup) {
	if len(ratios) == 0 {
		return aggregateHealthReportResult(checks), nil
	}
	// One grouping pass so each ratio family is evaluated over its own slice
	// instead of FamilyRatioVerdict re-scanning the whole run per family.
	grouped := make(map[adapter.Family][]adapter.CheckResult, len(ratios))
	for _, check := range checks {
		if _, ok := ratios[check.Family]; ok {
			grouped[check.Family] = append(grouped[check.Family], check)
		}
	}
	computed := make(map[adapter.Family]adapter.RatioRollup, len(grouped))
	for family, familyChecks := range grouped {
		computed[family] = adapter.FamilyRatioVerdict(familyChecks, family, ratios[family])
	}

	results := make([]fathomv1alpha1.HealthReportResult, 0, len(checks))
	folded := make(map[adapter.Family]bool, len(computed))
	var rollups []familyRatioRollup
	for _, check := range checks {
		rt, ok := ratios[check.Family]
		if !ok {
			results = append(results, healthReportResult(check.Outcome))
			continue
		}
		rollup := computed[check.Family]
		if rollup.Population == 0 && rollup.Verdict != adapter.OutcomeError {
			results = append(results, healthReportResult(check.Outcome))
			continue
		}
		if !folded[check.Family] {
			folded[check.Family] = true
			rollups = append(rollups, familyRatioRollup{family: check.Family, thresholds: rt, rollup: rollup})
			results = append(results, healthReportResult(rollup.Verdict))
		}
	}
	return fathomv1alpha1.WorstResult(results, false), rollups
}

// Well-known Details keys of the synthetic ratio-rollup report entry. The
// "rollup" discriminator lets consumers filter rollup entries from
// per-resource ones; the counts and echoed thresholds make every ratio
// verdict explainable from the persisted report alone (FR-010).
const (
	detailRollup           = "rollup"
	detailRollupRatio      = "ratio"
	detailRollupPopulation = "population"
	detailRollupUnhealthy  = "unhealthy"
	detailRollupDegraded   = "degraded"
)

// ratioRollupReportChecks renders one synthetic HealthReportCheck per
// ratio-evaluated family, targeting the driving AddonCheck. Per-resource
// entries are never modified; these ride alongside them.
func ratioRollupReportChecks(rollups []familyRatioRollup, sourceRef fathomv1alpha1.HealthReportTargetRef, observedAt metav1.Time) []fathomv1alpha1.HealthReportCheck {
	if len(rollups) == 0 {
		return nil
	}
	out := make([]fathomv1alpha1.HealthReportCheck, 0, len(rollups))
	for _, fr := range rollups {
		details := map[string]string{
			detailRollup:           detailRollupRatio,
			detailRollupPopulation: strconv.Itoa(fr.rollup.Population),
			detailRollupUnhealthy:  strconv.Itoa(fr.rollup.Unhealthy),
			detailRollupDegraded:   strconv.Itoa(fr.rollup.Degraded),
		}
		if fr.thresholds.Warn != nil {
			details[adapter.ThresholdKeyWarnRatio] = fr.thresholds.Warn.String()
		}
		if fr.thresholds.Fail != nil {
			details[adapter.ThresholdKeyFailRatio] = fr.thresholds.Fail.String()
		}
		out = append(out, fathomv1alpha1.HealthReportCheck{
			Family:     string(fr.family),
			Result:     healthReportResult(fr.rollup.Verdict),
			TargetRef:  sourceRef,
			Summary:    ratioRollupSummary(fr),
			Details:    details,
			ObservedAt: observedAt,
		})
	}
	return out
}

// ratioRollupSummary is the human-readable provenance line for a rollup
// entry, e.g.
//
//	ratio rollup: 1 unhealthy, 3 degraded of 200 evaluated, failRatio 5, warnRatio 1 -> Pass
func ratioRollupSummary(fr familyRatioRollup) string {
	if fr.rollup.Verdict == adapter.OutcomeError {
		return fmt.Sprintf("ratio rollup: adapter errors in family %q; ratio not evaluated -> Error", fr.family)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "ratio rollup: %d unhealthy, %d degraded of %d evaluated",
		fr.rollup.Unhealthy, fr.rollup.Degraded, fr.rollup.Population)
	if fr.thresholds.Fail != nil {
		fmt.Fprintf(&b, ", failRatio %s", fr.thresholds.Fail)
	}
	if fr.thresholds.Warn != nil {
		fmt.Fprintf(&b, ", warnRatio %s", fr.thresholds.Warn)
	}
	fmt.Fprintf(&b, " -> %s", fr.rollup.Verdict)
	return b.String()
}

func healthReportResult(outcome adapter.Outcome) fathomv1alpha1.HealthReportResult {
	switch outcome {
	case adapter.OutcomePass:
		return fathomv1alpha1.HealthReportResultPass
	case adapter.OutcomeWarn:
		return fathomv1alpha1.HealthReportResultWarn
	case adapter.OutcomeFail:
		return fathomv1alpha1.HealthReportResultFail
	case adapter.OutcomeError:
		return fathomv1alpha1.HealthReportResultError
	case adapter.OutcomeSkipped:
		return fathomv1alpha1.HealthReportResultSkipped
	default:
		return fathomv1alpha1.HealthReportResultUnknown
	}
}

// countAbsent returns the number of checks whose target was reported not
// installed (carrying the adapter.DetailAbsent marker) — required-absent Fails
// and optional-absent Skips alike. It feeds AddonCheck.status.absent (SKA-526).
func countAbsent(checks []adapter.CheckResult) int32 {
	var n int32
	for _, c := range checks {
		if adapter.IsAbsent(c.Details) {
			n++
		}
	}
	return n
}

// SetupWithManager sets up the controller with the Manager.
func (r *AddonCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.AddonCheck{}).
		Named("addoncheck").
		WithOptions(controller.Options{MaxConcurrentReconciles: addonCheckMaxConcurrentReconciles}).
		Complete(r)
}
