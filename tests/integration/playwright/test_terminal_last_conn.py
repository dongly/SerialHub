"""Web 终端记住上次端口与连接参数的 UI 测试。

需求：串口选择框默认选中上次端口（若仍可用），波特率/数据位/校验/停止位
等参数同样恢复上次值；连接前将选择写入 localStorage。
"""
import json
import pytest
from playwright.sync_api import Page


def _goto_terminal(page: Page, base_url: str, init_script: str = ""):
    if init_script:
        page.add_init_script(init_script)
    page.goto(f"{base_url}/terminal", wait_until="networkidle")
    page.wait_for_timeout(500)  # 等待 get_ports 响应与 updatePortList 执行


def _conn_init_script(raw: str) -> str:
    """构造写入 serialhub-last-conn 的初始化脚本（raw 为存入的原始字符串）"""
    return "localStorage.setItem('serialhub-last-conn', " + json.dumps(raw) + ");"


def _select_options(page: Page, select_id: str) -> list:
    return page.eval_on_selector(
        f"#{select_id}", "el => Array.from(el.options).map(o => o.value)"
    )


def test_params_restored_from_localstorage(page: Page, serialhub_server):
    """页面启动时恢复上次保存的连接参数（非法值回落默认）"""
    params = {"baudRate": "9600", "dataBits": "7", "parity": "even", "stopBits": "2"}
    _goto_terminal(
        page,
        serialhub_server["url"],
        init_script=_conn_init_script(json.dumps(params)),
    )
    assert page.input_value("#baud-rate") == "9600", "波特率未恢复为上次值"
    assert page.input_value("#data-bits") == "7", "数据位未恢复为上次值"
    assert page.input_value("#parity") == "even", "校验位未恢复为上次值"
    assert page.input_value("#stop-bits") == "2", "停止位未恢复为上次值"


def test_invalid_params_keep_defaults(page: Page, serialhub_server):
    """保存值不是现有选项时保持 HTML 默认选中项"""
    params = {"baudRate": "12345", "dataBits": "9", "parity": "mark", "stopBits": "3"}
    _goto_terminal(
        page,
        serialhub_server["url"],
        init_script=_conn_init_script(json.dumps(params)),
    )
    assert page.input_value("#baud-rate") == "115200", "非法波特率应回落默认值"
    assert page.input_value("#data-bits") == "8", "非法数据位应回落默认值"
    assert page.input_value("#parity") == "none", "非法校验位应回落默认值"
    assert page.input_value("#stop-bits") == "1", "非法停止位应回落默认值"


@pytest.mark.parametrize(
    "raw,desc",
    [
        ("null", "JSON null"),
        ("42", "数字标量"),
        ("[1,2]", "数组"),
        ('{"baudRate":{}}', "嵌套对象"),
        ("not-json{", "损坏 JSON"),
    ],
)
def test_corrupted_storage_keeps_defaults(
    page: Page, serialhub_server, raw: str, desc: str
):
    """存储内容为 null/标量/数组/嵌套对象/损坏 JSON 时保持全部默认且页面正常初始化"""
    _goto_terminal(
        page,
        serialhub_server["url"],
        init_script=_conn_init_script(raw),
    )
    # 四个参数框全部保持 HTML 默认
    assert page.input_value("#baud-rate") == "115200", f"{desc} 时波特率应回落默认值"
    assert page.input_value("#data-bits") == "8", f"{desc} 时数据位应回落默认值"
    assert page.input_value("#parity") == "none", f"{desc} 时校验位应回落默认值"
    assert page.input_value("#stop-bits") == "1", f"{desc} 时停止位应回落默认值"
    # 页面脚本未中断：恢复块之后的 fetch('/version') 仍执行（版本号已填充）
    page.wait_for_function(
        "() => document.getElementById('app-version').textContent.length > 0",
        timeout=5000,
    )
    version = page.text_content("#app-version")
    assert version.startswith("v"), f"{desc} 时页面初始化被中断（版本号未填充）"


def test_port_selected_when_available(page: Page, serialhub_server):
    """上次端口仍出现在列表中时默认选中（若串口可用）"""
    base_url = serialhub_server["url"]
    _goto_terminal(page, base_url)
    ports = [p for p in _select_options(page, "port-select") if p]
    if not ports:
        pytest.skip("当前环境无可用串口，跳过端口恢复用例")
    target = ports[0]
    page.evaluate(
        "t => localStorage.setItem('serialhub-last-conn', JSON.stringify({port: t}))",
        target,
    )
    page.reload(wait_until="networkidle")
    page.wait_for_timeout(500)
    assert page.input_value("#port-select") == target, "上次可用端口未被默认选中"


def test_port_falls_back_to_placeholder(page: Page, serialhub_server):
    """上次端口不在当前列表时保持占位项，不误选"""
    _goto_terminal(
        page,
        serialhub_server["url"],
        init_script=_conn_init_script(json.dumps({"port": "/dev/nonexistent"})),
    )
    assert page.input_value("#port-select") == "", "不可用端口不应被选中"


def test_refresh_returns_to_last_conn(page: Page, serialhub_server):
    """刷新列表后默认回到上次连接的端口（覆盖当前手选）"""
    base_url = serialhub_server["url"]
    _goto_terminal(page, base_url)
    ports = [p for p in _select_options(page, "port-select") if p]
    if len(ports) < 2:
        pytest.skip("可用串口少于 2 个，无法构造手选与上次端口不同的场景")
    last, other = ports[0], ports[1]
    page.evaluate(
        "v => localStorage.setItem('serialhub-last-conn', JSON.stringify({port: v}))",
        last,
    )
    page.reload(wait_until="networkidle")
    page.wait_for_timeout(500)
    assert page.input_value("#port-select") == last, "加载时应选中上次端口"
    page.select_option("#port-select", other)  # 手选另一个
    page.click("#refresh-btn")
    page.wait_for_timeout(500)
    assert page.input_value("#port-select") == last, "刷新后未回到上次连接端口"


def test_refresh_keeps_selection_without_last_conn(page: Page, serialhub_server):
    """无上次连接记录或其不可用时刷新保留当前选中"""
    _goto_terminal(page, serialhub_server["url"])
    ports = [p for p in _select_options(page, "port-select") if p]
    if not ports:
        pytest.skip("当前环境无可用串口，跳过刷新保持用例")
    page.select_option("#port-select", ports[0])
    page.click("#refresh-btn")
    page.wait_for_timeout(500)
    assert page.input_value("#port-select") == ports[0], "无上次连接记录时刷新不应丢失当前选中"
    # 有记录但端口不可用：同样保留当前手选
    page.evaluate(
        "() => localStorage.setItem('serialhub-last-conn', "
        'JSON.stringify({port: "/dev/nonexistent"}))'
    )
    page.click("#refresh-btn")
    page.wait_for_timeout(500)
    assert page.input_value("#port-select") == ports[0], "上次端口不可用时刷新不应丢失当前选中"


def test_refresh_restores_params(page: Page, serialhub_server):
    """刷新列表同时把参数框恢复为上次连接值（手改后点刷新即回退）"""
    params = {"baudRate": "9600", "dataBits": "7", "parity": "even", "stopBits": "2"}
    _goto_terminal(
        page,
        serialhub_server["url"],
        init_script=_conn_init_script(json.dumps(params)),
    )
    # 四项全部手改成非上次值
    page.select_option("#baud-rate", "230400")
    page.select_option("#data-bits", "5")
    page.select_option("#parity", "odd")
    page.select_option("#stop-bits", "1")
    page.click("#refresh-btn")
    page.wait_for_timeout(200)  # 参数恢复在点击时同步生效，不依赖列表响应
    assert page.input_value("#baud-rate") == "9600", "刷新后波特率未恢复为上次连接值"
    assert page.input_value("#data-bits") == "7", "刷新后数据位未恢复为上次连接值"
    assert page.input_value("#parity") == "even", "刷新后校验位未恢复为上次连接值"
    assert page.input_value("#stop-bits") == "2", "刷新后停止位未恢复为上次连接值"


def test_connect_saves_last_conn(page: Page, serialhub_server):
    """点击连接前将端口与参数写入 localStorage（吞掉 ws connect 消息避免真实连接）"""
    base_url = serialhub_server["url"]
    page.add_init_script(
        """
        const origSend = WebSocket.prototype.send;
        WebSocket.prototype.send = function(d) {
            if (typeof d === 'string' && d.indexOf('"type":"connect"') >= 0) return true;
            return origSend.call(this, d);
        };
        """
    )
    _goto_terminal(page, base_url)
    ports = [p for p in _select_options(page, "port-select") if p]
    if not ports:
        pytest.skip("当前环境无可用串口，跳过连接保存用例")
    page.select_option("#port-select", ports[0])
    page.select_option("#baud-rate", "9600")
    page.select_option("#data-bits", "7")
    page.select_option("#parity", "even")
    page.select_option("#stop-bits", "2")
    page.click("#connect-btn")
    page.wait_for_timeout(200)
    saved = page.evaluate(
        "JSON.parse(localStorage.getItem('serialhub-last-conn') || 'null')"
    )
    assert saved, "连接时未写入 serialhub-last-conn"
    assert saved.get("port") == ports[0], "连接时未保存端口"
    assert saved.get("baudRate") == "9600", "连接时未保存波特率"
    assert saved.get("dataBits") == "7", "连接时未保存数据位"
    assert saved.get("parity") == "even", "连接时未保存校验位"
    assert saved.get("stopBits") == "2", "连接时未保存停止位"
