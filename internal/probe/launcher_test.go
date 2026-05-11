/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package probe

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newFakeClient(t *testing.T) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add to scheme: %v", err)
	}
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&corev1.Pod{}).
		Build()
}

// simulateKubelet polls for the named pod, then patches its status to mimic
// a kubelet completing the container with the supplied phase + termination
// message. It runs once and exits. Cancel ctx to stop early.
func simulateKubelet(t *testing.T, c client.Client, name, namespace string, phase corev1.PodPhase, message string, exitCode int32) {
	t.Helper()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var pod corev1.Pod
		key := client.ObjectKey{Namespace: namespace, Name: name}
		for {
			if ctx.Err() != nil {
				return
			}
			err := c.Get(ctx, key, &pod)
			if err == nil {
				break
			}
			if !apierrors.IsNotFound(err) {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		pod.Status.Phase = phase
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name: "probe",
			State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					ExitCode: exitCode,
					Reason:   string(phase),
					Message:  message,
				},
			},
		}}
		_ = c.Status().Update(ctx, &pod)
	}()
}

const validRequestImage = "example.com/fathom-probe:test"

func dnsRequest(name string) Request {
	return Request{
		Name:      name,
		Namespace: "default",
		Image:     validRequestImage,
		Mode:      ModeDNS,
		Target:    "kubernetes.default.svc",
		Timeout:   1 * time.Second,
	}
}

func TestLauncherRun_ParsesPassResult(t *testing.T) {
	c := newFakeClient(t)
	l := &Launcher{Client: c, PollInterval: 5 * time.Millisecond}
	req := dnsRequest("probe-pass")

	simulateKubelet(t, c, req.Name, req.Namespace, corev1.PodSucceeded,
		`{"outcome":"Pass","summary":"DNS resolution succeeded","details":{"target":"kubernetes.default.svc"}}`, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := l.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Outcome != OutcomePass {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomePass)
	}
	if !strings.Contains(result.Summary, "DNS resolution succeeded") {
		t.Errorf("Summary = %q", result.Summary)
	}
}

func TestLauncherRun_DeletesPodAfterRun(t *testing.T) {
	c := newFakeClient(t)
	l := &Launcher{Client: c, PollInterval: 5 * time.Millisecond}
	req := dnsRequest("probe-cleanup")

	simulateKubelet(t, c, req.Name, req.Namespace, corev1.PodSucceeded,
		`{"outcome":"Pass","summary":"ok"}`, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := l.Run(ctx, req); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// bestEffortDelete uses its own context, so we need to give it a moment
	// to complete before asserting.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := c.Get(context.Background(), client.ObjectKey{Namespace: req.Namespace, Name: req.Name}, &corev1.Pod{})
		if apierrors.IsNotFound(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("probe pod %q was not deleted within 2s", req.Name)
}

func TestLauncherRun_FailedPhasePropagatesProbeJSON(t *testing.T) {
	// Probe binary writes a Fail outcome and exits non-zero. Pod phase = Failed.
	// The launcher should still parse the JSON and return Outcome=Fail.
	c := newFakeClient(t)
	l := &Launcher{Client: c, PollInterval: 5 * time.Millisecond}
	req := dnsRequest("probe-fail")

	simulateKubelet(t, c, req.Name, req.Namespace, corev1.PodFailed,
		`{"outcome":"Fail","summary":"DNS resolution returned no addresses"}`, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := l.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Outcome != OutcomeFail {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeFail)
	}
}

func TestLauncherRun_EmptyTerminationMessageIsError(t *testing.T) {
	c := newFakeClient(t)
	l := &Launcher{Client: c, PollInterval: 5 * time.Millisecond}
	req := dnsRequest("probe-empty-message")

	simulateKubelet(t, c, req.Name, req.Namespace, corev1.PodFailed, "", 137)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := l.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Outcome != OutcomeError {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeError)
	}
	if !strings.Contains(result.Summary, "no termination message") {
		t.Errorf("Summary = %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "exitCode=137") {
		t.Errorf("Summary should include exitCode 137: %q", result.Summary)
	}
}

func TestLauncherRun_PollDeadlineExceededReturnsError(t *testing.T) {
	// No kubelet simulator: pod stays Pending forever. The launcher's
	// Timeout+pollHeadroom bound should fire and surface a wait error.
	// Use a tiny request Timeout so the test finishes promptly.
	c := newFakeClient(t)
	l := &Launcher{Client: c, PollInterval: 5 * time.Millisecond}
	req := dnsRequest("probe-no-kubelet")
	req.Timeout = 0 // launcher uses pollHeadroom alone (60s) — too long

	// Override pollHeadroom for this test by capping context deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := l.Run(ctx, req)
	if err == nil {
		t.Fatalf("Run: want error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline") {
		t.Errorf("Run: error %q should reflect deadline exceeded", err.Error())
	}
}

func TestLauncherRun_PodCreateFailureReturnsError(t *testing.T) {
	c := newFakeClient(t)
	l := &Launcher{Client: c, PollInterval: 5 * time.Millisecond}
	req := dnsRequest("probe-collide")

	// Pre-create a Pod with the same name so Create returns AlreadyExists.
	preexisting, err := Pod(req)
	if err != nil {
		t.Fatalf("Pod: %v", err)
	}
	if err := c.Create(context.Background(), preexisting); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err = l.Run(ctx, req)
	if err == nil {
		t.Fatal("Run: want error from Create collision, got nil")
	}
	if !strings.Contains(err.Error(), "create probe pod") {
		t.Errorf("Run: error %q should name the create step", err.Error())
	}
}

func TestLauncherRun_NilClientReturnsError(t *testing.T) {
	l := &Launcher{}
	_, err := l.Run(context.Background(), dnsRequest("probe-no-client"))
	if err == nil || !strings.Contains(err.Error(), "Client is nil") {
		t.Fatalf("Run: want 'Client is nil' error, got %v", err)
	}
}

func TestLauncherRun_InvalidRequestReturnsError(t *testing.T) {
	c := newFakeClient(t)
	l := &Launcher{Client: c, PollInterval: 5 * time.Millisecond}
	// Missing Image — Pod() should reject before Create is attempted.
	bad := Request{Name: "x", Namespace: "default", Mode: ModeDNS, Target: "example.com"}
	_, err := l.Run(context.Background(), bad)
	if err == nil || !strings.Contains(err.Error(), "build probe pod") {
		t.Fatalf("Run: want build error, got %v", err)
	}
}

// TestLauncherRun_ConcurrentRunsAreIndependent exercises the launcher with
// two concurrent Runs against distinct probe pods to confirm there's no
// shared mutable state in Launcher itself.
func TestLauncherRun_ConcurrentRunsAreIndependent(t *testing.T) {
	c := newFakeClient(t)
	l := &Launcher{Client: c, PollInterval: 5 * time.Millisecond}

	simulateKubelet(t, c, "concurrent-a", "default", corev1.PodSucceeded, `{"outcome":"Pass","summary":"a"}`, 0)
	simulateKubelet(t, c, "concurrent-b", "default", corev1.PodSucceeded, `{"outcome":"Fail","summary":"b"}`, 1)

	var wg sync.WaitGroup
	results := make(map[string]Result)
	var mu sync.Mutex
	run := func(name string) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req := dnsRequest(name)
		r, err := l.Run(ctx, req)
		if err != nil {
			t.Errorf("Run(%s): %v", name, err)
			return
		}
		mu.Lock()
		results[name] = r
		mu.Unlock()
	}
	wg.Add(2)
	go run("concurrent-a")
	go run("concurrent-b")
	wg.Wait()

	if results["concurrent-a"].Outcome != OutcomePass {
		t.Errorf("concurrent-a outcome = %q, want Pass", results["concurrent-a"].Outcome)
	}
	if results["concurrent-b"].Outcome != OutcomeFail {
		t.Errorf("concurrent-b outcome = %q, want Fail", results["concurrent-b"].Outcome)
	}
}
