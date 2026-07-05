---
name: pxp
description: Emit a PXP changeset (.pxp.md) instead of editing files directly, then apply it deterministically with `pxp apply`. Use when implementing a planned code change in a git repo where PXP is set up.
---

# PXP changeset skill

This skill makes Claude Code produce a **PXP changeset** — a single `.pxp.md`
file describing the change — instead of editing files with the Edit tool. A
deterministic executor (`pxp apply`) then applies it: whitespace-normalized exact
matching, atomic git-checkpointed apply, and a loud failure with a repair packet
if anything doesn't match. Nothing is ever applied to the wrong place.

## When to use

- The repository has `pxp` available (`npx pxp` or a global install) and a git
  working tree.
- The user asks you to implement a change and prefers a reviewable, reversible
  changeset over direct edits.

## Workflow

1. Read the files you intend to change so your SEARCH blocks are **verbatim**.
2. Write a changeset to `changeset.pxp.md` following the format in
   [`PROMPT.md`](../../PROMPT.md) (run `pxp prompt` to print it). Keep the five
   hard rules in mind: verbatim SEARCH, unique context, fence length > any inner
   backtick run (min 4), one SEARCH/REPLACE pair per op, `run` steps last.
3. Dry-run it: `pxp plan changeset.pxp.md`. Show the plan to the user.
4. Apply it: `pxp apply changeset.pxp.md` (add `--allow-run` if it contains
   `run` ops). On `E_NO_MATCH` / `E_AMBIGUOUS_MATCH`, read the repair packet
   (`--json`), correct the failed op against the file's actual lines, and re-run.

## Contract

The normative format is [`PROMPT.md`](../../PROMPT.md). Append it to the system
prompt for a one-line integration:

```bash
claude --append-system-prompt "$(pxp prompt)"
```
