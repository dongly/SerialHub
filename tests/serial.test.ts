/**
 * 串口管理模块测试
 */

import { describe, test, expect, beforeEach, afterEach, mock } from "bun:test";
import { SerialManager, type SerialConfig, type SerialPortInfo } from "../src/serial/SerialManager";

describe("SerialManager", () => {
  const defaultConfig: SerialConfig = {
    port: "COM9",
    baudRate: 115200,
    dataBits: 8,
    parity: "none",
    stopBits: 1,
  };

  describe("构造函数和基本属性", () => {
    test("应正确创建实例", () => {
      const manager = new SerialManager(defaultConfig);

      expect(manager).toBeInstanceOf(SerialManager);
      expect(manager.isConnected).toBe(false);
      expect(manager.currentPort).toBe(null);
    });

    test("应正确存储配置", () => {
      const manager = new SerialManager(defaultConfig);
      const config = manager.getConfig();

      expect(config.port).toBe("COM9");
      expect(config.baudRate).toBe(115200);
      expect(config.dataBits).toBe(8);
      expect(config.parity).toBe("none");
      expect(config.stopBits).toBe(1);
    });

    test("getConfig 应返回配置的副本", () => {
      const manager = new SerialManager(defaultConfig);
      const config1 = manager.getConfig();
      const config2 = manager.getConfig();

      config1.baudRate = 9600;

      expect(config2.baudRate).toBe(115200);
    });
  });

  describe("updateConfig", () => {
    test("应更新部分配置", () => {
      const manager = new SerialManager(defaultConfig);

      manager.updateConfig({ baudRate: 9600 });

      const config = manager.getConfig();
      expect(config.baudRate).toBe(9600);
      expect(config.port).toBe("COM9"); // 保持原值
    });

    test("应更新完整配置", () => {
      const manager = new SerialManager(defaultConfig);

      manager.updateConfig({
        port: "COM10",
        baudRate: 57600,
        dataBits: 7,
        parity: "even",
        stopBits: 2,
      });

      const config = manager.getConfig();
      expect(config.port).toBe("COM10");
      expect(config.baudRate).toBe(57600);
      expect(config.dataBits).toBe(7);
      expect(config.parity).toBe("even");
      expect(config.stopBits).toBe(2);
    });
  });

  describe("isConnected 初始状态", () => {
    test("初始状态应为未连接", () => {
      const manager = new SerialManager(defaultConfig);
      expect(manager.isConnected).toBe(false);
    });
  });

  describe("currentPort 初始状态", () => {
    test("初始状态应为 null", () => {
      const manager = new SerialManager(defaultConfig);
      expect(manager.currentPort).toBe(null);
    });
  });

  describe("事件发射", () => {
    test("应继承 EventEmitter", () => {
      const manager = new SerialManager(defaultConfig);

      expect(typeof manager.on).toBe("function");
      expect(typeof manager.emit).toBe("function");
      expect(typeof manager.removeListener).toBe("function");
    });
  });

  describe("write 未连接时应抛出错误", () => {
    test("write 在未连接时应抛出错误", async () => {
      const manager = new SerialManager(defaultConfig);

      expect(manager.write("test")).rejects.toThrow("串口未连接");
    });

    test("writeLine 在未连接时应抛出错误", async () => {
      const manager = new SerialManager(defaultConfig);

      expect(manager.writeLine("test")).rejects.toThrow("串口未连接");
    });
  });

  describe("connect 无串口名时应抛出错误", () => {
    test("配置无端口且未指定端口时应抛出错误", async () => {
      const manager = new SerialManager({
        ...defaultConfig,
        port: "",
      });

      expect(manager.connect()).rejects.toThrow("未指定串口名");
    });
  });

  describe("disconnect 未连接时不应报错", () => {
    test("未连接时调用 disconnect 应正常返回", async () => {
      const manager = new SerialManager(defaultConfig);

      // 不应抛出错误
      await expect(manager.disconnect()).resolves.toBeUndefined();
    });
  });
});

describe("SerialManager.listPorts", () => {
  test("应返回数组格式", async () => {
    // 由于没有硬件，我们只验证返回格式
    const ports = await SerialManager.listPorts();

    expect(Array.isArray(ports)).toBe(true);
  });

  test("返回的端口信息应包含 path 字段", async () => {
    const ports = await SerialManager.listPorts();

    // 即使没有串口，也应该返回空数组
    ports.forEach((port) => {
      expect(typeof port.path).toBe("string");
    });
  });
});

describe("SerialConfig 类型验证", () => {
  test("应支持所有有效的波特率配置", () => {
    const baudRates = [9600, 19200, 38400, 57600, 115200, 230400];

    baudRates.forEach((baudRate) => {
      const manager = new SerialManager({
        port: "COM9",
        baudRate,
        dataBits: 8,
        parity: "none",
        stopBits: 1,
      });

      expect(manager.getConfig().baudRate).toBe(baudRate);
    });
  });

  test("应支持所有有效的数据位配置", () => {
    const dataBitsList: Array<5 | 6 | 7 | 8> = [5, 6, 7, 8];

    dataBitsList.forEach((dataBits) => {
      const manager = new SerialManager({
        port: "COM9",
        baudRate: 115200,
        dataBits,
        parity: "none",
        stopBits: 1,
      });

      expect(manager.getConfig().dataBits).toBe(dataBits);
    });
  });

  test("应支持所有有效的校验位配置", () => {
    const parities: Array<"none" | "even" | "odd"> = ["none", "even", "odd"];

    parities.forEach((parity) => {
      const manager = new SerialManager({
        port: "COM9",
        baudRate: 115200,
        dataBits: 8,
        parity,
        stopBits: 1,
      });

      expect(manager.getConfig().parity).toBe(parity);
    });
  });

  test("应支持所有有效的停止位配置", () => {
    const stopBitsList: Array<1 | 2> = [1, 2];

    stopBitsList.forEach((stopBits) => {
      const manager = new SerialManager({
        port: "COM9",
        baudRate: 115200,
        dataBits: 8,
        parity: "none",
        stopBits,
      });

      expect(manager.getConfig().stopBits).toBe(stopBits);
    });
  });
});

describe("SerialPortInfo 接口", () => {
  test("接口应定义正确的字段", () => {
    const portInfo: SerialPortInfo = {
      path: "COM9",
      manufacturer: "Test Manufacturer",
      serialNumber: "12345",
      pnpId: "USB\\VID_1234&PID_5678",
      vendorId: "1234",
      productId: "5678",
    };

    expect(portInfo.path).toBe("COM9");
    expect(portInfo.manufacturer).toBe("Test Manufacturer");
    expect(portInfo.serialNumber).toBe("12345");
    expect(portInfo.pnpId).toBe("USB\\VID_1234&PID_5678");
    expect(portInfo.vendorId).toBe("1234");
    expect(portInfo.productId).toBe("5678");
  });

  test("可选字段可以省略", () => {
    const portInfo: SerialPortInfo = {
      path: "COM9",
    };

    expect(portInfo.path).toBe("COM9");
    expect(portInfo.manufacturer).toBeUndefined();
    expect(portInfo.serialNumber).toBeUndefined();
    expect(portInfo.pnpId).toBeUndefined();
    expect(portInfo.vendorId).toBeUndefined();
    expect(portInfo.productId).toBeUndefined();
  });
});

// 集成测试（需要硬件 + Node.js）
// 注意: Bun 与 serialport 在 Windows USB CDC 设备上有兼容性问题
// 运行方式: npx tsx tests/hardware-test.ts
describe.skip("集成测试（需要硬件）", () => {
  const hardwareConfig: SerialConfig = {
    port: "COM9",
    baudRate: 115200,
    dataBits: 8,
    parity: "none",
    stopBits: 1,
  };

  test("连接 COM9 并发送 help 命令", async () => {
    const manager = new SerialManager(hardwareConfig);

    // 监听数据事件
    let receivedData = Buffer.alloc(0);
    manager.on("data", (data: Buffer) => {
      receivedData = Buffer.concat([receivedData, data]);
    });

    try {
      // 连接串口（带超时保护）
      const connectPromise = manager.connect();
      const connectTimeout = new Promise<never>((_, reject) =>
        setTimeout(() => reject(new Error("连接超时")), 5000)
      );
      await Promise.race([connectPromise, connectTimeout]);

      expect(manager.isConnected).toBe(true);
      expect(manager.currentPort).toBe("COM9");

      // 发送 help 命令
      await manager.writeLine("help");

      // 等待响应（最多 2 秒）
      await new Promise((resolve) => setTimeout(resolve, 2000));

      // 验证收到响应
      expect(receivedData.length).toBeGreaterThan(0);
      console.log("收到响应:", receivedData.toString("utf-8"));
    } finally {
      // 确保断开连接
      if (manager.isConnected) {
        await manager.disconnect();
      }
    }

    expect(manager.isConnected).toBe(false);
    expect(manager.currentPort).toBe(null);
  });
});
