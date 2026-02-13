# SerialHub - AI 代理指南

## 项目概述
SerialHub 是一个串口（MCU）与网络连接（telnet/AI）之间的双向桥接器。

### 核心架构
**Telnet 和 AI 接口同时连接同一个串口**，实现：
- **路径1**：串口 ↔ Telnet（MobaXterm、终端模拟器用于人工交互）
- **路径2**：串口 ↔ AI 接口（AI 工具的程序化访问）

### 数据流模式
```
MCU ←→ 串口 ←→ SerialHub
                 ├→ Telnet Server (人工监视/操作）
                 └→ AI Interface (AI 工具程序化访问）
```

**关键特性：**
- 串口数据**同时转发**到 Telnet 和 AI 接口
- Telnet 和 AI 的输入都**独立转发**到串口
- 支持多个 Telnet 连接并发访问
- AI 接口可选启用/禁用

## 技术栈

| 组件 | 技术 |
|------|------|
| 运行时 | Bun |
| 语言 | TypeScript |
| 串口 | @serialport |
| AI 接口 | MCP (Model Context Protocol) |
| 测试 | Bun test |
| 代码规范 | ESLint + Prettier |

## AI 接口协议

### MCP (Model Context Protocol)
AI 工具通过 MCP 协议访问串口功能，支持两种传输模式：

#### 1. stdio 传输
AI 工具启动 SerialHub 作为子进程，通过 stdin/stdout 通信。
```json
// AI 工具配置示例
{
  "mcpServers": {
    "serialhub": {
      "command": "serialhub",
      "args": ["mcp"]
    }
  }
}
```

#### 2. HTTP+SSE 传输
SerialHub 作为独立服务运行，AI 工具通过 HTTP 连接。
```bash
serialhub serve --mcp-port 3000
```

### MCP 工具列表

| 工具名 | 描述 | 参数 |
|--------|------|------|
| `serial_list` | 列出可用串口 | 无 |
| `serial_connect` | 连接串口 | port, baudRate? |
| `serial_disconnect` | 断开串口 | 无 |
| `serial_write` | 发送数据 | data, encoding? |
| `serial_read` | 读取数据 | timeout? |
| `serial_status` | 获取状态 | 无 |

## 文件结构

```
src/
├── index.ts              # stdio MCP 入口
├── server.ts             # HTTP 服务入口
├── config/
│   └── index.ts          # 配置管理
├── serial/
│   └── SerialManager.ts  # 串口管理
├── telnet/
│   └── TelnetServer.ts   # Telnet 服务
├── mcp/
│   ├── index.ts          # MCP 服务入口
│   ├── transport/
│   │   ├── stdio.ts      # stdio 传输
│   │   └── http-sse.ts   # HTTP+SSE 传输
│   └── tools/            # MCP 工具实现
└── bridge/
    └── DataBridge.ts     # 数据桥接
```

## 开发流程

每一步必须按顺序完成：
```
开发 → code-simplefy 精简 → typecheck → lint → test → commit
```

### 检查清单
- [ ] 运行 `bun run typecheck` - 零错误
- [ ] 运行 `bun run lint` - 零警告
- [ ] 在 `.test.ts` 文件中添加/更新测试
- [ ] 运行 `bun test` - 所有测试通过
- [ ] 使用 code-simplefy 精简代码
- [ ] `git commit` 提交更改

## 关键设计要点

1. **同时转发**
   - 串口数据必须同时转发到 Telnet 和 AI 接口
   - 使用 `broadcast()` 方法发送给所有 Telnet 客户端
   - AI 接口可以独立启用/禁用

2. **独立输入源**
   - Telnet 客户端的输入独立转发到串口
   - AI 接口的输入独立转发到串口
   - 两个输入源互不干扰

3. **连接管理**
   - 支持多个 Telnet 连接并发访问
   - 每个连接独立管理状态
   - AI 接口作为单一连接处理

4. **数据完整性**
   - 所有数据路径必须可追踪和可测试
   - 使用事件驱动架构记录所有数据传输
   - 提供调试模式用于问题诊断

5. **错误恢复**
   - 串口断开时不影响 Telnet/AI 接口（保持监听）
   - Telnet 客户端断开不影响其他连接
   - AI 接口故障时继续 Telnet 功能

## 架构原则

1. **关注点分离**：串口、Telnet 和 AI 接口是独立模块
2. **数据流完整性**：所有数据路径必须可追踪和可测试
3. **错误恢复力**：网络/串口故障不得导致应用崩溃
4. **配置驱动**：所有端口、超时和缓冲区可配置
5. **可观察性**：所有状态更改发出事件用于监控/调试

## 入门检查清单

实现新功能时：
- [ ] 运行 `bun run typecheck` - 零错误
- [ ] 运行 `bun run lint` - 零警告
- [ ] 在 `.test.ts` 文件中添加/更新测试
- [ ] 运行 `bun test` - 所有测试通过
- [ ] 如果添加新模式则更新此 AGENTS.md
