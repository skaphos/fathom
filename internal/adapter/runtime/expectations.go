/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime

import (
	"fmt"

	api "github.com/skaphos/fathom/api/v1alpha1"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DiscoveryExpectations records primary/helper scopes without guessing plurals.
// Live delegated discovery must confirm these before the mapper can use them.
func DiscoveryExpectations(d *api.AddonDefinition) (map[schema.GroupVersionKind]bool, error) {
	if err := limits.Validate(d); err != nil {
		return nil, err
	}
	out := map[schema.GroupVersionKind]bool{}
	add := func(version, kind string, namespaced bool) error {
		key := schema.FromAPIVersionAndKind(version, kind)
		if previous, found := out[key]; found && previous != namespaced {
			return fmt.Errorf("InvalidDefinition: contradictory scope for %s", key)
		}
		out[key] = namespaced
		return nil
	}
	for _, family := range d.Spec.Families {
		for _, check := range family.Checks {
			var version, kind string
			namespaced := false
			switch check.Kind {
			case "Workload":
				version = "apps/v1"
				kind = check.Workload.Kind
				namespaced = true
				if check.Workload.CheckPods {
					if err := add("v1", "Pod", true); err != nil {
						return nil, err
					}
				}
			case "CRD":
				version = "apiextensions.k8s.io/v1"
				kind = "CustomResourceDefinition"
			case "Condition":
				p := check.Condition
				version = p.APIVersion
				kind = string(p.Kind)
				namespaced = p.Target.Scope == "Namespaced"
				if p.VersionCRD != "" {
					if err := add("apiextensions.k8s.io/v1", "CustomResourceDefinition", false); err != nil {
						return nil, err
					}
					gv, _ := schema.ParseGroupVersion(version)
					for _, candidate := range p.SupportedVersions {
						gv.Version = string(candidate)
						if err := add(gv.String(), kind, namespaced); err != nil {
							return nil, err
						}
					}
				}
			case "Field":
				version = check.Field.APIVersion
				kind = string(check.Field.Kind)
				namespaced = check.Field.Target.Scope == "Namespaced"
			case "Webhook":
				version = "admissionregistration.k8s.io/v1"
				kind = check.Webhook.Kind
				if check.Webhook.VerifyEndpoints {
					if err := add("discovery.k8s.io/v1", "EndpointSlice", true); err != nil {
						return nil, err
					}
				}
			case "CronJob":
				version = "batch/v1"
				kind = "CronJob"
				namespaced = true
			case "ConfigMap":
				version = "v1"
				kind = "ConfigMap"
				namespaced = true
			case "AnnotationStaleness":
				version = check.AnnotationStaleness.APIVersion
				kind = string(check.AnnotationStaleness.Kind)
				namespaced = check.AnnotationStaleness.Target.Scope == "Namespaced"
			case "PodProjection":
				version = "v1"
				kind = "Pod"
				namespaced = true
			}
			if err := add(version, kind, namespaced); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
