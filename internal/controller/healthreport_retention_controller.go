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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1 "github.com/skaphos/fathom/api/v1"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/internal/nodehealth"
)

// HealthReport retention (spec 014, contracts/retention.md, ADR 0010).
//
// Reports are no longer dependents of their check: deleting a check keeps its
// history for a bounded window so "was this passing before the incident?"
// survives a deliberate stop (delete is the only stop since spec.paused went).
// This reconciler is keyed by namespace. On a check's deletion, and on a
// periodic sweep, it lists the namespace's reports, decides which ones belong
// to a check that no longer exists (by kind, name and UID, so a recreated
// check of the same name never inherits or loses its predecessor's history),
// stamps the moment it first found the check gone, deletes orphans older than
// the window, and keeps every kind under a per-namespace cap. Work per pass is
// bounded by a write budget; a pass that exhausts it continues after a short
// fixed pause.
const (
	// retentionWriteBudget bounds writes (stamps and deletes) per reconcile, so a
	// namespace whose checks went while the operator was down drains in bounded
	// passes instead of one startup burst (contracts/retention.md).
	retentionWriteBudget = 50
	// retentionResync is how often a namespace is swept without a trigger, so
	// a check deleted while the operator was down is still noticed and its
	// orphans still expire.
	retentionResync = time.Hour
	// retentionContinueAfter is the pause before the next pass once a sweep has
	// spent its delete budget: a fixed short delay instead of the rate-limited
	// immediate requeue, so a large backlog drains at a steady, bounded rate.
	retentionContinueAfter = time.Second

	eventReasonReportsPruned = "ReportsPruned"
	eventActionRetain        = "Retain"

	pruneReasonWindow = "window"
	pruneReasonCap    = "cap"
)

// HealthReportRetentionReconciler prunes orphaned and surplus HealthReports.
type HealthReportRetentionReconciler struct {
	client.Client
	// Recorder emits one ReportsPruned Event per pass that deleted anything.
	Recorder events.EventRecorder
	// Window is how long a deleted check's reports are kept; zero prunes
	// orphans on the first pass that sees them.
	Window time.Duration
	// CapPerKind is the most reports of one source kind a namespace keeps,
	// live and orphaned alike; the oldest go first.
	CapPerKind int
	// Clock is the time source; nil means time.Now. Tests inject it.
	Clock func() time.Time
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthreports,verbs=get;list;watch;patch;delete

func (r *HealthReportRetentionReconciler) now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now()
}

// reportSource is the identity a report carries in its labels.
type reportSource struct {
	kind, name, uid string
}

// Reconcile sweeps one namespace (req.Name).
func (r *HealthReportRetentionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	namespace := req.Name
	log := logf.FromContext(ctx).WithValues("namespace", namespace)
	now := r.now()

	var list fathomv1.HealthReportList
	if err := r.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return ctrl.Result{}, fmt.Errorf("list HealthReports in %s: %w", namespace, err)
	}
	if len(list.Items) == 0 {
		// Nothing to retain: drop the namespace's gauge series rather than pin
		// four zero-valued ones for every namespace ever swept.
		metrics.DeleteRetentionSeries(namespace)
		return ctrl.Result{}, nil
	}

	// Orphans: reports whose check (same kind, name and UID) no longer exists.
	alive := map[reportSource]bool{}
	protected := map[string]bool{} // a live check's latest report; the cap spares it
	var orphans []*fathomv1.HealthReport
	for i := range list.Items {
		report := &list.Items[i]
		src := reportSource{
			kind: report.Labels[fathomv1.LabelHealthReportSourceKind],
			name: report.Labels[fathomv1.LabelHealthReportSourceName],
			uid:  report.Labels[fathomv1.LabelHealthReportSourceUID],
		}
		if src.kind == "" || src.name == "" {
			continue // not a report this operator wrote; never touched
		}
		live, known := alive[src]
		if !known {
			var latest string
			var err error
			if live, latest, err = r.checkExists(ctx, namespace, src); err != nil {
				return ctrl.Result{}, err
			}
			alive[src] = live
			if live && latest != "" {
				protected[latest] = true
			}
		}
		if !live {
			orphans = append(orphans, report)
		}
	}

	// Writes are budgeted and tolerant: one report the API server refuses (an
	// admission policy, a missing verb) must not stop the rest of the namespace
	// from being stamped, expired or capped, so failures are collected and the
	// pass is returned as failed only once everything else has been tried.
	budget := retentionWriteBudget
	pruned := map[string]map[string]int{} // reason -> kind -> count
	deleted := map[types.UID]bool{}
	var last *fathomv1.HealthReport
	var failures []error
	remove := func(report *fathomv1.HealthReport, reason string) {
		budget-- // an attempt the API server saw, refused or not
		if err := r.Delete(ctx, report); err != nil && !apierrors.IsNotFound(err) {
			failures = append(failures, fmt.Errorf("delete HealthReport %s/%s: %w", report.Namespace, report.Name, err))
			return
		}
		deleted[report.UID] = true
		if pruned[reason] == nil {
			pruned[reason] = map[string]int{}
		}
		pruned[reason][report.Labels[fathomv1.LabelHealthReportSourceKind]]++
		last = report
	}

	// Window: stamp the first observation, expire the old ones, and remember
	// how soon the earliest survivor is due so the next pass is not left to
	// the hourly resync or to unrelated report traffic in the namespace.
	var nextExpiry time.Duration
	for _, report := range orphans {
		if budget == 0 {
			break
		}
		if r.Window <= 0 {
			remove(report, pruneReasonWindow)
			continue
		}
		deletedAt, ok := orphanedAt(report)
		if !ok {
			budget--
			if err := r.stamp(ctx, report, now); err != nil {
				failures = append(failures, err)
				continue
			}
			nextExpiry = soonerExpiry(nextExpiry, r.Window)
			continue
		}
		if remaining := r.Window - now.Sub(deletedAt); remaining > 0 {
			nextExpiry = soonerExpiry(nextExpiry, remaining)
			continue
		}
		remove(report, pruneReasonWindow)
	}

	// Cap: per kind, oldest first, live and orphaned alike — except the report
	// a live check's status.lastReportName still names, which stays referenceable.
	byKind := map[string][]*fathomv1.HealthReport{}
	for i := range list.Items {
		report := &list.Items[i]
		if deleted[report.UID] {
			continue
		}
		kind := report.Labels[fathomv1.LabelHealthReportSourceKind]
		if kind == "" {
			continue
		}
		byKind[kind] = append(byKind[kind], report)
	}
	if r.CapPerKind > 0 {
		for _, reports := range byKind {
			excess := len(reports) - r.CapPerKind
			if excess <= 0 {
				continue
			}
			sort.Slice(reports, func(i, j int) bool { return olderHealthReport(reports[i], reports[j], "") })
			for _, victim := range reports {
				if excess == 0 || budget == 0 {
					break
				}
				if protected[victim.Name] {
					continue
				}
				remove(victim, pruneReasonCap)
				excess--
			}
		}
	}

	// Observability: what is retained now, what went, and why.
	retained := map[string]int{}
	for _, report := range orphans {
		if !deleted[report.UID] {
			retained[report.Labels[fathomv1.LabelHealthReportSourceKind]]++
		}
	}
	r.observeRetained(namespace, retained)
	total := 0
	for reason, kinds := range pruned {
		for kind, n := range kinds {
			metrics.RecordReportsPruned(namespace, kind, reason, n)
			total += n
		}
	}
	if total > 0 {
		log.Info("pruned HealthReports", "deleted", total, "window", countOf(pruned, pruneReasonWindow), "cap", countOf(pruned, pruneReasonCap))
		if r.Recorder != nil && last != nil {
			// An Event may reference an object that no longer exists (Pod events
			// do); regarding the last pruned report keeps it in the namespace,
			// which a cluster-scoped subject could not (research R11).
			r.Recorder.Eventf(last, nil, corev1.EventTypeNormal, eventReasonReportsPruned, eventActionRetain,
				"pruned %d HealthReports (window: %d, cap: %d)", total, countOf(pruned, pruneReasonWindow), countOf(pruned, pruneReasonCap))
		}
	}
	if len(failures) > 0 {
		return ctrl.Result{}, fmt.Errorf("%d retention write(s) in %s failed: %w", len(failures), namespace, errors.Join(failures...))
	}
	if budget == 0 {
		return ctrl.Result{RequeueAfter: retentionContinueAfter}, nil
	}
	return ctrl.Result{RequeueAfter: nextPass(nextExpiry)}, nil
}

// soonerExpiry folds a window expiry into the running minimum; zero means none yet.
func soonerExpiry(current, candidate time.Duration) time.Duration {
	if current <= 0 || candidate < current {
		return candidate
	}
	return current
}

// nextPass is the pause before a namespace is swept again: the hourly resync,
// or sooner when a retained report's window is about to elapse, floored so a
// stamp truncated to whole seconds cannot schedule a zero-length pause.
func nextPass(nextExpiry time.Duration) time.Duration {
	if nextExpiry <= 0 || nextExpiry > retentionResync {
		return retentionResync
	}
	return max(nextExpiry, retentionContinueAfter)
}

func countOf(pruned map[string]map[string]int, reason string) int {
	n := 0
	for _, c := range pruned[reason] {
		n += c
	}
	return n
}

// orphanedAt reads the deleted-at stamp; absent or unparsable counts as unstamped.
func orphanedAt(report *fathomv1.HealthReport) (time.Time, bool) {
	raw, ok := report.Annotations[fathomv1.AnnotationHealthReportSourceDeletedAt]
	if !ok {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	return t, err == nil
}

// stamp records the first time the report's check was found gone. The
// annotation is written once and never rewritten.
func (r *HealthReportRetentionReconciler) stamp(ctx context.Context, report *fathomv1.HealthReport, now time.Time) error {
	patch := client.MergeFrom(report.DeepCopy())
	if report.Annotations == nil {
		report.Annotations = map[string]string{}
	}
	report.Annotations[fathomv1.AnnotationHealthReportSourceDeletedAt] = now.UTC().Format(time.RFC3339)
	if err := r.Patch(ctx, report, patch); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("stamp HealthReport %s/%s: %w", report.Namespace, report.Name, err)
	}
	return nil
}

// checkExists reports whether the check a report names still exists with the
// same UID, and the report its status currently names so the cap can spare
// it. A kind this operator does not write reports for is treated as alive, so
// foreign reports are never pruned.
func (r *HealthReportRetentionReconciler) checkExists(ctx context.Context, namespace string, src reportSource) (bool, string, error) {
	var obj client.Object
	var latest func() string
	switch src.kind {
	case "AddonCheck":
		o := &fathomv1.AddonCheck{}
		obj, latest = o, func() string { return o.Status.LastReportName }
	case dnsCheckKind:
		o := &fathomv1.DNSCheck{}
		obj, latest = o, func() string { return o.Status.LastReportName }
	case "NodeCertificateCheck":
		o := &fathomv1.NodeCertificateCheck{}
		obj, latest = o, func() string { return o.Status.LastReportName }
	case nodehealth.KindNodeHealthCheck:
		o := &fathomv1.NodeHealthCheck{}
		obj, latest = o, func() string { return o.Status.LastReportName }
	default:
		return true, "", nil
	}
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: src.name}, obj)
	switch {
	case apierrors.IsNotFound(err):
		return false, "", nil
	case err != nil:
		return false, "", fmt.Errorf("get %s %s/%s: %w", src.kind, namespace, src.name, err)
	}
	// A report written before UID labels existed (none survive the v1
	// replacement, but be defensive) follows the name alone.
	if src.uid != "" && string(obj.GetUID()) != src.uid {
		return false, "", nil
	}
	return true, latest(), nil
}

func (r *HealthReportRetentionReconciler) observeRetained(namespace string, retained map[string]int) {
	for _, kind := range []string{"AddonCheck", dnsCheckKind, "NodeCertificateCheck", nodehealth.KindNodeHealthCheck} {
		metrics.ObserveReportsRetained(namespace, kind, retained[kind])
	}
}

// SetupWithManager keys the reconciler by namespace: a check's deletion and a
// report's creation each enqueue their namespace; Reconcile's requeue keeps
// every namespace that has reports on the periodic sweep.
func (r *HealthReportRetentionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	toNamespace := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: obj.GetNamespace()}}}
	})
	deleteOnly := predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return false },
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
	}
	createOnly := predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("healthreport-retention").
		Watches(&fathomv1.HealthReport{}, toNamespace, builder.WithPredicates(createOnly)).
		Watches(&fathomv1.AddonCheck{}, toNamespace, builder.WithPredicates(deleteOnly)).
		Watches(&fathomv1.DNSCheck{}, toNamespace, builder.WithPredicates(deleteOnly)).
		Watches(&fathomv1.NodeCertificateCheck{}, toNamespace, builder.WithPredicates(deleteOnly)).
		Watches(&fathomv1.NodeHealthCheck{}, toNamespace, builder.WithPredicates(deleteOnly)).
		Complete(r)
}
