/**
 * DataBridge 数据桥接模块测试
 */

import { describe, test, expect, beforeEach, afterEach, mock, spyOn } from "bun:test";
import { EventEmitter } from "events";
import { DataBridge, type DataBridgeOptions } from "../src/bridge/DataBridge";
import type { SerialManager } from "../src/serial/SerialManager";
import type { TelnetServer, TelnetClient } from "../src/telnet/TelnetServer";
import type { SerialHubMCP } from "../src/mcp/index";
import { DataBuffer } from "../src/mcp/tools/serial_read";

// Mock SerialManager
class MockSerialManager extends EventEmitter {
  private _isConnected: boolean = false;
  private _currentPort: string | null = null;

  get isConnected(): boolean {
    return this._isConnected;
  }

  get currentPort(): string | null {
    return this._currentPort;
  }

  async connect(portName?: string): Promise<void> {
    this._isConnected = true;
    this._currentPort = portName ?? "COM_MOCK";
    this.emit("connected");
  }

  async disconnect(): Promise<void> {
    this._isConnected = false;
    this._currentPort = null;
    this.emit("disconnected");
  }

  async write(data: Buffer | string): Promise<number> {
    const buffer = typeof data === "string" ? Buffer.from(data, "utf-8") : data;
    return buffer.length;
  }

  // 模拟收到串口数据
  simulateData(data: Buffer | string): void {
    const buffer = typeof data === "string" ? Buffer.from(data, "utf-8") : data;
    this.emit("data", buffer);
  }
}

// Mock TelnetServer
class MockTelnetServer extends EventEmitter {
  private _isRunning: boolean = false;
  private _port: number = 0;
  private broadcastData: Buffer[] = [];

  get isRunning(): boolean {
    return this._isRunning;
  }

  get port(): number {
    return this._port;
  }

  async start(port: number): Promise<void> {
    this._isRunning = true;
    this._port = port;
    this.emit("started");
  }

  async stop(): Promise<void> {
    this._isRunning = false;
    this._port = 0;
    this.emit("stopped");
  }

  broadcast(data: Buffer | string): void {
    const buffer = typeof data === "string" ? Buffer.from(data, "utf-8") : data;
    this.broadcastData.push(buffer);
  }

  getBroadcastData(): Buffer[] {
    return [...this.broadcastData];
  }

  clearBroadcastData(): void {
    this.broadcastData = [];
  }

  // 模拟客户端数据
  simulateClientData(data: Buffer | string, client?: TelnetClient): void {
    const buffer = typeof data === "string" ? Buffer.from(data, "utf-8") : data;
    const mockClient: TelnetClient = client ?? {
      id: "mock-client-id",
      socket: {} as any,
      remoteAddress: "127.0.0.1:12345",
      connectedAt: new Date(),
    };
    this.emit("data", buffer, mockClient);
  }
}

// Mock SerialHubMCP
class MockSerialHubMCP {
  private dataBuffer: DataBuffer;

  constructor() {
    this.dataBuffer = new DataBuffer();
  }

  getDataBuffer(): DataBuffer {
    return this.dataBuffer;
  }

  dispose(): void {
    this.dataBuffer.clear();
  }
}

describe("DataBridge", () => {
  let mockSerial: MockSerialManager;
  let mockTelnet: MockTelnetServer;
  let mockMCP: MockSerialHubMCP;

  beforeEach(() => {
    mockSerial = new MockSerialManager();
    mockTelnet = new MockTelnetServer();
    mockMCP = new MockSerialHubMCP();
  });

  describe("构造函数和基本属性", () => {
    test("应正确创建实例", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);

      expect(bridge).toBeInstanceOf(DataBridge);
      expect(bridge.isRunning).toBe(false);
    });

    test("应使用默认选项", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      const options = bridge.getOptions();

      expect(options.enableTelnet).toBe(true);
      expect(options.enableMCP).toBe(true);
      expect(options.debugLog).toBe(false);
    });

    test("应接受自定义选项", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP, {
        enableTelnet: false,
        enableMCP: false,
        debugLog: true,
      });
      const options = bridge.getOptions();

      expect(options.enableTelnet).toBe(false);
      expect(options.enableMCP).toBe(false);
      expect(options.debugLog).toBe(true);
    });

    test("应接受 null Telnet 和 MCP", () => {
      const bridge = new DataBridge(mockSerial, null, null);

      expect(bridge).toBeInstanceOf(DataBridge);
      expect(bridge.isRunning).toBe(false);
    });
  });

  describe("start 和 stop", () => {
    test("启动后 isRunning 应为 true", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);

      bridge.start();

      expect(bridge.isRunning).toBe(true);
    });

    test("停止后 isRunning 应为 false", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);

      bridge.start();
      bridge.stop();

      expect(bridge.isRunning).toBe(false);
    });

    test("重复启动不应出错", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);

      bridge.start();
      bridge.start(); // 重复启动

      expect(bridge.isRunning).toBe(true);
    });

    test("重复停止不应出错", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);

      bridge.start();
      bridge.stop();
      bridge.stop(); // 重复停止

      expect(bridge.isRunning).toBe(false);
    });

    test("未启动时停止不应出错", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);

      bridge.stop(); // 未启动就停止

      expect(bridge.isRunning).toBe(false);
    });
  });

  describe("事件", () => {
    test("启动时应发出 started 事件", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      let started = false;

      bridge.on("started", () => {
        started = true;
      });

      bridge.start();

      expect(started).toBe(true);
    });

    test("停止时应发出 stopped 事件", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      let stopped = false;

      bridge.on("stopped", () => {
        stopped = true;
      });

      bridge.start();
      bridge.stop();

      expect(stopped).toBe(true);
    });
  });

  describe("串口数据转发", () => {
    test("串口数据应转发到 Telnet", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      bridge.start();

      mockSerial.simulateData("test data");

      const broadcastData = mockTelnet.getBroadcastData();
      expect(broadcastData.length).toBe(1);
      expect(broadcastData[0].toString()).toBe("test data");
    });

    test("串口数据应转发到 MCP 缓冲区", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      bridge.start();

      mockSerial.simulateData("test data");

      const buffer = mockMCP.getDataBuffer();
      expect(buffer.length).toBe(9); // "test data" 的长度
    });

    test("应发出 serial-data 事件", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      bridge.start();

      let receivedData: Buffer | null = null;
      let direction: string | null = null;

      bridge.on("serial-data", (data, dir) => {
        receivedData = data;
        direction = dir;
      });

      mockSerial.simulateData("test");

      expect(receivedData?.toString()).toBe("test");
      expect(direction).toBe("in");
    });

    test("应发出 forward 事件（serial -> telnet）", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      bridge.start();

      const forwards: { from: string; to: string }[] = [];

      bridge.on("forward", (data, f, t) => {
        forwards.push({ from: f, to: t });
      });

      mockSerial.simulateData("test");

      // 验证包含 serial -> telnet 的转发
      const telnetForward = forwards.find((f) => f.to === "telnet");
      expect(telnetForward).toBeDefined();
      expect(telnetForward?.from).toBe("serial");
    });

    test("应发出 forward 事件（serial -> mcp）", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      bridge.start();

      const forwards: { from: string; to: string }[] = [];

      bridge.on("forward", (data, f, t) => {
        forwards.push({ from: f, to: t });
      });

      mockSerial.simulateData("test");

      // 应该有两个 forward 事件：serial->telnet 和 serial->mcp
      expect(forwards.length).toBe(2);
      expect(forwards.map((f) => f.to)).toContain("telnet");
      expect(forwards.map((f) => f.to)).toContain("mcp");
    });

    test("禁用 Telnet 时不转发到 Telnet", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP, {
        enableTelnet: false,
      });
      bridge.start();

      mockSerial.simulateData("test");

      const broadcastData = mockTelnet.getBroadcastData();
      expect(broadcastData.length).toBe(0);
    });

    test("禁用 MCP 时不转发到 MCP", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP, {
        enableMCP: false,
      });
      bridge.start();

      mockSerial.simulateData("test");

      const buffer = mockMCP.getDataBuffer();
      expect(buffer.length).toBe(0);
    });

    test("Telnet 为 null 时不应出错", () => {
      const bridge = new DataBridge(mockSerial, null, mockMCP);
      bridge.start();

      expect(() => {
        mockSerial.simulateData("test");
      }).not.toThrow();
    });

    test("MCP 为 null 时不应出错", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, null);
      bridge.start();

      expect(() => {
        mockSerial.simulateData("test");
      }).not.toThrow();
    });
  });

  describe("Telnet 数据转发", () => {
    test("Telnet 数据应转发到串口", async () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      bridge.start();

      // 连接串口
      await mockSerial.connect();

      // 监听 write 调用
      const writeSpy = spyOn(mockSerial, "write");

      mockTelnet.simulateClientData("telnet test");

      expect(writeSpy).toHaveBeenCalled();

      writeSpy.mockRestore();
    });

    test("应发出 telnet-data 事件", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      bridge.start();

      let receivedData: Buffer | null = null;
      let receivedClient: TelnetClient | null = null;

      bridge.on("telnet-data", (data, client) => {
        receivedData = data;
        receivedClient = client;
      });

      mockTelnet.simulateClientData("test");

      expect(receivedData?.toString()).toBe("test");
      expect(receivedClient?.id).toBe("mock-client-id");
    });

    test("应发出 forward 事件（telnet -> serial）", async () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      bridge.start();

      await mockSerial.connect();

      let from = "";
      let to = "";

      bridge.on("forward", (data, f, t) => {
        from = f;
        to = t;
      });

      mockTelnet.simulateClientData("test");

      expect(from).toBe("telnet");
      expect(to).toBe("serial");
    });

    test("串口未连接时不转发", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      bridge.start();

      // 不连接串口

      let forwarded = false;
      bridge.on("forward", (data, f, t) => {
        if (f === "telnet" && t === "serial") {
          forwarded = true;
        }
      });

      mockTelnet.simulateClientData("test");

      expect(forwarded).toBe(false);
    });

    test("禁用 Telnet 时不应监听 Telnet 数据", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP, {
        enableTelnet: false,
      });
      bridge.start();

      let telnetDataReceived = false;
      bridge.on("telnet-data", () => {
        telnetDataReceived = true;
      });

      mockTelnet.simulateClientData("test");

      expect(telnetDataReceived).toBe(false);
    });
  });

  describe("停止后的行为", () => {
    test("停止后串口数据不应转发", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      bridge.start();
      bridge.stop();

      mockSerial.simulateData("test");

      const broadcastData = mockTelnet.getBroadcastData();
      expect(broadcastData.length).toBe(0);
    });

    test("停止后 Telnet 数据不应转发", async () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);
      await mockSerial.connect();
      bridge.start();
      bridge.stop();

      const writeSpy = spyOn(mockSerial, "write");

      mockTelnet.simulateClientData("test");

      expect(writeSpy).not.toHaveBeenCalled();

      writeSpy.mockRestore();
    });

    test("重新启动后应正常工作", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);

      // 第一次启动
      bridge.start();
      mockSerial.simulateData("first");
      bridge.stop();

      // 清除之前的数据
      mockTelnet.clearBroadcastData();

      // 重新启动
      bridge.start();
      mockSerial.simulateData("second");

      const broadcastData = mockTelnet.getBroadcastData();
      expect(broadcastData.length).toBe(1);
      expect(broadcastData[0].toString()).toBe("second");
    });
  });

  describe("updateOptions", () => {
    test("应更新选项", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP, {
        debugLog: false,
      });

      bridge.updateOptions({ debugLog: true });
      const options = bridge.getOptions();

      expect(options.debugLog).toBe(true);
    });

    test("应保留未更新的选项", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP, {
        enableTelnet: false,
        enableMCP: true,
      });

      bridge.updateOptions({ debugLog: true });
      const options = bridge.getOptions();

      expect(options.enableTelnet).toBe(false);
      expect(options.enableMCP).toBe(true);
      expect(options.debugLog).toBe(true);
    });
  });

  describe("getOptions", () => {
    test("应返回选项的副本", () => {
      const bridge = new DataBridge(mockSerial, mockTelnet, mockMCP);

      const options1 = bridge.getOptions();
      const options2 = bridge.getOptions();

      options1.debugLog = true;

      expect(options2.debugLog).toBe(false);
    });
  });
});

describe("DataBridgeOptions 类型", () => {
  test("所有选项都应可选", () => {
    const options: DataBridgeOptions = {};

    expect(options.enableTelnet).toBeUndefined();
    expect(options.enableMCP).toBeUndefined();
    expect(options.debugLog).toBeUndefined();
  });

  test("应支持所有选项", () => {
    const options: DataBridgeOptions = {
      enableTelnet: true,
      enableMCP: true,
      debugLog: true,
    };

    expect(options.enableTelnet).toBe(true);
    expect(options.enableMCP).toBe(true);
    expect(options.debugLog).toBe(true);
  });
});
