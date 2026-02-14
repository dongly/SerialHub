/**
 * MCP HTTP+SSE 传输层
 * 使用 StreamableHTTPServerTransport 实现
 */

import { createServer, IncomingMessage, ServerResponse, Server as HttpServer } from "node:http";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { randomUUID } from "node:crypto";

/**
 * HTTP 服务配置
 */
export interface HttpServerConfig {
  /** 监听端口，默认 3000 */
  port?: number;
  /** 主机地址，默认 127.0.0.1 */
  host?: string;
  /** 是否启用 CORS，默认 true */
  enableCors?: boolean;
  /** CORS 允许的来源，默认 "*" */
  corsOrigin?: string | string[];
}

/**
 * HTTP 服务结果
 */
export interface HttpServerResult {
  /** HTTP 服务器实例 */
  server: HttpServer;
  /** 传输实例 */
  transport: StreamableHTTPServerTransport;
  /** 关闭服务的函数 */
  close: () => Promise<void>;
}

/**
 * CORS 头配置
 */
function setCorsHeaders(
  res: ServerResponse,
  origin: string | string[] = "*"
): void {
  const allowOrigin = Array.isArray(origin)
    ? origin.join(", ")
    : origin;

  res.setHeader("Access-Control-Allow-Origin", allowOrigin);
  res.setHeader("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
  res.setHeader("Access-Control-Allow-Headers", "Content-Type, Authorization");
  res.setHeader("Access-Control-Max-Age", "86400");
}

/**
 * 处理 OPTIONS 预检请求
 */
function handleOptions(req: IncomingMessage, res: ServerResponse, corsOrigin: string | string[]): boolean {
  if (req.method === "OPTIONS") {
    setCorsHeaders(res, corsOrigin);
    res.writeHead(204);
    res.end();
    return true;
  }
  return false;
}

/**
 * 处理健康检查请求
 */
function handleHealthCheck(req: IncomingMessage, res: ServerResponse): boolean {
  const url = new URL(req.url || "/", `http://${req.headers.host}`);
  
  if (url.pathname === "/health" && req.method === "GET") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ status: "ok", timestamp: new Date().toISOString() }));
    return true;
  }
  return false;
}

/**
 * 解析请求体
 */
async function parseBody(req: IncomingMessage): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    
    req.on("data", (chunk: Buffer) => {
      chunks.push(chunk);
    });
    
    req.on("end", () => {
      const body = Buffer.concat(chunks).toString("utf-8");
      if (!body) {
        resolve(undefined);
        return;
      }
      
      try {
        resolve(JSON.parse(body));
      } catch {
        resolve(body);
      }
    });
    
    req.on("error", reject);
  });
}

/**
 * 发送错误响应
 */
function sendError(res: ServerResponse, statusCode: number, message: string): void {
  res.writeHead(statusCode, { "Content-Type": "application/json" });
  res.end(JSON.stringify({ error: message }));
}

/**
 * 创建 MCP HTTP 服务器
 * @param mcpServer MCP Server 实例
 * @param config HTTP 服务配置
 * @returns 服务结果
 */
export async function createHttpServer(
  mcpServer: McpServer,
  config: HttpServerConfig = {}
): Promise<HttpServerResult> {
  const {
    port = 3000,
    host = "127.0.0.1",
    enableCors = true,
    corsOrigin = "*",
  } = config;

  // 创建 StreamableHTTP 传输
  // 使用 stateless 模式（不保持会话状态）
  // 启用 JSON 响应模式（适合简单请求/响应场景）
  const transport = new StreamableHTTPServerTransport({
    sessionIdGenerator: undefined, // stateless 模式
    enableJsonResponse: true, // 启用 JSON 响应
  });

  // 连接 MCP Server 和传输
  await mcpServer.connect(transport);

  // 创建 HTTP 服务器
  const server = createServer(async (req: IncomingMessage, res: ServerResponse) => {
    // 处理 CORS 预检
    if (enableCors && handleOptions(req, res, corsOrigin)) {
      return;
    }

    // 设置 CORS 头
    if (enableCors) {
      setCorsHeaders(res, corsOrigin);
    }

    // 处理健康检查
    if (handleHealthCheck(req, res)) {
      return;
    }

    // 所有 MCP 请求都通过 transport 处理
    // StreamableHTTPServerTransport 支持所有路径
    try {
      // 解析请求体（仅 POST 请求）
      let parsedBody: unknown;
      if (req.method === "POST") {
        parsedBody = await parseBody(req);
      }

      console.error(`[SerialHub] MCP 请求: ${req.method} ${req.url}`, parsedBody);

      // 让 transport 处理请求
      await transport.handleRequest(req, res, parsedBody);
    } catch (error) {
      console.error("[SerialHub] 处理请求错误:", error);
      if (!res.headersSent) {
        sendError(res, 500, `Internal Server Error: ${error instanceof Error ? error.message : String(error)}`);
      }
    }
  });

  // 返回结果
  const result: HttpServerResult = {
    server,
    transport,
    close: async () => {
      await transport.close();
      return new Promise((resolve, reject) => {
        server.close((err) => {
          if (err) {
            reject(err);
          } else {
            resolve();
          }
        });
      });
    },
  };

  return new Promise((resolve, reject) => {
    server.listen(port, host, () => {
      console.error(`[SerialHub] HTTP 服务已启动: http://${host}:${port}`);
      resolve(result);
    });

    server.on("error", (err) => {
      console.error("[SerialHub] HTTP 服务错误:", err);
      reject(err);
    });
  });
}

/**
 * 创建 MCP HTTP 服务器（有状态模式）
 * @param mcpServer MCP Server 实例
 * @param config HTTP 服务配置
 * @returns 服务结果
 */
export async function createHttpServerStateful(
  mcpServer: McpServer,
  config: HttpServerConfig = {}
): Promise<HttpServerResult> {
  const {
    port = 3000,
    host = "127.0.0.1",
    enableCors = true,
    corsOrigin = "*",
  } = config;

  // 创建有状态的传输
  const transport = new StreamableHTTPServerTransport({
    sessionIdGenerator: () => randomUUID(), // 有状态模式
    enableJsonResponse: true, // 启用 JSON 响应
  });

  // 连接 MCP Server 和传输
  await mcpServer.connect(transport);

  // 存储会话和传输的映射
  const sessions = new Map<string, StreamableHTTPServerTransport>();

  // 创建 HTTP 服务器
  const server = createServer(async (req: IncomingMessage, res: ServerResponse) => {
    // 处理 CORS 预检
    if (enableCors && handleOptions(req, res, corsOrigin)) {
      return;
    }

    // 设置 CORS 头
    if (enableCors) {
      setCorsHeaders(res, corsOrigin);
    }

    // 处理健康检查
    if (handleHealthCheck(req, res)) {
      return;
    }

    try {
      // 解析请求体
      let parsedBody: unknown;
      if (req.method === "POST") {
        parsedBody = await parseBody(req);
      }

      // 获取会话 ID（如果有）
      const sessionId = req.headers["mcp-session-id"] as string | undefined;

      // 如果有会话 ID，使用对应的传输
      if (sessionId && sessions.has(sessionId)) {
        const sessionTransport = sessions.get(sessionId)!;
        await sessionTransport.handleRequest(req, res, parsedBody);
        return;
      }

      // 否则使用主传输
      await transport.handleRequest(req, res, parsedBody);

      // 如果传输分配了新会话 ID，保存它
      if (transport.sessionId && !sessions.has(transport.sessionId)) {
        sessions.set(transport.sessionId, transport);
      }
    } catch (error) {
      console.error("[SerialHub] 处理请求错误:", error);
      if (!res.headersSent) {
        sendError(res, 500, "Internal Server Error");
      }
    }
  });

  // 返回结果
  const result: HttpServerResult = {
    server,
    transport,
    close: async () => {
      // 关闭所有会话
      for (const sessionTransport of sessions.values()) {
        await sessionTransport.close();
      }
      sessions.clear();
      
      await transport.close();
      return new Promise((resolve, reject) => {
        server.close((err) => {
          if (err) {
            reject(err);
          } else {
            resolve();
          }
        });
      });
    },
  };

  return new Promise((resolve, reject) => {
    server.listen(port, host, () => {
      console.error(`[SerialHub] HTTP 服务已启动（有状态模式）: http://${host}:${port}`);
      resolve(result);
    });

    server.on("error", (err) => {
      console.error("[SerialHub] HTTP 服务错误:", err);
      reject(err);
    });
  });
}

// 导出类型
export { StreamableHTTPServerTransport };
