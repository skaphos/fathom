/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1_test

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// The runtime definition admission tests run against a real API server
// because that is the only thing that evaluates CEL and the structural
// schema. This harness mirrors the one in api/v1 (which moved there with the
// check kinds); the two packages start their own environments.

var (
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestMain(m *testing.M) {
	if err := fathomv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		panic("add fathom v1alpha1 scheme: " + err.Error())
	}

	// Most tests in this package are static and need no API server, so the
	// environment starts only when assets are available and the tests that
	// need one skip loudly otherwise. If assets ARE present, a failure to
	// start is fatal: a broken environment, not an absent one.
	if envTestAssetsAvailable() {
		testEnv = &envtest.Environment{
			CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
			ErrorIfCRDPathMissing: true,
		}
		if dir := firstEnvTestBinaryDir(); dir != "" {
			testEnv.BinaryAssetsDirectory = dir
		}

		var err error
		cfg, err = testEnv.Start()
		if err != nil {
			panic("start envtest: " + err.Error())
		}
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		if err != nil {
			_ = testEnv.Stop()
			panic("build client: " + err.Error())
		}
	}

	code := m.Run()
	if testEnv != nil {
		_ = testEnv.Stop()
	}
	os.Exit(code)
}

// envTestAssetsAvailable reports whether an API server can be started: either
// the task wrapper exported KUBEBUILDER_ASSETS, or setup-envtest left binaries
// in bin/k8s for an IDE run.
func envTestAssetsAvailable() bool {
	return os.Getenv("KUBEBUILDER_ASSETS") != "" || firstEnvTestBinaryDir() != ""
}

// requireAPIServer skips a test that cannot run without envtest. CI always has
// the assets, so this never silently drops coverage there.
func requireAPIServer(t *testing.T) {
	t.Helper()
	if k8sClient == nil {
		t.Skip("envtest assets unavailable; run via `go -C tools tool task test` to exercise admission validation")
	}
}

func firstEnvTestBinaryDir() string {
	entries, err := os.ReadDir(filepath.Join("..", "..", "bin", "k8s"))
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join("..", "..", "bin", "k8s", entry.Name())
		}
	}
	return ""
}
