/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodecert

// Report authenticity, and the division of labour that enforces it.
//
// Kubernetes does not record the authenticated writer on an object, so a
// controller reading a ConfigMap cannot ask "who wrote this?". Authenticity is
// therefore enforced at *admission*, by the ValidatingAdmissionPolicy the
// operator provisions: it binds the report's node-name annotation to the
// writing identity's ServiceAccount-token node claim
// (authentication.kubernetes.io/node-name), so only the agent running on node X
// can write a report annotated for node X.
//
// What this file provides is the second half: the *structural bindings* a
// genuine report always satisfies, re-checked at collect time regardless of what
// admission allowed. They are corroboration, not authentication — a writer who
// can forge the annotation can satisfy them all — but they are what remains on a
// cluster where the policy is unavailable (see admissionPolicyUnsupported), and
// they close the one forgery vector admission alone does not: a *second*,
// non-canonically-named ConfigMap claiming a node that already has a legitimate
// report.
//
// Both node-scoped kinds share this path. NodeHealthCheck (#206) must call
// VerifyReportBinding rather than reimplement it (SEC-1 acceptance).

// ReportRejection explains why a candidate node-report ConfigMap was not
// accepted. The zero value, ReportAccepted, means the report is well-bound.
type ReportRejection string

const (
	// ReportAccepted means every structural binding holds.
	ReportAccepted ReportRejection = ""
	// RejectWrongCheck: the payload names a different check than the one
	// collecting it. A mislabeled report, not necessarily a hostile one.
	RejectWrongCheck ReportRejection = "WrongCheck"
	// RejectMissingNode: the payload carries no node name, so it cannot be
	// attributed to anything.
	RejectMissingNode ReportRejection = "MissingNode"
	// RejectMissingNodeAnnotation: the ConfigMap has no node-name annotation, so
	// there is nothing for admission to have bound to a writer. Either it
	// predates the contract or it was written where the policy is unavailable.
	RejectMissingNodeAnnotation ReportRejection = "MissingNodeAnnotation"
	// RejectNodeMismatch: the payload's node disagrees with the annotation
	// admission binds to the writer. Only a writer passing off another node's
	// report produces this.
	RejectNodeMismatch ReportRejection = "NodeAnnotationMismatch"
	// RejectNonCanonicalName: the ConfigMap is not at the deterministic name the
	// agent for this (check, node) writes to. A genuine agent always uses
	// NodeReportConfigMapName, so an off-name report is a second, competing
	// claim on a node the canonical report already covers.
	RejectNonCanonicalName ReportRejection = "NonCanonicalName"
)

// IndicatesForgery reports whether a rejection is one only a writer attempting
// to pass off another node's report can produce.
//
// The distinction drives how loudly the controller reacts: a mislabeled or
// legacy report is skipped quietly, while a forgery signal is surfaced on a
// condition and an event, because it means some principal with ConfigMap write
// in the namespace is actively trying to steer a node's verdict.
func (r ReportRejection) IndicatesForgery() bool {
	switch r {
	case RejectNodeMismatch, RejectNonCanonicalName:
		return true
	default:
		return false
	}
}

// VerifyReportBinding checks every structural binding tying a decoded report to
// the ConfigMap that carried it, for the check named checkName.
//
// annotatedNode is the AnnotationNodeName value on the ConfigMap — the field
// admission binds to the writer's node claim — and cmName is the ConfigMap's
// own name. Checks run cheapest-and-most-benign first so the returned reason
// names the most specific problem.
func VerifyReportBinding(cmName, annotatedNode, checkName string, report NodeReport) ReportRejection {
	if report.CheckName != checkName {
		return RejectWrongCheck
	}
	if report.Node == "" {
		return RejectMissingNode
	}
	if annotatedNode == "" {
		return RejectMissingNodeAnnotation
	}
	if annotatedNode != report.Node {
		return RejectNodeMismatch
	}
	if cmName != NodeReportConfigMapName(checkName, report.Node) {
		return RejectNonCanonicalName
	}
	return ReportAccepted
}
