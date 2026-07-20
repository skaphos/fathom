/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package probe

import (
	"context"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// defaultResweepInterval is how often the Sweeper re-runs after the
	// initial leader-startup sweep. The periodic pass is a safety net for the
	// rare case where a launcher's bestEffortDelete failed (API hiccup inside
	// its 5s cleanup window) while the operator kept running — without it,
	// such a pod would leak until the next operator restart.
	defaultResweepInterval = time.Hour

	// defaultOrphanMinAge guards in-flight probes. A terminal probe pod is
	// normally deleted by its launcher within one poll interval (~500ms) of
	// termination; one that has existed for several minutes has no live
	// launcher waiting on it. The grace also covers the leader-handover
	// window where the outgoing leader may still be reading a termination
	// message the incoming leader can already see.
	defaultOrphanMinAge = 5 * time.Minute
)

// The Sweeper needs to list candidate probe pods cluster-wide (probes run in
// the checked workload's namespace, or wherever the probeNamespace threshold
// points) and delete the orphans. The label scoping below cannot be expressed
// in RBAC, so the grant is list+delete on all pods cluster-wide. Note that
// `list` returns full Pod objects — spec and status, not just metadata — so
// this grant does expose pod contents cluster-wide; the sweeper itself reads
// only labels, phase, and timestamps, and never logs pod contents.
// +kubebuilder:rbac:groups="",resources=pods,verbs=list;delete

// Sweeper reaps orphaned probe pods. Launcher.Run deletes its pod via an
// in-process defer, so a probe pod orphans whenever the operator dies between
// pod Create and that defer (OOM-kill, node drain, crash). The kubelet does
// NOT garbage-collect terminated pods — cluster pod-GC only engages at the
// terminated-pod threshold (~12500) — so without a sweep, orphans accumulate
// across every operator restart (#163).
//
// Sweeper runs on the elected leader: once at startup (reaping whatever the
// previous incarnation left behind) and then every Interval as a safety net.
// It only deletes pods that carry both probe labels, are in a terminal phase,
// and are older than MinAge — a still-Running orphan is left for its
// ActiveDeadlineSeconds to terminate, after which a later sweep reaps it.
type Sweeper struct {
	// Reader lists candidate pods and MUST be a live (uncached) reader, e.g.
	// mgr.GetAPIReader(). The manager cache deliberately carries no Pod
	// informer; listing through the cached client would open an unfiltered
	// cluster-wide Pod watch and reintroduce the memory blow-up removed in
	// #164.
	Reader client.Reader

	// Client performs the deletes. Writes never touch the informer cache, so
	// the manager's default client is fine here.
	Client client.Client

	// Log receives per-sweep outcomes. Optional; discarded when unset.
	Log logr.Logger

	// Interval between periodic sweeps. Zero means defaultResweepInterval.
	Interval time.Duration

	// MinAge is the minimum pod age before a terminal probe pod is considered
	// orphaned. Zero means defaultOrphanMinAge.
	MinAge time.Duration
}

// NeedLeaderElection gates the Sweeper on leadership so a non-leader replica
// cannot delete a pod the leader's launcher is still polling.
func (*Sweeper) NeedLeaderElection() bool { return true }

// Start implements manager.Runnable. It sweeps immediately, then on every
// tick, and returns nil when ctx is cancelled. Sweep failures are logged and
// retried on the next tick rather than crashing the manager.
func (s *Sweeper) Start(ctx context.Context) error {
	if s.Reader == nil || s.Client == nil {
		return errors.New("probe sweeper: Reader and Client are required")
	}
	interval := s.Interval
	if interval <= 0 {
		interval = defaultResweepInterval
	}
	s.sweep(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep deletes every orphaned probe pod visible right now. Best-effort: a
// pod that vanishes between List and Delete is fine (its launcher or a
// concurrent kubectl beat us to it), and other delete errors are logged and
// left for the next pass.
func (s *Sweeper) sweep(ctx context.Context) {
	minAge := s.MinAge
	if minAge <= 0 {
		minAge = defaultOrphanMinAge
	}
	var pods corev1.PodList
	if err := s.Reader.List(ctx, &pods, client.MatchingLabelsSelector{Selector: probePodSelector()}); err != nil {
		s.Log.Error(err, "list probe pods for orphan sweep")
		return
	}
	reaped := 0
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !terminalPhase(pod.Status.Phase) {
			continue
		}
		if time.Since(orphanSince(pod)) < minAge {
			continue
		}
		if err := s.Client.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
			s.Log.Error(err, "delete orphaned probe pod", "namespace", pod.Namespace, "name", pod.Name)
			continue
		}
		reaped++
	}
	if reaped > 0 {
		s.Log.Info("reaped orphaned probe pods", "count", reaped)
	}
}

// probePodSelector matches exactly the pods Pod() creates: managed-by=fathom
// AND the probe name label present. Requiring both keeps the sweep away from
// other Fathom-managed pods that share the managed-by label but are not
// probes (e.g. node-agent DaemonSet pods).
func probePodSelector() labels.Selector {
	selector := labels.SelectorFromSet(labels.Set{labelManagedBy: managedByValue})
	probeLabelExists, err := labels.NewRequirement(labelProbeName, selection.Exists, nil)
	if err != nil {
		// Unreachable: the requirement is built from compile-time constants.
		panic(err)
	}
	return selector.Add(*probeLabelExists)
}

func terminalPhase(phase corev1.PodPhase) bool {
	return phase == corev1.PodSucceeded || phase == corev1.PodFailed
}

// orphanSince returns the instant from which a terminal pod's orphan age is
// measured: the latest container termination time, falling back to the
// creation timestamp when no container reports one.
//
// Creation time alone is wrong. A probe's timeout is unbounded
// (AddonCheck.spec.timeout has no maximum), so a pod configured with, say, a
// 10m timeout is already older than MinAge at the instant it first turns
// terminal. Sweeping on creation age could then delete it inside the ~500ms
// window before its live launcher's next poll observes the termination
// message, surfacing a spurious "probe pod disappeared" error. Measuring
// from termination keeps the grace period doing what it is for: bounding how
// long we wait for a launcher that is still polling.
func orphanSince(pod *corev1.Pod) time.Time {
	latest := time.Time{}
	for _, statuses := range [][]corev1.ContainerStatus{
		pod.Status.ContainerStatuses,
		pod.Status.InitContainerStatuses,
	} {
		for i := range statuses {
			terminated := statuses[i].State.Terminated
			if terminated == nil || terminated.FinishedAt.IsZero() {
				continue
			}
			if terminated.FinishedAt.After(latest) {
				latest = terminated.FinishedAt.Time
			}
		}
	}
	if latest.IsZero() {
		return pod.CreationTimestamp.Time
	}
	return latest
}
