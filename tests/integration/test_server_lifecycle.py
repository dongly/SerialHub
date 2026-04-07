"""SerialHub 集成测试 - 服务器生命周期测试"""

import subprocess
import pytest
import requests
from pathlib import Path
import sys

PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
sys.path.insert(0, str(PROJECT_ROOT))

from tests.integration.conftest import (
    find_free_port,
    wait_for_health,
    mcp_call,
    ensure_binary,
)


class TestServerLifecycle:
    def test_start_stop(self, serialhub_server):
        info = serialhub_server
        resp = requests.get(f"http://127.0.0.1:{info['mcp_port']}/health", timeout=5)
        assert resp.status_code == 200
        data = resp.json()
        assert data.get("status") == "ok"

    def test_invalid_serial_port(self, binary):
        mcp_port = find_free_port()

        proc = subprocess.Popen(
            [
                str(binary),
                "--no-tray",
                "--serial-port",
                "INVALID_PORT_99999",
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
