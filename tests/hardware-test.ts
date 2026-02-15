/**
 * 硬件集成测试（需要 Node.js 运行）
 * 
 * 运行方式:
 *   npx tsx tests/hardware-test.ts
 * 
 * 注意: Bun 与 serialport 在 Windows USB CDC 设备上有兼容性问题
 *       必须使用 Node.js 运行此测试
 */

import { SerialManager, type SerialConfig } from "../src/serial/SerialManager";

const config: SerialConfig = {
  port: "COM9",
  baudRate: 115200,
  dataBits: 8,
  parity: "none",
  stopBits: 1,
};

async function runTest() {
  console.log("=== 硬件集成测试 ===");
  console.log("配置:", config);
  console.log();

  const manager = new SerialManager(config);

  let receivedData = Buffer.alloc(0);
  manager.on("data", (data: Buffer) => {
    receivedData = Buffer.concat([receivedData, data]);
    process.stdout.write(data.toString());
  });

  manager.on("connected", () => {
    console.log("[事件] 已连接");
  });

  manager.on("disconnected", () => {
    console.log("[事件] 已断开");
  });

  manager.on("error", (err: Error) => {
    console.error("[事件] 错误:", err.message);
  });

  try {
    // 连接串口
    console.log("连接串口...");
    const connectPromise = manager.connect();
    const connectTimeout = new Promise<never>((_, reject) =>
      setTimeout(() => reject(new Error("连接超时 (5秒)")), 5000)
    );
    await Promise.race([connectPromise, connectTimeout]);

    if (!manager.isConnected) {
      throw new Error("连接失败");
    }

    console.log("已连接，当前端口:", manager.currentPort);
    console.log();

    // 发送 help 命令
    console.log("发送: help");
    await manager.writeLine("help");

    // 等待响应
    console.log("等待响应...");
    await new Promise((resolve) => setTimeout(resolve, 3000));

    console.log();
    console.log("=== 测试结果 ===");
    console.log("收到数据:", receivedData.length, "字节");

    if (receivedData.length > 0) {
      console.log("内容预览:", receivedData.toString("utf-8").substring(0, 200));
      console.log();
      console.log("✅ 测试通过");
    } else {
      console.log("❌ 测试失败: 未收到响应");
      process.exit(1);
    }
  } catch (err) {
    console.error("❌ 测试失败:", err instanceof Error ? err.message : err);
    process.exit(1);
  } finally {
    if (manager.isConnected) {
      console.log();
      console.log("断开连接...");
      await manager.disconnect();
    }
  }
}

runTest();
