<p align="center">
  <img src="logo.png" alt="PXP logo" width="520">
</p>

# PXP: Plan eXecution Protocol

> Plan once. Review once. Run without waiting on the model.

PXP is a local executor for coding-agent plans. Your agent writes one YAML file with all patches and commands. You review that file and approve its exact SHA-256. PXP then verifies the plan and the expected Git commit before it applies the steps.

After approval, one local command starts the work. PXP does not pause between steps to ask a model what to do next. It makes no model calls during execution.

This can shorten the time between an approved plan and a completed change. It can also reduce token use. Actual savings depend on the task and whether a failed plan needs a new planning pass.

[Get started](#quick-start) or [read how PXP works](#how-it-works).

## Why use PXP?

A coding agent often plans and changes a workspace in the same session. It can send each result back to the model, wait for a reply, and then select the next action. This loop is useful while the agent explores a problem. It adds delay and token use when the plan is already complete.

PXP separates planning from execution. The model can reason while it creates the plan. After you approve that plan, a local process follows the fixed steps.

### Start as soon as you approve

Run one command after you review the plan. PXP checks its preconditions and starts the work. It does not wait for model replies between commands or patches.

The commands can still take time. PXP does not claim that every plan finishes instantly.

### Use fewer model tokens

PXP does not call a model while it executes a plan. This can use fewer tokens than an agent that plans, executes, inspects each result, and calls the model again.

If a plan fails, PXP stops. You can then inspect the workspace and create a new plan. A new planning pass uses more time and tokens, so the actual savings vary.

### Approve the complete change

A PXP plan contains the patches and shell commands that will run. You can inspect the whole change before the first step starts.

PXP can bind approval to two exact values:

- the SHA-256 of the plan file;
- the Git commit used during planning.

If either value changes, PXP stops before it runs a plan step.

## How it works

```text
CODING AGENT                 YOU                         PXP

inspect the repository       review the complete plan    verify the plan hash
write plan.pxp.yaml    ->    inspect every step     ->   verify the Git commit
stop planning                approve one SHA-256          apply steps in order
                                                          make no model calls
```

The agent is present during planning. It is not present during execution. PXP does not adapt the plan after it starts.

## Quick start

### 1. Check the requirements

PXP supports:

- macOS 12 or later;
- Linux with kernel 3.2 or later;
- Linux under WSL.

You also need Git 2.24 or later and Bash 3.2 or later. Native Windows shells are not supported.

### 2. Install PXP

Download the installer from the latest GitHub Release. Read it before you run it.

```sh
curl -fsSLO https://github.com/otar/pxp/releases/latest/download/install.sh
less install.sh
bash install.sh
```

The installer verifies the release checksum and installs `pxp` into `~/.local/bin` by default. See the [installation guide](docs/installation.md) for pinned versions, custom locations, upgrades, and removal.

To build from the current source instead, use Go 1.25 or later:

```sh
make build
./bin/pxp version
```

### 3. Connect your coding agent

PXP includes instruction-only skills for Codex and Claude Code. These commands install a skill for your user from this repository checkout.

For Codex:

```sh
mkdir -p "$HOME/.agents/skills/pxp"
cp integrations/codex/skills/pxp/SKILL.md "$HOME/.agents/skills/pxp/SKILL.md"
```

For Claude Code:

```sh
mkdir -p "$HOME/.claude/skills/pxp"
cp integrations/claude-code/skills/pxp/SKILL.md "$HOME/.claude/skills/pxp/SKILL.md"
```

See [agent integrations](docs/integrations.md) for repository-level installation and more details.

### 4. Ask the agent for a plan

In Codex:

```text
$pxp Implement the requested change and compile it into a complete PXP plan.
```

In Claude Code:

```text
/pxp Implement the requested change and compile it into a complete PXP plan.
```

The skill lets the agent inspect the repository. The agent writes only `plan.pxp.yaml`, reports its hash, and stops. It does not edit source files or run PXP.

### 5. Review and run

Read the complete plan. Check every patch and command.

```sh
less plan.pxp.yaml
```

Calculate the SHA-256 yourself after the review:

```sh
# Linux
sha256sum plan.pxp.yaml

# macOS
shasum -a 256 plan.pxp.yaml
```

Copy the reviewed hash into the command as a fixed value:

```sh
pxp run --expect-sha256 COPY_THE_REVIEWED_64_CHARACTER_HASH_HERE plan.pxp.yaml
```

Do not calculate the hash inside the `pxp run` command. Approval and execution must remain separate.

PXP checks the hash before it opens the repository. It checks the expected Git commit while it holds the worktree lock. A mismatch stops execution before the first plan step.

## Plan format

Agent integrations create PXP 1.1 plans for you. A plan has a name, an exact Git commit, and an ordered list of steps.

Use the complete output of `git rev-parse --verify 'HEAD^{commit}'` for `repository.head`.

```yaml
pxp_version: "1.1"
name: Add a greeting
repository:
  type: git
  head: 0123456789abcdef0123456789abcdef01234567
steps:
  - name: Add the file
    patch: |
      diff --git a/hello.txt b/hello.txt
      new file mode 100644
      --- /dev/null
      +++ b/hello.txt
      @@ -0,0 +1 @@
      +hello from PXP
  - name: Verify the file
    run:
      - test "$(cat hello.txt)" = "hello from PXP"
```

Each step contains one patch or a list of shell commands. PXP executes steps in document order and stops at the first failure.

## What PXP checks

Before the first plan step, PXP:

- reads and validates the complete plan;
- compares the plan with the approved SHA-256 when you use `--expect-sha256`;
- verifies the exact Git commit for a PXP 1.1 plan;
- obtains a lock for the current worktree;
- rejects tracked workspace changes unless you use `--allow-dirty`;
- checks patch structure, paths, and file types.

PXP runs each command in a fresh, strict, non-interactive Bash process at the worktree root. State such as `cd` or a shell variable does not continue into the next command. File changes do continue.

PXP itself makes no model call, update check, telemetry request, DNS lookup, or other network request during execution. A command in a trusted plan can use the network.

## Trust model

> [!WARNING]
> A PXP plan is a trusted program. It can run arbitrary shell commands with your user permissions. Review every plan before you run it.

PXP is an executor, not a sandbox. It does not make unsafe commands safe. Plan commands inherit most of your environment and can read files, print secrets, or contact external services.

PXP does not roll back. If a later step fails, earlier commands and patches remain. Commit or stash important work before execution. Use `--allow-dirty` only when you understand how the plan interacts with existing changes.

The plan hash identifies the exact approved file. Git binding identifies the expected starting commit. These checks do not freeze untracked files, external services, time, network responses, or changes from unrelated processes.

Follow the [security policy](SECURITY.md) and the [recovery guide](docs/recovery.md) for more information.

## CLI reference

```text
pxp run [--allow-dirty] [--expect-sha256 HASH] <plan.pxp.yaml|->
pxp help [run|version]
pxp version
pxp -h
pxp --help
pxp --version
```

Use `-` to read a plan from standard input. A plan file must be a regular file and must not be a symbolic link. Child commands do not receive the plan stream as interactive input.

Lifecycle messages go to standard error. Command output passes through without reformatting. PXP reports command numbers but does not print command contents.

| Exit code | Meaning |
|---:|---|
| `0` | Success |
| `2` | CLI usage error |
| `3` | Input, schema, or patch-policy error |
| `4` | Environment or repository precondition error |
| `5` | Command or patch execution failure |
| `70` | Internal error |
| `130` | Interrupted by `SIGINT` |
| `143` | Terminated by `SIGTERM` |

## Learn more

- [Installation and upgrades](docs/installation.md)
- [Codex and Claude Code integrations](docs/integrations.md)
- [PXP specification](SPEC.md)
- [PXP 1.1 JSON Schema](schema/pxp-v1.1.schema.json)
- [Troubleshooting](docs/troubleshooting.md)
- [Recovery after a failed plan](docs/recovery.md)
- [Benchmark framework](benchmark/README.md)
- [Security policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)
- [Release process](RELEASING.md)

The benchmark framework compares direct agent execution with PXP execution on reproducible fixtures. Its results apply only to the tested task, agent, model, and environment.

PXP is available under the [MIT License](LICENSE).
