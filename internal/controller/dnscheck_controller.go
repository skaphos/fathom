/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
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

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/internal/probe"
)

const (
	// dnsCheckKind is the kind label used across the check gauges and events, so
	// DNSCheck appears in cluster-wide check dashboards without per-kind handling.
	dnsCheckKind = "DNSCheck"

	// dnsCheckMaxConcurrentReconciles bounds how many DNSChecks reconcile at
	// once. Together with the per-check probe cap it fixes the cluster-wide
	// ceiling on concurrent probe pods at
	// dnsCheckMaxConcurrentReconciles × Options.DNSCheck.MaxConcurrentProbes,
	// computable from configuration without observing a running system (FR-103a).
	dnsCheckMaxConcurrentReconciles = 4

	// Condition types. Accepted and Ready follow the shape every check kind uses.
	// Complete is specific to DNSCheck: it is the truncation signal (FR-106a).
	dnsCheckConditionAccepted = checkConditionAccepted
	dnsCheckConditionReady    = checkConditionReady
	dnsCheckConditionComplete = "Complete"

	// HealthReport provenance. DNSCheck is not an adapter, but the report schema
	// records which component produced the evidence, and a reader should be able
	// to tell a DNSCheck record from an adapter's.
	dnsCheckReportFamily      = "dns_resolution"
	dnsCheckReportAdapterName = "dnscheck"
	dnsCheckReportAdapterVer  = "0.1.0"
)

// dnsProbeRunner is the launcher seam. Production uses *probe.Launcher; tests
// substitute a fake so the reconcile loop is exercised without scheduling pods.
type dnsProbeRunner interface {
	Run(ctx context.Context, req probe.Request) (probe.Result, error)
}

// DNSCheckReconciler reconciles a DNSCheck object.
//
// Each run expands the specification into (target, vantage point) pairs, runs
// one probe pod per pair inside the check's OWN namespace (FR-031), folds the
// per-pair outcomes into a single verdict with the project-wide fold, and
// mirrors the result into status, metrics, events, and — only when the verdict
// changes — a HealthReport.
type DNSCheckReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// ProbeClient creates, polls, and deletes probe pods. It MUST be an
	// uncached client.
	//
	// The manager's own client serves structured reads from the shared informer
	// cache, and scopedCacheOptions() does not list Pod — so a single cached Pod
	// Get would start an unfiltered cluster-wide Pod informer and pull every pod
	// in the cluster into memory, the failure removed in #164/SKA-581. The
	// adapter path avoids this only incidentally, by handing adapters the
	// uncached impersonating client; DNSCheck has no per-addon identity to
	// impersonate, so it must be given an uncached client explicitly.
	ProbeClient client.Client

	// ProbeImage is the container image probe pods run.
	ProbeImage string

	// MaxConcurrentProbes bounds probe pods in flight for a single check
	// (FR-103a). Values below 1 are treated as 1; Options.Validate rejects them
	// at startup, so this is only a guard against a zero-valued struct in tests.
	MaxConcurrentProbes int

	// Launcher runs one probe pod to completion. Optional: nil builds a
	// probe.Launcher over ProbeClient. Tests inject a fake.
	Launcher dnsProbeRunner

	// MinPairBudget overrides the least time worth giving a pair before the run
	// stops dispatching. Mostly useful in tests, where the production floor
	// would make a truncation case take tens of seconds; production callers
	// should leave it zero.
	MinPairBudget time.Duration

	// Tracer creates the per-Reconcile span. Optional; nil falls back to the
	// global provider (a no-op unless tracing is enabled).
	Tracer trace.Tracer

	// Recorder emits the Events contract on DNSCheck resources. Optional; nil
	// disables events without affecting the gauges.
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=dnschecks,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=dnschecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=dnschecks/finalizers,verbs=update
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthreports,verbs=create;get;list;watch;delete
// A DNSCheck resolves from its OWN namespace (FR-031), so the operator must
// place a pod in a namespace it does not own. It cannot borrow the per-addon
// impersonation path: that identity lives in the operator's namespace and no
// equivalent exists for an arbitrary tenant namespace. The grant cannot be
// namespaced either — a DNSCheck may be created anywhere, and the set of
// namespaces is unknown when this manifest is rendered.
//
// create and get are added here; list and delete already exist for the orphan
// sweeper (internal/probe/sweeper.go). get is required because the launcher
// polls the pod it created. watch is deliberately NOT requested: polling needs
// none, and a cluster-wide Pod watch would expose far more than pod placement
// requires. Neither are pods/exec, pods/log, or pods/portforward.
//
// docs/reference/operator-rbac.md carries the full justification, and
// operator_rbac_doc_test.go fails if this marker and that page disagree.
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;get

// Reconcile evaluates a DNSCheck and mirrors the outcome into status, metrics,
// events, and history.
func (r *DNSCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	ctx, span := reconcilerTracer(r.Tracer).Start(ctx, "dnscheck.reconcile", trace.WithAttributes(
		attribute.String("fathom.kind", dnsCheckKind),
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
		metrics.RecordReconcile(dnsCheckKind, outcome, time.Since(start))
	}()

	log := logf.FromContext(ctx).WithValues("namespacedName", req.NamespacedName)

	var check fathomv1alpha1.DNSCheck
	if err := r.Get(ctx, req.NamespacedName, &check); err != nil {
		if apierrors.IsNotFound(err) {
			// The check is gone: withdraw everything it was asserting, both the
			// check-level series and every per-target one (FR-114).
			metrics.DeleteCheckSeries(dnsCheckKind, req.Namespace, req.Name)
			metrics.DeleteDNSCheckTargetSeries(req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Deferred immediately after the Get so every exit path — including error
	// returns — mirrors status into the gauges and records the Events contract.
	// The "previous" values come from the status as fetched, never from process
	// memory, which is what stops an operator restart firing a false transition.
	before := check.Status.DeepCopy()
	defer func() {
		observeCheck(r.Recorder, &check, dnsCheckKind,
			fathomv1alpha1.HealthReportResult(before.LastResult), fathomv1alpha1.HealthReportResult(check.Status.LastResult),
			before.Conditions, check.Status.Conditions,
			check.Status.LastRunTime, dnsCheckInterval(&check), err)
	}()

	check.Status.ObservedGeneration = check.Generation
	accepted := metav1.Condition{
		Type:               dnsCheckConditionAccepted,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "SpecAccepted",
		Message:            "DNSCheck specification has been accepted for reconciliation.",
	}
	// A stored sub-floor cadence runs clamped rather than rejected; the Accepted
	// condition says so and observeCheck turns the transition into an event.
	if msgs := cadenceClampMessages(check.Spec.Interval, check.Spec.Timeout); len(msgs) > 0 {
		accepted.Reason = conditionReasonSpecClamped
		accepted.Message = strings.Join(msgs, "; ") + "."
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, accepted)

	interval := dnsCheckInterval(&check)
	startedAt := time.Now()
	outcomes := r.evaluate(ctx, &check, dnsCheckRunBound(&check), startedAt)

	verdict := foldDNSVerdict(outcomes)
	unreached := countUnreachedDNSPairs(outcomes)

	check.Status.LastResult = string(verdict)
	check.Status.Summary = summarizeDNSRun(outcomes, verdict, unreached)
	check.Status.TargetResults = dnsTargetResults(outcomes)
	check.Status.ObservedTargets = int32(len(outcomes)) //nolint:gosec // bounded at 48 by the schema
	observed := metav1.NewTime(startedAt)
	check.Status.LastRunTime = &observed

	r.setDNSCheckComplete(&check, len(outcomes), unreached)
	r.setDNSCheckReady(&check, outcomes)
	r.publishDNSTargetSeries(&check, outcomes)

	// Decided against the pre-run snapshot: the fields above already carry this
	// run's verdict, so comparing them to themselves would never see a change.
	if dnsCheckShouldPersistReport(before, verdict) {
		if err := r.persistDNSHealthReport(ctx, log, &check, outcomes, verdict, observed); err != nil {
			return ctrl.Result{}, err
		}
	}

	return r.finishDNSCheck(ctx, log, before, &check, interval, time.Since(startedAt))
}

// evaluate runs every pair the specification implies and returns one outcome
// each — always the full set, so a pair that never ran is reported rather than
// missing.
//
// Every pair is seeded Unknown before anything is launched. Whatever is still
// Unknown when the run ends is exactly what the run did not reach, so truncation
// needs no separate bookkeeping (FR-106), and the result set is rebuilt from the
// current specification every time, so a dropped pair cannot survive (FR-036).
func (r *DNSCheckReconciler) evaluate(ctx context.Context, check *fathomv1alpha1.DNSCheck, bound time.Duration, startedAt time.Time) []dnsPairOutcome {
	pairs := expandDNSPairs(&check.Spec)
	outcomes := make([]dnsPairOutcome, len(pairs))
	for i, pair := range pairs {
		outcomes[i] = dnsPairOutcome{
			pair: pair,
			result: fathomv1alpha1.DNSTargetResult{
				Name:       pair.Name,
				RecordType: pair.RecordType,
				Resolver:   pair.VantagePoint.Name,
				Result:     string(fathomv1alpha1.HealthReportResultUnknown),
			},
		}
	}
	if len(pairs) == 0 {
		return outcomes
	}

	// One deadline for the whole run (FR-104). Because the bound never exceeds
	// the cadence (FR-104a), a run cannot outlast the tick that scheduled it.
	runCtx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	deadline := startedAt.Add(bound)

	concurrency := r.concurrency()
	owner := dnsCheckOwnerReference(check)
	runID := strconv.FormatInt(startedAt.UnixNano(), 36)

	// A plain errgroup, deliberately not errgroup.WithContext: that cancels the
	// group on the first error, which would let one pair's failure abort the
	// rest. Fault isolation between pairs is required (FR-103b), so every
	// goroutine returns nil and records its own outcome.
	var group errgroup.Group
	group.SetLimit(concurrency)

	// Pairs dispatched but not yet started. group.Go blocks once the limit is
	// reached, so a pair can sit queued for a long time — and sizing its budget
	// at dispatch would hand it a share of a budget that has since been spent.
	// Each goroutine therefore claims its slot and sizes itself at the moment it
	// actually begins, which is what makes the #150 property real rather than
	// merely intended.
	notStarted := int64(len(pairs))

	for i := range pairs {
		group.Go(func() error {
			// +1 because the count returned excludes this pair, which is about
			// to run and must be included in its own fair share.
			left := int(atomic.AddInt64(&notStarted, -1)) + 1
			budget := perPairBudget(time.Until(deadline), left, concurrency)
			if budget < r.minPairBudget() {
				// Too little left for a pod to schedule, pull, and answer.
				// Launching anyway would burn the remainder to produce an Error
				// that says nothing about DNS. This pair stays Unknown — the run
				// did not reach it, which is precisely what that means (FR-106).
				return nil
			}
			outcomes[i].result = r.runPair(runCtx, check, outcomes[i].pair, budget, owner, runID)
			return nil
		})
	}
	_ = group.Wait()
	return outcomes
}

// runPair evaluates a single (target, vantage point) pair in the check's OWN
// namespace (FR-031) and maps the probe's answer onto a reported result.
func (r *DNSCheckReconciler) runPair(
	ctx context.Context,
	check *fathomv1alpha1.DNSCheck,
	pair dnsPair,
	budget time.Duration,
	owner metav1.OwnerReference,
	runID string,
) fathomv1alpha1.DNSTargetResult {
	out := fathomv1alpha1.DNSTargetResult{
		Name:       pair.Name,
		RecordType: pair.RecordType,
		Resolver:   pair.VantagePoint.Name,
	}

	req := probe.Request{
		Name:      dnsProbePodName(check.Name, pair, runID),
		Namespace: check.Namespace,
		Image:     r.ProbeImage,
		Mode:      probe.ModeDNS,
		Target:    pair.Name,
		Timeout:   budget,

		RecordType:      string(pair.RecordType),
		ExpectedAnswers: pair.Expected,
		Absent:          pair.Absent,

		// Same namespace as the check, so the reference is legal and deleting
		// the check garbage-collects any pod still in flight (FR-114).
		OwnerReferences: []metav1.OwnerReference{owner},
	}
	switch pair.VantagePoint.From {
	case fathomv1alpha1.DNSResolverNode:
		req.DNSFrom = probe.DNSFromNode
	case fathomv1alpha1.DNSResolverExplicit:
		nameserver, addrErr := dnsNameserverAddress(pair.VantagePoint.Address)
		if addrErr != nil {
			// Caught here rather than at pod build: probe.Pod() does not validate
			// nameserver syntax, so this would otherwise surface as an opaque
			// API-server rejection naming a field the check's author never wrote.
			out.Result = string(fathomv1alpha1.HealthReportResultError)
			out.Message = truncateTargetMessage(addrErr.Error())
			return out
		}
		req.DNSFrom = probe.DNSFromExplicit
		req.DNSNameservers = []string{nameserver}
	default:
		req.DNSFrom = probe.DNSFromCluster
	}

	started := time.Now()
	result, err := r.launcher().Run(ctx, req)
	out.LatencyMillis = time.Since(started).Milliseconds()

	if err != nil {
		if ctx.Err() != nil {
			// The RUN's own deadline cut this pair off; the pair was started but
			// never got an answer. That is "not reached" (FR-106), not a fault.
			//
			// Reporting Error here would be actively harmful: Error outranks Fail,
			// so a check whose bound was merely too small would mask a genuine
			// resolution failure among the pairs that did complete — the exact
			// masking that ruled Error out as the truncation outcome in the first
			// place. Unknown degrades the verdict and still loses to a real Fail.
			out.Result = string(fathomv1alpha1.HealthReportResultUnknown)
			out.Message = "not evaluated: the run bound elapsed before this pair completed"
			return out
		}
		// The pair could not be *performed* — quota, admission, image pull, an
		// unschedulable node. A fault on Fathom's side, never a resolver's
		// answer (FR-105).
		out.Result = string(fathomv1alpha1.HealthReportResultError)
		out.Message = truncateTargetMessage(fmt.Sprintf("probe execution failed: %v", err))
		return out
	}

	out.Result = string(dnsResultFromProbeOutcome(result.Outcome))
	out.Message = truncateTargetMessage(result.Summary)
	out.Answers = splitProbeAnswers(result.Details["answers"])
	return out
}

// dnsResultFromProbeOutcome maps the probe's vocabulary onto the project's.
//
// The mapping is deliberately literal. The probe already draws the distinction
// FR-014 turns on — a resolver that does not answer under an absent assertion is
// reported Error ("absence cannot be proven"), not Pass — so the controller's
// only job is to carry that through unchanged. Re-deriving it here would be a
// second place for the two to disagree.
func dnsResultFromProbeOutcome(outcome probe.Outcome) fathomv1alpha1.HealthReportResult {
	switch outcome {
	case probe.OutcomePass:
		return fathomv1alpha1.HealthReportResultPass
	case probe.OutcomeFail:
		return fathomv1alpha1.HealthReportResultFail
	case probe.OutcomeError:
		return fathomv1alpha1.HealthReportResultError
	default:
		// An outcome this build does not recognise is not evidence of anything.
		return fathomv1alpha1.HealthReportResultUnknown
	}
}

// dnsTargetMessageMaxLen mirrors the MaxLength on DNSTargetResult.Message.
const dnsTargetMessageMaxLen = 512

func truncateTargetMessage(s string) string {
	if utf8.RuneCountInString(s) <= dnsTargetMessageMaxLen {
		return s
	}
	const ellipsis = "…"
	return string([]rune(s)[:dnsTargetMessageMaxLen-1]) + ellipsis
}

// dnsAnswersMaxItems mirrors the MaxItems on DNSTargetResult.Answers.
const dnsAnswersMaxItems = 16

// splitProbeAnswers parses the probe's comma-joined answer list and bounds it to
// what the schema accepts, so a record set larger than the cap cannot fail the
// status write.
func splitProbeAnswers(joined string) []string {
	if joined == "" {
		return nil
	}
	parts := strings.Split(joined, ",")
	answers := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		// Answers is a listType=set; a duplicate would be rejected on write.
		if _, dup := seen[value]; dup {
			continue
		}
		seen[value] = struct{}{}
		answers = append(answers, value)
		if len(answers) == dnsAnswersMaxItems {
			break
		}
	}
	if len(answers) == 0 {
		return nil
	}
	return answers
}

// dnsCheckOwnerReference makes probe pods dependents of the check. Legal only
// because FR-031 places them in the check's own namespace — a namespaced owner
// in another namespace reads as dangling and invites immediate GC.
func dnsCheckOwnerReference(check *fathomv1alpha1.DNSCheck) metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return metav1.OwnerReference{
		APIVersion:         fathomv1alpha1.GroupVersion.String(),
		Kind:               dnsCheckKind,
		Name:               check.Name,
		UID:                check.UID,
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}
}

// dnsProbePodName derives a pod name unique to one pair within one run.
//
// The run component matters: a deterministic per-pair name would collide with an
// orphan left by a crashed operator, and the collision would surface as an
// AlreadyExists Error on every run until the sweeper's minimum age elapsed —
// turning a one-off crash into sustained false failures. Orphans are still
// reaped by label (the sweeper) and by owner reference (check deletion).
func dnsProbePodName(checkName string, pair dnsPair, runID string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		pair.Name, string(pair.RecordType), pair.VantagePoint.Name, runID,
	}, "\x00")))
	suffix := hex.EncodeToString(sum[:])[:10]

	const maxPodNameLength = 63
	base := "fathom-dnscheck-" + checkName
	if maxBase := maxPodNameLength - len(suffix) - 1; len(base) > maxBase {
		base = base[:maxBase]
	}
	base = strings.Trim(base, "-.")
	if base == "" {
		base = "fathom-dnscheck"
	}
	return base + "-" + suffix
}

// setDNSCheckComplete records whether the run reached every pair it planned.
//
// This is the actionable half of truncation (FR-106a): the verdict degrading to
// Unknown says something is wrong, but only the count tells an operator their
// run bound was too small for the number of pairs they declared.
func (r *DNSCheckReconciler) setDNSCheckComplete(check *fathomv1alpha1.DNSCheck, planned, unreached int) {
	condition := metav1.Condition{
		Type:               dnsCheckConditionComplete,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "AllPairsEvaluated",
		Message:            fmt.Sprintf("All %d (target, vantage point) pairs were evaluated.", planned),
	}
	if unreached > 0 {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "RunTruncated"
		condition.Message = fmt.Sprintf(
			"%d of %d pairs were not reached before the run bound elapsed; raise spec.timeout (and spec.interval) or declare fewer targets.",
			unreached, planned)
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, condition)
}

// setDNSCheckReady reports whether Fathom could evaluate the check at all.
//
// Ready is about Fathom's ability to run the check, not about what the check
// found: a check whose every name legitimately fails to resolve is Ready=True
// with a Fail verdict. Only pairs that could not be *performed* degrade it.
func (r *DNSCheckReconciler) setDNSCheckReady(check *fathomv1alpha1.DNSCheck, outcomes []dnsPairOutcome) {
	var errored int
	for _, outcome := range outcomes {
		if outcome.result.Result == string(fathomv1alpha1.HealthReportResultError) {
			errored++
		}
	}
	condition := metav1.Condition{
		Type:               dnsCheckConditionReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "EvaluationSucceeded",
		Message:            "DNSCheck evaluated successfully.",
	}
	if errored > 0 {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "ProbeExecutionFailed"
		condition.Message = fmt.Sprintf("%d of %d pairs could not be evaluated.", errored, len(outcomes))
	}
	apiMeta.SetStatusCondition(&check.Status.Conditions, condition)
}

// publishDNSTargetSeries rebuilds the per-target gauge for this check: withdraw
// everything, then set the pairs the current specification declares.
//
// Rebuilding rather than diffing is what delivers FR-036 — a pair the
// specification dropped is simply never re-set, so its series disappears with no
// removal detection, and the behaviour survives an operator restart.
func (r *DNSCheckReconciler) publishDNSTargetSeries(check *fathomv1alpha1.DNSCheck, outcomes []dnsPairOutcome) {
	metrics.DeleteDNSCheckTargetSeries(check.Namespace, check.Name)
	for _, outcome := range outcomes {
		metrics.ObserveDNSTarget(check.Namespace, check.Name,
			outcome.result.Name, string(outcome.result.RecordType), outcome.result.Resolver, outcome.result.Result)
	}
}

// persistDNSHealthReport writes one HealthReport for a verdict transition and
// prunes the history back to the declared limit.
//
// The report's name is derived from content rather than generated, so a
// reconcile that is retried after a partial failure reuses the same object
// instead of minting a duplicate for one transition.
func (r *DNSCheckReconciler) persistDNSHealthReport(
	ctx context.Context,
	log logr.Logger,
	check *fathomv1alpha1.DNSCheck,
	outcomes []dnsPairOutcome,
	verdict fathomv1alpha1.HealthReportResult,
	observedAt metav1.Time,
) error {
	report := healthReportForDNSCheck(check, outcomes, verdict, observedAt)
	useDeterministicHealthReportName(report, check.Name,
		dnsCheckKind,
		string(check.UID),
		strconv.FormatInt(check.Generation, 10),
		check.Status.LastReportName,
		string(verdict),
		observedAt.UTC().Format(time.RFC3339Nano),
	)
	if r.Scheme != nil {
		if err := controllerutil.SetControllerReference(check, report, r.Scheme); err != nil {
			return err
		}
	}
	persisted, created, err := createOrReuseHealthReport(ctx, r.Client, report)
	if err != nil {
		return err
	}
	if created {
		r.pruneDNSHealthReports(ctx, log, check)
	}

	check.Status.LastReportName = persisted.Name
	check.Status.LastResult = string(persisted.Spec.Result)
	return nil
}

// healthReportForDNSCheck builds the history record: one entry per (target,
// vantage point) pair, plus the folded verdict.
func healthReportForDNSCheck(
	check *fathomv1alpha1.DNSCheck,
	outcomes []dnsPairOutcome,
	verdict fathomv1alpha1.HealthReportResult,
	observedAt metav1.Time,
) *fathomv1alpha1.HealthReport {
	checks := make([]fathomv1alpha1.HealthReportCheck, 0, len(outcomes))
	for _, outcome := range outcomes {
		details := map[string]string{
			"recordType": string(outcome.result.RecordType),
			"resolver":   outcome.result.Resolver,
		}
		if len(outcome.result.Answers) > 0 {
			details["answers"] = strings.Join(outcome.result.Answers, ",")
		}
		if outcome.result.LatencyMillis > 0 {
			details["latencyMillis"] = strconv.FormatInt(outcome.result.LatencyMillis, 10)
		}
		// Polarity is not recoverable from the result alone, and a reader of the
		// history needs it for the same reason the summary does (FR-021).
		if outcome.pair.Absent {
			details["assertion"] = "absent"
		}
		checks = append(checks, fathomv1alpha1.HealthReportCheck{
			Family:     dnsCheckReportFamily,
			Result:     fathomv1alpha1.HealthReportResult(outcome.result.Result),
			TargetRef:  fathomv1alpha1.HealthReportTargetRef{Kind: "DNSName", Name: outcome.result.Name},
			Summary:    outcome.result.Message,
			Details:    details,
			ObservedAt: observedAt,
		})
	}

	return &fathomv1alpha1.HealthReport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    check.Namespace,
			GenerateName: check.Name + "-",
			Labels: map[string]string{
				labelHealthReportSourceKind: dnsCheckKind,
				labelHealthReportSourceName: check.Name,
			},
		},
		Spec: fathomv1alpha1.HealthReportSpec{
			SourceRef: fathomv1alpha1.HealthReportTargetRef{
				APIVersion: fathomv1alpha1.GroupVersion.String(),
				Kind:       dnsCheckKind,
				Namespace:  check.Namespace,
				Name:       check.Name,
			},
			AdapterName:    dnsCheckReportAdapterName,
			AdapterVersion: dnsCheckReportAdapterVer,
			Result:         verdict,
			Checks:         checks,
			ObservedAt:     observedAt,
		},
	}
}

// pruneDNSHealthReports enforces spec.historyLimit. Failures are logged rather
// than returned: the new report already landed, and the next run retries.
func (r *DNSCheckReconciler) pruneDNSHealthReports(ctx context.Context, log logr.Logger, check *fathomv1alpha1.DNSCheck) {
	limit := defaultHealthReportHistoryLimit
	if check.Spec.HistoryLimit != nil {
		limit = int(*check.Spec.HistoryLimit)
	}
	if limit < 1 {
		return
	}

	var reports fathomv1alpha1.HealthReportList
	if err := r.List(ctx, &reports,
		client.InNamespace(check.Namespace),
		client.MatchingLabels{
			labelHealthReportSourceKind: dnsCheckKind,
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
	for i := range excess {
		victim := &reports.Items[i]
		if err := r.Delete(ctx, victim); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "delete old HealthReport failed", "name", victim.Name)
		}
	}
	log.V(1).Info("pruned DNSCheck HealthReport history", "deleted", excess, "limit", limit)
}

// finishDNSCheck persists status when it changed and schedules the next run.
//
// The requeue is anchored to when this run STARTED, not to when it finished
// (FR-107): anchoring on completion, as the other check kinds do, silently adds
// the run's own duration to every interval.
func (r *DNSCheckReconciler) finishDNSCheck(
	ctx context.Context,
	log logr.Logger,
	before *fathomv1alpha1.DNSCheckStatus,
	check *fathomv1alpha1.DNSCheck,
	interval, elapsed time.Duration,
) (ctrl.Result, error) {
	if !equality.Semantic.DeepEqual(before, &check.Status) {
		if err := r.Status().Update(ctx, check); err != nil {
			return ctrl.Result{}, err
		}
		log.V(1).Info("updated DNSCheck status", "result", check.Status.LastResult)
	}
	return ctrl.Result{RequeueAfter: nextDNSRequeue(interval, elapsed, dnsCheckMinRunGap)}, nil
}

// launcher returns the configured runner, defaulting to a probe.Launcher over
// the uncached probe client.
func (r *DNSCheckReconciler) launcher() dnsProbeRunner {
	if r.Launcher != nil {
		return r.Launcher
	}
	return &probe.Launcher{Client: r.ProbeClient}
}

// concurrency is the in-flight probe cap, floored at 1 so a zero-valued struct
// cannot deadlock a run.
func (r *DNSCheckReconciler) concurrency() int {
	if r.MaxConcurrentProbes < 1 {
		return 1
	}
	return r.MaxConcurrentProbes
}

// minPairBudget is the least time worth giving a pair, defaulting to the
// production floor.
func (r *DNSCheckReconciler) minPairBudget() time.Duration {
	if r.MinPairBudget > 0 {
		return r.MinPairBudget
	}
	return dnsCheckMinPairBudget
}

// SetupWithManager registers the reconciler.
//
// It deliberately does NOT Own(&corev1.Pod{}): an Owns clause installs an
// informer for that type, which is precisely the cluster-wide Pod watch
// ProbeClient exists to avoid. Probe pods are short-lived and polled directly,
// so nothing needs to watch them — and a pod event could not usefully
// re-trigger a run that is already in flight.
func (r *DNSCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.DNSCheck{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: dnsCheckMaxConcurrentReconciles}).
		Named("dnscheck").
		Complete(r)
}
