/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package probe

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// sweepPod builds a pod for sweeper table tests. age is subtracted from now
// to produce the CreationTimestamp.
// Pods are probe-shaped by default so these tests exercise the label, phase,
// and age rules; the shape check itself is covered separately.
func sweepPod(name string, podLabels map[string]string, phase corev1.PodPhase, age time.Duration) *corev1.Pod {
	return probeShape(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			Labels:            podLabels,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
		},
		Status: corev1.PodStatus{Phase: phase},
	})
}

func probeLabels(probeName string) map[string]string {
	return map[string]string{labelManagedBy: managedByValue, labelProbeName: probeName}
}

// probeShape gives a pod the structural fingerprint Pod() produces, so that
// sweeper tests exercise the age/phase/label rules rather than tripping the
// shape check.
func probeShape(pod *corev1.Pod) *corev1.Pod {
	automount := false
	pod.Spec.RestartPolicy = corev1.RestartPolicyNever
	pod.Spec.AutomountServiceAccountToken = &automount
	pod.Spec.Containers = []corev1.Container{{Name: probeContainerName}}
	return pod
}

// A pod that merely wears the probe labels must not be deleted. The operator
// holds cluster-wide pod delete, so honouring labels alone would let anyone
// with label-patch access borrow that permission against a pod they cannot
// delete themselves.
func TestSweeper_LabelledPodNotMatchingProbeShapeIsSpared(t *testing.T) {
	automount := true
	victims := map[string]func(*corev1.Pod){
		"wrong restart policy": func(p *corev1.Pod) {
			p.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
		},
		"service account token mounted": func(p *corev1.Pod) {
			p.Spec.AutomountServiceAccountToken = &automount
		},
		"automount unset": func(p *corev1.Pod) {
			p.Spec.AutomountServiceAccountToken = nil
		},
		"extra container": func(p *corev1.Pod) {
			p.Spec.Containers = append(p.Spec.Containers, corev1.Container{Name: "sidecar"})
		},
		"container not named probe": func(p *corev1.Pod) {
			p.Spec.Containers = []corev1.Container{{Name: "app"}}
		},
	}
	for name, mutate := range victims {
		t.Run(name, func(t *testing.T) {
			// Labelled, terminal, and long past the grace period: reapable on
			// every axis except shape.
			pod := probeShape(sweepPod("victim", probeLabels("victim"), corev1.PodSucceeded, time.Hour))
			mutate(pod)

			c := newFakeClient(t, pod)
			s := &Sweeper{Reader: c, Client: c}
			s.sweep(context.Background())

			var observed corev1.Pod
			err := c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "victim"}, &observed)
			if err != nil {
				t.Fatalf("pod not matching probe shape was deleted: %v", err)
			}
		})
	}
}

// terminatedAt stamps a container termination time on a pod, modelling a
// probe that ran to completion rather than one whose status we never saw.
func terminatedAt(pod *corev1.Pod, since time.Duration) *corev1.Pod {
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "probe",
		State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				FinishedAt: metav1.NewTime(time.Now().Add(-since)),
			},
		},
	}}
	return pod
}

// A long-timeout probe is older than MinAge at the moment it turns terminal,
// so sweeping on creation age would delete it out from under the launcher
// that is still polling for its termination message. Age must be measured
// from termination, not creation.
func TestSweeper_LongRunningProbeIsNotReapedOnCreationAge(t *testing.T) {
	// Created 10m ago (a 10m spec.timeout), but only just terminated.
	pod := terminatedAt(
		sweepPod("long-probe", probeLabels("long-probe"), corev1.PodSucceeded, 10*time.Minute),
		time.Second,
	)
	c := newFakeClient(t, pod)
	s := &Sweeper{Reader: c, Client: c}
	s.sweep(context.Background())

	var observed corev1.Pod
	err := c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: pod.Name}, &observed)
	if err != nil {
		t.Fatalf("pod terminated 1s ago was reaped despite an in-flight launcher: %v", err)
	}
}

// Once the termination itself is older than MinAge, no launcher can still be
// waiting on it, so the pod is a genuine orphan.
func TestSweeper_ReapsPodTerminatedLongerThanMinAge(t *testing.T) {
	pod := terminatedAt(
		sweepPod("stale-probe", probeLabels("stale-probe"), corev1.PodSucceeded, 10*time.Minute),
		30*time.Minute,
	)
	c := newFakeClient(t, pod)
	s := &Sweeper{Reader: c, Client: c}
	s.sweep(context.Background())

	var observed corev1.Pod
	err := c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: pod.Name}, &observed)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected long-terminated orphan to be reaped, got err=%v", err)
	}
}

func TestSweeper_Sweep(t *testing.T) {
	old := time.Hour
	fresh := time.Second

	tests := []struct {
		name     string
		pod      *corev1.Pod
		survives bool
	}{
		{
			name:     "old failed probe pod is reaped",
			pod:      sweepPod("orphan-failed", probeLabels("orphan-failed"), corev1.PodFailed, old),
			survives: false,
		},
		{
			name:     "old succeeded probe pod is reaped",
			pod:      sweepPod("orphan-succeeded", probeLabels("orphan-succeeded"), corev1.PodSucceeded, old),
			survives: false,
		},
		{
			name:     "young terminal probe pod is left for its launcher",
			pod:      sweepPod("in-flight", probeLabels("in-flight"), corev1.PodSucceeded, fresh),
			survives: true,
		},
		{
			name:     "running probe pod is left for ActiveDeadlineSeconds",
			pod:      sweepPod("still-running", probeLabels("still-running"), corev1.PodRunning, old),
			survives: true,
		},
		{
			name:     "pending probe pod is left alone",
			pod:      sweepPod("still-pending", probeLabels("still-pending"), corev1.PodPending, old),
			survives: true,
		},
		{
			name: "managed-by pod without the probe label is not a probe",
			pod: sweepPod("node-agent-pod", map[string]string{labelManagedBy: managedByValue},
				corev1.PodFailed, old),
			survives: true,
		},
		{
			name:     "unlabeled terminal pod is untouched",
			pod:      sweepPod("bystander", nil, corev1.PodFailed, old),
			survives: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newFakeClient(t, tc.pod)
			s := &Sweeper{Reader: c, Client: c}
			s.sweep(context.Background())

			var observed corev1.Pod
			err := c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: tc.pod.Name}, &observed)
			if tc.survives && err != nil {
				t.Fatalf("pod should have survived the sweep: %v", err)
			}
			if !tc.survives && !apierrors.IsNotFound(err) {
				t.Fatalf("pod should have been deleted, got err=%v", err)
			}
		})
	}
}

func TestSweeper_SweepToleratesListFailure(t *testing.T) {
	base := newFakeClient(t)
	failing := fake.NewClientBuilder().
		WithScheme(base.Scheme()).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errors.New("synthetic list failure")
			},
		}).
		Build()
	s := &Sweeper{Reader: failing, Client: failing}
	// Must not panic; the failure is logged and retried on the next tick.
	s.sweep(context.Background())
}

func TestSweeper_SweepToleratesDeleteFailure(t *testing.T) {
	reapable := sweepPod("orphan-a", probeLabels("orphan-a"), corev1.PodFailed, time.Hour)
	other := sweepPod("orphan-b", probeLabels("orphan-b"), corev1.PodFailed, time.Hour)
	c := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(reapable, other).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if obj.GetName() == "orphan-a" {
					return errors.New("synthetic delete failure")
				}
				return cl.Delete(ctx, obj, opts...)
			},
		}).
		Build()
	s := &Sweeper{Reader: c, Client: c}
	s.sweep(context.Background())

	// The failed delete must not stop the sweep from reaping the rest.
	var observed corev1.Pod
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "orphan-b"}, &observed); !apierrors.IsNotFound(err) {
		t.Fatalf("orphan-b should have been deleted despite orphan-a failing, got err=%v", err)
	}
}

func TestSweeper_StartRequiresClients(t *testing.T) {
	if err := (&Sweeper{}).Start(context.Background()); err == nil {
		t.Fatal("expected error for nil Reader/Client")
	}
}

func TestSweeper_StartSweepsImmediatelyAndStopsOnCancel(t *testing.T) {
	orphan := sweepPod("orphan", probeLabels("orphan"), corev1.PodFailed, time.Hour)
	c := newFakeClient(t, orphan)
	s := &Sweeper{Reader: c, Client: c, Interval: time.Hour}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// The startup sweep runs before the first tick, so the orphan disappears
	// well before Interval elapses.
	deadline := time.After(5 * time.Second)
	for {
		var observed corev1.Pod
		err := c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "orphan"}, &observed)
		if apierrors.IsNotFound(err) {
			break
		}
		select {
		case <-deadline:
			t.Fatal("startup sweep never deleted the orphan")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error on cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

func TestSweeper_NeedLeaderElection(t *testing.T) {
	if !(&Sweeper{}).NeedLeaderElection() {
		t.Fatal("Sweeper must only run on the elected leader")
	}
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add to scheme: %v", err)
	}
	return scheme
}
