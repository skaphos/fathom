# Security Policy

Fathom is a cluster-health operator: it runs with read access across the
clusters it watches and reports health that operators act on. We treat reports
that could corrupt or spoof health results, escalate the operator's RBAC,
or compromise the confidentiality and availability of its controllers and
CRDs as security issues, and we handle them through coordinated disclosure.

## Reporting a vulnerability

**Please do not open a public issue for suspected vulnerabilities.**

Report privately via GitHub's private vulnerability reporting:

1. Go to the repository's **Security** tab.
2. Choose **Report a vulnerability** and fill in the advisory form.

Direct link: <https://github.com/skaphos/fathom/security/advisories/new>

If you cannot use GitHub, email <shawn@skaphos.io> with the details.

Include what you can of: affected component (controller, adapter, CRD/webhook,
beaconctl, Helm chart), a reproduction or proof of concept, the impact you
believe it has, and any suggested fix.

## What to expect

- **Acknowledgement** within 7 days.
- **Assessment** and severity triage in the advisory thread; we may ask
  follow-up questions there.
- **Fix development** happens in the advisory's private fork when the issue is
  exploitable; hardening-grade findings may be fixed in public PRs.
- **Disclosure**: we publish the GitHub security advisory together with the
  patched release and credit the reporter (unless you prefer otherwise).

## Supported versions

Fathom is pre-1.0. Only the latest minor release line receives security fixes;
please upgrade to the most recent release before reporting version-specific
issues.
