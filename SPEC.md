
# PXP — Plan eXecution Protocol

- **Version:** 0.1.0
- **Status:** Draft
- **Author:** Otar Chekurishvili

## Abstract

PXP (Plan eXecution Protocol) is a specification for separating LLM-based code planning from deterministic code execution. The protocol defines a structured format for code change plans that can be executed by simple CLI tools without requiring LLM inference during execution.

## Background

Current LLM-based coding assistants use a ReAct-style loop where the LLM reasons, calls a tool, observes the result, and repeats. This approach has several drawbacks:

- **Token Inefficiency**: Full conversation history is sent with each LLM call
- **Latency**: Each tool use requires a round-trip to the LLM
- **Unpredictability**: The LLM may take different paths on the same task
- **Cost**: Complex tasks can consume hundreds of thousands of tokens

## The PXP Approach

PXP inverts this model:

1. **Plan**: LLM analyzes the task and generates a complete execution plan
2. **Execute**: A deterministic executor runs the plan without LLM involvement

For code editing tasks, the output is the file changes themselves, not a natural language synthesis. Once a plan is validated, execution is mechanical: apply patches, run commands, check exit codes. No intelligence is required during execution.

### Goals

- **Deterministic execution:** Given the same plan, execution produces identical results.
- **Token efficiency:** Eliminate LLM token usage during the execution phase.

### Non-Goals

- Defining how LLMs should generate plans (PXP only specifies the output format).
- Conflict resolution or merge strategies.
- PXP does not handle tasks requiring dynamic response to execution results.
- Open-ended analysis, tasks like "review this code" without concrete outputs are out of scope

## Terminology

- **Changeset:** A single `.yml` file conforming to this specification.
- **Step:** An atomic unit of work within a changeset. Either a `run` step or a `patch` step.
- **Executor:** A program that parses and executes a changeset file.

## Changeset Format

A changeset is a YAML file with the following structure:

```yaml
name: <optional changeset name>
steps:
  - name: <optional step name>
    run: <shell command>
  - name: <optional step name>
    patch: |
      <unified diff>
```

Shortened version without name fields:

```yaml
steps:
  - run: <shell command>
  - patch: |
      <unified diff>
```

### Fields

#### `name` (optional required)

A short, human-readable description of the changeset. Displayed by the Executor at the start of execution.

```yaml
name: Add authentication middleware and install dependencies
```

#### `steps` (required)

An ordered list of steps. Steps execute sequentially from top to bottom. Execution halts on the first failure.

A step consists of an optional `name` field (a short, human-readable description of the step) and a step type.

There are exactly two step types: `run` and `patch`.

### Step Types

#### `run`

Executes a shell command via the system shell.

```yaml
- run: npm install lodash
```

The Executor invokes the command using `eval`. The step succeeds if the command exits with code `0`. Any non-zero exit code is a failure.

#### `patch`

Applies a unified diff to one or more files using `git apply`.

```yaml
- patch: |
    --- a/src/utils.js
    +++ b/src/utils.js
    @@ -1,3 +1,5 @@
    +import abc from 'example';
    +
     export function doSomething(value) {
       return abc(value);
     }
```

The patch body is a YAML block scalar (indicated by `|`) indented under the step. The Executor strips the YAML indentation before passing the content to `git apply`.

Patch rules:

- Standard unified diff format with `--- a/path` and `+++ b/path` headers.
- All paths are relative to the repository root.
- Each hunk must include a minimum of 3 context lines.
- A single patch step may contain diffs for multiple files, concatenated sequentially.
- The Executor validates the patch with `git apply --check` before applying. If the check fails, the step fails.

### Multi-file patches

Multiple files can be modified in a single patch step by concatenating their diffs:

```yaml
- patch: |
    --- a/src/utils.js
    +++ b/src/utils.js
    @@ -1,3 +1,5 @@
    +import _ from 'lodash';
    +
     export function formatDate(date) {
       return date.toISOString();
     }
    --- a/config/app.json
    +++ b/config/app.json
    @@ -1 +1,3 @@
    -{}
    +{
    +  "debug": true
    +}
```

## Execution Semantics

1. The Executor reads the changeset file.
2. The Executor extracts and displays the `name` field if present.
3. The Executor iterates through `steps` in order.
4. For each step:
   - Display the `name` field if present.
   - `run`: Execute the command. If exit code ≠ 0, halt.
   - `patch`: Validate with `git apply --check`. If invalid, halt. Otherwise apply with `git apply`.
5. If all steps complete, the Executor exits with code `0`.
6. If any step fails, the Executor exits with a non-zero code. No subsequent steps execute.

### Guarantees

- Steps execute in strict declaration order.
- A failed step prevents all subsequent steps from executing.
- A patch is validated before it is applied (no partial application on check failure).
- The Executor does not modify the changeset file.

### Requirements

- The working directory must be inside a git repository.
- `git` must be available on `PATH`.
- `bash` must be available (version 3.2+).

## Example

```yaml
name: Add lodash utility and enable debug mode

steps:
  - name: Install package
    run: npm install lodash
  - name: Apply changes
    patch: |
      --- a/src/utils.js
      +++ b/src/utils.js
      @@ -1,3 +1,5 @@
      +import _ from 'lodash';
      +
       export function formatDate(date) {
         return date.toISOString();
       }
      --- a/config/app.json
      +++ b/config/app.json
      @@ -1 +1,3 @@
      -{}
      +{
      +  "debug": true
      +}
  - run: npx some-tool generate
  - name: Lint the JS files
    run: npm run lint
```

Execution:

```
▶ Changeset: Add lodash utility and enable debug mode

[1] run: Install package
  ✓ Done
[2] patch: Apply changes
  ✓ Done
[3] run: npx some-tool generate
  ✓ Done
[4] run: Lint the JS files
  ✓ Done

✓ All 4 steps completed.
```

## LLM Integration

PXP is designed to be generated by LLMs as part of a code planning workflow. The following instruction, placed in a `AGENTS.md`/`CLAUDE.md` file or equivalent system prompt, directs an LLM to produce PXP changesets:

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

## File Extension

PXP changesets use the `.yml` extension. The recommended default filename is `changeset.yml`.

## Versioning

This specification follows semantic versioning. The version is declared in the document header. Executors should not enforce version checks at this stage.
