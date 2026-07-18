/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodecert

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// Label keys and values shared by the node-agent (writer) and the
// NodeCertificateCheck controller (reader) on per-node report ConfigMaps. The
// source-kind/source-name pair matches the scheme HealthReports already use, so
// reports trace back to their NodeCertificateCheck uniformly.
const (
	LabelManagedBy  = "fathom.skaphos.io/managed-by"
	LabelSourceKind = "fathom.skaphos.io/source-kind"
	LabelSourceName = "fathom.skaphos.io/source-name"
	LabelNode       = "fathom.skaphos.io/node"

	// ManagedByValue is the value of LabelManagedBy on Fathom-owned objects.
	ManagedByValue = "fathom"
	// KindNodeCertificateCheck is the LabelSourceKind value for this check.
	KindNodeCertificateCheck = "NodeCertificateCheck"

	// ConfigMapReportKey is the data key under which the JSON-encoded NodeReport
	// is stored in a per-node report ConfigMap.
	ConfigMapReportKey = "report.json"
)

// EncodeReport serializes a NodeReport to the JSON stored in a report ConfigMap.
func EncodeReport(r NodeReport) (string, error) {
	out, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("encode node report: %w", err)
	}
	return string(out), nil
}

// DecodeReport parses the JSON stored under ConfigMapReportKey back into a
// NodeReport.
func DecodeReport(data string) (NodeReport, error) {
	var r NodeReport
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return NodeReport{}, fmt.Errorf("decode node report: %w", err)
	}
	return r, nil
}

// NodeReportConfigMapName returns the deterministic, DNS-1123-subdomain name a
// node-agent uses for its per-node report ConfigMap. It combines the check
// name, a sanitized node name, and a short hash of the raw node name so the
// name is stable per (check, node), unique across nodes even after
// sanitization, and always a legal object name (<=253 chars). The operator does
// not need this name — it discovers report ConfigMaps by label — but a
// deterministic name lets the agent upsert the same object every scan.
func NodeReportConfigMapName(checkName, node string) string {
	h := sha256.Sum256([]byte(node))
	suffix := hex.EncodeToString(h[:])[:8]

	base := dnsSafe(checkName)
	if base == "" {
		base = "nodecertificatecheck"
	}
	if len(base) > 200 {
		base = strings.Trim(base[:200], "-.")
	}

	name := base + "-" + suffix
	if nodePart := dnsSafe(node); nodePart != "" {
		if candidate := base + "-" + nodePart + "-" + suffix; len(candidate) <= 253 {
			name = candidate
		}
	}
	return name
}

// dnsSafe lowercases s and maps anything outside [a-z0-9.-] to '-', then trims
// leading/trailing punctuation so the result is a legal DNS-1123 subdomain
// fragment. Returns "" if nothing usable remains.
func dnsSafe(s string) string {
	mapped := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, s)
	return strings.Trim(mapped, "-.")
}
