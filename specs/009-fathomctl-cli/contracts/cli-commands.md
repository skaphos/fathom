<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Contract: fathomctl command surface

This is the user-visible contract for the MVP. Anything not listed is not
promised. Exit codes are `0` on success and `1` on any error (kubectl
convention); the message distinguishes the cause.

## Global flags

| Flag | Default | Notes |
|---|---|---|
| `--kubeconfig <path>` | clientcmd discovery (`KUBECONFIG`, `~/.kube/config`, in-cluster) | |
| `--context <name>` | current context | |
| `-n, --namespace <ns>` | context namespace, else `default` | usage error with `-A` |
| `-A, --all-namespaces` | false | ignored by cluster-scoped kinds |
| `-o, --output table\|json\|yaml` | `table` | other values are a usage error |
| `--request-timeout <duration>` | `30s` | per API request |

Kind arguments accept the Kind, its lowercase form, the plural resource
name, or a CLI alias (`ac`, `dns`, `ncc`, `hc`, `ch`). A check is addressed
as `<kind>/<name>` or `<kind> <name>`.

## `fathomctl ls [kind] [-l selector]`

Lists checks with their current verdict.

- No kind: all five kinds, grouped, in the order AddonCheck, DNSCheck,
  NodeCertificateCheck, HealthCheck, ClusterHealth. ClusterHealth is listed
  regardless of `-n`/`-A`.
- Table columns: `KIND` (only in the grouped listing), `NAMESPACE` (only with
  `-A`, omitted for ClusterHealth), `NAME`, `VERDICT`, `SUMMARY` (truncated to
  80 columns), `LAST RUN` (age), `NEXT RUN` (age or `-`).
- Empty result: `No checks found in namespace <ns>.` (or `...in any
  namespace.`) on stdout, exit 0.
- `-o json`: a `v1.List` whose `items` are the resources unmodified;
  `-o yaml`: the same list as YAML.

## `fathomctl describe <kind>/<name>`

Shows one check in full.

- Sections: `Name`/`Namespace`/`Kind`; `Spec` (interval, timeout, paused,
  policy or targets or paths, checkRef, selector as applicable); `Status`
  (verdict, summary, last run, next run, consumed trigger, detected version,
  observed generation); `Conditions` table (type, status, reason, message,
  last transition); kind-specific detail rows (AddonCheck: absent count and
  policy families; DNSCheck: per target/resolver results; NodeCertificateCheck:
  desired/reporting nodes; ClusterHealth: children with result, summary,
  observed); `Latest report: <name> (see: fathomctl reports <kind>/<name>)`.
- Not found: `<kind> "<name>" not found in namespace <ns>` on stderr, exit 1.
- `-o json|yaml`: the resource unmodified.

## `fathomctl reports <kind>/<name> [--limit N] [--since <duration>] [--report <name>]`

Lists HealthReport history for a check.

- Executable kinds: reports labelled with the check's kind and name, newest
  first. HealthCheck: follows `spec.checkRef` and prints
  `Showing reports for <source>` first. ClusterHealth: exit 1 with the list
  of sources to query.
- Columns: `NAME`, `OBSERVED` (age), `RESULT`, `SUMMARY`, `CHANGE`.
- `--limit` default 10; `--since` filters by `spec.observedAt`.
- `--report <name>`: one report in full (table renders spec and per-check
  rows; json/yaml emit the object).
- No history: `No reports yet for <kind>/<name>.`, exit 0.
- Help text includes: "Reports are written when a verdict changes, not on
  every interval; a gap is not a missed run."
- `-o json|yaml`: a `v1.List` of the reports unmodified.

## `fathomctl run (<kind>/<name> | --all | -l selector) [--wait] [--timeout <duration>] [--yes] [--dry-run]`

Requests an immediate re-evaluation.

- Exactly one of a target, `--all`, or `-l` is required. `--all` and `-l`
  select executable kinds only (AddonCheck, DNSCheck, NodeCertificateCheck)
  within the `-n`/`-A` scope; derived kinds are never selected this way.
- Resolution: executable targets are used as-is; a HealthCheck resolves to
  its `checkRef` (a paused HealthCheck resolves to nothing, with a reason);
  a ClusterHealth resolves to the `checkRef` of every non-paused HealthCheck
  in `status.children`. The set is de-duplicated.
- Pre-flight: a paused target, a missing source, or a ClusterHealth with no
  children is reported and excluded; if nothing remains, exit 1.
- Count is printed. Above 10 targets, an interactive confirmation is
  required unless `--yes`; without a terminal and without `--yes`, exit 1
  before any write. `--dry-run` prints the set and exits 0.
- Write: one token per invocation, written to the
  `fathom.skaphos.io/run-now` annotation of every target with a merge patch
  whose field manager is `fathomctl` (visible in the object's managed
  fields).
  Output per target: `Triggered <kind>/<ns>/<name> (token <T>)` or the error.
- `--wait`: per target, poll until `status.lastRunTrigger == T`, the
  annotation changes (superseded), or the timeout elapses. Default timeout
  is the largest target's effective `spec.timeout` plus 30 seconds. Output
  per target: verdict and summary, or `superseded`, or `timed out after <d>`
  with hints (operator version via `fathomctl version`, paused, node-agent
  rollout).
- Exit: `0` when every target was written and, with `--wait`, every verdict
  is Pass, Warn, or Skipped; otherwise `1`.
- `-o json|yaml` with `run`: the list of per-target outcomes.

## `fathomctl version [--client]`

- Table:
  ```
  Client:   v0.6.0
  Operator: v0.6.0 (fathom-system/fathom-controller-manager)
  ```
  or `Operator: unavailable (<reason>)`.
- `--client`: no cluster contact.
- Exit 0 even when the operator is unavailable; exit 1 only for usage errors.
- `-o json|yaml`: `{client, operator{version, namespace, deployment, image, error}}`.

## Help and completion

`fathomctl help`, `--help` on every verb, and `fathomctl completion
<shell>` are provided by the command framework; `pause` and `resume` do not
exist.
