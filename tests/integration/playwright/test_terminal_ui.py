import pytest
import requests
from playwright.sync_api import Page

SERIALHUB_BASE_URL = "http://127.0.0.1:5000"
TERMINAL_URL = f"{SERIALHUB_BASE_URL}/terminal"


def test_terminal_page_loads():
    resp = requests.get(TERMINAL_URL, timeout=5)
    assert resp.status_code == 200
    assert "text/html" in resp.headers.get("Content-Type", "")


def test_html_contains_xterm_references(page: Page):
    page.goto(TERMINAL_URL, wait_until="load")
    html = page.content()
    assert "/static/xterm.css" in html
    assert "/static/xterm.min.js" in html
    assert "terminal-container" in html
    assert "new Terminal" in html


def test_static_resources_load():
    resources = [
        "/static/xterm.css",
        "/static/xterm.min.js",
        "/static/xterm-addon-fit.min.js",
    ]
    for path in resources:
        resp = requests.get(f"{SERIALHUB_BASE_URL}{path}", timeout=5)
        assert resp.status_code == 200, f"{path} 返回 {resp.status_code}"


def test_xterm_css_mime_type():
    resp = requests.get(f"{SERIALHUB_BASE_URL}/static/xterm.css", timeout=5)
    assert resp.status_code == 200
    content_type = resp.headers.get("Content-Type", "")
    assert "text/css" in content_type, f"实际 Content-Type: {content_type}"


def test_xterm_js_mime_type():
    resp = requests.get(f"{SERIALHUB_BASE_URL}/static/xterm.min.js", timeout=5)
    assert resp.status_code == 200
    content_type = resp.headers.get("Content-Type", "")
    assert "javascript" in content_type, f"实际 Content-Type: {content_type}"


def test_no_console_errors(page: Page):
    errors = []
    page.on("console", lambda msg: errors.append(msg) if msg.type == "error" else None)
    page.on("requestfailed", lambda req: errors.append(f"请求失败: {req.url}"))

    page.goto(TERMINAL_URL, wait_until="networkidle")
    page.wait_for_timeout(500)

    assert len(errors) == 0, f"发现 {len(errors)} 个错误: {[str(e) for e in errors]}"


def test_terminal_object_exists(page: Page):
    page.goto(TERMINAL_URL, wait_until="load")
    result = page.evaluate("typeof Terminal")
    assert result != "undefined", "Terminal 构造函数未定义，xterm.min.js 可能未加载"


def test_websocket_connects(page: Page):
    page.goto(TERMINAL_URL, wait_until="load")

    ws_opened = page.evaluate("""() => {
        return new Promise((resolve) => {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = protocol + '//' + window.location.host + '/ws';
            const ws = new WebSocket(wsUrl);
            ws.onopen = () => {
                ws.close();
                resolve(true);
            };
            ws.onerror = () => resolve(false);
            setTimeout(() => resolve(false), 5000);
        });
    }""")

    assert ws_opened is True, "WebSocket 连接 /ws 未能在 5 秒内建立"
