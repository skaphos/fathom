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

	"github.com/skaphos/fathom/pkg/adapter"
)

func TestKured_HealthyNoRebootPending(t *testing.T) {
	// DaemonSet Ready, no lock held (annotation absent), no node flagged for
	// reboot -> system_health passes, reboot_state is quiet.
	objs := []clientObject{
		daemonSetInNamespace("kured", "kube-system", 3),
		podInNamespace("kured-n1", "kured", "kube-system"),
		nodeWithAnnotations("node1", nil),
	}
	result, err := NewKuredEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "kured"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", "kured", adapter.OutcomePass, "fully ready")
	// Lock annotation absent -> Pass; node scan finds nothing flagged -> Skipped.
	assertHasOutcome(t, result.Checks, "DaemonSet", "kured", adapter.OutcomePass, "absent")
	assertHasOutcome(t, result.Checks, "Node", "nodes", adapter.OutcomeSkipped, "no Node objects carry annotation")
}

func TestKured_WedgedLockWarns(t *testing.T) {
	lock := map[string]string{"weave.works/kured-node-lock": lockJSON(time.Now().Add(-3 * time.Hour))}
	objs := []clientObject{
		daemonSetWithAnnotations("kured", "kube-system", lock),
		podInNamespace("kured-n1", "kured", "kube-system"),
	}
	result, err := NewKuredEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objs...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", "kured", adapter.OutcomeWarn, "older than the freshness window")
}

func TestKured_LongWaitingNodeWarns(t *testing.T) {
	ann := map[string]string{"weave.works/kured-most-recent-reboot-needed": time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)}
	objs := []clientObject{
		daemonSetInNamespace("kured", "kube-system", 1),
		podInNamespace("kured-n1", "kured", "kube-system"),
		nodeWithAnnotations("node1", ann),
	}
	result, err := NewKuredEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objs...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Node", "node1", adapter.OutcomeWarn, "older than the freshness window")
}

func TestKured_AbsentClusterAllSkipped(t *testing.T) {
	result, err := NewKuredEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "DaemonSet", "kured", adapter.OutcomeSkipped, "not found")
	assertHasDetail(t, result.Checks, "DaemonSet", "kured", adapter.DetailAbsent, "true")
	// Node scan: no nodes carry the reboot annotation -> Skipped.
	assertHasOutcome(t, result.Checks, "Node", "nodes", adapter.OutcomeSkipped, "no Node objects carry annotation")
	for _, c := range result.Checks {
		if c.Outcome == adapter.OutcomeFail {
			t.Fatalf("no check should Fail on an absent Optional addon: %#v", c)
		}
	}
}
