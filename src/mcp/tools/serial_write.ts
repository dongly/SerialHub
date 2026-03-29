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
  description:
    "向已连接的串口发送数据/命令。" +
    "使用场景：1) 向 MCU 发送 Shell 命令（如 help、version、 reboot）；2) 发送调试指令；3) 输入配置参数。" +
    "前提条件：必须先调用 serial_connect 成功连接串口；若未连接会返回错误提示。" +
    "典型工作流：连接后 → serial_write 发送命令 → 立即 serial_read 读取响应。" +
    "参数说明：data 为要发送的文本内容；addNewline 默认 true（自动追加换行符，适用于大多数 Shell 命令），设为 false 用于发送原始数据。" +
    "注意：写入操作立即返回成功与否，但设备响应需要单独调用 serial_read 获取。",
  inputSchema: serialWriteSchema,
};
