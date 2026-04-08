"""SerialHub MCP 工具测试

迁移自 test_serialhub.py 的 TestMCPTools 类。
"""

import requests

from conftest import mcp_call, _get_content_text


class TestMCPTools:
    """MCP 工具测试类"""

    def test_serial_list(self, serialhub_server):
        """测试列出可用串口"""
        info = serialhub_server
        result = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_list"})
        assert "result" in result
        content = result["result"].get("content", [])
        assert len(content) > 0
        text = _get_content_text(result).lower()
        assert "串口" in text or "serial" in text or "port" in text or "找到" in text

    def test_serial_status_response_format(self, serialhub_server):
        """测试串口状态查询响应格式"""
        info = serialhub_server
        result = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_status"})
        text = _get_content_text(result)
        assert "connected" in text.lower() or "连接" in text or "状态" in text
        assert "port" in text.lower() or "串口" in text

    def test_serial_connect_response(self, serialhub_server):
        """测试连接串口响应"""
        info = serialhub_server
        result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_connect",
                "arguments": {"port": "INVALID_99999"},
            },
        )
        text = _get_content_text(result).lower()
        assert "连接" in text or "connect" in text or "port" in text or "串口" in text

    def test_serial_disconnect_response(self, serialhub_server):
        """测试断开串口响应"""
        info = serialhub_server
        result = mcp_call(info["mcp_port"], "tools/call", {"name": "serial_disconnect"})
        text = _get_content_text(result)
        assert "断开" in text or "disconnect" in text.lower() or "port" in text.lower()

    def test_serial_write_response(self, serialhub_server):
        """测试写入串口响应"""
        info = serialhub_server
        result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_write",
                "arguments": {"data": "test"},
            },
        )
        text = _get_content_text(result)
        assert (
            "写入" in text
            or "write" in text.lower()
            or "字节" in text
            or "bytes" in text.lower()
        )

    def test_serial_read_not_connected(self, serialhub_server):
        """测试未连接时读取"""
        info = serialhub_server
        result = mcp_call(
            info["mcp_port"],
            "tools/call",
            {
                "name": "serial_read",
                "arguments": {"timeout": 100},
            },
        )
        text = _get_content_text(result)
        assert "未连接" in text or "超时" in text or "timedOut" in text

    def test_mcp_health_endpoint(self, serialhub_server):
        """测试 MCP 健康检查端点"""
        info = serialhub_server
        resp = requests.get(f"http://127.0.0.1:{info['mcp_port']}/health", timeout=5)
        assert resp.status_code == 200
        assert resp.json().get("status") == "ok"

    def test_mcp_unknown_tool(self, serialhub_server):
        """测试未知工具调用"""
        info = serialhub_server
        result = mcp_call(info["mcp_port"], "tools/call", {"name": "nonexistent_tool"})
        assert "error" in result
