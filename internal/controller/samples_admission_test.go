/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8syaml "sigs.k8s.io/yaml"
)

// Every shipped sample manifest must admit against the real CRD schemas
// (FR-008, spec 003): tightening validation — cadence floors, policy bounds —
// must never reject a configuration we document as correct. New samples are
// picked up automatically.
var _ = Describe("shipped sample manifests", func() {
	It("all admit unchanged", func() {
		dir := filepath.Join("..", "..", "config", "samples")
		entries, err := os.ReadDir(dir)
		Expect(err).NotTo(HaveOccurred())

		checked := 0
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || name == "kustomization.yaml" || !strings.HasSuffix(name, ".yaml") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, name))
			Expect(err).NotTo(HaveOccurred(), name)

			var obj unstructured.Unstructured
			Expect(k8syaml.Unmarshal(raw, &obj)).To(Succeed(), name)
			// Namespaced samples need a namespace to admit; on cluster-scoped
			// kinds (ClusterHealth) this is harmless — the API server's
			// BeforeCreate clears metadata.namespace for cluster-scoped
			// resources before validation.
			obj.SetNamespace("default")
			// Server-side dry run: full admission (schema + CEL) with no
			// persisted object, so samples cannot collide across specs.
			Expect(k8sClient.Create(ctx, &obj, client.DryRunAll)).To(Succeed(),
				"sample %s must admit against the generated CRDs", name)
			checked++
		}
		Expect(checked).To(BeNumerically(">", 0), "no samples found — wrong directory?")
	})
})
