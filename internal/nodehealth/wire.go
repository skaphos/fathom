/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodehealth

import (
	"encoding/json"
	"fmt"

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

// VerifyReportBinding applies the structural authenticity bindings shared with
// NodeCertificateCheck (nodecert.VerifyReportIdentity) to a decoded
// NodeHealthCheck report, using this kind's canonical ConfigMap name. It is
// corroboration layered on the admission policy, not authentication — see the
// package comment on nodecert/authenticity.go.
func VerifyReportBinding(cmName, annotatedNode, checkName string, report NodeReport) nodecert.ReportRejection {
	return nodecert.VerifyReportIdentity(cmName, annotatedNode, checkName, report.CheckName, report.Node, ReportConfigMapName(checkName, report.Node))
}
