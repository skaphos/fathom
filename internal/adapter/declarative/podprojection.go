/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/internal/adapter/podutil"
	"github.com/skaphos/fathom/pkg/adapter"
)

// maxNamedPods caps how many offending pod names a PodProjectionCheck lists in
// its Details, so one result stays bounded on a cluster with many uninjected
// pods (the count is always reported in full).
const maxNamedPods = 5

// Evaluate implements Evaluator for PodProjectionCheck. It lists the Pods that
// carry the webhook's opt-in Selector across the resolved namespaces (an empty
// policy scans all namespaces — opted-in workloads live anywhere) and verifies
// each live pod actually received the injection: the named projected
// serviceAccountToken volume and, when EnvVar is set, that env var in every
// container.
//
//   - No labeled pods anywhere -> Skipped (NoMatchingObjects): a cluster where
//     nothing has opted in is quiet by design.
//   - Terminating, failed, and completed pods are filtered out (podutil.Active,
//     mirroring checkPods): only live pods carry an actionable signal.
//   - One or more live pods missing the injection -> MissingOutcome (default
//     Fail), with the offending pods named in Details (capped at maxNamedPods).
//   - All live pods injected -> Pass.
//
// The whole scan folds into a single CheckResult rather than one per pod: the
// selector can match arbitrarily many pods cluster-wide, and a bounded
// HealthReport beats a per-pod ledger here. The check's own Selector is
// authoritative; policy.LabelSelector is intentionally not merged in —
// narrowing it could silently exempt uninjected pods.
func (pc PodProjectionCheck) Evaluate(ec EvalContext) ([]adapter.CheckResult, error) {
	started := time.Now()
	ref := adapter.TargetRef{APIVersion: "v1", Kind: "Pod", Name: pc.listName()}
	details := pc.scanDetails()

	var pods []corev1.Pod
	for _, ns := range policyNamespaces(ec.Policy, "") {
		var list corev1.PodList
		if err := ec.Client.List(ec.Ctx, &list, client.InNamespace(ns), client.MatchingLabels(pc.Selector)); err != nil {
			return []adapter.CheckResult{result(ec.Family, ref, adapter.OutcomeError,
				fmt.Sprintf("failed to list opted-in pods in %s: %v", namespaceScope(ns), err), details, started)}, nil
		}
		pods = append(pods, list.Items...)
	}

	if len(pods) == 0 {
		c := skippedResult(ec.Family, ref,
			fmt.Sprintf("no pods carry the %s opt-in label", formatSelector(pc.Selector)), "NoMatchingObjects")
		for k, v := range pc.scanDetails() {
			c.Details[k] = v
		}
		return []adapter.CheckResult{c}, nil
	}

	live := 0
	var missing []string
	for i := range pods {
		pod := &pods[i]
		if !podutil.Active(pod) {
			continue
		}
		live++
		if !pc.podInjected(pod) {
			missing = append(missing, pod.Namespace+"/"+pod.Name)
		}
	}
	details["matchedPods"] = strconv.Itoa(len(pods))
	details["livePods"] = strconv.Itoa(live)

	if live == 0 {
		// Mid-rollout churn or lingering completed pods only: nothing live to
		// grade, so stay informational rather than inventing a verdict.
		details["skipReason"] = "NoMatchingObjects"
		return []adapter.CheckResult{result(ec.Family, ref, adapter.OutcomeSkipped,
			"all opted-in pods are terminating, failed, or completed", details, started)}, nil
	}
	if len(missing) > 0 {
		outcome := pc.MissingOutcome
		if outcome == "" {
			outcome = adapter.OutcomeFail
		}
		details["uninjectedCount"] = strconv.Itoa(len(missing))
		details["uninjectedPods"] = capNames(missing, maxNamedPods)
		return []adapter.CheckResult{result(ec.Family, ref, outcome,
			fmt.Sprintf("%d of %d opted-in pods are missing the injected %s projection — admitted while the webhook was not mutating",
				len(missing), live, pc.VolumeName), details, started)}, nil
	}
	return []adapter.CheckResult{result(ec.Family, ref, adapter.OutcomePass,
		fmt.Sprintf("all %d opted-in pods carry the injected %s projection", live, pc.VolumeName),
		details, started)}, nil
}

// scanDetails returns the Details common to every result this check emits: the
// opt-in selector (so the aggregated row stays self-describing) and the
// component label when set.
func (pc PodProjectionCheck) scanDetails() map[string]string {
	details := map[string]string{"selector": formatSelector(pc.Selector)}
	if pc.Component != "" {
		details["component"] = pc.Component
	}
	return details
}

// podInjected reports whether the pod carries the webhook's injection: a
// projected volume named VolumeName with a serviceAccountToken source and,
// when EnvVar is set, that env var in every (non-init) container. Both are
// injected unconditionally by the webhook, so a live opted-in pod missing
// either was admitted while the webhook was absent or not mutating.
func (pc PodProjectionCheck) podInjected(pod *corev1.Pod) bool {
	if !hasProjectedTokenVolume(pod, pc.VolumeName) {
		return false
	}
	if pc.EnvVar == "" {
		return true
	}
	for i := range pod.Spec.Containers {
		if !containerHasEnv(&pod.Spec.Containers[i], pc.EnvVar) {
			return false
		}
	}
	return true
}

// hasProjectedTokenVolume reports whether the pod declares a projected volume
// named name carrying at least one serviceAccountToken source.
func hasProjectedTokenVolume(pod *corev1.Pod, name string) bool {
	for i := range pod.Spec.Volumes {
		v := &pod.Spec.Volumes[i]
		if v.Name != name || v.Projected == nil {
			continue
		}
		for _, src := range v.Projected.Sources {
			if src.ServiceAccountToken != nil {
				return true
			}
		}
	}
	return false
}

// containerHasEnv reports whether the container declares the named env var.
func containerHasEnv(c *corev1.Container, name string) bool {
	for _, env := range c.Env {
		if env.Name == name {
			return true
		}
	}
	return false
}

// listName is the stable placeholder Name on the aggregated result.
func (pc PodProjectionCheck) listName() string {
	if pc.ListName != "" {
		return pc.ListName
	}
	return "Pod"
}

// formatSelector renders a label map as a stable, comma-joined k=v string for
// summaries and details.
func formatSelector(sel map[string]string) string {
	parts := make([]string, 0, len(sel))
	for k, v := range sel {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// capNames joins names, listing at most limit of them and appending a
// "+N more" suffix for the rest.
func capNames(names []string, limit int) string {
	if len(names) <= limit {
		return strings.Join(names, ",")
	}
	return strings.Join(names[:limit], ",") + fmt.Sprintf(",+%d more", len(names)-limit)
}
