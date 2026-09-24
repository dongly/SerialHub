# SerialHub 发布指南

发布由 GitHub Actions 全自动完成：推送 `v*` 标签 → 质量门（vet + test）→ 六平台构建 → 自动创建 GitHub Release。

仓库：`github.com/dongly/SerialHub`（remote 名 `github`）

## 发布流程

### 1. 更新版本号

编辑 `pkg/version/version.go`，更新 fallback 版本号：

```go
var (
    Name    = "serialhub"
    Version = "0.5.0"  // 修改为新版本号
)
```

> CI 构建时会通过 `-ldflags "-X .../pkg/version.Version=<tag>"` 注入标签版本，
> 此常量仅作为本地构建的 fallback。

### 2. 提交并推送

```bash
git add pkg/version/version.go
git commit -m "chore: bump version to 0.6.0"
git push github main
```

### 3. 打标签并推送（触发发布）

```bash
git tag -a v0.6.0 -m "Release version 0.6.0"
git push github v0.6.0
```

推送标签后，[Actions](https://github.com/dongly/SerialHub/actions) 自动执行：

1. **质量门**：`go vet ./...` + `go test ./...`，全绿才继续
2. **矩阵构建**：linux/amd64、windows/amd64
   （`CGO_ENABLED=0`，版本号从标签注入）
3. **发布**：自动创建 GitHub Release 并附上压缩包与 SHA256 校验和

### 4. 验证

- Actions 页面工作流全绿
- Release 页面挂 2 个产物（zip/tar.gz）+ SHA256
- 下载一个包，运行 `serialhub --version` 确认版本号

## 发布包内容

| 文件 | 说明 |
|------|------|
| `serialhub(.exe)` | 主程序 |
| `config.toml` | 配置文件模板 |
| `start.ps1` | PowerShell 启动脚本（仅 Windows 包） |
| `README.md` / `MCP.md` / `QUICKSTART.md` | 文档 |
| `VERSION` | 版本信息文件 |
| `LICENSE` | Apache-2.0 许可证 |

Windows 打包为 `.zip`，其余平台为 `.tar.gz`。

## 故障排除

- **CI 失败**：在 Actions 页面查看日志；可在 Release 页面手动触发 `workflow_dispatch` 重跑
- **标签打错**：`git tag -d vX.Y.Z && git push github :refs/tags/vX.Y.Z` 删除后重打
- 本地验证 workflow 语法可直接用 `workflow_dispatch` 手动触发一次

## 相关文件

| 文件 | 说明 |
|------|------|
| `.github/workflows/release.yml` | 发布工作流 |
| `pkg/version/version.go` | 版本号定义 |
