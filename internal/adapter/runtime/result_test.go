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

// The pre-marshal string budget bounds allocation before JSON escaping, so it
// must reject on raw accumulated bytes and on invalid UTF-8 that the encoder
// would silently replace with U+FFFD. The invalid-UTF-8 arms isolate the guard
// by construction: each marshals to a tiny document the serialized-size check
// would wave through. The accumulated-bytes arm cannot be isolated that way —
// raw bytes over the cap always serialize over it too — so it is isolated by the
// rejection detail instead: every arm must be turned away by the string budget
// specifically, not by whichever later check happens to also catch it.
func TestEvidenceStringBudgetRejectsBeforeMarshal(t *testing.T) {
	broken := "not\xffutf8"
	withDetail := func(key, value string) adapter.Result {
		c := checkResult()
		c.Details = map[string]string{key: value}
		return adapter.Result{Checks: []adapter.CheckResult{c}}
	}
	accumulated := adapter.Result{}
	for i := 0; i < 400; i++ {
		c := checkResult()
		// Each summary is individually legal; only their sum crosses the cap.
		c.Summary = strings.Repeat("x", limits.MaxMessageBytes)
		accumulated.Checks = append(accumulated.Checks, c)
	}
	for _, tc := range []struct {
		name         string
		result       adapter.Result
		marshalsOver bool
	}{
		{name: "raw bytes accumulated across checks", result: accumulated, marshalsOver: true},
		{name: "invalid utf-8 detail value", result: withDetail("state", broken)},
		{name: "invalid utf-8 detail key", result: withDetail(broken, "state")},
		{name: "invalid utf-8 summary", result: func() adapter.Result {
			c := checkResult()
			c.Summary = broken
			return adapter.Result{Checks: []adapter.CheckResult{c}}
		}()},
		{name: "invalid utf-8 detected version", result: adapter.Result{DetectedVersion: broken, Checks: []adapter.CheckResult{checkResult()}}},
		{name: "invalid utf-8 target reference", result: func() adapter.Result {
			c := checkResult()
			c.TargetRef = adapter.TargetRef{APIVersion: "v1", Kind: "ConfigMap", Namespace: "default", Name: broken}
			return adapter.Result{Checks: []adapter.CheckResult{c}}
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, marshalErr := json.Marshal(tc.result)
			if marshalErr != nil {
				t.Fatalf("fixture is not serializable: %v", marshalErr)
			}
			if over := len(raw) > limits.MaxEvidenceBytes; over != tc.marshalsOver {
				t.Fatalf("serialized size %d does not isolate the pre-marshal guard", len(raw))
			}
			err := resultBudget(t).ValidateResult(tc.result)
			var failure *execution.Failure
			if !errors.As(err, &failure) || failure.Reason != "ResultLimitExceeded" {
				t.Fatalf("evidence accepted before serialization: %v", err)
			}
			if failure.Detail != "evidence string budget exceeded" {
				t.Fatalf("rejected by a later check, not the pre-marshal string budget: %v", err)
			}
		})
	}
}
