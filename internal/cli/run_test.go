/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// execRun runs `fathomctl run <args>` against the factory and returns stdout,
// stderr, and the command error (which is what drives exit 1).
func execRun(f *factory, args ...string) (string, string, error) {
	var out, errOut bytes.Buffer
	cmd := newRootCommand(f)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"run"}, args...))
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func annotationOf(t *testing.T, fc client.Client, obj client.Object, ns, name string) string {
	t.Helper()
	if err := fc.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, obj); err != nil {
		t.Fatalf("get %s/%s: %v", ns, name, err)
	}
	return obj.GetAnnotations()[fathomv1alpha1.AnnotationRunNow]
}

func healthCheckFor(ns, name, kind, target string) *fathomv1alpha1.HealthCheck {
	return &fathomv1alpha1.HealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       fathomv1alpha1.HealthCheckSpec{CheckRef: fathomv1alpha1.CheckTargetRef{Kind: kind, Name: target}},
	}
}

func TestNewToken_Format(t *testing.T) {
	now := time.Date(2026, 9, 7, 18, 4, 5, 0, time.UTC)
	tok, err := newToken(now)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^2026-09-07T18:04:05Z-[0-9a-f]{6}$`).MatchString(tok) {
		t.Fatalf("token %q does not match <RFC3339 UTC>-<6 hex>", tok)
	}
	other, _ := newToken(now)
	if other == tok {
		t.Fatalf("two tokens at the same instant must differ")
	}
}

func TestRun_RequiresExactlyOneSelection(t *testing.T) {
	f, _ := fakeFactory(t)
	for _, args := range [][]string{{}, {"addoncheck/x", "--all"}, {"--all", "-l", "a=b"}} {
		if _, _, err := execRun(f, args...); err == nil || !strings.Contains(err.Error(), "exactly one") {
			t.Errorf("run %v: error = %v, want exactly-one usage error", args, err)
		}
	}
}

func TestRun_TriggersExecutableDirectly(t *testing.T) {
	f, fc := fakeFactory(t, addonCheck("team-a", "coredns"))
	out, errOut, err := execRun(f, "addoncheck/coredns")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, errOut)
	}
	tok := annotationOf(t, fc, &fathomv1alpha1.AddonCheck{}, "team-a", "coredns")
	if tok == "" || !strings.Contains(out, "triggered (token "+tok+")") {
		t.Fatalf("annotation %q not reflected in output:\n%s", tok, out)
	}
	if !strings.Contains(errOut, "Triggering 1 check(s)") {
		t.Fatalf("count not printed:\n%s", errOut)
	}
}

func TestRun_NotFoundAndUnexpectedArgs(t *testing.T) {
	f, _ := fakeFactory(t)
	if _, _, err := execRun(f, "addoncheck/nope"); err == nil || !strings.Contains(err.Error(), `AddonCheck "nope" not found in namespace team-a`) {
		t.Errorf("not-found error = %v", err)
	}
	if _, _, err := execRun(f, "addoncheck/x", "y"); err == nil || !strings.Contains(err.Error(), "unexpected argument") {
		t.Errorf("extra-arg error = %v", err)
	}
	if _, _, err := execRun(f, "addoncheck", "x", "y"); err == nil || !strings.Contains(err.Error(), "at most 2 arg") {
		t.Errorf("three-arg error = %v", err)
	}
}

// TestRun_PropagatesFromDerivedKinds covers FR-025: a HealthCheck triggers
// its source, a ClusterHealth fans out to every child's source, shared
// sources are triggered once, and every target gets the same token.
func TestRun_PropagatesFromDerivedKinds(t *testing.T) {
	ch := &fathomv1alpha1.ClusterHealth{
		ObjectMeta: metav1.ObjectMeta{Name: "prod"},
		Status: fathomv1alpha1.ClusterHealthStatus{Children: []fathomv1alpha1.ClusterHealthChildSummary{
			{Namespace: "team-a", Name: "web"}, {Namespace: "team-a", Name: "dns"}, {Namespace: "team-a", Name: "web-again"},
		}},
	}
	dns := &fathomv1alpha1.DNSCheck{ObjectMeta: metav1.ObjectMeta{Name: "cluster-dns", Namespace: "team-a"}}
	f, fc := fakeFactory(t,
		addonCheck("team-a", "coredns"), dns, ch,
		healthCheckFor("team-a", "web", "AddonCheck", "coredns"),
		healthCheckFor("team-a", "dns", "DNSCheck", "cluster-dns"),
		healthCheckFor("team-a", "web-again", "AddonCheck", "coredns"),
	)

	out, errOut, err := execRun(f, "healthcheck/web")
	if err != nil {
		t.Fatalf("run healthcheck: %v\n%s", err, errOut)
	}
	if !strings.Contains(out, "addoncheck/team-a/coredns (via healthcheck/team-a/web)") {
		t.Fatalf("HealthCheck should trigger its source via itself:\n%s", out)
	}

	out, errOut, err = execRun(f, "clusterhealth/prod")
	if err != nil {
		t.Fatalf("run clusterhealth: %v\n%s", err, errOut)
	}
	if !strings.Contains(errOut, "Triggering 2 check(s)") {
		t.Fatalf("shared source must be de-duplicated:\n%s", errOut)
	}
	acTok := annotationOf(t, fc, &fathomv1alpha1.AddonCheck{}, "team-a", "coredns")
	dnsTok := annotationOf(t, fc, &fathomv1alpha1.DNSCheck{}, "team-a", "cluster-dns")
	if acTok == "" || acTok != dnsTok {
		t.Fatalf("every target must receive the same token: %q vs %q", acTok, dnsTok)
	}
	if !strings.Contains(out, "dnscheck/team-a/cluster-dns (via healthcheck/team-a/dns)") {
		t.Fatalf("fan-out output missing DNS source:\n%s", out)
	}
}

func TestRun_DerivedEdgeCases(t *testing.T) {
	pausedHC := healthCheckFor("team-a", "paused", "AddonCheck", "coredns")
	pausedHC.Spec.Paused = true
	emptyCH := &fathomv1alpha1.ClusterHealth{ObjectMeta: metav1.ObjectMeta{Name: "empty"}}
	partialCH := &fathomv1alpha1.ClusterHealth{
		ObjectMeta: metav1.ObjectMeta{Name: "partial"},
		Status: fathomv1alpha1.ClusterHealthStatus{Children: []fathomv1alpha1.ClusterHealthChildSummary{
			{Namespace: "team-a", Name: "gone"}, {Namespace: "team-a", Name: "paused"}, {Namespace: "team-a", Name: "orphan"}, {Namespace: "team-a", Name: "ok"},
		}},
	}
	f, fc := fakeFactory(t,
		addonCheck("team-a", "coredns"), pausedHC, emptyCH, partialCH,
		healthCheckFor("team-a", "orphan", "AddonCheck", "missing-source"),
		healthCheckFor("team-a", "ok", "AddonCheck", "coredns"),
	)

	if _, _, err := execRun(f, "healthcheck/paused"); err == nil || !strings.Contains(err.Error(), "is paused") {
		t.Errorf("paused HealthCheck should be refused: %v", err)
	}
	if _, _, err := execRun(f, "clusterhealth/empty"); err == nil || !strings.Contains(err.Error(), "selects no HealthChecks") {
		t.Errorf("empty ClusterHealth should be refused: %v", err)
	}

	out, errOut, err := execRun(f, "clusterhealth/partial")
	if err == nil || !strings.Contains(err.Error(), "did not succeed") {
		t.Fatalf("skipped sources must make the exit non-zero: %v", err)
	}
	for _, want := range []string{"HealthCheck not found", "paused HealthCheck", "not found"} {
		if !strings.Contains(errOut+out, want) {
			t.Errorf("expected %q in output:\n%s%s", want, errOut, out)
		}
	}
	if tok := annotationOf(t, fc, &fathomv1alpha1.AddonCheck{}, "team-a", "coredns"); tok == "" {
		t.Fatalf("the reachable source must still be triggered")
	}
}

func TestRun_PausedTargetIsNotWritten(t *testing.T) {
	ac := addonCheck("team-a", "paused")
	ac.Spec.Paused = true
	f, fc := fakeFactory(t, ac)
	_, errOut, err := execRun(f, "addoncheck/paused")
	if err == nil || !strings.Contains(err.Error(), "no checks to run") {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(errOut, "paused") {
		t.Fatalf("reason not printed:\n%s", errOut)
	}
	if tok := annotationOf(t, fc, &fathomv1alpha1.AddonCheck{}, "team-a", "paused"); tok != "" {
		t.Fatalf("paused check must not be written, got %q", tok)
	}
}

func TestRun_SelectorAndAll(t *testing.T) {
	labelled := func(ns, name string) *fathomv1alpha1.AddonCheck {
		ac := addonCheck(ns, name)
		ac.Labels = map[string]string{"tier": "core"}
		return ac
	}
	f, _ := fakeFactory(t,
		labelled("team-a", "a1"), labelled("team-a", "a2"), addonCheck("team-a", "a3"),
		&fathomv1alpha1.DNSCheck{ObjectMeta: metav1.ObjectMeta{Name: "d1", Namespace: "team-a"}},
		addonCheck("team-b", "b1"),
		healthCheckFor("team-a", "hc", "AddonCheck", "a1"),
	)

	_, errOut, err := execRun(f, "-l", "tier=core")
	if err != nil || !strings.Contains(errOut, "Triggering 2 check(s)") {
		t.Fatalf("selector: err=%v\n%s", err, errOut)
	}
	_, errOut, err = execRun(f, "--all")
	if err != nil || !strings.Contains(errOut, "Triggering 4 check(s)") {
		t.Fatalf("--all in namespace should select the 3 AddonChecks and 1 DNSCheck, never the HealthCheck: err=%v\n%s", err, errOut)
	}
	_, errOut, err = execRun(f, "--all", "-A")
	if err != nil || !strings.Contains(errOut, "Triggering 5 check(s)") {
		t.Fatalf("--all -A: err=%v\n%s", err, errOut)
	}
	if _, _, err := execRun(f, "-l", "!!bad"); err == nil || !strings.Contains(err.Error(), "invalid selector") {
		t.Fatalf("bad selector error = %v", err)
	}
	if _, _, err := execRun(f, "-l", "tier=nothing"); err == nil || !strings.Contains(err.Error(), "no checks to run") {
		t.Fatalf("empty selection error = %v", err)
	}
}

func TestRun_BulkConfirmation(t *testing.T) {
	var objs []client.Object
	for i := range bulkConfirmThreshold + 1 {
		objs = append(objs, addonCheck("team-a", fmt.Sprintf("ac-%02d", i)))
	}
	countTriggered := func(fc client.Client) int {
		n := 0
		list := &fathomv1alpha1.AddonCheckList{}
		_ = fc.List(context.Background(), list)
		for _, it := range list.Items {
			if it.Annotations[fathomv1alpha1.AnnotationRunNow] != "" {
				n++
			}
		}
		return n
	}

	t.Run("no terminal and no --yes refuses before writing", func(t *testing.T) {
		f, fc := fakeFactory(t, objs...)
		_, _, err := execRun(f, "--all")
		if err == nil || !strings.Contains(err.Error(), "--yes") {
			t.Fatalf("error = %v", err)
		}
		if countTriggered(fc) != 0 {
			t.Fatal("nothing may be written before confirmation")
		}
	})
	t.Run("terminal answer no aborts", func(t *testing.T) {
		f, fc := fakeFactory(t, objs...)
		f.isTerminal = func() bool { return true }
		f.stdin = strings.NewReader("n\n")
		_, errOut, err := execRun(f, "--all")
		if err == nil || !strings.Contains(err.Error(), "not confirmed") || !strings.Contains(errOut, "Continue? [y/N]") {
			t.Fatalf("error = %v\n%s", err, errOut)
		}
		if countTriggered(fc) != 0 {
			t.Fatal("declined confirmation must write nothing")
		}
	})
	t.Run("terminal answer yes proceeds", func(t *testing.T) {
		f, fc := fakeFactory(t, objs...)
		f.isTerminal = func() bool { return true }
		f.stdin = strings.NewReader("yes\n")
		if _, _, err := execRun(f, "--all"); err != nil {
			t.Fatalf("run: %v", err)
		}
		if got := countTriggered(fc); got != len(objs) {
			t.Fatalf("triggered %d, want %d", got, len(objs))
		}
	})
	t.Run("--yes skips the prompt", func(t *testing.T) {
		f, fc := fakeFactory(t, objs...)
		if _, errOut, err := execRun(f, "--all", "--yes"); err != nil || strings.Contains(errOut, "Continue?") {
			t.Fatalf("err=%v\n%s", err, errOut)
		}
		if got := countTriggered(fc); got != len(objs) {
			t.Fatalf("triggered %d, want %d", got, len(objs))
		}
	})
	t.Run("--dry-run lists and writes nothing", func(t *testing.T) {
		f, fc := fakeFactory(t, objs...)
		out, _, err := execRun(f, "--all", "--dry-run")
		if err != nil || !strings.Contains(out, fmt.Sprintf("Would trigger %d check(s)", len(objs))) || !strings.Contains(out, "addoncheck/team-a/ac-00") {
			t.Fatalf("err=%v\n%s", err, out)
		}
		if countTriggered(fc) != 0 {
			t.Fatal("--dry-run must write nothing")
		}
	})
}

func TestRun_WaitVerdictsAndExitCodes(t *testing.T) {
	tests := []struct {
		verdict string
		wantErr bool
	}{
		{"Pass", false}, {"Warn", false}, {"Skipped", false},
		{"Fail", true}, {"Error", true}, {"Unknown", true},
	}
	for _, tt := range tests {
		t.Run(tt.verdict, func(t *testing.T) {
			f, fc := fakeFactory(t, addonCheck("team-a", "coredns"))
			completeRun(t, fc, types.NamespacedName{Namespace: "team-a", Name: "coredns"}, tt.verdict)
			out, _, err := execRun(f, "addoncheck/coredns", "--wait", "--timeout", "3s")
			if (err != nil) != tt.wantErr {
				t.Fatalf("verdict %s: err = %v, wantErr %v\n%s", tt.verdict, err, tt.wantErr, out)
			}
			if !strings.Contains(out, tt.verdict) || !strings.Contains(out, "adapter ran") {
				t.Fatalf("verdict and summary must be printed:\n%s", out)
			}
		})
	}
}

func TestRun_WaitSupersededAndTimeout(t *testing.T) {
	t.Run("superseded", func(t *testing.T) {
		f, fc := fakeFactory(t, addonCheck("team-a", "coredns"))
		go func() {
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				ac := &fathomv1alpha1.AddonCheck{}
				if err := fc.Get(context.Background(), types.NamespacedName{Namespace: "team-a", Name: "coredns"}, ac); err == nil && ac.Annotations[fathomv1alpha1.AnnotationRunNow] != "" {
					ac.Annotations[fathomv1alpha1.AnnotationRunNow] = "someone-else"
					_ = fc.Update(context.Background(), ac)
					return
				}
				time.Sleep(2 * time.Millisecond)
			}
		}()
		out, _, err := execRun(f, "addoncheck/coredns", "--wait", "--timeout", "3s")
		if err == nil || !strings.Contains(out, "superseded") {
			t.Fatalf("err=%v\n%s", err, out)
		}
	})
	t.Run("timeout carries hints", func(t *testing.T) {
		f, _ := fakeFactory(t, addonCheck("team-a", "coredns"))
		out, _, err := execRun(f, "addoncheck/coredns", "--wait", "--timeout", "50ms")
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("err=%v", err)
		}
		for _, hint := range []string{"timed out", "fathomctl version", "paused", "node-agent rollout"} {
			if !strings.Contains(out, hint) {
				t.Errorf("timeout output should mention %q:\n%s", hint, out)
			}
		}
	})
}

func TestRun_DefaultWaitTimeoutUsesLargestCheckTimeout(t *testing.T) {
	short := addonCheck("team-a", "short")
	long := addonCheck("team-a", "long")
	long.Spec.Timeout = &metav1.Duration{Duration: 2 * time.Minute}
	targets := []runTarget{addonTarget("team-a", "short", short), addonTarget("team-a", "long", long)}
	var got time.Duration
	for _, tr := range targets {
		got = max(got, tr.ref.Kind.DefaultTimeout(tr.obj)+waitMargin)
	}
	if got != 2*time.Minute+waitMargin {
		t.Fatalf("default wait = %s, want 2m30s", got)
	}
}

func TestRun_StructuredOutput(t *testing.T) {
	f, fc := fakeFactory(t, addonCheck("team-a", "coredns"))
	completeRun(t, fc, types.NamespacedName{Namespace: "team-a", Name: "coredns"}, "Pass")
	out, _, err := execRun(f, "addoncheck/coredns", "--wait", "--timeout", "3s", "-o", "json")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var outcomes []runOutcome
	if err := json.Unmarshal([]byte(out), &outcomes); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(outcomes) != 1 || !outcomes[0].Triggered || outcomes[0].Verdict != "Pass" || outcomes[0].Token == "" {
		t.Fatalf("outcomes = %+v", outcomes)
	}
}

func TestVerdictPasses(t *testing.T) {
	for v, want := range map[string]bool{"Pass": true, "Warn": true, "Skipped": true, "Fail": false, "Error": false, "Unknown": false, "": false} {
		if verdictPasses(v) != want {
			t.Errorf("verdictPasses(%q) = %v, want %v", v, !want, want)
		}
	}
}
