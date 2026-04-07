"""SerialHub 测试 fixtures 和辅助函数

提取自 test_serialhub.py，用于共享测试逻辑。
"""

import json
import os
import socket
import subprocess
import threading
import time
from pathlib import Path
from typing import Generator, Optional

import pytest
import requests


# ─── 常量 ──────────────────────────────────────────────

PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
BINARY_PATH = PROJECT_ROOT / "bin" / "serialhub.exe"
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
) -> tuple[subprocess.Popen, int]:
    """启动服务器并等待健康检查，失败时重试。

    Returns:
        tuple: (proc, mcp_port)
    """
    for attempt in range(max_retries):
        mcp_port = find_free_port()

        cmd = [
            str(binary),
            "--no-tray",
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
        # 使用 40000-60000 范围内的随机端口，避免与系统常用端口冲突
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
            proc.wait(timeout=3)


@pytest.fixture
def serialhub_server_with_tray(binary, tmp_path) -> Generator[dict, None, None]:
    """启动 serialhub（带托盘，不带 --no-tray）用于 GUI 自动化测试。"""
    log_dir = tmp_path / "logs"
    proc = None
    last_error = None

    for attempt in range(3):
        mcp_port = find_free_port()

        proc = subprocess.Popen(
            [
                str(binary),
                "--mcp-port",
                str(mcp_port),
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env={**os.environ, "SERIALHUB_LOG_DIR": str(log_dir)},
        )

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
            last_error = f"带托盘服务器启动超时 (attempt {attempt + 1}/3)"
            continue

        break
    else:
        raise RuntimeError(f"带托盘服务器启动失败:\n{last_error}")

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
            proc.wait(timeout=3)
