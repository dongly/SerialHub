# 托盘菜单功能增强

## TL;DR

> **Quick Summary**: 增强 SerialHub 系统托盘菜单，添加完整串口参数配置、网络状态显示、后台运行模式和日志窗口功能。
> 
> **Deliverables**:
> - 串口选择子菜单（刷新 + 串口列表）
> - 串口参数子菜单（波特率/数据位/停止位/校验位 + 配置摘要）
> - 网络状态显示（带 IP 地址）
> - 后台运行模式（自动隐藏控制台）
> - 日志显示窗口
> 
> **Estimated Effort**: Medium
> **Parallel Execution**: YES - 4 Waves
> **Critical Path**: Wave 1 → Wave 2 → Wave 3 → Wave 4 → Verification

---

## Context

### Original Request
用户要求增强托盘菜单功能：
1. 列出所有可用串口
2. 可以配置串口参数（波特率/数据位/停止位/校验位）
3. 可配置 telnet/mcp 网络端口号
4. 可隐藏/显示主程序终端 → **改为显示日志**

### Current State
- `pkg/tray/tray.go` - 托盘管理器，已实现基本菜单
  - **CRITICAL BUG**: `QuitChan()` 未实现，但 `main.go` 调用它 → 编译失败
  - **CRITICAL BUG**: `getIcon()` 返回空字节 → 图标不显示
  - **CRITICAL BUG**: `embed` 指令引用 `.png` 而非 `.ico`
- `pkg/tray/console_windows.go` - 控制台隐藏/显示功能（**不存在，需创建**）
- `pkg/tray/assets/*.ico` - ICO 图标文件已存在
- `pkg/serial/manager.go` - `ListPorts()` 方法已存在
- `pkg/config/config.go` - 配置结构体已包含 DataBits/Parity/StopBits

### Technical Constraints
- Windows 平台
- systray 库子菜单 API：`AddSubMenuItem()`
- 控制台操作需要 Windows API (`user32.dll`, `ShowWindow`)
- TrayManager 需添加字段：`host`, `quitChan`, `consoleHid`

### Metis Review Findings (CRITICAL)

| Finding | Impact | Action |
|---------|--------|--------|
| `QuitChan()` 缺失 | 编译失败 | Wave 0 Task 0 |
| `getIcon()` 返回空 | 图标不显示 | Wave 0 Task 0b |
| `embed` 指令错误 | 图标加载失败 | Wave 0 Task 0b |
| 缺少 `host` 字段 | 无法显示 IP | Wave 3 Task 8 |
| QA 场景不够具体 | 验证困难 | 已更新 Final Verification |

---

## Work Objectives

### Core Objective
实现功能完整的托盘菜单，支持完整串口参数配置、网络状态显示、后台运行模式和日志窗口。

### Concrete Deliverables
1. **串口选择子菜单** - 刷新列表 + 可用串口列表，点击选择
2. **串口参数子菜单** - 波特率/数据位/停止位/校验位三级子菜单 + 配置摘要显示
3. **网络状态显示** - 显示监听 IP + Telnet/MCP 端口
4. **后台运行模式** - 启动后自动隐藏控制台窗口
5. **日志显示窗口** - 点击菜单打开独立日志窗口

### Definition of Done
- [ ] 托盘菜单包含所有新功能
- [ ] 选择串口后自动更新配置
- [ ] 串口参数可配置（波特率/数据位/停止位/校验位）
- [ ] 网络状态显示带 IP 地址
- [ ] 控制台自动隐藏（后台运行）
- [ ] 日志窗口可显示
- [ ] 所有测试通过

### Must Have
- 串口列表子菜单
- 串口参数子菜单（含数据位/停止位/校验位）
- 网络状态显示带 IP
- 后台运行模式
- 日志显示功能

### Must NOT Have
- 修改网络端口（需要重启服务，复杂度高）
- 保存配置到文件（当前会话有效）
- 修改串口参数后自动重连（需用户手动重连）

---

## 托盘菜单结构（优化版）

```
SerialHub 托盘菜单
├── ✅ 断开 COM9          (已连接时显示，点击断开)
│   或 ⭕ 连接 COM9       (未连接时显示，点击连接)
│
├── 📡 选择串口 ▶         (子菜单)
│   ├── 🔄 刷新列表
│   ├── ──────────────
│   ├── ✓ COM9           (当前选中的串口带 ✓)
│   ├──   COM3
│   ├──   COM4
│   └──   ...
│
├── ⚙️ 串口参数 ▶         (子菜单 - 合并所有串口配置)
│   ├── ⚡ 波特率 ▶
│   │   ├──   9600
│   │   ├──   19200
│   │   ├──   38400
│   │   ├──   57600
│   │   ├── ✓ 115200     (当前波特率带 ✓)
│   │   └──   230400
│   │
│   ├── 📊 数据位 ▶
│   │   ├──   5
│   │   ├──   6
│   │   ├──   7
│   │   └── ✓ 8          (当前数据位带 ✓)
│   │
│   ├── 📏 停止位 ▶
│   │   ├── ✓ 1          (当前停止位带 ✓)
│   │   ├──   1.5
│   │   └──   2
│   │
│   ├── 🔒 校验位 ▶
│   │   ├── ✓ None       (当前校验位带 ✓)
│   │   ├──   Even
│   │   └──   Odd
│   │
│   └── 📋 当前: 115200 8N1  (禁用，显示当前配置摘要)
│
├── ─────────────────────
│
├── 🌐 网络状态           (禁用，仅显示)
│   └── 127.0.0.1 | Telnet:2323 | MCP:5000
│
├── ─────────────────────
│
├── 📋 显示日志           (点击打开日志窗口)
│
├── ─────────────────────
│
├── 📋 版本 v0.1.0        (禁用，仅显示)
│
├── ─────────────────────
│
└── ❌ 退出               (退出整个程序)
```

### 菜单层级说明

| 层级 | 菜单项 | 功能 |
|------|--------|------|
| L1 | 连接/断开 | 切换串口连接状态，图标颜色同步变化 |
| L1 | 选择串口 | 子菜单：刷新 + 串口列表，当前选中带 ✓ |
| L1 | 串口参数 | 子菜单：波特率/数据位/停止位/校验位 + 配置摘要 |
| L2 | 波特率 | 选择波特率，修改后需重连生效 |
| L2 | 数据位 | 选择数据位（5/6/7/8） |
| L2 | 停止位 | 选择停止位（1/1.5/2） |
| L2 | 校验位 | 选择校验位（None/Even/Odd） |
| L2 | 当前配置 | 显示当前参数摘要（如 115200 8N1） |
| L1 | 网络状态 | 显示监听 IP + 端口，禁用不可点击 |
| L1 | 显示日志 | 打开独立日志窗口 |
| L1 | 版本 | 显示版本号，禁用 |
| L1 | 退出 | 关闭托盘 + 主程序 |

### 托盘图标状态

| 状态 | 图标颜色 | Tooltip |
|------|----------|---------|
| Idle (未连接) | 灰色 | SerialHub - 未连接 |
| Connected (已连接) | 绿色 | SerialHub - COM9 @ 115200 8N1 |
| Error (错误) | 红色 | SerialHub - 连接错误: [错误信息] |

### 主程序运行模式

程序启动后自动进入**后台运行模式**：
- 控制台窗口自动隐藏（启动后立即隐藏）
- 仅通过托盘图标与用户交互
- 日志通过独立窗口显示（点击"显示日志"菜单）
- 双击托盘图标 = 显示日志窗口

### 串口配置参数

| 参数 | 可选值 | 默认值 | 说明 |
|------|--------|--------|------|
| 波特率 | 9600, 19200, 38400, 57600, 115200, 230400 | 115200 | 数据传输速率 |
| 数据位 | 5, 6, 7, 8 | 8 | 每帧数据位数 |
| 停止位 | 1, 1.5, 2 | 1 | 帧结束标志位数 |
| 校验位 | None, Even, Odd | None | 奇偶校验方式 |

**配置格式**: 波特率 + 数据位 + 校验位 + 停止位，如 `115200 8N1`

---

## TODOs

### Wave 0: 修复现有编译错误 (CRITICAL - 必须先完成)

- [x] 0. 修复 `pkg/tray/tray.go` - QuitChan() 未实现

  **What to do**:
  - 在 `TrayManager` 结构体添加 `quitChan chan struct{}` 字段
  - 在 `NewTrayManager()` 初始化 `quitChan = make(chan struct{})`
  - 添加 `QuitChan() <-chan struct{}` 方法返回 quitChan
  - 在 `onExit()` 中关闭 quitChan：`close(t.quitChan)`
  
  **现状**: `main.go:140` 调用 `trayMgr.QuitChan()`，但 TrayManager 无此方法 → **编译失败**
  
  **References**:
  - `cmd/serialhub/main.go:140` - 调用 QuitChan()
  - `pkg/tray/tray.go:26-33` - TrayManager 结构体（需添加 quitChan）

- [x] 0b. 修复 `pkg/tray/tray.go` - getIcon() 返回空

  **What to do**:
  - 修复 `//go:embed assets/*.ico` 指令（当前是 `.png`）
  - 实现 `getIcon()` 从 `iconFS.ReadFile()` 加载 ICO 文件
  - 根据状态加载: `tray-idle.ico`, `tray-connected.ico`, `tray-error.ico`
  
  **现状**: `getIcon()` 返回 `[]byte{}` → **图标不显示**
  
  **References**:
  - `pkg/tray/tray.go:35` - embed 指令（需改为 `*.ico`）
  - `pkg/tray/tray.go:119-124` - getIcon() 实现（需加载实际文件）
  - `pkg/tray/assets/*.ico` - ICO 文件已存在

### Wave 1: 基础结构 + 串口菜单

- [x] 1. 创建 `pkg/tray/console_windows.go` - 控制台控制功能

  **What to do**:
  - 实现 `HideConsole()` 函数 - 启动时自动隐藏控制台
  - 实现 `ShowConsole()` 函数 - 用于调试时显示
  - 使用 Windows API: `GetConsoleWindow()`, `ShowWindow(SW_HIDE/SW_SHOW)`
  
  **References**:
  - Windows API: `user32.dll` - ShowWindow
  - `syscall.GetConsoleWindow()` - 获取控制台窗口句柄

- [x] 2. 更新 `pkg/tray/tray.go` - 添加串口选择子菜单

  **What to do**:
  - 调用 `serial.ListPorts()` 获取可用串口列表
  - 使用 `systray.AddSubMenuItem()` 创建"选择串口"子菜单
  - 添加"刷新列表"菜单项
  - 动态填充串口列表，当前选中项带 ✓ 标记
  - 点击串口项时更新配置
  
  **References**:
  - `pkg/serial/manager.go:138` - `ListPorts()` 方法
  - `systray.AddSubMenuItem()` - 子菜单 API

### Wave 2: 串口参数子菜单

- [x] 3. 添加"串口参数"子菜单框架

  **What to do**:
  - 创建二级子菜单"串口参数"
  - 内含三级子菜单: 波特率、数据位、停止位、校验位
  - 添加"当前配置"禁用菜单项显示摘要（如 115200 8N1）
  
  **References**:
  - `systray.AddSubMenuItem()` - 多级子菜单
  - `pkg/config/config.go:SerialConfig` - 配置结构体

- [x] 4. 添加波特率三级子菜单

  **What to do**:
  - 固定选项: 9600, 19200, 38400, 57600, 115200, 230400
  - 当前波特率带 ✓ 标记
  - 点击后更新 `config.Serial.BaudRate`
  - 已连接时提示"需重新连接生效"

- [x] 5. 添加数据位三级子菜单

  **What to do**:
  - 固定选项: 5, 6, 7, 8
  - 当前数据位带 ✓ 标记
  - 点击后更新 `config.Serial.DataBits`

- [x] 6. 添加停止位三级子菜单

  **What to do**:
  - 固定选项: 1, 1.5, 2
  - 当前停止位带 ✓ 标记
  - 点击后更新 `config.Serial.StopBits`

- [x] 7. 添加校验位三级子菜单

  **What to do**:
  - 固定选项: None, Even, Odd
  - 当前校验位带 ✓ 标记
  - 点击后更新 `config.Serial.Parity`

### Wave 3: 网络状态 + 日志窗口

- [x] 8. 添加网络状态菜单项

  **What to do**:
  - 显示格式: `127.0.0.1 | Telnet:2323 | MCP:5000`
  - 获取主机 IP 地址（config.Host 或默认 127.0.0.1）
  - 菜单项禁用，仅供查看
  
  **References**:
  - `pkg/config/config.go` - Host, Telnet.Port, MCP.HTTPPort
  - `systray.AddMenuItem()` - 禁用菜单项

- [x] 9. 添加日志显示窗口功能

  **What to do**:
  - 创建独立日志窗口（或使用控制台窗口显示日志）
  - 菜单项"显示日志"，点击打开日志窗口
  - 日志窗口实时显示 logrus 输出
  - 双击托盘图标触发显示日志
  
  **References**:
  - `github.com/sirupsen/logrus` - 日志库
  - Windows GUI 或简单的控制台显示

### Wave 4: 主程序后台运行 + 集成

- [x] 10. 实现主程序后台运行模式

  **What to do**:
  - `cmd/serialhub/main.go` 启动后立即调用 `HideConsole()`
  - 托盘初始化完成后进入后台运行
  - 退出时清理所有资源
  
  **References**:
  - `pkg/tray/console_windows.go` - HideConsole()
  - `cmd/serialhub/main.go` - 主程序入口

- [x] 11. 整合所有菜单项并测试

  **What to do**:
  - 验证菜单层级结构正确
  - 验证所有配置项可点击选择
  - 验证 ✓ 标记正确更新
  - 验证连接/断开状态图标变化

- [x] 12. 更新单元测试

  **What to do**:
  - 测试新增的串口参数菜单函数
  - 测试控制台隐藏/显示函数
  - 测试配置更新逻辑

---

## Final Verification Wave

- [x] F1. **菜单结构验证** - 运行程序，右键托盘图标，验证所有菜单项按层级显示
  - Tool: 人工交互 + screenshot
  - Steps: 运行 `serialhub.exe`，右键托盘图标，截图验证菜单结构
  - Expected: 菜单层级：连接/断开 → 选择串口(子菜单) → 串口参数(子菜单) → 网络状态 → 显示日志 → 版本 → 退出
  - Evidence: `.sisyphus/evidence/f1-menu-structure.png`

- [x] F2. **串口参数配置验证** - 选择不同波特率/数据位/停止位/校验位，验证 ✓ 标记和配置摘要更新
  - Tool: 人工交互 + screenshot
  - Steps:
    1. 点击"串口参数" → "波特率" → 选择 "9600"，观察 ✓ 标记变化
    2. 点击"串口参数" → "数据位" → 选择 "7"，观察 ✓ 标记变化
    3. 观察"当前配置"菜单项显示 "9600 7N1"
  - Expected: ✓ 标记正确移动，配置摘要正确显示
  - Evidence: `.sisyphus/evidence/f2-serial-config.png`

- [x] F3. **后台运行验证** - 程序启动后控制台自动隐藏，托盘正常工作
  - Tool: 人工观察
  - Steps: 运行 `serialhub.exe`，观察控制台窗口是否立即隐藏
  - Expected: 控制台窗口在托盘启动后自动隐藏，仅托盘图标可见
  - Evidence: `.sisyphus/evidence/f3-background-run.png`

- [x] F4. **日志窗口验证** - 点击"显示日志"菜单，日志窗口正常显示
  - Tool: 人工交互 + screenshot
  - Steps: 右键托盘图标 → 点击"显示日志"，观察是否弹出日志窗口
  - Expected: 日志窗口显示，内容包含 `[SerialHub]` 日志输出
  - Evidence: `.sisyphus/evidence/f4-log-window.png`

- [x] F5. **网络状态验证** - 验证 IP 地址正确显示
  - Tool: 人工观察
  - Steps: 右键托盘图标，观察"网络状态"菜单项
  - Expected: 显示格式为 `127.0.0.1 | Telnet:2323 | MCP:5000`（或实际 IP）
  - Evidence: `.sisyphus/evidence/f5-network-status.png`

- [x] F6. **编译验证** - 确保所有代码编译通过
  - Tool: Bash
  - Steps: `go build ./cmd/serialhub && go build ./...`
  - Expected: 编译成功，无错误
  - Evidence: `.sisyphus/evidence/f6-build.log`

- [x] F7. **测试验证** - 确保所有测试通过
  - Tool: Bash
  - Steps: `go test ./pkg/tray/...`
  - Expected: 所有测试 PASS
  - Evidence: `.sisyphus/evidence/f7-test.log`

---

## Known Issues (审查发现)

### CRITICAL - 必须修复才能编译/运行

| Issue | Status | Fix |
|-------|--------|-----|
| `QuitChan()` 未实现 | 待修复 | Wave 0 Task 0 |
| `getIcon()` 返回空字节 | 待修复 | Wave 0 Task 0b |
| `embed` 指令引用 `.png` 而非 `.ico` | 待修复 | Wave 0 Task 0b |

### MINOR - 需要在实现中添加

| Issue | Status | Fix |
|-------|--------|-----|
| `TrayManager` 缺少 `host` 字段 | 待添加 | Wave 3 Task 8 |
| `TrayManager` 缺少 `consoleHid` 字段 | 待添加 | Wave 4 Task 10 |
| 缺少配置更新回调机制 | 待添加 | Wave 2 Task 3-7 |

---

## Success Criteria

### Verification Commands
```bash
go build ./cmd/serialhub
go test ./pkg/tray/...
.\serialhub.exe  # 运行后验证托盘菜单
```

### Final Checklist
- [ ] 托盘菜单层级结构正确
- [ ] 串口选择子菜单可用
- [ ] 串口参数子菜单可用（波特率/数据位/停止位/校验位）
- [ ] 网络状态显示带 IP 地址
- [ ] 控制台自动隐藏（后台运行）
- [ ] 日志窗口可显示
- [ ] 所有测试通过