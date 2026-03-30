# SerialHub - AI 代理指南

## 项目概述
SerialHub 是一个串口（MCU）与网络连接（Telnet/AI）之间的双向桥接器。

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

| 组件 | 技术 |
|------|------|
| 运行时 | Node.js (tsx) |
| 语言 | TypeScript (ES2022, strict, ESM) |
| 串口 | serialport@13 |
| AI 接口 | MCP (@modelcontextprotocol/sdk) |
| 验证 | Zod |
| 系统托盘 | tray-hook (Rust daemon) + koffi (FFI) |
| 测试 | Bun test |
| 代码规范 | ESLint |

## 构建 / 检查 / 测试命令

```bash
npm run typecheck          # TypeScript 类型检查（零错误）
npm run lint               # ESLint 检查 src/（零警告）
npm test                   # 运行所有测试
npm run build              # 编译到 dist/
npm run serve              # 启动开发服务器

# 运行单个测试文件
bun test tests/serial.test.ts
bun test tests/mcp.test.ts

# 运行匹配名称的测试
bun test -t "应正确创建实例"

# 硬件集成测试（需要 Node.js，Bun 与 serialport 不兼容）
npx tsx tests/hardware-test.ts
```

## 文件结构

```
src/
├── index.ts              # CLI 入口 + MCP stdio 模式
├── server.ts             # HTTP 服务入口（含托盘集成）
├── service-manager.ts    # 服务进程状态管理（PID/端口文件）
├── config/
│   └── index.ts          # 配置管理（JSON 文件 + CLI 参数）
├── serial/
│   └── SerialManager.ts  # 串口管理（EventEmitter）
├── telnet/
│   └── TelnetServer.ts   # Telnet 服务（EventEmitter）
├── mcp/
│   ├── index.ts          # MCP 服务入口 + DataBuffer + 工具注册
│   ├── transport/
│   │   └── http-sse.ts   # HTTP+SSE JSON-RPC 传输
│   └── tools/            # MCP 工具（每个文件一个工具）
├── bridge/
│   └── DataBridge.ts     # 数据桥接（串口↔Telnet↔MCP）
└── tray/
    ├── TrayManager.ts    # 系统托盘管理
    └── console.ts        # Windows 控制台窗口控制（koffi FFI）
```

## 代码风格

### 导入
```typescript
// 1. Node.js 内置（使用 node: 前缀）
import { createServer } from "node:http";
import { EventEmitter } from "events";

// 2. 第三方包（无扩展名）
import { z } from "zod";
import { SerialPort } from "serialport";

// 3. 本地模块（相对路径 + .js 扩展名）
import { SerialManager } from "../serial/SerialManager.js";
import type { ToolDef } from "../index.js";  // 类型导入用 import type
```

### 命名规范

| 元素 | 规范 | 示例 |
|------|------|------|
| 类 | PascalCase | `SerialManager`, `DataBridge` |
| 接口 | PascalCase | `SerialConfig`, `TelnetClient` |
| 类型别名 | PascalCase | `TrayState = "idle" \| "connected" \| "error"` |
| 常量 | UPPER_SNAKE_CASE | `DEFAULT_CONFIG`, `SW_HIDE` |
| 私有字段（getter 后备） | `_` 前缀 + camelCase | `_isConnected`, `_isRunning` |
| 其他私有字段 | camelCase | `serialPort`, `config`, `server` |
| 公有方法 | camelCase | `connect()`, `writeLine()`, `broadcast()` |
| 工具函数 | `execute` 前缀 | `executeSerialWrite()` |
| Schema 对象 | camelCase + `Schema` 后缀 | `serialWriteSchema` |
| 工具定义 | camelCase + `Tool` 后缀 | `serialWriteTool` |
| 事件映射接口 | PascalCase + `Events` 后缀 | `SerialManagerEvents` |

### 导出
- **仅使用命名导出**，不用 `default export`
- 接口/类/函数/常量用 `export` 内联声明
- 模块聚合用 `export { ... }` 块

### 类型风格
- 对象形状用 `interface`，联合类型用 `type`
- 类型与使用处分开定义（文件顶部或类之前）
- Zod schema 和对应的 TypeScript 类型**手动并列定义**（不用 `z.infer<>`）
- 所有公有方法**显式标注返回类型**

### 错误处理
- 用户错误消息使用**中文**：`throw new Error("串口未连接")`
- MCP 工具**不抛异常**，返回 `{ success: false, message: "..." }`
- 检查 error 类型：`error instanceof Error ? error.message : String(error)`
- 非关键操作静默捕获：`catch { /* 忽略关闭错误 */ }`
- EventEmitter 错误：`this.emit("error", error)`

### 注释
- 文件顶部 `/** 模块名 - 简述 */`
- 所有公有方法/接口/属性用 **JSDoc**（中文）
- 行内注释用 `//`（中文）
- 测试描述用中文：`test("应正确创建实例", ...)`

### 其他约定
- **不做注释**（除非用户要求）——保持代码精简
- 构造函数对 config 做**浅拷贝**：`this.config = { ...config }`
- getter 访问器暴露状态，`getConfig()` 返回副本
- 事件处理器**预绑定**以便清理：`this.boundHandleSerialData = this.handleSerialData.bind(this)`
- 运行时日志用 `console.error()` + `[SerialHub]` 前缀，`console.log()` 仅用于用户输出
- 清理方法命名为 `dispose()`
- 用 `??` 而非 `||` 做默认值

## 架构原则

1. **关注点分离**：串口、Telnet、MCP 各自独立模块
2. **EventEmitter 模式**：核心类继承 EventEmitter，通过事件解耦
3. **错误恢复力**：串口/Telnet/MCP 任一故障不影响其他模块
4. **配置驱动**：所有端口、超时、缓冲区大小可配置
5. **MCP 工具模式**：每个工具文件导出 schema + type + result interface + execute 函数 + tool 定义

## 开发流程

```
开发 → typecheck → lint → test → commit
```

### 检查清单
- [ ] `npm run typecheck` — 零错误
- [ ] `npm run lint` — 零警告
- [ ] 在 `.test.ts` 文件中添加/更新测试
- [ ] `bun test` — 所有测试通过
- [ ] `git commit` 提交更改
