/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
)

type runAdapter struct {
	adapter.Adapter
	run func(context.Context, adapter.Request) (adapter.Result, error)
}

func (a runAdapter) Run(ctx context.Context, req adapter.Request) (adapter.Result, error) {
	return a.run(ctx, req)
}
func successfulCompiler(context.Context) (adapter.Adapter, error) {
	return runAdapter{run: func(context.Context, adapter.Request) (adapter.Result, error) {
		return adapter.Result{Checks: []adapter.CheckResult{checkResult()}}, nil
	}}, nil
}
func TestRunnerRecoversCompilationAndEvaluationPanic(t *testing.T) {
	for _, stage := range []string{"compile", "evaluate"} {
		t.Run(stage, func(t *testing.T) {
			b := resultBudget(t)
			released := 0
			compiler := func(context.Context) (adapter.Adapter, error) {
				if stage == "compile" {
					panic("untrusted compile payload")
				}
				return runAdapter{run: func(context.Context, adapter.Request) (adapter.Result, error) { panic("untrusted evaluation payload") }}, nil
			}
			attempt := execution.Execute(b, compiler, adapter.Request{}, func() { released++ })
			if attempt.Err == nil || attempt.Completed || len(attempt.Evidence.Checks) != 0 || released != 1 || b.Err() == nil {
				t.Fatalf("attempt=%+v released=%d", attempt, released)
			}
			peer := execution.Execute(resultBudget(t), successfulCompiler, adapter.Request{}, func() { released++ })
			if peer.Err != nil || !peer.Completed || released != 2 {
				t.Fatalf("healthy peer failed: %+v", peer)
			}
		})
	}
}
func TestRunnerCancellationDoesNotAbandonEvaluator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b, done := execution.NewBudget(ctx, 0)
	defer done()
	started, finish := make(chan struct{}), make(chan struct{})
	returned := make(chan execution.Attempt, 1)
	var released atomic.Bool
	compiler := func(context.Context) (adapter.Adapter, error) {
		return runAdapter{run: func(ctx context.Context, _ adapter.Request) (adapter.Result, error) {
			close(started)
			<-ctx.Done()
			<-finish
			return adapter.Result{Checks: []adapter.CheckResult{checkResult()}}, nil
		}}, nil
	}
	go func() { returned <- execution.Execute(b, compiler, adapter.Request{}, func() { released.Store(true) }) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("evaluator did not start")
	}
	cancel()
	if released.Load() {
		t.Fatal("slot released while evaluator still running")
	}
	select {
	case <-returned:
		t.Fatal("supervisor abandoned evaluator")
	default:
	}
	close(finish)
	select {
	case attempt := <-returned:
		if !errors.Is(attempt.Err, context.Canceled) || len(attempt.Evidence.Checks) != 0 || !released.Load() {
			t.Fatalf("attempt=%+v released=%v", attempt, released.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("supervisor did not finish")
	}
}
func TestRunnerCompilationDeadlineAndFailurePrecedence(t *testing.T) {
	b := resultBudget(t)
	called := false
	compiler := func(ctx context.Context) (adapter.Adapter, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Fatal("compile deadline missing")
		}
		_ = b.Fail("AccessDenied", "first failure")
		return runAdapter{run: func(context.Context, adapter.Request) (adapter.Result, error) {
			called = true
			return adapter.Result{}, nil
		}}, nil
	}
	attempt := execution.Execute(b, compiler, adapter.Request{}, func() {})
	if called || attempt.Err != b.Err() || attempt.Completed {
		t.Fatalf("called=%v attempt=%+v", called, attempt)
	}
}
func TestRunnerSuccessLeavesBudgetForFinalFence(t *testing.T) {
	b := resultBudget(t)
	attempt := execution.Execute(b, successfulCompiler, adapter.Request{}, func() {})
	if attempt.Err != nil || !attempt.Completed || b.Err() != nil {
		t.Fatalf("attempt=%+v budget=%v", attempt, b.Err())
	}
	if err := b.ChargeRequest(); err != nil {
		t.Fatal("final fence cannot use run budget")
	}
}

func TestRunnerExpiredCompilationNeverEvaluates(t *testing.T) {
	b := resultBudget(t)
	evaluated := false
	attempt := execution.Execute(b, func(ctx context.Context) (adapter.Adapter, error) {
		<-ctx.Done()
		return runAdapter{run: func(context.Context, adapter.Request) (adapter.Result, error) {
			evaluated = true
			return adapter.Result{}, nil
		}}, nil
	}, adapter.Request{}, func() {})
	if evaluated || !errors.Is(attempt.Err, context.DeadlineExceeded) || attempt.Completed {
		t.Fatalf("evaluated=%v attempt=%+v", evaluated, attempt)
	}
}

func TestRunnerPanicDoesNotCancelConcurrentPeer(t *testing.T) {
	peerBudget := resultBudget(t)
	entered, proceed := make(chan struct{}), make(chan struct{})
	result := make(chan execution.Attempt, 1)
	go func() {
		result <- execution.Execute(peerBudget, func(context.Context) (adapter.Adapter, error) {
			return runAdapter{run: func(ctx context.Context, _ adapter.Request) (adapter.Result, error) {
				close(entered)
				select {
				case <-proceed:
				case <-ctx.Done():
					return adapter.Result{}, ctx.Err()
				}
				return adapter.Result{Checks: []adapter.CheckResult{checkResult()}}, nil
			}}, nil
		}, adapter.Request{}, func() {})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("peer did not start")
	}
	bad := execution.Execute(resultBudget(t), func(context.Context) (adapter.Adapter, error) { panic("compile failure") }, adapter.Request{}, func() {})
	close(proceed)
	if bad.Err == nil {
		t.Fatal("panic accepted")
	}
	select {
	case peer := <-result:
		if !peer.Completed || peer.Err != nil {
			t.Fatalf("peer affected: %+v", peer)
		}
	case <-time.After(time.Second):
		t.Fatal("peer did not finish")
	}
}

func TestRunnerReleasesSlotOnErrorAndResultLimit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		compiler func(context.Context) (adapter.Adapter, error)
	}{
		{"compile error", func(context.Context) (adapter.Adapter, error) { return nil, errors.New("compile error") }},
		{"empty compiler result", func(context.Context) (adapter.Adapter, error) { return nil, nil }},
		{"evaluation error", func(context.Context) (adapter.Adapter, error) {
			return runAdapter{run: func(context.Context, adapter.Request) (adapter.Result, error) {
				return adapter.Result{Checks: []adapter.CheckResult{checkResult()}}, errors.New("evaluation error")
			}}, nil
		}},
		{"result limit", func(context.Context) (adapter.Adapter, error) {
			return runAdapter{run: func(context.Context, adapter.Request) (adapter.Result, error) {
				r := adapter.Result{}
				for i := 0; i < 1001; i++ {
					r.Checks = append(r.Checks, checkResult())
				}
				return r, nil
			}}, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			released := 0
			attempt := execution.Execute(resultBudget(t), tc.compiler, adapter.Request{}, func() { released++ })
			if attempt.Completed || attempt.Err == nil || len(attempt.Evidence.Checks) != 0 || attempt.Summary == "" || released != 1 {
				t.Fatalf("attempt=%+v released=%d", attempt, released)
			}
		})
	}
}
