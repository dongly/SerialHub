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
  timeout: z.number().int().min(0).optional().describe("超时时间(ms)，默认 1000"),
  maxSize: z.number().int().min(1).optional().describe("最大读取字节数，默认 4096"),
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
    };
  }

  // 等待数据或超时
  const startTime = Date.now();
  while (dataBuffer.length === 0 && Date.now() - startTime < timeout) {
    await new Promise((resolve) => setTimeout(resolve, 50));
  }

  // 读取数据
  const data = dataBuffer.read(maxSize);
  const timestamp = Date.now();

  return {
    data: data.toString("utf-8"),
    encoding: "utf-8",
    timestamp,
    bytes: data.length,
  };
}

/**
 * 工具定义
 */
export const serialReadTool = {
  name: "serial_read",
  description: "读取串口缓冲区中的数据",
  inputSchema: serialReadSchema,
};
