/**
 * serial_connect 工具
 * 连接到指定串口
 */

import { z } from "zod";
import { SerialManager } from "../../serial/SerialManager.js";

/**
 * 输入参数 Schema
 */
export const serialConnectSchema = {
  port: z.string().describe("串口名，如 COM9 或 /dev/ttyUSB0"),
  baudRate: z.number().int().min(1).optional().describe("波特率，默认 115200"),
};

/**
 * 输入参数类型
 */
export type SerialConnectInput = {
  port: string;
  baudRate?: number;
};

/**
 * 连接结果
 */
export interface SerialConnectResult {
  success: boolean;
  port: string;
  baudRate: number;
  message?: string;
}

/**
 * 执行 serial_connect 工具
 * @param serialManager 串口管理器实例
 * @param input 输入参数
 * @returns 工具执行结果
 */
export async function executeSerialConnect(
  serialManager: SerialManager,
  input: SerialConnectInput
): Promise<SerialConnectResult> {
  const { port, baudRate } = input;
  const targetBaudRate = baudRate ?? 115200;

  // 更新配置
  serialManager.updateConfig({
    port,
    baudRate: targetBaudRate,
  });

  try {
    await serialManager.connect(port);
    return {
      success: true,
      port,
      baudRate: targetBaudRate,
    };
  } catch (error) {
    return {
      success: false,
      port,
      baudRate: targetBaudRate,
      message: error instanceof Error ? error.message : String(error),
    };
  }
}

/**
 * 工具定义
 */
export const serialConnectTool = {
  name: "serial_connect",
  description: "连接到指定串口",
  inputSchema: serialConnectSchema,
};
