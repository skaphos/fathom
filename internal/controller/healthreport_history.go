/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"sort"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1 "github.com/skaphos/fathom/api/v1"
)

// historyLimit resolves a check's spec.historyLimit, falling back to the CRD
// default when the field is unset.
func historyLimit(limit *int32) int {
	if limit == nil {
		return defaultHealthReportHistoryLimit
	}
	return int(*limit)
}

// reportSelector is the label set a check's own reports carry: kind, name
// and the check's UID, so a recreated check of the same name never selects
// its predecessor's retained history (spec 014, FR-013a). The UID is omitted
// only for an object the API server never assigned one — unit-test fixtures;
// every persisted check has a UID.
func reportSelector(kind string, check client.Object) map[string]string {
	labels := map[string]string{
		fathomv1.LabelHealthReportSourceKind: kind,
		fathomv1.LabelHealthReportSourceName: check.GetName(),
	}
	if uid := string(check.GetUID()); uid != "" {
		labels[fathomv1.LabelHealthReportSourceUID] = uid
	}
	return labels
}

// pruneHealthReports enforces one check's history limit over the HealthReports
// selected by labels in namespace: the oldest beyond limit are deleted, the
// report named protectedName is never a victim, and ties are broken by name
// so retention does not depend on API list order. It is shared by every
// report-writing reconciler.
//
// "Oldest" orders by creation time (the API server's clock, which does not
// step back across an operator failover the way spec.observedAt can), then
// spec.observedAt, then name; protectedName is the report the caller's
// status is about to name, so no clock anomaly can strand it. A limit
// below 1 is a no-op: CRD validation rejects it, but a Minimum=1 would prune
// the just-created report and strand status.lastReportName. Delete errors are
// logged, not returned — one transient failure must not block pruning the
// rest, and the next reconcile retries. The number of reports deleted is
// returned for callers that account for it.
func pruneHealthReports(
	ctx context.Context,
	c client.Client,
	log logr.Logger,
	namespace string,
	labels map[string]string,
	limit int,
	protectedName string,
) int {
	if limit < 1 {
		return 0
	}

	var reports fathomv1.HealthReportList
	if err := c.List(ctx, &reports, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		log.Error(err, "list HealthReports for retention pruning failed; will retry on next reconcile")
		return 0
	}
	if len(reports.Items) <= limit {
		return 0
	}

	sort.Slice(reports.Items, func(i, j int) bool {
		return olderHealthReport(&reports.Items[i], &reports.Items[j], protectedName)
	})
	excess := len(reports.Items) - limit
	deleted := 0
	for i := range excess {
		victim := &reports.Items[i]
		if err := c.Delete(ctx, victim); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "delete old HealthReport failed", "name", victim.Name)
			continue
		}
		deleted++
	}
	log.V(1).Info("pruned HealthReport history", "deleted", deleted, "limit", limit)
	return deleted
}

// olderHealthReport reports whether left should be pruned before right. The
// protected report always sorts last.
func olderHealthReport(left, right *fathomv1.HealthReport, protectedName string) bool {
	if left.Name == protectedName {
		return false
	}
	if right.Name == protectedName {
		return true
	}
	if !left.CreationTimestamp.Equal(&right.CreationTimestamp) {
		return left.CreationTimestamp.Before(&right.CreationTimestamp)
	}
	if !left.Spec.ObservedAt.Equal(&right.Spec.ObservedAt) {
		return left.Spec.ObservedAt.Before(&right.Spec.ObservedAt)
	}
	return left.Name < right.Name
}
