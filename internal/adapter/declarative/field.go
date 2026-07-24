/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/pkg/adapter"
)

// Evaluate implements Evaluator for FieldCheck. It lists the kind across the
// resolved namespaces and scores each object's FieldPath value:
//
//	value == ExpectedValue     -> Pass
//	value in ValueOutcomes     -> that outcome
//	any other value            -> OtherOutcome (default Warn)
//	field missing/empty        -> AbsentOutcome (default Warn)
//
// An empty result set across all namespaces -> OutcomeSkipped with
// Details["skipReason"]="NoMatchingObjects". An uninstalled resource API is
// scored by the effective Absence posture with the absent marker. Transport
// and invalid-selector errors -> OutcomeError.
func (fc FieldCheck) Evaluate(ec EvalContext) ([]adapter.CheckResult, error) {
	started := time.Now()
	kindRef := adapter.TargetRef{APIVersion: fc.APIVersion, Kind: fc.Kind, Name: fc.listName()}

	sel, err := policySelector(ec.Policy.LabelSelector)
	if err != nil {
		return []adapter.CheckResult{result(ec.Family, kindRef, adapter.OutcomeError,
			fmt.Sprintf("invalid label selector: %v", err), fc.listDetails(), started)}, nil
	}
	gv, err := schema.ParseGroupVersion(fc.APIVersion)
	if err != nil {
		return []adapter.CheckResult{result(ec.Family, kindRef, adapter.OutcomeError,
			fmt.Sprintf("invalid apiVersion %q: %v", fc.APIVersion, err), fc.listDetails(), started)}, nil
	}
	listGVK := schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: fc.ListKind}

	var items []unstructured.Unstructured
	if fc.ClusterScoped {
		var list unstructured.UnstructuredList
		list.SetGroupVersionKind(listGVK)
		if err := ec.Client.List(ec.Ctx, &list, client.MatchingLabelsSelector{Selector: sel}); err != nil {
			if resourceAbsent(err) {
				return []adapter.CheckResult{fc.absentListResult(ec, kindRef, started)}, nil
			}
			return []adapter.CheckResult{result(ec.Family, kindRef, adapter.OutcomeError,
				fmt.Sprintf("failed to list %s: %v", fc.Kind, err), fc.listDetails(), started)}, nil
		}
		items = append(items, list.Items...)
	} else {
		for _, ns := range policyNamespaces(ec.Policy, "") {
			var list unstructured.UnstructuredList
			list.SetGroupVersionKind(listGVK)
			if err := ec.Client.List(ec.Ctx, &list, client.InNamespace(ns), client.MatchingLabelsSelector{Selector: sel}); err != nil {
				if resourceAbsent(err) {
					return []adapter.CheckResult{fc.absentListResult(ec, kindRef, started)}, nil
				}
				return []adapter.CheckResult{result(ec.Family, kindRef, adapter.OutcomeError,
					fmt.Sprintf("failed to list %s in %s: %v", fc.Kind, namespaceScope(ns), err), fc.listDetails(), started)}, nil
			}
			items = append(items, list.Items...)
		}
	}

	if len(items) == 0 {
		c := skippedResult(ec.Family, kindRef,
			fmt.Sprintf("no %s objects matched", fc.Kind), "NoMatchingObjects")
		c.Details["field"] = fc.fieldPath()
		return []adapter.CheckResult{c}, nil
	}

	out := make([]adapter.CheckResult, 0, len(items))
	for i := range items {
		out = append(out, fc.scoreObject(ec, &items[i], time.Now()))
	}
	return out, nil
}

// scoreObject evaluates the configured field on one object. started marks the
// beginning of the work this result's Duration should cover (a fresh timestamp
// in list mode — the fetch is accounted for once across the whole list).
func (fc FieldCheck) scoreObject(ec EvalContext, obj *unstructured.Unstructured, started time.Time) adapter.CheckResult {
	ref := adapter.TargetRef{
		APIVersion: fc.APIVersion,
		Kind:       fc.Kind,
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
	}
	details := map[string]string{"field": fc.fieldPath()}

	// A non-string value is treated like an absent one: the check contract is a
	// scalar string field, and anything else means the object has not been
	// reconciled into the expected shape.
	value, found, err := unstructured.NestedString(obj.Object, fc.FieldPath...)
	if err != nil || !found || value == "" {
		return result(ec.Family, ref, defaultOutcome(fc.AbsentOutcome, adapter.OutcomeWarn),
			fmt.Sprintf("%s %s is not set", fc.Kind, fc.fieldPath()), details, started)
	}
	details["value"] = value
	if value == fc.ExpectedValue {
		return result(ec.Family, ref, adapter.OutcomePass,
			fmt.Sprintf("%s %s is %s", fc.Kind, fc.fieldPath(), value), details, started)
	}
	details["expectedValue"] = fc.ExpectedValue
	outcome, ok := fc.ValueOutcomes[value]
	if !ok {
		outcome = defaultOutcome(fc.OtherOutcome, adapter.OutcomeWarn)
	}
	return result(ec.Family, ref, outcome,
		fmt.Sprintf("%s %s is %q, want %q", fc.Kind, fc.fieldPath(), value, fc.ExpectedValue), details, started)
}

// absentListResult scores an uninstalled resource API by the effective Absence
// posture, tagged with the absent marker (mirroring ConditionCheck).
func (fc FieldCheck) absentListResult(ec EvalContext, ref adapter.TargetRef, started time.Time) adapter.CheckResult {
	details := adapter.MarkAbsent(fc.listDetails())
	return result(ec.Family, ref, absenceOutcome(effectiveAbsence(fc.Absence, ec.DefaultPosture)),
		fmt.Sprintf("%s API is not installed", fc.Kind), details, started)
}

// fieldPath is the dotted rendering of FieldPath used in summaries and the
// Details["field"] entry (e.g. "status.sync.status").
func (fc FieldCheck) fieldPath() string { return strings.Join(fc.FieldPath, ".") }

// listName is the stable placeholder Name for list-level results, defaulting to
// the singular Kind when ListName is unset.
func (fc FieldCheck) listName() string {
	if fc.ListName != "" {
		return fc.ListName
	}
	return fc.Kind
}

// listDetails returns the Details carried by list-level results: the scored
// field path, so two FieldChecks over the same kind stay distinguishable.
func (fc FieldCheck) listDetails() map[string]string {
	return map[string]string{"field": fc.fieldPath()}
}

// defaultOutcome returns o, or dflt when o is unset.
func defaultOutcome(o, dflt adapter.Outcome) adapter.Outcome {
	if o == "" {
		return dflt
	}
	return o
}
