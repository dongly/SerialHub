# SerialHub

[English](./README.en.md) | 简体中文

串口（MCU）与网络连接（Web 终端/AI）之间的双向桥接器。

**📚 文档**：[快速开始](./QUICKSTART.md) | [MCP 使用指南](./MCP.md) | [配置与架构](./AGENTS.md) | [发布流程](./RELEASING.md) | [集成测试](./tests/integration/README.md)

## 项目简介

SerialHub 把 MCU 串口同时桥接给人和 AI：

- **人**：浏览器里的 xterm.js Web 终端，实时查看与输入（启动后自动打开浏览器，WSL 下也能弹出 Windows 宿主浏览器）
- **AI**：原生 MCP 服务器（Streamable HTTP + stdio），提供 7 个工具——`serial_list` / `serial_connect` / `serial_write` / `serial_read` / `serial_clear` / `serial_disconnect` / `serial_status`
- 双通道共享**同一串口连接与数据缓冲**，人和 AI 看到的是同一串字节
- **单实例互斥**：每个用户配置/安装目录只允许一个主实例（OS 文件锁）；stdio 模式自动发现并代理到已运行实例
- 单个 Go 二进制（前端内嵌），支持 Windows（系统托盘）/ Linux / macOS；所有参数均可通过 TOML 或命令行配置，数据流可按需记录（`--log-data`）

## 系统架构

```mermaid
flowchart TB
    MCU["MCU"] <-->|"串口 (COM9, 115200, 8N1)"| Serial["串口管理器<br/>Serial Manager"]

    subgraph SerialHub["SerialHub（单二进制）"]
        Serial <-->|"事件总线"| Bridge["数据桥接<br/>DataBridge"]
        Bridge <-->|"读写"| Buffer["DataBuffer<br/>共享缓冲"]
        Bridge <-->|"WebSocket (端口 5050)"| Web["Web 终端<br/>xterm.js"]
        Bridge <-->|"JSON-RPC (HTTP)"| MCP["MCP 服务<br/>7 个工具"]
    end

    Web <-->|"WebSocket"| Browser["浏览器<br/>人工操作"]
    MCP <-->|"MCP 协议"| AI["AI 工具<br/>OpenCode / iFlow CLI 等"]
```

## 安装

从 [GitHub Releases](https://github.com/dongly/serialhub/releases) 下载对应平台的压缩包，
解压即用（Windows 包内含 `serialhub.ps1`/`serialhub.bat` 启动脚本）。

已安装旧版本可直接自升级（配置与日志保留不动）：

```bash
serialhub upgrade          # 查询 GitHub Releases 最新版，下载校验并原子替换自身
serialhub uninstall        # 卸载：dry-run 列清单确认后清理 MCP 条目/配置/日志/二进制
```

网络代理遵从 `HTTPS_PROXY`/`HTTP_PROXY`；私有加速可设 `SERIALHUB_GITHUB_API`（默认 `https://api.github.com`）。
详细步骤（PATH 配置、安装验证、WSL USB 串口挂载）见 [QUICKSTART.md](./QUICKSTART.md)。

## 快速开始

**场景：人工 + AI 同时调试**

1. 启动 SerialHub：

```bash
serialhub -p COM9 --host 0.0.0.0 -D
```

2. 人工通过 Web 终端监视：浏览器打开 `http://localhost:5050/terminal`

3. AI 工具通过 HTTP MCP 连接：

```json
{
  "mcp": {
    "servers": {
      "serialhub": {
        "type": "remote",
        "url": "http://localhost:5050/mcp",
        "oauth": false
      }
    }
  }
}
```

4. 串口数据同时转发到 Web 终端和 AI 接口，两者可独立向串口发送命令。

> 一键写入 MCP 客户端配置：`serialhub setup`（默认 stdio 本地模式，支持 OpenCode / Claude Code / Cursor / Windsurf / VS Code / Codex）。

## 命令参考

### `serialhub`（默认：serve 模式）

```bash
serialhub                                    # 默认配置启动
serialhub -p COM8                            # 指定串口
serialhub -p COM8 -b 9600 --parity even      # 完整串口参数
serialhub -m 8080                            # 使用 8080 端口
serialhub --host 0.0.0.0                     # 监听所有网络接口（局域网访问需要）
serialhub --stdio                            # stdio 模式（MCP 客户端本地拉起）
serialhub -c config.toml                     # 使用配置文件
serialhub -D                                 # 调试模式
```

| 选项 | 简写 | 说明 | 默认值 |
|------|------|------|--------|
| `--serial-port <port>` | `-p` | 串口名 | 配置文件或空 |
| `--baud-rate <rate>` | `-b` | 波特率 | 115200 |
| `--data-bits <bits>` | `-d` | 数据位（5/6/7/8） | 8 |
| `--parity <type>` | - | 校验位（none/even/odd） | none |
| `--stop-bits <bits>` | `-s` | 停止位（1/2） | 1 |
| `--mcp-port <port>` | `-m` | MCP HTTP 服务端口 | 5050 |
| `--host <host>` | - | 监听地址 | 127.0.0.1 |
| `--config <path>` | `-c` | 配置文件路径 | - |
| `--debug` | `-D` | 启用调试模式 | false |
| `--log-data` | - | 输出数据内容日志（500ms 时间窗聚合、单条展示截断 512 字节；也可用 SERIALHUB_LOG_DATA=1，显式 `--log-data=false` 优先；两者不回写配置文件） | false |
| `--stdio` | - | stdio 模式：MCP 客户端本地拉起（发现主实例则透明代理） | false |
| `--minimized` | - | 由脚本启动，窗口最小化（Windows 下同时隐藏控制台）；浏览器仍默认自动打开 | false |
| `--no-browser` | - | 跳过自动打开浏览器（日志仍会提示 Web 终端地址） | false |

Windows 上默认启动系统托盘（图标颜色表示串口状态，右键菜单可连接/断开串口、显示控制台、退出），详见 [QUICKSTART.md](./QUICKSTART.md#windows-系统托盘)。

### Web 终端

`http://localhost:5050/terminal`：实时显示串口输出、键盘输入发送到串口（支持 Ctrl+C 等控制字符）、断线自动重连。

### MCP 工具

| 工具名 | 描述 | 参数 |
|--------|------|------|
| `serial_list` | 列出系统中所有可用的串口 | - |
| `serial_connect` | 连接到指定串口 | `port`（必填），`baudRate?`（默认 115200） |
| `serial_disconnect` | 断开当前串口连接 | - |
| `serial_write` | 向串口发送数据 | `data`（必填），`addNewline?`（默认 true，自动追加换行符，置 false 时禁用） |
| `serial_read` | 阻塞式读取串口数据，等待数据到达后返回 | `timeout?`（默认 1000ms，0=无限等待），`maxSize?`（默认 4096 字节） |
| `serial_clear` | 清空 read 缓冲区，丢弃尚未读取的数据 | - |
| `serial_status` | 获取串口连接状态 | - |

标准工作流：`serial_list` → `serial_connect` → `serial_write` → `serial_read` → `serial_disconnect`。
cURL/Python 调用示例、典型工作流（命令-响应/持续监听）、错误处理、使用时机与最佳实践见 [MCP.md](./MCP.md)。

## 配置说明

配置优先级：**CLI 参数 > 配置文件 > 默认值**

配置文件查找顺序（未指定 `-c` 时）：

- **Linux/macOS**：`./config.toml`（当前目录）> `~/.config/serialhub/config.toml`（`XDG_CONFIG_HOME`）；可执行文件目录下存在旧 `config.toml` 且用户配置目录无配置时，首次启动自动迁移（移动）到用户配置目录；三处皆无则新建写用户配置目录。
- **Windows**：可执行文件目录下的 `config.toml`（与历史版本一致）。

配置回写时机：合并 CLI 参数后的配置**在成功取得单实例锁（成为主实例）后才写回** `config.toml`；被拒绝的重复实例与 stdio 代理模式不修改配置文件（避免 `-m` 等本次参数污染磁盘配置）。

日志目录默认值：Linux/macOS 为 `~/.config/serialhub/logs/`（旧 `logs/` 历史日志不迁移），Windows 仍为可执行文件目录下 `logs/`；均可用 `logDir` 或 `SERIALHUB_LOG_DIR` 覆盖。

配置文件格式（TOML），支持 `#` 注释，完整示例见 [config.example.toml](./config.example.toml)：

```toml
[serial]
port = ""           # 串口号，为空时不自动连接
baudRate = 115200
dataBits = 8
parity = "none"     # none / even / odd
stopBits = 1

[mcp]
httpPort = 5050
```

## 开发

```bash
go run ./cmd/serialhub          # 开发运行
go build -o bin/serialhub ./cmd/serialhub
go test ./...                   # 测试（internal/testutil 提供 mock 串口/连接）
go vet ./...
```

- 在 WSL 中测试 Windows 版本（托盘、单实例锁、PowerShell 脚本）的方法与坑见 [docs/wsl-windows-testing.md](./docs/wsl-windows-testing.md)。
- 硬件在环测试由 `SERIALHUB_HARDWARE_TEST=1` 控制，`SERIALHUB_TEST_PORT` 指定端口。
- 发布流程见 [RELEASING.md](./RELEASING.md)。

## 许可证 / License

本项目采用 [Apache License 2.0](./LICENSE)（Copyright 2026 dongly）发布。

This project is licensed under the [Apache License 2.0](./LICENSE).
