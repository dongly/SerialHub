import { EventEmitter } from "node:events";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { createTray, type Tray, type MenuItemTemplate } from "tray-hook";
import type { SerialManager } from "../serial/SerialManager.js";
import { hideConsole, showConsole, toggleConsole, isConsoleHidden, isWindows } from "./console.js";

type TrayState = "idle" | "connected" | "error";

export interface TrayManagerEvents {
  quit: [];
  "serial-connect": [];
  "serial-disconnect": [];
}

export class TrayManager extends EventEmitter {
  private tray: Tray;
  private serial: SerialManager;
  private state: TrayState = "idle";
  private telnetPort: number;
  private mcpPort: number;
  private config: ReturnType<SerialManager["getConfig"]>;
  private iconDir: string;

  constructor(serial: SerialManager, ports: { telnetPort: number; mcpPort: number }) {
    super();
    this.serial = serial;
    this.telnetPort = ports.telnetPort;
    this.mcpPort = ports.mcpPort;
    this.config = serial.getConfig();

    const __dirname = dirname(fileURLToPath(import.meta.url));
    this.iconDir = join(__dirname, "..", "..", "assets");

    this.tray = createTray();
    this.bindEvents();
  }

  async start(): Promise<void> {
    console.error("[SerialHub] 正在启动托盘...");
    console.error("[SerialHub] 图标目录:", this.iconDir);
    
    try {
      await this.tray.start();
      console.error("[SerialHub] 托盘守护进程已启动");
    } catch (e) {
      console.error("[SerialHub] 托盘启动失败:", e);
      throw e;
    }

    const idleIcon = join(this.iconDir, "tray-idle.png");
    console.error("[SerialHub] 图标路径:", idleIcon);

    await this.tray.defineStates({
      idle: idleIcon,
      connected: join(this.iconDir, "tray-connected.png"),
      error: join(this.iconDir, "tray-error.png"),
    });

    await this.tray.setState("idle");
    await this.tray.setTooltip("SerialHub — 未连接");

    await this.buildMenu();

    this.tray.on("error", (err) => {
      console.error("[SerialHub] 托盘错误:", err.message);
    });

    this.tray.on("exit", (code) => {
      if (code !== 0) {
        console.error(`[SerialHub] 托盘守护进程异常退出 (code=${code})`);
      }
    });

    this.tray.on("restart", () => {
      console.error("[SerialHub] 托盘守护进程已恢复");
    });
    
    console.error("[SerialHub] 托盘初始化完成");
  }

  private buildMenu(): void {
    const menu: MenuItemTemplate[] = this.createMenuTemplate();
    this.tray.setMenu(menu);
  }

  private createMenuTemplate(): MenuItemTemplate[] {
    const portInfo = this.config.port
      ? `${this.config.port} @ ${this.config.baudRate}bps`
      : "未连接";

    return [
      { type: "item", id: "serial", title: `串口: ${portInfo}` },
      { type: "item", id: "ports", title: `Telnet: ${this.telnetPort}  MCP: ${this.mcpPort}`, enabled: false },
      { type: "separator", id: "sep1" },
      { type: "item", id: "toggle-console", title: isWindows ? (isConsoleHidden() ? "显示控制台" : "隐藏控制台") : "控制台 (仅 Windows)", enabled: isWindows },
      { type: "separator", id: "sep2" },
      { type: "item", id: "quit", title: "退出" },
    ];
  }

  private bindEvents(): void {
    this.tray.on("click", async (id) => {
      switch (id) {
        case "serial":
          await this.handleSerialToggle();
          break;
        case "toggle-console":
          toggleConsole();
          await this.tray.rename("toggle-console", isConsoleHidden() ? "显示控制台" : "隐藏控制台");
          break;
        case "quit":
          await this.tray.quit();
          this.emit("quit");
          process.exit(0);
          break;
      }
    });

    this.tray.on("tray_click", (button) => {
      if (button === "double" && isWindows) {
        toggleConsole();
      }
    });
  }

  private async handleSerialToggle(): Promise<void> {
    if (this.serial.isConnected) {
      await this.serial.disconnect();
      this.emit("serial-disconnect");
    } else if (this.config.port) {
      this.emit("serial-connect");
      try {
        await this.serial.connect();
      } catch (err) {
        console.error("[SerialHub] 从托盘连接串口失败:", err);
      }
    }
  }

  async updateState(state: TrayState): Promise<void> {
    this.state = state;
    await this.tray.setState(state);

    const tooltips: Record<TrayState, string> = {
      idle: "SerialHub — 未连接",
      connected: `SerialHub — ${this.config.port}`,
      error: "SerialHub — 错误",
    };
    await this.tray.setTooltip(tooltips[state]);
  }

  async updateSerialStatus(): Promise<void> {
    const portInfo = this.config.port
      ? `${this.config.port} @ ${this.config.baudRate}bps`
      : "未连接";
    await this.tray.rename("serial", portInfo);
  }

  async show(): Promise<void> {
    if (isWindows) {
      showConsole();
    }
  }

  async hide(): Promise<void> {
    if (isWindows) {
      hideConsole();
    }
  }

  async quit(): Promise<void> {
    await this.tray.quit();
  }
}
