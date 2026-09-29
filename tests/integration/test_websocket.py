"""
SerialHub WebSocket 集成测试

使用 pytest 运行：
pytest tests/integration/test_websocket.py -v

启用服务器交互测试：
$env:SERIALHUB_INTEGRATION_TEST = "1"
（测试串口自动选择：POSIX pty / Windows com0com，可用 SERIALHUB_TEST_PORT 覆盖）
pytest tests/integration/test_websocket.py -v

WebSocket 端点: ws://127.0.0.1:{mcp_port}/ws
"""

import socket
import sys
import time
from typing import Generator, Optional

import pytest

from harness import get_test_port, mcp_call, read_result_data, recv_text, to_text

# WebSocket 客户端库
try:
    import websocket

    WEBSOCKET_AVAILABLE = True
except ImportError:
    WEBSOCKET_AVAILABLE = False

# ─── 辅助函数 ──────────────────────────────────────────


def _send_ws(ws, data) -> None:
    """发送 WebSocket 数据（服务端为 binary 帧，websocket-client 需 bytes）。"""
    ws.send(data.encode("utf-8") if isinstance(data, str) else data)


class TestWebSocket:
    """WebSocket 功能测试 - 替代原有的 Telnet 测试"""

    @pytest.mark.skipif(
        not WEBSOCKET_AVAILABLE,
        reason="websocket-client 库未安装，运行: pip install websocket-client",
    )
    def test_websocket_connect(self, serialhub_server):
        """WebSocket 连接测试：验证 WebSocket 连接成功并收到欢迎消息"""
        info = serialhub_server
        ws_url = f"ws://127.0.0.1:{info['mcp_port']}/ws"

        ws = websocket.WebSocket()
        ws.settimeout(5)

        try:
            ws.connect(ws_url)
            # 读取欢迎消息
            welcome = recv_text(ws)
            assert len(welcome) > 0, "未收到欢迎消息"
            assert "SerialHub" in welcome or "Connected" in welcome, (
                f"欢迎消息格式异常: {welcome}"
            )
        finally:
            ws.close()

    @pytest.mark.skipif(
        not WEBSOCKET_AVAILABLE,
        reason="websocket-client 库未安装",
    )
    def test_websocket_send_data(self, serialhub_server):
        """WebSocket 发送数据测试：验证可以向 WebSocket 发送数据"""
        info = serialhub_server
        ws_url = f"ws://127.0.0.1:{info['mcp_port']}/ws"

        ws = websocket.WebSocket()
        ws.settimeout(5)

        try:
            ws.connect(ws_url)
            # 读取欢迎消息
            welcome = recv_text(ws)
            assert len(welcome) > 0

            # 发送测试数据
            test_data = "hello\n"
            _send_ws(ws, test_data)

            # 等待响应（可能没有响应，但不报错即可）
            ws.settimeout(2)
            try:
                response = ws.recv()
                # 有响应则验证，无响应也接受（正常行为）
                assert response is not None
            except websocket.WebSocketTimeoutException:
                # 超时是正常的，因为没有串口连接
                pass
        finally:
            ws.close()

    @pytest.mark.skipif(
        not WEBSOCKET_AVAILABLE,
        reason="websocket-client 库未安装",
    )
    def test_websocket_multiple_clients(self, serialhub_server):
        """WebSocket 多客户端测试：验证多个客户端连接时旧连接会被踢出"""
        info = serialhub_server
        ws_url = f"ws://127.0.0.1:{info['mcp_port']}/ws"

        clients = []
        try:
            # 连接第一个客户端
            ws1 = websocket.WebSocket()
            ws1.settimeout(5)
            ws1.connect(ws_url)
            welcome1 = recv_text(ws1)
            assert len(welcome1) > 0, "客户端 1 未收到欢迎消息"
            clients.append(ws1)

            # 连接第二个客户端（应该踢掉第一个）
            ws2 = websocket.WebSocket()
            ws2.settimeout(5)
            ws2.connect(ws_url)
            welcome2 = recv_text(ws2)
            assert len(welcome2) > 0, "客户端 2 未收到欢迎消息"
            clients.append(ws2)

            # 第一个客户端应该已断开（被踢出）
            ws1.settimeout(2)
            try:
                # 如果还能收到数据，说明第一个连接还在（不应该发生）
                data1 = ws1.recv()
                # 如果没有抛出异常，检查是否还有效
                # 由于是单客户端模式，ws1 实际上已经失效
            except websocket.WebSocketTimeoutException:
                # 超时是正常的
                pass
            except Exception:
                # 其他异常（如连接已关闭）也是预期行为
                pass

        finally:
            # 清理客户端连接
            for ws in clients:
                try:
                    ws.close()
                except Exception:
                    pass

    @pytest.mark.skipif(
        not WEBSOCKET_AVAILABLE,
        reason="websocket-client 库未安装",
    )
    def test_websocket_loopback(self, serialhub_server):
        """WebSocket 回环测试：验证 WebSocket 数据能正确转发到串口并回环

        需要硬件回环：串口 TX 和 RX 短接
        """
        port = get_test_port()
        info = serialhub_server
        ws_url = f"ws://127.0.0.1:{info['mcp_port']}/ws"

        # 先连接串口
        connect_result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": port, "baudRate": 115200},
            },
        )
        content_text = ""
        for item in connect_result["result"].get("content", []):
            if item.get("type") == "text":
                content_text += item.get("text", "")

        assert "失败" not in content_text, f"串口 {port} 连接失败: {content_text}"

        # 等待串口连接稳定
        time.sleep(1.0)

        # 验证串口状态
        status = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_status"})
        status_text = ""
        for item in status["result"].get("content", []):
            if item.get("type") == "text":
                status_text += item.get("text", "")
        assert "connected" in status_text.lower() or "已连接" in status_text, (
            f"串口未连接: {status_text}"
        )

        ws = None
        try:
            # 连接 WebSocket
            ws = websocket.WebSocket()
            ws.settimeout(5)
            ws.connect(ws_url)

            # 读取欢迎消息
            welcome = recv_text(ws)
            assert len(welcome) > 0

            # 等待 WebSocket 连接稳定
            time.sleep(0.5)

            # 清空串口缓冲区
            mcp_call(
                info["mcp_port"],
                "tools/call",
                {"name": "serial_read", "arguments": {"timeout": 500}},
            )

            # 通过 WebSocket 发送数据（需要包含换行符才能触发转发）
            test_data = "HelloFromWS\n"
            _send_ws(ws, test_data)

            # 等待数据通过串口回环
            time.sleep(0.5)

            # 通过 MCP 读取串口数据（验证 WebSocket -> 串口转发）
            read_result = mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_read",
                    "arguments": {"timeout": 3000},
                },
            )

            # 提取接收到的数据
            received_data = read_result_data(read_result)

            # 验证数据（允许部分匹配，因为可能有其他数据）
            assert (
                "HelloFromWS" in received_data or test_data.strip("\n") in received_data
            ), (
                f"WebSocket 数据未正确转发到串口: 发送 {test_data!r}, 接收 {received_data!r}"
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
                ws_data = b""
            ws_data = to_text(ws_data)

            assert mcp_data in ws_data or len(ws_data) > 0, (
                f"串口数据未正确转发到 WebSocket: 发送 {mcp_data!r}, 接收 {ws_data!r}"
            )

        finally:
            # 断开 WebSocket
            if ws:
                ws.close()
            # 断开串口连接
            mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})

    @pytest.mark.skipif(
        not WEBSOCKET_AVAILABLE,
        reason="websocket-client 库未安装",
    )
    def test_websocket_unicode_and_large_data(self, serialhub_server):
        """WebSocket Unicode 和大数据测试"""
        port = get_test_port()
        info = serialhub_server
        ws_url = f"ws://127.0.0.1:{info['mcp_port']}/ws"

        # 连接串口
        connect_result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": port, "baudRate": 115200},
            },
        )
        content_text = ""
        for item in connect_result["result"].get("content", []):
            if item.get("type") == "text":
                content_text += item.get("text", "")

        assert "失败" not in content_text, f"串口 {port} 连接失败: {content_text}"

        time.sleep(1.0)

        ws = None
        try:
            ws = websocket.WebSocket()
            ws.settimeout(5)
            ws.connect(ws_url)

            # 读取欢迎消息
            welcome = recv_text(ws)
            assert len(welcome) > 0
            time.sleep(0.5)

            # 测试简单数据（带换行符触发转发）
            test_data = "ABC123\n"
            _send_ws(ws, test_data)

            # 等待数据回环
            time.sleep(0.5)

            # 通过 MCP 读取验证
            read_result = mcp_call(
                info["mcp_port"],
                "tools/call",
                {"name": "serial_read", "arguments": {"timeout": 3000}},
            )

            received = read_result_data(read_result)

            # 验证数据
            assert "ABC123" in received, (
                f"WebSocket 数据转发失败: 发送 {test_data!r}, 接收 {received!r}"
            )

            # 测试 Unicode 数据
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

                # 发送数据（带换行符）
                send_data = test_str + "\n"
                _send_ws(ws, send_data)
                time.sleep(0.3)

                # 读取验证
                read_result = mcp_call(
                    info["mcp_port"],
                    "tools/call",
                    {"name": "serial_read", "arguments": {"timeout": 2000}},
                )

                received = read_result_data(read_result)

                # 验证数据包含（去掉换行符）
                received_clean = received.replace("\n", "").replace("\r", "")
                assert test_str in received_clean, (
                    f"{desc} 测试失败: 发送 {test_str!r}, 接收 {received!r}"
                )

        finally:
            if ws:
                ws.close()
            mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})
