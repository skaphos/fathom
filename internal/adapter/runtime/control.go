/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime

import (
	"fmt"
	"net/http"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

// ControlTargets pins the metadata reads allowed for one run's authority and
// publication fences. This is separate from the evaluator's binding scope.
type ControlTargets struct {
	OperatorNamespace, DefinitionName, LeaseName string
	Check                                        types.NamespacedName
}

// ControlGuard applies the same byte/work/deadline limits as evaluator traffic
// without recording manager reads as evidence of delegated permissions.
type ControlGuard struct {
	guard   *Guard
	targets ControlTargets
}

func NewControlGuard(b *Budget, targets ControlTargets) (*ControlGuard, error) {
	if b == nil || len(validation.IsDNS1123Label(targets.OperatorNamespace)) != 0 ||
		len(validation.IsDNS1123Label(targets.DefinitionName)) != 0 ||
		len(validation.IsDNS1123Label(targets.Check.Namespace)) != 0 ||
		len(validation.IsDNS1123Subdomain(targets.Check.Name)) != 0 ||
		len(validation.IsDNS1123Subdomain(targets.LeaseName)) != 0 {
		return nil, fmt.Errorf("AuthorizationUnavailable: budget and exact control-plane targets required")
	}
	g := &Guard{budget: b, cluster: true, namespaces: map[string]bool{targets.OperatorNamespace: true, targets.Check.Namespace: true}, retries: map[string]int{}}
	return &ControlGuard{guard: g, targets: targets}, nil
}

func (g *ControlGuard) Wrap(next http.RoundTripper) http.RoundTripper {
	return &guardedTransport{guard: g.guard, next: next, control: &g.targets}
}

func (t ControlTargets) permits(route apiRoute) bool {
	if route.discovery {
		return false
	} // The control reader uses a fixed local mapper.
	switch route.groupVersion + "/" + route.resource {
	case api.GroupVersion.String() + "/addondefinitions":
		return route.namespace == "" && route.name == t.DefinitionName
	case api.GroupVersion.String() + "/addondefinitionbindings":
		return route.namespace == t.OperatorNamespace && (route.list || route.name == t.DefinitionName)
	case api.GroupVersion.String() + "/addonchecks":
		return route.namespace == t.Check.Namespace && route.name == t.Check.Name
	case "v1/serviceaccounts":
		// The binding supplies the SA name only after the first metadata reads.
		return route.namespace == t.OperatorNamespace && !route.list
	case "coordination.k8s.io/v1/leases":
		return route.namespace == t.OperatorNamespace && route.name == t.LeaseName
	default:
		return false
	}
}
