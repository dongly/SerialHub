# SerialHub MCP 规范符合性审计报告

审计对象：`pkg/mcp/`（server.go + tools/）+ `pkg/mcp/proxy.go` + `internal/federation/`
规范版本：[MCP 2025-06-18](https://modelcontextprotocol.io/specification/2025-06-18) · SDK：`github.com/modelcontextprotocol/go-sdk v1.4.1`
审计日期：2026-09-23

## 一、符合项（7）

| # | 规范条款 | 级别 | 实现情况 |
|---|---------|------|---------|
| 1 | 单一 endpoint 同时支持 POST+GET | MUST | `/mcp` 由 `NewStreamableHTTPHandler` 提供（server.go） |
| 2 | POST 响应 `Content-Type: application/json`（单 JSON 对象）或 `text/event-stream` 二选一 | MUST | `JSONResponse: true`，每请求返回单个 `application/json`；实测 initialize/tools/call 响应头与载荷合规 |
| 3 | 通知/响应输入接受则 `202 Accepted` 无 body | MUST | SDK Stateless 模式内置处理 |
| 4 | Origin 校验（防 DNS rebinding） | MUST | go-sdk v1.4.1 默认启用 localhost 保护与 cross-origin 防护（`StreamableHTTPOptions.DisableLocalhostProtection` 默认 false）；SerialHub 未禁用 |
| 5 | 本地运行只绑 localhost | SHOULD | 默认 `--host 127.0.0.1`；`0.0.0.0` 为用户显式选择（文档已警示） |
| 6 | 工具错误双机制（协议错误=JSON-RPC error / 执行错误=`isError:true`） | MUST | 参数解析失败→`jsonrpc.CodeInvalidParams`；执行失败→`ToolResult.IsError`（toolResultToMCPResult） |
| 7 | `inputSchema` 为 JSON Schema | MUST | 7 个工具 `AddTool` 均提供 object schema，SDK 注册时校验 |

## 二、审计发现与处置（4，已修复）

| # | 发现 | 违反条款 | 处置 |
|---|------|---------|------|
| 1 | 工具结果 TextContent 曾为 Go 格式化文本（`%v` 输出 `map[...]`），非 JSON 序列化 | Tools · Structured Content「SHOULD also return the serialized JSON in a TextContent block」 | `toolResultToMCPResult` 重写：成功结果 JSON 序列化（message+数据合并对象）；`StructuredContent` 透传 `result.Data` |
| 2 | `serial_read` timeout=0 无限等待不响应取消，客户端断开后 handler goroutine 永久空转 | 服务器健壮性 / 工具超时惯例（客户端 SHOULD 实现超时的对偶义务） | `ExecuteSerialRead` 全分支加 `case <-ctx.Done()`，取消即返回「读取已取消」 |
| 3 | CORS `Access-Control-Allow-Headers` 缺 `Mcp-Session-Id` 等头，浏览器类 MCP 客户端预检失败 | 传输互操作实践 | 头补全为 `Content-Type, Mcp-Session-Id, Mcp-Protocol-Version, Authorization, Last-Event-ID` |
| 4 | WSL 下自动打开浏览器失败（`xdg-open` 不存在，静默 Warn） | 无（功能缺陷） | `openBrowser` Linux 分支降级链 `xdg-open` → `wslview` → `cmd.exe /c start`，实测 WSL2 经 `cmd.exe` 成功弹出 Windows 浏览器 |

## 三、记录性偏离（2）

### 1. 鉴权（SHOULD，未满足）

规范建议 Streamable HTTP 服务实现鉴权。SerialHub 定位为**本机/可信局域网工具**，本期不实现 token/OAuth。风险边界：

- 默认绑定 `127.0.0.1`，仅本机可访问；
- `--host 0.0.0.0`（联邦跨侧/局域网接入所需）暴露无鉴权端点，**仅限可信网络**，文档已警示。

### 2. 传输选型：非流式 Stateless JSON（设计决策）

规范允许每个 POST 返回单 `application/json` 或 SSE 流，客户端 MUST 同时支持两者。SerialHub 选择**非流式**（`Stateless: true, JSONResponse: true`）：

- 工具面全为短请求（list/connect/write/read+timeout），无流式收益；
- 工具列表编译期注册，永不变化，无 `listChanged` 通知需求；
- 实时数据流由 xterm web 的 WebSocket 通道专职承担（双通道分工），流式 MCP 与之职责重叠；
- Stateless 免会话管理，断线重连零成本，`curl` 可直接调试。

未来若出现「AI 持续订阅串口流」类工具，改两个 bool 即可升级（SDK 原生支持）。

## 四、联邦与 stdio 的规范符合性

- **stdio 主实例**：`StdioTransport` 由 SDK 提供，stdout 仅承载 MCP 协议流（日志强制只写文件，错误提示走 stderr，符合 stdio transport 条款「server MAY write UTF-8 strings to its stderr for logging」）。
- **stdio 代理**：子进程在 stdio 与主实例 `/mcp` 间做 `jsonrpc.Message` 双向透明转发；主实例 Stateless 逐请求自洽，转发即无会话逐 POST，规范语义不变。
- **联邦通道**：`/federation` WebSocket 为 SerialHub 私有扩展（JSON-RPC 2.0 双向），不属于 MCP 规范管辖面；MCP 语义严格由 `/mcp` 端点保证。

## 五、结论

SerialHub MCP 接口**符合 MCP 2025-06-18 规范**的 MUST 级条款；审计发现的功能性问题已全部修复；两项偏离（鉴权、非流式）为明确定位下的设计决策，已记录。联邦与 stdio 扩展不破坏规范兼容性——任意合规 MCP 客户端（OpenCode/Claude/curl）均可无差别接入。
