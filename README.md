# SerialHub

串口（MCU）与网络连接（Web终端/AI）之间的双向桥接器。

**📚 文档**: [MCP 使用指南](./MCP.md) | [项目架构](./AGENTS.md) | [集成测试](./tests/integration/README.md)

## 项目简介

SerialHub 通过以下方式实现 AI 辅助调试 MCU 程序：

- 将 MCU 串口输出同时转发到 Web 终端（供人工监视）和 AI 接口（供程序化分析）
- 支持人工操作员（通过浏览器）和 AI 工具的双向通信
- 支持 MCU Shell 操作，用于运行时控制和调试

## 系统架构

```
┌─────────────┐
│     MCU     │
└──────┬──────┘
       │ 串口 (COM9, 115200, 8N1)
       ▼
┌─────────────────────────────────────────┐
│              SerialHub                  │
│                                         │
│  ┌─────────────┐    ┌───────────────┐  │
│  │  串口管理器  │    │  数据桥接     │  │
│  │   Serial    │◄──►│  (事件总线)    │  │
│  │   Manager   │    │  DataBridge   │  │
│  └─────────────┘    └───────┬───────┘  │
│                             │          │
│              ┌──────────────┼────────┐ │
│              ▼              ▼        ▼ │
│       ┌───────────┐  ┌──────────┐ ... │
│       │   Web     │  │   MCP    │     │
│       │  终端     │  │  服务    │     │
│       │ (端口5000)│  │ (HTTP)   │     │
│       └───────────┘  └──────────┘     │
└─────────────────────────────────────────┘
       │                    │
       ▼                    ▼
┌─────────────┐     ┌─────────────┐
│   浏览器     │     │  AI 工具     │
│ (人工操作)   │     │ (OpenCode,  │
│             │     │  iFlow CLI) │
└─────────────┘     └─────────────┘
```

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.26+ |
| 串口通信 | go.bug.st/serial |
| AI 接口 | MCP (Model Context Protocol) / go-sdk |
| Web 终端 | WebSocket / xterm.js |
| CLI | spf13/cobra |
| 配置 | spf13/viper |
| 系统托盘 | getlantern/systray |
| 日志 | sirupsen/logrus |

## 功能特性

- **双路转发**：串口数据同时转发到 Web 终端和 AI 接口
- **双向通信**：Web 终端或 AI 发送的命令均可传输到 MCU
- **Web 终端**：基于 WebSocket 的浏览器终端，支持 xterm.js
- **MCP 协议**：通过 HTTP (StreamableHTTP) 提供 AI 工具集成
- **可配置**：所有端口、波特率、超时参数均可通过 TOML 或命令行配置
- **可观测**：所有数据流均可记录和追踪
- **错误恢复**：网络/串口故障时优雅处理，不影响其他功能

## 构建

```bash
go build -o bin/serialhub.exe ./cmd/serialhub
```

## 命令参考

### `serialhub`（默认：serve 模式）

启动 HTTP + Web 终端服务器：

```bash
serialhub                                    # 默认配置启动
serialhub -p COM8                            # 指定串口
serialhub -p COM8 -b 9600 --parity even      # 完整串口参数
serialhub -m 8080                            # 使用 8080 端口
serialhub --host 0.0.0.0                     # 监听所有网络接口
serialhub -c config.toml                     # 使用配置文件
serialhub -D                                 # 调试模式
```

完整的选项：

| 选项 | 简写 | 说明 | 默认值 |
|------|------|------|--------|
| `--serial-port <port>` | `-p` | 串口名 | 配置文件或空 |
| `--baud-rate <rate>` | `-b` | 波特率 | 115200 |
| `--data-bits <bits>` | `-d` | 数据位（5/6/7/8） | 8 |
| `--parity <type>` | - | 校验位（none/even/odd） | none |
| `--stop-bits <bits>` | `-s` | 停止位（1/2） | 1 |
| `--mcp-port <port>` | `-m` | MCP HTTP 服务端口 | 5000 |
| `--host <host>` | - | 监听地址 | 127.0.0.1 |
| `--config <path>` | `-c` | 配置文件路径 | - |
| `--debug` | `-D` | 启用调试模式 | false |
| `--no-tray` | - | 禁用系统托盘（仅 Windows） | false |

### 系统托盘（Windows）

`serialhub` 在 Windows 上默认启动系统托盘图标，启动后自动隐藏控制台窗口。

**托盘图标状态：**
- 灰色 — 未连接串口
- 绿色 — 串口已连接
- 红色 — 连接错误

**右键菜单功能：**

| 菜单项 | 功能 |
|--------|------|
| 串口信息 | 点击可连接/断开串口 |
| 端口信息 | 显示 MCP 端口（不可点击） |
| 显示/隐藏控制台 | 切换控制台窗口 |
| 退出 | 关闭 SerialHub |

**交互方式：**
- 双击托盘图标：切换控制台窗口显示/隐藏
- 右键托盘图标：打开菜单

```bash
# 禁用托盘（保持控制台窗口）
serialhub --no-tray
```

### 快速开始

**场景：人工 + AI 同时调试**

1. 启动 SerialHub：

```bash
serialhub -p COM9 --host 0.0.0.0 -D
```

2. 人工通过 Web 终端连接监视：

打开浏览器访问 `http://localhost:5000/terminal`

3. AI 工具通过 HTTP MCP 连接：

```json
{
  "mcp": {
    "serialhub": {
      "type": "remote",
      "url": "http://localhost:5000/mcp",
      "enabled": true
    }
  }
}
```

4. 串口数据同时转发到 Web 终端和 AI 接口，两者可独立向串口发送命令。

### 通过 Web 终端访问

SerialHub 内置基于 WebSocket 的终端界面，使用 xterm.js 提供完整的终端体验。

**访问地址**：`http://localhost:5000/terminal`

**功能特性**：
- 实时显示串口输出
- 支持键盘输入发送到串口
- 支持 Ctrl+C、Ctrl+D 等控制字符
- 自动重连

### MCP HTTP API 调用

服务器启动后，可通过 JSON-RPC 调用 MCP 工具：

```bash
# 健康检查
curl http://localhost:5000/health

# 列出可用串口
curl -X POST http://localhost:5000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_list"},"id":1}'

# 连接串口
curl -X POST http://localhost:5000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_connect","arguments":{"port":"COM9"}},"id":2}'

# 发送命令（自动追加换行符）
curl -X POST http://localhost:5000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_write","arguments":{"data":"help"}},"id":3}'

# 读取串口返回数据（阻塞等待，timeout=0 表示无限等待）
curl -X POST http://localhost:5000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_read","arguments":{"timeout":5000}},"id":4}'
```

## MCP 工具列表

| 工具名 | 描述 | 参数 |
|--------|------|------|
| `serial_list` | 列出系统中所有可用的串口 | - |
| `serial_connect` | 连接到指定串口 | `port`（必填），`baudRate?`（默认 115200） |
| `serial_disconnect` | 断开当前串口连接 | - |
| `serial_write` | 向串口发送数据 | `data`（必填），`addNewline?`（默认 true，自动追加换行符） |
| `serial_read` | 阻塞式读取串口数据，等待数据到达后返回 | `timeout?`（默认 1000ms，0=无限等待），`maxSize?`（默认 4096 字节） |
| `serial_status` | 获取串口连接状态 | - |

### AI 工具使用指南

#### 标准工作流程

```
serial_list → 识别目标串口 → serial_connect → serial_write 发送命令 → serial_read 读取响应
```

#### 工具使用时机

| 场景 | 推荐工具 | 说明 |
|------|----------|------|
| 不知道串口名 | `serial_list` | 获取可用串口列表，根据 vendorId/productId 或厂商名识别目标设备 |
| 开始调试前 | `serial_connect` | 必须先连接串口才能进行后续操作 |
| 发送 Shell 命令 | `serial_write` + `serial_read` | 写入后立即读取响应，如 `help`、`version`、`reboot` |
| 发送调试指令 | `serial_write` | 向 MCU 发送控制命令或配置参数 |
| 获取命令输出 | `serial_read` | 读取设备返回的数据，timeout=0 无限等待适合不确定响应时间 |
| 检查连接状态 | `serial_status` | 操作前确认已连接，或操作失败时检查连接是否断开 |
| 切换设备 | `serial_disconnect` → `serial_connect` | 先断开当前连接，再连接新设备 |
| 结束会话 | `serial_disconnect` | 释放串口资源 |

#### 常见操作示例

**1. 首次连接设备**

```
// 步骤 1: 查找可用串口
serial_list()
// 返回: { ports: [{ path: "COM6", vendorId: "0D28", productId: "0202" }, ...] }

// 步骤 2: 根据硬件 ID 识别目标设备，连接
serial_connect({ port: "COM6", baudRate: 115200 })
// 返回: { success: true, port: "COM6", baudRate: 115200 }
```

**2. 发送命令并获取响应**

```
// 发送命令（自动追加换行符）
serial_write({ data: "version" })
// 返回: { success: true, bytesWritten: 8 }

// 读取响应（等待 2 秒）
serial_read({ timeout: 2000 })
// 返回: { data: "MCU v1.2.3\nBuild: 2024-01-15\n", timedOut: false, bytes: 28 }
```

**3. 等待不确定时间的响应**

```
// timeout=0 表示无限等待，直到有数据到达
serial_write({ data: "flash_verify" })  // 耗时操作
serial_read({ timeout: 0 })  // 等待直到设备返回结果
```

**4. 切换到不同设备**

```
serial_disconnect()  // 断开当前连接
serial_list()        // 重新查找串口
serial_connect({ port: "COM7" })  // 连接新设备
```

**5. 检查连接状态**

```
serial_status()
// 已连接: { connected: true, port: "COM6", baudRate: 115200 }
// 未连接: { connected: false }
```

#### 错误处理

| 错误情况 | 原因 | 解决方案 |
|----------|------|----------|
| serial_write 返回 `串口未连接` | 未调用 serial_connect 或连接已断开 | 先调用 serial_connect |
| serial_read 返回 `timedOut: true` | 超时内无数据到达 | 增大 timeout 或检查设备是否正常响应 |
| serial_connect 返回 `success: false` | 串口不存在、权限问题或设备占用 | 检查 serial_list 结果、确认波特率配置 |
| 读取内容不完整 | 输出较长，一次读取未完全获取 | 循环调用 serial_read 直到 timedOut=true |

#### 最佳实践

1. **始终先检查状态**：复杂操作前调用 `serial_status` 确认连接有效
2. **匹配波特率**：`baudRate` 必须与目标设备配置一致，常见值 115200、9600
3. **合理设置 timeout**：常规命令 1-5 秒，耗时操作设为 0（无限等待）
4. **发送后立即读取**：`serial_write` 完成后立即 `serial_read`，避免数据堆积
5. **解析输出时考虑换行**：大多数 Shell 命令响应包含 `\n` 换行符

## 配置说明

配置优先级：**CLI 参数 > 配置文件 > 默认值**

配置文件格式（TOML），支持 `#` 注释：

```toml
# 日志目录，为空则保存到可执行文件目录下的 logs/
# logDir = "D:/Logs"

[serial]
port = ""           # 串口号，为空时不自动连接
baudRate = 115200
dataBits = 8
parity = "none"     # none / even / odd
stopBits = 1

[mcp]
httpPort = 5000
```

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `serial.port` | `""`（空） | 串口号，为空时不自动连接 |
| `serial.baudRate` | `115200` | 波特率 |
| `serial.dataBits` | `8` | 数据位（5/6/7/8） |
| `serial.parity` | `"none"` | 校验位（none/even/odd） |
| `serial.stopBits` | `1` | 停止位（1/2） |
| `mcp.httpPort` | `5000` | MCP HTTP 服务端口（同时提供 Web 终端） |
| `logDir` | `""` | 日志目录，为空则保存到可执行文件目录下的 `logs/` |
| `debug` | `false` | 调试模式开关 |

## 开发

```bash
# 开发运行
go run ./cmd/serialhub

# 构建
go build -o bin/serialhub.exe ./cmd/serialhub

# 测试
go test ./...

# 静态分析
go vet ./...

# 整理依赖
go mod tidy
```

## 许可证

MIT
