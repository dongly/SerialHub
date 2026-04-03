# SerialHub - AI 代理指南

## 项目概述
SerialHub 是串口（MCU）与网络连接（Telnet/AI）的双向桥接器，Go 语言实现。

```
MCU ←→ 串口 ←→ SerialHub
                 ├→ Telnet Server (人工监视)
                 └→ AI Interface (MCP HTTP+SSE)
```

## 技术栈

Go 1.26+ | go.bug.st/serial | modelcontextprotocol/go-sdk | spf13/cobra | spf13/viper | getlantern/systray | sirupsen/logrus

## 构建 / 测试命令

```bash
# 开发
go run ./cmd/serialhub              # 启动服务（默认开启托盘）
go run ./cmd/serialhub --no-tray    # 无托盘模式

# 构建
go build -o bin/serialhub.exe ./cmd/serialhub

# 测试
go test ./...                       # 所有测试
go test ./pkg/serial                # 单个包
go test -run TestConnect ./pkg/serial  # 单个测试函数
go test -cover ./...                # 覆盖率
SERIALHUB_HARDWARE_TEST=1 SERIALHUB_TEST_PORT=COM9 go test ./...  # 硬件测试

# 检查
go vet ./...                        # 静态分析（零错误）
go mod tidy                         # 整理依赖
```

## 文件结构

```
cmd/serialhub/main.go        # CLI 主入口
pkg/serial/                  # 串口管理（manager.go, config.go）
pkg/telnet/                  # Telnet 服务（server.go, client.go）
pkg/mcp/server.go            # MCP 服务（直接使用 SDK SSEHandler）
pkg/mcp/tools/               # MCP 工具（serial_list/connect/disconnect/write/read/status）
pkg/bridge/                  # 数据桥接（bridge.go, events.go）
pkg/config/config.go         # Viper 配置
pkg/tray/                    # 系统托盘（tray.go, assets/, console_*.go）
internal/buffer/             # 数据缓冲区
internal/service/            # 串口配置记忆
internal/testutil/           # 测试工具（helpers, mock_serial, mock_net）
tools/genicons.py            # 托盘图标生成（Python + PIL）
```

## 代码风格

### 导入顺序
```go
// 1. 标准库  2. 第三方库  3. 本地模块
import (
    "context"
    "fmt"
    "go.bug.st/serial"
    "github.com/spf13/cobra"
    "github.com/yourname/serialhub/pkg/config"
)
```

### 命名规范

| 元素 | 规范 | 示例 |
|------|------|------|
| 包名/常量/私有字段 | lowercase/camelCase | `serial`, `maxBufferSize`, `port` |
| 结构体/接口/公有方法 | PascalCase + `er` 后缀 | `SerialManager`, `Connector` |
| Channel | camelCase + `Chan` | `dataChan`, `errChan` |
| MCP 工具函数 | `Execute` 前缀 | `ExecuteSerialConnect()` |

### 错误处理
- 用户错误消息用**中文**：`return fmt.Errorf("串口未连接")`
- MCP 工具返回 `ToolResult{Success, Message, Data}` 结构
- 错误包装：`fmt.Errorf("连接失败: %w", err)`
- 非关键操作静默处理：`defer port.Close()`

### 其他约定
- 构造函数返回 error：`func NewXxx(cfg) (*Xxx, error)`
- Getter 命名：`GetConfig()` 返回副本，`IsConnected()` 返回 bool
- 日志前缀：`logrus` + `[SerialHub]`
- 清理方法：`Close()` 或 `Stop()`
- Channel 初始化：`make(chan []byte, bufferSize)`
- 测试描述用中文：`func Test应正确创建实例(t *testing.T)`

## 架构原则

1. **关注点分离**：串口、Telnet、MCP 各自独立包
2. **Channel 模式**：模块通过 channel 通信
3. **错误恢复力**：任一模块故障不影响其他模块
4. **MCP 工具模式**：每个工具定义 Input struct + Execute 函数 + ToolResult
5. **Context 传递**：所有阻塞操作接收 `context.Context`

## Git 提交规范

### 格式
```
<type>(<scope>): <subject>

<body>
```

### Type

| 类型 | 说明 |
|------|------|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `refactor` | 重构 |
| `test` | 测试 |
| `docs` | 文档 |
| `chore` | 构建/工具/依赖 |

### Scope

`serial`, `telnet`, `mcp`, `tray`, `cli`, `bridge`, `config`, `buffer`

### 原则

1. **原子提交**：每个 commit 只做一件事
2. **频繁提交**：完成小功能就提交
3. **中文描述**：subject 和 body 用中文

### 示例
```bash
git commit -m "feat(tray): 添加串口自动重连功能"
git commit -m "fix(serial): 修复断开连接后端口未释放的问题"
```

## 开发流程

```
开发 → go vet → go build → go test → review → commit
```

- `go vet ./...` — 零错误
- `go build ./...` — 编译成功
- `go test ./...` — 测试通过
- **review** — 自查代码（见下方清单）
- `git commit` — 遵循提交规范

### Review 自查清单

- [ ] 无 `as any`、`@ts-ignore` 等类型逃逸
- [ ] 无空 `catch` 块
- [ ] 无 `fmt.Println` 调试代码残留
- [ ] 无注释掉的死代码
- [ ] 导入顺序正确（标准库 → 第三方 → 本地）
- [ ] 错误消息使用中文
- [ ] 公有方法有文档注释
- [ ] 新功能有对应测试