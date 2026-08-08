# Migrating historical plans

PXP 1.0 intentionally rejects both experimental v0.1 formats. The separately shipped `pxp-convert` command converts data only; it never executes a plan and never guesses the input format.

```text
pxp-convert --from main-v0.1 [--name TEXT] <file|->
pxp-convert --from develop-v0.1 [--name TEXT] <file|->
```

Canonical PXP 1.0 YAML is written to standard output. Diagnostics go to standard error. Always review the result before passing it to `pxp run`.

For the original `main` format:

```sh
pxp-convert --from main-v0.1 old.yml > plan.pxp.yaml
```

It maps each scalar `run` command to a one-element `run` sequence and maps `patch` unchanged. The old top-level `name` becomes the v1 name. Supply `--name` when it is missing or when you want to override it.

For the experimental `develop` format:

```sh
pxp-convert --from develop-v0.1 old.pxp.yaml > plan.pxp.yaml
```

It maps `plan.task` to `name`, `type: shell` with either documented `commands` or the prototype's singular `command` to `run`, and `type: patch` plus `diff` to `patch`. Legacy IDs are dropped. `--name` overrides the task.

Unknown fields, ambiguous operations, malformed YAML, aliases, tags, unsupported versions, and over-limit values fail conversion. Conversion success does not establish that commands are safe or that patches apply to the current repository.
