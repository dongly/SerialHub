#!/bin/bash
# SerialHub 发布构建脚本 (Linux/macOS)
# 用法: ./build-release.sh [版本号]

set -e

VERSION=${1:-""}
OUTPUT_DIR="dist"

# 从代码中获取基础版本号
get_base_version() {
    grep -E 'const\s+baseVersion\s*=' "$(dirname "$0")/../cmd/serialhub/main.go" | sed -E 's/.*"([^"]+)".*/\1/'
}

BASE_VERSION=$(get_base_version)
if [ -z "$BASE_VERSION" ]; then
    BASE_VERSION="0.1.0"
fi

# 获取版本号: 手动指定 > 代码中的 baseVersion
if [ -z "$VERSION" ]; then
    VERSION="$BASE_VERSION"
fi

# 移除版本号前缀 v（如果有）
VERSION=${VERSION#v}

echo -e "\033[36m========================================\033[0m"
echo -e "\033[36mSerialHub 发布构建脚本\033[0m"
echo -e "\033[36m版本: $VERSION\033[0m"
echo -e "\033[36m========================================\033[0m"

# 获取项目根目录
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# 检测操作系统和架构
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    armv7l) ARCH="arm" ;;
esac

TARGET="$OS-$ARCH"
BUILD_DIR="$OUTPUT_DIR/serialhub-$VERSION-$TARGET"

# 创建输出目录
mkdir -p "$BUILD_DIR"

echo -e "\n\033[33m[1/5] 清理旧构建文件...\033[0m"
rm -f "$OUTPUT_DIR"/*.tar.gz

echo -e "\n\033[33m[2/5] 运行测试...\033[0m"
if ! go test ./...; then
    echo -e "\033[31m测试失败，停止构建\033[0m"
    exit 1
fi
echo -e "\033[32m测试通过!\033[0m"

echo -e "\n\033[33m[3/5] 构建可执行文件...\033[0m"
export CGO_ENABLED=0
go build -ldflags "-s -w" -o "$BUILD_DIR/serialhub" ./cmd/serialhub

echo -e "\033[32m构建成功: $BUILD_DIR/serialhub\033[0m"

echo -e "\n\033[33m[4/5] 复制配置文件和资源...\033[0m"
# 复制配置文件模板
cp -f ./config.example.toml "$BUILD_DIR/config.toml" 2>/dev/null || true
# 复制启动脚本
cp -f ./scripts/start.sh "$BUILD_DIR/" 2>/dev/null || true
cp -f ./scripts/start.ps1 "$BUILD_DIR/" 2>/dev/null || true
# 复制文档
cp -f ./README.md "$BUILD_DIR/" 2>/dev/null || true
cp -f ./DEPLOY.md "$BUILD_DIR/" 2>/dev/null || true
cp -f ./MCP.md "$BUILD_DIR/" 2>/dev/null || true

echo -e "\n\033[33m[5/5] 打包发布文件...\033[0m"
TAR_FILE="$OUTPUT_DIR/serialhub-$VERSION-$TARGET.tar.gz"

cd "$OUTPUT_DIR"
tar -czf "$(basename "$TAR_FILE")" "$(basename "$BUILD_DIR")"
cd ..

echo -e "\n\033[32m========================================\033[0m"
echo -e "\033[32m发布构建完成!\033[0m"
echo -e "\033[32m输出文件: $TAR_FILE\033[0m"
echo -e "\033[32m文件大小: $(du -h "$TAR_FILE" | cut -f1)\033[0m"
echo -e "\033[32m========================================\033[0m"

# 计算校验和
echo -e "\n\033[90mSHA256: $(sha256sum "$TAR_FILE" | cut -d' ' -f1)\033[0m"
