/**
 * serial_list 工具
 * 列出系统中所有可用的串口
 */

import { SerialManager } from "../../serial/SerialManager.js";
import { SerialPortInfo } from "../../serial/SerialManager.js";

/**
 * 串口列表结果
 */
export interface SerialListResult {
  ports: SerialPortInfo[];
}

/**
 * 执行 serial_list 工具
 * @param _serialManager 串口管理器实例（未使用，保留用于一致性）
 * @returns 工具执行结果
 */
export async function executeSerialList(
  _serialManager?: SerialManager
): Promise<SerialListResult> {
  // 使用静态方法列出串口，不依赖 SerialManager 实例
  const ports = await SerialManager.listPorts();
  return { ports };
}

/**
 * 工具定义
 */
export const serialListTool = {
  name: "serial_list",
  description:
    "列出系统中所有可用的串口设备。" +
    "使用场景：1) 首次连接前查找可用串口；2) 确认目标设备连接的端口号；3) 排查串口识别问题。" +
    "典型工作流：先调用 serial_list 获取可用串口列表 → 根据厂商ID(如 USB 设备的 vendorId/productId)识别目标设备 → 使用 serial_connect 连接。" +
    "返回值包含串口路径、厂商信息、硬件ID等，帮助识别正确的串口。",
  inputSchema: {},
};
