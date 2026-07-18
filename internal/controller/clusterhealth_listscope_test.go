/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

type failingHealthCheckListClient struct{ client.Client }

func (failingHealthCheckListClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return apierrors.NewServiceUnavailable("list failed")
}

// TestListSelectedHealthChecks_ErrorNamesScope pins the namespace scope in the
// wrapped list error, which surfaces in the Ready/ListFailed condition message
// (Copilot review on #137).
func TestListSelectedHealthChecks_ErrorNamesScope(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ClusterHealthReconciler{
		Client: failingHealthCheckListClient{fake.NewClientBuilder().WithScheme(scheme).Build()},
		Scheme: scheme,
	}

	tests := []struct {
		name       string
		namespaces []string
		want       string
	}{
		{name: "empty means all namespaces", want: "listing HealthChecks in all namespaces"},
		{name: "explicit namespace named", namespaces: []string{"tenant-a"}, want: `listing HealthChecks in namespace "tenant-a"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ch := &fathomv1alpha1.ClusterHealth{Spec: fathomv1alpha1.ClusterHealthSpec{Namespaces: tc.namespaces}}
			_, err := r.listSelectedHealthChecks(context.Background(), ch, labels.Everything())
			if err == nil {
				t.Fatal("listSelectedHealthChecks: expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name the scope %q", err, tc.want)
			}
		})
	}
}
