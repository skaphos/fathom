/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package addondefinition

import (
	"fmt"
	"sort"

	api "github.com/skaphos/fathom/api/v1alpha1"
)

// TargetScope returns the union of primary and helper scopes in a validated
// definition. It does not infer the scope of requestedReads or grant permissions.
func TargetScope(d *api.AddonDefinition) (api.DefinitionBindingScope, error) {
	if err := Validate(d); err != nil {
		return api.DefinitionBindingScope{}, err
	}
	var scope api.DefinitionBindingScope
	namespaces := map[api.DefinitionDNSLabel]bool{}
	for _, family := range d.Spec.Families {
		for _, c := range family.Checks {
			var target api.DefinitionTarget
			switch c.Kind {
			case "Workload":
				target = c.Workload.Target
			case "CRD":
				target = c.CRD.Target
			case "Condition":
				target = c.Condition.Target
				if c.Condition.VersionCRD != "" {
					scope.AllowClusterScoped = true
				}
			case "Field":
				target = c.Field.Target
			case "Webhook":
				target = c.Webhook.Target
				if c.Webhook.ServiceNamespace != "" {
					namespaces[c.Webhook.ServiceNamespace] = true
				}
			case "CronJob":
				target = c.CronJob.Target
			case "ConfigMap":
				target = c.ConfigMap.Target
			case "AnnotationStaleness":
				target = c.AnnotationStaleness.Target
			case "PodProjection":
				target = c.PodProjection.Target
			}
			if target.Scope == "Cluster" {
				scope.AllowClusterScoped = true
			}
			for _, ns := range target.Namespaces {
				namespaces[ns] = true
			}
		}
	}
	if len(namespaces) > MaxNamespaces {
		return scope, fmt.Errorf("combined target namespaces exceed %d", MaxNamespaces)
	}
	for ns := range namespaces {
		scope.Namespaces = append(scope.Namespaces, ns)
	}
	sort.Slice(scope.Namespaces, func(i, j int) bool { return scope.Namespaces[i] < scope.Namespaces[j] })
	return scope, nil
}
