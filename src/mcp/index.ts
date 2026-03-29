/**
 * SerialHub MCP 服务入口
 */

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { SerialManager } from "../serial/SerialManager.js";

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

export interface SerialHubMCPConfig {
  name?: string;
  version?: string;
  autoDataListener?: boolean;
}

export interface ToolDef {
  name: string;
  description: string;
  inputSchema: Record<string, z.ZodTypeAny>;
}

export class SerialHubMCP {
  private server: McpServer;
  private serial: SerialManager;
  private dataBuffer: DataBuffer;
  private dataListener: ((data: Buffer) => void) | null = null;
  private autoDataListener: boolean;

  private tools: ToolDef[] = [];

  constructor(serial: SerialManager, config: SerialHubMCPConfig = {}) {
    this.serial = serial;
    this.dataBuffer = new DataBuffer();
    this.autoDataListener = config.autoDataListener ?? true;

    this.server = new McpServer(
      {
        name: config.name ?? "SerialHub",
        version: config.version ?? "0.1.0",
      },
      { capabilities: {} }
    );

    this.registerTools();

    if (this.autoDataListener) {
      this.setupDataListener();
    }
  }

  private registerTools(): void {
    const toolDefs: [ToolDef, (input: Record<string, unknown>) => Promise<unknown>][] = [
      [
        { name: serialListTool.name, description: serialListTool.description, inputSchema: {} },
        async () => executeSerialList(this.serial),
      ],
      [
        { name: serialConnectTool.name, description: serialConnectTool.description, inputSchema: { port: z.string().describe("串口名，如 COM9 或 /dev/ttyUSB0"), baudRate: z.number().int().min(1).optional().describe("波特率，默认 115200") } },
        async (input) => executeSerialConnect(this.serial, input as SerialConnectInput),
      ],
      [
        { name: serialDisconnectTool.name, description: serialDisconnectTool.description, inputSchema: {} },
        async () => executeSerialDisconnect(this.serial),
      ],
      [
        { name: serialWriteTool.name, description: serialWriteTool.description, inputSchema: { data: z.string().describe("要发送的数据"), addNewline: z.boolean().optional().describe("是否添加换行符，默认 true") } },
        async (input) => executeSerialWrite(this.serial, input as SerialWriteInput),
      ],
      [
        { name: serialReadTool.name, description: serialReadTool.description, inputSchema: { timeout: z.number().int().min(0).optional().describe("超时时间(ms)，默认 1000，0 表示无限等待"), maxSize: z.number().int().min(1).optional().describe("最大读取字节数，默认 4096") } },
        async (input) => executeSerialRead(this.serial, this.dataBuffer, input as SerialReadInput),
      ],
      [
        { name: serialStatusTool.name, description: serialStatusTool.description, inputSchema: {} },
        async () => executeSerialStatus(this.serial),
      ],
    ];

    this.tools = toolDefs.map(([def]) => def);

    for (const [def, handler] of toolDefs) {
      if (Object.keys(def.inputSchema).length === 0) {
        this.server.tool(def.name, def.description, async () => {
          const result = await handler({});
          return { content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }] };
        });
      } else {
        this.server.tool(def.name, def.description, def.inputSchema, async (input: unknown) => {
          const result = await handler(input as Record<string, unknown>);
          return { content: [{ type: "text" as const, text: JSON.stringify(result, null, 2) }] };
        });
      }
    }
  }

  private setupDataListener(): void {
    this.dataListener = (data: Buffer) => {
      console.error(`[SerialHubMCP] Data received: ${data.length} bytes`);
      this.dataBuffer.append(data);
    };
    this.serial.on("data", this.dataListener);
    console.error("[SerialHubMCP] Data listener registered");
  }

  getToolsList(): ToolDef[] {
    return this.tools;
  }

  async callTool(name: string, args: Record<string, unknown> = {}): Promise<unknown> {
    switch (name) {
      case "serial_list":
        return executeSerialList(this.serial);
      case "serial_connect":
        return executeSerialConnect(this.serial, args as SerialConnectInput);
      case "serial_disconnect":
        return executeSerialDisconnect(this.serial);
      case "serial_write":
        return executeSerialWrite(this.serial, args as SerialWriteInput);
      case "serial_read":
        return executeSerialRead(this.serial, this.dataBuffer, args as SerialReadInput);
      case "serial_status":
        return executeSerialStatus(this.serial);
      default:
        throw new Error(`Unknown tool: ${name}`);
    }
  }

  getServer(): McpServer {
    return this.server;
  }

  getSerialManager(): SerialManager {
    return this.serial;
  }

  getDataBuffer(): DataBuffer {
    return this.dataBuffer;
  }

  dispose(): void {
    if (this.dataListener) {
      this.serial.off("data", this.dataListener);
      this.dataListener = null;
    }
    this.dataBuffer.clear();
  }
}

export {
  serialListTool,
  serialConnectTool,
  serialDisconnectTool,
  serialWriteTool,
  serialReadTool,
  serialStatusTool,
  executeSerialList,
  executeSerialConnect,
  executeSerialDisconnect,
  executeSerialWrite,
  executeSerialRead,
  executeSerialStatus,
  DataBuffer,
};

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
