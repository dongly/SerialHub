# SerialHub 实现计划

## 项目概述
- **名称**: SerialHub
- **描述**: 串口与网络双向桥接器
- **技术栈**: Bun + TypeScript + @serialport + MCP
- **测试环境**: COM9, 115200bps, 8N1, 验证命令 `help`

## 开发流程（每步必须完成）
```
subagent 开发 → code-simplefy 精简 → bun run typecheck → bun run lint → bun test → git commit
```

**重要**: 每个步骤由独立的 subagent (general-purpose) 处理，确保步骤隔离和独立性。

---

## 步骤 1: 项目配置

### 创建文件
- `package.json` - 项目清单
- `tsconfig.json` - TypeScript 配置

### package.json 内容
```json
{
  "name": "serialhub",
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "start": "bun run src/index.ts",
    "serve": "bun run src/server.ts",
    "mcp": "bun run src/index.ts mcp",
    "typecheck": "tsc --noEmit",
    "lint": "eslint src/",
    "test": "bun test"
  },
  "dependencies": {
    "@serialport/serialport": "^12.0.0",
    "@modelcontextprotocol/sdk": "^1.0.0"
  },
  "devDependencies": {
    "@types/bun": "latest",
    "typescript": "^5.0.0",
    "eslint": "^8.0.0",
    "@typescript-eslint/parser": "^6.0.0",
    "@typescript-eslint/eslint-plugin": "^6.0.0"
  }
}
```

### tsconfig.json 内容
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "outDir": "dist",
    "rootDir": "src"
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}
```

### Subagent 执行
```bash
# 使用 general-purpose agent 执行此步骤
task(subagent_type="general-purpose", prompt="执行 SerialHub 步骤 1: 项目配置...")
```

### 验证
- `bun run typecheck` - 零错误
- `git commit -m "feat: 初始化项目配置"`

---

## 步骤 2: 配置模块

### 创建文件
- `src/config/index.ts` - 配置管理
- `tests/config.test.ts` - 单元测试

### 功能需求
1. 定义配置接口
2. 加载 JSON 配置文件
3. CLI 参数解析
4. 配置合并（CLI > 文件 > 默认值）

### 配置接口
```typescript
interface SerialConfig {
  port: string;           // 串口名，如 "COM9"
  baudRate: number;       // 波特率，默认 115200
  dataBits: 5 | 6 | 7 | 8;
  parity: "none" | "even" | "odd";
  stopBits: 1 | 2;
}

interface TelnetConfig {
  port: number;           // Telnet 端口，默认 2323
}

interface MCPConfig {
  httpPort: number;       // MCP HTTP 端口，默认 3000
}

interface Config {
  serial: SerialConfig;
  telnet: TelnetConfig;
  mcp: MCPConfig;
  debug: boolean;
}
```

### 默认配置
```json
{
  "serial": {
    "port": "COM9",
    "baudRate": 115200,
    "dataBits": 8,
    "parity": "none",
    "stopBits": 1
  },
  "telnet": { "port": 2323 },
  "mcp": { "httpPort": 3000 },
  "debug": false
}
```

### 测试用例
- 加载默认配置
- 加载 JSON 文件配置
- CLI 参数覆盖配置
- 配置验证

### 验证
- `code-simplefy` 精简代码
- `bun run typecheck`
- `bun run lint`
- `bun test` - 全部通过
- `git commit -m "feat: 添加配置模块"`

---

## 步骤 3: 串口管理

### 创建文件
- `src/serial/SerialManager.ts` - 串口管理器
- `tests/serial.test.ts` - 单元测试

### 功能需求
1. 列出可用串口 (`listPorts()`)
2. 连接串口 (`connect()`)
3. 断开串口 (`disconnect()`)
4. 发送数据 (`write()`)
5. 读取数据 (事件驱动 `onData`)
6. 连接状态管理
7. 错误恢复（自动重连）

### 类设计
```typescript
class SerialManager extends EventEmitter {
  private port: SerialPort | null;
  private config: SerialConfig;
  
  // 方法
  async listPorts(): Promise<string[]>;
  async connect(portName?: string): Promise<void>;
  async disconnect(): Promise<void>;
  async write(data: Buffer | string): Promise<void>;
  
  // 属性
  get isConnected(): boolean;
  get currentPort(): string | null;
  
  // 事件
  // 'data' - 收到数据
  // 'connected' - 连接成功
  // 'disconnected' - 断开连接
  // 'error' - 发生错误
}
```

### 测试用例
- 列出串口列表
- 连接 COM9 串口
- 发送 `help` 命令
- 接收数据
- 断开连接
- 错误处理

### 硬件验证
- 连接 COM9 (115200, 8N1)
- 发送 `help<回车>` 命令，验证收到 MCU 响应

### 验证
- `code-simplefy` 精简代码
- `bun run typecheck`
- `bun run lint`
- `bun test` - 全部通过
- 硬件测试通过
- `git commit -m "feat: 添加串口管理模块"`

---

## 步骤 4: MCP 工具

### 创建文件
- `src/mcp/index.ts` - MCP 服务入口
- `src/mcp/tools/serial_list.ts` - 列出串口
- `src/mcp/tools/serial_connect.ts` - 连接串口
- `src/mcp/tools/serial_disconnect.ts` - 断开串口
- `src/mcp/tools/serial_write.ts` - 发送数据
- `src/mcp/tools/serial_read.ts` - 读取数据
- `src/mcp/tools/serial_status.ts` - 获取状态
- `tests/mcp.test.ts` - 单元测试

### MCP 工具定义

#### serial_list
```typescript
{
  name: "serial_list",
  description: "列出系统中所有可用的串口",
  inputSchema: { type: "object", properties: {} }
}
// 返回: { ports: ["COM1", "COM3", "COM9"] }
```

#### serial_connect
```typescript
{
  name: "serial_connect",
  description: "连接到指定串口",
  inputSchema: {
    type: "object",
    properties: {
      port: { type: "string", description: "串口名，如 COM9" },
      baudRate: { type: "number", description: "波特率，默认 115200" }
    },
    required: ["port"]
  }
}
// 返回: { success: true, port: "COM9" }
```

#### serial_disconnect
```typescript
{
  name: "serial_disconnect",
  description: "断开当前串口连接",
  inputSchema: { type: "object", properties: {} }
}
// 返回: { success: true }
```

#### serial_write
```typescript
{
  name: "serial_write",
  description: "向串口发送数据",
  inputSchema: {
    type: "object",
    properties: {
      data: { type: "string", description: "要发送的数据" },
      encoding: { type: "string", description: "编码方式，默认 utf-8" }
    },
    required: ["data"]
  }
}
// 返回: { success: true, bytesWritten: 4 }
```

#### serial_read
```typescript
{
  name: "serial_read",
  description: "读取串口缓冲区中的数据",
  inputSchema: {
    type: "object",
    properties: {
      timeout: { type: "number", description: "超时时间(ms)，默认 1000" }
    }
  }
}
// 返回: { data: "...", encoding: "utf-8", timestamp: 1234567890 }
```

#### serial_status
```typescript
{
  name: "serial_status",
  description: "获取串口连接状态",
  inputSchema: { type: "object", properties: {} }
}
// 返回: { connected: true, port: "COM9", baudRate: 115200 }
```

### 测试用例
- 每个工具的参数验证
- 工具调用返回格式
- 与 SerialManager 集成测试

### 验证
- `code-simplefy` 精简代码
- `bun run typecheck`
- `bun run lint`
- `bun test` - 全部通过
- `git commit -m "feat: 添加 MCP 工具定义"`

---

## 步骤 5: MCP stdio 传输

### 创建文件
- `src/mcp/transport/stdio.ts` - stdio 传输实现
- `tests/transport/stdio.test.ts` - 单元测试

### 功能需求
1. 通过 stdin 接收 MCP 请求
2. 通过 stdout 发送 MCP 响应
3. 实现 MCP 协议握手
4. 处理 JSON-RPC 2.0 消息

### 实现细节
- 监听 `process.stdin` 的 `data` 事件
- 解析 JSON-RPC 请求
- 调用对应工具处理
- 返回 JSON-RPC 响应

### 运行方式
```bash
serialhub mcp
# 或
bun run src/index.ts mcp
```

### AI 工具配置示例
```json
{
  "mcpServers": {
    "serialhub": {
      "command": "serialhub",
      "args": ["mcp"]
    }
  }
}
```

### 测试用例
- 模拟 stdin 输入
- 验证 stdout 输出格式
- 握手流程测试
- 工具调用测试

### 验证
- `code-simplefy` 精简代码
- `bun run typecheck`
- `bun run lint`
- `bun test` - 全部通过
- `git commit -m "feat: 添加 MCP stdio 传输"`

---

## 步骤 6: MCP HTTP+SSE 传输

### 创建文件
- `src/mcp/transport/http-sse.ts` - HTTP+SSE 传输实现
- `tests/transport/http-sse.test.ts` - 单元测试

### 功能需求
1. HTTP POST 端点接收 MCP 请求
2. SSE 端点推送 MCP 通知
3. 实现 MCP 协议握手
4. 支持 CORS

### API 端点
```
POST /mcp          - MCP 请求处理
GET  /mcp/sse      - SSE 事件流
GET  /health       - 健康检查
```

### 测试用例
- HTTP POST 请求测试
- SSE 连接测试
- CORS 测试
- 工具调用测试

### 手动测试
```bash
# 列出串口
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_list"},"id":1}'

# 连接串口
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_connect","arguments":{"port":"COM9"}},"id":2}'
```

### 验证
- `code-simplefy` 精简代码
- `bun run typecheck`
- `bun run lint`
- `bun test` - 全部通过
- `git commit -m "feat: 添加 MCP HTTP+SSE 传输"`

---

## 步骤 7: Telnet 服务

### 创建文件
- `src/telnet/TelnetServer.ts` - Telnet 服务器
- `tests/telnet.test.ts` - 单元测试

### 功能需求
1. TCP 服务器监听连接
2. 多客户端支持
3. 广播数据到所有客户端
4. 接收客户端输入并转发
5. 客户端连接/断开管理

### 类设计
```typescript
class TelnetServer extends EventEmitter {
  private server: NetServer;
  private clients: Set<Socket>;
  
  async start(port: number): Promise<void>;
  async stop(): Promise<void>;
  broadcast(data: Buffer): void;
  
  // 事件
  // 'connection' - 新客户端连接
  // 'data' - 收到客户端数据
  // 'disconnect' - 客户端断开
}
```

### 测试用例
- 启动/停止服务器
- 客户端连接
- 数据广播
- 多客户端测试

### 手动测试
```bash
# 启动服务
serialhub serve

# MobaXterm 连接
telnet localhost 2323
```

### 验证
- `code-simplefy` 精简代码
- `bun run typecheck`
- `bun run lint`
- `bun test` - 全部通过
- MobaXterm 连接测试
- `git commit -m "feat: 添加 Telnet 服务"`

---

## 步骤 8: 数据桥接

### 创建文件
- `src/bridge/DataBridge.ts` - 数据桥接中心
- `tests/bridge.test.ts` - 单元测试

### 功能需求
1. 串口数据同时转发到 Telnet 和 AI 接口
2. Telnet 输入转发到串口
3. AI 输入转发到串口
4. 事件驱动架构
5. 调试日志

### 类设计
```typescript
class DataBridge extends EventEmitter {
  private serial: SerialManager;
  private telnet: TelnetServer;
  private mcp: MCPService;
  
  constructor(serial, telnet, mcp);
  start(): void;
  stop(): void;
  
  // 数据流
  // serial:data -> telnet.broadcast + mcp.notify
  // telnet:data -> serial.write
  // mcp:data -> serial.write
}
```

### 测试用例
- 串口数据转发测试
- Telnet 输入测试
- AI 输入测试
- 同时多路径测试

### 验证
- `code-simplefy` 精简代码
- `bun run typecheck`
- `bun run lint`
- `bun test` - 全部通过
- `git commit -m "feat: 添加数据桥接模块"`

---

## 步骤 9: 主入口

### 创建文件
- `src/index.ts` - stdio MCP 入口
- `src/server.ts` - HTTP 服务入口

### src/index.ts
```typescript
// 用途: stdio MCP 模式（AI 工具直接启动）
// 命令: serialhub mcp

import { MCPService } from "./mcp";
import { SerialManager } from "./serial";
import { loadConfig } from "./config";

const config = loadConfig();
const serial = new SerialManager(config.serial);
const mcp = new MCPService(serial, "stdio");

mcp.start();
```

### src/server.ts
```typescript
// 用途: 服务模式（HTTP+SSE MCP + Telnet）
// 命令: serialhub serve

import { SerialManager } from "./serial";
import { TelnetServer } from "./telnet";
import { MCPService } from "./mcp";
import { DataBridge } from "./bridge";
import { loadConfig } from "./config";

const config = loadConfig();
const serial = new SerialManager(config.serial);
const telnet = new TelnetServer();
const mcp = new MCPService(serial, "http", config.mcp.httpPort);
const bridge = new DataBridge(serial, telnet, mcp);

await telnet.start(config.telnet.port);
await mcp.startHttp();
bridge.start();

// 自动连接串口
if (config.serial.port) {
  await serial.connect();
}
```

### CLI 参数
```bash
serialhub mcp                              # stdio MCP 模式
serialhub serve                            # 服务模式
serialhub serve --serial-port COM9         # 指定串口
serialhub serve --config config.json       # 指定配置文件
serialhub serve --debug                    # 调试模式
```

### 测试用例
- CLI 参数解析
- 各模式启动测试

### 验证
- `code-simplefy` 精简代码
- `bun run typecheck`
- `bun run lint`
- `bun test` - 全部通过
- `git commit -m "feat: 添加主入口"`

---

## 步骤 10: 集成验证

### 验证场景

#### 场景 1: AI 工具通过 stdio MCP 控制
```json
// AI 工具配置
{
  "mcpServers": {
    "serialhub": {
      "command": "serialhub",
      "args": ["mcp", "--serial-port", "COM9"]
    }
  }
}
```

AI 工具调用:
1. `serial_list` → 列出串口
2. `serial_connect` → 连接 COM9
3. `serial_write` → 发送 `help`
4. `serial_read` → 读取响应
5. `serial_status` → 查看状态

#### 场景 2: 服务模式 + Telnet + AI 并发
```bash
# 启动服务
serialhub serve --serial-port COM9

# Telnet 连接
telnet localhost 2323

# AI 通过 HTTP 连接
curl -X POST http://localhost:3000/mcp ...
```

验证:
- 串口数据同时显示在 Telnet 和 AI 接口
- Telnet 输入转发到串口
- AI 输入转发到串口
- 两个输入源互不干扰

#### 场景 3: 错误恢复
- 拔掉 MCU USB，验证服务不崩溃
- 重插 USB，验证自动重连
- Telnet 客户端断开不影响其他连接

### 最终验证
- `bun run typecheck` - 零错误
- `bun run lint` - 零警告
- `bun test` - 全部通过
- 所有场景测试通过

### Git 提交
```bash
git commit -m "feat: 完成集成验证"
```

---

## 文件结构总览

```
D:\Develop\SerialHub\
├── package.json
├── tsconfig.json
├── AGENTS.md
├── README.md
├── PLAN.md
├── src/
│   ├── index.ts           # stdio MCP 入口
│   ├── server.ts          # HTTP 服务入口
│   ├── config/
│   │   └── index.ts       # 配置管理
│   ├── serial/
│   │   └── SerialManager.ts
│   ├── telnet/
│   │   └── TelnetServer.ts
│   ├── mcp/
│   │   ├── index.ts       # MCP 服务
│   │   ├── transport/
│   │   │   ├── stdio.ts
│   │   │   └── http-sse.ts
│   │   └── tools/
│   │       ├── serial_list.ts
│   │       ├── serial_connect.ts
│   │       ├── serial_disconnect.ts
│   │       ├── serial_write.ts
│   │       ├── serial_read.ts
│   │       └── serial_status.ts
│   └── bridge/
│       └── DataBridge.ts
└── tests/
    ├── config.test.ts
    ├── serial.test.ts
    ├── mcp.test.ts
    ├── transport/
    │   ├── stdio.test.ts
    │   └── http-sse.test.ts
    ├── telnet.test.ts
    └── bridge.test.ts
```
