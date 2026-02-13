/**
 * MCP stdio 传输层封装
 * 提供标准输入/输出的 MCP 传输功能
 */

import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";

/**
 * 创建并连接 stdio 传输
 * @param server MCP Server 实例
 * @returns 已连接的传输实例
 */
export async function createStdioTransport(
  server: McpServer
): Promise<StdioServerTransport> {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  return transport;
}

/**
 * 优雅关闭处理
 * 处理 SIGINT/SIGTERM 信号，执行清理工作
 * @param cleanup 清理函数
 */
export function setupGracefulShutdown(cleanup: () => Promise<void> | void): void {
  const handler = async (signal: string) => {
    console.error(`[SerialHub] 收到 ${signal} 信号，正在关闭...`);
    try {
      await cleanup();
      console.error("[SerialHub] 已关闭");
      process.exit(0);
    } catch (error) {
      console.error("[SerialHub] 关闭时出错:", error);
      process.exit(1);
    }
  };

  process.on("SIGINT", () => handler("SIGINT"));
  process.on("SIGTERM", () => handler("SIGTERM"));
}

/**
 * 保持进程运行
 * 监听 stdin 关闭事件
 * @param onStdinClose stdin 关闭时的回调
 */
export function keepProcessRunning(
  onStdinClose?: () => Promise<void> | void
): void {
  // 当 stdin 关闭时（父进程断开），退出
  process.stdin.on("end", async () => {
    console.error("[SerialHub] stdin 已关闭，正在退出...");
    if (onStdinClose) {
      await onStdinClose();
    }
    process.exit(0);
  });

  // 保持 stdin 打开
  process.stdin.resume();
}

export { StdioServerTransport };
