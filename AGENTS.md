When implementing changes or presenting a finalized plan for implementation, do not edit files directly. Instead, generate a single PXP changeset (a `.pxp.md` file) using the format below. A deterministic executor applies it with `pxp apply`.

Format:
````md
# PXP Changeset

pxp: 2
title: Changeset short description
intent: One to three sentences on why this change.

## Op 1: edit — path/to/file

SEARCH
````
verbatim block copied from the file, with enough surrounding lines to be unique
````
REPLACE
````
the new block
````

## Op 2: create — path/to/new/file

````
full file content
````

## Op 3: run — verify

````
npm test
````
expect_exit: 0
````

Op types: `edit` (SEARCH/REPLACE), `create` (full file), `delete — path`, `rename — old -> new`, `run` (shell command, optional `expect_exit: N`).

Rules:
- Copy SEARCH **verbatim** from the current file — never reconstruct from memory. Only trailing whitespace and CRLF are normalized for you.
- Give each SEARCH enough surrounding lines to be unique in the file (≥ 3 lines); duplicate matches fail with `E_AMBIGUOUS_MATCH`.
- Fence every payload with a backtick run longer than any run inside it, minimum 4 (use 5 if the code itself contains a 4-backtick run).
- One SEARCH/REPLACE pair per `edit` op — split distant edits into separate ops.
- Put `run` verification steps last.
- Paths are repo-relative. An empty REPLACE block deletes the matched region.
- The changeset is applied with: `pxp apply changeset.pxp.md` (add `--allow-run` when it contains `run` ops).
