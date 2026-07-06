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

// Evaluate implements Evaluator for WebhookCheck. It fetches the named
// admission webhook configuration (a cluster-scoped singleton) and scores its
// wiring:
//
//   - NotFound -> the effective Absence posture (Required, the default -> Fail;
//     Optional -> Skipped), tagged with the adapter.DetailAbsent marker.
//   - Zero webhooks[] entries -> Fail: an entry-less configuration admits
//     nothing, so the addon's admission path is silently inert.
//   - An entry with an empty clientConfig.caBundle -> Fail. This is the
//     real-world failure mode the check exists for: the addon's CA injection
//     (istiod, cert-manager cainjector, ...) has not patched the bundle, so
//     the API server cannot trust the webhook and admission requests fail —
//     or, with failurePolicy Ignore, silently stop being enforced.
//   - When ExpectedService is set, an entry whose clientConfig does not
//     reference ServiceNamespace/ExpectedService (including URL-based
//     clientConfigs) -> Fail: the configuration is not wired to the in-cluster
//     service the addon serves admission on.
//
// Whether the backing service's endpoints are ready is deliberately not
// checked here: the serving workload is the same one a WorkloadCheck in the
// family already scores, so endpoint readiness is covered transitively.
func (wc WebhookCheck) Evaluate(ec EvalContext) ([]adapter.CheckResult, error) {
	started := time.Now()
	ref := adapter.TargetRef{APIVersion: "admissionregistration.k8s.io/v1", Kind: wc.Kind, Name: wc.Name}

	var entries []webhookEntry
	switch wc.Kind {
	case KindMutatingWebhookConfiguration:
		var cfg admissionv1.MutatingWebhookConfiguration
		if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Name: wc.Name}, &cfg); err != nil {
			return []adapter.CheckResult{wc.readErrorResult(ec, ref, err, started)}, nil
		}
		for i := range cfg.Webhooks {
			w := &cfg.Webhooks[i]
			entries = append(entries, webhookEntry{name: w.Name, caBundle: w.ClientConfig.CABundle, service: w.ClientConfig.Service})
		}
	case KindValidatingWebhookConfiguration:
		var cfg admissionv1.ValidatingWebhookConfiguration
		if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Name: wc.Name}, &cfg); err != nil {
			return []adapter.CheckResult{wc.readErrorResult(ec, ref, err, started)}, nil
		}
		for i := range cfg.Webhooks {
			w := &cfg.Webhooks[i]
			entries = append(entries, webhookEntry{name: w.Name, caBundle: w.ClientConfig.CABundle, service: w.ClientConfig.Service})
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

	var unpopulated, misdirected []string
	for _, e := range entries {
		if len(e.caBundle) == 0 {
			unpopulated = append(unpopulated, e.name)
		}
		if wc.ExpectedService != "" &&
			(e.service == nil || e.service.Name != wc.ExpectedService || e.service.Namespace != wc.ServiceNamespace) {
			misdirected = append(misdirected, e.name)
		}
	}

	details := map[string]string{"webhookCount": strconv.Itoa(len(entries))}
	if wc.ExpectedService != "" {
		details["expectedService"] = wc.ServiceNamespace + "/" + wc.ExpectedService
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
