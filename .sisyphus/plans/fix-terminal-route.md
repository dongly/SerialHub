# 修复 /terminal 页面无法访问 xterm.js 终端

## TL;DR

> **问题**：两个 HTTP 服务器竞争同一端口，WebSocketServer 先绑定导致 /terminal 被 `/` 兜底路由拦截。
> **方案**：移除 WebSocketServer 的独立 HTTP 服务器启动，仅保留 MCP 服务器的统一路由。
>
> **改动范围**：5 个文件（3 修复 + 2 测试新增），约 120 行变更
>
> **预估工作量**：Medium
> **并行执行**：部分（Task 4、5 并行）
> **关键路径**：Task 1 → Task 2 → Task 3 → Task 4/5（并行）→ 验证

---

## Context

### 原始问题
用户访问 `http://127.0.0.1:5000/terminal` 返回 `"SerialHub WebSocket Server\nUse /ws for WebSocket connection"`，而非 xterm.js 终端页面。

### 根因分析
`cmd/serialhub/main.go` 中依次启动两个 HTTP 服务器：

1. `wsSrv.Start()`（`pkg/web/server.go:122-142`）— 绑定 `host:mcpPort`，仅注册 `/ws` 和 `/` 两个路由
2. `mcpSrv.StartHTTPServer(addr)`（`pkg/mcp/server.go:162-218`）— 绑定同一 `addr`，注册 `/mcp`、`/health`、`/terminal`、`/ws`、`/static/` 路由

先绑定的赢。当前 WebSocketServer 先赢，因此：
- `/ws` → 由 WS 服务器处理（正常）
- `/terminal` → 命中 WS 服务器的 `/` 兜底路由（返回错误文本）
- `/health`、`/mcp` → 404（WS 服务器未注册这些路由）

### 硬件环境
- **串口**：COM4，TX-RX 短接（回环模式）
- **波特率**：115200
- **所有测试不能跳过，必须全部通过**

### Metis 审查补充
- `Stop()` 的 `close(s.stopChan)` 存在双重关闭 panic 风险 → 需加保护
- `Start()` 应保留为 no-op 而非直接删除 → 保持向后兼容
- `TestWebSocketServer_Start` 测试需要移除

---

## Work Objectives

### Core Objective
消除双 HTTP 服务器端口竞争，让 /terminal、/health、/mcp、/ws、/static/ 全部由 MCP 服务器的统一路由处理器提供。补充 Go 单元测试和 Python E2E 测试（含 xterm.js 终端全链路验证）。

### Concrete Deliverables
- `cmd/serialhub/main.go` — 移除 `wsSrv.Start()` 和 `wsSrv.Stop()` 调用
- `pkg/web/server.go` — 移除 `server` 字段，`Start()` 改为 no-op，`Stop()` 加双重关闭保护
- `pkg/web/server_test.go` — 移除 `TestWebSocketServer_Start`，新增路由注册测试
- `tests/integration/test_serialhub.py` — 新增 `TestWebTerminal` E2E 测试类

### Definition of Done
- [ ] `go vet ./...` 零错误
- [ ] `go build ./cmd/serialhub` 编译成功
- [ ] `go test ./...` 全部通过
- [ ] `pytest tests/integration/test_serialhub.py -v -k "TestWebTerminal"` 全部通过
- [ ] `pytest tests/integration/test_serialhub.py -v` 全部通过（含硬件回环）
- [ ] /terminal 返回 xterm.js HTML 页面
- [ ] /health 返回健康检查 JSON
- [ ] /ws WebSocket 连接正常
- [ ] xterm.js 终端完整链路：打开页面 → WebSocket 连接 → 发送数据 → 回环验证

### Must Have
- /terminal 返回完整 xterm.js 终端页面（包含 xterm.min.js、xterm.css 引用）
- /ws WebSocket 连接正常工作
- /health、/mcp 路由正常可达
- /static/ 静态资源正常加载
- 所有 Go 测试通过
- 所有 Python E2E 测试通过（含 COM4 回环）
- xterm.js 终端全链路测试通过

### Must NOT Have（防护栏）
- 不修改 WebSocket 协议处理逻辑（HandleWebSocket 方法）
- 不修改 MCP 服务器的路由注册（已正确）
- 不修改客户端连接管理（踢掉旧连接行为）
- 不修改 Broadcast、DataChan 功能
- 不添加新 Go 依赖
- 不重构 WebSocketServer 结构体的其他字段
- 不修改配置逻辑或端口/host 参数
- 不跳过任何测试
- 不添加 `@pytest.skip` 或条件跳过逻辑

---

## Verification Strategy

> **零人工干预** — 所有验证由 agent 执行。

### Test Decision
- **Infrastructure exists**: YES（go test + pytest）
- **Automated tests**: Tests-after（修复后运行测试验证）
- **Go Framework**: go test
- **Python Framework**: pytest + requests + websocket-client

### QA Policy
每个 task 包含 agent 执行的 QA 场景。
证据保存到 `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`。

---

## Execution Strategy

### 执行波次

```
Wave 1（顺序）:
├── Task 1: main.go 移除 wsSrv.Start/Stop [quick]
└── Task 2: server.go 重构 [quick]

Wave 2（顺序，依赖 Wave 1）:
└── Task 3: server_test.go 清理旧测试 + 新增路由测试 [quick]

Wave 3（并行，依赖 Wave 2）:
├── Task 4: Go 全量测试验证 [quick]
└── Task 5: Python E2E TestWebTerminal 测试 [unspecified-high]

Wave FINAL:
├── F1: 构建与静态分析
├── F2: Go 全量测试
├── F3: Python 全量测试（含硬件回环）
└── F4: 路由验证

Critical Path: Task 1 → Task 2 → Task 3 → Task 4 → F1-F4
```

### Agent Dispatch Summary
- Task 1: `quick` — main.go
- Task 2: `quick` — server.go
- Task 3: `quick` — server_test.go
- Task 4: `quick` — Go 测试验证
- Task 5: `unspecified-high` — Python E2E 测试
- F1-F3: `quick` — 验证
- F4: `quick` — 路由验证

---

## TODOs

- [ ] 1. 移除 main.go 中 wsSrv.Start() 和 wsSrv.Stop() 调用

  **What to do**:
  - 在 `runWithTray` 函数中：移除 `wsSrv.Start()` 调用及其错误检查代码块
  - 在 `runWithTray` 函数中：移除 `wsSrv.Stop()` 调用
  - 在 `runWithoutTray` 函数中：移除 `wsSrv.Start()` 调用及其错误检查代码块
  - 在 `runWithoutTray` 函数中：移除 `wsSrv.Stop()` 调用
  - 保留 `wsSrv` 变量的创建和传递（MCP 服务器的 `/ws` 路由仍需要 `wsSrv.HandleWebSocket`）
  - 保留 `NewWebSocketServer` 调用（仍需要其 DataChan、Broadcast 等功能）

  **Must NOT do**:
  - 不删除 `NewWebSocketServer` 调用
  - 不删除 `bridge.NewDataBridge` 调用
  - 不删除 `mcp.NewMCPServer` 调用
  - 不修改日志消息格式

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单文件、少量行删除，明确的改动
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 1（先于 Task 2）
  - **Blocks**: Task 2, Task 3
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `cmd/serialhub/main.go` — 搜索 `wsSrv.Start()` 和 `wsSrv.Stop()` 的所有调用位置，共 4 处（runWithTray 和 runWithoutTray 各 2 处）

  **API/Type References**:
  - `pkg/mcp/server.go:188-194` — MCP 服务器已注册 /ws 路由委托给 wsSrv.HandleWebSocket（确认移除后不丢功能）
  - `pkg/web/server.go:122-142` — WebSocketServer.Start() 当前创建的独立 HTTP 服务器（冲突源）

  **WHY Each Reference Matters**:
  - main.go 中的 Start/Stop 调用位置：精确定位要删除的代码
  - mcp/server.go 的 /ws 路由：确认移除后 WebSocket 功能不会丢失

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: 编译通过
    Tool: Bash
    Preconditions: 代码已修改
    Steps:
      1. 运行 `go build ./cmd/serialhub`
    Expected Result: 命令退出码 0，无编译错误
    Failure Indicators: 编译错误输出包含 "undefined" 或 "not used"
    Evidence: .sisyphus/evidence/task-1-build.txt

  Scenario: 静态分析通过
    Tool: Bash
    Steps:
      1. 运行 `go vet ./cmd/serialhub/...`
    Expected Result: 零错误输出
    Failure Indicators: 任何 vet 错误
    Evidence: .sisyphus/evidence/task-1-vet.txt
  ```

  **Commit**: NO（与后续 task 合并提交）

- [ ] 2. 重构 WebSocketServer：移除 server 字段，Start() 改为 no-op，Stop() 加双重关闭保护

  **What to do**:
  - 从 `WebSocketServer` 结构体中移除 `server *http.Server` 字段
  - 将 `Start()` 方法改为 no-op：`func (s *WebSocketServer) Start() error { return nil }`
  - 修改 `Stop()` 方法：
    - 移除 `s.server` 相关代码（字段已删）
    - 在 `close(s.stopChan)` 前加 select 保护防止双重关闭 panic：
      ```go
      select {
      case <-s.stopChan:
          // 已关闭
      default:
          close(s.stopChan)
      }
      ```

  **Must NOT do**:
  - 不修改 `HandleWebSocket` 方法
  - 不修改 `Broadcast` 方法
  - 不修改 `DataChan` / `ClientCount` / `HasClient` / `removeClient`
  - 不修改 `WebSocketClient` 及其方法
  - 不修改 `upgrader` 配置
  - 不修改 `stopChan` 的创建方式

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单文件、结构体字段和方法的小改动
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 1（依赖 Task 1）
  - **Blocks**: Task 3
  - **Blocked By**: Task 1

  **References**:

  **Pattern References**:
  - `pkg/web/server.go:17-29` — WebSocketServer 结构体定义（移除 server 字段）
  - `pkg/web/server.go:122-142` — Start() 方法（改为 no-op）
  - `pkg/web/server.go:145-164` — Stop() 方法（加双重关闭保护，移除 s.server 引用）

  **WHY Each Reference Matters**:
  - 结构体定义：精确定位要删除的 server 字段
  - Start() 方法：整个方法体替换为 return nil
  - Stop() 方法：保留 client 清理逻辑，移除 server 关闭逻辑，加 stopChan 保护

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: 编译通过
    Tool: Bash
    Preconditions: Task 1 和 Task 2 代码已修改
    Steps:
      1. 运行 `go build ./cmd/serialhub`
    Expected Result: 退出码 0
    Failure Indicators: "s.server undefined" 或 "server not found"
    Evidence: .sisyphus/evidence/task-2-build.txt

  Scenario: vet 通过
    Tool: Bash
    Steps:
      1. 运行 `go vet ./pkg/web/...`
    Expected Result: 零错误
    Evidence: .sisyphus/evidence/task-2-vet.txt
  ```

  **Commit**: NO（与后续 task 合并提交）

- [ ] 3. 更新 server_test.go：移除旧测试，新增路由注册测试

  **What to do**:
  - 移除 `TestWebSocketServer_Start` 测试函数（221-230 行，测试已移除的 Start() 功能）
  - 新增 `TestWebSocketServer_Start_NoOp` 测试：验证 Start() 返回 nil 不报错
  - 新增 `TestWebSocketServer_DoubleStop` 测试：验证 Stop() 调用两次不 panic
  - 新增 `TestTerminalRoute` 测试：用 httptest 验证 /terminal 路由返回 HTML（包含 `xterm.min.js`）
  - 新增 `TestHealthRoute` 测试：用 httptest 验证 /health 返回 `{"status":"ok"}`
  - 新增 `TestStaticRoute` 测试：用 httptest 验证 /static/ 路由可访问
  - 所有新测试使用 `httptest.NewServer` 模式（参照现有 `newTestServer` helper）
  - 路由测试需构建与 `mcp/server.go:StartHTTPServer` 相同的 mux（/terminal、/health、/static/、/ws）

  **Must NOT do**:
  - 不修改 `newTestServer` helper 函数
  - 不修改 `dialWS` / `readWelcome` helper
  - 不修改其他已有测试函数
  - 不修改 `testDialer` 变量
  - 不引入新依赖

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单文件测试修改，模式与现有测试一致
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 2（依赖 Task 1、2）
  - **Blocks**: Task 4, Task 5
  - **Blocked By**: Task 1, Task 2

  **References**:

  **Pattern References**:
  - `pkg/web/server_test.go:21-35` — newTestServer helper（httptest.Server 模式参考）
  - `pkg/web/server_test.go:49-80` — TestWebSocketServer_DataChan（现有测试风格参考）
  - `pkg/web/server_test.go:221-230` — TestWebSocketServer_Start（需移除）

  **API/Type References**:
  - `pkg/mcp/server.go:170-201` — StartHTTPServer 中的 mux 路由注册（/terminal、/health、/ws、/static/）
  - `pkg/web/embed.go` — StaticFiles 嵌入的静态资源（terminal.html、xterm.min.js 等）
  - `pkg/web/static/terminal.html:29` — 引用 `xterm.min.js`（测试断言依据）

  **WHY Each Reference Matters**:
  - newTestServer：新测试的 helper 模式参考
  - StartHTTPServer mux 路由：需要复制相同的路由注册逻辑到测试中
  - StaticFiles embed：测试需要访问嵌入的静态文件
  - terminal.html：验证返回的 HTML 包含正确的 JS/CSS 引用

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: pkg/web 包所有测试通过
    Tool: Bash
    Preconditions: 所有代码修改完成
    Steps:
      1. 运行 `go test ./pkg/web/... -v`
    Expected Result: 所有测试 PASS，无 FAIL，0 个 SKIP
    Failure Indicators: 任何 FAIL 或 panic
    Evidence: .sisyphus/evidence/task-3-web-tests.txt

  Scenario: 全量 Go 测试通过
    Tool: Bash
    Steps:
      1. 运行 `go test ./... -v`
    Expected Result: 所有包 PASS，0 个 FAIL，0 个 SKIP
    Failure Indicators: 任何 FAIL
    Evidence: .sisyphus/evidence/task-3-all-tests.txt
  ```

  **Commit**: NO（与 Task 1、2 合并提交）

- [ ] 4. Go 全量构建与测试验证

  **What to do**:
  - 运行 `go vet ./...` — 确认零错误
  - 运行 `go build ./cmd/serialhub` — 确认编译成功
  - 运行 `go test ./...` — 确认所有测试通过，0 个 FAIL，0 个 SKIP

  **Must NOT do**:
  - 不修改任何代码
  - 不跳过任何测试

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 纯验证任务
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 Task 5 并行）
  - **Parallel Group**: Wave 3
  - **Blocks**: Final Verification
  - **Blocked By**: Task 3

  **References**:
  - 无代码引用，纯验证

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: go vet 零错误
    Tool: Bash
    Steps:
      1. 运行 `go vet ./...`
    Expected Result: 退出码 0，无输出
    Evidence: .sisyphus/evidence/task-4-vet.txt

  Scenario: go build 编译成功
    Tool: Bash
    Steps:
      1. 运行 `go build -o bin/serialhub.exe ./cmd/serialhub`
    Expected Result: 退出码 0，bin/serialhub.exe 存在
    Evidence: .sisyphus/evidence/task-4-build.txt

  Scenario: go test 全部通过
    Tool: Bash
    Steps:
      1. 运行 `go test ./...`
    Expected Result: 所有包 PASS，0 FAIL
    Evidence: .sisyphus/evidence/task-4-tests.txt
  ```

  **Commit**: NO（验证任务）

- [ ] 5. 新增 TestWebTerminal E2E 测试类（tests/integration/test_serialhub.py）

  **What to do**:

  在 `tests/integration/test_serialhub.py` 文件末尾新增 `TestWebTerminal` 测试类。

  **安装依赖**（测试开始前执行）:
  ```bash
  pip install websocket-client pytest
  ```

  **测试方法清单**（全部使用 `serialhub_server` fixture）:

  **5a. test_terminal_page_html** — /terminal 返回 HTML 页面
  - `GET http://127.0.0.1:{mcp_port}/terminal`
  - 断言 status_code == 200
  - 断言 Content-Type 包含 "text/html"
  - 断言 body 包含 `xterm.min.js`
  - 断言 body 包含 `xterm.css`
  - 断言 body 包含 `terminal-container`
  - 断言 body 包含 `new Terminal`

  **5b. test_health_endpoint** — /health 返回 JSON
  - `GET http://127.0.0.1:{mcp_port}/health`
  - 断言 `{"status": "ok"}`

  **5c. test_static_files_accessible** — /static/ 静态资源可访问
  - `GET http://127.0.0.1:{mcp_port}/static/xterm.min.js`
  - 断言 status_code == 200
  - `GET http://127.0.0.1:{mcp_port}/static/xterm.css`
  - 断言 status_code == 200

  **5d. test_websocket_connect_and_welcome** — WebSocket 连接 + 欢迎消息
  - 用 `websocket-client` 连接 `ws://127.0.0.1:{mcp_port}/ws`
  - 断言收到 `"Connected to SerialHub"` 欢迎消息
  - 断开连接

  **5e. test_websocket_send_receive_loopback** — xterm WebSocket 回环测试（需要 COM4）
  - 连接串口 COM4 115200（通过 MCP serial_connect）
  - 连接 WebSocket `ws://127.0.0.1:{mcp_port}/ws`
  - 读取欢迎消息
  - 通过 WebSocket 发送 `"HelloFromWebTerminal\n"`
  - 等待 0.5s
  - 通过 MCP serial_read 读取（timeout: 3000ms）
  - 断言收到的数据包含 `"HelloFromWebTerminal"`
  - 断开 WebSocket，断开串口

  **5f. test_websocket_receive_from_serial** — 串口数据转发到 xterm
  - 连接串口 COM4 115200
  - 连接 WebSocket
  - 读取欢迎消息
  - 通过 MCP serial_write 发送 `"DataToWebTerminal"`（addNewline: false）
  - 等待 0.5s
  - 从 WebSocket 读取消息
  - 断言 WebSocket 收到的数据包含 `"DataToWebTerminal"`
  - 断开 WebSocket，断开串口

  **5g. test_websocket_unicode_data** — Unicode 数据传输
  - 连接串口 COM4
  - 连接 WebSocket
  - 发送 Unicode 字符串 `"你好世界🎉\n"` 到 WebSocket
  - 等待 0.5s
  - 通过 MCP serial_read 读取
  - 断言包含 `"你好世界🎉"`

  **5h. test_websocket_large_data** — 大数据传输
  - 连接串口 COM4
  - 连接 WebSocket
  - 发送 1024 字节随机 ASCII 数据（带 `\n`）
  - 等待 1s
  - 通过 MCP serial_read 多次读取
  - 断言接收字节数 >= 900（90% 成功率）

  **5i. test_websocket_close_and_reconnect** — 关闭后重连
  - 连接 WebSocket，收到欢迎消息
  - 关闭 WebSocket
  - 重新连接 WebSocket
  - 断言再次收到欢迎消息

  **5j. test_mcp_endpoint_accessible** — MCP 端点验证
  - POST `http://127.0.0.1:{mcp_port}/mcp` 调用 serial_list
  - 断言 status_code == 200
  - 断言响应包含 `"result"`

  **测试代码风格**:
  - 中文 docstring：`"""xxx 测试"""`
  - 断言风格：`assert condition, f"描述: 实际 {actual}"`
  - 复用现有 helper：`mcp_call()`、`_get_content_text()`、`wait_for_health()`
  - WebSocket 使用 `websocket.WebSocket()` 或 `websocket.create_connection()`

  **Must NOT do**:
  - 不修改 `serialhub_server` fixture
  - 不修改已有测试类和方法
  - 不添加 `@pytest.skip` 或条件跳过
  - 不修改 `README.md`
  - 不添加新的 Python 依赖到 requirements（`websocket-client` 是测试依赖）
  - 不修改 `mcp_call()` 等辅助函数

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 新增完整 E2E 测试类，涉及 WebSocket、MCP、串口回环多种交互，需要仔细处理时序和数据格式
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 Task 4 并行）
  - **Parallel Group**: Wave 3
  - **Blocks**: Final Verification
  - **Blocked By**: Task 1, Task 2, Task 3（需要修复后的代码才能通过测试）

  **References**:

  **Pattern References**:
  - `tests/integration/test_serialhub.py:747-818` — TestMCPTools 类（E2E 测试风格参考，使用 serialhub_server fixture + mcp_call）
  - `tests/integration/test_serialhub.py:859-984` — TestTelnet.test_telnet_loopback（回环测试模式参考：连接串口 → 发送 → MCP 读取 → 验证）
  - `tests/integration/test_serialhub.py:152-188` — mcp_call() helper（MCP JSON-RPC 调用模式）
  - `tests/integration/test_serialhub.py:91-101` — wait_for_health() helper
  - `tests/integration/test_serialhub.py:739-744` — _get_content_text() helper

  **API/Type References**:
  - `pkg/web/static/terminal.html:29-30` — xterm.min.js 和 xterm-addon-fit.min.js 的引用（测试断言依据）
  - `pkg/web/static/terminal.html:34` — `new Terminal(...)` 构造（测试断言依据）
  - `pkg/web/static/terminal.html:33` — `terminal-container` DOM 元素（测试断言依据）
  - `pkg/web/static/terminal.html:52` — WebSocket 连接 `ws://host/ws`（测试断言依据）
  - `pkg/mcp/server.go:178-186` — /terminal 路由处理（返回嵌入的 terminal.html）
  - `pkg/mcp/server.go:188-194` — /ws 路由（委托给 wsSrv.HandleWebSocket）
  - `pkg/mcp/server.go:196-201` — /static/ 路由（静态文件服务）
  - `pkg/web/server.go` — HandleWebSocket 中欢迎消息 `"Connected to SerialHub"`（5d 断言依据）

  **External References**:
  - websocket-client 库：`pip install websocket-client`，用法 `ws = websocket.create_connection(url)`，`ws.recv()`，`ws.send(data)`

  **WHY Each Reference Matters**:
  - TestMCPTools：新测试类的 fixture 使用模式和 mcp_call 调用模式参考
  - TestTelnet.test_telnet_loopback：完整的回环测试流程参考（连接串口→发送→读取→验证）
  - terminal.html：确认页面包含哪些可断言的元素（JS 文件名、DOM ID、构造函数）
  - /terminal 路由：理解服务端如何返回页面，测试如何验证
  - HandleWebSocket 欢迎消息：确认 WebSocket 连接后的第一条消息内容

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: TestWebTerminal 全部通过
    Tool: Bash
    Preconditions:
      - 代码修复完成（Task 1-3）
      - COM4 TX-RX 短接
      - pip install websocket-client pytest 已安装
    Steps:
      1. 运行 `pip install websocket-client pytest requests`
      2. 运行 `$env:SERIALHUB_INTEGRATION_TEST="1"; $env:SERIALHUB_TEST_PORT="COM4"; pytest tests/integration/test_serialhub.py -v -k "TestWebTerminal"`
    Expected Result: 10 个测试全部 PASS，0 FAIL，0 SKIP
    Failure Indicators: 任何 FAIL 或 SKIP
    Evidence: .sisyphus/evidence/task-5-web-terminal-tests.txt

  Scenario: Python 全量测试通过
    Tool: Bash
    Steps:
      1. 运行 `$env:SERIALHUB_INTEGRATION_TEST="1"; $env:SERIALHUB_TEST_PORT="COM4"; pytest tests/integration/test_serialhub.py -v`
    Expected Result: 所有测试全部 PASS，0 FAIL，0 SKIP
    Failure Indicators: 任何 FAIL 或 SKIP
    Evidence: .sisyphus/evidence/task-5-all-tests.txt
  ```

  **Commit**: YES（与 Task 1-3 合并）
  - Message: `fix(web): 移除 WebSocketServer 独立 HTTP 服务器，修复 /terminal 路由冲突`
  - Files: `cmd/serialhub/main.go`, `pkg/web/server.go`, `pkg/web/server_test.go`, `tests/integration/test_serialhub.py`
  - Pre-commit: `go vet ./... && go build ./cmd/serialhub && go test ./... && $env:SERIALHUB_INTEGRATION_TEST="1"; $env:SERIALHUB_TEST_PORT="COM4"; pytest tests/integration/test_serialhub.py -v -k "TestWebTerminal"`

---

## Final Verification Wave（所有 task 完成后）

- [ ] F1. **构建与静态分析** — `quick`
  运行 `go vet ./...` 和 `go build ./cmd/serialhub`，确认零错误、编译成功。
  Output: `Vet [PASS/FAIL] | Build [PASS/FAIL]`

- [ ] F2. **Go 全量测试** — `quick`
  运行 `go test ./...`，确认所有测试通过，0 FAIL。
  Output: `Tests [N pass / N fail] | VERDICT`

- [ ] F3. **Python E2E 全量测试** — `quick`
  运行 `$env:SERIALHUB_INTEGRATION_TEST="1"; $env:SERIALHUB_TEST_PORT="COM4"; pytest tests/integration/test_serialhub.py -v`
  确认所有测试通过，0 FAIL，0 SKIP。
  Output: `Tests [N pass / N fail / N skip] | VERDICT`

- [ ] F4. **路由验证** — `quick`
  启动服务后，用 curl 验证所有路由：
  - `curl -s http://127.0.0.1:5000/terminal` → 包含 `xterm.min.js`
  - `curl -s http://127.0.0.1:5000/health` → `{"status":"ok"}`
  - `curl -s -X POST http://127.0.0.1:5000/mcp -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_list"},"id":1}'` → 包含 `"result"`
  - `curl -s http://127.0.0.1:5000/static/xterm.min.js` → 包含 JavaScript 内容
  Output: `Terminal [PASS/FAIL] | Health [PASS/FAIL] | MCP [PASS/FAIL] | Static [PASS/FAIL] | VERDICT`

---

## Commit Strategy

- **单次提交**: `fix(web): 移除 WebSocketServer 独立 HTTP 服务器，修复 /terminal 路由冲突`
  - Files: `cmd/serialhub/main.go`, `pkg/web/server.go`, `pkg/web/server_test.go`, `tests/integration/test_serialhub.py`
  - Pre-commit: `go vet ./... && go build ./cmd/serialhub && go test ./... && $env:SERIALHUB_INTEGRATION_TEST="1"; $env:SERIALHUB_TEST_PORT="COM4"; pytest tests/integration/test_serialhub.py -v -k "TestWebTerminal"`

---

## Success Criteria

### Verification Commands
```bash
go vet ./...                                                    # Expected: 零错误
go build -o bin/serialhub.exe ./cmd/serialhub                   # Expected: 编译成功
go test ./...                                                    # Expected: 全部 PASS
$env:SERIALHUB_INTEGRATION_TEST="1"; $env:SERIALHUB_TEST_PORT="COM4"; pytest tests/integration/test_serialhub.py -v  # Expected: 全部 PASS
```

### Final Checklist
- [ ] /terminal 返回 xterm.js HTML 页面
- [ ] /health 返回健康检查 JSON
- [ ] /mcp MCP 工具调用正常
- [ ] /ws WebSocket 连接正常，收到欢迎消息
- [ ] /static/ 静态资源可访问
- [ ] xterm WebSocket → 串口回环测试通过（COM4）
- [ ] 串口 → xterm WebSocket 转发测试通过（COM4）
- [ ] Unicode 数据传输正常
- [ ] 大数据传输正常
- [ ] WebSocket 重连正常
- [ ] 所有 go test 通过（0 FAIL，0 SKIP）
- [ ] 所有 pytest 通过（0 FAIL，0 SKIP）
- [ ] go vet 零错误
