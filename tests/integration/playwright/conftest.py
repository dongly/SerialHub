import pytest
import requests
from playwright.sync_api import sync_playwright, Browser, Page

SERIALHUB_BASE_URL = "http://127.0.0.1:5000"


@pytest.fixture(scope="session", autouse=True)
def serialhub_running():
    """检查 SerialHub 服务是否运行，否则跳过所有 Playwright 测试。"""
    try:
        resp = requests.get(f"{SERIALHUB_BASE_URL}/health", timeout=3)
        if resp.status_code != 200:
            pytest.skip("SerialHub 服务健康检查未通过，跳过 Playwright 测试")
    except requests.ConnectionError:
        pytest.skip(
            "SerialHub 服务未运行 (http://127.0.0.1:5000)，跳过 Playwright 测试"
        )
    except requests.Timeout:
        pytest.skip("SerialHub 服务健康检查超时，跳过 Playwright 测试")


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
