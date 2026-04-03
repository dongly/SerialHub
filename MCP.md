# SerialHub MCP (Model Context Protocol) 使用指南

## 概述

SerialHub 实现了 MCP (Model Context Protocol) HTTP 服务，允许 AI 工具通过 HTTP API 与串口设备交互。

**传输模式**: StreamableHTTP (Stateless + JSONResponse)  
**端点**: `http://<host>:<port>/mcp`  
**协议**: JSON-RPC 2.0

---

## 快速开始

### 启动服务

```bash
# 默认端口 5000
./bin/serialhub --no-tray

# 指定 MCP 端口
./bin/serialhub --no-tray --mcp-port 55555
```

### 健康检查

```bash
curl http://127.0.0.1:5000/health
```

---

## MCP 工具列表

| 工具名 | 描述 | 必需参数 |
|--------|------|----------|
| `serial_list` | 列出可用串口 | 无 |
| `serial_status` | 获取串口连接状态 | 无 |
| `serial_connect` | 连接串口 | `port`: 串口号, `baudRate`: 波特率 |
| `serial_disconnect` | 断开串口连接 | 无 |
| `serial_write` | 向串口写入数据 | `data`: 数据内容 |
| `serial_read` | 从串口读取数据 | `timeout`: 超时时间(ms) |

---

## API 调用示例

### 1. 列出串口

**请求**:
```bash
curl -X POST http://127.0.0.1:5000/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "serial_list"
    },
    "id": 1
  }'
```

**响应**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "找到 3 个串口\n[COM1, COM3, COM4]"
      }
    ]
  }
}
```

### 2. 连接串口

**请求**:
```bash
curl -X POST http://127.0.0.1:5000/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "serial_connect",
      "arguments": {
        "port": "COM4",
        "baudRate": 115200
      }
    },
    "id": 2
  }'
```

**响应**:
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "串口连接成功: COM4@115200 8N1"
      }
    ]
  }
}
```

### 3. 写入数据

**请求**:
```bash
curl -X POST http://127.0.0.1:5000/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "serial_write",
      "arguments": {
        "data": "Hello World",
        "addNewline": true
      }
    },
    "id": 3
  }'
```

### 4. 读取数据

**请求**:
```bash
curl -X POST http://127.0.0.1:5000/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "serial_read",
      "arguments": {
        "timeout": 3000,
        "maxSize": 4096
      }
    },
    "id": 4
  }'
```

**响应**:
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "读取成功: 11 字节\ndata: Hello World\nbytes: 11\ntimedOut: false"
      }
    ]
  }
}
```

### 5. 断开连接

**请求**:
```bash
curl -X POST http://127.0.0.1:5000/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "serial_disconnect"
    },
    "id": 5
  }'
```

---

## 参数说明

### serial_connect

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `port` | string | 是 | - | 串口号，如 COM4、/dev/ttyUSB0 |
| `baudRate` | int | 否 | 115200 | 波特率：9600, 19200, 38400, 57600, 115200 |
| `dataBits` | int | 否 | 8 | 数据位：7, 8 |
| `parity` | string | 否 | "none" | 校验位：none, even, odd |
| `stopBits` | float | 否 | 1 | 停止位：1, 1.5, 2 |

### serial_write

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `data` | string | 是 | - | 要写入的数据 |
| `addNewline` | bool | 否 | false | 是否自动添加换行符 |

### serial_read

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `timeout` | int | 否 | 0 | 超时时间(毫秒)，0表示无限等待 |
| `maxSize` | int | 否 | 4096 | 最大读取字节数 |

---

## Python 调用示例

```python
import requests

def mcp_call(mcp_port, method, params=None):
    """调用 MCP 工具"""
    url = f"http://127.0.0.1:{mcp_port}/mcp"
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json",
    }
    body = {
        "jsonrpc": "2.0",
        "method": method,
        "id": 1,
    }
    if params:
        body["params"] = params
    
    resp = requests.post(url, json=body, headers=headers, timeout=10)
    return resp.json()

# 列出串口
result = mcp_call(5000, "tools/call", {"name": "serial_list"})

# 连接串口
result = mcp_call(5000, "tools/call", {
    "name": "serial_connect",
    "arguments": {"port": "COM4", "baudRate": 115200}
})

# 写入数据
result = mcp_call(5000, "tools/call", {
    "name": "serial_write",
    "arguments": {"data": "Hello", "addNewline": True}
})

# 读取数据
result = mcp_call(5000, "tools/call", {
    "name": "serial_read",
    "arguments": {"timeout": 3000}
})
```

---

## 错误处理

### 常见错误响应

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32600,
    "message": "串口未连接"
  }
}
```

### 错误码

| 错误码 | 含义 |
|--------|------|
| -32600 | 无效请求（如串口未连接时操作） |
| -32601 | 方法未找到 |
| -32700 | 解析错误 |

---

## 双向数据流向

SerialHub 实现 MCP、Telnet、串口之间的**双向数据桥接**：

### 1. MCP → 串口 → Telnet

```
MCP 工具 ──▶ 串口写入 ──▶ 串口回环 ──▶ DataBridge ──▶ MCP 读取（回环数据）
                              │
                              └──▶ DataBridge ──▶ Telnet 接收（转发数据）
```

**场景**: AI 工具通过 MCP 发送命令到 MCU，同时人工操作员在 Telnet 端可见。

### 2. Telnet → 串口 → MCP

```
Telnet 发送 ──▶ 串口写入 ──▶ 串口回环 ──▶ DataBridge ──▶ Telnet 接收（回环数据）
                               │
                               └──▶ DataBridge ──▶ MCP 缓冲区（转发数据）
```

**场景**: 人工操作员在 Telnet 端发送命令，AI 工具通过 MCP 读取响应。

### 3. 完整双向桥接

```
                         ┌──────────┐
                         │  MCP     │
                         │  工具    │
                         └────┬─────┘
                              │
                              │ 写入
                              ▼
                    ┌───────────────────┐
                    │     串口          │
                    │  ┌─────────────┐  │
        MCP 读取 ◄──┤  │ DataBridge  │  ├──► Telnet 读取
        (回环)    │  │ 转发到其他端 │  │  (回环)
                    │  └─────────────┘  │
                    └────────┬──────────┘
                             │
                             ▼
                        ┌──────────┐
                        │  MCU     │
                        │  设备    │
                        └──────────┘
                             │
                    ┌────────┴────────┐
                    │                 │
        Telnet 写入 ▼                 ▼ MCP 读取
       (转发到MCP) │                 │ (转发到Telnet)
                    │                 │
              ┌─────────┐       ┌─────────┐
              │  Telnet │◄─────►│  MCP    │
              │  客户端 │       │  缓冲区 │
              └─────────┘       └─────────┘
```

**特点**:
- MCP 和 Telnet 都可以读写串口
- 任何一端发送的数据，其他端都能收到
- DataBridge 负责转发数据到所有连接的客户端

---

## 相关文件

| 文件 | 说明 |
|------|------|
| `pkg/mcp/server.go` | MCP HTTP 服务实现 |
| `pkg/mcp/tools/*.go` | MCP 工具实现 |
| `tests/integration/test_serialhub.py` | Python 集成测试 |

---

## 更多信息

- [MCP 规范](https://modelcontextprotocol.io/)
- [AGENTS.md](./AGENTS.md) - 项目架构说明
- [README.md](./README.md) - 项目主文档
