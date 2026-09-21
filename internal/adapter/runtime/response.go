/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func urlQuery(raw string) (map[string]string, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid API query")
	}
	out := map[string]string{}
	for key, entries := range values {
		if len(entries) != 1 {
			return nil, fmt.Errorf("ambiguous repeated query parameter")
		}
		out[key] = entries[0]
	}
	return out, nil
}

func (g *Guard) inspect(data []byte, route apiRoute) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return g.budget.Fail("InputLimitExceeded", "invalid JSON API response")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return g.budget.Fail("InputLimitExceeded", "trailing API response data")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return g.budget.Fail("InputLimitExceeded", "API response must be an object")
	}
	if route.list {
		raw, exists := object["items"]
		if !exists {
			return g.budget.Fail("InputLimitExceeded", "collection response has no items")
		}
		items, ok := raw.([]any)
		if !ok && raw != nil {
			return g.budget.Fail("InputLimitExceeded", "invalid collection items")
		}
		if len(items) > limits.MaxPageObjects {
			return g.budget.Fail("WorkLimitExceeded", "API page exceeds object limit")
		}
		if err := g.budget.ChargeObjects(len(items)); err != nil {
			return err
		}
		for _, item := range items {
			if _, ok := item.(map[string]any); !ok {
				return g.budget.Fail("InputLimitExceeded", "collection item is not an object")
			}
			if err := g.inspectObject(item); err != nil {
				return err
			}
		}
		envelope := map[string]any{}
		for key, v := range object {
			if key != "items" {
				envelope[key] = v
			}
		}
		if err := g.inspectObject(envelope); err != nil {
			return err
		}
		if metadata, ok := object["metadata"].(map[string]any); ok {
			if token, ok := metadata["continue"].(string); ok && token != "" && g.budget.Objects() >= limits.MaxRunObjects {
				return g.budget.Fail("WorkLimitExceeded", "continuation remains at object cap")
			}
		}
	} else {
		if err := g.budget.ChargeObjects(1); err != nil {
			return err
		}
		if err := g.inspectObject(object); err != nil {
			return err
		}
	}
	if route.discovery && route.groupVersion != "" {
		return g.inspectDiscovery(object, route.groupVersion)
	}
	return nil
}

func (g *Guard) inspectObject(value any) error {
	nodes := 0
	var walk func(any, int) error
	walk = func(v any, depth int) error {
		nodes++
		if nodes > limits.MaxObjectNodes || depth > limits.MaxObjectDepth {
			return g.budget.Fail("InputLimitExceeded", "target object exceeds node/depth limit")
		}
		if err := g.budget.Visit(1); err != nil {
			return err
		}
		switch x := v.(type) {
		case map[string]any:
			for _, child := range x {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range x {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(value, 0)
}

func (g *Guard) inspectDiscovery(object map[string]any, version string) error {
	if object["kind"] != "APIResourceList" || object["groupVersion"] != version {
		return g.budget.Fail("InputLimitExceeded", "unexpected resource discovery response")
	}
	gv, err := schema.ParseGroupVersion(version)
	if err != nil {
		return g.budget.Fail("InputLimitExceeded", "invalid discovery group/version")
	}
	resources, ok := object["resources"].([]any)
	if !ok && object["resources"] != nil {
		return g.budget.Fail("InputLimitExceeded", "invalid discovery resources")
	}
	for _, raw := range resources {
		resource, ok := raw.(map[string]any)
		if !ok {
			return g.budget.Fail("InputLimitExceeded", "invalid discovery resource")
		}
		kind, _ := resource["kind"].(string)
		expected, declared := g.expected[schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: kind}]
		if !declared {
			continue
		}
		actual, ok := resource["namespaced"].(bool)
		if !ok || actual != expected {
			return g.budget.Fail("ScopeDenied", "discovered resource scope differs from declaration")
		}
	}
	return nil
}
