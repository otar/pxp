When implementing changes or presenting a finalized plan for implementation, do not edit files directly. Instead, generate a single `changeset.yml` changeset file using the PXP format.

Format:
```yaml
name: Changeset short description

steps:
  - name: Step short description
    run: shell commands for dependencies, tooling, verification
  - name: Step short description
    patch: |
      unified diffs for code changes
```

Rules:
- Steps execute sequentially: `run` for shell commands, `patch` for unified diffs
- Patches use standard unified diff format (`--- a/path` / `+++ b/path`, relative to project root)
- Minimum 3 context lines per hunk
- Multiple files can be included in a single patch step
- Step `name` is optional but recommended for clarity
- The changeset is applied with: `./pxp.sh changeset.yml`
