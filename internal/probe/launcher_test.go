/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package probe

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(&corev1.Pod{}).
		WithObjects(objs...).
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

// TestLauncherRun_TerminatedThenDeletedReturnsResult exercises the SKA-429
// race: kubelet writes the terminated container state (with a non-empty
// termination message) before transitioning the pod's overall phase to
// Succeeded, the pod is deleted between polls, and the launcher's next
// Get returns NotFound. The launcher must promote the captured observation
// — the probe finished and we have its result — rather than surface the
// disappearance as a terminal error.
func TestLauncherRun_TerminatedThenDeletedReturnsResult(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add to scheme: %v", err)
	}
	var getCount atomic.Int32
	intercepted := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&corev1.Pod{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				pod, isPod := obj.(*corev1.Pod)
				if !isPod {
					return cl.Get(ctx, key, obj, opts...)
				}
				switch getCount.Add(1) {
				case 1:
					// First poll: container has terminated with a non-empty
					// termination message but the pod's phase is still
					// Running (kubelet hasn't propagated the phase change).
					pod.ObjectMeta.Name = key.Name
					pod.ObjectMeta.Namespace = key.Namespace
					pod.Status = corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{{
							Name: "probe",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 0,
									Reason:   "Completed",
									Message:  `{"outcome":"Pass","summary":"DNS resolution succeeded"}`,
								},
							},
						}},
					}
					return nil
				default:
					// Subsequent polls: pod has been deleted (kubelet GC,
					// eviction, ttl controller) before phase reached
					// Succeeded.
					return apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, key.Name)
				}
			},
		}).
		Build()

	l := &Launcher{Client: intercepted, PollInterval: 5 * time.Millisecond}
	req := dnsRequest("probe-vanish-after-terminal")

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

// TestLauncherRun_TransientNotFoundIsTolerated mimics the most likely
// root cause of SKA-429: the client's cache hasn't propagated the Create
// before the first poll fires, so the initial Get returns NotFound. After
// a small number of polls the cache catches up and the pod is observable.
// The launcher must tolerate the transient NotFound and complete normally.
func TestLauncherRun_TransientNotFoundIsTolerated(t *testing.T) {
	c := newFakeClient(t)
	var notFoundsRemaining atomic.Int32
	notFoundsRemaining.Store(2) // tolerate at least 1, drop below tolerance
	intercepted := fake.NewClientBuilder().
		WithScheme(c.Scheme()).
		WithStatusSubresource(&corev1.Pod{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, isPod := obj.(*corev1.Pod); isPod && notFoundsRemaining.Load() > 0 {
					notFoundsRemaining.Add(-1)
					return apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, key.Name)
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	l := &Launcher{Client: intercepted, PollInterval: 5 * time.Millisecond}
	req := dnsRequest("probe-cache-lag")

	simulateKubelet(t, intercepted, req.Name, req.Namespace, corev1.PodSucceeded,
		`{"outcome":"Pass","summary":"DNS resolution succeeded"}`, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := l.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Outcome != OutcomePass {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomePass)
	}
}

// TestLauncherRun_PersistentNotFoundFails confirms the tolerance has an
// upper bound: if Get keeps returning NotFound and we never observed the
// pod, the launcher must surface the disappearance after the tolerance
// window expires, not block until the deadline.
func TestLauncherRun_PersistentNotFoundFails(t *testing.T) {
	c := newFakeClient(t)
	intercepted := fake.NewClientBuilder().
		WithScheme(c.Scheme()).
		WithStatusSubresource(&corev1.Pod{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, isPod := obj.(*corev1.Pod); isPod {
					return apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, key.Name)
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	l := &Launcher{Client: intercepted, PollInterval: 5 * time.Millisecond}
	req := dnsRequest("probe-truly-gone")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := l.Run(ctx, req)
	if err == nil {
		t.Fatal("Run: want disappeared error, got nil")
	}
	if !strings.Contains(err.Error(), "disappeared during poll") {
		t.Errorf("Run: error %q should mention disappearance", err.Error())
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

// TestLauncherRun_LaunchFailuresAreTyped pins the LaunchError contract: pod
// build and create failures are classifiable with errors.As (the AddonCheck
// controller maps them to the ProbeLaunchFailed event reason), while
// post-launch failures stay untyped.
func TestLauncherRun_LaunchFailuresAreTyped(t *testing.T) {
	c := newFakeClient(t)
	l := &Launcher{Client: c, PollInterval: 5 * time.Millisecond}

	// Build failure: missing Image.
	_, err := l.Run(context.Background(), Request{Name: "x", Namespace: "default", Mode: ModeDNS, Target: "example.com"})
	var launchErr *LaunchError
	if !errors.As(err, &launchErr) {
		t.Errorf("build failure: want LaunchError, got %T: %v", err, err)
	}

	// Create failure: name collision.
	req := dnsRequest("probe-typed-collide")
	preexisting, podErr := Pod(req)
	if podErr != nil {
		t.Fatalf("Pod: %v", podErr)
	}
	if err := c.Create(context.Background(), preexisting); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err = l.Run(context.Background(), req)
	launchErr = nil
	if !errors.As(err, &launchErr) {
		t.Errorf("create failure: want LaunchError, got %T: %v", err, err)
	}
	if launchErr != nil && !apierrors.IsAlreadyExists(launchErr.Err) && !strings.Contains(launchErr.Error(), "create probe pod") {
		t.Errorf("LaunchError should preserve the wrapped cause, got %v", launchErr)
	}

	// Post-launch failure (nil client is pre-launch config, but stays untyped:
	// it is not a pod-launch fault).
	_, err = (&Launcher{}).Run(context.Background(), dnsRequest("probe-untyped"))
	launchErr = nil
	if errors.As(err, &launchErr) {
		t.Errorf("nil-client failure must not be a LaunchError, got %v", err)
	}
}
