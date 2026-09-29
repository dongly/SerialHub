# 在 WSL 中测试 Windows 版本

适用场景：Windows 独有行为（托盘、`LockFileEx` 单实例锁、PowerShell 脚本、COM 口）在 Linux 上无法直接验证的部分。以下方法均经实测（pwsh 7.6.6 + Windows PowerShell 5.1，WSL2 interop）。

## 1. 交叉构建与部署

```bash
# 构建 Windows exe（产物放 /tmp，勿留在仓库根）
GOOS=windows GOARCH=amd64 go build -o /tmp/opencode/serialhub-test.exe ./cmd/serialhub

# 静态检查 Windows-only 包（Linux 上 go test 跑不到）
GOOS=windows GOARCH=amd64 go vet ./pkg/tray/ ./internal/instance/

# 部署到独立测试目录——不要直接覆盖正式安装（D:\DevTools\serialhub）
# Windows 单实例锁按 exe 目录分作用域，独立目录可并行测试、不干扰正式实例
mkdir -p /mnt/d/DevTools/serialhub-test
cp /tmp/opencode/serialhub-test.exe /mnt/d/DevTools/serialhub-test/serialhub.exe
```

## 2. 通过 pwsh.exe 驱动 Windows

两个解释器都在 PATH：`pwsh.exe`（7.x）与 `powershell.exe`（5.1）。**脚本要两个都过**（5.1 兼容性坑最多）。

```bash
# 运行 WSL 内的 ps1：UNC 路径 + -ExecutionPolicy Bypass
# （RemoteSigned 会拦"未签名的网络脚本"，UNC 属于网络位置）
pwsh.exe -NoProfile -ExecutionPolicy Bypass \
  -File '\\wsl.localhost\Ubuntu\home\dongly\SerialHub\tests\instance-check.ps1' -Port 5050
echo "rc=$?"   # -File 模式下脚本 exit 码透传

# 内联命令（zsh 外层用单引号，防 $ 展开）
pwsh.exe -NoProfile -Command 'Get-Process serialhub | Select-Object Id,Path'

# 语法检查（不执行，5.1 解析器最严格）
powershell.exe -NoProfile -Command '
$e=$null
[void][System.Management.Automation.Language.Parser]::ParseFile("<脚本UNC路径>",[ref]$null,[ref]$e)
if($e){"PARSE ERRORS: $($e.Count)"}else{"ParseFile OK"}'
```

## 3. 中文编码（最容易踩的坑）

- interop 管道下 PowerShell 输出按 **GBK（cp936）** 到达 WSL，中文乱码不是 bug：
  ```bash
  pwsh.exe ... 2>&1 | iconv -f GBK -t UTF-8
  ```
- 含中文的 `.ps1` **必须带 UTF-8 BOM**，否则 PS 5.1 按 ANSI 读，注释乱码甚至语法错。
- GBK 控制台显示不了 `✓`/`✗` 等符号（变 `?`），脚本诊断标记别依赖它们。

## 4. PowerShell 语言坑（5.1 实测）

| 坑 | 规避 |
|---|---|
| `try{}` 不能当表达式内联（`"x: " + (try {...})` 报错） | 先赋值再拼接 |
| `$args` 是自动变量，作参数名会被展开 | 改名 `$toolArgs` |
| `Invoke-WebRequest` 默认**不发 Accept 头**，MCP SDK 直接拒 400 | 显式 `-Headers @{Accept='application/json, text/event-stream'}` |
| `Invoke-RestMethod` 响应无 `.Headers`（拿不到 `Mcp-Session-Id`） | 取头用 `Invoke-WebRequest`，调用用 `Invoke-RestMethod` |
| 数组 `-match` 不填充 `$Matches` | 用 `Select-String` 或 `[int]::TryParse` |
| MCP 工具结果文本在 `result.content[0].text` | 不在 result 顶层 |

## 5. 网络与进程语义

- **pwsh.exe 里的 `127.0.0.1` 是 Windows 侧**，不是 WSL 侧（WSL2 NAT；Windows→WSL 的 localhost 转发通常可用，反向不成立）。
- HTTP 探测：`Invoke-WebRequest -UseBasicParsing http://127.0.0.1:5050/health`
- 进程管理：
  ```powershell
  Start-Process -FilePath "D:\...\serialhub.exe" -ArgumentList "-m","5151","--minimized","--no-browser" -PassThru
  Stop-Process -Id <pid> -Force    # 模拟崩溃：lock 文件残留、内核释放锁
  ```

## 6. Windows 实测清单（单实例锁等）

按序验证，全部可在 WSL 内经 pwsh.exe 完成：

1. 独立目录起实例 → `instance.lock` 生成于 `%LOCALAPPDATA%\serialhub\`（与 exe 位置无关），`Get-Content -Raw` 可读（内容 pid/port/host/started_at）。
2. 同目录第二实例（不同端口）→ 检测到主实例后以代理模式运行（stdin/stdout 透明代理挂起），不再报错退出。
3. 两实例并发启动 → 恰一个持锁成为主，另一个转代理模式。
4. `Stop-Process -Force` 后残留 lock → 新进程接管并覆盖元数据。
5. `tests/instance-check.ps1 -Port <p>` → rc=0；`-Port 0` / `-Port 70000` → rc=3。
6. 代理模式实例**不得**修改 `config.toml`（配置回写发生在成功取得单实例锁之后）——测试后核对 `HTTPPort` 未被 `-m` 参数污染。

## 7. 清理

```powershell
Stop-Process -Id <测试实例pid> -Force
Remove-Item "D:\DevTools\serialhub-test" -Recurse -Force
Get-Process serialhub   # 确认只剩正式实例
```
