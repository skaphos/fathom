/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"fmt"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/internal/adapter/podutil"
	"github.com/skaphos/fathom/pkg/adapter"
)

// Evaluate implements Evaluator for WorkloadCheck. It reads one controller
// singleton and, when CheckPods is set and the workload is gated (desired>0),
// its pods. It generalizes the hand-written checkDeployment/checkDaemonSet plus
// checkPods across Deployment / DaemonSet / StatefulSet.
func (w WorkloadCheck) Evaluate(ec EvalContext) ([]adapter.CheckResult, error) {
	ns := firstNamespace(ec.Policy, w.DefaultNamespace)
	name := w.DefaultName
	if w.NameThresholdKey != "" {
		name = stringThreshold(ec.Policy, w.NameThresholdKey, w.DefaultName)
	}
	warn := w.DefaultRestartWarn
	if w.RestartWarnThresholdKey != "" {
		warn = int32Threshold(ec.Policy, w.RestartWarnThresholdKey, w.DefaultRestartWarn)
	}

	found, wc, gated, selector, podNS := w.readWorkload(ec, ns, name)
	checks := []adapter.CheckResult{wc}
	if w.CheckPods && found && gated {
		checks = append(checks, checkPods(ec, podNS, selector, w.Component, warn)...)
	}
	return checks, nil
}

// readWorkload does the typed Get for w.Kind and returns a kind-neutral
// snapshot: whether the object was found, the workload CheckResult, whether the
// pod sub-check is gated on (desired>0), and the pod selector/namespace to use.
func (w WorkloadCheck) readWorkload(ec EvalContext, ns, name string) (found bool, wc adapter.CheckResult, gated bool, selector *metav1.LabelSelector, podNS string) {
	switch w.Kind {
	case KindDeployment:
		return w.readDeployment(ec, ns, name)
	case KindDaemonSet:
		return w.readDaemonSet(ec, ns, name)
	case KindStatefulSet:
		return w.readStatefulSet(ec, ns, name)
	default:
		// Defensive: NewEngine rejects unknown kinds, so this is unreachable in
		// a validated engine. Surface it as an adapter Error rather than panic.
		started := time.Now()
		ref := adapter.TargetRef{APIVersion: "apps/v1", Kind: string(w.Kind), Namespace: ns, Name: name}
		return false, result(ec.Family, ref, adapter.OutcomeError,
			fmt.Sprintf("unknown workload kind %q", w.Kind),
			map[string]string{"component": w.Component}, started), false, nil, ""
	}
}

func (w WorkloadCheck) readDeployment(ec EvalContext, ns, name string) (bool, adapter.CheckResult, bool, *metav1.LabelSelector, string) {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: ns, Name: name}
	details := map[string]string{"component": w.Component}

	var deployment appsv1.Deployment
	if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Namespace: ns, Name: name}, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			o := absenceOutcome(effectiveAbsence(w.Absence, ec.DefaultPosture))
			return false, result(ec.Family, target, o, "workload deployment not found", adapter.MarkAbsent(details), started), false, nil, ""
		}
		return false, result(ec.Family, target, adapter.OutcomeError, fmt.Sprintf("failed to read deployment: %v", err), details, started), false, nil, ""
	}

	desired := derefReplicas(deployment.Spec.Replicas)
	if desired == 0 {
		return true, result(ec.Family, target, adapter.OutcomeWarn, "deployment is scaled to zero", details, started), false, nil, ""
	}
	if !deploymentAvailable(&deployment) || deployment.Status.AvailableReplicas < desired {
		details["desiredReplicas"] = strconv.FormatInt(int64(desired), 10)
		details["availableReplicas"] = strconv.FormatInt(int64(deployment.Status.AvailableReplicas), 10)
		return true, result(ec.Family, target, adapter.OutcomeFail, "deployment is not fully available", details, started), true, deployment.Spec.Selector, deployment.Namespace
	}
	return true, result(ec.Family, target, adapter.OutcomePass, "deployment is available", details, started), true, deployment.Spec.Selector, deployment.Namespace
}

func (w WorkloadCheck) readDaemonSet(ec EvalContext, ns, name string) (bool, adapter.CheckResult, bool, *metav1.LabelSelector, string) {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "DaemonSet", Namespace: ns, Name: name}
	details := map[string]string{"component": w.Component}

	var daemonset appsv1.DaemonSet
	if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Namespace: ns, Name: name}, &daemonset); err != nil {
		if apierrors.IsNotFound(err) {
			o := absenceOutcome(effectiveAbsence(w.Absence, ec.DefaultPosture))
			return false, result(ec.Family, target, o, "workload daemonset not found", adapter.MarkAbsent(details), started), false, nil, ""
		}
		return false, result(ec.Family, target, adapter.OutcomeError, fmt.Sprintf("failed to read daemonset: %v", err), details, started), false, nil, ""
	}

	status := daemonset.Status
	details["desiredNumberScheduled"] = strconv.FormatInt(int64(status.DesiredNumberScheduled), 10)
	details["numberReady"] = strconv.FormatInt(int64(status.NumberReady), 10)
	details["numberAvailable"] = strconv.FormatInt(int64(status.NumberAvailable), 10)
	details["numberUnavailable"] = strconv.FormatInt(int64(status.NumberUnavailable), 10)
	details["updatedNumberScheduled"] = strconv.FormatInt(int64(status.UpdatedNumberScheduled), 10)

	// Order is load-bearing: zero-scheduled (Warn) precedes unavailable (Fail)
	// precedes mid-rollout (Warn).
	if status.DesiredNumberScheduled == 0 {
		return true, result(ec.Family, target, adapter.OutcomeWarn, "daemonset schedules zero pods (no matching nodes)", details, started), false, nil, ""
	}
	if status.NumberUnavailable > 0 || status.NumberReady < status.DesiredNumberScheduled {
		return true, result(ec.Family, target, adapter.OutcomeFail, "daemonset is not fully ready", details, started), true, daemonset.Spec.Selector, daemonset.Namespace
	}
	if status.UpdatedNumberScheduled < status.DesiredNumberScheduled {
		return true, result(ec.Family, target, adapter.OutcomeWarn, "daemonset rollout is in progress", details, started), true, daemonset.Spec.Selector, daemonset.Namespace
	}
	return true, result(ec.Family, target, adapter.OutcomePass, "daemonset is fully ready", details, started), true, daemonset.Spec.Selector, daemonset.Namespace
}

func (w WorkloadCheck) readStatefulSet(ec EvalContext, ns, name string) (bool, adapter.CheckResult, bool, *metav1.LabelSelector, string) {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "StatefulSet", Namespace: ns, Name: name}
	details := map[string]string{"component": w.Component}

	var sts appsv1.StatefulSet
	if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Namespace: ns, Name: name}, &sts); err != nil {
		if apierrors.IsNotFound(err) {
			o := absenceOutcome(effectiveAbsence(w.Absence, ec.DefaultPosture))
			return false, result(ec.Family, target, o, "workload statefulset not found", adapter.MarkAbsent(details), started), false, nil, ""
		}
		return false, result(ec.Family, target, adapter.OutcomeError, fmt.Sprintf("failed to read statefulset: %v", err), details, started), false, nil, ""
	}

	desired := derefReplicas(sts.Spec.Replicas)
	details["desiredReplicas"] = strconv.FormatInt(int64(desired), 10)
	details["readyReplicas"] = strconv.FormatInt(int64(sts.Status.ReadyReplicas), 10)
	if desired == 0 {
		return true, result(ec.Family, target, adapter.OutcomeWarn, "statefulset is scaled to zero", details, started), false, nil, ""
	}
	if sts.Status.ReadyReplicas < desired {
		return true, result(ec.Family, target, adapter.OutcomeFail, "statefulset is not fully ready", details, started), true, sts.Spec.Selector, sts.Namespace
	}
	return true, result(ec.Family, target, adapter.OutcomePass, "statefulset is fully ready", details, started), true, sts.Spec.Selector, sts.Namespace
}

// checkPods is the kind-independent pod sub-check: invalid selector -> Error;
// List error -> Error; zero matching pods -> Fail; only terminating/completed
// pods -> Skipped; per live pod not-ready -> Warn, restart count over threshold
// -> Warn, else Pass. Terminating (rolling-update) and Failed/Evicted pods are
// filtered before grading so they cannot force a false Fail (#160); a live
// not-ready pod is Warn, not Fail, because the workload-availability check is
// the authoritative outage signal and already Fails on unmet capacity. The
// selector is the workload's own Spec.Selector; policy.LabelSelector is
// intentionally ignored.
func checkPods(ec EvalContext, namespace string, selector *metav1.LabelSelector, component string, restartWarnCount int32) []adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "v1", Kind: "Pod", Namespace: namespace, Name: component}
	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomeError, fmt.Sprintf("%s has an invalid pod selector: %v", component, err), map[string]string{"component": component}, started)}
	}
	var pods corev1.PodList
	if err := ec.Client.List(ec.Ctx, &pods, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: sel}); err != nil {
		return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomeError, fmt.Sprintf("failed to list %s pods: %v", component, err), map[string]string{"component": component}, started)}
	}
	if len(pods.Items) == 0 {
		return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomeFail, fmt.Sprintf("%s has no matching pods", component), map[string]string{"component": component}, started)}
	}

	live := make([]*corev1.Pod, 0, len(pods.Items))
	for i := range pods.Items {
		if podutil.Active(&pods.Items[i]) {
			live = append(live, &pods.Items[i])
		}
	}
	if len(live) == 0 {
		// Every matching pod is terminating, failed, or completed (mid-rollout
		// churn or lingering Evicted pods). Defer to the authoritative workload
		// check — Skipped is informational and never drags the roll-up down.
		return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomeSkipped, fmt.Sprintf("%s has only terminating, failed, or completed pods", component), map[string]string{"component": component}, started)}
	}

	checks := make([]adapter.CheckResult, 0, len(live))
	for _, pod := range live {
		if !podReady(pod) {
			checks = append(checks, result(ec.Family, podTarget(pod), adapter.OutcomeWarn, fmt.Sprintf("%s pod is not ready", component), map[string]string{"component": component, "phase": string(pod.Status.Phase)}, started))
			continue
		}
		if restarts := maxRestartCount(pod); restarts > restartWarnCount {
			checks = append(checks, result(ec.Family, podTarget(pod), adapter.OutcomeWarn, fmt.Sprintf("%s pod restart count exceeds warning threshold", component), map[string]string{
				"component":        component,
				"restartCount":     strconv.FormatInt(int64(restarts), 10),
				"restartWarnCount": strconv.FormatInt(int64(restartWarnCount), 10),
			}, started))
			continue
		}
		checks = append(checks, result(ec.Family, podTarget(pod), adapter.OutcomePass, fmt.Sprintf("%s pod is ready", component), map[string]string{"component": component}, started))
	}
	return checks
}
