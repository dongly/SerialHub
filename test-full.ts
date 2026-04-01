/**
 * SerialHub 全功能 E2E 测试脚本
 *
 * 测试范围：
 *   1. 串口列表、连接、读写、断开
 *   2. Telnet 服务器启动、客户端连接、数据收发、广播、断开
 *   3. MCP 服务（DataBuffer + 6 个工具）直接调用
 *   4. HTTP 服务器（/health、/mcp JSON-RPC、CORS）
 *   5. DataBridge 双向桥接
 *   6. 系统托盘（tray-hook）状态切换 + 截图
 *   7. ServiceManager PID/Port 文件
 *
 * 用法:  npx tsx test-full.ts
 *        npx tsx test-full.ts --port COM7        # 指定串口
 *        npx tsx test-full.ts --skip-tray         # 跳过托盘测试
 *
 * 输出:
 *   - test-screenshots/  截图目录
 *   - test-report.html   HTML 测试报告
 */

import { SerialManager } from "./src/serial/SerialManager.js";
import { TelnetServer } from "./src/telnet/TelnetServer.js";
import { SerialHubMCP } from "./src/mcp/index.js";
import { DataBridge } from "./src/bridge/DataBridge.js";
import { createMcpHttpServer } from "./src/mcp/transport/http-sse.js";
import { TrayManager } from "./src/tray/TrayManager.js";
import { writeServiceStatus, readServiceStatus, clearServiceStatus } from "./src/service-manager.js";

import { createConnection, type Socket } from "node:net";
import { execSync } from "node:child_process";
import { existsSync, mkdirSync, rmSync, writeFileSync, readdirSync } from "node:fs";
import { resolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

// ─── 配置 ───────────────────────────────────────────────────

const __dirname = dirname(fileURLToPath(import.meta.url));

const args = process.argv.slice(2);
const SERIAL_PORT = (args[args.indexOf("--port") + 1]) || "COM7";
const SKIP_TRAY = args.includes("--skip-tray");
const TELNET_PORT = 2324;   // 避免与正在运行的服务冲突
const MCP_PORT = 5001;       // 避免与正在运行的服务冲突
const SCREENSHOT_DIR = resolve(__dirname, "test-screenshots");

// ─── 工具函数 ───────────────────────────────────────────────

interface TestResult {
  name: string;
  pass: boolean;
  detail: string;
  duration: number;
}

const results: TestResult[] = [];
const timers: { label: string; start: number }[] = [];
let currentLabel = "";

function startTimer(label: string): void {
  currentLabel = label;
  timers.push({ label, start: performance.now() });
}

function record(name: string, pass: boolean, detail: string): void {
  const elapsed = timers.length > 0 ? Math.round(performance.now() - timers[timers.length - 1].start) : 0;
  results.push({ name, pass, detail, duration: elapsed });
  const icon = pass ? "✅" : "❌";
  console.log(`  ${icon} ${name}: ${detail} (${elapsed}ms)`);
}

function section(title: string): void {
  console.log(`\n${"─".repeat(60)}`);
  console.log(`  ${title}`);
  console.log(`${"─".repeat(60)}`);
}

function screenshot(name: string): string {
  const filepath = join(SCREENSHOT_DIR, `${name}.png`);
  try {
    execSync(
      `powershell -NoProfile -ExecutionPolicy Bypass -File "${join(__dirname, "screenshot.ps1")}" -OutputPath "${filepath}"`,
      { stdio: "pipe", timeout: 10000 }
    );
    if (existsSync(filepath)) {
      console.log(`    📸 ${name}.png`);
      return filepath;
    }
  } catch {
    console.log(`    ⚠️ 截图失败: ${name}`);
  }
  return "";
}

function screenshotRegion(name: string, x: number, y: number, w: number, h: number): string {
  const filepath = join(SCREENSHOT_DIR, `${name}.png`);
  try {
    execSync(
      `powershell -NoProfile -ExecutionPolicy Bypass -File "${join(__dirname, "screenshot.ps1")}" -OutputPath "${filepath}" -X ${x} -Y ${y} -Width ${w} -Height ${h}`,
      { stdio: "pipe", timeout: 10000 }
    );
    if (existsSync(filepath)) {
      console.log(`    📸 ${name}.png (区域)`);
      return filepath;
    }
  } catch {
    console.log(`    ⚠️ 截图失败: ${name}`);
  }
  return "";
}

function expandTrayIcons(): void {
  try {
    execSync(
      `powershell -NoProfile -ExecutionPolicy Bypass -File "${join(__dirname, "expand-tray.ps1")}"`,
      { stdio: "pipe", timeout: 10000 }
    );
    console.log("    托盘图标已展开");
  } catch {
    console.log("    ⚠️ 托盘展开失败");
  }
}

async function httpGet(url: string): Promise<{ ok: boolean; status?: number; body?: string }> {
  try {
    const res = await fetch(url, { signal: AbortSignal.timeout(5000) });
    const body = await res.text();
    return { ok: res.ok, status: res.status, body };
  } catch (e) {
    return { ok: false, body: e instanceof Error ? e.message : String(e) };
  }
}

async function httpPost(url: string, body: object): Promise<{ ok: boolean; status?: number; body?: string }> {
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(10000),
    });
    const text = await res.text();
    return { ok: res.ok, status: res.status, body: text };
  } catch (e) {
    return { ok: false, body: e instanceof Error ? e.message : String(e) };
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function telnetConnect(port: number, host: string = "127.0.0.1"): Promise<Socket> {
  return new Promise((resolve, reject) => {
    const socket = createConnection({ port, host }, () => resolve(socket));
    socket.on("error", reject);
    setTimeout(() => reject(new Error("Telnet 连接超时")), 5000);
  });
}

function socketRead(socket: Socket, timeoutMs: number = 3000): Promise<Buffer> {
  return new Promise((resolve) => {
    const chunks: Buffer[] = [];
    const onTimeout = () => {
      socket.off("data", onData);
      resolve(Buffer.concat(chunks));
    };
    const onData = (data: Buffer) => {
      chunks.push(data);
    };
    socket.on("data", onData);
    setTimeout(onTimeout, timeoutMs);
  });
}

// ─── 主测试流程 ─────────────────────────────────────────────

async function main(): Promise<void> {
  console.log("╔══════════════════════════════════════════════════════════╗");
  console.log("║          SerialHub 全功能 E2E 测试                      ║");
  console.log(`║          ${new Date().toLocaleString()}                            ║`);
  console.log("╚══════════════════════════════════════════════════════════╝");
  console.log(`  串口: ${SERIAL_PORT}  Telnet: ${TELNET_PORT}  MCP: ${MCP_PORT}`);
  console.log(`  跳过托盘: ${SKIP_TRAY}`);

  // 截图目录
  if (existsSync(SCREENSHOT_DIR)) rmSync(SCREENSHOT_DIR, { recursive: true });
  mkdirSync(SCREENSHOT_DIR, { recursive: true });

  // 资源引用（用于 finally 清理）
  let serial: SerialManager | null = null;
  let telnet: TelnetServer | null = null;
  let mcp: SerialHubMCP | null = null;
  let bridge: DataBridge | null = null;
  let httpServer: { close: () => Promise<void> } | null = null;
  let tray: TrayManager | null = null;

  try {
    // ═══════════════════════════════════════════════════════════
    // 1. 串口管理
    // ═══════════════════════════════════════════════════════════
    section("1. 串口列表");
    startTimer("串口列表");
    const ports = await SerialManager.listPorts();
    record("serial_list", ports.length > 0, `发现 ${ports.length} 个串口: ${ports.map(p => p.path).join(", ")}`);

    const hasTarget = ports.some(p => p.path === SERIAL_PORT);
    record("target_port", hasTarget, `${SERIAL_PORT} ${hasTarget ? "存在" : "不存在"}`);

    // ═══════════════════════════════════════════════════════════
    // 2. 串口连接
    // ═══════════════════════════════════════════════════════════
    section("2. 串口连接与通信");
    startTimer("串口连接");

    const serialConfig = {
      port: SERIAL_PORT,
      baudRate: 115200,
      dataBits: 8 as const,
      parity: "none" as const,
      stopBits: 1 as const,
    };
    serial = new SerialManager(serialConfig);

    // 事件测试
    let connectedFired = false;
    serial.on("connected", () => { connectedFired = true; });

    try {
      await serial.connect();
      record("serial_connect", serial.isConnected && connectedFired,
        `isConnected=${serial.isConnected}, port=${serial.currentPort}, event=${connectedFired}`);
    } catch (e) {
      record("serial_connect", false, `连接失败: ${e instanceof Error ? e.message : String(e)}`);
      throw e; // 无法继续后续测试
    }

    // getConfig
    const cfg = serial.getConfig();
    record("serial_getConfig",
      cfg.port === SERIAL_PORT && cfg.baudRate === 115200,
      `port=${cfg.port}, baudRate=${cfg.baudRate}`);

    // 读写测试
    startTimer("串口写读");
    let serialData = Buffer.alloc(0);
    const dataHandler = (data: Buffer) => { serialData = Buffer.concat([serialData, data]); };
    serial.on("data", dataHandler);

    await serial.writeLine("help");
    await sleep(3000);
    record("serial_write+read_help", serialData.length > 0,
      `写入 "help\\r\\n", 收到 ${serialData.length} 字节`);

    const helpText = serialData.toString("utf-8");
    record("help_content", helpText.length > 0,
      `内容预览: ${helpText.substring(0, 100).replace(/\n/g, "\\n")}...`);

    serialData = Buffer.alloc(0);
    await serial.writeLine("version");
    await sleep(2000);
    record("serial_write+read_version", serialData.length > 0,
      `写入 "version\\r\\n", 收到 ${serialData.length} 字节`);

    serialData = Buffer.alloc(0);
    await serial.write("raw data without newline");
    await sleep(1000);
    record("serial_write_raw", true,
      `原始写入 (无换行), 收到 ${serialData.length} 字节`);

    serial.off("data", dataHandler);

    // ═══════════════════════════════════════════════════════════
    // 3. Telnet 服务
    // ═══════════════════════════════════════════════════════════
    section("3. Telnet 服务");
    startTimer("Telnet 启动");

    telnet = new TelnetServer();

    let serverStarted = false;
    let clientConnected = false;
    let clientDisconnected = false;
    telnet.on("started", () => { serverStarted = true; });
    telnet.on("connection", () => { clientConnected = true; });
    telnet.on("disconnect", () => { clientDisconnected = true; });

    await telnet.start(TELNET_PORT);
    record("telnet_start", telnet.isRunning && serverStarted,
      `isRunning=${telnet.isRunning}, port=${telnet.port}, event=${serverStarted}`);

    // 客户端连接
    startTimer("Telnet 客户端");
    const client1 = await telnetConnect(TELNET_PORT);
    await sleep(300); // 等待欢迎消息

    record("telnet_client_connect",
      telnet.clientCount === 1 && clientConnected,
      `clientCount=${telnet.clientCount}, event=${clientConnected}`);

    // 读取欢迎消息
    const welcomeData = await socketRead(client1, 1000);
    const welcomeText = welcomeData.toString("utf-8");
    record("telnet_welcome",
      welcomeText.includes("SerialHub"),
      `欢迎消息: "${welcomeText.trim()}"`);

    // 广播
    startTimer("Telnet 广播");
    const client2 = await telnetConnect(TELNET_PORT);
    await sleep(200);
    record("telnet_second_client",
      telnet.clientCount === 2,
      `clientCount=${telnet.clientCount}`);

    telnet.broadcast("Hello from SerialHub!\r\n");
    await sleep(500);

    const broadcastData1 = await socketRead(client1, 500);
    const broadcastData2 = await socketRead(client2, 500);
    record("telnet_broadcast",
      broadcastData1.toString().includes("Hello") && broadcastData2.toString().includes("Hello"),
      `客户端1: ${broadcastData1.length}B, 客户端2: ${broadcastData2.length}B`);

    // sendToClient
    telnet.sendToClient(telnet.connectedClients[0].id, "Private message\r\n");
    await sleep(300);
    const privateData = await socketRead(client1, 500);
    record("telnet_sendToClient",
      privateData.toString().includes("Private"),
      `私有消息: "${privateData.toString().trim()}"`);

    // 客户端断开
    client1.destroy();
    await sleep(300);
    record("telnet_disconnect",
      telnet.clientCount === 1 && clientDisconnected,
      `断开后 clientCount=${telnet.clientCount}`);

    client2.destroy();
    await sleep(200);

    // connectedClients
    record("telnet_connectedClients",
      telnet.connectedClients.length === 0,
      `connectedClients.length=${telnet.connectedClients.length}`);

    // ═══════════════════════════════════════════════════════════
    // 4. MCP 服务
    // ═══════════════════════════════════════════════════════════
    section("4. MCP 工具测试");
    startTimer("MCP 创建");

    mcp = new SerialHubMCP(serial, { name: "SerialHub", version: "0.1.0" });
    record("mcp_create", true, "SerialHubMCP 实例已创建");

    // getToolsList
    const toolsList = mcp.getToolsList();
    record("mcp_tools_list", toolsList.length === 6,
      `注册了 ${toolsList.length} 个工具: ${toolsList.map(t => t.name).join(", ")}`);

    // serial_status
    startTimer("serial_status");
    const statusResult = await mcp.callTool("serial_status", {}) as Record<string, unknown>;
    record("mcp_serial_status",
      (statusResult as { connected?: boolean }).connected === true,
      `connected=${(statusResult as { connected?: boolean }).connected}`);

    // serial_write (via MCP)
    startTimer("serial_write (MCP)");
    const writeResult = await mcp.callTool("serial_write", { data: "help" }) as Record<string, unknown>;
    record("mcp_serial_write",
      (writeResult as { success?: boolean }).success === true,
      `success=${(writeResult as { success?: boolean }).success}, bytesWritten=${(writeResult as { bytesWritten?: number }).bytesWritten}`);

    // serial_read (via MCP)
    startTimer("serial_read (MCP)");
    await sleep(2000);
    const readResult = await mcp.callTool("serial_read", { timeout: 3000 }) as Record<string, unknown>;
    const readData = (readResult as { data?: string }).data ?? "";
    record("mcp_serial_read",
      readData.length > 0,
      `${readData.length} 字符, timedOut=${(readResult as { timedOut?: boolean }).timedOut}`);

    // serial_write (no newline)
    startTimer("serial_write no-newline");
    const writeResult2 = await mcp.callTool("serial_write", { data: "version", addNewline: false }) as Record<string, unknown>;
    record("mcp_serial_write_no_newline",
      (writeResult2 as { success?: boolean }).success === true,
      `addNewline=false, success=${(writeResult2 as { success?: boolean }).success}`);

    // serial_list
    startTimer("serial_list (MCP)");
    const listResult = await mcp.callTool("serial_list", {}) as Record<string, unknown>;
    const listPorts = (listResult as { ports?: unknown[] }).ports ?? [];
    record("mcp_serial_list",
      listPorts.length > 0,
      `通过 MCP 列出 ${listPorts.length} 个串口`);

    // DataBuffer
    startTimer("DataBuffer");
    const dataBuffer = mcp.getDataBuffer();
    record("mcp_dataBuffer", true, `DataBuffer 实例获取成功`);

    // ═══════════════════════════════════════════════════════════
    // 5. DataBridge
    // ═══════════════════════════════════════════════════════════
    section("5. DataBridge 数据桥接");
    startTimer("DataBridge");

    bridge = new DataBridge(serial, telnet, mcp, { debugLog: true });
    record("bridge_create", true, "DataBridge 实例已创建");

    let bridgeStarted = false;
    let forwardSerialTelnet = false;
    let forwardTelnetSerial = false;
    bridge.on("started", () => { bridgeStarted = true; });
    bridge.on("forward", (_data: Buffer, from: string, to: string) => {
      if (from === "serial" && to === "telnet") forwardSerialTelnet = true;
      if (from === "telnet" && to === "serial") forwardTelnetSerial = true;
    });

    bridge.start();
    record("bridge_start", bridge.isRunning && bridgeStarted,
      `isRunning=${bridge.isRunning}`);

    // Serial -> Telnet 转发测试
    startTimer("Serial→Telnet 转发");
    const bridgeClient = await telnetConnect(TELNET_PORT);
    await sleep(300);

    await serial.writeLine("help");
    await sleep(2000);
    const bridgeClientData = await socketRead(bridgeClient, 500);
    record("bridge_serial_to_telnet",
      bridgeClientData.length > 0 && forwardSerialTelnet,
      `转发 ${bridgeClientData.length} 字节到 Telnet, event=${forwardSerialTelnet}`);

    // Telnet -> Serial 转发测试
    startTimer("Telnet→Serial 转发");
    let bridgeSerialData = Buffer.alloc(0);
    const bridgeDataHandler = (data: Buffer) => { bridgeSerialData = Buffer.concat([bridgeSerialData, data]); };
    serial.on("data", bridgeDataHandler);

    bridgeClient.write("version\r\n");
    await sleep(2000);
    record("bridge_telnet_to_serial",
      forwardTelnetSerial,
      `Telnet 写入已转发到串口, event=${forwardTelnetSerial}`);

    serial.off("data", bridgeDataHandler);
    bridgeClient.destroy();
    await sleep(200);

    // getOptions
    const bridgeOpts = bridge.getOptions();
    record("bridge_getOptions",
      bridgeOpts.enableTelnet && bridgeOpts.enableMCP,
      `enableTelnet=${bridgeOpts.enableTelnet}, enableMCP=${bridgeOpts.enableMCP}`);

    // ═══════════════════════════════════════════════════════════
    // 6. HTTP 服务器
    // ═══════════════════════════════════════════════════════════
    section("6. HTTP 服务器");
    startTimer("HTTP 启动");

    httpServer = await createMcpHttpServer(mcp, {
      port: MCP_PORT,
      host: "127.0.0.1",
      enableCors: true,
      corsOrigin: "*",
    });
    record("http_start", true, `http://127.0.0.1:${MCP_PORT}`);

    // /health
    startTimer("/health");
    const health = await httpGet(`http://127.0.0.1:${MCP_PORT}/health`);
    const healthBody = health.body ? JSON.parse(health.body) : {};
    record("http_health",
      health.ok && healthBody.status === "ok",
      `status=${health.status}, body.status=${healthBody.status}`);

    // /mcp - initialize
    startTimer("/mcp initialize");
    const initResp = await httpPost(`http://127.0.0.1:${MCP_PORT}/mcp`, {
      jsonrpc: "2.0", method: "initialize", id: 1,
    });
    const initBody = initResp.body ? JSON.parse(initResp.body) : {};
    record("http_mcp_initialize",
      initBody.result?.serverInfo?.name === "SerialHub",
      `serverInfo.name=${initBody.result?.serverInfo?.name}`);

    // /mcp - ping
    startTimer("/mcp ping");
    const pingResp = await httpPost(`http://127.0.0.1:${MCP_PORT}/mcp`, {
      jsonrpc: "2.0", method: "ping", id: 2,
    });
    const pingBody = pingResp.body ? JSON.parse(pingResp.body) : {};
    record("http_mcp_ping",
      pingResp.ok && pingBody.result !== undefined,
      `status=${pingResp.status}`);

    // /mcp - tools/list
    startTimer("/mcp tools/list");
    const toolsResp = await httpPost(`http://127.0.0.1:${MCP_PORT}/mcp`, {
      jsonrpc: "2.0", method: "tools/list", id: 3,
    });
    const toolsBody = toolsResp.body ? JSON.parse(toolsResp.body) : {};
    const httpTools = toolsBody.result?.tools ?? [];
    record("http_mcp_tools_list",
      httpTools.length === 6,
      `通过 HTTP 列出 ${httpTools.length} 个工具`);

    // /mcp - tools/call serial_status
    startTimer("/mcp tools/call");
    const statusResp = await httpPost(`http://127.0.0.1:${MCP_PORT}/mcp`, {
      jsonrpc: "2.0",
      method: "tools/call",
      params: { name: "serial_status" },
      id: 4,
    });
    const statusBody = statusResp.body ? JSON.parse(statusResp.body) : {};
    record("http_mcp_serial_status",
      statusResp.ok && statusBody.result !== undefined,
      `status=${statusResp.status}`);

    // /mcp - tools/call serial_write + serial_read
    startTimer("/mcp tools/call write+read");
    const httpWriteResp = await httpPost(`http://127.0.0.1:${MCP_PORT}/mcp`, {
      jsonrpc: "2.0",
      method: "tools/call",
      params: { name: "serial_write", arguments: { data: "help" } },
      id: 5,
    });
    const httpWriteBody = httpWriteResp.body ? JSON.parse(httpWriteResp.body) : {};
    const writeContent = httpWriteBody.result?.content?.[0]?.text ?? "";
    const writeObj = writeContent ? JSON.parse(writeContent) : {};
    record("http_mcp_serial_write",
      writeObj.success === true,
      `success=${writeObj.success}`);

    await sleep(2000);
    const httpReadResp = await httpPost(`http://127.0.0.1:${MCP_PORT}/mcp`, {
      jsonrpc: "2.0",
      method: "tools/call",
      params: { name: "serial_read", arguments: { timeout: 3000 } },
      id: 6,
    });
    const httpReadBody = httpReadResp.body ? JSON.parse(httpReadResp.body) : {};
    const readContent = httpReadBody.result?.content?.[0]?.text ?? "";
    const readObj = readContent ? JSON.parse(readContent) : {};
    record("http_mcp_serial_read",
      readObj.data?.length > 0,
      `${readObj.data?.length ?? 0} 字符`);

    // /mcp - invalid method
    startTimer("/mcp invalid");
    const invalidResp = await httpPost(`http://127.0.0.1:${MCP_PORT}/mcp`, {
      jsonrpc: "2.0", method: "nonexistent", id: 7,
    });
    const invalidBody = invalidResp.body ? JSON.parse(invalidResp.body) : {};
    record("http_mcp_invalid_method",
      invalidBody.error?.code === -32601,
      `error.code=${invalidBody.error?.code}`);

    // /mcp - bad JSON
    startTimer("/mcp bad request");
    const badResp = await fetch(`http://127.0.0.1:${MCP_PORT}/mcp`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "not json",
      signal: AbortSignal.timeout(5000),
    });
    const badBody = badResp.ok ? await badResp.json() : {};
    record("http_mcp_bad_json",
      (badBody as { error?: { code: number } }).error?.code === -32700,
      `error.code=${(badBody as { error?: { code: number } }).error?.code}`);

    // 404
    startTimer("404");
    const notFound = await httpGet(`http://127.0.0.1:${MCP_PORT}/nonexistent`);
    record("http_404", notFound.status === 404, `status=${notFound.status}`);

    // CORS headers
    startTimer("CORS");
    const corsResp = await fetch(`http://127.0.0.1:${MCP_PORT}/health`, {
      method: "OPTIONS",
      signal: AbortSignal.timeout(5000),
    });
    const corsHeader = corsResp.headers.get("Access-Control-Allow-Origin");
    record("http_cors",
      corsHeader === "*",
      `Access-Control-Allow-Origin=${corsHeader}`);

    // ═══════════════════════════════════════════════════════════
    // 7. ServiceManager
    // ═══════════════════════════════════════════════════════════
    section("7. ServiceManager");
    startTimer("ServiceManager");
    clearServiceStatus();
    writeServiceStatus(MCP_PORT);
    const svcStatus = readServiceStatus();
    record("service_status",
      svcStatus.running && svcStatus.pid === process.pid && svcStatus.port === MCP_PORT,
      `running=${svcStatus.running}, pid=${svcStatus.pid}, port=${svcStatus.port}`);
    clearServiceStatus();

    // ═══════════════════════════════════════════════════════════
    // 8. 系统托盘
    // ═══════════════════════════════════════════════════════════
    section("8. 系统托盘");
    if (SKIP_TRAY) {
      console.log("  ⏭️ 跳过托盘测试 (--skip-tray)");
      record("tray", true, "已跳过 (--skip-tray)");
    } else {
      screenshot("08-before-tray");

      startTimer("托盘启动");
      tray = new TrayManager(serial, { telnetPort: TELNET_PORT, mcpPort: MCP_PORT });
      try {
        await tray.start();
        record("tray_start", true, "托盘守护进程已启动");
      } catch (e) {
        record("tray_start", false, `启动失败: ${e instanceof Error ? e.message : String(e)}`);
      }

      // 状态切换: connected
      if (tray) {
        startTimer("托盘 connected");
        await tray.updateState("connected");
        await sleep(1000);
        expandTrayIcons();
        await sleep(500);
        screenshot("08-tray-connected");
        // 托盘区域截图（右下角）
        screenshotRegion("08-tray-connected-area", 1960, 1310, 600, 150);
        record("tray_state_connected", true, "状态: connected");

        // idle
        startTimer("托盘 idle");
        await tray.updateState("idle");
        await sleep(1000);
        expandTrayIcons();
        await sleep(500);
        screenshot("08-tray-idle");
        screenshotRegion("08-tray-idle-area", 1960, 1310, 600, 150);
        record("tray_state_idle", true, "状态: idle");

        // error
        startTimer("托盘 error");
        await tray.updateState("error");
        await sleep(1000);
        expandTrayIcons();
        await sleep(500);
        screenshot("08-tray-error");
        screenshotRegion("08-tray-error-area", 1960, 1310, 600, 150);
        record("tray_state_error", true, "状态: error");

        // 退出托盘
        startTimer("托盘退出");
        try {
          await tray.quit();
          record("tray_quit", true, "托盘已退出");
          tray = null;
        } catch (e) {
          record("tray_quit", false, `退出失败: ${e instanceof Error ? e.message : String(e)}`);
        }
      }
    }

    // ═══════════════════════════════════════════════════════════
    // 9. 串口断开 + 重连
    // ═══════════════════════════════════════════════════════════
    section("9. 串口断开与重连");
    startTimer("串口断开");

    let disconnectedFired = false;
    serial.on("disconnected", () => { disconnectedFired = true; });

    await serial.disconnect();
    record("serial_disconnect",
      !serial.isConnected && disconnectedFired,
      `isConnected=${serial.isConnected}, event=${disconnectedFired}`);

    // 重连
    startTimer("串口重连");
    let reconnected = false;
    serial.on("connected", () => { reconnected = true; });
    await serial.connect();
    record("serial_reconnect",
      serial.isConnected && reconnected,
      `isConnected=${serial.isConnected}, event=${reconnected}`);

    // MCP disconnect
    startTimer("MCP disconnect");
    const discResult = await mcp.callTool("serial_disconnect", {}) as Record<string, unknown>;
    record("mcp_serial_disconnect",
      (discResult as { success?: boolean }).success === true,
      `success=${(discResult as { success?: boolean }).success}`);

    // MCP connect
    startTimer("MCP connect");
    const connResult = await mcp.callTool("serial_connect", { port: SERIAL_PORT }) as Record<string, unknown>;
    record("mcp_serial_connect",
      (connResult as { success?: boolean }).success === true,
      `success=${(connResult as { success?: boolean }).success}, port=${(connResult as { port?: string }).port}`);

    // MCP connect to non-existent port (error handling)
    startTimer("MCP connect error");
    const errResult = await mcp.callTool("serial_connect", { port: "COM999" }) as Record<string, unknown>;
    record("mcp_serial_connect_error",
      (errResult as { success?: boolean }).success === false,
      `success=${(errResult as { success?: boolean }).success} (预期失败)`);

  } catch (e) {
    console.error("\n❌ 测试异常:", e instanceof Error ? e.message : String(e));
    if (e instanceof Error && e.stack) {
      console.error(e.stack);
    }
  } finally {
    // ═══════════════════════════════════════════════════════════
    // 清理
    // ═══════════════════════════════════════════════════════════
    section("清理");
    try {
      if (tray) { await tray.quit(); console.log("  托盘已清理"); }
      if (bridge) { bridge.stop(); console.log("  桥接已停止"); }
      if (telnet?.isRunning) { await telnet.stop(); console.log("  Telnet 已停止"); }
      if (httpServer) { await httpServer.close(); console.log("  HTTP 已关闭"); }
      if (serial?.isConnected) { await serial.disconnect(); console.log("  串口已断开"); }
      if (mcp) { mcp.dispose(); console.log("  MCP 已清理"); }
      clearServiceStatus();
      console.log("  清理完成 ✓");
    } catch (e) {
      console.error("  清理出错:", e);
    }
  }

  // ═══════════════════════════════════════════════════════════
  // 报告生成
  // ═══════════════════════════════════════════════════════════
  generateReport();
}

// ─── HTML 报告 ──────────────────────────────────────────────

function generateReport(): void {
  const passed = results.filter(r => r.pass).length;
  const failed = results.filter(r => !r.pass).length;
  const total = results.length;
  const totalTime = results.reduce((sum, r) => sum + r.duration, 0);

  const screenshots = existsSync(SCREENSHOT_DIR)
    ? readdirSync(SCREENSHOT_DIR).filter(f => f.endsWith(".png")).sort()
    : [];

  const html = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>SerialHub E2E Test Report</title>
<style>
  :root { --bg: #0f0f0f; --card: #1a1a1a; --border: #2a2a2a; --text: #e0e0e0; --muted: #888; --green: #4ade80; --red: #f87171; --blue: #60a5fa; --yellow: #fbbf24; }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: 'Segoe UI', system-ui, -apple-system, sans-serif; background: var(--bg); color: var(--text); padding: 40px 32px; max-width: 1400px; margin: 0 auto; line-height: 1.6; }
  h1 { font-size: 32px; font-weight: 700; margin-bottom: 4px; }
  .subtitle { color: var(--muted); font-size: 14px; margin-bottom: 32px; }
  .summary { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; margin-bottom: 32px; }
  .card { background: var(--card); border: 1px solid var(--border); border-radius: 12px; padding: 20px 24px; text-align: center; }
  .card .num { font-size: 36px; font-weight: 700; }
  .card .label { font-size: 13px; color: var(--muted); margin-top: 4px; }
  .pass-rate { font-size: 28px; }
  table { width: 100%; border-collapse: collapse; margin-bottom: 40px; }
  th { text-align: left; padding: 12px 16px; background: var(--card); border-bottom: 2px solid var(--border); font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: 0.5px; }
  td { padding: 10px 16px; border-bottom: 1px solid var(--border); font-size: 14px; }
  tr:hover td { background: #ffffff08; }
  .badge { display: inline-block; padding: 2px 12px; border-radius: 999px; font-size: 12px; font-weight: 600; }
  .badge-pass { background: #16a34a22; color: var(--green); }
  .badge-fail { background: #dc262622; color: var(--red); }
  .detail { color: var(--muted); font-size: 13px; font-family: 'Cascadia Code', 'Fira Code', monospace; }
  h2 { font-size: 20px; margin: 32px 0 16px; font-weight: 600; }
  .screenshots { display: grid; grid-template-columns: repeat(auto-fill, minmax(400px, 1fr)); gap: 16px; }
  .screenshot-card { background: var(--card); border: 1px solid var(--border); border-radius: 12px; overflow: hidden; }
  .screenshot-card img { width: 100%; display: block; cursor: pointer; }
  .screenshot-card img:hover { opacity: 0.9; }
  .screenshot-card .caption { padding: 10px 16px; font-size: 13px; color: var(--muted); }
  .duration { color: var(--yellow); font-size: 12px; font-variant-numeric: tabular-nums; }
  @media (max-width: 768px) {
    body { padding: 20px 16px; }
    .summary { grid-template-columns: repeat(2, 1fr); }
    .screenshots { grid-template-columns: 1fr; }
  }
</style>
</head>
<body>
  <h1>SerialHub E2E Test Report</h1>
  <p class="subtitle">${new Date().toLocaleString()} | ${SERIAL_PORT} @ 115200bps | Telnet:${TELNET_PORT} MCP:${MCP_PORT}</p>

  <div class="summary">
    <div class="card"><div class="num" style="color:var(--blue)">${total}</div><div class="label">Total</div></div>
    <div class="card"><div class="num" style="color:var(--green)">${passed}</div><div class="label">Passed</div></div>
    <div class="card"><div class="num" style="color:var(--red)">${failed}</div><div class="label">Failed</div></div>
    <div class="card"><div class="num pass-rate" style="color:${total > 0 && failed === 0 ? 'var(--green)' : 'var(--yellow)'}">${total > 0 ? Math.round(passed / total * 100) : 0}%</div><div class="label">Pass Rate</div></div>
  </div>

  <table>
    <thead>
      <tr><th>#</th><th>Status</th><th>Test</th><th>Detail</th><th>Time</th></tr>
    </thead>
    <tbody>
${results.map((r, i) => {
  const rowColor = r.pass ? '' : ' style="background:#f8717108"';
  return `      <tr${rowColor}>
        <td>${i + 1}</td>
        <td><span class="badge ${r.pass ? 'badge-pass' : 'badge-fail'}">${r.pass ? 'PASS' : 'FAIL'}</span></td>
        <td>${r.name}</td>
        <td class="detail">${escapeHtml(r.detail)}</td>
        <td class="duration">${r.duration}ms</td>
      </tr>`;
}).join("\n")}
    </tbody>
  </table>

${screenshots.length > 0 ? `  <h2>Screenshots (${screenshots.length})</h2>
  <div class="screenshots">
${screenshots.map(s => `    <div class="screenshot-card">
      <img src="test-screenshots/${s}" alt="${s}" loading="lazy">
      <div class="caption">${s.replace(/\.png$/, '')}</div>
    </div>`).join("\n")}
  </div>` : ""}
</body>
</html>`;

  const reportPath = resolve(__dirname, "test-report.html");
  writeFileSync(reportPath, html, "utf-8");

  console.log("\n" + "═".repeat(60));
  console.log(`  📊 结果: ${passed}/${total} 通过, ${failed} 失败 (${Math.round(passed / total * 100)}%)`);
  console.log(`  ⏱️ 总耗时: ${totalTime}ms`);
  console.log(`  📄 报告: ${reportPath}`);
  console.log(`  📸 截图: ${SCREENSHOT_DIR} (${screenshots.length} 张)`);
  console.log("═".repeat(60));

  // 打开报告
  try {
    execSync(`start "" "${reportPath}"`, { stdio: "pipe" });
  } catch { /* 非关键 */ }

  process.exit(failed > 0 ? 1 : 0);
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

// ─── 入口 ───────────────────────────────────────────────────
main();
