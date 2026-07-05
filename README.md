
> [!CAUTION]
> This project is in an **experimental** stage and under active development. APIs, features, and the changeset format may change without notice. Please report any issues you encounter.

<img src="logo.png" width="400">

# PXP — Plan eXecution Protocol

**Your agent already figured out the change. Let a deterministic tool apply it — exactly, or not at all.**

---

## The idea

By the time an AI agent has read your code, asked its questions, and planned the
change, it already knows what to do: which files, which lines, which commands.
PXP captures that plan as a **changeset** and applies it with a small,
deterministic CLI. No model runs during execution.

```
Analyze codebase ──→ Plan changes ──→ Execute plan
    (needs a model)    (needs a model)   (deterministic — pxp)
```

The reasoning is the hard part. Applying the result is mechanical — and mechanical
work should be exact, reviewable, and reversible. That is what PXP is for.

## Safety first, by construction

PXP's first commitment is that a changeset **never applies to the wrong place**.

- **Whitespace-normalized exact matching, and nothing fuzzier.** An `edit` finds
  its target by exact match (ignoring trailing whitespace and line endings), then
  by an indentation-normalized exact match. There is no similarity threshold, no
  edit-distance fallback, no "closest guess." If the target isn't unambiguously
  there, the apply **fails loudly** and hands back the file's real surrounding
  lines so the model can correct itself.
- **Ambiguity is an error.** If a SEARCH block matches two places, PXP stops and
  tells you — it never picks one.
- **Atomic, git-checkpointed apply.** The whole changeset is dry-run-verified
  before a single byte is written. Any failure rolls the tree back to where it
  started. You end up either fully applied or exactly as you were.
- **Runs are opt-in.** A changeset can execute commands (that's how it runs your
  tests). `pxp apply` refuses to run them unless you pass `--allow-run`.

Failing to apply is fine — you just fix the changeset and re-run. Applying to the
wrong place is not, and PXP is built so it can't.

## What a changeset looks like

````md
# PXP Changeset

pxp: 2
title: Add a request-rate limit
intent: Cap API requests per client using a shared limiter.

## Op 1: create — src/config/limits.js

````
export const MAX_PER_MINUTE = 100;
````

## Op 2: edit — src/routes/api.js

SEARCH
````
const router = express.Router();
````
REPLACE
````
const router = express.Router();
router.use(limiter);
````

## Op 3: run — verify

````
npm test
````
````

Five op types — `edit`, `create`, `delete`, `rename`, `run` — carried in fenced
Markdown so that arbitrary source code (tabs, backticks, mixed indentation)
survives verbatim.

## Use it

```bash
npx pxp validate changeset.pxp.md   # parse + static checks
npx pxp plan     changeset.pxp.md   # dry-run against your tree, no writes
npx pxp apply    changeset.pxp.md   # apply atomically, commit on success
```

```
$ pxp plan changeset.pxp.md
PXP: Add a request-rate limit

  + create src/config/limits.js
  ~ edit   src/routes/api.js (1 match)
  ▶ run    verify (deferred)

Plan: 1 to create, 1 to change, 1 to run

┌─ run steps (require --allow-run to apply) ─────────
│ npm test
└──────────────────────────────────────────

Dry run only — no files were changed.
```

`plan` is your review artifact — read it, then `apply --allow-run` when you're
happy. On success PXP commits `pxp: <title>` with a `PXP-Changeset:` trailer.

### Flags

| Flag | Effect |
|---|---|
| `--strict` | Byte-exact matching only (disable the indent-normalized pass) |
| `--allow-run` | Approve `run` ops non-interactively |
| `--json <path>` | Write the machine-readable report / repair packet |
| `--keep-on-fail` | Leave partial state on failure instead of rolling back |
| `--no-commit` | Stage changes instead of committing (apply only) |
| `--no-color` | Disable ANSI color |

## When an edit doesn't apply

PXP tells you exactly where it looked and what it found:

```
  ✗ edit src/calc.js — E_NO_MATCH
      file content differs from SEARCH at 1 line(s); nearest window at line 1:
      >    1  const total = items.sum();
           2  return total;
```

With `--json`, the same information is emitted as a compact **repair packet**
(capped for token budget) designed to be pasted straight back to the planning
model, which then emits a corrected changeset for just the failed op.

## Honest token economics

The savings are on the **execution loop**, not the thinking. A model still reads
and plans using tokens; PXP removes the part after that — the tool-call
round-trips where the model re-emits edits, reads back results, and narrates its
own typing. Prompt caching already discounts re-reads, so treat PXP as a
determinism-and-reviewability tool that also trims execution overhead, not as a
magic token coupon.

## Tell your agent to use it

Add the contract to your `CLAUDE.md`, `AGENTS.md`, or system prompt — or pipe it
in directly:

```bash
claude --append-system-prompt "$(pxp prompt)"
```

`pxp prompt` prints [`PROMPT.md`](PROMPT.md), the normative planner contract. A
reference Claude Code integration lives in
[`integrations/claude-code/`](integrations/claude-code/).

## Project

| File | Description |
|---|---|
| [`bin/pxp.js`](bin/pxp.js) | CLI entry point |
| [`src/`](src/) | Parser, matcher, executor, reports (zero runtime deps) |
| [`SPEC.md`](SPEC.md) | Normative format specification |
| [`PROMPT.md`](PROMPT.md) | Planner prompt contract (`pxp prompt`) |
| [`CLAUDE.md`](CLAUDE.md) · [`AGENTS.md`](AGENTS.md) | Drop-in agent instructions |
| [`test/`](test/) | `node:test` suite incl. adversarial fixtures |

## FAQ

**What if the code changed since the plan was written?**
The `edit` fails with `E_NO_MATCH` and a repair packet. Nothing is partially
applied. You regenerate the changeset (or just the failed op) and re-run.

**Does this work with Claude Code / Cursor / Copilot / Aider?**
Any model that can emit fenced Markdown can produce a PXP changeset. Add the
contract from `pxp prompt` to its system prompt.

**Is this like Terraform plan/apply?** Same philosophy, much smaller scope:
plan, review, apply, deterministic every time.

---

MIT licensed. Happy prompting — may your tokens be well spent.

[Otar](https://otar.me)
