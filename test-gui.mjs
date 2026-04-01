import { SerialManager } from "./dist/serial/SerialManager.js";
import { TelnetServer } from "./dist/telnet/TelnetServer.js";
import { SerialHubMCP } from "./dist/mcp/index.js";
import { DataBridge } from "./dist/bridge/DataBridge.js";
import { createMcpHttpServer } from "./dist/mcp/transport/http-sse.js";
import { TrayManager } from "./dist/tray/TrayManager.js";
import { execSync, spawn } from "node:child_process";
import { mkdirSync, existsSync, writeFileSync, rmSync } from "node:fs";
import { resolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout } from "timers/promises";

const __dirname = dirname(fileURLToPath(import.meta.url));

const SCREENSHOT_DIR = resolve("test-screenshots");
const PORT = 5000;

function screenshot(name) {
  const filepath = join(SCREENSHOT_DIR, `${name}.png`);
  try {
    execSync(
      `powershell -NoProfile -ExecutionPolicy Bypass -File "${join(__dirname, "screenshot.ps1")}" -OutputPath "${filepath}"`,
      { stdio: "pipe", timeout: 10000 }
    );
    if (existsSync(filepath)) {
      console.log(`  Screenshot: ${name}.png`);
      return filepath;
    }
  } catch {
    console.log(`  Screenshot failed: ${name}`);
  }
  return "";
}

function screenshotTray(name) {
  const filepath = join(SCREENSHOT_DIR, `${name}.png`);
  try {
    const screenWidth = 2560;
    const screenHeight = 1440;
    const x = screenWidth - 600;
    const y = screenHeight - 150;
    execSync(
      `powershell -NoProfile -ExecutionPolicy Bypass -File "${join(__dirname, "screenshot.ps1")}" -OutputPath "${filepath}" -X ${x} -Y ${y} -Width 600 -Height 150`,
      { stdio: "pipe", timeout: 10000 }
    );
    if (existsSync(filepath)) {
      console.log(`  Tray screenshot: ${name}.png`);
      return filepath;
    }
  } catch {
    console.log(`  Tray screenshot failed: ${name}`);
  }
  return "";
}

function expandTrayIcons() {
  try {
    execSync(
      `powershell -NoProfile -ExecutionPolicy Bypass -File "${join(__dirname, "expand-tray.ps1")}"`,
      { stdio: "pipe", timeout: 10000 }
    );
    console.log("  Expanded tray icons");
  } catch {
    console.log("  Could not expand tray");
  }
}

async function httpGet(url) {
  try {
    const res = await fetch(url, { signal: AbortSignal.timeout(5000) });
    const body = await res.text();
    return { ok: res.ok, status: res.status, body };
  } catch {
    return { ok: false };
  }
}

async function httpPost(url, body) {
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(10000),
    });
    const text = await res.text();
    return { ok: res.ok, status: res.status, body: text };
  } catch {
    return { ok: false };
  }
}

async function main() {
  const results = [];

  function record(name, pass, detail) {
    results.push({ name, pass, detail });
    console.log(`  ${pass ? "OK" : "FAIL"} ${name}: ${detail}`);
  }

  if (existsSync(SCREENSHOT_DIR)) rmSync(SCREENSHOT_DIR, { recursive: true });
  mkdirSync(SCREENSHOT_DIR, { recursive: true });

  console.log("=".repeat(60));
  console.log("  SerialHub GUI Test");
  console.log("  " + new Date().toLocaleString());
  console.log("=".repeat(60));

  let serial = null;
  let telnet = null;
  let mcp = null;
  let bridge = null;
  let httpServer = null;
  let tray = null;

  try {
    console.log("\n[1] Serial port list");
    const ports = await SerialManager.listPorts();
    record("list", ports.length > 0, `Found ${ports.length} ports: ${ports.map(p => p.path).join(", ")}`);
    const hasCom7 = ports.some(p => p.path === "COM7");
    record("COM7 exists", hasCom7, hasCom7 ? "COM7 found" : "COM7 not found");

    console.log("\n[2] Connect COM7");
    serial = new SerialManager({
      port: "COM7",
      baudRate: 115200,
      dataBits: 8,
      parity: "none",
      stopBits: 1,
    });
    await serial.connect();
    record("connect", serial.isConnected, `isConnected=${serial.isConnected}, port=${serial.currentPort}`);

    console.log("\n[3] Serial communication");
    let received = Buffer.alloc(0);
    serial.on("data", (data) => { received = Buffer.concat([received, data]); });
    await serial.writeLine("help");
    await setTimeout(3000);
    record("help response", received.length > 0, `${received.length} bytes`);

    received = Buffer.alloc(0);
    await serial.writeLine("version");
    await setTimeout(2000);
    record("version response", received.length > 0, `${received.length} bytes`);

    console.log("\n[4] Telnet server");
    telnet = new TelnetServer();
    await telnet.start(2323);
    record("telnet", telnet.isRunning, `port 2323, isRunning=${telnet.isRunning}`);

    console.log("\n[5] MCP service");
    mcp = new SerialHubMCP(serial, { name: "SerialHub", version: "0.1.0" });
    record("mcp", true, "MCP created");

    console.log("\n[6] Data bridge");
    bridge = new DataBridge(serial, telnet, mcp, { debugLog: true });
    bridge.start();
    record("bridge", true, "Bridge started");

    console.log("\n[7] HTTP server");
    httpServer = await createMcpHttpServer(mcp, { port: PORT, host: "127.0.0.1", enableCors: true });
    record("http", true, `http://127.0.0.1:${PORT}`);

    const health = await httpGet(`http://127.0.0.1:${PORT}/health`);
    record("health", health.ok, `status=${health.status}`);

    console.log("\n[8] MCP tools");
    const statusResult = await mcp.callTool("serial_status", {});
    record("serial_status", statusResult?.connected === true, `connected=${statusResult?.connected}`);

    const writeResult = await mcp.callTool("serial_write", { data: "help" });
    record("serial_write", writeResult?.success === true, `success=${writeResult?.success}`);

    await setTimeout(2000);
    const readResult = await mcp.callTool("serial_read", { timeout: 3000 });
    record("serial_read", (readResult?.data ?? "").length > 0, `${(readResult?.data ?? "").length} chars`);

    console.log("\n[9] HTTP MCP endpoint");
    const mcpResp = await httpPost(`http://127.0.0.1:${PORT}/mcp`, {
      jsonrpc: "2.0", method: "tools/call",
      params: { name: "serial_status" }, id: 1,
    });
    record("mcp http", mcpResp.ok, `status=${mcpResp.status}`);

    console.log("\n[10] System tray");
    screenshot("10-before-tray");
    expandTrayIcons();
    await setTimeout(500);
    screenshotTray("10-tray-before");

    tray = new TrayManager(serial, { telnetPort: 2323, mcpPort: PORT });
    try {
      await tray.start();
      record("tray start", true, "Tray started");
    } catch (e) {
      record("tray start", false, `Failed: ${e instanceof Error ? e.message : String(e)}`);
    }

    await tray.updateState("connected");
    await setTimeout(1000);
    expandTrayIcons();
    await setTimeout(500);
    screenshot("10-tray-connected");
    screenshotTray("10-tray-connected-area");

    await tray.updateState("idle");
    await setTimeout(1000);
    expandTrayIcons();
    await setTimeout(500);
    screenshot("10-tray-idle");
    screenshotTray("10-tray-idle-area");

    await tray.updateState("error");
    await setTimeout(1000);
    expandTrayIcons();
    await setTimeout(500);
    screenshot("10-tray-error");
    screenshotTray("10-tray-error-area");

    record("tray states", true, "idle->connected->idle->error");

    try {
      await tray.quit();
      record("tray quit", true, "Tray quit");
    } catch (e) {
      record("tray quit", false, `Failed: ${e instanceof Error ? e.message : String(e)}`);
    }
    tray = null;

  } catch (e) {
    console.error("\nTest error:", e instanceof Error ? e.message : String(e));
  } finally {
    console.log("\n[Cleanup]");
    if (tray) { try { await tray.quit(); } catch {} }
    if (bridge) bridge.stop();
    if (telnet?.isRunning) await telnet.stop();
    if (httpServer) await httpServer.close();
    if (serial?.isConnected) await serial.disconnect();
    if (mcp) mcp.dispose();
    console.log("[Cleanup] Done");
  }

  const passed = results.filter(r => r.pass).length;
  const failed = results.filter(r => !r.pass).length;
  const total = results.length;

  console.log("\n" + "=".repeat(60));
  console.log(`  Results: ${passed}/${total} passed, ${failed} failed`);
  console.log("=".repeat(60));

  const screenshots = existsSync(SCREENSHOT_DIR)
    ? Array.from(new Bun.Glob("*").scanSync({ cwd: SCREENSHOT_DIR, absolute: false }))
    : [];

  console.log(`Screenshots: ${SCREENSHOT_DIR} (${screenshots.length} files)`);

  process.exit(failed > 0 ? 1 : 0);
}

main();
