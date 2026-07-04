/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/pkg/adapter"
)

// Evaluate implements Evaluator for ConditionCheck. It lists managed CRs (or
// APIServices) across the resolved namespaces and scores a
// status.conditions[ConditionType]==ExpectedStatus predicate per object.
//
// An empty result set across all namespaces -> OutcomeSkipped with
// Details["skipReason"]="NoMatchingObjects". A missing condition ->
// AbsentCondition (default Fail); a status mismatch -> Mismatch (default Fail).
// Transport and invalid-selector errors -> OutcomeError.
func (cc ConditionCheck) Evaluate(ec EvalContext) ([]adapter.CheckResult, error) {
	started := time.Now()
	absent := cc.AbsentCondition
	if absent == "" {
		absent = adapter.OutcomeFail
	}
	mismatch := cc.Mismatch
	if mismatch == "" {
		mismatch = adapter.OutcomeFail
	}

	kindRef := adapter.TargetRef{APIVersion: cc.APIVersion, Kind: cc.Kind}

	sel, err := metav1.LabelSelectorAsSelector(ec.Policy.LabelSelector)
	if err != nil {
		return []adapter.CheckResult{result(ec.Family, kindRef, adapter.OutcomeError,
			fmt.Sprintf("invalid label selector: %v", err), nil, started)}, nil
	}

	gv, err := schema.ParseGroupVersion(cc.APIVersion)
	if err != nil {
		return []adapter.CheckResult{result(ec.Family, kindRef, adapter.OutcomeError,
			fmt.Sprintf("invalid apiVersion %q: %v", cc.APIVersion, err), nil, started)}, nil
	}
	listGVK := schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: cc.ListKind}

	var items []unstructured.Unstructured
	if cc.ClusterScoped {
		var list unstructured.UnstructuredList
		list.SetGroupVersionKind(listGVK)
		if err := ec.Client.List(ec.Ctx, &list, client.MatchingLabelsSelector{Selector: sel}); err != nil {
			return []adapter.CheckResult{result(ec.Family, kindRef, adapter.OutcomeError,
				fmt.Sprintf("failed to list %s: %v", cc.Kind, err), nil, started)}, nil
		}
		items = append(items, list.Items...)
	} else {
		for _, ns := range policyNamespaces(ec.Policy, cc.DefaultNamespace) {
			var list unstructured.UnstructuredList
			list.SetGroupVersionKind(listGVK)
			if err := ec.Client.List(ec.Ctx, &list, client.InNamespace(ns), client.MatchingLabelsSelector{Selector: sel}); err != nil {
				return []adapter.CheckResult{result(ec.Family, kindRef, adapter.OutcomeError,
					fmt.Sprintf("failed to list %s in namespace %q: %v", cc.Kind, ns, err), nil, started)}, nil
			}
			items = append(items, list.Items...)
		}
	}

	if len(items) == 0 {
		return []adapter.CheckResult{skippedResult(ec.Family, kindRef,
			fmt.Sprintf("no %s objects matched", cc.Kind), "NoMatchingObjects")}, nil
	}

	out := make([]adapter.CheckResult, 0, len(items))
	for i := range items {
		obj := &items[i]
		out = append(out, cc.scoreObject(ec, obj, absent, mismatch))
	}
	return out, nil
}

// scoreObject evaluates the configured condition on one listed object.
func (cc ConditionCheck) scoreObject(ec EvalContext, obj *unstructured.Unstructured, absent, mismatch adapter.Outcome) adapter.CheckResult {
	started := time.Now()
	ref := adapter.TargetRef{
		APIVersion: cc.APIVersion,
		Kind:       cc.Kind,
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
	}
	details := map[string]string{"conditionType": cc.ConditionType}

	status, found := conditionStatus(obj, cc.ConditionType)
	if !found {
		return result(ec.Family, ref, absent,
			fmt.Sprintf("%s condition %s is absent", cc.Kind, cc.ConditionType), details, started)
	}
	details["status"] = status
	if status != cc.ExpectedStatus {
		details["expectedStatus"] = cc.ExpectedStatus
		return result(ec.Family, ref, mismatch,
			fmt.Sprintf("%s condition %s is %q, want %q", cc.Kind, cc.ConditionType, status, cc.ExpectedStatus), details, started)
	}
	return result(ec.Family, ref, adapter.OutcomePass,
		fmt.Sprintf("%s condition %s is %s", cc.Kind, cc.ConditionType, cc.ExpectedStatus), details, started)
}

// conditionStatus reads status.conditions[type==condType].status from an
// unstructured object. The second return is false when the condition is absent.
func conditionStatus(obj *unstructured.Unstructured, condType string) (string, bool) {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return "", false
	}
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		ctype, _, _ := unstructured.NestedString(cond, "type")
		if ctype != condType {
			continue
		}
		status, _, _ := unstructured.NestedString(cond, "status")
		return status, true
	}
	return "", false
}
