/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

import "time"

// Admission floors for check cadence fields. These values are part of the
// published API contract: the CEL XValidation rules on AddonCheckSpec and
// NodeCertificateCheckSpec encode the same durations as string literals
// (CEL markers cannot reference Go constants), and the controllers clamp
// stored objects that predate the rules to these same floors. A test in
// validation_test.go asserts the generated CRD schemas embed these values so
// the schema and the clamp cannot drift apart silently.
const (
	// MinCheckInterval is the lowest admissible spec.interval. Anything
	// faster risks a hot reconcile loop that launches probe workloads
	// continuously and starves other checks (issue #152).
	MinCheckInterval = 10 * time.Second

	// MinCheckTimeout is the lowest admissible spec.timeout. A sub-second
	// timeout can never complete a real check run; fail-fast timeouts at or
	// above one second remain legal.
	MinCheckTimeout = time.Second
)
