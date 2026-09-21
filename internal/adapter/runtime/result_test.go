/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
)

func resultBudget(t *testing.T) *execution.Budget {
	t.Helper()
	b, done := execution.NewBudget(context.Background(), 0)
	t.Cleanup(done)
	return b
}
func checkResult() adapter.CheckResult {
	return adapter.CheckResult{Family: "health", Outcome: adapter.OutcomePass, Summary: "healthy"}
}
func TestResultBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		make func(int) adapter.Result
		cap  int
	}{
		{"entries", func(n int) adapter.Result {
			r := adapter.Result{}
			for i := 0; i < n; i++ {
				r.Checks = append(r.Checks, checkResult())
			}
			return r
		}, limits.MaxResults},
		{"message", func(n int) adapter.Result {
			c := checkResult()
			c.Summary = strings.Repeat("x", n)
			return adapter.Result{Checks: []adapter.CheckResult{c}}
		}, limits.MaxMessageBytes},
		{"details", func(n int) adapter.Result {
			c := checkResult()
			c.Details = map[string]string{}
			for i := 0; i < n; i++ {
				c.Details[string(rune('A'+i))] = "x"
			}
			return adapter.Result{Checks: []adapter.CheckResult{c}}
		}, limits.MaxDetails},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, n := range []int{tc.cap, tc.cap + 1} {
				b := resultBudget(t)
				err := b.ValidateResult(tc.make(n))
				if (err != nil) != (n > tc.cap) {
					t.Fatalf("size=%d err=%v", n, err)
				}
			}
		})
	}
}
func TestSerializedEvidenceExactBoundary(t *testing.T) {
	c := checkResult()
	c.Details = map[string]string{"payload": ""}
	r := adapter.Result{Checks: []adapter.CheckResult{c}}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	c.Details["payload"] = strings.Repeat("x", limits.MaxEvidenceBytes-len(raw))
	raw, err = json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != limits.MaxEvidenceBytes {
		t.Fatal("invalid boundary fixture")
	}
	if err := resultBudget(t).ValidateResult(r); err != nil {
		t.Fatal(err)
	}
	c.Details["payload"] += "x"
	if err := resultBudget(t).ValidateResult(r); err == nil {
		t.Fatal("oversized serialized evidence accepted")
	}
}
func TestResultFailureDiscardsPartialSuccessAndOwnsSnapshot(t *testing.T) {
	r := adapter.Result{Checks: []adapter.CheckResult{checkResult()}}
	r.Checks[0].Details = map[string]string{"state": "original"}
	sealed, err := execution.SealResult(resultBudget(t), r)
	if err != nil {
		t.Fatal(err)
	}
	r.Checks[0].Details["state"] = "mutated"
	if sealed.Checks[0].Details["state"] != "original" {
		t.Fatal("result aliases evaluator memory")
	}
	r.Checks = append(r.Checks, adapter.CheckResult{Outcome: adapter.OutcomeError, Summary: "failed"})
	rejected, err := execution.SealResult(resultBudget(t), r)
	if err == nil || len(rejected.Checks) != 0 {
		t.Fatalf("partial success escaped: %+v %v", rejected, err)
	}
}
func TestResultPreservesFirstFailureAndDeadline(t *testing.T) {
	b := resultBudget(t)
	first := b.Fail("AccessDenied", "forbidden")
	r := adapter.Result{Checks: []adapter.CheckResult{{Summary: strings.Repeat("x", 2048)}}}
	if err := b.ValidateResult(r); err != first {
		t.Fatalf("first failure replaced: %v", err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	expired, done := execution.NewBudget(ctx, 0)
	defer done()
	if err := expired.ValidateResult(r); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline replaced: %v", err)
	}
	summary := execution.FailureSummary(errors.New(strings.Repeat("sensitive", 10000)))
	if len(summary) > limits.MaxMessageBytes || strings.Contains(summary, "sensitive") {
		t.Fatal("failure summary copied unbounded error payload")
	}
}

func TestResultCountsUTF8BytesAndJSONEscaping(t *testing.T) {
	c := checkResult()
	c.Summary = strings.Repeat("é", limits.MaxMessageBytes/2)
	if err := resultBudget(t).ValidateResult(adapter.Result{Checks: []adapter.CheckResult{c}}); err != nil {
		t.Fatal(err)
	}
	c.Summary += "a"
	if err := resultBudget(t).ValidateResult(adapter.Result{Checks: []adapter.CheckResult{c}}); err == nil {
		t.Fatal("counted runes instead of UTF-8 bytes")
	}
	c = checkResult()
	c.Details = map[string]string{"escaped": strings.Repeat("<", limits.MaxEvidenceBytes/6)}
	if err := resultBudget(t).ValidateResult(adapter.Result{Checks: []adapter.CheckResult{c}}); err == nil {
		t.Fatal("JSON expansion bypassed evidence cap")
	}
}

func TestCompletedVerdictsAreEvidenceButEmptyRunIsNot(t *testing.T) {
	for _, outcome := range []adapter.Outcome{adapter.OutcomePass, adapter.OutcomeWarn, adapter.OutcomeFail, adapter.OutcomeSkipped} {
		check := checkResult()
		check.Outcome = outcome
		sealed, err := execution.SealResult(resultBudget(t), adapter.Result{Checks: []adapter.CheckResult{check}})
		if err != nil || len(sealed.Checks) != 1 || sealed.Checks[0].Outcome != outcome {
			t.Fatalf("outcome=%s evidence=%+v err=%v", outcome, sealed, err)
		}
	}
	if _, err := execution.SealResult(resultBudget(t), adapter.Result{}); err == nil {
		t.Fatal("empty result counted as completed evidence")
	}
}
