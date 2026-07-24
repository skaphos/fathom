/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import "github.com/skaphos/fathom/pkg/adapter"

// AzureWorkloadIdentityDefinition is the declarative Azure Workload Identity
// webhook adapter (skaphos/fathom#185). It validates the mutating admission
// path that projects federated identity tokens into opted-in pods:
//
//   - system_health verifies the azure-wi-webhook-controller-manager
//     Deployment and its pods.
//   - webhook_wiring verifies the azure-wi-webhook-mutating-webhook-configuration
//     MutatingWebhookConfiguration is present, every entry carries a populated
//     caBundle and points at the azure-wi-webhook-webhook-service Service, and
//     that service has ready endpoints. The webhook manages its own serving
//     cert and patches the caBundle at runtime, so an unpopulated bundle means
//     its cert rotation is broken.
//   - projection_sanity verifies pods opted in via the
//     azure.workload.identity/use=true label actually carry the injection the
//     webhook performs: the azure-identity-token projected serviceAccountToken
//     volume and the AZURE_FEDERATED_TOKEN_FILE env var in every non-init
//     container (init containers are not inspected).
//     This is the silent-failure mode the adapter exists for (#185): the
//     webhook's objectSelector only matches labeled pods, so if the
//     configuration is deleted — or admission stops mutating — labeled pods
//     are created WITHOUT their federated identity and nothing else in the
//     cluster ever flags them. A cluster with no opted-in pods is Skipped.
//
// Everything here is read-only against the Kubernetes API. Whether the
// projected token actually exchanges for an Entra ID credential is a
// cloud-side concern this adapter deliberately does not claim to verify.
//
// Names assume the upstream workload-identity-webhook Helm chart, which
// hardcodes the azure-wi-webhook- prefix; the install namespace is
// policy-overridable (the chart deploys wherever the release is installed,
// conventionally azure-workload-identity-system), and the Deployment and
// configuration names have threshold overrides for repackaged installs.
var AzureWorkloadIdentityDefinition = AddonDefinition{
	AddonType:      "azure-workload-identity",
	AdapterVersion: "0.1.0",
	// Detect the installed webhook version off the controller Deployment (its
	// app.kubernetes.io/version label, else the webhook image tag, e.g.
	// v1.6.0). Detection-only: SupportedVersions is left empty so the adapter
	// never Warns on a version — gating is opt-in (SKA-527).
	VersionSource: &VersionSource{FromFamily: adapter.Family("system_health")},
	RBAC: []adapter.PolicyRule{
		// Verbs mirror the engine's actual reads through the direct (uncached)
		// impersonating client: the Deployment and the webhook configuration are
		// fetched by name (get); Pods and EndpointSlices by label selector
		// (list). Nothing watches — the client is deliberately cache-free (see
		// internal/adapter/impersonation).
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"},
			Justification: "Get the azure-wi-webhook-controller-manager Deployment by name to score webhook readiness and detect the installed version. The name/namespace are policy-overridable but always resolve to a single named Get; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"},
			Justification: "List Pods by label selector twice over: the webhook's own pods behind the Deployment (readiness and restart counts), and pods opted in via azure.workload.identity/use=true to verify the federated-token projection was actually injected — the projection_sanity signal. list (not get) because pod names are dynamic; read-only."},
		{APIGroups: []string{"admissionregistration.k8s.io"}, Resources: []string{"mutatingwebhookconfigurations"}, Verbs: []string{"get"},
			Justification: "Get the azure-wi-webhook-mutating-webhook-configuration by name to verify admission wiring (caBundle populated, backed by the webhook Service). get only — exactly one named Get; read-only."},
		{APIGroups: []string{"discovery.k8s.io"}, Resources: []string{"endpointslices"}, Verbs: []string{"list"},
			Justification: "List the EndpointSlices labeled with the azure-wi-webhook-webhook-service name to verify the admission endpoint has ready backends. list (not get) because slice names are dynamic; read-only."},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("system_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDeployment,
				DefaultNamespace:        "azure-workload-identity-system",
				NameThresholdKey:        "deploymentName",
				DefaultName:             "azure-wi-webhook-controller-manager",
				Component:               "azure-wi-webhook",
				Absence:                 Required,
				CheckPods:               true,
				RestartWarnThresholdKey: "restartWarnCount",
				DefaultRestartWarn:      3,
			}},
		},
		{
			Name:           adapter.Family("webhook_wiring"),
			DefaultEnabled: true,
			Webhooks: []WebhookCheck{{
				Kind:             KindMutatingWebhookConfiguration,
				Name:             "azure-wi-webhook-mutating-webhook-configuration",
				NameThresholdKey: "webhookName",
				ExpectedService:  "azure-wi-webhook-webhook-service",
				ServiceNamespace: "azure-workload-identity-system",
				// Required: a deleted configuration is exactly the silent
				// failure this adapter exists to catch — labeled pods would be
				// admitted unmutated with no error anywhere.
				Absence: Required,
				// The webhook family stands apart from system_health, so assert
				// the backing service's endpoint readiness here instead of
				// relying on the workload check transitively.
				VerifyEndpoints: true,
			}},
		},
		{
			Name:           adapter.Family("projection_sanity"),
			DefaultEnabled: true,
			PodProjections: []PodProjectionCheck{{
				Selector:   map[string]string{"azure.workload.identity/use": "true"},
				ListName:   "workload-pods",
				Component:  "azure-wi-webhook",
				VolumeName: "azure-identity-token",
				EnvVar:     "AZURE_FEDERATED_TOKEN_FILE",
			}},
		},
	},
}

// NewAzureWorkloadIdentityEngine returns the declarative Azure Workload
// Identity adapter. RBAC markers live on the package doc in definition.go.
func NewAzureWorkloadIdentityEngine() *Engine {
	return MustEngine(AzureWorkloadIdentityDefinition)
}
