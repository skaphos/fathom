/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"fmt"
	"strings"
	"time"

	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/skaphos/fathom/internal/adapter/crdutil"
	"github.com/skaphos/fathom/pkg/adapter"
)

// Evaluate implements Evaluator for CRDCheck. It verifies each named CRD is
// established and serves a supported version, emitting one CheckResult per name.
// NotFound is scored by the effective Posture (Required, the default -> Fail;
// Optional -> Skipped) and tagged with the adapter.DetailAbsent marker; an
// established CRD serving no recognized version is scored by
// UnsupportedVersionOutcome (default OutcomeWarn).
func (cc CRDCheck) Evaluate(ec EvalContext) ([]adapter.CheckResult, error) {
	unsupported := cc.UnsupportedVersionOutcome
	if unsupported == "" {
		unsupported = adapter.OutcomeWarn
	}
	out := make([]adapter.CheckResult, 0, len(cc.Names))
	for _, name := range cc.Names {
		started := time.Now()
		ref := adapter.TargetRef{APIVersion: "apiextensions.k8s.io/v1", Kind: "CustomResourceDefinition", Name: name}

		var crd apixv1.CustomResourceDefinition
		if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Name: name}, &crd); err != nil {
			if apierrors.IsNotFound(err) {
				o := absenceOutcome(effectiveAbsence(cc.Absence, ec.DefaultPosture))
				out = append(out, result(ec.Family, ref, o, "CRD not found",
					adapter.MarkAbsent(map[string]string{"crd": name}), started))
				continue
			}
			out = append(out, result(ec.Family, ref, adapter.OutcomeError, fmt.Sprintf("failed to read CRD: %v", err),
				map[string]string{"crd": name}, started))
			continue
		}

		if !crdutil.Established(&crd) {
			out = append(out, result(ec.Family, ref, adapter.OutcomeFail, "CRD not established",
				map[string]string{"crd": name}, started))
			continue
		}

		if v, ok := crdutil.PreferredServedVersion(&crd, cc.SupportedVersions); !ok {
			out = append(out, result(ec.Family, ref, unsupported, "CRD serves no recognized version",
				map[string]string{"crd": name, "expectedVersions": strings.Join(cc.SupportedVersions, ",")}, started))
		} else {
			out = append(out, result(ec.Family, ref, adapter.OutcomePass, "CRD established",
				map[string]string{"crd": name, "version": v}, started))
		}
	}
	return out, nil
}
