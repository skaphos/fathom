/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"time"

	"github.com/skaphos/fathom/pkg/adapter"
)

// KuredDefinition is the declarative kured adapter (SKA-512). kured (KUbernetes
// REboot Daemon) drains and reboots nodes when a reboot-required sentinel
// appears, serializing reboots behind a cluster lock. Two families cover it:
//
//   - system_health — the kured DaemonSet and its pods are Ready.
//   - reboot_state — the reboot lock is not wedged, and no node has been waiting
//     on a reboot window too long. kured stores the lock as a JSON annotation
//     (weave.works/kured-node-lock) on its own DaemonSet with a "created"
//     timestamp; a lock held past the freshness window means reboots have stopped
//     progressing (Warn). When kured is run with --annotate-nodes it stamps each
//     node needing a reboot with weave.works/kured-most-recent-reboot-needed; a
//     node carrying that annotation past the window has waited too long (Warn).
//     Nodes without the annotation are not emitted, and a cluster with no pending
//     reboots yields a Skipped node scan.
//
// kured is Optional: on a cluster that does not run it every NotFound target
// scores Skipped with the adapter.DetailAbsent marker, and — because the lock
// annotation is absent on a healthy idle cluster — the lock check passes
// quietly. Node-level reboot detection only surfaces when --annotate-nodes is
// enabled; without it the node scan is Skipped, which is the honest signal that
// the data is not published rather than a false all-clear.
var KuredDefinition = AddonDefinition{
	AddonType:      "kured",
	AdapterVersion: "0.1.0",
	Optional:       true,
	// Detect the installed kured version off the DaemonSet (its
	// app.kubernetes.io/version label, else the ghcr.io/kubereboot/kured image
	// tag). Detection-only: SupportedVersions is left empty so kured never Warns
	// on a version — gating is opt-in (SKA-527).
	VersionSource: &VersionSource{FromFamily: adapter.Family("system_health")},
	RBAC: []adapter.PolicyRule{
		{APIGroups: []string{"apps"}, Resources: []string{"daemonsets"}, Verbs: []string{"get"},
			Justification: "Get the kured DaemonSet by name to score readiness and to read its reboot-lock annotation. get only — both the WorkloadCheck and the named lock check fetch the DaemonSet by name and the impersonating client is cache-free, so no list/watch; read-only."},
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"},
			Justification: "List the kured Pods by label selector for restart counts and readiness behind the DaemonSet. list (not get) because Pod names are dynamic; read-only."},
		{APIGroups: []string{""}, Resources: []string{"nodes"}, Verbs: []string{"list"},
			Justification: "List Nodes to read the weave.works/kured-most-recent-reboot-needed annotation and surface nodes waiting too long on a reboot. list only — the node check lists all nodes and never Gets one by name; read-only, and only node metadata annotations are inspected."},
	},
	Families: []FamilyDefinition{
		{
			Name:           adapter.Family("system_health"),
			DefaultEnabled: true,
			Workloads: []WorkloadCheck{{
				Kind:                    KindDaemonSet,
				DefaultNamespace:        "kube-system",
				NameThresholdKey:        "daemonSetName",
				DefaultName:             "kured",
				Component:               "kured",
				CheckPods:               true,
				RestartWarnThresholdKey: "restartWarnCount",
				DefaultRestartWarn:      3,
			}},
		},
		{
			Name:           adapter.Family("reboot_state"),
			DefaultEnabled: true,
			Annotations: []AnnotationStalenessCheck{
				{
					// The reboot lock: a JSON annotation on the kured DaemonSet
					// whose "created" field times the lock's acquisition. Held past
					// the window -> the reboot pipeline is wedged.
					APIVersion:         "apps/v1",
					Kind:               "DaemonSet",
					DefaultNamespace:   "kube-system",
					NameThresholdKey:   "daemonSetName",
					DefaultName:        "kured",
					Component:          "kured",
					AnnotationKey:      "weave.works/kured-node-lock",
					TimestampJSONField: "created",
					MaxAgeThresholdKey: "lockMaxAge",
					DefaultMaxAge:      time.Hour,
					StaleOutcome:       adapter.OutcomeWarn,
				},
				{
					// Per-node reboot-required stamp (only present with
					// --annotate-nodes). A node carrying it past the window has
					// waited too long for its reboot slot.
					APIVersion:         "v1",
					Kind:               "Node",
					ListKind:           "NodeList",
					ListName:           "nodes",
					ClusterScoped:      true,
					Component:          "kured",
					AnnotationKey:      "weave.works/kured-most-recent-reboot-needed",
					MaxAgeThresholdKey: "rebootPendingMaxAge",
					DefaultMaxAge:      24 * time.Hour,
					StaleOutcome:       adapter.OutcomeWarn,
				},
			},
		},
	},
}

// NewKuredEngine returns the declarative kured adapter. RBAC markers live on the
// package doc in definition.go.
func NewKuredEngine() *Engine {
	return MustEngine(KuredDefinition)
}
