# Supply-Chain / CI-Integrity Review — Fathom @ cb845dd

Reviewer perspective: supply-chain / CI integrity only.

Surface examined: `.github/workflows/{ci,e2e,release,release-please,charts}.yml(.yaml)`,
`.github/dependabot.yml`, `Taskfile.yml`, `tools/go.mod`, `Dockerfile`,
`Dockerfile.probe`, `Dockerfile.node-agent`, `scripts/{check-coverage,check-crd-compat,check-version-lockstep,e2e-shards}.sh`,
`.crd-compat-allowlist.yaml`, `.crdify.yaml`, `release-please-config.json`,
`test/e2e/fixtures/kind-cluster.yaml`, and the go-git/v5 bump (#246).

Overall posture is strong: every third-party action is SHA-pinned with a version
comment; base images are digest-pinned and run non-root; containers are
multi-stage (scratch/distroless); `permissions:` is declared workflow- and
job-level at `contents: read` for CI/e2e; PR builds use `pull_request` (not
`pull_request_target`), so forks get a read-only token and no secrets;
user-controlled values (`github.base_ref`, `github.event.repository.name`) are
passed through `env:` rather than interpolated into `run:`; the release job
does keyless cosign signing + build-provenance attestation by immutable digest.
The findings below are the residual gaps.

---

### SCM-1: SBOMs are published as release assets only — not attached to images as OCI referrers nor signed (medium)
- Location: `.github/workflows/release.yml:271-304`
- Failure scenario: The release generates SPDX SBOMs for the operator/probe/node-agent images and uploads them as GitHub *release assets* (`softprops/action-gh-release` `files: dist/sbom/*.spdx.json`). It never attaches them to the image manifests in GHCR (no `cosign attest --type spdxjson`, no `anchore/sbom-action` push/attest, no OCI referrer). A consumer who pulls `ghcr.io/skaphos/fathom-operator@sha256:...` by digest has no way to discover or verify the SBOM from the registry — it lives on a separate, mutable GitHub release page and carries no signature binding it to the image digest. An attacker who can edit release assets (or simply a mismatch between the loose file and the actual image) yields an SBOM whose integrity is not cryptographically tied to the artifact. `supply-chain.md` flags "SBOMs generated but not attached or not signed" as an anti-pattern.
- Evidence: `anchore/sbom-action` steps use `upload-artifact: false` and write to `dist/sbom/*.spdx.json`; the only consumer is the `action-gh-release` `files:` list (line 301-303). Provenance, by contrast, *is* pushed to the registry (`push-to-registry: true`, lines 234/241/248/255/262/269). The SBOM is the one artifact in the chain that is neither signed nor registry-attached.
- Refutation notes: Provenance attestation (which is registry-attached and signed) already records source+builder, so this is a defense-in-depth / discoverability gap rather than a code-shipping hole. The images themselves are cosign-signed by digest, so the artifact integrity is intact — only the SBOM's binding is weak. Fix is small: `cosign attest --predicate <sbom> --type spdxjson <name>@<digest>` or `anchore/sbom-action` with attestation.

### SCM-2: `release.yml` publishes a fully-signed, provenanced release from *any* `v*` tag with no guard that the commit is on `main` / came from the release flow (medium)
- Location: `.github/workflows/release.yml:3-9` (trigger) and `.github/workflows/release-please.yml:176-192` (the intended tag origin)
- Failure scenario: The release workflow triggers on `push: tags: v*` and immediately builds, pushes, cosign-signs, and attests provenance for the checked-out ref. Nothing verifies that the tagged commit is reachable from `main` or that it originated from the reviewed release-please PR. Anyone able to push a tag — a maintainer, or a leaked/compromised token or PAT carrying `contents: write` — can `git tag v99.0.0 <arbitrary-attacker-commit> && git push origin v99.0.0` and the pipeline will emit an *official, signed, provenanced* release of unreviewed code. Because the cosign identity a downstream verifier pins is only the workflow path + `refs/tags/v*` ref pattern, that malicious release passes signature/provenance verification indistinguishably from a legitimate one.
- Evidence: `on: push: tags: ["v*"]` with no `if:` ref/branch guard; `checkout` uses the tag ref as-is (`release.yml:37-39`); the "trusted" tag path (release-please) is entirely separate and self-imposed, not enforced by the release job. No `concurrency` and no "tag commit must be an ancestor of main" check.
- Refutation notes: Pushing a `v*` tag requires repo push access (a trusted role), and GitHub *tag protection rules* / ruleset restrictions may be configured server-side (not visible in-repo) to limit who can create `v*` tags — if so, blast radius is contained. This is inherent to tag-triggered release designs. Mitigation: gate the job on `contains(github.event.base_ref, 'refs/heads/main')` or verify the tagged SHA is an ancestor of `origin/main` before building, and add a tag-protection ruleset.

### SCM-3: release/e2e tool binaries are verified only against a checksums file fetched from the same release URL — no signature/provenance check (low)
- Location: `.github/workflows/release.yml:45-75` (operator-sdk, opm), `.github/workflows/e2e.yml:97-111` (helmfile)
- Failure scenario: Each installer downloads `binary` and `checksums.txt` from the *same* `github.com/<proj>/releases/download/v<ver>/` path and runs `sha256sum -c`. This defends against transport corruption and a swapped-single-asset, but not against a compromised upstream release (where both the binary and its checksums file are replaced together) — the check is self-referential. The verified-but-unsigned binary is then `install`ed onto `PATH` and executes with the release job's `packages: write` + `id-token: write` (signing) privileges. A compromised operator-sdk/opm release would run inside the signing context.
- Evidence: `curl -fsSLO "${base}/${binary}"; curl -fsSLO "${base}/checksums.txt"; grep ... | sha256sum -c -` — both URLs share `${base}`. No `cosign verify-blob` / slsa-verifier against a pinned upstream identity, and the versions are pinned by tag string, not by a recorded digest.
- Refutation notes: This is the standard limitation of checksum-file verification and is common practice; the versions are at least pinned (operator-sdk via `.tool-versions`, opm/helmfile via literals) and re-pinning is a reviewed diff. Only reachable if the upstream project's GitHub releases are themselves compromised. Defense-in-depth: pin the expected sha256 in-repo instead of trusting the fetched checksums file, or `cosign verify-blob` where the upstream publishes signatures.

### SCM-4: `checkout` never sets `persist-credentials: false`; the write-scoped token/app token stays in `.git/config` across later tool steps (low)
- Location: `.github/workflows/release.yml:37-39`, `.github/workflows/release-please.yml:100-103`, and all CI/e2e checkouts
- Failure scenario: `actions/checkout` defaults to persisting the job token in `.git/config`. In `release.yml` that token carries `contents: write` + `packages: write`; in `release-please.yml` the persisted credential is the *GitHub App token*. Every subsequent step (`go -C tools tool task ...`, `go run <tool>@<ver>` which fetches modules from the network, `docker buildx`, cosign) runs with that credential sitting on disk. A malicious dependency or tool pulled during those steps could read `.git/config` and reuse the write-scoped token.
- Evidence: no `with: persist-credentials: false` on any checkout; release-please explicitly checks out with `token: ${{ steps.app-token.outputs.token }}` (needed for the later `git push`, but it also then runs `git`/`gh`/`go` steps with it persisted).
- Refutation notes: These are trusted-ref workflows (tag push / push-to-main), CI and e2e tokens are read-only so persistence there is harmless, and release-please genuinely needs the credential to push the tag. Real exploitation requires an already-malicious build dependency. Best practice is still `persist-credentials: false` on jobs that don't `git push`, and scoping the persisted token to only the push step.

### SCM-5: coverage gate silently excludes any package whose import path contains `/e2e` as a substring, not just `test/e2e` (low)
- Location: `Taskfile.yml:206` (`go test $(go list ./... | grep -v /e2e)`) feeding `scripts/check-coverage.sh` (`ci.yml:135-136`)
- Failure scenario: The unit-test/coverage run filters packages with `grep -v /e2e`, intending to drop `test/e2e`. But `grep -v /e2e` is a substring match: a package such as `internal/e2ehelpers` or `internal/foo/e2e` is also excluded — from both testing *and* the coverage profile. Since `check-coverage.sh` only evaluates packages present in `coverage.out`, such a package is never held to the 50% floor. A contributor (or an attacker slipping logic past review) can park untested/low-quality code in an `e2e`-named package and the coverage gate will not notice.
- Evidence: confirmed locally — `echo "github.com/skaphos/fathom/internal/e2ehelpers" | grep -v /e2e` → excluded. `check-coverage.sh` derives its package set purely from rows in the profile (awk over `coverage.out`), so an unlisted package is invisible to the gate; `skip_pkg()` is intentionally empty precisely to hold *every* listed package.
- Refutation notes: No such package exists in the tree today, so this is latent, not active. The pattern would have to be introduced in a reviewed PR. Fix: anchor the filter (`grep -v '/test/e2e$'` or `grep -v '/test/e2e'`) so only the intended suite is dropped.

---

## Checked and clean (no candidate)

- **go-git/v5 bump (#246)** — `tools/go.mod` moves `go-git/v5 v5.12.0 → v5.19.1` and `go-billy/v5 → v5.9.0`, both **indirect** deps of the tooling module only (pulled via go-getter/task); neither appears in the operator's runtime `go.mod`. It is a forward version bump, not a downgrade or pin removal. No regression; no runtime blast radius.
- **Action pinning** — every third-party action (checkout, setup-go, setup-python, upload-artifact, docker/{setup-qemu,setup-buildx,login}, azure/setup-helm, helm/{kind-action,chart-testing-action}, sigstore/cosign-installer, anchore/sbom-action, softprops/action-gh-release, googleapis/release-please-action, actions/create-github-app-token) is SHA-pinned with a trailing version comment. No `@main`/`@vN` mutable tags anywhere.
- **Dockerfiles** — all three pin bases by digest (`golang:1.26.5@sha256:...`, `distroless/static:nonroot@sha256:...`), builder cross-compiles from `$BUILDPLATFORM` with `CGO_ENABLED=0`, runtime is `scratch`/distroless with explicit `USER 65532`. `images:refresh` task exists to re-pin digests.
- **Script injection** — untrusted inputs (`github.base_ref`, `github.event.repository.name`) are routed through `env:` and quoted `"$VAR"` in `run:` blocks; no `${{ github.event.* }}` interpolated directly into shell.
- **Fork-PR secret exposure** — CI, e2e, and charts run on `pull_request` (not `pull_request_target`) at `permissions: contents: read`; e2e builds images but does not push; the only secret-bearing workflow (release-please, app private key) runs solely on `push: main`; release runs only on tag push. No secret is reachable from a fork PR.
- **OIDC / credentials** — release signing uses keyless cosign via `id-token: write` (no long-lived signing keys); GHCR auth uses the ephemeral `github.token`; release-please uses a scoped GitHub App token (`repositories:` limited).
- **crd-compat gate** — the allowlist is a committed, reviewed file; every entry requires crd/path/reason/issue and is validated; unmatched entries are reported STALE. No out-of-band CI state.
- **e2e shard planner** — fails open to the full matrix on unknown paths and on `git diff` failure; any change under `api/ internal/ pkg/ cmd/ config/` classifies as `all`, so runtime changes cannot trick it into emitting `[]`.
