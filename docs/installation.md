# Installation and upgrades

Release archives contain `pxp`, `pxp-convert`, `pxp-bench`, license information, and release metadata. The standalone installer installs only `pxp`.

```sh
curl -fsSLO https://github.com/otar/pxp/releases/latest/download/install.sh
bash install.sh
```

Defaults:

- version: latest published release
- destination: `${PXP_BIN_DIR:-$HOME/.local/bin}`
- replacement: atomic after SHA-256 verification

Use `--version v1.0.0` to pin a release, `--bin-dir /absolute/path` to choose a destination, and `--force` when intentionally replacing any existing destination. The installer never executes a pre-existing file. Run `bash install.sh --help` for the exact interface. Ensure the destination is on `PATH`.

To use the converter or benchmark harness, download the matching release archive, verify it against `SHA256SUMS`, and copy `pxp-convert` and/or `pxp-bench` into a directory on `PATH`.

Upgrade by rerunning the inspected installer with a new pinned version and `--force`. PXP itself never checks the network for updates. Uninstall by removing the installed `pxp` binary; PXP creates no user configuration or background service.
