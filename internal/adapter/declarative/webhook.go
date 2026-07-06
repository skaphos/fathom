/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/skaphos/fathom/pkg/adapter"
)

// webhookEntry is the kind-neutral view of one webhooks[] element shared by
// the mutating and validating configuration types.
type webhookEntry struct {
	name     string
	caBundle []byte
	service  *admissionv1.ServiceReference
}

// appendEntry converts one webhooks[] element to the kind-neutral view. Both
// configuration kinds funnel through here so the captured fields can never
// drift between the mutating and validating arms.
func appendEntry(entries []webhookEntry, name string, cc admissionv1.WebhookClientConfig) []webhookEntry {
	return append(entries, webhookEntry{name: name, caBundle: cc.CABundle, service: cc.Service})
}

// Evaluate implements Evaluator for WebhookCheck. It fetches the named
// admission webhook configuration (a cluster-scoped singleton; the name is
// policy-overridable via NameThresholdKey) and scores its wiring:
//
//   - NotFound -> the effective Absence posture (Required, the default -> Fail;
//     Optional -> Skipped), tagged with the adapter.DetailAbsent marker.
//   - Zero webhooks[] entries -> Fail: an entry-less configuration admits
//     nothing, so the addon's admission path is silently inert.
//   - A service-based entry with an empty clientConfig.caBundle -> Fail. This
//     is the real-world failure mode the check exists for: the addon's CA
//     injection (istiod, cert-manager cainjector, ...) has not patched the
//     bundle, so the API server cannot trust the webhook and admission
//     requests fail — or, with failurePolicy Ignore, silently stop being
//     enforced. URL-based entries are exempt: the API may legally omit their
//     caBundle, in which case the API server verifies the webhook's
//     certificate against its system trust roots.
//   - When ExpectedService is set, an entry whose clientConfig does not
//     reference the expected backing service (including URL-based
//     clientConfigs) -> Fail. The expected namespace follows the family's
//     resolved namespace (first policy namespace, else ServiceNamespace) —
//     the backing service lives wherever the addon was installed.
//
// Whether the backing service's endpoints are ready is deliberately not
// checked here: the serving workload is the same one a WorkloadCheck in the
// family already scores, so endpoint readiness is covered transitively.
func (wc WebhookCheck) Evaluate(ec EvalContext) ([]adapter.CheckResult, error) {
	started := time.Now()
	name := wc.Name
	if wc.NameThresholdKey != "" {
		name = stringThreshold(ec.Policy, wc.NameThresholdKey, wc.Name)
	}
	ref := adapter.TargetRef{APIVersion: "admissionregistration.k8s.io/v1", Kind: wc.Kind, Name: name}

	var entries []webhookEntry
	switch wc.Kind {
	case KindMutatingWebhookConfiguration:
		var cfg admissionv1.MutatingWebhookConfiguration
		if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Name: name}, &cfg); err != nil {
			return []adapter.CheckResult{wc.readErrorResult(ec, ref, err, started)}, nil
		}
		for i := range cfg.Webhooks {
			entries = appendEntry(entries, cfg.Webhooks[i].Name, cfg.Webhooks[i].ClientConfig)
		}
	case KindValidatingWebhookConfiguration:
		var cfg admissionv1.ValidatingWebhookConfiguration
		if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Name: name}, &cfg); err != nil {
			return []adapter.CheckResult{wc.readErrorResult(ec, ref, err, started)}, nil
		}
		for i := range cfg.Webhooks {
			entries = appendEntry(entries, cfg.Webhooks[i].Name, cfg.Webhooks[i].ClientConfig)
		}
	default:
		// Defensive: NewEngine rejects unknown kinds, so this is unreachable in
		// a validated engine. Surface it as an adapter Error rather than panic.
		return []adapter.CheckResult{result(ec.Family, ref, adapter.OutcomeError,
			fmt.Sprintf("unknown webhook configuration kind %q", wc.Kind), nil, started)}, nil
	}

	if len(entries) == 0 {
		return []adapter.CheckResult{result(ec.Family, ref, adapter.OutcomeFail,
			"webhook configuration has no webhook entries", nil, started)}, nil
	}

	expectedNS := ""
	if wc.ExpectedService != "" {
		expectedNS = firstNamespace(ec.Policy, wc.ServiceNamespace)
	}

	var unpopulated, misdirected []string
	for _, e := range entries {
		if e.service != nil && len(e.caBundle) == 0 {
			unpopulated = append(unpopulated, e.name)
		}
		if wc.ExpectedService != "" &&
			(e.service == nil || e.service.Name != wc.ExpectedService || e.service.Namespace != expectedNS) {
			misdirected = append(misdirected, e.name)
		}
	}

	details := map[string]string{"webhookCount": strconv.Itoa(len(entries))}
	if wc.ExpectedService != "" {
		details["expectedService"] = expectedNS + "/" + wc.ExpectedService
	}

	var problems []string
	if len(unpopulated) > 0 {
		details["unpopulatedCABundle"] = strings.Join(unpopulated, ",")
		problems = append(problems, fmt.Sprintf("%d of %d webhooks have an unpopulated caBundle", len(unpopulated), len(entries)))
	}
	if len(misdirected) > 0 {
		details["misdirectedWebhooks"] = strings.Join(misdirected, ",")
		problems = append(problems, fmt.Sprintf("%d of %d webhooks do not target the expected backing service", len(misdirected), len(entries)))
	}
	if len(problems) > 0 {
		return []adapter.CheckResult{result(ec.Family, ref, adapter.OutcomeFail,
			strings.Join(problems, "; "), details, started)}, nil
	}

	return []adapter.CheckResult{result(ec.Family, ref, adapter.OutcomePass,
		"webhook configuration is wired", details, started)}, nil
}

// readErrorResult scores a failed Get: NotFound is a verdict decided by the
// effective Absence posture and tagged with the absent marker (SKA-526); any
// other error is indeterminate (OutcomeError).
func (wc WebhookCheck) readErrorResult(ec EvalContext, ref adapter.TargetRef, err error, started time.Time) adapter.CheckResult {
	if apierrors.IsNotFound(err) {
		o := absenceOutcome(effectiveAbsence(wc.Absence, ec.DefaultPosture))
		return result(ec.Family, ref, o, "webhook configuration not found", adapter.MarkAbsent(nil), started)
	}
	return result(ec.Family, ref, adapter.OutcomeError,
		fmt.Sprintf("failed to read %s: %v", wc.Kind, err), nil, started)
}
