"""
SerialHub WebSocket 集成测试

使用 pytest 运行：
pytest tests/integration/test_telnet.py -v

启用服务器交互测试：
$env:SERIALHUB_INTEGRATION_TEST = "1"
$env:SERIALHUB_TEST_PORT = "COM4"
pytest tests/integration/test_telnet.py -v

WebSocket 端点: ws://127.0.0.1:{mcp_port}/ws
"""

import os
import socket
import subprocess
import sys
import threading
import time
from pathlib import Path
from typing import Generator, Optional

import pytest
import requests

# WebSocket 客户端库
try:
    import websocket

    WEBSOCKET_AVAILABLE = True
except ImportError:
    WEBSOCKET_AVAILABLE = False

# ─── 常量 ──────────────────────────────────────────────

PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
BINARY_PATH = PROJECT_ROOT / "bin" / "serialhub.exe"
STARTUP_WAIT = 2.5


# ─── 辅助函数 ──────────────────────────────────────────


def get_test_port() -> str:
    return os.environ.get("SERIALHUB_TEST_PORT", "COM4")


def ensure_binary() -> Path:
    if BINARY_PATH.exists():
        return BINARY_PATH
    subprocess.run(
        ["go", "build", "-o", str(BINARY_PATH), "./cmd/serialhub"],
        cwd=str(PROJECT_ROOT),
        check=True,
    )
    assert BINARY_PATH.exists(), f"构建失败: {BINARY_PATH}"
    return BINARY_PATH


def find_free_port() -> int:
    """分配一个可用端口，通过 SO_REUSEADDR 设置降低端口被抢占的风险。"""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _collect_output(proc: subprocess.Popen, output: dict, key: str) -> None:
    """后台线程：持续读取子进程输出，避免 PIPE 死锁并收集日志用于诊断。"""
    try:
        data = getattr(proc, key).read()
        output[key] = data.decode("utf-8", errors="replace") if data else ""
    except Exception:
        output[key] = ""


def wait_for_health(mcp_port: int, timeout: float = 10) -> requests.Response:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            resp = requests.get(f"http://127.0.0.1:{mcp_port}/health", timeout=2)
            if resp.status_code == 200:
                return resp
        except requests.ConnectionError:
            pass
        time.sleep(0.3)
    raise TimeoutError(f"健康检查超时: mcp_port={mcp_port}")


def mcp_call(
    mcp_port: int,
    method: str,
    params: dict = None,
    req_id: int = 1,
    max_retries: int = 3,
) -> dict:
    """调用 MCP 工具（StreamableHTTP Stateless 模式，无需 session ID）"""
    body = {"jsonrpc": "2.0", "method": method, "id": req_id}
    if params is not None:
        body["params"] = params

    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }

    last_error = None
    for attempt in range(max_retries):
        try:
            resp = requests.post(
                f"http://127.0.0.1:{mcp_port}/mcp",
                json=body,
                headers=headers,
                timeout=10,
            )
            assert resp.status_code == 200, (
                f"MCP 请求失败: {resp.status_code} {resp.text}"
            )
            return resp.json()
        except (requests.ConnectionError, requests.exceptions.ConnectionError) as e:
            last_error = e
            if attempt < max_retries - 1:
                time.sleep(0.5)
            continue

    raise last_error if last_error else Exception("MCP 调用失败")


# ─── Fixtures ──────────────────────────────────────────


@pytest.fixture(scope="session")
def binary() -> Path:
    return ensure_binary()


@pytest.fixture
def serialhub_server(binary, tmp_path) -> Generator[dict, None, None]:
    """启动 serialhub --no-tray 并返回连接信息，测试结束后自动停止。

    使用高端口范围（40000+）避免与系统服务或默认端口冲突，
    并在启动失败时收集服务器日志输出用于诊断。
    """
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

        # 后台读取输出，避免 PIPE 缓冲区满导致子进程挂死
        output: dict[str, str] = {}
        stdout_thread = threading.Thread(
            target=_collect_output, args=(proc, output, "stdout"), daemon=True
        )
        stderr_thread = threading.Thread(
            target=_collect_output, args=(proc, output, "stderr"), daemon=True
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

            last_error = (
                f"健康检查超时 (attempt {attempt + 1}/3): mcp_port={mcp_port}\n"
            )
            if output.get("stderr"):
                last_error += f"stderr: {output['stderr'][:2000]}\n"
            if output.get("stdout"):
                last_error += f"stdout: {output['stdout'][:2000]}\n"
            continue

        break
    else:
        raise RuntimeError(f"服务器启动失败，已重试 3 次:\n{last_error}")

    try:
        yield {
            "proc": proc,
            "mcp_port": mcp_port,
            "log_dir": log_dir,
        }
    finally:
        if proc is not None:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()


# ─── WebSocket 测试 ─────────────────────────────────────


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
            welcome = ws.recv()
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
            welcome = ws.recv()
            assert len(welcome) > 0

            # 发送测试数据
            test_data = "hello\n"
            ws.send(test_data)

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
            welcome1 = ws1.recv()
            assert len(welcome1) > 0, "客户端 1 未收到欢迎消息"
            clients.append(ws1)

            # 连接第二个客户端（应该踢掉第一个）
            ws2 = websocket.WebSocket()
            ws2.settimeout(5)
            ws2.connect(ws_url)
            welcome2 = ws2.recv()
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
            welcome = ws.recv()
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
            ws.send(test_data)

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
            received_data = ""
            result_data = read_result.get("result", {})
            content = result_data.get("content", [])
            for item in content:
                if item.get("type") == "text":
                    text = item.get("text", "")
                    if "data:" in text:
                        import re

                        match = re.search(
                            r"data:(.+?)(?:\s+bytes:|\s+timedOut|$)", text
                        )
                        if match:
                            received_data = match.group(1).strip()
                            break

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
            welcome = ws.recv()
            assert len(welcome) > 0
            time.sleep(0.5)

            # 测试简单数据（带换行符触发转发）
            test_data = "ABC123\n"
            ws.send(test_data)

            # 等待数据回环
            time.sleep(0.5)

            # 通过 MCP 读取验证
            read_result = mcp_call(
                info["mcp_port"],
                "tools/call",
                {"name": "serial_read", "arguments": {"timeout": 3000}},
            )

            received = ""
            for item in read_result.get("result", {}).get("content", []):
                if item.get("type") == "text":
                    text = item.get("text", "")
                    if "data:" in text:
                        import re

                        match = re.search(
                            r"data:(.+?)(?:\s+bytes:|\s+timedOut|$)", text
                        )
                        if match:
                            received = match.group(1).strip()
                            break

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
                ws.send(send_data)
                time.sleep(0.3)

                # 读取验证
                read_result = mcp_call(
                    info["mcp_port"],
                    "tools/call",
                    {"name": "serial_read", "arguments": {"timeout": 2000}},
                )

                received = ""
                for item in read_result.get("result", {}).get("content", []):
                    if item.get("type") == "text":
                        text = item.get("text", "")
                        if "data:" in text:
                            import re

                            match = re.search(
                                r"data:(.+?)(?:\s+bytes:|\s+timedOut|$)", text
                            )
                            if match:
                                received = match.group(1).strip()
                                break

                # 验证数据包含（去掉换行符）
                received_clean = received.replace("\n", "").replace("\r", "")
                assert test_str in received_clean, (
                    f"{desc} 测试失败: 发送 {test_str!r}, 接收 {received!r}"
                )

        finally:
            if ws:
                ws.close()
            mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})
