/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime

import (
	"context"

	"github.com/skaphos/fathom/internal/adapter/declarative"
	"github.com/skaphos/fathom/pkg/adapter"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
)

// Attempt contains either completed, owned evidence or a bounded failure
// summary. Completed is execution eligibility, not a Pass verdict. Publication
// still requires the controller's final authority/revision/leadership fences.
type Attempt struct {
	Evidence  adapter.Result
	Completed bool
	Err       error
	Summary   string
}

// Execute supervises compilation and evaluation in the calling worker goroutine.
// The caller supplies the shared outer budget (created with the check timeout)
// and its slot release callback. No deadline/select race can leave an evaluator
// running after its slot is released. Evaluators must cooperate with cancellation.
// Successful execution leaves the outer budget alive for final validation.
func Execute(b *Budget, compile func(context.Context) (adapter.Adapter, error), request adapter.Request, release func()) (attempt Attempt) {
	stage := "compile"
	defer func() {
		if recovered := recover(); recovered != nil {
			if b != nil {
				attempt.Err = b.Fail("ExecutionPanic", stage+" panicked")
			} else {
				attempt.Err = &Failure{Reason: "ExecutionPanic", Detail: "runtime supervision panicked"}
			}
		}
		if b != nil && b.Err() != nil {
			attempt.Err = b.Err()
		}
		if attempt.Err != nil {
			attempt.Evidence = adapter.Result{}
			attempt.Completed = false
			attempt.Summary = FailureSummary(attempt.Err)
		}
	}()
	if release != nil {
		defer release()
	}
	if b == nil || compile == nil || release == nil {
		attempt.Err = &Failure{Reason: "AuthorizationUnavailable", Detail: "runtime execution requires budget, compiler and held slot"}
		if b != nil {
			attempt.Err = b.Abort(attempt.Err)
		}
		return attempt
	}
	if err := b.Err(); err != nil {
		attempt.Err = err
		return attempt
	}
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()
	ctx = declarative.WithExecutionBudget(ctx, b)
	compileCtx, stopCompile := context.WithTimeout(ctx, limits.MaxCompileDuration)
	defer stopCompile()
	compiled, err := compile(compileCtx)
	if compileCtx.Err() != nil {
		err = compileCtx.Err()
	}
	stopCompile()
	if err != nil {
		attempt.Err = b.Abort(err)
		return attempt
	}
	if compiled == nil {
		attempt.Err = b.Fail("InvalidDefinition", "compiler returned no executable definition")
		return attempt
	}
	if err := b.Err(); err != nil {
		attempt.Err = err
		return attempt
	}
	stage = "evaluation"
	evidence, err := compiled.Run(ctx, request)
	if err != nil {
		attempt.Err = b.Abort(err)
		return attempt
	}
	attempt.Evidence, attempt.Err = SealResult(b, evidence)
	attempt.Completed = attempt.Err == nil
	return attempt
}
