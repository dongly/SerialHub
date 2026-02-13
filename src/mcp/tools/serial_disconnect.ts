/**
 * serial_disconnect 工具
 * 断开当前串口连接
 */

import { SerialManager } from "../../serial/SerialManager.js";

/**
 * 断开结果
 */
export interface SerialDisconnectResult {
  success: boolean;
  message?: string;
}

/**
 * 执行 serial_disconnect 工具
 * @param serialManager 串口管理器实例
 * @returns 工具执行结果
 */
export async function executeSerialDisconnect(
  serialManager: SerialManager
): Promise<SerialDisconnectResult> {
  if (!serialManager.isConnected) {
    return {
      success: true,
      message: "串口未连接",
    };
  }

  try {
    await serialManager.disconnect();
    return {
      success: true,
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
export const serialDisconnectTool = {
  name: "serial_disconnect",
  description: "断开当前串口连接",
  inputSchema: {}, // 无参数
};
