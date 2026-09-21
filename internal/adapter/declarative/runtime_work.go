/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"errors"

	"github.com/skaphos/fathom/pkg/adapter"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ExecutionBudget is the runtime-owned work boundary. Keeping this interface in
// declarative prevents evaluators from depending on the runtime implementation.
type ExecutionBudget interface {
	Visit(int) error
	ValidateResult(adapter.Result) error
	Fail(string, string) error
	Err() error
	WalkPages(context.Context, client.Reader, client.ObjectList, func(context.Context, client.ObjectList) error, func(context.Context) error, ...client.ListOption) error
}
type executionBudgetKey struct{}

// WithExecutionBudget supplies the same run budget used by delegated transport.
func WithExecutionBudget(ctx context.Context, b ExecutionBudget) context.Context {
	return context.WithValue(ctx, executionBudgetKey{}, b)
}
func (ec EvalContext) budget() ExecutionBudget {
	b, _ := ec.Ctx.Value(executionBudgetKey{}).(ExecutionBudget)
	return b
}

// errUnbudgetedRuntime is the fail-closed answer for a runtime execution that
// reached an evaluator without the shared budget. runtimeAdapter.Run refuses to
// start without one, so this can only mean an unbudgeted caller got in.
var errUnbudgetedRuntime = errors.New("AuthorizationUnavailable: runtime execution requires a shared budget")

func (ec EvalContext) requireBudget() (ExecutionBudget, error) {
	b := ec.budget()
	if b == nil {
		return nil, errUnbudgetedRuntime
	}
	return b, nil
}

func (ec EvalContext) walkPages(list client.ObjectList, consume func(client.ObjectList) error, reset func(), opts ...client.ListOption) error {
	if !ec.runtime {
		if err := ec.Client.List(ec.Ctx, list, opts...); err != nil {
			return err
		}
		return consume(list)
	}
	b, err := ec.requireBudget()
	if err != nil {
		return err
	}
	return b.WalkPages(ec.Ctx, ec.Client, list, func(_ context.Context, page client.ObjectList) error { return consume(page) }, func(context.Context) error { reset(); return nil }, opts...)
}

func (ec EvalContext) appendResults(out *[]adapter.CheckResult, values ...adapter.CheckResult) error {
	if !ec.runtime {
		*out = append(*out, values...)
		return nil
	}
	b, err := ec.requireBudget()
	if err != nil {
		return err
	}
	if len(*out)+len(values) > limits.MaxResults {
		return b.Fail("ResultLimitExceeded", "evaluator result count exceeded")
	}
	*out = append(*out, values...)
	return b.ValidateResult(adapter.Result{Checks: *out})
}

// workClient reserves traversal work before evaluators or version helpers
// inspect a returned object. Charging its entire bounded tree is conservative:
// one visit funds this scan and one reserves the evaluator traversal; checks
// usually inspect only a few fields. Transport inspection has its own
// charge, so parser/transport work cannot consume the evaluator's allowance for
// free. This wrapper is used only for runtime definitions.
type workClient struct {
	client.Client
	work ExecutionBudget
}

func (c workClient) Get(ctx context.Context, key client.ObjectKey, out client.Object, opts ...client.GetOption) error {
	if err := c.work.Err(); err != nil {
		return err
	}
	if err := c.Client.Get(ctx, key, out, opts...); err != nil {
		return err
	}
	return c.inspect(ctx, out)
}
func (c workClient) List(ctx context.Context, out client.ObjectList, opts ...client.ListOption) error {
	if err := c.work.Err(); err != nil {
		return err
	}
	if err := c.Client.List(ctx, out, opts...); err != nil {
		return err
	}
	items, err := meta.ExtractList(out)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := c.inspect(ctx, item); err != nil {
			return err
		}
	}
	return nil
}
func (c workClient) inspect(ctx context.Context, obj runtime.Object) error {
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return c.work.Fail("InputLimitExceeded", "cannot inspect runtime target")
	}
	nodes := 0
	var visit func(any, int) error
	visit = func(value any, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		nodes++
		if nodes > limits.MaxObjectNodes || depth > limits.MaxObjectDepth {
			return c.work.Fail("InputLimitExceeded", "runtime target exceeds node/depth limit")
		}
		if err := c.work.Visit(2); err != nil {
			return err
		}
		switch value := value.(type) {
		case map[string]any:
			for _, child := range value {
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range value {
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(data, 0)
}

func (e *Engine) validateRuntimeResult(ctx context.Context, result adapter.Result) error {
	if !e.runtime {
		return nil
	}
	b, _ := ctx.Value(executionBudgetKey{}).(ExecutionBudget)
	if b == nil {
		return errUnbudgetedRuntime
	}
	return b.ValidateResult(result)
}
