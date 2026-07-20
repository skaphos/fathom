/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package podutil_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/skaphos/fathom/internal/adapter/podutil"
)

func TestActive(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{"running no deletion", &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}, true},
		{"pending", &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}}, true},
		{"running not ready still active", &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}}}, true},
		{"terminating", &corev1.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}, false},
		{"failed (evicted)", &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}}, false},
		{"succeeded (completed)", &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := podutil.Active(tt.pod); got != tt.want {
				t.Errorf("Active() = %v, want %v", got, tt.want)
			}
		})
	}
}
