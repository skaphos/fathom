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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/skaphos/fathom/internal/adapter/crdutil"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/pkg/adapter"
)

// tracer is the OpenTelemetry instrumentation scope for the cert-manager
// adapter. It draws from the global provider, so it is a no-op unless the
// operator enabled tracing (SKA-293).
var tracer = otel.Tracer("github.com/skaphos/fathom/internal/adapter/certmanager")

const (
	Name               = "cert-manager"
	Version            = "0.1.1"
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
	thresholdControllerName   = "controllerName"
	thresholdWebhookName      = "webhookName"
	thresholdCainjectorName   = "cainjectorName"
	thresholdWebhookService   = "webhookServiceName"
	thresholdWebhookConfig    = "webhookConfigName"
)

// crdSupport pairs a cert-manager CRD with the API versions Fathom understands
// for it, in descending preference. Each CRD carries its own slice so a future
// heterogeneous CRD — one that ships a v2 or a beta while the others stay v1 —
// is a one-line addition rather than a structural change (SKA-425).
type crdSupport struct {
	name     string
	versions []string
}

var (
	defaultComponents = []string{"cert-manager", "cert-manager-webhook", "cert-manager-cainjector"}
	// crds is the fixed set of cert-manager CRDs Fathom verifies, each with its
	// own descending-preference served-version list. cert-manager has been stable
	// at v1 since 1.6 (2021), so every entry is {"v1"} today; a CRD that ships a
	// v2 or a beta becomes a one-line entry with its own list, no shared global.
	crds = []crdSupport{
		{"certificates.cert-manager.io", []string{"v1"}},
		{"certificaterequests.cert-manager.io", []string{"v1"}},
		{"challenges.acme.cert-manager.io", []string{"v1"}},
		{"clusterissuers.cert-manager.io", []string{"v1"}},
		{"issuers.cert-manager.io", []string{"v1"}},
		{"orders.acme.cert-manager.io", []string{"v1"}},
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

// RBACRules declares cert-manager's least-privilege grants (adapter.RBACDeclarer):
// the passive workload/webhook/CRD/cert-manager reads, plus the create verb the
// admission dry-run probe needs — all under cert-manager's impersonated
// ServiceAccount (SKA-58). Each rule carries a defensive Justification (why it is
// needed and why less would not suffice); the generator emits a scoped ClusterRole
// from these and renders the justifications into docs/reference/rbac.md, and the
// guard permits the certificates;issuers create only because it is justified.
func (Adapter) RBACRules() []adapter.PolicyRule {
	return []adapter.PolicyRule{
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read the cert-manager controller/webhook/cainjector Deployments to score rollout readiness. list+watch (not just get) because the component names are policy-overridable and may span namespaces; read-only — health is derived from status, never mutated."},
		{APIGroups: []string{""}, Resources: []string{"pods", "services"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read component Pods (restart counts, readiness) and the webhook Service (that the admission endpoint exists) before probing. list is required because Pod names are dynamic (ReplicaSet hashes); read-only and scoped to these two core kinds, not the core group at large."},
		{APIGroups: []string{"admissionregistration.k8s.io"}, Resources: []string{"mutatingwebhookconfigurations", "validatingwebhookconfigurations"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read cert-manager's webhook configurations to confirm its admission wiring is present. Read-only — Fathom never edits webhook config; a narrower single-name get is impossible because the configuration names vary by install."},
		{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read the cert-manager CRDs to verify they are Established and serve the expected API version. list is needed to check several CRDs and to distinguish absent from unhealthy; read-only."},
		{APIGroups: []string{"cert-manager.io"}, Resources: []string{"certificates", "clusterissuers", "issuers"}, Verbs: []string{"get", "list", "watch"},
			Justification: "Read Certificate/Issuer/ClusterIssuer status to score issuance health. Scoped to cert-manager's own API group only (not a blanket custom-resource read) and read-only."},
		{APIGroups: []string{"cert-manager.io"}, Resources: []string{"certificates", "issuers"}, Verbs: []string{"create"},
			Justification: "WRITE EXCEPTION: create a Certificate/Issuer with DryRunAll to exercise the admission webhook path without persisting anything. create is the only verb that triggers admission, and DryRun guarantees no object is stored; a read cannot prove the webhook is actually serving. Scoped to these two kinds only, no update/delete."},
	}
}

// cert-manager's cluster permissions are NOT granted to the operator ServiceAccount.
// They live on this adapter's per-addon ServiceAccount (RBACRules above), generated
// into config/rbac/addons/addon-cert-manager.yaml; the operator only impersonates
// that ServiceAccount at run time (SKA-58). No +kubebuilder:rbac markers here.

func (a Adapter) Run(ctx context.Context, req adapter.Request) (result adapter.Result, err error) {
	ctx, span := tracer.Start(ctx, Name+".run")
	span.SetAttributes(attribute.String("fathom.adapter", Name))
	defer func() { endAdapterRunSpan(span, result, err) }()

	started := time.Now()
	systemPolicy, enabled := familyPolicy(req.Policy, FamilySystemHealth, true)
	if !enabled && !familyEnabled(req.Policy, FamilyIssuerHealth) && !familyEnabled(req.Policy, FamilyCertHealth) {
		return adapter.Result{
			Checks:   []adapter.CheckResult{skipped(FamilySystemHealth, req.Target, "system_health family is disabled by policy")},
			Duration: time.Since(started),
		}, nil
	}

	checks := make([]adapter.CheckResult, 0, len(certManagerComponents(systemPolicy))*2+len(crds)+1)
	namespaces := systemPolicy.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{defaultNamespace}
	}

	// Per-family timing for fathom_adapter_run_duration_seconds: time each
	// family independently so a multi-family run does not observe the same
	// wall-clock duration more than once (SKA-290).
	var issuerDur, certDur time.Duration
	systemStart := time.Now()
	if enabled {
		restartWarnCount := restartWarnCount(systemPolicy)

		for _, namespace := range namespaces {
			for _, component := range certManagerComponents(systemPolicy) {
				deployment, check := a.checkDeployment(ctx, req.Client, namespace, component)
				checks = append(checks, check)
				if deployment != nil {
					checks = append(checks, a.checkPods(ctx, req.Client, deployment, restartWarnCount)...)
				}
			}
		}
		for _, cv := range crds {
			checks = append(checks, a.checkCRD(ctx, req.Client, cv.name, cv.versions))
		}
		if webhookProbeEnabled(systemPolicy) {
			webhookServiceName := stringThreshold(systemPolicy, thresholdWebhookService, stringThreshold(systemPolicy, thresholdWebhookName, "cert-manager-webhook"))
			webhookConfigName := stringThreshold(systemPolicy, thresholdWebhookConfig, webhookServiceName)
			for _, namespace := range namespaces {
				checks = append(checks, a.checkWebhookService(ctx, req.Client, namespace, webhookServiceName))
			}
			checks = append(checks, a.checkValidatingWebhookConfiguration(ctx, req.Client, webhookConfigName, webhookServiceName, namespaces[0]))
			checks = append(checks, a.checkMutatingWebhookConfiguration(ctx, req.Client, webhookConfigName, webhookServiceName, namespaces[0]))
			for _, namespace := range namespaces {
				checks = append(checks, a.checkAdmissionDryRun(ctx, req.Client, namespace))
			}
		}
	}
	systemDur := time.Since(systemStart)
	if issuerPolicy, ok := familyPolicy(req.Policy, FamilyIssuerHealth, false); ok {
		issuerStart := time.Now()
		checks = append(checks, a.checkIssuers(ctx, req.Client, issuerPolicy)...)
		issuerDur = time.Since(issuerStart)
	}
	if certPolicy, ok := familyPolicy(req.Policy, FamilyCertHealth, false); ok {
		certStart := time.Now()
		checks = append(checks, a.checkCertificates(ctx, req.Client, certPolicy)...)
		certDur = time.Since(certStart)
	}

	// Record per family that ran, with that family's own duration. FamilyOutcome
	// considers only that family's checks, so one family's failure does not
	// taint another's metric (SKA-290).
	if _, enabled := familyPolicy(req.Policy, FamilySystemHealth, true); enabled {
		metrics.RecordAdapterRun(Name, string(FamilySystemHealth), string(adapter.FamilyOutcome(checks, FamilySystemHealth)), systemDur)
	}
	if _, ok := familyPolicy(req.Policy, FamilyIssuerHealth, false); ok {
		metrics.RecordAdapterRun(Name, string(FamilyIssuerHealth), string(adapter.FamilyOutcome(checks, FamilyIssuerHealth)), issuerDur)
	}
	if _, ok := familyPolicy(req.Policy, FamilyCertHealth, false); ok {
		metrics.RecordAdapterRun(Name, string(FamilyCertHealth), string(adapter.FamilyOutcome(checks, FamilyCertHealth)), certDur)
	}

	return adapter.Result{Checks: checks, Duration: time.Since(started)}, nil
}

// endAdapterRunSpan annotates span with the check count and a per-family
// outcome (the same adapter.FamilyOutcome roll-up the per-family metrics use),
// records err, and ends the span. Only families that actually produced checks
// are tagged, so a disabled family is not mislabeled as a passing one.
func endAdapterRunSpan(span trace.Span, result adapter.Result, err error) {
	span.SetAttributes(attribute.Int("fathom.adapter.check_count", len(result.Checks)))
	seen := map[adapter.Family]struct{}{}
	for _, c := range result.Checks {
		if _, ok := seen[c.Family]; ok {
			continue
		}
		seen[c.Family] = struct{}{}
		span.SetAttributes(attribute.String(
			"fathom.outcome."+string(c.Family),
			string(adapter.FamilyOutcome(result.Checks, c.Family)),
		))
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
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

func certManagerComponents(policy adapter.FamilyPolicy) []string {
	return []string{
		stringThreshold(policy, thresholdControllerName, defaultComponents[0]),
		stringThreshold(policy, thresholdWebhookName, defaultComponents[1]),
		stringThreshold(policy, thresholdCainjectorName, defaultComponents[2]),
	}
}

func stringThreshold(policy adapter.FamilyPolicy, key, defaultValue string) string {
	if policy.Thresholds == nil {
		return defaultValue
	}
	value := strings.TrimSpace(policy.Thresholds[key])
	if value == "" {
		return defaultValue
	}
	return value
}

func (Adapter) checkDeployment(ctx context.Context, c client.Client, namespace, name string) (*appsv1.Deployment, adapter.CheckResult) {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: namespace, Name: name}
	var deployment appsv1.Deployment
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager deployment is missing", adapter.MarkAbsent(map[string]string{"component": name}), started)
		}
		return nil, check(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("failed to read cert-manager deployment: %v", err), map[string]string{"component": name}, started)
	}

	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	if desired == 0 {
		return &deployment, check(FamilySystemHealth, target, adapter.OutcomeWarn, "cert-manager deployment is scaled to zero", map[string]string{"component": name}, started)
	}
	if !deploymentAvailable(&deployment) || deployment.Status.AvailableReplicas < desired {
		details := map[string]string{
			"component":         name,
			"desiredReplicas":   strconv.FormatInt(int64(desired), 10),
			"availableReplicas": strconv.FormatInt(int64(deployment.Status.AvailableReplicas), 10),
		}
		return &deployment, check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager deployment is not fully available", details, started)
	}
	return &deployment, check(FamilySystemHealth, target, adapter.OutcomePass, "cert-manager deployment is available", map[string]string{"component": name}, started)
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
		return []adapter.CheckResult{check(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("deployment has invalid pod selector: %v", err), map[string]string{"component": deployment.Name}, started)}
	}
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(deployment.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return []adapter.CheckResult{check(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("failed to list cert-manager pods: %v", err), map[string]string{"component": deployment.Name}, started)}
	}
	if len(pods.Items) == 0 {
		return []adapter.CheckResult{check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager deployment has no matching pods", map[string]string{"component": deployment.Name}, started)}
	}

	checks := make([]adapter.CheckResult, 0, len(pods.Items))
	for _, pod := range pods.Items {
		if !podReady(&pod) {
			checks = append(checks, check(FamilySystemHealth, podTarget(&pod), adapter.OutcomeFail, "cert-manager pod is not ready", map[string]string{"component": deployment.Name, "phase": string(pod.Status.Phase)}, started))
			continue
		}
		if restarts := maxRestartCount(&pod); restarts > restartWarnCount {
			checks = append(checks, check(FamilySystemHealth, podTarget(&pod), adapter.OutcomeWarn, "cert-manager pod restart count exceeds warning threshold", map[string]string{
				"component":        deployment.Name,
				"restartCount":     strconv.FormatInt(int64(restarts), 10),
				"restartWarnCount": strconv.FormatInt(int64(restartWarnCount), 10),
			}, started))
			continue
		}
		checks = append(checks, check(FamilySystemHealth, podTarget(&pod), adapter.OutcomePass, "cert-manager pod is ready", map[string]string{"component": deployment.Name}, started))
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

func (Adapter) checkCRD(ctx context.Context, c client.Client, name string, versions []string) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "apiextensions.k8s.io/v1", Kind: "CustomResourceDefinition", Name: name}
	var crd apixv1.CustomResourceDefinition
	if err := c.Get(ctx, types.NamespacedName{Name: name}, &crd); err != nil {
		if apierrors.IsNotFound(err) {
			return check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager CRD is missing", adapter.MarkAbsent(map[string]string{"crd": name}), started)
		}
		return check(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("failed to read cert-manager CRD: %v", err), map[string]string{"crd": name}, started)
	}
	details := map[string]string{"crd": name}
	if !crdutil.Established(&crd) {
		return check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager CRD is not established", details, started)
	}
	servedVersion, ok := crdutil.PreferredServedVersion(&crd, versions)
	if !ok {
		details["expectedVersions"] = strings.Join(versions, ",")
		return check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager CRD serves no supported version", details, started)
	}
	details["version"] = servedVersion
	return check(FamilySystemHealth, target, adapter.OutcomePass, "cert-manager CRD is established", details, started)
}

func (Adapter) checkWebhookService(ctx context.Context, c client.Client, namespace, serviceName string) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "v1", Kind: "Service", Namespace: namespace, Name: serviceName}
	var service corev1.Service
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: serviceName}, &service); err != nil {
		if apierrors.IsNotFound(err) {
			return check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager webhook service is missing", adapter.MarkAbsent(map[string]string{"component": serviceName}), started)
		}
		return check(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("failed to read cert-manager webhook service: %v", err), map[string]string{"component": serviceName}, started)
	}
	if service.Spec.ClusterIP == "" || service.Spec.ClusterIP == corev1.ClusterIPNone {
		return check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager webhook service has no cluster IP", map[string]string{"component": serviceName}, started)
	}
	return check(FamilySystemHealth, target, adapter.OutcomePass, "cert-manager webhook service is routable", map[string]string{"component": serviceName}, started)
}

func (Adapter) checkValidatingWebhookConfiguration(ctx context.Context, c client.Client, configName, serviceName, serviceNamespace string) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "admissionregistration.k8s.io/v1", Kind: "ValidatingWebhookConfiguration", Name: configName}
	var config admissionv1.ValidatingWebhookConfiguration
	if err := c.Get(ctx, types.NamespacedName{Name: configName}, &config); err != nil {
		if apierrors.IsNotFound(err) {
			return check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager validating webhook configuration is missing", adapter.MarkAbsent(map[string]string{"component": configName}), started)
		}
		return check(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("failed to read cert-manager validating webhook configuration: %v", err), map[string]string{"component": configName}, started)
	}
	if err := validateWebhookClients(validatingWebhookClients(config.Webhooks), serviceName, serviceNamespace); err != nil {
		return check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager validating webhook configuration is not ready", map[string]string{"component": configName, "reason": err.Error()}, started)
	}
	return check(FamilySystemHealth, target, adapter.OutcomePass, "cert-manager validating webhook configuration is ready", map[string]string{"component": configName}, started)
}

func (Adapter) checkMutatingWebhookConfiguration(ctx context.Context, c client.Client, configName, serviceName, serviceNamespace string) adapter.CheckResult {
	started := time.Now()
	target := adapter.TargetRef{APIVersion: "admissionregistration.k8s.io/v1", Kind: "MutatingWebhookConfiguration", Name: configName}
	var config admissionv1.MutatingWebhookConfiguration
	if err := c.Get(ctx, types.NamespacedName{Name: configName}, &config); err != nil {
		if apierrors.IsNotFound(err) {
			return check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager mutating webhook configuration is missing", adapter.MarkAbsent(map[string]string{"component": configName}), started)
		}
		return check(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("failed to read cert-manager mutating webhook configuration: %v", err), map[string]string{"component": configName}, started)
	}
	if err := validateWebhookClients(mutatingWebhookClients(config.Webhooks), serviceName, serviceNamespace); err != nil {
		return check(FamilySystemHealth, target, adapter.OutcomeFail, "cert-manager mutating webhook configuration is not ready", map[string]string{"component": configName, "reason": err.Error()}, started)
	}
	return check(FamilySystemHealth, target, adapter.OutcomePass, "cert-manager mutating webhook configuration is ready", map[string]string{"component": configName}, started)
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

func validateWebhookClients(clients []webhookClient, serviceName, serviceNamespace string) error {
	if len(clients) == 0 {
		return fmt.Errorf("no webhooks configured")
	}
	for _, webhook := range clients {
		if webhook.client.Service == nil {
			return fmt.Errorf("webhook %q does not target a service", webhook.name)
		}
		if webhook.client.Service.Name != serviceName || webhook.client.Service.Namespace != serviceNamespace {
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
		return check(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("cert-manager issuer dry-run admission failed: %v", err), map[string]string{"component": "cert-manager-webhook", "resource": "Issuer"}, started)
	}
	certificate := dryRunCertificate(namespace)
	if err := c.Create(ctx, certificate, dryRunCreate()); err != nil {
		return check(FamilySystemHealth, target, adapter.OutcomeError, fmt.Sprintf("cert-manager certificate dry-run admission failed: %v", err), map[string]string{"component": "cert-manager-webhook", "resource": "Certificate"}, started)
	}
	return check(FamilySystemHealth, target, adapter.OutcomePass, "cert-manager issuer and certificate dry-run admission succeeded", map[string]string{"component": "cert-manager-webhook"}, started)
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
					checks = append(checks, check(FamilyIssuerHealth, adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "Issuer", Namespace: namespace, Name: "issuers"}, adapter.OutcomeError, fmt.Sprintf("issuer label selector is invalid: %v", err), nil, time.Now()))
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
				checks = append(checks, check(FamilyIssuerHealth, adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "ClusterIssuer", Name: "clusterissuers"}, adapter.OutcomeError, fmt.Sprintf("clusterissuer label selector is invalid: %v", err), nil, time.Now()))
			} else {
				listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
			}
		}
		checks = append(checks, listIssuerObjects(ctx, c, &issuers, listOpts, "ClusterIssuer", "")...)
	}
	if len(checks) == 0 {
		checks = append(checks, skipped(FamilyIssuerHealth, adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "Issuer", Name: "issuers"}, "issuer_health found no matching issuers"))
	}
	return checks
}

func listIssuerObjects(ctx context.Context, c client.Client, list *unstructured.UnstructuredList, opts []client.ListOption, kind, namespace string) []adapter.CheckResult {
	started := time.Now()
	if err := c.List(ctx, list, opts...); err != nil {
		return []adapter.CheckResult{check(FamilyIssuerHealth, adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: kind, Namespace: namespace, Name: strings.ToLower(kind) + "s"}, adapter.OutcomeError, fmt.Sprintf("failed to list cert-manager %s resources: %v", strings.ToLower(kind), err), nil, started)}
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
		return check(FamilyIssuerHealth, target, adapter.OutcomeFail, strings.ToLower(obj.GetKind())+" has no Ready condition", details, started)
	}
	if conditionStatus(condition) != "True" {
		return check(FamilyIssuerHealth, target, adapter.OutcomeFail, strings.ToLower(obj.GetKind())+" is not ready", details, started)
	}
	return check(FamilyIssuerHealth, target, adapter.OutcomePass, strings.ToLower(obj.GetKind())+" is ready", details, started)
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
				checks = append(checks, check(FamilyCertHealth, adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "Certificate", Namespace: namespace, Name: "certificates"}, adapter.OutcomeError, fmt.Sprintf("certificate label selector is invalid: %v", err), nil, time.Now()))
				continue
			}
			listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
		}
		started := time.Now()
		if err := c.List(ctx, &certificates, listOpts...); err != nil {
			checks = append(checks, check(FamilyCertHealth, adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "Certificate", Namespace: namespace, Name: "certificates"}, adapter.OutcomeError, fmt.Sprintf("failed to list cert-manager certificates: %v", err), nil, started))
			continue
		}
		for i := range certificates.Items {
			checks = append(checks, certificateCheck(&certificates.Items[i], policy))
		}
	}
	if len(checks) == 0 {
		checks = append(checks, skipped(FamilyCertHealth, adapter.TargetRef{APIVersion: "cert-manager.io/v1", Kind: "Certificate", Name: "certificates"}, "certificate_health found no matching certificates"))
	}
	return checks
}

func certificateCheck(obj *unstructured.Unstructured, policy adapter.FamilyPolicy) adapter.CheckResult {
	started := time.Now()
	target := objectTarget(obj)
	condition := readyCondition(obj)
	details := certificateDetails(obj, condition)
	if condition == nil {
		return check(FamilyCertHealth, target, adapter.OutcomeFail, "certificate has no Ready condition", details, started)
	}
	if conditionStatus(condition) != "True" {
		return check(FamilyCertHealth, target, adapter.OutcomeFail, "certificate is not ready", details, started)
	}
	if renewalTime, ok := stringAt(obj.Object, "status", "renewalTime"); ok {
		details["renewalTime"] = renewalTime
		if parsed, err := time.Parse(time.RFC3339, renewalTime); err == nil && time.Now().After(parsed) {
			return check(FamilyCertHealth, target, adapter.OutcomeWarn, "certificate renewal time has passed", details, started)
		}
	}
	if notAfter, ok := stringAt(obj.Object, "status", "notAfter"); ok {
		details["notAfter"] = notAfter
		parsed, err := time.Parse(time.RFC3339, notAfter)
		if err != nil {
			return check(FamilyCertHealth, target, adapter.OutcomeError, "certificate expiry timestamp is invalid", details, started)
		}
		remaining := time.Until(parsed)
		details["daysRemaining"] = strconv.Itoa(daysRemaining(remaining))
		if remaining <= daysThreshold(policy, thresholdFailDays, defaultFailDays) {
			return check(FamilyCertHealth, target, adapter.OutcomeFail, "certificate expires within failDays threshold", details, started)
		}
		if remaining <= daysThreshold(policy, thresholdWarnDays, defaultWarnDays) {
			return check(FamilyCertHealth, target, adapter.OutcomeWarn, "certificate expires within warnDays threshold", details, started)
		}
	}
	return check(FamilyCertHealth, target, adapter.OutcomePass, "certificate is ready", details, started)
}

func policyNamespaces(policy adapter.FamilyPolicy) []string {
	if len(policy.Namespaces) == 0 {
		return []string{""}
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

func webhookProbeEnabled(policy adapter.FamilyPolicy) bool {
	return policy.Thresholds != nil && policy.Thresholds[thresholdWebhookProbe] == "true"
}

// skipped emits a CheckResult marking the named target as intentionally
// not executed. Family is required so empty-list contracts surface under
// the policy family that gated them (issuer_health, certificate_health,
// system_health, …) rather than collapsing into a single bucket.
func skipped(family adapter.Family, target adapter.TargetRef, summary string) adapter.CheckResult {
	return adapter.CheckResult{Family: family, Outcome: adapter.OutcomeSkipped, TargetRef: target, Summary: summary, ObservedAt: time.Now()}
}

// check emits a CheckResult tagged with the caller's policy family.
// Callers must pass the family that gates the surrounding work so the
// HealthReport's family attribution stays aligned with AddonCheck.spec.policy.
func check(family adapter.Family, target adapter.TargetRef, outcome adapter.Outcome, summary string, details map[string]string, started time.Time) adapter.CheckResult {
	return adapter.CheckResult{
		Family:     family,
		Outcome:    outcome,
		TargetRef:  target,
		Summary:    summary,
		Details:    details,
		ObservedAt: time.Now(),
		Duration:   time.Since(started),
	}
}
