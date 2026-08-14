# Claude Code and Codex integrations

PXP's integrations are instruction-only skills. They keep vendor-specific agent behavior outside the executor: the agent investigates and writes one closed plan, then stops; the `pxp` binary performs execution without an agent or model.

The adapters follow the current official extension mechanisms: [Claude Code skills](https://code.claude.com/docs/en/slash-commands) and [Codex skills](https://learn.chatgpt.com/docs/build-skills). The repository files are intentionally small enough to review before installation.

## Claude Code

Install for your user from a PXP checkout:

```sh
mkdir -p "$HOME/.claude/skills/pxp"
cp integrations/claude-code/skills/pxp/SKILL.md "$HOME/.claude/skills/pxp/SKILL.md"
```

For one repository instead, copy it to `.claude/skills/pxp/SKILL.md` in that repository. Start Claude Code there and invoke the skill explicitly:

```text
/pxp Implement the requested change and compile it into a complete PXP plan.
```

Claude Code may read files and run read-only investigation commands. The skill instructs it to write only `plan.pxp.yaml`; it must not change source files or run PXP. The skill pre-approves only read-only Git commands, so the plan-file write still follows the user's normal permission policy.

## Codex

Install for your user from a PXP checkout:

```sh
mkdir -p "$HOME/.agents/skills/pxp"
cp integrations/codex/skills/pxp/SKILL.md "$HOME/.agents/skills/pxp/SKILL.md"
```

For one repository instead, copy it to `.agents/skills/pxp/SKILL.md` in that repository. Codex automatically discovers repository and user skills. Invoke it explicitly:

```text
$pxp Implement the requested change and compile it into a complete PXP plan.
```

The skill complements normal `AGENTS.md` discovery; repository instructions still apply. It limits its mutation to `plan.pxp.yaml` and must not execute the plan.

## Review and apply

Both skills produce PXP 1.1 and report:

- `repository.head`: the full `HEAD^{commit}` used during planning;
- the path to the saved plan.

Review the complete file:

```sh
less plan.pxp.yaml
```

Then run it:

```sh
pxp run plan.pxp.yaml
```

PXP validates the complete plan and checks Git HEAD while holding the worktree lock before any step. A mismatch stops with exit 4.

If a plan step fails, PXP stops and earlier mutations remain. Return to a separate planning phase, produce a new complete plan, and review it; do not ask the agent to repair or continue during execution.

This boundary guarantees the initial Git HEAD, not deterministic results from arbitrary shell commands. Commands can still observe clocks, files outside Git, environment variables, networks, package registries, or concurrently changing external systems.

## Optional exact plan pinning

When review and execution are separated by time, tooling, or people, `--expect-sha256` can require the exact reviewed plan bytes. Calculate the hash after reviewing the file:

```sh
# Linux
sha256sum plan.pxp.yaml

# macOS
shasum -a 256 plan.pxp.yaml
```

Pass the reviewed hash as a fixed literal:

```sh
pxp run --expect-sha256 COPY_THE_REVIEWED_64_CHARACTER_HASH_HERE plan.pxp.yaml
```

Compute the hash after review and do not recompute it inline in the run command; inline computation does not preserve a separate approval boundary.

The hash pins exact plan identity but does not authenticate the planner or reviewer. Human review remains the source of trust.
