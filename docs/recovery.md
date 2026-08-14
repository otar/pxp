# Recovery after a failed plan

PXP stops at the first failure and does not roll back. The failure message identifies the completed and failing step.

1. Preserve command output, the reported plan hash, and the bound repository HEAD.
2. Inspect `git status --short` and `git diff` from the worktree root.
3. Determine which non-Git side effects completed: generated files, installed dependencies, network operations, or external services.
4. Restore or commit changes deliberately with normal project tools. Do not blindly rerun a non-idempotent plan.
5. Correct the plan or workspace in a separate planning phase, review the entire replacement plan and its new exact hash, and run again only when the preconditions are understood. Do not let an agent adapt or retry inside the execution phase.

PXP does not delete its lock file, but the operating-system lock is released when the process exits. A stale file with no held lock is harmless. If another live PXP process owns the lock, wait for it or terminate it normally; never remove the lock to bypass a live executor.
