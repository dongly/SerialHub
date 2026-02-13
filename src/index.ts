#!/usr/bin/env node
/**
 * SerialHub 主入口
 * 支持 MCP stdio 模式
 */

import { SerialHubMCP } from "./mcp/index.js";
import { SerialManager } from "./serial/SerialManager.js";
import { loadConfig } from "./config/index.js";
import {
  createStdioTransport,
  setupGracefulShutdown,
  keepProcessRunning,
} from "./mcp/transport/stdio.js";

/**
 * CLI 命令类型
 */
type CliCommand = "mcp" | "help" | "version";

/**
 * 解析 CLI 命令
 * @returns 命令和参数
 */
function parseCommand(): { command: CliCommand; args: string[] } {
  const args = process.argv.slice(2);
  const firstArg = args[0];

  // 默认命令是 mcp
  if (!firstArg || firstArg.startsWith("--")) {
    return { command: "mcp", args };
  }

  switch (firstArg) {
    case "mcp":
      return { command: "mcp", args: args.slice(1) };
    case "help":
    case "--help":
    case "-h":
      return { command: "help", args: [] };
    case "version":
    case "--version":
    case "-v":
      return { command: "version", args: [] };
    default:
      console.error(`未知命令: ${firstArg}`);
      return { command: "help", args: [] };
  }
}

/**
 * 显示帮助信息
 */
function showHelp(): void {
  console.log(`
SerialHub - 串口与网络连接的双向桥接器

用法:
  serialhub [命令] [选项]

命令:
  mcp       启动 MCP stdio 服务（默认）
  help      显示帮助信息
  version   显示版本号

选项:
  --serial-port <port>   串口名，如 COM9 或 /dev/ttyUSB0
  --baud-rate <rate>     波特率，默认 115200
  --config <path>        配置文件路径
  --debug                启用调试模式

示例:
  serialhub                              # 启动 MCP stdio 服务
  serialhub mcp                          # 同上
  serialhub mcp --serial-port COM9       # 指定串口
  serialhub --baud-rate 9600             # 指定波特率
`);
}

/**
 * 显示版本号
 */
async function showVersion(): Promise<void> {
  // 读取 package.json
  const fs = await import("fs");
  const path = await import("path");
  const packageJsonPath = path.join(import.meta.dir, "..", "package.json");
  const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, "utf-8"));
  console.log(`SerialHub v${packageJson.version}`);
}

/**
 * 运行 MCP stdio 服务
 */
async function runMcpService(): Promise<void> {
  // 加载配置
  const config = loadConfig();

  // 创建串口管理器
  const serial = new SerialManager(config.serial);

  // 创建 MCP 服务
  const mcp = new SerialHubMCP(serial, {
    name: "SerialHub",
    version: "0.1.0",
  });

  // 设置优雅关闭
  setupGracefulShutdown(async () => {
    if (serial.isConnected) {
      await serial.disconnect();
    }
    mcp.dispose();
  });

  // 连接 stdio 传输
  const _transport = await createStdioTransport(mcp.getServer());

  console.error("[SerialHub] MCP stdio 服务已启动");
  if (config.debug) {
    console.error("[SerialHub] 调试模式已启用");
    console.error(`[SerialHub] 配置: ${JSON.stringify(config, null, 2)}`);
  }

  // 保持进程运行，stdin 关闭时退出
  keepProcessRunning(async () => {
    if (serial.isConnected) {
      await serial.disconnect();
    }
    mcp.dispose();
  });
}

/**
 * 主函数
 */
async function main(): Promise<void> {
  const { command } = parseCommand();

  switch (command) {
    case "help":
      showHelp();
      break;
    case "version":
      await showVersion();
      break;
    case "mcp":
    default:
      await runMcpService();
      break;
  }
}

// 启动
main().catch((error) => {
  console.error("[SerialHub] 启动失败:", error);
  process.exit(1);
});
