/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package cli builds the fathomctl command tree. It is the unit-testable seam
// between cmd/fathomctl/main.go and the Kubernetes API, playing the role
// internal/app plays for the operator binary.
//
// Configuration is deliberately small: the global flags plus the standard
// kubeconfig discovery (--kubeconfig, then KUBECONFIG, then ~/.kube/config,
// then in-cluster). There is no config file and no FATHOM_* environment
// mapping. A client tool run from a workstation or a CI job has no ConfigMap
// to mount, and duplicating the operator's viper plumbing for a handful of
// flags would add a second configuration style with no consumer. This
// deviation from the operator's cobra+viper model is recorded in the feature
// plan's Complexity Tracking (specs/009-fathomctl-cli/plan.md).
//
// Test seams are unexported function fields on factory (kubeconfig loader,
// client constructor, and later the poll interval and prompt input), so every
// verb is tested in-package over the controller-runtime fake client without a
// kubeconfig or an API server.
//
// The package must stay free of imports from internal/app, internal/adapter,
// and internal/controller so the client binary links only the API types and
// the client libraries.
package cli
