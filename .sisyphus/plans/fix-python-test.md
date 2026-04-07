# 修复 Python E2E 测试 - 移除 --telnet-port 检查

## TL;DR

> **问题**: `test_help_flags` 测试期望 `--telnet-port` 存在于帮助输出中，但 Telnet 服务已删除
> **方案**: 从测试的 flag 列表中移除 `--telnet-port`
>
> **改动范围**: 1 个文件，1 行变更
> **预估工作量**: Quick

---

## Context

### 当前问题
Python E2E 测试 `TestCLI.test_help_flags` 失败：
```
AssertionError: 缺少 --telnet-port
```

这是因为 Telnet 服务已被删除，`--telnet-port` 参数不再存在，但测试仍在检查它。

### 已完成的修复
- Go 测试已全部通过（mcpPort 逻辑已修复）
- Python 测试语法正常

---

## Work Objectives

### Core Objective
修复 `test_help_flags` 测试，移除对 `--telnet-port` 的检查。

### Concrete Deliverables
- 修改 `tests/integration/test_serialhub.py` 第 378 行
- 从 flag 列表中移除 `"--telnet-port"`

### Definition of Done
- [ ] `pytest tests/integration/test_serialhub.py::TestCLI::test_help_flags` PASS
- [ ] `pytest tests/integration/test_serialhub.py::TestCLI -v` 全部 PASS

---

## Execution Strategy

### 单任务执行

```
Task 1: 修复 test_help_flags 测试
```

---

## TODOs

- [x] 1. 修复 test_help_flags 测试

  **What to do**:
  - 修改 `tests/integration/test_serialhub.py` 第 375-385 行
  - 从 flag 列表中移除 `"--telnet-port"`（第 378 行）

  **Code change**:
  ```python
  # Before:
  for flag in [
      "--serial-port",
      "--baud-rate",
      "--telnet-port",  # <-- 移除这一行
      "--mcp-port",
      "--config",
      "--debug",
      "--no-tray",
      "--minimized",
  ]:

  # After:
  for flag in [
      "--serial-port",
      "--baud-rate",
      "--mcp-port",
      "--config",
      "--debug",
      "--no-tray",
      "--minimized",
  ]:
  ```

  **Acceptance Criteria**:
  1. `pytest tests/integration/test_serialhub.py::TestCLI::test_help_flags` PASS
  2. `pytest tests/integration/test_serialhub.py::TestCLI -v` 全部 PASS

  **Commit**: YES
  - Message: `test: 移除 --telnet-port 检查（Telnet 已删除）`
  - Files: `tests/integration/test_serialhub.py`

---

## Final Verification

- [x] F1. **Python CLI 测试** — `quick`
  运行 `pytest tests/integration/test_serialhub.py::TestCLI -v`，确认全部 PASS。

---

## Commit Strategy

- **单次提交**: `test: 移除 --telnet-port 检查（Telnet 已删除）`
  - Files: `tests/integration/test_serialhub.py`
  - Pre-commit: `pytest tests/integration/test_serialhub.py::TestCLI -v`

---

## Success Criteria

### Verification Commands
```bash
pytest tests/integration/test_serialhub.py::TestCLI -v  # Expected: 全部 PASS
```

### Final Checklist
- [x] test_help_flags PASS
- [x] 所有 TestCLI 测试 PASS
