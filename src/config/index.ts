/**
 * SerialHub 配置模块
 * 管理串口、Telnet、MCP 的配置
 */

import { existsSync, readFileSync } from "fs";
import { resolve } from "path";

/**
 * 串口配置
 */
export interface SerialConfig {
  /** 串口名，如 "COM9" */
  port: string;
  /** 波特率，默认 115200 */
  baudRate: number;
  /** 数据位 */
  dataBits: 5 | 6 | 7 | 8;
  /** 校验位 */
  parity: "none" | "even" | "odd";
  /** 停止位 */
  stopBits: 1 | 2;
}

/**
 * Telnet 配置
 */
export interface TelnetConfig {
  /** Telnet 端口，默认 2323 */
  port: number;
}

/**
 * MCP 配置
 */
export interface MCPConfig {
  /** MCP HTTP 端口，默认 3000 */
  httpPort: number;
}

/**
 * 应用配置
 */
export interface Config {
  serial: SerialConfig;
  telnet: TelnetConfig;
  mcp: MCPConfig;
  debug: boolean;
}

/**
 * 默认配置
 */
const DEFAULT_CONFIG: Config = {
  serial: {
    port: "",
    baudRate: 115200,
    dataBits: 8,
    parity: "none",
    stopBits: 1,
  },
  telnet: {
    port: 2323,
  },
  mcp: {
    httpPort: 3000,
  },
  debug: false,
};

/**
 * 获取默认配置
 * @returns 默认配置对象的深拷贝
 */
export function getDefaultConfig(): Config {
  return JSON.parse(JSON.stringify(DEFAULT_CONFIG));
}

/**
 * 深度合并配置
 * @param base 基础配置
 * @param override 覆盖配置
 * @returns 合并后的配置
 */
export function mergeConfig(
  base: Config,
  override: Partial<Config>
): Config {
  const result = JSON.parse(JSON.stringify(base)) as Config;

  if (override.serial) {
    result.serial = { ...result.serial, ...override.serial } as SerialConfig;
  }
  if (override.telnet) {
    result.telnet = { ...result.telnet, ...override.telnet } as TelnetConfig;
  }
  if (override.mcp) {
    result.mcp = { ...result.mcp, ...override.mcp } as MCPConfig;
  }
  if (typeof override.debug === "boolean") {
    result.debug = override.debug;
  }

  return result;
}

/**
 * 解析 CLI 参数
 * 支持的参数:
 * --serial-port <port>
 * --baud-rate <rate>
 * --telnet-port <port>
 * --mcp-port <port>
 * --config <path>
 * --debug
 * @returns 解析出的部分配置
 */
export function parseCliArgs(): Partial<Config> & { configPath?: string } {
  const result: Partial<Config> & { configPath?: string } = {};
  const args = process.argv.slice(2);

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];

    switch (arg) {
      case "--serial-port":
        if (args[i + 1]) {
          result.serial = { ...result.serial, port: args[++i] } as SerialConfig;
        }
        break;

      case "--baud-rate":
        if (args[i + 1]) {
          const baudRate = parseInt(args[++i], 10);
          if (!isNaN(baudRate)) {
            result.serial = { ...result.serial, baudRate } as SerialConfig;
          }
        }
        break;

      case "--telnet-port":
        if (args[i + 1]) {
          const port = parseInt(args[++i], 10);
          if (!isNaN(port)) {
            result.telnet = { port };
          }
        }
        break;

      case "--mcp-port":
        if (args[i + 1]) {
          const httpPort = parseInt(args[++i], 10);
          if (!isNaN(httpPort)) {
            result.mcp = { httpPort };
          }
        }
        break;

      case "--config":
        if (args[i + 1]) {
          result.configPath = args[++i];
        }
        break;

      case "--debug":
        result.debug = true;
        break;
    }
  }

  return result;
}

/**
 * 从 JSON 文件加载配置
 * @param configPath 配置文件路径（相对或绝对）
 * @returns 加载的配置
 */
function loadConfigFromFile(configPath: string): Partial<Config> {
  const absolutePath = resolve(configPath);

  if (!existsSync(absolutePath)) {
    console.warn(`配置文件不存在: ${absolutePath}`);
    return {};
  }

  try {
    const content = readFileSync(absolutePath, "utf-8");
    const config = JSON.parse(content) as Partial<Config>;
    return config;
  } catch (error) {
    console.error(`加载配置文件失败: ${absolutePath}`, error);
    return {};
  }
}

/**
 * 加载配置
 * 优先级: CLI > 文件 > 默认值
 * @param configPath 可选的配置文件路径
 * @returns 完整配置
 */
export function loadConfig(configPath?: string): Config {
  // 1. 获取默认配置
  const defaultConfig = getDefaultConfig();

  // 2. 解析 CLI 参数
  const cliConfig = parseCliArgs();
  const fileConfigPath = configPath ?? cliConfig.configPath;

  // 3. 加载文件配置
  const fileConfig = fileConfigPath
    ? loadConfigFromFile(fileConfigPath)
    : {};

  // 4. 合并配置: 默认值 < 文件 < CLI
  let result = mergeConfig(defaultConfig, fileConfig);
  // 移除 configPath，它不是 Config 的一部分
  const { configPath: _, ...cliConfigWithoutPath } = cliConfig as Partial<Config> & { configPath?: string };
  result = mergeConfig(result, cliConfigWithoutPath);

  return result;
}

// 导出默认配置常量
export { DEFAULT_CONFIG };
