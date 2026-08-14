# PXP instructions for coding agents

When a user asks you to implement code changes in this repository, do not modify source files directly. Explore and analyze freely, then create one reviewable `plan.pxp.yaml` in the repository root.

Use PXP protocol 1.1 for newly generated plans:

```yaml
pxp_version: "1.1"
name: One-line task description
repository:
  type: git
  head: full HEAD commit object ID
steps:
  - name: Apply source changes
    patch: |
      diff --git a/path/to/file b/path/to/file
      --- a/path/to/file
      +++ b/path/to/file
      @@ -1,3 +1,3 @@
       context
      -old
      +new
  - name: Verify
    run:
      - go test ./...
```

Rules:

- The top level contains only `pxp_version`, `name`, `repository`, and `steps`.
- Resolve `repository.head` with `git rev-parse --verify 'HEAD^{commit}'` before writing the plan.
- Every step has optional `name` and exactly one of `run` or `patch`.
- `run` is always a YAML sequence, even for one command.
- `patch` is a standard unified Git diff using repository-relative paths.
- Put file creation, removal, moves, dependency work, and checks into explicit reviewable steps.
- Commands are non-interactive and each starts in a fresh strict Bash process at the Git worktree root.
- Include relevant formatting, tests, static checks, and build commands in the final steps.
- Do not assume rollback. Tell the user that prior mutations remain if a later step fails.
- Never place credentials or secret values in the plan.

After saving the plan, report its path and bound repository HEAD without executing it. Tell the user: `Plan saved to plan.pxp.yaml. Review and execute with pxp run plan.pxp.yaml`.
