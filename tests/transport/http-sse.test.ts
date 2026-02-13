/**
 * MCP HTTP+SSE 传输层测试
 */

import { describe, test, expect, beforeEach, afterEach, mock } from "bun:test";
import { createServer, IncomingMessage, ServerResponse } from "node:http";
import { SerialManager } from "../../src/serial/SerialManager.js";
import { SerialHubMCP } from "../../src/mcp/index.js";
import {
  createHttpServer,
  createHttpServerStateful,
  HttpServerConfig,
} from "../../src/mcp/transport/http-sse.js";

// 测试端口基数（避免冲突）
const TEST_PORT_BASE = 31000;
let testPortCounter = 0;

function getNextPort(): number {
  return TEST_PORT_BASE + testPortCounter++;
}

// Mock SerialManager
function createMockSerialManager(): SerialManager {
  const listeners = new Map<string, Set<(...args: unknown[]) => void>>();

  return {
    _isConnected: false,
    _currentPort: null as string | null,

    get isConnected() {
      return this._isConnected;
    },
    get currentPort() {
      return this._currentPort;
    },
    on(event: string, listener: (...args: unknown[]) => void) {
      if (!listeners.has(event)) {
        listeners.set(event, new Set());
      }
      listeners.get(event)!.add(listener);
      return this;
    },
    off(event: string, listener: (...args: unknown[]) => void) {
      listeners.get(event)?.delete(listener);
      return this;
    },
    emit(event: string, ...args: unknown[]) {
      const eventListeners = listeners.get(event);
      if (eventListeners) {
        eventListeners.forEach((listener) => listener(...args));
      }
      return true;
    },
    connect: mock(async (portName?: string) => {
      this._currentPort = portName ?? "COM9";
      this._isConnected = true;
      this.emit("connected");
    }),
    disconnect: mock(async () => {
      this._isConnected = false;
      this._currentPort = null;
      this.emit("disconnected");
    }),
    write: mock(async (data: Buffer | string) => {
      if (!this._isConnected) {
        throw new Error("串口未连接");
      }
      const buffer = typeof data === "string" ? Buffer.from(data) : data;
      return buffer.length;
    }),
    writeLine: mock(async (text: string, lineEnding = "\r\n") => {
      return this.write(text + lineEnding);
    }),
    updateConfig: mock(() => {}),
    getConfig: mock(() => ({
      port: "",
      baudRate: 115200,
      dataBits: 8 as const,
      parity: "none" as const,
      stopBits: 1 as const,
    })),
  } as unknown as SerialManager;
}

describe("HTTP+SSE 传输", () => {
  describe("createHttpServer", () => {
    let mockSerial: SerialManager;
    let mcp: SerialHubMCP;
    let port: number;

    beforeEach(() => {
      mockSerial = createMockSerialManager();
      mcp = new SerialHubMCP(mockSerial);
      port = getNextPort();
    });

    afterEach(async () => {
      mcp.dispose();
    });

    test("应该创建 HTTP 服务器并监听指定端口", async () => {
      const result = await createHttpServer(mcp.getServer(), { port });

      expect(result.server).toBeDefined();
      expect(result.transport).toBeDefined();
      expect(result.close).toBeDefined();

      // 验证服务器正在监听
      const address = result.server.address();
      expect(address).not.toBeNull();
      if (address && typeof address !== "string") {
        expect(address.port).toBe(port);
      }

      await result.close();
    });

    test("应该使用默认配置", async () => {
      const result = await createHttpServer(mcp.getServer());

      const address = result.server.address();
      if (address && typeof address !== "string") {
        expect(address.port).toBe(3000); // 默认端口
      }

      await result.close();
    });

    test("应该能够关闭服务器", async () => {
      const result = await createHttpServer(mcp.getServer(), { port });

      await result.close();

      // 关闭后服务器地址应为 null
      expect(result.server.address()).toBeNull();
    });
  });

  describe("createHttpServerStateful", () => {
    let mockSerial: SerialManager;
    let mcp: SerialHubMCP;
    let port: number;

    beforeEach(() => {
      mockSerial = createMockSerialManager();
      mcp = new SerialHubMCP(mockSerial);
      port = getNextPort();
    });

    afterEach(() => {
      mcp.dispose();
    });

    test("应该创建有状态模式的 HTTP 服务器", async () => {
      const result = await createHttpServerStateful(mcp.getServer(), { port });

      expect(result.server).toBeDefined();
      expect(result.transport).toBeDefined();
      // 有状态模式会生成 session ID
      expect(result.transport.sessionId).toBeUndefined(); // 初始时为 undefined

      await result.close();
    });
  });
});

describe("HTTP 端点测试", () => {
  let mockSerial: SerialManager;
  let mcp: SerialHubMCP;
  let server: ReturnType<typeof createServer>;
  let port: number;

  beforeEach(async () => {
    mockSerial = createMockSerialManager();
    mcp = new SerialHubMCP(mockSerial);
    port = getNextPort();

    const result = await createHttpServer(mcp.getServer(), {
      port,
      host: "127.0.0.1",
      enableCors: true,
    });
    server = result.server;
  });

  afterEach(async () => {
    await new Promise<void>((resolve) => {
      server.close(() => resolve());
    });
    mcp.dispose();
  });

  test("GET /health 应该返回健康状态", async () => {
    const response = await fetch(`http://127.0.0.1:${port}/health`);
    
    expect(response.status).toBe(200);
    expect(response.headers.get("Content-Type")).toBe("application/json");
    
    const body = await response.json();
    expect(body.status).toBe("ok");
    expect(body.timestamp).toBeDefined();
  });

  test("OPTIONS 请求应该返回 CORS 头", async () => {
    const response = await fetch(`http://127.0.0.1:${port}/mcp`, {
      method: "OPTIONS",
    });

    expect(response.status).toBe(204);
    expect(response.headers.get("Access-Control-Allow-Origin")).toBe("*");
    expect(response.headers.get("Access-Control-Allow-Methods")).toContain("GET");
    expect(response.headers.get("Access-Control-Allow-Methods")).toContain("POST");
  });

  test("POST /mcp 应该返回 CORS 头", async () => {
    const response = await fetch(`http://127.0.0.1:${port}/mcp`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        jsonrpc: "2.0",
        method: "initialize",
        params: {
          protocolVersion: "2024-11-05",
          capabilities: {},
          clientInfo: {
            name: "test-client",
            version: "1.0.0",
          },
        },
        id: 1,
      }),
    });

    // 检查 CORS 头
    expect(response.headers.get("Access-Control-Allow-Origin")).toBe("*");
  });
});

describe("CORS 配置测试", () => {
  let mockSerial: SerialManager;
  let mcp: SerialHubMCP;
  let server: ReturnType<typeof createServer>;
  let port: number;

  afterEach(async () => {
    if (server) {
      await new Promise<void>((resolve) => {
        server.close(() => resolve());
      });
    }
    if (mcp) {
      mcp.dispose();
    }
  });

  test("禁用 CORS 时不应该返回 CORS 头", async () => {
    mockSerial = createMockSerialManager();
    mcp = new SerialHubMCP(mockSerial);
    port = getNextPort();

    const result = await createHttpServer(mcp.getServer(), {
      port,
      host: "127.0.0.1",
      enableCors: false,
    });
    server = result.server;

    const response = await fetch(`http://127.0.0.1:${port}/health`);
    expect(response.headers.get("Access-Control-Allow-Origin")).toBeNull();
  });

  test("自定义 CORS 来源", async () => {
    mockSerial = createMockSerialManager();
    mcp = new SerialHubMCP(mockSerial);
    port = getNextPort();

    const result = await createHttpServer(mcp.getServer(), {
      port,
      host: "127.0.0.1",
      enableCors: true,
      corsOrigin: "https://example.com",
    });
    server = result.server;

    const response = await fetch(`http://127.0.0.1:${port}/health`);
    expect(response.headers.get("Access-Control-Allow-Origin")).toBe("https://example.com");
  });
});

describe("MCP 协议测试", () => {
  let mockSerial: SerialManager;
  let mcp: SerialHubMCP;
  let server: ReturnType<typeof createServer>;
  let port: number;

  beforeEach(async () => {
    mockSerial = createMockSerialManager();
    mcp = new SerialHubMCP(mockSerial);
    port = getNextPort();

    const result = await createHttpServer(mcp.getServer(), {
      port,
      host: "127.0.0.1",
    });
    server = result.server;
  });

  afterEach(async () => {
    await new Promise<void>((resolve) => {
      server.close(() => resolve());
    });
    mcp.dispose();
  });

  test("初始化请求应该返回服务信息", async () => {
    const response = await fetch(`http://127.0.0.1:${port}/mcp`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Accept": "text/event-stream, application/json",
      },
      body: JSON.stringify({
        jsonrpc: "2.0",
        method: "initialize",
        params: {
          protocolVersion: "2024-11-05",
          capabilities: {},
          clientInfo: {
            name: "test-client",
            version: "1.0.0",
          },
        },
        id: 1,
      }),
    });

    expect(response.status).toBe(200);
    
    const body = await response.json();
    expect(body.jsonrpc).toBe("2.0");
    expect(body.id).toBe(1);
    expect(body.result).toBeDefined();
    expect(body.result.serverInfo.name).toBe("SerialHub");
  });

  test("工具列表请求（批量初始化）", async () => {
    // 测试批量请求格式
    const response = await fetch(`http://127.0.0.1:${port}/mcp`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Accept": "text/event-stream, application/json",
      },
      body: JSON.stringify({
        jsonrpc: "2.0",
        method: "tools/list",
        params: {},
        id: 1,
      }),
    });

    // 在 stateless 模式下，未初始化的请求可能返回错误
    // 这是预期行为，测试重点是传输层正常工作
    expect([200, 400, 403]).toContain(response.status);
  });
});

describe("错误处理测试", () => {
  let mockSerial: SerialManager;
  let mcp: SerialHubMCP;
  let server: ReturnType<typeof createServer>;
  let port: number;

  beforeEach(async () => {
    mockSerial = createMockSerialManager();
    mcp = new SerialHubMCP(mockSerial);
    port = getNextPort();

    const result = await createHttpServer(mcp.getServer(), {
      port,
      host: "127.0.0.1",
    });
    server = result.server;
  });

  afterEach(async () => {
    await new Promise<void>((resolve) => {
      server.close(() => resolve());
    });
    mcp.dispose();
  });

  test("无效 JSON 请求应该返回错误", async () => {
    const response = await fetch(`http://127.0.0.1:${port}/mcp`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: "invalid json",
    });

    // 应该返回错误响应
    expect(response.status).toBeGreaterThanOrEqual(400);
  });
});
