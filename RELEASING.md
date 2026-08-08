# Release runbook

Releases are built by GitHub Actions from annotated semantic-version tags such as `v1.0.0`.

## One-time repository settings

Before the first release:

- Set `main` as the default branch. Protect it with a ruleset that blocks force-pushes and deletion and requires the CI and Security checks.
- Protect `v*` tags from deletion or unauthorized creation.
- Create a protected `release` environment with required maintainer approval.
- Permit the release workflow to receive `contents: write`, enable immutable GitHub Releases, and keep all other workflow permissions read-only by default.
- Enable the dependency graph, Dependabot alerts, CodeQL/code scanning, and private vulnerability reporting.

These settings are part of the release control plane and cannot be enforced by files in this repository alone.

1. Ensure `main` is green and the worktree is clean.
2. Update `CHANGELOG.md`; verify the executor version, protocol docs, schema, examples, and migration notes agree.
3. Run `make check`, `make test-race`, and the supported-platform integration matrix.
4. Create and push an unsigned annotated `vMAJOR.MINOR.PATCH` tag.
5. Let `.github/workflows/release.yml` build Darwin/Linux amd64/arm64 archives with Go 1.26.5, generate checksums and SBOMs, attest the artifacts, smoke-test them, pass the protected-environment approval, and publish the immutable GitHub Release.
6. Download each asset from the release, verify checksums and attestations, test the installer in an empty prefix, and confirm `pxp version` reports the tag and commit.

The workflow uploads a draft first and publishes it only after every asset upload succeeds. If an upload fails, inspect the draft and delete it before rerunning, or complete it manually only after reproducing all gates. Never replace assets on a published tag. Fix released defects forward with a new patch version and document the affected version. GitHub Releases are the source of truth; mutable raw-branch downloads are not a distribution channel.

Release tags are intentionally unsigned. Protect `v*` tags against update and deletion, require release commits to be on `main`, and verify the GitHub artifact attestations published with every release.
