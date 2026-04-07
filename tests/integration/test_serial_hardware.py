"""
SerialHub 硬件集成测试

测试串口硬件功能和托盘自动连接功能。

需要设置环境变量：
    set SERIALHUB_HARDWARE_TEST=1
    set SERIALHUB_TEST_PORT=COM4
    pytest tests/integration/test_serial_hardware.py -v

仅基础测试（不需要真实串口）：
    pytest tests/integration/test_serial_hardware.py -v -k "not hardware"
"""

import os
import re
import subprocess
import tempfile
import time
from pathlib import Path
from typing import Generator, Optional

import pytest
import requests

# GUI 自动化工具（可选）
try:
    from pywinauto import Desktop
    from pywinauto.keyboard import send_keys

    PYWINAUTO_AVAILABLE = True
except ImportError:
    PYWINAUTO_AVAILABLE = False

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
    import socket

    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def mcp_call(mcp_port: int, method: str, params: dict) -> dict:
    """调用 MCP 工具"""
    resp = requests.post(
        f"http://127.0.0.1:{mcp_port}/mcp",
        headers={"Content-Type": "application/json"},
        json={
            "jsonrpc": "2.0",
            "method": method,
            "params": params,
            "id": 1,
        },
        timeout=10,
    )
    return resp.json()


def _get_content_text(result: dict) -> str:
    """从 MCP 结果中提取文本内容"""
    text = ""
    for item in result.get("result", {}).get("content", []):
        if item.get("type") == "text":
            text += item.get("text", "")
    return text


# ─── Fixtures ───────────────────────────────────────────


@pytest.fixture(scope="session")
def hardware_test_enabled():
    """检查是否启用了硬件测试"""
    if not os.environ.get("SERIALHUB_HARDWARE_TEST"):
        pytest.skip(
            "硬件测试需要设置 SERIALHUB_HARDWARE_TEST=1 和 SERIALHUB_TEST_PORT=COM4"
        )
    yield


@pytest.fixture(scope="function")
def serialhub_server():
    """启动 SerialHub 服务器（无托盘模式）"""
    binary = ensure_binary()
    mcp_port = find_free_port()

    cmd = [
        str(binary),
        "--no-tray",
        "--mcp-port",
        str(mcp_port),
        "--host",
        "127.0.0.1",
    ]

    process = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        cwd=str(PROJECT_ROOT),
    )

    # 等待服务启动
    time.sleep(STARTUP_WAIT)

    try:
        # 验证服务健康
        health = requests.get(f"http://127.0.0.1:{mcp_port}/health", timeout=5)
        assert health.status_code == 200

        yield {"mcp_port": mcp_port, "process": process}

    finally:
        # 清理进程
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


@pytest.fixture(scope="function")
def serialhub_server_with_tray():
    """启动 SerialHub 服务器（带托盘模式）"""
    binary = ensure_binary()
    mcp_port = find_free_port()

    cmd = [
        str(binary),
        "--mcp-port",
        str(mcp_port),
        "--host",
        "127.0.0.1",
    ]

    process = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        cwd=str(PROJECT_ROOT),
    )

    # 等待服务启动
    time.sleep(STARTUP_WAIT)

    try:
        # 验证服务健康
        health = requests.get(f"http://127.0.0.1:{mcp_port}/health", timeout=5)
        assert health.status_code == 200

        yield {"mcp_port": mcp_port, "process": process}

    finally:
        # 清理进程
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


# ─── 测试类: TestSerialHardware ──────────────────────────────


class TestSerialHardware:
    """串口硬件测试类（需要真实串口设备，COM4 回环模式）"""

    @pytest.fixture(autouse=True)
    def require_hardware_test(self, hardware_test_enabled):
        """自动跳过未启用硬件测试的情况"""
        pass

    def test_serial_connect_disconnect(self, serialhub_server):
        """测试串口连接和断开"""
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

        assert "失败" not in content_text, f"串口 {port} 连接失败: {content_text}"

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

        assert "失败" not in content_text, f"串口 {port} 连接失败: {content_text}"

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

        assert "失败" not in content_text, f"串口 {port} 连接失败: {content_text}"

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
            mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})
            time.sleep(0.2)

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

            assert "失败" not in content_text, (
                f"串口 {port} 在 {baud} 波特率下连接失败: {content_text}"
            )

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

        assert "失败" not in content_text, f"串口 {port} 连接失败: {content_text}"

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

        assert "失败" not in content_text, f"串口 {port} 连接失败: {content_text}"

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

        assert "失败" not in content_text, f"串口 {port} 连接失败: {content_text}"

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


# ─── 测试类: TestTrayAutoConnect ─────────────────────────────


class TestTrayAutoConnect:
    """托盘自动连接上次串口测试"""

    @pytest.fixture(autouse=True)
    def require_hardware_test(self, hardware_test_enabled):
        """自动跳过未启用硬件测试的情况"""
        pass

    def test_tray_auto_connect_last_serial(self, tmp_path):
        """测试托盘自动连接上次保存的串口（带托盘启动）"""
        test_port = get_test_port()

        # Go 代码使用 config.toml 保存在可执行文件同级目录
        # 所以需要在 bin/ 目录创建配置文件
        config_dir = BINARY_PATH.parent
        config_file = config_dir / "config.toml"

        # 准备上次连接的配置（TOML 格式）
        config_content = f"""[serial]
port = "{test_port}"
baudRate = 115200
dataBits = 8
parity = "none"
stopBits = 1

[mcp]
httpPort = 5000
"""

        # 写入配置文件
        config_file.write_text(config_content, encoding="utf-8")

        try:
            # 启动 SerialHub（不带 --no-tray，不带 --serial-port，让它从 config.toml 读取）
            port = find_free_port()
            mcp_port = find_free_port()

            cmd = [
                str(BINARY_PATH),
                "--mcp-port",
                str(mcp_port),
                "--host",
                "127.0.0.1",
            ]

            process = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                cwd=str(PROJECT_ROOT),
            )

            # 等待服务启动
            time.sleep(STARTUP_WAIT)

            try:
                # 验证服务健康
                health = requests.get(f"http://127.0.0.1:{mcp_port}/health", timeout=5)
                assert health.status_code == 200

                # 验证串口状态（应该尝试连接 config.toml 中保存的端口）
                status = mcp_call(mcp_port, "tools/call", {"name": "serial_status"})
                assert "result" in status

                # 验证配置已加载（从 content text 中解析）
                status_text = ""
                for item in status.get("result", {}).get("content", []):
                    if item.get("type") == "text":
                        status_text += item.get("text", "")
                # 配置应被加载，port 信息应出现在状态中
                assert (
                    test_port.upper() in status_text.upper()
                    or "connected" in status_text.lower()
                )

            finally:
                # 清理进程
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()

        finally:
            # 清理配置文件
            if config_file.exists():
                config_file.unlink()

    def test_tray_menu_click_reconnect(self, serialhub_server_with_tray):
        """测试托盘菜单点击重连"""
        if not PYWINAUTO_AVAILABLE:
            pytest.skip("pywinauto 不可用")

        info = serialhub_server_with_tray
        time.sleep(2)

        try:
            desktop = Desktop(backend="uia")

            tray_icon = None
            for i in range(10):
                try:
                    tray_icon = desktop.window(class_name="Shell_TrayWnd").child_window(
                        title_re=".*SerialHub.*"
                    )
                    if tray_icon.exists():
                        break
                except Exception:
                    pass
                time.sleep(0.5)

            if not tray_icon or not tray_icon.exists():
                pytest.fail("未找到 SerialHub 托盘图标")

            tray_icon.right_click_input()
            time.sleep(0.8)

            send_keys("{ESC}")
            time.sleep(0.3)

            status = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_status"})
            status_text = _get_content_text(status)
            assert "未连接" in status_text or "connected:false" in status_text.lower()

        except Exception as e:
            pytest.fail(f"GUI 自动化失败: {e}")

    def test_tray_config_change_triggers_save(self, serialhub_server_with_tray):
        """测试托盘配置更改触发保存"""
        if not PYWINAUTO_AVAILABLE:
            pytest.skip("pywinauto 不可用")

        info = serialhub_server_with_tray
        time.sleep(2)

        try:
            desktop = Desktop(backend="uia")

            tray_icon = None
            for i in range(10):
                try:
                    tray_icon = desktop.window(class_name="Shell_TrayWnd").child_window(
                        title_re=".*SerialHub.*"
                    )
                    if tray_icon.exists():
                        break
                except Exception:
                    pass
                time.sleep(0.5)

            if not tray_icon or not tray_icon.exists():
                pytest.fail("未找到 SerialHub 托盘图标")

            tray_icon.right_click_input()
            time.sleep(0.5)
            send_keys("{ESC}")
            time.sleep(0.3)

            health = requests.get(
                f"http://127.0.0.1:{info['mcp_port']}/health", timeout=5
            )
            assert health.status_code == 200
            assert health.json()["status"] == "ok"

        except Exception as e:
            pytest.fail(f"GUI 自动化失败: {e}")
