/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

const healthReportNameHashLength = 16

func deterministicHealthReportName(sourceName string, keyParts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(keyParts, "\x00")))
	suffix := hex.EncodeToString(sum[:])[:healthReportNameHashLength]

	const maxNameLength = 253
	maxBaseLength := maxNameLength - len(suffix) - 1
	base := sourceName
	if len(base) > maxBaseLength {
		base = base[:maxBaseLength]
	}
	base = strings.Trim(base, "-.")
	if base == "" {
		base = "healthreport"
	}
	return base + "-" + suffix
}

func useDeterministicHealthReportName(report *fathomv1alpha1.HealthReport, sourceName string, keyParts ...string) {
	report.GenerateName = ""
	report.Name = deterministicHealthReportName(sourceName, keyParts...)
}

func createOrReuseHealthReport(ctx context.Context, c client.Client, report *fathomv1alpha1.HealthReport) (*fathomv1alpha1.HealthReport, bool, error) {
	if err := c.Create(ctx, report); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, false, err
		}
		existing := &fathomv1alpha1.HealthReport{}
		if getErr := c.Get(ctx, types.NamespacedName{Namespace: report.Namespace, Name: report.Name}, existing); getErr != nil {
			return nil, false, getErr
		}
		return existing, false, nil
	}
	return report, true, nil
}
