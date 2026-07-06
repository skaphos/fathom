/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"fmt"
	"time"

	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/internal/adapter/crdutil"
	"github.com/skaphos/fathom/pkg/adapter"
)

// Evaluate implements Evaluator for ConditionCheck. It scores a
// status.conditions[ConditionType]==ExpectedStatus predicate per object — over
// a list of managed CRs across the resolved namespaces, or, when Names is set,
// over each named singleton fetched individually.
//
// List mode: an empty result set across all namespaces -> OutcomeSkipped with
// Details["skipReason"]="NoMatchingObjects". Named mode: a NotFound name ->
// the effective Absence posture (Required default -> Fail) with the absent
// marker. Either mode: a missing condition -> AbsentCondition (default Fail);
// a status mismatch -> Mismatch (default Fail). Transport and invalid-selector
// errors -> OutcomeError.
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

	kindRef := adapter.TargetRef{APIVersion: cc.APIVersion, Kind: cc.Kind, Name: cc.listName()}

	// Named mode branches before the selector parse: policy.LabelSelector does
	// not apply to named gets, so a malformed selector must not error a check
	// that never uses it (mirroring WorkloadCheck, which ignores it too).
	if len(cc.Names) > 0 {
		gv, err := schema.ParseGroupVersion(cc.APIVersion)
		if err != nil {
			return []adapter.CheckResult{result(ec.Family, kindRef, adapter.OutcomeError,
				fmt.Sprintf("invalid apiVersion %q: %v", cc.APIVersion, err), nil, started)}, nil
		}
		return cc.evaluateNamed(ec,
			schema.GroupVersionKind{Group: gv.Group, Version: cc.resolveVersion(ec, gv.Version), Kind: cc.Kind},
			absent, mismatch), nil
	}

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
	// Resolve the served version so the list succeeds on clusters that serve only
	// a legacy version (e.g. external-secrets.io/v1beta1); falls back to gv.Version.
	listGVK := schema.GroupVersionKind{Group: gv.Group, Version: cc.resolveVersion(ec, gv.Version), Kind: cc.ListKind}

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
		out = append(out, cc.scoreObject(ec, obj, absent, mismatch, time.Now()))
	}
	return out, nil
}

// evaluateNamed is the named (get-by-name) mode: each entry in Names is
// fetched and scored individually. A NotFound name is a verdict — the
// effective Absence posture (Required, the default -> Fail; Optional ->
// Skipped) tagged with the adapter.DetailAbsent marker (SKA-526) — never the
// list mode's NoMatchingObjects skip, because a named singleton's existence is
// itself the check (e.g. an aggregated APIService). policy.LabelSelector is
// intentionally not applied to named gets.
func (cc ConditionCheck) evaluateNamed(ec EvalContext, gvk schema.GroupVersionKind, absent, mismatch adapter.Outcome) []adapter.CheckResult {
	namespace := ""
	if !cc.ClusterScoped {
		namespace = firstNamespace(ec.Policy, cc.DefaultNamespace)
	}
	out := make([]adapter.CheckResult, 0, len(cc.Names))
	for _, name := range cc.Names {
		started := time.Now()
		ref := adapter.TargetRef{APIVersion: cc.APIVersion, Kind: cc.Kind, Namespace: namespace, Name: name}

		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(gvk)
		if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
			if apierrors.IsNotFound(err) {
				o := absenceOutcome(effectiveAbsence(cc.Absence, ec.DefaultPosture))
				out = append(out, result(ec.Family, ref, o,
					fmt.Sprintf("%s %s not found", cc.Kind, name),
					adapter.MarkAbsent(map[string]string{"conditionType": cc.ConditionType}), started))
				continue
			}
			out = append(out, result(ec.Family, ref, adapter.OutcomeError,
				fmt.Sprintf("failed to read %s %s: %v", cc.Kind, name, err), nil, started))
			continue
		}
		out = append(out, cc.scoreObject(ec, obj, absent, mismatch, started))
	}
	return out
}

// resolveVersion returns the CR version to list. When VersionCRD is set it uses
// the CRD's preferred served version among SupportedVersions -- matching the
// crd-established check -- and falls back to the given default when the CRD is
// absent, unreadable, or serves no supported version. This preserves the
// hand-written adapter's behavior of listing whichever version the cluster
// actually serves.
func (cc ConditionCheck) resolveVersion(ec EvalContext, fallback string) string {
	if cc.VersionCRD == "" {
		return fallback
	}
	var crd apixv1.CustomResourceDefinition
	if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Name: cc.VersionCRD}, &crd); err != nil {
		return fallback
	}
	if v, ok := crdutil.PreferredServedVersion(&crd, cc.SupportedVersions); ok {
		return v
	}
	return fallback
}

// listName is the stable placeholder Name for list-level results, defaulting to
// the singular Kind when ListName is unset.
func (cc ConditionCheck) listName() string {
	if cc.ListName != "" {
		return cc.ListName
	}
	return cc.Kind
}

// scoreObject evaluates the configured condition on one object. started marks
// the beginning of the work this result's Duration should cover: a fresh
// timestamp in list mode (the fetch is already accounted for once, across the
// whole list), or the per-name Get's start in named mode, so the Duration
// reflects the full get+evaluate work for that object.
func (cc ConditionCheck) scoreObject(ec EvalContext, obj *unstructured.Unstructured, absent, mismatch adapter.Outcome, started time.Time) adapter.CheckResult {
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
