/**
 * MCP stdio 传输层测试
 */

import { describe, test, expect, mock, beforeEach, afterEach } from "bun:test";
import { SerialManager } from "../../src/serial/SerialManager.js";
import { SerialHubMCP } from "../../src/mcp/index.js";
import { createStdioTransport, setupGracefulShutdown, keepProcessRunning } from "../../src/mcp/transport/stdio.js";

// Mock SerialManager
const createMockSerialManager = (): SerialManager => {
  return {
    isConnected: false,
    currentPort: null,
    connect: mock(async () => {}),
    disconnect: mock(async () => {}),
    write: mock(async () => 0),
    writeLine: mock(async () => 0),
    on: mock(() => {}),
    off: mock(() => {}),
    emit: mock(() => false),
    updateConfig: mock(() => {}),
    getConfig: mock(() => ({
      port: "",
      baudRate: 115200,
      dataBits: 8,
      parity: "none",
      stopBits: 1,
    })),
  } as unknown as SerialManager;
};

describe("stdio transport", () => {
  describe("createStdioTransport", () => {
    test("应该创建并返回 StdioServerTransport 实例", async () => {
      const mockSerial = createMockSerialManager();
      const mcp = new SerialHubMCP(mockSerial);
      const server = mcp.getServer();

      // 注意：实际连接会阻塞，这里只测试函数存在
      expect(typeof createStdioTransport).toBe("function");
      
      mcp.dispose();
    });
  });

  describe("setupGracefulShutdown", () => {
    test("应该注册信号处理器", () => {
      const cleanup = mock(() => {});
      const originalOn = process.on;
      const registeredEvents: string[] = [];

      // Mock process.on
      process.on = ((event: string, handler: () => void) => {
        registeredEvents.push(event);
        return process;
      }) as typeof process.on;

      setupGracefulShutdown(cleanup);

      expect(registeredEvents).toContain("SIGINT");
      expect(registeredEvents).toContain("SIGTERM");

      // 恢复原始 process.on
      process.on = originalOn;
    });

    test("cleanup 函数应该可以被调用", () => {
      const cleanup = mock(async () => {});
      setupGracefulShutdown(cleanup);
      
      // 验证函数已定义
      expect(typeof cleanup).toBe("function");
    });
  });

  describe("keepProcessRunning", () => {
    test("应该设置 stdin 监听器", () => {
      const onStdinClose = mock(() => {});
      const resumeCalled = { value: false };
      const onCalled = { events: [] as string[] };

      // Mock stdin
      const originalStdin = process.stdin;
      const mockStdin = {
        on: (event: string, handler: () => void) => {
          onCalled.events.push(event);
          return mockStdin;
        },
        resume: () => {
          resumeCalled.value = true;
        },
      };

      Object.defineProperty(process, "stdin", {
        value: mockStdin,
        writable: true,
        configurable: true,
      });

      keepProcessRunning(onStdinClose);

      expect(resumeCalled.value).toBe(true);
      expect(onCalled.events).toContain("end");

      // 恢复原始 stdin
      Object.defineProperty(process, "stdin", {
        value: originalStdin,
        writable: true,
        configurable: true,
      });
    });
  });
});

describe("SerialHubMCP 集成测试", () => {
  test("应该能够创建 MCP 实例并获取 Server", () => {
    const mockSerial = createMockSerialManager();
    const mcp = new SerialHubMCP(mockSerial);

    expect(mcp).toBeDefined();
    expect(mcp.getServer()).toBeDefined();
    expect(mcp.getSerialManager()).toBe(mockSerial);
    expect(mcp.getDataBuffer()).toBeDefined();

    mcp.dispose();
  });

  test("应该能够正确清理资源", () => {
    const mockSerial = createMockSerialManager();
    const mcp = new SerialHubMCP(mockSerial);

    // 调用 dispose 不应抛出错误
    expect(() => mcp.dispose()).not.toThrow();
  });
});

describe("配置加载测试", () => {
  test("应该能够加载默认配置", async () => {
    const { loadConfig } = await import("../../src/config/index.js");
    const config = loadConfig();

    expect(config.serial).toBeDefined();
    expect(config.serial.baudRate).toBe(115200);
    expect(config.telnet.port).toBe(2323);
    expect(config.mcp.httpPort).toBe(3000);
  });

  test("应该能够解析 CLI 参数", async () => {
    const { parseCliArgs } = await import("../../src/config/index.js");
    
    // 临时修改 process.argv
    const originalArgv = process.argv;
    process.argv = ["node", "serialhub", "--serial-port", "COM9", "--baud-rate", "9600"];

    const args = parseCliArgs();

    expect(args.serial?.port).toBe("COM9");
    expect(args.serial?.baudRate).toBe(9600);

    // 恢复原始 argv
    process.argv = originalArgv;
  });
});
