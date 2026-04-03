# MCP StreamableHTTP 迁移计划

## TL;DR

> **目标**：将 SerialHub 的 MCP 服务从 SSE 传输模式迁移到 StreamableHTTP 模式，并添加完整测试
> 
> **改动范围**：`pkg/mcp/server.go` + `pkg/mcp/server_test.go` + Python 集成测试 + `AGENTS.md` + `README.md`
> 
> **预计工作量**：Medium
> **并行执行**：NO - 顺序依赖

---

## Context

### 问题背景
当前 MCP 服务使用 `SSEHandler`，需要先通过 SSE 建立会话获取 session ID，然后才能调用工具。这导致：
1. Python 集成测试失败（需要复杂的 SSE 客户端逻辑）
2. 客户端使用复杂度增加
3. 不符合 MCP 规范推荐的 StreamableHTTP 模式
4. 缺少针对 StreamableHTTP 的单元测试

### 解决方案
MCP go-sdk 提供 `StreamableHTTPHandler`，支持：
- **Stateless 模式**：无需会话 ID，直接调用工具
- **JSONResponse 模式**：返回 JSON 而非 SSE 流
- 简化客户端调用流程

### 技术依据
```go
// StreamableHTTPOptions 配置
type StreamableHTTPOptions struct {
    Stateless    bool  // 无状态模式，无需 session ID
    JSONResponse bool  // 返回 JSON 而非 SSE
    // ...
}
```

---

## Work Objectives

### Core Objective
将 MCP 服务传输层从 SSE 迁移到 StreamableHTTP，添加完整测试覆盖。

### Concrete Deliverables
- `pkg/mcp/server.go` - 使用 `NewStreamableHTTPHandler`
- `pkg/mcp/server_test.go` - 新增 StreamableHTTP 单元测试
- `tests/integration/test_serialhub.py` - StreamableHTTP MCP 完整测试
- `AGENTS.md` - 更新架构说明
- `README.md` - 更新 API 使用示例

### Definition of Done
- [ ] `go build` 编译成功
- [ ] `go test ./...` 全部通过
- [ ] `pkg/mcp` 测试覆盖率 >= 70%
- [ ] Python 集成测试全部通过
- [ ] 手动验证 `curl` 可直接调用 MCP 工具
- [ ] `AGENTS.md` 更新完成

### Must Have
- StreamableHTTP 替换 SSE
- Stateless + JSONResponse 配置
- Go 单元测试覆盖 StreamableHTTP
- Python 集成测试完整 MCP 工具测试
- `AGENTS.md` 更新为 "MCP HTTP"

### Must NOT Have
- 破坏现有 MCP 工具功能
- 删除 /health 端点
- 删除 CORS 支持
- 降低测试覆盖率

---

## Verification Strategy

### Test Strategy
- **Go 单元测试**：`pkg/mcp/server_test.go` 新增 StreamableHTTP 测试
- **Python 集成测试**：完整的 MCP 工具测试（Stateless JSON 模式）
- **手动验证**：`curl` 直接调用工具

### QA Policy
每个任务包含 Agent-Executed QA Scenarios：
- **Backend/API**：Bash (curl) — 发送请求，验证响应格式和状态码

---

## Execution Strategy

### Sequential Tasks（顺序执行）

```
Task 1 → Task 2 → Task 3 → Task 4 → Task 5 → Verify → Commit
```

### Dependency Matrix
- **Task 2** 依赖 Task 1（代码修改后才能编写测试）
- **Task 3** 依赖 Task 2（单元测试通过后才能运行集成测试）
- **Task 4** 依赖 Task 3（测试通过后更新文档）
- **Task 5** 依赖 Task 4（最后更新 AGENTS.md）

---

## TODOs

- [x] 1. 修改 pkg/mcp/server.go 使用 StreamableHTTP

  **What to do**:
  - 替换 `NewSSEHandler` 为 `NewStreamableHTTPHandler`
  - 配置 `StreamableHTTPOptions{Stateless: true, JSONResponse: true}`
  - 更新日志消息（移除 "+SSE"）
  - 保持 `/mcp` 路径和 `/health` 端点不变

  **Must NOT do**:
  - 删除 CORS 中间件
  - 修改工具注册逻辑
  - 改变 HTTP 服务器启动方式

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单文件小改动，明确的 API 替换
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 2, 3, 4, 5
  - **Blocked By**: None

  **References**:
  - `pkg/mcp/server.go:141-167` - 当前 SSE 实现
  - MCP go-sdk: `NewStreamableHTTPHandler` API

  **Acceptance Criteria**:
  - [ ] 使用 `NewStreamableHTTPHandler`
  - [ ] 配置 Stateless 和 JSONResponse
  - [ ] `go build ./cmd/serialhub` 成功

  **QA Scenarios**:

  ```
  Scenario: 启动服务器并验证 MCP 端点返回 JSON
    Tool: Bash (curl)
    Preconditions: serialhub.exe 已构建
    Steps:
      1. .\bin\serialhub.exe --no-tray --mcp-port 55555
      2. 等待 2 秒
      3. curl -X POST http://127.0.0.1:55555/mcp -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_list"},"id":1}'
    Expected Result: 返回 JSON 格式（Content-Type: application/json），包含 result.content
    Failure Indicators: 返回 400 sessionid must be provided
    Evidence: .sisyphus/evidence/task-1-streamablehttp.txt
  ```

  **Commit**: YES
  - Message: `refactor(mcp): 从 SSE 迁移到 StreamableHTTP`
  - Files: `pkg/mcp/server.go`

---

- [x] 2. 添加 StreamableHTTP 单元测试

  **What to do**:
  - 在 `pkg/mcp/server_test.go` 添加 `TestStreamableHTTPHandler` 测试函数
  - 测试 Stateless 模式下直接调用工具（无需 session ID）
  - 测试 JSONResponse 模式返回 JSON 格式
  - 测试 `/health` 端点仍然工作
  - 更新已有测试中的 `TestNewMCPServer_串口管理器为空` 等

  **Must NOT do**:
  - 删除现有测试
  - 降低测试覆盖率
  - 修改被测代码

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 添加测试函数，模式明确
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 3, 4, 5
  - **Blocked By**: Task 1

  **References**:
  - `pkg/mcp/server_test.go` - 现有测试模式
  - `pkg/mcp/server.go` - StreamableHTTP 实现

  **Acceptance Criteria**:
  - [ ] 新增 `TestStreamableHTTPHandler` 测试
  - [ ] 测试 Stateless 直接调用
  - [ ] 测试 JSONResponse 格式
  - [ ] `go test ./pkg/mcp/...` 通过
  - [ ] 覆盖率 >= 70%

  **QA Scenarios**:

  ```
  Scenario: 运行 MCP 单元测试
    Tool: Bash (go test)
    Steps:
      1. go test ./pkg/mcp/... -v -cover
    Expected Result: 所有测试 PASS，覆盖率 >= 70%
    Evidence: .sisyphus/evidence/task-2-go-test-mcp.txt
  ```

  **Commit**: YES
  - Message: `test(mcp): 添加 StreamableHTTP 单元测试`
  - Files: `pkg/mcp/server_test.go`

---

- [x] 3. Python 集成测试添加完整 MCP 测试

  **What to do**:
  - 删除 `MCPSSEClient` 类（不再需要 SSE 逻辑）
  - 简化 `mcp_call()` 函数，直接 POST JSON 到 `/mcp`
  - 移除 session ID 相关代码
  - **新增** `TestMCPToolsStateless` 测试类，验证 Stateless JSON 模式
  - **新增** 测试：无 session ID 调用、JSON 响应格式、所有 6 个工具
  - 保留现有 `TestMCPTools` 测试（验证工具功能）

  **Must NOT do**:
  - 删除测试用例
  - 修改 fixture 逻辑（除了 SSE 相关）
  - 降低测试覆盖范围

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 简化 SSE 逻辑 + 添加新测试
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 4, 5
  - **Blocked By**: Task 2

  **References**:
  - `tests/integration/test_serialhub.py:27-89` - 当前 SSE 客户端实现
  - `tests/integration/test_serialhub.py::TestMCPTools` - 现有 MCP 测试

  **Acceptance Criteria**:
  - [ ] 删除 `MCPSSEClient` 类
  - [ ] `mcp_call()` 直接 POST JSON
  - [ ] 新增 `TestMCPToolsStateless` 测试类
  - [ ] 测试 JSON 响应格式
  - [ ] 测试所有 6 个工具在 Stateless 模式下工作
  - [ ] pytest 全部通过

  **QA Scenarios**:

  ```
  Scenario: 运行 Python MCP 集成测试
    Tool: Bash (pytest)
    Preconditions: serialhub.exe 已构建，Task 2 完成
    Steps:
      1. $env:SERIALHUB_INTEGRATION_TEST = "1"
      2. pytest tests/integration/test_serialhub.py::TestMCPTools -v
      3. pytest tests/integration/test_serialhub.py::TestMCPToolsStateless -v
    Expected Result: 所有 MCP 测试通过
    Failure Indicators: AssertionError 或 ConnectionError
    Evidence: .sisyphus/evidence/task-3-pytest-mcp.txt
  ```

  **Commit**: YES
  - Message: `test(integration): 添加 StreamableHTTP MCP 完整测试`
  - Files: `tests/integration/test_serialhub.py`

---

- [x] 4. 更新 README.md 文档

  **What to do**:
  - 更新 `README.md` 中 MCP HTTP API 调用示例
  - 移除 SSE 相关说明
  - 添加 StreamableHTTP Stateless JSON 模式说明
  - 更新 curl 示例（展示直接调用，无 session ID）
  - 更新 `tests/integration/README.md` 说明

  **Must NOT do**:
  - 删除工具列表文档
  - 修改配置说明
  - 改变项目简介

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 文档更新，替换示例
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 5
  - **Blocked By**: Task 3

  **References**:
  - `README.md` - MCP HTTP API 调用章节
  - `tests/integration/README.md` - 测试说明

  **Acceptance Criteria**:
  - [ ] README 中 curl 示例正确（无 session ID）
  - [ ] 无 SSE 相关说明
  - [ ] 说明 Stateless JSON 模式
  - [ ] 更新架构图（MCP HTTP 而非 MCP HTTP+SSE）

  **QA Scenarios**:

  ```
  Scenario: 验证文档示例可执行
    Tool: Bash (curl)
    Preconditions: serialhub 运行中
    Steps:
      1. 按 README 中的 curl 示例执行
      2. 验证返回 JSON 格式
    Expected Result: 与文档描述一致
    Evidence: .sisyphus/evidence/task-4-docs.txt
  ```

  **Commit**: YES
  - Message: `docs: 更新 MCP API 文档为 StreamableHTTP`
  - Files: `README.md`, `tests/integration/README.md`

---

- [x] 5. 更新 AGENTS.md

  **What to do**:
  - 第 9 行：`MCP HTTP+SSE` → `MCP HTTP (StreamableHTTP)`
  - 第 44 行：`pkg/mcp/server.go # MCP 服务（直接使用 SDK SSEHandler）` → `pkg/mcp/server.go # MCP 服务（StreamableHTTP，Stateless JSON 模式）`
  - 确保与代码实现一致

  **Must NOT do**:
  - 删除其他章节
  - 修改代码风格规范
  - 改变 Git 提交规范

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单行文本更新
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Final Verification
  - **Blocked By**: Task 4

  **References**:
  - `AGENTS.md:9` - 项目概述
  - `AGENTS.md:44` - 文件结构

  **Acceptance Criteria**:
  - [ ] 第 9 行更新为 `MCP HTTP`
  - [ ] 第 44 行更新说明 StreamableHTTP
  - [ ] 与代码实现一致

  **QA Scenarios**:

  ```
  Scenario: 验证 AGENTS.md 与代码一致
    Tool: Read
    Steps:
      1. 读取 AGENTS.md 第 9 行和第 44 行
      2. 验证无 "SSE" 字样
      3. 验证包含 "StreamableHTTP" 或 "HTTP"
    Expected Result: 文档与实现一致
    Evidence: .sisyphus/evidence/task-5-agents.md
  ```

  **Commit**: YES
  - Message: `docs(agents): 更新 MCP 架构说明为 StreamableHTTP`
  - Files: `AGENTS.md`

---

## Final Verification Wave (MANDATORY)

- [ ] F1. **Plan Compliance Audit** — `oracle`
  验证所有 "Must Have" 实现，所有 "Must NOT Have" 避免。

- [ ] F2. **Code Quality Review** — `unspecified-high`
  运行 `go test ./...` + `go build` + `pytest`。

- [ ] F3. **Test Coverage Check** — `unspecified-high`
  验证 `pkg/mcp` 覆盖率 >= 70%，Python 测试全部通过。

- [ ] F4. **Documentation Consistency** — `deep`
  验证 `AGENTS.md`、`README.md` 与代码实现一致。

---

## Commit Strategy

- **Task 1**: `refactor(mcp): 从 SSE 迁移到 StreamableHTTP` — `pkg/mcp/server.go`
- **Task 2**: `test(mcp): 添加 StreamableHTTP 单元测试` — `pkg/mcp/server_test.go`
- **Task 3**: `test(integration): 添加 StreamableHTTP MCP 完整测试` — `tests/integration/test_serialhub.py`
- **Task 4**: `docs: 更新 MCP API 文档为 StreamableHTTP` — `README.md`, `tests/integration/README.md`
- **Task 5**: `docs(agents): 更新 MCP 架构说明为 StreamableHTTP` — `AGENTS.md`

---

## Success Criteria

### Verification Commands
```bash
go test ./...                           # 所有 Go 测试通过
go test ./pkg/mcp/... -cover            # MCP 覆盖率 >= 70%
go build -o bin/serialhub.exe ./cmd/serialhub  # 构建成功
pytest tests/integration/test_serialhub.py -v  # Python 测试通过
```

### Final Checklist
- [ ] 所有 "Must Have" 实现
- [ ] 所有 "Must NOT Have" 避免
- [ ] 所有测试通过
- [ ] MCP 覆盖率 >= 70%
- [ ] 文档更新完整
- [ ] AGENTS.md 更新
- [ ] Git 提交完成