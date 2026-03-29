import { createServer, IncomingMessage, ServerResponse, Server as HttpServer } from "node:http";
import { SerialHubMCP } from "../index.js";
import { z } from "zod";

import type { ToolDef } from "../index.js";

export interface HttpServerConfig {
  port?: number;
  host?: string;
  enableCors?: boolean;
  corsOrigin?: string | string[];
}

export interface HttpServerResult {
  server: HttpServer;
  close: () => Promise<void>;
}

interface JsonRpcRequest {
  jsonrpc: "2.0";
  method: string;
  params?: Record<string, unknown>;
  id?: string | number | null;
}

interface JsonRpcSuccessResponse {
  jsonrpc: "2.0";
  result: unknown;
  id: string | number | null;
}

interface JsonRpcErrorResponse {
  jsonrpc: "2.0";
  error: { code: number; message: string };
  id: string | number | null;
}

type JsonRpcResponse = JsonRpcSuccessResponse | JsonRpcErrorResponse;

function setCorsHeaders(res: ServerResponse, origin: string | string[] = "*"): void {
  res.setHeader("Access-Control-Allow-Origin", Array.isArray(origin) ? origin.join(", ") : origin);
  res.setHeader("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
  res.setHeader("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Session-Id");
  res.setHeader("Access-Control-Max-Age", "86400");
}

function parseBody(req: IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    req.on("data", (chunk: Buffer) => chunks.push(chunk));
    req.on("end", () => resolve(Buffer.concat(chunks).toString("utf-8")));
    req.on("error", reject);
  });
}

function jsonResponse(res: ServerResponse, status: number, body: unknown): void {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
}

export async function createMcpHttpServer(
  mcp: SerialHubMCP,
  config: HttpServerConfig = {}
): Promise<HttpServerResult> {
  const { port = 5000, host = "127.0.0.1", enableCors = true, corsOrigin = "*" } = config;
  const tools = mcp.getToolsList();

  const server = createServer(async (req: IncomingMessage, res: ServerResponse) => {
    if (enableCors && req.method === "OPTIONS") {
      setCorsHeaders(res, corsOrigin);
      res.writeHead(204);
      res.end();
      return;
    }

    if (enableCors) setCorsHeaders(res, corsOrigin);

    const url = new URL(req.url || "/", `http://${req.headers.host}`);

    if (url.pathname === "/health" && req.method === "GET") {
      jsonResponse(res, 200, { status: "ok", timestamp: new Date().toISOString() });
      return;
    }

    if (url.pathname === "/shutdown" && req.method === "POST") {
      jsonResponse(res, 200, { status: "shutting down" });
      process.kill(process.pid, "SIGTERM");
      return;
    }

    if (url.pathname === "/mcp" && req.method === "POST") {
      await handleHttpRequest(mcp, tools, req, res);
      return;
    }

    jsonResponse(res, 404, { error: "Not found" });
  });

  return new Promise<HttpServerResult>((resolve, reject) => {
    server.listen(port, host, () => {
      console.error(`[SerialHub] MCP HTTP 服务已启动: http://${host}:${port}`);
      resolve({ server, close: () => new Promise((res, rej) => server.close(e => e ? rej(e) : res())) });
    });
    server.on("error", reject);
  });
}

async function handleHttpRequest(
  mcp: SerialHubMCP,
  tools: ToolDef[],
  req: IncomingMessage,
  res: ServerResponse
): Promise<void> {
  let body: string;
  try {
    body = await parseBody(req);
  } catch {
    jsonResponse(res, 400, { error: "Invalid request body" });
    return;
  }

  let parsed: JsonRpcRequest;
  try {
    parsed = JSON.parse(body);
  } catch {
    jsonResponse(res, 200, { jsonrpc: "2.0", error: { code: -32700, message: "Parse error" }, id: null });
    return;
  }

  if (!parsed.jsonrpc || !parsed.method) {
    jsonResponse(res, 200, { jsonrpc: "2.0", error: { code: -32600, message: "Invalid Request" }, id: parsed?.id ?? null });
    return;
  }

  try {
    const result = await handleJsonRpc(mcp, tools, parsed);
    jsonResponse(res, 200, result);
  } catch (e) {
    jsonResponse(res, 200, {
      jsonrpc: "2.0",
      error: { code: -32603, message: e instanceof Error ? e.message : String(e) },
      id: parsed.id ?? null,
    });
  }
}

async function handleJsonRpc(
  mcp: SerialHubMCP,
  tools: ToolDef[],
  msg: JsonRpcRequest
): Promise<JsonRpcResponse> {
  const { method, params, id } = msg;
  const rpcId = id ?? null;

  switch (method) {
    case "initialize":
      return {
        jsonrpc: "2.0",
        result: {
          protocolVersion: "2024-11-05",
          capabilities: { tools: {} },
          serverInfo: { name: "SerialHub", version: "0.1.0" },
        },
        id: rpcId,
      };

    case "notifications/initialized":
      return { jsonrpc: "2.0", result: {}, id: rpcId };

    case "ping":
      return { jsonrpc: "2.0", result: {}, id: rpcId };

    case "tools/list":
      return {
        jsonrpc: "2.0",
        result: {
          tools: tools.map(t => ({
            name: t.name,
            description: t.description,
            inputSchema: buildInputSchema(t.inputSchema),
          })),
        },
        id: rpcId,
      };

    case "tools/call": {
      const toolName = params?.name as string;
      const toolArgs = (params?.arguments as Record<string, unknown>) ?? {};

      if (!toolName) {
        return { jsonrpc: "2.0", error: { code: -32602, message: "Missing tool name" }, id: rpcId };
      }

      const result = await mcp.callTool(toolName, toolArgs);
      return {
        jsonrpc: "2.0",
        result: {
          content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],
        },
        id: rpcId,
      };
    }

    default:
      return { jsonrpc: "2.0", error: { code: -32601, message: `Method not found: ${method}` }, id: rpcId };
  }
}

function buildInputSchema(schema: Record<string, z.ZodTypeAny>): Record<string, unknown> {
  if (!schema || Object.keys(schema).length === 0) {
    return { type: "object", properties: {} };
  }

  const properties: Record<string, unknown> = {};
  const required: string[] = [];

  for (const [key, val] of Object.entries(schema)) {
    const unwrapped = val instanceof z.ZodOptional ? val.unwrap() : val;
    const prop: Record<string, unknown> = {};

    if (unwrapped instanceof z.ZodString) prop.type = "string";
    else if (unwrapped instanceof z.ZodNumber) prop.type = "number";
    else if (unwrapped instanceof z.ZodBoolean) prop.type = "boolean";
    else if (unwrapped instanceof z.ZodArray) prop.type = "array";
    else prop.type = "string";

    const desc = (val as { description?: string }).description;
    if (desc) prop.description = desc;

    properties[key] = prop;

    if (!(val instanceof z.ZodOptional)) {
      required.push(key);
    }
  }

  return { type: "object", properties, required: required.length > 0 ? required : undefined };
}
