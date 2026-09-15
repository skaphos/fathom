/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodehealth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/skaphos/fathom/internal/nodecert"
)

// The wire contract reuses nodecert's label keys, node-name annotation, and
// report data key verbatim: the report-authenticity ValidatingAdmissionPolicy
// selects ConfigMaps by managed-by alone and binds the same annotation, so a
// NodeHealthCheck report is authenticated by the same policy with no second,
// drifting copy (#206, SEC-1). Only the source-kind value and the ConfigMap
// name differ.
const (
	// KindNodeHealthCheck is the nodecert.LabelSourceKind value for this check.
	KindNodeHealthCheck = "NodeHealthCheck"

	// kindSlug qualifies the report ConfigMap name so a NodeHealthCheck and a
	// NodeCertificateCheck sharing a name in one namespace never collide.
	kindSlug = "nodehealth"
)

// ReportConfigMapName returns the deterministic, DNS-1123-subdomain name a
// node-agent uses for its per-node NodeHealthCheck report ConfigMap. It is the
// canonical name VerifyReportBinding requires, so an off-name report is
// rejected as a competing claim on the node.
func ReportConfigMapName(checkName, node string) string {
	return nodecert.NodeReportConfigMapNameFor(kindSlug, checkName, node)
}

// EncodeReport serializes a NodeReport to the JSON stored in a report ConfigMap.
func EncodeReport(r NodeReport) (string, error) {
	out, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("encode node health report: %w", err)
	}
	return string(out), nil
}

// DecodeReport parses the JSON stored under nodecert.ConfigMapReportKey back
// into a NodeReport.
func DecodeReport(data string) (NodeReport, error) {
	var r NodeReport
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return NodeReport{}, fmt.Errorf("decode node health report: %w", err)
	}
	return r, nil
}

// EncodeItems serializes the resolved check items for the agent's --checks
// argument. JSON rather than a bespoke CSV: the items carry per-type fields and
// a hand-rolled format would have to grow with every new type.
func EncodeItems(items []Item) (string, error) {
	out, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("encode node health items: %w", err)
	}
	return string(out), nil
}

// DecodeItems parses the agent's --checks argument.
func DecodeItems(data string) ([]Item, error) {
	var items []Item
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		return nil, fmt.Errorf("decode node health items: %w", err)
	}
	return items, nil
}

// ItemKey identifies what an item measures, matching CheckResult.Path for the
// result the agent emits: the filesystem path for headroom, the socket for
// ContainerRuntime, empty for KubeletHealthz. It is the item's identity
// together with Type, and the sort key that keeps the agent arguments stable.
func ItemKey(it Item) string {
	if it.Type == TypeContainerRuntime {
		return it.SocketPath
	}
	return it.Path
}

// itemsDigestLength truncates the digest to 32 hex characters (128 bits):
// short enough for a report field, collision-safe for one check's item set.
const itemsDigestLength = 32

// ItemsDigest is the identity of an agent-side item set: a short hex SHA-256
// of the agent-evaluated items in canonical order, thresholds included. The
// operator computes it from the spec's resolved items and the agent from the
// items it was started with; the two agree only when the report was
// evaluated against exactly the current spec semantics. NodeCondition items
// are excluded because the operator, not the agent, grades them, and order
// is irrelevant: items are sorted by (type, key) before hashing.
func ItemsDigest(items []Item) string {
	canonical := make([]Item, 0, len(items))
	for _, it := range items {
		if it.AgentEvaluated() {
			canonical = append(canonical, it)
		}
	}
	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].Type != canonical[j].Type {
			return canonical[i].Type < canonical[j].Type
		}
		return ItemKey(canonical[i]) < ItemKey(canonical[j])
	})
	raw, err := json.Marshal(canonical)
	if err != nil {
		// Item always marshals; on the impossible error an empty digest never
		// matches, so the report is left unconsumed rather than wrongly trusted.
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:itemsDigestLength]
}

// ReportCovers reports whether report carries exactly one result for each
// agent-side item in items and nothing else: the report's (type, key) set
// must equal the items'. A superset is not enough — a report written before
// an item was removed still carries that item's result, and forwarding it
// would let a removed (possibly failing) check keep shaping the verdict of a
// spec that no longer declares it. Items the agent does not evaluate
// (NodeCondition) are ignored. An empty item set is covered only by a report
// with no checks.
func ReportCovers(report NodeReport, items []Item) bool {
	want := make(map[string]struct{}, len(items))
	for _, it := range items {
		if it.AgentEvaluated() {
			want[it.Type+"\x00"+ItemKey(it)] = struct{}{}
		}
	}
	if len(report.Checks) != len(want) {
		return false
	}
	for _, c := range report.Checks {
		if _, ok := want[c.Type+"\x00"+c.Path]; !ok {
			return false
		}
	}
	return true
}

// VerifyReportBinding applies the structural authenticity bindings shared with
// NodeCertificateCheck (nodecert.VerifyReportIdentity) to a decoded
// NodeHealthCheck report, using this kind's canonical ConfigMap name. It is
// corroboration layered on the admission policy, not authentication — see the
// package comment on nodecert/authenticity.go.
func VerifyReportBinding(cmName, annotatedNode, checkName string, report NodeReport) nodecert.ReportRejection {
	return nodecert.VerifyReportIdentity(cmName, annotatedNode, checkName, report.CheckName, report.Node, ReportConfigMapName(checkName, report.Node))
}
