---
name: pxp
description: Create a complete, reviewable PXP plan for a coding task without modifying source files or executing the plan.
---

# Create a closed PXP plan

Use this skill when the user wants Codex to investigate a repository and compile the requested change into `plan.pxp.yaml` for deterministic execution by PXP.

1. Read the complete request, every applicable `AGENTS.md`, and the relevant repository code, tests, CI, and documentation. Ask only questions whose answers materially change the implementation.
2. Do not modify source files. The only file you may create or edit is `plan.pxp.yaml` at the Git worktree root.
3. Resolve the worktree root and exact commit with `git rev-parse --show-toplevel` and `git rev-parse --verify 'HEAD^{commit}'`.
4. Emit one complete PXP 1.1 document. It must contain every source change as a standard unified Git diff in a `patch` step and every non-interactive verification command in a `run` step. Do not defer decisions, leave placeholders, add retries, or include any instruction to call an LLM.
5. Bind the plan to the exact commit from step 3:

   ```yaml
   pxp_version: "1.1"
   name: One-line task description
   repository:
     type: git
     head: 0123456789abcdef0123456789abcdef01234567
   steps:
     - name: Apply source changes
       patch: |
         diff --git a/path/to/file b/path/to/file
         --- a/path/to/file
         +++ b/path/to/file
         @@ -1 +1 @@
         -old
         +new
     - name: Verify
       run:
         - command that exits non-zero on failure
   ```

6. Report the repository HEAD, plan path, and a literal command of the form `pxp run plan.pxp.yaml`.
7. Stop. Do not run PXP, apply the diff, execute plan steps, or continue with an automatic fix. The human must review the complete file before execution. If PXP later stops, a new planning turn and a newly reviewed plan are required.

PXP execution is deterministic control flow, not a guarantee that arbitrary shell commands or external systems behave deterministically. Earlier mutations remain if a later step fails.
