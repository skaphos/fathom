/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type conflictOnceStatusClient struct {
	client.Client
	conflict *bool
}

func (c conflictOnceStatusClient) Status() client.SubResourceWriter {
	return conflictOnceStatusWriter{SubResourceWriter: c.Client.Status(), conflict: c.conflict}
}

type conflictOnceStatusWriter struct {
	client.SubResourceWriter
	conflict *bool
}

func (w conflictOnceStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if w.conflict != nil && *w.conflict {
		*w.conflict = false
		return apierrors.NewConflict(
			schema.GroupResource{Group: "fathom.skaphos.io", Resource: "status"},
			obj.GetName(),
			errors.New("injected status conflict"),
		)
	}
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}
