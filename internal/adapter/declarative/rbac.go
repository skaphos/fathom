/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// RBAC for the declarative adapters. controller-gen aggregates these markers into
// config/rbac/role.yaml; they are the union of the reads every declarative
// AddonDefinition performs (the RBACRule field on each definition is
// documentation, not enforced). They live here at package level -- one scanned
// location owning the declarative reads -- rather than on each engine
// constructor: controller-gen did not reliably collect the per-constructor
// markers, and a single package-level home is clearer regardless.
//
// +kubebuilder:rbac:groups=apps,resources=deployments;daemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=external-secrets.io,resources=externalsecrets,verbs=get;list;watch
package declarative
