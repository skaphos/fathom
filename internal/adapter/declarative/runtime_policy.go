/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"fmt"
	"strconv"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/pkg/adapter"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
)

func resolveRuntimePolicy(ctx context.Context, d *api.AddonDefinition, policy map[adapter.Family]adapter.FamilyPolicy) error {
	if len(policy) > definitions.MaxMapEntries {
		return fmt.Errorf("InvalidDefinition: too many policy families")
	}
	for _, f := range d.Spec.Families {
		p := policy[adapter.Family(f.Name)]
		if len(p.Namespaces) > definitions.MaxNamespaces || len(p.Thresholds) > definitions.MaxMapEntries {
			return fmt.Errorf("InvalidDefinition: policy exceeds input limits")
		}
		if err := definitions.ValidateSelector(p.LabelSelector); err != nil {
			return err
		}
		for k, v := range p.Thresholds {
			if len(k) > definitions.MaxIdentifierBytes || len(v) > definitions.MaxStringBytes {
				return fmt.Errorf("InvalidDefinition: threshold exceeds byte limit")
			}
		}
		for i := range f.Checks {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := resolveRuntimeCheck(&f.Checks[i], p); err != nil {
				return err
			}
		}
	}
	if err := definitions.Validate(d); err != nil {
		return err
	}
	return ctx.Err()
}
func resolveRuntimeCheck(c *api.DefinitionCheck, p adapter.FamilyPolicy) error {
	target := runtimeTarget(c)
	if len(p.Namespaces) > 0 {
		if c.Webhook != nil {
			if len(p.Namespaces) != 1 || c.Webhook.ExpectedService == "" {
				return fmt.Errorf("ScopeDenied: webhook override needs one explicit service namespace")
			}
			c.Webhook.ServiceNamespace = api.DefinitionDNSLabel(p.Namespaces[0])
		} else {
			if target.Scope == "Cluster" {
				return fmt.Errorf("ScopeDenied: namespace override on cluster target")
			}
			target.Namespaces = make([]api.DefinitionDNSLabel, len(p.Namespaces))
			for i, n := range p.Namespaces {
				target.Namespaces[i] = api.DefinitionDNSLabel(n)
			}
		}
	}
	switch {
	case c.Workload != nil:
		w := c.Workload
		resolveRuntimeName(p, w.NameThresholdKey, &w.DefaultName)
		if value, ok := runtimeOverride(p, w.RestartWarnThresholdKey); ok {
			n, err := strconv.ParseInt(value, 10, 32)
			if err != nil || n < 0 {
				return fmt.Errorf("InvalidDefinition: restart override must be a nonnegative int32")
			}
			w.DefaultRestartWarn = int32(n)
		}
	case c.Webhook != nil:
		resolveRuntimeName(p, c.Webhook.NameThresholdKey, &c.Webhook.Name)
	case c.CronJob != nil:
		w := c.CronJob
		resolveRuntimeName(p, w.NameThresholdKey, &w.DefaultName)
		if value, ok := runtimeOverride(p, w.SuccessMaxAgeThresholdKey); ok {
			if value == "" {
				return fmt.Errorf("InvalidDefinition: empty duration override")
			}
			w.DefaultSuccessMaxAge = api.DefinitionDuration(value)
		}
	case c.ConfigMap != nil:
		resolveRuntimeName(p, c.ConfigMap.NameThresholdKey, &c.ConfigMap.DefaultName)
	case c.AnnotationStaleness != nil:
		w := c.AnnotationStaleness
		resolveRuntimeName(p, w.NameThresholdKey, &w.DefaultName)
		if value, ok := runtimeOverride(p, w.MaxAgeThresholdKey); ok {
			w.DefaultMaxAge = api.DefinitionDuration(value)
		}
	}
	return nil
}
func runtimeOverride(p adapter.FamilyPolicy, key api.DefinitionThresholdKey) (string, bool) {
	if key == "" {
		return "", false
	}
	value, ok := p.Thresholds[string(key)]
	return value, ok
}
func resolveRuntimeName(p adapter.FamilyPolicy, key api.DefinitionThresholdKey, name *api.DefinitionResourceName) {
	if value, ok := runtimeOverride(p, key); ok {
		*name = api.DefinitionResourceName(value)
	}
}
func runtimeTarget(c *api.DefinitionCheck) *api.DefinitionTarget {
	switch {
	case c.Workload != nil:
		return &c.Workload.Target
	case c.CRD != nil:
		return &c.CRD.Target
	case c.Condition != nil:
		return &c.Condition.Target
	case c.Field != nil:
		return &c.Field.Target
	case c.Webhook != nil:
		return &c.Webhook.Target
	case c.CronJob != nil:
		return &c.CronJob.Target
	case c.ConfigMap != nil:
		return &c.ConfigMap.Target
	case c.AnnotationStaleness != nil:
		return &c.AnnotationStaleness.Target
	case c.PodProjection != nil:
		return &c.PodProjection.Target
	}
	panic("validated runtime check has no target")
}
