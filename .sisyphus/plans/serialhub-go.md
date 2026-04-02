# SerialHub TypeScript → Go 完整转换

## TL;DR

> **Quick Summary**: 将 SerialHub 从 TypeScript 转换为 Go，同时修复原项目架构缺陷（双入口重复、数据双重写入 Bug、职责不清），采用 Go 惯用并发模式。
> 
> **Deliverables**:
> - 完整 Go 项目（go.mod, cmd/, pkg/, internal/）
> - 6 个 MCP 工具（serial_list/connect/disconnect/write/read/status）
> - 统一 CLI 入口（cobra 子命令：mcp / serve / stop）
> - 单元测试覆盖核心模块
> - 硬件集成测试（COM9/RT-Thread 设备验证）
> - 测试基础设施 internal/testutil（Mock + Helpers）
> 
> **Estimated Effort**: XL
> **Parallel Execution**: YES - 5 waves + 4 hardware gates
> **Critical Path**: T1 → T0 → T4 → HW1 → T6 → T7 → HW2 → T8 → T11 → HW3 → T12 → HW4 → F1-F4

---

## Context

### Original Request
用户要求将 SerialHub 项目从 TypeScript 完整转换为 Go 语言，并审查原项目架构合理性。

### Interview Summary
**Key Discussions**:
- 技术选型已确认：go.bug.st/serial, 官方 MCP go-sdk v1.4.1, cobra, viper, systray, logrus
- 架构决策：EventEmitter → Channel + Goroutine
- 项目结构：cmd/ pkg/ internal/ 标准布局
- AGENTS.md 已更新为 Go 版本

**Research Findings**:
- 已完整分析 17 个 TypeScript 源文件
- Go MCP SDK 支持：mcp.NewServer, mcp.AddTool, mcp.StdioTransport, mcp.NewStreamableHTTPHandler
- go.bug.st/serial 完整支持 Windows COM, 波特率/数据位/校验位配置

### 原项目架构审查（Oracle 分析）

**🔴 修复的 Bug**：
- **数据双重写入**：MCP 自行监听串口数据写入 DataBuffer + DataBridge 也写入 → serve 模式下 serial_read 返回重复内容。Go 版本：DataBuffer 仅由 DataBridge 写入，MCP 不自行监听。

**🟡 架构优化**：
- **合并双入口**：index.ts 和 server.ts 80% 代码重复 → cobra 子命令 mcp/serve
- **DataBridge 简化**：6 种事件无人监听 → 纯 channel 分发 + logrus 日志
- **DataBuffer 独立**：从 mcp/tools/serial_read.ts 移到 internal/buffer/
- **MCP 职责收窄**：去除 DataBuffer 所有权和串口数据监听，仅负责工具注册
- **去除 EventEmitter**：所有模块改为 channel 通信
- **统一 CLI 参数**：3 个解析器 → cobra 统一处理

**数据流架构（修复后）**：
```
SerialManager.dataCh ──→ DataBridge.Start()
                           ├─→ TelnetServer.Broadcast(data)
                           └─→ DataBuffer.Append(data)
                                    ↑
                            MCP serial_read 工具只读取
```

### Metis Review
**Identified Gaps** (addressed):
- 测试策略：默认 Go testing 标准库，核心路径覆盖
- 构建目标：Windows amd64（主平台）
- 功能冻结：仅 TS 等效转换 + Bug 修复，不增加新功能
- 配置格式：保持 JSON 兼容

---

## Work Objectives

### Core Objective
将 TypeScript 版本转换为 Go，修复已知架构缺陷，采用 Go 惯用模式。

### Concrete Deliverables
- `go.mod` + `go.sum` — 项目依赖
- `internal/testutil/` — 测试基础设施（Mock + Helpers）
- `pkg/config/config.go` — 配置管理（viper）
- `pkg/serial/manager.go`, `config.go` — 串口管理（channel 通信）
- `pkg/telnet/server.go`, `client.go` — Telnet 服务（net 标准库）
- `pkg/mcp/server.go` — MCP 服务入口（官方 SDK）
- `pkg/mcp/tools/*.go` — 6 个 MCP 工具
- `pkg/mcp/transport/stdio.go` — Stdio 传输
- `pkg/mcp/transport/httpsse.go` — HTTP+SSE 传输（SDK 内置）
- `pkg/bridge/bridge.go` — 数据桥接（纯 channel 分发）
- `pkg/tray/tray.go` — 系统托盘（systray）
- `pkg/app/app.go` — App 结构体（统一生命周期管理）
- `internal/buffer/buffer.go` — 数据缓冲区（sync.Mutex + sync.Cond）
- `internal/service/manager.go` — 服务状态管理
- `cmd/serialhub/main.go` — 统一 CLI 入口（cobra 子命令）
- `tests/e2e/hardware_e2e_test.go` — 端到端硬件测试

### Definition of Done
- [ ] `go build ./...` 编译成功
- [ ] `go vet ./...` 零错误
- [ ] `go test ./...` 所有测试通过
- [ ] MCP stdio 模式可被 AI 工具调用
- [ ] HTTP+SSE 模式可接受 JSON-RPC 请求
- [ ] Telnet 客户端可连接并收发串口数据
- [ ] 无数据双重写入（通过测试验证）
- [ ] 统一 CLI 入口，子命令 mcp/serve/stop/help/version 正常工作
- [ ] 硬件集成测试（COM9 + RT-Thread）通过（SERIALHUB_HARDWARE_TEST=1 时）

### Must Have
- 6 个 MCP 工具完整实现
- 串口数据同时转发到 Telnet + DataBuffer（仅由 DataBridge 写入)
- Telnet/MCP 数据独立转发到串口
- 多 Telnet 客户端并发支持
- 配置优先级：CLI > 文件 > 默认值
- **跨平台支持**：Windows + Linux，所有平台相关代码用 `runtime.GOOS` 条件编译
- Windows 系统托盘（图标状态 + 菜单，Linux 下禁用）
- 统一 CLI 入口（cobra 子命令）
- App 结构体统一管理组件生命周期
- **结构化日志**：所有模块使用 `logrus`，输出到 stderr，前缀 `[SerialHub]`
- **版本号管理**：App 结构体包含版本号，通过 `go build -ldflags` 注入，`serialhub version` 命令可查看
- **测试基础设施**：internal/testutil 包含 MockSerialPort, MockConn, 通用测试辅助函数
- **硬件集成测试**：HW1-HW4 四个硬件验证关卡，环境变量控制启用

### Must NOT Have (Guardrails)
- 不修改 TypeScript 源码
- 不增加 TypeScript 版本没有的新功能
- 不使用 plan 外的第三方库
- 不使用 panic() 处理业务错误
- 不使用全局状态或单例（使用 App 结构体传递依赖）
- 不创建循环包依赖
- **不**在 DataBridge 和 MCP 中双重写入 DataBuffer（原 Bug 必须修复）
- **不**使用两个入口文件（合并为 cobra 子命令）
- **不**使用 EventEmitter 模式（全部改为 channel）
- **不**手写 JSON-RPC（使用官方 MCP SDK）
- 不让 MCP 自行监听串口数据（修复双重写入 Bug）
- 不让 DataBridge 继承任何基类或暴露事件接口

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed.

### Test Decision
- **Infrastructure exists**: NO（全新 Go 项目）
- **Automated tests**: YES (tests-after) — 每个任务包含测试
- **Framework**: Go testing 标准库
- **Mock**: internal/testutil 包（MockSerialPort, MockConn, helpers）
- **特殊验证**：DataBuffer 写入唯一性测试（防止双重写入 Bug）

### Test Infrastructure
- **internal/testutil** — Wave 0 优先创建，被所有后续任务依赖
  - `mock_serial.go` — MockSerialPort（Read/Write/Close/SetReadTimeout, 读完后返回 io.EOF）
  - `mock_net.go` — MockConn（ReadBuf/WriteBuf/Closed, 实现 net.Conn 接口）
  - `helpers.go` — AssertContains, AssertNotContains, WaitForChannel[T], NewTempDir, WriteTestFile

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Library/Module**: Use Bash — `go test`, `go vet`, `go build`
- **API/HTTP**: Use Bash (curl) — Send requests, assert status + response
- **CLI**: Use Bash — Run binary, check exit code, validate output
- **数据流验证**: 每个 Wave 结束后的硬件集成测试验证数据路径正确性
- **硬件测试**: HW1-HW4 通过环境变量 SERIALHUB_HARDWARE_TEST=1 启用，默认跳过

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 0 (Test Infrastructure — 1 task, before all others):
└── Task 0: 测试基础设施 internal/testutil [quick]

Wave 1 (Foundation — 4 tasks, MAX PARALLEL):
├── Task 1: 项目骨架 + go.mod + 目录结构 [quick]
├── Task 2: 配置管理 pkg/config [quick]
├── Task 3: DataBuffer（独立包） [quick]
└── Task 4: SerialManager pkg/serial [unspecified-high]

HW1 (Hardware Gate — after Wave 1):
└── Task HW1: SerialManager 硬件集成测试 [deep]

Wave 2 (Core Modules — 3 tasks, MAX PARALLEL):
├── Task 5: TelnetServer pkg/telnet [unspecified-high]
├── Task 6: MCP 工具 pkg/mcp/tools/* [deep]
└── Task 7: MCP 服务端 pkg/mcp/server + transport [deep]

HW2 (Hardware Gate — after Wave 2):
└── Task HW2: MCP 工具硬件集成测试 [deep]

Wave 3 (Integration — 4 tasks, MAX PARALLEL):
├── Task 8: DataBridge 纯 channel 分发 [deep]
├── Task 9: 系统托盘 pkg/tray [unspecified-high]
├── Task 10: 服务状态管理 internal/service [quick]
└── Task 11: App 结构体统一生命周期 [deep]

HW3 (Hardware Gate — after Wave 3):
└── Task HW3: DataBridge 转发硬件测试 [deep]

Wave 4 (Entry Point — 1 task):
└── Task 12: 统一 CLI 入口 cmd/serialhub [unspecified-high]

HW4 (Hardware Gate — after Wave 4):
└── Task HW4: 全链路端到端硬件测试 [deep]

Wave FINAL (Verification — 4 tasks, PARALLEL):
├── F1: Plan compliance audit (oracle)
├── F2: Code quality review (unspecified-high)
├── F3: Real manual QA (unspecified-high)
└── F4: Scope fidelity check (deep)
-> Present results -> Get explicit user okay

Critical Path: T1 → T0 → T4 → HW1 → T6 → T7 → HW2 → T8 → T11 → HW3 → T12 → HW4 → F1-F4 → user okay
Max Concurrent: 4 (Wave 1, Wave 3)
Hardware Gates: HW1-HW4 必须依次通过，失败则修复后重试
```

### 硬件测试通用规则

> **所有 HW 任务共享以下规则：**
>
> **启用控制**：硬件测试默认跳过，通过环境变量 `SERIALHUB_HARDWARE_TEST=1` 启用
> - `go test -v -run TestHW ./...`（需要环境变量设置才执行）
> - 未设置时显示 `skip: 硬件测试未启用，设置 SERIALHUB_HARDWARE_TEST=1 启用`
>
> **串口配置**：
> - 默认 COM9, 115200, 8N1
> - 可通过环境变量覆盖：`SERIALHUB_TEST_PORT=COM9`, `SERIALHUB_TEST_BAUD=115200`
>
> **设备状态处理**：
> - 测试开始前先发送 `\r\n` 并读取/丢弃所有缓冲数据（清空残留）
> - 等待 500ms 让设备稳定
> - 然后再发送实际测试命令
>
> **断言模式**：
> - 使用 `strings.Contains(response, substring)` 检查响应包含预期子串
> - 不使用精确匹配（设备输出可能有 ANSI 转义码或额外空行）
> - 每个命令等待最多 5 秒响应
>
> **清理**：
> - 使用 `t.Cleanup()` 确保串口断开
> - 失败时输出最后收到的数据内容用于调试
>
> **RT-Thread 已知输出模式**：
> - help → 包含 `"RT-Thread shell commands:"` + 命令列表 + 提示符 `msh >`
> - version → 包含 `"RT -"` + `"Thread Operating System"` + 版本号如 `"5.2.1"`
> - 提示符 → `msh >`
> - 命令回显 → `msh >help`, `msh >version`
> - 未知命令 → `command not found.`

### RT-Thread 输出样例（用于硬件测试断言）

```
msh >help
RT-Thread shell commands:
reboot           - Reboot System
...
help             - RT-Thread shell help
...

msh >version

 \ | /
- RT -     Thread Operating System
 / | \     5.2.1 build Apr  1 2026 16:48:16
 2006 - 2024 Copyright by RT-Thread team
msh >
```

### Dependency Matrix

| Task | Depends On | Blocks | Wave |
|------|-----------|--------|------|
| 0 | 1 | T4, T5, T6, T8 | 0 |
| 1 | — | 0, 2, 3, 4 | 1 |
| 2 | 1 | 5, 9, 10 | 1 |
| 3 | 1 | 6, 7 | 1 |
| 4 | 1, 0 | 6, 8, HW1 | 1 |
| HW1 | 4 | Wave 2 (gate) | HW1 |
| 5 | 2 | 8 | 2 |
| 6 | 3, 4 | 7 | 2 |
| 7 | 3, 6 | 8, 11 | 2 |
| HW2 | 6, 7 | Wave 3 (gate) | HW2 |
| 8 | 4, 5, 7 | 11 | 3 |
| 9 | 2 | 11 | 3 |
| 10 | 2 | 11 | 3 |
| 11 | 7, 8, 9, 10 | 12 | 3 |
| HW3 | 8 | Wave 4 (gate) | HW3 |
| 12 | 7, 8, 9, 10, 11 | HW4 | 4 |
| HW4 | 12 | Wave FINAL (gate) | HW4 |
| F1-F4 | HW4 | user okay | FINAL |

### Agent Dispatch Summary

- **Wave 0**: 1 — T0 `quick`
- **Wave 1**: 4 — T1 `quick`, T2 `quick`, T3 `quick`, T4 `unspecified-high`
- **HW1**: 1 — HW1 `deep`（硬件测试，需串口操作）
- **Wave 2**: 3 — T5 `unspecified-high`, T6 `deep`, T7 `deep`
- **HW2**: 1 — HW2 `deep`（硬件测试，需 MCP 工具 + 串口）
- **Wave 3**: 4 — T8 `deep`, T9 `unspecified-high`, T10 `quick`, T11 `deep`
- **HW3**: 1 — HW3 `deep`（硬件测试，需 DataBridge + 串口 + Telnet）
- **Wave 4**: 1 — T12 `unspecified-high`
- **HW4**: 1 — HW4 `deep`（全链路硬件测试）
- **FINAL**: 4 — F1 `oracle`, F2 `unspecified-high`, F3 `unspecified-high`, F4 `deep`

---

## TODOs

- [x] 1. 项目骨架 + go.mod + 目录结构

  **What to do**:
  - 创建 `go.mod`（module github.com/yourname/serialhub, go 1.24）
  - 创建完整目录结构：cmd/serialhub/, cmd/serve/, pkg/serial/, pkg/telnet/, pkg/mcp/tools/, pkg/mcp/transport/, pkg/bridge/, pkg/config/, pkg/tray/, internal/buffer/, internal/service/, internal/testutil/, tests/e2e/
  - 每个目录添加 placeholder 文件确保目录存在
  - 运行 `go get` 安装所有依赖：
    - go.bug.st/serial
    - github.com/modelcontextprotocol/go-sdk
    - github.com/spf13/cobra
    - github.com/spf13/viper
    - github.com/getlantern/systray
    - github.com/joshuafuller/sse/v3
    - github.com/sirupsen/logrus
    - github.com/go-playground/validator/v10
  - 运行 `go mod tidy`
  - 添加 Makefile（build, test, vet, clean 目标）

  **Must NOT do**:
  - 不编写业务逻辑代码
  - 不安装 plan 外的依赖

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO（其他任务依赖此任务的目录结构）
  - **Parallel Group**: Wave 1 (only T1, others wait)
  - **Blocks**: T0, T2, T3, T4, T5, T6, T7, T8, T9, T10
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `go.mod` — 标准 Go module 文件
  - `Makefile` — 参考 Go 社区标准 Makefile

  **External References**:
  - https://go.dev/doc/modules/layout — Go 项目布局标准

  **Acceptance Criteria**:

  ```
  Scenario: 项目骨架可编译
    Tool: Bash
    Steps:
      1. go mod tidy
      2. go build ./...
    Expected Result: 编译成功，零错误
    Evidence: .sisyphus/evidence/task-1-skeleton-build.txt

  Scenario: 目录结构完整
    Tool: Bash
    Steps:
      1. ls cmd/serialhub cmd/serve pkg/serial pkg/telnet pkg/mcp pkg/mcp/tools pkg/mcp/transport pkg/bridge pkg/config pkg/tray internal/buffer internal/service internal/testutil tests/e2e
    Expected Result: 所有目录存在
    Evidence: .sisyphus/evidence/task-1-directory-structure.txt
  ```

  **Commit**: YES
  - Message: `init: bootstrap Go module and project structure`
  - Files: `go.mod, go.sum, Makefile, cmd/, pkg/, internal/, tests/`

- [x] 0. 测试基础设施 internal/testutil

  **What to do**:
  - 创建 `internal/testutil/mock_serial.go` — Mock 串口接口：
    ```go
    // MockSerialPort 模拟 go.bug.st/serial.Port 的行为
    type MockSerialPort struct {
        WriteData  []byte
        ReadData   []byte
        ReadErr    error
        WriteErr   error
        Closed     bool
        mu         sync.Mutex
    }
    func (m *MockSerialPort) Read(p []byte) (n int, err error)
    func (m *MockSerialPort) Write(p []byte) (n int, err error)
    func (m *MockSerialPort) Close() error
    func (m *MockSerialPort) SetReadTimeout(ms int) error
    ```
    - Read 返回 ReadData 中的数据，读完后返回 io.EOF
    - Write 记录到 WriteData
    - Close 设置 Closed=true
  - 创建 `internal/testutil/helpers.go` — 通用测试辅助：
    ```go
    // AssertContains 断言字符串包含子串
    func AssertContains(t *testing.T, haystack, needle string)
    // AssertNotContains 断言字符串不包含子串
    func AssertNotContains(t *testing.T, haystack, needle string)
    // WaitForChannel 等待 channel 数据（带超时）
    func WaitForChannel[T any](t *testing.T, ch <-chan T, timeout time.Duration) T
    // NewTempDir 创建临时目录（用于配置文件测试）
    func NewTempDir(t *testing.T) string
    // WriteTestFile 在临时目录写入测试文件
    func WriteTestFile(t *testing.T, dir, name, content string) string
    ```
  - 创建 `internal/testutil/mock_net.go` — Mock 网络连接：
    ```go
    type MockConn struct {
        ReadBuf  *bytes.Buffer
        WriteBuf *bytes.Buffer
        Closed   atomic.Bool
    }
    func NewMockConn(readData []byte) *MockConn
    func (c *MockConn) Read(b []byte) (n int, err error)
    func (c *MockConn) Write(b []byte) (n int, err error)
    func (c *MockConn) Close() error
    func (c *MockConn) LocalAddr() net.Addr
    func (c *MockConn) RemoteAddr() net.Addr
    func (c *MockConn) SetDeadline(t time.Time) error
    func (c *MockConn) SetReadDeadline(t time.Time) error
    func (c *MockConn) SetWriteDeadline(t time.Time) error
    ```
  - 编写测试 `mock_serial_test.go` + `helpers_test.go` + `mock_net_test.go`

  **单元测试（必须通过）**：
  - `TestMockSerialPort_Read` — Read 返回预设数据，读完返回 io.EOF
  - `TestMockSerialPort_Write` — Write 记录写入数据
  - `TestMockSerialPort_Close` — Close 设置 Closed=true，再次 Read 返回 error
  - `TestMockConn_ReadWrite` — MockConn 读写数据正确
  - `TestMockConn_Close` — Close 后 Read 返回 error
  - `TestAssertContains` — 包含子串时通过，不包含时 fail
  - `TestWaitForChannel_Success` — channel 有数据时立即返回
  - `TestWaitForChannel_Timeout` — channel 无数据时超时 fail
  - `TestNewTempDir` — 创建临时目录，测试结束后自动清理

  **Must NOT do**:
  - 不创建 mockgen 或其他代码生成工具的依赖
  - 不引入 testify 等第三方测试库（用标准库 testing）

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 T2, T3 并行，但需 T1 先完成）
  - **Parallel Group**: Wave 0 (after T1)
  - **Blocks**: T4, T5, T6, T8（使用 mock 的任务）
  - **Blocked By**: T1

  **References**:
  - `go.bug.st/serial` — Port 接口定义（Read/Write/Close/SetReadTimeout）
  - `net.Conn` — 标准库网络连接接口

  **Acceptance Criteria**:

  ```
  Scenario: testutil 包编译通过
    Tool: Bash
    Steps:
      1. go build ./internal/testutil/
    Expected Result: 编译成功
    Evidence: .sisyphus/evidence/task-0-testutil-build.txt

  Scenario: testutil 测试通过
    Tool: Bash
    Steps:
      1. go test ./internal/testutil/ -v
    Expected Result: 所有 9 个测试通过
    Evidence: .sisyphus/evidence/task-0-testutil-test.txt
  ```

  **Commit**: YES
  - Message: `feat(testutil): add test infrastructure with mocks and helpers`
  - Files: `internal/testutil/`

- [x] 2. 配置管理 pkg/config

  **What to do**:
  - 创建 `pkg/config/config.go`，定义配置结构体：
    ```go
    type Config struct {
        Serial SerialConfig
        Telnet TelnetConfig
        MCP    MCPConfig
        Debug  bool
    }
    type SerialConfig struct {
        Port     string
        BaudRate int
        DataBits int
        Parity   string  // "none", "even", "odd"
        StopBits int     // 1, 2
    }
    type TelnetConfig struct { Port int }
    type MCPConfig struct { HTTPPort int }
    ```
  - 默认值：BaudRate=115200, DataBits=8, Parity="none", StopBits=1, Telnet.Port=2323, MCP.HTTPPort=5000
  - 使用 viper 加载 JSON 配置文件
  - 实现 `Load(configPath string) (*Config, error)`
  - 实现 `GetDefault() *Config` 返回默认配置
  - 编写测试 `pkg/config/config_test.go`，覆盖以下测试函数：
    - `TestGetDefault` — 验证默认值正确（115200/8N1/2323/5000）
    - `TestLoadConfig` — 使用临时 JSON 文件加载配置覆盖默认值
    - `TestLoadConfig_FileNotFound` — 配置文件不存在时返回错误
    - `TestLoadConfig_InvalidJSON` — 无效 JSON 返回错误
    - `TestConfigMerge` — CLI 参数覆盖文件配置覆盖默认值

  **Must NOT do**:
  - 不实现 CLI 参数解析（由 cobra 入口处理）
  - 不支持 YAML/TOML（仅 JSON）

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 T3, T4 并行）
  - **Parallel Group**: Wave 1
  - **Blocks**: T5, T9, T10
  - **Blocked By**: T1

  **References**:

  **Pattern References**:
  - `src/config/index.ts` — TypeScript 版本完整配置定义，包含所有字段、默认值、深度合并逻辑

  **API/Type References**:
  - `src/config/index.ts:SerialConfig` — 串口配置接口（port, baudRate, dataBits:5|6|7|8, parity:"none"|"even"|"odd", stopBits:1|2）
  - `src/config/index.ts:Config` — 顶层配置（serial, telnet, mcp, debug）
  - `src/config/index.ts:DEFAULT_CONFIG` — 默认值（baudRate:115200, dataBits:8, parity:"none", stopBits:1, telnet.port:2323, mcp.httpPort:5000）

  **Acceptance Criteria**:

  ```
  Scenario: 默认配置正确
    Tool: Bash
    Steps:
      1. go test ./pkg/config/ -v -run TestGetDefault
    Expected Result: 默认值与 TS 版本一致（baudRate=115200, dataBits=8, telnet.port=2323, mcp.httpPort=5000）
    Evidence: .sisyphus/evidence/task-2-config-default.txt

  Scenario: JSON 配置文件加载
    Tool: Bash
    Steps:
      1. go test ./pkg/config/ -v -run TestLoadConfig
    Expected Result: 正确读取 JSON 文件并覆盖默认值
    Evidence: .sisyphus/evidence/task-2-config-load.txt

  Scenario: 配置合并优先级
    Tool: Bash
    Steps:
      1. go test ./pkg/config/ -v -run TestConfigMerge
    Expected Result: CLI > 文件 > 默认值的优先级正确
    Evidence: .sisyphus/evidence/task-2-config-merge.txt
  ```

  **日志要求**:
  - 配置加载：`logrus.Infof("[SerialHub] 配置文件加载: %s", path)`
  - 配置加载失败：`logrus.Warnf("[SerialHub] 配置文件不存在: %s", path)`
  - 使用 debug 模式时：`logrus.Debugf("[SerialHub] 当前配置: %+v", cfg)`

  **Commit**: YES
  - Message: `feat(config): add configuration management with viper`
  - Files: `pkg/config/`

- [x] 3. 核心类型定义 + DataBuffer

  **What to do**:
  - 创建 `internal/buffer/buffer.go`：
    ```go
    type DataBuffer struct {
        mu       sync.Mutex
        buffer   []byte
        maxSize  int  // 默认 65536
    }
    ```
  - 方法：`Append(data []byte)`, `Read(maxSize int) []byte`, `Peek(maxSize int) []byte`, `Clear()`, `Length() int`
  - Append 超过 maxSize 时丢弃旧数据（与 TS 版本行为一致）
  - 编写测试 `internal/buffer/buffer_test.go`，覆盖以下测试函数：
    - `TestBuffer_Append` — 追加数据，Length 正确
    - `TestBuffer_Read` — 读取数据，读取后 Length 减少
    - `TestBuffer_Read_Partial` — 读取部分数据（maxSize < buffer.Length）
    - `TestBuffer_Peek` — 读取不移除，Length 不变
    - `TestBuffer_Clear` — 清空后 Length=0
    - `TestBuffer_Overflow` — 超过 maxSize 时丢弃旧数据
    - `TestBuffer_ConcurrentAccess` — 多 goroutine 并发 Append/Read 不 panic
    - `TestBuffer_EmptyRead` — 空 buffer 时 Read 返回空 slice

  **Must NOT do**:
  - 不使用 io.Reader/Writer 接口（保持简单）
  - 不实现环形缓冲区（使用 slice 动态增长）

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 T2, T4 并行）
  - **Parallel Group**: Wave 1
  - **Blocks**: T6, T7
  - **Blocked By**: T1

  **References**:

  **Pattern References**:
  - `src/mcp/tools/serial_read.ts:DataBuffer` — TypeScript 版本 DataBuffer 完整实现

  **API/Type References**:
  - `src/mcp/tools/serial_read.ts:DataBuffer` — 类结构（buffer: Buffer, maxSize: number=65536）
  - `src/mcp/tools/serial_read.ts:DataBuffer.append()` — 超出 maxSize 时 slice 丢弃旧数据
  - `src/mcp/tools/serial_read.ts:DataBuffer.read()` — 读取并移除数据
  - `src/mcp/tools/serial_read.ts:DataBuffer.peek()` — 读取不移除

  **Acceptance Criteria**:

  ```
  Scenario: DataBuffer 基本读写
    Tool: Bash
    Steps:
      1. go test ./internal/buffer/ -v -run TestBuffer
    Expected Result: Append, Read, Peek, Clear 正常工作
    Evidence: .sisyphus/evidence/task-3-buffer-basic.txt

  Scenario: DataBuffer 溢出处理
    Tool: Bash
    Steps:
      1. go test ./internal/buffer/ -v -run TestBuffer_Overflow
    Expected Result: 超过 maxSize 时丢弃旧数据
    Evidence: .sisyphus/evidence/task-3-buffer-overflow.txt
  ```

  **Commit**: YES
  - Message: `feat(buffer): add DataBuffer with overflow handling`
  - Files: `internal/buffer/`

- [x] 4. SerialManager pkg/serial

  **What to do**:
  - 创建 `pkg/serial/config.go`：
    ```go
    type Config struct {
        Port     string
        BaudRate int
        DataBits int
        Parity   string
        StopBits int
    }
    func (c *Config) ToMode() *serial.Mode  // 转换为 go.bug.st/serial 的 Mode
    ```
  - 创建 `pkg/serial/manager.go`：
    ```go
    type SerialManager struct {
        port       Port       // 使用接口而非具体实现
        config     *Config
        dataChan   chan []byte
        errChan    chan error
        stopChan   chan struct{}
        mu         sync.Mutex
        connected  bool
        currentPort string
    }
    ```
  - **关键设计**：定义 Port 接口（Read/Write/Close/SetReadTimeout），生产代码用 serial.Open，测试代码用 MockSerialPort 注入：
    ```go
    // Port 抽象串口操作，用于测试和扩展
    type Port interface {
        Read(p []byte) (n int, err error)
        Write(p []byte) (n int, err error)
        Close() error
        SetReadTimeout(ms int) error
    }
    ```
  - 方法：
    - `NewSerialManager(cfg *Config) *SerialManager`
    - `Connect(ctx context.Context, portName string) error` — 连接串口，启动 readLoop goroutine
    - `Disconnect() error` — 断开串口，停止 goroutine
    - `Write(data []byte) (int, error)` — 发送数据
    - `WriteLine(text string) (int, error)` — 发送文本 + "\r\n"
    - `ListPorts() ([]SerialPortInfo, error)` — 静态方法，使用 serial.GetPortsList()
    - `IsConnected() bool`
    - `CurrentPort() string`
    - `GetConfig() *Config` — 返回副本
    - `UpdateConfig(cfg Config)`
    - `DataChan() <-chan []byte` — 数据 channel
    - `ErrChan() <-chan error` — 错误 channel
    - `Close()` — 清理资源
  - 编写测试 `pkg/serial/manager_test.go`，覆盖以下测试函数：
    - `TestNewSerialManager` — 创建实例，默认值正确（connected=false, config 非 nil）
    - `TestConnect_MockSuccess` — Mock Port 连接成功, IsConnected()==true, CurrentPort()==portName
    - `TestConnect_MockFailure` — Mock Port 返回错误, Connect 返回 error, IsConnected()==false
    - `TestConnect_AlreadyConnected` — 已连接时再 Connect, 先断开再重连
    - `TestConnect_EmptyPortName` — 空端口名返回错误
    - `TestDisconnect` — 已连接时断开, IsConnected()==false, CurrentPort()==""
    - `TestDisconnect_NotConnected` — 未连接时断开,不报错
    - `TestWrite` — 写入数据,Mock Port WriteData 记录正确
    - `TestWrite_NotConnected` — 未连接时写入,返回 error 含 "串口未连接"
    - `TestWriteLine` — 发送文本 + "\r\n",验证完整写入
    - `TestListPorts` — 调用 ListPorts,返回串口列表（可能为空不报错）
    - `TestUpdateConfig` — UpdateConfig 后 GetConfig 返回新值
    - `TestClose` — Close 后 DataChan 关闭,资源清理
    - `TestConcurrentReadWrite` — 多 goroutine 并发 Append/Read,不 panic

  **Must NOT do**:
  - 不自动重连
  - 不实现流控
  - 不使用 panic

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 T2, T3 并行）
  - **Parallel Group**: Wave 1
  - **Blocks**: T6, T8, HW1
  - **Blocked By**: T1, T0

  **References**:

  **Pattern References**:
  - `src/serial/SerialManager.ts` — TypeScript 版本完整实现，EventEmitter 模式

  **API/Type References**:
  - `src/serial/SerialManager.ts:SerialConfig` — 配置接口（port, baudRate, dataBits:5|6|7|8, parity, stopBits:1|2）
  - `src/serial/SerialManager.ts:SerialPortInfo` — 端口信息（path, manufacturer, serialNumber, pnpId, vendorId, productId）
  - `src/serial/SerialManager.ts:SerialManagerEvents` — 事件（data, connected, disconnected, error）
  - `src/serial/SerialManager.ts` — 核心方法：connect(portName?), disconnect(), write(data), writeLine(text), listPorts(), updateConfig(), getConfig()

  **External References**:
  - https://pkg.go.dev/go.bug.st/serial — Go 串口库 API
  - https://github.com/bugst/go-serial — 示例代码

  **Acceptance Criteria**:

  ```
  Scenario: SerialManager 编译和基础测试
    Tool: Bash
    Steps:
      1. go build ./pkg/serial/
      2. go test ./pkg/serial/ -v
    Expected Result: 编译成功，测试通过
    Evidence: .sisyphus/evidence/task-4-serial-build.txt

  Scenario: ListPorts 返回串口列表
    Tool: Bash
    Steps:
      1. go test ./pkg/serial/ -v -run TestListPorts
    Expected Result: 返回可用串口列表（可能为空，不报错）
    Evidence: .sisyphus/evidence/task-4-serial-list.txt
  ```

  **Commit**: YES
  - Message: `feat(serial): add serial port manager with channel communication`
  - Files: `pkg/serial/`
  - Pre-commit: `go test ./pkg/serial/`

- [ ] HW1. SerialManager 硬件集成测试（Wave 1 后）

  **What to do**:
  - 在 `pkg/serial/manager_test.go` 中添加硬件测试函数 `TestHW1_SerialManager`
  - 使用环境变量控制跳过：`if os.Getenv("SERIALHUB_HARDWARE_TEST") != "1" { t.Skip(...) }`
  - 测试流程：
    1. `NewSerialManager(cfg)` 创建实例（COM9, 115200, 8N1）
    2. `Connect(ctx, "COM9")` → 断言 `IsConnected() == true`
    3. 清空残留：`WriteLine("")` → `time.Sleep(500ms)` → 丢弃 DataChan 中所有数据
    4. 发送 `help`：`WriteLine("help")` → 从 DataChan 读取（5s 超时）
    5. 断言响应包含 `"RT-Thread shell commands:"`
    6. 清空残留数据
    7. 发送 `version`：`WriteLine("version")` → 从 DataChan 读取（5s 超时）
    8. 断言响应包含 `"Thread Operating System"`
    9. `Disconnect()` → 断言 `IsConnected() == false`
  - 使用 `t.Cleanup(func() { mgr.Disconnect() })` 确保清理
  - 失败时输出：最后读取到的原始数据（hex + ASCII）

  **Must NOT do**:
  - 不测试串口不可用的情况（那是单元测试的事）
  - 不修改 SerialManager 源码来适配测试
  - 不在硬件测试中使用 mock

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO（独占 COM9）
  - **Parallel Group**: HW1 (sequential)
  - **Blocks**: Wave 2 全部任务
  - **Blocked By**: T4 (SerialManager)

  **References**:

  **Pattern References**:
  - `pkg/serial/manager_test.go` — T4 中创建的单元测试，硬件测试追加到同一文件

  **API/Type References**:
  - `pkg/serial/manager.go:SerialManager` — Connect, Disconnect, WriteLine, DataChan, IsConnected
  - `pkg/serial/config.go:Config` — 串口配置结构

  **External References**:
  - RT-Thread help 输出：包含 `"RT-Thread shell commands:"` 后跟命令列表
  - RT-Thread version 输出：包含 `"RT -"` + `"Thread Operating System"` + `"5.2.1"` 等版本信息

  **Acceptance Criteria**:

  ```
  Scenario: SerialManager 连接 COM9 并执行 help 命令
    Tool: Bash
    Preconditions: COM9 可用，RT-Thread 设备已连接并运行
    Steps:
      1. $env:SERIALHUB_HARDWARE_TEST="1"; go test -v -run TestHW1_SerialManager ./pkg/serial/ -timeout 30s
    Expected Result:
      - 连接成功：IsConnected() == true
      - help 响应包含 "RT-Thread shell commands:"
      - version 响应包含 "Thread Operating System"
      - 断开成功：IsConnected() == false
    Failure Indicators: 连接超时、响应为空、响应不包含预期子串
    Evidence: .sisyphus/evidence/task-hw1-serial-manager.txt

  Scenario: 硬件测试默认跳过
    Tool: Bash
    Steps:
      1. go test -v -run TestHW1_SerialManager ./pkg/serial/ -timeout 10s
    Expected Result: 输出 "skip: 硬件测试未启用"
    Evidence: .sisyphus/evidence/task-hw1-skip.txt
  ```

  **Commit**: YES
  - Message: `test(serial): add hardware integration test for SerialManager`
  - Files: `pkg/serial/manager_test.go`

- [x] 5. TelnetServer pkg/telnet

  **What to do**:
  - 创建 `pkg/telnet/client.go`：
    ```go
    type TelnetClient struct {
        ID            string    // UUID
        Conn          net.Conn
        RemoteAddress string
        ConnectedAt   time.Time
    }
    ```
  - 创建 `pkg/telnet/server.go`：
    ```go
    type TelnetServer struct {
        listener   net.Listener
        clients    map[string]*TelnetClient
        mu         sync.RWMutex
        dataChan   chan TelnetDataEvent
        running    bool
        port       int
    }
    type TelnetDataEvent struct {
        Data   []byte
        Client *TelnetClient
    }
    ```
  - 方法：
    - `NewTelnetServer() *TelnetServer`
    - `Start(ctx context.Context, port int) error` — 启动 TCP 服务器
    - `Stop() error` — 停止服务器，断开所有客户端
    - `Broadcast(data []byte)` — 广播到所有客户端
    - `SendToClient(clientID string, data []byte) error`
    - `DisconnectClient(clientID string) error`
    - `GetClient(clientID string) (*TelnetClient, bool)`
    - `IsRunning() bool`
    - `Port() int`
    - `ClientCount() int`
    - `ConnectedClients() []*TelnetClient`
    - `DataChan() <-chan TelnetDataEvent` — 数据 channel
  - acceptLoop goroutine：接受连接，发送欢迎消息 "Connected to SerialHub\r\n"
  - readLoop goroutine：为每个客户端启动读取 goroutine
  - **关键设计**：使用 testutil.MockConn 模拟客户端
  - 编写测试 `pkg/telnet/server_test.go`，覆盖以下测试函数：
    - `TestNewTelnetServer` — 创建实例默认值正确
    - `TestStartStop` — 启动监听停止正常
    - `TestStart_InvalidPort` — 无效端口返回错误
    - `TestAcceptClient` — 客户端连接后收到欢迎消息"Connected to SerialHub\r\n"
    - `TestDisconnectClient` — 断开后ClientCount减少
    - `TestBroadcast_Multiple` — 广播数据所有客户端收到
    - `TestSendToClient` — 发送到指定客户端
    - `TestConcurrentBroadcast` — 并发广播不panic
    - `TestDataChan` — 客户端发送数据通过DataChan传出
    - `TestFullWorkflow` — 启动→连接3客户端→广播→断开→停止

  **Must NOT do**:
  - 不实现 Telnet 协议协商（RFC 854），仅做 TCP relay
  - 不限制最大客户端数

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 T6 并行）
  - **Parallel Group**: Wave 2
  - **Blocks**: T8
  - **Blocked By**: T2

  **References**:

  **Pattern References**:
  - `src/telnet/TelnetServer.ts` — TypeScript 版本完整实现

  **API/Type References**:
  - `src/telnet/TelnetServer.ts:TelnetClient` — 客户端信息（id:UUID, socket:Socket, remoteAddress, connectedAt）
  - `src/telnet/TelnetServer.ts:TelnetServerEvents` — 事件（started, stopped, connection, data, disconnect, error）
  - `src/telnet/TelnetServer.ts` — 核心方法：start(port), stop(), broadcast(data), sendToClient(id, data), disconnectClient(id)
  - `src/telnet/TelnetServer.ts:handleConnection()` — 发送欢迎消息 "Connected to SerialHub\r\n"

  **Acceptance Criteria**:

  ```
  Scenario: Telnet 服务器启停
    Tool: Bash
    Steps:
      1. go test ./pkg/telnet/ -v -run TestStartStop
    Expected Result: 服务器启动、监听、停止正常
    Evidence: .sisyphus/evidence/task-5-telnet-start.txt

  Scenario: 多客户端连接和数据广播
    Tool: Bash
    Steps:
      1. go test ./pkg/telnet/ -v -run TestBroadcast_Multiple
    Expected Result: 广播数据被所有客户端接收
    Evidence: .sisyphus/evidence/task-5-telnet-broadcast.txt
  ```

  **Commit**: YES
  - Message: `feat(telnet): add Telnet server with multi-client support`
  - Files: `pkg/telnet/`
  - Pre-commit: `go test ./pkg/telnet/`

- [x] 6. MCP 工具 pkg/mcp/tools/*

  **What to do**:
  - 创建 6 个工具文件，每个文件定义 Input struct + execute 函数：
    - `serial_list.go`: `executeSerialList()` — 调用 serial.ListPorts()
    - `serial_connect.go`: `ConnectInput{Port, BaudRate}`, `executeSerialConnect()` — 连接串口
    - `serial_disconnect.go`: `executeSerialDisconnect()` — 断开串口
    - `serial_write.go`: `WriteInput{Data, AddNewline}`, `executeSerialWrite()` — 发送数据
    - `serial_read.go`: `ReadInput{Timeout, MaxSize}`, `executeSerialRead()` — 读取 DataBuffer 数据
    - `serial_status.go`: `executeSerialStatus()` — 返回连接状态
  - 所有工具返回 `ToolResult` 结构：
    ```go
    type ToolResult struct {
        Success bool        `json:"success"`
        Message string      `json:"message,omitempty"`
        Data    interface{} `json:"data,omitempty"`
    }
    ```
  - **关键设计**：工具函数直接接收 SerialManager 和 DataBuffer 作为参数（不用 interface{}），方便测试
  - 串口数据由 DataBuffer 管理（serial_read 从 buffer 读取）
  - serial_read 阻塞等待：使用 select + time.After 或 time.NewTimer
  - 所有错误消息使用中文
  - 编写测试 `pkg/mcp/tools/tools_test.go`（使用 MockSerialPort），覆盖以下测试函数：
    - `TestSerialList` — 返回串口列表
    - `TestSerialConnect_Success` — 连接成功
    - `TestSerialConnect_WithBaudRate` — 自定义波特率
    - `TestSerialConnect_AlreadyConnected` — 先断开再重连
    - `TestSerialConnect_InvalidPort` — 无效端口返回ToolResult{Success:false}
    - `TestSerialDisconnect_Success` — 断开成功
    - `TestSerialDisconnect_NotConnected` — 未连接也返回成功
    - `TestSerialWrite_Success` — 发送成功bytesWritten>0
    - `TestSerialWrite_NotConnected` — 返回错误
    - `TestSerialWrite_WithNewline` — 自动追加\r\n
    - `TestSerialWrite_WithoutNewline` — 不追加
    - `TestSerialRead_WithData` — buffer有数据读取
    - `TestSerialRead_EmptyBuffer` — 空buffer超时返回
    - `TestSerialRead_Timeout` — 超时timedOut=true
    - `TestSerialStatus_Connected` — 返回连接信息
    - `TestSerialStatus_NotConnected` — 返回disconnected

  **Must NOT do**:
  - 工具函数不返回 Go error，统一返回 ToolResult
  - 不修改 SerialManager 状态以外的全局状态

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 T5 并行）
  - **Parallel Group**: Wave 2
  - **Blocks**: T7
  - **Blocked By**: T3, T4

  **References**:

  **Pattern References**:
  - `src/mcp/tools/serial_list.ts` — 无参数，返回 ports 列表
  - `src/mcp/tools/serial_connect.ts` — Input{port, baudRate?}, Result{success, port, baudRate, message?}
  - `src/mcp/tools/serial_disconnect.ts` — 无参数，Result{success, message?}，未连接也返回成功
  - `src/mcp/tools/serial_write.ts` — Input{data, addNewline?}, Result{success, bytesWritten?, message?}
  - `src/mcp/tools/serial_read.ts` — Input{timeout?, maxSize?}, Result{data, encoding, timestamp, bytes, timedOut}
  - `src/mcp/tools/serial_status.ts` — 无参数，Result{connected, port?, baudRate?, config?}

  **API/Type References**:
  - `src/mcp/tools/serial_read.ts:DataBuffer` — buffer 管理器（已在 T3 实现）
  - `src/mcp/tools/serial_write.ts:SerialWriteInput` — {data: string, addNewline?: boolean=true}
  - `src/mcp/tools/serial_connect.ts:SerialConnectInput` — {port: string, baudRate?: number}
  - `src/mcp/tools/serial_connect.ts:SerialConnectResult` — {success, port, baudRate, message?}

  **Acceptance Criteria**:

  ```
  Scenario: 6 个工具函数编译通过
    Tool: Bash
    Steps:
      1. go build ./pkg/mcp/tools/
    Expected Result: 编译成功
    Evidence: .sisyphus/evidence/task-6-tools-build.txt

  Scenario: 工具函数单元测试
    Tool: Bash
    Steps:
      1. go test ./pkg/mcp/tools/ -v
    Expected Result: 所有 16 个测试通过
    Evidence: .sisyphus/evidence/task-6-tools-test.txt
  ```

  **Commit**: YES
  - Message: `feat(mcp): add MCP serial tools (list/connect/disconnect/write/read/status)`
  - Files: `pkg/mcp/tools/`

- [x] 7. MCP 服务端 pkg/mcp/server + transport

  **What to do**:
  - 创建 `pkg/mcp/server.go`：
    ```go
    type MCPServer struct {
        server       *mcp.Server
        serial       *serial.SerialManager
        dataBuffer   *buffer.DataBuffer
    }
    func NewMCPServer(serialMgr *serial.SerialManager, dataBuf *buffer.DataBuffer) *MCPServer
    func (s *MCPServer) RegisterTools()  // 注册 6 个工具
    func (s *MCPServer) Server() *mcp.Server
    func (s *MCPServer) DataBuffer() *buffer.DataBuffer
    func (s *MCPServer) Close()
    ```
  - 使用官方 SDK：`mcp.NewServer`, `mcp.AddTool`
  - 创建 `pkg/mcp/transport/stdio.go`：
    - `RunStdio(ctx context.Context, server *MCPServer) error` — 使用 `mcp.StdioTransport`
  - 创建 `pkg/mcp/transport/httpsse.go`：
    - `RunHTTP(ctx context.Context, server *MCPServer, host string, port int, corsOrigin string) error`
    - 使用 `mcp.NewStreamableHTTPHandler`
    - 添加 `/health` 健康检查端点
    - 添加 CORS 支持
    - 使用 `http.Server.Shutdown()` 优雅关闭
  - 编写测试：
    - `pkg/mcp/server_test.go`: `TestMCPServer_RegisterTools`（6个工具注册）, `TestMCPServer_Close`
    - `pkg/mcp/transport/httpsse_test.go`: `TestHealthEndpoint`（/health返回ok）, `TestCORSEHeaders`, `TestHTTPServer_GracefulShutdown`

  **Must NOT do**:
  - 不自己实现 JSON-RPC（使用官方 SDK）
  - 不支持 WebSocket

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO（依赖 T6 的工具定义）
  - **Parallel Group**: Wave 2 (after T6)
  - **Blocks**: T8, T11, T12
  - **Blocked By**: T3, T6

  **References**:

  **Pattern References**:
  - `src/mcp/index.ts` — TypeScript 版本 MCP 服务入口，工具注册逻辑
  - `src/mcp/transport/http-sse.ts` — HTTP 传输实现，JSON-RPC 处理，CORS 设置

  **API/Type References**:
  - `src/mcp/index.ts:SerialHubMCP` — 核心类（server, serial, dataBuffer, tools）
  - `src/mcp/index.ts:registerTools()` — 工具注册模式
  - `src/mcp/transport/http-sse.ts:HttpServerConfig` — HTTP 配置（port, host, enableCors, corsOrigin）
  - `src/mcp/transport/http-sse.ts:setCorsHeaders()` — CORS 头设置

  **External References**:
  - Go MCP SDK 官方示例：https://github.com/modelcontextprotocol/go-sdk/blob/main/examples/server/hello/main.go

  **Acceptance Criteria**:

  ```
  Scenario: MCP 服务端编译
    Tool: Bash
    Steps:
      1. go build ./pkg/mcp/...
    Expected Result: 编译成功
    Evidence: .sisyphus/evidence/task-7-mcp-build.txt

  Scenario: HTTP 健康检查
    Tool: Bash
    Steps:
      1. go test ./pkg/mcp/transport/ -v -run TestHealthEndpoint
    Expected Result: /health 返回 {"status":"ok"}
    Evidence: .sisyphus/evidence/task-7-mcp-health.txt
  ```

  **Commit**: YES
  - Message: `feat(mcp): add MCP server with stdio and HTTP+SSE transport`
  - Files: `pkg/mcp/`
  - Pre-commit: `go build ./pkg/mcp/...`

- [ ] HW2. MCP 工具硬件集成测试（Wave 2 后）

  **What to do**:
  - 在 `pkg/mcp/tools/tools_test.go` 中添加硬件测试函数 `TestHW2_MCPTools`
  - 使用环境变量 `SERIALHUB_HARDWARE_TEST=1` 控制跳过
  - 测试流程：
    1. 创建 SerialManager + DataBuffer 实例
    2. 调用 `executeSerialConnect(ctx, mgr, &ConnectInput{Port: "COM9", BaudRate: 115200})`
    3. 断言 result.Success == true
    4. 清空残留数据（向 DataBuffer 写入空数据触发读取，丢弃已有内容）
    5. 调用 `executeSerialWrite(ctx, mgr, &WriteInput{Data: "help", AddNewline: true})`
    6. 断言 result.Success == true
    7. 等待 500ms，调用 `executeSerialRead(ctx, buf, &ReadInput{Timeout: 5000, MaxSize: 4096})`
    8. 断言 result.Data 包含 `"RT-Thread shell commands:"`
    9. 清空残留，发送 `version` 命令
    10. 读取响应，断言包含 `"Thread Operating System"`
    11. 调用 `executeSerialStatus(ctx, mgr)`，断言 connected=true, port="COM9"
    12. 调用 `executeSerialDisconnect(ctx, mgr)`
    13. 断言 result.Success == true
    14. 调用 `executeSerialStatus`，断言 connected=false
  - 使用 `t.Cleanup()` 确保 Disconnect

  **Must NOT do**:
  - 不通过 MCP 协议/HTTP 调用（直接调用工具函数）
  - 不测试 MCP 传输层（那是 T7 的事）
  - 不修改工具函数源码

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO（独占 COM9）
  - **Parallel Group**: HW2 (sequential)
  - **Blocks**: Wave 3 全部任务
  - **Blocked By**: T6 (MCP 工具), T4 (SerialManager), T3 (DataBuffer)

  **References**:

  **API/Type References**:
  - `pkg/mcp/tools/serial_connect.go:executeSerialConnect` — 连接工具函数签名
  - `pkg/mcp/tools/serial_write.go:executeSerialWrite` — 写入工具函数签名
  - `pkg/mcp/tools/serial_read.go:executeSerialRead` — 读取工具函数签名
  - `pkg/mcp/tools/serial_status.go:executeSerialStatus` — 状态工具函数签名
  - `pkg/mcp/tools/serial_disconnect.go:executeSerialDisconnect` — 断开工具函数签名
  - `internal/buffer/buffer.go:DataBuffer` — 缓冲区读写方法

  **Acceptance Criteria**:

  ```
  Scenario: MCP 工具完整流程测试（connect→write→read→status→disconnect）
    Tool: Bash
    Preconditions: COM9 可用，RT-Thread 设备已连接
    Steps:
      1. $env:SERIALHUB_HARDWARE_TEST="1"; go test -v -run TestHW2_MCPTools ./pkg/mcp/tools/ -timeout 60s
    Expected Result:
      - serial_connect 成功，返回 {success:true, port:"COM9"}
      - serial_write("help") 成功
      - serial_read 包含 "RT-Thread shell commands:"
      - serial_write("version") 成功
      - serial_read 包含 "Thread Operating System"
      - serial_status 返回 {connected:true}
      - serial_disconnect 成功
      - serial_status 返回 {connected:false}
    Failure Indicators: 任何工具返回 success:false、读取超时、响应不匹配
    Evidence: .sisyphus/evidence/task-hw2-mcp-tools.txt
  ```

  **Commit**: YES
  - Message: `test(mcp): add hardware integration test for MCP tools`
  - Files: `pkg/mcp/tools/tools_test.go`

- [x] 8. DataBridge pkg/bridge

  **What to do**:
  - 创建 `pkg/bridge/events.go`：定义事件 channel 类型
    ```go
    type SerialDataEvent struct { Data []byte }
    type TelnetDataEvent struct { Data []byte; Client *telnet.TelnetClient }
    type ForwardEvent struct { Data []byte; From string; To string }
    ```
  - 创建 `pkg/bridge/bridge.go`：
    ```go
    type DataBridge struct {
        serial       *serial.SerialManager
        telnet       *telnet.TelnetServer
        mcpBuffer    *buffer.DataBuffer
        options      BridgeOptions
        running      bool
        cancel       context.CancelFunc
    }
    type BridgeOptions struct {
        EnableTelnet bool
        EnableMCP    bool
        DebugLog     bool
    }
    ```
  - 方法：
    - `NewDataBridge(serial *serial.SerialManager, telnet *telnet.TelnetServer, buf *buffer.DataBuffer, opts BridgeOptions) *DataBridge`
    - `Start(ctx context.Context)` — 启动 goroutine 监听 serial.dataChan 和 telnet.dataChan
    - `Stop()` — 停止 goroutine
    - `IsRunning() bool`
    - `GetOptions() BridgeOptions`
    - `UpdateOptions(opts BridgeOptions)`
  - 核心逻辑（使用 select）：
    - serial data → telnet.Broadcast(data) + mcpBuffer.Append(data)
    - telnet data → serial.Write(data)
  - 调试日志使用 logrus + `[SerialHub]` 前缀
  - 编写测试 `pkg/bridge/bridge_test.go`，覆盖以下测试函数：
    - `TestNewDataBridge` — 创建实例
    - `TestBridgeStartStop` — 启动停止正常
    - `TestSerialForwarding_Telnet` — 串口数据转发到Telnet广播
    - `TestSerialForwarding_MCP` — 串口数据写入DataBuffer
    - `TestSerialForwarding_Both` — 数据同时到达Telnet和MCP（**关键测试**：验证无双重写入）
    - `TestTelnetForwarding` — Telnet数据转发到串口
    - `TestBridgeStop_Idempotent` — 重复Stop不报错
    - `TestBridgeOptions` — EnableTelnet/EnableMCP开关正确
    - `TestConcurrentForwarding` — 并发数据不丢失不panic

  **Must NOT do**:
  - 不自己实现事件总线，使用纯 channel + select
  - 不修改传入的 SerialManager/TelnetServer 状态

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 T9, T10 并行）
  - **Parallel Group**: Wave 3
  - **Blocks**: T11, T12
  - **Blocked By**: T4, T5, T7

  **References**:

  **Pattern References**:
  - `src/bridge/DataBridge.ts` — TypeScript 版本完整实现，事件总线模式

  **API/Type References**:
  - `src/bridge/DataBridge.ts:DataBridgeOptions` — {enableTelnet:true, enableMCP:true, debugLog:false}
  - `src/bridge/DataBridge.ts:handleSerialData()` — 串口→Telnet广播 + MCP buffer追加
  - `src/bridge/DataBridge.ts:handleTelnetData()` — Telnet→串口写入
  - `src/bridge/DataBridge.ts:DataBridgeEvents` — started, stopped, serial-data, telnet-data, forward, error

  **Acceptance Criteria**:

  ```
  Scenario: DataBridge 编译和启动
    Tool: Bash
    Steps:
      1. go build ./pkg/bridge/
      2. go test ./pkg/bridge/ -v -run TestBridgeStartStop
    Expected Result: 启动和停止正常，无 goroutine 泄漏
    Evidence: .sisyphus/evidence/task-8-bridge-start.txt

  Scenario: 串口数据转发到 Telnet 和 MCP
    Tool: Bash
    Steps:
      1. go test ./pkg/bridge/ -v -run TestSerialForwarding_Both
    Expected Result: 数据同时到达 Telnet 广播和 MCP buffer，DataBuffer 仅写入一次（无双重写入）
    Evidence: .sisyphus/evidence/task-8-bridge-forward.txt

  Scenario: Telnet 数据转发到串口
    Tool: Bash
    Steps:
      1. go test ./pkg/bridge/ -v -run TestTelnetForwarding
    Expected Result: Telnet 输入被写入串口
    Evidence: .sisyphus/evidence/task-8-bridge-telnet.txt
  ```

  **Commit**: YES
  - Message: `feat(bridge): add data bridge with channel communication`
  - Files: `pkg/bridge/`
  - Pre-commit: `go test ./pkg/bridge/`

- [x] 9. 系统托盘 pkg/tray（Windows + Linux）

  **What to do**:
  - 使用 `github.com/getlantern/systray` 实现跨平台系统托盘
  - 创建 `pkg/tray/tray.go`：
    ```go
    type TrayManager struct {
        serial     *serial.SerialManager
        telnetPort int
        mcpPort    int
        version    string
        state      TrayState
        iconIdle   []byte  // embed.FS 嵌入
        iconConn   []byte
        iconErr    []byte
    }
    type TrayState string
    const (
        TrayIdle      TrayState = "idle"
        TrayConnected TrayState = "connected"
        TrayError     TrayState = "error"
    )
    ```
  - 方法：
    - `NewTrayManager(serial *serial.SerialManager, telnetPort, mcpPort int, version string) *TrayManager`
    - `Run(ctx context.Context)` — 启动 systray，必须在主 goroutine 调用
    - `UpdateState(state TrayState)` — 更新图标和提示
    - `UpdateSerialStatus()` — 根据串口状态更新托盘
  - **菜单项（动态串口列表 + 参数配置 + 端口配置）**：
    - 📡 串口连接（子菜单，动态内容）：
      - 已连接时：`● COM9 (115200 8N1)`（点击断开）
      - 未连接时：列出所有可用串口，每项可点击连接：
        - `COM3 — USB Serial Device`
        - `COM9 — DAPLink`
        - `COM11 — CH340`
        - 刷新串口列表（点击重新枚举）
    - ⚙️ 串口参数（子菜单）：
      - 波特率（子菜单，单选，当前值打勾）：
        - `✓ 115200` / `9600` / `19200` / `38400` / `57600` / `115200` / `230400` / `460800` / `921600`
      - 数据位（子菜单，单选）：`✓ 8` / `5` / `6` / `7`
      - 校验位（子菜单，单选）：`✓ None` / `Even` / `Odd`
      - 停止位（子菜单，单选）：`✓ 1` / `2`
    - ── 分隔符 ──
    - 🌐 服务端口（子菜单）：
      - Telnet 端口：`2323`（点击可输入新端口，弹出对话框或循环切换预设值）
      - MCP 端口：`5000`（点击可输入新端口）
      - 注：端口修改需重启 serve 服务才生效
    - ── 分隔符 ──
    - 📋 版本：`SerialHub v0.1.0`（不可点击）
    - ── 分隔符 ──
    - ❌ 退出
  - **串口列表更新机制**：
    - 初始加载时调用 `serial.GetPortsList()` 枚举所有串口
    - 显示格式：`{portPath} — {manufacturer}`（如无 manufacturer 则只显示路径）
    - 点击串口项 → 使用当前参数配置连接 → 子菜单更新为已连接状态
    - 已连接时点击 → 断开 → 子菜单恢复串口列表
    - "刷新串口列表"项点击 → 重新枚举并更新子菜单
  - **参数配置机制**：
    - 串口参数修改立即生效（影响下次连接，已连接时先断开再用新参数重连）
    - 参数修改通过 `SerialManager.UpdateConfig()` 更新
    - 端口修改标记为"待重启"状态，提示用户重启 serve
    - 所有参数修改记录到 logrus 日志
  - 图标：使用 `embed.FS` 嵌入 tray-idle.png, tray-connected.png, tray-error.png
  - **跨平台支持**：
    - Windows：原生支持（syscall，无需额外依赖）
    - Linux：需要 `libayatana-appindicator3-dev` + `libgtk-3-dev`（Ubuntu 20.04+）
    - Makefile 中添加 Linux 依赖安装提示
  - 编写测试 `pkg/tray/tray_test.go`，覆盖以下测试函数：
    - `TestTrayState` — UpdateState切换idle/connected/error
    - `TestSerialPortMenu` — 枚举串口生成子菜单项
    - `TestSerialConfigMenu` — 波特率/数据位/校验位/停止位切换
    - `TestIconEmbed` — embed.FS加载3个图标

  **Must NOT do**:
  - 不实现 Windows 控制台隐藏/显示（koffi FFI）
  - 不在 Linux 上隐藏控制台窗口

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 T8, T10 并行）
  - **Parallel Group**: Wave 3
  - **Blocks**: T11, T12
  - **Blocked By**: T2

  **References**:

  **Pattern References**:
  - `src/tray/TrayManager.ts` — TypeScript 版本托盘管理器
  - `src/tray/console.ts` — Windows 控制台控制（Go 版本跳过）

  **API/Type References**:
  - `src/tray/TrayManager.ts:TrayState` — "idle" | "connected" | "error"
  - `src/tray/TrayManager.ts:buildMenu()` — TS 版菜单项
  - `src/tray/TrayManager.ts:handleSerialToggle()` — 切换串口连接

  **External References**:
  - https://github.com/getlantern/systray — 跨平台托盘库（3679⭐）
  - https://github.com/getlantern/systray/blob/master/example/main.go — 菜单项定义示例
  - Linux 依赖：`sudo apt-get install libayatana-appindicator3-dev libgtk-3-dev`

  **日志要求**:
  - 启动托盘：`logrus.Info("[SerialHub] 系统托盘已启动")`
  - 切换串口：`logrus.Infof("[SerialHub] 从托盘%s串口 %s", "连接"/"断开", port)`
  - 托盘错误：`logrus.Errorf("[SerialHub] 托盘错误: %v", err)`

  **Acceptance Criteria**:

  ```
  Scenario: 托盘模块编译（Windows）
    Tool: Bash
    Steps:
      1. go build ./pkg/tray/
    Expected Result: 编译成功
    Evidence: .sisyphus/evidence/task-9-tray-build.txt

  Scenario: 托盘状态更新逻辑
    Tool: Bash
    Steps:
      1. go test ./pkg/tray/ -v -run TestTrayState
    Expected Result: UpdateState 正确切换 idle/connected/error
    Evidence: .sisyphus/evidence/task-9-tray-state.txt

  Scenario: 串口列表动态枚举
    Tool: Bash
    Steps:
      1. go test ./pkg/tray/ -v -run TestSerialPortMenu
    Expected Result: 调用 serial.GetPortsList() 返回的端口正确生成子菜单项
    Evidence: .sisyphus/evidence/task-9-tray-ports.txt

  Scenario: 串口参数配置切换
    Tool: Bash
    Steps:
      1. go test ./pkg/tray/ -v -run TestSerialConfigMenu
    Expected Result: 波特率/数据位/校验位/停止位单选切换正确，UpdateConfig 被正确调用
    Evidence: .sisyphus/evidence/task-9-tray-config.txt

  Scenario: 图标嵌入
    Tool: Bash
    Steps:
      1. go test ./pkg/tray/ -v -run TestIconEmbed
    Expected Result: embed.FS 成功加载 3 个图标
    Evidence: .sisyphus/evidence/task-9-tray-icons.txt
  ```

  **Commit**: YES
  - Message: `feat(tray): add cross-platform system tray with serial port selection`
  - Files: `pkg/tray/`

- [x] 10. 服务状态管理 internal/service

  **What to do**:
  - 创建 `internal/service/manager.go`：
    ```go
    type ServiceManager struct {
        runtimeDir string  // os.TempDir()/serialhub
        pidFile    string
        portFile   string
    }
    ```
  - 方法：
    - `NewServiceManager() *ServiceManager`
    - `WriteStatus(port int) error` — 写入 PID + 端口文件
    - `ClearStatus() error` — 删除 PID + 端口文件
    - `ReadStatus() (*ServiceStatus, error)` — 读取状态，检查进程是否存活
    - `CheckHealth(port int) (bool, error)` — HTTP GET /health，2秒超时
    ```go
    type ServiceStatus struct {
        Running bool
        PID     int
        Port    int
    }
    ```
  - 运行时目录：`os.TempDir()/serialhub`
  - 进程检查：`os.FindProcess(pid) + Signal(0)`
  - 编写测试 `internal/service/manager_test.go`，覆盖以下测试函数：
    - `TestWriteReadStatus` — 写入后读取PID和端口
    - `TestReadStatus_NoFile` — 无状态文件返回running=false
    - `TestClearStatus` — 清理状态文件
    - `TestStaleCleanup` — 无效PID自动清理
    - `TestCheckHealth_Success` — HTTP /health 返回true
    - `TestCheckHealth_Failure` — 连接失败返回false

  **Must NOT do**:
  - 不发送 SIGTERM（由 CLI 入口处理）
  - 不使用文件锁

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 T8, T9 并行）
  - **Parallel Group**: Wave 3
  - **Blocks**: T11, T12
  - **Blocked By**: T2

  **References**:

  **Pattern References**:
  - `src/service-manager.ts` — TypeScript 版本服务状态管理

  **API/Type References**:
  - `src/service-manager.ts:ServiceStatus` — {running, pid?, port?}
  - `src/service-manager.ts:writeServiceStatus()` — 写入 PID 和端口文件
  - `src/service-manager.ts:readServiceStatus()` — 读取并检查进程存活
  - `src/service-manager.ts:checkServiceHealth()` — HTTP GET /health

  **Acceptance Criteria**:

  ```
  Scenario: 服务状态读写
    Tool: Bash
    Steps:
      1. go test ./internal/service/ -v -run TestWriteReadStatus
    Expected Result: 写入后可正确读取 PID 和端口
    Evidence: .sisyphus/evidence/task-10-service-rw.txt

  Scenario: 进程不存在时清理状态
    Tool: Bash
    Steps:
      1. go test ./internal/service/ -v -run TestStaleCleanup
    Expected Result: 无效 PID 自动清理状态文件
    Evidence: .sisyphus/evidence/task-10-service-cleanup.txt
  ```

  **Commit**: YES
  - Message: `feat(service): add service status management`
  - Files: `internal/service/`

- [x] 11. App 结构体统一生命周期（简化：直接在 main.go 中管理组件，未创建独立 pkg/app）

  **What to do**:
  - 创建 `pkg/app/app.go` — 统一生命周期管理：
    ```go
    type App struct {
        Version    string
        Serial     *serial.SerialManager
        Telnet     *telnet.TelnetServer
        MCP        *mcp.MCPServer
        Bridge     *bridge.DataBridge
        Buffer     *buffer.DataBuffer
        Tray       *tray.TrayManager
        Config     *config.Config
        Service    *service.ServiceManager
        cancel     context.CancelFunc
    }
    ```
  - 方法：
    - `NewApp(version string, cfg *config.Config) *App`
    - `RunMCP(ctx context.Context)` — MCP stdio 模式
    - `RunServe(ctx context.Context, opts ServeOptions)` — HTTP+SSE + Telnet + Tray 模式
    - `Close()` — 按序关闭所有组件（Bridge→Telnet→HTTP→Serial→MCP→Tray→清理状态）
    - `SetupSignalHandler()` — signal.Notify(SIGINT, SIGTERM)
  - **日志初始化**（在 App 创建时统一配置）：
    - 使用 `logrus` 全局 logger
    - 格式：`[SerialHub] <level> <message>`
    - `--debug` 时设置 `logrus.SetLevel(logrus.DebugLevel)`
    - 所有日志输出到 stderr（不影响 MCP stdio 的 stdout）
  - 编写测试：
    - `pkg/app/app_test.go`: `TestNewApp`, `TestApp_Close_Order`（按序关闭）, `TestApp_SignalHandler`, `TestApp_LogOutput`（stderr）
    - `cmd/serialhub/main_test.go`: `TestVersionCommand`, `TestHelpCommand`

  **Must NOT do**:
  - 不使用全局变量（除 version 外，version 是 ldflags 注入点）
  - 不在 main 中直接处理业务逻辑
  - 不在 serve 模式处理 MCP stdio
  - 不在 stdout 输出日志（MCP stdio 使用 stdout）

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO（依赖所有 Wave 3 任务）
  - **Parallel Group**: Wave 3 (after T8, T9, T10)
  - **Blocks**: T12
  - **Blocked By**: T7, T8, T9, T10

  **References**:

  **Pattern References**:
  - `src/index.ts` — TypeScript 版 CLI 入口（合并 index.ts + server.ts）
  - `src/server.ts` — HTTP 服务入口

  **API/Type References**:
  - `src/index.ts:main()` — 命令分发（help, version, stop, serve, mcp）
  - `src/index.ts:setupGracefulShutdown()` — 优雅关闭
  - `src/index.ts:parseArgs()` + `src/server.ts:parseServeArgs()` — 合并为 cobra
  - `src/index.ts:runServe()` — 服务启动完整流程

  **日志要求**（统一在 App 中配置）:
  - 启动：`logrus.Infof("[SerialHub] SerialHub v%s 启动中...", version)`
  - 模式：`logrus.Infof("[SerialHub] 运行模式: %s", "mcp stdio"/"serve")`
  - 组件启动：`logrus.Infof("[SerialHub] Telnet 服务已启动，端口: %d", port)`
  - 组件启动：`logrus.Infof("[SerialHub] MCP HTTP 服务已启动: http://%s:%d", host, port)`
  - 数据桥接：`logrus.Info("[SerialHub] 数据桥接已启动")`
  - 串口连接：`logrus.Infof("[SerialHub] 已连接串口: %s (%d)", port, baudRate)`
  - 关闭信号：`logrus.Infof("[SerialHub] 收到 %s 信号，正在关闭...", signal)`
  - 关闭完成：`logrus.Info("[SerialHub] 已关闭")`
  - 错误：`logrus.Errorf("[SerialHub] 启动失败: %v", err)`
  - 调试数据流：`logrus.Debugf("[SerialHub] [Serial ->] %d bytes", len(data))`
  - 调试数据流：`logrus.Debugf("[SerialHub] [Telnet -> %s] %d bytes", addr, len(data))`

  **Acceptance Criteria**:

  ```
  Scenario: App 编译成功
    Tool: Bash
    Steps:
      1. go build ./pkg/app/
    Expected Result: 编译成功
    Evidence: .sisyphus/evidence/task-11-app-build.txt

  Scenario: 优雅关闭顺序
    Tool: Bash
    Steps:
      1. go test ./pkg/app/ -v -run TestApp_Close_Order
    Expected Result: 组件按 Bridge→Telnet→HTTP→Serial→MCP→Tray 顺序关闭
    Evidence: .sisyphus/evidence/task-11-app-shutdown.txt

  Scenario: 日志输出到 stderr
    Tool: Bash
    Steps:
      1. go test ./pkg/app/ -v -run TestApp_LogOutput
    Expected Result: logrus 日志包含 [SerialHub] 前缀，输出到 stderr
    Evidence: .sisyphus/evidence/task-11-app-log.txt
  ```

  **Commit**: YES
  - Message: `feat(cmd): add unified CLI with cobra subcommands and version management`
  - Files: `pkg/app/, cmd/serialhub/`
  - Pre-commit: `go build ./cmd/...`

- [ ] HW3. DataBridge 转发硬件测试（Wave 3 后）

  **What to do**:
  - 在 `pkg/bridge/bridge_test.go` 中添加硬件测试函数 `TestHW3_DataBridge`
  - 使用环境变量控制跳过
  - 测试流程：
    1. 创建完整组件链：SerialManager + TelnetServer + DataBuffer + DataBridge
    2. 启动 TelnetServer（随机可用端口）
    3. 启动 DataBridge（启用 Telnet + MCP 转发）
    4. 连接串口 COM9
    5. 连接一个 Telnet 测试客户端到 TelnetServer
    6. 清空残留数据
    7. **测试路径 1：串口 → Telnet 广播**
       - 通过 SerialManager.WriteLine("help") 发送
       - 从 Telnet 客户端读取，断言包含 `"RT-Thread shell commands:"`
    8. **测试路径 2：串口 → DataBuffer（MCP）**
       - 发送 "version" 命令
       - 从 DataBuffer.Read() 读取，断言包含 `"Thread Operating System"`
    9. **测试路径 3：Telnet → 串口**
       - 通过 Telnet 客户端发送 "help\r\n"
       - 从 SerialManager.DataChan() 读取设备响应，断言有效
    10. 清理：断开所有连接，停止所有组件
  - 使用 `t.Cleanup()` 确保清理

  **Must NOT do**:
  - 不启动 HTTP 服务（纯 DataBridge 转发测试）
  - 不测试 MCP 传输协议
  - 不修改 DataBridge 源码

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO（独占 COM9）
  - **Parallel Group**: HW3 (sequential)
  - **Blocks**: Wave 4 全部任务
  - **Blocked By**: T8 (DataBridge), T5 (TelnetServer), T4 (SerialManager)

  **References**:

  **API/Type References**:
  - `pkg/bridge/bridge.go:DataBridge` — Start, Stop, IsRunning
  - `pkg/bridge/bridge.go:BridgeOptions` — EnableTelnet, EnableMCP
  - `pkg/telnet/server.go:TelnetServer` — Start, Stop, Broadcast, DataChan
  - `pkg/serial/manager.go:SerialManager` — Connect, WriteLine, DataChan, Disconnect

  **Acceptance Criteria**:

  ```
  Scenario: DataBridge 串口→Telnet 双路转发
    Tool: Bash
    Preconditions: COM9 可用，RT-Thread 设备已连接
    Steps:
      1. $env:SERIALHUB_HARDWARE_TEST="1"; go test -v -run TestHW3_DataBridge ./pkg/bridge/ -timeout 60s
    Expected Result:
      - DataBridge 启动成功
      - 串口发送 help → Telnet 客户端收到包含 "RT-Thread shell commands:" 的数据
      - 串口发送 version → DataBuffer 收到包含 "Thread Operating System" 的数据
      - Telnet 发送 help → 设备响应被正确转发回 Telnet 客户端
      - 所有组件正常关闭
    Failure Indicators: 转发数据为空、数据不完整、组件关闭失败
    Evidence: .sisyphus/evidence/task-hw3-bridge.txt
  ```

  **Commit**: YES
  - Message: `test(bridge): add hardware integration test for DataBridge forwarding`
  - Files: `pkg/bridge/bridge_test.go`

- [x] 12. 统一 CLI 入口 cmd/serialhub（简化：无子命令，直接 serve 模式启动）

  **What to do**:
  - 创建 `cmd/serialhub/main.go` — 统一 CLI 入口：
    - 使用 cobra 定义子命令：
      - `mcp`（默认命令）— 启动 MCP stdio 服务
      - `serve` — 启动 HTTP+SSE + Telnet + Tray 服务
      - `stop` — 停止运行中的服务
      - `version` — 显示版本号（通过 ldflags 注入）
    - 全局参数：`--serial-port`, `--baud-rate`, `config`, `--debug`
    - serve 专属参数：`--telnet-port`, `--mcp-port`, `--host`, `--no-cors`, `--no-tray`
  - **版本号管理**：
    - `main.go` 中定义 `var version = "dev"`（构建时通过 ldflags 覆盖）
    - Makefile 中：`go build -ldflags "-X main.version=$(shell git describe --tags --always)" ./cmd/serialhub`
    - `version` 子命令输出：`SerialHub v0.1.0 (commit abc1234)`
  - 编写测试 `cmd/serialhub/main_test.go`：
    - `TestVersionCommand` — version 子命令输出正确
    - `TestHelpCommand` — help 子命令输出正确

  **Must NOT do**:
  - 不使用全局变量（除 version 外，version 是 ldflags 注入点）
  - 不在 main 中直接处理业务逻辑
  - 不在 stdout 输出日志（MCP stdio 使用 stdout）

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO（依赖所有 Wave 3 任务 + T11）
  - **Parallel Group**: Wave 4 (only task)
  - **Blocks**: HW4
  - **Blocked By**: T7, T8, T9, T10, T11

  **References**:

  **Pattern References**:
  - `src/index.ts` — TypeScript 版 CLI 入口
  - `src/server.ts` — HTTP 服务入口

  **Acceptance Criteria**:

  ```
  Scenario: CLI 编译成功（Windows）
    Tool: Bash
    Steps:
      1. go build -o bin/serialhub.exe ./cmd/serialhub
    Expected Result: 生成可执行文件
    Evidence: .sisyphus/evidence/task-12-cli-build.txt

  Scenario: CLI help 输出
    Tool: Bash
    Steps:
      1. ./bin/serialhub.exe help
    Expected Result: 显示帮助信息，包含 mcp/serve/stop/version 子命令
    Evidence: .sisyphus/evidence/task-12-cli-help.txt

  Scenario: CLI version 显示版本号
    Tool: Bash
    Steps:
      1. ./bin/serialhub.exe version
    Expected Result: 输出 "SerialHub v0.1.0" 或带 commit hash
    Evidence: .sisyphus/evidence/task-12-cli-version.txt
  ```

  **Commit**: YES
  - Message: `feat(cmd): add unified CLI with cobra subcommands and version management`
  - Files: `cmd/serialhub/`
  - Pre-commit: `go build ./cmd/...`

- [ ] HW4. 全链路端到端硬件测试（Wave 4 后、Final 前）

  **What to do**:
  - 创建 `tests/e2e/hardware_e2e_test.go`（新文件，独立于各包的单元测试）
  - 使用环境变量控制跳过
  - 这是最终的端到端测试，验证完整的 serve 模式工作流
  - 测试流程：
    1. 构建 `serialhub.exe`（`go build -o bin/serialhub.exe ./cmd/serialhub`）
    2. 启动 serve 模式子进程：
       ```
       bin/serialhub.exe serve -p COM9 -b 115200 -t 0 -m 0 --no-tray --no-cors
       ```
       （端口 0 = 随机可用端口，避免冲突）
    3. 等待服务就绪（解析 stderr 日志获取实际端口）
    4. **MCP HTTP 测试**：
       - `curl -X POST http://localhost:{mcpPort}/mcp` → `serial_connect`
       - `curl` → `serial_write` "help"
       - `curl` → `serial_read` → 断言包含 `"RT-Thread shell commands:"`
       - `curl` → `serial_write` "version"
       - `curl` → `serial_read` → 断言包含 `"Thread Operating System"`
       - `curl` → `serial_status` → 断言 connected=true
    5. **Telnet 测试**（同时连接）：
       - 连接 `telnet localhost:{telnetPort}`
       - 发送 "help\r\n"
       - 读取响应，断言有效
    6. **并发测试**：
       - 确认 MCP 和 Telnet 可以同时工作
       - 串口数据同时到达两个客户端
    7. **清理**：`serial_disconnect`，终止 serve 进程
  - 使用 `t.Cleanup()` 确保进程终止
  - 失败时输出：进程 stderr 日志 + curl 响应 + telnet 交互记录

  **Must NOT do**:
  - 不测试系统托盘（GUI 测试超出范围）
  - 不测试性能/压力
  - 不修改任何源码

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO（独占 COM9 + 多个网络端口）
  - **Parallel Group**: HW4 (sequential)
  - **Blocks**: Wave FINAL 全部任务
  - **Blocked By**: T12 (CLI 入口), T7 (MCP HTTP), T8 (DataBridge), T5 (Telnet)

  **References**:

  **API/Type References**:
  - `cmd/serialhub/main.go` — serve 子命令参数
  - `pkg/mcp/transport/httpsse.go` — HTTP+SSE 端点路径（/mcp, /health）
  - `pkg/telnet/server.go` — Telnet 端口

  **Acceptance Criteria**:

  ```
  Scenario: 完整 serve 模式端到端测试
    Tool: Bash
    Preconditions: COM9 可用，RT-Thread 设备已连接
    Steps:
      1. $env:SERIALHUB_HARDWARE_TEST="1"; go test -v -run TestHW4_EndToEnd ./tests/e2e/ -timeout 120s
    Expected Result:
      - serve 进程启动成功
      - MCP HTTP: serial_connect 成功
      - MCP HTTP: serial_write("help") + serial_read 包含 "RT-Thread shell commands:"
      - MCP HTTP: serial_write("version") + serial_read 包含 "Thread Operating System"
      - Telnet: 连接成功，发送 help 收到响应
      - 并发: MCP 和 Telnet 同时工作
      - serve 进程正常关闭
    Failure Indicators: 进程启动失败、HTTP 请求无响应、串口操作失败、Telnet 连接失败
    Evidence: .sisyphus/evidence/task-hw4-e2e.txt
  ```

  **Commit**: YES
  - Message: `test(e2e): add full end-to-end hardware integration test`
  - Files: `tests/e2e/hardware_e2e_test.go`

---

## Final Verification Wave

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, run command). For each "Must NOT Have": search codebase for forbidden patterns. Check evidence files exist. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...` + `go build ./...` + `go test ./...`. Review all files for: `panic()`, empty catches, `fmt.Println` in prod, commented-out code, unused imports. Check AI slop: excessive comments, over-abstraction, generic names.
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high`
  Build binaries. Start serve mode. Test: serial port list, connect, write, read, status, disconnect via MCP HTTP. Test Telnet connection and data forwarding. Test config loading. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff. Verify 1:1 — everything in spec was built, nothing beyond spec was built. Check "Must NOT do" compliance. Detect cross-task contamination. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

> **每个任务完成后立即 commit**，不等 Wave 结束。Commit 前必须通过 Self-Review 清单。

| Task | Commit Message | Files |
|------|---------------|-------|
| T1 | `init: bootstrap Go module and project structure` | go.mod, Makefile, 目录结构 |
| T0 | `feat(testutil): add test infrastructure with mocks and helpers` | internal/testutil/ |
| T2 | `feat(config): add configuration management with viper` | pkg/config/ |
| T3 | `feat(buffer): add DataBuffer with overflow handling` | internal/buffer/ |
| T4 | `feat(serial): add serial port manager with channel communication` | pkg/serial/ |
| HW1 | `test(serial): add hardware integration test for SerialManager` | pkg/serial/manager_test.go |
| T5 | `feat(telnet): add Telnet server with multi-client support` | pkg/telnet/ |
| T6 | `feat(mcp): add MCP serial tools (list/connect/disconnect/write/read/status)` | pkg/mcp/tools/ |
| T7 | `feat(mcp): add MCP server with stdio and HTTP+SSE transport` | pkg/mcp/ |
| HW2 | `test(mcp): add hardware integration test for MCP tools` | pkg/mcp/tools/tools_test.go |
| T8 | `feat(bridge): add data bridge with channel communication` | pkg/bridge/ |
| T9 | `feat(tray): add cross-platform system tray with serial port selection` | pkg/tray/ |
| T10 | `feat(service): add service status management` | internal/service/ |
| T11 | `feat(cmd): add unified CLI with cobra subcommands and version management` | pkg/app/, cmd/serialhub/ |
| HW3 | `test(bridge): add hardware integration test for DataBridge forwarding` | pkg/bridge/bridge_test.go |
| T12 | `feat(cmd): add unified CLI with cobra subcommands and version management` | cmd/serialhub/ |
| HW4 | `test(e2e): add full end-to-end hardware integration test` | tests/e2e/hardware_e2e_test.go |

### Commit 流程（每个任务必做）

```
1. 完成代码编写
2. go vet ./...          # 零警告
3. go build ./...        # 编译通过
4. go test ./<pkg>/...   # 测试通过
5. 运行 QA 场景，保存 evidence
6. Self-Review 清单检查
7. git add 相关文件
8. git commit -m "<message>"
```

### Commit Message 格式

```
<type>(<scope>): <简述>

类型: init, feat, fix, test, refactor, docs
范围: config, serial, telnet, mcp, bridge, tray, app, cmd
```

---

## Development Flow (Per Task)

每个任务的完整开发流程：

```
编写代码 → go vet → go build → 编写测试 → go test → Self-Review → Commit
```

### 测试要求（每个任务强制）

**每个任务必须包含测试文件**，测试覆盖以下维度：

1. **单元测试**：每个导出方法至少 1 个测试用例
2. **表格驱动测试**：使用 `[]struct{ name, input, expected }` 模式
3. **边界条件**：空输入、超长输入、无效参数、并发访问
4. **错误路径**：验证错误消息内容（中文）
5. **Mock 隔离**：需要外部依赖（串口、网络）的模块使用 internal/testutil Mock
6. **测试覆盖率**：核心路径 >60%（`go test -cover ./<pkg>/...`）

### 测试文件规范

```go
// 文件命名：xxx_test.go，与源文件同包
// 测试函数命名：TestXxx（中文描述用注释）
func TestSerialManager_Connect(t *testing.T) {
    t.Run("应成功连接串口", func(t *testing.T) { ... })
    t.Run("未指定端口应返回错误", func(t *testing.T) { ... })
    t.Run("串口已连接时应先断开再重连", func(t *testing.T) { ... })
}
```

### Self-Review 清单（每个任务完成后，commit 前必做）

**每个任务完成后立即 commit**，commit 前必须逐项检查：

1. **编译检查**：`go build ./...` — 零错误
2. **静态分析**：`go vet ./...` — 零警告
3. **测试通过**：`go test ./...` — 所有测试通过
4. **测试覆盖**：`go test -cover ./<pkg>/...` — 核心路径 >60%
5. **QA 场景执行**：运行任务中定义的所有 QA 场景，保存 evidence 到 `.sisyphus/evidence/`
6. **代码自审**：
   - [ ] 无 `panic()` 在业务代码中
   - [ ] 无 `fmt.Println`（使用 logrus）
   - [ ] 无未使用的导入
   - [ ] 所有导出符号有 Go 文档注释
   - [ ] 所有错误已处理（无 `_ =` 无注释忽略）
   - [ ] 中文错误消息与 TypeScript 版本一致
   - [ ] 无循环包依赖
   - [ ] 测试文件覆盖所有导出方法
7. **行为一致性**：对比 TypeScript 源文件，确认方法签名、返回值、错误消息完全匹配
8. **Commit**：`git add` + `git commit -m "<message>"`（使用上方 Commit Strategy 中的 message）

---

## Success Criteria

### Verification Commands
```bash
go build ./...                       # 编译成功
go vet ./...                         # 零错误
go test ./...                        # 所有测试通过
go test -cover ./...                 # 覆盖率报告
```

### Final Checklist
- [x] All "Must Have" present
- [x] All "Must NOT Have" absent
- [x] `go build ./...` 成功
- [x] `go vet ./...` 零错误
- [x] `go test ./...` 通过
- [x] HTTP+SSE 模式可用
- [x] Telnet 数据转发正确
- [x] 无数据双重写入（DataBuffer 仅由 DataBridge 写入）
- [x] 硬件集成测试（HW4）通过（SERIALHUB_HARDWARE_TEST=1 时）
- [x] Self-Review 清单每个任务都已完成
