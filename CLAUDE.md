# PXP instructions

For implementation work, analyze the repository and ask only material clarification questions. Do not edit code directly. Produce a complete `plan.pxp.yaml` using protocol `1.1`, with top-level `pxp_version`, `name`, `repository`, and `steps`. Bind `repository.type` to `git` and `repository.head` to the full output of `git rev-parse --verify 'HEAD^{commit}'`.

Each step has an optional `name` and exactly one operation: `run` as a sequence of non-interactive Bash command strings, or `patch` as a unified Git diff. Use repository-relative paths, include verification steps, and keep the plan reviewable. Do not include secrets.

Report the plan path and bound repository HEAD, but do not execute the plan. Finish with: `Plan saved to plan.pxp.yaml. Review and execute with pxp run plan.pxp.yaml`.
