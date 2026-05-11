/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package probe

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// defaultPollInterval bounds how often the launcher Gets the probe pod while
// waiting for completion. Probes typically finish in milliseconds to seconds;
// 500ms keeps end-to-end latency low without flooding the API server.
const defaultPollInterval = 500 * time.Millisecond

// pollHeadroom is the slack added to the probe's own Timeout when computing
// the launcher's wait deadline. It covers pod scheduling, image pull, and
// the probe binary's own ActiveDeadlineSeconds margin (timeout + 5s, set in
// Pod()). The launcher must outlast both, otherwise it would abandon a
// probe that the kubelet would have killed and reported on momentarily.
const pollHeadroom = 60 * time.Second

// cleanupTimeout bounds the best-effort delete that runs even after the
// caller's context is cancelled. Long enough for the API call to complete
// under typical load, short enough not to wedge a shutting-down operator.
const cleanupTimeout = 5 * time.Second

// Launcher creates a hardened probe pod (per Pod()), waits for it to reach a
// terminal phase, parses the JSON Result the probe binary wrote to its
// termination message, and then deletes the pod. It is the lifecycle layer
// over the manifest builder; adapters (CoreDNS dns_resolution, future
// DNSCheck/ReachabilityCheck) call Run rather than reaching for Pod()
// directly.
//
// Callers must have RBAC for pods (create;get;list;watch;delete) in the
// target namespace. The marker lives on the controller that consumes a
// Launcher, not on Launcher itself, because the same Launcher could be
// invoked from several controllers.
type Launcher struct {
	// Client is the controller-runtime client used to manage the probe pod.
	Client client.Client

	// PollInterval overrides the default 500ms poll cadence. Mostly useful in
	// tests; production callers should leave it zero.
	PollInterval time.Duration
}

// Run launches a probe pod for req, waits for it to terminate, and returns
// the parsed Result. The pod is always deleted before Run returns, even on
// error and even if ctx is cancelled mid-poll.
//
// Outcomes:
//
//   - The probe ran and produced a JSON Result on its termination message:
//     ParseResult is returned (Outcome reflects the probe's own assessment).
//   - The pod terminated but produced no usable termination message:
//     Result{Outcome: OutcomeError} with a descriptive Summary; nil error.
//   - The pod could not be created or the API server returned an error
//     while polling: non-nil error.
//   - ctx was cancelled while waiting: Result zero, ctx.Err().
func (l *Launcher) Run(ctx context.Context, req Request) (Result, error) {
	if l.Client == nil {
		return Result{}, errors.New("probe launcher: Client is nil")
	}
	pod, err := Pod(req)
	if err != nil {
		return Result{}, fmt.Errorf("build probe pod: %w", err)
	}
	if err := l.Client.Create(ctx, pod); err != nil {
		return Result{}, fmt.Errorf("create probe pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	defer l.bestEffortDelete(pod)

	completed, waitErr := l.waitForCompletion(ctx, pod, req.Timeout)
	if waitErr != nil {
		return Result{}, waitErr
	}
	return extractResult(completed)
}

// waitForCompletion polls the probe pod until it reaches Succeeded or Failed
// or the deadline expires. It returns the latest Pod state observed.
func (l *Launcher) waitForCompletion(ctx context.Context, pod *corev1.Pod, probeTimeout time.Duration) (*corev1.Pod, error) {
	interval := l.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	deadline := probeTimeout + pollHeadroom
	if deadline <= 0 {
		deadline = pollHeadroom
	}
	pollCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	key := client.ObjectKeyFromObject(pod)
	var observed corev1.Pod
	err := wait.PollUntilContextCancel(pollCtx, interval, true, func(ctx context.Context) (bool, error) {
		if err := l.Client.Get(ctx, key, &observed); err != nil {
			if apierrors.IsNotFound(err) {
				// Someone (or something) deleted the pod out from under us.
				// Treat as a terminal not-found so the caller can surface it
				// rather than spinning until the deadline.
				return false, fmt.Errorf("probe pod %s/%s disappeared during poll", pod.Namespace, pod.Name)
			}
			return false, err
		}
		switch observed.Status.Phase {
		case corev1.PodSucceeded, corev1.PodFailed:
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("wait for probe pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	return &observed, nil
}

// extractResult reads the probe's JSON Result from the first container's
// termination message. terminationMessagePolicy=FallbackToLogsOnError on
// the pod spec means the kubelet copies the tail of stdout into the
// termination message when the container fails without writing one — so a
// failing probe still surfaces some signal here.
func extractResult(pod *corev1.Pod) (Result, error) {
	if len(pod.Status.ContainerStatuses) == 0 {
		return Result{
			Outcome: OutcomeError,
			Summary: fmt.Sprintf("probe pod %s/%s has no container statuses", pod.Namespace, pod.Name),
		}, nil
	}
	cs := pod.Status.ContainerStatuses[0]
	if cs.State.Terminated == nil {
		return Result{
			Outcome: OutcomeError,
			Summary: fmt.Sprintf("probe pod %s/%s container %s is not terminated", pod.Namespace, pod.Name, cs.Name),
		}, nil
	}
	msg := cs.State.Terminated.Message
	if msg == "" {
		return Result{
			Outcome: OutcomeError,
			Summary: fmt.Sprintf("probe pod %s/%s produced no termination message (exitCode=%d, reason=%s)",
				pod.Namespace, pod.Name, cs.State.Terminated.ExitCode, cs.State.Terminated.Reason),
		}, nil
	}
	return ParseResult(msg)
}

// bestEffortDelete removes the probe pod after Run completes. It uses a
// fresh context so cancellation of the caller's context does not skip the
// delete; kubelets eventually clean up orphaned pods, but explicit cleanup
// is the contract.
func (l *Launcher) bestEffortDelete(pod *corev1.Pod) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	background := metav1.DeletePropagationBackground
	_ = l.Client.Delete(ctx, pod, &client.DeleteOptions{PropagationPolicy: &background})
}
