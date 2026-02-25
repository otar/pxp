
<img src="logo.png" width="400">

# PXP — Plan eXecution Protocol

**Your agent already figured it out. Stop feeding tokens to `cat`-ing and `ls`-ing.**

---

## Why

By the time your AI agent has analyzed the codebase, asked clarifying questions, and planned the changes, it already knows what needs to happen. Which files, which lines, which commands.

So why does it then spend 50,000 more tokens executing those changes one tool call at a time? It re-reads files it already understood. It feeds each result back into context. You watch a spinner and your rate limit evaporates.

```
Analyze codebase ──→ Plan changes ──→ Execute plan
    (needs LLM)        (needs LLM)      (doesn't need LLM)
```

The reasoning *is* the hard part. Execution is mechanical. PXP makes that split explicit: the LLM outputs a changeset, a dumb bash script applies it. Zero tokens on execution. Zero waiting.

## What it looks like

```yaml
name: Add rate limiting to API routes

steps:
  - name: Install dependencies
    run: npm install express-rate-limit

  - name: Add rate limiter middleware
    patch: |
      --- a/src/middleware/index.js
      +++ b/src/middleware/index.js
      @@ -1,4 +1,6 @@
      +import rateLimit from 'express-rate-limit';
      +
       import { auth } from './auth.js';
       import { cors } from './cors.js';

      -export { auth, cors };
      +export const limiter = rateLimit({ windowMs: 15 * 60 * 1000, max: 100 });
      +export { auth, cors };

  - name: Apply to routes
    patch: |
      --- a/src/routes/api.js
      +++ b/src/routes/api.js
      @@ -1,5 +1,7 @@
       import express from 'express';
      -import { auth } from '../middleware/index.js';
      +import { auth, limiter } from '../middleware/index.js';

       const router = express.Router();
      +router.use(limiter);

  - name: Verify
    run: npm test
```

YAML steps. Standard unified diffs. Shell commands. Nothing you don't already know.

## Run it

```bash
./pxp.sh changeset.yml
```

```
▶ Changeset: Add rate limiting to API routes

[1] run: Install dependencies
  ✓ Done
[2] patch: Add rate limiter middleware
  ✓ Done
[3] patch: Apply to routes
  ✓ Done
[4] run: Verify
  ✓ Done

✓ All 4 steps completed.
```

Instant. No LLM in the loop. Fails fast: if step 2 breaks, step 3 never runs. Patches are validated with `git apply --check` before touching your files.

## Where your tokens actually go

Here's a typical Claude Code session that edits 5 files:

```
[LLM call]  Read file A                           →  3,200 tokens
[LLM call]  Read file B                           →  4,100 tokens
[LLM call]  Read file C                           →  2,800 tokens
[LLM call]  Plan changes                          →  1,500 tokens
[LLM call]  Edit file A, read result              →  6,400 tokens  ← unnecessary
[LLM call]  Edit file B, read result              →  7,200 tokens  ← unnecessary
[LLM call]  Edit file C, read result              →  5,800 tokens  ← unnecessary
[LLM call]  Run npm install, read output          →  3,100 tokens  ← unnecessary
[LLM call]  Run tests, read output                →  4,500 tokens  ← unnecessary
```

The LLM understood everything it needed by step 4. Steps 5 through 9 are the model narrating its own typing. Each call carries the full conversation history. The context window fills with tool results the model already anticipated.

With PXP:

```
[LLM call]  Read, analyze, plan                   → 11,600 tokens
[LLM call]  Generate changeset.yml                →  1,500 tokens
[executor]  Apply all changes                     →      0 tokens
```

Same result. Fraction of the cost.

## Why this is better

The LLM already did the thinking. Applying a patch doesn't require intelligence, and `git apply` does it for free.

Your context window stays clean, too. Every tool call result goes back into context. A single `cat src/big-file.ts` eats 2,000 tokens that the model carries for the rest of the session. With PXP, the window stays focused on reasoning instead of echoing file contents the model already read.

Same changeset, same result. Always. The agent doesn't take a different path each time, and you don't end up with partial edits from a model that got confused halfway through.

You also get to review before anything happens. The changeset *is* the review artifact. You read the diffs, you see the full sequence, you run it or you don't. Nothing touches your code until you say so.

And the patches are just patches. Standard unified diffs, the same format `git diff` produces. If you can read a pull request, you can read a changeset.

## Setup

### 1. Get the executor

```bash
curl -o pxp.sh https://raw.githubusercontent.com/otar/pxp/main/pxp.sh
chmod +x pxp.sh
```

Single bash script. No dependencies beyond `git` and `bash 3.2+`. Works on Linux, macOS, and Windows (Git Bash).

### 2. Tell your AI to use it

Add this to your `CLAUDE.md`, `AGENTS.md`, or system prompt:

```markdown
When implementing changes or presenting a finalized plan for implementation, do not edit files directly. Instead, generate a single `changeset.yml` changeset file using the PXP format.

Format:
  name: Changeset short description
  steps:
    - name: Step short description
      run: shell commands for dependencies, tooling, verification
    - name: Step short description
      patch: | unified diffs for code changes

Rules:
- Steps execute sequentially: `run` for shell commands, `patch` for unified diffs
- Patches use standard unified diff format, paths relative to project root
- Minimum 3 context lines per hunk
- Multiple files can be included in a single patch step
- The changeset is applied with: ./pxp.sh changeset.yml
```

### 3. Work normally, execute instantly

```
You:    "Refactor the auth module to use JWT"
AI:     *analyzes codebase, asks questions, iterates on the plan*
AI:     *generates changeset.yml*
You:    *reviews the changeset*
You:    ./pxp.sh changeset.yml   ← instant, zero tokens
```

You still iterate. You still plan. The AI still does the hard work of understanding your code. It just stops burning tokens on the part that doesn't need a brain.

## Format reference

```yaml
name: Short description          # optional

steps:
  - name: Step description       # optional
    run: npm install express      # shell command

  - name: Step description       # optional
    patch: |                      # unified diff (one or more files)
      --- a/src/file.js
      +++ b/src/file.js
      @@ -1,3 +1,4 @@
      +import foo from 'foo';
       const bar = 1;
```

Two step types. That's the whole spec. [Read the full specification →](SPEC.md)

## FAQ

**Does this work with Claude Code / Cursor / Copilot / Aider?**
Yes. Any LLM that can output YAML and unified diffs can produce PXP changesets. You just add the instruction to your system prompt or project config.

**What about changes that need runtime feedback?**
Use PXP for the parts that don't. Most implementation work (installing deps, applying patches, running codegen, linting) is predictable. If the AI genuinely needs to react to command output mid-execution, that's still a ReAct loop.

**What if a patch doesn't apply?**
`git apply --check` runs before applying. If the code changed since the plan was generated, it fails immediately with a clear error. Nothing gets partially applied.

**Why not just `git apply` directly?**
Because your changeset isn't just patches. It's `npm install`, then patches, then `npx codegen`, then `npm test`, all sequenced in one reviewable file.

**Is this like Terraform plan/apply?**
Same philosophy, much smaller scope. PXP is specifically for AI-generated code changes. Two step types, standard diffs, plain YAML.

## Project

| File | Description |
|---|---|
| [`pxp.sh`](pxp.sh) | The executor (single bash script, no dependencies) |
| [`SPEC.md`](SPEC.md) | Full format specification |
| [`test_pxp.sh`](test_pxp.sh) | Test suite (17 tests, 59 assertions) |
| [`example-changeset.yml`](example-changeset.yml) | Example changeset |
| [`CLAUDE.md`](CLAUDE.md) | Drop-in instructions for Claude Code |
| [`AGENTS.md`](AGENTS.md) | Drop-in instructions for AI Agents |

---

Happy prompting! May your tokens be well-spent!

[Otar](https://otar.me)
