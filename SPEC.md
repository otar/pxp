# PXP — Plan eXecution Protocol

- **Version:** 2.0.0
- **Status:** Normative
- **Author:** Otar Chekurishvili

## Abstract

PXP separates LLM-based code *planning* from deterministic code *execution*. A
model emits a **changeset** describing a set of file operations; a deterministic
executor applies it with zero further model calls. PXP v2 uses a fenced-Markdown
carrier with `SEARCH`/`REPLACE` blocks — a format frontier models emit reliably
— and applies it with a whitespace-normalized exact matcher that fails loudly
rather than ever misapplying.

This is a breaking change from v1 (YAML + unified diffs). There is no
compatibility path.

## Design priorities (in order)

1. Frontier-model emission reliability.
2. Impossibility of silent misapplication — failing to apply is acceptable;
   applying to the wrong place is not.
3. Node parsing simplicity.
4. Token compactness.

## Terminology

- **Changeset** — a single `.pxp.md` file conforming to this spec.
- **Op** — one operation: `edit`, `create`, `delete`, `rename`, or `run`.
- **Executor** — a program that parses and applies a changeset (the `pxp` CLI).

## Carrier: fenced Markdown

A changeset is Markdown-carried, not YAML/JSON, because arbitrary source code
(tabs, backticks, mixed indentation) must survive verbatim. Every code payload
is wrapped in a fence of backticks whose length is **longer than the longest
backtick run inside the payload, minimum 4**. The parser detects fence length
from the opening fence and scans to a closing run of at least that length at the
start of a line. This makes the format safe for payloads that themselves contain
fenced code.

The parser is a hand-rolled, single-pass, line-based reader of this dialect — not
a general Markdown parser.

## Document structure

```
# PXP Changeset

pxp: 2
title: <one line>
intent: <1–3 sentences, free prose>

## Op 1: edit — path/to/file

<optional rationale prose>

SEARCH
<fence>
<verbatim block copied from the file>
<fence>
REPLACE
<fence>
<the new block>
<fence>

## Op 2: create — path/to/new

<fence>
<full file content>
<fence>

## Op 3: rename — old/path -> new/path

## Op 4: delete — path/to/remove

## Op 5: run — label

<fence>
shell command
<fence>
expect_exit: 0
```

### Header

Line-based `key: value` pairs before the first op. Recognized keys: `pxp`
(must be `2`), `title`, `intent`. Unknown keys produce a validation warning.

### Op headers

`## Op N: <type> — <path>`. The separator may be an em-dash (—), en-dash (–), or
hyphen (-). For `rename`, the path is `<from> -> <to>` (arrow `->` or `→`). The
heading level and the `N` are advisory; ops apply in document order.

### Op payloads

- `edit` — exactly one `SEARCH` fenced block then one `REPLACE` fenced block.
  The keyword lines are bare (`SEARCH`) or heading-prefixed (`### SEARCH`).
- `create` — one fenced block: the full file content.
- `run` — one fenced block (the command) and an optional `expect_exit: N`
  (default `0`).
- `delete`, `rename` — no payload.

Prose between an op header and its first fence is rationale: preserved for the
report, ignored by the executor.

## Matching semantics (`edit`)

For each `edit`, in order:

1. Read the target file. Missing → `E_FILE_NOT_FOUND`.
2. Normalize both file and SEARCH: CRLF→LF, strip trailing whitespace per line.
   Leading whitespace is **not** normalized yet.
3. **Pass 1 — byte-exact** (post-normalization): count occurrences of SEARCH.
   - Exactly 1 → apply.
   - More than 1 → `E_AMBIGUOUS_MATCH` (all locations reported).
   - Zero → Pass 2.
4. **Pass 2 — indent-normalized:** strip the common leading-whitespace prefix
   from SEARCH and from each candidate window, then compare. On a unique match,
   apply REPLACE re-based to the window's indentation (REPLACE's own base indent
   is stripped and the window's prefix prepended, preserving REPLACE's internal
   relative indentation). Recorded as `fuzz: indent`.
   - More than 1 → `E_AMBIGUOUS_MATCH`. Zero → `E_NO_MATCH`.
5. There is **no Pass 3.** No similarity/fuzzy/edit-distance matching exists.
   `--strict` disables Pass 2 (byte-exact only).

An empty REPLACE block deletes the matched region. Multiple edits on one file
apply top-to-bottom, each seeing the previous op's result. REPLACE bytes are
written verbatim (trailing whitespace preserved); normalization only ever affects
comparison, never written content.

## Other ops

- **create** — fails if the path exists (`E_EXISTS`); creates parent dirs;
  content written with LF endings.
- **delete** — fails if missing; stages the deletion (`git rm`).
- **rename** — `git mv`; fails if source missing or dest exists.
- **run** — executed via `bash -c` at the repo root, inheriting env and streaming
  output. A non-matching exit code fails the op. Because `run` executes arbitrary
  commands, `pxp apply` refuses run ops unless `--allow-run` is given.

## Execution flow

1. **Preflight** — must be inside a git repo with a clean working tree
   (`git status --porcelain` empty, ignoring the changeset file itself), else
   `E_DIRTY_TREE`. Record `HEAD` as the checkpoint.
2. **Dry-run** — run the full matching algorithm for every file op against the
   working tree before any write. Any error aborts with zero writes.
3. **Apply** — execute ops in order; file ops write and stage, `run` ops execute.
4. **On failure** — reset the tree hard to the checkpoint and remove created
   files (`--keep-on-fail` disables this). `run` side effects outside the repo
   cannot be rolled back and are reported.
5. **On success** — commit `pxp: <title>` with `intent` in the body and a
   `PXP-Changeset: <sha256 of the changeset file>` trailer (`--no-commit` leaves
   changes staged).

Convention: put `run` ops last. File ops are dry-run-verifiable; commands are not.

## Error codes

`E_FILE_NOT_FOUND`, `E_NO_MATCH`, `E_AMBIGUOUS_MATCH`, `E_EXISTS`,
`E_DIRTY_TREE`, `E_NOT_REPO`, `E_RUN_FAILED`, `E_RUN_NOT_ALLOWED`.

## Requirements

- Node.js ≥ 20, `git` on `PATH`, `bash` for `run` ops (POSIX only).

## File extension

Changesets use `.pxp.md`. Recommended default filename: `changeset.pxp.md`.
