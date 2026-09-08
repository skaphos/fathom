/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func TestRunTriggerDue(t *testing.T) {
	tests := []struct {
		name         string
		annotations  map[string]string
		lastConsumed string
		wantToken    string
		wantDue      bool
	}{
		{name: "no annotations", annotations: nil, lastConsumed: "", wantToken: "", wantDue: false},
		{name: "annotation absent with a consumed token", annotations: map[string]string{"other": "x"}, lastConsumed: "t1", wantToken: "", wantDue: false},
		{name: "new token is due", annotations: map[string]string{fathomv1alpha1.AnnotationRunNow: "t1"}, lastConsumed: "", wantToken: "t1", wantDue: true},
		{name: "different token is due", annotations: map[string]string{fathomv1alpha1.AnnotationRunNow: "t2"}, lastConsumed: "t1", wantToken: "t2", wantDue: true},
		{name: "same token is not due", annotations: map[string]string{fathomv1alpha1.AnnotationRunNow: "t1"}, lastConsumed: "t1", wantToken: "t1", wantDue: false},
		{name: "empty annotation value is not due", annotations: map[string]string{fathomv1alpha1.AnnotationRunNow: ""}, lastConsumed: "t1", wantToken: "", wantDue: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, due := runTriggerDue(tt.annotations, tt.lastConsumed)
			if token != tt.wantToken || due != tt.wantDue {
				t.Fatalf("runTriggerDue = (%q, %v), want (%q, %v)", token, due, tt.wantToken, tt.wantDue)
			}
		})
	}
}
