# PXP instructions for coding agents

When a user asks you to implement code changes in this repository, do not modify source files directly. Explore and analyze freely, then create one reviewable `plan.pxp.yaml` in the repository root.

Use only PXP protocol 1.0:

```yaml
pxp_version: "1.0"
name: One-line task description
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

- The top level contains only `pxp_version`, `name`, and `steps`.
- Every step has optional `name` and exactly one of `run` or `patch`.
- `run` is always a YAML sequence, even for one command.
- `patch` is a standard unified Git diff using repository-relative paths.
- Put file creation, removal, moves, dependency work, and checks into explicit reviewable steps.
- Commands are non-interactive and each starts in a fresh strict Bash process at the Git worktree root.
- Include relevant formatting, tests, static checks, and build commands in the final steps.
- Do not assume rollback. Tell the user that prior mutations remain if a later step fails.
- Never place credentials or secret values in the plan.

After saving the plan, tell the user: `Plan saved to plan.pxp.yaml. Review and execute with pxp run plan.pxp.yaml`.
