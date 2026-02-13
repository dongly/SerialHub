/**
 * TelnetServer - Telnet 服务器
 * 管理 TCP 连接、数据收发和事件通知
 */

import { EventEmitter } from "events";
import { createServer, type Socket, type Server as NetServer } from "net";
import { randomUUID } from "crypto";

/**
 * Telnet 客户端信息
 */
export interface TelnetClient {
  /** 客户端唯一标识 */
  id: string;
  /** TCP Socket */
  socket: Socket;
  /** 远程地址（IP:Port） */
  remoteAddress: string;
  /** 连接时间 */
  connectedAt: Date;
}

/**
 * TelnetServer 事件映射
 */
export interface TelnetServerEvents {
  /** 服务启动 */
  started: () => void;
  /** 服务停止 */
  stopped: () => void;
  /** 新客户端连接 */
  connection: (client: TelnetClient) => void;
  /** 收到客户端数据 */
  data: (data: Buffer, client: TelnetClient) => void;
  /** 客户端断开 */
  disconnect: (clientId: string) => void;
  /** 发生错误 */
  error: (error: Error) => void;
}

/**
 * Telnet 服务器
 * 负责管理 TCP 连接、数据收发和广播
 */
export class TelnetServer extends EventEmitter {
  private server: NetServer | null = null;
  private clients: Map<string, TelnetClient> = new Map();
  private _isRunning: boolean = false;
  private _port: number = 0;

  /**
   * 创建 Telnet 服务器实例
   */
  constructor() {
    super();
  }

  /**
   * 服务是否正在运行
   */
  get isRunning(): boolean {
    return this._isRunning;
  }

  /**
   * 当前监听端口
   */
  get port(): number {
    return this._port;
  }

  /**
   * 已连接的客户端数量
   */
  get clientCount(): number {
    return this.clients.size;
  }

  /**
   * 获取所有已连接的客户端列表
   */
  get connectedClients(): TelnetClient[] {
    return Array.from(this.clients.values());
  }

  /**
   * 启动 Telnet 服务器
   * @param port 监听端口
   */
  async start(port: number): Promise<void> {
    if (this._isRunning) {
      throw new Error("服务器已在运行");
    }

    return new Promise((resolve, reject) => {
      this.server = createServer((socket) => {
        const client = this.handleConnection(socket);

        socket.on("data", (data: Buffer) => {
          this.handleData(data, client);
        });

        socket.on("close", () => {
          this.handleDisconnect(client);
        });

        socket.on("error", (err: Error) => {
          this.handleError(err, client);
        });
      });

      this.server.on("error", (err: Error) => {
        if (!this._isRunning) {
          reject(new Error(`启动服务器失败: ${err.message}`));
        } else {
          this.emit("error", err);
        }
      });

      this.server.listen(port, () => {
        this._isRunning = true;
        this._port = port;
        this.emit("started");
        resolve();
      });
    });
  }

  /**
   * 停止 Telnet 服务器
   */
  async stop(): Promise<void> {
    if (!this._isRunning || !this.server) {
      return;
    }

    // 断开所有客户端
    for (const client of this.clients.values()) {
      try {
        client.socket.destroy();
      } catch {
        // 忽略关闭错误
      }
    }
    this.clients.clear();

    return new Promise((resolve, reject) => {
      if (!this.server) {
        resolve();
        return;
      }

      this.server.close((err) => {
        if (err) {
          reject(new Error(`关闭服务器失败: ${err.message}`));
          return;
        }

        this._isRunning = false;
        this._port = 0;
        this.server = null;
        this.emit("stopped");
        resolve();
      });
    });
  }

  /**
   * 广播数据到所有客户端
   * @param data 要发送的数据
   */
  broadcast(data: Buffer | string): void {
    const buffer = Buffer.isBuffer(data) ? data : Buffer.from(data, "utf-8");

    for (const client of this.clients.values()) {
      try {
        client.socket.write(buffer);
      } catch {
        // 忽略写入错误，客户端可能已断开
      }
    }
  }

  /**
   * 发送数据到指定客户端
   * @param clientId 客户端 ID
   * @param data 要发送的数据
   * @returns 是否发送成功
   */
  sendToClient(clientId: string, data: Buffer | string): boolean {
    const client = this.clients.get(clientId);
    if (!client) {
      return false;
    }

    const buffer = Buffer.isBuffer(data) ? data : Buffer.from(data, "utf-8");

    try {
      client.socket.write(buffer);
      return true;
    } catch {
      // 客户端可能已断开
      return false;
    }
  }

  /**
   * 断开指定客户端
   * @param clientId 客户端 ID
   */
  disconnectClient(clientId: string): void {
    const client = this.clients.get(clientId);
    if (!client) {
      return;
    }

    try {
      client.socket.destroy();
    } catch {
      // 忽略关闭错误
    }
  }

  /**
   * 获取客户端信息
   * @param clientId 客户端 ID
   * @returns 客户端信息或 undefined
   */
  getClient(clientId: string): TelnetClient | undefined {
    return this.clients.get(clientId);
  }

  /**
   * 处理新连接
   */
  private handleConnection(socket: Socket): TelnetClient {
    const client: TelnetClient = {
      id: randomUUID(),
      socket,
      remoteAddress: `${socket.remoteAddress ?? "unknown"}:${socket.remotePort ?? 0}`,
      connectedAt: new Date(),
    };

    this.clients.set(client.id, client);
    this.emit("connection", client);

    // 发送欢迎消息
    try {
      socket.write("Connected to SerialHub\r\n");
    } catch {
      // 忽略写入错误
    }

    return client;
  }

  /**
   * 处理客户端数据
   */
  private handleData(data: Buffer, client: TelnetClient): void {
    this.emit("data", data, client);
  }

  /**
   * 处理客户端断开
   */
  private handleDisconnect(client: TelnetClient): void {
    const wasConnected = this.clients.has(client.id);
    this.clients.delete(client.id);

    if (wasConnected) {
      this.emit("disconnect", client.id);
    }
  }

  /**
   * 处理客户端错误
   */
  private handleError(err: Error, client: TelnetClient): void {
    // 错误通常会导致断开，但我们也发出事件
    this.emit("error", new Error(`客户端 ${client.id} 错误: ${err.message}`));
  }
}
