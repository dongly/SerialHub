#!/usr/bin/env python3
"""XTerm UI Test for COM7"""

import time
import subprocess
import sys
import os

def test_xterm():
    from playwright.sync_api import sync_playwright
    
    # Start SerialHub using start.ps1
    print("Starting SerialHub with start.ps1...")
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
            
            print("Opening terminal...")
            page.goto("http://127.0.0.1:5000/terminal")
            time.sleep(3)
            
            # Take screenshot
            page.screenshot(path="tests/e2e/xterm_com7_initial.png")
            print("Screenshot saved: xterm_com7_initial.png")
            
            # Wait for connection (COM7 should auto-connect via start.ps1)
            print("Waiting for connection...")
            time.sleep(3)
            
            # Check if connected
            indicator = page.locator("#status-indicator")
            class_attr = indicator.get_attribute("class")
            print(f"Status indicator class: {class_attr}")
            
            if "connected" in str(class_attr):
                print("Connected to COM7!")
                
                # Send help command
                print("Sending 'help' command...")
                page.keyboard.type("help")
                page.keyboard.press("Enter")
                time.sleep(5)
                
                # Screenshot after help
                page.screenshot(path="tests/e2e/xterm_com7_after_help.png")
                print("Screenshot saved: xterm_com7_after_help.png")
                
                # Check bottom row
                result = page.evaluate("""() => {
                    const viewport = document.querySelector('.xterm-viewport');
                    const rows = document.querySelector('.xterm-rows');
                    const lastRow = rows ? rows.lastElementChild : null;
                    
                    if (!viewport || !lastRow) return { error: 'Elements not found' };
                    
                    const viewportRect = viewport.getBoundingClientRect();
                    const lastRowRect = lastRow.getBoundingClientRect();
                    
                    return {
                        viewportHeight: viewport.clientHeight,
                        scrollHeight: viewport.scrollHeight,
                        scrollTop: viewport.scrollTop,
                        lastRowBottom: lastRowRect.bottom,
                        viewportBottom: viewportRect.bottom,
                        isVisible: lastRowRect.bottom <= viewportRect.bottom + 5
                    };
                }""")
                
                print(f"Check result: {result}")
                
                if result.get('isVisible'):
                    print("\n[PASS] TEST PASSED: Bottom row is visible!")
                else:
                    print("\n[FAIL] TEST FAILED: Bottom row is cut off!")
            else:
                print("Not connected. Check if COM7 is available.")
            
            browser.close()
            
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except:
            proc.kill()

if __name__ == "__main__":
    test_xterm()
