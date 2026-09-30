#!/usr/bin/env bash
# SerialHub online installer (prebuilt Linux x86_64 package).
# Usage: curl -fsSL https://raw.githubusercontent.com/dongly/serialhub/main/install.sh | bash
# Environment variables:
#   SERIALHUB_GITHUB_API   GitHub API base URL (default: https://api.github.com)
#   SERIALHUB_INSTALL_DIR  Installation directory (default: ~/.local/bin)
set -euo pipefail

REPO="dongly/serialhub"
API_BASE="${SERIALHUB_GITHUB_API:-https://api.github.com}"
BIN_DIR="${SERIALHUB_INSTALL_DIR:-$HOME/.local/bin}"

fail() { echo "Error: $*" >&2; exit 1; }

[ "$(uname -s)" = "Linux" ] || fail "This installer supports prebuilt Linux packages only; see the README for source builds"
[ "$(uname -m)" = "x86_64" ] || fail "Prebuilt packages are available for linux-amd64 only; see the README for source builds"
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"
command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required"

echo ">> Checking the latest release..."
TAG=$(curl -fsSL "$API_BASE/repos/$REPO/releases/latest" | grep -oP '"tag_name":\s*"\K[^"]+' || true)
[ -n "$TAG" ] || fail "Cannot fetch the latest release (try HTTPS_PROXY or SERIALHUB_GITHUB_API)"
VER="${TAG#v}"
echo ">> Latest release: $TAG"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
# SERIALHUB_DOWNLOAD_BASE lets mirrors/CI redirect asset downloads
# (the GitHub API override above only covers release lookup).
BASE="${SERIALHUB_DOWNLOAD_BASE:-https://github.com/$REPO/releases/download}/$TAG"
PKG="serialhub-$VER-linux-amd64"
cd "$TMP"

echo ">> Downloading $PKG.tar.gz ..."
curl -fL --progress-bar -o "$PKG.tar.gz" "$BASE/$PKG.tar.gz" || fail "Download failed"
echo ">> Verifying sha256 ..."
curl -fsSL -o "$PKG.tar.gz.sha256" "$BASE/$PKG.tar.gz.sha256" || fail "Checksum download failed"
sha256sum -c --quiet "$PKG.tar.gz.sha256" 2>/dev/null || fail "sha256 verification failed; archive may be corrupt"
tar -xzf "$PKG.tar.gz"
[ -f "$PKG/serialhub" ] || fail "Archive does not contain the serialhub executable"

mkdir -p "$BIN_DIR"
install -m 0755 "$PKG/serialhub" "$BIN_DIR/serialhub"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo ">> Note: $BIN_DIR is not in PATH. Add it to your shell configuration: export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac

"$BIN_DIR/serialhub" --version
echo ">> Installation complete: $BIN_DIR/serialhub (run serialhub -h for help; see README for quick start)"
