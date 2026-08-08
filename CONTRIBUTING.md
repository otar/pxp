# Contributing

Thank you for improving PXP.

1. Open an issue before substantial protocol or CLI changes.
2. Create a focused branch from `main`.
3. Run `make check` and, for process or repository changes, `make test-race`.
4. Add tests for behavior and failure paths. Security fixes need a regression test.
5. Update `SPEC.md`, the JSON Schema, examples, and changelog when the public contract changes.
6. Submit a pull request explaining compatibility and security effects.

Go code must be formatted with `gofmt`. Avoid new dependencies unless the standard library and existing packages cannot provide the required correctness. Commits must not contain generated release artifacts, credentials, or user plan data.

By contributing, you agree that your contribution is licensed under the MIT License.
