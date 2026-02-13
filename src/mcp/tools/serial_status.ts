/**
 * serial_status 工具
 * 获取串口连接状态
 */

import { SerialManager } from "../../serial/SerialManager.js";

/**
 * 状态结果
 */
export interface SerialStatusResult {
  connected: boolean;
  port?: string;
  baudRate?: number;
  config?: {
    dataBits: number;
    parity: string;
    stopBits: number;
  };
}

/**
 * 执行 serial_status 工具
 * @param serialManager 串口管理器实例
 * @returns 工具执行结果
 */
export async function executeSerialStatus(
  serialManager: SerialManager
): Promise<SerialStatusResult> {
  const isConnected = serialManager.isConnected;
  const currentPort = serialManager.currentPort;
  const config = serialManager.getConfig();

  if (isConnected && currentPort) {
    return {
      connected: true,
      port: currentPort,
      baudRate: config.baudRate,
      config: {
        dataBits: config.dataBits,
        parity: config.parity,
        stopBits: config.stopBits,
      },
    };
  }

  return {
    connected: false,
    port: config.port || undefined,
    baudRate: config.baudRate,
    config: {
      dataBits: config.dataBits,
      parity: config.parity,
      stopBits: config.stopBits,
    },
  };
}

/**
 * 工具定义
 */
export const serialStatusTool = {
  name: "serial_status",
  description: "获取串口连接状态",
  inputSchema: {}, // 无参数
};
