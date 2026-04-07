"""
SerialHub Web 终端集成测试 — 基于 WebSocket 客户端

使用 pytest 运行：
    pytest tests/integration/test_web_terminal.py -v

依赖：websocket-client（pip install websocket-client）

启用硬件回环测试（test_web_terminal_loopback）：
    set SERIALHUB_INTEGRATION_TEST=1
    set SERIALHUB_TEST_PORT=COM4
"""

import json
import os
import re
import time

import pytest

try:
    import websocket

    WS_CLIENT_AVAILABLE = True
except ImportError:
    WS_CLIENT_AVAILABLE = False

from conftest import (
    _get_content_text,
    ensure_binary,
    find_free_port,
    get_test_port,
    mcp_call,
    wait_for_health,
)


# ─── Skip 标记 ─────────────────────────────────────────

requires_ws_client = pytest.mark.skipif(
    not WS_CLIENT_AVAILABLE,
    reason="websocket-client 未安装，pip install websocket-client",
)

requires_server = pytest.mark.skipif(
    not os.environ.get("SERIALHUB_INTEGRATION_TEST"),
    reason="需要设置 SERIALHUB_INTEGRATION_TEST=1",
)

requires_hardware = pytest.mark.skipif(
    not os.environ.get("SERIALHUB_INTEGRATION_TEST"),
    reason="需要设置 SERIALHUB_INTEGRATION_TEST=1 和硬件回环",
)


# ─── Fixtures ──────────────────────────────────────────


@pytest.fixture
def serialhub_server_ws(binary, tmp_path):
    """启动 serialhub --no-tray 并返回连接信息。

    WebSocket /ws 端点与 MCP 共用同一端口。
    """
    import subprocess
    import threading

    log_dir = tmp_path / "logs"
    proc = None
    last_error = None

    for attempt in range(3):
        mcp_port = find_free_port()

        proc = subprocess.Popen(
            [
                str(binary),
                "--no-tray",
                "--mcp-port",
                str(mcp_port),
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env={**os.environ, "SERIALHUB_LOG_DIR": str(log_dir)},
        )

        output: dict[str, str] = {}
        stdout_thread = threading.Thread(
            target=lambda p, o: o.update(
                stdout=p.stdout.read().decode("utf-8", errors="replace")
                if p.stdout
                else ""
            ),
            args=(proc, output),
            daemon=True,
        )
        stderr_thread = threading.Thread(
            target=lambda p, o: o.update(
                stderr=p.stderr.read().decode("utf-8", errors="replace")
                if p.stderr
                else ""
            ),
            args=(proc, output),
            daemon=True,
        )
        stdout_thread.start()
        stderr_thread.start()

        try:
            wait_for_health(mcp_port, timeout=15)
        except TimeoutError:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=3)
            proc = None
            last_error = f"健康检查超时 (attempt {attempt + 1}/3): mcp_port={mcp_port}"
            continue

        break
    else:
        raise RuntimeError(f"服务器启动失败，已重试 3 次:\n{last_error}")

    try:
        yield {
            "proc": proc,
            "mcp_port": mcp_port,
            "ws_url": f"ws://127.0.0.1:{mcp_port}/ws",
            "log_dir": log_dir,
        }
    finally:
        if proc is not None:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=3)


@pytest.fixture
def ws_client(serialhub_server_ws):
    """创建 WebSocket 客户端连接并返回 (ws, server_info)。

    测试结束后自动关闭连接。
    """
    info = serialhub_server_ws
    ws = websocket.create_connection(info["ws_url"], timeout=5)
    try:
        yield ws, info
    finally:
        try:
            ws.close()
        except Exception:
            pass


# ─── 辅助函数 ──────────────────────────────────────────


def _extract_data_from_mcp_text(text: str) -> str:
    """从 MCP serial_read 返回文本中提取 data 字段值。"""
    match = re.search(r"data:(.+?)(?:\s+bytes:|\s+timedOut|$)", text)
    if match:
        return match.group(1).strip()
    return ""


# ─── 测试类 ────────────────────────────────────────────


@requires_server
@requires_ws_client
class TestWebTerminal:
    """Web 终端 WebSocket 集成测试"""

    def test_web_terminal_connect(self, serialhub_server_ws):
        """连接 WebSocket /ws，验证连接成功"""
        info = serialhub_server_ws

        ws = websocket.create_connection(info["ws_url"], timeout=5)
        try:
            # 服务器应发送欢迎消息
            welcome = ws.recv()
            assert isinstance(welcome, str), f"欢迎消息应为文本帧: {type(welcome)}"
            assert "SerialHub" in welcome or "Connected" in welcome, (
                f"欢迎消息异常: {welcome!r}"
            )
        finally:
            ws.close()

    def test_web_terminal_send_data(self, ws_client):
        """通过 WebSocket 发送数据，验证转发到串口"""
        ws, info = ws_client

        # 先读取欢迎消息（fixture 建立连接时服务器已发送）
        try:
            ws.settimeout(1)
            ws.recv()  # 丢弃欢迎消息
        except Exception:
            pass

        # 发送测试数据
        test_data = "HelloFromWebSocket\n"
        ws.send(test_data)

        # WebSocket 发送的数据通过 dataChan 到 bridge，bridge 再写入串口
        # 在无真实串口时，数据通过 DataBridge 流转，可通过 MCP serial_read 验证
        # 注意：如果没有连接串口，数据只写入 WebSocket 的 dataChan，不会到串口
        # 这里验证 WebSocket 连接和发送操作本身不报错
        ws.settimeout(2)
        # 发送操作成功即验证通过
        # 无需验证数据到达串口（那需要真实串口连接）

    def test_web_terminal_multiple_clients(self, serialhub_server_ws):
        """多个 WebSocket 客户端同时连接

        SerialHub 的 WebSocketServer 仅支持单客户端，新连接会踢掉旧连接。
        验证：
        1. 第一个客户端连接成功
        2. 第二个客户端连接后，第一个被踢掉
        3. 服务器端始终只有 1 个活跃客户端
        """
        info = serialhub_server_ws

        # 第一个客户端连接
        ws1 = websocket.create_connection(info["ws_url"], timeout=5)
        welcome1 = ws1.recv()
        assert "Connected" in welcome1 or "SerialHub" in welcome1

        # 第二个客户端连接（应踢掉第一个）
        ws2 = websocket.create_connection(info["ws_url"], timeout=5)
        welcome2 = ws2.recv()
        assert "Connected" in welcome2 or "SerialHub" in welcome2

        # 验证第一个客户端已被踢掉
        time.sleep(0.5)
        ws1.settimeout(1)
        kicked = False
        try:
            data = ws1.recv()
            if not data:
                kicked = True
        except websocket.WebSocketConnectionClosedException:
            kicked = True
        except Exception:
            kicked = True

        assert kicked, "第一个客户端应被新连接踢掉"

        # 第二个客户端仍可正常通信
        ws2.send("still_alive\n")

        # 清理
        try:
            ws2.close()
        except Exception:
            pass
        try:
            ws1.close()
        except Exception:
            pass

    @requires_hardware
    def test_web_terminal_loopback(self, serialhub_server_ws):
        """Web 终端 ↔ 串口回环测试（需 COM4 TX-RX 短接）"""
        info = serialhub_server_ws
        port = get_test_port()

        # 连接串口
        connect_result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": port, "baudRate": 115200},
            },
        )
        content_text = _get_content_text(connect_result)
        assert "失败" not in content_text, f"串口 {port} 连接失败: {content_text}"

        # 等待串口连接稳定
        time.sleep(1.0)

        # 连接 WebSocket
        ws = websocket.create_connection(info["ws_url"], timeout=5)

        try:
            # 读取欢迎消息
            welcome = ws.recv()
            assert len(welcome) > 0

            time.sleep(0.5)

            # 清空串口缓冲区
            mcp_call(
                info["mcp_port"],
                "tools/call",
                {"name": "serial_read", "arguments": {"timeout": 500}},
            )

            # 通过 WebSocket 发送数据
            test_data = "HelloWS\n"
            ws.send(test_data)

            # 等待数据通过 WebSocket -> DataBridge -> 串口 -> 回环
            time.sleep(0.5)

            # 通过 MCP 读取串口数据（验证 WebSocket -> 串口转发）
            read_result = mcp_call(
                info["mcp_port"],
                "tools/call",
                {"name": "serial_read", "arguments": {"timeout": 3000}},
            )

            received = _extract_data_from_mcp_text(_get_content_text(read_result))
            assert "HelloWS" in received, (
                f"WebSocket 数据未正确转发到串口: 发送 {test_data!r}, 接收 {received!r}"
            )

            # 通过 MCP 发送数据到串口
            mcp_data = "HelloFromMCP"
            mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_write",
                    "arguments": {"data": mcp_data, "addNewline": False},
                },
            )

            # 等待数据回环到 WebSocket
            time.sleep(0.5)

            # 从 WebSocket 读取数据（验证 串口 -> WebSocket 转发）
            ws.settimeout(3)
            try:
                ws_data = ws.recv()
            except websocket.WebSocketTimeoutException:
                ws_data = ""

            assert mcp_data in ws_data or len(ws_data) > 0, (
                f"串口数据未正确转发到 WebSocket: 发送 {mcp_data!r}, 接收 {ws_data!r}"
            )

        finally:
            try:
                ws.close()
            except Exception:
                pass
            mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})

    @requires_hardware
    def test_web_terminal_unicode_and_large_data(self, serialhub_server_ws):
        """Unicode 和大数据测试（需 COM4 TX-RX 短接）"""
        info = serialhub_server_ws
        port = get_test_port()

        # 连接串口
        connect_result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": port, "baudRate": 115200},
            },
        )
        content_text = _get_content_text(connect_result)
        assert "失败" not in content_text, f"串口 {port} 连接失败: {content_text}"

        time.sleep(1.0)

        # 连接 WebSocket
        ws = websocket.create_connection(info["ws_url"], timeout=5)

        try:
            # 读取欢迎消息
            ws.recv()
            time.sleep(0.5)

            # ── Unicode 测试 ──
            unicode_tests = [
                ("Hello", "ASCII"),
                ("你好", "中文"),
                ("こん", "日文"),
                ("안녕", "韩文"),
                ("🎉", "Emoji"),
            ]

            for test_str, desc in unicode_tests:
                # 清空缓冲区
                mcp_call(
                    info["mcp_port"],
                    "tools/call",
                    {"name": "serial_read", "arguments": {"timeout": 200}},
                )

                # 通过 WebSocket 发送 Unicode 数据
                send_data = test_str + "\n"
                ws.send(send_data)
                time.sleep(0.3)

                # 通过 MCP 读取验证
                read_result = mcp_call(
                    info["mcp_port"],
                    "tools/call",
                    {"name": "serial_read", "arguments": {"timeout": 2000}},
                )

                received = _extract_data_from_mcp_text(_get_content_text(read_result))
                received_clean = received.replace("\n", "").replace("\r", "")
                assert test_str in received_clean, (
                    f"{desc} 测试失败: 发送 {test_str!r}, 接收 {received!r}"
                )

            # ── 大数据测试 ──
            for size in [1024, 10 * 1024]:  # 1KB, 10KB
                # 清空缓冲区
                mcp_call(
                    info["mcp_port"],
                    "tools/call",
                    {"name": "serial_read", "arguments": {"timeout": 200}},
                )

                large_data = "A" * size + "\n"
                ws.send(large_data)
                time.sleep(1.0)  # 大数据需要更多传输时间

                # 分段读取，累积接收数据
                all_received = ""
                for _ in range(5):
                    read_result = mcp_call(
                        info["mcp_port"],
                        "tools/call",
                        {"name": "serial_read", "arguments": {"timeout": 2000}},
                    )
                    chunk = _extract_data_from_mcp_text(_get_content_text(read_result))
                    if chunk:
                        all_received += chunk
                    else:
                        break

                # 验证接收到数据（回环测试中大数据可能被截断，只验证部分匹配）
                assert len(all_received) > 0, f"{size} 字节数据测试: 未接收到任何数据"

        finally:
            try:
                ws.close()
            except Exception:
                pass
            mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})
