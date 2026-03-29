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
  description:
    "查询当前串口连接状态。" +
    "使用场景：1) 开始会话时检查是否已连接；2) 操作失败时确认连接是否意外断开；3) 查看当前串口配置参数。" +
    "返回值说明：connected=true 表示已连接并可通信；connected=false 表示未连接，需要先调用 serial_connect。" +
    "典型用法：在执行 serial_write/serial_read 前先检查状态，避免因未连接而失败；或在长时间操作后检查连接是否仍然有效。" +
    "注意：此工具不改变任何状态，仅查询当前状态信息。",
  inputSchema: {},
};
