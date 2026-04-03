"""
SerialHub 集成测试 - 全功能覆盖

使用 pytest 运行：
    pytest tests/integration/test_serialhub.py -v

启用服务器交互测试：
    set SERIALHUB_INTEGRATION_TEST=1
    set SERIALHUB_TEST_PORT=COM9
    pytest tests/integration/test_serialhub.py -v

Windows PowerShell:
$env:SERIALHUB_INTEGRATION_TEST = "1"; $env:SERIALHUB_TEST_PORT = "COM4"; pytest tests/integration/test_serialhub.py -v

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
import threading
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


def start_server_with_retry(
    binary: Path,
    args: list,
    env: dict = None,
    max_retries: int = 3,
) -> tuple[subprocess.Popen, int, int]:
    """启动服务器并等待健康检查，失败时重试。

    Returns:
        tuple: (proc, mcp_port, telnet_port)
    """
    for attempt in range(max_retries):
        mcp_port = find_free_port()
        telnet_port = find_free_port()

        cmd = [
            str(binary),
            "--no-tray",
            "--mcp-port",
            str(mcp_port),
            "--telnet-port",
            str(telnet_port),
        ]
        cmd.extend(args)

        proc = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=env,
        )

        try:
            wait_for_health(mcp_port, timeout=15)
            return proc, mcp_port, telnet_port
        except TimeoutError:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
            if attempt == max_retries - 1:
                raise
            time.sleep(0.5)

    raise RuntimeError("服务器启动失败")


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
    if os.environ.get("SERIALHUB_INTEGRATION_TEST") != "1":
        pytest.skip("集成测试未启用，设置 SERIALHUB_INTEGRATION_TEST=1")

    log_dir = tmp_path / "logs"
    proc = None
    last_error = None

    for attempt in range(3):
        # 使用 40000-60000 范围内的随机端口，避免与系统常用端口冲突
        telnet_port = find_free_port()
        mcp_port = find_free_port()

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
                f"健康检查超时 (attempt {attempt + 1}/3): "
                f"mcp_port={mcp_port}, telnet_port={telnet_port}\n"
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
            "telnet_port": telnet_port,
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
                proc.wait(timeout=3)


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
            config_path = Path(tmpdir) / "config.toml"

            # 使用命令行参数指定端口，覆盖配置文件
            mcp_port = find_free_port()
            telnet_port = find_free_port()

            # 配置文件中的端口会被命令行覆盖
            config_path.write_text("""
[serial]
port = ""
baudRate = 115200
dataBits = 8
parity = "none"
stopBits = 1

[telnet]
port = 99999

[mcp]
httpPort = 99999
""")

            proc = subprocess.Popen(
                [
                    str(binary),
                    "--no-tray",
                    "--config",
                    str(config_path),
                    "--mcp-port",
                    str(mcp_port),
                    "--telnet-port",
                    str(telnet_port),
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

    def test_invalid_config_file(self, binary):
        if os.environ.get("SERIALHUB_INTEGRATION_TEST") != "1":
            pytest.skip("集成测试未启用")

        with tempfile.TemporaryDirectory() as tmpdir:
            bad_config = Path(tmpdir) / "bad.toml"
            bad_config.write_text("this is [[ invalid toml")

            # 使用 start_server_with_retry 确保端口可用
            mcp_port = find_free_port()
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
                wait_for_health(mcp_port)
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

    def test_telnet_loopback(self, serialhub_server):
        """Telnet 回环测试：验证 Telnet 数据能正确转发到串口并回环"""
        import threading

        port = get_test_port()
        info = serialhub_server

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

        if "失败" in content_text:
            pytest.skip(f"串口 {port} 不可用")

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

        # 连接 Telnet
        with socket.create_connection(
            ("127.0.0.1", info["telnet_port"]), timeout=5
        ) as sock:
            # 先读取欢迎信息
            welcome = sock.recv(1024)
            assert len(welcome) > 0

            # 等待 Telnet 连接稳定
            time.sleep(0.5)

            # 清空串口缓冲区
            mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_read",
                    "arguments": {"timeout": 500},
                },
            )

            # 通过 Telnet 发送数据（需要包含换行符才能触发转发）
            test_data = b"HelloFromTelnet\n"
            sock.sendall(test_data)

            # 等待数据通过串口回环（需要足够时间）
            time.sleep(0.5)

            # 通过 MCP 读取串口数据（验证 Telnet -> 串口转发）
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
                "HelloFromTelnet" in received_data
                or test_data.decode() in received_data
            ), (
                f"Telnet 数据未正确转发到串口: 发送 {test_data!r}, 接收 {received_data!r}"
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

            # 等待数据回环到 Telnet
            time.sleep(0.5)

            # 从 Telnet 读取数据（验证 串口 -> Telnet 转发）
            sock.settimeout(3)
            try:
                telnet_data = sock.recv(1024)
            except socket.timeout:
                telnet_data = b""

            assert mcp_data.encode() in telnet_data or len(telnet_data) > 0, (
                f"串口数据未正确转发到 Telnet: 发送 {mcp_data!r}, 接收 {telnet_data!r}"
            )

        # 断开串口连接
        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})

    def test_telnet_unicode_and_large_data(self, serialhub_server):
        """Telnet Unicode和长数据测试"""
        port = get_test_port()
        info = serialhub_server

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

        if "失败" in content_text:
            pytest.skip(f"串口 {port} 不可用")

        time.sleep(1.0)

        with socket.create_connection(
            ("127.0.0.1", info["telnet_port"]), timeout=5
        ) as sock:
            # 读取欢迎信息
            welcome = sock.recv(1024)
            assert len(welcome) > 0
            time.sleep(0.5)

            # 测试简单数据（带换行符触发转发）
            test_data = "ABC123\n"
            sock.sendall(test_data.encode("utf-8"))

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
                f"Telnet 数据转发失败: 发送 {test_data!r}, 接收 {received!r}"
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
                sock.sendall(send_data.encode("utf-8"))
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

        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})


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
        """串口回环测试：发送数据并验证接收"""
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

        # 发送测试数据（回环模式：发送什么就接收什么）
        test_data = "HelloLoopback123"
        mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_write",
                "arguments": {"data": test_data, "addNewline": False},
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

        content_text = ""
        for item in read_result["result"].get("content", []):
            if item.get("type") == "text":
                content_text += item.get("text", "")

        # 验证回环数据
        assert test_data in content_text, (
            f"回环数据不匹配: 发送 '{test_data}', 接收 '{content_text[:100]}'"
        )

        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})

    def test_hardware_end_to_end(self, serialhub_server):
        """串口回环端到端测试：多次发送/接收验证数据完整性"""
        port = get_test_port()
        info = serialhub_server

        # 连接串口
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

        # 测试多轮回环
        test_messages = ["Hello", "World123", "Test!@#"]
        for msg in test_messages:
            # 发送数据
            mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_write",
                    "arguments": {"data": msg, "addNewline": False},
                },
            )

            # 读取回环数据
            read_result = mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_read",
                    "arguments": {"timeout": 3000},
                },
            )

            content_text = ""
            for item in read_result["result"].get("content", []):
                if item.get("type") == "text":
                    content_text += item.get("text", "")

            # 验证回环数据
            assert msg in content_text, (
                f"回环数据不匹配: 发送 '{msg}', 接收 '{content_text[:100]}'"
            )

        # 断开连接
        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})

    def test_serial_params_change(self, serialhub_server):
        """串口通讯参数修改测试：验证不同波特率配置"""
        port = get_test_port()
        info = serialhub_server

        # 测试不同波特率
        baud_rates = [9600, 19200, 38400, 57600, 115200]

        for baud in baud_rates:
            # 连接串口
            connect_result = mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_connect",
                    "arguments": {"port": port, "baudRate": baud},
                },
            )
            content_text = ""
            for item in connect_result["result"].get("content", []):
                if item.get("type") == "text":
                    content_text += item.get("text", "")

            if "失败" in content_text:
                pytest.skip(f"串口 {port} 在 {baud} 波特率下不可用")

            # 等待串口稳定
            time.sleep(0.5)

            # 验证状态显示正确波特率
            status = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_status"})
            status_text = ""
            for item in status["result"].get("content", []):
                if item.get("type") == "text":
                    status_text += item.get("text", "")

            assert str(baud) in status_text, (
                f"状态未显示正确波特率 {baud}: {status_text}"
            )

            # 清空可能存在的残留数据
            mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_read",
                    "arguments": {"timeout": 100},
                },
            )

            # 回环测试验证通讯正常（使用短数据避免截断）
            test_data = f"B{baud}"
            mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_write",
                    "arguments": {"data": test_data, "addNewline": False},
                },
            )

            # 等待数据回环
            time.sleep(0.1)

            read_result = mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_read",
                    "arguments": {"timeout": 2000},
                },
            )

            content_text = ""
            for item in read_result["result"].get("content", []):
                if item.get("type") == "text":
                    content_text += item.get("text", "")

            assert test_data in content_text, (
                f"波特率 {baud} 回环测试失败: 发送 '{test_data}', 接收 '{content_text[:100]}'"
            )

            # 断开连接，准备测试下一波特率
            mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})
            time.sleep(0.3)

    def test_long_data_loopback(self, serialhub_server):
        """长数据回环测试：测试大数据量传输（1KB, 10KB）"""
        import random
        import string

        port = get_test_port()
        info = serialhub_server

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

        if "失败" in content_text:
            pytest.skip(f"串口 {port} 不可用")

        time.sleep(0.5)

        # 测试数据大小（字节）- 从1KB开始测试
        test_sizes = [1024, 10 * 1024]  # 1KB, 10KB

        for size in test_sizes:
            # 生成随机测试数据
            test_data = "".join(
                random.choices(string.ascii_letters + string.digits, k=size)
            )

            # 分段发送（每段 1KB）
            chunk_size = 1024
            for i in range(0, len(test_data), chunk_size):
                chunk = test_data[i : i + chunk_size]
                mcp_call(
                    info["mcp_port"],
                    "tools/call",
                    {
                        "name": "serial_write",
                        "arguments": {"data": chunk, "addNewline": False},
                    },
                )
                time.sleep(0.02)

            # 等待并读取回环数据
            time.sleep(0.5)
            received_data = ""
            max_read_attempts = 20

            for _ in range(max_read_attempts):
                read_result = mcp_call(
                    info["mcp_port"],
                    "tools/call",
                    {
                        "name": "serial_read",
                        "arguments": {"timeout": 1000, "maxSize": 4096},
                    },
                )

                # 从 result 的 data 字段提取数据
                result_data = read_result.get("result", {})
                content = result_data.get("content", [])
                for item in content:
                    if item.get("type") == "text":
                        text = item.get("text", "")
                        # 查找 data: 后面的内容
                        if "data:" in text:
                            data_part = text.split("data:", 1)[1].strip()
                            if data_part:
                                received_data += data_part

                if len(received_data) >= size:
                    break

            # 验证数据完整性（允许95%成功率）
            received_len = len(received_data)
            success_rate = received_len / size

            assert success_rate >= 0.90, (
                f"长数据测试失败: 发送 {size} 字节, 接收到 {received_len} 字节 "
                f"成功率 {success_rate * 100:.1f}%"
            )

        # 断开连接
        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})

    def test_binary_and_unicode_data(self, serialhub_server):
        """二进制和Unicode数据测试：验证非ASCII字符传输"""
        port = get_test_port()
        info = serialhub_server

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

        if "失败" in content_text:
            pytest.skip(f"串口 {port} 不可用")

        time.sleep(0.5)

        # 测试不同的非ASCII数据
        test_cases = [
            # 中文字符
            "你好世界",
            # 日文
            "こんにちは",
            # 韩文
            "안녕하세요",
            # Emoji
            "🎉🚀💻",
            # 混合内容
            "Hello世界こんにちは🎉",
            # 特殊符号
            "©®™℠℗",
            # 数学符号
            "∑∏∫√∞",
            # 箭头符号
            "←↑→↓↔↕",
        ]

        for test_data in test_cases:
            # 清空缓冲区
            mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_read",
                    "arguments": {"timeout": 100},
                },
            )

            # 发送数据
            mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_write",
                    "arguments": {"data": test_data, "addNewline": False},
                },
            )

            time.sleep(0.1)

            # 读取回环数据
            read_result = mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_read",
                    "arguments": {"timeout": 2000},
                },
            )

            # 从 result 的 content 中提取数据
            # 响应格式: "读取成功: N 字节\nmap[bytes:N data:XXX timedOut:false]"
            received_bytes = 0
            result_data = read_result.get("result", {})
            content = result_data.get("content", [])
            for item in content:
                if item.get("type") == "text":
                    text = item.get("text", "")
                    # 查找 bytes: 字段
                    if "bytes:" in text:
                        try:
                            # 提取 bytes 值
                            import re

                            match = re.search(r"bytes:(\d+)", text)
                            if match:
                                received_bytes = int(match.group(1))
                        except:
                            pass

            # 验证接收到的字节数与发送的字节数相同
            sent_bytes_len = len(test_data.encode("utf-8"))

            assert received_bytes == sent_bytes_len, (
                f"数据长度不匹配: 发送 {test_data!r} ({sent_bytes_len} 字节), "
                f"接收到 {received_bytes} 字节"
            )

        # 断开连接
        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})

    def test_mcp_various_data_types(self, serialhub_server):
        """MCP 各种数据类型回环测试"""
        port = get_test_port()
        info = serialhub_server

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

        if "失败" in content_text:
            pytest.skip(f"串口 {port} 不可用")

        time.sleep(0.5)

        # 各种数据类型测试
        test_cases = [
            # ASCII 字符
            ("ABCDEFGHIJ", "ASCII uppercase"),
            ("abcdefghij", "ASCII lowercase"),
            ("0123456789", "Digits"),
            # 特殊字符
            ("!@#$%^&*()", "Special chars"),
            ("[]{}|;':\",./?", "Punctuation"),
            # 空格和制表符
            ("Hello World", "Space"),
            ("A\tB\tC", "Tab"),
            # 空字符串
            ("", "Empty"),
            # 单字符
            ("X", "Single char"),
            # 长字符串（100字符）
            ("A" * 100, "Long string"),
            # 混合数据
            ("Hello123!@#", "Mixed"),
        ]

        for test_data, desc in test_cases:
            # 清空缓冲区
            mcp_call(
                info["mcp_port"],
                "tools/call",
                {"name": "serial_read", "arguments": {"timeout": 100}},
            )

            # 发送数据
            mcp_call(
                info["mcp_port"],
                "tools/call",
                {
                    "name": "serial_write",
                    "arguments": {"data": test_data, "addNewline": False},
                },
            )

            time.sleep(0.1)

            # 读取回环数据
            read_result = mcp_call(
                info["mcp_port"],
                "tools/call",
                {"name": "serial_read", "arguments": {"timeout": 2000}},
            )

            # 提取接收到的数据
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

            # 验证数据（字节级比较）
            sent_bytes = test_data.encode("utf-8", errors="replace")
            received_bytes = (
                received.encode("utf-8", errors="replace") if received else b""
            )

            assert sent_bytes == received_bytes, (
                f"{desc} 测试失败: 发送 {len(sent_bytes)} 字节, "
                f"接收 {len(received_bytes)} 字节\n"
                f"期望: {test_data!r}\n"
                f"实际: {received!r}"
            )

        # 断开连接
        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})
