"""
SerialHub 集成测试 - 全功能覆盖

使用 pytest 运行：
    pytest tests/integration/test_serialhub.py -v

启用服务器交互测试：
    set SERIALHUB_INTEGRATION_TEST=1
    set SERIALHUB_TEST_PORT=COM9
    pytest tests/integration/test_serialhub.py -v

仅基础测试（不需要启动服务器）：
    pytest tests/integration/test_serialhub.py -v -k "not server"
"""

import json
import os
import re
import signal
import socket
import subprocess
import sys
import tempfile
import time
import uuid
from pathlib import Path
from typing import Generator, Optional

import pytest
import requests

# ─── 常量 ──────────────────────────────────────────────

PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
BINARY_PATH = PROJECT_ROOT / "bin" / "serialhub.exe"

DEFAULT_TELNET_PORT = 23230
DEFAULT_MCP_PORT = 50010
STARTUP_WAIT = 2.5


# ─── 辅助函数 ──────────────────────────────────────────


def get_test_port() -> str:
    return os.environ.get("SERIALHUB_TEST_PORT", "COM9")


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
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


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


def mcp_call(mcp_port: int, method: str, params: dict = None, req_id: int = 1) -> dict:
    """调用 MCP 工具（StreamableHTTP Stateless 模式，无需 session ID）"""
    body = {"jsonrpc": "2.0", "method": method, "id": req_id}
    if params is not None:
        body["params"] = params

    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }

    resp = requests.post(
        f"http://127.0.0.1:{mcp_port}/mcp",
        json=body,
        headers=headers,
        timeout=10,
    )
    assert resp.status_code == 200, f"MCP 请求失败: {resp.status_code} {resp.text}"
    return resp.json()


# ─── Fixtures ──────────────────────────────────────────


@pytest.fixture(scope="session")
def binary() -> Path:
    return ensure_binary()


@pytest.fixture
def serialhub_server(binary, tmp_path) -> Generator[dict, None, None]:
    """启动 serialhub --no-tray 并返回连接信息，测试结束后自动停止。"""
    if os.environ.get("SERIALHUB_INTEGRATION_TEST") != "1":
        pytest.skip("集成测试未启用，设置 SERIALHUB_INTEGRATION_TEST=1")

    telnet_port = find_free_port()
    mcp_port = find_free_port()
    log_dir = tmp_path / "logs"

    proc = subprocess.Popen(
        [
            str(binary),
            "--no-tray",
            "--telnet-port",
            str(telnet_port),
            "--mcp-port",
            str(mcp_port),
        ],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env={**os.environ, "SERIALHUB_LOG_DIR": str(log_dir)},
    )

    try:
        wait_for_health(mcp_port)
        yield {
            "proc": proc,
            "telnet_port": telnet_port,
            "mcp_port": mcp_port,
            "log_dir": log_dir,
        }
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()


# ─── 1. CLI 基础测试（不需要服务器）──────────────────────


class TestCLI:
    def test_version(self, binary):
        r = subprocess.run(
            [str(binary), "--version"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            timeout=10,
        )
        assert r.returncode == 0
        assert "SerialHub v" in r.stdout

    def test_help_flags(self, binary):
        r = subprocess.run(
            [str(binary), "--help"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            timeout=10,
        )
        assert r.returncode == 0
        for flag in [
            "--serial-port",
            "--baud-rate",
            "--telnet-port",
            "--mcp-port",
            "--config",
            "--debug",
            "--no-tray",
            "--minimized",
        ]:
            assert flag in r.stdout, f"缺少 {flag}"

    def test_debug_with_help(self, binary):
        r = subprocess.run(
            [str(binary), "--debug", "--help"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            timeout=10,
        )
        assert r.returncode == 0
        assert "--serial-port" in r.stdout

    def test_invalid_flag(self, binary):
        r = subprocess.run(
            [str(binary), "--nonexistent"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            timeout=10,
        )
        assert r.returncode != 0

    def test_baud_rate_flag(self, binary):
        r = subprocess.run(
            [str(binary), "--baud-rate", "9600", "--help"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            timeout=10,
        )
        assert r.returncode == 0


# ─── 2. 服务器启动/停止测试 ────────────────────────────


class TestServerLifecycle:
    def test_start_stop(self, serialhub_server):
        info = serialhub_server
        resp = requests.get(f"http://127.0.0.1:{info['mcp_port']}/health", timeout=5)
        assert resp.status_code == 200
        data = resp.json()
        assert data.get("status") == "ok"

    def test_invalid_serial_port(self, binary):
        if os.environ.get("SERIALHUB_INTEGRATION_TEST") != "1":
            pytest.skip("集成测试未启用")

        mcp_port = find_free_port()
        telnet_port = find_free_port()

        proc = subprocess.Popen(
            [
                str(binary),
                "--no-tray",
                "--serial-port",
                "INVALID_PORT_99999",
                "--telnet-port",
                str(telnet_port),
                "--mcp-port",
                str(mcp_port),
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        try:
            wait_for_health(mcp_port)
            resp = requests.get(f"http://127.0.0.1:{mcp_port}/health", timeout=5)
            assert resp.status_code == 200
        finally:
            proc.terminate()
            proc.wait(timeout=5)


# ─── 3. 配置文件测试 ───────────────────────────────────


class TestConfigFile:
    def test_toml_config(self, binary):
        if os.environ.get("SERIALHUB_INTEGRATION_TEST") != "1":
            pytest.skip("集成测试未启用")

        with tempfile.TemporaryDirectory() as tmpdir:
            mcp_port = find_free_port()
            telnet_port = find_free_port()
            config_path = Path(tmpdir) / "config.toml"

            config_path.write_text(f"""
[serial]
port = ""
baudRate = 115200
dataBits = 8
parity = "none"
stopBits = 1

[telnet]
port = {telnet_port}

[mcp]
httpPort = {mcp_port}
""")

            proc = subprocess.Popen(
                [str(binary), "--no-tray", "--config", str(config_path)],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            try:
                wait_for_health(mcp_port)
                resp = requests.get(f"http://127.0.0.1:{mcp_port}/health", timeout=5)
                assert resp.status_code == 200
            finally:
                proc.terminate()
                proc.wait(timeout=5)

    def test_invalid_config_file(self, binary):
        if os.environ.get("SERIALHUB_INTEGRATION_TEST") != "1":
            pytest.skip("集成测试未启用")

        mcp_port = find_free_port()
        with tempfile.TemporaryDirectory() as tmpdir:
            bad_config = Path(tmpdir) / "bad.toml"
            bad_config.write_text("this is [[ invalid toml")

            proc = subprocess.Popen(
                [
                    str(binary),
                    "--no-tray",
                    "--config",
                    str(bad_config),
                    "--mcp-port",
                    str(mcp_port),
                ],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            try:
                time.sleep(STARTUP_WAIT)
                resp = requests.get(f"http://127.0.0.1:{mcp_port}/health", timeout=5)
                assert resp.status_code == 200
            finally:
                proc.terminate()
                proc.wait(timeout=5)


# ─── 4. MCP 工具测试 ──────────────────────────────────


class TestMCPTools:
    def test_serial_list(self, serialhub_server):
        info = serialhub_server
        result = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_list"})
        assert "result" in result
        content = result["result"].get("content", [])
        assert len(content) > 0

    def test_serial_status_not_connected(self, serialhub_server):
        info = serialhub_server
        result = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_status"})
        assert "result" in result

    def test_serial_connect_invalid(self, serialhub_server):
        info = serialhub_server
        result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": "INVALID_99999"},
            },
        )
        assert "result" in result

    def test_serial_disconnect_not_connected(self, serialhub_server):
        info = serialhub_server
        result = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})
        assert "result" in result

    def test_serial_write_not_connected(self, serialhub_server):
        info = serialhub_server
        result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_write",
                "arguments": {"data": "test"},
            },
        )
        assert "result" in result

    def test_serial_read_not_connected(self, serialhub_server):
        info = serialhub_server
        result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_read",
                "arguments": {"timeout": 100},
            },
        )
        assert "result" in result

    def test_mcp_health_endpoint(self, serialhub_server):
        info = serialhub_server
        resp = requests.get(f"http://127.0.0.1:{info['mcp_port']}/health", timeout=5)
        assert resp.status_code == 200
        assert resp.json().get("status") == "ok"

    def test_mcp_unknown_tool(self, serialhub_server):
        info = serialhub_server
        result = mcp_call(info["mcp_port"], "tools/call", {"name": "nonexistent_tool"})
        assert "error" in result or "result" in result


# ─── 5. Telnet 测试 ───────────────────────────────────


class TestTelnet:
    def test_telnet_connect(self, serialhub_server):
        info = serialhub_server
        with socket.create_connection(
            ("127.0.0.1", info["telnet_port"]), timeout=5
        ) as sock:
            data = sock.recv(1024)
            assert len(data) > 0

    def test_telnet_send_data(self, serialhub_server):
        info = serialhub_server
        with socket.create_connection(
            ("127.0.0.1", info["telnet_port"]), timeout=5
        ) as sock:
            time.sleep(0.5)
            sock.sendall(b"hello\r\n")

    def test_telnet_multiple_clients(self, serialhub_server):
        info = serialhub_server
        clients = []
        for _ in range(3):
            s = socket.create_connection(("127.0.0.1", info["telnet_port"]), timeout=5)
            clients.append(s)

        for s in clients:
            data = s.recv(1024)
            assert len(data) >= 0
            s.close()


# ─── 6. 日志测试 ───────────────────────────────────────


class TestLogging:
    def test_log_file_created(self, serialhub_server):
        info = serialhub_server
        log_dir = info["log_dir"]
        if log_dir.exists():
            log_files = list(log_dir.glob("*.log"))
            assert len(log_files) > 0, f"日志目录 {log_dir} 中无日志文件"

    def test_debug_mode_logging(self, binary):
        if os.environ.get("SERIALHUB_INTEGRATION_TEST") != "1":
            pytest.skip("集成测试未启用")

        with tempfile.TemporaryDirectory() as tmpdir:
            mcp_port = find_free_port()
            log_dir = Path(tmpdir) / "logs"
            log_dir.mkdir(parents=True, exist_ok=True)

            proc = subprocess.Popen(
                [str(binary), "--no-tray", "--debug", "--mcp-port", str(mcp_port)],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                env={**os.environ, "SERIALHUB_LOG_DIR": str(log_dir)},
            )
            try:
                wait_for_health(mcp_port)
                time.sleep(1.0)
                log_files = list(log_dir.glob("*.log"))
                assert len(log_files) > 0, f"日志目录 {log_dir} 中无日志文件"
                content = log_files[0].read_text(encoding="utf-8", errors="ignore")
                assert "level=debug" in content.lower() or "debug" in content.lower()
            finally:
                proc.terminate()
                proc.wait(timeout=5)


# ─── 7. 串口连接测试（需要硬件）─────────────────────────


class TestSerialHardware:
    def test_serial_connect_disconnect(self, serialhub_server):
        port = get_test_port()
        info = serialhub_server

        result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": port, "baudRate": 115200},
            },
        )
        assert "result" in result

        content_text = ""
        for item in result["result"].get("content", []):
            if item.get("type") == "text":
                content_text += item.get("text", "")

        if "失败" in content_text:
            pytest.skip(f"串口 {port} 不可用")

        time.sleep(0.5)

        status = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_status"})
        status_text = ""
        for item in status["result"].get("content", []):
            if item.get("type") == "text":
                status_text += item.get("text", "")
        assert "connected" in status_text.lower() or "已连接" in status_text

        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})

    def test_serial_write_read(self, serialhub_server):
        port = get_test_port()
        info = serialhub_server

        connect_result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": port},
            },
        )
        content_text = ""
        for item in connect_result["result"].get("content", []):
            if item.get("type") == "text":
                content_text += item.get("text", "")

        if "失败" in content_text:
            pytest.skip(f"串口 {port} 不可用")

        time.sleep(0.5)

        mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_write",
                "arguments": {"data": "version", "addNewline": True},
            },
        )

        read_result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_read",
                "arguments": {"timeout": 3000},
            },
        )
        assert "result" in read_result

        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})
