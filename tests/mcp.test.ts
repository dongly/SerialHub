/**
 * MCP 工具测试
 */

import { describe, test, expect, beforeEach, afterEach, mock } from "bun:test";
import {
  SerialHubMCP,
  serialListTool,
  serialConnectTool,
  serialDisconnectTool,
  serialWriteTool,
  serialReadTool,
  serialStatusTool,
  executeSerialList,
  executeSerialConnect,
  executeSerialDisconnect,
  executeSerialWrite,
  executeSerialRead,
  executeSerialStatus,
  DataBuffer,
} from "../src/mcp/index.js";
import { SerialManager } from "../src/serial/SerialManager.js";

// Mock SerialPort
const mockSerialPort = {
  on: mock(() => {}),
  open: mock((callback: (error?: Error) => void) => callback()),
  close: mock((callback: (error?: Error) => void) => callback()),
  write: mock((data: Buffer, callback: (error?: Error) => void) => callback()),
  drain: mock((callback: (error?: Error) => void) => callback()),
};

// Mock SerialPort.list
const mockListPorts = mock(async () => [
  { path: "COM1", manufacturer: "Test", pnpId: "test-1" },
  { path: "COM2", manufacturer: "Test2", pnpId: "test-2" },
]);

// 创建 mock SerialManager
function createMockSerialManager(): SerialManager {
  const manager = {
    _isConnected: false,
    _currentPort: null as string | null,
    _config: {
      port: "",
      baudRate: 115200,
      dataBits: 8 as const,
      parity: "none" as const,
      stopBits: 1 as const,
    },
    _listeners: new Map<string, Set<(...args: unknown[]) => void>>(),

    get isConnected() {
      return this._isConnected;
    },
    get currentPort() {
      return this._currentPort;
    },
    on(event: string, listener: (...args: unknown[]) => void) {
      if (!this._listeners.has(event)) {
        this._listeners.set(event, new Set());
      }
      this._listeners.get(event)!.add(listener);
      return this;
    },
    off(event: string, listener: (...args: unknown[]) => void) {
      this._listeners.get(event)?.delete(listener);
      return this;
    },
    emit(event: string, ...args: unknown[]) {
      const listeners = this._listeners.get(event);
      if (listeners) {
        listeners.forEach((listener) => listener(...args));
      }
      return true;
    },
    async connect(portName?: string) {
      this._currentPort = portName ?? this._config.port;
      this._isConnected = true;
      this.emit("connected");
    },
    async disconnect() {
      this._isConnected = false;
      this._currentPort = null;
      this.emit("disconnected");
    },
    async write(data: Buffer | string) {
      if (!this._isConnected) {
        throw new Error("串口未连接");
      }
      const buffer = typeof data === "string" ? Buffer.from(data) : data;
      return buffer.length;
    },
    async writeLine(text: string, lineEnding = "\r\n") {
      return this.write(text + lineEnding);
    },
    updateConfig(config: Partial<typeof manager._config>) {
      this._config = { ...this._config, ...config };
    },
    getConfig() {
      return { ...this._config };
    },
    static: {
      listPorts: mockListPorts,
    },
  } as unknown as SerialManager;

  // 添加静态方法
  (SerialManager as unknown as Record<string, unknown>).listPorts = mockListPorts;

  return manager;
}

describe("工具定义", () => {
  test("serial_list 工具定义正确", () => {
    expect(serialListTool.name).toBe("serial_list");
    expect(serialListTool.description).toContain("串口");
    expect(serialListTool.inputSchema).toEqual({});
  });

  test("serial_connect 工具定义正确", () => {
    expect(serialConnectTool.name).toBe("serial_connect");
    expect(serialConnectTool.description).toContain("连接");
    expect(serialConnectTool.inputSchema).toHaveProperty("port");
    expect(serialConnectTool.inputSchema).toHaveProperty("baudRate");
  });

  test("serial_disconnect 工具定义正确", () => {
    expect(serialDisconnectTool.name).toBe("serial_disconnect");
    expect(serialDisconnectTool.description).toContain("断开");
    expect(serialDisconnectTool.inputSchema).toEqual({});
  });

  test("serial_write 工具定义正确", () => {
    expect(serialWriteTool.name).toBe("serial_write");
    expect(serialWriteTool.description).toContain("发送");
    expect(serialWriteTool.inputSchema).toHaveProperty("data");
    expect(serialWriteTool.inputSchema).toHaveProperty("addNewline");
  });

  test("serial_read 工具定义正确", () => {
    expect(serialReadTool.name).toBe("serial_read");
    expect(serialReadTool.description).toContain("读取");
    expect(serialReadTool.inputSchema).toHaveProperty("timeout");
    expect(serialReadTool.inputSchema).toHaveProperty("maxSize");
  });

  test("serial_status 工具定义正确", () => {
    expect(serialStatusTool.name).toBe("serial_status");
    expect(serialStatusTool.description).toContain("状态");
    expect(serialStatusTool.inputSchema).toEqual({});
  });
});

describe("DataBuffer", () => {
  let buffer: DataBuffer;

  beforeEach(() => {
    buffer = new DataBuffer(100); // 小缓冲区便于测试溢出
  });

  test("初始状态为空", () => {
    expect(buffer.length).toBe(0);
  });

  test("追加数据后长度增加", () => {
    buffer.append(Buffer.from("hello"));
    expect(buffer.length).toBe(5);
  });

  test("读取数据后缓冲区清空", () => {
    buffer.append(Buffer.from("hello"));
    const data = buffer.read();
    expect(data.toString()).toBe("hello");
    expect(buffer.length).toBe(0);
  });

  test("读取部分数据", () => {
    buffer.append(Buffer.from("hello world"));
    const data = buffer.read(5);
    expect(data.toString()).toBe("hello");
    expect(buffer.length).toBe(6);
  });

  test("peek 不清空缓冲区", () => {
    buffer.append(Buffer.from("hello"));
    const data = buffer.peek();
    expect(data.toString()).toBe("hello");
    expect(buffer.length).toBe(5);
  });

  test("clear 清空缓冲区", () => {
    buffer.append(Buffer.from("hello"));
    buffer.clear();
    expect(buffer.length).toBe(0);
  });

  test("溢出时丢弃旧数据", () => {
    buffer.append(Buffer.from("a".repeat(60)));
    buffer.append(Buffer.from("b".repeat(60)));
    // 60 + 60 = 120 > 100, 应该丢弃前 20 个字节
    // 结果: 40 个 'a' + 60 个 'b' = 100 字节
    expect(buffer.length).toBe(100);
    const data = buffer.read();
    // 前 20 个 'a' 被丢弃，剩余 40 个 'a' 在前面
    expect(data.toString().startsWith("a")).toBe(true);
    expect(data.toString().endsWith("b")).toBe(true);
    // 验证总长度
    expect(data.length).toBe(100);
  });
});

describe("executeSerialConnect", () => {
  let mockManager: SerialManager;

  beforeEach(() => {
    mockManager = createMockSerialManager();
  });

  test("连接成功", async () => {
    const result = await executeSerialConnect(mockManager, {
      port: "COM9",
      baudRate: 115200,
    });
    expect(result.success).toBe(true);
    expect(result.port).toBe("COM9");
    expect(result.baudRate).toBe(115200);
  });

  test("使用默认波特率", async () => {
    const result = await executeSerialConnect(mockManager, {
      port: "COM9",
    });
    expect(result.success).toBe(true);
    expect(result.baudRate).toBe(115200);
  });
});

describe("executeSerialDisconnect", () => {
  let mockManager: SerialManager;

  beforeEach(() => {
    mockManager = createMockSerialManager();
  });

  test("断开已连接的串口", async () => {
    // 先连接
    await mockManager.connect("COM9");
    expect(mockManager.isConnected).toBe(true);

    // 再断开
    const result = await executeSerialDisconnect(mockManager);
    expect(result.success).toBe(true);
    expect(mockManager.isConnected).toBe(false);
  });

  test("断开未连接的串口", async () => {
    const result = await executeSerialDisconnect(mockManager);
    expect(result.success).toBe(true);
    expect(result.message).toContain("未连接");
  });
});

describe("executeSerialWrite", () => {
  let mockManager: SerialManager;

  beforeEach(() => {
    mockManager = createMockSerialManager();
  });

  test("写入数据成功", async () => {
    await mockManager.connect("COM9");
    const result = await executeSerialWrite(mockManager, {
      data: "test",
      addNewline: false,
    });
    expect(result.success).toBe(true);
    expect(result.bytesWritten).toBe(4);
  });

  test("写入数据并添加换行", async () => {
    await mockManager.connect("COM9");
    const result = await executeSerialWrite(mockManager, {
      data: "test",
      addNewline: true,
    });
    expect(result.success).toBe(true);
    // "test\r\n" = 6 字节
    expect(result.bytesWritten).toBe(6);
  });

  test("未连接时写入失败", async () => {
    const result = await executeSerialWrite(mockManager, {
      data: "test",
    });
    expect(result.success).toBe(false);
    expect(result.message).toContain("未连接");
  });
});

describe("executeSerialRead", () => {
  let mockManager: SerialManager;
  let dataBuffer: DataBuffer;

  beforeEach(() => {
    mockManager = createMockSerialManager();
    dataBuffer = new DataBuffer();
  });

  test("读取缓冲区数据", async () => {
    await mockManager.connect("COM9");
    dataBuffer.append(Buffer.from("test data"));

    const result = await executeSerialRead(mockManager, dataBuffer, {
      timeout: 100,
    });
    expect(result.data).toBe("test data");
    expect(result.bytes).toBe(9);
    expect(result.encoding).toBe("utf-8");
  });

  test("缓冲区为空时等待超时", async () => {
    await mockManager.connect("COM9");

    const result = await executeSerialRead(mockManager, dataBuffer, {
      timeout: 100,
    });
    expect(result.data).toBe("");
    expect(result.bytes).toBe(0);
  });

  test("未连接时返回空数据", async () => {
    const result = await executeSerialRead(mockManager, dataBuffer, {
      timeout: 100,
    });
    expect(result.data).toBe("");
    expect(result.bytes).toBe(0);
  });
});

describe("executeSerialStatus", () => {
  let mockManager: SerialManager;

  beforeEach(() => {
    mockManager = createMockSerialManager();
  });

  test("获取已连接状态", async () => {
    await mockManager.connect("COM9");
    const result = await executeSerialStatus(mockManager);
    expect(result.connected).toBe(true);
    expect(result.port).toBe("COM9");
    expect(result.baudRate).toBe(115200);
  });

  test("获取未连接状态", async () => {
    const result = await executeSerialStatus(mockManager);
    expect(result.connected).toBe(false);
  });
});

describe("SerialHubMCP", () => {
  let mockManager: SerialManager;
  let mcp: SerialHubMCP;

  beforeEach(() => {
    mockManager = createMockSerialManager();
    mcp = new SerialHubMCP(mockManager);
  });

  afterEach(() => {
    mcp.dispose();
  });

  test("创建 MCP 服务实例", () => {
    expect(mcp).toBeDefined();
    expect(mcp.getServer()).toBeDefined();
    expect(mcp.getSerialManager()).toBe(mockManager);
    expect(mcp.getDataBuffer()).toBeDefined();
  });

  test("数据监听器工作正常", () => {
    const buffer = mcp.getDataBuffer();
    mockManager.emit("data", Buffer.from("test"));
    expect(buffer.length).toBe(4);
  });

  test("dispose 清理资源", () => {
    mockManager.emit("data", Buffer.from("test"));
    expect(mcp.getDataBuffer().length).toBe(4);

    mcp.dispose();

    expect(mcp.getDataBuffer().length).toBe(0);
    // dispose 后不再接收数据
    mockManager.emit("data", Buffer.from("more"));
    expect(mcp.getDataBuffer().length).toBe(0);
  });
});
