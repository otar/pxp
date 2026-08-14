# Troubleshooting

## Precondition exit (4)

Run inside a Git worktree and confirm Git 2.24+ and Bash 3.2+ are available. For `plan SHA-256 mismatch`, re-review the current file and pass its newly approved hash; never bypass the check with an automatically recomputed value. For `git HEAD mismatch`, return to planning against the current commit or deliberately restore the expected commit. Commit or stash tracked changes; untracked and ignored files do not block execution. If another PXP process owns the worktree lock, wait for it. Use `--allow-dirty` only after reviewing how the plan interacts with existing tracked changes.

## Input exit (3)

Confirm the document is UTF-8 PXP `1.1` (bound) or `1.0` (legacy unbound), contains one YAML document, and follows the matching schema. Quote the version. A 1.1 plan requires exactly `repository.type: git` and a full lowercase 40- or 64-character `repository.head`. Remove aliases, anchors, tags, duplicate or unknown fields. Use `pxp-convert` for historical plans.

For rejected patches, use repository-relative Git diffs. Symlinks, gitlinks, renames, copies, binary patches, traversal paths, and unsupported mode changes are outside v1.

## Execution exit (5)

The workspace may be partially changed. Read the failing step's child output, inspect Git state, and follow [recovery.md](recovery.md). Commands are separate strict Bash processes, so a `cd`, variable, or shell option in one command does not persist to the next.

## Version and release checks

`pxp version` prints executor build metadata. PXP performs no automatic update request. Compare against GitHub Releases manually and verify downloaded assets with the published checksum and attestation.
