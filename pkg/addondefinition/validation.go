/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package addondefinition

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Masterminds/semver/v3"
	api "github.com/skaphos/fathom/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	validation "k8s.io/apimachinery/pkg/util/validation"
)

var (
	identifier    = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	thresholdKey  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)
	kindToken     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)
	versionToken  = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	resourceToken = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	envToken      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Validate checks stored and offline definitions independently of admission.
// It performs no cluster reads and never interprets requestedReads as authority.
func Validate(d *api.AddonDefinition) error {
	if d == nil {
		return fmt.Errorf("InvalidDefinition: definition is nil")
	}
	if err := boundedSpec(d.Spec, MaxSpecBytes); err != nil {
		return err
	}
	s := d.Spec
	if !dnsLabel(string(s.AddonType)) || d.Name != string(s.AddonType) {
		return fmt.Errorf("InvalidDefinition: name must equal canonical addonType")
	}
	if len(s.AdapterVersion) > MaxAdapterVersionBytes {
		return fmt.Errorf("InvalidDefinition: adapterVersion too long")
	}
	if _, err := semver.StrictNewVersion(s.AdapterVersion); err != nil {
		return fmt.Errorf("InvalidDefinition: adapterVersion: %w", err)
	}
	if s.SemanticsVersion != 1 {
		return fmt.Errorf("InvalidDefinition: unsupported semanticsVersion %d", s.SemanticsVersion)
	}
	if err := ValidateVersionRange(s.SupportedVersions); err != nil {
		return err
	}
	if s.SupportedVersions != "" && s.VersionSource == nil {
		return fmt.Errorf("InvalidDefinition: supportedVersions requires versionSource")
	}
	if len(s.Families) == 0 || len(s.Families) > MaxFamilies {
		return fmt.Errorf("InvalidDefinition: families must contain 1–16 entries")
	}
	families := map[string]bool{}
	total := 0
	sourceMatches := 0
	for _, f := range s.Families {
		name := string(f.Name)
		if !ident(name) || families[name] {
			return fmt.Errorf("InvalidDefinition: invalid or duplicate family %q", name)
		}
		families[name] = true
		if len(f.Checks) == 0 || len(f.Checks) > MaxChecksPerFamily {
			return fmt.Errorf("InvalidDefinition: family %q requires 1–32 checks", name)
		}
		total += len(f.Checks)
		checks := map[string]bool{}
		components := map[string]bool{}
		for _, c := range f.Checks {
			cn := string(c.Name)
			if !ident(cn) || checks[cn] {
				return fmt.Errorf("InvalidDefinition: invalid or duplicate check %q", cn)
			}
			checks[cn] = true
			if err := validateCheck(c); err != nil {
				return fmt.Errorf("InvalidDefinition: %s/%s: %w", name, cn, err)
			}
			if c.Workload != nil {
				component := string(c.Workload.Component)
				if component == "" {
					component = cn
				}
				if components[component] {
					return fmt.Errorf("InvalidDefinition: duplicate workload component %q", component)
				}
				components[component] = true
				if s.VersionSource != nil && string(s.VersionSource.FromFamily) == name && string(s.VersionSource.FromComponent) == component {
					sourceMatches++
				}
			}
		}
	}
	if total > MaxChecks {
		return fmt.Errorf("InvalidDefinition: too many checks")
	}
	if vs := s.VersionSource; vs != nil {
		if !ident(string(vs.FromFamily)) || !ident(string(vs.FromComponent)) || (vs.Container != "" && !dnsLabel(string(vs.Container))) || sourceMatches != 1 {
			return fmt.Errorf("InvalidDefinition: versionSource must resolve exactly one workload")
		}
	}
	for _, rule := range s.RequestedReads {
		if err := validateRead(rule); err != nil {
			return fmt.Errorf("InvalidDefinition: requestedReads: %w", err)
		}
	}
	return nil
}

// ValidateBinding validates only the declared contract; live UID ownership,
// uniqueness and actual RBAC permissions must still be verified before each run.
func ValidateBinding(b *api.AddonDefinitionBinding) error {
	if b == nil {
		return fmt.Errorf("InvalidBinding: binding is nil")
	}
	if err := boundedSpec(b.Spec, MaxBindingBytes); err != nil {
		return err
	}
	if !dnsLabel(b.Name) || b.Name != string(b.Spec.DefinitionRef.Name) {
		return fmt.Errorf("InvalidBinding: name must equal definitionRef.name")
	}
	for _, r := range []api.DefinitionObjectReference{{Name: api.DefinitionResourceName(b.Spec.DefinitionRef.Name), UID: b.Spec.DefinitionRef.UID}, b.Spec.ServiceAccountRef} {
		if !resourceName(string(r.Name)) || r.UID == "" || len(r.UID) > MaxUIDBytes {
			return fmt.Errorf("InvalidBinding: invalid name or UID")
		}
	}
	if !dnsLabel(b.Namespace) {
		return fmt.Errorf("InvalidBinding: explicit operator namespace required")
	}
	s := b.Spec.TargetScope
	if len(s.Namespaces) == 0 && !s.AllowClusterScoped {
		return fmt.Errorf("InvalidBinding: target scope is empty")
	}
	return namespaces(s.Namespaces)
}

// boundedSpec limits recursive work before canonical JSON allocation. The typed
// wire contract contains no cycles or unstructured extension points.
func boundedSpec(spec any, maxBytes int) error {
	if err := walk(reflect.ValueOf(spec), 0, ""); err != nil {
		return err
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("InvalidDefinition: %w", err)
	}
	if len(raw) > maxBytes {
		return fmt.Errorf("DefinitionTooLarge: canonical spec exceeds %d bytes", maxBytes)
	}
	return nil
}
func walk(v reflect.Value, depth int, path string) error {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if depth > MaxSpecDepth {
		return fmt.Errorf("InvalidDefinition: %s exceeds nesting depth", path)
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if tag == "-" {
				continue
			}
			if err := walk(v.Field(i), depth+1, path+"."+tag); err != nil {
				return err
			}
		}
	case reflect.Map:
		if v.Len() > MaxMapEntries {
			return fmt.Errorf("InvalidDefinition: %s has too many entries", path)
		}
		iter := v.MapRange()
		for iter.Next() {
			if err := boundedString(iter.Key().String(), MaxStringBytes); err != nil {
				return err
			}
			if err := walk(iter.Value(), depth+1, path); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		cap := MaxListEntries
		if path == ".families" {
			cap = MaxFamilies
		}
		if v.Len() > cap {
			return fmt.Errorf("InvalidDefinition: %s has too many entries", path)
		}
		for i := 0; i < v.Len(); i++ {
			if err := walk(v.Index(i), depth+1, path); err != nil {
				return err
			}
		}
	case reflect.String:
		if err := boundedString(v.String(), MaxStringBytes); err != nil {
			return fmt.Errorf("InvalidDefinition: %s: %w", path, err)
		}
	}
	return nil
}
func boundedString(s string, maxBytes int) error {
	if !utf8.ValidString(s) || len(s) > maxBytes || utf8.RuneCountInString(s) > MaxStringRunes {
		return fmt.Errorf("invalid UTF-8 or string exceeds %d bytes", maxBytes)
	}
	return nil
}
func ident(s string) bool    { return len(s) <= MaxIdentifierBytes && identifier.MatchString(s) }
func dnsLabel(s string) bool { return len(validation.IsDNS1123Label(s)) == 0 }
func resourceName(s string) bool {
	return len(s) <= MaxResourceSegmentBytes && len(validation.IsDNS1123Subdomain(s)) == 0
}
func token(s string) bool { return len(s) <= MaxResourceSegmentBytes && kindToken.MatchString(s) }
func apiVersion(s string) bool {
	p := strings.Split(s, "/")
	if len(p) > 2 || len(p) == 0 {
		return false
	}
	if len(p) == 2 && !resourceName(p[0]) {
		return false
	}
	v := p[len(p)-1]
	return len(v) <= MaxResourceSegmentBytes && versionToken.MatchString(v)
}
func namespaces(ns []api.DefinitionDNSLabel) error {
	if len(ns) > MaxNamespaces {
		return fmt.Errorf("ScopeDenied: too many namespaces")
	}
	seen := map[api.DefinitionDNSLabel]bool{}
	for _, n := range ns {
		if !dnsLabel(string(n)) || seen[n] {
			return fmt.Errorf("ScopeDenied: invalid or duplicate namespace %q", n)
		}
		seen[n] = true
	}
	return nil
}
func target(t api.DefinitionTarget, singleton bool, scope string) error {
	if err := namespaces(t.Namespaces); err != nil {
		return err
	}
	if t.Scope != "Namespaced" && t.Scope != "Cluster" {
		return fmt.Errorf("invalid target scope")
	}
	if scope != "" && t.Scope != scope {
		return fmt.Errorf("target must be %s", scope)
	}
	if t.Scope == "Cluster" && len(t.Namespaces) != 0 {
		return fmt.Errorf("cluster target cannot have namespaces")
	}
	if t.Scope == "Namespaced" && (len(t.Namespaces) == 0 || (singleton && len(t.Namespaces) != 1)) {
		return fmt.Errorf("explicit target namespaces required; singletons need exactly one")
	}
	return nil
}
func outcomes(values ...api.DefinitionOutcome) error {
	for _, v := range values {
		switch v {
		case "", "Pass", "Warn", "Fail", "Error", "Skipped":
		default:
			return fmt.Errorf("invalid outcome %q", v)
		}
	}
	return nil
}
func posture(p api.DefinitionPosture) error {
	if p != "" && p != "Required" && p != "Optional" {
		return fmt.Errorf("invalid absence posture")
	}
	return nil
}
func thresholds(keys ...api.DefinitionThresholdKey) error {
	for _, k := range keys {
		if k == "" {
			continue
		}
		if len(k) > MaxIdentifierBytes || !thresholdKey.MatchString(string(k)) || k == "warnRatio" || k == "failRatio" {
			return fmt.Errorf("invalid or reserved threshold key %q", k)
		}
	}
	return nil
}
func component(c api.DefinitionIdentifier) error {
	if c != "" && !ident(string(c)) {
		return fmt.Errorf("invalid component %q", c)
	}
	return nil
}
func duration(s api.DefinitionDuration, positive bool) error {
	if s == "" && !positive {
		return nil
	}
	if len(s) > MaxDurationBytes {
		return fmt.Errorf("duration too long")
	}
	d, err := time.ParseDuration(string(s))
	if err != nil {
		return fmt.Errorf("invalid duration: %w", err)
	}
	if d < 0 || (positive && d == 0) {
		return fmt.Errorf("duration must be positive (or zero only when supported)")
	}
	return nil
}
func versions(v []api.DefinitionToken, required bool) error {
	if len(v) > MaxAPIVersions || (required && len(v) == 0) {
		return fmt.Errorf("expected 1–8 API versions")
	}
	seen := map[api.DefinitionToken]bool{}
	for _, x := range v {
		if !versionToken.MatchString(string(x)) || len(x) > MaxResourceSegmentBytes || seen[x] {
			return fmt.Errorf("invalid or duplicate API version")
		}
		seen[x] = true
	}
	return nil
}
func names(v []api.DefinitionResourceName) error {
	if len(v) > MaxListEntries {
		return fmt.Errorf("too many names")
	}
	seen := map[api.DefinitionResourceName]bool{}
	for _, x := range v {
		if !resourceName(string(x)) || seen[x] {
			return fmt.Errorf("invalid or duplicate resource name")
		}
		seen[x] = true
	}
	return nil
}

// ValidateVersionRange caps parser input and work before invoking SemVer.
func ValidateVersionRange(s string) error {
	if s == "" {
		return nil
	}
	if len(s) > MaxVersionRangeBytes {
		return fmt.Errorf("InvalidVersionRange: exceeds byte cap")
	}
	alternatives := strings.Split(s, "||")
	if len(alternatives) > MaxVersionAlternatives {
		return fmt.Errorf("InvalidVersionRange: too many alternatives")
	}
	// Count comparison operands, including both endpoints of hyphen ranges. Bare
	// operators are not operands; malformed text is rejected by the parser below.
	count := 0
	for _, a := range alternatives {
		for _, f := range strings.FieldsFunc(a, func(r rune) bool { return unicode.IsSpace(r) || r == ',' }) {
			if strings.Trim(f, "<>=!~^- ") != "" {
				count++
			}
		}
	}
	if count > MaxVersionComparators {
		return fmt.Errorf("InvalidVersionRange: too many comparators")
	}
	if _, err := semver.NewConstraint(s); err != nil {
		return fmt.Errorf("InvalidVersionRange: %w", err)
	}
	return nil
}

// ValidateSelector bounds both matchLabels and matchExpressions before parsing.
func ValidateSelector(s *metav1.LabelSelector) error {
	if s == nil {
		return nil
	}
	if len(s.MatchLabels)+len(s.MatchExpressions) > MaxSelectorTerms {
		return fmt.Errorf("InvalidDefinition: too many selector terms")
	}
	for k, v := range s.MatchLabels {
		if len(validation.IsQualifiedName(k)) != 0 || len(v) > MaxSelectorValueBytes || len(validation.IsValidLabelValue(v)) != 0 {
			return fmt.Errorf("InvalidDefinition: invalid selector label")
		}
	}
	for _, e := range s.MatchExpressions {
		if len(e.Values) > MaxSelectorValues || len(validation.IsQualifiedName(e.Key)) != 0 {
			return fmt.Errorf("InvalidDefinition: invalid selector expression")
		}
		for _, v := range e.Values {
			if len(v) > MaxSelectorValueBytes || len(validation.IsValidLabelValue(v)) != 0 {
				return fmt.Errorf("InvalidDefinition: invalid selector value")
			}
		}
	}
	if _, err := metav1.LabelSelectorAsSelector(s); err != nil {
		return fmt.Errorf("InvalidDefinition: %w", err)
	}
	return nil
}

func validateRead(r api.DefinitionReadRule) error {
	resource := len(r.Resources) > 0
	discovery := len(r.NonResourceURLs) > 0
	if resource == discovery || len(r.Verbs) == 0 || len(r.Verbs) > 2 || len(r.Resources) > 32 || len(r.NonResourceURLs) > 32 || len(r.ResourceNames) > 32 {
		return fmt.Errorf("invalid rule shape")
	}
	seen := map[string]bool{}
	for _, v := range r.Verbs {
		if seen[v] || (v != "get" && v != "list") || (discovery && v != "get") {
			return fmt.Errorf("only distinct read verbs permitted")
		}
		seen[v] = true
	}
	if resource {
		if r.APIGroup == nil || (*r.APIGroup != "" && !resourceName(*r.APIGroup)) {
			return fmt.Errorf("invalid apiGroup")
		}
		seen = map[string]bool{}
		for _, v := range r.Resources {
			if len(v) > MaxResourceSegmentBytes || !resourceToken.MatchString(v) || seen[v] {
				return fmt.Errorf("exact resource plural required")
			}
			seen[v] = true
		}
		return names(r.ResourceNames)
	}
	if r.APIGroup != nil || len(r.ResourceNames) > 0 {
		return fmt.Errorf("discovery cannot carry resource fields")
	}
	seen = map[string]bool{}
	for _, path := range r.NonResourceURLs {
		if seen[path] || !discoveryPath(path) {
			return fmt.Errorf("exact discovery URL required")
		}
		seen[path] = true
	}
	return nil
}
func discoveryPath(path string) bool {
	if path == "/api" || path == "/apis" {
		return true
	}
	p := strings.Split(path, "/")
	if len(p) == 3 && p[0] == "" && p[1] == "api" {
		return versionToken.MatchString(p[2]) && len(p[2]) <= 253
	}
	if (len(p) == 3 || len(p) == 4) && p[0] == "" && p[1] == "apis" && resourceName(p[2]) {
		return len(p) == 3 || (versionToken.MatchString(p[3]) && len(p[3]) <= 253)
	}
	return false
}
