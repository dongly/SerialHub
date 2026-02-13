/**
 * 配置模块测试
 */

import { describe, test, expect, beforeEach, afterEach } from "bun:test";
import { writeFileSync, mkdirSync, rmSync, existsSync } from "fs";
import { join } from "path";
import {
  getDefaultConfig,
  mergeConfig,
  parseCliArgs,
  loadConfig,
  DEFAULT_CONFIG,
  type Config,
} from "../src/config/index";

describe("配置模块", () => {
  describe("getDefaultConfig", () => {
    test("应返回默认配置", () => {
      const config = getDefaultConfig();

      expect(config.serial.port).toBe("");
      expect(config.serial.baudRate).toBe(115200);
      expect(config.serial.dataBits).toBe(8);
      expect(config.serial.parity).toBe("none");
      expect(config.serial.stopBits).toBe(1);
      expect(config.telnet.port).toBe(2323);
      expect(config.mcp.httpPort).toBe(3000);
      expect(config.debug).toBe(false);
    });

    test("返回的配置应该是独立的副本", () => {
      const config1 = getDefaultConfig();
      const config2 = getDefaultConfig();

      config1.serial.baudRate = 9600;

      expect(config2.serial.baudRate).toBe(115200);
    });
  });

  describe("mergeConfig", () => {
    test("应合并串口配置", () => {
      const base = getDefaultConfig();
      const override: Partial<Config> = {
        serial: {
          port: "COM9",
          baudRate: 9600,
          dataBits: 7,
          parity: "even",
          stopBits: 2,
        },
      };

      const result = mergeConfig(base, override);

      expect(result.serial.port).toBe("COM9");
      expect(result.serial.baudRate).toBe(9600);
      expect(result.serial.dataBits).toBe(7);
      expect(result.serial.parity).toBe("even");
      expect(result.serial.stopBits).toBe(2);
    });

    test("应合并 Telnet 配置", () => {
      const base = getDefaultConfig();
      const override: Partial<Config> = {
        telnet: { port: 8023 },
      };

      const result = mergeConfig(base, override);

      expect(result.telnet.port).toBe(8023);
    });

    test("应合并 MCP 配置", () => {
      const base = getDefaultConfig();
      const override: Partial<Config> = {
        mcp: { httpPort: 8080 },
      };

      const result = mergeConfig(base, override);

      expect(result.mcp.httpPort).toBe(8080);
    });

    test("应合并 debug 配置", () => {
      const base = getDefaultConfig();
      const override: Partial<Config> = {
        debug: true,
      };

      const result = mergeConfig(base, override);

      expect(result.debug).toBe(true);
    });

    test("部分覆盖应保留其他默认值", () => {
      const base = getDefaultConfig();
      const override: Partial<Config> = {
        serial: { port: "COM9" } as any,
      };

      const result = mergeConfig(base, override);

      expect(result.serial.port).toBe("COM9");
      expect(result.serial.baudRate).toBe(115200);
    });

    test("不应修改原始配置", () => {
      const base = getDefaultConfig();
      const originalBaudRate = base.serial.baudRate;
      const override: Partial<Config> = {
        serial: { baudRate: 9600 } as any,
      };

      mergeConfig(base, override);

      expect(base.serial.baudRate).toBe(originalBaudRate);
    });
  });

  describe("parseCliArgs", () => {
    let originalArgv: string[];

    beforeEach(() => {
      originalArgv = process.argv;
    });

    afterEach(() => {
      process.argv = originalArgv;
    });

    test("应解析 --serial-port 参数", () => {
      process.argv = ["bun", "run", "src/index.ts", "--serial-port", "COM9"];
      const result = parseCliArgs();

      expect(result.serial?.port).toBe("COM9");
    });

    test("应解析 --baud-rate 参数", () => {
      process.argv = ["bun", "run", "src/index.ts", "--baud-rate", "9600"];
      const result = parseCliArgs();

      expect(result.serial?.baudRate).toBe(9600);
    });

    test("应解析 --telnet-port 参数", () => {
      process.argv = ["bun", "run", "src/index.ts", "--telnet-port", "8023"];
      const result = parseCliArgs();

      expect(result.telnet?.port).toBe(8023);
    });

    test("应解析 --mcp-port 参数", () => {
      process.argv = ["bun", "run", "src/index.ts", "--mcp-port", "8080"];
      const result = parseCliArgs();

      expect(result.mcp?.httpPort).toBe(8080);
    });

    test("应解析 --config 参数", () => {
      process.argv = ["bun", "run", "src/index.ts", "--config", "/path/to/config.json"];
      const result = parseCliArgs();

      expect(result.configPath).toBe("/path/to/config.json");
    });

    test("应解析 --debug 标志", () => {
      process.argv = ["bun", "run", "src/index.ts", "--debug"];
      const result = parseCliArgs();

      expect(result.debug).toBe(true);
    });

    test("应解析多个参数", () => {
      process.argv = [
        "bun",
        "run",
        "src/index.ts",
        "--serial-port",
        "COM9",
        "--baud-rate",
        "9600",
        "--debug",
      ];
      const result = parseCliArgs();

      expect(result.serial?.port).toBe("COM9");
      expect(result.serial?.baudRate).toBe(9600);
      expect(result.debug).toBe(true);
    });

    test("无参数时应返回空对象", () => {
      process.argv = ["bun", "run", "src/index.ts"];
      const result = parseCliArgs();

      expect(Object.keys(result).length).toBe(0);
    });
  });

  describe("loadConfig", () => {
    const testDir = join(import.meta.dir, "test-configs");

    beforeEach(() => {
      if (!existsSync(testDir)) {
        mkdirSync(testDir, { recursive: true });
      }
    });

    afterEach(() => {
      if (existsSync(testDir)) {
        rmSync(testDir, { recursive: true, force: true });
      }
    });

    test("应返回默认配置（无参数）", () => {
      const config = loadConfig();

      expect(config.serial.baudRate).toBe(DEFAULT_CONFIG.serial.baudRate);
      expect(config.telnet.port).toBe(DEFAULT_CONFIG.telnet.port);
      expect(config.mcp.httpPort).toBe(DEFAULT_CONFIG.mcp.httpPort);
    });

    test("应从 JSON 文件加载配置", () => {
      const configPath = join(testDir, "test-config.json");
      const fileConfig: Partial<Config> = {
        serial: {
          port: "COM9",
          baudRate: 9600,
          dataBits: 7,
          parity: "even",
          stopBits: 2,
        },
        telnet: { port: 8023 },
      };

      writeFileSync(configPath, JSON.stringify(fileConfig));

      const config = loadConfig(configPath);

      expect(config.serial.port).toBe("COM9");
      expect(config.serial.baudRate).toBe(9600);
      expect(config.serial.dataBits).toBe(7);
      expect(config.serial.parity).toBe("even");
      expect(config.serial.stopBits).toBe(2);
      expect(config.telnet.port).toBe(8023);
      // 默认值应保留
      expect(config.mcp.httpPort).toBe(3000);
    });

    test("文件配置不存在时应返回默认配置", () => {
      const config = loadConfig("/nonexistent/config.json");

      expect(config.serial.baudRate).toBe(DEFAULT_CONFIG.serial.baudRate);
    });

    test("CLI 参数应覆盖文件配置", () => {
      const configPath = join(testDir, "override-test.json");
      const fileConfig: Partial<Config> = {
        serial: { baudRate: 9600 } as any,
      };

      writeFileSync(configPath, JSON.stringify(fileConfig));

      // 模拟 CLI 参数
      const originalArgv = process.argv;
      process.argv = ["bun", "run", "src/index.ts", "--baud-rate", "57600"];

      const config = loadConfig(configPath);

      process.argv = originalArgv;

      expect(config.serial.baudRate).toBe(57600);
    });

    test("部分文件配置应与默认值合并", () => {
      const configPath = join(testDir, "partial-config.json");
      const fileConfig: Partial<Config> = {
        telnet: { port: 9999 },
      };

      writeFileSync(configPath, JSON.stringify(fileConfig));

      const config = loadConfig(configPath);

      expect(config.telnet.port).toBe(9999);
      expect(config.serial.baudRate).toBe(115200); // 默认值
      expect(config.mcp.httpPort).toBe(3000); // 默认值
    });
  });
});
