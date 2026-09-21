/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package addondefinition

import (
	"encoding/json"
	"fmt"
	"sort"

	api "github.com/skaphos/fathom/api/v1alpha1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// ScopedGrant binds a read rule to exactly one namespace, or cluster scope when
// Namespace is empty. Consumers must never merge the two scopes.
type ScopedGrant struct {
	Namespace string
	Rule      rbacv1.PolicyRule
}

// GrantPlan is offline advice for the reviewed defaults, not runtime authority.
// Diagnostics identify permissions that still require administrator review.
type GrantPlan struct {
	Grants      []ScopedGrant
	Diagnostics []string
}

// PlanGrants derives only resource mappings fixed by typed evaluators. Arbitrary
// GVKs and requestedReads cannot prove scope or plural names without discovery.
func PlanGrants(d *api.AddonDefinition) (GrantPlan, error) {
	if err := Validate(d); err != nil {
		return GrantPlan{}, err
	}
	var plan GrantPlan
	add := func(target api.DefinitionTarget, group, resource, verb string, names ...string) {
		namespaces := target.Namespaces
		if target.Scope == "Cluster" {
			namespaces = []api.DefinitionDNSLabel{""}
		}
		for _, ns := range namespaces {
			plan.Grants = append(plan.Grants, ScopedGrant{Namespace: string(ns), Rule: rbacv1.PolicyRule{
				APIGroups: []string{group}, Resources: []string{resource}, Verbs: []string{verb}, ResourceNames: append([]string(nil), names...),
			}})
		}
	}
	cluster := api.DefinitionTarget{Scope: "Cluster"}
	for _, family := range d.Spec.Families {
		for _, c := range family.Checks {
			path := string(family.Name) + "/" + string(c.Name)
			override := func(key api.DefinitionThresholdKey) {
				if key != "" {
					plan.Diagnostics = append(plan.Diagnostics, path+": name overrides require reviewed resourceNames grants; only the default name is granted")
				}
			}
			unresolved := func(version string, kind api.DefinitionToken) {
				plan.Diagnostics = append(plan.Diagnostics, fmt.Sprintf("%s: manually complete grants for %s %s after verifying resource plural, scope, and discovery reads", path, version, kind))
			}
			switch c.Kind {
			case "Workload":
				p := c.Workload
				resource := map[string]string{"Deployment": "deployments", "DaemonSet": "daemonsets", "StatefulSet": "statefulsets"}[p.Kind]
				add(p.Target, "apps", resource, "get", string(p.DefaultName))
				if p.CheckPods {
					add(p.Target, "", "pods", "list")
				}
				override(p.NameThresholdKey)
			case "CRD":
				for _, name := range c.CRD.Names {
					add(cluster, "apiextensions.k8s.io", "customresourcedefinitions", "get", string(name))
				}
			case "Webhook":
				p := c.Webhook
				resource := "validatingwebhookconfigurations"
				if p.Kind == "MutatingWebhookConfiguration" {
					resource = "mutatingwebhookconfigurations"
				}
				add(cluster, "admissionregistration.k8s.io", resource, "get", string(p.Name))
				if p.VerifyEndpoints {
					add(api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{p.ServiceNamespace}}, "discovery.k8s.io", "endpointslices", "list")
				}
				override(p.NameThresholdKey)
			case "CronJob":
				p := c.CronJob
				add(p.Target, "batch", "cronjobs", "get", string(p.DefaultName))
				override(p.NameThresholdKey)
			case "ConfigMap":
				p := c.ConfigMap
				add(p.Target, "", "configmaps", "get", string(p.DefaultName))
				override(p.NameThresholdKey)
			case "PodProjection":
				add(c.PodProjection.Target, "", "pods", "list")
			case "Condition":
				p := c.Condition
				unresolved(p.APIVersion, p.Kind)
				if p.VersionCRD != "" {
					add(cluster, "apiextensions.k8s.io", "customresourcedefinitions", "get", string(p.VersionCRD))
				}
			case "Field":
				unresolved(c.Field.APIVersion, c.Field.Kind)
			case "AnnotationStaleness":
				unresolved(c.AnnotationStaleness.APIVersion, c.AnnotationStaleness.Kind)
			}
		}
	}
	if len(d.Spec.RequestedReads) != 0 {
		plan.Diagnostics = append(plan.Diagnostics, "requestedReads requires manual comparison with generated grants; declarations alone never establish resource scope or authorize additional grants")
	}
	// JSON is a deterministic ordering key for these string/slice-only structs.
	keys := make(map[string]ScopedGrant, len(plan.Grants))
	for _, grant := range plan.Grants {
		key, _ := json.Marshal(grant)
		keys[string(key)] = grant
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	plan.Grants = nil
	for _, key := range ordered {
		plan.Grants = append(plan.Grants, keys[key])
	}
	sort.Strings(plan.Diagnostics)
	return plan, nil
}
