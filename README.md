# PXP — Plan eXecution Protocol

PXP turns a reviewed YAML plan into a deterministic sequence of shell commands and Git patches. Planning stays with the coding agent; approval locks the exact plan bytes and Git commit; execution is performed by a small, auditable CLI with no model calls, telemetry, or automatic network access.

> [!WARNING]
> A PXP plan can run arbitrary shell commands with your user permissions. Read every plan and use PXP only with plans you trust. PXP is an executor, not a sandbox.

## Requirements

- macOS 12 or later, Linux with kernel 3.2 or later, or Linux under WSL
- Git 2.24 or later
- Bash 3.2 or later

Native Windows shells are not supported in v1.

## Install

Download and inspect the installer published with a GitHub Release:

```sh
curl -fsSLO https://github.com/otar/pxp/releases/latest/download/install.sh
less install.sh
bash install.sh
```

The installer selects the current OS and architecture, verifies `SHA256SUMS`, and atomically installs `pxp` into `~/.local/bin` by default. See [installation](docs/installation.md) for pinned versions, custom destinations, upgrades, and removal.

To build from source with Go 1.25 or later:

```sh
make build
./bin/pxp version
```

## Quick start

Create `plan.pxp.yaml`, substituting the full output of `git rev-parse --verify 'HEAD^{commit}'` for the example `head`:

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
  - name: Verify it
    run:
      - test "$(cat hello.txt)" = "hello from PXP"
```

Review every byte, compute its SHA-256 with `sha256sum plan.pxp.yaml` on Linux or `shasum -a 256 plan.pxp.yaml` on macOS, and copy that fixed hash into the command:

```sh
pxp run --expect-sha256 9f00000000000000000000000000000000000000000000000000000000000000 plan.pxp.yaml
```

The displayed hash is intentionally illustrative; use the exact hash you reviewed, never that value. If the file changes after approval, the command fails before opening the repository. PXP then obtains a worktree lock, verifies the declared Git HEAD, and refuses tracked changes unless `--allow-dirty` is explicit. Untracked and ignored files are allowed, so an untracked plan can be executed. Commands run sequentially in fresh non-interactive Bash processes. Patches are checked with Git before they are applied.

```text
PLAN
  AI reasoning allowed
        ↓
  complete PXP artifact

APPROVE
  lock exact plan hash
  lock expected repository HEAD

APPLY
  PXP only; zero LLM
  stop on any deviation
```

## CLI

```text
pxp run [--allow-dirty] [--expect-sha256 HASH] <plan.pxp.yaml|->
pxp help [run|version]
pxp version
pxp -h
pxp --help
pxp --version
```

Use `--expect-sha256` to make an independently approved, 64-character lowercase SHA-256 a precondition. Use `-` to read a plan from standard input. File inputs must be non-symlink regular files. Child commands never receive the plan stream as interactive input. Lifecycle messages go to standard error; command output is streamed unchanged. PXP reports command ordinals but never echoes command contents.

Exit codes are stable: `0` success, `2` usage, `3` input/schema error, `4` precondition error, `5` execution failure, `70` internal error, and `130`/`143` for interrupted processes.

## Protocol and migration

The normative behavior is in [SPEC.md](SPEC.md), with machine-readable companions for [PXP 1.1](schema/pxp-v1.1.schema.json) and [PXP 1.0](schema/pxp-v1.schema.json). PXP 1.1 requires exact Git HEAD binding. Existing PXP 1.0 plans remain valid but are explicitly unbound. Older `main-v0.1` and `develop-v0.1` documents are intentionally rejected by `pxp`; convert them with the separately shipped `pxp-convert` tool described in [MIGRATION.md](MIGRATION.md).

## Agent integrations and benchmarks

Thin skills for Claude Code and Codex keep AI activity in the planning phase and stop after producing a closed plan. See [agent integration setup](docs/integrations.md) for copy-pastable installation and approval examples.

`pxp-bench` compares one-shot normal agent execution with one-shot agent planning followed by hash-locked PXP execution. It records agent-reported tokens and tool events when available, harness-measured durations and outcomes, and `null` for unavailable metrics. See the [benchmark guide](benchmark/README.md) for the reproducible fixtures, commands, report schema, and security caveats.

PXP does not roll back shell commands or patches after a later step fails. Commit or stash important work first and follow [recovery guidance](docs/recovery.md) if execution stops partway through.

## Project policy

Security reports follow [SECURITY.md](SECURITY.md). Contributions follow [CONTRIBUTING.md](CONTRIBUTING.md). Releases and reproducibility are documented in [RELEASING.md](RELEASING.md).

MIT licensed. See [LICENSE](LICENSE).
