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
  description:
    "连接到指定串口设备。" +
    "使用场景：1) 开始与 MCU/嵌入式设备通信前必须先连接；2) 切换到不同串口设备；3) 重新建立断开的连接。" +
    "前提条件：先用 serial_list 确认目标串口路径。" +
    "典型工作流：serial_list → serial_connect → serial_write 发送命令 → serial_read 读取响应。" +
    "重要提示：连接后设备会保持连接状态，后续所有 serial_write/serial_read 操作都针对此连接；若要切换设备，先用 serial_disconnect 断开当前连接。" +
    "参数说明：port 为必填项（如 COM3、/dev/ttyUSB0）；baudRate 默认 115200，常见值还有 9600、57600、230400，需与目标设备配置一致。",
  inputSchema: serialConnectSchema,
};
