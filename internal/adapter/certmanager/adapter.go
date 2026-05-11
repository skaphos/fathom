/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package certmanager provides the built-in cert-manager addon adapter.
package certmanager

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
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
	FamilyIssuerHealth = adapter.Family("issuer_health")
	FamilyCertHealth   = adapter.Family("certificate_health")

	defaultNamespace          = "cert-manager"
	defaultRestartWarnCount   = int32(3)
	defaultWarnDays           = 30
	defaultFailDays           = 7
	thresholdRestartWarnCount = "restartWarnCount"
	thresholdWebhookProbe     = "webhookProbe"
	thresholdKinds            = "kinds"
	thresholdWarnDays         = "warnDays"
	thresholdFailDays         = "failDays"
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
	return adapter.Capabilities{AddonTypes: []string{Name}, Families: []adapter.Family{FamilySystemHealth, FamilyIssuerHealth, FamilyCertHealth}}
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates;clusterissuers;issuers,verbs=get;list;watch
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates;issuers,verbs=create

func (a Adapter) Run(ctx context.Context, req adapter.Request) (adapter.Result, error) {
	started := time.Now()
	systemPolicy, enabled := familyPolicy(req.Policy, FamilySystemHealth, true)
	if !enabled && !familyEnabled(req.Policy, FamilyIssuerHealth) && !familyEnabled(req.Policy, FamilyCertHealth) {
		return adapter.Result{
			Checks:   []adapter.CheckResult{skipped(req.Target, "system_health family is disabled by policy")},
			Duration: time.Since(started),
		}, nil
	}

	checks := make([]adapter.CheckResult, 0, len(components)*2+len(crds)+1)
	namespaces := systemPolicy.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{defaultNamespace}
	}
	if enabled {
		restartWarnCount := restartWarnCount(systemPolicy)

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
		if webhookProbeEnabled(systemPolicy) {
			for _, namespace := range namespaces {
				checks = append(checks, a.checkWebhookService(ctx, req.Client, namespace))
			}
			checks = append(checks, a.checkValidatingWebhookConfiguration(ctx, req.Client))
			checks = append(checks, a.checkMutatingWebhookConfiguration(ctx, req.Client))
			for _, namespace := range namespaces {
				checks = append(checks, a.checkAdmissionDryRun(ctx, req.Client, namespace))
			}
		}
	}
	if issuerPolicy, ok := familyPolicy(req.Policy, FamilyIssuerHealth, false); ok {
		checks = append(checks, a.checkIssuers(ctx, req.Client, issuerPolicy)...)
	}
	if certPolicy, ok := familyPolicy(req.Policy, FamilyCertHealth, false); ok {
		checks = append(checks, a.checkCertificates(ctx, req.Client, certPolicy)...)
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

func familyEnabled(policy map[adapter.Family]adapter.FamilyPolicy, family adapter.Family) bool {
	_, enabled := familyPolicy(policy, family, false)
	return enabled
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

func (Adapter) checkIssuers(ctx context.Context, c client.Client, policy adapter.FamilyPolicy) []adapter.CheckResult {
	checks := []adapter.CheckResult{}
	if includesKind(policy, "Issuer", true) {
		for _, namespace := range policyNamespaces(policy) {
			var issuers unstructured.UnstructuredList
			issuers.SetAPIVersion("cert-manager.io/v1")
			issuers.SetKind("IssuerList")
			listOpts := []client.ListOption{client.InNamespace(namespace)}
			if policy.LabelSelector != nil {
				selector, err := metav1.LabelSelectorAsSelector(policy.LabelSelector)
				if err != nil {
					checks = append(checks, check(adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "Issuer", Namespace: namespace, Name: namespace}, adapter.OutcomeError, fmt.Sprintf("issuer label selector is invalid: %v", err), nil, time.Now()))
					continue
				}
				listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
			}
			checks = append(checks, listIssuerObjects(ctx, c, &issuers, listOpts, "Issuer", namespace)...)
		}
	}
	if includesKind(policy, "ClusterIssuer", true) {
		var issuers unstructured.UnstructuredList
		issuers.SetAPIVersion("cert-manager.io/v1")
		issuers.SetKind("ClusterIssuerList")
		listOpts := []client.ListOption{}
		if policy.LabelSelector != nil {
			selector, err := metav1.LabelSelectorAsSelector(policy.LabelSelector)
			if err != nil {
				checks = append(checks, check(adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "ClusterIssuer", Name: "clusterissuers"}, adapter.OutcomeError, fmt.Sprintf("clusterissuer label selector is invalid: %v", err), nil, time.Now()))
			} else {
				listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
			}
		}
		checks = append(checks, listIssuerObjects(ctx, c, &issuers, listOpts, "ClusterIssuer", "")...)
	}
	if len(checks) == 0 {
		checks = append(checks, skipped(adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "Issuer", Name: "issuers"}, "issuer_health found no matching issuers"))
	}
	return checks
}

func listIssuerObjects(ctx context.Context, c client.Client, list *unstructured.UnstructuredList, opts []client.ListOption, kind, namespace string) []adapter.CheckResult {
	started := time.Now()
	if err := c.List(ctx, list, opts...); err != nil {
		return []adapter.CheckResult{check(adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: kind, Namespace: namespace, Name: strings.ToLower(kind) + "s"}, adapter.OutcomeError, fmt.Sprintf("failed to list cert-manager %s resources: %v", strings.ToLower(kind), err), nil, started)}
	}
	checks := make([]adapter.CheckResult, 0, len(list.Items))
	for i := range list.Items {
		checks = append(checks, issuerCheck(&list.Items[i]))
	}
	return checks
}

func issuerCheck(obj *unstructured.Unstructured) adapter.CheckResult {
	started := time.Now()
	condition := readyCondition(obj)
	target := objectTarget(obj)
	details := conditionDetails(condition)
	if condition == nil {
		return check(target, adapter.OutcomeFail, strings.ToLower(obj.GetKind())+" has no Ready condition", details, started)
	}
	if conditionStatus(condition) != "True" {
		return check(target, adapter.OutcomeFail, strings.ToLower(obj.GetKind())+" is not ready", details, started)
	}
	return check(target, adapter.OutcomePass, strings.ToLower(obj.GetKind())+" is ready", details, started)
}

func (Adapter) checkCertificates(ctx context.Context, c client.Client, policy adapter.FamilyPolicy) []adapter.CheckResult {
	checks := []adapter.CheckResult{}
	for _, namespace := range policyNamespaces(policy) {
		var certificates unstructured.UnstructuredList
		certificates.SetAPIVersion("cert-manager.io/v1")
		certificates.SetKind("CertificateList")
		listOpts := []client.ListOption{client.InNamespace(namespace)}
		if policy.LabelSelector != nil {
			selector, err := metav1.LabelSelectorAsSelector(policy.LabelSelector)
			if err != nil {
				checks = append(checks, check(adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "Certificate", Namespace: namespace, Name: namespace}, adapter.OutcomeError, fmt.Sprintf("certificate label selector is invalid: %v", err), nil, time.Now()))
				continue
			}
			listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
		}
		started := time.Now()
		if err := c.List(ctx, &certificates, listOpts...); err != nil {
			checks = append(checks, check(adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "Certificate", Namespace: namespace, Name: "certificates"}, adapter.OutcomeError, fmt.Sprintf("failed to list cert-manager certificates: %v", err), nil, started))
			continue
		}
		for i := range certificates.Items {
			checks = append(checks, certificateCheck(&certificates.Items[i], policy))
		}
	}
	if len(checks) == 0 {
		checks = append(checks, skipped(adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "Certificate", Name: "certificates"}, "certificate_health found no matching certificates"))
	}
	return checks
}

func certificateCheck(obj *unstructured.Unstructured, policy adapter.FamilyPolicy) adapter.CheckResult {
	started := time.Now()
	target := objectTarget(obj)
	condition := readyCondition(obj)
	details := certificateDetails(obj, condition)
	if condition == nil {
		return check(target, adapter.OutcomeFail, "certificate has no Ready condition", details, started)
	}
	if conditionStatus(condition) != "True" {
		return check(target, adapter.OutcomeFail, "certificate is not ready", details, started)
	}
	if renewalTime, ok := stringAt(obj.Object, "status", "renewalTime"); ok {
		details["renewalTime"] = renewalTime
		if parsed, err := time.Parse(time.RFC3339, renewalTime); err == nil && time.Now().After(parsed) {
			return check(target, adapter.OutcomeWarn, "certificate renewal time has passed", details, started)
		}
	}
	if notAfter, ok := stringAt(obj.Object, "status", "notAfter"); ok {
		details["notAfter"] = notAfter
		parsed, err := time.Parse(time.RFC3339, notAfter)
		if err != nil {
			return check(target, adapter.OutcomeError, "certificate expiry timestamp is invalid", details, started)
		}
		remaining := time.Until(parsed)
		details["daysRemaining"] = strconv.Itoa(daysRemaining(remaining))
		if remaining <= daysThreshold(policy, thresholdFailDays, defaultFailDays) {
			return check(target, adapter.OutcomeFail, "certificate expires within failDays threshold", details, started)
		}
		if remaining <= daysThreshold(policy, thresholdWarnDays, defaultWarnDays) {
			return check(target, adapter.OutcomeWarn, "certificate expires within warnDays threshold", details, started)
		}
	}
	return check(target, adapter.OutcomePass, "certificate is ready", details, started)
}

func policyNamespaces(policy adapter.FamilyPolicy) []string {
	if len(policy.Namespaces) == 0 {
		return []string{defaultNamespace}
	}
	return policy.Namespaces
}

func includesKind(policy adapter.FamilyPolicy, kind string, defaultIncluded bool) bool {
	if policy.Thresholds == nil || policy.Thresholds[thresholdKinds] == "" {
		return defaultIncluded
	}
	for _, candidate := range strings.Split(policy.Thresholds[thresholdKinds], ",") {
		if strings.EqualFold(strings.TrimSpace(candidate), kind) {
			return true
		}
	}
	return false
}

func daysThreshold(policy adapter.FamilyPolicy, key string, defaultDays int) time.Duration {
	if policy.Thresholds == nil {
		return time.Duration(defaultDays) * 24 * time.Hour
	}
	value, ok := policy.Thresholds[key]
	if !ok {
		return time.Duration(defaultDays) * 24 * time.Hour
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return time.Duration(defaultDays) * 24 * time.Hour
	}
	return time.Duration(parsed) * 24 * time.Hour
}

func daysRemaining(remaining time.Duration) int {
	days := remaining.Hours() / 24
	if days < 0 {
		return int(math.Floor(days))
	}
	return int(math.Ceil(days))
}

func objectTarget(obj *unstructured.Unstructured) adapter.TargetRef {
	return adapter.TargetRef{APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Namespace: obj.GetNamespace(), Name: obj.GetName()}
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
	for _, key := range []string{"reason", "message", "status"} {
		if value, ok := condition[key].(string); ok && value != "" {
			details[key] = value
		}
	}
	return details
}

func certificateDetails(obj *unstructured.Unstructured, condition map[string]any) map[string]string {
	details := conditionDetails(condition)
	for key, path := range map[string][]string{
		"secretName":               {"spec", "secretName"},
		"issuerRefName":            {"spec", "issuerRef", "name"},
		"issuerRefKind":            {"spec", "issuerRef", "kind"},
		"issuerRefGroup":           {"spec", "issuerRef", "group"},
		"revision":                 {"status", "revision"},
		"nextPrivateKeySecretName": {"status", "nextPrivateKeySecretName"},
	} {
		if value, ok := stringAt(obj.Object, path...); ok {
			details[key] = value
		}
	}
	return details
}

func stringAt(obj map[string]any, fields ...string) (string, bool) {
	value, ok, _ := unstructured.NestedString(obj, fields...)
	return value, ok
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
