/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime

import (
	"slices"
	"strings"
	"sync"

	api "github.com/skaphos/fathom/api/v1alpha1"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PermissionDiagnostic is run-local evidence for PermissionsVerified, independent
// of metrics and execution verdicts. Callers supply generation/time when writing
// status. DeclarationReason is advisory and must never authorize or deny reads.
type PermissionDiagnostic struct {
	Status                             metav1.ConditionStatus
	Reason, Message, DeclarationReason string
}

type observedRead struct{ group, resource, verb, name, discoveryURL string }

// At most MaxRunRequests entries are retained; only Guard's charged network
// attempts reach this recorder. No response bodies, credentials or queries enter it.
type permissionObservations struct {
	mu                  sync.Mutex
	pending, succeeded  int
	denied, unavailable bool
	reads               []observedRead
}

func (p *permissionObservations) begin(route apiRoute, path string) {
	read := observedRead{verb: "get", group: strings.Split(route.groupVersion, "/")[0], resource: route.resource, name: route.name}
	if !strings.Contains(route.groupVersion, "/") {
		read.group = ""
	}
	if route.list {
		read.verb = "list"
	}
	if route.discovery {
		read.discoveryURL = path
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending++
	p.reads = append(p.reads, read)
}

func (p *permissionObservations) finish(status int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending--
	switch {
	case status == 403:
		p.denied = true
	case status >= 200 && status < 300:
		p.succeeded++
	default:
		p.unavailable = true
	}
}

// Diagnostics compares declarations locally with known default evaluator/helper
// needs and observed requests. It performs no IO and proves neither effective
// grants nor future access. Use the immutable definition for this guard's run.
func (g *Guard) Diagnostics(d *api.AddonDefinition) (PermissionDiagnostic, error) {
	plan, err := definitions.PlanGrants(d)
	if err != nil {
		return PermissionDiagnostic{}, err
	}
	expected, err := DiscoveryExpectations(d)
	if err != nil {
		return PermissionDiagnostic{}, err
	}
	p := &g.permissions
	p.mu.Lock()
	reads := slices.Clone(p.reads)
	result := PermissionDiagnostic{Status: metav1.ConditionUnknown, Reason: "NotEvaluated", Message: "No delegated requests have completed."}
	switch {
	case p.denied:
		result.Status, result.Reason, result.Message = metav1.ConditionFalse, "AccessDenied", "A delegated API request was forbidden."
	case p.unavailable || p.pending != 0:
		result.Reason, result.Message = "AccessCheckUnavailable", "Some delegated requests have no successful response; access could not be verified."
	case p.succeeded > 0:
		result.Status, result.Reason, result.Message = metav1.ConditionTrue, "RequestsSucceeded", "Access was confirmed only for the delegated requests that succeeded in this run."
	}
	p.mu.Unlock()
	for _, grant := range plan.Grants {
		for _, verb := range grant.Rule.Verbs {
			for _, resource := range grant.Rule.Resources {
				names := grant.Rule.ResourceNames
				if len(names) == 0 {
					names = []string{""}
				}
				for _, name := range names {
					reads = append(reads, observedRead{group: grant.Rule.APIGroups[0], resource: resource, verb: verb, name: name})
				}
			}
		}
	}
	for kind := range expected {
		path := "/api/" + kind.Version
		if kind.Group != "" {
			path = "/apis/" + kind.Group + "/" + kind.Version
		}
		reads = append(reads, observedRead{verb: "get", discoveryURL: path})
	}
	result.DeclarationReason = "DeclaredReadsCovered"
	declaration := "requestedReads covers locally known defaults and observed reads; unresolved mappings and policy overrides still require review."
	if len(d.Spec.RequestedReads) == 0 {
		result.DeclarationReason = "RequestedReadsOmitted"
		declaration = "requestedReads was omitted; required reads are not fully declared."
	} else {
		for _, read := range reads {
			if !declaresRead(d.Spec.RequestedReads, read) {
				result.DeclarationReason = "RequestedReadsIncomplete"
				declaration = "requestedReads omits known evaluator, helper, discovery or observed reads."
				break
			}
		}
		if result.DeclarationReason == "DeclaredReadsCovered" && unresolvedReads(d) {
			result.DeclarationReason = "RequestedReadsUnresolved"
			declaration = "Some evaluator resource mappings or override names cannot be fully checked locally; review requestedReads against actual targets."
		}
	}
	result.Message += " This does not establish effective grants or future access. " + declaration
	return result, nil
}

func unresolvedReads(d *api.AddonDefinition) bool {
	for _, family := range d.Spec.Families {
		for _, c := range family.Checks {
			switch c.Kind {
			case "Field", "Condition", "AnnotationStaleness":
				return true // Arbitrary GVKs require live discovery, never guessed plurals.
			case "Workload":
				if c.Workload.NameThresholdKey != "" {
					return true
				}
			case "Webhook":
				if c.Webhook.NameThresholdKey != "" {
					return true
				}
			case "CronJob":
				if c.CronJob.NameThresholdKey != "" {
					return true
				}
			case "ConfigMap":
				if c.ConfigMap.NameThresholdKey != "" {
					return true
				}
			}
		}
	}
	return false
}

func declaresRead(rules []api.DefinitionReadRule, read observedRead) bool {
	for _, rule := range rules {
		if !slices.Contains(rule.Verbs, read.verb) {
			continue
		}
		if read.discoveryURL != "" {
			if slices.Contains(rule.NonResourceURLs, read.discoveryURL) {
				return true
			}
			continue
		}
		if rule.APIGroup == nil || *rule.APIGroup != read.group || !slices.Contains(rule.Resources, read.resource) {
			continue
		}
		// Unfiltered list requests cannot be covered by a resourceNames restriction.
		if len(rule.ResourceNames) == 0 || (read.name != "" && slices.Contains(rule.ResourceNames, api.DefinitionResourceName(read.name))) {
			return true
		}
	}
	return false
}
