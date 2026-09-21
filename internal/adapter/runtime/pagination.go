/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime

import (
	"context"

	limits "github.com/skaphos/fathom/pkg/addondefinition"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WalkPages consumes one bounded page at a time using the guarded delegated
// reader. The consumer must retain only bounded derived evidence and publish
// nothing until this function succeeds. reset discards all derived evidence for
// this collection before an expired snapshot is restarted. The shared transport
// accounts for requests, objects and bytes, including discarded pages/retries.
func WalkPages(ctx context.Context, budget *Budget, reader client.Reader, prototype client.ObjectList,
	consume func(context.Context, client.ObjectList) error, reset func(context.Context) error, opts ...client.ListOption) error {
	if budget == nil {
		return &Failure{Reason: "AuthorizationUnavailable", Detail: "pagination requires a run budget"}
	}
	if reader == nil || prototype == nil || consume == nil || reset == nil {
		return budget.Fail("AuthorizationUnavailable", "pagination requires reader, list and transactional consumer")
	}
	options := new(client.ListOptions).ApplyOptions(opts)
	if options.Raw != nil || options.Continue != "" {
		return budget.Fail("ScopeDenied", "pagination must start at the beginning without raw list options")
	}
	if options.Limit < 0 {
		return budget.Fail("ScopeDenied", "invalid page limit")
	}
	if options.Limit == 0 || options.Limit > limits.MaxPageObjects {
		options.Limit = limits.MaxPageObjects
	}
	seen := map[string]bool{}
	for pageNumber := 0; pageNumber < limits.MaxRunRequests; pageNumber++ {
		if err := budget.Err(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return budget.Abort(err)
		}
		page := prototype.DeepCopyObject().(client.ObjectList)
		// Never let the previous page or a caller's prototype become input to the
		// decoder. Only the prototype type/GVK is retained.
		if err := meta.SetList(page, nil); err != nil {
			return budget.Abort(err)
		}
		page.SetContinue("")
		if err := reader.List(ctx, page, options); err != nil {
			if !apierrors.IsResourceExpired(err) {
				return budget.Abort(err)
			}
			if err := budget.ChargePaginationRestart(); err != nil {
				return err
			}
			if err := reset(ctx); err != nil {
				return budget.Abort(err)
			}
			options.Continue = ""
			seen = map[string]bool{}
			continue
		}
		items, err := meta.ExtractList(page)
		if err != nil {
			return budget.Abort(err)
		}
		if len(items) > limits.MaxPageObjects {
			return budget.Fail("WorkLimitExceeded", "API page exceeds object limit")
		}
		token := page.GetContinue()
		if token != "" && (seen[token] || budget.Objects() >= limits.MaxRunObjects) {
			return budget.Fail("WorkLimitExceeded", "pagination cannot complete within run limits")
		}
		if err := budget.Visit(len(items)); err != nil {
			return err
		}
		if err := consume(ctx, page); err != nil {
			return budget.Abort(err)
		}
		if err := budget.Err(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return budget.Abort(err)
		}
		if token == "" {
			return nil
		}
		seen[token] = true
		options.Continue = token
	}
	return budget.Fail("WorkLimitExceeded", "pagination request limit exceeded")
}
