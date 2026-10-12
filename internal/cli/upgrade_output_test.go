/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func olderDNSCheck(c *apiextensionsv1.CustomResourceDefinition) {
	if c.Name == dnsCRD {
		spec := crdSchema(c).Properties["spec"]
		delete(spec.Properties, "timeout")
		crdSchema(c).Properties["spec"] = spec
	}
}

func TestUpgradeDryRunPrintsThePlanAndWritesNothing(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := upgradeEnv(t, "0.5.1", "0.5.1", olderDNSCheck, nil)
	before := len(env.log.all())
	out, _, err := execVerb(env.f, "upgrade", "--dry-run")
	if err != nil {
		t.Fatalf("upgrade --dry-run: %v", err)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("dry run wrote %v", w)
	}
	for _, want := range []string{
		"Fathom upgrade plan: v0.5.1 -> v0.6.0 (release fathom in fathom-system)",
		"Path:      direct",
		dnsCRD, "update", "unchanged",
		"Operator:  " + operatorRepository + ":v0.5.1 -> " + operatorRepository + "@" + fakeOperatorDigest,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan missing %q:\n%s", want, out)
		}
	}
}

// A refused dry run still prints the plan, including the refusal's change.
func TestUpgradeDryRunShowsSanctionedChanges(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := upgradeEnv(t, "0.5.1", "0.5.1", unboundClusterHealthChildren, nil)
	out, _, err := execVerb(env.f, "upgrade", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), reasonAckRequired) {
		t.Fatalf("err = %v, want %s", err, reasonAckRequired)
	}
	if !strings.Contains(out, "incompatible (sanctioned by v0.6.0 (https://github.com/skaphos/fathom/issues/277))") {
		t.Errorf("plan does not show the sanctioned change:\n%s", out)
	}
}

func TestUpgradePlanJSONAndYAML(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			withVersion(t, "v0.6.0", true)
			env := upgradeEnv(t, "0.5.1", "0.5.1", olderDNSCheck, map[string]any{"apiToken": "s3cr3t"})
			out, _, err := execVerb(env.f, "upgrade", "--dry-run", "-o", format)
			if err != nil {
				t.Fatalf("upgrade --dry-run -o %s: %v", format, err)
			}
			raw := []byte(out)
			if format == "yaml" {
				if raw, err = yaml.YAMLToJSON(raw); err != nil {
					t.Fatal(err)
				}
			}
			var plan struct {
				From, To, Path string
				CRDs           []struct {
					Name, Action string
					Findings     []map[string]any
				} `json:"crds"`
				Operator struct {
					FromImages map[string]string `json:"fromImages"`
					ToImages   map[string]string `json:"toImages"`
				}
				Values map[string]any
			}
			if err := json.Unmarshal(raw, &plan); err != nil {
				t.Fatalf("decode %s plan: %v\n%s", format, err, out)
			}
			if plan.From != "0.5.1" || plan.To != "0.6.0" || plan.Path != "direct" || len(plan.CRDs) != 9 {
				t.Errorf("plan = %+v", plan)
			}
			actions := map[string]bool{}
			for _, c := range plan.CRDs {
				actions[c.Action] = true
			}
			if !actions[crdUpdate] || !actions[crdUnchanged] {
				t.Errorf("actions = %v, want update and unchanged", actions)
			}
			if plan.Operator.ToImages["operator"] != operatorRepository+"@"+fakeOperatorDigest {
				t.Errorf("toImages = %v", plan.Operator.ToImages)
			}
			if plan.Values["apiToken"] != "<redacted>" {
				t.Errorf("secret-looking value printed: %v", plan.Values["apiToken"])
			}
		})
	}
}
