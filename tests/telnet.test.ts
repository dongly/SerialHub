/**
 * Telnet 服务器模块测试
 */

import { describe, test, expect, beforeEach, afterEach } from "bun:test";
import { TelnetServer, type TelnetClient } from "../src/telnet/TelnetServer";
import { createConnection } from "net";

describe("TelnetServer", () => {
  let server: TelnetServer;

  beforeEach(() => {
    server = new TelnetServer();
  });

  afterEach(async () => {
    if (server.isRunning) {
      await server.stop();
    }
  });

  describe("构造函数和基本属性", () => {
    test("应正确创建实例", () => {
      expect(server).toBeInstanceOf(TelnetServer);
      expect(server.isRunning).toBe(false);
      expect(server.clientCount).toBe(0);
      expect(server.port).toBe(0);
    });

    test("connectedClients 初始应为空数组", () => {
      expect(server.connectedClients).toEqual([]);
    });
  });

  describe("服务启动和停止", () => {
    test("应成功启动服务器", async () => {
      await server.start(2323);

      expect(server.isRunning).toBe(true);
      expect(server.port).toBe(2323);
    });

    test("启动时应发出 started 事件", async () => {
      let eventFired = false;
      server.on("started", () => {
        eventFired = true;
      });

      await server.start(2324);

      expect(eventFired).toBe(true);
    });

    test("重复启动应抛出错误", async () => {
      await server.start(2325);

      expect(server.start(2326)).rejects.toThrow("服务器已在运行");
    });

    test("应成功停止服务器", async () => {
      await server.start(2327);
      await server.stop();

      expect(server.isRunning).toBe(false);
      expect(server.port).toBe(0);
    });

    test("停止时应发出 stopped 事件", async () => {
      let eventFired = false;
      server.on("stopped", () => {
        eventFired = true;
      });

      await server.start(2328);
      await server.stop();

      expect(eventFired).toBe(true);
    });

    test("未运行时停止应正常返回", async () => {
      // 不应抛出错误
      await expect(server.stop()).resolves.toBeUndefined();
    });

    test("停止时应断开所有客户端", async () => {
      await server.start(2329);

      // 连接两个客户端
      const client1 = createConnection({ port: 2329, host: "127.0.0.1" });
      const client2 = createConnection({ port: 2329, host: "127.0.0.1" });

      // 等待连接建立
      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(server.clientCount).toBe(2);

      // 停止服务器
      await server.stop();

      expect(server.clientCount).toBe(0);

      // 清理
      client1.destroy();
      client2.destroy();
    });
  });

  describe("客户端连接管理", () => {
    beforeEach(async () => {
      await server.start(2330);
    });

    test("应接受新连接", async () => {
      let connectedClient: TelnetClient | null = null;
      server.on("connection", (client) => {
        connectedClient = client;
      });

      const client = createConnection({ port: 2330, host: "127.0.0.1" });

      // 等待连接建立
      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(server.clientCount).toBe(1);
      expect(connectedClient).not.toBeNull();
      expect(connectedClient!.id).toBeDefined();
      expect(connectedClient!.remoteAddress).toBeDefined();
      expect(connectedClient!.connectedAt).toBeInstanceOf(Date);

      client.destroy();
    });

    test("应支持多客户端连接", async () => {
      const client1 = createConnection({ port: 2330, host: "127.0.0.1" });
      const client2 = createConnection({ port: 2330, host: "127.0.0.1" });
      const client3 = createConnection({ port: 2330, host: "127.0.0.1" });

      // 等待连接建立
      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(server.clientCount).toBe(3);

      client1.destroy();
      client2.destroy();
      client3.destroy();
    });

    test("客户端断开时应发出 disconnect 事件", async () => {
      let disconnectedId: string | null = null;
      server.on("disconnect", (clientId) => {
        disconnectedId = clientId;
      });

      const client = createConnection({ port: 2330, host: "127.0.0.1" });

      // 等待连接建立
      await new Promise((resolve) => setTimeout(resolve, 100));
      const clientId = server.connectedClients[0].id;

      // 关闭客户端
      client.destroy();
      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(disconnectedId).toBe(clientId);
      expect(server.clientCount).toBe(0);
    });

    test("connectedClients 应返回正确的客户端列表", async () => {
      const client1 = createConnection({ port: 2330, host: "127.0.0.1" });
      const client2 = createConnection({ port: 2330, host: "127.0.0.1" });

      await new Promise((resolve) => setTimeout(resolve, 100));

      const clients = server.connectedClients;
      expect(clients.length).toBe(2);
      expect(clients[0]).toHaveProperty("id");
      expect(clients[1]).toHaveProperty("id");

      client1.destroy();
      client2.destroy();
    });

    test("getClient 应返回正确的客户端信息", async () => {
      const client = createConnection({ port: 2330, host: "127.0.0.1" });

      await new Promise((resolve) => setTimeout(resolve, 100));
      const clientId = server.connectedClients[0].id;

      const clientInfo = server.getClient(clientId);
      expect(clientInfo).toBeDefined();
      expect(clientInfo!.id).toBe(clientId);

      client.destroy();
    });

    test("getClient 不存在的客户端应返回 undefined", () => {
      const clientInfo = server.getClient("non-existent-id");
      expect(clientInfo).toBeUndefined();
    });
  });

  describe("数据传输", () => {
    beforeEach(async () => {
      await server.start(2331);
    });

    test("应接收客户端数据", async () => {
      let receivedData: Buffer | null = null;
      server.on("data", (data) => {
        receivedData = data;
      });

      const client = createConnection({ port: 2331, host: "127.0.0.1" });

      await new Promise((resolve) => setTimeout(resolve, 100));

      client.write("test data");
      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(receivedData).not.toBeNull();
      expect(receivedData!.toString()).toBe("test data");

      client.destroy();
    });

    test("数据事件应包含客户端信息", async () => {
      let receivedClient: TelnetClient | null = null;
      server.on("data", (_, client) => {
        receivedClient = client;
      });

      const client = createConnection({ port: 2331, host: "127.0.0.1" });

      await new Promise((resolve) => setTimeout(resolve, 100));
      const clientId = server.connectedClients[0].id;

      client.write("test");
      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(receivedClient).not.toBeNull();
      expect(receivedClient!.id).toBe(clientId);

      client.destroy();
    });

    test("应广播数据到所有客户端", async () => {
      const client1 = createConnection({ port: 2331, host: "127.0.0.1" });
      const client2 = createConnection({ port: 2331, host: "127.0.0.1" });

      await new Promise((resolve) => setTimeout(resolve, 100));

      const received1: Buffer[] = [];
      const received2: Buffer[] = [];

      client1.on("data", (data: Buffer) => received1.push(data));
      client2.on("data", (data: Buffer) => received2.push(data));

      // 跳过欢迎消息
      await new Promise((resolve) => setTimeout(resolve, 50));

      server.broadcast("broadcast message");

      await new Promise((resolve) => setTimeout(resolve, 100));

      const msg1 = received1.map((b) => b.toString()).join("");
      const msg2 = received2.map((b) => b.toString()).join("");

      expect(msg1).toContain("broadcast message");
      expect(msg2).toContain("broadcast message");

      client1.destroy();
      client2.destroy();
    });

    test("应广播 Buffer 数据", async () => {
      const client = createConnection({ port: 2331, host: "127.0.0.1" });

      await new Promise((resolve) => setTimeout(resolve, 100));

      const received: Buffer[] = [];
      client.on("data", (data: Buffer) => received.push(data));

      await new Promise((resolve) => setTimeout(resolve, 50));

      server.broadcast(Buffer.from("buffer data"));

      await new Promise((resolve) => setTimeout(resolve, 100));

      const msg = received.map((b) => b.toString()).join("");
      expect(msg).toContain("buffer data");

      client.destroy();
    });

    test("应发送数据到指定客户端", async () => {
      const client1 = createConnection({ port: 2331, host: "127.0.0.1" });
      const client2 = createConnection({ port: 2331, host: "127.0.0.1" });

      await new Promise((resolve) => setTimeout(resolve, 100));

      const received1: Buffer[] = [];
      const received2: Buffer[] = [];

      client1.on("data", (data: Buffer) => received1.push(data));
      client2.on("data", (data: Buffer) => received2.push(data));

      await new Promise((resolve) => setTimeout(resolve, 50));

      const clientId1 = server.connectedClients[0].id;
      const result = server.sendToClient(clientId1, "private message");

      expect(result).toBe(true);

      await new Promise((resolve) => setTimeout(resolve, 100));

      const msg1 = received1.map((b) => b.toString()).join("");
      const msg2 = received2.map((b) => b.toString()).join("");

      expect(msg1).toContain("private message");
      expect(msg2).not.toContain("private message");

      client1.destroy();
      client2.destroy();
    });

    test("发送到不存在的客户端应返回 false", () => {
      const result = server.sendToClient("non-existent-id", "test");
      expect(result).toBe(false);
    });
  });

  describe("客户端断开管理", () => {
    beforeEach(async () => {
      await server.start(2332);
    });

    test("disconnectClient 应断开指定客户端", async () => {
      const client = createConnection({ port: 2332, host: "127.0.0.1" });

      await new Promise((resolve) => setTimeout(resolve, 100));
      expect(server.clientCount).toBe(1);

      const clientId = server.connectedClients[0].id;
      server.disconnectClient(clientId);

      await new Promise((resolve) => setTimeout(resolve, 100));
      expect(server.clientCount).toBe(0);

      client.destroy();
    });

    test("disconnectClient 不存在的客户端应无操作", () => {
      // 不应抛出错误
      server.disconnectClient("non-existent-id");
    });
  });

  describe("边界情况", () => {
    test("未启动时广播应无操作", () => {
      // 不应抛出错误
      server.broadcast("test");
    });

    test("未启动时 sendToClient 应返回 false", () => {
      const result = server.sendToClient("any-id", "test");
      expect(result).toBe(false);
    });

    test("应处理客户端意外断开", async () => {
      await server.start(2333);

      const client = createConnection({ port: 2333, host: "127.0.0.1" });

      await new Promise((resolve) => setTimeout(resolve, 100));
      expect(server.clientCount).toBe(1);

      // 模拟意外断开
      client.destroy();

      await new Promise((resolve) => setTimeout(resolve, 100));
      expect(server.clientCount).toBe(0);
    });
  });

  describe("事件发射", () => {
    test("应继承 EventEmitter", () => {
      expect(typeof server.on).toBe("function");
      expect(typeof server.emit).toBe("function");
      expect(typeof server.removeListener).toBe("function");
    });
  });

  describe("TelnetClient 接口", () => {
    test("接口应定义正确的字段", async () => {
      await server.start(2334);

      let testClient: TelnetClient | null = null;
      server.on("connection", (client) => {
        testClient = client;
      });

      const client = createConnection({ port: 2334, host: "127.0.0.1" });
      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(testClient).not.toBeNull();
      expect(typeof testClient!.id).toBe("string");
      expect(testClient!.socket).toBeDefined();
      expect(typeof testClient!.remoteAddress).toBe("string");
      expect(testClient!.connectedAt).toBeInstanceOf(Date);

      client.destroy();
    });
  });
});
