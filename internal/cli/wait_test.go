/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// fakeFactory returns a factory bound to one shared fake client (so tests can
// mutate objects the verb is reading), a fast poll interval, no terminal, and
// the context namespace "team-a".
func fakeFactory(t *testing.T, objs ...client.Object) (*factory, client.Client) {
	t.Helper()
	scheme, err := newScheme()
	if err != nil {
		t.Fatal(err)
	}
	fc := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&fathomv1alpha1.AddonCheck{}, &fathomv1alpha1.DNSCheck{}, &fathomv1alpha1.NodeCertificateCheck{},
			&fathomv1alpha1.HealthCheck{}, &fathomv1alpha1.ClusterHealth{}).
		Build()
	f := newFactory()
	f.clientConfig = func(*globalOptions) clientcmd.ClientConfig {
		return stubClientConfig{cfg: &rest.Config{}, namespace: "team-a"}
	}
	f.newClient = func(*rest.Config, client.Options) (client.Client, error) { return fc, nil }
	f.pollInterval = 5 * time.Millisecond
	f.isTerminal = func() bool { return false }
	f.stdin = strings.NewReader("")
	return f, fc
}

func addonCheck(ns, name string) *fathomv1alpha1.AddonCheck {
	return &fathomv1alpha1.AddonCheck{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "coredns"},
	}
}

func addonTarget(ns, name string, obj client.Object) runTarget {
	return runTarget{ref: checkRef{Kind: kindByName("AddonCheck"), Namespace: ns, Name: name}, obj: obj}
}

// completeRun simulates the operator: once the annotation carries a token,
// write the verdict and the consumed token in one status update.
func completeRun(t *testing.T, fc client.Client, key types.NamespacedName, verdict string) {
	t.Helper()
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			ac := &fathomv1alpha1.AddonCheck{}
			if err := fc.Get(context.Background(), key, ac); err == nil {
				if tok := ac.Annotations[fathomv1alpha1.AnnotationRunNow]; tok != "" {
					now := metav1.Now()
					ac.Status.LastRunTrigger = tok
					ac.Status.LastResult = verdict
					ac.Status.LastRunTime = &now
					ac.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "RunCompleted", Message: "adapter ran"}}
					_ = fc.Status().Update(context.Background(), ac)
					return
				}
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
}

func TestWaitForRun_CompletesOnConsumedToken(t *testing.T) {
	ac := addonCheck("team-a", "coredns")
	ac.Annotations = map[string]string{fathomv1alpha1.AnnotationRunNow: "tok-1"}
	f, fc := fakeFactory(t, ac)
	completeRun(t, fc, types.NamespacedName{Namespace: "team-a", Name: "coredns"}, "Warn")

	res := f.waitForRun(context.Background(), fc, addonTarget("team-a", "coredns", ac), "tok-1", 2*time.Second)
	if res.err != nil || res.superseded || res.timedOut {
		t.Fatalf("unexpected result %+v", res)
	}
	if res.snap.Verdict != "Warn" || res.snap.ConsumedTrigger != "tok-1" || res.snap.Summary != "adapter ran" {
		t.Fatalf("snapshot = %+v", res.snap)
	}
}

func TestWaitForRun_Superseded(t *testing.T) {
	ac := addonCheck("team-a", "coredns")
	ac.Annotations = map[string]string{fathomv1alpha1.AnnotationRunNow: "tok-other"}
	f, fc := fakeFactory(t, ac)

	res := f.waitForRun(context.Background(), fc, addonTarget("team-a", "coredns", ac), "tok-1", time.Second)
	if !res.superseded || res.err != nil || res.timedOut {
		t.Fatalf("expected superseded, got %+v", res)
	}
}

func TestWaitForRun_TimesOut(t *testing.T) {
	ac := addonCheck("team-a", "coredns")
	ac.Annotations = map[string]string{fathomv1alpha1.AnnotationRunNow: "tok-1"}
	f, fc := fakeFactory(t, ac)

	start := time.Now()
	res := f.waitForRun(context.Background(), fc, addonTarget("team-a", "coredns", ac), "tok-1", 40*time.Millisecond)
	if !res.timedOut || res.err != nil || res.superseded {
		t.Fatalf("expected timeout, got %+v", res)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("timeout was not honoured")
	}
}

func TestWaitForRun_ReadErrorIsReported(t *testing.T) {
	f, fc := fakeFactory(t) // object does not exist
	res := f.waitForRun(context.Background(), fc, addonTarget("team-a", "missing", addonCheck("team-a", "missing")), "tok-1", time.Second)
	if res.err == nil || !strings.Contains(res.err.Error(), "re-read") {
		t.Fatalf("expected a re-read error, got %+v", res)
	}
}
