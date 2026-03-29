#!/usr/bin/env node
/**
 * SerialHub 主入口
 */

import { SerialHubMCP } from "./mcp/index.js";
import { SerialManager } from "./serial/SerialManager.js";
import { TelnetServer } from "./telnet/TelnetServer.js";
import { DataBridge } from "./bridge/DataBridge.js";
import { loadConfig } from "./config/index.js";
import { createMcpHttpServer, type HttpServerResult } from "./mcp/transport/http-sse.js";
import { TrayManager } from "./tray/TrayManager.js";
import { hideConsole, isWindows } from "./tray/console.js";
import { writeServiceStatus, clearServiceStatus, readServiceStatus } from "./service-manager.js";
import { execSync } from "node:child_process";

let serverResult: HttpServerResult | null = null;
let serial: SerialManager | null = null;
let telnet: TelnetServer | null = null;
let mcp: SerialHubMCP | null = null;
let bridge: DataBridge | null = null;

function showHelp(): void {
  console.log(`
SerialHub - 串口与网络连接的双向桥接器

用法:
  serialhub [命令] [选项]

命令:
  serve     启动 HTTP 服务（默认，含 MCP 和系统托盘）
  stop      停止运行中的服务
  help      显示帮助信息
  version   显示版本号

选项:
  -p, --serial-port <port>   串口名，如 COM9 或 /dev/ttyUSB0
  -b, --baud-rate <rate>     波特率，默认 115200
  -m, --mcp-port <port>      HTTP 服务端口，默认 5000
  --host <host>              监听地址，默认 127.0.0.1
  --no-tray                  禁用系统托盘
  --no-cors                  禁用 CORS
  -c, --config <path>        配置文件路径
  -D, --debug                启用调试模式

示例:
  serialhub                        # 启动服务
  serialhub serve -p COM7          # 启动并连接串口
  serialhub serve -m 8080          # 使用 8080 端口
  serialhub stop                   # 停止服务
`);
}

async function showVersion(): Promise<void> {
  const fs = await import("fs");
  const path = await import("path");
  const url = await import("url");
  const __dirname = path.dirname(url.fileURLToPath(import.meta.url));
  const packageJsonPath = path.join(__dirname, "..", "package.json");
  const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, "utf-8"));
  console.log(`SerialHub v${packageJson.version}`);
}

async function runStop(): Promise<void> {
  const status = readServiceStatus();

  if (!status.running || !status.pid) {
    console.log("没有运行中的 SerialHub 服务");
    clearServiceStatus();
    return;
  }

  try {
    if (process.platform === "win32") {
      execSync(`taskkill /pid ${status.pid} /T /F`, { stdio: "pipe" });
    } else {
      process.kill(status.pid, "SIGTERM");
    }
    clearServiceStatus();
    console.log(`SerialHub 服务已停止 (PID ${status.pid})`);
  } catch {
    clearServiceStatus();
    console.log("服务进程已不存在，已清理状态文件");
  }
}

function parseArgs(): { command: string; showHelp: boolean; host: string; noTray: boolean; noCors: boolean } {
  const args = process.argv.slice(2);
  const result = {
    command: "serve",
    showHelp: false,
    host: "127.0.0.1",
    noTray: false,
    noCors: false,
  };

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];

    if (arg === "serve" || arg === "stop" || arg === "help" || arg === "version") {
      result.command = arg;
    } else if (arg === "--help" || arg === "-h") {
      result.showHelp = true;
    } else if (arg === "--host" && args[i + 1]) {
      result.host = args[++i];
    } else if (arg === "--no-tray") {
      result.noTray = true;
    } else if (arg === "--no-cors") {
      result.noCors = true;
    }
  }

  return result;
}

function setupGracefulShutdown(): void {
  const shutdown = async (signal: string) => {
    console.error(`\n[SerialHub] 收到 ${signal} 信号，正在关闭...`);

    try {
      if (bridge) {
        bridge.stop();
        console.error("[SerialHub] 数据桥接已停止");
      }

      if (telnet?.isRunning) {
        await telnet.stop();
        console.error("[SerialHub] Telnet 服务已关闭");
      }

      if (serverResult) {
        await serverResult.close();
        console.error("[SerialHub] HTTP 服务已关闭");
      }

      if (serial?.isConnected) {
        await serial.disconnect();
        console.error("[SerialHub] 串口已断开");
      }

      if (mcp) {
        mcp.dispose();
      }

      clearServiceStatus();
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

async function runServe(host: string, noTray: boolean, noCors: boolean): Promise<void> {
  const config = loadConfig();

  serial = new SerialManager(config.serial);
  telnet = new TelnetServer();
  mcp = new SerialHubMCP(serial, { name: "SerialHub", version: "0.1.0" });
  bridge = new DataBridge(serial, telnet, mcp, { debugLog: config.debug });

  setupGracefulShutdown();

  try {
    await telnet.start(config.telnet.port);
    console.error(`[SerialHub] Telnet 服务已启动: 端口 ${config.telnet.port}`);
  } catch (error) {
    console.error(`[SerialHub] 启动 Telnet 服务失败: ${error}`);
  }

  bridge.start();
  console.error("[SerialHub] 数据桥接已启动");

  if (config.serial.port) {
    try {
      await serial.connect();
      console.error(`[SerialHub] 已连接串口: ${config.serial.port}`);
    } catch (error) {
      console.error(`[SerialHub] 连接串口失败: ${error}`);
    }
  }

  console.error(`[SerialHub] 正在启动 HTTP 服务...`);

  serverResult = await createMcpHttpServer(mcp, {
    port: config.mcp.httpPort,
    host,
    enableCors: !noCors,
  });

  writeServiceStatus(config.mcp.httpPort);

  console.error(`[SerialHub] MCP HTTP 服务已启动: http://${host}:${config.mcp.httpPort}`);
  console.error(`[SerialHub] 端点:`);
  console.error(`  - MCP: POST http://${host}:${config.mcp.httpPort}/mcp`);
  console.error(`  - Health: GET http://${host}:${config.mcp.httpPort}/health`);
  console.error(`  - Telnet: telnet ${host} ${config.telnet.port}`);

  if (isWindows && !noTray) {
    const tray = new TrayManager(serial, {
      telnetPort: config.telnet.port,
      mcpPort: config.mcp.httpPort,
    });
    await tray.start();

    serial.on("connected", () => tray.updateState("connected"));
    serial.on("disconnected", () => tray.updateState("idle"));
    serial.on("error", () => tray.updateState("error"));

    console.error("[SerialHub] 系统托盘已启动");
    hideConsole();
  }
}

async function main(): Promise<void> {
  const args = parseArgs();

  if (args.showHelp) {
    showHelp();
    process.exit(0);
  }

  switch (args.command) {
    case "help":
      showHelp();
      break;
    case "version":
      await showVersion();
      break;
    case "stop":
      await runStop();
      break;
    case "serve":
    default:
      await runServe(args.host, args.noTray, args.noCors);
      break;
  }
}

main().catch((error) => {
  console.error("[SerialHub] 启动失败:", error);
  process.exit(1);
});