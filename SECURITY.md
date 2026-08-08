# Security policy

## Supported versions

The latest 1.x executor release receives security fixes. Pre-1.0 Bash prototypes are unsupported and should not execute untrusted paths or documents.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting for `otar/pxp`. Do not open a public issue for an unpatched vulnerability. Include the affected version, platform, reproduction, impact, and any suggested mitigation. You should receive an acknowledgement within three business days and a status update within seven.

## Trust model

PXP plans are trusted programs. `run` steps execute arbitrary Bash with the caller's permissions, inherit most of the caller's environment, and can access the network. PXP does not sandbox, prompt per command, conceal secrets a plan deliberately reads, or roll back mutations.

PXP does validate the document before execution, rejects unsafe patch metadata and paths, checks tracked-worktree cleanliness by default, and serializes execution per worktree. Review plans, run with least privilege, keep valuable work committed, and reserve `--allow-dirty` for deliberate use.

Release archives include SHA-256 checksums and GitHub artifact attestations. The installer verifies checksums before replacing a binary. Verify release origin and pin a version in automated environments.
