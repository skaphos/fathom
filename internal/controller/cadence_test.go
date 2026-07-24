/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func durp(d time.Duration) *metav1.Duration { return &metav1.Duration{Duration: d} }

func TestClampCadence(t *testing.T) {
	cases := []struct {
		name  string
		d     time.Duration
		floor time.Duration
		want  time.Duration
	}{
		{"below is raised", time.Millisecond, 10 * time.Second, 10 * time.Second},
		{"at floor passes", 10 * time.Second, 10 * time.Second, 10 * time.Second},
		{"above passes", time.Minute, 10 * time.Second, time.Minute},
		{"zero keeps treat-as-unset semantics", 0, 10 * time.Second, 0},
		{"negative keeps treat-as-unset semantics", -time.Second, 10 * time.Second, -time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampCadence(tc.d, tc.floor); got != tc.want {
				t.Errorf("clampCadence(%v, %v) = %v, want %v", tc.d, tc.floor, got, tc.want)
			}
		})
	}
}

func TestCadenceClampMessages(t *testing.T) {
	cases := []struct {
		name     string
		interval *metav1.Duration
		timeout  *metav1.Duration
		want     []string
	}{
		{"nothing set", nil, nil, nil},
		{"both above floors", durp(5 * time.Minute), durp(30 * time.Second), nil},
		{"at the floors", durp(10 * time.Second), durp(time.Second), nil},
		{
			"interval below", durp(time.Millisecond), nil,
			[]string{"spec.interval 1ms is below the minimum 10s; using 10s"},
		},
		{
			"timeout below", nil, durp(500 * time.Millisecond),
			[]string{"spec.timeout 500ms is below the minimum 1s; using 1s"},
		},
		{
			"both below, interval first", durp(9 * time.Second), durp(time.Millisecond),
			[]string{
				"spec.interval 9s is below the minimum 10s; using 10s",
				"spec.timeout 1ms is below the minimum 1s; using 1s",
			},
		},
		{"zero values are treat-as-unset, not clamped", durp(0), durp(0), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cadenceClampMessages(tc.interval, tc.timeout)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("message %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestAddonCheckCadenceHelpersClamp(t *testing.T) {
	sub := &fathomv1alpha1.AddonCheck{Spec: fathomv1alpha1.AddonCheckSpec{
		Interval: durp(time.Millisecond), Timeout: durp(time.Millisecond),
	}}
	if got := addonCheckInterval(sub); got != fathomv1alpha1.MinCheckInterval {
		t.Errorf("sub-floor interval = %v, want %v", got, fathomv1alpha1.MinCheckInterval)
	}
	if got := addonCheckTimeout(sub); got != fathomv1alpha1.MinCheckTimeout {
		t.Errorf("sub-floor timeout = %v, want %v", got, fathomv1alpha1.MinCheckTimeout)
	}

	set := &fathomv1alpha1.AddonCheck{Spec: fathomv1alpha1.AddonCheckSpec{
		Interval: durp(time.Minute), Timeout: durp(30 * time.Second),
	}}
	if got := addonCheckInterval(set); got != time.Minute {
		t.Errorf("legal interval = %v, want 1m", got)
	}
	if got := addonCheckTimeout(set); got != 30*time.Second {
		t.Errorf("legal timeout = %v, want 30s", got)
	}

	unset := &fathomv1alpha1.AddonCheck{}
	if got := addonCheckInterval(unset); got != defaultAddonCheckInterval {
		t.Errorf("unset interval = %v, want default %v", got, defaultAddonCheckInterval)
	}
	if got := addonCheckTimeout(unset); got != defaultAddonCheckTimeout {
		t.Errorf("unset timeout = %v, want default %v", got, defaultAddonCheckTimeout)
	}
}

func TestNodeCertCadenceHelpersClamp(t *testing.T) {
	sub := &fathomv1alpha1.NodeCertificateCheck{Spec: fathomv1alpha1.NodeCertificateCheckSpec{
		Interval: durp(time.Millisecond), Timeout: durp(time.Millisecond),
	}}
	if got := nodeCertInterval(sub); got != fathomv1alpha1.MinCheckInterval {
		t.Errorf("sub-floor interval = %v, want %v", got, fathomv1alpha1.MinCheckInterval)
	}
	if got := nodeCertTimeout(sub); got != fathomv1alpha1.MinCheckTimeout {
		t.Errorf("sub-floor timeout = %v, want %v", got, fathomv1alpha1.MinCheckTimeout)
	}

	unset := &fathomv1alpha1.NodeCertificateCheck{}
	if got := nodeCertInterval(unset); got != defaultNodeCertInterval {
		t.Errorf("unset interval = %v, want default %v", got, defaultNodeCertInterval)
	}
	if got := nodeCertTimeout(unset); got != defaultNodeCertTimeout {
		t.Errorf("unset timeout = %v, want default %v", got, defaultNodeCertTimeout)
	}
}

func TestSetAddonCheckAcceptedClamped(t *testing.T) {
	check := &fathomv1alpha1.AddonCheck{Spec: fathomv1alpha1.AddonCheckSpec{
		Interval: durp(time.Millisecond),
	}}

	setAddonCheckAccepted(check, nil)
	cond := findCondition(t, check.Status.Conditions, checkConditionAccepted)
	if cond.Status != metav1.ConditionTrue || cond.Reason != conditionReasonSpecClamped {
		t.Errorf("clamped spec: got %s/%s, want True/SpecClamped", cond.Status, cond.Reason)
	}
	if !strings.Contains(cond.Message, "spec.interval 1ms is below the minimum 10s; using 10s") {
		t.Errorf("clamp message missing field/configured/effective: %q", cond.Message)
	}

	// An invalid policy outranks the clamp notice: the check is stopped, not
	// merely degraded.
	setAddonCheckAccepted(check, []string{`unknown family "bogus"`})
	cond = findCondition(t, check.Status.Conditions, checkConditionAccepted)
	if cond.Status != metav1.ConditionFalse || cond.Reason != "InvalidPolicy" {
		t.Errorf("invalid policy + clamp: got %s/%s, want False/InvalidPolicy", cond.Status, cond.Reason)
	}

	// A clean spec stays SpecAccepted.
	clean := &fathomv1alpha1.AddonCheck{Spec: fathomv1alpha1.AddonCheckSpec{Interval: durp(time.Minute)}}
	setAddonCheckAccepted(clean, nil)
	cond = findCondition(t, clean.Status.Conditions, checkConditionAccepted)
	if cond.Reason != "SpecAccepted" {
		t.Errorf("clean spec: reason = %s, want SpecAccepted", cond.Reason)
	}
}

func findCondition(t *testing.T, conditions []metav1.Condition, condType string) metav1.Condition {
	t.Helper()
	for _, c := range conditions {
		if c.Type == condType {
			return c
		}
	}
	t.Fatalf("condition %s not found in %v", condType, conditions)
	return metav1.Condition{}
}

func acceptedCondition(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               checkConditionAccepted,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
}

func TestObserveCheckEmitsCadenceClampedOncePerEpisode(t *testing.T) {
	msg := "spec.interval 1ms is below the minimum 10s; using 10s."
	clamped := []metav1.Condition{acceptedCondition(conditionReasonSpecClamped, msg)}

	// Newly clamped → one Warning with the condition's message.
	rec := events.NewFakeRecorder(10)
	observeCheck(rec, testCheckObject("clamp"), "AddonCheck", "", "", nil, clamped, nil, nil)
	got := drainEvents(rec)
	if len(got) != 1 || got[0] != "Warning CadenceClamped "+msg {
		t.Errorf("newly clamped: events = %v", got)
	}

	// Unchanged clamp on a later reconcile → silent.
	rec = events.NewFakeRecorder(10)
	observeCheck(rec, testCheckObject("clamp"), "AddonCheck", "", "", clamped, clamped, nil, nil)
	if got := drainEvents(rec); len(got) != 0 {
		t.Errorf("persistent clamp should be silent, got %v", got)
	}

	// Clamp cleared (spec fixed past admission) → no clamp event.
	rec = events.NewFakeRecorder(10)
	fixed := []metav1.Condition{acceptedCondition("SpecAccepted", "AddonCheck specification has been accepted for reconciliation.")}
	observeCheck(rec, testCheckObject("clamp"), "AddonCheck", "", "", clamped, fixed, nil, nil)
	if got := drainEvents(rec); len(got) != 0 {
		t.Errorf("cleared clamp should be silent, got %v", got)
	}
}
