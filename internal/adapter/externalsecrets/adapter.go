/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package externalsecrets provides the built-in External Secrets Operator addon adapter.
package externalsecrets

import (
	"context"
	"fmt"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/pkg/adapter"
)

const (
	Name               = "external-secrets"
	Version            = "0.1.0"
	FamilySystemHealth = adapter.Family("system_health")
	FamilySecretSync   = adapter.Family("secret_sync")

	defaultNamespace        = "external-secrets"
	defaultRestartWarnCount = int32(3)
	defaultStaleMinutes     = 60

	thresholdRestartWarnCount = "restartWarnCount"
	thresholdStaleMinutes     = "staleMinutes"
)

var (
	components = []string{"external-secrets", "external-secrets-webhook", "external-secrets-cert-controller"}
	crds       = []string{
		"externalsecrets.external-secrets.io",
		"secretstores.external-secrets.io",
		"clustersecretstores.external-secrets.io",
		"clusterexternalsecrets.external-secrets.io",
		"pushsecrets.external-secrets.io",
	}
)

// Adapter implements External Secrets Operator health checks.
type Adapter struct{}

// New returns the built-in External Secrets Operator adapter.
func New() Adapter { return Adapter{} }

func (Adapter) Name() string            { return Name }
func (Adapter) Version() string         { return Version }
func (Adapter) ContractVersion() string { return adapter.ContractVersion }

func (Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{Name}, Families: []adapter.Family{FamilySystemHealth, FamilySecretSync}}
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=external-secrets.io,resources=externalsecrets,verbs=get;list;watch

func (a Adapter) Run(ctx context.Context, req adapter.Request) (adapter.Result, error) {
	started := time.Now()
	checks := []adapter.CheckResult{}
	if systemPolicy, enabled := familyPolicy(req.Policy, FamilySystemHealth, true); enabled {
		checks = append(checks, a.checkSystemHealth(ctx, req.Client, systemPolicy)...)
	}
	if syncPolicy, enabled := familyPolicy(req.Policy, FamilySecretSync, true); enabled {
		checks = append(checks, a.checkSecretSync(ctx, req.Client, syncPolicy)...)
	}
	if len(checks) == 0 {
		checks = append(checks, skipped(req.Target, "all External Secrets Operator check families are disabled by policy"))
	}
	return adapter.Result{Checks: checks, Duration: time.Since(started)}, nil
}

func familyPolicy(policy map[adapter.Family]adapter.FamilyPolicy, family adapter.Family, defaultEnabled bool) (adapter.FamilyPolicy, bool) {
	if policy == nil {
		return adapter.FamilyPolicy{Enabled: defaultEnabled}, defaultEnabled
	}
	familyPolicy, ok := policy[family]
	if !ok {
		return adapter.FamilyPolicy{}, false
	}
	return familyPolicy, familyPolicy.Enabled
}

func (a Adapter) checkSystemHealth(ctx context.Context, c client.Client, policy adapter.FamilyPolicy) []adapter.CheckResult {
	checks := []adapter.CheckResult{}
	restartWarnCount := int32Threshold(policy, thresholdRestartWarnCount, defaultRestartWarnCount)
	for _, namespace := range policyNamespaces(policy) {
		for _, component := range components {
			deployment, check := a.checkDeployment(ctx, c, namespace, component)
			checks = append(checks, check)
			if deployment != nil {
				checks = append(checks, a.checkPods(ctx, c, deployment, restartWarnCount)...)
			}
		}
	}
	for _, crd := range crds {
		checks = append(checks, a.checkCRD(ctx, c, crd))
	}
	return checks
}

func (Adapter) checkDeployment(ctx context.Context, c client.Client, namespace, name string) (*appsv1.Deployment, adapter.CheckResult) {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: namespace, Name: name}
	var deployment appsv1.Deployment
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, check(target, adapter.OutcomeFail, "External Secrets Operator deployment is missing", map[string]string{"component": name}, started)
		}
		return nil, check(target, adapter.OutcomeError, fmt.Sprintf("failed to read External Secrets Operator deployment: %v", err), map[string]string{"component": name}, started)
	}
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	if desired == 0 {
		return &deployment, check(target, adapter.OutcomeWarn, "External Secrets Operator deployment is scaled to zero", map[string]string{"component": name}, started)
	}
	if !deploymentAvailable(&deployment) || deployment.Status.AvailableReplicas < desired {
		return &deployment, check(target, adapter.OutcomeFail, "External Secrets Operator deployment is not fully available", map[string]string{"component": name, "desiredReplicas": strconv.FormatInt(int64(desired), 10), "availableReplicas": strconv.FormatInt(int64(deployment.Status.AvailableReplicas), 10)}, started)
	}
	return &deployment, check(target, adapter.OutcomePass, "External Secrets Operator deployment is available", map[string]string{"component": name}, started)
}

func deploymentAvailable(deployment *appsv1.Deployment) bool {
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (Adapter) checkPods(ctx context.Context, c client.Client, deployment *appsv1.Deployment, restartWarnCount int32) []adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "v1", Kind: "Pod", Namespace: deployment.Namespace, Name: deployment.Name}
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return []adapter.CheckResult{check(target, adapter.OutcomeError, fmt.Sprintf("deployment has invalid pod selector: %v", err), map[string]string{"component": deployment.Name}, started)}
	}
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(deployment.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return []adapter.CheckResult{check(target, adapter.OutcomeError, fmt.Sprintf("failed to list External Secrets Operator pods: %v", err), map[string]string{"component": deployment.Name}, started)}
	}
	if len(pods.Items) == 0 {
		return []adapter.CheckResult{check(target, adapter.OutcomeFail, "External Secrets Operator deployment has no matching pods", map[string]string{"component": deployment.Name}, started)}
	}
	checks := make([]adapter.CheckResult, 0, len(pods.Items))
	for _, pod := range pods.Items {
		if !podReady(&pod) {
			checks = append(checks, check(podTarget(&pod), adapter.OutcomeFail, "External Secrets Operator pod is not ready", map[string]string{"component": deployment.Name, "phase": string(pod.Status.Phase)}, started))
			continue
		}
		if restarts := maxRestartCount(&pod); restarts > restartWarnCount {
			checks = append(checks, check(podTarget(&pod), adapter.OutcomeWarn, "External Secrets Operator pod restart count exceeds warning threshold", map[string]string{"component": deployment.Name, "restartCount": strconv.FormatInt(int64(restarts), 10), "restartWarnCount": strconv.FormatInt(int64(restartWarnCount), 10)}, started))
			continue
		}
		checks = append(checks, check(podTarget(&pod), adapter.OutcomePass, "External Secrets Operator pod is ready", map[string]string{"component": deployment.Name}, started))
	}
	return checks
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func maxRestartCount(pod *corev1.Pod) int32 {
	var max int32
	for _, status := range pod.Status.ContainerStatuses {
		if status.RestartCount > max {
			max = status.RestartCount
		}
	}
	return max
}

func podTarget(pod *corev1.Pod) adapter.TargetRef {
	return adapter.TargetRef{APIVersion: "v1", Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name}
}

func (Adapter) checkCRD(ctx context.Context, c client.Client, name string) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apiextensions.k8s.io/v1", Kind: "CustomResourceDefinition", Name: name}
	var crd apixv1.CustomResourceDefinition
	if err := c.Get(ctx, types.NamespacedName{Name: name}, &crd); err != nil {
		if apierrors.IsNotFound(err) {
			return check(target, adapter.OutcomeFail, "External Secrets Operator CRD is missing", map[string]string{"crd": name}, started)
		}
		return check(target, adapter.OutcomeError, fmt.Sprintf("failed to read External Secrets Operator CRD: %v", err), map[string]string{"crd": name}, started)
	}
	if !crdEstablished(&crd) {
		return check(target, adapter.OutcomeFail, "External Secrets Operator CRD is not established", map[string]string{"crd": name}, started)
	}
	if !crdServesV1Beta1(&crd) {
		return check(target, adapter.OutcomeWarn, "External Secrets Operator CRD does not serve v1beta1", map[string]string{"crd": name}, started)
	}
	return check(target, adapter.OutcomePass, "External Secrets Operator CRD is established", map[string]string{"crd": name}, started)
}

func crdEstablished(crd *apixv1.CustomResourceDefinition) bool {
	for _, condition := range crd.Status.Conditions {
		if condition.Type == apixv1.Established {
			return condition.Status == apixv1.ConditionTrue
		}
	}
	return false
}

func crdServesV1Beta1(crd *apixv1.CustomResourceDefinition) bool {
	for _, version := range crd.Spec.Versions {
		if version.Name == "v1beta1" && version.Served {
			return true
		}
	}
	return false
}

func (Adapter) checkSecretSync(ctx context.Context, c client.Client, policy adapter.FamilyPolicy) []adapter.CheckResult {
	checks := []adapter.CheckResult{}
	for _, namespace := range policyNamespaces(policy) {
		var externalSecrets unstructured.UnstructuredList
		externalSecrets.SetAPIVersion("external-secrets.io/v1beta1")
		externalSecrets.SetKind("ExternalSecretList")
		listOpts := []client.ListOption{client.InNamespace(namespace)}
		if policy.LabelSelector != nil {
			selector, err := metav1.LabelSelectorAsSelector(policy.LabelSelector)
			if err != nil {
				checks = append(checks, check(adapter.TargetRef{APIVersion: "external-secrets.io/v1beta1", Kind: "ExternalSecret", Namespace: namespace, Name: namespace}, adapter.OutcomeError, fmt.Sprintf("ExternalSecret label selector is invalid: %v", err), nil, time.Now()))
				continue
			}
			listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
		}
		started := time.Now()
		if err := c.List(ctx, &externalSecrets, listOpts...); err != nil {
			checks = append(checks, check(adapter.TargetRef{APIVersion: "external-secrets.io/v1beta1", Kind: "ExternalSecret", Namespace: namespace, Name: "externalsecrets"}, adapter.OutcomeError, fmt.Sprintf("failed to list ExternalSecret resources: %v", err), nil, started))
			continue
		}
		for i := range externalSecrets.Items {
			checks = append(checks, externalSecretCheck(&externalSecrets.Items[i], policy))
		}
	}
	if len(checks) == 0 {
		checks = append(checks, skipped(adapter.TargetRef{APIVersion: "external-secrets.io/v1beta1", Kind: "ExternalSecret", Name: "externalsecrets"}, "secret_sync found no matching ExternalSecret resources"))
	}
	return checks
}

func externalSecretCheck(obj *unstructured.Unstructured, policy adapter.FamilyPolicy) adapter.CheckResult {
	started := time.Now()
	target := objectTarget(obj)
	condition := readyCondition(obj)
	details := externalSecretDetails(obj, condition)
	if condition == nil {
		return check(target, adapter.OutcomeFail, "ExternalSecret has no Ready condition", details, started)
	}
	if conditionStatus(condition) != "True" {
		return check(target, adapter.OutcomeFail, "ExternalSecret sync is not ready", details, started)
	}
	if refreshTime, ok := stringAt(obj.Object, "status", "refreshTime"); ok {
		details["refreshTime"] = refreshTime
		parsed, err := time.Parse(time.RFC3339, refreshTime)
		if err != nil {
			return check(target, adapter.OutcomeError, "ExternalSecret refresh timestamp is invalid", details, started)
		}
		staleAfter := time.Duration(intThreshold(policy, thresholdStaleMinutes, defaultStaleMinutes)) * time.Minute
		if time.Since(parsed) > staleAfter {
			details["staleMinutes"] = strconv.Itoa(int(time.Since(parsed).Minutes()))
			return check(target, adapter.OutcomeWarn, "ExternalSecret sync is stale", details, started)
		}
	}
	return check(target, adapter.OutcomePass, "ExternalSecret sync is ready", details, started)
}

func readyCondition(obj *unstructured.Unstructured) map[string]any {
	conditions, ok, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !ok {
		return nil
	}
	for _, candidate := range conditions {
		condition, ok := candidate.(map[string]any)
		if !ok {
			continue
		}
		if conditionType(condition) == "Ready" {
			return condition
		}
	}
	return nil
}

func conditionType(condition map[string]any) string {
	value, _ := condition["type"].(string)
	return value
}

func conditionStatus(condition map[string]any) string {
	value, _ := condition["status"].(string)
	return value
}

func conditionDetails(condition map[string]any) map[string]string {
	details := map[string]string{}
	if condition == nil {
		return details
	}
	for _, key := range []string{"status", "reason", "message"} {
		if value, ok := condition[key].(string); ok && value != "" {
			details[key] = value
		}
	}
	return details
}

func externalSecretDetails(obj *unstructured.Unstructured, condition map[string]any) map[string]string {
	details := conditionDetails(condition)
	for key, path := range map[string][]string{
		"secretStoreRefName":     {"spec", "secretStoreRef", "name"},
		"secretStoreRefKind":     {"spec", "secretStoreRef", "kind"},
		"targetName":             {"spec", "target", "name"},
		"syncedResourceVersion":  {"status", "syncedResourceVersion"},
		"bindingName":            {"status", "binding", "name"},
		"bindingResourceVersion": {"status", "binding", "resourceVersion"},
	} {
		if value, ok := stringAt(obj.Object, path...); ok {
			details[key] = value
		}
	}
	return details
}

func policyNamespaces(policy adapter.FamilyPolicy) []string {
	if len(policy.Namespaces) == 0 {
		return []string{defaultNamespace}
	}
	return policy.Namespaces
}

func int32Threshold(policy adapter.FamilyPolicy, key string, defaultValue int32) int32 {
	return int32(intThreshold(policy, key, int(defaultValue)))
}

func intThreshold(policy adapter.FamilyPolicy, key string, defaultValue int) int {
	if policy.Thresholds == nil {
		return defaultValue
	}
	value, ok := policy.Thresholds[key]
	if !ok {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return defaultValue
	}
	return parsed
}

func objectTarget(obj *unstructured.Unstructured) adapter.TargetRef {
	return adapter.TargetRef{APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Namespace: obj.GetNamespace(), Name: obj.GetName()}
}

func stringAt(obj map[string]any, fields ...string) (string, bool) {
	value, ok, _ := unstructured.NestedString(obj, fields...)
	return value, ok
}

func skipped(target adapter.TargetRef, summary string) adapter.CheckResult {
	return adapter.CheckResult{Family: FamilySecretSync, Outcome: adapter.OutcomeSkipped, TargetRef: target, Summary: summary, ObservedAt: time.Now()}
}

func check(target adapter.TargetRef, outcome adapter.Outcome, summary string, details map[string]string, started time.Time) adapter.CheckResult {
	return adapter.CheckResult{Family: familyForTarget(target), Outcome: outcome, TargetRef: target, Summary: summary, Details: details, ObservedAt: time.Now(), Duration: time.Since(started)}
}

func familyForTarget(target adapter.TargetRef) adapter.Family {
	if target.APIVersion == "external-secrets.io/v1beta1" && target.Kind == "ExternalSecret" {
		return FamilySecretSync
	}
	return FamilySystemHealth
}
