#!/bin/bash
# SerialHub 发布构建脚本 (Linux/macOS)
# 用法: ./build-release.sh [版本号] [选项]
# 示例:
#   ./build-release.sh                    # 使用代码中的版本号
#   ./build-release.sh 0.2.0              # 指定版本号
#   ./build-release.sh --skip-vet         # 跳过 go vet
#
# 选项:
#   --skip-vet      跳过 go vet 检查

set -e

# 默认配置
VERSION=""
OUTPUT_DIR="dist"
SKIP_VET=false

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-vet)
            SKIP_VET=true
            shift
            ;;
        -*|--*)
            echo "未知选项: $1"
            echo "用法: $0 [版本号] [--skip-vet]"
            exit 1
            ;;
        *)
            if [ -z "$VERSION" ]; then
                VERSION="$1"
            fi
            shift
            ;;
    esac
done

# 从代码中获取基础版本号
get_base_version() {
    local main_file="$(dirname "$0")/../cmd/serialhub/main.go"
    if [ -f "$main_file" ]; then
        grep -E 'const\s+baseVersion\s*=' "$main_file" | sed -E 's/.*"([^"]+)".*/\1/'
    else
        echo "0.1.0"
    fi
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

# 颜色定义
CYAN='\033[36m'
YELLOW='\033[33m'
GREEN='\033[32m'
RED='\033[31m'
GRAY='\033[90m'
WHITE='\033[37m'
NC='\033[0m' # No Color

echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}SerialHub 发布构建脚本${NC}"
echo -e "${CYAN}版本: $VERSION${NC}"
echo -e "${CYAN}========================================${NC}"

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

# 构建信息
BUILD_TIME=$(date '+%Y-%m-%d %H:%M:%S')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

step=1
total_steps=5

# 步骤 1: 代码检查
echo -e "\n${YELLOW}[$step/$total_steps] 代码检查...${NC}"
if [ "$SKIP_VET" = false ]; then
    echo -e "${GRAY}  运行 go vet...${NC}"
    if ! go vet ./...; then
        echo -e "${RED}go vet 检查失败，请修复错误后再构建${NC}"
        exit 1
    fi
    echo -e "${GREEN}  go vet 通过${NC}"
else
    echo -e "${GRAY}  跳过 go vet (使用 --skip-vet 参数)${NC}"
fi

# 步骤 2: 清理旧构建文件
step=$((step + 1))
echo -e "\n${YELLOW}[$step/$total_steps] 清理旧构建文件...${NC}"
echo -e "${GRAY}  输出目录: $OUTPUT_DIR${NC}"

# 清理并创建构建目录
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR/bin"

# 清理旧的打包文件
rm -f "$OUTPUT_DIR"/*.tar.gz

# 步骤 3: 构建可执行文件
step=$((step + 1))
echo -e "\n${YELLOW}[$step/$total_steps] 构建可执行文件...${NC}"

export CGO_ENABLED=0

# 构建设置
LDFLAGS="-s -w -X main.version=$VERSION -X main.buildTime=$BUILD_TIME -X main.gitCommit=$GIT_COMMIT"

echo -e "${GRAY}  构建参数: -ldflags \"$LDFLAGS\"${NC}"
echo -e "${GRAY}  编译: go build -o $BUILD_DIR/bin/serialhub ./cmd/serialhub${NC}"

go build -ldflags "$LDFLAGS" -o "$BUILD_DIR/bin/serialhub" ./cmd/serialhub

if [ $? -ne 0 ]; then
    echo -e "${RED}构建失败${NC}"
    exit 1
fi

EXE_SIZE=$(du -h "$BUILD_DIR/bin/serialhub" | cut -f1)
echo -e "${GREEN}  构建成功: bin/serialhub ($EXE_SIZE)${NC}"

# 步骤 4: 复制配置文件和资源
step=$((step + 1))
echo -e "\n${YELLOW}[$step/$total_steps] 复制配置文件和资源...${NC}"

copy_file() {
    local src="$1"
    local dst="$2"
    local name="$3"
    
    if [ -f "$src" ]; then
        cp -f "$src" "$dst"
        local size=$(du -h "$dst" | cut -f1)
        echo -e "${GRAY}  复制: $name -> $(basename "$dst") ($size)${NC}"
    else
        echo -e "${YELLOW}  警告: $name (文件不存在)${NC}"
    fi
}

copy_file "./config.example.toml" "$BUILD_DIR/config.toml" "配置文件"
copy_file "./start.ps1" "$BUILD_DIR/start.ps1" "启动脚本"
copy_file "./README.md" "$BUILD_DIR/README.md" "README"
copy_file "./MCP.md" "$BUILD_DIR/MCP.md" "MCP文档"
copy_file "./QUICKSTART.md" "$BUILD_DIR/QUICKSTART.md" "快速开始"
copy_file "./scripts/opencode.json" "$BUILD_DIR/opencode.json" "OpenCode配置"

# 创建版本信息文件
cat > "$BUILD_DIR/VERSION" << EOF
SerialHub v$VERSION
Build Time: $BUILD_TIME
Git Commit: $GIT_COMMIT
Platform: $TARGET
EOF
echo -e "${GRAY}  创建: VERSION 文件${NC}"

# 步骤 5: 打包发布文件
step=$((step + 1))
echo -e "\n${YELLOW}[$step/$total_steps] 打包发布文件...${NC}"

TAR_FILE="$OUTPUT_DIR/serialhub-$VERSION-$TARGET.tar.gz"
echo -e "${GRAY}  输出文件: $TAR_FILE${NC}"

# 显示打包内容
echo -e "${GRAY}  打包内容:${NC}"
cd "$OUTPUT_DIR"
find "$(basename "$BUILD_DIR")" -type f -o -type d | while read item; do
    if [ -d "$item" ]; then
        echo -e "${GRAY}    [DIR]  $item${NC}"
    else
        size=$(du -h "$item" 2>/dev/null | cut -f1)
        echo -e "${GRAY}    [FILE] $item ($size)${NC}"
    fi
done

# 打包
tar -czf "$(basename "$TAR_FILE")" "$(basename "$BUILD_DIR")"
cd "$PROJECT_ROOT"

TAR_SIZE=$(du -h "$TAR_FILE" | cut -f1)
echo -e "${GREEN}  打包完成: $(basename "$TAR_FILE") ($TAR_SIZE)${NC}"

# 完成信息
echo -e "\n${GREEN}========================================${NC}"
echo -e "${GREEN}发布构建完成!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "${WHITE}版本号:     $VERSION${NC}"
echo -e "${WHITE}构建时间:   $BUILD_TIME${NC}"
echo -e "${WHITE}Git Commit: $GIT_COMMIT${NC}"
echo -e "${WHITE}输出文件:   $TAR_FILE${NC}"
echo -e "${WHITE}文件大小:   $TAR_SIZE${NC}"

# 计算校验和
echo -e "\n${GRAY}SHA256 校验和:${NC}"
echo -e "${GRAY}  $(sha256sum "$TAR_FILE" | cut -d' ' -f1)${NC}"
echo -e "\n${GRAY}校验和命令:${NC}"
echo -e "${GRAY}  sha256sum $TAR_FILE${NC}"
