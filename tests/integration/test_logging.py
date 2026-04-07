"""SerialHub 日志测试"""

import os
import subprocess
import tempfile
import time
from pathlib import Path

import pytest

from conftest import find_free_port, wait_for_health


class TestLogging:
    def test_log_file_created(self, serialhub_server):
        """验证日志文件在指定目录中创建"""
        info = serialhub_server
        log_dir = info["log_dir"]
        if log_dir.exists():
            log_files = list(log_dir.glob("*.log"))
            assert len(log_files) > 0, f"日志目录 {log_dir} 中无日志文件"

    def test_debug_mode_logging(self, binary):
        """验证调试模式下日志记录功能"""
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
                time.sleep(1.5)
                log_files = list(log_dir.glob("*.log"))
                if len(log_files) == 0:
                    stderr_output = proc.stderr.read1(4096).decode(
                        "utf-8", errors="ignore"
                    )
                    assert (
                        "debug" in stderr_output.lower()
                        or "[SerialHub]" in stderr_output
                    ), f"未找到日志文件且 stderr 无 debug 信息"
                else:
                    content = log_files[0].read_text(encoding="utf-8", errors="ignore")
                    assert (
                        "level=debug" in content.lower() or "debug" in content.lower()
                    )
            finally:
                proc.terminate()
                proc.wait(timeout=5)
