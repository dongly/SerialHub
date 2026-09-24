# SerialHub MCP 使用指南

## 概述

SerialHub 实现 [MCP (Model Context Protocol)](https://modelcontextprotocol.io/) HTTP 服务，允许 AI 工具通过标准 HTTP API 与串口设备交互。

| 属性 | 值 |
|------|-----|
| 传输模式 | Streamable HTTP（Stateless · 非流式 JSON 响应） |
| 端点 | `http://<host>:<port>/mcp` |
| 协议 | JSON-RPC 2.0 |
| 默认端口 | 5000 |

支持 **联邦模式**：Windows 与 WSL 两侧可各运行一个 SerialHub，后启动的自动以从实例身份接入，主实例通过 `serial_list` 聚合双侧串口（详见 [联邦模式](#联邦模式windows-wsl-双侧串口)）。

---

## 快速开始

### 1. 启动服务

```bash
# 默认配置启动
./bin/serialhub

# 指定 MCP 端口与监听地址
./bin/serialhub --mcp-port 55555 --host 0.0.0.0
```

### 2. 验证服务

```bash
# 健康检查
curl http://127.0.0.1:5000/health

# 列出可用串口
curl -X POST http://127.0.0.1:5000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_list"},"id":1}'
```

---

## MCP 工具参考

### 工具列表

| 工具名 | 功能 | 必需参数 |
|--------|------|----------|
| `serial_list` | 列出所有可用串口（联邦模式聚合双侧） | 无 |
| `serial_status` | 获取当前串口连接状态 | 无 |
| `serial_connect` | 连接到指定串口 | `port` |
| `serial_disconnect` | 断开当前串口连接 | 无 |
| `serial_write` | 向串口写入数据 | `data` |
| `serial_read` | 从串口读取数据（阻塞式） | 无 |
| `serial_clear` | 清空 read 缓冲区（丢弃未消费数据） | 无 |

`serial_list` 返回结构（`ports` 数组，每项含 `name`/`origin`/`side`/`port`）：

```json
{
  "message": "找到 2 个串口",
  "ports": [
    {"name": "/dev/ttyUSB1", "origin": "local", "side": "wsl", "port": "/dev/ttyUSB1"},
    {"name": "windows:COM3", "origin": "federated", "side": "windows", "port": "COM3"}
  ]
}
```

- `origin`: `local` = 主实例本侧直连端口（`name` 为裸名）；`federated` = 从实例上报端口（`name` 为 `side:port` 全名）
- 连接联邦端口时直接使用 `name` 全名（如 `windows:COM3`），断开/写入自动路由

### 参数详解

#### serial_connect

```json
{
  "port": "COM4",        // 必填：串口号
  "baudRate": 115200,    // 可选：波特率，默认 115200
  "dataBits": 8,         // 可选：数据位 (7/8)，默认 8
  "parity": "none",      // 可选：校验位 (none/even/odd)，默认 none
  "stopBits": 1          // 可选：停止位 (1/1.5/2)，默认 1
}
```

#### serial_write

```json
{
  "data": "Hello World",  // 必填：要写入的数据
  "addNewline": true      // 可选：是否自动追加 \n，默认 true（未指定时也追加）
}
```

#### serial_read

```json
{
  "timeout": 3000,   // 可选：超时时间(ms)，0=无限等待，默认 1000
  "maxSize": 4096    // 可选：最大读取字节数，默认 4096
}
```

---

## 使用示例

### cURL 示例

#### 连接串口
```bash
curl -X POST http://127.0.0.1:5000/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "serial_connect",
      "arguments": {"port": "COM4", "baudRate": 115200}
    },
    "id": 1
  }'
```

#### 写入数据
```bash
curl -X POST http://127.0.0.1:5000/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "serial_write",
      "arguments": {"data": "help", "addNewline": true}
    },
    "id": 2
  }'
```

#### 读取响应
```bash
curl -X POST http://127.0.0.1:5000/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "serial_read",
      "arguments": {"timeout": 5000}
    },
    "id": 3
  }'
```

### Python 示例

```python
import requests

def mcp_call(port, tool_name, arguments=None):
    """调用 MCP 工具"""
    resp = requests.post(
        f"http://127.0.0.1:{port}/mcp",
        headers={"Content-Type": "application/json"},
        json={
            "jsonrpc": "2.0",
            "method": "tools/call",
            "params": {"name": tool_name, "arguments": arguments or {}},
            "id": 1
        },
        timeout=10
    )
    return resp.json()

# 标准工作流程
mcp_call(5000, "serial_list")                                    # 1. 查找串口
mcp_call(5000, "serial_connect", {"port": "COM4"})              # 2. 连接
mcp_call(5000, "serial_write", {"data": "version"})             # 3. 发送命令
result = mcp_call(5000, "serial_read", {"timeout": 3000})       # 4. 读取响应
print(result["result"]["content"][0]["text"])
mcp_call(5000, "serial_disconnect")                             # 5. 断开连接
```

### OpenCode MCP 配置

OpenCode 支持两种配置级别，**项目级 > 用户级**。

| 级别 | 配置文件路径 | 适用场景 |
|------|-------------|---------|
| 用户级 | `~/.opencode/opencode.json` | 个人开发，全局共用 |
| 项目级 | `opencode.json`（项目根目录） | 团队协作，独立配置 |

**方式一：remote（HTTP，推荐）**

```json
{
  "mcp": {
    "serialhub": {
      "type": "remote",
      "url": "http://127.0.0.1:5000/mcp",
      "enabled": true
    }
  }
}
```

- **铁律：永远写 `127.0.0.1:5000`**。联邦模式下从实例会在本侧反代 `/mcp` 到主实例，两侧的 `127.0.0.1:5000/mcp` 都可用，无需关心主实例在哪侧。
- 访问局域网其他机器上的 SerialHub 时才改地址，如 `"url": "http://192.168.1.100:5000/mcp"`。

**方式二：local（stdio，免手动启动）**

```json
{
  "mcp": {
    "serialhub": {
      "type": "local",
      "command": "serialhub",
      "args": ["--stdio"],
      "enabled": true
    }
  }
}
```

- OpenCode 启动时自动拉起子进程，通过 stdio 通信，退出时子进程随之退出。
- 子进程若发现已有主实例在运行，自动退化为**透明代理**（stdio ↔ 主实例 `/mcp` 转发）；若没有主实例，则自己成为主实例（HTTP 服务照常，但不弹浏览器）。
- `command` 需指向 serialhub 可执行文件的路径（不在 `PATH` 时写绝对路径）。

---

## 联邦模式（Windows + WSL 双侧串口）

SerialHub 支持 **Windows 与 WSL 两侧同时运行**，聚合一台机器上双侧的串口设备：

### 角色与发现

| 角色 | 触发条件 | 提供能力 |
|------|---------|---------|
| **主实例** | 先启动（本侧无主） | 完整服务：`/mcp` + `/terminal` + `/ws` + `/health` + 联邦入口 `/federation`，持有本侧串口 |
| **从实例** | 启动时探测到主实例（`127.0.0.1:5000/health`，WSL 侧加探网关 IP） | 前台进程：上报本侧串口、受主实例调度读写本侧串口；本侧反代 `/mcp` + `/health` |

```bash
# Windows 侧先启动（WSL 从实例要跨侧发现，主实例必须监听所有接口）
serialhub.exe --host 0.0.0.0

# WSL 侧启动 → 自动检测到 Windows 主实例 → 从实例模式
./bin/serialhub
```

### 关键行为

- **端口聚合**：主实例 `serial_list` 返回双侧端口；联邦端口以 `side:port` 全名标识（如 `windows:COM3`），`serial_connect` 等工具自动路由。
- **数据上行**：从实例串口收到的数据实时上行至主实例，进入 xterm web 与 MCP 读缓冲，与本地端口无差别。
- **从实例退出**：`Ctrl+C` 退出即脱离联邦，主实例端口列表即时移除该侧端口。
- **自动晋升**：主实例退出后，从实例重连失败（3 次 × 1s）即自动晋升为主实例，`127.0.0.1:5000` 服务无缝恢复；期间对侧再启动则反向加入。
- **同侧多开**：同侧第二个实例以从实例运行，反代端口被主占用时仅贡献串口（日志有提示）。

### 部署前提与限制

- **WSL 从实例访问 Windows 主实例**：Windows 侧主实例需 `--host 0.0.0.0`（默认 `127.0.0.1` 时 WSL 探测不到，Windows 防火墙需放行 5000 端口）。
- **WSL 侧串口**：USB 串口设备需先 `usbipd attach` 到 WSL（枚举仅保留 `ttyUSB*`/`ttyACM*`，自动过滤 WSL 虚拟假端口）。usbipd attach 后 Windows 侧将暂时失去该设备。
- **双主竞态**：两侧在 1 秒内同时首启可能互探不到而形成双主（各自独立服务）。先后启动即可避免。
- **无鉴权**：`--host 0.0.0.0` 暴露到局域网时无任何鉴权，仅适用于可信网络；主实例建议保持 `127.0.0.1`（单侧使用时）。

---

## 典型工作流

### 1. 标准命令-响应模式

```mermaid
sequenceDiagram
    participant AI as AI 工具
    participant MCP as MCP 服务
    participant Serial as 串口
    participant MCU as MCU 设备

    AI->>MCP: serial_list
    MCP-->>AI: 返回可用串口列表
    AI->>MCP: serial_connect(port="COM4")
    MCP-->>AI: 连接成功
    AI->>MCP: serial_write(data="version")
    MCP->>Serial: 写入数据
    Serial->>MCU: UART 传输
    MCU-->>Serial: 响应数据
    Serial-->>MCP: 读取数据
    AI->>MCP: serial_read(timeout=3000)
    MCP-->>AI: 返回响应内容
    AI->>MCP: serial_disconnect
    MCP-->>AI: 断开成功
```

### 2. 持续监听模式

```mermaid
sequenceDiagram
    participant AI as AI 工具
    participant MCP as MCP 服务
    participant Buffer as MCP 缓冲区
    participant Serial as 串口

    AI->>MCP: serial_connect
    MCP-->>AI: 连接成功

    loop 持续监听
        AI->>MCP: serial_read(timeout=1000)
        alt 缓冲区有数据
            Buffer-->>MCP: 返回数据
            MCP-->>AI: 返回数据
        else 超时无数据
            MCP-->>AI: 返回超时
        end
    end

    AI->>MCP: serial_disconnect
```

```python
# 连接后循环读取
mcp_call(5000, "serial_connect", {"port": "COM4"})
while True:
    result = mcp_call(5000, "serial_read", {"timeout": 1000})
    if not result["result"]["content"][0]["text"].endswith("timedOut: true"):
        print("收到数据:", result)
```

### 3. 系统架构

```mermaid
flowchart TB
    subgraph Clients["客户端"]
        AI["AI 工具<br/>MCP HTTP"]
        Web["Web 终端<br/>浏览器"]
    end

    subgraph SerialHub["SerialHub<br/>端口 5000"]
        MCP["MCP 服务"]
        WS["WebSocket 服务"]
        Bridge["DataBridge"]
        Serial["串口"]
    end

    subgraph Device["设备"]
        MCU["MCU"]
    end

    AI <-->|HTTP/MCP| MCP
    Web <-->|WebSocket| WS
    MCP <-->|读/写| Bridge
    WS <-->|读/写| Bridge
    Bridge <-->|读/写| Serial
    Serial <-->|UART| MCU
```

**架构特点**:
- **双向通信**: MCP 和 Web 终端均可独立读写串口
- **数据共享**: 串口数据同时广播到所有客户端
- **故障隔离**: 任一端故障不影响其他端

---

## 错误处理

### 常见错误

| 错误码 | 场景 | 解决方案 |
|--------|------|----------|
| -32700 | JSON 解析错误 | 检查请求格式 |
| -32600 | 请求不是有效的 JSON-RPC 对象 | 检查请求结构 |
| -32601 | 工具名不存在 | 检查工具名拼写 |
| -32602 | 参数解析失败或参数无效 | 检查 arguments 格式 |
| `isError: true` | 工具执行失败（如串口未连接、写入失败） | 查看 `result.content` 中的错误描述，调用 `serial_connect` 后重试 |

> 说明：工具执行类错误（如"串口未连接"）通过 `result` 中的 `isError: true` 返回，而非 JSON-RPC 协议级错误，便于 LLM 感知失败并自我纠正。

### 错误响应示例

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32602,
    "message": "参数解析错误: json: cannot unmarshal ..."
  }
}
```

### 工具失败响应示例（isError）

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "串口未连接"
      }
    ],
    "isError": true
  }
}
```

---

## 相关文件

| 文件 | 说明 |
|------|------|
| `pkg/mcp/server.go` | MCP HTTP 服务实现 |
| `pkg/mcp/tools/*.go` | MCP 工具实现 |
| `pkg/bridge/bridge.go` | 数据桥接核心 |
| `tests/integration/test_serialhub.py` | Python 集成测试 |

---

## 参考链接

- [MCP 规范](https://modelcontextprotocol.io/)
- [项目架构](./AGENTS.md)
- [主文档](./README.md)
