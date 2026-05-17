/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package crdutil provides shared helpers for adapters that read
// CustomResourceDefinition objects to validate addon health. Adapters
// express their version compatibility as a descending-preference slice
// (e.g. {"v1", "v1beta1"}) and consume PreferredServedVersion to pick
// the served version the cluster actually exposes.
package crdutil

import (
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// Established reports whether the CRD carries the Established condition
// with status True. A CRD is "established" once the apiserver has
// accepted its schema and is serving requests against it.
func Established(crd *apixv1.CustomResourceDefinition) bool {
	for _, condition := range crd.Status.Conditions {
		if condition.Type == apixv1.Established {
			return condition.Status == apixv1.ConditionTrue
		}
	}
	return false
}

// PreferredServedVersion returns the highest-priority entry in preference
// that the CRD actually serves. preference is a descending-preference
// slice declared by the adapter (e.g. {"v1", "v1beta1"}); the function
// scans the CRD's Spec.Versions for Served=true entries and returns the
// first preference that matches.
//
// The second return is false when no preferred version is served — the
// caller decides whether that is a Fail (strict adapter) or a Warn
// (tolerant adapter).
func PreferredServedVersion(crd *apixv1.CustomResourceDefinition, preference []string) (string, bool) {
	served := make(map[string]bool, len(crd.Spec.Versions))
	for _, v := range crd.Spec.Versions {
		if v.Served {
			served[v.Name] = true
		}
	}
	for _, candidate := range preference {
		if served[candidate] {
			return candidate, true
		}
	}
	return "", false
}
