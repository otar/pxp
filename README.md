# PXP — Plan eXecution Protocol

PXP turns a reviewed YAML plan into a deterministic sequence of shell commands and Git patches. Planning stays with the coding agent; execution is performed by a small, auditable CLI with no model calls, telemetry, or automatic network access.

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

Create `plan.pxp.yaml`:

```yaml
pxp_version: "1.0"
name: Add a greeting
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

Review it, then run:

```sh
pxp run plan.pxp.yaml
```

PXP validates the entire document before any mutation, locates the enclosing Git worktree, obtains a worktree lock, and refuses tracked changes unless `--allow-dirty` is explicit. Untracked and ignored files are allowed, so an untracked plan can be executed. Commands run sequentially in fresh non-interactive Bash processes. Patches are checked with Git before they are applied.

## CLI

```text
pxp run [--allow-dirty] <plan.pxp.yaml|->
pxp help [run|version]
pxp version
pxp -h
pxp --help
pxp --version
```

Use `-` to read a plan from standard input. File inputs must be non-symlink regular files. Child commands never receive the plan stream as interactive input. Lifecycle messages go to standard error; command output is streamed unchanged. PXP reports command ordinals but never echoes command contents.

Exit codes are stable: `0` success, `2` usage, `3` input/schema error, `4` precondition error, `5` execution failure, `70` internal error, and `130`/`143` for interrupted processes.

## Protocol and migration

The normative v1 behavior is in [SPEC.md](SPEC.md), with a machine-readable companion at [schema/pxp-v1.schema.json](schema/pxp-v1.schema.json). Older `main-v0.1` and `develop-v0.1` documents are intentionally rejected by `pxp`; convert them with the separately shipped `pxp-convert` tool described in [MIGRATION.md](MIGRATION.md).

PXP does not roll back shell commands or patches after a later step fails. Commit or stash important work first and follow [recovery guidance](docs/recovery.md) if execution stops partway through.

## Project policy

Security reports follow [SECURITY.md](SECURITY.md). Contributions follow [CONTRIBUTING.md](CONTRIBUTING.md). Releases and reproducibility are documented in [RELEASING.md](RELEASING.md).

MIT licensed. See [LICENSE](LICENSE).
