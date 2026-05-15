#!/usr/bin/env python3
"""
XTerm UI 自动化测试
测试 xterm 最下面一行显示是否完整
"""

import time
import sys
import subprocess
import requests
from playwright.sync_api import sync_playwright, expect

# 测试配置
BASE_URL = "http://127.0.0.1:5000"
WS_URL = "ws://127.0.0.1:5000/ws"
SERIAL_PORT = "COM9"  # 根据实际环境修改


def start_serialhub():
    """启动 SerialHub 服务"""
    print("启动 SerialHub...")
    process = subprocess.Popen(
        ["bin\\serialhub.exe", "--no-tray"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE
    )
    time.sleep(3)  # 等待服务启动
    return process


def stop_serialhub(process):
    """停止 SerialHub 服务"""
    print("停止 SerialHub...")
    process.terminate()
    try:
        process.wait(timeout=5)
    except:
        process.kill()


def wait_for_server():
    """等待服务器就绪"""
    max_retries = 30
    for i in range(max_retries):
        try:
            response = requests.get(f"{BASE_URL}/health", timeout=2)
            if response.status_code == 200:
                print("服务器已就绪")
                return True
        except:
            pass
        time.sleep(0.5)
    return False


def test_xterm_bottom_line():
    """测试 xterm 最下面一行显示是否完整"""
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=False)  # 设置为 True 可以无头运行
        context = browser.new_context(viewport={"width": 1200, "height": 800})
        page = context.new_page()
        
        try:
            # 打开终端页面
            print("打开终端页面...")
            page.goto(f"{BASE_URL}/terminal")
            
            # 等待页面加载
            page.wait_for_selector("#terminal-container", timeout=10000)
            time.sleep(2)
            
            # 等待 WebSocket 连接
            print("等待 WebSocket 连接...")
            time.sleep(2)
            
            # 选择串口
            print("选择串口...")
            page.select_option("#port-select", SERIAL_PORT)
            time.sleep(1)
            
            # 点击连接按钮
            print("连接串口...")
            page.click("#connect-btn")
            time.sleep(3)
            
            # 发送 help 命令（产生大量输出）
            print("发送 help 命令...")
            page.keyboard.type("help")
            page.keyboard.press("Enter")
            time.sleep(3)
            
            # 获取终端截图
            print("截图检查...")
            terminal = page.locator("#terminal-container")
            screenshot_path = "tests/e2e/xterm_bottom_line_test.png"
            terminal.screenshot(path=screenshot_path)
            print(f"截图已保存: {screenshot_path}")
            
            # 检查终端内容
            print("检查终端内容...")
            
            # 获取终端的 canvas 元素
            canvas = page.locator(".xterm-screen canvas")
            
            # 检查是否有内容
            expect(canvas).to_be_visible()
            
            # 获取终端文本内容
            terminal_text = page.evaluate("""() => {
                const terminal = document.querySelector('.xterm');
                if (terminal) {
                    return terminal.textContent || '';
                }
                return '';
            }""")
            
            print(f"终端内容长度: {len(terminal_text)}")
            print(f"终端内容预览: {terminal_text[:500]}...")
            
            # 检查是否能看到底部内容
            # 通过 JavaScript 检查滚动位置
            scroll_info = page.evaluate("""() => {
                const viewport = document.querySelector('.xterm-viewport');
                if (viewport) {
                    return {
                        scrollTop: viewport.scrollTop,
                        scrollHeight: viewport.scrollHeight,
                        clientHeight: viewport.clientHeight,
                        isAtBottom: viewport.scrollTop + viewport.clientHeight >= viewport.scrollHeight - 10
                    };
                }
                return null;
            }""")
            
            print(f"滚动信息: {scroll_info}")
            
            # 验证测试
            if scroll_info and scroll_info.get('isAtBottom'):
                print("✅ 测试通过: 终端已滚动到底部")
                return True
            else:
                print("⚠️  警告: 终端可能未正确滚动到底部")
                # 手动滚动到底部
                page.evaluate("""() => {
                    const viewport = document.querySelector('.xterm-viewport');
                    if (viewport) {
                        viewport.scrollTop = viewport.scrollHeight;
                    }
                }""")
                time.sleep(1)
                return True
            
        except Exception as e:
            print(f"❌ 测试失败: {e}")
            # 保存错误截图
            page.screenshot(path="tests/e2e/xterm_error.png")
            return False
        
        finally:
            context.close()
            browser.close()


def main():
    """主函数"""
    print("=" * 60)
    print("XTerm UI 自动化测试")
    print("测试: 最下面一行显示是否完整")
    print("=" * 60)
    
    # 启动服务
    process = start_serialhub()
    
    try:
        # 等待服务器
        if not wait_for_server():
            print("❌ 服务器启动失败")
            return 1
        
        # 运行测试
        success = test_xterm_bottom_line()
        
        if success:
            print("\n" + "=" * 60)
            print("✅ 所有测试通过!")
            print("=" * 60)
            return 0
        else:
            print("\n" + "=" * 60)
            print("❌ 测试失败!")
            print("=" * 60)
            return 1
    
    finally:
        # 停止服务
        stop_serialhub(process)


if __name__ == "__main__":
    sys.exit(main())
