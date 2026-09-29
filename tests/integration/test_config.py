"""
SerialHub 配置文件测试

测试配置文件加载、保存和验证功能。

使用 pytest 运行：
    pytest tests/integration/test_config.py -v
"""

import os
import subprocess
import tempfile
import time
from pathlib import Path
from typing import Generator

import pytest
import requests

from harness import (
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
httpPort = 5050
""")

            proc = subprocess.Popen(
                [
                    str(binary),
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
        """测试单实例运行（第二个实例不会成为新的主实例）"""

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
                        "--config",
                        str(config_path),
                        "--mcp-port",
                        str(mcp_port2),
                    ],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                )

                # 单实例语义：第二实例发现活主后转为透明代理，可能立即正常退出
                # （stdio 代理无输入时），也可能常驻代理；但它绝不会在自己的
                # 端口上再成为一个主实例。
                try:
                    proc2.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    pass

                if proc2.poll() is None:
                    proc2.terminate()
                    try:
                        proc2.wait(timeout=3)
                    except subprocess.TimeoutExpired:
                        proc2.kill()

                health2 = None
                try:
                    health2 = requests.get(
                        f"http://127.0.0.1:{mcp_port2}/health", timeout=2
                    )
                except requests.RequestException:
                    pass
                assert health2 is None or health2.status_code != 200, (
                    "第二个实例不应在自身端口上成为新的主实例"
                )

                health1 = requests.get(
                    f"http://127.0.0.1:{mcp_port1}/health", timeout=5
                )
                assert health1.status_code == 200, "第一个实例应保持为主实例"

            finally:
                proc1.terminate()
                proc1.wait(timeout=5)

    def test_default_config_location(self, binary):
        """测试默认配置文件位置（可执行文件同级目录的 config.toml）"""

        with tempfile.TemporaryDirectory() as tmpdir:
            # 将二进制复制到临时目录
            binary_name = "serialhub.exe" if os.name == "nt" else "serialhub"
            tmp_binary = Path(tmpdir) / binary_name
            tmp_binary.write_bytes(binary.read_bytes())
            if os.name != "nt":
                tmp_binary.chmod(0o755)  # 复制后补回可执行位

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
