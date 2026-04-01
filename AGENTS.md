# SerialHub - AI 代理指南

## 项目概述
SerialHub 是一个串口（MCU）与网络连接（Telnet/AI）之间的双向桥接器，使用 Go 语言实现。

### 核心架构
**Telnet 和 AI 接口同时连接同一个串口**：
- 串口数据**同时转发**到 Telnet 和 AI 接口
- Telnet 和 AI 的输入都**独立转发**到串口
- 支持多个 Telnet 连接并发访问

```
MCU ←→ 串口 ←→ SerialHub
                 ├→ Telnet Server (人工监视/操作）
                 └→ AI Interface (AI 工具程序化访问）
```

## 技术栈

| 组件 | Go 库 |
|------|------|
| 运行时 | Go 1.24+ |
| 串口 | go.bug.st/serial (842⭐) |
| AI 接口 | github.com/modelcontextprotocol/go-sdk (官方 v1.4.1) |
| Telnet | net 标准库 |
| HTTP+SSE | github.com/joshuafuller/sse/v3 + net/http |
| CLI | github.com/spf13/cobra |
| 配置 | github.com/spf13/viper |
| 系统托盘 | github.com/getlantern/systray |
| 验证 | struct tags + github.com/go-playground/validator |
| 测试 | testing 标准库 |
| 日志 | github.com/sirupsen/logrus |

## 构建 / 检查 / 测试命令

```bash
# 开发运行
go run ./cmd/serialhub              # 运行 CLI（MCP stdio 模式）
go run ./cmd/serve                  # 运行 HTTP 服务

# 构建命令
go build -o bin/serialhub.exe ./cmd/serialhub
go build -o bin/serve.exe ./cmd/serve

# 测试命令
go test ./...                       # 运行所有测试
go test ./pkg/serial                # 运行单个包测试
go test -v ./pkg/serial             # 详细输出
go test -run TestConnect ./pkg/serial  # 运行单个测试函数
go test -cover ./...                # 测试覆盖率

# 代码检查
go vet ./...                        # 静态分析（零错误）
golangci-lint run                   # 完整 lint（需安装）

# 类型检查（编译时自动）
go build ./...                      # 编译检查类型错误

# 依赖管理
go mod tidy                         # 整理依赖
go get go.bug.st/serial@latest      # 更新依赖
```

## 文件结构

```
github.com/yourname/serialhub/
├── cmd/
│   ├── serialhub/           # CLI 主入口 + MCP stdio 模式
│   │   └── main.go
│   └── serve/               # HTTP 服务入口（含托盘集成）
│       └── main.go
├── pkg/
│   ├── serial/              # 串口管理
│   │   ├── manager.go       # SerialManager（channel 通信）
│   │   └── config.go        # 串口配置结构
│   ├── telnet/              # Telnet 服务
│   │   ├── server.go        # TelnetServer（net 标准库）
│   │   └── client.go        # 客户端管理
│   ├── mcp/                 # MCP 服务
│   │   ├── server.go        # MCP 服务入口 + 工具注册
│   │   ├── transport/
│   │   │   ├── stdio.go     # Stdio 传输（内置）
│   │   │   └── httpsse.go   # HTTP+SSE JSON-RPC 传输
│   │   └── tools/           # MCP 工具（每个文件一个工具）
│   │       ├── serial_list.go
│   │       ├── serial_connect.go
│   │       ├── serial_disconnect.go
│   │       ├── serial_write.go
│   │       ├── serial_read.go
│   │       └── serial_status.go
│   ├── bridge/              # 数据桥接
│   │   ├── bridge.go        # DataBridge（串口↔Telnet↔MCP）
│   │   └── events.go        # Channel 定义
│   ├── config/              # 配置管理
│   │   └── config.go        # Viper 配置（JSON 文件 + CLI 参数）
│   └── tray/                # 系统托盘
│       └── tray.go          # Systray 管理（跨平台）
├── internal/
│   ├── buffer/              # DataBuffer
│   │   └── buffer.go        # 数据缓冲区
│   └── service/             # 服务状态管理
│       └── manager.go       # PID/端口文件
├── go.mod
├── go.sum
├── Makefile                 # 构建脚本
└── README.md
```

## 代码风格

### 导入顺序
```go
// 1. 标准库
import (
    "context"
    "fmt"
    "net"
)

// 2. 第三方库
import (
    "go.bug.st/serial"
    "github.com/spf13/cobra"
)

// 3. 本地模块
import (
    "github.com/yourname/serialhub/pkg/config"
    "github.com/yourname/serialhub/pkg/bridge"
)
```

### 命名规范

| 元素 | 规范 | 示例 |
|------|------|------|
| 包名 | lowercase，单个单词 | `serial`, `telnet`, `mcp` |
| 结构体 | PascalCase | `SerialManager`, `DataBridge` |
| 接口 | PascalCase + `er` 后缀 | `Connector`, `DataReader` |
| 公有方法 | PascalCase | `Connect()`, `WriteLine()`, `Broadcast()` |
| 私有方法 | camelCase | `handleSerialData()`, `readLoop()` |
| 常量 | camelCase 或 PascalCase | `DefaultBaudRate`, `maxBufferSize` |
| 导出字段 | PascalCase | `Port`, `BaudRate` |
| 私有字段 | camelCase | `port`, `baudRate`, `stopChan` |
| Channel | camelCase + `Chan` 后缀 | `dataChan`, `errChan` |
| 工具函数 | `execute` 前缀 | `executeSerialWrite()` |

### 导出
- **仅导出必要的内容**，最小化公开 API
- 结构体字段按需导出，使用 struct tags 标注

### 类型风格
- 对象形状用 `struct`，行为用 `interface`
- 类型与使用处分开定义（文件顶部）
- 使用 struct tags 进行验证和配置映射
- 所有公有方法**显式标注返回类型**

### 错误处理
- 用户错误消息使用**中文**：`return fmt.Errorf("串口未连接")`
- MCP 工具**不返回 error**，返回 `ToolResult` 结构：
  ```go
  type ToolResult struct {
      Success bool   `json:"success"`
      Message string `json:"message"`
      Data    any    `json:"data,omitempty"`
  }
  ```
- 错误包装：`fmt.Errorf("连接失败: %w", err)`
- 非关键操作静默处理：`defer port.Close()`

### 注释
- 文件顶部 `// Package xxx 提供功能描述`
- 所有公有类型/方法用 **Go 文档注释**（中文）
- 行内注释用 `//`（中文）
- 测试描述用中文：`func Test应正确创建实例(t *testing.T)`

### 其他约定
- **不做注释**（除非用户要求）——保持代码精简
- 构造函数返回 error：`func NewSerialManager(cfg *Config) (*SerialManager, error)`
- Getter 方法命名：`GetConfig()` 返回副本，`IsConnected()` 返回布尔值
- 使用 `context.Context` 控制超时和取消
- 运行时日志用 `logrus` + `[SerialHub]` 前缀
- 清理方法命名为 `Close()` 或 `Stop()`
- Channel 初始化：`make(chan []byte, bufferSize)`
- 使用 `select` 监听多个 channel

## 架构原则

1. **关注点分离**：串口、Telnet、MCP 各自独立包
2. **Channel 模式**：模块通过 channel 通信，替代 EventEmitter
3. **错误恢复力**：串口/Telnet/MCP 任一故障不影响其他模块
4. **配置驱动**：所有端口、超时、缓冲区大小可配置
5. **MCP 工具模式**：每个工具文件定义 Input struct + execute 函数 + ToolResult
6. **Context 传递**：所有阻塞操作接收 `context.Context`

## 开发流程

```
开发 → go vet → go build → go test → commit
```

### 检查清单
- [ ] `go vet ./...` — 零错误
- [ ] `go build ./...` — 编译成功
- [ ] 在 `_test.go` 文件中添加/更新测试
- [ ] `go test ./...` — 所有测试通过
- [ ] `git commit` 提交更改