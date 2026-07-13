/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/skaphos/fathom/pkg/adapter"
)

// cronJob builds a batch/v1 CronJob with the given suspend flag and optional
// last-schedule / last-success times (nil leaves the status field unset).
func cronJob(name, namespace string, suspend bool, lastSchedule, lastSuccess *time.Time) *batchv1.CronJob {
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       batchv1.CronJobSpec{Suspend: &suspend},
	}
	if lastSchedule != nil {
		cj.Status.LastScheduleTime = &metav1.Time{Time: *lastSchedule}
	}
	if lastSuccess != nil {
		cj.Status.LastSuccessfulTime = &metav1.Time{Time: *lastSuccess}
	}
	return cj
}

// runCronJob runs an engine whose single enabled family carries cc as its only
// CronJobCheck, against objs.
func runCronJob(t *testing.T, cc CronJobCheck, objs ...clientObject) []adapter.CheckResult {
	t.Helper()
	eng := MustEngine(AddonDefinition{
		AddonType:      "cronjobtest",
		AdapterVersion: "0.0.1",
		Families:       []FamilyDefinition{{Name: "run", DefaultEnabled: true, CronJobs: []CronJobCheck{cc}}},
	})
	res, err := eng.Run(context.Background(), adapter.Request{Client: newFakeClient(t, objs...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res.Checks
}

func TestCronJobCheck(t *testing.T) {
	recent := time.Now().Add(-1 * time.Minute)
	stale := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(1 * time.Hour) // beyond the clock-skew grace

	systemHealth := CronJobCheck{DefaultNamespace: "kube-system", DefaultName: "descheduler", Component: "descheduler"}
	lastRun := CronJobCheck{DefaultNamespace: "kube-system", DefaultName: "descheduler", Component: "descheduler", DefaultSuccessMaxAge: 24 * time.Hour, StaleOutcome: adapter.OutcomeWarn}

	tests := []struct {
		name    string
		check   CronJobCheck
		obj     *batchv1.CronJob
		outcome adapter.Outcome
		summary string
	}{
		{"present and healthy", systemHealth, cronJob("descheduler", "kube-system", false, &recent, &recent), adapter.OutcomePass, "cronjob is present"},
		{"suspended warns", systemHealth, cronJob("descheduler", "kube-system", true, &recent, &recent), adapter.OutcomeWarn, "suspended"},
		{"recent success passes", lastRun, cronJob("descheduler", "kube-system", false, &recent, &recent), adapter.OutcomePass, "recent"},
		{"stale success warns", lastRun, cronJob("descheduler", "kube-system", false, &recent, &stale), adapter.OutcomeWarn, "older than the freshness window"},
		{"recently scheduled, not yet successful passes", lastRun, cronJob("descheduler", "kube-system", false, &recent, nil), adapter.OutcomePass, "awaiting its first successful completion"},
		{"stale schedule, never successful warns", lastRun, cronJob("descheduler", "kube-system", false, &stale, nil), adapter.OutcomeWarn, "not completed a successful run within the freshness window"},
		{"future success time warns", lastRun, cronJob("descheduler", "kube-system", false, &recent, &future), adapter.OutcomeWarn, "in the future"},
		{"future schedule, never successful warns", lastRun, cronJob("descheduler", "kube-system", false, &future, nil), adapter.OutcomeWarn, "in the future"},
		{"never scheduled passes", lastRun, cronJob("descheduler", "kube-system", false, nil, nil), adapter.OutcomePass, "not scheduled a run yet"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checks := runCronJob(t, tc.check, tc.obj)
			assertHasOutcome(t, checks, "CronJob", "descheduler", tc.outcome, tc.summary)
		})
	}
}

func TestCronJobCheck_AbsentInheritsOptional(t *testing.T) {
	// No CronJob object: the addon-level Optional default makes a NotFound a
	// Skipped carrying the absent marker.
	eng := MustEngine(AddonDefinition{
		AddonType:      "cronjobtest",
		AdapterVersion: "0.0.1",
		Optional:       true,
		Families:       []FamilyDefinition{{Name: "run", DefaultEnabled: true, CronJobs: []CronJobCheck{{DefaultNamespace: "kube-system", DefaultName: "descheduler", Component: "descheduler"}}}},
	})
	res, err := eng.Run(context.Background(), adapter.Request{Client: newFakeClient(t), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, res.Checks, "CronJob", "descheduler", adapter.OutcomeSkipped, "not found")
	assertHasDetail(t, res.Checks, "CronJob", "descheduler", adapter.DetailAbsent, "true")
}

func TestCronJobCheck_SuccessMaxAgeThresholdOverride(t *testing.T) {
	// A policy threshold tightens the freshness window so a run that would pass
	// the default 24h window is stale under 30m.
	fortyMinAgo := time.Now().Add(-40 * time.Minute)
	check := CronJobCheck{
		DefaultNamespace:          "kube-system",
		DefaultName:               "descheduler",
		Component:                 "descheduler",
		SuccessMaxAgeThresholdKey: "successMaxAge",
		DefaultSuccessMaxAge:      24 * time.Hour,
		StaleOutcome:              adapter.OutcomeWarn,
	}
	eng := MustEngine(AddonDefinition{
		AddonType:      "cronjobtest",
		AdapterVersion: "0.0.1",
		Families:       []FamilyDefinition{{Name: "run", DefaultEnabled: true, CronJobs: []CronJobCheck{check}}},
	})
	res, err := eng.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, cronJob("descheduler", "kube-system", false, &fortyMinAgo, &fortyMinAgo)),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{"run": {Enabled: true, Thresholds: map[string]string{"successMaxAge": "30m"}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, res.Checks, "CronJob", "descheduler", adapter.OutcomeWarn, "older than the freshness window")
}
