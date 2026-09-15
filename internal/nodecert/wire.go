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

	// AnnotationNodeName carries the raw, unsanitized node name on a per-node
	// report ConfigMap. Unlike LabelNode (coerced into a valid label value, and
	// therefore lossy for long or non-conforming node names), this annotation
	// holds the exact node name. It is the authenticity anchor for a report:
	//   - the report-authenticity ValidatingAdmissionPolicy compares it to the
	//     writing agent's ServiceAccount-token node claim
	//     (authentication.kubernetes.io/node-name) at admission time, so a
	//     node-agent can only publish a report attributed to its own node; and
	//   - the controller re-checks it equals the report payload's Node field at
	//     collection time as defense-in-depth on clusters where the policy is not
	//     enforced.
	AnnotationNodeName = "fathom.skaphos.io/node-name"

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
// name, a sanitized node name, and a short hash so the name is stable per
// (check, node), unique across nodes even after sanitization, and always a
// legal object name (<=253 chars). The operator does not need this name — it
// discovers report ConfigMaps by label — but a deterministic name lets the
// agent upsert the same object every scan, and it is the canonical name the
// authenticity bindings require.
//
// NodeCertificateCheck hashes the node alone, unchanged from the first
// release so reports already on clusters keep their names.
func NodeReportConfigMapName(checkName, node string) string {
	return reportConfigMapName(checkName, "nodecertificatecheck", node, node)
}

// NodeReportConfigMapNameFor returns the deterministic report ConfigMap name
// for a node-scoped kind other than NodeCertificateCheck. The kind is folded
// into both the readable base and the hash: a base prefix alone is not a
// namespace — a NodeHealthCheck named "foo" and a NodeCertificateCheck named
// "nodehealth-foo" would share every per-node name — whereas hashing
// (kind, check, node) makes the two kinds' names disjoint for any check
// names, while NodeCertificateCheck keeps its unqualified, node-only hash.
func NodeReportConfigMapNameFor(kindSlug, checkName, node string) string {
	return reportConfigMapName(kindSlug+"-"+checkName, kindSlug, node, kindSlug+"\x00"+checkName+"\x00"+node)
}

// reportConfigMapName builds "<base>-<node>-<hash>" (or "<base>-<hash>" when
// the sanitized node would push the name past 253 characters), hashing seed.
func reportConfigMapName(rawBase, fallback, node, seed string) string {
	h := sha256.Sum256([]byte(seed))
	suffix := hex.EncodeToString(h[:])[:8]

	base := dnsSafe(rawBase)
	if base == "" {
		base = fallback
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
