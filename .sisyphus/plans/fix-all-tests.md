# 完善所有单元测试、集成测试并修正错误

## TL;DR

> **问题**：Go 测试 `TestLoadConfig_WithConfigFile` 失败，Python E2E 测试存在 fixture 问题
> **方案**：
> 1. 修复 `loadConfig()` 中 `mcpPort` 覆盖逻辑（添加 `mcpPort != 0` 检查）
> 2. 修复 Python E2E 测试的 `serialhub_server` fixture（移除 `--telnet-port` 参数）
> 3. 确保所有测试通过（0 FAIL，0 SKIP）
>
> **改动范围**：2-3 个文件
> **预估工作量**：Quick
> **并行执行**：NO

---

## Context

### 当前问题

**Go 测试失败**（1 个）：
- `TestLoadConfig_WithConfigFile` — 期望 6000，但得到 0
- **根因**：`loadConfig()` 中 `if mcpPort != 5000` 在测试时 mcpPort=0，导致配置文件中的 6000 被覆盖为 0

**Python E2E 测试问题**（多个）：
- `serialhub_server` fixture 使用 `--telnet-port` 参数，但该参数已不存在
- 导致服务启动失败，所有依赖此 fixture 的测试 ERROR

### 已完成的修复
- Task 1-3: 修复了 /terminal 路由冲突
- Task 4-5: 新增了 Go 和 Python 测试

---

## Work Objectives

### Core Objective
修复所有测试失败，确保 Go 测试和 Python E2E 测试全部通过。

### Concrete Deliverables
1. 修复 `cmd/serialhub/main.go` 中的 `loadConfig()` 函数
2. 修复 `tests/integration/test_serialhub.py` 中的 `serialhub_server` fixture
3. 验证所有测试通过

### Definition of Done
- [ ] `go test ./...` 全部 PASS，0 FAIL
- [ ] `pytest tests/integration/test_serialhub.py -v` 全部 PASS，0 FAIL，0 SKIP
- [ ] 无回归问题

### Must NOT Have
- 不修改业务逻辑
- 不修改功能代码（除 loadConfig 外）
- 不添加新功能

---

## Verification Strategy

> **零人工干预** — 所有验证由 agent 执行。

### Test Decision
- **Go 测试**: 修复后立即运行
- **Python 测试**: 修复 fixture 后运行

---

## Execution Strategy

### 顺序执行

```
Task 1: 修复 loadConfig() mcpPort 逻辑
    ↓
Task 2: 修复 Python fixture
    ↓
Task 3: 验证所有测试通过
```

---

## TODOs

- [ ] 1. 修复 loadConfig() 中的 mcpPort 覆盖逻辑

  **What to do**:
  - 修改 `cmd/serialhub/main.go` 第 348 行
  - 将 `if mcpPort != 5000` 改为 `if mcpPort != 5000 && mcpPort != 0`
  - 这样当 mcpPort 为 0（未初始化）时不会覆盖配置文件中的值

  **Code change**:
  ```go
  // Before:
  if mcpPort != 5000 {
      cfg.MCP.HTTPPort = mcpPort
  }
  
  // After:
  if mcpPort != 5000 && mcpPort != 0 {
      cfg.MCP.HTTPPort = mcpPort
  }
  ```

  **Acceptance Criteria**:
  - `go test ./cmd/serialhub/... -run TestLoadConfig_WithConfigFile` PASS
  - `go test ./cmd/serialhub/...` 全部 PASS

  **Commit**: YES
  - Message: `fix(config): 修复 mcpPort 覆盖配置文件逻辑`
  - Files: `cmd/serialhub/main.go`

- [ ] 2. 修复 Python E2E 测试的 serialhub_server fixture

  **What to do**:
  - 修改 `tests/integration/test_serialhub.py`
  - 找到 `serialhub_server` fixture（约 line 200-300）
  - 移除 `--telnet-port` 参数（Telnet 服务已删除）
  - 确保 fixture 使用正确的参数启动 SerialHub

  **Acceptance Criteria**:
  - `pytest tests/integration/test_serialhub.py::TestCLI -v` PASS
  - `pytest tests/integration/test_serialhub.py -v -k "not hardware"` PASS

  **Commit**: YES（与 Task 1 合并）
  - Message: `fix(config): 修复 mcpPort 覆盖配置文件逻辑`
  - Files: `cmd/serialhub/main.go`, `tests/integration/test_serialhub.py`

- [ ] 3. 全量测试验证

  **What to do**:
  - 运行 `go test ./...` 确认全部 PASS
  - 运行 `pytest tests/integration/test_serialhub.py -v` 确认全部 PASS

  **Acceptance Criteria**:
  - Go 测试: 0 FAIL
  - Python 测试: 0 FAIL, 0 SKIP

---

## Final Verification Wave

- [ ] F1. **Go 全量测试** — `quick`
  运行 `go test ./...`，确认 0 FAIL。

- [ ] F2. **Python E2E 测试** — `quick`
  运行 `pytest tests/integration/test_serialhub.py -v`，确认 0 FAIL, 0 SKIP。

---

## Commit Strategy

- **单次提交**: `fix(config): 修复 mcpPort 覆盖配置文件逻辑，修复 Python fixture`
  - Files: `cmd/serialhub/main.go`, `tests/integration/test_serialhub.py`
  - Pre-commit: `go test ./... && pytest tests/integration/test_serialhub.py -v`

---

## Success Criteria

### Verification Commands
```bash
go test ./...                                                    # Expected: 全部 PASS
pytest tests/integration/test_serialhub.py -v                    # Expected: 全部 PASS
```

### Final Checklist
- [ ] Go 测试全部通过
- [ ] Python E2E 测试全部通过
- [ ] 无回归问题
