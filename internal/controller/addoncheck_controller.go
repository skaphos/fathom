/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/registry"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/pkg/adapter"
)

const (
	addonCheckConditionAccepted = "Accepted"
	addonCheckConditionPaused   = "Paused"
	addonCheckConditionReady    = "Ready"

	defaultAddonCheckTimeout = 30 * time.Second

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
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=addonchecks/finalizers,verbs=update
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthreports,verbs=create;get;list;watch;delete

// Reconcile records that the AddonCheck spec has been observed. Adapter
// dispatch and HealthReport creation are wired in follow-up SKA-46 work once
// the registry is available to the reconciler.
func (r *AddonCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	defer func() {
		outcome := "success"
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
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               addonCheckConditionAccepted,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "SpecAccepted",
		Message:            "AddonCheck specification has been accepted for reconciliation.",
	})

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

	if adapterReady && (check.Status.LastRunTime == nil || previousObservedGeneration != check.Generation) {
		if err := r.runAddonCheck(ctx, log, &check, selectedAdapter); err != nil {
			return ctrl.Result{}, err
		}
	}

	if equality.Semantic.DeepEqual(before, &check.Status) {
		return ctrl.Result{}, nil
	}
	if err := r.Status().Update(ctx, &check); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("updated AddonCheck status")

	return ctrl.Result{}, nil
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
	apiMeta.SetStatusCondition(&check.Status.Conditions, metav1.Condition{
		Type:               addonCheckConditionReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: check.Generation,
		Reason:             "AdapterResolved",
		Message:            "AddonCheck has a registered adapter and is ready for execution.",
	})
	return selectedAdapter, true
}

func (r *AddonCheckReconciler) runAddonCheck(ctx context.Context, log logr.Logger, check *fathomv1alpha1.AddonCheck, selectedAdapter adapter.Adapter) error {
	timeout := addonCheckTimeout(check)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	started := time.Now()
	result, runErr := selectedAdapter.Run(runCtx, adapter.Request{
		Client:     r.Client,
		Logger:     log.WithValues("adapter", selectedAdapter.Name(), "addonType", check.Spec.AddonType),
		Target:     addonCheckTargetRef(check),
		Policy:     addonCheckPolicy(check),
		Timeout:    timeout,
		ProbeImage: r.ProbeImage,
	})
	observedAt := metav1.NewTime(time.Now())
	if result.Duration == 0 {
		result.Duration = time.Since(started)
	}

	// Record adapter execution metrics (SKA-290)
	outcome := "pass"
	if runErr != nil {
		outcome = "error"
	} else if len(result.Checks) > 0 {
		// Simple first-cut: use the worst outcome from the checks.
		// Can be refined later.
		for _, c := range result.Checks {
			if c.Outcome == adapter.OutcomeFail || c.Outcome == adapter.OutcomeError {
				outcome = string(c.Outcome)
				break
			}
			if c.Outcome == adapter.OutcomeWarn && outcome == "pass" {
				outcome = string(c.Outcome)
			}
		}
	}

	// Compute a representative family label from the policy for better cardinality
	family := "overall"
	for f := range addonCheckPolicy(check) {
		family = string(f)
		break // use the first enabled family as the representative label
	}

	metrics.AdapterRunDuration.WithLabelValues(
		selectedAdapter.Name(),
		family,
		outcome,
	).Observe(result.Duration.Seconds())


	report := healthReportForAddonCheck(check, selectedAdapter, result, observedAt, runErr)
	if r.Scheme != nil {
		if err := controllerutil.SetControllerReference(check, report, r.Scheme); err != nil {
			return err
		}
	}
	if err := r.Create(ctx, report); err != nil {
		return err
	}
	r.pruneHealthReportHistory(ctx, log, check)

	check.Status.LastRunTime = &observedAt
	check.Status.LastReportName = report.Name
	check.Status.LastResult = string(report.Spec.Result)
	readyStatus := metav1.ConditionTrue
	readyReason := "RunCompleted"
	readyMessage := "AddonCheck adapter run completed and a HealthReport was created."
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

func addonCheckPolicy(check *fathomv1alpha1.AddonCheck) map[adapter.Family]adapter.FamilyPolicy {
	if len(check.Spec.Policy) == 0 {
		return nil
	}
	policy := make(map[adapter.Family]adapter.FamilyPolicy, len(check.Spec.Policy))
	for family, familyPolicy := range check.Spec.Policy {
		policy[adapter.Family(family)] = adapter.FamilyPolicy{
			Enabled:       familyPolicy.Enabled,
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

// SetupWithManager sets up the controller with the Manager.
func (r *AddonCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.AddonCheck{}).
		Named("addoncheck").
		Complete(r)
}
