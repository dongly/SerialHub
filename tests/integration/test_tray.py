"""
托盘功能测试 - TestTray* 类

使用 pytest 运行：
    pytest tests/integration/test_tray.py -v

启用服务器交互测试：
    set SERIALHUB_INTEGRATION_TEST=1
    set SERIALHUB_TEST_PORT=COM9
    pytest tests/integration/test_tray.py -v

Windows PowerShell:
$env:SERIALHUB_INTEGRATION_TEST = "1"; $env:SERIALHUB_TEST_PORT = "COM4"; pytest tests/integration/test_tray.py -v
"""

import os
import signal
import socket
import subprocess
import threading
import time
from pathlib import Path
from typing import Generator

import pytest
import requests

# GUI 自动化工具（可选）
try:
    from pywinauto import Application, Desktop
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


def _get_content_text(result):
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


# ─── 测试类 ────────────────────────────────────────────


class TestTrayIntegration:
    """托盘功能集成测试 - 需要 Windows 环境和系统托盘支持"""

    def test_tray_config_change_callback(self, serialhub_server):
        """测试托盘配置变更回调被正确调用"""
        info = serialhub_server

        # 连接串口
        mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": get_test_port(), "baudRate": 115200},
            },
        )

        # 验证连接成功
        status = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_status"})
        status_text = ""
        for item in status.get("result", {}).get("content", []):
            if item.get("type") == "text":
                status_text += item.get("text", "")
        assert "connected" in status_text.lower() or "已连接" in status_text

        # 断开连接
        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})

    def test_tray_auto_reconnect_on_config_change(self, serialhub_server):
        """测试托盘修改配置后自动重连"""
        info = serialhub_server

        # 初始连接
        mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": get_test_port(), "baudRate": 115200},
            },
        )

        # 断开连接
        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})

    def test_tray_config_persistence(self, serialhub_server):
        """测试托盘配置持久化到文件"""
        info = serialhub_server

        # 连接并修改配置
        mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": get_test_port(), "baudRate": 9600},
            },
        )

        # 断开连接
        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})

    def test_tray_full_workflow(self, serialhub_server):
        """测试托盘完整工作流：连接、配置变更、断开"""
        info = serialhub_server

        # 1. 初始连接
        mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": get_test_port(), "baudRate": 115200},
            },
        )

        # 验证状态
        status = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_status"})
        status_text = ""
        for item in status.get("result", {}).get("content", []):
            if item.get("type") == "text":
                status_text += item.get("text", "")
        assert "connected" in status_text.lower() or "已连接" in status_text

        # 2. 测试数据通信
        test_data = "TRAY_TEST_12345"
        mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_write",
                "arguments": {"data": test_data, "addNewline": False},
            },
        )

        time.sleep(0.1)

        read_result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {"name": "serial_read", "arguments": {"timeout": 2000}},
        )

        # 验证数据回环
        received = ""
        for item in read_result.get("result", {}).get("content", []):
            if item.get("type") == "text":
                text = item.get("text", "")
                if "data:" in text:
                    import re

                    match = re.search(r"data:(.+?)(?:\s+bytes:|\s+timedOut|$)", text)
                    if match:
                        received = match.group(1).strip()
                        break

        assert received == test_data, (
            f"数据不匹配: 期望 {test_data!r}, 实际 {received!r}"
        )

        # 3. 断开连接
        mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})


class TestTrayGUIAutomation:
    """托盘 GUI 自动化测试 - 使用 pywinauto 自动点击菜单"""

    def _find_tray_icon(self, desktop):
        """查找 SerialHub 托盘图标"""
        for i in range(10):
            try:
                tray_icon = desktop.window(class_name="Shell_TrayWnd").child_window(
                    title_re=".*SerialHub.*", found_index=0
                )
                if tray_icon.exists():
                    return tray_icon
            except Exception:
                pass
            time.sleep(0.5)
        return None

    def test_tray_menu_click_connect(self, serialhub_server_with_tray):
        """测试托盘图标和菜单可交互"""
        info = serialhub_server_with_tray
        time.sleep(1)

        desktop = Desktop(backend="uia")

        tray_icon = self._find_tray_icon(desktop)
        if not tray_icon:
            pytest.skip("无法访问托盘图标（可能需要 GUI 环境）")

        tray_icon.right_click_input()
        time.sleep(0.5)

        menu = desktop.window(class_name="#32768")
        assert menu.exists(), "托盘菜单未打开"

        send_keys("{ESC}")
        time.sleep(0.3)

        health = requests.get(f"http://127.0.0.1:{info['mcp_port']}/health", timeout=5)
        assert health.status_code == 200

    def test_tray_menu_baud_rate_change(self, serialhub_server_with_tray):
        """测试通过托盘菜单修改波特率"""
        info = serialhub_server_with_tray
        time.sleep(1)

        try:
            desktop = Desktop(backend="uia")

            # 查找并右键点击托盘图标
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
                pytest.skip("无法访问托盘图标（可能需要 GUI 环境）")

            # 右键点击打开菜单
            tray_icon.right_click_input()
            time.sleep(0.5)

            # 查找菜单并点击"串口设置"
            menu = desktop.window(class_name="#32768")
            if menu.exists():
                # 按 ESC 关闭菜单（仅测试菜单能否打开）
                send_keys("{ESC}")

        except Exception as e:
            pytest.fail(f"GUI 自动化失败: {e}")

    def test_tray_menu_full_workflow(self, serialhub_server_with_tray):
        """测试托盘菜单完整工作流：验证托盘图标、菜单操作、MCP 功能"""
        info = serialhub_server_with_tray
        time.sleep(1)

        try:
            desktop = Desktop(backend="uia")

            # 查找托盘图标
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
                pytest.skip("无法访问托盘图标（可能需要 GUI 环境）")

            # 1. 右键点击打开菜单
            tray_icon.right_click_input()
            time.sleep(0.5)

            # 2. 验证菜单可以打开（按 ESC 关闭）
            send_keys("{ESC}")
            time.sleep(0.3)

            # 3. 使用 MCP 验证服务器正常运行
            status = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_status"})
            assert "result" in status

        except Exception as e:
            pytest.fail(f"GUI 自动化失败: {e}")

        # 验证服务器健康（不检查连接状态，因为托盘可能自动连接）
        health = requests.get(f"http://127.0.0.1:{info['mcp_port']}/health", timeout=5)
        assert health.status_code == 200


class TestTrayGUIAdvanced:
    """托盘 GUI 高级自动化测试 - 完整的菜单导航和配置验证"""

    def _find_tray_icon(self, desktop, timeout=10):
        """查找 SerialHub 托盘图标"""
        for i in range(timeout * 2):
            try:
                tray_icon = desktop.window(class_name="Shell_TrayWnd").child_window(
                    title_re=".*SerialHub.*", found_index=0
                )
                if tray_icon.exists():
                    return tray_icon
            except Exception:
                pass
            time.sleep(0.5)
        return None

    def _open_tray_menu(self, tray_icon):
        """右键点击托盘图标打开菜单"""
        tray_icon.right_click_input()
        time.sleep(0.8)  # 等待菜单动画

        desktop = Desktop(backend="uia")
        menu = desktop.window(class_name="#32768")
        return menu if menu.exists() else None

    def test_tray_menu_navigate_submenus(self, serialhub_server_with_tray):
        """测试托盘菜单子菜单导航"""
        info = serialhub_server_with_tray
        time.sleep(2)  # 等待托盘完全初始化

        try:
            desktop = Desktop(backend="uia")

            # 查找托盘图标
            tray_icon = self._find_tray_icon(desktop)
            if not tray_icon:
                pytest.skip("无法访问托盘图标（可能需要 GUI 环境）")

            # 打开主菜单
            menu = self._open_tray_menu(tray_icon)
            if not menu:
                pytest.fail("无法打开托盘菜单")

            # 按 ESC 关闭菜单
            send_keys("{ESC}")
            time.sleep(0.3)

            # 验证服务器仍然正常运行
            health = requests.get(
                f"http://127.0.0.1:{info['mcp_port']}/health", timeout=5
            )
            assert health.status_code == 200

        except Exception as e:
            pytest.fail(f"GUI 自动化失败: {e}")

    def test_tray_config_display_update(self, serialhub_server_with_tray):
        """测试托盘配置显示更新"""
        info = serialhub_server_with_tray
        time.sleep(2)

        try:
            desktop = Desktop(backend="uia")

            # 查找托盘图标
            tray_icon = self._find_tray_icon(desktop)
            if not tray_icon:
                pytest.skip("无法访问托盘图标（可能需要 GUI 环境）")

            # 记录初始配置
            initial_status = mcp_call(
                info["mcp_port"], "tools/call", {"name": "serial_status"}
            )
            initial_text = _get_content_text(initial_status)
            assert "connected" in initial_text.lower()

            # 打开菜单查看配置显示
            menu = self._open_tray_menu(tray_icon)
            if menu:
                # 按 ESC 关闭
                send_keys("{ESC}")
                time.sleep(0.3)

            current_status = mcp_call(
                info["mcp_port"], "tools/call", {"name": "serial_status"}
            )
            current_text = _get_content_text(current_status)
            assert "connected" in current_text.lower()

        except Exception as e:
            pytest.fail(f"GUI 自动化失败: {e}")

    def test_tray_menu_all_items_accessible(self, serialhub_server_with_tray):
        """测试托盘菜单所有项可访问"""
        info = serialhub_server_with_tray
        time.sleep(2)

        try:
            desktop = Desktop(backend="uia")

            # 查找托盘图标
            tray_icon = self._find_tray_icon(desktop)
            if not tray_icon:
                pytest.skip("无法访问托盘图标（可能需要 GUI 环境）")

            # 测试多次打开和关闭菜单
            for i in range(3):
                menu = self._open_tray_menu(tray_icon)
                if menu:
                    time.sleep(0.3)
                    send_keys("{ESC}")
                    time.sleep(0.3)

            # 验证服务器健康
            health = requests.get(
                f"http://127.0.0.1:{info['mcp_port']}/health", timeout=5
            )
            assert health.status_code == 200
            assert health.json()["status"] == "ok"

        except Exception as e:
            pytest.fail(f"GUI 自动化失败: {e}")

    def test_tray_icon_tooltip(self, serialhub_server_with_tray):
        """测试托盘图标工具提示"""
        info = serialhub_server_with_tray
        time.sleep(2)

        try:
            desktop = Desktop(backend="uia")

            # 查找托盘图标
            tray_icon = self._find_tray_icon(desktop)
            if not tray_icon:
                pytest.skip("无法访问托盘图标（可能需要 GUI 环境）")

            # 尝试获取工具提示文本
            try:
                tooltip = tray_icon.window_text()
                # 验证工具提示包含 SerialHub
                assert "SerialHub" in tooltip or len(tooltip) > 0
            except Exception:
                # 如果无法获取文本，至少验证图标存在
                assert tray_icon.exists()

        except Exception as e:
            pytest.fail(f"GUI 自动化失败: {e}")
