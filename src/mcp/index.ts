/**
 * SerialHub MCP 服务入口
 * 封装 MCP Server 和工具注册
 */

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { SerialManager } from "../serial/SerialManager.js";

// 导入工具
import {
  serialListTool,
  executeSerialList,
  SerialListResult,
} from "./tools/serial_list.js";
import {
  serialConnectTool,
  executeSerialConnect,
  SerialConnectInput,
  SerialConnectResult,
} from "./tools/serial_connect.js";
import {
  serialDisconnectTool,
  executeSerialDisconnect,
  SerialDisconnectResult,
} from "./tools/serial_disconnect.js";
import {
  serialWriteTool,
  executeSerialWrite,
  SerialWriteInput,
  SerialWriteResult,
} from "./tools/serial_write.js";
import {
  serialReadTool,
  executeSerialRead,
  DataBuffer,
  SerialReadInput,
  SerialReadResult,
} from "./tools/serial_read.js";
import {
  serialStatusTool,
  executeSerialStatus,
  SerialStatusResult,
} from "./tools/serial_status.js";

/**
 * SerialHub MCP 服务配置
 */
export interface SerialHubMCPConfig {
  /** 服务名称 */
  name?: string;
  /** 服务版本 */
  version?: string;
  /** 自动设置数据监听器，默认 true。设为 false 时由 DataBridge 控制数据流 */
  autoDataListener?: boolean;
}

/**
 * SerialHub MCP 服务类
 * 封装 MCP Server 和工具注册
 */
export class SerialHubMCP {
  private server: McpServer;
  private serial: SerialManager;
  private dataBuffer: DataBuffer;
  private dataListener: ((data: Buffer) => void) | null = null;
  private autoDataListener: boolean;
  private subscribers: Set<string> = new Set(); // 订阅者列表

  /**
   * 创建 SerialHub MCP 服务实例
   * @param serial 串口管理器实例
   * @param config 服务配置
   */
  constructor(serial: SerialManager, config: SerialHubMCPConfig = {}) {
    this.serial = serial;
    this.dataBuffer = new DataBuffer();
    this.autoDataListener = config.autoDataListener ?? true;

    // 创建 MCP Server，声明支持资源订阅
    this.server = new McpServer({
      name: config.name ?? "SerialHub",
      version: config.version ?? "0.1.0",
    }, {
      capabilities: {
        resources: { subscribe: true, listChanged: true },
      },
    });

    // 注册工具
    this.registerTools();

    // 注册资源
    this.registerResources();

    // 设置数据监听器（可选）
    if (this.autoDataListener) {
      this.setupDataListener();
    }
  }

  /**
   * 注册所有 MCP 工具
   */
  private registerTools(): void {
    // serial_list - 无参数工具
    this.server.tool(
      serialListTool.name,
      serialListTool.description,
      async () => {
        const result: SerialListResult = await executeSerialList(this.serial);
        return {
          content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],
        };
      }
    );

    // serial_connect
    this.server.tool(
      serialConnectTool.name,
      serialConnectTool.description,
      {
        port: z.string().describe("串口名，如 COM9 或 /dev/ttyUSB0"),
        baudRate: z.number().int().min(1).optional().describe("波特率，默认 115200"),
      },
      async (input: SerialConnectInput) => {
        const result: SerialConnectResult = await executeSerialConnect(
          this.serial,
          input
        );
        return {
          content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],
        };
      }
    );

    // serial_disconnect - 无参数工具
    this.server.tool(
      serialDisconnectTool.name,
      serialDisconnectTool.description,
      async () => {
        const result: SerialDisconnectResult =
          await executeSerialDisconnect(this.serial);
        return {
          content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],
        };
      }
    );

    // serial_write
    this.server.tool(
      serialWriteTool.name,
      serialWriteTool.description,
      {
        data: z.string().describe("要发送的数据"),
        addNewline: z.boolean().optional().describe("是否添加换行符，默认 true"),
      },
      async (input: SerialWriteInput) => {
        const result: SerialWriteResult = await executeSerialWrite(
          this.serial,
          input
        );
        return {
          content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],
        };
      }
    );

    // serial_read
    this.server.tool(
      serialReadTool.name,
      serialReadTool.description,
      {
        timeout: z.number().int().min(0).optional().describe("超时时间(ms)，默认 1000"),
        maxSize: z.number().int().min(1).optional().describe("最大读取字节数，默认 4096"),
      },
      async (input: SerialReadInput) => {
        const result: SerialReadResult = await executeSerialRead(
          this.serial,
          this.dataBuffer,
          input
        );
        return {
          content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],}; 
      }
    );

    // serial_status - 无参数工具
    this.server.tool(
      serialStatusTool.name,
      serialStatusTool.description,
      async () => {
        const result: SerialStatusResult = await executeSerialStatus(this.serial);
        return {
          content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }],
        };
      }
    );
  }

  /**
   * 注册 MCP 资源
   */
  private registerResources(): void {
    // 注册串口输出资源 - 支持流式读取
    this.server.registerResource(
      "serial-output",
      "serial://output",
      {
        title: "串口输出流",
        description: "MCU 串口输出的实时数据流，支持订阅通知",
        mimeType: "text/plain",
      },
      async (uri: URL) => {
        // 读取当前缓冲区数据
        const data = this.dataBuffer.peek(65536);
        return {
          contents: [
            {
              uri: uri.toString(),
              mimeType: "text/plain",
              text: data.toString("utf-8"),
            },
          ],
        };
      }
    );

    // 注册串口状态资源
    this.server.registerResource(
      "serial-status",
      "serial://status",
      {
        title: "串口状态",
        description: "当前串口连接状态",
        mimeType: "application/json",
      },
      async (uri: URL) => {
        const status = {
          connected: this.serial.isConnected,
          port: this.serial.currentPort,
          config: this.serial.getConfig(),
        };
        return {
          contents: [
            {
              uri: uri.toString(),
              mimeType: "application/json",
              text: JSON.stringify(status, null, 2),
            },
          ],
        };
      }
    );
  }

  /**
   * 设置数据监听器
   * 将串口接收的数据存入缓冲区，并通知订阅者
   */
  private setupDataListener(): void {
    this.dataListener = (data: Buffer) => {
      this.dataBuffer.append(data);
      // 通知所有订阅者
      this.notifySubscribers(data);
    };
    this.serial.on("data", this.dataListener);
  }

  /**
   * 通知所有订阅者有新数据
   */
  private notifySubscribers(_data: Buffer): void {
    if (this.subscribers.size === 0) return;

    // 发送资源更新通知
    this.server.server.notification({
      method: "notifications/resources/updated",
      params: {
        uri: "serial://output",
      },
    });
  }

  /**
   * 添加订阅者
   */
  addSubscriber(sessionId: string): void {
    this.subscribers.add(sessionId);
  }

  /**
   * 移除订阅者
   */
  removeSubscriber(sessionId: string): void {
    this.subscribers.delete(sessionId);
  }

  /**
   * 获取 MCP Server 实例
   */
  getServer(): McpServer {
    return this.server;
  }

  /**
   * 获取串口管理器实例
   */
  getSerialManager(): SerialManager {
    return this.serial;
  }

  /**
   * 获取数据缓冲区实例
   */
  getDataBuffer(): DataBuffer {
    return this.dataBuffer;
  }

  /**
   * 清理资源
   */
  dispose(): void {
    if (this.dataListener) {
      this.serial.off("data", this.dataListener);
      this.dataListener = null;
    }
    this.dataBuffer.clear();
    this.subscribers.clear();
  }
}

// 导出工具和值
export {
  // 工具定义
  serialListTool,
  serialConnectTool,
  serialDisconnectTool,
  serialWriteTool,
  serialReadTool,
  serialStatusTool,
  // 执行函数
  executeSerialList,
  executeSerialConnect,
  executeSerialDisconnect,
  executeSerialWrite,
  executeSerialRead,
  executeSerialStatus,
  // 类（值）
  DataBuffer,
};

// 导出类型（使用 type 关键字）
export type {
  SerialListResult,
  SerialConnectInput,
  SerialConnectResult,
  SerialDisconnectResult,
  SerialWriteInput,
  SerialWriteResult,
  SerialReadInput,
  SerialReadResult,
  SerialStatusResult,
};