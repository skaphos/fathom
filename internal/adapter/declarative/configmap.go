/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/skaphos/fathom/pkg/adapter"
)

// Evaluate implements Evaluator for ConfigMapCheck. It reads one ConfigMap and
// scores the well-formedness of the policy document under Key:
//
//   - ConfigMap NotFound -> Absence posture (with the absent marker).
//   - Key missing from data -> InvalidOutcome (default Fail).
//   - value does not parse as YAML -> InvalidOutcome (default Fail).
//   - RecognizedAPIVersions set and the document's apiVersion is not among them
//     -> UnrecognizedOutcome (default Warn).
//   - otherwise -> Pass.
//
// This catches the silent no-op where an addon keeps running against a policy it
// can never load. It is intentionally shape-level (see the type doc): validating
// individual strategy/plugin names against a specific addon release is
// version-coupled knowledge the generic engine does not carry.
func (c ConfigMapCheck) Evaluate(ec EvalContext) ([]adapter.CheckResult, error) {
	started := time.Now()
	ns := firstNamespace(ec.Policy, c.DefaultNamespace)
	name := c.DefaultName
	if c.NameThresholdKey != "" {
		name = stringThreshold(ec.Policy, c.NameThresholdKey, c.DefaultName)
	}
	invalid := c.InvalidOutcome
	if invalid == "" {
		invalid = adapter.OutcomeFail
	}
	unrecognized := c.UnrecognizedOutcome
	if unrecognized == "" {
		unrecognized = adapter.OutcomeWarn
	}

	target := adapter.TargetRef{APIVersion: "v1", Kind: "ConfigMap", Namespace: ns, Name: name}
	details := map[string]string{"component": c.Component, "key": c.Key}

	var cm corev1.ConfigMap
	if err := ec.Client.Get(ec.Ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			o := absenceOutcome(effectiveAbsence(c.Absence, ec.DefaultPosture))
			return []adapter.CheckResult{result(ec.Family, target, o, "configmap not found", adapter.MarkAbsent(details), started)}, nil
		}
		return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomeError, fmt.Sprintf("failed to read configmap: %v", err), details, started)}, nil
	}

	value, ok := cm.Data[c.Key]
	if !ok {
		return []adapter.CheckResult{result(ec.Family, target, invalid, fmt.Sprintf("configmap has no %q key", c.Key), details, started)}, nil
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(value), &doc); err != nil {
		return []adapter.CheckResult{result(ec.Family, target, invalid, fmt.Sprintf("policy under %q is not valid YAML: %v", c.Key, err), details, started)}, nil
	}

	if len(c.RecognizedAPIVersions) > 0 {
		apiVersion, _ := doc["apiVersion"].(string)
		details["apiVersion"] = apiVersion
		details["recognizedAPIVersions"] = strings.Join(c.RecognizedAPIVersions, ",")
		if !containsString(c.RecognizedAPIVersions, apiVersion) {
			return []adapter.CheckResult{result(ec.Family, target, unrecognized, fmt.Sprintf("policy apiVersion %q is not recognized", apiVersion), details, started)}, nil
		}
	}

	return []adapter.CheckResult{result(ec.Family, target, adapter.OutcomePass, "policy parses and is well-formed", details, started)}, nil
}

// containsString reports whether s is in list.
func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
