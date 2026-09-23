/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package addondefinition

import (
	"fmt"
	"reflect"

	api "github.com/skaphos/fathom/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	validation "k8s.io/apimachinery/pkg/util/validation"
)

func validateCheck(c api.DefinitionCheck) error {
	payloads := map[string]any{"Workload": c.Workload, "CRD": c.CRD, "Condition": c.Condition, "Field": c.Field, "Webhook": c.Webhook, "CronJob": c.CronJob, "ConfigMap": c.ConfigMap, "AnnotationStaleness": c.AnnotationStaleness, "PodProjection": c.PodProjection}
	selected, known := payloads[c.Kind]
	if !known || reflect.ValueOf(selected).IsNil() {
		return fmt.Errorf("unknown kind or missing matching payload")
	}
	for k, v := range payloads {
		if k != c.Kind && !reflect.ValueOf(v).IsNil() {
			return fmt.Errorf("exactly one payload required")
		}
	}
	switch c.Kind {
	case "Workload":
		return validateWorkload(c.Workload)
	case "CRD":
		return validateCRD(c.CRD)
	case "Condition":
		return validateCondition(c.Condition)
	case "Field":
		return validateField(c.Field)
	case "Webhook":
		return validateWebhook(c.Webhook)
	case "CronJob":
		return validateCronJob(c.CronJob)
	case "ConfigMap":
		return validateConfigMap(c.ConfigMap)
	case "AnnotationStaleness":
		return validateAnnotation(c.AnnotationStaleness)
	case "PodProjection":
		return validateProjection(c.PodProjection)
	}
	return fmt.Errorf("unknown kind")
}
func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
func requiredName(s api.DefinitionResourceName) error {
	if !resourceName(string(s)) {
		return fmt.Errorf("invalid resource name")
	}
	return nil
}
func labelName(s api.DefinitionIdentifier) error {
	if s != "" && !ident(string(s)) {
		return fmt.Errorf("invalid listName")
	}
	return nil
}
func resourceType(version string, kind api.DefinitionToken, listKind api.DefinitionToken, collection bool) error {
	if !apiVersion(version) || !token(string(kind)) || (collection && !token(string(listKind))) {
		return fmt.Errorf("invalid apiVersion/kind/listKind")
	}
	return nil
}
func validateWorkload(p *api.DefinitionWorkload) error {
	if p.Kind != "Deployment" && p.Kind != "DaemonSet" && p.Kind != "StatefulSet" {
		return fmt.Errorf("unknown workload kind")
	}
	if p.DefaultRestartWarn < 0 {
		return fmt.Errorf("restart threshold must be nonnegative")
	}
	return firstError(target(p.Target, true, "Namespaced"), requiredName(p.DefaultName), thresholds(p.NameThresholdKey, p.RestartWarnThresholdKey), component(p.Component), posture(p.Absence))
}
func validateCRD(p *api.DefinitionCRD) error {
	if len(p.Names) == 0 {
		return fmt.Errorf("CRD names required")
	}
	return firstError(target(p.Target, false, "Cluster"), names(p.Names), versions(p.SupportedVersions, true), posture(p.Absence), outcomes(p.UnsupportedVersionOutcome))
}
func validateCondition(p *api.DefinitionCondition) error {
	named := len(p.Names) > 0
	if named == (p.ListKind != "") {
		return fmt.Errorf("choose names or listKind")
	}
	if (p.VersionCRD != "") != (len(p.SupportedVersions) > 0) {
		return fmt.Errorf("versionCRD and supportedVersions must be paired")
	}
	if p.VersionCRD != "" && !resourceName(string(p.VersionCRD)) {
		return fmt.Errorf("invalid version CRD")
	}
	if p.ConditionType == "" || len(p.ConditionType) > 253 {
		return fmt.Errorf("invalid conditionType")
	}
	if p.ExpectedStatus != "True" && p.ExpectedStatus != "False" && p.ExpectedStatus != "Unknown" {
		return fmt.Errorf("invalid expectedStatus")
	}
	return firstError(target(p.Target, named, ""), resourceType(p.APIVersion, p.Kind, p.ListKind, !named), labelName(p.ListName), names(p.Names), versions(p.SupportedVersions, false), posture(p.Absence), outcomes(p.AbsentCondition, p.Mismatch))
}
func validateField(p *api.DefinitionField) error {
	if len(p.FieldPath) == 0 || len(p.FieldPath) > MaxFieldSegments {
		return fmt.Errorf("fieldPath requires 1–16 literal segments")
	}
	for _, s := range p.FieldPath {
		if s == "" || len(s) > MaxFieldSegmentBytes {
			return fmt.Errorf("invalid fieldPath segment")
		}
	}
	if p.ExpectedValue == "" {
		return fmt.Errorf("expectedValue required")
	}
	if len(p.ValueOutcomes) > 32 {
		return fmt.Errorf("too many value outcomes")
	}
	if _, exists := p.ValueOutcomes[string(p.ExpectedValue)]; exists {
		return fmt.Errorf("unreachable expectedValue override")
	}
	for _, v := range p.ValueOutcomes {
		if v == "" {
			return fmt.Errorf("explicit value outcome cannot be empty")
		}
		if err := outcomes(v); err != nil {
			return err
		}
	}
	return firstError(target(p.Target, false, ""), resourceType(p.APIVersion, p.Kind, p.ListKind, true), labelName(p.ListName), posture(p.Absence), outcomes(p.AbsentOutcome, p.OtherOutcome))
}
func validateWebhook(p *api.DefinitionWebhook) error {
	if p.Kind != "MutatingWebhookConfiguration" && p.Kind != "ValidatingWebhookConfiguration" {
		return fmt.Errorf("invalid webhook kind")
	}
	if (p.ExpectedService != "") != (p.ServiceNamespace != "") {
		return fmt.Errorf("service and namespace must be paired")
	}
	if p.VerifyEndpoints && p.ExpectedService == "" {
		return fmt.Errorf("verifyEndpoints requires service")
	}
	if p.ExpectedService != "" && (!dnsLabel(string(p.ExpectedService)) || !dnsLabel(string(p.ServiceNamespace))) {
		return fmt.Errorf("invalid service reference")
	}
	return firstError(target(p.Target, false, "Cluster"), requiredName(p.Name), thresholds(p.NameThresholdKey), posture(p.Absence))
}
func validateCronJob(p *api.DefinitionCronJob) error {
	return firstError(target(p.Target, true, "Namespaced"), requiredName(p.DefaultName), thresholds(p.NameThresholdKey, p.SuccessMaxAgeThresholdKey), component(p.Component), posture(p.Absence), duration(p.DefaultSuccessMaxAge, false), outcomes(p.StaleOutcome))
}
func validateConfigMap(p *api.DefinitionConfigMap) error {
	if len(validation.IsConfigMapKey(p.Key)) != 0 {
		return fmt.Errorf("invalid ConfigMap key")
	}
	if len(p.RecognizedAPIVersions) > MaxAPIVersions {
		return fmt.Errorf("too many recognized API versions")
	}
	seen := map[string]bool{}
	for _, v := range p.RecognizedAPIVersions {
		if !apiVersion(v) || seen[v] {
			return fmt.Errorf("invalid or duplicate recognized API version")
		}
		seen[v] = true
	}
	return firstError(target(p.Target, true, "Namespaced"), requiredName(p.DefaultName), thresholds(p.NameThresholdKey), component(p.Component), posture(p.Absence), outcomes(p.UnrecognizedOutcome, p.InvalidOutcome))
}
func validateAnnotation(p *api.DefinitionAnnotationStaleness) error {
	collection := p.ListKind != ""
	if collection && (p.DefaultName != "" || p.NameThresholdKey != "") {
		return fmt.Errorf("collection cannot select singleton")
	}
	if !collection && !resourceName(string(p.DefaultName)) {
		return fmt.Errorf("named annotation target required")
	}
	if len(validation.IsQualifiedName(p.AnnotationKey)) != 0 {
		return fmt.Errorf("invalid annotation key")
	}
	if len(p.TimestampJSONField) > MaxFieldSegmentBytes {
		return fmt.Errorf("JSON timestamp key too long")
	}
	return firstError(target(p.Target, !collection, ""), resourceType(p.APIVersion, p.Kind, p.ListKind, collection), labelName(p.ListName), component(p.Component), posture(p.Absence), thresholds(p.NameThresholdKey, p.MaxAgeThresholdKey), duration(p.DefaultMaxAge, true), outcomes(p.StaleOutcome))
}
func validateProjection(p *api.DefinitionPodProjection) error {
	if len(p.Selector) == 0 {
		return fmt.Errorf("projection selector required")
	}
	if !dnsLabel(string(p.VolumeName)) {
		return fmt.Errorf("invalid volume name")
	}
	if p.EnvVar != "" && (len(p.EnvVar) > 253 || !envToken.MatchString(p.EnvVar)) {
		return fmt.Errorf("invalid environment variable name")
	}
	return firstError(target(p.Target, false, "Namespaced"), ValidateSelector(&metav1.LabelSelector{MatchLabels: selectorLabels(p.Selector)}), labelName(p.ListName), component(p.Component), outcomes(p.MissingOutcome))
}

func selectorLabels(in map[string]api.DefinitionSelectorValue) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = string(v)
	}
	return out
}
