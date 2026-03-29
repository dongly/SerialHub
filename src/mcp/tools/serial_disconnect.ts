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
  description:
    "断开当前串口连接。" +
    "使用场景：1) 完成调试后释放串口资源；2) 切换到不同串口设备前先断开当前连接；3) 设备出现异常需要重连时先断开。" +
    "重要提示：断开后所有 serial_write/serial_read 操作都会失败，需要重新 serial_connect。" +
    "典型工作流：serial_disconnect → serial_list → serial_connect（切换设备）。" +
    "安全特性：即使串口未连接，调用此工具也会返回成功，不会报错。",
  inputSchema: {},
};
