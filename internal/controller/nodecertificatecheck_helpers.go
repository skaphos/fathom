/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodecert"
)

// agentResourceName is the shared name of the per-check ServiceAccount,
// RoleBinding, and DaemonSet the operator manages for a NodeCertificateCheck.
func agentResourceName(check *fathomv1alpha1.NodeCertificateCheck) string {
	return check.Name + "-node-agent"
}

// agentLabels are applied to every operator-managed node-agent object so they
// trace back to their check and are discoverable.
func agentLabels(check *fathomv1alpha1.NodeCertificateCheck) map[string]string {
	return map[string]string{
		nodecert.LabelManagedBy:  nodecert.ManagedByValue,
		nodecert.LabelSourceKind: nodecert.KindNodeCertificateCheck,
		nodecert.LabelSourceName: check.Name,
		nodeAgentComponentLabel:  nodeAgentComponentValue,
	}
}

func mergeLabels(existing, desired map[string]string) map[string]string {
	if existing == nil {
		existing = make(map[string]string, len(desired))
	}
	for k, v := range desired {
		existing[k] = v
	}
	return existing
}

func resolveCertPaths(check *fathomv1alpha1.NodeCertificateCheck) []string {
	if len(check.Spec.Paths) > 0 {
		return append([]string(nil), check.Spec.Paths...)
	}
	return nodecert.DefaultCertPaths()
}

// resolveThresholds returns the effective warn/critical day thresholds, applying
// defaults for unset fields and clamping criticalDays to be no larger than
// warnDays so the Fail boundary never sits above the Warn boundary.
func resolveThresholds(check *fathomv1alpha1.NodeCertificateCheck) (warnDays, criticalDays int) {
	warnDays = defaultNodeCertWarnDays
	criticalDays = defaultNodeCertCriticalDays
	if check.Spec.WarnDays != nil {
		warnDays = int(*check.Spec.WarnDays)
	}
	if check.Spec.CriticalDays != nil {
		criticalDays = int(*check.Spec.CriticalDays)
	}
	if criticalDays > warnDays {
		criticalDays = warnDays
	}
	return warnDays, criticalDays
}

// resolveTolerations returns the DaemonSet tolerations. A nil spec value applies
// a default set that tolerates control-plane taints so the agent reaches the
// kubeadm certificates that live on control-plane nodes. An explicit (possibly
// empty) list is used verbatim.
func resolveTolerations(check *fathomv1alpha1.NodeCertificateCheck) []corev1.Toleration {
	if check.Spec.Tolerations != nil {
		return append([]corev1.Toleration(nil), check.Spec.Tolerations...)
	}
	return []corev1.Toleration{
		{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		{Key: "node-role.kubernetes.io/master", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
	}
}

func joinPaths(paths []string) string {
	return strings.Join(paths, ",")
}

func nodeCertInterval(check *fathomv1alpha1.NodeCertificateCheck) time.Duration {
	if check.Spec.Interval != nil && check.Spec.Interval.Duration > 0 {
		return check.Spec.Interval.Duration
	}
	return defaultNodeCertInterval
}

func nodeCertTimeout(check *fathomv1alpha1.NodeCertificateCheck) time.Duration {
	if check.Spec.Timeout != nil && check.Spec.Timeout.Duration > 0 {
		return check.Spec.Timeout.Duration
	}
	return defaultNodeCertTimeout
}

func nodeOutcomeToResult(o nodecert.Outcome) fathomv1alpha1.HealthReportResult {
	switch o {
	case nodecert.OutcomePass:
		return fathomv1alpha1.HealthReportResultPass
	case nodecert.OutcomeWarn:
		return fathomv1alpha1.HealthReportResultWarn
	case nodecert.OutcomeFail:
		return fathomv1alpha1.HealthReportResultFail
	case nodecert.OutcomeError:
		return fathomv1alpha1.HealthReportResultError
	case nodecert.OutcomeSkipped:
		return fathomv1alpha1.HealthReportResultSkipped
	default:
		return fathomv1alpha1.HealthReportResultUnknown
	}
}

// aggregateNodeReports returns the worst-case HealthReportResult across every
// certificate of every reporting node. No certificates at all (every node
// scanned nothing) yields Skipped.
func aggregateNodeReports(reports []nodecert.NodeReport) fathomv1alpha1.HealthReportResult {
	worst := fathomv1alpha1.HealthReportResultPass
	worstRank := worst.Severity()
	any := false
	for _, rep := range reports {
		for _, c := range rep.Certs {
			any = true
			res := nodeOutcomeToResult(c.Outcome)
			if rank := res.Severity(); rank > worstRank {
				worst = res
				worstRank = rank
			}
		}
	}
	if !any {
		return fathomv1alpha1.HealthReportResultSkipped
	}
	return worst
}

// healthReportForNodeCert builds the rolled-up HealthReport: one check per
// (node, certificate), aggregated into a single worst-case Result.
func healthReportForNodeCert(check *fathomv1alpha1.NodeCertificateCheck, reports []nodecert.NodeReport, aggregate fathomv1alpha1.HealthReportResult, observedAt metav1.Time) *fathomv1alpha1.HealthReport {
	var checks []fathomv1alpha1.HealthReportCheck
	for _, rep := range reports {
		certObservedAt := observedAt
		if !rep.ObservedAt.IsZero() {
			certObservedAt = metav1.NewTime(rep.ObservedAt)
		}
		for _, c := range rep.Certs {
			details := map[string]string{
				"node":          rep.Node,
				"path":          c.Path,
				"daysRemaining": strconv.Itoa(c.DaysRemaining),
			}
			putIfNotEmpty(details, "source", c.Source)
			putIfNotEmpty(details, "subject", c.Subject)
			putIfNotEmpty(details, "issuer", c.Issuer)
			putIfNotEmpty(details, "serial", c.Serial)
			if !c.NotAfter.IsZero() {
				details["notAfter"] = c.NotAfter.UTC().Format(time.RFC3339)
			}
			if len(c.SANs) > 0 {
				details["sans"] = strings.Join(c.SANs, ",")
			}
			checks = append(checks, fathomv1alpha1.HealthReportCheck{
				Family:     nodeCertReportFamily,
				Result:     nodeOutcomeToResult(c.Outcome),
				TargetRef:  fathomv1alpha1.HealthReportTargetRef{APIVersion: "v1", Kind: "Node", Name: rep.Node},
				Summary:    c.Summary,
				Details:    details,
				ObservedAt: certObservedAt,
			})
		}
	}

	return &fathomv1alpha1.HealthReport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    check.Namespace,
			GenerateName: check.Name + "-",
			Labels: map[string]string{
				labelHealthReportSourceKind: "NodeCertificateCheck",
				labelHealthReportSourceName: check.Name,
			},
		},
		Spec: fathomv1alpha1.HealthReportSpec{
			SourceRef: fathomv1alpha1.HealthReportTargetRef{
				APIVersion: fathomv1alpha1.GroupVersion.String(),
				Kind:       "NodeCertificateCheck",
				Namespace:  check.Namespace,
				Name:       check.Name,
			},
			AdapterName:    nodeCertReportAdapterName,
			AdapterVersion: nodeCertReportAdapterVer,
			Result:         aggregate,
			Checks:         checks,
			ObservedAt:     observedAt,
		},
	}
}

func putIfNotEmpty(m map[string]string, key, value string) {
	if value != "" {
		m[key] = value
	}
}

// pruneNodeCertHealthReports enforces Spec.HistoryLimit by deleting the oldest
// HealthReports for this check beyond the cap. Failures are logged, not
// returned: the new HealthReport already landed and the next reconcile retries.
func pruneNodeCertHealthReports(ctx context.Context, c client.Client, log logr.Logger, check *fathomv1alpha1.NodeCertificateCheck) {
	limit := defaultHealthReportHistoryLimit
	if check.Spec.HistoryLimit != nil {
		limit = int(*check.Spec.HistoryLimit)
	}
	if limit < 1 {
		return
	}

	var reports fathomv1alpha1.HealthReportList
	if err := c.List(ctx, &reports,
		client.InNamespace(check.Namespace),
		client.MatchingLabels{
			labelHealthReportSourceKind: "NodeCertificateCheck",
			labelHealthReportSourceName: check.Name,
		},
	); err != nil {
		log.Error(err, "list HealthReports for retention pruning failed; will retry on next reconcile")
		return
	}
	if len(reports.Items) <= limit {
		return
	}

	sort.Slice(reports.Items, func(i, j int) bool {
		return reports.Items[i].CreationTimestamp.Before(&reports.Items[j].CreationTimestamp)
	})
	excess := len(reports.Items) - limit
	for i := 0; i < excess; i++ {
		victim := &reports.Items[i]
		if err := c.Delete(ctx, victim); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "delete old HealthReport failed", "name", victim.Name)
		}
	}
	log.V(1).Info("pruned NodeCertificateCheck HealthReport history", "deleted", excess, "limit", limit)
}
