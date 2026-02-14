/**
 * SerialManager - 串口管理器
 * 管理串口连接、数据收发和事件通知
 */

import { EventEmitter } from "events";
import { SerialPort } from "serialport";

/**
 * 串口配置接口
 */
export interface SerialConfig {
  /** 串口名，如 "COM9" 或 "/dev/ttyUSB0" */
  port: string;
  /** 波特率，默认 115200 */
  baudRate: number;
  /** 数据位 */
  dataBits: 5 | 6 | 7 | 8;
  /** 校验位 */
  parity: "none" | "even" | "odd";
  /** 停止位 */
  stopBits: 1 | 2;
}

/**
 * 串口信息接口
 */
export interface SerialPortInfo {
  path: string;
  manufacturer?: string;
  serialNumber?: string;
  pnpId?: string;
  vendorId?: string;
  productId?: string;
}

/**
 * SerialManager 事件映射
 */
export interface SerialManagerEvents {
  /** 收到串口数据 */
  data: (data: Buffer) => void;
  /** 连接成功 */
  connected: () => void;
  /** 断开连接 */
  disconnected: () => void;
  /** 发生错误 */
  error: (error: Error) => void;
}

/**
 * 串口管理器
 * 负责串口的连接、数据收发和状态管理
 */
export class SerialManager extends EventEmitter {
  private serialPort: SerialPort | null = null;
  private config: SerialConfig;
  private _isConnected: boolean = false;
  private _currentPort: string | null = null;

  /**
   * 创建串口管理器实例
   * @param config 串口配置
   */
  constructor(config: SerialConfig) {
    super();
    this.config = { ...config };
  }

  /**
   * 列出所有可用串口
   * @returns 串口信息列表
   */
  static async listPorts(): Promise<SerialPortInfo[]> {
    const ports = await SerialPort.list();
    return ports.map((port) => ({
      path: port.path,
      manufacturer: port.manufacturer,
      serialNumber: port.serialNumber,
      pnpId: port.pnpId,
      vendorId: port.vendorId,
      productId: port.productId,
    }));
  }

  /**
   * 当前是否已连接
   */
  get isConnected(): boolean {
    return this._isConnected;
  }

  /**
   * 当前连接的串口名
   */
  get currentPort(): string | null {
    return this._currentPort;
  }

  /**
   * 连接串口
   * @param portName 可选的串口名，不指定则使用配置中的端口
   */
  async connect(portName?: string): Promise<void> {
    const targetPort = portName ?? this.config.port;

    if (!targetPort) {
      throw new Error("未指定串口名");
    }

    // 如果已连接，先断开
    if (this._isConnected) {
      await this.disconnect();
    }

    return new Promise((resolve, reject) => {
      try {
        this.serialPort = new SerialPort({
          path: targetPort,
          baudRate: this.config.baudRate,
          dataBits: this.config.dataBits,
          parity: this.config.parity,
          stopBits: this.config.stopBits,
          autoOpen: false,
        });

        // 设置数据监听器
        this.serialPort.on("data", (data: Buffer) => {
          console.log("[SerialManager] 收到数据:", data.length, "bytes:", data.toString("utf-8"));
          this.emit("data", data);
        });

        // 设置错误监听器
        this.serialPort.on("error", (error: Error) => {
          this.emit("error", error);
        });

        // 设置关闭监听器
        this.serialPort.on("close", () => {
          this._isConnected = false;
          this._currentPort = null;
          this.serialPort = null;
          this.emit("disconnected");
        });

        // 打开串口
        this.serialPort.open((error) => {
          if (error) {
            this.serialPort = null;
            reject(new Error(`打开串口失败: ${error.message}`));
            return;
          }

          this._isConnected = true;
          this._currentPort = targetPort;
          this.emit("connected");
          resolve();
        });
      } catch (error) {
        reject(
          new Error(
            `创建串口失败: ${error instanceof Error ? error.message : String(error)}`
          )
        );
      }
    });
  }

  /**
   * 断开串口连接
   */
  async disconnect(): Promise<void> {
    if (!this.serialPort || !this._isConnected) {
      return;
    }

    return new Promise((resolve, reject) => {
      if (!this.serialPort) {
        resolve();
        return;
      }

      this.serialPort.close((error) => {
        if (error) {
          reject(new Error(`关闭串口失败: ${error.message}`));
          return;
        }

        this._isConnected = false;
        this._currentPort = null;
        this.serialPort = null;
        this.emit("disconnected");
        resolve();
      });
    });
  }

  /**
   * 发送数据到串口
   * @param data 要发送的数据（字符串或 Buffer）
   * @returns 写入的字节数
   */
  async write(data: Buffer | string): Promise<number> {
    if (!this.serialPort || !this._isConnected) {
      throw new Error("串口未连接");
    }

    const buffer = typeof data === "string" ? Buffer.from(data, "utf-8") : data;

    return new Promise((resolve, reject) => {
      if (!this.serialPort) {
        reject(new Error("串口未连接"));
        return;
      }

      this.serialPort.write(buffer, (error) => {
        if (error) {
          reject(new Error(`写入串口失败: ${error.message}`));
          return;
        }

        this.serialPort!.drain((drainError) => {
          if (drainError) {
            reject(new Error(`刷新串口缓冲区失败: ${drainError.message}`));
            return;
          }
          resolve(buffer.length);
        });
      });
    });
  }

  /**
   * 发送字符串并自动添加换行符
   * @param text 要发送的文本
   * @param lineEnding 换行符，默认 "\r\n"
   * @returns 写入的字节数
   */
  async writeLine(text: string, lineEnding: string = "\r\n"): Promise<number> {
    return this.write(text + lineEnding);
  }

  /**
   * 更新串口配置
   * @param config 新的串口配置
   */
  updateConfig(config: Partial<SerialConfig>): void {
    this.config = { ...this.config, ...config };
  }

  /**
   * 获取当前配置
   */
  getConfig(): SerialConfig {
    return { ...this.config };
  }
}

// 导出类型已内联定义
