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

const deschedulerPolicy = "apiVersion: descheduler/v1alpha2\nkind: DeschedulerPolicy\nprofiles: []\n"

func TestDescheduler_HealthyDeploymentMode(t *testing.T) {
	objs := []clientObject{
		deploymentInNamespace("descheduler", "kube-system"),
		podInNamespace("descheduler-abc", "descheduler", "kube-system"),
		configMap("descheduler", "kube-system", map[string]string{"policy.yaml": deschedulerPolicy}),
	}
	result, err := NewDeschedulerEngine().Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Target: adapter.TargetRef{Kind: "AddonCheck", Namespace: "default", Name: "descheduler"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "descheduler", adapter.OutcomePass, "available")
	assertHasOutcome(t, result.Checks, "ConfigMap", "descheduler", adapter.OutcomePass, "well-formed")
	assertFamily(t, result.Checks, "ConfigMap", "descheduler", adapter.Family("policy_validity"))
	// Deployment-mode install: the CronJob is absent, so its system_health and
	// last_run checks are Skipped (Optional), never Fail.
	assertHasOutcome(t, result.Checks, "CronJob", "descheduler", adapter.OutcomeSkipped, "not found")
}

func TestDescheduler_HealthyCronJobMode(t *testing.T) {
	recent := time.Now().Add(-1 * time.Hour)
	objs := []clientObject{
		cronJob("descheduler", "kube-system", false, &recent, &recent),
		configMap("descheduler", "kube-system", map[string]string{"policy.yaml": deschedulerPolicy}),
	}
	result, err := NewDeschedulerEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objs...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// system_health CronJob (existence) and last_run CronJob (recency) both pass.
	assertHasOutcome(t, result.Checks, "CronJob", "descheduler", adapter.OutcomePass, "cronjob is present")
	assertHasOutcome(t, result.Checks, "CronJob", "descheduler", adapter.OutcomePass, "recent")
	// Deployment-mode workload absent -> Skipped, not Fail.
	assertHasOutcome(t, result.Checks, "Deployment", "descheduler", adapter.OutcomeSkipped, "not found")
}

func TestDescheduler_InvalidPolicyFails(t *testing.T) {
	objs := []clientObject{
		deploymentInNamespace("descheduler", "kube-system"),
		podInNamespace("descheduler-abc", "descheduler", "kube-system"),
		configMap("descheduler", "kube-system", map[string]string{"policy.yaml": "\tbroken: [yaml"}),
	}
	result, err := NewDeschedulerEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t, objs...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "ConfigMap", "descheduler", adapter.OutcomeFail, "not valid YAML")
}

func TestDescheduler_AbsentClusterAllSkipped(t *testing.T) {
	result, err := NewDeschedulerEngine().Run(context.Background(), adapter.Request{Client: newFakeClient(t), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, result.Checks, "Deployment", "descheduler", adapter.OutcomeSkipped, "not found")
	assertHasOutcome(t, result.Checks, "CronJob", "descheduler", adapter.OutcomeSkipped, "not found")
	assertHasOutcome(t, result.Checks, "ConfigMap", "descheduler", adapter.OutcomeSkipped, "not found")
	for _, c := range result.Checks {
		if c.Outcome == adapter.OutcomeFail {
			t.Fatalf("no check should Fail on an absent Optional addon: %#v", c)
		}
	}
}
