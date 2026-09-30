# tests/ — SerialHub 测试

| 条目 | 类型 | 说明 |
|------|------|------|
| `test-deployed.sh` | 编译产物端到端 | 见下节 |
| `instance-check.sh` | 实例状态自检（Linux/WSL 侧） | 只读检查 lock 文件、`/health`、MCP `serial_list` 完整握手；退出码 0=全部通过 / 1=未发现实例 / 2=MCP 检查失败 / 3=参数错误 |
| `instance-check.ps1` | 实例状态自检（Windows 侧） | 同上（PowerShell 版，`-Port 5050,5098` 追加探测端口） |
| `integration/` | Python 集成测试（pytest） | 覆盖 CLI、服务器生命周期、配置、MCP 工具、WebSocket、Web 终端、日志与串口硬件；详见 `integration/README.md` |
| `integration/playwright/` | 浏览器 UI 测试 | pytest + Playwright |

## test-deployed.sh — 编译产物端到端测试

对**编译出来的真实二进制**（而非 `go test`）验证 uninstall / upgrade 全链路：

```bash
bash tests/test-deployed.sh            # 默认 all：Windows + Linux 两套场景
bash tests/test-deployed.sh windows    # 仅 Windows 产物（需在 WSL 中运行）
bash tests/test-deployed.sh linux      # 仅 Linux 产物
```

- **Windows 场景**：交叉编译 exe，经 WSL interop 在沙箱用户目录
  （`USERPROFILE`/`LOCALAPPDATA`/`APPDATA` 重定向）实跑——dry-run 清单、
  延迟自删（PowerShell 码元构造）、随包清理、空目录回收、MCP 条目合并逆
  操作、mock GitHub API（`SERIALHUB_GITHUB_API` 指向 WSL IP）升级到伪造
  release（9.9.9）、弯引号路径（U+2019）。
- **Linux 场景**：ELF 产物同链路；探活拦截用真实（或自动拉起的临时）5050
  实例验证同侧 local 拒绝；dry-run / uninstall 经 `unshare -rn` 网络命名
  空间隔离，避免本机真实实例干扰。
- **隔离与安全**：全程沙箱目录，不触碰真实配置、MCP 条目与运行实例；
  所有命令带 `timeout` + 全局看门狗，任何环节挂死自动退出。
- **环境依赖**：`timeout`、`unshare`（Linux 场景）、`python3`（mock API 与
  打包）；Windows 场景须在 WSL 内运行——不在 WSL 时提示无法测试 Windows
  产物（`windows` 模式退出，`all` 自动降级仅跑 Linux）。

其余测试入口（Go 单元测试、`tools/test-windows.sh`、硬件测试）见根目录
`AGENTS.md` 的「测试」一节。
