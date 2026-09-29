"""集成测试公共辅助函数（避免与子目录 conftest.py 的模块名冲突）。

原位于 conftest.py；pytest 会以模块名 `conftest` 缓存各级 conftest，子目录同名文件
会遮蔽父级 conftest，因此 helper 独立成模块供测试直接导入。
"""


import os
import re
import select
import socket
import subprocess
import threading
import time
from pathlib import Path

import pytest
import requests


# ─── 常量 ──────────────────────────────────────────────

PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
BINARY_NAME = "serialhub.exe" if os.name == "nt" else "serialhub"
BINARY_PATH = PROJECT_ROOT / "bin" / BINARY_NAME
DEFAULT_MCP_PORT = 50010
STARTUP_WAIT = 2.5


# ─── 辅助函数 ──────────────────────────────────────────


# ─── 伪串口（无硬件测试）───────────────────────────────
# POSIX：用 pty 模拟串口，master 侧自动回写模拟 TX-RX 短接。
# Windows：自动检测 com0com 虚拟串口对（如 COM22↔COM23），一端作测试串口、
# 另一端起回显线程；也可用 SERIALHUB_TEST_PORT/SERIALHUB_TEST_PORT_PEER 覆盖。
_PSEUDO_SERIAL: dict = {}
_PEER_ECHO_STARTED = False


def pseudo_serial_port() -> str:
    """惰性创建 pty 伪串口并启动回环泵，返回设备路径。"""
    if "path" in _PSEUDO_SERIAL:
        return _PSEUDO_SERIAL["path"]

    pty = pytest.importorskip("pty")
    master, slave = pty.openpty()
    _PSEUDO_SERIAL.update({"path": os.ttyname(slave), "master": master, "slave": slave})

    def _pump() -> None:
        while True:
            ready, _, _ = select.select([master], [], [], 0.2)
            if not ready:
                continue
            try:
                data = os.read(master, 4096)
            except BlockingIOError:
                continue
            except OSError:
                return
            if not data:
                return
            # 回环：把数据写回 master，SerialHub 从 slave 侧即可读到。
            # master 为非阻塞 fd：写缓冲满（如 10KB 大数据）时抛
            # BlockingIOError，必须重试而不是终止泵线程，否则丢包。
            written = 0
            deadline = time.monotonic() + 5.0
            while written < len(data) and time.monotonic() < deadline:
                try:
                    written += os.write(master, data[written:])
                except BlockingIOError:
                    time.sleep(0.005)
                except OSError:
                    return

    threading.Thread(target=_pump, daemon=True).start()
    return _PSEUDO_SERIAL["path"]


def _start_peer_echo(peer: str) -> None:
    """Windows：在 com0com 对端端口回显收到的数据（模拟 TX-RX 短接）。

    需要 pyserial；未安装时 pytest.importorskip 会跳过整个测试。
    """
    global _PEER_ECHO_STARTED
    if _PEER_ECHO_STARTED:
        return
    serial = pytest.importorskip("serial")

    def _pump() -> None:
        try:
            conn = serial.Serial(peer, 115200, timeout=0.2)
        except Exception as exc:  # noqa: BLE001 - 打开失败时给出可读提示
            print(f"[harness] 无法打开回显端口 {peer}: {exc}")
            return
        with conn:
            while True:
                data = conn.read(4096)
                if data:
                    conn.write(data)

    threading.Thread(target=_pump, daemon=True).start()
    _PEER_ECHO_STARTED = True


def _com0com_ports() -> list[str]:
    """Windows：从注册表 SERIALCOMM 读取 com0com 虚拟串口（\\Device\\com0com*）。

    返回排序后的端口名（通常一对：COM22/COM23）。非 Windows 或读取失败返回 []。
    """
    if os.name != "nt":
        return []
    try:
        import winreg
    except ImportError:  # 理论上不会发生
        return []

    ports: list[str] = []
    try:
        with winreg.OpenKey(
            winreg.HKEY_LOCAL_MACHINE, r"HARDWARE\DEVICEMAP\SERIALCOMM"
        ) as key:
            index = 0
            while True:
                try:
                    name, value, _ = winreg.EnumValue(key, index)
                except OSError:
                    break
                if "com0com" in name.lower():
                    ports.append(str(value))
                index += 1
    except OSError:
        return []
    return sorted(set(ports))


def get_test_port() -> str:
    """测试串口：SERIALHUB_TEST_PORT > POSIX pty 伪设备 > Windows com0com > COM4。

    Windows 上未显式指定时自动检测 com0com 虚拟串口对（如 COM22↔COM23），
    并在对端启动回显线程；也可用 SERIALHUB_TEST_PORT / SERIALHUB_TEST_PORT_PEER
    显式覆盖（只设前者时需自备回环）。
    """
    env_port = os.environ.get("SERIALHUB_TEST_PORT")
    if env_port:
        peer = os.environ.get("SERIALHUB_TEST_PORT_PEER")
        if peer:
            _start_peer_echo(peer)
        return env_port
    if os.name == "posix":
        return pseudo_serial_port()
    pair = _com0com_ports()
    if len(pair) >= 2:
        _start_peer_echo(pair[1])
        return pair[0]
    return "COM4"


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
) -> tuple[subprocess.Popen, int]:
    """启动服务器并等待健康检查，失败时重试。

    Returns:
        tuple: (proc, mcp_port)
    """
    for attempt in range(max_retries):
        mcp_port = find_free_port()

        cmd = [
            str(binary),
            "--mcp-port",
            str(mcp_port),
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
            return proc, mcp_port
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


def _get_content_text(result) -> str:
    """从 MCP 工具调用结果中提取文本内容。"""
    texts = []
    for item in result.get("result", {}).get("content", []):
        if item.get("type") == "text":
            texts.append(item.get("text", ""))
    return "".join(texts)

def read_result_parts(result) -> tuple:
    """提取 serial_read 等工具的负载与字节数 (data, n)。

    优先 structuredContent（ToolResult.Data），回退旧文本 data:/bytes: 格式。
    """
    res = result.get("result", {}) or {}
    structured = res.get("structuredContent")
    if isinstance(structured, dict) and isinstance(structured.get("data"), str):
        data = structured["data"]
        try:
            n = int(structured.get("bytes", len(data.encode("utf-8"))))
        except (TypeError, ValueError):
            n = len(data.encode("utf-8"))
        return data, n

    text = ""
    for item in res.get("content", []):
        if item.get("type") == "text":
            text += item.get("text", "")
    match = re.search(r"data[:=]\s*(.*?)(?:\s+bytes[:=]|\s+timedOut|$)", text, re.S)
    data = match.group(1).strip() if match else ""
    match_bytes = re.search(r"bytes[:=]\s*(\d+)", text)
    n = int(match_bytes.group(1)) if match_bytes else len(data.encode("utf-8"))
    return data, n


def read_result_data(result) -> str:
    """提取负载字符串（不需要字节数时的便捷封装）。"""
    return read_result_parts(result)[0]


def to_text(data) -> str:
    """WebSocket 帧统一转 str（服务端以 binary 帧发送）。"""
    if isinstance(data, (bytes, bytearray)):
        return data.decode("utf-8", errors="replace")
    return data


def recv_text(ws) -> str:
    """接收一帧并转 str。"""
    return to_text(ws.recv())
