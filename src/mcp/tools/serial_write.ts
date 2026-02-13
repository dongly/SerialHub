/**
 * serial_write 工具
 * 向串口发送数据
 */

import { z } from "zod";
import { SerialManager } from "../../serial/SerialManager.js";

/**
 * 输入参数 Schema
 */
export const serialWriteSchema = {
  data: z.string().describe("要发送的数据"),
  addNewline: z.boolean().optional().describe("是否添加换行符，默认 true"),
};

/**
 * 输入参数类型
 */
export type SerialWriteInput = {
  data: string;
  addNewline?: boolean;
};

/**
 * 写入结果
 */
export interface SerialWriteResult {
  success: boolean;
  bytesWritten?: number;
  message?: string;
}

/**
 * 执行 serial_write 工具
 * @param serialManager 串口管理器实例
 * @param input 输入参数
 * @returns 工具执行结果
 */
export async function executeSerialWrite(
  serialManager: SerialManager,
  input: SerialWriteInput
): Promise<SerialWriteResult> {
  const { data, addNewline = true } = input;

  if (!serialManager.isConnected) {
    return {
      success: false,
      message: "串口未连接，请先使用 serial_connect 连接串口",
    };
  }

  try {
    let bytesWritten: number;
    if (addNewline) {
      bytesWritten = await serialManager.writeLine(data);
    } else {
      bytesWritten = await serialManager.write(data);
    }

    return {
      success: true,
      bytesWritten,
    };
  } catch (error) {
    return {
      success: false,
      message: error instanceof Error ? error.message : String(error),
    };
  }
}

/**
 * 工具定义
 */
export const serialWriteTool = {
  name: "serial_write",
  description: "向串口发送数据",
  inputSchema: serialWriteSchema,
};
