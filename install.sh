#!/usr/bin/env bash
# SerialHub 在线安装脚本（Linux x86_64 预编译包；其他系统/架构请源码构建，见 README）
# 用法: curl -fsSL https://raw.githubusercontent.com/dongly/serialhub/main/install.sh | bash
# 可用环境变量:
#   SERIALHUB_GITHUB_API   GitHub API 基址（默认 https://api.github.com，私有加速用）
#   SERIALHUB_INSTALL_DIR  安装目录（默认 ~/.local/bin）
set -euo pipefail

REPO="dongly/serialhub"
API_BASE="${SERIALHUB_GITHUB_API:-https://api.github.com}"
BIN_DIR="${SERIALHUB_INSTALL_DIR:-$HOME/.local/bin}"

fail() { echo "错误: $*" >&2; exit 1; }

[ "$(uname -s)" = "Linux" ] || fail "本脚本仅安装 Linux 预编译包；其他系统请源码构建（README「方式二」）"
[ "$(uname -m)" = "x86_64" ] || fail "预编译包仅提供 linux-amd64；其他架构请源码构建（README「方式二」）"
command -v curl >/dev/null 2>&1 || fail "需要 curl"
command -v tar >/dev/null 2>&1 || fail "需要 tar"
command -v sha256sum >/dev/null 2>&1 || fail "需要 sha256sum"

echo ">> 查询最新版本..."
TAG=$(curl -fsSL "$API_BASE/repos/$REPO/releases/latest" | grep -oP '"tag_name":\s*"\K[^"]+' || true)
[ -n "$TAG" ] || fail "无法获取最新版本（网络受限可设 HTTPS_PROXY，或用 SERIALHUB_GITHUB_API 指定加速基址）"
VER="${TAG#v}"
echo ">> 最新版本: $TAG"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
BASE="https://github.com/$REPO/releases/download/$TAG"
PKG="serialhub-$VER-linux-amd64"
cd "$TMP"

echo ">> 下载 $PKG.tar.gz ..."
curl -fL --progress-bar -o "$PKG.tar.gz" "$BASE/$PKG.tar.gz" || fail "下载失败"
echo ">> 校验 sha256 ..."
curl -fsSL -o "$PKG.tar.gz.sha256" "$BASE/$PKG.tar.gz.sha256" || fail "校验文件下载失败"
sha256sum -c --quiet "$PKG.tar.gz.sha256" 2>/dev/null || fail "sha256 校验失败，文件可能损坏"
tar -xzf "$PKG.tar.gz"
[ -f "$PKG/serialhub" ] || fail "压缩包内未找到 serialhub 二进制"

mkdir -p "$BIN_DIR"
install -m 0755 "$PKG/serialhub" "$BIN_DIR/serialhub"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo ">> 提示: $BIN_DIR 不在 PATH，请加入 shell 配置: export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac

"$BIN_DIR/serialhub" --version
echo ">> 安装完成: $BIN_DIR/serialhub（serialhub -h 查看用法，README 有快速开始）"
