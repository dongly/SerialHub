import pytest
import requests
import subprocess
import time
from pathlib import Path
from playwright.sync_api import sync_playwright, Browser, Page

SERIALHUB_BASE_URL = "http://127.0.0.1:5000"


@pytest.fixture(scope="session", autouse=True)
def serialhub_server():
    """启动 SerialHub 服务器，测试完成后关闭。"""
    import socket

    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()

    binary_path = Path(__file__).parent.parent.parent.parent / "bin" / "serialhub.exe"
    if not binary_path.exists():
        binary_path = Path(__file__).parent.parent.parent.parent / "serialhub.exe"

    proc = subprocess.Popen(
        [str(binary_path), "--mcp-port", str(port)],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
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
