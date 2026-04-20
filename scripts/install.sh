#!/usr/bin/env bash
#
# onboardctl installer
#
# Downloads the latest (or requested) onboardctl release binary and places
# it in the install dir. User-local by default — no sudo needed.
#
# One-liner:
#   curl -fsSL https://raw.githubusercontent.com/ZlatanOmerovic/onboardctl/main/scripts/install.sh | bash
#
# Options (pass after --):
#   --install-dir DIR   Install to DIR instead of ~/.local/bin
#   --version VER       Pin to a specific version (e.g. v0.1.0) instead of latest
#   --help              Print usage and exit
#
# Env (for CI / advanced users):
#   ONBOARDCTL_INSTALL_DIR     same as --install-dir
#   ONBOARDCTL_VERSION         same as --version
#
# Requirements: bash, curl, tar, uname. sha256sum optional (used for integrity
# check when the release ships a checksums.txt).

set -euo pipefail

OWNER="ZlatanOmerovic"
REPO="onboardctl"
BIN_NAME="onboardctl"
DEFAULT_INSTALL_DIR="${HOME}/.local/bin"

install_dir="${ONBOARDCTL_INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
version="${ONBOARDCTL_VERSION:-latest}"

# ---- helpers --------------------------------------------------------------

say() {
  printf '\033[1;35m==\033[0m %s\n' "$*"
}

warn() {
  printf '\033[1;33mWARN:\033[0m %s\n' "$*" >&2
}

err() {
  printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || err "missing required tool: $1"
}

usage() {
  sed -n 's/^# //p; s/^#$//p' "$0" | head -20
}

# ---- parse flags ----------------------------------------------------------

while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir) install_dir="$2"; shift 2 ;;
    --version)     version="$2";     shift 2 ;;
    --help|-h)     usage; exit 0 ;;
    *)             err "unknown flag: $1 (use --help)" ;;
  esac
done

# ---- preflight ------------------------------------------------------------

need curl
need tar
need uname

case "$(uname -s)" in
  Linux) os="linux" ;;
  *)     err "onboardctl targets Debian-family Linux; uname -s reports $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *)             err "unsupported architecture: $(uname -m) (amd64 and arm64 only)" ;;
esac

# ---- resolve version ------------------------------------------------------

if [[ "$version" == "latest" ]]; then
  say "Resolving latest onboardctl release..."
  # Scrape the tag_name without depending on jq.
  api_url="https://api.github.com/repos/${OWNER}/${REPO}/releases/latest"
  version="$(curl -fsSL "$api_url" \
    | grep '"tag_name"' \
    | head -n1 \
    | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/' )"
  if [[ -z "$version" || "$version" == "$api_url" ]]; then
    err "could not resolve a latest release — see https://github.com/${OWNER}/${REPO}/releases"
  fi
fi

# Accept both "0.1.0" and "v0.1.0"; normalise to the asset-name form.
version_nov="${version#v}"

# ---- download + verify ----------------------------------------------------

asset="${BIN_NAME}_${version_nov}_${os}_${arch}.tar.gz"
checksum="${BIN_NAME}_${version_nov}_checksums.txt"
base="https://github.com/${OWNER}/${REPO}/releases/download/${version}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

say "Downloading $asset from $version..."
if ! curl -fsSL "$base/$asset" -o "$tmp/$asset"; then
  err "download failed. Expected URL: $base/$asset
If the release exists but the asset name differs, open an issue at
https://github.com/${OWNER}/${REPO}/issues and we'll reconcile."
fi

# Optional sha256 integrity check.
if command -v sha256sum >/dev/null 2>&1; then
  if curl -fsSL "$base/$checksum" -o "$tmp/$checksum" 2>/dev/null; then
    say "Verifying checksum..."
    (cd "$tmp" && grep " $asset\$" "$checksum" | sha256sum -c -) \
      || err "checksum mismatch for $asset"
  else
    warn "no checksums.txt in release; skipping sha256 verification"
  fi
else
  warn "sha256sum not found; skipping integrity check"
fi

# ---- extract + install ----------------------------------------------------

say "Extracting..."
tar -xzf "$tmp/$asset" -C "$tmp"

src="$tmp/$BIN_NAME"
if [[ ! -f "$src" ]]; then
  # Some release pipelines keep the binary inside a top-level dir.
  src="$(find "$tmp" -maxdepth 3 -type f -name "$BIN_NAME" | head -n1)"
fi
if [[ -z "$src" || ! -f "$src" ]]; then
  err "extracted archive did not contain a '${BIN_NAME}' binary"
fi

mkdir -p "$install_dir"
install -m 0755 "$src" "$install_dir/$BIN_NAME"
say "Installed to $install_dir/$BIN_NAME"

# ---- PATH hint ------------------------------------------------------------

case ":$PATH:" in
  *:"$install_dir":*) ;;
  *)
    echo
    warn "$install_dir is not on your \$PATH."
    echo "  Add it to your shell rc, for example:"
    echo "    echo 'export PATH=\"$install_dir:\$PATH\"' >> ~/.bashrc   # or .zshrc"
    echo "    source ~/.bashrc"
    ;;
esac

# ---- sanity call ----------------------------------------------------------

if command -v "$install_dir/$BIN_NAME" >/dev/null 2>&1; then
  echo
  "$install_dir/$BIN_NAME" version || true
fi

echo
say "Done. Next step:  $BIN_NAME init    (or see '$BIN_NAME --help')"
