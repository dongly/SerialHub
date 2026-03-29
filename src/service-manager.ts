import { writeFileSync, readFileSync, unlinkSync, existsSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { request } from "node:http";

const RUNTIME_DIR = join(tmpdir(), "serialhub");
const PID_FILE = join(RUNTIME_DIR, "serialhub.pid");
const PORT_FILE = join(RUNTIME_DIR, "serialhub.port");

function ensureRuntimeDir(): void {
  if (!existsSync(RUNTIME_DIR)) {
    mkdirSync(RUNTIME_DIR, { recursive: true });
  }
}

export interface ServiceStatus {
  running: boolean;
  pid?: number;
  port?: number;
}

export function writeServiceStatus(port: number): void {
  ensureRuntimeDir();
  writeFileSync(PID_FILE, String(process.pid), "utf-8");
  writeFileSync(PORT_FILE, String(port), "utf-8");
}

export function clearServiceStatus(): void {
  try {
    if (existsSync(PID_FILE)) unlinkSync(PID_FILE);
    if (existsSync(PORT_FILE)) unlinkSync(PORT_FILE);
  } catch (_e) { /* ignored */ }
}

export function readServiceStatus(): ServiceStatus {
  try {
    if (!existsSync(PID_FILE) || !existsSync(PORT_FILE)) {
      return { running: false };
    }

    const pid = parseInt(readFileSync(PID_FILE, "utf-8"), 10);
    const port = parseInt(readFileSync(PORT_FILE, "utf-8"), 10);

    try {
      process.kill(pid, 0);
      return { running: true, pid, port };
    } catch {
      clearServiceStatus();
      return { running: false };
    }
  } catch {
    return { running: false };
  }
}

export async function checkServiceHealth(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const req = request(
      { hostname: "127.0.0.1", port, path: "/health", method: "GET", timeout: 2000 },
      (res) => {
        resolve(res.statusCode === 200);
      }
    );
    req.on("error", () => resolve(false));
    req.on("timeout", () => { req.destroy(); resolve(false); });
    req.end();
  });
}