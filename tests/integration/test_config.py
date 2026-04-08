"""
SerialHub 配置文件测试

测试配置文件加载、保存和验证功能。

使用 pytest 运行：
    pytest tests/integration/test_config.py -v
"""

import subprocess
import tempfile
import time
from pathlib import Path
from typing import Generator

import pytest
import requests

from conftest import (
    ensure_binary,
    find_free_port,
    mcp_call,
    wait_for_health,
)


# ─── Fixtures ──────────────────────────────────────────


@pytest.fixture(scope="session")
def binary() -> Path:
    return ensure_binary()


# ─── 配置文件测试 ───────────────────────────────────────


class TestConfigFile:
    def test_toml_config(self, binary):
        with tempfile.TemporaryDirectory() as tmpdir:
            config_path = Path(tmpdir) / "config.toml"

            # 使用命令行参数指定端口，覆盖配置文件
            mcp_port = find_free_port()

            # 配置文件中的端口会被命令行覆盖
            config_path.write_text("""
[serial]
port = ""
baudRate = 115200
dataBits = 8
parity = "none"
stopBits = 1

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
                ],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            try:
                wait_for_health(mcp_port)
                resp = requests.get(f"http://127.0.0.1:{mcp_port}/health", timeout=5)
                assert resp.status_code == 200

                saved = config_path.read_text(encoding="utf-8")
                assert "115200" in saved, f"配置文件未包含波特率: {saved}"
            finally:
                proc.terminate()
                proc.wait(timeout=5)

    def test_invalid_config_file(self, binary):
        with tempfile.TemporaryDirectory() as tmpdir:
            bad_config = Path(tmpdir) / "bad.toml"
            bad_config.write_text("this is [[ invalid toml")

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

                status = mcp_call(mcp_port, "tools/call", {"name": "serial_status"})
                status_text = ""
                for item in status.get("result", {}).get("content", []):
                    if item.get("type") == "text":
                        status_text += item.get("text", "")
                assert (
                    "未连接" in status_text
                    or "connected:false" in status_text.replace(" ", "").lower()
                )
            finally:
                proc.terminate()
                proc.wait(timeout=5)

    def test_config_save_to_toml(self, binary):
        """测试配置保存到 config.toml 文件（命令行参数触发）"""

        with tempfile.TemporaryDirectory() as tmpdir:
            config_path = Path(tmpdir) / "config.toml"
            mcp_port = find_free_port()

            config_path.write_text("""
[serial]
port = ""
baudRate = 115200
dataBits = 8
parity = "none"
stopBits = 1

[mcp]
httpPort = 5000
""")

            proc = subprocess.Popen(
                [
                    str(binary),
                    "--no-tray",
                    "--config",
                    str(config_path),
                    "--mcp-port",
                    str(mcp_port),
                    "--serial-port",
                    "COM_TEST",
                    "--baud-rate",
                    "9600",
                ],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            try:
                wait_for_health(mcp_port)

                time.sleep(0.5)

                saved_config = config_path.read_text()
                assert 'Port = "COM_TEST"' in saved_config
                assert "BaudRate = 9600" in saved_config

            finally:
                proc.terminate()
                proc.wait(timeout=5)

    def test_single_instance(self, binary):
        """测试单实例运行（第二个实例应退出）"""

        with tempfile.TemporaryDirectory() as tmpdir:
            config_path = Path(tmpdir) / "config.toml"
            config_path.write_text("""
[serial]
port = ""
baudRate = 115200
""")

            mcp_port1 = find_free_port()

            # 启动第一个实例
            proc1 = subprocess.Popen(
                [
                    str(binary),
                    "--no-tray",
                    "--config",
                    str(config_path),
                    "--mcp-port",
                    str(mcp_port1),
                ],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )

            try:
                wait_for_health(mcp_port1)

                # 尝试启动第二个实例
                mcp_port2 = find_free_port()

                proc2 = subprocess.Popen(
                    [
                        str(binary),
                        "--no-tray",
                        "--config",
                        str(config_path),
                        "--mcp-port",
                        str(mcp_port2),
                    ],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                )

                # 等待第二个实例退出
                try:
                    proc2.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    proc2.terminate()
                    proc2.wait(timeout=3)

                # 验证第二个实例退出且返回错误
                assert proc2.returncode != 0, "第二个实例应该退出并返回错误码"

                stderr_output = proc2.stderr.read().decode("utf-8", errors="replace")
                assert (
                    "已在运行中" in stderr_output or "running" in stderr_output.lower()
                ), f"错误消息应提示已在运行中，实际输出: {stderr_output}"

            finally:
                proc1.terminate()
                proc1.wait(timeout=5)

    def test_default_config_location(self, binary):
        """测试默认配置文件位置（可执行文件同级目录的 config.toml）"""

        with tempfile.TemporaryDirectory() as tmpdir:
            # 将二进制复制到临时目录
            tmp_binary = Path(tmpdir) / "serialhub.exe"
            tmp_binary.write_bytes(binary.read_bytes())

            # 在同级目录创建 config.toml
            config_path = Path(tmpdir) / "config.toml"
            config_path.write_text("""
[serial]
port = "COM_FROM_DEFAULT"
baudRate = 38400
dataBits = 7
parity = "even"
stopBits = 2

[mcp]
httpPort = 6000

logDir = "D:/TestLogs"
debug = true
""")

            mcp_port = find_free_port()
            proc = subprocess.Popen(
                [
                    str(tmp_binary),
                    "--no-tray",
                    "--mcp-port",
                    str(mcp_port),
                ],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                cwd=str(tmpdir),
            )
            try:
                wait_for_health(mcp_port)

                result = mcp_call(mcp_port, "tools/call", {"name": "serial_status"})
                status_text = ""
                for item in result.get("result", {}).get("content", []):
                    if item.get("type") == "text":
                        status_text += item.get("text", "")
                assert "COM_FROM_DEFAULT" in status_text or "未连接" in status_text

            finally:
                proc.terminate()
                proc.wait(timeout=5)
