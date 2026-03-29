/**
 * serial_read 工具
 * 读取串口缓冲区中的数据
 */

import { z } from "zod";
import { SerialManager } from "../../serial/SerialManager.js";

/**
 * 输入参数 Schema
 */
export const serialReadSchema = {
  timeout: z
    .number()
    .int()
    .min(0)
    .optional()
    .describe("超时时间(ms)，默认 1000，0 表示无限等待"),
  maxSize: z
    .number()
    .int()
    .min(1)
    .optional()
    .describe("最大读取字节数，默认 4096"),
};

/**
 * 输入参数类型
 */
export type SerialReadInput = {
  timeout?: number;
  maxSize?: number;
};

/**
 * 读取结果
 */
export interface SerialReadResult {
  data: string;
  encoding: string;
  timestamp: number;
  bytes: number;
  timedOut: boolean;
}

/**
 * 数据缓冲区类
 * 用于存储从串口接收的数据
 */
export class DataBuffer {
  private buffer: Buffer = Buffer.alloc(0);
  private maxSize: number;

  constructor(maxSize: number = 65536) {
    this.maxSize = maxSize;
  }

  /**
   * 追加数据到缓冲区
   * @param data 接收到的数据
   */
  append(data: Buffer): void {
    // 如果新数据会超过最大大小，丢弃旧数据
    if (this.buffer.length + data.length > this.maxSize) {
      const overflow = this.buffer.length + data.length - this.maxSize;
      this.buffer = this.buffer.slice(overflow);
    }
    this.buffer = Buffer.concat([this.buffer, data]);
  }

  /**
   * 读取并清空缓冲区
   * @param maxSize 最大读取字节数
   * @returns 读取的数据
   */
  read(maxSize?: number): Buffer {
    const size = Math.min(maxSize ?? this.buffer.length, this.buffer.length);
    const data = this.buffer.slice(0, size);
    this.buffer = this.buffer.slice(size);
    return data;
  }

  /**
   * 查看缓冲区数据（不清空）
   * @param maxSize 最大查看字节数
   * @returns 查看的数据
   */
  peek(maxSize?: number): Buffer {
    const size = Math.min(maxSize ?? this.buffer.length, this.buffer.length);
    return this.buffer.slice(0, size);
  }

  /**
   * 清空缓冲区
   */
  clear(): void {
    this.buffer = Buffer.alloc(0);
  }

  /**
   * 获取缓冲区当前大小
   */
  get length(): number {
    return this.buffer.length;
  }
}

/**
 * 执行 serial_read 工具
 * 阻塞等待数据到达，timeout=0 表示无限等待
 * @param serialManager 串口管理器实例
 * @param dataBuffer 数据缓冲区
 * @param input 输入参数
 * @returns 工具执行结果
 */
export async function executeSerialRead(
  serialManager: SerialManager,
  dataBuffer: DataBuffer,
  input: SerialReadInput = {}
): Promise<SerialReadResult> {
  const { timeout = 1000, maxSize = 4096 } = input;

  // 如果没有连接，返回空数据
  if (!serialManager.isConnected) {
    return {
      data: "",
      encoding: "utf-8",
      timestamp: Date.now(),
      bytes: 0,
      timedOut: false,
    };
  }

  // 等待数据
  const startTime = Date.now();
  const checkInterval = 50;
  const hasTimeout = timeout > 0;

  while (dataBuffer.length === 0) {
    // 检查超时
    if (hasTimeout && Date.now() - startTime >= timeout) {
      return {
        data: "",
        encoding: "utf-8",
        timestamp: Date.now(),
        bytes: 0,
        timedOut: true,
      };
    }
    await new Promise((resolve) => setTimeout(resolve, checkInterval));
  }

  // 读取数据
  const data = dataBuffer.read(maxSize);
  const timestamp = Date.now();

  return {
    data: data.toString("utf-8"),
    encoding: "utf-8",
    timestamp,
    bytes: data.length,
    timedOut: false,
  };
}

/**
 * 工具定义
 */
export const serialReadTool = {
  name: "serial_read",
  description:
    "阻塞式读取串口返回的数据。" +
    "使用场景：1) 发送命令后获取设备响应；2) 监视设备输出日志；3) 等待特定事件/数据到达。" +
    "前提条件：必须先 serial_connect 连接串口。" +
    "典型工作流：serial_write 发送命令 → 立即 serial_read 获取响应 → 解析输出内容。" +
    "参数策略：timeout=0 表示无限等待（适用于不确定响应时间的场景）；timeout=1000~5000 适合常规命令响应；timeout=100 适合快速检查是否有数据。" +
    "返回值说明：data 为读取的文本内容；timedOut=true 表示超时未收到数据（不是错误，只是无数据）；bytes 为实际读取字节数。" +
    "重要提示：读取操作会清空缓冲区已读数据，多次调用可分段读取长输出；若设备持续输出，可循环调用 serial_read 获取完整内容。",
  inputSchema: serialReadSchema,
};
