# SerialHub 发布指南

本文档说明如何构建和发布 SerialHub 的新版本。

## 发布前检查清单

在发布新版本前，请确保完成以下检查：

- [ ] 所有测试通过 (`go test ./...`)
- [ ] 代码检查通过 (`go vet ./...`)
- [ ] 版本号已更新 (`cmd/serialhub/main.go` 中的 `baseVersion`)
- [ ] CHANGELOG 已更新（如适用）
- [ ] 文档已更新（README、MCP.md 等）

## 版本号规范

SerialHub 使用语义化版本号（SemVer）：

```
主版本号.次版本号.修订号
例如: 0.1.0, 0.2.1, 1.0.0
```

- **主版本号**: 不兼容的 API 修改
- **次版本号**: 向下兼容的功能新增
- **修订号**: 向下兼容的问题修复

## 快速发布

### 方法 1: 使用 Makefile (推荐)

```bash
# 自动检测平台并构建
make release

# 构建 Windows 版本
make release-windows

# 构建 Linux/macOS 版本
make release-linux

# 快速构建 (跳过测试)
make release-fast
```

### 方法 2: 使用脚本直接构建

**Windows (PowerShell):**
```powershell
# 使用代码中的版本号
.\scripts\build-release.ps1

# 指定版本号
.\scripts\build-release.ps1 -Version "0.2.0"

# 跳过测试和代码检查
.\scripts\build-release.ps1 -SkipTests -SkipVet
```

**Linux/macOS (Bash):**
```bash
# 使用代码中的版本号
./scripts/build-release.sh

# 指定版本号
./scripts/build-release.sh 0.2.0

# 跳过测试和代码检查
./scripts/build-release.sh --skip-tests --skip-vet
```

## 发布流程

### 1. 更新版本号

编辑 `cmd/serialhub/main.go`，更新 `baseVersion` 常量：

```go
const baseVersion = "0.2.0"  // 修改为新版本号
```

### 2. 构建发布包

```bash
# 提交版本号更新
git add cmd/serialhub/main.go
git commit -m "chore: bump version to 0.2.0"

# 构建发布包
make release
```

### 3. 验证构建结果

构建完成后，检查输出：

```
dist/
├── serialhub-0.2.0-windows-amd64/
│   ├── bin/
│   │   └── serialhub.exe
│   ├── config.toml
│   ├── start.ps1
│   ├── README.md
│   ├── MCP.md
│   ├── AGENTS.md
│   └── VERSION
└── serialhub-0.2.0-windows-amd64.zip
```

### 4. 创建 Git 标签

```bash
# 创建标签
git tag -a v0.2.0 -m "Release version 0.2.0"

# 推送标签到远程
git push origin v0.2.0
```

### 5. 创建 GitHub Release

1. 访问 GitHub 仓库的 Releases 页面
2. 点击 "Draft a new release"
3. 选择刚才推送的标签 `v0.2.0`
4. 填写发布标题和说明
5. 上传构建好的 `.zip` 或 `.tar.gz` 文件
6. 发布

## 构建输出说明

### Windows 构建

- **输出文件**: `dist/serialhub-<version>-windows-amd64.zip`
- **包含内容**:
  - `bin/serialhub.exe` - 主程序
  - `config.toml` - 配置文件模板
  - `start.ps1` - PowerShell 启动脚本
  - `README.md` - 项目说明
  - `MCP.md` - MCP 使用指南
  - `AGENTS.md` - 开发指南
  - `VERSION` - 版本信息文件

### Linux/macOS 构建

- **输出文件**: `dist/serialhub-<version>-<os>-<arch>.tar.gz`
- **包含内容**: 与 Windows 相同（除 `.exe` 后缀）

## 跨平台构建

如需为其他平台构建，可以设置 Go 的环境变量：

```bash
# 为 Linux ARM64 构建
GOOS=linux GOARCH=arm64 go build -o bin/serialhub-linux-arm64 ./cmd/serialhub

# 为 macOS ARM64 (Apple Silicon) 构建
GOOS=darwin GOARCH=arm64 go build -o bin/serialhub-darwin-arm64 ./cmd/serialhub

# 为 Windows x86 构建
GOOS=windows GOARCH=386 go build -o bin/serialhub-windows-386.exe ./cmd/serialhub
```

## 故障排除

### 构建失败

1. **检查 Go 版本**: 需要 Go 1.26+
   ```bash
   go version
   ```

2. **检查依赖**: 运行 `go mod tidy`

3. **清理缓存**: 运行 `make clean` 后重试

### 测试失败

1. 检查是否有硬件测试环境变量设置
2. 临时跳过测试: `make release-fast`

### 权限问题 (Linux/macOS)

确保脚本有执行权限：
```bash
chmod +x scripts/build-release.sh
```

## 发布脚本参数

### PowerShell 脚本参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `-Version` | 指定版本号 | `-Version "0.2.0"` |
| `-OutputDir` | 指定输出目录 | `-OutputDir "dist"` |
| `-SkipTests` | 跳过测试 | `-SkipTests` |
| `-SkipVet` | 跳过 go vet | `-SkipVet` |

### Bash 脚本参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `version` | 指定版本号 | `0.2.0` |
| `--skip-tests` | 跳过测试 | `--skip-tests` |
| `--skip-vet` | 跳过 go vet | `--skip-vet` |

## 相关文件

| 文件 | 说明 |
|------|------|
| `scripts/build-release.ps1` | Windows 发布脚本 |
| `scripts/build-release.sh` | Linux/macOS 发布脚本 |
| `Makefile` | 构建命令集合 |
| `cmd/serialhub/main.go` | 版本号定义 |

## 参考

- [Go 跨平台编译](https://golang.org/doc/install/source#environment)
- [语义化版本规范](https://semver.org/lang/zh-CN/)
