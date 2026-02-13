#!/usr/bin/env node
/**
 * SerialHub HTTP 服务入口
 * 启动 MCP HTTP+SSE 服务、Telnet 服务和数据桥接
 */

import { SerialHubMCP } from "./mcp/index.js";
import { SerialManager } from "./serial/SerialManager.js";
import { TelnetServer } from "./telnet/TelnetServer.js";
import { DataBridge } from "./bridge/DataBridge.js";
import { loadConfig } from "./config/index.js";
import {
  createHttpServer,
  createHttpServerStateful,
  HttpServerResult,
} from "./mcp/transport/http-sse.js";

/**
 * 服务运行时状态
 */
let serverResult: HttpServerResult | null = null;
let serial: SerialManager | null = null;
let telnet: TelnetServer | null = null;
let mcp: SerialHubMCP | null = null;
let bridge: DataBridge | null = null;

/**
 * 显示帮助信息
 */
function showHelp(): void {
  console.log(`
SerialHub HTTP 服务 - 启动 MCP HTTP+SSE 服务

用法:
  serialhub serve [选项]

选项:
  --serial-port <port>   串口名，如 COM9 或 /dev/ttyUSB0
  --baud-rate <rate>     波特率，默认 115200
  --telnet-port <port>   Telnet 端口，默认 2323
  --mcp-port <port>      HTTP 服务端口，默认 3000
  --host <host>          监听地址，默认 127.0.0.1
  --stateful             启用有状态模式（支持会话）
  --no-cors              禁用 CORS
  --cors-origin <origin> CORS 允许的来源，默认 "*"
  --config <path>        配置文件路径
  --debug                启用调试模式

示例:
  serialhub serve                              # 启动服务，默认端口
  serialhub serve --serial-port COM9           # 启动时连接串口
  serialhub serve --mcp-port 8080              # 使用 8080 端口
  serialhub serve --telnet-port 2323           # Telnet 端口 2323
  serialhub serve --host 0.0.0.0               # 监听所有网络接口
  serialhub serve --stateful                   # 启用有状态模式
`);
}

/**
 * 解析 serve 命令参数
 */
function parseServeArgs(): {
  showHelp: boolean;
  host: string;
  stateful: boolean;
  enableCors: boolean;
  corsOrigin: string;
} {
  const args = process.argv.slice(2);
  const result = {
    showHelp: false,
    host: "127.0.0.1",
    stateful: false,
    enableCors: true,
    corsOrigin: "*",
  };

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];

    switch (arg) {
      case "--help":
      case "-h":
        result.showHelp = true;
        break;
      case "--host":
        if (args[i + 1]) {
          result.host = args[++i];
        }
        break;
      case "--stateful":
        result.stateful = true;
        break;
      case "--no-cors":
        result.enableCors = false;
        break;
      case "--cors-origin":
        if (args[i + 1]) {
          result.corsOrigin = args[++i];
        }
        break;
    }
  }

  return result;
}

/**
 * 设置优雅关闭
 */
function setupGracefulShutdown(): void {
  const shutdown = async (signal: string) => {
    console.error(`\n[SerialHub] 收到 ${signal} 信号，正在关闭...`);

    try {
      // 停止数据桥接
      if (bridge) {
        bridge.stop();
        console.error("[SerialHub] 数据桥接已停止");
      }

      // 关闭 Telnet 服务
      if (telnet?.isRunning) {
        await telnet.stop();
        console.error("[SerialHub] Telnet 服务已关闭");
      }

      // 关闭 HTTP 服务
      if (serverResult) {
        await serverResult.close();
        console.error("[SerialHub] HTTP 服务已关闭");
      }

      // 断开串口
      if (serial?.isConnected) {
        await serial.disconnect();
        console.error("[SerialHub] 串口已断开");
      }

      // 清理 MCP
      if (mcp) {
        mcp.dispose();
      }

      console.error("[SerialHub] 已关闭");
      process.exit(0);
    } catch (error) {
      console.error("[SerialHub] 关闭时出错:", error);
      process.exit(1);
    }
  };

  process.on("SIGINT", () => shutdown("SIGINT"));
  process.on("SIGTERM", () => shutdown("SIGTERM"));
}

/**
 * 主函数
 */
async function main(): Promise<void> {
  // 解析 serve 命令参数
  const serveArgs = parseServeArgs();

  if (serveArgs.showHelp) {
    showHelp();
    process.exit(0);
  }

  // 加载配置
  const config = loadConfig();

  // 创建串口管理器
  serial = new SerialManager(config.serial);

  // 创建 Telnet 服务器
  telnet = new TelnetServer();

  // 创建 MCP 服务
  mcp = new SerialHubMCP(serial, {
    name: "SerialHub",
    version: "0.1.0",
  });

  // 创建数据桥接
  bridge = new DataBridge(serial, telnet, mcp, {
    debugLog: config.debug,
  });

  // 设置优雅关闭
  setupGracefulShutdown();

  // 启动 Telnet 服务
  try {
    await telnet.start(config.telnet.port);
    console.error(`[SerialHub] Telnet 服务已启动: 端口 ${config.telnet.port}`);
  } catch (error) {
    console.error(`[SerialHub] 启动 Telnet 服务失败: ${error}`);
    // Telnet 失败不阻塞主服务
  }

  // 启动数据桥接
  bridge.start();
  console.error("[SerialHub] 数据桥接已启动");

  // 自动连接串口（如果配置了）
  if (config.serial.port) {
    try {
      await serial.connect();
      console.error(`[SerialHub] 已连接串口: ${config.serial.port}`);
    } catch (error) {
      console.error(`[SerialHub] 连接串口失败: ${error}`);
      // 继续运行，允许手动连接
    }
  }

  // 创建 HTTP 服务
  const httpConfig = {
    port: config.mcp.httpPort,
    host: serveArgs.host,
    enableCors: serveArgs.enableCors,
    corsOrigin: serveArgs.corsOrigin,
  };

  console.error(`[SerialHub] 正在启动 HTTP 服务...`);

  if (serveArgs.stateful) {
    serverResult = await createHttpServerStateful(mcp.getServer(), httpConfig);
  } else {
    serverResult = await createHttpServer(mcp.getServer(), httpConfig);
  }

  console.error(`[SerialHub] 服务模式: ${serveArgs.stateful ? "有状态" : "无状态"}`);
  console.error(`[SerialHub] CORS: ${serveArgs.enableCors ? `已启用 (${serveArgs.corsOrigin})` : "已禁用"}`);

  if (config.debug) {
    console.error(`[SerialHub] 配置: ${JSON.stringify(config, null, 2)}`);
  }

  console.error(`[SerialHub] 端点:`);
  console.error(`  - MCP: POST http://${serveArgs.host}:${config.mcp.httpPort}/mcp`);
  console.error(`  - Health: GET http://${serveArgs.host}:${config.mcp.httpPort}/health`);
  console.error(`  - Telnet: telnet ${serveArgs.host} ${config.telnet.port}`);
}

// 启动
main().catch((error) => {
  console.error("[SerialHub] 启动失败:", error);
  process.exit(1);
});