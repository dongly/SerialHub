#!/usr/bin/env python3
"""XTerm UI Test - 测试最下面一行显示是否完整"""

import time
import subprocess
import sys
import os

# 添加项目根目录到路径
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.dirname(__file__))))

def test_xterm():
    """测试 xterm 显示"""
    from playwright.sync_api import sync_playwright
    
    # 启动 SerialHub
    print("Starting SerialHub...")
    proc = subprocess.Popen(
        ["powershell", "-ExecutionPolicy", "Bypass", "-File", "start.ps1"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        cwd="D:\\Develop\\SerialHub"
    )
    time.sleep(5)
    
    try:
        with sync_playwright() as p:
            browser = p.chromium.launch(headless=False)
            page = browser.new_page(viewport={"width": 1200, "height": 800})
            
            # 打开终端
            print("Opening terminal...")
            page.goto("http://127.0.0.1:5000/terminal")
            time.sleep(3)
            
            # 截图初始状态
            page.screenshot(path="tests/e2e/screenshot_1_initial.png")
            print("Screenshot 1: Initial state saved")
            
            # 刷新端口列表并选择串口
            print("Refreshing port list...")
            page.click("#refresh-btn")
            time.sleep(2)
            
            print("Connecting to serial port...")
            # 等待端口选择器可用
            page.wait_for_selector("#port-select:not([disabled])", timeout=10000)
            page.select_option("#port-select", "COM9")
            time.sleep(1)
            page.click("#connect-btn")
            time.sleep(3)
            
            # 截图连接后
            page.screenshot(path="tests/e2e/screenshot_2_connected.png")
            print("Screenshot 2: Connected state saved")
            
            # 发送 help 命令（产生大量输出）
            print("Sending 'help' command...")
            page.keyboard.type("help")
            time.sleep(0.5)
            page.keyboard.press("Enter")
            time.sleep(5)  # 等待输出
            
            # 截图大量输出后
            page.screenshot(path="tests/e2e/screenshot_3_after_help.png")
            print("Screenshot 3: After help command saved")
            
            # 检查终端底部是否可见
            result = page.evaluate("""() => {
                const viewport = document.querySelector('.xterm-viewport');
                const screen = document.querySelector('.xterm-screen');
                const rows = document.querySelector('.xterm-rows');
                
                if (!viewport || !screen) return { error: 'Elements not found' };
                
                const lastRow = rows ? rows.lastElementChild : null;
                const lastRowRect = lastRow ? lastRow.getBoundingClientRect() : null;
                const viewportRect = viewport.getBoundingClientRect();
                
                return {
                    viewportHeight: viewport.clientHeight,
                    scrollHeight: viewport.scrollHeight,
                    scrollTop: viewport.scrollTop,
                    isAtBottom: viewport.scrollTop + viewport.clientHeight >= viewport.scrollHeight - 10,
                    lastRowVisible: lastRowRect ? lastRowRect.bottom <= viewportRect.bottom + 5 : false,
                    screenPaddingBottom: screen.style.paddingBottom,
                    rowsPaddingBottom: rows ? rows.style.paddingBottom : 'not found'
                };
            }""")
            
            print(f"Check result: {result}")
            
            # 最终截图
            page.screenshot(path="tests/e2e/screenshot_4_final.png")
            print("Screenshot 4: Final state saved")
            
            browser.close()
            
            # 验证结果
            if result.get('isAtBottom') and result.get('lastRowVisible'):
                print("\n✅ TEST PASSED: Bottom row is visible!")
                return True
            else:
                print("\n❌ TEST FAILED: Bottom row may be cut off!")
                return False
                
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except:
            proc.kill()

if __name__ == "__main__":
    success = test_xterm()
    sys.exit(0 if success else 1)
