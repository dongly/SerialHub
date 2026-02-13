/**
 * DataBridge - 数据桥接中心
 * 管理串口、Telnet 和 MCP 之间的数据转发
 */

import { EventEmitter } from "events";
import type { SerialManager } from "../serial/SerialManager.js";
import type { TelnetServer, TelnetClient } from "../telnet/TelnetServer.js";
import type { SerialHubMCP } from "../mcp/index.js";

/**
 * DataBridge 配置选项
 */
export interface DataBridgeOptions {
  /** 启用 Telnet 转发，默认 true */
  enableTelnet?: boolean;
  /** 启用 MCP 转发，默认 true */
  enableMCP?: boolean;
  /** 启用调试日志，默认 false */
  debugLog?: boolean;
}

/**
 * DataBridge 事件映射
 */
export interface DataBridgeEvents {
  /** 桥接启动 */
  started: () => void;
  /** 桥接停止 */
  stopped: () => void;
  /** 串口数据 */
  "serial-data": (data: Buffer, direction: "in" | "out") => void;
  /** Telnet 数据 */
  "telnet-data": (data: Buffer, client: TelnetClient) => void;
  /** MCP 数据 */
  "mcp-data": (data: Buffer) => void;
  /** 数据转发 */
  forward: (data: Buffer, from: string, to: string) => void;
  /** 发生错误 */
  error: (error: Error) => void;
}

/**
 * 数据桥接中心
 * 负责管理串口、Telnet 和 MCP 之间的数据转发
 */
export class DataBridge extends EventEmitter {
  private serial: SerialManager;
  private telnet: TelnetServer | null;
  private mcp: SerialHubMCP | null;
  private options: Required<DataBridgeOptions>;
  private _running: boolean = false;

  // 绑定的事件处理器，用于移除监听器
  private boundHandleSerialData: (data: Buffer) => void;
  private boundHandleTelnetData: (data: Buffer, client: TelnetClient) => void;

  /**
   * 创建数据桥接实例
   * @param serial 串口管理器
   * @param telnet Telnet 服务器（可选）
   * @param mcp MCP 服务实例（可选）
   * @param options 配置选项
   */
  constructor(
    serial: SerialManager,
    telnet: TelnetServer | null,
    mcp: SerialHubMCP | null,
    options: DataBridgeOptions = {}
  ) {
    super();

    this.serial = serial;
    this.telnet = telnet;
    this.mcp = mcp;

    this.options = {
      enableTelnet: options.enableTelnet ?? true,
      enableMCP: options.enableMCP ?? true,
      debugLog: options.debugLog ?? false,
    };

    // 预绑定事件处理器
    this.boundHandleSerialData = this.handleSerialData.bind(this);
    this.boundHandleTelnetData = this.handleTelnetData.bind(this);
  }

  /**
   * 桥接是否正在运行
   */
  get isRunning(): boolean {
    return this._running;
  }

  /**
   * 启动数据桥接
   */
  start(): void {
    if (this._running) {
      return;
    }

    // 串口数据 -> Telnet + MCP
    this.serial.on("data", this.boundHandleSerialData);

    // Telnet 数据 -> 串口
    if (this.telnet && this.options.enableTelnet) {
      this.telnet.on("data", this.boundHandleTelnetData);
    }

    this._running = true;
    this.emit("started");

    if (this.options.debugLog) {
      console.log("[DataBridge] 桥接已启动");
    }
  }

  /**
   * 停止数据桥接
   */
  stop(): void {
    if (!this._running) {
      return;
    }

    // 移除串口数据监听
    this.serial.off("data", this.boundHandleSerialData);

    // 移除 Telnet 数据监听
    if (this.telnet) {
      this.telnet.off("data", this.boundHandleTelnetData);
    }

    this._running = false;
    this.emit("stopped");

    if (this.options.debugLog) {
      console.log("[DataBridge] 桥接已停止");
    }
  }

  /**
   * 处理串口数据
   * 转发到 Telnet 和 MCP
   */
  private handleSerialData(data: Buffer): void {
    // 发出串口数据事件
    this.emit("serial-data", data, "in");

    // 转发到 Telnet（广播）
    if (this.telnet && this.options.enableTelnet) {
      try {
        this.telnet.broadcast(data);
        this.emit("forward", data, "serial", "telnet");
      } catch (error) {
        this.emit("error", error instanceof Error ? error : new Error(String(error)));
      }
    }

    // 转发到 MCP 缓冲区
    if (this.mcp && this.options.enableMCP) {
      try {
        this.mcp.getDataBuffer().append(data);
        this.emit("forward", data, "serial", "mcp");
      } catch (error) {
        this.emit("error", error instanceof Error ? error : new Error(String(error)));
      }
    }

    // 调试日志
    if (this.options.debugLog) {
      console.log("[Serial ->]", data.toString("utf-8"));
    }
  }

  /**
   * 处理 Telnet 数据
   * 转发到串口
   */
  private handleTelnetData(data: Buffer, client: TelnetClient): void {
    // 发出 Telnet 数据事件
    this.emit("telnet-data", data, client);

    // 转发到串口
    if (this.serial.isConnected) {
      try {
        this.serial.write(data);
        this.emit("serial-data", data, "out");
        this.emit("forward", data, "telnet", "serial");
      } catch (error) {
        this.emit("error", error instanceof Error ? error : new Error(String(error)));
      }
    }

    // 调试日志
    if (this.options.debugLog) {
      console.log(`[Telnet -> ${client.remoteAddress}]`, data.toString("utf-8"));
    }
  }

  /**
   * 获取当前配置
   */
  getOptions(): Required<DataBridgeOptions> {
    return { ...this.options };
  }

  /**
   * 更新配置
   * 注意：某些配置更改需要重启桥接才能生效
   */
  updateOptions(options: Partial<DataBridgeOptions>): void {
    this.options = {
      ...this.options,
      ...options,
    };
  }
}
