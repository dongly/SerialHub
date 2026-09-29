import os
import pytest
import requests
import subprocess
import time
from pathlib import Path

# 未安装 playwright 时整个目录跳过（否则 conftest 导入失败会中断 pytest 收集）
pytest.importorskip("playwright.sync_api", reason="浏览器 UI 测试需要 playwright")

from playwright.sync_api import sync_playwright, Browser, Page  # noqa: E402

SERIALHUB_BASE_URL = "http://127.0.0.1:5050"


@pytest.fixture(scope="module", autouse=True)
def serialhub_server():
    """启动 SerialHub 服务器，测试完成后关闭。

    使用独立的 XDG_CONFIG_HOME/LOCALAPPDATA：否则服务器会在整个会话期间
    持有会话级隔离目录里的单实例锁，使后续测试文件启动的实例转入透明代理，
    无法在自身端口提供服务（健康检查必然超时）。
    """
    import socket
    import tempfile

    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()

    binary_name = "serialhub.exe" if os.name == "nt" else "serialhub"
    root = Path(__file__).parent.parent.parent.parent
    binary_path = root / "bin" / binary_name
    if not binary_path.exists():
        binary_path = root / binary_name

    env = {
        **os.environ,
        "XDG_CONFIG_HOME": tempfile.mkdtemp(prefix="serialhub-ui-xdg-"),
        "LOCALAPPDATA": tempfile.mkdtemp(prefix="serialhub-ui-localappdata-"),
    }
    proc = subprocess.Popen(
        [str(binary_path), "--no-browser", "--mcp-port", str(port)],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=env,
    )

    max_retries = 30
    for i in range(max_retries):
        try:
            resp = requests.get(f"http://127.0.0.1:{port}/health", timeout=1)
            if resp.status_code == 200:
                break
        except:
            pass
        time.sleep(0.5)
    else:
        proc.terminate()
        proc.wait(timeout=5)
        raise RuntimeError(f"SerialHub 服务启动失败 (port={port})")

    global SERIALHUB_BASE_URL
    SERIALHUB_BASE_URL = f"http://127.0.0.1:{port}"

    yield {"proc": proc, "port": port, "url": SERIALHUB_BASE_URL}

    proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait()


@pytest.fixture(scope="session")
def browser():
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        yield browser
        browser.close()


@pytest.fixture
def page(browser):
    page = browser.new_page()
    yield page
    page.close()
