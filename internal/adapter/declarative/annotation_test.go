/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/skaphos/fathom/pkg/adapter"
)

func daemonSetWithAnnotations(name, namespace string, annotations map[string]string) *appsv1.DaemonSet {
	ds := daemonSetInNamespace(name, namespace, 1)
	ds.Annotations = annotations
	return ds
}

func nodeWithAnnotations(name string, annotations map[string]string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations}}
}

// lockJSON builds a kured-style lock annotation value carrying a "created"
// timestamp.
func lockJSON(created time.Time) string {
	return fmt.Sprintf(`{"nodeID":"node1","created":%q,"TTL":0}`, created.UTC().Format(time.RFC3339))
}

func runAnnotation(t *testing.T, ac AnnotationStalenessCheck, objs ...clientObject) []adapter.CheckResult {
	t.Helper()
	eng := MustEngine(AddonDefinition{
		AddonType:      "annotationtest",
		AdapterVersion: "0.0.1",
		Optional:       true,
		Families:       []FamilyDefinition{{Name: "state", DefaultEnabled: true, Annotations: []AnnotationStalenessCheck{ac}}},
	})
	res, err := eng.Run(context.Background(), adapter.Request{Client: newFakeClient(t, objs...), Logger: logr.Discard()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res.Checks
}

// lockCheck is the named-mode kured lock check (JSON "created" field on the DS).
func lockCheck() AnnotationStalenessCheck {
	return AnnotationStalenessCheck{
		APIVersion:         "apps/v1",
		Kind:               "DaemonSet",
		DefaultNamespace:   "kube-system",
		DefaultName:        "kured",
		Component:          "kured",
		AnnotationKey:      "weave.works/kured-node-lock",
		TimestampJSONField: "created",
		DefaultMaxAge:      time.Hour,
		StaleOutcome:       adapter.OutcomeWarn,
	}
}

func TestAnnotationStaleness_NamedLock(t *testing.T) {
	t.Run("absent annotation passes (lock free)", func(t *testing.T) {
		checks := runAnnotation(t, lockCheck(), daemonSetWithAnnotations("kured", "kube-system", nil))
		assertHasOutcome(t, checks, "DaemonSet", "kured", adapter.OutcomePass, "absent")
	})
	t.Run("fresh lock passes", func(t *testing.T) {
		lock := map[string]string{"weave.works/kured-node-lock": lockJSON(time.Now().Add(-2 * time.Minute))}
		checks := runAnnotation(t, lockCheck(), daemonSetWithAnnotations("kured", "kube-system", lock))
		assertHasOutcome(t, checks, "DaemonSet", "kured", adapter.OutcomePass, "within the freshness window")
	})
	t.Run("wedged lock warns", func(t *testing.T) {
		lock := map[string]string{"weave.works/kured-node-lock": lockJSON(time.Now().Add(-3 * time.Hour))}
		checks := runAnnotation(t, lockCheck(), daemonSetWithAnnotations("kured", "kube-system", lock))
		assertHasOutcome(t, checks, "DaemonSet", "kured", adapter.OutcomeWarn, "older than the freshness window")
	})
	t.Run("future lock timestamp warns", func(t *testing.T) {
		lock := map[string]string{"weave.works/kured-node-lock": lockJSON(time.Now().Add(1 * time.Hour))}
		checks := runAnnotation(t, lockCheck(), daemonSetWithAnnotations("kured", "kube-system", lock))
		assertHasOutcome(t, checks, "DaemonSet", "kured", adapter.OutcomeWarn, "in the future")
	})
	t.Run("unparseable lock warns", func(t *testing.T) {
		lock := map[string]string{"weave.works/kured-node-lock": "not-json"}
		checks := runAnnotation(t, lockCheck(), daemonSetWithAnnotations("kured", "kube-system", lock))
		assertHasOutcome(t, checks, "DaemonSet", "kured", adapter.OutcomeWarn, "unreadable")
	})
	t.Run("absent daemonset inherits Optional skip", func(t *testing.T) {
		checks := runAnnotation(t, lockCheck())
		assertHasOutcome(t, checks, "DaemonSet", "kured", adapter.OutcomeSkipped, "not found")
		assertHasDetail(t, checks, "DaemonSet", "kured", adapter.DetailAbsent, "true")
	})
}

// nodeRebootCheck is the list-mode node reboot-needed check (bare RFC3339 value).
func nodeRebootCheck() AnnotationStalenessCheck {
	return AnnotationStalenessCheck{
		APIVersion:    "v1",
		Kind:          "Node",
		ListKind:      "NodeList",
		ListName:      "nodes",
		ClusterScoped: true,
		Component:     "kured",
		AnnotationKey: "weave.works/kured-most-recent-reboot-needed",
		DefaultMaxAge: 24 * time.Hour,
		StaleOutcome:  adapter.OutcomeWarn,
	}
}

func TestAnnotationStaleness_NodeList(t *testing.T) {
	rebootKey := "weave.works/kured-most-recent-reboot-needed"

	t.Run("no annotated node is skipped", func(t *testing.T) {
		checks := runAnnotation(t, nodeRebootCheck(), nodeWithAnnotations("node1", nil), nodeWithAnnotations("node2", nil))
		assertHasOutcome(t, checks, "Node", "nodes", adapter.OutcomeSkipped, "no Node objects carry annotation")
	})
	t.Run("recently-flagged node passes", func(t *testing.T) {
		ann := map[string]string{rebootKey: time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)}
		checks := runAnnotation(t, nodeRebootCheck(), nodeWithAnnotations("node1", ann))
		assertHasOutcome(t, checks, "Node", "node1", adapter.OutcomePass, "within the freshness window")
	})
	t.Run("long-waiting node warns", func(t *testing.T) {
		ann := map[string]string{rebootKey: time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)}
		checks := runAnnotation(t, nodeRebootCheck(), nodeWithAnnotations("node1", ann), nodeWithAnnotations("node2", nil))
		assertHasOutcome(t, checks, "Node", "node1", adapter.OutcomeWarn, "older than the freshness window")
		for _, c := range checks {
			if c.TargetRef.Name == "node2" {
				t.Fatalf("node2 carries no annotation and must not be emitted: %#v", c)
			}
		}
	})
}
