# PXP 1.0 specification

Status: stable
Protocol version: `1.0`

## 1. Purpose

PXP is a reviewed-plan execution protocol for code workspaces. A planner creates one UTF-8 YAML document; a v1 executor validates it and then executes its steps in order. PXP does not define planning, sandboxing, package management, remote execution, rollback, or adaptive response to command output.

Plans are trusted programs. A conforming executor MUST NOT imply that validation makes arbitrary shell commands safe.

## 2. Document model

```yaml
pxp_version: "1.0"
name: Human-readable plan name
steps:
  - name: Optional step name
    run:
      - command one
      - command two
  - name: Optional step name
    patch: |
      diff --git a/file.txt b/file.txt
      --- a/file.txt
      +++ b/file.txt
      @@ -1 +1 @@
      -old
      +new
```

The top-level mapping MUST contain exactly `pxp_version`, `name`, and `steps`. `pxp_version` MUST be the string `1.0`. `name` MUST be a non-blank string of at most 1024 UTF-8 bytes. `steps` MUST contain 1 through 1000 step mappings.

A step MAY contain a non-blank `name` of at most 1024 UTF-8 bytes and MUST contain exactly one of:

- `run`: a sequence of 1 through 100 non-blank strings, each at most 65536 UTF-8 bytes.
- `patch`: a non-blank unified Git diff string.

No other fields are allowed. YAML keys and all protocol scalars MUST have their declared types; implicit numeric or boolean coercion is not allowed.

## 3. Encoding and YAML profile

Input MUST be valid UTF-8, contain exactly one YAML document, and be no larger than 16 MiB. A byte-order mark MAY appear only at the beginning. NUL is forbidden.

The v1 profile rejects unknown and duplicate fields, aliases, anchors, merge keys, explicit tags, directives, and multiple documents. These restrictions keep review and decoding behavior consistent across implementations.

The executor MUST read, hash, parse, and validate the complete plan before changing the workspace. The SHA-256 hash shown in lifecycle output identifies the exact input bytes.

## 4. Repository preconditions

Execution starts in the current directory and targets the enclosing Git worktree returned by Git. Git 2.24 or later and Bash 3.2 or later are required. A v1 executor supports macOS, Linux, and Linux under WSL; native Windows is outside v1.

Before the first step, the executor MUST:

1. verify all runtime requirements;
2. acquire an exclusive, non-blocking PXP lock in the worktree Git directory;
3. reject tracked staged, unstaged, or unmerged changes unless `--allow-dirty` was supplied; and
4. leave untracked and ignored files out of the cleanliness check.

The lock is held until execution ends. A contending executor exits with a precondition error. Worktrees use their own Git directories and therefore their own locks.

## 5. Run steps

Commands execute sequentially. Each command starts a fresh `bash --noprofile --norc` process with `errexit`, `nounset`, and `pipefail` enabled. The working directory is the worktree root. `BASH_ENV`, `ENV`, and `CDPATH` are removed; the remainder of the invoking environment is inherited.

Commands receive no interactive standard input. Their standard output and standard error are streamed to the corresponding executor streams. A non-zero status stops the plan immediately and becomes an execution failure. State in one shell process, such as `cd` or a shell variable, does not carry to the next command; filesystem mutations do.

PXP defines no command timeout. Callers that require a timeout MUST impose one externally or in the command itself.

## 6. Patch steps

Every patch is parsed during plan validation. Before applying a patch, the executor runs:

```text
git apply --check --whitespace=nowarn
```

and applies it only if the check succeeds. Both operations run at the worktree root and read the validated patch bytes through standard input.

V1 permits text additions, modifications, deletions, and multi-file patches for regular files. New regular files MAY use mode `100644` or `100755`. It rejects binary patches, absolute paths, backslashes, empty paths, `.` or `..` path components, `.git` components, symlinks, gitlinks, rename/copy metadata, and mode-only or unsupported mode changes. Git's own path and index checks remain authoritative in addition to these protocol checks.

## 7. Ordering, failure, and interruption

Steps execute once in document order and stop at the first failure. PXP performs no rollback. Earlier command and patch effects remain in the worktree and recovery is a user decision.

On `SIGINT` or `SIGTERM`, the executor forwards the signal to the active command process group, waits for it, stops the plan, and releases the lock. Conventional exit codes are `130` for `SIGINT` and `143` for `SIGTERM`.

## 8. Output and exit status

Lifecycle output is human-readable and written to standard error. It includes the plan name, exact-input SHA-256 hash, numbered step start/success/failure events, command ordinals, and final status. Command contents MUST NOT be echoed. During plan execution, the executor itself writes nothing to standard output; child process output is streamed to the corresponding stream and is not reformatted. Color MAY be used only when standard error is a terminal and MUST respect `NO_COLOR`.

Stable exit codes:

| Code | Meaning |
|---:|---|
| 0 | Success |
| 2 | CLI usage error |
| 3 | Input, YAML, schema, or patch-policy error |
| 4 | Environment, repository, cleanliness, or lock precondition error |
| 5 | A command or patch failed during execution |
| 70 | Unexpected internal error |
| 130 | Interrupted by `SIGINT` |
| 143 | Terminated by `SIGTERM` |

Diagnostics MUST identify the category and failing step without printing secrets intentionally. PXP cannot prevent a trusted plan from printing inherited environment values.

## 9. CLI

The normative interface is:

```text
pxp run [--allow-dirty] <plan.pxp.yaml|->
pxp help [run|version]
pxp version
pxp -h
pxp --help
pxp --version
```

`-` reads at most 16 MiB from standard input before execution and is not forwarded to child commands. A file input MUST resolve to a non-symlink regular file. Unknown flags, commands, help topics, and extra operands are usage errors.

## 10. Security and networking

Ordinary execution performs no update check, telemetry, DNS lookup, or other protocol-initiated network request. A plan command may use the network because it has the caller's permissions.

Reviewers SHOULD inspect the whole plan, especially shell commands, patch paths, generated files, and data sent to external programs. Users SHOULD commit or stash valuable work before running a plan, even when `--allow-dirty` is used.

## 11. Compatibility

The protocol version is independent of the executor release version. Executors MUST reject unsupported protocol versions. Historical formats are not accepted by the v1 executor; an explicit, non-executing converter is provided separately. Future compatible clarifications retain `1.0`; incompatible document changes require another protocol version.
