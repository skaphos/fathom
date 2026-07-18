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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	corev1 "k8s.io/api/core/v1"

	"github.com/go-logr/logr"
	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/impersonation"
	"github.com/skaphos/fathom/internal/adapter/registry"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/pkg/adapter"
)

const (
	addonCheckConditionAccepted = "Accepted"
	addonCheckConditionPaused   = "Paused"
	addonCheckConditionReady    = "Ready"

	defaultAddonCheckTimeout = 30 * time.Second

	// defaultAddonCheckInterval is the cadence at which an AddonCheck's adapter
	// re-runs when Spec.Interval is unset. Periodic re-execution is what keeps a
	// HealthReport current: without it a check runs once and its result goes
	// stale the moment the underlying addon degrades.
	defaultAddonCheckInterval = 5 * time.Minute

	// annotationRunNow forces an immediate adapter run, out of band from the
	// interval, whenever its value changes. The controller records the consumed
	// value in Status.LastRunTrigger so a given trigger fires exactly once —
	// callers (the fathom CLI's on-demand run, SKA-45) must therefore write a fresh
	// value each time (e.g. a timestamp or nonce), not a constant.
	annotationRunNow = "fathom.skaphos.io/run-now"

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

	// labelHealthReportSourceKind/Name pin a HealthReport to the resource that
	// produced it. The pair is queried via MatchingLabels so retention pruning
	// can list reports for a given AddonCheck without scanning every report in
	// the namespace. Future specialized check kinds (DNSCheck, NodeHealthCheck,
	// etc.) reuse the same label scheme — kind disambiguates name collisions.
	labelHealthReportSourceKind = "fathom.skaphos.io/source-kind"
	labelHealthReportSourceName = "fathom.skaphos.io/source-name"
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
	// cluster. Empty disables impersonation (falls back to the operator client).
	Namespace string
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks/finalizers,verbs=update
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthreports,verbs=create;get;list;watch;delete

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
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	before := check.Status.DeepCopy()
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
	selectedAdapter, adapterReady := resolveAddonAdapter(&check, r.Adapters)

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
	} else {
		setAddonCheckAccepted(&check, nil)
	}

	interval := addonCheckInterval(&check)
	runNow := check.Annotations[annotationRunNow]
	if adapterReady && policyValid && addonCheckDueForRun(&check, previousObservedGeneration, runNow, interval) {
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
	if adapterReady && policyValid {
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
// carries a new value, or when Interval has elapsed since the last run.
func addonCheckDueForRun(check *fathomv1alpha1.AddonCheck, previousObservedGeneration int64, runNow string, interval time.Duration) bool {
	switch {
	case check.Status.LastRunTime == nil:
		return true
	case previousObservedGeneration != check.Generation:
		return true
	case runNow != "" && runNow != check.Status.LastRunTrigger:
		return true
	default:
		return time.Since(check.Status.LastRunTime.Time) >= interval
	}
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
	selectedAdapter, err := adapters.Lookup(check.Spec.AddonType)
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
// Without impersonation configured, it returns the operator client: unit tests
// and local out-of-cluster runs, where the manager already uses a privileged
// kubeconfig, run unscoped.
func (r *AddonCheckReconciler) adapterClient(ctx context.Context, addon string) (client.Client, error) {
	if r.AddonClients == nil || r.Namespace == "" {
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
			labelHealthReportSourceKind: "AddonCheck",
			labelHealthReportSourceName: check.Name,
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
		return check.Spec.Timeout.Duration
	}
	return defaultAddonCheckTimeout
}

func addonCheckInterval(check *fathomv1alpha1.AddonCheck) time.Duration {
	if check.Spec.Interval != nil && check.Spec.Interval.Duration > 0 {
		return check.Spec.Interval.Duration
	}
	return defaultAddonCheckInterval
}

// setAddonCheckAccepted records the Accepted condition from policy validation:
// True/SpecAccepted when policyErrs is empty, otherwise False/InvalidPolicy
// carrying the (deterministically ordered) list of problems.
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
		cond.Message = "AddonCheck policy is invalid: " + strings.Join(policyErrs, "; ") + "."
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
// and advertises keys for the family; threshold values remain adapter-private
// and are never validated here.
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
		problems = append(problems, unknownThresholdKeys(family, check.Spec.Policy[family].Thresholds, advertised)...)
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
			Thresholds:    copyStringMap(familyPolicy.Thresholds),
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

func addonCheckTargetRef(check *fathomv1alpha1.AddonCheck) adapter.TargetRef {
	return adapter.TargetRef{
		APIVersion: fathomv1alpha1.GroupVersion.String(),
		Kind:       "AddonCheck",
		Namespace:  check.Namespace,
		Name:       check.Name,
	}
}

func healthReportForAddonCheck(check *fathomv1alpha1.AddonCheck, selectedAdapter adapter.Adapter, result adapter.Result, observedAt metav1.Time, runErr error) *fathomv1alpha1.HealthReport {
	aggregate := aggregateHealthReportResult(result.Checks)
	if runErr != nil {
		aggregate = fathomv1alpha1.HealthReportResultError
	}
	duration := metav1.Duration{Duration: result.Duration}
	report := &fathomv1alpha1.HealthReport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    check.Namespace,
			GenerateName: check.Name + "-",
			Labels: map[string]string{
				labelHealthReportSourceKind: "AddonCheck",
				labelHealthReportSourceName: check.Name,
			},
		},
		Spec: fathomv1alpha1.HealthReportSpec{
			SourceRef: fathomv1alpha1.HealthReportTargetRef{
				APIVersion: fathomv1alpha1.GroupVersion.String(),
				Kind:       "AddonCheck",
				Namespace:  check.Namespace,
				Name:       check.Name,
			},
			AddonType:       check.Spec.AddonType,
			AdapterName:     selectedAdapter.Name(),
			AdapterVersion:  selectedAdapter.Version(),
			DetectedVersion: result.DetectedVersion,
			ContractVersion: selectedAdapter.ContractVersion(),
			Result:          aggregate,
			Checks:          healthReportChecks(result.Checks, observedAt),
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
// adapter's per-check outcomes via the shared HealthReportResult.Severity
// ranking. An empty checks slice means the adapter ran but produced no
// outcomes — surfaced as Skipped so the report carries some signal.
func aggregateHealthReportResult(checks []adapter.CheckResult) fathomv1alpha1.HealthReportResult {
	if len(checks) == 0 {
		return fathomv1alpha1.HealthReportResultSkipped
	}
	worst := fathomv1alpha1.HealthReportResultPass
	worstRank := worst.Severity()
	for _, check := range checks {
		r := healthReportResult(check.Outcome)
		if rank := r.Severity(); rank > worstRank {
			worst = r
			worstRank = rank
		}
	}
	return worst
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
