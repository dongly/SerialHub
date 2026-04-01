import { SerialManager } from "./dist/serial/SerialManager.js";
import { TelnetServer } from "./dist/telnet/TelnetServer.js";
import { SerialHubMCP } from "./dist/mcp/index.js";
import { DataBridge } from "./dist/bridge/DataBridge.js";
import { createMcpHttpServer } from "./dist/mcp/transport/http-sse.js";
import { TrayManager } from "./dist/tray/TrayManager.js";
import { execSync } from "node:child_process";
import { mkdirSync, existsSync, writeFileSync, rmSync } from "node:fs";
import { resolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout } from "timers/promises";

const __dirname = dirname(fileURLToPath(import.meta.url));

const SCREENSHOT_DIR = resolve("test-screenshots");
const PORT = 5000;

function screenshot(name: string): string {
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

function screenshotTray(name: string): string {
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

function expandTrayIcons(): void {
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

async function httpGet(url: string): Promise<{ ok: boolean; status?: number; body?: string }> {
  try {
    const res = await fetch(url, { signal: AbortSignal.timeout(5000) });
    const body = await res.text();
    return { ok: res.ok, status: res.status, body };
  } catch {
    return { ok: false };
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
  } catch {
    return { ok: false };
  }
}

async function main() {
  const results: { name: string; pass: boolean; detail: string }[] = [];

  function record(name: string, pass: boolean, detail: string) {
    results.push({ name, pass, detail });
    console.log(`  ${pass ? "OK" : "FAIL"} ${name}: ${detail}`);
  }

  if (existsSync(SCREENSHOT_DIR)) rmSync(SCREENSHOT_DIR, { recursive: true });
  mkdirSync(SCREENSHOT_DIR, { recursive: true });

  console.log("=".repeat(60));
  console.log("  SerialHub GUI Test");
  console.log("  " + new Date().toLocaleString());
  console.log("=".repeat(60));

  let serial: SerialManager | null = null;
  let telnet: TelnetServer | null = null;
  let mcp: SerialHubMCP | null = null;
  let bridge: DataBridge | null = null;
  let httpServer: { close: () => Promise<void> } | null = null;
  let tray: TrayManager | null = null;

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
    serial.on("data", (data: Buffer) => { received = Buffer.concat([received, data]); });
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
    const statusResult = await mcp.callTool("serial_status", {}) as Record<string, unknown>;
    record("serial_status", (statusResult as { connected?: boolean }).connected === true, `connected=${(statusResult as { connected?: boolean }).connected}`);

    const writeResult = await mcp.callTool("serial_write", { data: "help" }) as Record<string, unknown>;
    record("serial_write", (writeResult as { success?: boolean }).success === true, `success=${(writeResult as { success?: boolean }).success}`);

    await setTimeout(2000);
    const readResult = await mcp.callTool("serial_read", { timeout: 3000 }) as Record<string, unknown>;
    const readData = (readResult as { data?: string }).data ?? "";
    record("serial_read", readData.length > 0, `${readData.length} chars`);

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
    ? execSync(`ls ${SCREENSHOT_DIR}`, { encoding: "utf-8" }).trim().split("\n").filter(Boolean)
    : [];

  const html = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<title>SerialHub Test Report</title>
<style>
  body { font-family: system-ui, sans-serif; background: #0a0a0a; color: #e0e0e0; padding: 40px; }
  h1 { font-size: 28px; margin-bottom: 8px; }
  .subtitle { color: #888; margin-bottom: 32px; font-size: 14px; }
  .summary { display: flex; gap: 16px; margin-bottom: 32px; }
  .card { background: #1a1a1a; border: 1px solid #333; border-radius: 12px; padding: 20px 28px; flex: 1; text-align: center; }
  .card .num { font-size: 36px; font-weight: 700; }
  .card .label { font-size: 13px; color: #888; margin-top: 4px; }
  .pass { color: #4ade80; }
  .fail { color: #f87171; }
  table { width: 100%; border-collapse: collapse; margin-bottom: 32px; }
  th { text-align: left; padding: 12px 16px; background: #1a1a1a; border-bottom: 2px solid #333; font-size: 13px; color: #888; }
  td { padding: 12px 16px; border-bottom: 1px solid #222; font-size: 14px; }
  tr:hover td { background: #111; }
  .badge { display: inline-block; padding: 2px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; }
  .badge-pass { background: #16a34a22; color: #4ade80; }
  .badge-fail { background: #dc262622; color: #f87171; }
  h2 { font-size: 20px; margin: 32px 0 16px; }
  .screenshots { display: grid; grid-template-columns: repeat(auto-fill, minmax(360px, 1fr)); gap: 16px; }
  .screenshot-card { background: #1a1a1a; border: 1px solid #333; border-radius: 12px; overflow: hidden; }
  .screenshot-card img { width: 100%; display: block; }
  .screenshot-card .caption { padding: 10px 16px; font-size: 13px; color: #888; }
</style>
</head>
<body>
  <h1>SerialHub Test Report</h1>
  <p class="subtitle">${new Date().toLocaleString()} | COM7 @ 115200bps</p>

  <div class="summary">
    <div class="card"><div class="num" style="color:#60a5fa">${total}</div><div class="label">Total</div></div>
    <div class="card"><div class="num pass">${passed}</div><div class="label">Passed</div></div>
    <div class="card"><div class="num fail">${failed}</div><div class="label">Failed</div></div>
  </div>

  <table>
    <thead><tr><th>#</th><th>Status</th><th>Test</th><th>Detail</th></tr></thead>
    <tbody>
${results.map((r, i) => `      <tr>
        <td>${i + 1}</td>
        <td><span class="badge ${r.pass ? "badge-pass" : "badge-fail"}">${r.pass ? "PASS" : "FAIL"}</span></td>
        <td>${r.name}</td>
        <td style="color:#aaa;font-size:13px">${r.detail}</td>
      </tr>`).join("\n")}
    </tbody>
  </table>

${screenshots.length > 0 ? `  <h2>Screenshots</h2>
  <div class="screenshots">
${screenshots.map(s => `    <div class="screenshot-card">
      <img src="test-screenshots/${s}" alt="${s}">
      <div class="caption">${s}</div>
    </div>`).join("\n")}
  </div>` : ""}
</body>
</html>`;

  const reportPath = resolve("test-report.html");
  writeFileSync(reportPath, html, "utf-8");
  console.log(`\nReport: ${reportPath}`);
  console.log(`Screenshots: ${SCREENSHOT_DIR} (${screenshots.length} files)`);

  try {
    execSync(`start "" "${reportPath}"`, { stdio: "pipe" });
    console.log("Report opened in browser");
  } catch {}

  process.exit(failed > 0 ? 1 : 0);
}

main();
