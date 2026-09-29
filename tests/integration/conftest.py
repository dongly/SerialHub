"""SerialHub 测试 fixtures（辅助函数见 harness.py）"""

import os
import subprocess
import threading
import time
from pathlib import Path
from typing import Generator

import pytest
import requests

from harness import (  # noqa: F401
    read_result_data,
    _collect_output,
    _com0com_ports,
    _get_content_text,
    _start_peer_echo,
    ensure_binary,
    find_free_port,
    get_test_port,
    mcp_call,
    pseudo_serial_port,
    start_server_with_retry,
    wait_for_health,
    BINARY_NAME,
    BINARY_PATH,
    DEFAULT_MCP_PORT,
    PROJECT_ROOT,
    STARTUP_WAIT,
)


# ─── Fixtures ──────────────────────────────────────────


@pytest.fixture(scope="session")
def binary() -> Path:
    return ensure_binary()


@pytest.fixture
def serialhub_server(binary, tmp_path) -> Generator[dict, None, None]:
    """启动 serialhub 并返回连接信息，测试结束后自动停止。

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


@pytest.fixture(scope="session", autouse=True)
def _isolate_serialhub_env(tmp_path_factory):
    """把配置、日志与单实例锁隔离到临时目录，避免与本机已运行的 SerialHub 冲突。"""
    keys = ("XDG_CONFIG_HOME", "LOCALAPPDATA", "SERIALHUB_LOG_DIR")
    saved = {k: os.environ.get(k) for k in keys}
    base_dir = tmp_path_factory.mktemp("serialhub-env")
    os.environ["XDG_CONFIG_HOME"] = str(base_dir / "xdg")
    os.environ["LOCALAPPDATA"] = str(base_dir / "localappdata")
    os.environ["SERIALHUB_LOG_DIR"] = str(base_dir / "logs")
    try:
        yield
    finally:
        for k, v in saved.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v


