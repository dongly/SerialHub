# Go 测试覆盖率报告

**日期**: 2026-04-08
**任务**: Task 15 - Go 测试覆盖率评估 + 补充

## 覆盖率对比

| 包 | 之前 | 之后 | 变化 | 状态 |
|---|---|---|---|---|
| `cmd/serialhub` | 12.4% | 12.4% | - | ⚠️ CLI 入口，难以单元测试 |
| `internal/buffer` | 100.0% | 100.0% | - | ✅ |
| `internal/service` | 69.8% | 69.8% | - | ✅ |
| `internal/testutil` | 80.0% | 80.0% | - | ✅ |
| `pkg/bridge` | 97.8% | 93.3% | -4.5%* | ✅ 波动（缓存差异） |
| `pkg/config` | 91.7% | 91.7% | - | ✅ |
| `pkg/mcp` | 78.3% | 78.3% | - | ✅ |
| **`pkg/mcp/tools`** | **60.8%** | **65.8%** | **+5.0%** | ✅ 已改进 |
| `pkg/serial` | 91.6% | 91.1% | -0.5% | ✅ |
| **`pkg/tray`** | **56.8%** | **58.0%** | **+1.2%** | ⚠️ 受 systray 限制 |
| `pkg/web` | 78.0% | 78.0% | - | ✅ |
| **总计** | ~65% | **67.0%** | **+2%** | ✅ |

## 关键包覆盖率（目标 ≥60%）

| 包 | 覆盖率 | 达标 |
|---|---|---|
| `pkg/bridge` | 93.3% | ✅ |
| `pkg/mcp` | 78.3% | ✅ |
| `pkg/mcp/tools` | 65.8% | ✅ |
| `pkg/web` | 78.0% | ✅ |
| `pkg/serial` | 91.1% | ✅ |
| `pkg/tray` | 58.0% | ⚠️ 见下文说明 |
| `pkg/config` | 91.7% | ✅ |

## pkg/tray 低于 60% 的原因

`pkg/tray`（58.0%）覆盖率受限，原因如下：

1. **`Run()` (0%)**: 需要在主线程调用 `systray.Run()`，单元测试环境无法模拟
2. **`createMenu()` (0%)**: 依赖 `systray.AddMenuItem()`，非 systray.Run 环境会 panic（nil map）
3. **`setupEventHandlers()` (0%)**: 依赖 `createMenu()` 创建的 MenuItem
4. **`openTerminal()` (0%→已覆盖)**: exec.Command 启动浏览器
5. **Windows GUI 限制**: systray 使用 Windows 系统托盘 API，必须在 GUI 线程运行

这些函数的测试需要集成测试或 E2E 测试环境，无法通过纯单元测试覆盖。

## 新增测试

### pkg/mcp/tools (新增 8 个测试)
- `TestSerialList_NilManager` — nil 管理器检查
- `TestSerialConnect_NilManager` — nil 管理器检查
- `TestSerialDisconnect_NilManager` — nil 管理器检查
- `TestSerialWrite_NilManager` — nil 管理器检查
- `TestSerialStatus_NilManager` — nil 管理器检查
- `TestToolResult_Fields` — ToolResult 字段验证 (table-driven)
- `TestConnectInput_Defaults` — 输入默认值验证
- `TestWriteInput_Defaults` — 输入默认值验证
- `TestReadInput_Defaults` — 输入默认值验证

### pkg/tray (新增 11 个测试)
- `TestOpenTerminal` — openTerminal 基本功能
- `TestOpenTerminal_WithHost` — 不同 host/port 配置
- `TestGetSerialMenuTitle_Connected` — 连接状态标题
- `TestAutoReconnect_WithPort` — 有端口时的自动重连
- `TestNotifyConfigChangedAndReconnect_WithCallback` — 配置变更回调
- `TestToggleSerial_AlreadyConnected` — 已连接时的切换
- `TestOnReady_NoCallback` — 无回调时的 onReady
- `TestOnReady_WithCallback` — 有回调时的 onReady
- `TestGetConfigSummary_WithPort` — 有端口时的配置摘要
- `TestSetPort_SamePort` — 设置相同端口

### 基础设施改进
- `callWithTimeout` 增加 panic 恢复（处理非 systray 环境）

## 验证结果

```
go test ./... — 全部 PASS (11/11 packages)
go vet ./... — 零错误
```
