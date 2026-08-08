# PXP instructions

For implementation work, analyze the repository and ask only material clarification questions. Do not edit code directly. Produce a complete `plan.pxp.yaml` using protocol `1.0`, with top-level `pxp_version`, `name`, and `steps`.

Each step has an optional `name` and exactly one operation: `run` as a sequence of non-interactive Bash command strings, or `patch` as a unified Git diff. Use repository-relative paths, include verification steps, and keep the plan reviewable. Do not include secrets.

Finish with: `Plan saved to plan.pxp.yaml. Review and execute with pxp run plan.pxp.yaml`.
