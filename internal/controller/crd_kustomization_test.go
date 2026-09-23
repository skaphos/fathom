/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// The CRD kustomization guard is a plain file-contract test (no envtest).
// controller-gen writes config/crd/bases, but config/crd/kustomization.yaml is
// hand-maintained, and nothing else compares them: verify-generated only diffs
// generator output, envtest loads bases/ directly, and helm:sync copies the
// whole directory. A base missing from the kustomization therefore ships in the
// Helm chart while being silently absent from every kustomize install (#298).
package controller_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"sigs.k8s.io/yaml"
)

const crdKustomizeDir = "../../config/crd"

func TestCRDKustomizationListsEveryBase(t *testing.T) {
	unlisted, dangling, err := crdKustomizationDrift(crdKustomizeDir)
	if err != nil {
		t.Fatal(err)
	}
	kustomization := filepath.Join(crdKustomizeDir, "kustomization.yaml")
	for _, entry := range unlisted {
		t.Errorf("CRD base %s is not listed in %s — add %q to its resources: list, or kustomize installs (task install / task deploy) will not create the CRD",
			filepath.Join(crdKustomizeDir, entry), kustomization, entry)
	}
	for _, entry := range dangling {
		t.Errorf("%s lists %q in resources:, but %s does not exist — remove or rename the stale entry",
			kustomization, entry, filepath.Join(crdKustomizeDir, entry))
	}
}

func TestCRDKustomizationDriftDetection(t *testing.T) {
	tests := []struct {
		name         string
		bases        []string
		resources    []string
		wantUnlisted []string
		wantDangling []string
	}{
		{
			name:      "in sync",
			bases:     []string{"a.yaml", "b.yaml"},
			resources: []string{"bases/a.yaml", "bases/b.yaml"},
		},
		{
			name:         "base added without a kustomization entry",
			bases:        []string{"a.yaml", "b.yaml"},
			resources:    []string{"bases/a.yaml"},
			wantUnlisted: []string{"bases/b.yaml"},
		},
		{
			name:         "entry points at a missing file",
			bases:        []string{"a.yaml"},
			resources:    []string{"bases/a.yaml", "bases/renamed.yaml"},
			wantDangling: []string{"bases/renamed.yaml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "bases"), 0o755); err != nil {
				t.Fatal(err)
			}
			for _, base := range tt.bases {
				if err := os.WriteFile(filepath.Join(dir, "bases", base), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			kustomization, err := yaml.Marshal(map[string][]string{"resources": tt.resources})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "kustomization.yaml"), kustomization, 0o644); err != nil {
				t.Fatal(err)
			}

			unlisted, dangling, err := crdKustomizationDrift(dir)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(unlisted, tt.wantUnlisted) {
				t.Errorf("unlisted = %v, want %v", unlisted, tt.wantUnlisted)
			}
			if !slices.Equal(dangling, tt.wantDangling) {
				t.Errorf("dangling = %v, want %v", dangling, tt.wantDangling)
			}
		})
	}
}

// crdKustomizationDrift compares dir/bases/*.yaml against the resources: list
// of dir/kustomization.yaml. It returns the bases no entry lists and the
// entries that resolve to no file, both as dir-relative paths in sorted order.
func crdKustomizationDrift(dir string) (unlisted, dangling []string, err error) {
	raw, err := os.ReadFile(filepath.Join(dir, "kustomization.yaml"))
	if err != nil {
		return nil, nil, err
	}
	var kustomization struct {
		Resources []string `json:"resources"`
	}
	if err := yaml.Unmarshal(raw, &kustomization); err != nil {
		return nil, nil, err
	}

	listed := make(map[string]bool, len(kustomization.Resources))
	for _, entry := range kustomization.Resources {
		listed[filepath.Clean(entry)] = true
		if _, statErr := os.Stat(filepath.Join(dir, entry)); statErr != nil {
			dangling = append(dangling, entry)
		}
	}

	bases, err := filepath.Glob(filepath.Join(dir, "bases", "*.yaml"))
	if err != nil {
		return nil, nil, err
	}
	for _, base := range bases {
		entry, relErr := filepath.Rel(dir, base)
		if relErr != nil {
			return nil, nil, relErr
		}
		if !listed[entry] {
			unlisted = append(unlisted, entry)
		}
	}

	slices.Sort(unlisted)
	slices.Sort(dangling)
	return unlisted, dangling, nil
}
