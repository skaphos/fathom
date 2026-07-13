/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/skaphos/fathom/pkg/adapter"
)

// Evaluate implements Evaluator for CronJobCheck. It reads one batch/v1 CronJob
// and scores, in order: absence (NotFound -> Absence posture), suspension
// (spec.suspend -> Warn), then — only when DefaultSuccessMaxAge (or its policy
// override) is positive — last-success recency:
//
//   - never scheduled a Job yet -> Pass (a freshly installed CronJob is healthy).
//   - scheduled a Job but never succeeded, CronJob younger than the window ->
//     Pass (the first run may still be in progress; not stuck).
//   - scheduled a Job but never succeeded and the CronJob is older than the
//     window -> StaleOutcome (it keeps scheduling but never completes — gating
//     on object age, not the last schedule, catches perpetual fast-failing runs
//     whose LastScheduleTime stays recent forever).
//   - last successful completion older than the window -> StaleOutcome.
//   - last successful completion within the window -> Pass.
//
// It reads no pods: a CronJob owns transient Jobs, not a long-running replica
// set, so there is no steady-state pod population to inspect.
func (c CronJobCheck) Evaluate(ec EvalContext) ([]adapter.CheckResult, error) {
	started := time.Now()
	ns := firstNamespace(ec.Policy, c.DefaultNamespace)
	name := c.DefaultName
	if c.NameThresholdKey != "" {
		name = stringThreshold(ec.Policy, c.NameThresholdKey, c.DefaultName)
	}
	stale := c.StaleOutcome
	if stale == "" {
		stale = adapter.OutcomeWarn
	}
	maxAge := c.DefaultSuccessMaxAge
	if c.SuccessMaxAgeThresholdKey != "" {
		maxAge = durationThreshold(ec.Policy, c.SuccessMaxAgeThresholdKey, c.DefaultSuccessMaxAge)
	}

	target := adapter.TargetRef{APIVersion: "batch/v1", Kind: "CronJob", Namespace: ns, Name: name}
	details := map[string]string{"component": c.Component}

	var cj batchv1.CronJob
	if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Namespace: ns, Name: name}, &cj); err != nil {
		if apierrors.IsNotFound(err) {
			o := absenceOutcome(effectiveAbsence(c.Absence, ec.DefaultPosture))
			return []adapter.CheckResult{result(ec.Family, target, o, "cronjob not found", adapter.MarkAbsent(details), started)}, nil
		}
		return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomeError, fmt.Sprintf("failed to read cronjob: %v", err), details, started)}, nil
	}

	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomeWarn, "cronjob is suspended", details, started)}, nil
	}

	// Existence + suspend only: the system_health mode leaves DefaultSuccessMaxAge
	// zero so a present, un-suspended CronJob is a clean Pass without reasoning
	// about run history.
	if maxAge <= 0 {
		return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomePass, "cronjob is present", details, started)}, nil
	}

	last := cj.Status.LastSuccessfulTime
	if last == nil {
		schedule := cj.Status.LastScheduleTime
		if schedule == nil {
			return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomePass, "cronjob has not scheduled a run yet", details, started)}, nil
		}
		// Scheduled but no success recorded yet. Only stale once the schedule
		// itself is older than the window: a run that was just scheduled (or is
		// still in progress) is awaiting its first completion, not stuck — so a
		// fresh install or a long first run does not falsely Warn.
		details["lastScheduleTime"] = schedule.UTC().Format(time.RFC3339)
		details["createdAt"] = cj.CreationTimestamp.UTC().Format(time.RFC3339)
		details["successMaxAge"] = maxAge.String()
		if isFutureTimestamp(schedule.Time) {
			return []adapter.CheckResult{result(ec.Family, target, stale, "cronjob last schedule time is in the future (clock skew or malformed status)", details, started)}, nil
		}
		// Never succeeded: stale once the CronJob has existed longer than the
		// freshness window. Gate on the object's age, not the last schedule — a
		// CronJob whose every run fails fast keeps LastScheduleTime recent
		// forever and would otherwise stay Pass despite never completing.
		if time.Since(cj.CreationTimestamp.Time) > maxAge {
			return []adapter.CheckResult{result(ec.Family, target, stale, "cronjob has not completed a successful run within the freshness window", details, started)}, nil
		}
		return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomePass, "cronjob scheduled a run recently and is awaiting its first successful completion", details, started)}, nil
	}

	details["lastSuccessfulTime"] = last.UTC().Format(time.RFC3339)
	details["successMaxAge"] = maxAge.String()
	if isFutureTimestamp(last.Time) {
		return []adapter.CheckResult{result(ec.Family, target, stale, "cronjob last successful run time is in the future (clock skew or malformed status)", details, started)}, nil
	}
	if time.Since(last.Time) > maxAge {
		return []adapter.CheckResult{result(ec.Family, target, stale, "cronjob last successful run is older than the freshness window", details, started)}, nil
	}
	return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomePass, "cronjob last successful run is recent", details, started)}, nil
}
