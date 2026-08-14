#!/usr/bin/env bash
set -euo pipefail

readonly REPOSITORY="otar/pxp"
readonly DEFAULT_INSTALL_DIR="${HOME:+${HOME}/.local/bin}"

die() {
  printf 'pxp installer: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Install a checksummed PXP release.

Usage:
  install.sh [--version VERSION] [--bin-dir DIR] [--force]

Environment:
  PXP_VERSION   Release to install, for example v1.0.0 (default: latest)
  PXP_BIN_DIR   Destination directory (default: $HOME/.local/bin)

Options:
  --version VERSION  Pin a semantic release version
  --bin-dir DIR      Install into an absolute directory
  --force            Replace an existing destination file
  -h, --help         Show this help
EOF
}

version="${PXP_VERSION:-latest}"
install_dir="${PXP_BIN_DIR:-$DEFAULT_INSTALL_DIR}"
force=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      [[ $# -ge 2 ]] || die "--version requires a value"
      version="$2"
      shift 2
      ;;
    --bin-dir)
      [[ $# -ge 2 ]] || die "--bin-dir requires a value"
      install_dir="$2"
      shift 2
      ;;
    --force)
      force=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unexpected argument: $1"
      ;;
  esac
done

for dependency in awk chmod cp curl mkdir mktemp mv rm tar tr uname; do
  command -v "$dependency" >/dev/null 2>&1 || die "required command not found: $dependency"
done

case "$(uname -s)" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *) die "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

if [[ "$version" == "latest" ]]; then
  latest_url=$(curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL \
    --connect-timeout 10 --max-time 120 \
    -o /dev/null -w '%{url_effective}' \
    "https://github.com/${REPOSITORY}/releases/latest") || die "could not resolve the latest release"
  version="${latest_url##*/}"
fi
[[ "$version" == v* ]] || version="v${version}"
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || die "invalid release version: $version"

archive="pxp_${version#v}_${os}_${arch}.tar.gz"
base_url="https://github.com/${REPOSITORY}/releases/download/${version}"
[[ -n "$install_dir" ]] || die "PXP_BIN_DIR must not be empty"
[[ "$install_dir" == /* ]] || die "installation directory must be absolute: $install_dir"

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/pxp-install.XXXXXX")
destination_tmp=""
cleanup() {
  [[ -z "$destination_tmp" ]] || rm -f "$destination_tmp"
  rm -rf "$tmp_dir"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

curl --proto '=https' --proto-redir '=https' --tlsv1.2 --connect-timeout 10 --max-time 120 \
  -fsSLo "$tmp_dir/$archive" "$base_url/$archive" \
  || die "failed to download $archive"
curl --proto '=https' --proto-redir '=https' --tlsv1.2 --connect-timeout 10 --max-time 120 \
  -fsSLo "$tmp_dir/SHA256SUMS" "$base_url/SHA256SUMS" \
  || die "failed to download SHA256SUMS"

expected=$(awk -v file="$archive" '$2 == file || $2 == "*" file { print $1; exit }' "$tmp_dir/SHA256SUMS")
[[ "$expected" =~ ^[0-9a-fA-F]{64}$ ]] || die "checksum entry not found for $archive"
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp_dir/$archive" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$tmp_dir/$archive" | awk '{print $1}')
else
  die "sha256sum or shasum is required"
fi
actual=$(printf '%s' "$actual" | tr '[:upper:]' '[:lower:]')
expected=$(printf '%s' "$expected" | tr '[:upper:]' '[:lower:]')
[[ "$actual" == "$expected" ]] || die "checksum verification failed for $archive"

mkdir -p "$tmp_dir/extract"
tar -tzf "$tmp_dir/$archive" > "$tmp_dir/members"
awk '
  $0 == "pxp" { pxp++; next }
  $0 == "pxp-bench" { bench++; next }
  $0 == "pxp-convert" { convert++; next }
  $0 == "LICENSE" { license++; next }
  $0 == "README.md" { readme++; next }
  { bad = 1 }
  END { exit bad || pxp != 1 || bench > 1 || convert != 1 || license != 1 || readme != 1 }
' "$tmp_dir/members" || die "release archive has missing or unexpected contents"
tar -xzf "$tmp_dir/$archive" -C "$tmp_dir/extract" pxp
[[ -f "$tmp_dir/extract/pxp" && ! -L "$tmp_dir/extract/pxp" ]] || die "release archive does not contain pxp"
chmod 0755 "$tmp_dir/extract/pxp"
downloaded_version=$("$tmp_dir/extract/pxp" version) || die "downloaded pxp binary failed its smoke test"
[[ "$downloaded_version" != *$'\n'* ]] || die "downloaded pxp reports an unexpected version"
case "$downloaded_version" in
  "pxp ${version#v} ("?*")") ;;
  *) die "downloaded pxp reports an unexpected version" ;;
esac

mkdir -p "$install_dir"
destination="$install_dir/pxp"
if [[ -e "$destination" || -L "$destination" ]]; then
  [[ ! -d "$destination" ]] || die "destination is a directory: $destination"
  [[ $force -eq 1 ]] || die "destination already exists; pass --force to replace it: $destination"
fi
destination_tmp=$(mktemp "$install_dir/.pxp.install.XXXXXX")
cp "$tmp_dir/extract/pxp" "$destination_tmp"
chmod 0755 "$destination_tmp"
mv -f "$destination_tmp" "$destination"
destination_tmp=""

printf 'Installed pxp %s to %s\n' "$version" "$destination"
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *) printf 'Add %s to PATH to run pxp.\n' "$install_dir" >&2 ;;
esac
