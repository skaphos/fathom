<!-- SPDX-FileCopyrightText: 2026 Rillan AI LLC -->
<!-- SPDX-License-Identifier: MIT -->

# Data model

No CRD, report-wire, or persistent-storage changes.

- **Certificate series**: `(namespace, check, node) -> minimum DaysRemaining` across accepted certificate results with nonzero NotAfter. Unknown expiry produces no sample. Certificate filenames and DNs are never dimensions.
- **Health result series**: `(namespace, check, node, type, path, result) -> 0|1`, preserving the six-state one-hot contract for agent-evaluated items.
- **Health headroom series**: `(namespace, check, node, path, resource) -> percent free`, where resource is bytes or inodes.
- **Lifecycle**: absent after restart until reconciliation; withdrawn at reconcile entry; rebuilt from fresh, accepted reports restricted to expected nodes. Pause, delete, invalid inputs, errors, stale/rejected reports, and node/item removal do not repopulate those series. Other checks are unaffected.

Metric projection is independent from frozen last-known CR status and HealthReport history. A stalled controller retains its previous in-process values until restart/reconciliation; existing check age/cadence metrics detect that condition.
