# 删除 Telnet 服务，改用 xterm.js Web 终端

## TL;DR

> **Quick Summary**: 删除 pkg/telnet/ 整个目录，新建 pkg/web/ 实现 WebSocketServer（仅单连接），在 MCP HTTP 服务上注册 /terminal 和 /ws 路由，xterm.js 通过 go:embed 嵌入二进制。
>
> **Deliverables**:
> - pkg/web/ 包：WebSocketServer 实现 TelnetBroadcaster 接口
> - pkg/web/static/terminal.html：xterm.js 终端页面（go:embed 嵌入）
> - MCP HTTP 服务新增 /terminal 和 /ws 路由
> - 删除 pkg/telnet/ 及所有 Telnet 引用
> - CLI 参数、配置、托盘菜单更新
>
> **Estimated Effort**: Medium
> **Parallel Execution**: YES - 3 waves
> **Critical Path**: Task 1 → Task 3 → Task 5 → Task 6 → Final

---

## Context

### Original Request
用户使用 Telnet 连接 RT-Thread 设备时，msh> 提示符出现阶梯状重复显示。根因是 Telnet 协议对 \r\n 换行符处理不当。用户决定删除 Telnet 服务，改用 xterm.js Web 终端方案。

### Interview Summary
**Key Decisions**:
- 端口策略：共用 MCP 端口 5000，路由 /terminal（HTML）和 /ws（WebSocket）
- xterm.js 分发：go:embed 嵌入二进制，离线可用
- 多客户端：仅单连接，新连接踢掉旧连接
- WebSocket 库：gorilla/websocket
- DataBridge 接口不变：WebSocketServer 实现 TelnetBroadcaster（DataChan + Broadcast）

**Research Findings**:
- gorilla/websocket 是 Go 生态最成熟的 WebSocket 库（4k+ stars）
- xterm.js 最新版本通过 npm @xterm/xterm 获取
- DataBridge 依赖 TelnetBroadcaster 接口（DataChan() 和 Broadcast()），新实现必须兼容
- go:embed 从 Go 1.16 起原生支持，项目用 Go 1.26 无兼容性问题

### Metis Review
**Identified Gaps** (addressed):
- 端口策略：已决定共用 MCP 端口 5000
- 多客户端：已决定仅单连接
- 认证：不加认证，维持与 Telnet 一致的零认证策略
- 终端 resize：xterm.js 默认处理，无需额外代码
- 数据帧类型：使用 WebSocket text 帧传输串口数据

---

## Work Objectives

### Core Objective
用 xterm.js Web 终端替代 Telnet 服务，解决换行符显示问题，同时简化架构（从两个服务合并为一个 HTTP 服务）。

### Concrete Deliverables
- `pkg/web/server.go` — WebSocketServer 实现
- `pkg/web/client.go` — WebSocketClient（单连接）
- `pkg/web/static/terminal.html` — xterm.js 终端页面
- `pkg/web/static/xterm.min.js` — xterm.js 核心库
- `pkg/web/static/xterm.css` — xterm.js 样式
- `pkg/web/static/xterm-addon-fit.min.js` — xterm.js fit 插件
- `pkg/web/embed.go` — go:embed 声明
- 修改 `pkg/mcp/server.go` — 新增路由
- 修改 `cmd/serialhub/main.go` — 启动逻辑
- 修改 `pkg/config/config.go` — 配置项
- 修改 `pkg/tray/` — 托盘菜单
- 删除 `pkg/telnet/` — 整个目录

### Definition of Done
- [ ] 浏览器打开 http://localhost:5000/terminal 显示 xterm.js 终端
- [ ] 串口数据实时显示在终端中，无阶梯状问题
- [ ] 键盘输入可发送到串口
- [ ] `go build ./...` 零错误
- [ ] `go vet ./...` 零警告
- [ ] `go test ./...` 原有测试通过（telnet 测试删除）

### Must Have
- WebSocketServer 实现 TelnetBroadcaster 接口（DataChan + Broadcast）
- 仅单连接：新连接时踢掉旧连接
- xterm.js 文件通过 go:embed 嵌入
- /terminal 返回 HTML 页面，/ws 处理 WebSocket 升级
- 删除所有 Telnet 相关代码和引用
- 配置项 telnet.port 改为 web.port 或直接移除（共用 mcp 端口）

### Must NOT Have (Guardrails)
- 不修改 DataBridge 的接口定义或核心逻辑（forwardLoop 不动）
- 不添加认证/登录功能
- 不添加 Web UI 配置界面（串口配置等）
- 不添加终端主题切换、搜索等高级功能
- 不添加 WebSocket 控制协议（仅透传串口数据）
- 不添加 Playwright/浏览器自动化测试
- 不从 CDN 加载 xterm.js（必须 embed）

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed.

### Test Decision
- **Infrastructure exists**: YES (go test)
- **Automated tests**: YES (tests-after)
- **Framework**: go test

### QA Policy
Every task includes agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Backend/Go**: Use Bash (go test, go build, go vet)
- **Web/Terminal**: Use Playwright — Navigate, interact, assert DOM, screenshot
- **API**: Use Bash (curl) — Send requests, assert status + response

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — 无依赖的基础任务):
├── Task 1: 创建 pkg/web/ WebSocketServer [deep]
├── Task 2: 下载 xterm.js 并创建静态文件 [quick]
└── Task 3: 删除 pkg/telnet/ 及清理引用 [quick]

Wave 2 (After Wave 1 — 集成):
├── Task 4: 修改 MCP HTTP 服务注册新路由 [unspecified-high]
├── Task 5: 修改 main.go 启动逻辑 [quick]
├── Task 6: 修改配置和托盘 [quick]

Wave 3 (After Wave 2 — 验证):
├── Task 7: 构建验证和测试修复 [unspecified-high]
└── Task 8: 更新文档 [writing]

Wave FINAL (After ALL tasks):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Real manual QA (unspecified-high + playwright)
└── Task F4: Scope fidelity check (deep)

Critical Path: Task 1 → Task 4 → Task 7 → F1-F4
Parallel Speedup: ~50% faster than sequential
Max Concurrent: 3 (Wave 1)
```

### Dependency Matrix

| Task | Depends On | Blocks | Wave |
|------|-----------|--------|------|
| 1    | —         | 4, 5   | 1    |
| 2    | —         | 4      | 1    |
| 3    | —         | 5, 6   | 1    |
| 4    | 1, 2      | 7      | 2    |
| 5    | 1, 3      | 7      | 2    |
| 6    | 3         | 7      | 2    |
| 7    | 4, 5, 6   | F1-F4  | 3    |
| 8    | 7         | F1-F4  | 3    |

### Agent Dispatch Summary

- **Wave 1**: 3 tasks — T1 `deep`, T2 `quick`, T3 `quick`
- **Wave 2**: 3 tasks — T4 `unspecified-high`, T5 `quick`, T6 `quick`
- **Wave 3**: 2 tasks — T7 `unspecified-high`, T8 `writing`
- **FINAL**: 4 tasks — F1 `oracle`, F2 `unspecified-high`, F3 `unspecified-high`, F4 `deep`

---

## TODOs

- [ ] 1. 创建 pkg/web/ WebSocketServer 实现 TelnetBroadcaster 接口

  **What to do**:
  - 创建 `pkg/web/server.go`：WebSocketServer 结构体，实现 TelnetBroadcaster 接口
    - `DataChan() <-chan []byte`：返回 WebSocket 客户端发送的数据 channel
    - `Broadcast(data []byte) int`：将串口数据发送给当前 WebSocket 客户端
  - 创建 `pkg/web/client.go`：WebSocketClient，管理单个 WebSocket 连接
    - readLoop：从 WebSocket 读取数据，写入 server.dataChan
    - writeLoop：从 writeChan 读取数据，写入 WebSocket
  - 仅支持单连接：新连接时踢掉旧连接（关闭旧连接的 stopChan）
  - 使用 `github.com/gorilla/websocket` 库
  - 添加 `go get github.com/gorilla/websocket` 依赖
  - 创建单元测试 `pkg/web/server_test.go`：使用 httptest + gorilla/websocket 测试

  **Must NOT do**:
  - 不修改 DataBridge 的接口定义
  - 不添加认证功能
  - 不添加控制协议（仅透传串口数据）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 需要理解 TelnetBroadcaster 接口并实现兼容的 WebSocket 版本
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2, 3)
  - **Blocks**: Tasks 4, 5
  - **Blocked By**: None

  **References**:

  **Pattern References** (existing code to follow):
  - `pkg/telnet/server.go` — TelnetServer 结构体和 TelnetBroadcaster 接口实现，新 WebSocketServer 必须完全兼容此接口
  - `pkg/telnet/client.go` — readLoop/writeLoop 模式，新 WebSocketClient 复用相同模式
  - `pkg/bridge/bridge.go:20-24` — TelnetBroadcaster 接口定义（DataChan + Broadcast），这是必须兼容的契约

  **API/Type References**:
  - `pkg/telnet/server.go:TelnetServer` — 参考其 dataChan、stopChan、mu、clients map 等字段设计
  - `pkg/bridge/bridge.go:TelnetBroadcaster` — 接口签名：`DataChan() <-chan []byte` 和 `Broadcast(data []byte) int`

  **External References**:
  - gorilla/websocket: `https://pkg.go.dev/github.com/gorilla/websocket` — Upgrader.Upgrade() 使用模式
  - gorilla/websocket Chat Example: `https://github.com/gorilla/websocket/tree/main/examples/chat` — 参考其 Conn.ReadMessage/WriteMessage 模式

  **WHY Each Reference Matters**:
  - TelnetServer：理解现有接口实现的字段和方法模式，确保 WebSocketServer 是 drop-in replacement
  - TelnetBroadcaster：这是编译时约束，DataBridge 不变，新实现必须满足此接口
  - gorilla/websocket：了解 WebSocket upgrade、read/write message 的正确用法

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: WebSocketServer 实现 TelnetBroadcaster 接口
    Tool: Bash
    Preconditions: pkg/web/server.go 已创建
    Steps:
      1. 运行 `go build ./pkg/web/...`
      2. 运行 `go vet ./pkg/web/...`
    Expected Result: 零错误零警告
    Failure Indicators: 编译失败或 vet 警告
    Evidence: .sisyphus/evidence/task-1-interface-check.txt

  Scenario: DataChan 和 Broadcast 功能
    Tool: Bash
    Preconditions: WebSocketServer 启动在随机端口
    Steps:
      1. 运行 `go test ./pkg/web/... -v`
      2. 检查测试输出是否通过
    Expected Result: 所有测试通过，DataChan() 返回 channel，Broadcast() 发送数据到客户端
    Failure Indicators: 测试失败
    Evidence: .sisyphus/evidence/task-1-unit-test.txt

  Scenario: 单连接踢出逻辑
    Tool: Bash
    Preconditions: WebSocketServer 启动
    Steps:
      1. 连接第一个 WebSocket 客户端
      2. 连接第二个 WebSocket 客户端
      3. 验证第一个客户端被断开
    Expected Result: 第一个客户端收到关闭帧，第二个客户端正常连接
    Failure Indicators: 两个客户端同时存在
    Evidence: .sisyphus/evidence/task-1-single-conn.txt
  ```

  **Commit**: YES (groups with Task 2)
  - Message: `feat(web): 添加 WebSocket 终端服务`
  - Files: `pkg/web/server.go`, `pkg/web/client.go`, `pkg/web/server_test.go`
  - Pre-commit: `go build ./pkg/web/... && go test ./pkg/web/...`

- [ ] 2. 下载 xterm.js 并创建静态文件（go:embed）

  **What to do**:
  - 通过 npm 下载 xterm.js 官方文件到 `pkg/web/static/`：
    - `xterm.min.js`（核心库）
    - `xterm.css`（样式）
    - `xterm-addon-fit.min.js`（自适应大小插件）
    - 命令：`npm pack @xterm/xterm` → 解压获取文件
  - 创建 `pkg/web/static/terminal.html`：
    - 引用嵌入的 xterm.js 和 xterm.css
    - 创建 Terminal 实例并连接 WebSocket（ws://host/ws）
    - WebSocket onmessage → terminal.write(data)
    - terminal.onData → websocket.send(data)
    - 使用 FitAddon 自适应窗口大小
    - 页面标题 "SerialHub Terminal"
    - 深色背景（#1e1e1e），全屏终端
  - 创建 `pkg/web/embed.go`：`//go:embed static/*` 声明嵌入变量

  **Must NOT do**:
  - 不从 CDN 加载 xterm.js
  - 不添加主题切换、搜索、复制等高级功能
  - 不添加连接配置 UI

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 下载文件+写 HTML，结构清晰
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 3)
  - **Blocks**: Task 4
  - **Blocked By**: None

  **References**:

  **External References**:
  - xterm.js npm: `https://www.npmjs.com/package/@xterm/xterm` — 官方包名和版本
  - xterm.js API: `https://xtermjs.org/docs/api/terminal/classes/Terminal/` — Terminal API 参考
  - xterm.js FitAddon: `https://www.npmjs.com/package/@xterm/addon-fit` — 自适应插件

  **WHY Each Reference Matters**:
  - npm 包名：确保从官方源下载正确包
  - Terminal API：了解 Terminal 构造函数参数、write/onData 方法签名
  - FitAddon：了解如何正确集成自适应插件

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: 静态文件存在且 go:embed 正常
    Tool: Bash
    Preconditions: pkg/web/static/ 已创建
    Steps:
      1. 检查 pkg/web/static/xterm.min.js 存在
      2. 检查 pkg/web/static/xterm.css 存在
      3. 检查 pkg/web/static/xterm-addon-fit.min.js 存在
      4. 检查 pkg/web/static/terminal.html 存在
      5. 检查 pkg/web/embed.go 包含 go:embed 指令
      6. 运行 `go build ./pkg/web/...`
    Expected Result: 所有文件存在，编译成功
    Failure Indicators: 文件缺失或编译失败
    Evidence: .sisyphus/evidence/task-2-static-files.txt

  Scenario: terminal.html 可被 HTTP 服务返回
    Tool: Bash
    Preconditions: HTTP 服务启动
    Steps:
      1. 运行 curl http://localhost:5000/terminal
      2. 检查返回 HTML 包含 "xterm"
    Expected Result: HTTP 200，HTML 包含 xterm 引用
    Failure Indicators: 404 或 HTML 不包含 xterm
    Evidence: .sisyphus/evidence/task-2-terminal-page.txt
  ```

  **Commit**: YES (groups with Task 1)
  - Message: `feat(web): 添加 WebSocket 终端服务`
  - Files: `pkg/web/static/*`, `pkg/web/embed.go`
  - Pre-commit: `go build ./pkg/web/...`

- [ ] 3. 删除 pkg/telnet/ 目录及清理引用

  **What to do**:
  - 删除整个 `pkg/telnet/` 目录
  - 从 `cmd/serialhub/main.go` 中移除所有 telnet 相关代码：
    - 移除 `import "github.com/yourname/serialhub/pkg/telnet"`
    - 移除 `--telnet-port` CLI 参数定义
    - 移除 telnetSrv 创建和启动代码
    - 移除 telnetSrv.Stop() 关闭调用
    - 将 DataBridge 改为使用 WebSocketServer（`pkg/web`）
  - 从 `pkg/bridge/bridge.go` 中移除 telnet 相关注释（如有）
  - 从 `pkg/config/config.go` 中移除 Telnet 配置（TelnetPort 等）
  - 从 `pkg/tray/` 中移除 telnet 端口引用，更新托盘菜单文本
  - 搜索整个代码库确认无残留的 "telnet" 引用（测试文件中的引用也需删除）
  - 运行 `go mod tidy` 清理依赖

  **Must NOT do**:
  - 不修改 DataBridge 接口定义
  - 不修改 SerialManager
  - 不修改 MCP 服务

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 删除代码和更新引用，结构清晰
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2)
  - **Blocks**: Tasks 5, 6
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `cmd/serialhub/main.go:23` — `import "github.com/yourname/serialhub/pkg/telnet"` 需删除
  - `cmd/serialhub/main.go:52` — `--telnet-port` CLI 参数定义需删除
  - `cmd/serialhub/main.go:240-289` — runWithoutTray 中 telnetSrv 创建、启动、关闭逻辑需替换
  - `cmd/serialhub/main.go:141-199` — runWithTray 中 telnetSrv 创建逻辑需替换
  - `pkg/config/config.go` — Telnet 配置项需查找并移除
  - `pkg/tray/tray.go` — 托盘菜单中 telnet 端口显示需更新

  **WHY Each Reference Matters**:
  - main.go：这是启动入口，所有 telnet 服务实例化和生命周期管理都在这里
  - config/tray：这些是 telnet 配置和 UI 的消费方，需要同步清理

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: pkg/telnet/ 目录已删除
    Tool: Bash
    Preconditions: 无
    Steps:
      1. 运行 `ls pkg/telnet/` 确认目录不存在
    Expected Result: 目录不存在或为空
    Failure Indicators: 目录仍存在
    Evidence: .sisyphus/evidence/task-3-telnet-deleted.txt

  Scenario: 代码库无 telnet 引用残留
    Tool: Bash
    Preconditions: 代码修改完成
    Steps:
      1. 运行 `grep -r "telnet" --include="*.go" pkg/ cmd/` 排除测试文件
      2. 检查结果是否为空
    Expected Result: 零匹配（或仅存在于 AGENTS.md 文档中）
    Failure Indicators: 存在 telnet import 或变量引用
    Evidence: .sisyphus/evidence/task-3-no-telnet-refs.txt

  Scenario: 编译成功
    Tool: Bash
    Preconditions: 代码修改完成
    Steps:
      1. 运行 `go build ./...`
    Expected Result: 零错误
    Failure Indicators: 编译失败
    Evidence: .sisyphus/evidence/task-3-build.txt
  ```

  **Commit**: YES
  - Message: `refactor: 删除 Telnet 服务`
  - Files: `pkg/telnet/`, `cmd/serialhub/main.go`, `pkg/config/config.go`, `pkg/tray/`

- [ ] 4. 修改 MCP HTTP 服务注册新路由（/terminal + /ws）

  **What to do**:
  - 修改 `pkg/mcp/server.go` 的 `StartHTTPServer` 方法：
    - 添加 `/terminal` 路由：返回 terminal.html（从 embed 读取）
    - 添加 `/ws` 路由：WebSocket 升级，调用 WebSocketServer 的 HandleWebSocket
    - 添加 `/static/*` 路由：返回嵌入的静态文件（xterm.js、xterm.css）
  - MCPServer 需要持有 WebSocketServer 引用：
    - 构造函数 NewMCPServer 增加一个 WebSocketServer 参数（或通过 setter 注入）
  - WebSocket 升级使用 gorilla/websocket.Upgrader
  - CORS 中间件需允许 WebSocket 升级请求

  **Must NOT do**:
  - 不修改 /mcp 和 /health 路由
  - 不添加认证
  - 不修改 MCP 协议处理逻辑

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 需要理解 MCP HTTP 服务架构并正确集成 WebSocket
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 1, 2)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 7
  - **Blocked By**: Tasks 1, 2

  **References**:

  **Pattern References**:
  - `pkg/mcp/server.go:152-184` — StartHTTPServer 方法，在这里添加新路由
  - `pkg/mcp/server.go:193-206` — withCORS 中间件，WebSocket 升级需要通过 CORS

  **API/Type References**:
  - `pkg/mcp/server.go:MCPServer` — 需要添加 wsServer 字段
  - `pkg/web/server.go` (Task 1 产出) — WebSocketServer 的 HandleWebSocket 方法

  **External References**:
  - gorilla/websocket Upgrader: `https://pkg.go.dev/github.com/gorilla/websocket#Upgrader` — CheckOrigin 设置

  **WHY Each Reference Matters**:
  - StartHTTPServer：这是路由注册的唯一点，新路由必须加在这里
  - CORS：WebSocket 升级请求也需要通过 CORS 检查
  - WebSocketServer：需要知道如何调用 HandleWebSocket

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: /terminal 路由返回 HTML
    Tool: Bash (curl)
    Preconditions: SerialHub 启动在 5000 端口
    Steps:
      1. 运行 `curl -s http://localhost:5000/terminal`
      2. 检查返回内容包含 "xterm"
    Expected Result: HTTP 200，HTML 包含 xterm 引用
    Failure Indicators: 404 或内容不包含 xterm
    Evidence: .sisyphus/evidence/task-4-terminal-route.txt

  Scenario: /ws 路由支持 WebSocket 升级
    Tool: Bash (websocat 或 Go 测试)
    Preconditions: SerialHub 启动
    Steps:
      1. 使用 Go 测试代码连接 ws://localhost:5000/ws
      2. 发送 "hello" 消息
      3. 验证连接成功
    Expected Result: WebSocket 连接成功建立
    Failure Indicators: HTTP 400 或连接失败
    Evidence: .sisyphus/evidence/task-4-ws-upgrade.txt

  Scenario: /health 和 /mcp 路由仍正常
    Tool: Bash (curl)
    Preconditions: SerialHub 启动
    Steps:
      1. `curl http://localhost:5000/health`
      2. 检查返回 {"status":"ok"}
    Expected Result: HTTP 200，原有功能不受影响
    Failure Indicators: 路由丢失或返回错误
    Evidence: .sisyphus/evidence/task-4-existing-routes.txt
  ```

  **Commit**: YES (groups with Task 5)
  - Message: `feat(mcp): 集成 WebSocket 终端到 HTTP 服务`
  - Files: `pkg/mcp/server.go`

- [ ] 5. 修改 main.go 启动逻辑

  **What to do**:
  - 修改 `cmd/serialhub/main.go`：
    - 将 `import telnet` 替换为 `import web`
    - 将 `telnetSrv` 替换为 `wsSrv`（类型为 `*web.WebSocketServer`）
    - 将 DataBridge 的第二个参数改为 wsSrv
    - 移除 `--telnet-port` 参数的使用（共用 MCP 端口）
    - 更新日志消息（"Telnet 服务已启动" → "Web 终端已启动"）
  - 确保 runWithTray 和 runWithoutTray 都更新

  **Must NOT do**:
  - 不修改 DataBridge 创建逻辑
  - 不修改 MCP 服务创建逻辑
  - 不修改串口管理器逻辑

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简单的引用替换
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 1, 3)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 7
  - **Blocked By**: Tasks 1, 3

  **References**:

  **Pattern References**:
  - `cmd/serialhub/main.go:240-289` — runWithoutTray 中 telnet → web 替换
  - `cmd/serialhub/main.go:141-199` — runWithTray 中 telnet → web 替换
  - `cmd/serialhub/main.go:256-262` — DataBridge 创建，第二个参数改为 wsSrv

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: 编译通过
    Tool: Bash
    Preconditions: 代码修改完成
    Steps:
      1. `go build ./cmd/serialhub/...`
    Expected Result: 零错误
    Evidence: .sisyphus/evidence/task-5-build.txt
  ```

  **Commit**: YES (groups with Task 4)
  - Message: `feat(mcp): 集成 WebSocket 终端到 HTTP 服务`
  - Files: `cmd/serialhub/main.go`

- [ ] 6. 修改配置和托盘

  **What to do**:
  - 修改 `pkg/config/config.go`：
    - 移除 TelnetPort 配置项（共用 MCP 端口）
    - 或保留但重命名为 WebPort（如果需要单独端口）
  - 修改 `pkg/tray/tray.go`：
    - 更新托盘菜单：移除 "Telnet 端口" 显示
    - 更新网络信息显示为 "Web: http://host:port/terminal"
  - 修改 `config.toml`（如存在）：移除 [telnet] 配置节

  **Must NOT do**:
  - 不修改串口配置
  - 不修改 MCP 配置
  - 不修改托盘的核心功能（串口连接/断开）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简单的配置和文本更新
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 3)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 7
  - **Blocked By**: Task 3

  **References**:

  **Pattern References**:
  - `pkg/config/config.go` — 查找 TelnetPort 字段
  - `pkg/tray/tray.go` — 查找 telnet 端口显示逻辑

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: 编译通过
    Tool: Bash
    Preconditions: 代码修改完成
    Steps:
      1. `go build ./...`
    Expected Result: 零错误
    Evidence: .sisyphus/evidence/task-6-build.txt
  ```

  **Commit**: YES (groups with Tasks 4, 5)
  - Message: `feat(mcp): 集成 WebSocket 终端到 HTTP 服务`
  - Files: `pkg/config/config.go`, `pkg/tray/tray.go`

- [ ] 7. 构建验证和测试修复

  **What to do**:
  - 运行 `go build ./...` 确保零错误
  - 运行 `go vet ./...` 确保零警告
  - 运行 `go test ./...` 确保所有测试通过
  - 修复所有因 Telnet 删除导致的测试失败：
    - 删除 `pkg/telnet/server_test.go` 和 `pkg/telnet/client_test.go`
    - 更新 `pkg/bridge/bridge_test.go` 中引用 telnet mock 的部分
    - 确保所有测试不依赖 telnet 包
  - 运行 `go mod tidy` 清理依赖

  **Must NOT do**:
  - 不跳过任何测试失败
  - 不使用 `_test` 后缀的文件作为非测试文件

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 需要修复多个文件的测试，工作量较大
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Tasks 4, 5, 6)
  - **Parallel Group**: Wave 3
  - **Blocks**: Tasks 8, F1-F4
  - **Blocked By**: Tasks 4, 5, 6

  **References**:

  **Pattern References**:
  - `pkg/bridge/bridge_test.go` — mockTelnetBroadcaster 需要重命名或确认仍兼容
  - `cmd/serialhub/main_test.go` — 可能引用 telnet 配置

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: 完整构建和测试
    Tool: Bash
    Preconditions: 所有代码修改完成
    Steps:
      1. `go build ./...`
      2. `go vet ./...`
      3. `go test ./...`
    Expected Result: 零错误、零警告、所有测试通过
    Failure Indicators: 任何构建错误或测试失败
    Evidence: .sisyphus/evidence/task-7-full-test.txt
  ```

  **Commit**: YES
  - Message: `test: 修复 Telnet 删除后的测试`
  - Files: 相关测试文件
  - Pre-commit: `go test ./...`

- [ ] 8. 更新文档

  **What to do**:
  - 更新 `README.md`：
    - 移除 Telnet 相关内容
    - 添加 Web 终端使用说明（http://localhost:5000/terminal）
    - 更新架构图
    - 更新 CLI 参数说明（移除 --telnet-port）
    - 更新快速开始指南
  - 更新 `AGENTS.md`：
    - 移除 Telnet 相关技术栈和文件结构
    - 添加 Web 终端相关说明
    - 更新构建/测试命令（如有变化）
  - 更新 `MCP.md`（如有 Telnet 相关内容）
  - 更新 `bin/config.toml`：移除 [telnet] 配置节

  **Must NOT do**:
  - 不修改代码逻辑
  - 不添加新的功能描述

  **Recommended Agent Profile**:
  - **Category**: `writing`
    - Reason: 纯文档更新
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on Task 7)
  - **Parallel Group**: Wave 3
  - **Blocks**: F1-F4
  - **Blocked By**: Task 7

  **References**:

  **Pattern References**:
  - `README.md` — 现有文档结构
  - `AGENTS.md` — AI 代理指南

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: 文档无 Telnet 残留引用
    Tool: Bash
    Preconditions: 文档更新完成
    Steps:
      1. grep -i "telnet" README.md AGENTS.md MCP.md
    Expected Result: 零匹配（或仅存在于历史说明中）
    Evidence: .sisyphus/evidence/task-8-docs-check.txt
  ```

  **Commit**: YES
  - Message: `docs: 更新文档，移除 Telnet，添加 Web 终端`
  - Files: `README.md`, `AGENTS.md`, `MCP.md`, `bin/config.toml`

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE.

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, curl endpoint, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./...` + `go build ./...` + `go test ./...`. Review all changed files for: `as any`/`@ts-ignore`, empty catches, console.log in prod, commented-out code, unused imports. Check AI slop: excessive comments, over-abstraction, generic names.
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [ ] F3. **Real Manual QA** — `unspecified-high` (+ `playwright` skill)
  Start from clean state. Execute EVERY QA scenario from EVERY task — follow exact steps, capture evidence. Test cross-task integration. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | VERDICT`

- [ ] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built, nothing beyond spec was built. Check "Must NOT do" compliance.
  Output: `Tasks [N/N compliant] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **Wave 1**: `feat(web): 添加 WebSocket 终端服务替代 Telnet` — pkg/web/
- **Wave 2**: `refactor(mcp): 集成 WebSocket 终端到 HTTP 服务` — pkg/mcp/server.go, cmd/serialhub/main.go
- **Wave 3**: `chore: 删除 Telnet 服务并更新文档` — pkg/telnet/, README.md, AGENTS.md

---

## Success Criteria

### Verification Commands
```bash
go build ./...                    # Expected: 零错误
go vet ./...                      # Expected: 零警告
go test ./...                     # Expected: 所有测试通过（telnet 测试已删除）
curl http://localhost:5000/health  # Expected: {"status":"ok"}
curl http://localhost:5000/terminal # Expected: HTML 页面
```

### Final Checklist
- [ ] 浏览器 /terminal 显示 xterm.js 终端
- [ ] 串口数据实时显示，无阶梯状换行问题
- [ ] 键盘输入发送到串口
- [ ] DataBridge 接口不变
- [ ] 所有 Telnet 代码和引用已删除
- [ ] pkg/telnet/ 目录不存在
