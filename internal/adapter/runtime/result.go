/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"unicode/utf8"

	"github.com/skaphos/fathom/pkg/adapter"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
)

// ValidateResult gates accumulated evidence before another evaluator runs and
// before publication. Limits do not truncate evidence or change a verdict.
func (b *Budget) ValidateResult(result adapter.Result) error {
	if err := b.Err(); err != nil {
		return err
	}
	reject := func(detail string) error { return b.Fail("ResultLimitExceeded", detail) }
	if len(result.Checks) > limits.MaxResults {
		return reject("result entry limit exceeded")
	}
	// Bound allocation before JSON escaping (which can expand strings). The
	// exact serialized representation is checked separately below.
	remaining := limits.MaxEvidenceBytes
	charge := func(value string) bool {
		if !utf8.ValidString(value) || len(value) > remaining {
			return false
		}
		remaining -= len(value)
		return true
	}
	if !charge(result.DetectedVersion) {
		return reject("evidence string budget exceeded")
	}
	for _, check := range result.Checks {
		if err := b.Err(); err != nil {
			return err
		}
		if !check.Outcome.Valid() || check.Outcome == adapter.OutcomeError {
			return b.Fail("ExecutionFailed", "evaluation did not complete every check")
		}
		if len(check.Summary) > limits.MaxMessageBytes || len(check.Details) > limits.MaxDetails {
			return reject("message or detail count limit exceeded")
		}
		target := check.TargetRef
		for _, value := range []string{string(check.Family), string(check.Outcome), check.Summary, target.APIVersion, target.Kind, target.Namespace, target.Name} {
			if !charge(value) {
				return reject("evidence string budget exceeded")
			}
		}
		for key, value := range check.Details {
			if !charge(key) || !charge(value) {
				return reject("evidence string budget exceeded")
			}
		}
	}
	data, err := json.Marshal(result)
	if prior := b.Err(); prior != nil {
		return prior
	}
	if err != nil {
		return reject("evidence cannot be serialized")
	}
	if len(data) > limits.MaxEvidenceBytes {
		return reject("serialized evidence limit exceeded")
	}
	return nil
}

// SealResult returns owned completed evidence, or no evidence on any failure.
// Failure summaries are allocated separately so a full evidence buffer cannot
// crowd out the diagnostic or leave partial Pass entries available for rollup.
func SealResult(b *Budget, result adapter.Result) (adapter.Result, error) {
	if b == nil {
		return adapter.Result{}, &Failure{Reason: "AuthorizationUnavailable", Detail: "result requires a run budget"}
	}
	if err := b.ValidateResult(result); err != nil {
		return adapter.Result{}, err
	}
	if len(result.Checks) == 0 {
		return adapter.Result{}, b.Fail("ExecutionFailed", "evaluation produced no observations")
	}
	copy := result
	copy.Checks = append([]adapter.CheckResult(nil), result.Checks...)
	for i := range copy.Checks {
		copy.Checks[i].Details = maps.Clone(result.Checks[i].Details)
	}
	if err := b.Err(); err != nil {
		return adapter.Result{}, err
	}
	return copy, nil
}

// FailureSummary reserves a small diagnostic independent of evidence capacity.
// It never formats an arbitrary API error or panic value into persisted status.
func FailureSummary(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "runtime evaluation timed out; no new evidence was published"
	}
	if errors.Is(err, context.Canceled) {
		return "runtime evaluation was canceled; no new evidence was published"
	}
	var failure *Failure
	if errors.As(err, &failure) {
		switch failure.Reason {
		case "ResultLimitExceeded":
			return "runtime evidence exceeded its limits; no new evidence was published"
		case "InputLimitExceeded", "ResponseLimitExceeded", "WorkLimitExceeded":
			return "runtime input or work exceeded its limits; no new evidence was published"
		case "AccessDenied", "ScopeDenied", "AuthorizationUnavailable", "AuthorizationRevoked":
			return "runtime authorization failed; no new evidence was published"
		case "ExecutionPanic":
			return "runtime execution panicked; no new evidence was published"
		}
	}
	return "runtime evaluation failed; no new evidence was published"
}
