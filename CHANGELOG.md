# Changelog

All notable changes are recorded here. This project follows Semantic Versioning for executor releases; the PXP document version is tracked separately.

## [Unreleased]

- Add PXP 1.1 plans with required exact Git HEAD binding while retaining unbound PXP 1.0 compatibility.
- Add `pxp run --expect-sha256` as an exact-byte approval precondition checked before repository access or execution.
- Add instruction-only Claude Code and Codex skills that stop after producing a closed, hashable plan.
- Add `pxp-bench`, representative offline fixtures, raw metric artifacts, and honest unavailable-metric reporting for direct-agent versus PXP comparisons.

## [1.0.0] - 2026-08-08

- Define the strict PXP 1.0 protocol and stable CLI/exit-code contract.
- Replace the experimental Bash parser with compiled Go executors for macOS, Linux, and WSL.
- Validate complete YAML documents, patch structure, paths, repository state, and process locks before mutation.
- Add the explicit `pxp-convert` migration utility for both historical v0.1 formats.
- Add checksummed multi-platform releases, CI, security automation, documentation, and an atomic installer.

[Unreleased]: https://github.com/otar/pxp/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/otar/pxp/releases/tag/v1.0.0
