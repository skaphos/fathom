/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"encoding/json"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/pkg/adapter"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
)

// Evaluate implements Evaluator for AnnotationStalenessCheck. It reads an
// object's metadata annotation and scores the age of the timestamp it carries.
//
// Named mode (ListKind empty) Gets one object; a NotFound object is scored by
// Absence, an absent annotation is Pass (nothing is held), and a timestamp older
// than the window is StaleOutcome. List mode (ListKind set) lists the kind and
// scores only objects that carry the annotation — objects without it are not
// emitted, so a quiescent cluster stays quiet, and an empty match set is Skipped.
func (a AnnotationStalenessCheck) Evaluate(ec EvalContext) ([]adapter.CheckResult, error) {
	gv, err := schema.ParseGroupVersion(a.APIVersion)
	if err != nil {
		ref := adapter.TargetRef{APIVersion: a.APIVersion, Kind: a.Kind, Name: a.listName()}
		return []adapter.CheckResult{result(ec.Family, ref, adapter.OutcomeError,
			fmt.Sprintf("invalid apiVersion %q: %v", a.APIVersion, err), a.baseDetails(), time.Now())}, nil
	}
	if a.ListKind != "" {
		return a.evaluateList(ec, gv), nil
	}
	return a.evaluateNamed(ec, gv), nil
}

func (a AnnotationStalenessCheck) evaluateNamed(ec EvalContext, gv schema.GroupVersion) []adapter.CheckResult {
	started := time.Now()
	ns := ""
	if !a.ClusterScoped {
		ns = firstNamespace(ec.Policy, a.DefaultNamespace)
	}
	name := a.DefaultName
	if a.NameThresholdKey != "" {
		name = stringThreshold(ec.Policy, a.NameThresholdKey, a.DefaultName)
	}
	ref := adapter.TargetRef{APIVersion: a.APIVersion, Kind: a.Kind, Namespace: ns, Name: name}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gv.WithKind(a.Kind))
	if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Namespace: ns, Name: name}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			o := absenceOutcome(effectiveAbsence(a.Absence, ec.DefaultPosture))
			return []adapter.CheckResult{result(ec.Family, ref, o,
				fmt.Sprintf("%s %s not found", a.Kind, name), adapter.MarkAbsent(a.baseDetails()), started)}
		}
		return []adapter.CheckResult{result(ec.Family, ref, adapter.OutcomeError,
			fmt.Sprintf("failed to read %s %s: %v", a.Kind, name, err), a.baseDetails(), started)}
	}

	value, present := obj.GetAnnotations()[a.AnnotationKey]
	if !present {
		details := a.baseDetails()
		details["held"] = "false"
		return []adapter.CheckResult{result(ec.Family, ref, adapter.OutcomePass,
			fmt.Sprintf("%s annotation is absent", a.AnnotationKey), details, started)}
	}
	return []adapter.CheckResult{a.scoreAnnotation(ec, ref, value, started)}
}

func (a AnnotationStalenessCheck) evaluateList(ec EvalContext, gv schema.GroupVersion) []adapter.CheckResult {
	listRef := adapter.TargetRef{APIVersion: a.APIVersion, Kind: a.Kind, Name: a.listName()}

	var out []adapter.CheckResult
	namespaces := policyNamespaces(ec.Policy, a.DefaultNamespace)
	if a.ClusterScoped {
		namespaces = []string{""}
	}
	for _, ns := range namespaces {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gv.WithKind(a.ListKind))
		before := len(out)
		err := ec.walkPages(list, func(raw client.ObjectList) error {
			page := raw.(*unstructured.UnstructuredList)
			for i := range page.Items {
				obj := &page.Items[i]
				value, present := obj.GetAnnotations()[a.AnnotationKey]
				if !present {
					continue
				}
				ref := adapter.TargetRef{APIVersion: a.APIVersion, Kind: a.Kind, Namespace: obj.GetNamespace(), Name: obj.GetName()}
				if err := ec.appendResults(&out, a.scoreAnnotation(ec, ref, value, time.Now())); err != nil {
					return err
				}
			}
			return nil
		}, func() { out = out[:before] }, client.InNamespace(ns))
		if err != nil {
			return []adapter.CheckResult{result(ec.Family, listRef, adapter.OutcomeError, fmt.Sprintf("failed to list %s: %v", a.Kind, err), a.baseDetails(), time.Now())}
		}
	}

	if len(out) == 0 {
		// Carry the same annotation/component details the scored and error results
		// include, so a NoMatchingObjects skip stays disambiguated when a family
		// has more than one AnnotationStalenessCheck over the same kind.
		skip := skippedResult(ec.Family, listRef,
			fmt.Sprintf("no %s objects carry annotation %s", a.Kind, a.AnnotationKey), "NoMatchingObjects")
		for k, v := range a.baseDetails() {
			skip.Details[k] = v
		}
		return []adapter.CheckResult{skip}
	}
	return out
}

// scoreAnnotation parses the timestamp out of a present annotation value and
// scores its age against the resolved window. An unparseable timestamp is a
// Warn (the annotation is held but its age cannot be determined) rather than an
// Error — the object is reachable, only its payload is unexpected.
func (a AnnotationStalenessCheck) scoreAnnotation(ec EvalContext, ref adapter.TargetRef, value string, started time.Time) adapter.CheckResult {
	if ec.runtime && len(value) > limits.MaxAnnotationBytes {
		err := ec.inputFailure("annotation value exceeds byte limit")
		return result(ec.Family, ref, adapter.OutcomeError, err.Error(), a.baseDetails(), started)
	}

	stale := a.StaleOutcome
	if stale == "" {
		stale = adapter.OutcomeWarn
	}
	maxAge := a.DefaultMaxAge
	if a.MaxAgeThresholdKey != "" {
		maxAge = durationThreshold(ec.Policy, a.MaxAgeThresholdKey, a.DefaultMaxAge)
	}
	details := a.baseDetails()
	details["held"] = "true"

	ts, err := a.parseTimestamp(value)
	if err != nil {
		return result(ec.Family, ref, adapter.OutcomeWarn,
			fmt.Sprintf("%s annotation is present but its timestamp is unreadable: %v", a.AnnotationKey, err), details, started)
	}

	details["timestamp"] = ts.UTC().Format(time.RFC3339)
	details["maxAge"] = maxAge.String()
	if isFutureTimestamp(ts) {
		return result(ec.Family, ref, stale,
			fmt.Sprintf("%s annotation timestamp is in the future (clock skew or malformed payload)", a.AnnotationKey), details, started)
	}
	if time.Since(ts) > maxAge {
		return result(ec.Family, ref, stale,
			fmt.Sprintf("%s annotation is older than the freshness window", a.AnnotationKey), details, started)
	}
	return result(ec.Family, ref, adapter.OutcomePass,
		fmt.Sprintf("%s annotation is within the freshness window", a.AnnotationKey), details, started)
}

// parseTimestamp extracts the RFC3339 timestamp from the annotation value:
// directly, or — when TimestampJSONField is set — from that field of a JSON
// object (the shape kured's lock annotation uses).
func (a AnnotationStalenessCheck) parseTimestamp(value string) (time.Time, error) {
	raw := value
	if a.TimestampJSONField != "" {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(value), &payload); err != nil {
			return time.Time{}, fmt.Errorf("annotation is not a JSON object: %w", err)
		}
		field, ok := payload[a.TimestampJSONField].(string)
		if !ok {
			return time.Time{}, fmt.Errorf("JSON field %q is missing or not a string", a.TimestampJSONField)
		}
		raw = field
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("not an RFC3339 timestamp: %w", err)
	}
	return ts, nil
}

func (a AnnotationStalenessCheck) baseDetails() map[string]string {
	details := map[string]string{"annotation": a.AnnotationKey}
	if a.Component != "" {
		details["component"] = a.Component
	}
	return details
}

// listName is the stable placeholder Name for list-level results, defaulting to
// Kind when ListName is unset.
func (a AnnotationStalenessCheck) listName() string {
	if a.ListName != "" {
		return a.ListName
	}
	return a.Kind
}
