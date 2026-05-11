/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package certmanager provides the built-in cert-manager addon adapter.
package certmanager

import (
	"context"
	"fmt"
	"strconv"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
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
	Name               = "cert-manager"
	Version            = "0.1.0"
	FamilySystemHealth = adapter.Family("system_health")

	defaultNamespace          = "cert-manager"
	defaultRestartWarnCount   = int32(3)
	thresholdRestartWarnCount = "restartWarnCount"
	thresholdWebhookProbe     = "webhookProbe"
)

var (
	components = []string{"cert-manager", "cert-manager-webhook", "cert-manager-cainjector"}
	crds       = []string{
		"certificates.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"challenges.acme.cert-manager.io",
		"clusterissuers.cert-manager.io",
		"issuers.cert-manager.io",
		"orders.acme.cert-manager.io",
	}
)

// Adapter implements cert-manager system health checks.
type Adapter struct{}

// New returns the built-in cert-manager adapter.
func New() Adapter {
	return Adapter{}
}

func (Adapter) Name() string            { return Name }
func (Adapter) Version() string         { return Version }
func (Adapter) ContractVersion() string { return adapter.ContractVersion }

func (Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{Name}, Families: []adapter.Family{FamilySystemHealth}}
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates;issuers,verbs=create

func (a Adapter) Run(ctx context.Context, req adapter.Request) (adapter.Result, error) {
	started := time.Now()
	policy, enabled := systemHealthPolicy(req.Policy)
	if !enabled {
		return adapter.Result{
			Checks:   []adapter.CheckResult{skipped(req.Target, "system_health family is disabled by policy")},
			Duration: time.Since(started),
		}, nil
	}

	checks := make([]adapter.CheckResult, 0, len(components)*2+len(crds)+1)
	namespaces := policy.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{defaultNamespace}
	}
	restartWarnCount := restartWarnCount(policy)

	for _, namespace := range namespaces {
		for _, component := range components {
			deployment, check := a.checkDeployment(ctx, req.Client, namespace, component)
			checks = append(checks, check)
			if deployment != nil {
				checks = append(checks, a.checkPods(ctx, req.Client, deployment, restartWarnCount)...)
			}
		}
	}
	for _, name := range crds {
		checks = append(checks, a.checkCRD(ctx, req.Client, name))
	}
	if webhookProbeEnabled(policy) {
		for _, namespace := range namespaces {
			checks = append(checks, a.checkWebhookService(ctx, req.Client, namespace))
		}
		checks = append(checks, a.checkValidatingWebhookConfiguration(ctx, req.Client))
		checks = append(checks, a.checkMutatingWebhookConfiguration(ctx, req.Client))
		for _, namespace := range namespaces {
			checks = append(checks, a.checkAdmissionDryRun(ctx, req.Client, namespace))
		}
	}

	return adapter.Result{Checks: checks, Duration: time.Since(started)}, nil
}

func systemHealthPolicy(policy map[adapter.Family]adapter.FamilyPolicy) (adapter.FamilyPolicy, bool) {
	if policy == nil {
		return adapter.FamilyPolicy{Enabled: true}, true
	}
	familyPolicy, ok := policy[FamilySystemHealth]
	if !ok {
		return adapter.FamilyPolicy{}, false
	}
	return familyPolicy, familyPolicy.Enabled
}

func restartWarnCount(policy adapter.FamilyPolicy) int32 {
	if policy.Thresholds == nil {
		return defaultRestartWarnCount
	}
	value, ok := policy.Thresholds[thresholdRestartWarnCount]
	if !ok {
		return defaultRestartWarnCount
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 {
		return defaultRestartWarnCount
	}
	return int32(parsed)
}

func (Adapter) checkDeployment(ctx context.Context, c client.Client, namespace, name string) (*appsv1.Deployment, adapter.CheckResult) {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: namespace, Name: name}
	var deployment appsv1.Deployment
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, check(target, adapter.OutcomeFail, "cert-manager deployment is missing", map[string]string{"component": name}, started)
		}
		return nil, check(target, adapter.OutcomeError, fmt.Sprintf("failed to read cert-manager deployment: %v", err), map[string]string{"component": name}, started)
	}

	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	if desired == 0 {
		return &deployment, check(target, adapter.OutcomeWarn, "cert-manager deployment is scaled to zero", map[string]string{"component": name}, started)
	}
	if !deploymentAvailable(&deployment) || deployment.Status.AvailableReplicas < desired {
		details := map[string]string{
			"component":         name,
			"desiredReplicas":   strconv.FormatInt(int64(desired), 10),
			"availableReplicas": strconv.FormatInt(int64(deployment.Status.AvailableReplicas), 10),
		}
		return &deployment, check(target, adapter.OutcomeFail, "cert-manager deployment is not fully available", details, started)
	}
	return &deployment, check(target, adapter.OutcomePass, "cert-manager deployment is available", map[string]string{"component": name}, started)
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
		return []adapter.CheckResult{check(target, adapter.OutcomeError, fmt.Sprintf("failed to list cert-manager pods: %v", err), map[string]string{"component": deployment.Name}, started)}
	}
	if len(pods.Items) == 0 {
		return []adapter.CheckResult{check(target, adapter.OutcomeFail, "cert-manager deployment has no matching pods", map[string]string{"component": deployment.Name}, started)}
	}

	checks := make([]adapter.CheckResult, 0, len(pods.Items))
	for _, pod := range pods.Items {
		if !podReady(&pod) {
			checks = append(checks, check(podTarget(&pod), adapter.OutcomeFail, "cert-manager pod is not ready", map[string]string{"component": deployment.Name, "phase": string(pod.Status.Phase)}, started))
			continue
		}
		if restarts := maxRestartCount(&pod); restarts > restartWarnCount {
			checks = append(checks, check(podTarget(&pod), adapter.OutcomeWarn, "cert-manager pod restart count exceeds warning threshold", map[string]string{
				"component":        deployment.Name,
				"restartCount":     strconv.FormatInt(int64(restarts), 10),
				"restartWarnCount": strconv.FormatInt(int64(restartWarnCount), 10),
			}, started))
			continue
		}
		checks = append(checks, check(podTarget(&pod), adapter.OutcomePass, "cert-manager pod is ready", map[string]string{"component": deployment.Name}, started))
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
			return check(target, adapter.OutcomeFail, "cert-manager CRD is missing", map[string]string{"crd": name}, started)
		}
		return check(target, adapter.OutcomeError, fmt.Sprintf("failed to read cert-manager CRD: %v", err), map[string]string{"crd": name}, started)
	}
	if !crdEstablished(&crd) {
		return check(target, adapter.OutcomeFail, "cert-manager CRD is not established", map[string]string{"crd": name}, started)
	}
	if !crdServesV1(&crd) {
		return check(target, adapter.OutcomeFail, "cert-manager CRD does not serve v1", map[string]string{"crd": name}, started)
	}
	return check(target, adapter.OutcomePass, "cert-manager CRD is established", map[string]string{"crd": name}, started)
}

func (Adapter) checkWebhookService(ctx context.Context, c client.Client, namespace string) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "v1", Kind: "Service", Namespace: namespace, Name: "cert-manager-webhook"}
	var service corev1.Service
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "cert-manager-webhook"}, &service); err != nil {
		if apierrors.IsNotFound(err) {
			return check(target, adapter.OutcomeFail, "cert-manager webhook service is missing", map[string]string{"component": "cert-manager-webhook"}, started)
		}
		return check(target, adapter.OutcomeError, fmt.Sprintf("failed to read cert-manager webhook service: %v", err), map[string]string{"component": "cert-manager-webhook"}, started)
	}
	if service.Spec.ClusterIP == "" || service.Spec.ClusterIP == corev1.ClusterIPNone {
		return check(target, adapter.OutcomeFail, "cert-manager webhook service has no cluster IP", map[string]string{"component": "cert-manager-webhook"}, started)
	}
	return check(target, adapter.OutcomePass, "cert-manager webhook service is routable", map[string]string{"component": "cert-manager-webhook"}, started)
}

func (Adapter) checkValidatingWebhookConfiguration(ctx context.Context, c client.Client) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "admissionregistration.k8s.io/v1", Kind: "ValidatingWebhookConfiguration", Name: "cert-manager-webhook"}
	var config admissionv1.ValidatingWebhookConfiguration
	if err := c.Get(ctx, types.NamespacedName{Name: "cert-manager-webhook"}, &config); err != nil {
		if apierrors.IsNotFound(err) {
			return check(target, adapter.OutcomeFail, "cert-manager validating webhook configuration is missing", map[string]string{"component": "cert-manager-webhook"}, started)
		}
		return check(target, adapter.OutcomeError, fmt.Sprintf("failed to read cert-manager validating webhook configuration: %v", err), map[string]string{"component": "cert-manager-webhook"}, started)
	}
	if err := validateWebhookClients(validatingWebhookClients(config.Webhooks)); err != nil {
		return check(target, adapter.OutcomeFail, "cert-manager validating webhook configuration is not ready", map[string]string{"component": "cert-manager-webhook", "reason": err.Error()}, started)
	}
	return check(target, adapter.OutcomePass, "cert-manager validating webhook configuration is ready", map[string]string{"component": "cert-manager-webhook"}, started)
}

func (Adapter) checkMutatingWebhookConfiguration(ctx context.Context, c client.Client) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "admissionregistration.k8s.io/v1", Kind: "MutatingWebhookConfiguration", Name: "cert-manager-webhook"}
	var config admissionv1.MutatingWebhookConfiguration
	if err := c.Get(ctx, types.NamespacedName{Name: "cert-manager-webhook"}, &config); err != nil {
		if apierrors.IsNotFound(err) {
			return check(target, adapter.OutcomeFail, "cert-manager mutating webhook configuration is missing", map[string]string{"component": "cert-manager-webhook"}, started)
		}
		return check(target, adapter.OutcomeError, fmt.Sprintf("failed to read cert-manager mutating webhook configuration: %v", err), map[string]string{"component": "cert-manager-webhook"}, started)
	}
	if err := validateWebhookClients(mutatingWebhookClients(config.Webhooks)); err != nil {
		return check(target, adapter.OutcomeFail, "cert-manager mutating webhook configuration is not ready", map[string]string{"component": "cert-manager-webhook", "reason": err.Error()}, started)
	}
	return check(target, adapter.OutcomePass, "cert-manager mutating webhook configuration is ready", map[string]string{"component": "cert-manager-webhook"}, started)
}

type webhookClient struct {
	name   string
	client admissionv1.WebhookClientConfig
}

func validatingWebhookClients(webhooks []admissionv1.ValidatingWebhook) []webhookClient {
	clients := make([]webhookClient, 0, len(webhooks))
	for _, webhook := range webhooks {
		clients = append(clients, webhookClient{name: webhook.Name, client: webhook.ClientConfig})
	}
	return clients
}

func mutatingWebhookClients(webhooks []admissionv1.MutatingWebhook) []webhookClient {
	clients := make([]webhookClient, 0, len(webhooks))
	for _, webhook := range webhooks {
		clients = append(clients, webhookClient{name: webhook.Name, client: webhook.ClientConfig})
	}
	return clients
}

func validateWebhookClients(clients []webhookClient) error {
	if len(clients) == 0 {
		return fmt.Errorf("no webhooks configured")
	}
	for _, webhook := range clients {
		if webhook.client.Service == nil {
			return fmt.Errorf("webhook %q does not target a service", webhook.name)
		}
		if webhook.client.Service.Name != "cert-manager-webhook" || webhook.client.Service.Namespace != defaultNamespace {
			return fmt.Errorf("webhook %q targets service %s/%s", webhook.name, webhook.client.Service.Namespace, webhook.client.Service.Name)
		}
		if len(webhook.client.CABundle) == 0 {
			return fmt.Errorf("webhook %q has no CA bundle", webhook.name)
		}
	}
	return nil
}

func (Adapter) checkAdmissionDryRun(ctx context.Context, c client.Client, namespace string) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "Certificate", Namespace: namespace, Name: "fathom-webhook-probe"}
	issuer := dryRunIssuer(namespace)
	if err := c.Create(ctx, issuer, dryRunCreate()); err != nil {
		return check(target, adapter.OutcomeError, fmt.Sprintf("cert-manager issuer dry-run admission failed: %v", err), map[string]string{"component": "cert-manager-webhook", "resource": "Issuer"}, started)
	}
	certificate := dryRunCertificate(namespace)
	if err := c.Create(ctx, certificate, dryRunCreate()); err != nil {
		return check(target, adapter.OutcomeError, fmt.Sprintf("cert-manager certificate dry-run admission failed: %v", err), map[string]string{"component": "cert-manager-webhook", "resource": "Certificate"}, started)
	}
	return check(target, adapter.OutcomePass, "cert-manager issuer and certificate dry-run admission succeeded", map[string]string{"component": "cert-manager-webhook"}, started)
}

func dryRunCreate() *client.CreateOptions {
	return &client.CreateOptions{DryRun: []string{metav1.DryRunAll}}
}

func dryRunIssuer(namespace string) *unstructured.Unstructured {
	issuer := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "Issuer",
		"metadata": map[string]any{
			"name":      "fathom-webhook-probe",
			"namespace": namespace,
		},
		"spec": map[string]any{
			"selfSigned": map[string]any{},
		},
	}}
	return issuer
}

func dryRunCertificate(namespace string) *unstructured.Unstructured {
	certificate := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "Certificate",
		"metadata": map[string]any{
			"name":      "fathom-webhook-probe",
			"namespace": namespace,
		},
		"spec": map[string]any{
			"secretName": "fathom-webhook-probe-tls",
			"dnsNames":   []any{"fathom-webhook-probe.local"},
			"issuerRef": map[string]any{
				"name": "fathom-webhook-probe",
				"kind": "Issuer",
			},
		},
	}}
	return certificate
}

func crdEstablished(crd *apixv1.CustomResourceDefinition) bool {
	for _, condition := range crd.Status.Conditions {
		if condition.Type == apixv1.Established {
			return condition.Status == apixv1.ConditionTrue
		}
	}
	return false
}

func crdServesV1(crd *apixv1.CustomResourceDefinition) bool {
	for _, version := range crd.Spec.Versions {
		if version.Name == "v1" && version.Served {
			return true
		}
	}
	return false
}

func webhookProbeEnabled(policy adapter.FamilyPolicy) bool {
	return policy.Thresholds != nil && policy.Thresholds[thresholdWebhookProbe] == "true"
}

func skipped(target adapter.TargetRef, summary string) adapter.CheckResult {
	return adapter.CheckResult{Family: FamilySystemHealth, Outcome: adapter.OutcomeSkipped, TargetRef: target, Summary: summary, ObservedAt: time.Now()}
}

func check(target adapter.TargetRef, outcome adapter.Outcome, summary string, details map[string]string, started time.Time) adapter.CheckResult {
	return adapter.CheckResult{
		Family:     FamilySystemHealth,
		Outcome:    outcome,
		TargetRef:  target,
		Summary:    summary,
		Details:    details,
		ObservedAt: time.Now(),
		Duration:   time.Since(started),
	}
}
