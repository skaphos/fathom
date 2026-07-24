/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package controller_test hosts the operator-RBAC documentation guard. It is a
// plain file-contract test (no envtest, no controller internals): every rule in
// the generated operator ClusterRole must carry a justification row in
// docs/reference/operator-rbac.md, and every row must still correspond to a
// live rule — the same lockstep doctrine the addon-RBAC generator enforces for
// docs/reference/rbac.md (#153).
package controller_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

const (
	operatorRolePath  = "../../config/rbac/role.yaml"
	operatorRBACDoc   = "../../docs/reference/operator-rbac.md"
	docTableBeginMark = "<!-- operator-clusterrole-table:begin -->"
	docTableEndMark   = "<!-- operator-clusterrole-table:end -->"

	// minJustificationLen rejects placeholder justifications ("needed", "TODO")
	// without judging prose quality; real defensive justifications are long.
	minJustificationLen = 80
)

// docRow is one parsed justification-table row, keyed the same way rules are.
type docRow struct {
	key           string
	verbs         []string
	justification string
	line          int
}

func TestOperatorClusterRoleRulesAreJustifiedInDoc(t *testing.T) {
	role := loadOperatorClusterRole(t)
	rows := loadJustificationRows(t)

	rowsByKey := make(map[string]docRow, len(rows))
	for _, row := range rows {
		if dup, ok := rowsByKey[row.key]; ok {
			t.Fatalf("%s: rows at lines %d and %d document the same rule (%s); merge them", operatorRBACDoc, dup.line, row.line, row.key)
		}
		rowsByKey[row.key] = row
	}

	seen := make(map[string]bool, len(role.Rules))
	for _, rule := range role.Rules {
		key := ruleKey(rule.APIGroups, rule.Resources)
		seen[key] = true
		row, ok := rowsByKey[key]
		if !ok {
			t.Errorf("rule {groups: %v, resources: %v} in %s has no justification row in %s — every operator grant must be defended (see the table between the %q markers)",
				rule.APIGroups, rule.Resources, operatorRolePath, operatorRBACDoc, docTableBeginMark)
			continue
		}
		if got, want := normalizeSet(row.verbs), normalizeSet(rule.Verbs); !equalStrings(got, want) {
			t.Errorf("%s line %d (%s): documented verbs %v do not match the generated rule's verbs %v — update the row (and its justification) to match config/rbac/role.yaml",
				operatorRBACDoc, row.line, key, got, want)
		}
		if len(row.justification) < minJustificationLen {
			t.Errorf("%s line %d (%s): justification is too short to be defensive — explain why the grant is needed and why a narrower one would not suffice", operatorRBACDoc, row.line, key)
		}
	}

	for _, row := range rows {
		if !seen[row.key] {
			t.Errorf("%s line %d documents rule %s which no longer exists in %s — remove or update the stale row", operatorRBACDoc, row.line, row.key, operatorRolePath)
		}
	}
}

func loadOperatorClusterRole(t *testing.T) *rbacv1.ClusterRole {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(operatorRolePath))
	if err != nil {
		t.Fatalf("read %s: %v", operatorRolePath, err)
	}
	role := &rbacv1.ClusterRole{}
	if err := yaml.UnmarshalStrict(raw, role); err != nil {
		t.Fatalf("parse %s: %v", operatorRolePath, err)
	}
	if len(role.Rules) == 0 {
		t.Fatalf("%s: no rules parsed; the guard would vacuously pass", operatorRolePath)
	}
	return role
}

func loadJustificationRows(t *testing.T) []docRow {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(operatorRBACDoc))
	if err != nil {
		t.Fatalf("read %s: %v (the operator RBAC justification doc is mandatory)", operatorRBACDoc, err)
	}
	lines := strings.Split(string(raw), "\n")

	begin, end := -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case docTableBeginMark:
			begin = i
		case docTableEndMark:
			end = i
		}
	}
	if begin == -1 || end == -1 || end <= begin {
		t.Fatalf("%s: missing %q/%q markers around the justification table", operatorRBACDoc, docTableBeginMark, docTableEndMark)
	}

	var rows []docRow
	for i := begin + 1; i < end; i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := splitTableRow(line)
		if len(cells) != 4 {
			continue
		}
		// Skip the header and its separator row.
		if cells[0] == "API group" || strings.HasPrefix(cells[0], "---") {
			continue
		}
		rows = append(rows, docRow{
			key:           ruleKey(splitList(cells[0]), splitList(cells[1])),
			verbs:         splitList(cells[2]),
			justification: cells[3],
			line:          i + 1,
		})
	}
	if len(rows) == 0 {
		t.Fatalf("%s: no justification rows parsed between the table markers", operatorRBACDoc)
	}
	return rows
}

// splitTableRow returns the trimmed cells of a markdown table row.
func splitTableRow(line string) []string {
	parts := strings.Split(strings.Trim(line, "|"), "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells
}

// splitList parses a comma-separated doc cell. API groups stay in display form
// ("core" for the empty group, matching docs/reference/rbac.md); ruleKey
// normalizes the manifest side to the same form.
func splitList(cell string) []string {
	var out []string
	for _, item := range strings.Split(cell, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// ruleKey identifies a rule by its sorted API groups and resources, the same
// granularity controller-gen emits rules at.
func ruleKey(groups, resources []string) string {
	display := func(group string) string {
		if group == "" {
			return "core"
		}
		return group
	}
	g := make([]string, 0, len(groups))
	for _, group := range groups {
		g = append(g, display(group))
	}
	r := normalizeSet(resources)
	g = normalizeSet(g)
	return strings.Join(g, ",") + " / " + strings.Join(r, ",")
}

func normalizeSet(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
